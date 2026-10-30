package core

import (
	"image"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget"
)

// Событие смены мониторов (E5): общий слой окна без платформы. Платформенные
// источники событий проверяются в platform/win32/monitor_change_windows_test.go,
// platform/x11/x11_monitor_change_linux_test.go,
// platform/wayland/wayland_monitor_change_linux_test.go.

// monTestSpy — бэкенд, умеющий сообщать о смене мониторов.
type monTestSpy struct {
	fakeCloseNative
	mu       sync.Mutex
	notify   func()
	watches  int
	stopped  int
	noSource bool
}

func (m *monTestSpy) WatchMonitors(notify func()) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watches++
	m.notify = notify
	return func() {
		m.mu.Lock()
		m.stopped++
		m.notify = nil
		m.mu.Unlock()
	}
}

func (m *monTestSpy) fire() {
	m.mu.Lock()
	fn := m.notify
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// monTestList подменяет источник списка мониторов и дебаунс на время теста.
type monTestList struct {
	mu   sync.Mutex
	list []native.Monitor
	n    int
}

func (l *monTestList) get() []native.Monitor {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.n++
	return append([]native.Monitor(nil), l.list...)
}

func (l *monTestList) reads() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.n
}

func (l *monTestList) set(list ...native.Monitor) {
	l.mu.Lock()
	l.list = list
	l.mu.Unlock()
}

func monTestUseList(t *testing.T, first ...native.Monitor) *monTestList {
	t.Helper()
	l := &monTestList{list: first}
	oldFn, oldDeb := monitorListFunc, monitorChangeDebounce
	monitorListFunc = l.get
	monitorChangeDebounce = 20 * time.Millisecond
	t.Cleanup(func() { monitorListFunc, monitorChangeDebounce = oldFn, oldDeb })
	return l
}

func monTestWindow(t *testing.T) (*Window, *monTestSpy) {
	t.Helper()
	win := New(&postEngine{root: widget.NewWindow("T", 100, 100)}, "T")
	spy := &monTestSpy{}
	win.native = spy
	return win, spy
}

var (
	monTestA  = native.MonitorWithKey(native.Monitor{Name: "A", Bounds: image.Rect(0, 0, 1920, 1080), Work: image.Rect(0, 0, 1920, 1040), Scale: 1, Primary: true}, 1)
	monTestB  = native.MonitorWithKey(native.Monitor{Name: "B", Bounds: image.Rect(1920, 0, 3840, 1080), Work: image.Rect(1920, 0, 3840, 1080), Scale: 1}, 2)
	monTestA2 = native.MonitorWithKey(native.Monitor{Name: "A", Bounds: image.Rect(0, 0, 2560, 1440), Work: image.Rect(0, 0, 2560, 1400), Scale: 1, Primary: true}, 1)
)

// monTestFn — колбэк приложения, кладущий списки в канал.
func monTestFn() (func([]native.Monitor), chan []native.Monitor) {
	ch := make(chan []native.Monitor, 8)
	return func(l []native.Monitor) { ch <- l }, ch
}

func monTestExpect(t *testing.T, ch chan []native.Monitor, want int) []native.Monitor {
	t.Helper()
	select {
	case l := <-ch:
		if len(l) != want {
			t.Fatalf("колбэк получил %d мониторов, ждал %d: %+v", len(l), want, l)
		}
		return l
	case <-time.After(2 * time.Second):
		t.Fatal("колбэк смены мониторов не вызван")
	}
	return nil
}

func monTestExpectNone(t *testing.T, ch chan []native.Monitor) {
	t.Helper()
	select {
	case l := <-ch:
		t.Fatalf("колбэк вызван без перемены: %+v", l)
	case <-time.After(150 * time.Millisecond):
	}
}

// Подключили второй монитор: колбэк получает свежий список один раз.
func TestMonitorsChanged_NewMonitor(t *testing.T) {
	l := monTestUseList(t, monTestA)
	win, spy := monTestWindow(t)
	fn, ch := monTestFn()
	win.SetOnMonitorsChanged(fn) // окно уже «запущено» (native != nil) — подписка сразу
	if spy.watches != 1 {
		t.Fatalf("платформенных подписок %d, ждал 1", spy.watches)
	}

	l.set(monTestA, monTestB)
	spy.fire()
	got := monTestExpect(t, ch, 2)
	if got[1].Name != "B" {
		t.Errorf("список %+v", got)
	}
	monTestExpectNone(t, ch)
}

// Сообщения приходят пачкой: пережидаем паузу и сообщаем один раз.
func TestMonitorsChanged_BurstCoalesced(t *testing.T) {
	l := monTestUseList(t, monTestA)
	// Пауза с запасом против промежутков между сообщениями: под нагрузкой
	// (-race, параллельные пакеты) сон в 3 мс длится дольше 20 мс общей паузы
	// тестов, и всплеск переставал склеиваться. Восстанавливает monTestUseList.
	monitorChangeDebounce = 250 * time.Millisecond
	win, spy := monTestWindow(t)
	fn, ch := monTestFn()
	win.SetOnMonitorsChanged(fn)

	l.set(monTestA2)
	for i := 0; i < 5; i++ {
		spy.fire()
		time.Sleep(3 * time.Millisecond)
	}
	got := monTestExpect(t, ch, 1)
	if got[0].Bounds != monTestA2.Bounds {
		t.Errorf("получили %+v, ждал новое разрешение", got[0])
	}
	monTestExpectNone(t, ch)
	// Список читался дважды: при подписке и один раз после паузы, а не на
	// каждое из пяти сообщений.
	if n := l.reads(); n != 2 {
		t.Errorf("список мониторов прочитан %d раз, ждал 2", n)
	}
}

