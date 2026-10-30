//go:build windows

package win32

// cursorimage_windows.go — Win32: курсор из картинки (Engine.SetCursorImage).
//
// HCURSOR строится CreateIconIndirect: цветной 32-битный DIB с альфой плюс
// 1-битная маска (rgbaToHandle). Готовые курсоры лежат в кэше окна по ключу содержимого
// (pointer.CursorLRU, до pointer.CursorCacheSize штук); вытесненный уничтожается DestroyCursor,
// кроме того, что выбран прямо сейчас.

import (
	"image"

	"github.com/oops1/headless-gui/v3/platform/pointer"
	"github.com/oops1/headless-gui/v3/widget/base"
)

// SetCursorImage показывает курсор из картинки. Реализует cursorImageSetter;
// зовётся на потоке окна, как SetCursor.
func (w *Win32Window) SetCursorImage(ci *base.CursorImage) {
	if ci == nil {
		return
	}
	h := w.imageCursor(ci)
	if h == 0 {
		w.SetCursor(pointer.CursorArrow) // построить не вышло: системная стрелка
		return
	}
	if w.cursorHandle == h && !w.cursorHidden {
		return
	}
	w.cursorHandle, w.cursorHidden = h, false
	if w.pointerOverWindow() {
		procSetCursor.Call(h)
	}
}

// imageCursor возвращает HCURSOR картинки: из кэша либо новый. 0 — система
// отказала. Только поток окна.
func (w *Win32Window) imageCursor(ci *base.CursorImage) uintptr {
	if w.imgCursors == nil {
		w.imgCursors = pointer.NewCursorLRU[uintptr](pointer.CursorCacheSize,
			func(h uintptr) {
				if h != 0 {
					procDestroyCursor.Call(h)
				}
			},
			func(h uintptr) bool { return h == w.cursorHandle })
	}
	if h, ok := w.imgCursors.Get(ci.Key); ok {
		return h
	}
	// Цветной DIB значка система читает как ПРЯМУЮ альфу (проверено
	// DrawIconEx: домноженные байты дали бы тёмную кайму), поэтому байты
	// NRGBA идут как есть.
	straight := &image.RGBA{Pix: ci.Pix, Stride: ci.W * 4, Rect: image.Rect(0, 0, ci.W, ci.H)}
	h := uintptr(rgbaToHandle(straight, false, uint32(ci.HotX), uint32(ci.HotY)))
	if h == 0 {
		return 0
	}
	w.imgCreated++
	w.imgCursors.Put(ci.Key, h)
	return h
}

// destroyImageCursors освобождает курсоры-картинки окна.
func (w *Win32Window) destroyImageCursors() {
	if w.imgCursors != nil {
		w.imgCursors.Clear()
	}
}
