package win32

import (
	"image/color"
	"testing"
	"unsafe"
)

// Win32: на Windows 11 углы скругляет DWM, на Windows 10 — нет (тогда окно
// прямоугольное и с системной тенью от рамки DWM).
func TestWin32RoundsCorners(t *testing.T) {
	w := newFsWindow(t)
	w.SetCornerRadius(8)
	if !w.RoundsCorners() {
		t.Skip("DWM не скругляет углы — это Windows 10, проверять нечего")
	}
	w.SetCornerRadius(0)
	if w.RoundsCorners() {
		t.Error("радиус 0: RoundsCorners должен быть false")
	}
}

// Win32 (GG-99): рамку рисует DWM там же, где скругляет углы; цвет рамки
// уходит в DWMWA_BORDER_COLOR и снимается системным значением.
func TestWin32DrawsFrame(t *testing.T) {
	w := newFsWindow(t)
	w.SetCornerRadius(8)
	if !w.RoundsCorners() {
		if w.DrawsFrame() {
			t.Error("DWM не скругляет углы, а DrawsFrame=true")
		}
		t.Skip("DWM не скругляет углы — это Windows 10, проверять нечего")
	}
	if !w.DrawsFrame() {
		t.Fatal("Windows 11, окно со скруглением: DrawsFrame=false")
	}
	// DWM отдаёт DWMWA_BORDER_COLOR только на запись: проверяем, что атрибут
	// этим окном принимается (S_OK), а SetFrameColor запоминает COLORREF.
	v := uint32(0x563412)
	if r, _, _ := procDwmSetWindowAttribute.Call(uintptr(w.hwnd), dwmwaBorderColor,
		uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v)); r != 0 {
		t.Fatalf("DWMWA_BORDER_COLOR не принят: %#x", r)
	}
	w.SetFrameColor(color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 255}, true)
	if got := w.frameColor.Load(); got != 0x563412 || !w.frameColorOn.Load() {
		t.Errorf("SetFrameColor: COLORREF %#x, задан=%v; ждал 0x563412", got, w.frameColorOn.Load())
	}
	w.SetFrameColor(color.RGBA{}, false)
	if w.frameColorOn.Load() {
		t.Error("set=false: цвет рамки остался заданным")
	}
	w.SetCornerRadius(0)
	if w.DrawsFrame() {
		t.Error("радиус 0: DrawsFrame должен быть false")
	}
}
