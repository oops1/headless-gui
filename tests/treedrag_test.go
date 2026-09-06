package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/treeview"
)

// Перетаскивание узлов дерева — вторая половина GG-11.
//
// Порядок и вложенность узлов задавались только из кода: перетащить ветку в
// другую группу или переставить репозиторий выше было нечем.

const treeItemH = 22 // defaultItemHeight

func dragScene(t *testing.T) (*engine.Engine, *widget.TreeViewWidget, []*treeview.TreeViewItem) {
	t.Helper()
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 24, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 300, 300))

	tw := widget.NewTreeViewWidget()
	tw.SetBounds(image.Rect(0, 0, 300, 300))
	tw.Tree.CanUserDragNodes = true

	var items []*treeview.TreeViewItem
	for _, n := range []string{"группа", "repo-a", "repo-b"} {
		it := treeview.NewItem(n)
		tw.Tree.AddRoot(it)
		items = append(items, it)
	}
	root.AddChild(tw)

	eng := engine.New(300, 300, 30)
	eng.SetRoot(root)
	eng.RenderOnce()
	return eng, tw, items
}

// rowCenter — середина строки idx, rowTop — её верх плюс пара точек.
func rowCenter(idx int) int { return idx*treeItemH + treeItemH/2 }
func rowTop(idx int) int    { return idx*treeItemH + 1 }

func drag(eng *engine.Engine, fromY, toY int) {
	eng.SendMouseButton(40, fromY, widget.MouseLeft, true)
	eng.SendMouseMove(40, toY)
	eng.SendMouseButton(40, toY, widget.MouseLeft, false)
}

func rootNames(tw *widget.TreeViewWidget) []string {
	var out []string
	for _, r := range tw.Tree.Roots() {
		out = append(out, r.DisplayText())
	}
	return out
}

// Бросок в середину строки кладёт узел ВНУТРЬ неё.
func TestTreeDrag_DropInsideMakesChild(t *testing.T) {
	eng, tw, items := dragScene(t)

	drag(eng, rowCenter(1), rowCenter(0)) // repo-a внутрь группы

	if got := rootNames(tw); len(got) != 2 || got[0] != "группа" {
		t.Fatalf("корни после переноса: %v", got)
	}
	if got := len(items[0].Children); got != 1 {
		t.Fatalf("у группы %d детей, ждали одного", got)
	}
	if items[0].Children[0] != items[1] {
		t.Errorf("внутрь группы попал не тот узел: %q", items[0].Children[0].DisplayText())
	}
	if !items[0].Expanded {
		t.Error("ветка осталась свёрнутой — узел «исчез» внутрь неё")
	}
}

// Бросок в верхнюю треть строки ставит узел ПЕРЕД ней.
func TestTreeDrag_DropBeforeReorders(t *testing.T) {
	eng, tw, _ := dragScene(t)

	drag(eng, rowCenter(2), rowTop(0)) // repo-b перед группой

	if got := rootNames(tw); len(got) != 3 || got[0] != "repo-b" {
		t.Errorf("порядок корней %v, ждали repo-b первым", got)
	}
}

// Узел нельзя положить внутрь собственного потомка: дерево перестало бы быть
// деревом, а поддерево потерялось бы вместе с узлом.
func TestTreeDrag_CannotDropIntoOwnChild(t *testing.T) {
	_, tw, items := dragScene(t)

	parent, child := items[0], items[1]
	tw.Tree.MoveNode(child, parent, treeview.DropInside)
	if len(parent.Children) != 1 {
		t.Fatalf("подготовка не удалась: у группы %d детей", len(parent.Children))
	}

	if tw.Tree.CanDrop(parent, child, treeview.DropInside) {
		t.Error("CanDrop разрешил положить узел внутрь собственного потомка")
	}
	if tw.Tree.MoveNode(parent, child, treeview.DropInside) {
		t.Error("MoveNode выполнил перенос узла внутрь собственного потомка")
	}
	if parent.Parent() != nil {
		t.Error("группа оказалась внутри своего же ребёнка")
	}
}

// Дрожь руки — не перетаскивание: порог не пройден, узел просто выбран.
func TestTreeDrag_TinyMoveIsAClick(t *testing.T) {
	eng, tw, items := dragScene(t)

	y := rowCenter(1)
	eng.SendMouseButton(40, y, widget.MouseLeft, true)
	eng.SendMouseMove(40, y+2)
	eng.SendMouseButton(40, y+2, widget.MouseLeft, false)

	if got := rootNames(tw); len(got) != 3 {
		t.Errorf("дрожь переставила узлы: %v", got)
	}
	if tw.SelectedNode() != items[1] {
		t.Error("узел под курсором не выбран — щелчок пропал")
	}
}

// Выключенное перетаскивание не меняет прежнего поведения.
func TestTreeDrag_OffByDefault(t *testing.T) {
	eng, tw, _ := dragScene(t)
	tw.Tree.CanUserDragNodes = false

	drag(eng, rowCenter(1), rowCenter(0))

	if got := rootNames(tw); len(got) != 3 || got[0] != "группа" {
		t.Errorf("с выключенным перетаскиванием порядок изменился: %v", got)
	}
}

// Приложение вправе взять перенос на себя: возврат true означает «справился
// сам», и дерево не переставляет узлы за его спиной.
func TestTreeDrag_HandlerCanTakeOver(t *testing.T) {
	eng, tw, items := dragScene(t)

	var gotDragged, gotTarget *treeview.TreeViewItem
	var gotPos treeview.DropPosition
	tw.Tree.OnNodeDrop = func(d, tgt *treeview.TreeViewItem, p treeview.DropPosition) bool {
		gotDragged, gotTarget, gotPos = d, tgt, p
		return true // модель приложения — источник правды
	}

	drag(eng, rowCenter(1), rowCenter(0))

	if gotDragged != items[1] || gotTarget != items[0] || gotPos != treeview.DropInside {
		t.Errorf("обработчику досталось (%v, %v, %v)", gotDragged, gotTarget, gotPos)
	}
	if got := rootNames(tw); len(got) != 3 {
		t.Errorf("дерево переставило узлы, хотя обработчик взял это на себя: %v", got)
	}
}

// CanDropNode запрещает конкретный перенос.
func TestTreeDrag_CanDropNodeVetoes(t *testing.T) {
	eng, tw, _ := dragScene(t)
	tw.Tree.CanDropNode = func(*treeview.TreeViewItem, *treeview.TreeViewItem, treeview.DropPosition) bool {
		return false
	}

	drag(eng, rowCenter(1), rowCenter(0))

	if got := rootNames(tw); len(got) != 3 {
		t.Errorf("запрещённый перенос состоялся: %v", got)
	}
}

// Место вставки видно на экране, пока узел тащат.
func TestTreeDrag_IndicatorIsDrawn(t *testing.T) {
	eng, _, _ := dragScene(t)

	eng.Invalidate()
	before := snapshotRGBA(eng.RenderOnce())

	eng.SendMouseButton(40, rowCenter(2), widget.MouseLeft, true)
	eng.SendMouseMove(40, rowTop(0))
	eng.Invalidate()
	during := snapshotRGBA(eng.RenderOnce())

	diff := 0
	for i := range before.Pix {
		if before.Pix[i] != during.Pix[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Error("во время перетаскивания место вставки ничем не показано")
	}

	eng.SendMouseButton(40, rowTop(0), widget.MouseLeft, false)
}
