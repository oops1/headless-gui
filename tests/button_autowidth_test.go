package tests

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"

	"github.com/oops1/headless-gui/v3/widget"
)

// GG-66: кнопка без Width в DockPanel получала ровно 80 точек, не глядя ни на
// подпись, ни на значок, и «Рабочая копия» со значком показывалась «Рабоч…».

func pngIcon(t *testing.T, size int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	img.Set(0, 0, color.RGBA{R: 0x20, G: 0x80, B: 0xE0, A: 0xff})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// Длинная подпись со значком не обрезается, короткая кнопка остаётся прежних
// 80 точек — раскладки, где их хватало, не меняются.
func TestButton_AutoWidthInDockPanel(t *testing.T) {
	fsys := fstest.MapFS{"icons/wc.png": {Data: pngIcon(t, 16)}}
	root, reg, err := widget.LoadUIFromXAMLFS([]byte(`<DockPanel Width="800" Height="32">
  <Button x:Name="wc" DockPanel.Dock="Left" Content="Рабочая копия" Icon="icons/wc.png"/>
  <Button x:Name="ok" DockPanel.Dock="Left" Content="OK"/>
  <Label Content="остаток"/>
</DockPanel>`), fsys)
	if err != nil {
		t.Fatalf("разметка: %v", err)
	}
	root.SetBounds(image.Rect(0, 0, 800, 32))

	wc, ok := reg["wc"].(*widget.Button), reg["ok"].(*widget.Button)
	if wc.Icon == nil {
		t.Fatal("подготовка: значок кнопки не загрузился")
	}

	text := widget.MeasureUIText("Рабочая копия", widget.DefaultFontSizePt)
	// Значок, зазор и поля — не меньше 16 + 4 + 14 сверх подписи.
	if got, need := wc.Bounds().Dx(), text+16+4+14; got < need {
		t.Fatalf("кнопка «Рабочая копия» шириной %d, а подписи со значком нужно не меньше %d", got, need)
	}
	if got := ok.Bounds().Dx(); got != 80 {
		t.Fatalf("короткая кнопка шириной %d — прежние 80 должны остаться нижней границей", got)
	}
	// Кнопки стоят друг за другом, а не поверх: вторая начинается за первой.
	if ok.Bounds().Min.X < wc.Bounds().Max.X {
		t.Fatalf("кнопки наложились: %v и %v", wc.Bounds(), ok.Bounds())
	}
}

// Заданный Width по-прежнему главнее измерения.
func TestButton_ExplicitWidthWins(t *testing.T) {
	root, reg, err := widget.LoadUIFromXAMLFS([]byte(`<DockPanel Width="800" Height="32">
  <Button x:Name="b" DockPanel.Dock="Left" Width="60" Content="Очень длинная подпись кнопки"/>
  <Label Content="остаток"/>
</DockPanel>`), fstest.MapFS{})
	if err != nil {
		t.Fatalf("разметка: %v", err)
	}
	root.SetBounds(image.Rect(0, 0, 800, 32))
	if got := reg["b"].(*widget.Button).Bounds().Dx(); got != 60 {
		t.Fatalf("Width=60 не соблюдён: ширина %d", got)
	}
}
