//go:build linux && !android

package core

// wayland_popup_linux.go — меню и выпадающие списки за пределами окна
// (xdg_popup).
//
// До этого на Wayland оверлеи рисовались ВНУТРИ окна: холст движка — это и
// есть окно, и всё, что не помещалось, обрезалось его краем. В узком окне
// (Блокнот WinLine, эталон 490 px) выпадающее меню «Файл» и контекстное меню
// теряли правый край и нижние пункты. На Win32 и X11 этой беды нет: там
// popupHost выносит оверлей в отдельное окно ОС по экранным координатам.
//
// На Wayland такого пути нет вовсе: клиент не знает, где стоит его окно, и
// поставить второе окно в точку экрана не может. Для этого случая в протоколе
// есть xdg_popup — окно-потомок, чьё место задаётся ОТНОСИТЕЛЬНО родителя, а
// окончательное решение принимает компоновщик: он один знает, где край
// экрана, и сам сдвинет или перевернёт попап, чтобы тот влез.
//
// Отсюда и устройство: попапы живут не отдельными окнами, а поверхностями
// ЭТОГО соединения (xdg_popup требует родителя в том же клиенте), и хост им
// нужен другой — childPopups (popup_host_child.go).

import (
	"encoding/binary"
	"fmt"
	"image"
	"sync"

	"github.com/oops1/headless-gui/v3/engine"

	"golang.org/x/sys/unix"
)

const (
	// wl_compositor.create_region и wl_region — ими задаётся область,
	// которой попап ловит клики.
	wlCompositorCreateRegion = 1
	wlRegionDestroy          = 0
	wlRegionAdd              = 1

	// wl_surface
	wlSurfaceDestroy         = 0
	wlSurfaceSetOpaqueRegion = 4
	wlSurfaceSetInputRegion  = 5

	// xdg_surface
	xdgSurfaceDestroy = 0
)

// wlPopupBufs — сколько кадровых буферов у попапа. Два, как у окна: пока
// компоновщик читает один, кадр пишется во второй.
const wlPopupBufs = 2

// wlPopup — одно всплывающее окно: поверхность, её xdg-обвязка и два
// кадровых буфера.
type wlPopup struct {
	id uintptr // идентификатор оверлея движка

	surfaceID uint32
	xdgSurfID uint32
	popupID   uint32

	// Буферы кадра: своя память, свой пул — размер попапа меняется отдельно
	// от окна (раскрылось подменю). Буфер i лежит в пуле со смещения
	// i × stride × h.
	poolID     uint32
	bufID      [wlPopupBufs]uint32
	viewportID uint32 // область просмотра на HiDPI (0 — масштаб единица)
	fd         int
	data       []byte
	w, h       int
	stride     int
	busy       [wlPopupBufs]bool // буфер у компоновщика: прикреплён, release ещё не пришёл

	configured bool
	closed     bool

	// pending — последний кадр, ещё не положенный в буфер: пришёл до первого
	// configure (прикреплять буфер до ответа на первый commit нельзя) или
	// когда оба буфера были у компоновщика. Кладётся по configure или по
	// wl_buffer.release; новый кадр заменяет придержанный — показывать имеет
	// смысл только последний.
	pending      *image.RGBA
	pendingBands []image.Rectangle

	// drawMu — вывод в буферы по одному за раз: кадр кладут и горутина
	// движка, и цикл событий (по configure и release), и коммиты не должны
	// обгонять друг друга. Под ним же popupDestroy снимает память: блит не
	// пишет в уже снятое отображение.
	drawMu sync.Mutex

	// rect — место попапа в координатах окна-носителя (физические пиксели).
	// По нему считается положение дочерних попапов.
	rect image.Rectangle
}

// wlPopups — всплывающие окна одного окна-носителя.
type wlPopups struct {
	mu     sync.Mutex
	byID   map[uintptr]*wlPopup
	bySurf map[uint32]*wlPopup
	byXdg  map[uint32]*wlPopup
	byPop  map[uint32]*wlPopup
	byBuf  map[uint32]*wlPopup

	// root — попап с захватом ввода. Пока он жив, остальные попапы по
	// протоколу обязаны быть его потомками, иначе компоновщик отвечает
	// протокольной ошибкой, а это разрыв соединения.
	root *wlPopup

	handlers childPopupHandlers
}

