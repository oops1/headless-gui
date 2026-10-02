//go:build linux && !android

package window

// wayland_repeat_linux.go — автоповтор удерживаемой клавиши.
//
// Под Wayland повтор — забота КЛИЕНТА: композитор присылает нажатие и
// отпускание, а между ними молчит. Пока этого не было, удержание стрелки или
// Backspace срабатывало ровно один раз, и редактировать текст в сессии было
// нельзя: каждый символ приходилось выбивать отдельным нажатием.
//
// Частоту и задержку сообщает сам композитор (wl_keyboard.repeat_info,
// версия 4 протокола); у композитора постарше берём привычные значения
// рабочего стола. rate == 0 означает «повтор выключен» — это тоже ответ
// композитора, и спорить с ним нельзя.

import (
	"sync"
	"time"
)

const (
	// Значения по умолчанию — те же, что у GNOME и KDE: полсекунды до
	// первого повтора, дальше двадцать пять раз в секунду.
	wlDefaultRepeatDelay = 500 * time.Millisecond
	wlDefaultRepeatRate  = 25 // нажатий в секунду
)

// wlRepeater — таймер автоповтора одной клавиши.
//
// Повторяется всегда ровно одна клавиша, последняя нажатая: так делают и
// композиторы, и X-сервер. Новое нажатие отменяет повтор предыдущего.
type wlRepeater struct {
	mu    sync.Mutex
	delay time.Duration
	rate  int // нажатий в секунду; 0 — повтор выключен композитором
	stop  chan struct{}
	key   uint32 // evdev-код повторяемой клавиши
	busy  bool
}

// newWlRepeater — повторитель с значениями по умолчанию.
func newWlRepeater() *wlRepeater {
	return &wlRepeater{delay: wlDefaultRepeatDelay, rate: wlDefaultRepeatRate}
}

// setInfo применяет repeat_info композитора.
func (r *wlRepeater) setInfo(rate int32, delayMS int32) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rate = int(rate)
	if delayMS > 0 {
		r.delay = time.Duration(delayMS) * time.Millisecond
	}
}

// start запускает повтор клавиши key: fire зовётся через delay, дальше с
// частотой rate, пока не позовут stop.
//
// fire выполняется в своей горутине — доставка события и так идёт через
// очередь движка, и держать на ней таймер незачем.
func (r *wlRepeater) start(key uint32, fire func()) {
	if r == nil {
		return
	}
	r.cancel()
	r.mu.Lock()
	if r.rate <= 0 || fire == nil {
		r.mu.Unlock()
		return // композитор выключил повтор
	}
	stop := make(chan struct{})
	delay, period := r.delay, time.Second/time.Duration(r.rate)
	r.stop, r.key, r.busy = stop, key, true
	r.mu.Unlock()

	go func() {
		select {
		case <-stop:
			return
		case <-time.After(delay):
		}
		t := time.NewTicker(period)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				fire()
			}
		}
	}()
}

// stopKey останавливает повтор, если повторяется именно эта клавиша.
//
// Проверка по коду нужна из-за порядка событий: отпускание ПРЕДЫДУЩЕЙ
// клавиши нередко приходит уже после нажатия следующей, и без проверки оно
// гасило бы чужой повтор — удержание стрелки прекращалось бы от того, что
// человек отпустил соседнюю.
func (r *wlRepeater) stopKey(key uint32) {
	if r == nil {
		return
	}
	r.mu.Lock()
	same := r.busy && r.key == key
	r.mu.Unlock()
	if same {
		r.cancel()
	}
}

// cancel останавливает повтор любой клавиши — отпускание, потеря фокуса,
// закрытие окна.
func (r *wlRepeater) cancel() {
	if r == nil {
		return
	}
	r.mu.Lock()
	stop := r.stop
	r.stop, r.busy = nil, false
	r.mu.Unlock()
	if stop != nil {
		close(stop)
	}
}

// active сообщает, идёт ли сейчас повтор (для тестов и отладки).
func (r *wlRepeater) active() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.busy
}
