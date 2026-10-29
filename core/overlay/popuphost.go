// popuphost.go — вынос popup-оверлеев (dropdown, контекстные/каскадные меню)
// в отдельные нативные окна ОС («нативный вьюпорт»).
//
// Логика попапа (hover, выбор, закрытие) остаётся в этом движке: нативное
// окно-попап — лишь вьюпорт. Движок рендерит содержимое оверлея в отдельный
// маленький буфер (RenderOverlay через транслирующий DrawContext) и отдаёт его
// хосту (window.popupHost) через PopupSink. Хост создаёт/двигает/закрывает
// окна ОС и транслирует их события мыши обратно в этот движок.
//
// Без установленного sink (headless и бэкенды без поддержки окон-попапов)
// поведение прежнее до пикселя: оверлеи рисуются в основной холст.

package overlay

import (
	"hash/fnv"
	"image"
	"reflect"
	"slices"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/core/input"
	"github.com/oops1/headless-gui/v3/core/raster"
	"github.com/oops1/headless-gui/v3/widget"
)

// PopupFrame — кадр одного активного оверлея для нативного окна-попапа.
type PopupFrame struct {
	ID   uintptr         // стабильный ключ (указатель на виджет-оверлей)
	Rect image.Rectangle // абсолютные ЛОГИЧЕСКИЕ координаты холста (могут выходить за холст)
	Img  *image.RGBA     // отрендеренный контент (ФИЗИЧЕСКИЕ пиксели, Rect.Size()×scale)
	// Transition — оверлей появляется или исчезает (SetOverlayTransitions
	// при хосте, объявившем SetPopupFade). nil — в покое. Img на шагах
	// перехода тот же: меняется только окно.
	Transition *PopupTransition
}

// PopupTransition — шаг перехода окна-попапа.
type PopupTransition struct {
	// Opacity — непрозрачность окна целиком, 0…1. Img — в покое,
	// непрозрачность прикладывает хост (прозрачность окна ОС или умножение
	// копии кадра).
	Opacity float64
	// Offset — сдвиг окна от Rect в логических точках (выезд от точки
	// привязки).
	Offset image.Point
	// Closing — оверлей уже закрыт: окно держится до конца перехода (пока
	// ID в кадрах) и ввода не принимает. Хост, не умеющий прозрачности,
	// закрывает его сразу.
	Closing bool
}

// SetPopupFade сообщает, что хост попапов понимает PopupFrame.Transition:
// держит окно исчезающего оверлея, пока его ID в кадрах, и прикладывает
// непрозрачность и сдвиг. По умолчанию — нет: хостовые попапы появляются и
// исчезают сразу, как прежде. Переходы включаются ещё и
// SetOverlayTransitions — общим выключателем всех оверлеев.
func (m *Manager) SetPopupFade(on bool) { m.popupFade.Store(on) }

// PopupFade сообщает, объявил ли хост попапов переходы.
func (m *Manager) PopupFade() bool { return m.popupFade.Load() }

// popupSig — сигнатура одного попапа для детекта изменений (состав/Rect/контент).
type popupSig struct {
	id   uintptr
	rect image.Rectangle
	hash uint64
	// Шаг перехода: непрозрачность (в тысячных), сдвиг, закрыт ли.
	op      int
	off     image.Point
	closing bool
	trans   bool
}

// PopupItem — найденный в дереве хостируемый оверлей (до рендера).
type PopupItem struct {
	ID   uintptr
	Rect image.Rectangle // ЭКРАННЫЕ координаты (границы оверлея минус Off)
	OD   widget.OverlayDrawer
	// Off — кадр виджета-владельца (core/input, frames.go): оверлей внутри
	// прокрутки рисуется в координатах содержимого, а окно-попап стоит на
	// экране.
	Off image.Point
}

// popupEntry — межкадровый кэш одного оверлея: готовая картинка и её хэш.
// Холста у записи нет: все попапы рендерятся в один холст-заготовку арены
// (Arena, ADR-0009).
type popupEntry struct {
	rect image.Rectangle
	img  *image.RGBA
	hash uint64
	seen uint64

	// Переход (popuptrans.go): trans — идёт, closing — оверлей уже закрыт,
	// slide — сдвиг при доле 0 (логический). Доля и признак конца пишет тик.
	trans   bool
	closing bool
	self    bool // анимирует себя сам (widget.SelfAnimatedOverlay)
	slide   image.Point
	tv      atomic.Uint64
	tdone   atomic.Bool
	anim    *widget.Animation
}

