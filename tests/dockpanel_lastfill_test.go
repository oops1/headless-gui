package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// LastChildFill — запрос GG-32.
//
// Последний ребёнок ВСЕГДА растягивался на остаток, и его собственный Dock не
// действовал. Строку, где все дети докованы — поле поиска слева, счётчик и
// кнопки справа, — собрать было нечем: в конец приходилось дописывать пустой
// Border только затем, чтобы растяжение досталось ему.

func lastFillPanel(fill bool) (*widget.DockPanel, *widget.Panel, *widget.Panel) {
	dp := widget.NewDockPanel()
	dp.LastChildFill = fill

	left := widget.NewPanel(widget.Theme{}.PanelBG)
	left.ShowHeader = false
	left.SetBounds(image.Rect(0, 0, 100, 30))
	left.SetDock(widget.DockLeft)
	dp.AddChild(left)

	right := widget.NewPanel(widget.Theme{}.PanelBG)
	right.ShowHeader = false
	right.SetBounds(image.Rect(0, 0, 50, 30))
	right.SetDock(widget.DockRight)
	dp.AddChild(right)

	dp.SetBounds(image.Rect(0, 0, 400, 30))
	return dp, left, right
}

func TestDockPanel_LastChildFillOffHonoursDock(t *testing.T) {
	_, left, right := lastFillPanel(false)

	if got := left.Bounds(); got.Dx() != 100 || got.Min.X != 0 {
		t.Errorf("левый ребёнок занял %v, ждали 100px слева", got)
	}
	if got := right.Bounds(); got.Dx() != 50 || got.Max.X != 400 {
		t.Errorf("правый ребёнок занял %v, ждали 50px у правого края", got)
	}
}

// По умолчанию поведение прежнее: последний растягивается.
func TestDockPanel_LastChildFillOnByDefault(t *testing.T) {
	dp := widget.NewDockPanel()
	if !dp.LastChildFill {
		t.Fatal("NewDockPanel выключил растяжение последнего ребёнка")
	}

	_, _, right := lastFillPanel(true)
	if got := right.Bounds(); got.Dx() == 50 {
		t.Errorf("последний ребёнок не растянулся: %v", got)
	}
}

// Атрибут разметки.
func TestDockPanel_LastChildFillFromXAML(t *testing.T) {
	xaml := `<Window Width="400" Height="60">
	  <DockPanel x:Name="row" LastChildFill="False" Left="0" Top="0" Width="400" Height="30">
	    <Button x:Name="a" DockPanel.Dock="Left" Width="100" Height="30" Content="A"/>
	    <Button x:Name="b" DockPanel.Dock="Right" Width="50" Height="30" Content="B"/>
	  </DockPanel>
	</Window>`

	_, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	dp, ok := reg["row"].(*widget.DockPanel)
	if !ok {
		t.Fatalf("DockPanel собрался как %T", reg["row"])
	}
	if dp.LastChildFill {
		t.Error(`LastChildFill="False" не прочитан`)
	}
	if b, ok := reg["b"].(*widget.Button); ok {
		if got := b.Bounds().Dx(); got != 50 {
			t.Errorf("правая кнопка шириной %d, ждали 50", got)
		}
	}
}
