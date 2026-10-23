package window

// monitor_change.go — событие смены конфигурации мониторов (задание
// RDP-клиента E5).
//
// Monitors() отвечает на вопрос «что сейчас», но приложению нужно узнать и о
// перемене: подключили второй экран, сменили разрешение или масштаб, вынесли
// панель задач на другой край. Клиент удалённого рабочего стола в ответ
// отправляет серверу новую раскладку мониторов (RDPEDISP), окно слайд-шоу
// переезжает на оставшийся экран.
//
//	Win32   WM_DISPLAYCHANGE (разрешение, число мониторов), WM_DPICHANGED
//	        (масштаб), WM_SETTINGCHANGE с SPI_SETWORKAREA (рабочая область:
//	        панель задач сдвинули, скрыли, поменяли высоту);
//	X11     RandR: ScreenChangeNotify, CrtcChangeNotify и OutputChangeNotify
//	        на корневом окне (RRSelectInput) по отдельному соединению, плюс
//	        PropertyNotify по _NET_WORKAREA корня;
//	Wayland события нашего же соединения: новые и исчезнувшие глобалы
//	        wl_output (wl_registry.global / global_remove), wl_output.done и
//	        geometry/mode/scale, а также логические положение и размер из
//	        zxdg_output_v1. Рабочей области Wayland не сообщает вовсе (Work
//	        равна Bounds), так что её изменений нет;
//	Cocoa   нет (мониторов нет и в Monitors).
//
// Платформы шлют события пачками и по любому поводу: одно переключение
// разрешения — это несколько сообщений подряд, перенос окна на другой монитор
// порождает DPI-сообщение, хотя состав мониторов прежний. Поэтому событие
// только будит окно, а решает оно само: пережидает короткую паузу, берёт
// свежий список и сравнивает с тем, о чём приложение уже знает. Колбэк
// вызывается, лишь если список действительно изменился.

import (
	"sync"
	"time"
)

// monitorChangeSource — бэкенд сообщает, что с мониторами что-то случилось.
// Необязательная способность: без неё SetOnMonitorsChanged молчит.
type monitorChangeSource interface {
	// watchMonitors подписывается на изменения. notify вызывается из любого
	// потока, без замков и быстро; повторы и лишние вызовы допустимы.
	// stop снимает подписку. nil — платформа наблюдать не умеет.
	watchMonitors(notify func()) (stop func())
}

// monitorChangeDebounce — сколько ждём тишины после последнего сообщения
// системы, прежде чем перечитать мониторы. Переменная — чтобы тест не ждал.
var monitorChangeDebounce = 150 * time.Millisecond

// monitorListFunc — откуда окно берёт список мониторов. Переменная для тестов.
var monitorListFunc = Monitors

// monitorWatchState — подписка окна на смену мониторов.
type monitorWatchState struct {
	mu    sync.Mutex
	fn    func([]Monitor)
	stop  func() // снятие платформенной подписки; nil — её нет
	last  []Monitor
	timer *time.Timer
	gen   uint64 // растёт при каждой подписке и отписке: старый таймер выдыхается
}

// SetOnMonitorsChanged подписывает приложение на смену конфигурации мониторов:
// подключили или отключили экран, сменили разрешение, масштаб, положение,
// главный монитор, а на Windows и X11 — сдвинули панель задач (меняется
// Monitor.Work).
//
// В колбэк приходит свежий список в том же виде, что у Monitors() (основной
// первым, остальные слева направо, сверху вниз). Его зовут только при
// настоящей перемене: список сравнивается с последним, который видело
// приложение (границы, рабочая область, масштаб, имя, признак основного).
// Системные сообщения приходят пачками, поэтому колбэк срабатывает с
// небольшой задержкой (около 150 мс) после последнего из них.
//
// Колбэк выполняется на горутине движка, как обработчики виджетов: из него
// можно сразу менять виджеты. Первый вызов Monitors() для сравнения делается
// при подписке; на Linux это короткое отдельное подключение к серверу.
//
// Можно вызывать до Run (подписка начнётся при создании окна) и во время
// работы; nil снимает подписку. Дополнительные окна (OpenWindow) подписываются
// каждое отдельно. macOS: события нет (Monitors там пуст).
//
// Поле Monitor.Work — это рабочая область (Bounds без панелей); Monitors()
// возвращает её именно под этим именем, и оно не менялось.
func (win *Window) SetOnMonitorsChanged(fn func([]Monitor)) {
	win.mon.mu.Lock()
	win.mon.fn = fn
	running := win.native != nil
	win.mon.mu.Unlock()
	// До Run окна ОС ещё нет: подписку поднимет bringUp.
	if running {
		win.syncMonitorWatch()
	}
}

