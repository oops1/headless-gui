package engine

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
func (e *Engine) serveFocusRequests() (restore func()) {
	return focusreq.Serve(focusServer{e})
}

type focusServer struct{ e *Engine }

func (s focusServer) RequestFocus(v any) {
	w, ok := v.(widget.Widget)
	if !ok || w == nil {
		return
	}
	e := s.e
	cur := e.focus.get()
	if cur == w {
		return
	}
	e.loan.mu.Lock()
	e.loan.by, e.loan.from = w, cur
	e.loan.mu.Unlock()
	e.setFocusInvalidating(w)
}

func (s focusServer) ReturnFocus(v any) {
	w, ok := v.(widget.Widget)
	if !ok || w == nil {
		return
	}
	e := s.e
	e.loan.mu.Lock()
	if e.loan.by != w {
		e.loan.mu.Unlock()
		return
	}
	from := e.loan.from
	e.loan.by, e.loan.from = nil, nil
	e.loan.mu.Unlock()
	// Фокус успели передать дальше (щелчок по другому виджету) — его не
	// трогаем: возвращать есть смысл, только пока он у взявшего.
	if e.focus.get() == w {
		e.setFocusInvalidating(from)
	}
}
