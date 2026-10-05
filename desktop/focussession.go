// focussession.go — сеанс «Фокусировки» под календарём Windows 11.
//
// Календарь рисует длительность, кнопки «−» и «+», «Начать» и идущий отсчёт;
// что фокусировка делает на самом деле (включает «Не беспокоить», глушит
// звук, пишет статистику), знает только потребитель. Поэтому сеанс — интерфейс:
// состояние и действия приходят от потребителя, как уведомления и быстрые
// действия.
package desktop

import (
	"sync"
	"time"
)

// Пределы и шаг длительности, которыми календарь двигает кнопки «−» и «+».
// Потребитель вправе ограничить сильнее в своём SetDuration.
const (
	FocusStep        = 5 * time.Minute
	FocusMinDuration = 5 * time.Minute
	FocusMaxDuration = 240 * time.Minute
)

// FocusSessionState — что календарь показывает в модуле «Фокусировка».
type FocusSessionState struct {
	// Duration — выбранная длительность сеанса (в покое — то, что стоит между
	// «−» и «+»).
	Duration time.Duration
	// Running — сеанс идёт; EndsAt — момент окончания по часам календаря
	// (CalendarFlyout.Clock). Остаток считается как EndsAt − Now и не бывает
	// отрицательным: по окончании потребитель сам останавливает сеанс (Stop
	// идемпотентна), а до тех пор модуль показывает 00:00.
	Running bool
	EndsAt  time.Time
}

// FocusSession — сеанс «Фокусировки», который ведёт потребитель.
//
// Календарь зовёт SetDuration, Start и Stop по нажатиям; новое состояние
// потребитель сообщает обычным путём — через Subscribe (замыкание зовётся из
// любой горутины). Секундный отсчёт календарь рисует сам, пока открыт и идёт
// сеанс (CalendarFlyout.Ticker), потребителю слать в Subscribe каждую секунду
// не нужно.
type FocusSession interface {
	State() FocusSessionState
	SetDuration(time.Duration)
	Start()
	Stop()
	Subscribe(func()) (unsubscribe func())
}

// FakeFocusSession — сеанс в памяти: готовая модель для демо и тестов. Часы
// задаются явно, чтобы остаток не зависел от системного времени.
type FakeFocusSession struct {
	clk Clock

	mu      sync.Mutex
	st      FocusSessionState
	subs    map[int]func()
	nextSub int

	// Started и Stopped — журнал нажатий для тестов.
	Started, Stopped int
}

// NewFakeFocusSession создаёт сеанс длительностью 30 минут на часах clk.
func NewFakeFocusSession(clk Clock) *FakeFocusSession {
	if clk == nil {
		clk = SystemClock{}
	}
	return &FakeFocusSession{
		clk:  clk,
		st:   FocusSessionState{Duration: 30 * time.Minute},
		subs: map[int]func(){},
	}
}

var _ FocusSession = (*FakeFocusSession)(nil)

// State возвращает текущее состояние.
func (f *FakeFocusSession) State() FocusSessionState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.st
}

// SetDuration задаёт длительность. Во время сеанса она не меняется.
func (f *FakeFocusSession) SetDuration(d time.Duration) {
	f.mu.Lock()
	changed := !f.st.Running && f.st.Duration != d
	if changed {
		f.st.Duration = d
	}
	f.mu.Unlock()
	if changed {
		f.notify()
	}
}

// Start начинает сеанс на выбранную длительность.
func (f *FakeFocusSession) Start() {
	f.mu.Lock()
	if f.st.Running {
		f.mu.Unlock()
		return
	}
	f.st.Running = true
	f.st.EndsAt = f.clk.Now().Add(f.st.Duration)
	f.Started++
	f.mu.Unlock()
	f.notify()
}

// Stop прекращает сеанс.
func (f *FakeFocusSession) Stop() {
	f.mu.Lock()
	if !f.st.Running {
		f.mu.Unlock()
		return
	}
	f.st.Running = false
	f.st.EndsAt = time.Time{}
	f.Stopped++
	f.mu.Unlock()
	f.notify()
}

// Subscribe подписывает на изменения.
func (f *FakeFocusSession) Subscribe(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	f.mu.Lock()
	f.nextSub++
	id := f.nextSub
	f.subs[id] = fn
	f.mu.Unlock()
	return func() {
		f.mu.Lock()
		delete(f.subs, id)
		f.mu.Unlock()
	}
}

func (f *FakeFocusSession) notify() {
	f.mu.Lock()
	list := make([]func(), 0, len(f.subs))
	for _, fn := range f.subs {
		list = append(list, fn)
	}
	f.mu.Unlock()
	for _, fn := range list {
		fn()
	}
}

// focusRemaining — остаток сеанса на момент now; не меньше нуля.
func focusRemaining(st FocusSessionState, now time.Time) time.Duration {
	if !st.Running {
		return st.Duration
	}
	d := st.EndsAt.Sub(now)
	if d < 0 {
		return 0
	}
	return d
}

// focusClamp ограничивает длительность пределами модуля.
func focusClamp(d time.Duration) time.Duration {
	if d < FocusMinDuration {
		return FocusMinDuration
	}
	if d > FocusMaxDuration {
		return FocusMaxDuration
	}
	return d
}
