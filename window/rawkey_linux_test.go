//go:build linux && !android

package window

import (
	"encoding/binary"
	"reflect"
	"testing"
)

// v3.39 на X11 и Wayland: скан-код (evdev) у каждой клавиши, в том числе у тех,
// которых нет в таблице VK; автоповтор доходит вместе с ним; колесо сообщается
// сырой дельтой и прежним путём. Проверяется на проводе, как в соседних
// тестах: события собираются байтами и разбираются настоящими обработчиками.

// x11ButtonEv собирает ButtonPress (4) / ButtonRelease (5) в точке (x, y).
func x11ButtonEv(typ, button byte, x, y int) []byte {
	b := make([]byte, 32)
	b[0], b[1] = typ, button
	binary.LittleEndian.PutUint16(b[24:26], uint16(int16(x)))
	binary.LittleEndian.PutUint16(b[26:28], uint16(int16(y)))
	return b
}

// ─── X11: клавиши ───────────────────────────────────────────────────────────

func TestX11_RawKeyCarriesEvdevScanCode(t *testing.T) {
	w := &X11Window{}
	var got []rawKey
	w.SetOnRawKey(func(k rawKey) { got = append(got, k) })

	w.handleX11Event(x11KeyEv(2, x11KeyA, 100)) // нажатие «A»
	w.handleX11Event(x11KeyEv(2, x11KeyA, 200)) // тот же код без отпускания — автоповтор
	w.handleX11Event(x11KeyEv(3, x11KeyA, 300)) // отпускание
	w.handleX11Event(x11KeyEv(2, 134+8, 400))   // evdev 134 (KEY_OPEN): нет в таблице
	w.handleX11Event(x11KeyEv(3, 134+8, 500))   //
	w.handleX11Event(x11KeyEv(2, 194+8, 600))   // F24: в таблице появилась в v3.39
	w.handleX11Event(x11KeyEv(2, 126+8+0, 700)) // evdev 126 — правая Win

	want := []rawKey{
		{VK: VK_A, Scan: 30, Pressed: true},
		{VK: VK_A, Scan: 30, Pressed: true, Repeat: true},
		{VK: VK_A, Scan: 30},
		{VK: 0, Scan: 134, Pressed: true},
		{VK: 0, Scan: 134},
		{VK: VK_F24, Scan: 194, Pressed: true},
		{VK: VK_RWIN, Scan: 126, Pressed: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("до окна дошло:\n  %+v\nждал:\n  %+v", got, want)
	}
}

// Без rawKeySource клавиши вне таблицы по-прежнему отбрасываются на месте.
func TestX11_UnnamedKeyDroppedWithoutRawReceiver(t *testing.T) {
	w := &X11Window{}
	n := 0
	w.SetOnKeyDown(func(vk int) { n++ })
	w.SetOnKeyUp(func(vk int) { n++ })
	w.handleX11Event(x11KeyEv(2, 134+8, 100))
	w.handleX11Event(x11KeyEv(3, 134+8, 200))
	if n != 0 {
		t.Errorf("безымянная клавиша дошла старым приёмникам %d раз", n)
	}
}

// ─── X11: колесо ────────────────────────────────────────────────────────────

func TestX11_WheelRaw(t *testing.T) {
	w := &X11Window{}
	var raw []wheelCall
	var ticks, pixels []string
	w.SetOnMouseButton(func(x, y, b int, p bool) {
		ticks = append(ticks, map[bool]string{true: "down", false: "up"}[p]+string(rune('0'+b)))
	})
	w.SetOnMouseWheelPixels(func(x, y int, dx, dy float64) {
		pixels = append(pixels, "px")
	})
	consume := false
	w.SetOnWheelRaw(func(x, y int, dx, dy float64, precise bool, legacy func()) {
		raw = append(raw, wheelCall{x, y, dx, dy, precise})
		if !consume {
			legacy()
		}
	})

	// 4 — вверх, 5 — вниз, 6 — влево, 7 — вправо; отпускания тоже приходят.
	for _, b := range []byte{4, 5, 6, 7} {
		w.handleX11Event(x11ButtonEv(4, b, 11, 22))
		w.handleX11Event(x11ButtonEv(5, b, 11, 22))
	}
	wantRaw := []wheelCall{
		{11, 22, 0, -120, false}, {11, 22, 0, 120, false},
		{11, 22, -120, 0, false}, {11, 22, 120, 0, false},
	}
	if !reflect.DeepEqual(raw, wantRaw) {
		t.Errorf("сырое колесо:\n  %+v\nждал:\n  %+v", raw, wantRaw)
	}
	// Не принято: тики 4/5 — парой «нажатие+отпускание» (кнопки 3 и 4 движка),
	// отпускание на проводе не пересылается второй раз; горизонталь — пиксели.
	wantTicks := []string{"down3", "up3", "down4", "up4"}
	if !reflect.DeepEqual(ticks, wantTicks) {
		t.Errorf("тики: %q, ждал %q", ticks, wantTicks)
	}
	if len(pixels) != 2 {
		t.Errorf("пиксельных событий горизонтали %d, ждал 2", len(pixels))
	}

	// Принято: прежний путь молчит совсем.
	consume = true
	ticks, pixels = nil, nil
	for _, b := range []byte{4, 7} {
		w.handleX11Event(x11ButtonEv(4, b, 1, 2))
		w.handleX11Event(x11ButtonEv(5, b, 1, 2))
	}
	if len(ticks) != 0 || len(pixels) != 0 {
		t.Errorf("прежний путь сработал при принятом колесе: тики %q пиксели %q", ticks, pixels)
	}
}

// Без приёмника сырого колеса X11 ведёт себя как раньше: нажатие и отпускание
// кнопки 4/5 приходят порознь.
func TestX11_WheelWithoutRawUnchanged(t *testing.T) {
	w := &X11Window{}
	var ticks []string
	w.SetOnMouseButton(func(x, y, b int, p bool) {
		ticks = append(ticks, map[bool]string{true: "down", false: "up"}[p]+string(rune('0'+b)))
	})
	w.handleX11Event(x11ButtonEv(4, 4, 1, 1))
	w.handleX11Event(x11ButtonEv(5, 4, 1, 1))
	if want := []string{"down3", "up3"}; !reflect.DeepEqual(ticks, want) {
		t.Errorf("тики: %q, ждал %q", ticks, want)
	}
}

// ─── Wayland: клавиши ───────────────────────────────────────────────────────

func wlKeyBody(evdev uint32, pressed bool) []byte {
	body := make([]byte, 16)
	binary.LittleEndian.PutUint32(body[8:12], evdev)
	if pressed {
		binary.LittleEndian.PutUint32(body[12:16], 1)
	}
	return body
}

func TestWayland_RawKeyCarriesEvdevScanCode(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.keyboardID = 21
	c.w.repeat.setInfo(0, 0) // повтор композитора выключен: проверяем только прямое событие
	var got []rawKey
	c.w.SetOnRawKey(func(k rawKey) { got = append(got, k) })

	c.w.handleEvent(c.w.keyboardID, wlKeyboardEvKey, wlKeyBody(31, true))   // «S»
	c.w.handleEvent(c.w.keyboardID, wlKeyboardEvKey, wlKeyBody(31, false))  //
	c.w.handleEvent(c.w.keyboardID, wlKeyboardEvKey, wlKeyBody(117, true))  // KEY_KPEQUAL: нет в таблице
	c.w.handleEvent(c.w.keyboardID, wlKeyboardEvKey, wlKeyBody(117, false)) //
	c.w.handleEvent(c.w.keyboardID, wlKeyboardEvKey, wlKeyBody(114, true))  // KEY_VOLUMEDOWN
	c.w.handleEvent(c.w.keyboardID, wlKeyboardEvKey, wlKeyBody(114, false))

	want := []rawKey{
		{VK: VK_S, Scan: 31, Pressed: true},
		{VK: VK_S, Scan: 31},
		{VK: 0, Scan: 117, Pressed: true},
		{VK: 0, Scan: 117},
		{VK: VK_VOLUME_DOWN, Scan: 114, Pressed: true},
		{VK: VK_VOLUME_DOWN, Scan: 114},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("до окна дошло:\n  %+v\nждал:\n  %+v", got, want)
	}
}

// Повтор, который ведёт клиент, тоже несёт скан-код и пометку Repeat.
func TestWayland_RawKeyRepeatKeepsScan(t *testing.T) {
	c := newWlTestWindow(t)
	var got []rawKey
	c.w.SetOnRawKey(func(k rawKey) { got = append(got, k) })
	c.w.deliverKey(31, false)
	c.w.deliverKey(31, true)
	want := []rawKey{
		{VK: VK_S, Scan: 31, Pressed: true},
		{VK: VK_S, Scan: 31, Pressed: true, Repeat: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("до окна дошло:\n  %+v\nждал:\n  %+v", got, want)
	}
}

// ─── Wayland: колесо ────────────────────────────────────────────────────────

func wlAxisBody(axis uint32, px float64) []byte {
	body := make([]byte, 12)
	binary.LittleEndian.PutUint32(body[4:8], axis)
	binary.LittleEndian.PutUint32(body[8:12], uint32(int32(px*256)))
	return body
}

func wlAxisInt(axis uint32, n int32) []byte {
	body := make([]byte, 8)
	binary.LittleEndian.PutUint32(body[0:4], axis)
	binary.LittleEndian.PutUint32(body[4:8], uint32(n))
	return body
}

func TestWayland_WheelRaw(t *testing.T) {
	c := newWlTestWindow(t)
	var raw []wheelCall
	var pixels []wheelCall
	consume := false
	c.w.SetOnMouseWheelPixels(func(x, y int, dx, dy float64) {
		pixels = append(pixels, wheelCall{x: x, y: y, dx: dx, dy: dy})
	})
	c.w.SetOnWheelRaw(func(x, y int, dx, dy float64, precise bool, legacy func()) {
		raw = append(raw, wheelCall{x, y, dx, dy, precise})
		if !consume {
			legacy()
		}
	})
	c.w.ptrX, c.w.ptrY = 33, 44

	// Версия 4: только axis. Щелчок колеса ≈ 10 единиц; тачпад — дробные.
	c.w.handleEvent(c.w.pointerID, wlPointerEvAxis, wlAxisBody(0, 10))   // вниз на щелчок
	c.w.handleEvent(c.w.pointerID, wlPointerEvAxis, wlAxisBody(0, -2.5)) // вверх, тачпад
	c.w.handleEvent(c.w.pointerID, wlPointerEvAxis, wlAxisBody(1, 10))   // вправо
	wantRaw := []wheelCall{
		{33, 44, 0, 120, true}, {33, 44, 0, -30, true}, {33, 44, 120, 0, true},
	}
	if !reflect.DeepEqual(raw, wantRaw) {
		t.Errorf("сырое колесо:\n  %+v\nждал:\n  %+v", raw, wantRaw)
	}
	// Прежний путь: 4 пикселя на единицу axis.
	wantPix := []wheelCall{
		{x: 33, y: 44, dy: 40}, {x: 33, y: 44, dy: -10}, {x: 33, y: 44, dx: 40},
	}
	if !reflect.DeepEqual(pixels, wantPix) {
		t.Errorf("пиксели:\n  %+v\nждал:\n  %+v", pixels, wantPix)
	}

	// Версии 5 и 8: discrete / value120 перед axis делают событие щелчками.
	raw, pixels = nil, nil
	c.w.handleEvent(c.w.pointerID, wlPointerEvAxisDiscrete, wlAxisInt(0, 2))
	c.w.handleEvent(c.w.pointerID, wlPointerEvAxis, wlAxisBody(0, 20))
	c.w.handleEvent(c.w.pointerID, wlPointerEvAxisValue120, wlAxisInt(0, -60))
	c.w.handleEvent(c.w.pointerID, wlPointerEvAxis, wlAxisBody(0, -5))
	// Запомненное значение не переходит на следующее событие.
	c.w.handleEvent(c.w.pointerID, wlPointerEvAxis, wlAxisBody(0, 10))
	wantRaw = []wheelCall{
		{33, 44, 0, 240, false}, {33, 44, 0, -60, false}, {33, 44, 0, 120, true},
	}
	if !reflect.DeepEqual(raw, wantRaw) {
		t.Errorf("сырое колесо с discrete:\n  %+v\nждал:\n  %+v", raw, wantRaw)
	}

	// Принято: прежний путь молчит.
	consume = true
	pixels = nil
	c.w.handleEvent(c.w.pointerID, wlPointerEvAxis, wlAxisBody(0, 10))
	if len(pixels) != 0 {
		t.Errorf("пиксели при принятом колесе: %+v", pixels)
	}
}

// Без приёмника сырого колеса Wayland шлёт пиксели, а без них — тики, как и раньше.
func TestWayland_WheelWithoutRawUnchanged(t *testing.T) {
	c := newWlTestWindow(t)
	var pixels []wheelCall
	c.w.SetOnMouseWheelPixels(func(x, y int, dx, dy float64) {
		pixels = append(pixels, wheelCall{x: x, y: y, dx: dx, dy: dy})
	})
	c.w.handleEvent(c.w.pointerID, wlPointerEvAxis, wlAxisBody(0, 10))
	if want := []wheelCall{{dy: 40}}; !reflect.DeepEqual(pixels, want) {
		t.Errorf("пиксели %+v, ждал %+v", pixels, want)
	}

	c.w.onMouseWheelPixels = nil
	var ticks []int
	c.w.SetOnMouseButton(func(x, y, b int, p bool) {
		if p {
			ticks = append(ticks, b)
		}
	})
	c.w.handleEvent(c.w.pointerID, wlPointerEvAxis, wlAxisBody(0, -10))
	if want := []int{3}; !reflect.DeepEqual(ticks, want) {
		t.Errorf("тики %v, ждал %v", ticks, want)
	}
}

// ─── E9: активность окна на X11 ──────────────────────────────────────────────

// FocusIn / FocusOut доходят до SetOnActivate; потерянное отпускание клавиши
// не превращает следующее нажатие в «повтор».
func TestX11_FocusEventsReachOnActivate(t *testing.T) {
	w := &X11Window{}
	var got []bool
	w.SetOnActivate(func(a bool) { got = append(got, a) })
	var keys []rawKey
	w.SetOnRawKey(func(k rawKey) { keys = append(keys, k) })

	w.handleX11Event(x11KeyEv(2, x11KeyA, 100)) // «A» зажата
	focusEv := func(typ byte) []byte {
		b := make([]byte, 32)
		b[0] = typ
		return b
	}
	w.handleX11Event(focusEv(10)) // FocusOut
	w.handleX11Event(focusEv(9))  // FocusIn
	w.handleX11Event(x11KeyEv(2, x11KeyA, 200))

	if want := []bool{false, true}; !reflect.DeepEqual(got, want) {
		t.Errorf("SetOnActivate: %v, ждал %v", got, want)
	}
	if len(keys) != 2 || keys[1].Repeat {
		t.Errorf("после потери фокуса нажатие принято за повтор: %+v", keys)
	}
}
