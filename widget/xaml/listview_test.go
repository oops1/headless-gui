package xaml_test

import (
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
	"github.com/oops1/headless-gui/v4/widget/xaml"
)

// Разметка: <ListView Reorderable="True">, а SelectedIndex не шлёт событие.
func TestListReorder_XAML(t *testing.T) {
	_, reg, err := xaml.LoadUIFromXAML([]byte(`<Window Title="T" Width="300" Height="200">
  <ListView Name="lv" Left="0" Top="0" Width="200" Height="120" Reorderable="True" SelectedIndex="1">
    <Item Content="a"/>
    <Item Content="b"/>
  </ListView>
</Window>`))
	if err != nil {
		t.Fatal(err)
	}
	lv := reg["lv"].(*base.ListView)
	if !lv.Reorderable {
		t.Fatal("Reorderable из разметки не применился")
	}
	if got := lv.Selected(); got != 1 {
		t.Fatalf("SelectedIndex из разметки: %d", got)
	}
}
