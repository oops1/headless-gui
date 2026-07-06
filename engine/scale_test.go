package engine

import (
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Смежные логические прямоугольники на дробном масштабе не оставляют
// ни щелей, ни нахлёстов (edge-based округление).
func TestScale_NoGapsAdjacentRects(t *testing.T) {
	eng := New(100, 100, 20)
	eng.SetScale(1.5)
	c := eng.canvas
	if c.W != 150 || c.H != 150 {
		t.Fatalf("физический размер %dx%d, ожидался 150x150", c.W, c.H)
	}
	c.blitBackground()

	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	// Шахматка смежных ячеек 7x7 логических (7*1.5=10.5 — дробная граница).
	for gy := 0; gy < 6; gy++ {
		for gx := 0; gx < 6; gx++ {
			col := red
			if (gx+gy)%2 == 1 {
				col = blue
			}
			c.FillRect(gx*7, gy*7, 7, 7, col)
		}
	}
	// Вся физическая область [0,63)x[0,63) обязана быть закрашена только
	// красным или синим — чёрный пиксель означал бы щель.
	maxP := c.sx(42)
	for y := 0; y < maxP; y++ {
		for x := 0; x < maxP; x++ {
			off := c.back.PixOffset(x, y)
			r, g, b := c.back.Pix[off], c.back.Pix[off+1], c.back.Pix[off+2]
			if g != 0 || (r == 0 && b == 0) || (r != 0 && b != 0) {
				t.Fatalf("щель/нахлёст в физическом пикселе (%d,%d): rgb=%d,%d,%d", x, y, r, g, b)
			}
		}
	}
}

// Логическая ширина текста стабильна между масштабами (±2px на округление),
// а физическая растёт пропорционально.
func TestScale_TextMeasureStable(t *testing.T) {
	eng1 := New(400, 100, 20)
	w1 := eng1.canvas.MeasureText("Пример Example 123", 12)

	eng2 := New(400, 100, 20)
	eng2.SetScale(2)
	w2 := eng2.canvas.MeasureText("Пример Example 123", 12)

	if diff := w2 - w1; diff < -2 || diff > 2 {
		t.Errorf("логическая ширина при 2x = %d, при 1x = %d (дрейф %d)", w2, w1, diff)
	}
	// Физическое измерение при 2x примерно вдвое больше логического.
	phys := eng2.canvas.measureWithFallback(eng2.canvas.fontCache, "Пример Example 123", 12)
	if phys < w1*2-4 || phys > w1*2+8 {
		t.Errorf("физическая ширина %d, ожидалась ~%d", phys, w1*2)
	}
}

// События мыши приходят в физических пикселях и делятся на масштаб.
func TestScale_MouseEventsDivide(t *testing.T) {
	eng := New(300, 200, 20)
	eng.SetScale(2)
	btn := widget.NewButton("btn")
	btn.SetBounds(image.Rect(50, 50, 150, 80)) // логические
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 300, 200))
	root.AddChild(btn)
	eng.SetRoot(root)

	eng.SendMouseMove(200, 130) // физические → логические (100, 65) → внутри кнопки
	if !btn.IsHovered() {
		t.Error("кнопка не получила hover: физические координаты не поделились на масштаб")
	}
	eng.SendMouseMove(600, 396) // → (300, 198) → вне кнопки
	if btn.IsHovered() {
		t.Error("hover не снялся")
	}
}

// SetScale сохраняет именованные шрифты и fallback-цепочку (cloneForSize).
func TestScale_PreservesFonts(t *testing.T) {
	eng := New(200, 100, 20)
	before := len(eng.AvailableFonts())
	if before == 0 {
		t.Skip("нет именованных шрифтов (assets не найдены)")
	}
	fallbacks := len(eng.canvas.fallbacks)
	eng.SetScale(2)
	if after := len(eng.AvailableFonts()); after != before {
		t.Errorf("именованные шрифты потеряны при SetScale: %d → %d", before, after)
	}
	if got := len(eng.canvas.fallbacks); got != fallbacks {
		t.Errorf("fallback-шрифты потеряны при SetScale: %d → %d", fallbacks, got)
	}
}

