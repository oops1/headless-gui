package widget

import (
	"image/color"
	"sync"
	"testing"
	"time"
)

// Регистрация анимации из чужой горутины не гонится с шагом анимаций.
//
// Контракт Animate/AnimateOwned обещает вызов из любой горутины, и часы на
// панели задач — ровно тот случай, ради которого обещание писалось: тик
// взводится не в горутине рендера. Потребитель сообщил, что go test -race
// становится флаки, как только на экране что-то анимируется само.
func TestAnimations_RegisterWhileStepping(t *testing.T) {
	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Горутина рендера: шагает анимации, как это делает Engine.loop.
	wg.Add(1)
	go func() {
		defer wg.Done()
		now := time.Now()
		for {
			select {
			case <-stop:
				return
			default:
			}
			now = now.Add(16 * time.Millisecond)
			StepAnimations(now)
		}
	}()

	// Чужие горутины: регистрируют анимации, как это делают часы и виджеты.
	owner := NewPanel(color.RGBA{A: 255})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for n := 0; n < 200; n++ {
				a := Animate(20*time.Millisecond, nil, func(float64) {})
				b := AnimateOwned(owner, "tag", 20*time.Millisecond, nil, func(float64) {})
				a.Stop()
				b.Stop()
			}
		}(i)
	}

	// Ждём регистраторов, затем гасим шагающую горутину.
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	time.Sleep(150 * time.Millisecond)
	close(stop)
	<-done
}
