package widget

import (
	"testing"
	"time"
)

// GG-68: анимация шагает на том цикле, на горутине которого её завели.
//
// Реестр анимаций общий, а каждый движок шагал его целиком: тик анимации в
// диалоге вторичного движка мог выполниться на горутине главного —
// параллельно с обработчиками диалога.

type testLoop struct{ name string }

// animOn заводит анимацию с часовой длительностью на цикле key (nil — без
// цикла) и возвращает счётчик её тиков.
func animOn(key any) *int {
	n := new(int)
	if key != nil {
		unbind := BindAnimationLoop(key)
		defer unbind()
	}
	Animate(time.Hour, nil, func(float64) { *n++ })
	return n
}

// Цикл шагает свои анимации и не трогает чужие.
func TestAnimLoop_StepsOnlyOwn(t *testing.T) {
	StopAllAnimations()
	defer StopAllAnimations()
	a, b := &testLoop{"a"}, &testLoop{"b"}
	ta := animOn(a)
	tb := animOn(b)
	now := time.Now()

	StepAnimationsFor(b, now)
	if *ta != 0 || *tb != 1 {
		t.Fatalf("шаг цикла b: тиков a=%d b=%d, ждал 0 и 1", *ta, *tb)
	}
	StepAnimationsFor(a, now)
	if *ta != 1 || *tb != 1 {
		t.Fatalf("шаг цикла a: тиков a=%d b=%d, ждал 1 и 1", *ta, *tb)
	}
}

// Ничью анимацию (заведённую вне цикла) забирает первый шагнувший цикл, и
// другой её уже не шагает.
func TestAnimLoop_UnboundClaimedByFirst(t *testing.T) {
	StopAllAnimations()
	defer StopAllAnimations()
	a, b := &testLoop{"a"}, &testLoop{"b"}
	tc := animOn(nil)
	now := time.Now()

	StepAnimationsFor(a, now)
	StepAnimationsFor(b, now)
	StepAnimationsFor(a, now)
	if *tc != 2 {
		t.Fatalf("ничья анимация получила %d тиков, ждал 2 (оба — от цикла a)", *tc)
	}
	if StepAnimationsFor(b, now) {
		t.Fatal("у цикла b нет своих анимаций, а шаг сообщил об активных")
	}
}

// StepAnimations без цикла шагает все — для тестов и приложений без движка.
func TestAnimLoop_StepAllStepsEverything(t *testing.T) {
	StopAllAnimations()
	defer StopAllAnimations()
	ta := animOn(&testLoop{"a"})
	tn := animOn(nil)

	StepAnimations(time.Now())
	if *ta != 1 || *tn != 1 {
		t.Fatalf("StepAnimations: тиков %d и %d, ждал по одному", *ta, *tn)
	}
}

// Проход одного цикла не блокирует проход другого: раньше флаг прохода был
// общим, и шаг второго движка во время шага первого молча пропускался.
func TestAnimLoop_PassesDoNotBlockEachOther(t *testing.T) {
	StopAllAnimations()
	defer StopAllAnimations()
	a, b := &testLoop{"a"}, &testLoop{"b"}
	tb := animOn(b)
	now := time.Now()

	nested := 0
	unbind := BindAnimationLoop(a)
	Animate(time.Hour, nil, func(float64) {
		nested++
		StepAnimationsFor(b, now) // посреди прохода a
		StepAnimationsFor(a, now) // вложенный шаг своего цикла — не выполняется
	})
	unbind()

	StepAnimationsFor(a, now)
	if nested != 1 {
		t.Fatalf("тик a выполнен %d раз, ждал 1 (вложенный шаг своего цикла не нужен)", nested)
	}
	if *tb != 1 {
		t.Fatalf("шаг b посреди прохода a дал %d тиков, ждал 1", *tb)
	}
}

// Анимация, заведённая в тике, привязана к тому же циклу и получает шаг на
// следующем проходе, а не в текущем.
func TestAnimLoop_BornInTickStaysOnLoop(t *testing.T) {
	StopAllAnimations()
	defer StopAllAnimations()
	a, b := &testLoop{"a"}, &testLoop{"b"}
	now := time.Now()

	child := 0
	started := false
	unbindA := BindAnimationLoop(a)
	Animate(time.Hour, nil, func(float64) {
		if !started {
			started = true
			Animate(time.Hour, nil, func(float64) { child++ })
		}
	})
	unbindA()

	// Шагаем с горутины, объявленной циклом a: тик заводит дочернюю там же.
	unbindA = BindAnimationLoop(a)
	StepAnimationsFor(a, now)
	unbindA()
	if child != 0 {
		t.Fatal("дочерняя анимация получила шаг в проходе, где родилась")
	}
	StepAnimationsFor(b, now)
	if child != 0 {
		t.Fatal("дочерняя анимация цикла a шагнула на цикле b")
	}
	StepAnimationsFor(a, now)
	if child != 1 {
		t.Fatalf("дочерняя анимация на следующем проходе a: %d тиков, ждал 1", child)
	}
}
