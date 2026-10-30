//go:build linux && !android

package wayland

// wayland_drag_linux.go — окно как ИСТОЧНИК перетаскивания файлов (v3.34):
// wl_data_device.start_drag с wl_data_source, который предлагает
// text/uri-list.
//
// Порядок: создаём источник, объявляем тип и допустимые действия (v≥3),
// отдаём компоновщику вместе с serial'ом нажатия кнопки — тот доказывает, что
// перетаскивание начал пользователь. Дальше ведёт компоновщик: рисует курсор,
// выбирает приёмник, а у нас просит данные событием send (в нём file
// descriptor, в который пишем список). Тело списка собирается лениво, в
// момент send, и пишется в своей горутине: приёмник читает не торопясь, а
// запись в канал блокируется.
//
// Конец перетаскивания:
//   - v≥3: dnd_finished — приёмник забрал данные; итоговое действие пришло
//     событием action. cancelled — отказ или отмена (Esc, сброс не на окно);
//
// Нужен wl_data_device_manager v≥3 (set_actions, dnd_finished): без них
// окончание перетаскивания не узнать — приёмник может запрашивать данные
// ещё на лету, и по первому send итог не определить. На более старом
// компоновщике BeginDragFiles возвращает false.
//
// Значок (BeginDragFilesImage): третий аргумент start_drag — wl_surface с
// буфером ARGB8888 из wl_shm. Левый верхний угол поверхности компоновщик
// ставит в точку курсора, а attach(buffer, -hx, -hy) сдвигает его так, чтобы
// под курсором оказалась точка захвата картинки. На HiDPI картинка (пиксели
// экрана) показывается 1:1 через wl_surface.set_buffer_scale. Поверхность и
// буфер живут до конца перетаскивания (dragFinish). Без значка (BeginDragFiles
// или картинка не получилась) start_drag идёт с icon = NULL — обычный курсор.
//
// Чего нет: действие «ссылка» (в протоколе его нет: ask — это «спросить
// пользователя», а не link); перетаскивание с попапа.

import (
	"encoding/binary"
	"image"
	"io"
	"os"
	"strings"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/oops1/headless-gui/v3/platform/native"
)

const (
	// wl_data_device
	wlDataDeviceStartDrag = 0

	// wl_data_source
	wlDataSourceSetActions        = 2
	wlDataSourceEvDndDropPerformed = 3
	wlDataSourceEvDndFinished     = 4
	wlDataSourceEvAction          = 5

	// wl_data_device_manager.dnd_action (copy — wlDndActionCopy в native_wayland.go)
	wlDndActionMove = 2
)

// wlDragSource — перетаскивание, начатое окном. Нулевое значение — перетаскивания нет.
type wlDragSource struct {
	mu      sync.Mutex
	id      uint32 // wl_data_source; 0 — перетаскивания нет
	body    string // text/uri-list
	done    func(native.DragEffect)
	action  native.DragEffect // действие, о котором сообщил компоновщик
	dropped bool              // кнопку отпустили над приёмником (dnd_drop_performed)

	// Значок: поверхность и буфер (0 — значка нет). Освобождаются в dragFinish.
	iconSurf, iconBuf uint32
}

// is сообщает, наш ли это wl_data_source (для разбора событий).
func (d *wlDragSource) is(id uint32) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return id != 0 && d.id == id
}

// wlActionsFor переводит допустимые действия в маску wl_data_device_manager.
// Link в протоколе нет: он отбрасывается.
func wlActionsFor(effect native.DragEffect) uint32 {
	var m uint32
	if effect&native.DragEffectCopy != 0 {
		m |= wlDndActionCopy
	}
	if effect&native.DragEffectMove != 0 {
		m |= wlDndActionMove
	}
	return m
}

// wlEffectFromAction — обратный перевод итогового действия компоновщика.
func wlEffectFromAction(a uint32) native.DragEffect {
	switch a {
	case wlDndActionCopy:
		return native.DragEffectCopy
	case wlDndActionMove:
		return native.DragEffectMove
	}
	return 0
}

// BeginDragFiles начинает перетаскивание файлов (fileDragSource):
// wl_data_device.start_drag. Нужны: wl_data_device, зажатая кнопка мыши и её
// serial; фокус указателя — на самом окне, не на попапе.
func (w *WaylandWindow) BeginDragFiles(paths []string, effect native.DragEffect, done func(native.DragEffect)) bool {
	return w.beginDrag(paths, effect, nil, image.Point{}, done)
}

// BeginDragFilesImage — BeginDragFiles со значком (fileDragImageSource).
func (w *WaylandWindow) BeginDragFilesImage(paths []string, effect native.DragEffect, img *image.RGBA, hotspot image.Point, done func(native.DragEffect)) bool {
	return w.beginDrag(paths, effect, img, hotspot, done)
}

