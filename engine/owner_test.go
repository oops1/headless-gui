package engine

// owner_test.go — владелец дерева (ADR-0008, приёмка 5.1): уведомления
// виджета идут движку его дерева; дерево без владельца и widget.InvalidateRect
// — всем; модальные окна, смена корня, Stop, перенос поддерева между
// движками.

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// ownerScene — движок с корнем-панелью и лейблом.
type ownerScene struct {
	eng   *Engine
	root  *widget.Panel
	label *widget.Label
}

func newOwnerScene(t *testing.T) ownerScene {
	t.Helper()
	eng := New(320, 240, 20)
	t.Cleanup(eng.Stop)
	eng.SetRenderOnDemand(true)
	root := widget.NewPanel(color.RGBA{R: 20, G: 30, B: 40, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 320, 240))
	label := widget.NewLabel("текст", color.RGBA{R: 250, G: 250, B: 250, A: 255})
	label.SetBounds(image.Rect(20, 20, 200, 44))
	root.AddChild(label)
	eng.SetRoot(root)
	eng.renderFrame()
	eng.consumeDamage()
	return ownerScene{eng: eng, root: root, label: label}
}

// pending — заявлено ли движку повреждение (области или всё) и выросло ли
// поколение изменений с gen.
func pending(e *Engine, gen uint64) (damaged bool, woke bool) {
	rects, all := e.consumeDamage()
	return all || len(rects) > 0, e.invGen.Load() != gen
}

func TestOwner_InvalidateReachesOnlyOwnEngine(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)
	ga, gb := a.eng.invGen.Load(), b.eng.invGen.Load()

	a.label.SetText("правка в A")
	if d, w := pending(a.eng, ga); !d || !w {
		t.Fatalf("A: SetText лейбла не дошёл до своего движка (damage=%v, wake=%v)", d, w)
	}
	if d, w := pending(b.eng, gb); d || w {
		t.Fatalf("B разбужен правкой в A (damage=%v, wake=%v)", d, w)
	}

	gb = b.eng.invGen.Load()
	a.label.SetBounds(image.Rect(30, 30, 210, 54))
	a.root.AddChild(widget.NewLabel("ещё", color.RGBA{A: 255}))
	a.root.Invalidate()
	if d, w := pending(b.eng, gb); d || w {
		t.Fatalf("B разбужен SetBounds/AddChild/Invalidate в A (damage=%v, wake=%v)", d, w)
	}
	if d, _ := pending(a.eng, 0); !d {
		t.Fatal("A: собственные изменения не заявлены")
	}
	if widget.OwnerOf(a.label) != widget.Owner(treeOwner{a.eng}) {
		t.Fatal("владелец лейбла A — не движок A")
	}
}

// Раскрывающийся список (оверлей вне границ) уведомляет только своё дерево.
func TestOwner_OverlayWidgetInvalidatesOwnEngine(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)
	dd := widget.NewDropdown("один", "два")
	dd.SetBounds(image.Rect(20, 100, 200, 124))
	a.root.AddChild(dd)
	a.eng.renderFrame()
	a.eng.consumeDamage()
	b.eng.consumeDamage()
	gb := b.eng.invGen.Load()
	dd.SetOpen(true)
	if d, w := pending(b.eng, gb); d || w {
		t.Fatalf("B разбужен раскрытием списка в A (damage=%v, wake=%v)", d, w)
	}
	if d, _ := pending(a.eng, 0); !d {
		t.Fatal("A: раскрытие списка не заявлено")
	}
}

// widget.InvalidateRect — ничейная область: доходит до всех, как прежде.
func TestOwner_GlobalInvalidateRectReachesAll(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)
	ga, gb := a.eng.invGen.Load(), b.eng.invGen.Load()
	widget.InvalidateRect(image.Rect(10, 10, 50, 50))
	if d, w := pending(a.eng, ga); !d || !w {
		t.Fatal("A не получил widget.InvalidateRect")
	}
	if d, w := pending(b.eng, gb); !d || !w {
		t.Fatal("B не получил widget.InvalidateRect")
	}
}

// Дерево без SetRoot владельца не имеет: его уведомления широковещательные.
func TestOwner_TreeWithoutOwnerBroadcasts(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)
	orphan := widget.NewPanel(color.RGBA{A: 255})
	orphan.SetBounds(image.Rect(0, 0, 100, 100))
	l := widget.NewLabel("сирота", color.RGBA{A: 255})
	l.SetBounds(image.Rect(0, 0, 80, 20))
	orphan.AddChild(l)
	if widget.OwnerOf(l) != nil {
		t.Fatal("у дерева без SetRoot нашёлся владелец")
	}
	ga, gb := a.eng.invGen.Load(), b.eng.invGen.Load()
	l.SetText("правка сироты")
	if d, w := pending(a.eng, ga); !d || !w {
		t.Fatal("A не получил уведомление дерева без владельца")
	}
	if d, w := pending(b.eng, gb); !d || !w {
		t.Fatal("B не получил уведомление дерева без владельца")
	}
}

