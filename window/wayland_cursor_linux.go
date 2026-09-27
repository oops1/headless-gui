//go:build linux && !android

package window

// wayland_cursor_linux.go — форма курсора под Wayland.
//
// Курсора «системного вида» у Wayland-клиента нет: компоновщик показывает ту
// поверхность, которую ему дали. Путей два.
//
//  1. wp_cursor_shape_manager_v1 — курсор просят по имени, рисует компоновщик
//     из системной темы. Дёшево и выглядит как везде, но расширение есть не у
//     всех (объявляется глобалом registry).
//  2. wl_pointer.set_cursor с собственной поверхностью — работает всюду, где
//     есть wl_pointer версии 1. Картинки рисует waylandcursor.go.
//
// Слот на каждую форму создаётся один раз и больше не переписывается:
// компоновщик читает буфер курсора асинхронно, и правка «на месте» меняла бы
// картинку у него под руками.

import (
	"fmt"

	"golang.org/x/sys/unix"
)

const (
	// wp_cursor_shape_manager_v1
	wpCursorShapeMgrGetPointer = 1
	// wp_cursor_shape_device_v1
	wpCursorShapeDevSetShape = 1

	// wl_pointer
	wlPointerSetCursor = 0

	// wl_shm формат с альфой (обязателен к поддержке).
	wlShmFormatARGB8888 = 0
)

// SetCursor задаёт форму курсора (значения widget.Cursor). Форма применяется
// сразу, если указатель уже в окне; иначе — при ближайшем enter.
func (w *WaylandWindow) SetCursor(c int) {
	w.cursorMu.Lock()
	same := w.cursorSet && w.cursorShape == c
	w.cursorShape, w.cursorSet = c, true
	w.cursorMu.Unlock()
	if same {
		return
	}
	w.applyCursor()
}

// applyCursor отдаёт компоновщику текущую форму курсора.
//
// Без serial'а enter просьбу отклонят: set_cursor принимается только от окна,
// в котором указатель действительно находится.
func (w *WaylandWindow) applyCursor() {
	if w.pointerID == 0 || w.ptrEnterSerial.Load() == 0 {
		return
	}
	w.cursorMu.Lock()
	defer w.cursorMu.Unlock()
	if !w.cursorSet {
		w.cursorShape, w.cursorSet = wlCursorArrow, true
	}
	if w.setCursorByShape(w.cursorShape) {
		return
	}
	w.setCursorBySurface(w.cursorShape)
}

// setCursorByShape просит форму у компоновщика (wp_cursor_shape_manager_v1).
// false — расширения нет или для этой формы имени не нашлось.
// Вызывать под w.cursorMu.
func (w *WaylandWindow) setCursorByShape(c int) bool {
	if w.gCursorShapeMgr == 0 {
		return false
	}
	shape := wlCursorShape(c)
	if shape == 0 {
		return false
	}
	if w.cursorShapeMgrID == 0 {
		w.cursorShapeMgrID = w.bind(w.gCursorShapeMgr, "wp_cursor_shape_manager_v1", 1)
		w.cursorShapeDevID = w.newID()
		w.send(newWlMsg(w.cursorShapeMgrID, wpCursorShapeMgrGetPointer).
			putUint(w.cursorShapeDevID).putUint(w.pointerID), -1)
	}
	w.send(newWlMsg(w.cursorShapeDevID, wpCursorShapeDevSetShape).
		putUint(w.ptrEnterSerial.Load()).putUint(shape), -1)
	return true
}

// setCursorBySurface показывает собственную картинку курсора.
// Вызывать под w.cursorMu.
func (w *WaylandWindow) setCursorBySurface(c int) {
	slot, ok := w.cursorSlot(c)
	if !ok {
		return
	}
	if w.cursorSurfID == 0 {
		w.cursorSurfID = w.newID()
		w.send(newWlMsg(w.compositorID, wlCompositorCreateSurface).putUint(w.cursorSurfID), -1)
	}
	w.send(newWlMsg(w.cursorSurfID, wlSurfaceAttach).putUint(slot.bufID).putInt(0).putInt(0), -1)
	w.send(newWlMsg(w.cursorSurfID, wlSurfaceDamage).
		putInt(0).putInt(0).putInt(wlCursorSize).putInt(wlCursorSize), -1)
	w.send(newWlMsg(w.cursorSurfID, wlSurfaceCommit), -1)
	w.send(newWlMsg(w.pointerID, wlPointerSetCursor).
		putUint(w.ptrEnterSerial.Load()).putUint(w.cursorSurfID).
		putInt(int32(slot.hotX)).putInt(int32(slot.hotY)), -1)
}

// wlCursorSlot — готовый буфер одной формы курсора.
type wlCursorSlot struct {
	bufID uint32
	hotX  int
	hotY  int
}

// cursorSlot возвращает буфер формы, создавая его при первом обращении.
func (w *WaylandWindow) cursorSlot(c int) (wlCursorSlot, bool) {
	if c < 0 || c >= wlCursorCount {
		c = wlCursorArrow
	}
	if slot, ok := w.cursorSlots[c]; ok {
		return slot, true
	}
	if err := w.setupCursorPool(); err != nil {
		wlLog("курсор: %v", err)
		return wlCursorSlot{}, false
	}
	pix, cw, ch, hx, hy := wlCursorPixels(c)
	stride := cw * 4
	off := c * stride * ch
	if off+len(pix) > len(w.cursorData) {
		return wlCursorSlot{}, false
	}
	copy(w.cursorData[off:], pix)

	id := w.newID()
	w.send(newWlMsg(w.cursorPoolID, wlShmPoolCreateBuffer).
		putUint(id).
		putInt(int32(off)).
		putInt(int32(cw)).putInt(int32(ch)).
		putInt(int32(stride)).
		putUint(wlShmFormatARGB8888), -1)
	slot := wlCursorSlot{bufID: id, hotX: hx, hotY: hy}
	w.cursorSlots[c] = slot
	return slot, true
}

// setupCursorPool создаёт shm-пул под все формы курсора (по слоту на форму).
func (w *WaylandWindow) setupCursorPool() error {
	if w.cursorPoolID != 0 {
		return nil
	}
	if w.shmID == 0 || w.compositorID == 0 {
		return fmt.Errorf("нет wl_shm/wl_compositor")
	}
	size := wlCursorSize * wlCursorSize * 4 * wlCursorCount
	fd, err := unix.MemfdCreate("headless-gui-cursor", unix.MFD_CLOEXEC)
	if err != nil {
		return fmt.Errorf("memfd_create: %w", err)
	}
	if err := unix.Ftruncate(fd, int64(size)); err != nil {
		unix.Close(fd)
		return fmt.Errorf("ftruncate: %w", err)
	}
	data, err := unix.Mmap(fd, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		unix.Close(fd)
		return fmt.Errorf("mmap: %w", err)
	}
	w.cursorFD, w.cursorData = fd, data
	w.cursorSlots = make(map[int]wlCursorSlot, wlCursorCount)
	w.cursorPoolID = w.newID()
	w.send(newWlMsg(w.shmID, wlShmCreatePool).putUint(w.cursorPoolID).putInt(int32(size)), fd)
	return nil
}

// closeCursor освобождает память курсорного пула.
func (w *WaylandWindow) closeCursor() {
	if w.cursorData != nil {
		unix.Munmap(w.cursorData)
		w.cursorData = nil
	}
	if w.cursorFD > 0 {
		unix.Close(w.cursorFD)
		w.cursorFD = 0
	}
}
