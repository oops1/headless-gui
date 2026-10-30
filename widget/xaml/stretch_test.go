package xaml_test

import (
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
	"github.com/oops1/headless-gui/v4/widget/xaml"
)

// Разметка: <Stretch/> и <ToolBarStretch/>.
func TestStretch_XAML(t *testing.T) {
	_, reg, err := xaml.LoadUIFromXAML([]byte(`<Window Title="T" Width="600" Height="120">
  <ToolBar Name="tb" Left="0" Top="0" Width="600" Height="40">
    <Button Name="first" Content="Pull"/>
    <ToolBarStretch/>
    <Button Name="last" Content="Log"/>
  </ToolBar>
  <StackPanel Name="sp" Orientation="Horizontal" Left="0" Top="50" Width="600" Height="40">
    <Button Content="A" Width="60" Height="24"/>
    <Stretch Weight="2"/>
    <Button Name="spLast" Content="B" Width="60" Height="24"/>
  </StackPanel>
</Window>`))
	if err != nil {
		t.Fatal(err)
	}
	// Окно смещает детей на рамку и полосу заголовка — считаем от границ
	// самих панелей.
	tb := reg["tb"].(*base.ToolBar)
	last := reg["last"].(*base.Button)
	if got, want := last.Bounds().Max.X, tb.Bounds().Max.X-tb.Padding; got != want {
		t.Fatalf("кнопка панели кончается на %d, ждал %d", got, want)
	}
	sp := reg["sp"].(*base.StackPanel)
	spLast := reg["spLast"].(*base.Button)
	if got, want := spLast.Bounds().Max.X, sp.Bounds().Max.X-sp.Padding; got != want {
		t.Fatalf("кнопка столбика кончается на %d, ждал %d", got, want)
	}
}
