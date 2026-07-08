package tests

import (
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// linear — линейная кривая для детерминированных проверок прогресса.
func linear(t float64) float64 { return t }

// TestAnim_DeterministicProgress: с руками заданными моментами времени
// прогресс точный, последний тик ровно 1.0, OnDone один раз.
func TestAnim_DeterministicProgress(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	var ticks []float64
	var doneCount int
	a := widget.Animate(200*time.Millisecond, linear, func(v float64) {
		ticks = append(ticks, v)
	})
	a.OnDone = func() { doneCount++ }

	t0 := time.Now()
	widget.StepAnimations(t0)                             // старт: t=0
	widget.StepAnimations(t0)                             // повторно тот же момент
	widget.StepAnimations(t0.Add(50 * time.Millisecond))  // t=0.25
	widget.StepAnimations(t0.Add(100 * time.Millisecond)) // t=0.5
	widget.StepAnimations(t0.Add(150 * time.Millisecond)) // t=0.75
	still := widget.StepAnimations(t0.Add(200 * time.Millisecond)) // t=1.0, done

	if len(ticks) < 2 {
		t.Fatalf("слишком мало тиков: %v", ticks)
	}
	if ticks[0] != 0 {
		t.Fatalf("первый тик должен быть 0, got %v", ticks[0])
	}
	last := ticks[len(ticks)-1]
	if last != 1.0 {
		t.Fatalf("последний тик должен быть ровно 1.0, got %v", last)
	}
	if doneCount != 1 {
		t.Fatalf("OnDone должен вызваться ровно один раз, got %d", doneCount)
	}
	if still {
		t.Fatalf("после завершения анимаций быть не должно")
	}
	if a.Running() {
		t.Fatalf("завершённая анимация не должна быть Running")
	}
	// Проверяем точные промежуточные значения (0.25, 0.5, 0.75 присутствуют).
	want := map[float64]bool{0.25: false, 0.5: false, 0.75: false}
	for _, v := range ticks {
		if _, ok := want[v]; ok {
			want[v] = true
		}
	}
	for v, seen := range want {
		if !seen {
			t.Errorf("ожидался промежуточный прогресс %v, тики: %v", v, ticks)
		}
	}
}

// TestAnim_NilCurveLinear: nil-кривая = линейная.
func TestAnim_NilCurveLinear(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	var last float64
	widget.Animate(100*time.Millisecond, nil, func(v float64) { last = v })
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(50 * time.Millisecond))
	if last < 0.49 || last > 0.51 {
		t.Fatalf("nil-кривая должна быть линейной: t=0.5 ожидается, got %v", last)
	}
	widget.StepAnimations(t0.Add(100 * time.Millisecond))
	if last != 1.0 {
		t.Fatalf("финальный тик = 1.0, got %v", last)
	}
}

// TestAnim_OwnedReplaces: AnimateOwned заменяет предыдущую с тем же (owner,tag)
// и НЕ трогает чужие.
func TestAnim_OwnedReplaces(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	ownerA := widget.NewButton("a")
	ownerB := widget.NewButton("b")

	var firstTicks, secondTicks, otherTicks int
	first := widget.AnimateOwned(ownerA, "fade", 200*time.Millisecond, linear, func(float64) { firstTicks++ })
	other := widget.AnimateOwned(ownerB, "fade", 200*time.Millisecond, linear, func(float64) { otherTicks++ })

	t0 := time.Now()
	widget.StepAnimations(t0) // обе стартуют
	if firstTicks == 0 || otherTicks == 0 {
		t.Fatalf("обе анимации должны были тикнуть: first=%d other=%d", firstTicks, otherTicks)
	}

	// Заменяем анимацию ownerA с тем же тегом — first должна остановиться.
	second := widget.AnimateOwned(ownerA, "fade", 200*time.Millisecond, linear, func(float64) { secondTicks++ })
	if first.Running() {
		t.Fatalf("первая анимация (owner,tag) должна быть остановлена заменой")
	}
	if !other.Running() {
		t.Fatalf("чужая анимация (другой owner) не должна быть тронута")
	}

	beforeFirst := firstTicks
	widget.StepAnimations(t0.Add(50 * time.Millisecond))
	if firstTicks != beforeFirst {
		t.Fatalf("остановленная анимация не должна тикать: %d → %d", beforeFirst, firstTicks)
	}
	if secondTicks == 0 {
		t.Fatalf("вторая анимация должна тикать после регистрации")
	}
	if otherTicks == 0 {
		t.Fatalf("чужая анимация должна продолжать тикать")
	}
	_ = second
}

