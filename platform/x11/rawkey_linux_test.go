//go:build linux && !android

package x11

import (
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v3/platform/keyboard"
	"github.com/oops1/headless-gui/v3/platform/native"
)

func TestX11_RawKeyCarriesEvdevScanCode(t *testing.T) {
	w := &X11Window{}
	var got []keyboard.RawKey
	w.SetOnRawKey(func(k keyboard.RawKey) { got = append(got, k) })

	w.handleX11Event(x11KeyEv(2, x11KeyA, 100)) // нажатие «A»
	w.handleX11Event(x11KeyEv(2, x11KeyA, 200)) // тот же код без отпускания — автоповтор
	w.handleX11Event(x11KeyEv(3, x11KeyA, 300)) // отпускание
	w.handleX11Event(x11KeyEv(2, 134+8, 400))   // evdev 134 (KEY_OPEN): нет в таблице
	w.handleX11Event(x11KeyEv(3, 134+8, 500))   //
	w.handleX11Event(x11KeyEv(2, 194+8, 600))   // F24: в таблице появилась в v3.39
	w.handleX11Event(x11KeyEv(2, 126+8+0, 700)) // evdev 126 — правая Win

	want := []keyboard.RawKey{
		{VK: native.VK_A, Scan: 30, Pressed: true},
		{VK: native.VK_A, Scan: 30, Pressed: true, Repeat: true},
		{VK: native.VK_A, Scan: 30},
		{VK: 0, Scan: 134, Pressed: true},
		{VK: 0, Scan: 134},
		{VK: native.VK_F24, Scan: 194, Pressed: true},
		{VK: native.VK_RWIN, Scan: 126, Pressed: true},
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

// FocusIn / FocusOut доходят до SetOnActivate; потерянное отпускание клавиши
// не превращает следующее нажатие в «повтор».
func TestX11_FocusEventsReachOnActivate(t *testing.T) {
	w := &X11Window{}
	var got []bool
	w.SetOnActivate(func(a bool) { got = append(got, a) })
	var keys []keyboard.RawKey
	w.SetOnRawKey(func(k keyboard.RawKey) { keys = append(keys, k) })

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

// x11ButtonEv собирает ButtonPress (4) / ButtonRelease (5) в точке (x, y).
func x11ButtonEv(typ, button byte, x, y int) []byte {
	b := make([]byte, 32)
	b[0], b[1] = typ, button
	binary.LittleEndian.PutUint16(b[24:26], uint16(int16(x)))
	binary.LittleEndian.PutUint16(b[26:28], uint16(int16(y)))
	return b
}
