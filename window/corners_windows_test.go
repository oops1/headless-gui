package window

import "testing"

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
