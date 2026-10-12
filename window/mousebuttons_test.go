package window

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Боковые кнопки мыши «назад/вперёд» (X1/X2) доходят до движка на всех
// бэкендах под одними и теми же кодами; прежние кнопки не сдвигаются.

func TestNativeButton_X1X2(t *testing.T) {
	cases := []struct {
		code int
		want widget.MouseButton
		ok   bool
	}{
		{0, widget.MouseLeft, true},
		{1, widget.MouseRight, true},
		{2, widget.MouseMiddle, true},
		{3, widget.MouseWheelUp, true},
		{4, widget.MouseWheelDown, true},
		{5, widget.MouseButtonX1, true},
		{6, widget.MouseButtonX2, true},
		{7, 0, false},
		{-1, 0, false},
	}
	for _, c := range cases {
		got, ok := nativeButton(c.code)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("nativeButton(%d) = %v,%v; ждал %v,%v", c.code, got, ok, c.want, c.ok)
		}
	}
}

func TestX11ButtonID(t *testing.T) {
	cases := map[int]int{
		1: buttonCodeLeft,
		2: buttonCodeMiddle, // X11: 2 — средняя
		3: buttonCodeRight,  // X11: 3 — правая
		4: 3,                // колесо вверх
		5: 4,                // колесо вниз
		8: buttonCodeX1,     // «назад»
		9: buttonCodeX2,     // «вперёд»
	}
	for raw, want := range cases {
		got, ok := x11ButtonID(raw)
		if !ok || got != want {
			t.Errorf("кнопка X11 %d → %d,%v; ждал %d", raw, got, ok, want)
		}
	}
	for _, raw := range []int{0, 6, 7, 10, 11, 255} {
		if _, ok := x11ButtonID(raw); ok {
			t.Errorf("кнопка X11 %d не должна отображаться (6 и 7 — горизонтальное колесо)", raw)
		}
	}
}

func TestWlButtonID(t *testing.T) {
	cases := map[uint32]int{
		btnLeft:      buttonCodeLeft,
		btnRight:     buttonCodeRight,
		btnMiddle:    buttonCodeMiddle,
		wlBtnSide:    buttonCodeX1, // BTN_SIDE — как кнопка 8 на X11
		wlBtnExtra:   buttonCodeX2, // BTN_EXTRA — как кнопка 9
		wlBtnBack:    buttonCodeX1,
		wlBtnForward: buttonCodeX2,
	}
	for btn, want := range cases {
		got, ok := wlButtonID(btn)
		if !ok || got != want {
			t.Errorf("evdev 0x%x → %d,%v; ждал %d", btn, got, ok, want)
		}
	}
	for _, btn := range []uint32{0, 0x117 /* BTN_TASK */, 0x100, 0x2000} {
		if _, ok := wlButtonID(btn); ok {
			t.Errorf("evdev 0x%x не должен отображаться", btn)
		}
	}
	// Коды, которые Wayland сообщал и раньше, не изменились.
	if wlBtnSide != 0x113 || wlBtnExtra != 0x114 || wlBtnForward != 0x115 || wlBtnBack != 0x116 {
		t.Error("коды evdev боковых кнопок сдвинулись")
	}
}

func TestXButtonFromWParam(t *testing.T) {
	// GET_XBUTTON_WPARAM — старшее слово; в младшем — состояние остальных
	// кнопок и модификаторов (MK_*), оно ответа не меняет.
	cases := []struct {
		wparam uintptr
		want   int
		ok     bool
	}{
		{1 << 16, buttonCodeX1, true},
		{2 << 16, buttonCodeX2, true},
		{1<<16 | 0x0020, buttonCodeX1, true}, // MK_XBUTTON1 в младшем слове
		{2<<16 | 0x0040 | 0x0008, buttonCodeX2, true},
		{3 << 16, 0, false}, // третья боковая кнопка — не наша
		{0, 0, false},
	}
	for _, c := range cases {
		got, ok := xButtonFromWParam(c.wparam)
		if ok != c.ok || got != c.want {
			t.Errorf("wParam 0x%x → %d,%v; ждал %d,%v", c.wparam, got, ok, c.want, c.ok)
		}
	}
}

func TestCocoaOtherButton(t *testing.T) {
	cases := map[int]int{2: buttonCodeMiddle, 3: buttonCodeX1, 4: buttonCodeX2}
	for n, want := range cases {
		got, ok := cocoaOtherButton(n)
		if !ok || got != want {
			t.Errorf("buttonNumber %d → %d,%v; ждал %d", n, got, ok, want)
		}
	}
	if _, ok := cocoaOtherButton(5); ok {
		t.Error("buttonNumber 5 не должен отображаться")
	}
}

// Событие X1/X2 с насоса ОС добирается до движка и не превращается в левую
// кнопку.
func TestSurface_X1X2ReachEngine(t *testing.T) {
	_, fn, eng := inputSurface(t)
	fn.onButton(10, 20, 5, true)
	fn.onButton(10, 20, 5, false)
	fn.onButton(10, 20, 6, true)
	fn.onButton(10, 20, 6, false)
	eng.run()
	wantLog(t, eng,
		"button 10,20 5 true",
		"button 10,20 5 false",
		"button 10,20 6 true",
		"button 10,20 6 false")
	if widget.MouseButtonX1 != 5 || widget.MouseButtonX2 != 6 {
		t.Error("числовые значения X1/X2 сдвинулись")
	}
}
