//go:build linux && !android

package x11

import "time"

// Автоповтор клавиши на X11.
//
// X-сервер по умолчанию изображает автоповтор честно, но нелепо: на каждый
// повтор он шлёт пару KeyRelease + KeyPress с ОДНИМ временем и одним кодом
// клавиши. Приложение видело при удержании стрелки поток «отпустили — нажали»:
// вместо повтора приходили новые нажатия (KeyEvent.Repeat всегда false), а
// клавиши, которые по нажатию что-то переключают, срабатывали на каждый тик.
//
// Расширение XKB умеет отдавать повтор прямо (DetectableAutoRepeat), но движок
// говорит с сервером голым протоколом и XKB не использует — подключать
// расширение ради одного флага не стали. Вместо этого применён стандартный
// приём тулкитов (SDL, GLFW): на KeyRelease заглянуть, не следует ли за ним
// KeyPress с тем же временем и кодом. Если следует — это автоповтор: отпускание
// проглатывается, нажатие помечается повтором.
//
// Дополнительно ведётся таблица удерживаемых клавиш: нажатие клавиши, которая
// уже считается нажатой, — повтор в любом случае. Так же выглядит повтор на
// сервере с включённым DetectableAutoRepeat (отпусканий между нажатиями нет),
// и такой сервер работает без изменений.

// x11RepeatPeek — сколько ждать парное KeyPress после KeyRelease. Сервер шлёт
// обе половины одной записью в сокет, так что обычно они уже ждут в буфере;
// запас — на медленное соединение. Платится один раз на каждое обычное
// отпускание клавиши.
const x11RepeatPeek = 2 * time.Millisecond

// x11KeyState — состояние клавиш и заглянутого вперёд события. Принадлежит
// горутине, читающей события окна, и блокировок не имеет.
type x11KeyState struct {
	down    [256]bool // клавиша (по коду) сейчас нажата
	pend    [32]byte  // событие, прочитанное при заглядывании, но ещё не обработанное
	hasPend bool
}

// note учитывает событие клавиши или потерю фокуса. Для KeyPress возвращает
// true, если клавиша уже была нажата, то есть это автоповтор.
func (k *x11KeyState) note(evType, keycode byte) (repeat bool) {
	switch evType {
	case 2: // KeyPress
		repeat = k.down[keycode]
		k.down[keycode] = true
	case 3: // KeyRelease
		k.down[keycode] = false
	case 10: // FocusOut: отпускания придут не нам
		k.down = [256]bool{}
	}
	return repeat
}

// isX11RepeatPair сообщает, что press следует сразу за release как вторая
// половина автоповтора: нажатие того же кода клавиши в то же время сервера.
// Оба события — 32-байтовые X11-события: тип в buf[0], код клавиши в buf[1],
// время в buf[4:8].
func isX11RepeatPair(release, press []byte) bool {
	if len(release) < 8 || len(press) < 8 {
		return false
	}
	return release[0]&0x7F == 3 && press[0]&0x7F == 2 &&
		release[1] == press[1] &&
		release[4] == press[4] && release[5] == press[5] &&
		release[6] == press[6] && release[7] == press[7]
}

// readEvent читает следующее событие соединения в buf (32 байта),
// склеивая пары автоповтора: KeyRelease, за которым тут же идёт парное
// KeyPress, не отдаётся — вместо него возвращается само KeyPress. false —
// соединение закрыто или сломано.
func (w *X11Window) readEvent(buf []byte) bool {
	if w.keys.hasPend {
		copy(buf, w.keys.pend[:])
		w.keys.hasPend = false
	} else if !w.readFull(buf) {
		return false
	}
	if buf[0]&0x7F != 3 { // не KeyRelease — заглядывать не за чем
		return true
	}
	if !w.peekEvent(w.keys.pend[:], x11RepeatPeek) {
		return true
	}
	if isX11RepeatPair(buf, w.keys.pend[:]) {
		copy(buf, w.keys.pend[:]) // отпускание проглочено, дальше пойдёт повтор
		return true
	}
	w.keys.hasPend = true // другое событие: обработается следующим вызовом
	return true
}

// peekEvent читает следующее событие в buf, ожидая не дольше d до его начала.
// false — за это время ничего не пришло (или соединение закрыто: об этом
// скажет следующее чтение). Уже начавшееся событие дочитывается до конца без
// ограничения по времени: потерять половину события значило бы сбить разбор
// всего потока.
func (w *X11Window) peekEvent(buf []byte, d time.Duration) bool {
	c := w.conn
	if c == nil {
		return false
	}
	_ = c.SetReadDeadline(time.Now().Add(d))
	got := 0
	var err error
	for got < len(buf) && err == nil {
		var n int
		n, err = c.Read(buf[got:])
		got += n
	}
	_ = c.SetReadDeadline(time.Time{})
	if got == 0 {
		return false
	}
	if got < len(buf) {
		return w.readFull(buf[got:])
	}
	return true
}

// SetOnKeyDownRepeat подписывает приёмник нажатий, отличающий автоповтор от
// нового нажатия. Реализует keyRepeatSource.
func (w *X11Window) SetOnKeyDownRepeat(fn func(vk int, repeat bool)) {
	w.onKeyDownRepeat = fn
}
