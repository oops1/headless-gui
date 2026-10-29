package keyboard

import (
	"reflect"
	"sort"
	"testing"

	"github.com/oops1/headless-gui/v3/platform/native"
)

func runGrabSteps(t *testing.T, steps []grabStep) {
	t.Helper()
	var f GrabFilter
	for i, s := range steps {
		got := f.Event(s.vk, s.up, s.alt, s.ctrl, !s.inactive)
		want := GrabAction{Swallow: s.swallow, Deliver: s.deliver, Repeat: s.rep}
		if got != want {
			t.Errorf("шаг %d (vk=%#x up=%v alt=%v ctrl=%v inactive=%v): %+v, ждал %+v",
				i, s.vk, s.up, s.alt, s.ctrl, s.inactive, got, want)
		}
	}
}

// Сочетания, которые система обрабатывает раньше окна.
func TestKeyGrabFilter_SystemCombos(t *testing.T) {
	cases := []struct {
		name      string
		vk        int
		alt, ctrl bool
		grab      bool
	}{
		{"Alt+Tab", native.VK_TAB, true, false, true},
		{"Tab без Alt", native.VK_TAB, false, false, false},
		{"Alt+Esc", native.VK_ESCAPE, true, false, true},
		{"Ctrl+Esc", native.VK_ESCAPE, false, true, true},
		{"Esc без модификаторов", native.VK_ESCAPE, false, false, false},
		{"PrintScreen", native.VK_SNAPSHOT, false, false, true},
		{"Alt+F4", native.VK_F4, true, false, true},
		{"F4 без Alt", native.VK_F4, false, false, false},
		{"Alt+Space", native.VK_SPACE, true, false, true},
		{"Space без Alt", native.VK_SPACE, false, false, false},
		{"Ctrl+C", native.VK_C, false, true, false},
		{"Alt+Enter", native.VK_ENTER, true, false, false},
	}
	for _, c := range cases {
		var f GrabFilter
		got := f.Event(c.vk, false, c.alt, c.ctrl, true)
		if got.Swallow != c.grab || got.Deliver != c.grab {
			t.Errorf("%s: %+v, перехват ожидался %v", c.name, got, c.grab)
		}
		// Отпускание следует за нажатием: перехвачено ровно тогда, когда
		// было перехвачено нажатие.
		up := f.Event(c.vk, true, c.alt, c.ctrl, true)
		if up.Swallow != c.grab {
			t.Errorf("%s: отпускание %+v при перехвате нажатия %v", c.name, up, c.grab)
		}
	}
}

// Ctrl и Alt, нажатые ДО нужной клавиши, фильтр видит сам (хук слышит их), и
// флага из события для этого не нужно.
func TestKeyGrabFilter_TracksCtrlAlt(t *testing.T) {
	runGrabSteps(t, []grabStep{
		{vk: grabVKLMenu}, // Alt нажат — сам по себе не перехватывается
		{vk: native.VK_TAB, swallow: true, deliver: true}, // Alt+Tab
		{vk: native.VK_TAB, up: true, swallow: true, deliver: true},
		{vk: grabVKLMenu, up: true},
		{vk: native.VK_TAB}, // Alt отпущен
		{vk: grabVKRControl},
		{vk: native.VK_ESCAPE, swallow: true, deliver: true}, // Ctrl+Esc
		{vk: native.VK_ESCAPE, up: true, swallow: true, deliver: true},
		{vk: grabVKRControl, up: true},
		{vk: native.VK_ESCAPE},
	})
}

// held отдаёт перехваченные и ещё не отпущенные клавиши (для «отпускания» при
// выключении захвата); reset забывает всё.
func TestKeyGrabFilter_HeldAndReset(t *testing.T) {
	var f GrabFilter
	f.Event(native.VK_LWIN, false, false, false, true)
	f.Event(native.VK_E, false, false, false, true)
	got := f.Held()
	sort.Ints(got)
	want := []int{native.VK_E, native.VK_LWIN}
	sort.Ints(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("held() = %v, ждал %v", got, want)
	}
	f.Reset()
	if len(f.Held()) != 0 {
		t.Errorf("после reset held() = %v", f.Held())
	}
	if act := f.Event(native.VK_E, false, false, false, true); act.Swallow {
		t.Error("после reset Win считается зажатой")
	}
}

// grabEv прогоняет событие клавиши через фильтр с активным окном.
type grabStep struct {
	vk           int
	up           bool
	alt, ctrl    bool
	inactive     bool
	swallow      bool
	deliver, rep bool
}

// Клавиша Win целиком уходит окну — нажатие, автоповтор и отпускание, — а пока
// она зажата, перехватывается вся клавиатура (Win+E, Win+D…).
func TestKeyGrabFilter_WinKey(t *testing.T) {
	runGrabSteps(t, []grabStep{
		{vk: native.VK_LWIN, swallow: true, deliver: true},
		{vk: native.VK_LWIN, swallow: true, deliver: true, rep: true}, // автоповтор
		{vk: native.VK_E, swallow: true, deliver: true},               // Win+E
		{vk: native.VK_E, up: true, swallow: true, deliver: true},
		{vk: 0x44 /* D */, swallow: true, deliver: true}, // Win+D
		{vk: 0x44, up: true, swallow: true, deliver: true},
		{vk: native.VK_LWIN, up: true, swallow: true, deliver: true},
		// Win отпущена: обычные клавиши снова идут системе.
		{vk: native.VK_E},
		{vk: native.VK_E, up: true},
	})
}

// Правая и левая Win держатся независимо: с одной отпущенной вторая по-прежнему
// требует перехвата всех клавиш.
func TestKeyGrabFilter_BothWinKeys(t *testing.T) {
	runGrabSteps(t, []grabStep{
		{vk: native.VK_LWIN, swallow: true, deliver: true},
		{vk: native.VK_RWIN, swallow: true, deliver: true},
		{vk: native.VK_LWIN, up: true, swallow: true, deliver: true},
		{vk: native.VK_A, swallow: true, deliver: true}, // правая ещё зажата
		{vk: native.VK_A, up: true, swallow: true, deliver: true},
		{vk: native.VK_RWIN, up: true, swallow: true, deliver: true},
		{vk: native.VK_A},
	})
}

// Пока окно не активно, система работает как всегда.
func TestKeyGrabFilter_InactivePassesThrough(t *testing.T) {
	runGrabSteps(t, []grabStep{
		{vk: native.VK_LWIN, inactive: true},
		{vk: native.VK_E, inactive: true},
		{vk: native.VK_TAB, alt: true, inactive: true},
		{vk: native.VK_SNAPSHOT, inactive: true},
	})
}

// Win нажали при активном окне, а фокус ушёл до отпускания: повтор и отпускание
// всё равно перехвачены (система не должна увидеть отпускание без нажатия), и
// отпускание окно получает — иначе приложение считало бы Win зажатой вечно.
func TestKeyGrabFilter_FocusLostWhileHeld(t *testing.T) {
	runGrabSteps(t, []grabStep{
		{vk: native.VK_LWIN, swallow: true, deliver: true},
		// Автоповтор уже при неактивном окне: перехват держится, окну он не нужен.
		{vk: native.VK_LWIN, inactive: true, swallow: true, deliver: false, rep: true},
		{vk: native.VK_LWIN, up: true, inactive: true, swallow: true, deliver: true},
	})
}