// ─── Интерфейс childPopupHost ────────────────────────────────────────────────

// SetChildPopupHandlers подключает приёмники ввода попапов.
func (w *WaylandWindow) SetChildPopupHandlers(h childPopupHandlers) {
	w.popups.mu.Lock()
	w.popups.handlers = h
	w.popups.mu.Unlock()
}

// OpenChildPopup поднимает попап под оверлей id: (x, y) — его левый верхний
// угол в координатах окна-носителя, (pw, ph) — размер. Всё в физических
// пикселях.
func (w *WaylandWindow) OpenChildPopup(id uintptr, x, y, pw, ph int) error {
	if w.closed.Load() || w.xdgSurfaceID == 0 || w.compositorID == 0 || w.wmBaseID == 0 {
		return fmt.Errorf("wayland: окно не готово для попапа")
	}
	if pw <= 0 || ph <= 0 {
		return fmt.Errorf("wayland: размер попапа %dx%d", pw, ph)
	}

	p := &wlPopup{id: id, rect: image.Rect(x, y, x+pw, y+ph)}
	if err := w.popupSetupBuffer(p, pw, ph); err != nil {
		return err
	}

	// Родитель: пока жив попап с захватом ввода, новые попапы обязаны быть
	// его потомками — так требует протокол. Якорь тогда считается в его
	// координатах.
	w.popups.mu.Lock()
	root := w.popups.root
	w.popups.mu.Unlock()

	parentXdg := w.xdgSurfaceID
	ax, ay := x, y
	if root != nil && !root.closed {
		parentXdg = root.xdgSurfID
		ax, ay = x-root.rect.Min.X, y-root.rect.Min.Y
	}

	p.surfaceID = w.newID()
	w.send(newWlMsg(w.compositorID, wlCompositorCreateSurface).putUint(p.surfaceID), -1)
	w.popupApplyScale(p, pw, ph)
	p.xdgSurfID = w.newID()
	w.send(newWlMsg(w.wmBaseID, xdgWmBaseGetXdgSurface).putUint(p.xdgSurfID).putUint(p.surfaceID), -1)

	// Позиционер считает в ПОВЕРХНОСТНЫХ единицах, а размеры здесь — в
	// пикселях буфера: на HiDPI это разные числа.
	posID := w.newID()
	w.send(newWlMsg(w.wmBaseID, xdgWmBaseCreatePositioner).putUint(posID), -1)
	for _, raw := range wlPositionerRequests(posID,
		w.toSurface(ax), w.toSurface(ay), w.toSurface(pw), w.toSurface(ph)) {
		w.sendRaw(raw)
	}

	p.popupID = w.newID()
	w.send(newWlMsg(p.xdgSurfID, xdgSurfaceGetPopup).
		putUint(p.popupID).putUint(parentXdg).putUint(posID), -1)
	w.send(newWlMsg(posID, xdgPositionerDestroy), -1)

	// Захват ввода — только у первого попапа. С ним компоновщик отдаёт
	// попапу весь ввод и сам закрывает его по щелчку снаружи: так ведут
	// себя системные меню.
	if root == nil || root.closed {
		if serial := w.inputSerial.Load(); serial != 0 && w.seatID != 0 {
			w.send(newWlMsg(p.popupID, xdgPopupGrab).putUint(w.seatID).putUint(serial), -1)
		}
	}

	w.send(newWlMsg(p.xdgSurfID, xdgSurfaceSetWindowGeometry).
		putInt(0).putInt(0).
		putInt(int32(w.toSurface(pw))).putInt(int32(w.toSurface(ph))), -1)
	// Первый commit — без буфера: компоновщик ответит на него configure, и
	// только после ack можно показывать кадр.
	w.send(newWlMsg(p.surfaceID, wlSurfaceCommit), -1)

	w.popups.mu.Lock()
	w.popups.put(p)
	if w.popups.root == nil || w.popups.root.closed {
		w.popups.root = p
	}
	w.popups.mu.Unlock()
	return nil
}

