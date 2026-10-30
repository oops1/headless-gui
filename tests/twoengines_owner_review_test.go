package tests

// twoengines_owner_review_test.go — случаи из ревью 5.4: анимация виджета
// будит только свой движок, дерево остановленного движка уведомляет всех,
// раскладка при чужом глобальном измерителе.

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// Анимация виджета дерева A, заведённая с чужой горутины, шагает циклом A и
// будит только A.
func TestTwoEnginesReview_TwoEngines_AnimationWakesOnlyOwnEngine(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)
	fa, fb := settle(a.eng), settle(b.eng)
	ticks := 0
	done := make(chan struct{})
	widget.AnimateOwned(a.label, "zz54", 200*time.Millisecond, widget.EaseLinear, func(p float64) {
		ticks++
		a.label.SetBounds(image.Rect(10+int(p*20), 10, 200, 34))
		if p >= 1 {
			close(done)
		}
	})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("анимация виджета A не завершилась — её никто не шагает")
	}
	if !woke(a.eng, fa) {
		t.Error("анимация A не дала кадров A")
	}
	if got := settle(b.eng); got != fb {
		t.Errorf("анимация виджета A будила B: кадров %d → %d", fb, got)
	}
	if ticks < 2 {
		t.Errorf("тиков анимации %d", ticks)
	}
}

// Остановленный движок отпускает дерево (ADR-0008): его виджеты снова
// уведомляют широковещанием — правка будит оставшийся движок B. Фиксирует
// задокументированное поведение «Stop снимает у корня».
func TestTwoEnginesReview_TwoEngines_StoppedEngineTreeBroadcasts(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)
	a.eng.Stop()
	if widget.OwnerOf(a.label) != nil {
		t.Fatal("после Stop дерево A осталось привязанным")
	}
	fb := settle(b.eng)
	a.label.SetText("правка после Stop A")
	if !woke(b.eng, fb) {
		t.Error("правка ничейного дерева не разбудила B")
	}
}

// Раскладка с разным DPI, где глобальный измеритель — ЧУЖОЙ движок: лейбл
// дерева 144 dpi меряется после создания движка 96 dpi (последний
// зарегистрированный — он). В исходном тесте каждый замер шёл сразу после
// создания своего движка, то есть глобальный измеритель совпадал со своим.
func TestTwoEnginesReview_TwoEngines_LayoutWhenGlobalMeasurerIsOther(t *testing.T) {
	testreset.Globals(t)
	mk := func(dpi float64) (*engine.Engine, *widget.Label) {
		eng := engine.New(400, 300, 60)
		t.Cleanup(eng.Stop)
		if dpi > 0 {
			eng.SetDPI(dpi)
		}
		root := widget.NewPanel(color.RGBA{A: 255})
		root.SetBounds(image.Rect(0, 0, 400, 300))
		l := widget.NewLabel("Пример строки для замера", color.RGBA{A: 255})
		root.AddChild(l)
		eng.SetRoot(root)
		return eng, l
	}
	_, solo := mk(144)
	ref144, _ := solo.DesiredSize()
	_, l144 := mk(144)
	_, l96 := mk(0) // последний: глобальный измеритель — 96 dpi
	ref96, _ := l96.DesiredSize()
	if ref96 == ref144 {
		t.Fatalf("сцена не различает DPI: %d", ref96)
	}
	if w, _ := l144.DesiredSize(); w != ref144 {
		t.Errorf("лейбл движка 144 dpi при глобальном 96: %d, ждали %d (96 dpi — %d)", w, ref144, ref96)
	}
}