// Модальное окно в холсте принадлежит своему движку; после CloseModal — нет.
func TestOwner_ModalBelongsToItsEngine(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)
	m := &dimModal{modal: true}
	m.SetBounds(image.Rect(40, 40, 240, 200))
	a.eng.ShowModal(m)
	a.eng.renderFrame()
	a.eng.consumeDamage()
	b.eng.consumeDamage()
	if widget.OwnerOf(m) != widget.Owner(treeOwner{a.eng}) {
		t.Fatal("модальное не привязано к движку A")
	}
	gb := b.eng.invGen.Load()
	m.Invalidate()
	if d, w := pending(b.eng, gb); d || w {
		t.Fatal("B разбужен модальным окном A")
	}
	if d, _ := pending(a.eng, 0); !d {
		t.Fatal("A: модальное не заявило повреждения")
	}
	m.modal = false
	a.eng.CloseModal(m)
	if widget.OwnerOf(m) != nil {
		t.Fatal("после CloseModal привязка осталась")
	}
}

// Смена корня переносит привязку; Stop снимает её.
func TestOwner_SetRootMovesBindingAndStopClears(t *testing.T) {
	testreset.Globals(t)
	a := newOwnerScene(t)
	other := widget.NewPanel(color.RGBA{A: 255})
	other.ShowHeader = false
	a.eng.SetRoot(other)
	if widget.OwnerOf(a.root) != nil {
		t.Fatal("прежний корень остался привязан после SetRoot другого")
	}
	if widget.OwnerOf(other) == nil {
		t.Fatal("новый корень не привязан")
	}
	// Корень, отданный второму движку, принадлежит ему.
	b := New(320, 240, 20)
	defer b.Stop()
	b.SetRoot(a.root)
	if widget.OwnerOf(a.label) != widget.Owner(treeOwner{b}) {
		t.Fatal("дерево, отданное B, не принадлежит B")
	}
	b.Stop()
	if widget.OwnerOf(a.label) != nil {
		t.Fatal("после Stop привязка осталась")
	}
	a.eng.Stop()
	if widget.OwnerOf(other) != nil {
		t.Fatal("после Stop A привязка корня осталась")
	}
}

// Поддерево, бывшее корнем B, добавлено ребёнком в дерево A: привязка
// верхнего виджета побеждает — уведомляет A.
func TestOwner_ReparentedSubtreeFollowsNewTop(t *testing.T) {
	testreset.Globals(t)
	a, b := newOwnerScene(t), newOwnerScene(t)
	pane := widget.NewPanel(color.RGBA{R: 200, A: 255})
	pane.ShowHeader = false
	pane.SetBounds(image.Rect(0, 0, 100, 60))
	inner := widget.NewLabel("внутри", color.RGBA{A: 255})
	inner.SetBounds(image.Rect(4, 4, 90, 24))
	pane.AddChild(inner)
	b.eng.SetRoot(pane) // как плавающая панель дока в своём окне
	if widget.OwnerOf(inner) != widget.Owner(treeOwner{b.eng}) {
		t.Fatal("панель в B не принадлежит B")
	}
	// Панель вернулась в главное дерево; привязка B на ней ещё висит.
	a.root.AddChild(pane)
	a.eng.consumeDamage()
	b.eng.consumeDamage()
	gb := b.eng.invGen.Load()
	inner.SetText("после возврата")
	if d, w := pending(b.eng, gb); d || w {
		t.Fatal("B разбужен виджетом панели, вернувшейся в A")
	}
	if d, _ := pending(a.eng, 0); !d {
		t.Fatal("A не получил уведомление вернувшейся панели")
	}
}

// Анимация с виджетом-владельцем будит только его движок; без владельца —
// всех (у движка пробуждение — no-op, проверяется отсутствие паники и
// повреждения у чужого движка).
func TestOwner_AnimationWakeIsAddressed(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	a, b := newOwnerScene(t), newOwnerScene(t)
	gb := b.eng.invGen.Load()
	widget.AnimateOwned(a.label, "test", 0, nil, func(float64) {})
	if d, w := pending(b.eng, gb); d || w {
		t.Fatal("B разбужен анимацией виджета A")
	}
	widget.Animate(0, nil, func(float64) {})
}

// Наблюдатель реестра (приёмник без владельца) и внешний слот видят
// уведомления дерева, привязанного к движку: адресность пропускает только
// другие движки.
func TestOwner_ObserversStillSeeOwnedNotifications(t *testing.T) {
	testreset.Globals(t)
	a := newOwnerScene(t)
	var rects, ext []image.Rectangle
	fulls := 0
	h := widget.RegisterUINotifierWake(func() { fulls++ }, func(r image.Rectangle) { rects = append(rects, r) }, func() {})
	defer widget.UnregisterUINotifier(h)
	widget.SetUIRectChangeNotifier(func(r image.Rectangle) { ext = append(ext, r) })
	defer widget.SetUIRectChangeNotifier(nil)
	a.label.SetText("для наблюдателя")
	if len(rects) == 0 || len(ext) == 0 {
		t.Fatalf("наблюдатель не увидел уведомление привязанного дерева: реестр %d, слот %d", len(rects), len(ext))
	}
	if d, _ := pending(a.eng, 0); !d {
		t.Fatal("движок-владелец не получил повреждения")
	}
}