// MoveChildPopup переносит попап и меняет его размер.
//
// Отдельного «переехать» у xdg_popup версии 1 нет (reposition появился в
// третьей, и компоновщик WinLine её не обещает), поэтому попап
// пересоздаётся — для меню это незаметно: оно и так перерисовывается целиком,
// когда раскрывается подменю.
func (w *WaylandWindow) MoveChildPopup(id uintptr, x, y, pw, ph int) {
	w.CloseChildPopup(id)
	if err := w.OpenChildPopup(id, x, y, pw, ph); err != nil {
		wlLog("перенос попапа: %v", err)
	}
}

// BlitChildPopup кладёт кадр в попап. bands — закрашенные полосы картинки
// (engine.OpaqueBands): ими задаётся область, которой попап ловит клики, —
// в дырах каскадного меню щелчок должен уходить мимо, то есть закрывать меню.
func (w *WaylandWindow) BlitChildPopup(id uintptr, img *image.RGBA, bands []image.Rectangle) {
	if img == nil {
		return
	}
	w.popups.mu.Lock()
	p := w.popups.byID[id]
	if p == nil || p.closed {
		w.popups.mu.Unlock()
		return
	}
	// Кадр придерживается всегда, а кладёт его popupFlush: до первого
	// configure — когда компоновщик ответит, при занятых буферах — когда
	// один из них освободится.
	p.pending, p.pendingBands = img, bands
	ready := p.configured
	w.popups.mu.Unlock()
	if ready {
		w.popupFlush(p)
	}
}

// CloseChildPopup убирает попап и освобождает его ресурсы.
func (w *WaylandWindow) CloseChildPopup(id uintptr) {
	w.popups.mu.Lock()
	p := w.popups.byID[id]
	if p == nil {
		w.popups.mu.Unlock()
		return
	}
	p.closed = true
	w.popups.drop(p)
	w.popups.mu.Unlock()
	w.popupDestroy(p)
}

// closeAllPopups убирает все попапы (окно закрывается).
func (w *WaylandWindow) closeAllPopups() {
	w.popups.mu.Lock()
	all := make([]*wlPopup, 0, len(w.popups.byID))
	for _, p := range w.popups.byID {
		p.closed = true
		all = append(all, p)
	}
	w.popups.byID = nil
	w.popups.bySurf = nil
	w.popups.byXdg = nil
	w.popups.byPop = nil
	w.popups.byBuf = nil
	w.popups.root = nil
	w.popups.mu.Unlock()
	for _, p := range all {
		w.popupDestroy(p)
	}
}

// ─── Реестр попапов ──────────────────────────────────────────────────────────

func (ps *wlPopups) put(p *wlPopup) {
	if ps.byID == nil {
		ps.byID = map[uintptr]*wlPopup{}
		ps.bySurf = map[uint32]*wlPopup{}
		ps.byXdg = map[uint32]*wlPopup{}
		ps.byPop = map[uint32]*wlPopup{}
		ps.byBuf = map[uint32]*wlPopup{}
	}
	ps.byID[p.id] = p
	ps.bySurf[p.surfaceID] = p
	ps.byXdg[p.xdgSurfID] = p
	ps.byPop[p.popupID] = p
	for _, b := range p.bufID {
		if b != 0 {
			ps.byBuf[b] = p
		}
	}
}

func (ps *wlPopups) drop(p *wlPopup) {
	delete(ps.byID, p.id)
	delete(ps.bySurf, p.surfaceID)
	delete(ps.byXdg, p.xdgSurfID)
	delete(ps.byPop, p.popupID)
	for _, b := range p.bufID {
		delete(ps.byBuf, b)
	}
	if ps.root == p {
		ps.root = nil
	}
}

// ─── Буфер и отрисовка ───────────────────────────────────────────────────────

