//go:build windows

package core

import (
	"fmt"
	"reflect"
	"testing"
)

// v3.39 на Win32: lParam клавиш разбирается в скан-код, расширенный бит и
// автоповтор; колесо сообщается и сырой дельтой, и прежним пиксельным путём.
// wndProc получает сообщения так же, как от живого окна (fakeWin32).

func TestWin32_KeyMessagesCarryScanCode(t *testing.T) {
	w := fakeWin32(t)
	var got []rawKey
	w.SetOnRawKey(func(k rawKey) { got = append(got, k) })

	// «A»: скан 0x1E; затем автоповтор; затем отпускание.
	wndProc(fakeHwnd, wmKeydown, VK_A, 0x001E0001)
	wndProc(fakeHwnd, wmKeydown, VK_A, 0x401E0001)
	wndProc(fakeHwnd, wmKeyup, VK_A, 0xC01E0001)
	// Стрелка вправо — расширенная; системное нажатие (Alt зажат) идёт тем же путём.
	wndProc(fakeHwnd, wmSyskeydown, VK_RIGHT, 0x214D0001)
	wndProc(fakeHwnd, wmSyskeyup, VK_RIGHT, 0xE14D0001)
	// Мультимедийная клавиша: скан-код приходит, и тоже расширенный.
	wndProc(fakeHwnd, wmKeydown, VK_VOLUME_UP, 0x01300001)

	want := []rawKey{
		{VK: VK_A, Scan: 0x1E, Pressed: true},
		{VK: VK_A, Scan: 0x1E, Pressed: true, Repeat: true},
		{VK: VK_A, Scan: 0x1E},
		{VK: VK_RIGHT, Scan: 0x4D, Extended: true, Pressed: true},
		{VK: VK_RIGHT, Scan: 0x4D, Extended: true},
		{VK: VK_VOLUME_UP, Scan: 0x30, Extended: true, Pressed: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("до приложения дошло:\n  %+v\nждал:\n  %+v", got, want)
	}
}

// Без rawKeySource остаются прежние приёмники — со своим списком параметров.
func TestWin32_KeyMessagesLegacyReceivers(t *testing.T) {
	w := fakeWin32(t)
	var got []string
	w.SetOnKeyDownRepeat(func(vk int, repeat bool) { got = append(got, fmt.Sprint("down ", vk, repeat)) })
	w.SetOnKeyUp(func(vk int) { got = append(got, fmt.Sprint("up ", vk)) })
	wndProc(fakeHwnd, wmKeydown, VK_A, 0x401E0001)
	wndProc(fakeHwnd, wmKeyup, VK_A, 0xC01E0001)
	want := []string{"down 65 true", "up 65"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("получено %q, ждал %q", got, want)
	}
}

func wheelWParam(delta int16) uintptr { return uintptr(uint16(delta)) << 16 }

func TestWin32_WheelRawAndLegacy(t *testing.T) {
	w := fakeWin32(t)
	var raw []wheelCall
	var pixels []wheelCall
	runLegacy := true
	w.SetOnMouseWheelPixels(func(x, y int, dx, dy float64) { pixels = append(pixels, wheelCall{x: x, y: y, dx: dx, dy: dy}) })
	w.SetOnWheelRaw(func(x, y int, dx, dy float64, precise bool, legacy func()) {
		raw = append(raw, wheelCall{x, y, dx, dy, precise})
		if runLegacy {
			legacy()
		}
	})

	// Вертикаль: щелчок вверх, щелчок вниз, малая дельта тачпада.
	for _, d := range []int16{120, -120, 30} {
		wndProc(fakeHwnd, wmMousewheel, wheelWParam(d), mkLParam(7, 9))
	}
	// Горизонталь: вправо, влево.
	for _, d := range []int16{120, -60} {
		wndProc(fakeHwnd, wmMousehwheel, wheelWParam(d), mkLParam(7, 9))
	}
	wantRaw := []wheelCall{
		{7, 9, 0, -120, false}, {7, 9, 0, 120, false}, {7, 9, 0, -30, true},
		{7, 9, 120, 0, false}, {7, 9, -60, 0, true},
	}
	if !reflect.DeepEqual(raw, wantRaw) {
		t.Errorf("сырое колесо:\n  %+v\nждал:\n  %+v", raw, wantRaw)
	}
	// Прежний путь сохранил своё значение: 40 px на щелчок, вниз — положительно.
	wantPix := []wheelCall{
		{x: 7, y: 9, dy: -40}, {x: 7, y: 9, dy: 40}, {x: 7, y: 9, dy: -10},
		{x: 7, y: 9, dx: 40}, {x: 7, y: 9, dx: -20},
	}
	if !reflect.DeepEqual(pixels, wantPix) {
		t.Errorf("пиксели:\n  %+v\nждал:\n  %+v", pixels, wantPix)
	}

	// Принято виджетами: окно не зовёт legacy — пикселей больше нет.
	pixels, raw, runLegacy = nil, nil, false
	wndProc(fakeHwnd, wmMousewheel, wheelWParam(120), mkLParam(7, 9))
	if len(raw) != 1 || len(pixels) != 0 {
		t.Errorf("raw=%d pixels=%d, ждал 1 и 0", len(raw), len(pixels))
	}

	// Ноль — не событие.
	raw = nil
	wndProc(fakeHwnd, wmMousewheel, wheelWParam(0), mkLParam(7, 9))
	if len(raw) != 0 {
		t.Errorf("нулевая дельта дала событие: %+v", raw)
	}
}

// Без приёмника сырого колеса поведение прежнее.
func TestWin32_WheelWithoutRawUnchanged(t *testing.T) {
	w := fakeWin32(t)
	var pixels []wheelCall
	w.SetOnMouseWheelPixels(func(x, y int, dx, dy float64) { pixels = append(pixels, wheelCall{x: x, y: y, dx: dx, dy: dy}) })
	wndProc(fakeHwnd, wmMousewheel, wheelWParam(-240), mkLParam(1, 2))
	if want := []wheelCall{{x: 1, y: 2, dy: 80}}; !reflect.DeepEqual(pixels, want) {
		t.Errorf("пиксели %+v, ждал %+v", pixels, want)
	}
}

// ─── E9: WM_ACTIVATE ─────────────────────────────────────────────────────────

func TestWin32_ActivateMessages(t *testing.T) {
	w := fakeWin32(t)
	var got []bool
	w.SetOnActivate(func(a bool) { got = append(got, a) })

	wndProc(fakeHwnd, wmActivate, 1, 0) // WA_ACTIVE
	wndProc(fakeHwnd, wmActivate, 2, 0) // WA_CLICKACTIVE
	wndProc(fakeHwnd, wmActivate, 0, 0) // WA_INACTIVE — уходит чужому окну
	// Уход в пользу другого НАШЕГО окна (попап меню, диалог) — не деактивация.
	wndProc(fakeHwnd, wmActivate, 0, fakeHwnd)

	if want := []bool{true, true, false}; !reflect.DeepEqual(got, want) {
		t.Errorf("SetOnActivate: %v, ждал %v", got, want)
	}
}
