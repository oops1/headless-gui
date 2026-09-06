package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/treeview"
)

// Оформление строки дерева — запрос GG-34.
//
// Дерево рисовало текст любого узла одинаково: цветом темы и обычным
// начертанием. Активный репозиторий приходилось помечать подкраской иконки,
// потому что больше нечем.

func treeScene(t *testing.T) (*engine.Engine, *widget.TreeViewWidget, []*treeview.TreeViewItem) {
	t.Helper()
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 24, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 300, 200))

	tw := widget.NewTreeViewWidget()
	tw.SetBounds(image.Rect(0, 0, 300, 200))
	var items []*treeview.TreeViewItem
	for _, name := range []string{"headless-gui", "Go.Git", "winline"} {
		it := treeview.NewItem(name)
		tw.Tree.AddRoot(it)
		items = append(items, it)
	}
	root.AddChild(tw)

	eng := engine.New(300, 200, 30)
	eng.SetRoot(root)
	eng.RenderOnce()
	return eng, tw, items
}

// frameOf снимает кадр после полной перерисовки.
func frameOf(eng *engine.Engine) *image.RGBA {
	eng.Invalidate()
	return snapshotRGBA(eng.RenderOnce())
}

// diffCount — сколько байт разошлось у двух кадров.
func diffCount(a, b *image.RGBA) int {
	n := 0
	for i := range a.Pix {
		if a.Pix[i] != b.Pix[i] {
			n++
		}
	}
	return n
}

// Свой цвет текста узла виден на экране.
func TestTreeItem_ForegroundIsDrawn(t *testing.T) {
	eng, _, items := treeScene(t)

	before := frameOf(eng)
	items[1].Foreground = color.RGBA{R: 255, G: 64, B: 64, A: 255}
	after := frameOf(eng)

	if diffCount(before, after) == 0 {
		t.Error("Foreground узла не изменил картинку")
	}

	// Нулевая альфа означает «цвет темы» — прежний вид возвращается.
	items[1].Foreground = color.RGBA{}
	back := frameOf(eng)
	if diffCount(before, back) != 0 {
		t.Error("сброс Foreground не вернул цвет темы")
	}
}

// Жирное начертание рисуется другим шрифтом, а не тем же самым.
func TestTreeItem_BoldIsDrawn(t *testing.T) {
	eng, _, items := treeScene(t)

	before := frameOf(eng)
	items[0].Bold = true
	after := frameOf(eng)

	if diffCount(before, after) == 0 {
		t.Error("Bold узла не изменил картинку — жирное начертание не применилось")
	}
}

// Колбэк перекрывает поля узла: он знает о состоянии приложения то, чего узел
// не знает.
func TestTreeItem_StyleCallbackWins(t *testing.T) {
	eng, tw, items := treeScene(t)

	items[2].Foreground = color.RGBA{R: 0, G: 255, B: 0, A: 255}
	viaField := frameOf(eng)

	called := 0
	tw.Tree.ItemStyle = func(it *treeview.TreeViewItem) (color.RGBA, bool, bool) {
		called++
		if it == items[2] {
			return color.RGBA{R: 255, G: 0, B: 255, A: 255}, true, true
		}
		return color.RGBA{}, false, false
	}
	viaCallback := frameOf(eng)

	if called == 0 {
		t.Fatal("ItemStyle не вызывался ни разу")
	}
	if diffCount(viaField, viaCallback) == 0 {
		t.Error("колбэк не перекрыл цвет, заданный полем узла")
	}
}

// Узлы без оформления рисуются как прежде: ни поле, ни колбэк их не трогают.
func TestTreeItem_UntouchedNodesLookTheSame(t *testing.T) {
	eng, tw, items := treeScene(t)

	before := frameOf(eng)
	tw.Tree.ItemStyle = func(it *treeview.TreeViewItem) (color.RGBA, bool, bool) {
		return color.RGBA{}, false, false // «решай сам»
	}
	items[0].Foreground = color.RGBA{}
	after := frameOf(eng)

	if diffCount(before, after) != 0 {
		t.Error("колбэк, ничего не решивший, изменил картинку")
	}
}
