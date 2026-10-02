//go:build linux && !android

package window

import (
	"encoding/binary"
	"sync/atomic"
	"testing"
	"time"
)

// Под Wayland повтор удерживаемой клавиши — забота клиента: композитор
// присылает нажатие и отпускание, а между ними молчит. Пока этого не было,
// удержание стрелки или Backspace срабатывало ровно один раз.

func TestWlRepeater_FiresAfterDelayThenPeriodically(t *testing.T) {
	r := newWlRepeater()
	r.setInfo(100, 20) // 100 раз в секунду, задержка 20 мс

	var n int32
	r.start(31, func() { atomic.AddInt32(&n, 1) })
	defer r.cancel()

	// До задержки повторов нет: первое нажатие доставил сам композитор.
	time.Sleep(10 * time.Millisecond)
	if got := atomic.LoadInt32(&n); got != 0 {
		t.Errorf("повтор начался раньше задержки: %d", got)
	}
	time.Sleep(120 * time.Millisecond)
	if got := atomic.LoadInt32(&n); got < 3 {
		t.Errorf("повторов %d, ждал несколько", got)
	}
}

// Отпускание гасит повтор; отпускание ДРУГОЙ клавиши — нет. Порядок событий
// в жизни именно такой: отпускание предыдущей клавиши нередко приходит уже
// после нажатия следующей.
func TestWlRepeater_StopsOnlyItsOwnKey(t *testing.T) {
	r := newWlRepeater()
	r.setInfo(100, 10)

	var n int32
	r.start(31, func() { atomic.AddInt32(&n, 1) })
	defer r.cancel()

	r.stopKey(42) // отпустили соседнюю
	if !r.active() {
		t.Fatal("чужое отпускание погасило повтор")
	}
	time.Sleep(60 * time.Millisecond)
	before := atomic.LoadInt32(&n)
	if before == 0 {
		t.Fatal("повтор не идёт")
	}

	r.stopKey(31) // отпустили свою
	if r.active() {
		t.Error("повтор не остановлен")
	}
	time.Sleep(50 * time.Millisecond)
	if after := atomic.LoadInt32(&n); after != before {
		t.Errorf("после отпускания пришло ещё %d повторов", after-before)
	}
}

// Новое нажатие отменяет повтор предыдущей клавиши: повторяется всегда одна,
// последняя, — так делают и композиторы, и X-сервер.
func TestWlRepeater_NewKeyReplacesOld(t *testing.T) {
	r := newWlRepeater()
	r.setInfo(100, 10)

	var first, second int32
	r.start(31, func() { atomic.AddInt32(&first, 1) })
	time.Sleep(40 * time.Millisecond)
	r.start(42, func() { atomic.AddInt32(&second, 1) })
	defer r.cancel()

	got := atomic.LoadInt32(&first)
	time.Sleep(60 * time.Millisecond)
	if now := atomic.LoadInt32(&first); now != got {
		t.Errorf("прежняя клавиша повторяется после нажатия новой: было %d, стало %d", got, now)
	}
	if atomic.LoadInt32(&second) == 0 {
		t.Error("новая клавиша не повторяется")
	}
}

// rate == 0 — композитор выключил повтор; спорить с ним нельзя.
func TestWlRepeater_ZeroRateDisables(t *testing.T) {
	r := newWlRepeater()
	r.setInfo(0, 100)

	var n int32
	r.start(31, func() { atomic.AddInt32(&n, 1) })
	defer r.cancel()

	if r.active() {
		t.Error("повтор запущен, хотя композитор его выключил")
	}
	time.Sleep(80 * time.Millisecond)
	if got := atomic.LoadInt32(&n); got != 0 {
		t.Errorf("пришло %d повторов при выключенном повторе", got)
	}
}

// У композитора без repeat_info (версия протокола младше 4) берутся
// привычные значения рабочего стола, а не ноль.
func TestWlRepeater_DefaultsWithoutRepeatInfo(t *testing.T) {
	r := newWlRepeater()
	if r.rate != wlDefaultRepeatRate || r.delay != wlDefaultRepeatDelay {
		t.Errorf("по умолчанию rate=%d delay=%v, ждал %d и %v",
			r.rate, r.delay, wlDefaultRepeatRate, wlDefaultRepeatDelay)
	}
}

// ─── Разбор событий клавиатуры ──────────────────────────────────────────────

func TestWayland_RepeatInfoParsed(t *testing.T) {
	c := newWlTestWindow(t)
	body := make([]byte, 8)
	binary.LittleEndian.PutUint32(body[0:4], 40)  // 40 раз в секунду
	binary.LittleEndian.PutUint32(body[4:8], 300) // задержка 300 мс
	c.w.keyboardID = 21
	c.w.handleEvent(c.w.keyboardID, wlKeyboardEvRepeatInfo, body)

	if c.w.repeat.rate != 40 || c.w.repeat.delay != 300*time.Millisecond {
		t.Errorf("repeat_info разобран как rate=%d delay=%v",
			c.w.repeat.rate, c.w.repeat.delay)
	}
}

// Уход фокуса гасит повтор: отпускания после него уже не придёт, и клавиша
// повторялась бы вечно.
func TestWayland_LeaveCancelsRepeat(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.keyboardID = 21
	c.w.repeat.setInfo(100, 10)
	c.w.repeat.start(31, func() {})
	if !c.w.repeat.active() {
		t.Fatal("повтор не запустился")
	}

	c.w.handleEvent(c.w.keyboardID, wlKeyboardEvLeave, make([]byte, 8))
	if c.w.repeat.active() {
		t.Error("повтор пережил потерю фокуса")
	}
}

// Нажатие доставляется с пометкой «не повтор», автоповтор — с пометкой
// «повтор»; порядок внутри события прежний: сначала код, потом символ.
func TestWayland_DeliverKeyMarksRepeat(t *testing.T) {
	c := newWlTestWindow(t)
	var marks []bool
	c.w.SetOnKeyDownRepeat(func(vk int, repeat bool) { marks = append(marks, repeat) })

	c.w.deliverKey(31, false) // evdev 31 = «S»
	c.w.deliverKey(31, true)

	if len(marks) != 2 || marks[0] || !marks[1] {
		t.Errorf("пометки повтора: %v, ждал [false true]", marks)
	}
}

// Модификатор не повторяется: повторять нечего, а поток событий он бы засорил.
func TestWayland_ModifiersDoNotRepeat(t *testing.T) {
	c := newWlTestWindow(t)
	c.w.keyboardID = 21
	c.w.repeat.setInfo(100, 10)

	key := func(evdev uint32, pressed bool) {
		body := make([]byte, 16)
		binary.LittleEndian.PutUint32(body[8:12], evdev)
		if pressed {
			binary.LittleEndian.PutUint32(body[12:16], 1)
		}
		c.w.handleEvent(c.w.keyboardID, wlKeyboardEvKey, body)
	}

	key(42, true) // evdev 42 — левый Shift
	if c.w.repeat.active() {
		t.Error("модификатор поставлен на повтор")
	}
	key(42, false)

	key(31, true) // «S» — повторяется
	defer c.w.repeat.cancel()
	if !c.w.repeat.active() {
		t.Error("обычная клавиша не поставлена на повтор")
	}
}
