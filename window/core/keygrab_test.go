package core

import (
	"runtime"
	"testing"
)

// Захват клавиатуры (E3): общий слой окна и фильтр перехвата без платформы.
// Хук Win32, XGrabKeyboard и inhibitor Wayland проверяются в
// keygrab_windows_test.go, x11_keygrab_linux_test.go, wayland_keygrab_linux_test.go.

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