// SetPopupSink регистрирует хост popup-оверлеев. sink вызывается из рендер-цикла
// со всеми активными оверлеями текущего кадра (пустой slice — закрыть все окна).
// nil снимает хост (оверлеи снова рисуются в холст).
//
// Дополнительно выставляет глобальный флаг widget.SetPopupsHosted — при нём
// popup-меню не клэмпятся в границы канваса (экранным позиционированием
// занимается хост).
func (m *Manager) SetPopupSink(sink func(frames []PopupFrame)) {
	m.popupMu.Lock()
	m.popupSink = sink
	m.lastPopupSig = nil
	for _, ent := range m.popupCache {
		ent.stopTrans()
	}
	m.popupCache = nil
	m.popupMu.Unlock()
	// Заготовку арены трогает только кадр (под frameMu): без хоста её
	// отпускает ближайший кадр (renderFrame), здесь замка кадра не берём —
	// хост вправе снять сток из своего колбэка, а тот идёт внутри кадра.
	widget.SetPopupsHosted(sink != nil)
}

// PopupSink — установленный хост попапов (nil — оверлеи рисуются в холст).
func (m *Manager) PopupSink() func([]PopupFrame) {
	m.popupMu.Lock()
	defer m.popupMu.Unlock()
	return m.popupSink
}

// RenderPopups отдаёт sink изменившиеся оверлеи (под frameMu).
// Перерисовывается только оверлей с новым Rect или в damage.
func (m *Manager) RenderPopups(canvas *raster.Canvas, root widget.Widget, modals []widget.ModalWidget,
	damage image.Rectangle, dirtyAll bool) {
	sink := m.PopupSink()
	if sink == nil {
		return
	}

	items := m.popupItems[:0]
	if root != nil {
		items = CollectPopups(items, root, 0)
	}
	for _, m := range modals {
		if m.IsModal() {
			items = CollectPopups(items, m, 0)
		}
	}
	m.popupItems = items

	sig := m.popupSigBu[:0]

	m.popupMu.Lock()
	if m.popupCache == nil {
		m.popupCache = make(map[uintptr]*popupEntry)
	}
	m.popupGen++
	gen := m.popupGen
	tr := m.popupTransSpec(canvas)
	for _, it := range items {
		ent := m.popupCache[it.ID]
		fresh := ent == nil
		if fresh {
			ent = &popupEntry{}
			m.popupCache[it.ID] = ent
		}
		ent.seen = gen
		if ent.img == nil || ent.rect != it.Rect || dirtyAll || canvas.PhysRect(it.Rect).Overlaps(damage) {
			oc := m.popupArena.CanvasFor(canvas, it.Rect)
			ent.img = RenderOverlayShifted(oc, it.OD, it.Rect, it.Off)
			ent.hash = hashRGBA(ent.img)
			ent.rect = it.Rect
		}
		// Панель, анимирующая себя сама (desktop.Flyout), перехода движка не
		// получает ни при появлении, ни при исчезновении.
		ent.self = isSelfAnimated(it.OD)
		itr := tr
		if ent.self {
			itr = popupTrans{}
		}
		m.popupOpenStep(ent, it, fresh, itr)
		sig = append(sig, ent.sig(it.ID))
	}
	// Закрытые оверлеи: исчезают переходом, пока он идёт, — с последней
	// картинкой и на прежнем месте; без перехода уходят сразу. Переходом —
	// только тот, чей владелец ещё в дереве и погасил оверлей сам: ушедший
	// из дерева (или из закрытого модального окна) уносит его сразу, как в
	// холсте. Порядок — по ID: кадры и подпись не должны зависеть от
	// порядка обхода карты.
	closing := m.popupClosingBu[:0]
	var owners map[uintptr]bool
	for id, ent := range m.popupCache {
		if ent.seen == gen {
			continue
		}
		if !ent.closing && tr.dur > 0 && !ent.self {
			if owners == nil {
				owners = m.popupOwners(root, modals)
			}
			if !owners[id] {
				ent.stopTrans()
				delete(m.popupCache, id)
				continue
			}
		}
		itr := tr
		if ent.self {
			itr = popupTrans{}
		}
		if m.popupCloseStep(ent, itr) {
			ent.seen = gen
			closing = append(closing, id)
			continue
		}
		ent.stopTrans()
		delete(m.popupCache, id)
	}
	slices.Sort(closing)
	for _, id := range closing {
		sig = append(sig, m.popupCache[id].sig(id))
	}
	m.popupClosingBu = closing
	m.popupSigBu = sig

	// Арена живёт, пока есть открытые попапы (исчезающим она не нужна: у них
	// готовая картинка), и держит размер самого большого из них — закрытое
	// большое меню не оставляет её раздутой.
	m.popupArena.trim(items)
	m.popupBytes.Store(uint64(m.popupBytesLocked()))

	changed := !samePopupSig(m.lastPopupSig, sig)
	var frames []PopupFrame
	if changed {
		m.lastPopupSig = append([]popupSig(nil), sig...)
		frames = make([]PopupFrame, 0, len(items)+len(closing))
		for _, it := range items {
			frames = append(frames, m.popupCache[it.ID].frame(it.ID))
		}
		for _, id := range closing {
			frames = append(frames, m.popupCache[id].frame(id))
		}
	}
	m.popupMu.Unlock()

	if changed {
		sink(frames)
	}
}

