package window

import (
	"reflect"
	"runtime"
	"sort"
	"testing"
)

// Захват клавиатуры (E3): общий слой окна и фильтр перехвата без платформы.
// Хук Win32, XGrabKeyboard и inhibitor Wayland проверяются в
// keygrab_windows_test.go, x11_keygrab_linux_test.go, wayland_keygrab_linux_test.go.

// grabEv прогоняет событие клавиши через фильтр с активным окном.
type grabStep struct {
	vk           int
	up           bool
	alt, ctrl    bool
	inactive     bool
	swallow      bool
	deliver, rep bool
}

func runGrabSteps(t *testing.T, steps []grabStep) {
	t.Helper()
	var f keyGrabFilter
	for i, s := range steps {
		got := f.event(s.vk, s.up, s.alt, s.ctrl, !s.inactive)
		want := keyGrabAction{Swallow: s.swallow, Deliver: s.deliver, Repeat: s.rep}
		if got != want {
			t.Errorf("шаг %d (vk=%#x up=%v alt=%v ctrl=%v inactive=%v): %+v, ждал %+v",
				i, s.vk, s.up, s.alt, s.ctrl, s.inactive, got, want)
		}
	}
}

// Клавиша Win целиком уходит окну — нажатие, автоповтор и отпускание, — а пока
// она зажата, перехватывается вся клавиатура (Win+E, Win+D…).
func TestKeyGrabFilter_WinKey(t *testing.T) {
	runGrabSteps(t, []grabStep{
		{vk: VK_LWIN, swallow: true, deliver: true},
		{vk: VK_LWIN, swallow: true, deliver: true, rep: true}, // автоповтор
		{vk: VK_E, swallow: true, deliver: true},               // Win+E
		{vk: VK_E, up: true, swallow: true, deliver: true},
		{vk: 0x44 /* D */, swallow: true, deliver: true}, // Win+D
		{vk: 0x44, up: true, swallow: true, deliver: true},
		{vk: VK_LWIN, up: true, swallow: true, deliver: true},
		// Win отпущена: обычные клавиши снова идут системе.
		{vk: VK_E},
		{vk: VK_E, up: true},
	})
}

// Правая и левая Win держатся независимо: с одной отпущенной вторая по-прежнему
// требует перехвата всех клавиш.
func TestKeyGrabFilter_BothWinKeys(t *testing.T) {
	runGrabSteps(t, []grabStep{
		{vk: VK_LWIN, swallow: true, deliver: true},
		{vk: VK_RWIN, swallow: true, deliver: true},
		{vk: VK_LWIN, up: true, swallow: true, deliver: true},
		{vk: VK_A, swallow: true, deliver: true}, // правая ещё зажата
		{vk: VK_A, up: true, swallow: true, deliver: true},
		{vk: VK_RWIN, up: true, swallow: true, deliver: true},
		{vk: VK_A},
	})
}

// Сочетания, которые система обрабатывает раньше окна.
func TestKeyGrabFilter_SystemCombos(t *testing.T) {
	cases := []struct {
		name      string
		vk        int
		alt, ctrl bool
		grab      bool
	}{
		{"Alt+Tab", VK_TAB, true, false, true},
		{"Tab без Alt", VK_TAB, false, false, false},
		{"Alt+Esc", VK_ESCAPE, true, false, true},
		{"Ctrl+Esc", VK_ESCAPE, false, true, true},
		{"Esc без модификаторов", VK_ESCAPE, false, false, false},
		{"PrintScreen", VK_SNAPSHOT, false, false, true},
		{"Alt+F4", VK_F4, true, false, true},
		{"F4 без Alt", VK_F4, false, false, false},
		{"Alt+Space", VK_SPACE, true, false, true},
		{"Space без Alt", VK_SPACE, false, false, false},
		{"Ctrl+C", VK_C, false, true, false},
		{"Alt+Enter", VK_ENTER, true, false, false},
	}
	for _, c := range cases {
		var f keyGrabFilter
		got := f.event(c.vk, false, c.alt, c.ctrl, true)
		if got.Swallow != c.grab || got.Deliver != c.grab {
			t.Errorf("%s: %+v, перехват ожидался %v", c.name, got, c.grab)
		}
		// Отпускание следует за нажатием: перехвачено ровно тогда, когда
		// было перехвачено нажатие.
		up := f.event(c.vk, true, c.alt, c.ctrl, true)
		if up.Swallow != c.grab {
			t.Errorf("%s: отпускание %+v при перехвате нажатия %v", c.name, up, c.grab)
		}
	}
}

// Ctrl и Alt, нажатые ДО нужной клавиши, фильтр видит сам (хук слышит их), и
// флага из события для этого не нужно.
func TestKeyGrabFilter_TracksCtrlAlt(t *testing.T) {
	runGrabSteps(t, []grabStep{
		{vk: grabVKLMenu},                                 // Alt нажат — сам по себе не перехватывается
		{vk: VK_TAB, swallow: true, deliver: true},        // Alt+Tab
		{vk: VK_TAB, up: true, swallow: true, deliver: true},
		{vk: grabVKLMenu, up: true},
		{vk: VK_TAB}, // Alt отпущен
		{vk: grabVKRControl},
		{vk: VK_ESCAPE, swallow: true, deliver: true}, // Ctrl+Esc
		{vk: VK_ESCAPE, up: true, swallow: true, deliver: true},
		{vk: grabVKRControl, up: true},
		{vk: VK_ESCAPE},
	})
}

