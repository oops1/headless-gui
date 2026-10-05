package widget

import (
	"image"
	"math"
	"testing"
	"time"
)

// escPanel — виджет, закрывающийся по Esc, пока open.
type escPanel struct {
	Base
	name string
	open bool
	log  *[]string
}

func (e *escPanel) Draw(DrawContext) {}
func (e *escPanel) DismissOnEscape() bool {
	if !e.open {
		return false
	}
	e.open = false
	*e.log = append(*e.log, e.name)
	return true
}

func TestDismissOnEscape_ClosesTopmostFirstOneAtATime(t *testing.T) {
	var log []string
	a := &escPanel{name: "a", open: true, log: &log}
	b := &escPanel{name: "b", open: true, log: &log}
	c := &escPanel{name: "c", open: false, log: &log}
	host := &escPanel{name: "root", log: &log}
	host.AddChild(a)
	host.AddChild(b)
	host.AddChild(c)

	// c закрыт, b выше a: первый Esc закрывает b, второй — a, третий ничего.
	if !DismissOnEscape(host) || len(log) != 1 || log[0] != "b" {
		t.Fatalf("первый Esc: %v", log)
	}
	if !DismissOnEscape(host) || len(log) != 2 || log[1] != "a" {
		t.Fatalf("второй Esc: %v", log)
	}
	if DismissOnEscape(host) {
		t.Error("Esc без открытых панелей сообщил о закрытии")
	}
	if DismissOnEscape(nil) {
		t.Error("nil-корень закрыл что-то")
	}
}

func TestEasingByName(t *testing.T) {
	for _, name := range []string{"linear", "in-quad", "out-quad", "in-out-quad", "in-cubic", "out-cubic",
		"in-out-cubic", "in-sine", "out-sine", "in-out-sine", "out-back", "out-elastic", "out-bounce"} {
		e := EasingByName(name)
		if e == nil {
			t.Errorf("кривая %q не найдена", name)
			continue
		}
		if math.Abs(e(0)) > 1e-9 || math.Abs(e(1)-1) > 1e-9 {
			t.Errorf("кривая %q: f(0)=%v f(1)=%v", name, e(0), e(1))
		}
	}
	if EasingByName("") != nil || EasingByName("нет-такой") != nil {
		t.Error("пустое и незнакомое имя должны давать nil (линейная)")
	}
	if got := EasingByName("out-cubic")(0.5); got != EaseOutCubic(0.5) {
		t.Errorf("out-cubic вернула другую кривую: %v", got)
	}
}

// TestAnimateWakesWithoutFullInvalidation — приёмник, объявивший wake, на
// заведённую анимацию не получает полной инвалидации; старый приёмник — получает.
func TestAnimateWakesWithoutFullInvalidation(t *testing.T) {
	defer StopAllAnimations()

	var fullNew, wakeNew, fullOld int
	hNew := RegisterUINotifierWake(func() { fullNew++ }, func(image.Rectangle) {}, func() { wakeNew++ })
	hOld := RegisterUINotifier(func() { fullOld++ }, func(image.Rectangle) {})
	defer UnregisterUINotifier(hNew)
	defer UnregisterUINotifier(hOld)

	Animate(100*time.Millisecond, nil, func(float64) {})

	if fullNew != 0 || wakeNew != 1 {
		t.Errorf("приёмник с wake: full=%d wake=%d, ждали 0 и 1", fullNew, wakeNew)
	}
	if fullOld != 1 {
		t.Errorf("старый приёмник получил full=%d, ждали 1 (совместимость)", fullOld)
	}
}
