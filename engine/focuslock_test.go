package engine

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// focusProbe — виджет с фокусом, который может что-то сделать из SetFocused.
type focusProbe struct {
	widget.Base
	name    string
	focused bool
	log     *[]string
	onFocus func(bool)
}

func (p *focusProbe) SetFocused(f bool) {
	p.focused = f
	if p.log != nil {
		s := "-"
		if f {
			s = "+"
		}
		*p.log = append(*p.log, s+p.name)
	}
	if p.onFocus != nil {
		p.onFocus(f)
	}
}
func (p *focusProbe) IsFocused() bool         { return p.focused }
func (p *focusProbe) Draw(widget.DrawContext) {}

// withinFocusDeadline падает, если fn не вернулась вовремя: прежде фокус
// держал замок во время SetFocused и такой вызов висел навсегда.
func withinFocusDeadline(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { fn(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("перевод фокуса завис: SetFocused вызван под замком фокуса")
	}
}

// Виджет по потере фокуса переводит фокус дальше (поле адреса галереи
// PixGo 1.0.0) — не зависает, фокус у последнего, уведомления по порядку.
func TestFocus_SetFocusFromSetFocusedDoesNotDeadlock(t *testing.T) {
	testreset.Globals(t)
	e := New(100, 100, 30)
	defer e.Stop()
	var log []string
	a := &focusProbe{name: "a", log: &log}
	b := &focusProbe{name: "b", log: &log}
	c := &focusProbe{name: "c", log: &log}
	a.onFocus = func(f bool) {
		if !f {
			e.SetFocus(c) // по потере фокуса — дальше, в c
		}
	}
	e.SetFocus(a)
	withinFocusDeadline(t, func() { e.SetFocus(b) })
	if got := e.focus.get(); got != c {
		t.Fatalf("фокус у %v, ждали c", got)
	}
	if b.focused || !c.focused || a.focused {
		t.Fatalf("состояния: a=%v b=%v c=%v, ждали только c", a.focused, b.focused, c.focused)
	}
	want := []string{"+a", "-a", "+c"}
	if len(log) != len(want) {
		t.Fatalf("уведомления %v, ждали %v", log, want)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Fatalf("уведомления %v, ждали %v", log, want)
		}
	}
}

// Виджет в SetFocused(true) спрашивает, у кого фокус (CaretRect, Focused),
// — получает себя и не зависает.
func TestFocus_QueryFromSetFocusedDoesNotDeadlock(t *testing.T) {
	testreset.Globals(t)
	e := New(100, 100, 30)
	defer e.Stop()
	p := &focusProbe{name: "p"}
	var seen widget.Widget
	p.onFocus = func(f bool) {
		if f {
			seen = e.focus.get()
		}
	}
	withinFocusDeadline(t, func() { e.SetFocus(p) })
	if seen != p {
		t.Fatalf("из SetFocused(true) фокус виден у %v, ждали себя", seen)
	}
}

// Виджеты, бесконечно перекидывающие фокус друг другу, не вешают движок:
// число кругов уведомлений ограничено.
func TestFocus_PingPongIsBounded(t *testing.T) {
	testreset.Globals(t)
	e := New(100, 100, 30)
	defer e.Stop()
	a := &focusProbe{name: "a"}
	b := &focusProbe{name: "b"}
	a.onFocus = func(f bool) {
		if f {
			e.SetFocus(b)
		}
	}
	b.onFocus = func(f bool) {
		if f {
			e.SetFocus(a)
		}
	}
	withinFocusDeadline(t, func() { e.SetFocus(a) })
}
