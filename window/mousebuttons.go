package window

// mousebuttons.go — кнопки мыши бэкендов → коды NativeWindow.SetOnMouseButton.
//
// Коды общие для всех бэкендов (см. nativeButton): 0 — левая, 1 — правая,
// 2 — средняя, 3/4 — тики колеса вверх/вниз, 5 — X1 («назад»), 6 — X2
// («вперёд»). Боковые кнопки раньше терялись: Win32 не разбирал
// WM_XBUTTON*, X11 отбрасывал кнопки 8 и 9, Wayland — BTN_SIDE/BTN_EXTRA.
//
// Файл без платформенного суффикса: сопоставление — чистые функции, их тесты
// идут в общем прогоне.

const (
	buttonCodeLeft   = 0
	buttonCodeRight  = 1
	buttonCodeMiddle = 2
	buttonCodeX1     = 5 // «назад»
	buttonCodeX2     = 6 // «вперёд»
)

// x11ButtonID переводит номер кнопки X11 (поле detail ButtonPress/Release) в
// код окна. ok=false — кнопка, которой окно не знает (6 и 7 — горизонтальное
// колесо, их разбирают до вызова; 10 и выше — кнопки экзотических мышей).
//
// 4 и 5 — тики вертикального колеса: X11 присылает их как кнопки.
func x11ButtonID(raw int) (id int, ok bool) {
	switch raw {
	case 1:
		return buttonCodeLeft, true
	case 2:
		return buttonCodeMiddle, true // X11: 2 — средняя, 3 — правая
	case 3:
		return buttonCodeRight, true
	case 4:
		return 3, true
	case 5:
		return 4, true
	case 8:
		return buttonCodeX1, true
	case 9:
		return buttonCodeX2, true
	}
	return 0, false
}

// Коды кнопок evdev (linux/input-event-codes.h), приходящие в
// wl_pointer.button.
const (
	btnLeft   = 0x110 // BTN_LEFT
	btnRight  = 0x111 // BTN_RIGHT
	btnMiddle = 0x112 // BTN_MIDDLE

	wlBtnSide    = 0x113 // BTN_SIDE: боковая кнопка под большим пальцем — «назад»
	wlBtnExtra   = 0x114 // BTN_EXTRA: вторая боковая — «вперёд»
	wlBtnForward = 0x115 // BTN_FORWARD
	wlBtnBack    = 0x116 // BTN_BACK
)

// wlButtonID переводит код evdev кнопки Wayland в код окна. ok=false —
// кнопка, которой окно не знает.
//
// libinput отдаёт боковые кнопки обычной мыши как BTN_SIDE (ближняя к
// запястью, «назад») и BTN_EXTRA («вперёд»): так же их показывает X11 кнопками
// 8 и 9. BTN_BACK и BTN_FORWARD шлют мультимедийные мыши и драйверы, которые
// сами называют кнопки по назначению, — они ложатся на те же X1 и X2.
func wlButtonID(btn uint32) (id int, ok bool) {
	switch btn {
	case btnLeft:
		return buttonCodeLeft, true
	case btnRight:
		return buttonCodeRight, true
	case btnMiddle:
		return buttonCodeMiddle, true
	case wlBtnSide, wlBtnBack:
		return buttonCodeX1, true
	case wlBtnExtra, wlBtnForward:
		return buttonCodeX2, true
	}
	return 0, false
}

// xButtonFromWParam разбирает wParam WM_XBUTTONDOWN/UP/DBLCLK:
// GET_XBUTTON_WPARAM — старшее слово, 1 — XBUTTON1, 2 — XBUTTON2.
// ok=false — другая боковая кнопка (их у мышей бывает больше).
func xButtonFromWParam(wparam uintptr) (id int, ok bool) {
	switch (wparam >> 16) & 0xFFFF {
	case 1:
		return buttonCodeX1, true
	case 2:
		return buttonCodeX2, true
	}
	return 0, false
}

// cocoaOtherButton переводит buttonNumber события NSOtherMouseDown/Up в код
// окна: 2 — средняя, 3 и 4 — боковые «назад» и «вперёд». ok=false — кнопка,
// которой окно не знает. Раньше любая «другая» кнопка считалась средней.
func cocoaOtherButton(buttonNumber int) (id int, ok bool) {
	switch buttonNumber {
	case 2:
		return buttonCodeMiddle, true
	case 3:
		return buttonCodeX1, true
	case 4:
		return buttonCodeX2, true
	}
	return 0, false
}
