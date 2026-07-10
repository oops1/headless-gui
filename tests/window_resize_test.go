package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// ─── Вспомогательные ────────────────────────────────────────────────────────

// pressAt / moveTo / releaseAt — эмуляция мыши на уровне виджета Window.
func pressAt(w *widget.Window, x, y int) {
	w.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
}
func releaseAt(w *widget.Window, x, y int) {
	w.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: false})
}

// assembleFrame собирает полный кадр из дельта-тайлов в *image.RGBA.
func assembleFrame(w, h int, f output.Frame) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for _, t := range f.Tiles {
		for row := 0; row < t.H; row++ {
			for col := 0; col < t.W; col++ {
				si := (row*t.W + col) * 4
				img.Set(t.X+col, t.Y+row, color.RGBA{
					R: t.Data[si], G: t.Data[si+1], B: t.Data[si+2], A: t.Data[si+3],
				})
			}
		}
	}
	return img
}

// firstRenderedFrame возвращает первый непустой кадр (полная начальная отрисовка
// покрывает весь холст).
func firstRenderedFrame(t *testing.T, eng *engine.Engine) output.Frame {
	t.Helper()
	for i := 0; i < 20; i++ {
		f := <-eng.Frames()
		if len(f.Tiles) > 0 {
			return f
		}
	}
	t.Fatal("не дождались непустого кадра")
	return output.Frame{}
}

// xorInv — тестовая копия инверсии (widget.xorBorderColor не экспортируется).
func xorInv(bg color.RGBA) color.RGBA {
	return color.RGBA{R: ^bg.R, G: ^bg.G, B: ^bg.B, A: 255}
}

// ─── Курсор resize-зон ──────────────────────────────────────────────────────

func TestWindowCursor_ResizeEdges(t *testing.T) {
	w := widget.NewWindow("Cur", 400, 300)
	w.SetBounds(image.Rect(100, 100, 500, 400))

	// Правый край (середина) → ↔
	if c := w.Cursor(499, 250); c != widget.CursorSizeWE {
		t.Fatalf("правый край: Cursor = %v, want SizeWE", c)
	}
	// Нижний край → ↕
	if c := w.Cursor(300, 399); c != widget.CursorSizeNS {
		t.Fatalf("нижний край: Cursor = %v, want SizeNS", c)
	}
	// Угол NW → ⤡
	if c := w.Cursor(101, 101); c != widget.CursorSizeNWSE {
		t.Fatalf("угол NW: Cursor = %v, want SizeNWSE", c)
	}
	// Угол NE → ⤢
	if c := w.Cursor(499, 101); c != widget.CursorSizeNESW {
		t.Fatalf("угол NE: Cursor = %v, want SizeNESW", c)
	}
	// Центр → обычная стрелка
	if c := w.Cursor(300, 250); c != widget.CursorArrow {
		t.Fatalf("центр: Cursor = %v, want Arrow", c)
	}
}

// TestWindowCursor_NoResize — при ResizeModeNoResize resize-курсоров нет.
func TestWindowCursor_NoResize(t *testing.T) {
	w := widget.NewWindow("Fixed", 400, 300)
	w.Resize = widget.ResizeModeNoResize
	w.SetBounds(image.Rect(100, 100, 500, 400))
	if c := w.Cursor(499, 250); c != widget.CursorArrow {
		t.Fatalf("NoResize правый край: Cursor = %v, want Arrow", c)
	}
}

// TestWindowCursor_ViaEngine — интеграция через engine.CursorAt.
func TestWindowCursor_ViaEngine(t *testing.T) {
	w := widget.NewWindow("Cur", 400, 300)
	w.SetBounds(image.Rect(0, 0, 400, 300))

	eng := engine.New(400, 300, 1)
	eng.SetRoot(w)

	if c := eng.CursorAt(399, 150); c != widget.CursorSizeWE {
		t.Fatalf("CursorAt правый край = %v, want SizeWE", c)
	}
	if c := eng.CursorAt(200, 150); c != widget.CursorArrow {
		t.Fatalf("CursorAt центр = %v, want Arrow", c)
	}
}

// ─── Механика resize ────────────────────────────────────────────────────────

func TestWindowResize_RightEdge_Grows(t *testing.T) {
	w := widget.NewWindow("R", 400, 300)
	w.SetBounds(image.Rect(100, 100, 500, 400))

	pressAt(w, 499, 250) // правый край
	w.OnMouseMove(560, 250)
	releaseAt(w, 560, 250)

	b := w.Bounds()
	if b.Dx() != 461 { // 400 + (560-499)
		t.Fatalf("ширина = %d, want 461", b.Dx())
	}
	if b.Dy() != 300 {
		t.Fatalf("высота изменилась: %d, want 300", b.Dy())
	}
	if b.Min.X != 100 || b.Min.Y != 100 {
		t.Fatalf("левый/верхний край сдвинулись: Min = %v, want (100,100)", b.Min)
	}
}

