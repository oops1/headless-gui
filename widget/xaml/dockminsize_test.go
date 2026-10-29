package xaml

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget/base"
)

// Разметка: <DockPane MinSize="…">.
func TestDockMinSize_XAML(t *testing.T) {
	_, reg, err := LoadUIFromXAML([]byte(`<Window Title="T" Width="1000" Height="700">
  <DockManager Name="dm" Left="0" Top="0" Width="1000" Height="700">
    <DockPane Id="commit" Title="Коммит" Side="Right" Size="120" MinSize="300">
      <Label Text="вкладки"/>
    </DockPane>
  </DockManager>
</Window>`))
	if err != nil {
		t.Fatal(err)
	}
	p, ok := reg["commit"].(*base.DockPane)
	if !ok {
		t.Fatalf("commit — %T", reg["commit"])
	}
	if p.MinSize != 300 {
		t.Fatalf("MinSize из разметки %d", p.MinSize)
	}
	dm := reg["dm"].(*base.DockManager)
	dm.SetBounds(image.Rect(0, 0, 1000, 700))
	if got := p.Bounds().Dx(); got != 300 {
		t.Fatalf("ширина стороны из разметки %d, ждал 300", got)
	}
}
