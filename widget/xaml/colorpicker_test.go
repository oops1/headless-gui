package xaml

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
)

func TestColorPicker_XAML(t *testing.T) {
	const src = `<Canvas Width="400" Height="200">
  <ColorPicker Name="accent" Left="10" Top="10" Width="180" Height="28"
               Value="#0078D7" Palette="#000000, #FFFFFF, #E81123"/>
  <Swatch Name="mark" Left="10" Top="50" Width="18" Height="18" Color="#C42B1C"/>
</Canvas>`
	root, reg, err := LoadUIFromXAML([]byte(src))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	defer ReleaseXAML(root)

	p, ok := reg["accent"].(*base.ColorPicker)
	if !ok {
		t.Fatalf("accent — %T, ждал *ColorPicker", reg["accent"])
	}
	if got := p.Value(); got != (color.RGBA{R: 0x00, G: 0x78, B: 0xD7, A: 255}) {
		t.Errorf("Value = %v", got)
	}
	if got := p.Palette(); len(got) != 3 || got[2] != (color.RGBA{R: 0xE8, G: 0x11, B: 0x23, A: 255}) {
		t.Errorf("палитра из разметки: %v", got)
	}

	s, ok := reg["mark"].(*base.Swatch)
	if !ok {
		t.Fatalf("mark — %T, ждал *Swatch", reg["mark"])
	}
	if s.Color != (color.RGBA{R: 0xC4, G: 0x2B, B: 0x1C, A: 255}) {
		t.Errorf("цвет образца %v", s.Color)
	}
}