func TestWindowResize_CornerNW(t *testing.T) {
	w := widget.NewWindow("R", 400, 300)
	w.SetBounds(image.Rect(100, 100, 500, 400))

	pressAt(w, 101, 101) // угол NW
	w.OnMouseMove(81, 71) // сдвиг (-20,-30)
	releaseAt(w, 81, 71)

	b := w.Bounds()
	if b.Min.X != 80 || b.Min.Y != 70 {
		t.Fatalf("NW Min = %v, want (80,70)", b.Min)
	}
	if b.Max.X != 500 || b.Max.Y != 400 {
		t.Fatalf("NW Max сдвинулся: %v, want (500,400)", b.Max)
	}
}

func TestWindowResize_MinSize(t *testing.T) {
	w := widget.NewWindow("R", 400, 300)
	w.SetBounds(image.Rect(100, 100, 500, 400))

	// Тянем правый край далеко влево — окно не должно схлопнуться уже минимума.
	pressAt(w, 499, 250)
	w.OnMouseMove(150, 250)
	releaseAt(w, 150, 250)

	if b := w.Bounds(); b.Dx() < 120 {
		t.Fatalf("ширина %d пробила минимум 120", b.Dx())
	}
	if b := w.Bounds(); b.Dx() != 120 {
		t.Fatalf("ширина = %d, want ровно 120 (минимум)", b.Dx())
	}
}

func TestWindowResize_NoResizeMode(t *testing.T) {
	w := widget.NewWindow("Fixed", 400, 300)
	w.Resize = widget.ResizeModeNoResize
	w.SetBounds(image.Rect(100, 100, 500, 400))
	before := w.Bounds()

	pressAt(w, 499, 250)
	w.OnMouseMove(560, 250)
	releaseAt(w, 560, 250)

	if w.Bounds() != before {
		t.Fatalf("NoResize: bounds изменились %v → %v", before, w.Bounds())
	}
}

// TestWindowResize_CustomMinSize — публичные MinWidth/MinHeight ограничивают
// виджетный edge-resize вместо захардкоженных 120×80.
func TestWindowResize_CustomMinSize(t *testing.T) {
	w := widget.NewWindow("R", 400, 300)
	w.MinWidth = 250
	w.MinHeight = 200
	w.SetBounds(image.Rect(100, 100, 500, 400))

	// Тянем правый-нижний угол далеко внутрь — не должно схлопнуться ниже минимума.
	pressAt(w, 499, 399)
	w.OnMouseMove(120, 120)
	releaseAt(w, 120, 120)

	b := w.Bounds()
	if b.Dx() != 250 {
		t.Fatalf("ширина = %d, want ровно 250 (MinWidth)", b.Dx())
	}
	if b.Dy() != 200 {
		t.Fatalf("высота = %d, want ровно 200 (MinHeight)", b.Dy())
	}
}

// TestWindowResize_MinSizeDefault — при MinWidth/MinHeight=0 действует прежний
// дефолт 120×80.
func TestWindowResize_MinSizeDefault(t *testing.T) {
	w := widget.NewWindow("R", 400, 300)
	w.SetBounds(image.Rect(100, 100, 500, 400))

	pressAt(w, 499, 250)
	w.OnMouseMove(150, 250)
	releaseAt(w, 150, 250)

	if b := w.Bounds(); b.Dx() != 120 {
		t.Fatalf("ширина = %d, want 120 (дефолтный минимум)", b.Dx())
	}
}

// TestXAMLWindow_MinSize — атрибуты MinWidth/MinHeight парсятся в поля Window.
func TestXAMLWindow_MinSize(t *testing.T) {
	xaml := []byte(`<Window Title="T" Width="800" Height="600" MinWidth="640" MinHeight="480"><Canvas/></Window>`)
	root, _, err := widget.LoadUIFromXAML(xaml)
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}
	w, ok := root.(*widget.Window)
	if !ok {
		t.Fatalf("root тип = %T, want *widget.Window", root)
	}
	if w.MinWidth != 640 {
		t.Fatalf("MinWidth = %d, want 640", w.MinWidth)
	}
	if w.MinHeight != 480 {
		t.Fatalf("MinHeight = %d, want 480", w.MinHeight)
	}
}