// popupSetupBuffer выделяет память под кадры попапа и регистрирует буферы.
//
// Формат с альфой, а не XRGB как у окна: вынесенное каскадное меню — это
// ступенька, и между её полосами площадь, которую никто не закрашивает. В
// непрозрачном буфере она была бы чёрным прямоугольником.
func (w *WaylandWindow) popupSetupBuffer(p *wlPopup, pw, ph int) error {
	stride := pw * 4
	frame := stride * ph
	size := frame * wlPopupBufs
	fd, err := unix.MemfdCreate("headless-gui-popup", unix.MFD_CLOEXEC)
	if err != nil {
		return fmt.Errorf("wayland: memfd_create попапа: %w", err)
	}
	if err := unix.Ftruncate(fd, int64(size)); err != nil {
		unix.Close(fd)
		return fmt.Errorf("wayland: ftruncate попапа: %w", err)
	}
	data, err := unix.Mmap(fd, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		unix.Close(fd)
		return fmt.Errorf("wayland: mmap попапа: %w", err)
	}
	p.fd, p.data, p.stride, p.w, p.h = fd, data, stride, pw, ph

	p.poolID = w.newID()
	w.send(newWlMsg(w.shmID, wlShmCreatePool).putUint(p.poolID).putInt(int32(size)), fd)
	for i := range p.bufID {
		p.bufID[i] = w.newID()
		w.send(newWlMsg(p.poolID, wlShmPoolCreateBuffer).
			putUint(p.bufID[i]).putInt(int32(i*frame)).
			putInt(int32(pw)).putInt(int32(ph)).putInt(int32(stride)).
			putUint(wlShmFormatARGB8888), -1)
	}
	return nil
}

// popupFlush кладёт придержанный кадр попапа в свободный буфер и показывает
// его.
//
// Оба буфера у компоновщика — кадр остаётся ждать, его положит
// wl_buffer.release. Ни ждать, ни пропускать нельзя: зовут и с горутины
// движка, и из цикла событий, который release и разбирает, а хост шлёт кадры
// только при изменениях — потерянный последний кадр (шаг перехода, подсветка
// пункта) не повторился бы, и попап застыл бы на промежуточном.
func (w *WaylandWindow) popupFlush(p *wlPopup) {
	p.drawMu.Lock()
	defer p.drawMu.Unlock()

	w.popups.mu.Lock()
	img, bands := p.pending, p.pendingBands
	if img == nil || p.closed || !p.configured || p.data == nil {
		w.popups.mu.Unlock()
		return
	}
	if b := img.Bounds(); b.Dx() != p.w || b.Dy() != p.h {
		// Размер сменился — хост пересоздаст попап (MoveChildPopup).
		p.pending, p.pendingBands = nil, nil
		w.popups.mu.Unlock()
		return
	}
	i := -1
	for k, busy := range p.busy {
		if !busy {
			i = k
			break
		}
	}
	if i < 0 {
		w.popups.mu.Unlock()
		return // оба буфера у компоновщика — кадр положит release
	}
	p.pending, p.pendingBands = nil, nil
	p.busy[i] = true
	w.popups.mu.Unlock()

	frame := p.stride * p.h
	convRectBGRAPremul(p.data[i*frame:(i+1)*frame], p.stride, img.Pix, img.Stride, img.Bounds())

	w.popupSetRegions(p, bands, engine.SolidBands(img))
	w.send(newWlMsg(p.surfaceID, wlSurfaceAttach).putUint(p.bufID[i]).putInt(0).putInt(0), -1)
	w.popupDamage(p)
	w.send(newWlMsg(p.surfaceID, wlSurfaceCommit), -1)
}