// syncMonitorWatch приводит платформенную подписку в соответствие с тем,
// задан колбэк или нет. Подписка ленивая: наблюдение за мониторами не нужно
// приложению, которое о них не просило.
func (win *Window) syncMonitorWatch() {
	st := &win.mon
	st.mu.Lock()
	hasFn, watching := st.fn != nil, st.stop != nil
	st.mu.Unlock()

	switch {
	case hasFn && !watching:
		src, ok := win.native.(monitorChangeSource)
		if !ok {
			return
		}
		// Исходный список читаем ДО подписки и вне замка: на Linux это
		// подключение к серверу. Без него первое же сообщение с прежним
		// составом мониторов сошло бы за перемену.
		base := monitorListFunc()
		stop := src.watchMonitors(win.monitorsEvent)
		st.mu.Lock()
		if st.stop != nil || st.fn == nil {
			// Пока читали, подписку успели поднять или снять — вторая не нужна.
			st.mu.Unlock()
			if stop != nil {
				stop()
			}
			return
		}
		st.last = base
		if stop == nil {
			stop = func() {} // платформа наблюдать не умеет: больше не пытаемся
		}
		st.stop = stop
		st.gen++
		st.mu.Unlock()
	case !hasFn && watching:
		win.stopMonitorWatch()
	}
}

// stopMonitorWatch снимает подписку. Зовётся при закрытии окна и при
// SetOnMonitorsChanged(nil).
func (win *Window) stopMonitorWatch() {
	st := &win.mon
	st.mu.Lock()
	stop := st.stop
	st.stop = nil
	st.gen++
	if st.timer != nil {
		st.timer.Stop()
		st.timer = nil
	}
	st.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// monitorsEvent принимает от платформы «с мониторами что-то случилось» и
// запускает (или откладывает) перечитывание.
func (win *Window) monitorsEvent() {
	st := &win.mon
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.fn == nil || st.stop == nil {
		return
	}
	gen := st.gen
	if st.timer != nil {
		st.timer.Stop()
	}
	// Источник списка берём здесь, а не в таймере: переменная меняется только
	// в тестах, и читать её из чужой горутины значило бы гонку с ними.
	list := monitorListFunc
	st.timer = time.AfterFunc(monitorChangeDebounce, func() { win.monitorsRefresh(gen, list) })
}

// monitorsRefresh перечитывает мониторы и, если они изменились, будит
// приложение. gen отсекает таймер, пережившего отписку.
func (win *Window) monitorsRefresh(gen uint64, read func() []Monitor) {
	list := read()
	st := &win.mon
	st.mu.Lock()
	if gen != st.gen || st.fn == nil {
		st.mu.Unlock()
		return
	}
	fn := st.fn
	same := monitorListsEqual(st.last, list)
	if !same {
		st.last = list
	}
	st.mu.Unlock()
	if same {
		return
	}
	// Колбэк приложения красит виджеты, то есть работает на горутине движка.
	win.post(func() { fn(append([]Monitor(nil), list...)) })
}

// monitorListsEqual — два списка описывают одну и ту же конфигурацию. Порядок
// записей у Monitors() устойчив (sortMonitors), так что сравниваем по месту.
func monitorListsEqual(a, b []Monitor) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.Name != y.Name || x.Bounds != y.Bounds || x.Work != y.Work ||
			x.Scale != y.Scale || x.Primary != y.Primary || x.id != y.id {
			return false
		}
	}
	return true
}
