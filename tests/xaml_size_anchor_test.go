// Тесты XAML-билдеров (ENGINE_ISSUES avanpost-pam):
// A — спец-билдеры Canvas/Grid сохраняют XAML Width/Height (alignment);
// D — якорь Right/Bottom без Width/Height не даёт мусорный размер.
package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestXAML_CanvasInGrid_CenterAlignment — Canvas с явными Width/Height и
// Center-alignment внутри Grid центрируется, а не растягивается (issue A).
func TestXAML_CanvasInGrid_CenterAlignment(t *testing.T) {
	const xaml = `<Grid Name="root">
		<Canvas Name="cv" Width="200" Height="100"
			HorizontalAlignment="Center" VerticalAlignment="Center"/>
	</Grid>`
	ui, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatal(err)
	}
	// Размер root задаёт хост (как SetRoot).
	ui.SetBounds(image.Rect(0, 0, 600, 400))

	want := image.Rect(200, 150, 400, 250)
	if got := reg["cv"].Bounds(); got != want {
		t.Errorf("cv.Bounds() = %v, ждали %v (XAML Width/Height потеряны)", got, want)
	}
}

// TestXAML_GridInGrid_CenterAlignment — то же для вложенного Grid (issue A).
func TestXAML_GridInGrid_CenterAlignment(t *testing.T) {
	const xaml = `<Grid Name="root">
		<Grid Name="inner" Width="300" Height="200"
			HorizontalAlignment="Center" VerticalAlignment="Center"/>
	</Grid>`
	ui, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatal(err)
	}
	ui.SetBounds(image.Rect(0, 0, 500, 400))

	want := image.Rect(100, 100, 400, 300)
	if got := reg["inner"].Bounds(); got != want {
		t.Errorf("inner.Bounds() = %v, ждали %v", got, want)
	}
}

// TestXAML_CanvasAnchorOnly_NoGarbageSize — ребёнок только с
// Canvas.Right/Bottom получает дефолтный размер, а не мусорный (issue D).
func TestXAML_CanvasAnchorOnly_NoGarbageSize(t *testing.T) {
	const xaml = `<Canvas Width="400" Height="300">
		<Panel Name="rb" Canvas.Right="10" Canvas.Bottom="10"/>
		<Panel Name="both" Canvas.Left="16" Canvas.Top="40" Canvas.Right="16" Canvas.Bottom="40"/>
	</Canvas>`
	_, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatal(err)
	}

	// Дефолт движка 80×30, прижат к правому-нижнему углу.
	want := image.Rect(400-10-80, 300-10-30, 400-10, 300-10)
	if got := reg["rb"].Bounds(); got != want {
		t.Errorf("rb.Bounds() = %v, ждали %v", got, want)
	}

	// Двухъякорный ребёнок растянут по якорям.
	want = image.Rect(16, 40, 384, 260)
	if got := reg["both"].Bounds(); got != want {
		t.Errorf("both.Bounds() = %v, ждали %v", got, want)
	}
}
