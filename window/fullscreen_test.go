package window

import (
	"image"
	"testing"
)

// Полный экран (E1): общий слой окна без платформы. Платформенные части
// проверяются на проводе в *_windows_test.go / *_linux_test.go.

// fsSpy — бэкенд с полным экраном: запоминает просьбы и умеет менять состояние
// «по воле системы».
type fsSpy struct {
	fakeCloseNative
	supported bool
	on        bool
	calls     []bool
	onChange  func(bool)
	maximized bool
}

func (f *fsSpy) SetFullscreen(on bool) bool {
	f.calls = append(f.calls, on)
	return f.supported
}
func (f *fsSpy) IsFullscreen() bool                   { return f.on }
func (f *fsSpy) SetOnFullscreenChanged(fn func(bool)) { f.onChange = fn }
func (f *fsSpy) IsMaximized() bool                    { return f.maximized }

func fsTestWindow(t *testing.T) (*Window, *fsSpy) {
	t.Helper()
	win, _, _, _ := closeTestWindow(t)
	spy := &fsSpy{supported: true}
	win.native = spy
	return win, spy
}

// Бэкенд без полного экрана: «не умею», а не молчаливая успешность.
func TestFullscreen_UnsupportedBackend(t *testing.T) {
	win, _, _, _ := closeTestWindow(t) // fakeCloseNative не знает про полный экран
	if win.SetFullscreen(true) {
		t.Error("бэкенд без полного экрана ответил true")
	}
	if win.IsFullscreen() {
		t.Error("IsFullscreen true без поддержки")
	}
}

// Вызовы доходят до бэкенда, а его отказ — до приложения.
func TestFullscreen_DelegatesToBackend(t *testing.T) {
	win, spy := fsTestWindow(t)
	if !win.SetFullscreen(true) {
		t.Error("SetFullscreen(true) вернул false при поддержке")
	}
	spy.on = true
	if !win.IsFullscreen() {
		t.Error("IsFullscreen не прочитал состояние бэкенда")
	}
	win.SetFullscreen(false)
	if len(spy.calls) != 2 || !spy.calls[0] || spy.calls[1] {
		t.Errorf("бэкенд получил %v, ждал [true false]", spy.calls)
	}

	spy.supported = false
	if win.SetFullscreen(true) {
		t.Error("отказ бэкенда не дошёл до приложения")
	}
}

// Запрос до Run запоминается и применяется при создании окна; колбэк,
// заданный до Run, получает смены состояния на горутине движка.
func TestFullscreen_BeforeRunAndCallback(t *testing.T) {
	win, spy := fsTestWindow(t)
	win.native = nil
	var got []bool
	win.SetOnFullscreenChanged(func(on bool) { got = append(got, on) })
	if !win.SetFullscreen(true) {
		t.Skip("платформа без полного экрана")
	}

	win.native = spy
	win.setupFullscreen()
	if len(spy.calls) != 1 || !spy.calls[0] {
		t.Fatalf("запрос до Run не применён: %v", spy.calls)
	}
	if spy.onChange == nil {
		t.Fatal("бэкенду не подписан приёмник смены")
	}

	// Второй setup не должен повторять запрос.
	win.setupFullscreen()
	if len(spy.calls) != 1 {
		t.Errorf("запрос применён повторно: %v", spy.calls)
	}

	spy.onChange(true)
	spy.onChange(false)
	if len(got) != 2 || !got[0] || got[1] {
		t.Errorf("колбэк получил %v, ждал [true false]", got)
	}
}

// Кнопка □ полноэкранного окна возвращает его, а не разворачивает: так вело
// себя окно, когда Wayland считал полный экран «развёрнутым».
func TestFullscreen_MaximizeButtonLeavesFullscreen(t *testing.T) {
	win, _, _, root := closeTestWindow(t)
	spy := &fsSpy{supported: true, on: true}
	win.native = spy
	win.setupWidgetWindow()

	root.OnMaximize()
	if len(spy.calls) != 1 || spy.calls[0] {
		t.Errorf("кнопка □ в полном экране: вызовы %v, ждал [false]", spy.calls)
	}
}

// Мониторы: выбор по прямоугольнику, упорядочивание, точка.
func TestMonitorHelpers(t *testing.T) {
	left := Monitor{Bounds: image.Rect(0, 0, 1920, 1080), Work: image.Rect(0, 0, 1920, 1040), Scale: 1}
	right := Monitor{Bounds: image.Rect(1920, 0, 3840, 1080), Scale: 1.5, Primary: true}
	list := sortMonitors([]Monitor{left, right})
	if !list[0].Primary || list[1].Bounds != left.Bounds {
		t.Errorf("порядок: основной первым, got %v", list)
	}

	// Окно, наполовину выступающее на правый монитор, но больше на левом.
	m, ok := MonitorForRect(list, image.Rect(1500, 100, 2100, 500))
	if !ok || m.Bounds != left.Bounds {
		t.Errorf("монитор по наибольшей площади: %v %v", m, ok)
	}
	// Окно целиком за отключённым монитором.
	if _, ok := MonitorForRect(list, image.Rect(5000, 0, 5400, 300)); ok {
		t.Error("прямоугольник вне мониторов нашёл монитор")
	}
	if m, ok := monitorAt(list, 2000, 10); !ok || !m.Primary {
		t.Errorf("точка на правом мониторе: %v %v", m, ok)
	}
	if _, ok := monitorAt(list, -5, 10); ok {
		t.Error("точка вне мониторов")
	}
}
