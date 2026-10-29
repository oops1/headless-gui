package desktop

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/testreset"
)

// Срабатывание таймера паузы, которое ждало замка, пока hide или pin
// останавливали таймер (Stop опоздал), — устаревшее: оно не трогает ни
// снятый таймер (прежде — Reset у nil и паника в горутине таймера), ни
// таймер следующего poke.
func TestFadeBar_StaleTimerFireIgnored(t *testing.T) {
	testreset.Globals(t)
	noop := func() {}

	// hide снял таймер, а старое срабатывание пришло после: пауза ещё идёт.
	f := &fadeBar{}
	f.poke(noop)
	gen := f.timerGen
	f.hide()
	f.mu.Lock()
	f.last = time.Now()
	f.mu.Unlock()
	f.expire(gen, noop) // не паникует и ничего не заводит
	if f.timer != nil {
		t.Fatal("устаревшее срабатывание завело таймер после hide")
	}

	// После hide новый poke завёл свой таймер: старое срабатывание его не
	// сбрасывает и затухание не запускает.
	f.poke(noop)
	cur := f.timer
	f.mu.Lock()
	f.last = time.Now().Add(-time.Hour) // пауза «истекла»
	f.mu.Unlock()
	f.expire(gen, noop)
	if f.timer != cur || f.live {
		t.Fatalf("устаревшее срабатывание тронуло таймер нового poke: timer %v, live %v", f.timer != cur, f.live)
	}
	f.hide()
}