// Системные сообщения без перемены (перенос окна между экранами порождает
// DPI-сообщение) приложение не будят.
func TestMonitorsChanged_NoChangeNoCallback(t *testing.T) {
	monTestUseList(t, monTestA, monTestB)
	win, spy := monTestWindow(t)
	fn, ch := monTestFn()
	win.SetOnMonitorsChanged(fn)

	spy.fire()
	spy.fire()
	monTestExpectNone(t, ch)
}

// Рабочая область тоже часть конфигурации: панель задач сдвинули.
func TestMonitorsChanged_WorkAreaCounts(t *testing.T) {
	l := monTestUseList(t, monTestA)
	win, spy := monTestWindow(t)
	fn, ch := monTestFn()
	win.SetOnMonitorsChanged(fn)

	moved := monTestA
	moved.Work = image.Rect(0, 40, 1920, 1080) // панель переехала наверх
	l.set(moved)
	spy.fire()
	got := monTestExpect(t, ch, 1)
	if got[0].Work != moved.Work {
		t.Errorf("Work %v, ждал %v", got[0].Work, moved.Work)
	}
}

// Колбэк, заданный до Run, подписывается при создании окна (bringUp).
func TestMonitorsChanged_BeforeRun(t *testing.T) {
	l := monTestUseList(t, monTestA)
	win, spy := monTestWindow(t)
	win.native = nil
	fn, ch := monTestFn()
	win.SetOnMonitorsChanged(fn)
	if spy.watches != 0 {
		t.Fatal("подписка поднята до создания окна")
	}

	win.native = spy
	win.syncMonitorWatch()
	if spy.watches != 1 {
		t.Fatalf("подписок %d, ждал 1", spy.watches)
	}
	win.syncMonitorWatch() // повторный sync не плодит подписки
	if spy.watches != 1 {
		t.Errorf("повторный sync поднял подписку: %d", spy.watches)
	}
	l.set(monTestA, monTestB)
	spy.fire()
	monTestExpect(t, ch, 2)
}

// nil снимает подписку, закрытие окна — тоже; таймер, пережитый отпиской,
// колбэка не зовёт.
func TestMonitorsChanged_Unsubscribe(t *testing.T) {
	l := monTestUseList(t, monTestA)
	win, spy := monTestWindow(t)
	fn, ch := monTestFn()
	win.SetOnMonitorsChanged(fn)

	l.set(monTestA, monTestB)
	spy.fire()
	win.SetOnMonitorsChanged(nil) // до срабатывания дебаунса
	if spy.stopped != 1 {
		t.Errorf("платформенная подписка не снята: stopped=%d", spy.stopped)
	}
	monTestExpectNone(t, ch)

	// Повторная подписка работает, а закрытие окна снимает её снова.
	l.set(monTestA)
	win.SetOnMonitorsChanged(fn)
	l.set(monTestA2)
	spy.fire()
	monTestExpect(t, ch, 1)
	win.stopMonitorWatch()
	win.stopMonitorWatch() // идемпотентно
	if spy.stopped != 2 {
		t.Errorf("stopped=%d, ждал 2", spy.stopped)
	}
}

// Бэкенд без источника событий (macOS): подписка молча ничего не делает.
func TestMonitorsChanged_BackendWithoutSource(t *testing.T) {
	monTestUseList(t, monTestA)
	win, _, _, _ := closeTestWindow(t)
	fn, ch := monTestFn()
	win.SetOnMonitorsChanged(fn)
	win.monitorsEvent() // без подписки — не падает
	monTestExpectNone(t, ch)
	win.stopMonitorWatch()
}

// Колбэк ставится в очередь движка, а не зовётся из потока платформы.
func TestMonitorsChanged_RunsOnEngineGoroutine(t *testing.T) {
	l := monTestUseList(t, monTestA)
	root := widget.NewWindow("T", 100, 100)
	eng := &postEngine{root: root}
	win := New(eng, "T")
	spy := &monTestSpy{}
	win.native = spy
	fn, ch := monTestFn()
	win.SetOnMonitorsChanged(fn)
	l.set(monTestA, monTestB)
	spy.fire()
	monTestExpect(t, ch, 2)
	if eng.posts != 1 {
		t.Errorf("через Post ушло %d вызовов, ждал 1", eng.posts)
	}
}

func TestMonitorListsEqual(t *testing.T) {
	if !monitorListsEqual([]native.Monitor{monTestA, monTestB}, []native.Monitor{monTestA, monTestB}) {
		t.Error("одинаковые списки признаны разными")
	}
	if !monitorListsEqual(nil, nil) {
		t.Error("пустые списки признаны разными")
	}
	cases := map[string][]native.Monitor{
		"меньше": {monTestA},
		"границы": {monTestA2, monTestB},
		"имя":     {native.MonitorWithKey(native.Monitor{Name: "Z", Bounds: monTestA.Bounds, Work: monTestA.Work, Scale: 1, Primary: true}, 1), monTestB},
		"масштаб": {native.MonitorWithKey(native.Monitor{Name: "A", Bounds: monTestA.Bounds, Work: monTestA.Work, Scale: 2, Primary: true}, 1), monTestB},
		"главный": {native.MonitorWithKey(native.Monitor{Name: "A", Bounds: monTestA.Bounds, Work: monTestA.Work, Scale: 1, Primary: false}, 1), monTestB},
	}
	for name, other := range cases {
		if monitorListsEqual([]native.Monitor{monTestA, monTestB}, other) {
			t.Errorf("%s: разные списки признаны одинаковыми", name)
		}
	}
}
