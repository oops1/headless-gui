//go:build linux && !android

package wayland

// wayland_cursorimage_linux.go — Wayland: курсор из картинки
// (Engine.SetCursorImage).
//
// Курсор Wayland — поверхность, которую показывает компоновщик
// (wl_pointer.set_cursor), поэтому картинка просто кладётся в wl_shm-буфер
// ARGB8888 и прикрепляется к той же курсорной поверхности, что и нарисованные
// формы (wayland_cursor_linux.go).
//
// Буфер на картинку. Компоновщик читает буфер курсора асинхронно, переписывать
// его «на месте» нельзя (см. комментарий в wayland_cursor_linux.go), поэтому
// каждая картинка получает свой wl_buffer: память отдаётся компоновщику пулом,
// который тут же уничтожается (отображение живёт, пока жив буфер), и своё
// отображение снимается. Готовые буферы лежат в кэше по ключу картинки, не более
// pointer.CursorCacheSize; вытесненный — wl_buffer.destroy, кроме того, что прикреплён к
// курсорной поверхности сейчас.
//
// Масштаб. Пиксели курсора — физические. На масштабе окна k курсорная
// поверхность получает целый буферный масштаб round(k) (не меньше 1), так что
// картинка занимает на экране свои пиксели, а горячая точка пересчитывается в
// поверхностные единицы делением на тот же масштаб. Дробный масштаб округляется
// до ближайшего целого: у курсора нет wp_viewport. Буферный масштаб нужен
// версии wl_compositor не ниже 4 (так окно его и связывает); на старшем нет —
// картинка показывается без масштаба.

import (
	"golang.org/x/sys/unix"

	"github.com/oops1/headless-gui/v3/platform/pointer"
	"github.com/oops1/headless-gui/v3/widget/base"
)

// wlImgBuf — готовый буфер курсора-картинки.
type wlImgBuf struct {
	bufID uint32
}

// SetCursorImage показывает курсор из картинки. Реализует cursorImageSetter.
// Применяется сразу, если указатель уже в окне; иначе — при ближайшем enter.
func (w *WaylandWindow) SetCursorImage(ci *base.CursorImage) {
	if ci == nil {
		return
	}
	w.cursorMu.Lock()
	same := w.cursorImg != nil && w.cursorImg.Key == ci.Key
	w.cursorImg, w.cursorSet = ci, true
	w.cursorMu.Unlock()
	if same {
		return
	}
	w.applyCursor()
}

// SetPopupCursorImage показывает курсор-картинку над попапами
// (childPopupCursorImage): указатель и курсор у попапа общие с окном, и без этого
// картинка, заданная пока указатель на попапе, осталась бы невидимой до возврата
// в окно. Следующий SetPopupCursor заменяет её формой.
func (w *WaylandWindow) SetPopupCursorImage(ci *base.CursorImage) {
	w.cursorMu.Lock()
	same := w.popupImg != nil && ci != nil && w.popupImg.Key == ci.Key
	w.popupImg = ci
	w.cursorMu.Unlock()
	if !same && w.ptrOverPopup.Load() {
		w.applyCursor()
	}
}

// setCursorByImage прикрепляет картинку к курсорной поверхности. false — не
// вышло (нет shm и compositor, память): вызывающий показывает форму. Вызывать
// под w.cursorMu.
func (w *WaylandWindow) setCursorByImage(ci *base.CursorImage) bool {
	buf, ok := w.imageBuffer(ci)
	if !ok {
		return false
	}
	w.ensureCursorSurface()
	scale := w.cursorBufferScale()
	if scale != w.curSurfScale {
		w.send(newWlMsg(w.cursorSurfID, wlSurfaceSetBufferScale).putInt(int32(scale)), -1)
		w.curSurfScale = scale
	}
	w.send(newWlMsg(w.cursorSurfID, wlSurfaceAttach).putUint(buf.bufID).putInt(0).putInt(0), -1)
	w.send(newWlMsg(w.cursorSurfID, wlSurfaceDamage).
		putInt(0).putInt(0).putInt(int32(ci.W)).putInt(int32(ci.H)), -1) // с запасом: в поверхностных единицах ≤ пикселей
	w.send(newWlMsg(w.cursorSurfID, wlSurfaceCommit), -1)
	w.send(newWlMsg(w.pointerID, wlPointerSetCursor).
		putUint(w.ptrEnterSerial.Load()).putUint(w.cursorSurfID).
		putInt(int32(pointer.CursorScaledHotspot(ci.HotX, scale))).
		putInt(int32(pointer.CursorScaledHotspot(ci.HotY, scale))), -1)
	w.curSurfBuf = buf.bufID
	return true
}

