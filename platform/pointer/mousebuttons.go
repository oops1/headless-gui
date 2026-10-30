package pointer

// mousebuttons.go — кнопки мыши бэкендов → коды NativeWindow.SetOnMouseButton.
//
// Коды общие для всех бэкендов (см. nativeButton в window/core): 0 — левая, 1 — правая,
// 2 — средняя, 3/4 — тики колеса вверх/вниз, 5 — X1 («назад»), 6 — X2
// («вперёд»). Боковые кнопки раньше терялись: Win32 не разбирал
// WM_XBUTTON*, X11 отбрасывал кнопки 8 и 9, Wayland — BTN_SIDE/BTN_EXTRA.
//
// Файл без платформенного суффикса: сопоставление — чистые функции, их тесты
// идут в общем прогоне.

const (
	ButtonLeft   = 0
	ButtonRight  = 1
	ButtonMiddle = 2
	ButtonX1     = 5 // «назад»
	ButtonX2     = 6 // «вперёд»
)

// X11ButtonID переводит номер кнопки X11 (поле detail ButtonPress/Release) в
// код окна. ok=false — кнопка, которой окно не знает (6 и 7 — горизонтальное
// колесо, их разбирают до вызова; 10 и выше — кнопки экзотических мышей).
//
// 4 и 5 — тики вертикального колеса: X11 присылает их как кнопки.
func X11ButtonID(raw int) (id int, ok bool) {
	switch raw {
	case 1:
		return ButtonLeft, true
	case 2:
		return ButtonMiddle, true // X11: 2 — средняя, 3 — правая
	case 3:
		return ButtonRight, true
	case 4:
		return 3, true
	case 5:
		return 4, true
	case 8:
		return ButtonX1, true
	case 9:
		return ButtonX2, true
	}
	return 0, false
}

// Коды кнопок evdev (linux/input-event-codes.h), приходящие в
// wl_pointer.button.
const (
	WlBtnLeft   = 0x110 // BTN_LEFT
	WlBtnRight  = 0x111 // BTN_RIGHT
	WlBtnMiddle = 0x112 // BTN_MIDDLE

	WlBtnSide    = 0x113 // BTN_SIDE: боковая кнопка под большим пальцем — «назад»
	WlBtnExtra   = 0x114 // BTN_EXTRA: вторая боковая — «вперёд»
	WlBtnForward = 0x115 // BTN_FORWARD
	WlBtnBack    = 0x116 // BTN_BACK
)

// WlButtonID переводит код evdev кнопки Wayland в код окна. ok=false —
// кнопка, которой окно не знает.
//
// libinput отдаёт боковые кнопки обычной мыши как BTN_SIDE (ближняя к
// запястью, «назад») и BTN_EXTRA («вперёд»): так же их показывает X11 кнопками
// 8 и 9. BTN_BACK и BTN_FORWARD шлют мультимедийные мыши и драйверы, которые
// сами называют кнопки по назначению, — они ложатся на те же X1 и X2.
func WlButtonID(btn uint32) (id int, ok bool) {
	switch btn {
	case WlBtnLeft:
		return ButtonLeft, true
	case WlBtnRight:
		return ButtonRight, true
	case WlBtnMiddle:
		return ButtonMiddle, true
	case WlBtnSide, WlBtnBack:
		return ButtonX1, true
	case WlBtnExtra, WlBtnForward:
		return ButtonX2, true
	}
	return 0, false
}

// XButtonFromWParam разбирает wParam WM_XBUTTONDOWN/UP/DBLCLK:
// GET_XBUTTON_WPARAM — старшее слово, 1 — XBUTTON1, 2 — XBUTTON2.
// ok=false — другая боковая кнопка (их у мышей бывает больше).
func XButtonFromWParam(wparam uintptr) (id int, ok bool) {
	switch (wparam >> 16) & 0xFFFF {
	case 1:
		return ButtonX1, true
	case 2:
		return ButtonX2, true
	}
	return 0, false
}

// CocoaOtherButton переводит buttonNumber события NSOtherMouseDown/Up в код
// окна: 2 — средняя, 3 и 4 — боковые «назад» и «вперёд». ok=false — кнопка,
// которой окно не знает. Раньше любая «другая» кнопка считалась средней.
func CocoaOtherButton(buttonNumber int) (id int, ok bool) {
	switch buttonNumber {
	case 2:
		return ButtonMiddle, true
	case 3:
		return ButtonX1, true
	case 4:
		return ButtonX2, true
	}
	return 0, false
}
