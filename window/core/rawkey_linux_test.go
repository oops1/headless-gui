//go:build linux && !android

package core

import (
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/oops1/headless-gui/v3/platform/keyboard"
	"github.com/oops1/headless-gui/v3/platform/native"
)

// v3.39 на X11 и Wayland: скан-код (evdev) у каждой клавиши, в том числе у тех,
// которых нет в таблице VK; автоповтор доходит вместе с ним; колесо сообщается
// сырой дельтой и прежним путём. Проверяется на проводе, как в соседних
// тестах: события собираются байтами и разбираются настоящими обработчиками.

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
	var got []keyboard.RawKey
	c.w.SetOnRawKey(func(k keyboard.RawKey) { got = append(got, k) })

	c.w.handleEvent(c.w.keyboardID, wlKeyboardEvKey, wlKeyBody(31, true))   // «S»
	c.w.handleEvent(c.w.keyboardID, wlKeyboardEvKey, wlKeyBody(31, false))  //
	c.w.handleEvent(c.w.keyboardID, wlKeyboardEvKey, wlKeyBody(117, true))  // KEY_KPEQUAL: нет в таблице
	c.w.handleEvent(c.w.keyboardID, wlKeyboardEvKey, wlKeyBody(117, false)) //
	c.w.handleEvent(c.w.keyboardID, wlKeyboardEvKey, wlKeyBody(114, true))  // KEY_VOLUMEDOWN
	c.w.handleEvent(c.w.keyboardID, wlKeyboardEvKey, wlKeyBody(114, false))

	want := []keyboard.RawKey{
		{VK: native.VK_S, Scan: 31, Pressed: true},
		{VK: native.VK_S, Scan: 31},
		{VK: 0, Scan: 117, Pressed: true},
		{VK: 0, Scan: 117},
		{VK: native.VK_VOLUME_DOWN, Scan: 114, Pressed: true},
		{VK: native.VK_VOLUME_DOWN, Scan: 114},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("до окна дошло:\n  %+v\nждал:\n  %+v", got, want)
	}
}

// Повтор, который ведёт клиент, тоже несёт скан-код и пометку Repeat.
func TestWayland_RawKeyRepeatKeepsScan(t *testing.T) {
	c := newWlTestWindow(t)
	var got []keyboard.RawKey
	c.w.SetOnRawKey(func(k keyboard.RawKey) { got = append(got, k) })
	c.w.deliverKey(31, false)
	c.w.deliverKey(31, true)
	want := []keyboard.RawKey{
		{VK: native.VK_S, Scan: 31, Pressed: true},
		{VK: native.VK_S, Scan: 31, Pressed: true, Repeat: true},
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
