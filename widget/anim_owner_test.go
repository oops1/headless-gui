package widget

import (
	"testing"
	"time"
)

// Loop-анимация скрытого владельца снимается сама.
func TestAnim_LoopStopsWhenOwnerHidden(t *testing.T) {
	StopAllAnimations()
	t.Cleanup(StopAllAnimations)

	owner := NewLabel("x", win10.LabelText)
	ticks := 0
	a := AnimateOwned(owner, "spin", 20*time.Millisecond, nil, func(float64) { ticks++ })
	a.Loop = true

	now := time.Now()
	StepAnimations(now)
	StepAnimations(now.Add(10 * time.Millisecond))
	if ticks == 0 {
		t.Fatal("видимый владелец: тиков нет")
	}
	if !a.Running() {
		t.Fatal("анимация снялась при видимом владельце")
	}

	owner.SetVisible(false)
	before := ticks
	StepAnimations(now.Add(20 * time.Millisecond))
	if a.Running() {
		t.Fatal("Loop-анимация скрытого владельца осталась активной")
	}
	if ticks != before {
		t.Fatalf("тик у скрытого владельца: %d → %d", before, ticks)
	}
	if AnimationsActive() {
		t.Fatal("движок не заснёт: анимация всё ещё в реестре")
	}
}

// Однократная анимация скрытого владельца доигрывает как прежде.
func TestAnim_OnceKeepsRunningWhenHidden(t *testing.T) {
	StopAllAnimations()
	t.Cleanup(StopAllAnimations)

	owner := NewLabel("x", win10.LabelText)
	owner.SetVisible(false)
	done := false
	last := -1.0
	a := AnimateOwned(owner, "fade", 10*time.Millisecond, nil, func(v float64) { last = v })
	a.OnDone = func() { done = true }

	now := time.Now()
	StepAnimations(now)
	StepAnimations(now.Add(20 * time.Millisecond))
	if !done || last != 1 {
		t.Fatalf("однократная анимация не доиграла: done=%v last=%v", done, last)
	}
}

// Без владельца Loop продолжает крутиться (прежнее поведение).
func TestAnim_LoopWithoutOwner(t *testing.T) {
	StopAllAnimations()
	t.Cleanup(StopAllAnimations)

	a := Animate(10*time.Millisecond, nil, func(float64) {})
	a.Loop = true
	now := time.Now()
	StepAnimations(now)
	StepAnimations(now.Add(20 * time.Millisecond))
	if !a.Running() {
		t.Fatal("Loop без владельца снялась")
	}
}