// Arena — арена попапов (ADR-0009): один холст-заготовка на все
// открытые хостовые попапы движка вместо холста на каждый. Попапы
// рендерятся в неё по очереди (горутина кадра, под frameMu), картинка каждого
// копируется в его запись; заготовка держит только буфер рисования размером
// с самый большой попап и переводится на размер очередного без новой памяти.
type Arena struct {
	c       *raster.Canvas
	src     *raster.Canvas // холст движка, с которого снят клон
	fontRev uint64
	scale   float64
}

// CanvasFor отдаёт заготовку, переведённую на попап r (логический размер),
// чистую, — как свежий клон холста src этого размера. Клон пересоздаётся при
// смене холста движка, его шрифтов или масштаба.
func (a *Arena) CanvasFor(src *raster.Canvas, r image.Rectangle) *raster.Canvas {
	if a.c == nil || a.src != src || a.fontRev != src.FontRev() || a.scale != src.Scale() {
		a.c = src.CloneForSize(r.Dx(), r.Dy(), src.Scale(), nil)
		a.c.InheritWallpaper(src)
		a.src, a.fontRev, a.scale = src, src.FontRev(), src.Scale()
	}
	a.c.ResizeScratch(r.Dx(), r.Dy())
	return a.c
}

// Canvas — заготовка (nil — отпущена). Для диагностики и тестов.
func (a *Arena) Canvas() *raster.Canvas { return a.c }

// Source — холст движка, с которого снята заготовка. Для диагностики и тестов.
func (a *Arena) Source() *raster.Canvas { return a.src }

// Release отдаёт заготовку сборщику (попапов нет, хост снят).
func (a *Arena) Release() { *a = Arena{} }

// trim отпускает заготовку, если открытых попапов нет или её буфер больше
// нужного самому большому из них: следующий рендер заведёт её по размеру.
// Пока открыты попапы разного размера, буфер равен большему и не
// перевыделяется от кадра к кадру.
func (a *Arena) trim(items []PopupItem) {
	if a.c == nil {
		return
	}
	need := 0
	for _, it := range items {
		need = max(need, a.c.ScratchBytes(it.Rect.Dx(), it.Rect.Dy()))
	}
	if need == 0 || cap(a.c.Back().Pix) > need {
		a.Release()
	}
}

// Bytes — байт, занятых буферами заготовки: по ёмкости — буфер рисования
// переведён на очередной попап срезом, а память держит размер самого
// большого.
func (a *Arena) Bytes() int {
	if a.c == nil {
		return 0
	}
	return cap(a.c.Back().Pix) + cap(a.c.Front().Pix)
}