// Пока окно не активно, система работает как всегда.
func TestKeyGrabFilter_InactivePassesThrough(t *testing.T) {
	runGrabSteps(t, []grabStep{
		{vk: VK_LWIN, inactive: true},
		{vk: VK_E, inactive: true},
		{vk: VK_TAB, alt: true, inactive: true},
		{vk: VK_SNAPSHOT, inactive: true},
	})
}

// Win нажали при активном окне, а фокус ушёл до отпускания: повтор и отпускание
// всё равно перехвачены (система не должна увидеть отпускание без нажатия), и
// отпускание окно получает — иначе приложение считало бы Win зажатой вечно.
func TestKeyGrabFilter_FocusLostWhileHeld(t *testing.T) {
	runGrabSteps(t, []grabStep{
		{vk: VK_LWIN, swallow: true, deliver: true},
		// Автоповтор уже при неактивном окне: перехват держится, окну он не нужен.
		{vk: VK_LWIN, inactive: true, swallow: true, deliver: false, rep: true},
		{vk: VK_LWIN, up: true, inactive: true, swallow: true, deliver: true},
	})
}

// held отдаёт перехваченные и ещё не отпущенные клавиши (для «отпускания» при
// выключении захвата); reset забывает всё.
func TestKeyGrabFilter_HeldAndReset(t *testing.T) {
	var f keyGrabFilter
	f.event(VK_LWIN, false, false, false, true)
	f.event(VK_E, false, false, false, true)
	got := f.held()
	sort.Ints(got)
	want := []int{VK_E, VK_LWIN}
	sort.Ints(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("held() = %v, ждал %v", got, want)
	}
	f.reset()
	if len(f.held()) != 0 {
		t.Errorf("после reset held() = %v", f.held())
	}
	if act := f.event(VK_E, false, false, false, true); act.Swallow {
		t.Error("после reset Win считается зажатой")
	}
}

// grabSpy — бэкенд с захватом клавиатуры.
type grabSpy struct {
	fakeCloseNative
	accept bool
	calls  []bool
	active bool
}

func (g *grabSpy) SetKeyboardGrab(on bool) bool {
	g.calls = append(g.calls, on)
	return g.accept || !on
}
func (g *grabSpy) KeyboardGrabActive() bool { return g.active }

// Бэкенд без захвата: включить нельзя, выключить — «и так выключено».
func TestSetKeyboardGrab_UnsupportedBackend(t *testing.T) {
	win, _, _, _ := closeTestWindow(t)
	if win.SetKeyboardGrab(true) {
		t.Error("бэкенд без захвата ответил true")
	}
	if !win.SetKeyboardGrab(false) {
		t.Error("выключение захвата, которого нет, должно быть успешным")
	}
	if win.KeyboardGrabbed() {
		t.Error("KeyboardGrabbed true без захвата")
	}
}

// Вызовы доходят до бэкенда, отказ — до приложения, а состояние читается у
// бэкенда (на Wayland композитор вправе не согласиться).
func TestSetKeyboardGrab_DelegatesToBackend(t *testing.T) {
	win, _, _, _ := closeTestWindow(t)
	spy := &grabSpy{accept: true}
	win.native = spy

	if !win.SetKeyboardGrab(true) {
		t.Error("SetKeyboardGrab(true) вернул false при согласии бэкенда")
	}
	if win.KeyboardGrabbed() {
		t.Error("KeyboardGrabbed true, пока бэкенд не подтвердил")
	}
	spy.active = true
	if !win.KeyboardGrabbed() {
		t.Error("KeyboardGrabbed не прочитал состояние бэкенда")
	}
	win.SetKeyboardGrab(false)
	if len(spy.calls) != 2 || !spy.calls[0] || spy.calls[1] {
		t.Errorf("бэкенд получил %v, ждал [true false]", spy.calls)
	}

	spy.accept = false
	if win.SetKeyboardGrab(true) {
		t.Error("отказ бэкенда не дошёл до приложения")
	}
}

// Запрос до Run запоминается и применяется один раз при создании окна;
// закрытие окна отпускает клавиатуру.
func TestSetKeyboardGrab_BeforeRunAndRelease(t *testing.T) {
	win, _, _, _ := closeTestWindow(t)
	win.native = nil
	ok := win.SetKeyboardGrab(true)
	if runtime.GOOS == "darwin" {
		if ok {
			t.Error("macOS: захвата нет, а SetKeyboardGrab ответил true")
		}
		return
	}
	if !ok {
		t.Fatal("до Run ответ должен быть предварительным true")
	}

	spy := &grabSpy{accept: true}
	win.native = spy
	win.setupKeyboardGrab()
	if len(spy.calls) != 1 || !spy.calls[0] {
		t.Fatalf("запрос до Run не применён: %v", spy.calls)
	}
	win.setupKeyboardGrab() // повторный setup запрос не дублирует
	if len(spy.calls) != 1 {
		t.Errorf("запрос применён повторно: %v", spy.calls)
	}

	win.releaseKeyboardGrab()
	if len(spy.calls) != 2 || spy.calls[1] {
		t.Errorf("закрытие окна не отпустило клавиатуру: %v", spy.calls)
	}
}

// Запрос выключить до Run ничего не включает.
func TestSetKeyboardGrab_BeforeRunOffDoesNothing(t *testing.T) {
	win, _, _, _ := closeTestWindow(t)
	win.native = nil
	win.SetKeyboardGrab(false)
	spy := &grabSpy{accept: true}
	win.native = spy
	win.setupKeyboardGrab()
	if len(spy.calls) != 0 {
		t.Errorf("выключенный захват дошёл до бэкенда: %v", spy.calls)
	}
}
