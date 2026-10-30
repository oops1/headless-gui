package tests

// Два движка и владелец дерева (ADR-0008, задача 5.4): изменения виджета
// будят только движок его дерева — по открытому API это видно по счётчику
// кадров работающего движка (Stats().Frames растёт, только когда цикл готовит
// кадр); ничейная область (widget.InvalidateRect) будит оба; модальное окно в
// холсте принадлежит своему движку; раскладка вне Draw меряет текст шрифтами
// своего движка.

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// ownerScene — запущенный движок с корнем, лейблом и раскрывающимся списком.
type ownerScene struct {
	eng   *engine.Engine
	root  *widget.Panel
	label *widget.Label
	dd    *widget.Dropdown
}

func newOwnerScene(t *testing.T) *ownerScene {
	t.Helper()
	eng := engine.New(400, 300, 60)
	t.Cleanup(eng.Stop)
	eng.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 30, G: 40, B: 60, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 300))
	l := widget.NewLabel("текст", color.RGBA{R: 255, G: 255, B: 255, A: 255})
	l.SetBounds(image.Rect(10, 10, 200, 34))
	root.AddChild(l)
	dd := widget.NewDropdown("один", "два", "три")
	dd.SetBounds(image.Rect(10, 60, 200, 86))
	root.AddChild(dd)
	eng.SetRoot(root)
	eng.Start()
	return &ownerScene{eng: eng, root: root, label: l, dd: dd}
}

// settle ждёт затишья движка и отдаёт счётчик кадров. Барьер Flush: цикл
// дошёл до поставленной функции — не посреди кадра (счётчик растёт в начале
// кадра); три стабильных замера подряд.
func settle(e *engine.Engine) uint64 {
	prev := ^uint64(0)
	stable := 0
	for i := 0; i < 80 && stable < 3; i++ {
		time.Sleep(25 * time.Millisecond)
		e.Flush()
		e.Flush()
		cur := e.Stats().Frames
		if cur == prev {
			stable++
		} else {
			stable = 0
		}
		prev = cur
	}
	return prev
}

// woke ждёт кадр сверх from (положительная проверка с дедлайном).
func woke(e *engine.Engine, from uint64) bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if e.Stats().Frames > from {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

func TestTwoEnginesOwner_ChangesWakeOnlyOwnEngine(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)
	for _, step := range []struct {
		name string
		do   func()
	}{
		{"SetText", func() { a.label.SetText("правка в A") }},
		{"SetBounds", func() { a.label.SetBounds(image.Rect(12, 12, 210, 36)) }},
		{"AddChild", func() {
			// Сначала в дерево, потом границы: виджет вне дерева владельца не
			// имеет и уведомляет все движки (ADR-0008).
			p := widget.NewPanel(color.RGBA{R: 200, A: 255})
			a.root.AddChild(p)
			p.SetBounds(image.Rect(250, 200, 300, 250))
		}},
		{"раскрытие списка", func() { a.dd.SetOpen(true) }},
		{"закрытие списка", func() { a.dd.SetOpen(false) }},
	} {
		fa, fb := settle(a.eng), settle(b.eng)
		step.do()
		if !woke(a.eng, fa) {
			t.Errorf("%s: движок A не подготовил кадр", step.name)
		}
		if got := settle(b.eng); got != fb {
			t.Errorf("%s в дереве A разбудил движок B: кадров %d → %d", step.name, fb, got)
		}
	}
}

func TestTwoEnginesOwner_UnclaimedRectWakesBoth(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)
	fa, fb := settle(a.eng), settle(b.eng)
	widget.InvalidateRect(image.Rect(0, 0, 50, 50))
	if !woke(a.eng, fa) || !woke(b.eng, fb) {
		t.Error("ничейная область должна будить оба движка")
	}
}

func TestTwoEnginesOwner_ModalBelongsToItsEngine(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)
	dlg := widget.NewDialog("Диалог", 240, 160)
	inner := widget.NewLabel("в диалоге", color.RGBA{A: 255})
	inner.SetBounds(image.Rect(20, 60, 200, 84))
	dlg.AddChild(inner)
	a.eng.ShowModal(dlg)
	fa, fb := settle(a.eng), settle(b.eng)
	inner.SetText("правка в диалоге")
	if !woke(a.eng, fa) {
		t.Error("правка в модальном окне A не дала кадра A")
	}
	if got := settle(b.eng); got != fb {
		t.Errorf("правка в модальном окне A разбудила B: кадров %d → %d", fb, got)
	}
	a.eng.CloseModal(dlg)
}
