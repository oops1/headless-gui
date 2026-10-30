package overlay

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// GG-77: у кнопок в заголовке DockPane свои подсказки. Движок спрашивал только
// общую подсказку виджета (GetToolTip) и различать части не умел; виджет с
// разными подсказками по частям отвечает через ToolTipAt(x, y).
func TestTooltipAt_PerPartTooltip(t *testing.T) {
	testreset.Globals(t)
	p := widget.NewDockPane("branches", "Ветки", nil)
	p.ToolTip = "Панель веток"
	p.SetBounds(image.Rect(0, 0, 300, 200))
	p.SetTitleButtons([]widget.DockPaneButton{
		{Tooltip: "GitHub"},
		{Tooltip: "Вид"},
	})

	// Кнопки: «GitHub» 195..213, «Вид» 215..233 по X; 3..21 по Y.
	if got := TooltipAt(p, 204, 12); got != "GitHub" {
		t.Errorf("над первой кнопкой %q, ждал GitHub", got)
	}
	if got := TooltipAt(p, 224, 12); got != "Вид" {
		t.Errorf("над второй кнопкой %q, ждал Вид", got)
	}
	// Мимо кнопок — общая подсказка виджета, как раньше.
	if got := TooltipAt(p, 100, 100); got != "Панель веток" {
		t.Errorf("мимо кнопок %q, ждал общую подсказку", got)
	}
}