// popupSetRegions задаёт попапу область ввода и непрозрачную область.
//
// bands — закрашенные полосы картинки. У каскадного меню между полосами есть
// дыра: щелчок в неё не должен попадать в меню, он должен считаться щелчком
// СНАРУЖИ — и тогда компоновщик закроет попап сам, как закрывает системные
// меню. Непрозрачная область — подсказка компоновщику: под ней ничего
// рисовать не нужно.
//
// Нет полос (сплошная картинка) — обе области возвращаются к значению по
// умолчанию: вся поверхность.
//
// solid — точки с альфой 255 (engine.SolidBands). Непрозрачной областью
// объявляются только они: полосы bands включают и полупрозрачную тень, а
// компоновщик, поверивший непрозрачной области, копирует её без смешивания —
// вокруг меню выходила сплошная чёрная рамка.
func (w *WaylandWindow) popupSetRegions(p *wlPopup, bands, solid []image.Rectangle) {
	if len(bands) == 0 {
		w.send(newWlMsg(p.surfaceID, wlSurfaceSetInputRegion).putUint(0), -1)
		w.send(newWlMsg(p.surfaceID, wlSurfaceSetOpaqueRegion).putUint(0), -1)
		return
	}
	for _, reg := range []struct {
		op    int
		rects []image.Rectangle
	}{{wlSurfaceSetInputRegion, bands}, {wlSurfaceSetOpaqueRegion, solid}} {
		op := reg.op
		if len(reg.rects) == 0 {
			w.send(newWlMsg(p.surfaceID, uint16(op)).putUint(0), -1)
			continue
		}
		rid := w.newID()
		w.send(newWlMsg(w.compositorID, wlCompositorCreateRegion).putUint(rid), -1)
		for _, r := range reg.rects {
			// Область — в поверхностных единицах, полосы пришли в пикселях.
			w.send(newWlMsg(rid, wlRegionAdd).
				putInt(int32(w.toSurface(r.Min.X))).putInt(int32(w.toSurface(r.Min.Y))).
				putInt(int32(w.toSurface(r.Dx()))).putInt(int32(w.toSurface(r.Dy()))), -1)
		}
		w.send(newWlMsg(p.surfaceID, uint16(op)).putUint(rid), -1)
		w.send(newWlMsg(rid, wlRegionDestroy), -1)
	}
}

// popupDestroy уничтожает объекты попапа и освобождает его память.
//
// Порядок обратный созданию: сначала попап, потом xdg_surface, потом сама
// поверхность — и только затем буферы с памятью, из которой компоновщик ещё
// мог читать.
func (w *WaylandWindow) popupDestroy(p *wlPopup) {
	// Блит, начатый до закрытия, дописывает кадр, и лишь потом снимается
	// память, в которую он пишет.
	p.drawMu.Lock()
	defer p.drawMu.Unlock()
	if p.popupID != 0 {
		w.send(newWlMsg(p.popupID, xdgPopupDestroy), -1)
	}
	if p.xdgSurfID != 0 {
		w.send(newWlMsg(p.xdgSurfID, xdgSurfaceDestroy), -1)
	}
	if p.surfaceID != 0 {
		w.send(newWlMsg(p.surfaceID, wlSurfaceDestroy), -1)
	}
	if p.viewportID != 0 {
		w.send(newWlMsg(p.viewportID, wlSurfaceDestroy), -1) // wp_viewport.destroy — тоже 0
	}
	for _, b := range p.bufID {
		if b != 0 {
			w.send(newWlMsg(b, wlBufferDestroy), -1)
		}
	}
	if p.poolID != 0 {
		w.send(newWlMsg(p.poolID, wlShmPoolDestroy), -1)
	}
	if p.data != nil {
		unix.Munmap(p.data)
		p.data = nil
	}
	if p.fd > 0 {
		unix.Close(p.fd)
		p.fd = 0
	}
}

// ─── События ─────────────────────────────────────────────────────────────────