// TestAnim_StopFromTick: Stop из собственного тика.
func TestAnim_StopFromTick(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	var ticks int
	var a *widget.Animation
	a = widget.Animate(1*time.Second, linear, func(float64) {
		ticks++
		if ticks == 2 {
			a.Stop() // из собственного тика — без дедлока
		}
	})

	t0 := time.Now()
	widget.StepAnimations(t0)                          // tick 1
	widget.StepAnimations(t0.Add(10 * time.Millisecond)) // tick 2 → Stop
	if a.Running() {
		t.Fatalf("анимация должна быть остановлена из тика")
	}
	still := widget.StepAnimations(t0.Add(20 * time.Millisecond))
	if ticks != 2 {
		t.Fatalf("после Stop тиков быть не должно: got %d", ticks)
	}
	if still {
		t.Fatalf("активных анимаций после Stop быть не должно")
	}
}

// TestAnim_ChainFromTick: запуск новой анимации из тика — новая живёт и НЕ
// получает шаг в том же Step.
func TestAnim_ChainFromTick(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	var childTicks int
	var spawned bool
	widget.Animate(1*time.Second, linear, func(float64) {
		if !spawned {
			spawned = true
			widget.Animate(1*time.Second, linear, func(float64) { childTicks++ })
		}
	})

	t0 := time.Now()
	widget.StepAnimations(t0) // родитель тикает, рождает ребёнка
	if childTicks != 0 {
		t.Fatalf("новорождённая анимация НЕ должна получать шаг в том же Step: got %d", childTicks)
	}
	widget.StepAnimations(t0.Add(10 * time.Millisecond)) // теперь ребёнок тикает
	if childTicks == 0 {
		t.Fatalf("ребёнок должен тикать в следующем Step")
	}
}

// TestAnim_ChainFromOnDone: OnDone запускает новую анимацию — цепочка живёт.
func TestAnim_ChainFromOnDone(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	var secondTicks int
	first := widget.Animate(100*time.Millisecond, linear, func(float64) {})
	first.OnDone = func() {
		widget.Animate(100*time.Millisecond, linear, func(float64) { secondTicks++ })
	}

	t0 := time.Now()
	widget.StepAnimations(t0)
	still := widget.StepAnimations(t0.Add(100 * time.Millisecond)) // first done, OnDone запускает вторую
	if !still {
		t.Fatalf("после OnDone-цепочки должны остаться активные анимации")
	}
	widget.StepAnimations(t0.Add(110 * time.Millisecond))
	if secondTicks == 0 {
		t.Fatalf("вторая анимация из OnDone должна тикать")
	}
}

// TestAnim_Loop: Loop перезапускает анимацию, прогресс корректен, OnDone не
// вызывается.
func TestAnim_Loop(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	var maxVal float64
	var doneCount int
	a := widget.Animate(100*time.Millisecond, linear, func(v float64) {
		if v > maxVal {
			maxVal = v
		}
	})
	a.Loop = true
	a.OnDone = func() { doneCount++ }

	t0 := time.Now()
	widget.StepAnimations(t0)                          // цикл 1: t=0
	widget.StepAnimations(t0.Add(50 * time.Millisecond)) // t=0.5
	still := widget.StepAnimations(t0.Add(100 * time.Millisecond)) // t=1.0 → перезапуск
	if !still {
		t.Fatalf("Loop-анимация должна оставаться активной")
	}
	if doneCount != 0 {
		t.Fatalf("Loop не должен вызывать OnDone: got %d", doneCount)
	}
	// Второй цикл: часы сброшены на t0+100ms.
	widget.StepAnimations(t0.Add(150 * time.Millisecond)) // цикл 2: t=0.5
	if !a.Running() {
		t.Fatalf("Loop-анимация должна быть Running после нескольких циклов")
	}
	if maxVal != 1.0 {
		t.Fatalf("прогресс цикла должен достигать 1.0, got %v", maxVal)
	}
}

// TestAnim_AutoReverse: 0→1→0 считается одним циклом; без Loop завершается
// после обратной фазы.
func TestAnim_AutoReverse(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	var vals []float64
	var doneCount int
	a := widget.Animate(100*time.Millisecond, linear, func(v float64) {
		vals = append(vals, v)
	})
	a.AutoReverse = true
	a.OnDone = func() { doneCount++ }

	t0 := time.Now()
	widget.StepAnimations(t0)                          // прямая: t=0
	widget.StepAnimations(t0.Add(50 * time.Millisecond)) // прямая: t=0.5
	widget.StepAnimations(t0.Add(100 * time.Millisecond)) // конец прямой → старт обратной, часы сброшены
	widget.StepAnimations(t0.Add(150 * time.Millisecond)) // обратная: t=0.5
	still := widget.StepAnimations(t0.Add(200 * time.Millisecond)) // конец обратной → done

	if doneCount != 1 {
		t.Fatalf("AutoReverse: OnDone ровно один раз после обратной фазы, got %d", doneCount)
	}
	if still {
		t.Fatalf("AutoReverse без Loop должна завершиться")
	}
	// Прогресс должен подняться к ~1.0 и вернуться к ~0.0.
	var hi, lo float64 = 0, 1
	for _, v := range vals {
		if v > hi {
			hi = v
		}
		if v < lo {
			lo = v
		}
	}
	if hi < 0.99 {
		t.Fatalf("прямая фаза должна достигать ~1.0, max=%v", hi)
	}
	if lo > 0.01 {
		t.Fatalf("обратная фаза должна возвращаться к ~0.0, min=%v", lo)
	}
	// Финальный тик обратной фазы = 0.0.
	if vals[len(vals)-1] != 0.0 {
		t.Fatalf("финал AutoReverse = 0.0, got %v", vals[len(vals)-1])
	}
}

