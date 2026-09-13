package tests

import (
	"image"
	"testing"
	"testing/fstest"

	"github.com/oops1/headless-gui/v3/widget"
)

// GG-69 — пара к GG-66: флажок без Width в DockPanel получал ровно 80 точек, и
// длинная подпись ложилась поверх соседа.

// Флажок у правого края с длинной подписью не налезает ни на соседний флажок,
// ни на подпись слева; короткий остаётся прежних 80 точек.
func TestCheckBox_AutoWidthInDockPanel(t *testing.T) {
	root, reg, err := widget.LoadUIFromXAMLFS([]byte(`<DockPanel Width="800" Height="28">
  <CheckBox x:Name="rx" DockPanel.Dock="Right" Content="регулярное выражение"/>
  <CheckBox x:Name="yes" DockPanel.Dock="Right" Content="Да"/>
  <Label x:Name="rest" Content="Период:"/>
</DockPanel>`), fstest.MapFS{})
	if err != nil {
		t.Fatalf("разметка: %v", err)
	}
	root.SetBounds(image.Rect(0, 0, 800, 28))

	rx, ok := reg["rx"].(*widget.CheckBox)
	if !ok {
		t.Fatalf("флажок не собрался: %T", reg["rx"])
	}
	yes := reg["yes"].(*widget.CheckBox)
	rest := reg["rest"]

	text := widget.MeasureUIText("регулярное выражение", widget.DefaultFontSizePt)
	if got, need := rx.Bounds().Dx(), 16+6+text; got < need {
		t.Fatalf("флажок шириной %d, а квадрату с подписью нужно не меньше %d", got, need)
	}
	if got := yes.Bounds().Dx(); got != 80 {
		t.Fatalf("короткий флажок шириной %d — прежние 80 должны остаться нижней границей", got)
	}
	if yes.Bounds().Max.X > rx.Bounds().Min.X {
		t.Fatalf("флажки наложились: %v и %v", yes.Bounds(), rx.Bounds())
	}
	if rest.Bounds().Max.X > yes.Bounds().Min.X {
		t.Fatalf("подпись слева залезла под флажок: %v и %v", rest.Bounds(), yes.Bounds())
	}
}
