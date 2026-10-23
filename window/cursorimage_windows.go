//go:build windows

package window

// cursorimage_windows.go — Win32: курсор из картинки (Engine.SetCursorImage).
//
// HCURSOR строится CreateIconIndirect: цветной 32-битный DIB с альфой плюс
// 1-битная маска (rgbaToHandle). Готовые курсоры лежат в кэше окна по ключу содержимого
// (cursorLRU, до cursorCacheSize штук); вытесненный уничтожается DestroyCursor,
// кроме того, что выбран прямо сейчас.

import (
	"image"

	"github.com/oops1/headless-gui/v3/widget"
)

// SetCursorImage показывает курсор из картинки. Реализует cursorImageSetter;
// зовётся на потоке окна, как SetCursor.
func (w *Win32Window) SetCursorImage(ci *widget.CursorImage) {
	if ci == nil {
		return
	}
	h := w.imageCursor(ci)
	if h == 0 {
		w.SetCursor(wlCursorArrow) // построить не вышло: системная стрелка
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
func (w *Win32Window) imageCursor(ci *widget.CursorImage) uintptr {
	if w.imgCursors == nil {
		w.imgCursors = newCursorLRU[uintptr](cursorCacheSize,
			func(h uintptr) {
				if h != 0 {
					procDestroyCursor.Call(h)
				}
			},
			func(h uintptr) bool { return h == w.cursorHandle })
	}
	if h, ok := w.imgCursors.get(ci.Key); ok {
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
	w.imgCursors.put(ci.Key, h)
	return h
}

// destroyImageCursors освобождает курсоры-картинки окна.
func (w *Win32Window) destroyImageCursors() {
	if w.imgCursors != nil {
		w.imgCursors.clear()
	}
}
