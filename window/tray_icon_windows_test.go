//go:build windows

package window

import (
	"image"
	"image/color"
	"testing"

	"golang.org/x/sys/windows"
)

// TestTrayIconAfterNotify проверяет судьбу HICON после Shell_NotifyIconW:
// при неудаче новая иконка уничтожается, а прежняя остаётся жить.
func TestTrayIconAfterNotify(t *testing.T) {
	const prev, next = windows.Handle(11), windows.Handle(22)

	keep, drop := trayIconAfterNotify(true, prev, next)
	if keep != next || drop != prev {
		t.Errorf("успех: keep=%v drop=%v, ожидалось keep=%v drop=%v", keep, drop, next, prev)
	}

	keep, drop = trayIconAfterNotify(false, prev, next)
	if keep != prev || drop != next {
		t.Errorf("неудача: keep=%v drop=%v, ожидалось keep=%v drop=%v", keep, drop, prev, next)
	}

	// Первая установка без прежней иконки: ронять нечего.
	keep, drop = trayIconAfterNotify(true, 0, next)
	if keep != next || drop != 0 {
		t.Errorf("первая установка: keep=%v drop=%v", keep, drop)
	}
	// Неудачная первая установка: новая иконка должна быть уничтожена.
	keep, drop = trayIconAfterNotify(false, 0, next)
	if keep != 0 || drop != next {
		t.Errorf("неудачная первая установка: keep=%v drop=%v", keep, drop)
	}
}

// TestRemoveTrayIconFreesHandle проверяет, что removeTrayIcon уничтожает HICON
// даже если иконка не была добавлена (Shell_NotifyIconW провалился).
func TestRemoveTrayIconFreesHandle(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	hicon := rgbaToHICON(img)
	if hicon == 0 {
		t.Skip("CreateIconIndirect недоступен в этом окружении")
	}

	w := &Win32Window{trayHIcon: hicon, trayAdded: false} // hwnd = 0
	w.removeTrayIcon()

	if w.trayHIcon != 0 {
		t.Errorf("trayHIcon = %v, ожидался 0 — иконка утекла", w.trayHIcon)
	}
	if w.trayAdded {
		t.Error("trayAdded остался true")
	}
}

// TestRemoveTrayIconIdempotent проверяет повторный вызов без иконки.
func TestRemoveTrayIconIdempotent(t *testing.T) {
	w := &Win32Window{}
	w.removeTrayIcon()
	w.removeTrayIcon()
	if w.trayHIcon != 0 || w.trayAdded {
		t.Error("состояние трея испорчено повторным снятием")
	}
}
