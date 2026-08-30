package widget

import (
	"testing"
	"time"
)

// Анимация без покадрового колбэка — законный таймер.
//
// Такая анимация ничего не рисует: она нужна, чтобы движок позвал OnDone
// через заданный срок. На ней держатся задержки предпросмотра окна — до
// появления панели и между обновлениями миниатюры. StepAnimations звала
// покадровый колбэк без проверки и на таком таймере падала.
func TestAnimate_NilTickIsATimer(t *testing.T) {
	StopAllAnimations()
	t.Cleanup(StopAllAnimations)

	done := make(chan struct{}, 1)
	a := Animate(20*time.Millisecond, nil, nil)
	a.OnDone = func() { done <- struct{}{} }

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		StepAnimations(time.Now()) // без правки здесь была паника
		select {
		case <-done:
			return
		default:
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("таймер без покадрового колбэка не сработал")
}

// Таймер можно перезаводить из его же OnDone: так строится повторяющееся
// действие без зацикленной анимации (зациклённая по кругу зовёт покадровый
// колбэк и никогда — OnDone, потому что штатно не завершается).
func TestAnimate_TimerCanRearmItself(t *testing.T) {
	StopAllAnimations()
	t.Cleanup(StopAllAnimations)

	fired := 0
	var arm func()
	arm = func() {
		a := Animate(10*time.Millisecond, nil, nil)
		a.OnDone = func() {
			fired++
			if fired < 3 {
				arm()
			}
		}
	}
	arm()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && fired < 3 {
		StepAnimations(time.Now())
		time.Sleep(2 * time.Millisecond)
	}
	if fired < 3 {
		t.Errorf("цепочка таймеров сработала %d раз из трёх", fired)
	}
}