// SetResolution тоже сохраняет шрифты (латентный баг до HiDPI).
func TestScale_SetResolutionPreservesFonts(t *testing.T) {
	eng := New(200, 100, 20)
	fallbacks := len(eng.canvas.fallbacks)
	if fallbacks == 0 {
		t.Skip("нет fallback-шрифтов")
	}
	eng.SetResolution(400, 300)
	if got := len(eng.canvas.fallbacks); got != fallbacks {
		t.Errorf("fallback-шрифты потеряны при SetResolution: %d → %d", fallbacks, got)
	}
}

// Кадры (тайлы) — физические: damage от логического InvalidateRect
// покрывает соответствующие физические тайлы.
func TestScale_FramesPhysical(t *testing.T) {
	eng := New(100, 100, 20)
	eng.SetScale(2)
	if pw, ph := eng.PhysicalSize(); pw != 200 || ph != 200 {
		t.Fatalf("PhysicalSize = %dx%d, ожидался 200x200", pw, ph)
	}
	if lw, lh := eng.CanvasSize(); lw != 100 || lh != 100 {
		t.Fatalf("CanvasSize = %dx%d, ожидался 100x100 (логический)", lw, lh)
	}
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.SetBounds(image.Rect(0, 0, 100, 100))
	eng.SetRoot(root)
	eng.renderFrame() // первый кадр — синхронизация front

	// Появление виджета в нижнем правом ЛОГИЧЕСКОМ квадранте: авто-damage
	// (AddChild) должен отскейлиться в физический (100,100)-(200,200).
	lbl := widget.NewLabel("X", color.RGBA{R: 255, A: 255})
	lbl.HasBG = true
	lbl.Background = color.RGBA{R: 200, A: 255}
	lbl.SetBounds(image.Rect(50, 50, 100, 100))
	root.AddChild(lbl)

	frame := eng.renderFrame()
	if len(frame.Tiles) == 0 {
		t.Fatal("нет тайлов после появления виджета")
	}
	for _, tile := range frame.Tiles {
		if tile.X+tile.W <= 100 && tile.Y+tile.H <= 100 {
			t.Errorf("тайл (%d,%d) целиком в верхне-левом физическом квадранте — damage не отскейлился", tile.X, tile.Y)
		}
	}
}

// TestScale_VisualSample рендерит одну сцену при 1x и 2x
// (HEADLESS_GUI_SCALE_PNG_PREFIX=path — сохранит path_1x.png и path_2x.png).
func TestScale_VisualSample(t *testing.T) {
	prefix := os.Getenv("HEADLESS_GUI_SCALE_PNG_PREFIX")
	if prefix == "" {
		t.Skip("HEADLESS_GUI_SCALE_PNG_PREFIX не задан")
	}
	for _, k := range []float64{1, 2} {
		eng := New(300, 120, 20)
		eng.SetScale(k)
		c := eng.canvas
		c.blitBackground()
		accent := color.RGBA{R: 0, G: 120, B: 212, A: 255}
		white := color.RGBA{R: 235, G: 235, B: 235, A: 255}
		c.FillRoundRect(12, 12, 130, 34, 8, accent)
		c.DrawTextSize("Кнопка Button", 24, 22, 12, white)
		c.DrawRoundBorder(160, 12, 120, 34, 8, white)
		c.DrawTextSize("Мир Hello مرحبا", 12, 60, 14, white)
		c.FillEllipseAA(40, 100, 14, 10, accent)
		c.DrawLineAA(80, 92, 270, 108, 2, white)
		name := prefix + "_1x.png"
		if k == 2 {
			name = prefix + "_2x.png"
		}
		savePNG(c.back, name)
		t.Logf("сохранено: %s", name)
	}
}