// popupEvent разбирает события, адресованные объектам попапов. Возвращает
// false, если объект не наш — тогда событие разбирает общий диспетчер.
func (w *WaylandWindow) popupEvent(obj uint32, opcode uint16, b []byte) bool {
	w.popups.mu.Lock()
	xdg := w.popups.byXdg[obj]
	pop := w.popups.byPop[obj]
	buf := w.popups.byBuf[obj]
	w.popups.mu.Unlock()

	switch {
	case xdg != nil && opcode == xdgSurfaceEvConfigure:
		serial := uint32(0)
		if len(b) >= 4 {
			serial = binary.LittleEndian.Uint32(b[0:4])
		}
		w.send(newWlMsg(xdg.xdgSurfID, xdgSurfaceAckConfigure).putUint(serial), -1)
		w.popups.mu.Lock()
		xdg.configured = true
		w.popups.mu.Unlock()
		w.popupFlush(xdg) // кадр, пришедший до configure
		return true

	case pop != nil && opcode == xdgPopupEvConfigure:
		// Компоновщик мог сдвинуть или перевернуть попап, чтобы тот влез на
		// экран. Для ввода это неважно — клики приходят в координатах
		// поверхности, и движку они переводятся по месту, куда он САМ
		// нарисовал оверлей. Записываем в журнал и идём дальше.
		if x, y, pw, ph, ok := wlParsePopupConfigure(b); ok {
			wlLog("popup.configure: %d,%d %dx%d", x, y, pw, ph)
		}
		return true

	case pop != nil && opcode == xdgPopupEvPopupDone:
		// Щелчок снаружи или Escape у компоновщика: меню закрывает он сам,
		// а движку нужно узнать об этом и убрать свой оверлей — иначе он
		// останется открытым в дереве виджетов.
		w.popups.mu.Lock()
		done := w.popups.handlers.Done
		id := pop.id
		w.popups.mu.Unlock()
		if done != nil {
			done(id)
		}
		return true

	case buf != nil && opcode == wlBufferEvRelease:
		w.popups.mu.Lock()
		for i, id := range buf.bufID {
			if id == obj {
				buf.busy[i] = false
			}
		}
		w.popups.mu.Unlock()
		w.popupFlush(buf) // кадр, ждавший свободного буфера
		return true
	}
	return false
}

// popupForSurface возвращает попап, которому принадлежит поверхность.
func (w *WaylandWindow) popupForSurface(surf uint32) *wlPopup {
	w.popups.mu.Lock()
	defer w.popups.mu.Unlock()
	return w.popups.bySurf[surf]
}

// popupHandlers — снимок приёмников ввода попапов.
func (w *WaylandWindow) popupHandlers() childPopupHandlers {
	w.popups.mu.Lock()
	defer w.popups.mu.Unlock()
	return w.popups.handlers
}

// popupApplyScale готовит поверхность попапа к HiDPI.
//
// Кадр попапа движок рисует в пикселях, а компоновщик размещает окно в
// поверхностных единицах. При дробном масштабе их связывает область
// просмотра, при целом хватает буферного масштаба.
func (w *WaylandWindow) popupApplyScale(p *wlPopup, pw, ph int) {
	k := w.scaleFactor()
	if k == 1 {
		return
	}
	if w.viewporterID != 0 {
		p.viewportID = w.newID()
		w.send(newWlMsg(w.viewporterID, wpViewporterGetViewport).
			putUint(p.viewportID).putUint(p.surfaceID), -1)
		w.send(newWlMsg(p.viewportID, wpViewportSetDestination).
			putInt(int32(w.toSurface(pw))).putInt(int32(w.toSurface(ph))), -1)
		return
	}
	if k == float64(int(k)) {
		w.send(newWlMsg(p.surfaceID, wlSurfaceSetBufferScale).putInt(int32(k)), -1)
	}
}

// popupDamage помечает весь кадр попапа изменившимся — в тех единицах,
// которые понимает компоновщик (см. damageSurface).
func (w *WaylandWindow) popupDamage(p *wlPopup) {
	if w.scaleFactor() != 1 && w.gCompositorVer >= wlCompositorVersionBufferDamage {
		w.send(newWlMsg(p.surfaceID, wlSurfaceDamageBuffer).
			putInt(0).putInt(0).putInt(int32(p.w)).putInt(int32(p.h)), -1)
		return
	}
	w.send(newWlMsg(p.surfaceID, wlSurfaceDamage).
		putInt(0).putInt(0).
		putInt(int32(w.toSurface(p.w))).putInt(int32(w.toSurface(p.h))), -1)
}