// cursorBufferScale — целый буферный масштаб курсорной поверхности.
func (w *WaylandWindow) cursorBufferScale() int {
	if w.gCompositorVer < wlCompositorVersionBufferDamage {
		return 1 // wl_compositor связан на версии 1: set_buffer_scale недоступен
	}
	k := int(w.scaleFactor() + 0.5)
	if k < 1 {
		return 1
	}
	return k
}

// imageBuffer возвращает буфер картинки: из кэша либо новый. Под w.cursorMu.
func (w *WaylandWindow) imageBuffer(ci *base.CursorImage) (wlImgBuf, bool) {
	if w.imgCursors == nil {
		w.imgCursors = pointer.NewCursorLRU[wlImgBuf](pointer.CursorCacheSize,
			func(b wlImgBuf) { w.send(newWlMsg(b.bufID, wlBufferDestroy), -1) },
			func(b wlImgBuf) bool { return b.bufID == w.curSurfBuf })
	}
	if b, ok := w.imgCursors.Get(ci.Key); ok {
		return b, true
	}
	if w.shmID == 0 || w.compositorID == 0 {
		return wlImgBuf{}, false
	}
	stride := ci.W * 4
	size := stride * ci.H
	fd, err := unix.MemfdCreate("headless-gui-cursor-image", unix.MFD_CLOEXEC)
	if err != nil {
		wlLog("курсор-картинка: memfd_create: %v", err)
		return wlImgBuf{}, false
	}
	defer unix.Close(fd)
	if err := unix.Ftruncate(fd, int64(size)); err != nil {
		wlLog("курсор-картинка: ftruncate: %v", err)
		return wlImgBuf{}, false
	}
	if _, err := unix.Write(fd, pointer.CursorPremulBGRA(ci)); err != nil {
		wlLog("курсор-картинка: write: %v", err)
		return wlImgBuf{}, false
	}
	poolID := w.newID()
	w.send(newWlMsg(w.shmID, wlShmCreatePool).putUint(poolID).putInt(int32(size)), fd)
	bufID := w.newID()
	w.send(newWlMsg(poolID, wlShmPoolCreateBuffer).
		putUint(bufID).putInt(0).
		putInt(int32(ci.W)).putInt(int32(ci.H)).putInt(int32(stride)).
		putUint(wlShmFormatARGB8888), -1)
	w.send(newWlMsg(poolID, wlShmPoolDestroy), -1)

	b := wlImgBuf{bufID: bufID}
	w.imgCursors.Put(ci.Key, b)
	w.imgCreated++
	return b, true
}

// ensureCursorSurface создаёт курсорную поверхность при первом обращении. Под
// w.cursorMu.
func (w *WaylandWindow) ensureCursorSurface() {
	if w.cursorSurfID == 0 {
		w.cursorSurfID = w.newID()
		w.send(newWlMsg(w.compositorID, wlCompositorCreateSurface).putUint(w.cursorSurfID), -1)
		w.curSurfScale = 1
	}
}

// resetCursorSurfaceScale возвращает курсорной поверхности буферный масштаб 1:
// нарисованные формы рассчитаны на него. Под w.cursorMu.
func (w *WaylandWindow) resetCursorSurfaceScale() {
	if w.curSurfScale > 1 {
		w.send(newWlMsg(w.cursorSurfID, wlSurfaceSetBufferScale).putInt(1), -1)
		w.curSurfScale = 1
	}
}