// DragImageSupported — у компоновщика есть всё для поверхности-значка.
func (w *WaylandWindow) DragImageSupported() bool {
	return w.dataDevMgrID != 0 && w.dataDeviceID != 0 && w.dataDevVersion >= 3 &&
		w.shmID != 0 && w.compositorID != 0
}

func (w *WaylandWindow) beginDrag(paths []string, effect native.DragEffect, img *image.RGBA, hot image.Point, done func(native.DragEffect)) bool {
	actions := wlActionsFor(effect)
	serial := w.ptrButtonSerial.Load()
	if len(paths) == 0 || actions == 0 || done == nil {
		return false
	}
	if w.dataDevMgrID == 0 || w.dataDeviceID == 0 || w.surfaceID == 0 || w.dataDevVersion < 3 || serial == 0 ||
		w.ptrButtons.Load() == 0 || w.ptrOnPopup {
		return false // нечем доказать, что перетаскивание начал пользователь
	}
	d := &w.drag
	d.mu.Lock()
	if d.id != 0 {
		d.mu.Unlock()
		return false // одно перетаскивание за раз
	}
	id := w.newID()
	d.id, d.body, d.done = id, native.BuildURIList(paths), done
	d.action, d.dropped = 0, false
	d.mu.Unlock()

	w.send(newWlMsg(w.dataDevMgrID, wlDataDevMgrCreateDataSource).putUint(id), -1)
	w.send(newWlMsg(id, wlDataSourceOffer).putString(native.MimeTextURIList), -1)
	w.send(newWlMsg(id, wlDataSourceSetActions).putUint(actions), -1)

	// Значок: поверхность с буфером; без него (или если не вышло) — NULL.
	var icon wlDragIcon
	if img != nil && w.DragImageSupported() {
		icon = w.dragIconCreate(img, hot)
	}
	d.mu.Lock()
	d.iconSurf, d.iconBuf = icon.surf, icon.buf
	d.mu.Unlock()
	// start_drag(source, origin, icon, serial)
	w.send(newWlMsg(w.dataDeviceID, wlDataDeviceStartDrag).
		putUint(id).putUint(w.surfaceID).putUint(icon.surf).putUint(serial), -1)
	w.dragIconShow(icon)
	// Нажатие ушло компоновщику, отпускания мы не получим (как после
	// xdg_toplevel.move): снимаем кнопки в движке сами.
	w.releaseHeldButtons()
	return true
}

// handleDragSourceEvent разбирает события нашего wl_data_source.
func (w *WaylandWindow) handleDragSourceEvent(obj uint32, opcode uint16, b []byte) {
	d := &w.drag
	switch opcode {
	case wlDataSourceEvSend:
		mime, _ := wlString(b, 0)
		w.dragHandleSend(mime, w.takeFD())

	case wlDataSourceEvCancelled:
		w.dragFinish(obj, 0)

	case wlDataSourceEvDndDropPerformed:
		d.mu.Lock()
		d.dropped = true
		d.mu.Unlock()

	case wlDataSourceEvAction:
		if len(b) >= 4 {
			act := wlEffectFromAction(binary.LittleEndian.Uint32(b[0:4]))
			d.mu.Lock()
			d.action = act
			d.mu.Unlock()
		}

	case wlDataSourceEvDndFinished:
		d.mu.Lock()
		act := d.action
		d.mu.Unlock()
		w.dragFinish(obj, act)
	}
}

// dragHandleSend отдаёт запросившему список файлов: компоновщик передал fd, в
// который нужно записать данные запрошенного типа. На чужой тип fd просто
// закрывается — приёмник получит пустоту.
func (w *WaylandWindow) dragHandleSend(mime string, fd int) {
	d := &w.drag
	d.mu.Lock()
	body := d.body
	id := d.id
	d.mu.Unlock()
	if fd < 0 {
		return
	}
	f := os.NewFile(uintptr(fd), "wl-drag-send")
	if mime != native.MimeTextURIList || id == 0 {
		f.Close()
		return
	}
	go func() {
		defer f.Close()
		io.Copy(f, strings.NewReader(body))
	}()
}

// dragFinish заканчивает перетаскивание: источник уничтожается, приложению
// сообщается итог (0 — отмена). Повторный вызов — no-op.
func (w *WaylandWindow) dragFinish(src uint32, effect native.DragEffect) {
	d := &w.drag
	d.mu.Lock()
	if d.id == 0 || d.id != src {
		d.mu.Unlock()
		return
	}
	done := d.done
	iconSurf, iconBuf := d.iconSurf, d.iconBuf
	d.id, d.body, d.done = 0, "", nil
	d.iconSurf, d.iconBuf = 0, 0
	d.mu.Unlock()
	w.send(newWlMsg(src, wlDataSourceDestroy), -1)
	w.dragIconDestroy(iconSurf, iconBuf)
	if done != nil {
		done(effect)
	}
}

// ─── Значок перетаскивания ───────────────────────────────────────────────────