// popupBytesLocked — память хостовых попапов: заготовка арены и картинки
// записей (под popupMu).
func (m *Manager) popupBytesLocked() int {
	n := m.popupArena.Bytes()
	for _, ent := range m.popupCache {
		if ent.img != nil {
			n += len(ent.img.Pix)
		}
	}
	return n
}

// CollectPopups рекурсивно собирает хостируемые оверлеи дерева (без рендера).
func CollectPopups(out []PopupItem, w widget.Widget, depth int) []PopupItem {
	return collectPopupsAt(out, w, image.Point{}, depth)
}

// collectPopupsAt — CollectPopups со смещением: off — смещение кадра w. Границы оверлея виджет отдаёт в
// своём кадре; окну-попапу нужны экранные — вычитаем кадр.
func collectPopupsAt(out []PopupItem, w widget.Widget, off image.Point, depth int) []PopupItem {
	if tooDeep(depth) {
		return out
	}
	if od, ok := w.(widget.OverlayDrawer); ok && od.HasOverlay() {
		if ob, ok := w.(widget.OverlayBoundsProvider); ok {
			if r := ob.OverlayBounds(); !r.Empty() {
				out = append(out, PopupItem{ID: WidgetID(w), Rect: r.Sub(off), OD: od, Off: off})
			}
		}
	}
	children := w.Children()
	if len(children) == 0 {
		return out
	}
	coff := off.Add(input.ContentShift(w))
	for _, child := range children {
		out = collectPopupsAt(out, child, coff, depth+1)
	}
	return out
}

// WidgetID возвращает стабильный ключ виджета (указатель на его структуру).
func WidgetID(w widget.Widget) uintptr {
	v := reflect.ValueOf(w)
	if v.Kind() == reflect.Ptr {
		return v.Pointer()
	}
	return 0
}

// RenderOverlay рендерит оверлей od в отдельный буфер размером с r (логический),
// в физическом масштабе движка. Возвращает ФИЗИЧЕСКИЙ RGBA.
func RenderOverlay(c *raster.Canvas, od widget.OverlayDrawer, r image.Rectangle) *image.RGBA {
	oc := c.CloneForSize(r.Dx(), r.Dy(), c.Scale(), nil)
	oc.InheritWallpaper(c)
	return RenderOverlayInto(oc, od, r)
}

// RenderOverlayInto рисует оверлей в подготовленный канвас oc. Координаты
// оверлея (абсолютные логические) транслируются на -r.Min.
func RenderOverlayInto(oc *raster.Canvas, od widget.OverlayDrawer, r image.Rectangle) *image.RGBA {
	return RenderOverlayShifted(oc, od, r, image.Point{})
}

// RenderOverlayShifted — RenderOverlayInto для оверлея виджета, лежащего в
// кадре off (внутри прокрутки): r — ЭКРАННЫЙ прямоугольник попапа, а оверлей
// рисуется в координатах своего виджета, поэтому сперва сдвиг кадра, потом
// сдвиг на начало попапа.
func RenderOverlayShifted(oc *raster.Canvas, od widget.OverlayDrawer, r image.Rectangle, off image.Point) *image.RGBA {
	tc := &Translating{Inner: oc, DX: r.Min.X, DY: r.Min.Y}
	if w, ok := od.(widget.Widget); ok {
		widget.DrawOverlayTraced(w, od, widget.OffsetContext(tc, off.X, off.Y))
	} else {
		od.DrawOverlay(widget.OffsetContext(tc, off.X, off.Y))
	}
	// Копия развязывает владение с хостом: он блитит асинхронно, а канвас
	// оверлея переиспользуется следующими кадрами.
	out := image.NewRGBA(oc.Back().Rect)
	copy(out.Pix, oc.Back().Pix)
	return out
}

// hashRGBA — быстрый хэш пикселей (детект изменения содержимого попапа).
func hashRGBA(img *image.RGBA) uint64 {
	if img == nil {
		return 0
	}
	h := fnv.New64a()
	h.Write(img.Pix)
	return h.Sum64()
}

// samePopupSig сравнивает две сигнатуры набора попапов (порядок стабилен —
// обход дерева детерминирован).
func samePopupSig(a, b []popupSig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
