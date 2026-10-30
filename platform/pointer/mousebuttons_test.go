package pointer

import (
	"testing"
)

func TestX11ButtonID(t *testing.T) {
	cases := map[int]int{
		1: ButtonLeft,
		2: ButtonMiddle, // X11: 2 — средняя
		3: ButtonRight,  // X11: 3 — правая
		4: 3,            // колесо вверх
		5: 4,            // колесо вниз
		8: ButtonX1,     // «назад»
		9: ButtonX2,     // «вперёд»
	}
	for raw, want := range cases {
		got, ok := X11ButtonID(raw)
		if !ok || got != want {
			t.Errorf("кнопка X11 %d → %d,%v; ждал %d", raw, got, ok, want)
		}
	}
	for _, raw := range []int{0, 6, 7, 10, 11, 255} {
		if _, ok := X11ButtonID(raw); ok {
			t.Errorf("кнопка X11 %d не должна отображаться (6 и 7 — горизонтальное колесо)", raw)
		}
	}
}

func TestWlButtonID(t *testing.T) {
	cases := map[uint32]int{
		WlBtnLeft:    ButtonLeft,
		WlBtnRight:   ButtonRight,
		WlBtnMiddle:  ButtonMiddle,
		WlBtnSide:    ButtonX1, // BTN_SIDE — как кнопка 8 на X11
		WlBtnExtra:   ButtonX2, // BTN_EXTRA — как кнопка 9
		WlBtnBack:    ButtonX1,
		WlBtnForward: ButtonX2,
	}
	for btn, want := range cases {
		got, ok := WlButtonID(btn)
		if !ok || got != want {
			t.Errorf("evdev 0x%x → %d,%v; ждал %d", btn, got, ok, want)
		}
	}
	for _, btn := range []uint32{0, 0x117 /* BTN_TASK */, 0x100, 0x2000} {
		if _, ok := WlButtonID(btn); ok {
			t.Errorf("evdev 0x%x не должен отображаться", btn)
		}
	}
	// Коды, которые Wayland сообщал и раньше, не изменились.
	if WlBtnSide != 0x113 || WlBtnExtra != 0x114 || WlBtnForward != 0x115 || WlBtnBack != 0x116 {
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
		{1 << 16, ButtonX1, true},
		{2 << 16, ButtonX2, true},
		{1<<16 | 0x0020, ButtonX1, true}, // MK_XBUTTON1 в младшем слове
		{2<<16 | 0x0040 | 0x0008, ButtonX2, true},
		{3 << 16, 0, false}, // третья боковая кнопка — не наша
		{0, 0, false},
	}
	for _, c := range cases {
		got, ok := XButtonFromWParam(c.wparam)
		if ok != c.ok || got != c.want {
			t.Errorf("wParam 0x%x → %d,%v; ждал %d,%v", c.wparam, got, ok, c.want, c.ok)
		}
	}
}

func TestCocoaOtherButton(t *testing.T) {
	cases := map[int]int{2: ButtonMiddle, 3: ButtonX1, 4: ButtonX2}
	for n, want := range cases {
		got, ok := CocoaOtherButton(n)
		if !ok || got != want {
			t.Errorf("buttonNumber %d → %d,%v; ждал %d", n, got, ok, want)
		}
	}
	if _, ok := CocoaOtherButton(5); ok {
		t.Error("buttonNumber 5 не должен отображаться")
	}
}