// TestXAMLWindow_MinSize_Absent — без атрибутов MinWidth/MinHeight поля = 0.
func TestXAMLWindow_MinSize_Absent(t *testing.T) {
	xaml := []byte(`<Window Title="T" Width="800" Height="600"><Canvas/></Window>`)
	root, _, err := widget.LoadUIFromXAML(xaml)
	if err != nil {
		t.Fatalf("LoadUIFromXAML: %v", err)
	}
	w := root.(*widget.Window)
	if w.MinWidth != 0 || w.MinHeight != 0 {
		t.Fatalf("MinWidth/MinHeight = %d/%d, want 0/0", w.MinWidth, w.MinHeight)
	}
}

// TestWindowResize_NativeGuard — при выставленном OnDragMove (нативный режим)
// виджетный edge-resize отключён, чтобы не конфликтовать с ресайзом ОС.
func TestWindowResize_NativeGuard(t *testing.T) {
	w := widget.NewWindow("Native", 400, 300)
	w.OnDragMove = func(dx, dy int) {} // имитируем нативный режим
	w.SetBounds(image.Rect(100, 100, 500, 400))

	if c := w.Cursor(499, 250); c != widget.CursorArrow {
		t.Fatalf("нативный режим: Cursor = %v, want Arrow", c)
	}
	before := w.Bounds()
	pressAt(w, 499, 250)
	w.OnMouseMove(560, 250)
	releaseAt(w, 560, 250)
	if w.Bounds() != before {
		t.Fatalf("нативный режим: bounds изменились %v → %v", before, w.Bounds())
	}
}

// ─── XOR-рамка (пиксельные проверки) ────────────────────────────────────────

// renderWindowBorderPixel рендерит окно на канвасе и возвращает пиксель на
// левом крае окна (середина высоты).
func renderWindowBorderPixel(t *testing.T, w *widget.Window, canvasW, canvasH int) color.RGBA {
	t.Helper()
	eng := engine.New(canvasW, canvasH, 30)
	eng.SetTooltipsEnabled(false)
	eng.SetRoot(w)
	eng.Start()
	f := firstRenderedFrame(t, eng)
	img := assembleFrame(canvasW, canvasH, f)
	eng.Stop()
	b := w.Bounds()
	return img.RGBAAt(b.Min.X, (b.Min.Y+b.Max.Y)/2)
}

func TestWindowXorBorder_MainWindow_NonClassic(t *testing.T) {
	widget.ApplyGlobalTheme(widget.ThemeByName("Win10 Light"))
	defer widget.ApplyGlobalTheme(widget.ThemeByName("Win10 Dark"))

	w := widget.NewWindow("XOR", 400, 300)
	w.CornerRadius = 0
	w.TitleStyle = widget.WindowTitleWin
	w.Background = color.RGBA{R: 240, G: 240, B: 240, A: 255}
	w.SetBounds(image.Rect(100, 75, 500, 375))

	got := renderWindowBorderPixel(t, w, 600, 450)
	want := xorInv(w.Background) // {15,15,15,255}
	if got != want {
		t.Fatalf("XOR-рамка главного окна: пиксель = %v, want %v", got, want)
	}
}

func TestWindowXorBorder_NotMainWindow(t *testing.T) {
	widget.ApplyGlobalTheme(widget.ThemeByName("Win10 Light"))
	defer widget.ApplyGlobalTheme(widget.ThemeByName("Win10 Dark"))

	w := widget.NewWindow("Nested", 400, 300)
	w.MainWindow = false
	w.CornerRadius = 0
	w.TitleStyle = widget.WindowTitleWin
	w.Background = color.RGBA{R: 240, G: 240, B: 240, A: 255}
	w.SetBounds(image.Rect(100, 75, 500, 375))

	got := renderWindowBorderPixel(t, w, 600, 450)
	if got == xorInv(w.Background) {
		t.Fatalf("не-главное окно не должно иметь XOR-рамку, а пиксель = %v", got)
	}
}

func TestWindowXorBorder_Classic_NotDrawn(t *testing.T) {
	widget.ApplyGlobalTheme(widget.ThemeByName("Win2000"))
	defer widget.ApplyGlobalTheme(widget.ThemeByName("Win10 Dark"))

	w := widget.NewWindow("Classic", 400, 300)
	w.CornerRadius = 0
	w.TitleStyle = widget.WindowTitleWin
	w.SetBounds(image.Rect(100, 75, 500, 375))

	got := renderWindowBorderPixel(t, w, 600, 450)
	if got == xorInv(w.Background) {
		t.Fatalf("Classic3D не должен рисовать XOR-рамку, а пиксель = %v", got)
	}
}
