package tests

import (
	"image"
	"testing"
	"testing/fstest"

	"github.com/oops1/headless-gui/v3/widget"
)

// Переключатель устроен как флажок (кружок, отступ, подпись) и получал те же
// ровные 80 точек без Width, что флажок в GG-69, — исправлен вместе с ним.
func TestRadioButton_AutoWidthInDockPanel(t *testing.T) {
	root, reg, err := widget.LoadUIFromXAMLFS([]byte(`<DockPanel Width="800" Height="28">
  <RadioButton x:Name="long" DockPanel.Dock="Left" GroupName="g" Content="по дате фиксации"/>
  <RadioButton x:Name="short" DockPanel.Dock="Left" GroupName="g" Content="Все"/>
  <Label Content="остаток"/>
</DockPanel>`), fstest.MapFS{})
	if err != nil {
		t.Fatalf("разметка: %v", err)
	}
	root.SetBounds(image.Rect(0, 0, 800, 28))

	long, ok := reg["long"].(*widget.RadioButton)
	if !ok {
		t.Fatalf("переключатель не собрался: %T", reg["long"])
	}
	short := reg["short"].(*widget.RadioButton)

	text := widget.MeasureUIText("по дате фиксации", widget.DefaultFontSizePt)
	if got, need := long.Bounds().Dx(), 16+6+text; got < need {
		t.Fatalf("переключатель шириной %d, а кружку с подписью нужно не меньше %d", got, need)
	}
	if got := short.Bounds().Dx(); got != 80 {
		t.Fatalf("короткий переключатель шириной %d, ждал прежние 80", got)
	}
	if short.Bounds().Min.X < long.Bounds().Max.X {
		t.Fatalf("переключатели наложились: %v и %v", long.Bounds(), short.Bounds())
	}
}