// TestAnim_AutoReverseLoop: AutoReverse+Loop — несколько полных циклов.
func TestAnim_AutoReverseLoop(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	var cycles, doneCount int
	a := widget.Animate(100*time.Millisecond, linear, func(float64) {})
	a.AutoReverse = true
	a.Loop = true
	a.OnDone = func() { doneCount++ }

	t0 := time.Now()
	// Один полный AutoReverse-цикл = 200ms (100 прямая + 100 обратная).
	for i := 0; i < 5; i++ {
		widget.StepAnimations(t0.Add(time.Duration(i*100) * time.Millisecond))
	}
	if doneCount != 0 {
		t.Fatalf("AutoReverse+Loop не вызывает OnDone: got %d", doneCount)
	}
	if !a.Running() {
		t.Fatalf("AutoReverse+Loop должна оставаться активной")
	}
	_ = cycles
}

// TestAnim_RaceSmoke: параллельные Animate/Step под -race.
func TestAnim_RaceSmoke(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Горутина шагов.
	wg.Add(1)
	go func() {
		defer wg.Done()
		t0 := time.Now()
		for {
			select {
			case <-stop:
				return
			default:
				widget.StepAnimations(time.Now())
				widget.AnimationsActive()
				_ = t0
				time.Sleep(time.Millisecond)
			}
		}
	}()

	// Несколько горутин регистрируют анимации.
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				a := widget.Animate(20*time.Millisecond, widget.EaseOutQuad, func(float64) {})
				if i%3 == 0 {
					a.Stop()
				}
				owner := &struct{ int }{id}
				widget.AnimateOwned(owner, "x", 20*time.Millisecond, nil, func(float64) {})
				time.Sleep(time.Millisecond)
			}
		}(g)
	}

	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
	widget.StopAllAnimations()
	if widget.AnimationsActive() {
		t.Fatalf("после StopAllAnimations активных быть не должно")
	}
}

// TestAnim_EngineIntegration: engine.New + ProgressBar, Animate двигает
// SetValue; RenderCount растёт, пока анимация идёт, и останавливается после.
func TestAnim_EngineIntegration(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	eng := engine.New(320, 200, 50)
	eng.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 30, A: 255})
	root.SetBounds(image.Rect(0, 0, 320, 200))
	pb := widget.NewProgressBar()
	pb.SetBounds(image.Rect(10, 10, 300, 40))
	root.AddChild(pb)
	eng.SetRoot(root)
	eng.Start()
	defer eng.Stop()

	if !waitCount(eng, 1) {
		t.Fatal("первый кадр не отрендерился")
	}
	time.Sleep(100 * time.Millisecond)
	base := eng.RenderCount()

	// Анимация ~300ms, двигает значение прогресс-бара.
	// OnDone НЕ выставляем после старта: движок читает поля анимации в своей
	// горутине, а запись публичного поля в живую анимацию — гонка по дизайну
	// (поля конфигурируются ДО регистрации). Завершение детектируем через
	// Running() — потокобезопасный опрос под мьютексом аниматора.
	a := widget.AnimateFloat(0, 1, 300*time.Millisecond, widget.EaseLinear, func(v float64) {
		pb.SetValue(v)
	})

	// Ждём завершения анимации (>2 кадров при 50fps укладывается в 300ms).
	deadline := time.Now().Add(3 * time.Second)
	for a.Running() {
		if time.Now().After(deadline) {
			t.Fatal("анимация не завершилась")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Во время анимации кадры рендерились.
	afterAnim := eng.RenderCount()
	if afterAnim < base+3 {
		t.Fatalf("во время анимации RenderCount должен вырасти: %d → %d", base, afterAnim)
	}
	if pb.Value() < 0.99 {
		t.Fatalf("прогресс-бар должен дойти до ~1.0, got %v", pb.Value())
	}

	// После завершения кадры замирают (анимация больше не держит цикл).
	time.Sleep(150 * time.Millisecond)
	settled := eng.RenderCount()
	time.Sleep(300 * time.Millisecond)
	if got := eng.RenderCount(); got > settled+1 {
		t.Fatalf("после завершения анимации кадры должны замереть: %d → %d", settled, got)
	}
}
