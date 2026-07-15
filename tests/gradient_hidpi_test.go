package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// TestGradientHiDPI_NoBandingVertical проверяет, что на дробном HiDPI-масштабе
// вертикальный градиент интерполируется по ФИЗИЧЕСКОЙ координате: в области
// монотонного логического градиента нет двух подряд одинаковых физических
// строк (иначе — бандинг, ради которого и делался фикс gradient.go).
func TestGradientHiDPI_NoBandingVertical(t *testing.T) {
	eng := engine.New(80, 100, 30)
	eng.SetScale(1.5)
	pw, ph := eng.PhysicalSize() // 120 x 150
	if pw != 120 || ph != 150 {
		t.Fatalf("физический размер %dx%d, ожидался 120x150", pw, ph)
	}

	p := widget.NewPanel(color.RGBA{A: 255})
	p.SetBounds(image.Rect(0, 0, 80, 100))
	p.Gradient = &widget.LinearGradient{
		Stops: []widget.GradientStop{
			{Offset: 0, Color: color.RGBA{R: 0, G: 0, B: 0, A: 255}},
			{Offset: 1, Color: color.RGBA{R: 255, G: 255, B: 255, A: 255}},
		},
	}
	eng.SetRoot(p)
	eng.Start()
	defer eng.Stop()

	f := firstRenderedFrame(t, eng)
	img := assembleFrame(pw, ph, f)

	// Столбец в середине; пропускаем крайние строки (клампинг у стопов).
	x := pw / 2
	dupes := 0
	for y := 3; y < ph-3; y++ {
		if img.RGBAAt(x, y-1) == img.RGBAAt(x, y) {
			dupes++
		}
	}
	if dupes != 0 {
		t.Errorf("вертикальный градиент при scale=1.5: %d пар подряд одинаковых физических строк (бандинг)", dupes)
	}

	// Санити: градиент действительно монотонно растёт (верх темнее низа).
	top := img.RGBAAt(x, 4)
	bot := img.RGBAAt(x, ph-5)
	if top.G >= bot.G {
		t.Errorf("градиент не монотонный: верх G=%d, низ G=%d", top.G, bot.G)
	}
}

// TestGradientHiDPI_NoBandingHorizontal — то же для горизонтального градиента
// (соседние физические столбцы не должны совпадать).
func TestGradientHiDPI_NoBandingHorizontal(t *testing.T) {
	eng := engine.New(100, 60, 30)
	eng.SetScale(1.5)
	pw, ph := eng.PhysicalSize() // 150 x 90

	p := widget.NewPanel(color.RGBA{A: 255})
	p.SetBounds(image.Rect(0, 0, 100, 60))
	p.Gradient = &widget.LinearGradient{
		Horizontal: true,
		Stops: []widget.GradientStop{
			{Offset: 0, Color: color.RGBA{R: 0, G: 0, B: 0, A: 255}},
			{Offset: 1, Color: color.RGBA{R: 255, G: 255, B: 255, A: 255}},
		},
	}
	eng.SetRoot(p)
	eng.Start()
	defer eng.Stop()

	f := firstRenderedFrame(t, eng)
	img := assembleFrame(pw, ph, f)

	y := ph / 2
	dupes := 0
	for x := 3; x < pw-3; x++ {
		if img.RGBAAt(x-1, y) == img.RGBAAt(x, y) {
			dupes++
		}
	}
	if dupes != 0 {
		t.Errorf("горизонтальный градиент при scale=1.5: %d пар подряд одинаковых физических столбцов (бандинг)", dupes)
	}
}

// TestGradientScale1_Unchanged — на целом масштабе поведение прежнее: каждая
// логическая строка — сплошная полоса (fallback-путь), градиент рисуется
// построчно. Проверяем, что на scale=1 путь тождественный (нет паники, есть
// монотонность), поскольку golden-сцены на scale=1 меняться не должны.
func TestGradientScale1_Unchanged(t *testing.T) {
	eng := engine.New(40, 40, 30)
	pw, ph := eng.PhysicalSize()
	if pw != 40 || ph != 40 {
		t.Fatalf("scale=1: физический размер %dx%d, ожидался 40x40", pw, ph)
	}
	p := widget.NewPanel(color.RGBA{A: 255})
	p.SetBounds(image.Rect(0, 0, 40, 40))
	p.Gradient = &widget.LinearGradient{
		Stops: []widget.GradientStop{
			{Offset: 0, Color: color.RGBA{R: 0, G: 0, B: 0, A: 255}},
			{Offset: 1, Color: color.RGBA{R: 255, G: 255, B: 255, A: 255}},
		},
	}
	eng.SetRoot(p)
	eng.Start()
	defer eng.Stop()

	f := firstRenderedFrame(t, eng)
	img := assembleFrame(pw, ph, f)
	if img.RGBAAt(20, 2).G >= img.RGBAAt(20, 37).G {
		t.Errorf("scale=1: градиент не монотонный")
	}
}