// wlDragIcon — поверхность значка, ещё не показанная (attach/commit после
// start_drag: роль «значок перетаскивания» поверхность получает от него).
type wlDragIcon struct {
	surf, buf uint32
	scale     int // целый масштаб буфера (1 — без set_buffer_scale)
	w, h      int // размер буфера в пикселях
	hot       image.Point
}

// dragIconCreate готовит поверхность значка: shm-буфер с картинкой и пустая
// wl_surface. Любая неудача — нулевой wlDragIcon (перетаскивание пойдёт без
// значка). Память пула отдаётся компоновщику сразу и здесь же освобождается:
// пул уничтожается после create_buffer, а отображение живёт, пока жив буфер.
func (w *WaylandWindow) dragIconCreate(img *image.RGBA, hot image.Point) wlDragIcon {
	b := img.Bounds()
	pw, ph := b.Dx(), b.Dy()
	if pw <= 0 || ph <= 0 || w.shmID == 0 || w.compositorID == 0 {
		return wlDragIcon{}
	}
	stride := pw * 4
	size := stride * ph
	fd, err := unix.MemfdCreate("headless-gui-dragicon", unix.MFD_CLOEXEC)
	if err != nil {
		return wlDragIcon{}
	}
	if err := unix.Ftruncate(fd, int64(size)); err != nil {
		unix.Close(fd)
		return wlDragIcon{}
	}
	data, err := unix.Mmap(fd, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		unix.Close(fd)
		return wlDragIcon{}
	}
	wlIconBGRA(data, stride, img)

	poolID := w.newID()
	w.send(newWlMsg(w.shmID, wlShmCreatePool).putUint(poolID).putInt(int32(size)), fd)
	bufID := w.newID()
	w.send(newWlMsg(poolID, wlShmPoolCreateBuffer).
		putUint(bufID).putInt(0).
		putInt(int32(pw)).putInt(int32(ph)).putInt(int32(stride)).
		putUint(wlShmFormatARGB8888), -1)
	w.send(newWlMsg(poolID, wlShmPoolDestroy), -1)
	unix.Munmap(data)
	unix.Close(fd)

	surfID := w.newID()
	w.send(newWlMsg(w.compositorID, wlCompositorCreateSurface).putUint(surfID), -1)
	scale := 1
	if k := w.scaleFactor(); k >= 2 && k == float64(int(k)) && w.gCompositorVer >= 3 {
		scale = int(k)
		w.send(newWlMsg(surfID, wlSurfaceSetBufferScale).putInt(int32(scale)), -1)
	}
	return wlDragIcon{surf: surfID, buf: bufID, scale: scale, w: pw, h: ph, hot: hot}
}

// wlIconBGRA раскладывает премультиплицированный RGBA в ARGB8888 wl_shm
// (в памяти B, G, R, A — тоже премультиплицированный).
func wlIconBGRA(dst []byte, dstStride int, img *image.RGBA) {
	b := img.Bounds()
	for y := 0; y < b.Dy(); y++ {
		src := img.Pix[img.PixOffset(b.Min.X, b.Min.Y+y):]
		row := dst[y*dstStride : y*dstStride+b.Dx()*4]
		for x := 0; x < b.Dx(); x++ {
			row[x*4+0], row[x*4+1], row[x*4+2], row[x*4+3] = src[x*4+2], src[x*4+1], src[x*4+0], src[x*4+3]
		}
	}
}

// dragIconShow показывает значок: attach с отрицательным смещением на
// точку захвата, damage и commit. Зовётся после start_drag.
func (w *WaylandWindow) dragIconShow(ic wlDragIcon) {
	if ic.surf == 0 {
		return
	}
	// Компоновщик ставит левый верхний угол в курсор; сдвиг на -hotspot (в
	// единицах поверхности) подводит под курсор точку захвата.
	ax, ay := -ic.hot.X/ic.scale, -ic.hot.Y/ic.scale
	w.send(newWlMsg(ic.surf, wlSurfaceAttach).putUint(ic.buf).putInt(int32(ax)).putInt(int32(ay)), -1)
	if ic.scale > 1 || w.gCompositorVer >= wlCompositorVersionBufferDamage {
		w.send(newWlMsg(ic.surf, wlSurfaceDamageBuffer).
			putInt(0).putInt(0).putInt(int32(ic.w)).putInt(int32(ic.h)), -1)
	} else {
		w.send(newWlMsg(ic.surf, wlSurfaceDamage).
			putInt(0).putInt(0).putInt(int32(ic.w)).putInt(int32(ic.h)), -1)
	}
	w.send(newWlMsg(ic.surf, wlSurfaceCommit), -1)
}

// dragIconDestroy освобождает поверхность и буфер значка.
func (w *WaylandWindow) dragIconDestroy(surf, buf uint32) {
	if surf != 0 {
		w.send(newWlMsg(surf, wlSurfaceDestroy), -1)
	}
	if buf != 0 {
		w.send(newWlMsg(buf, wlBufferDestroy), -1)
	}
}
