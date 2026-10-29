package input

// focusreq.go — фокус по просьбе виджета (internal/focusreq).
//
// На время доставки клавиши или щелчка движок принимает просьбы фокуса от
// виджетов: строка меню, открытая по Alt+буква, берёт фокус, чтобы стрелки и
// буква пункта дошли до неё, а закрывшись — возвращает его тому, у кого
// взяла. Без возврата набор текста после меню уходил бы строке меню.

import (
	"sync"

	"github.com/oops1/headless-gui/v3/internal/focusreq"
	"github.com/oops1/headless-gui/v3/widget"
)

// focusLoan — фокус, взятый виджетом по просьбе: кто взял и у кого.
type focusLoan struct {
	mu   sync.Mutex
	by   widget.Widget
	from widget.Widget
}

// serveFocusRequests объявляет движок исполнителем просьб на время
// доставки события.
func (d *Dispatcher) serveFocusRequests() (restore func()) {
	return focusreq.Serve(focusServer{d})
}

type focusServer struct{ d *Dispatcher }

func (s focusServer) RequestFocus(v any) {
	w, ok := v.(widget.Widget)
	if !ok || w == nil {
		return
	}
	d := s.d
	cur := d.focus.get()
	if cur == w {
		return
	}
	d.loan.mu.Lock()
	d.loan.by, d.loan.from = w, cur
	d.loan.mu.Unlock()
	d.setFocusInvalidating(w)
}

func (s focusServer) ReturnFocus(v any) {
	w, ok := v.(widget.Widget)
	if !ok || w == nil {
		return
	}
	d := s.d
	d.loan.mu.Lock()
	if d.loan.by != w {
		d.loan.mu.Unlock()
		return
	}
	from := d.loan.from
	d.loan.by, d.loan.from = nil, nil
	d.loan.mu.Unlock()
	// Фокус успели передать дальше (щелчок по другому виджету) — его не
	// трогаем: возвращать есть смысл, только пока он у взявшего.
	if d.focus.get() == w {
		d.setFocusInvalidating(from)
	}
}

// RequestFocus отдаёт фокус виджету w так, как это делает просьба самого
// виджета во время доставки события (internal/focusreq): прежний владелец
// запоминается, и ReturnFocus вернёт ему фокус. Зовётся приложением там, где
// виджет открыт НЕ событием движка — панель, поднятая по глобальной клавише
// (Win) из сессии, по таймеру или из другой горутины: событие в такой миг никто
// не доставляет, и виджет сам фокус попросить не может.
//
// Безопасно из любой горутины. На горутине кадра (и у незапущенного движка)
// выполняется сразу, из других — ставится в очередь Post и выполнится в
// начале следующего прохода цикла, в порядке вызовов относительно
// ReturnFocus. Фокус при этом, как и у SetFocus, инвалидирует области старого
// и нового владельца. nil и w, уже владеющий фокусом, ничего не меняют.
func (d *Dispatcher) RequestFocus(w widget.Widget) {
	if w == nil {
		return
	}
	d.onLoop(func() { focusServer{d}.RequestFocus(w) })
}

// ReturnFocus возвращает фокус тому, у кого его забрал RequestFocus для w, —
// когда программно открытая панель закрылась. Если фокус успели передать
// дальше (щелчок по другому виджету), его не трогает. Потокобезопасно так же,
// как RequestFocus.
func (d *Dispatcher) ReturnFocus(w widget.Widget) {
	if w == nil {
		return
	}
	d.onLoop(func() { focusServer{d}.ReturnFocus(w) })
}

// onLoop выполняет fn на горутине кадра, не блокируя вызывающего: на самой
// горутине кадра и у незапущенного движка — тут же, иначе через Post.
func (d *Dispatcher) onLoop(fn func()) {
	if d.host.InLoop() {
		fn()
		return
	}
	d.host.Post(fn)
}
