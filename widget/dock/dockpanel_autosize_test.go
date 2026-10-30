package dock

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
)

func dockWithCheckBox(text string) (*DockPanel, *base.CheckBox) {
	dp := NewDockPanel()
	cb := base.NewCheckBox(text)
	cb.SetDock(base.DockRight)
	dp.AddChild(cb)
	dp.AddChild(base.NewButton("заполнитель")) // последний растягивается
	dp.SetBounds(image.Rect(0, 0, 600, 30))
	return dp, cb
}

// Новая подпись — новая ширина.
func TestDockPanel_AutoWidthFollowsText(t *testing.T) {
	dp, cb := dockWithCheckBox("regexp")
	before := cb.Bounds().Dx()

	cb.SetText("регулярное выражение")
	dp.Relayout()

	want := max(80, checkBoxAutoWidth(cb))
	if got := cb.Bounds().Dx(); got != want || got <= before {
		t.Fatalf("ширина после смены подписи %d (до — %d), ждал %d", got, before, want)
	}

	// И обратно: короткая подпись — прежняя ширина.
	cb.SetText("regexp")
	dp.Relayout()
	if got := cb.Bounds().Dx(); got != before {
		t.Fatalf("ширина после возврата подписи %d, ждал %d", got, before)
	}
}

// Ширина, заданная кодом, раскладкой не перетирается.
func TestDockPanel_ExplicitWidthKept(t *testing.T) {
	dp, cb := dockWithCheckBox("regexp")
	b := cb.Bounds()
	cb.SetBounds(image.Rect(b.Min.X, b.Min.Y, b.Min.X+200, b.Max.Y))

	cb.SetText("регулярное выражение")
	dp.Relayout()

	if got := cb.Bounds().Dx(); got != 200 {
		t.Fatalf("ширина, заданная кодом, стала %d, ждал 200", got)
	}
}

// Бывший последний ребёнок, растянутый на остаток, возвращается к ширине по
// содержимому, а не к застывшей.
func TestDockPanel_FormerFillChildRemeasures(t *testing.T) {
	dp := NewDockPanel()
	cb := base.NewCheckBox("regexp")
	cb.SetDock(base.DockLeft)
	dp.AddChild(cb)
	dp.SetBounds(image.Rect(0, 0, 600, 30))    // пока cb последний — растянут
	dp.AddChild(base.NewButton("заполнитель")) // теперь докуется по своей ширине

	cb.SetText("регулярное выражение")
	dp.Relayout()

	if got, want := cb.Bounds().Dx(), max(80, checkBoxAutoWidth(cb)); got != want {
		t.Fatalf("ширина бывшего заполнителя %d, ждал %d", got, want)
	}
}
