package engine

// owner_review_test.go — случаи из ревью 5.1 (ADR-0008): поддерево со
// старыми parent/recParent, ставшее корнем или модальным другого движка;
// панель дока в нативном окне; контент снятой вкладки; Stop старого движка.

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// Панель дока, оторванная в нативное окно (OnFloatNative → SetRoot у
// вторичного движка B): floatPane не снимает p.parent (DockManager главного
// дерева), поэтому верх цепочки — корень A, и всё, что меняется внутри
// плавающей панели, уведомляет A, а не движок её окна.
func TestOwnerReview_NativeFloatingDockPaneNotifiesHostEngine(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)

	dm := widget.NewDockManager()
	dm.SetBounds(image.Rect(0, 0, 320, 240))
	inner := widget.NewLabel("в панели", color.RGBA{A: 255})
	inner.SetBounds(image.Rect(4, 4, 90, 24))
	pane := widget.NewDockPane("p", "Панель", inner)
	a.root.AddChild(dm)
	dm.AddPane(pane, widget.DockLeft)
	a.eng.renderFrame()

	// Хост отрыва (window/core/dock_host.go): панель — корень вторичного движка.
	pane.OnFloatNative = func(p *widget.DockPane) {
		p.SetBounds(image.Rect(0, 0, 200, 150))
		b.eng.SetRoot(p)
	}
	pane.Float()
	b.eng.renderFrame()
	a.eng.consumeDamage()
	b.eng.consumeDamage()

	if got := widget.OwnerOf(inner); got != widget.Owner(treeOwner{b.eng}) {
		t.Errorf("владелец содержимого плавающей панели: %v, ждали движок её окна B", got)
	}
	gb := b.eng.lp.Gen()
	inner.SetText("правка в плавающей панели")
	if d, w := pending(b.eng, gb); !d || !w {
		t.Fatalf("B (окно панели) не получил правку содержимого панели (damage=%v, wake=%v): уведомление ушло в A", d, w)
	}
}

// Поддерево, нарисованное ребёнком в дереве A (recParent записан рекордером),
// снято RemoveChild и сделано корнем B: parent снят, recParent — нет, и
// ownerOf поднимается в дерево A. Привязка B на корне игнорируется.
func TestOwnerReview_RootWithStaleRecParentNotifiesOldEngine(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)

	sub := widget.NewPanel(color.RGBA{R: 200, A: 255})
	sub.ShowHeader = false
	sub.SetBounds(image.Rect(0, 60, 100, 120))
	inner := widget.NewLabel("внутри", color.RGBA{A: 255})
	inner.SetBounds(image.Rect(4, 64, 90, 84))
	sub.AddChild(inner)
	a.root.AddChild(sub)
	a.eng.renderFrame() // путь команд: recParent у sub = root A

	if !a.root.RemoveChild(sub) {
		t.Fatal("RemoveChild не нашёл поддерево")
	}
	b.eng.SetRoot(sub)
	b.eng.renderFrame()
	a.eng.consumeDamage()
	b.eng.consumeDamage()

	if got := widget.OwnerOf(inner); got != widget.Owner(treeOwner{b.eng}) {
		t.Errorf("владелец перенесённого поддерева: %v, ждали B", got)
	}
	ga, gb := a.eng.lp.Gen(), b.eng.lp.Gen()
	inner.SetText("после переноса")
	if d, w := pending(b.eng, gb); !d || !w {
		t.Fatalf("B не получил правку своего корня (damage=%v, wake=%v)", d, w)
	}
	if d, w := pending(a.eng, ga); d || w {
		t.Errorf("A разбужен деревом, отданным B (damage=%v, wake=%v)", d, w)
	}
}

// То же для модального окна в холсте: диалог, когда-то нарисованный ребёнком
// дерева A и снятый, показан модальным в B.
func TestOwnerReview_ModalWithStaleRecParentNotifiesOldEngine(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)

	m := &dimModal{modal: true}
	m.SetBounds(image.Rect(40, 40, 240, 200))
	a.root.AddChild(m)
	a.eng.renderFrame()
	a.root.RemoveChild(m)

	b.eng.ShowModal(m)
	b.eng.renderFrame()
	a.eng.consumeDamage()
	b.eng.consumeDamage()
	gb := b.eng.lp.Gen()
	m.Invalidate()
	if d, w := pending(b.eng, gb); !d || !w {
		t.Fatalf("B не получил Invalidate своего модального (damage=%v, wake=%v)", d, w)
	}
}

// Вкладка, снятая с TabControl (RemoveTab), и панель, закрытая у дока
// (closePane/RemovePane): parent не снимается — поддерево, ставшее корнем B,
// уведомляет A.
func TestOwnerReview_RemovedTabContentKeepsParent(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)

	tc := widget.NewTabControl()
	tc.SetBounds(image.Rect(0, 0, 320, 240))
	content := widget.NewPanel(color.RGBA{R: 10, A: 255})
	content.ShowHeader = false
	content.SetBounds(image.Rect(0, 30, 320, 240))
	inner := widget.NewLabel("вкладка", color.RGBA{A: 255})
	inner.SetBounds(image.Rect(4, 34, 90, 54))
	content.AddChild(inner)
	tc.AddTab("T", content)
	a.root.AddChild(tc)
	a.eng.renderFrame()

	tc.RemoveTab(0)
	b.eng.SetRoot(content)
	b.eng.renderFrame()
	a.eng.consumeDamage()
	b.eng.consumeDamage()
	gb := b.eng.lp.Gen()
	inner.SetText("в своём окне")
	if d, w := pending(b.eng, gb); !d || !w {
		t.Fatalf("B не получил правку содержимого снятой вкладки (damage=%v, wake=%v)", d, w)
	}
}

// Пока подъём по родителям честен, стоп движка A не ломает B: контроль.
func TestOwnerReview_StopOfOldEngineRestoresBroadcast(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)
	sub := widget.NewPanel(color.RGBA{R: 200, A: 255})
	sub.ShowHeader = false
	sub.SetBounds(image.Rect(0, 60, 100, 120))
	inner := widget.NewLabel("внутри", color.RGBA{A: 255})
	inner.SetBounds(image.Rect(4, 64, 90, 84))
	sub.AddChild(inner)
	a.root.AddChild(sub)
	a.eng.renderFrame()
	a.root.RemoveChild(sub)
	b.eng.SetRoot(sub)
	b.eng.renderFrame()
	a.eng.Stop()
	b.eng.consumeDamage()
	gb := b.eng.lp.Gen()
	inner.SetText("после Stop A")
	if d, w := pending(b.eng, gb); !d || !w {
		t.Fatalf("после Stop A дерево B не достучалось до B (damage=%v, wake=%v)", d, w)
	}
}
