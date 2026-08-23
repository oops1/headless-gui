package window

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/output"
)

// TestHandleFitResize — FitScale: логическое разрешение фиксировано,
// масштаб = min(w/baseW, h/baseH), контент центрируется (letterbox).
func TestHandleFitResize(t *testing.T) {
	eng := engine.New(800, 600, 30)
	win := New(eng, "fit")
	win.SetContentFit(FitScale)
	win.fitBaseW, win.fitBaseH = 800, 600

	// Ровно ×2 — без полей.
	win.handleFitResize(1600, 1200)
	if win.scale != 2 {
		t.Fatalf("scale = %v, ждали 2", win.scale)
	}
	if lw, lh := eng.CanvasSize(); lw != 800 || lh != 600 {
		t.Fatalf("логическое разрешение %dx%d, ждали 800x600", lw, lh)
	}
	if win.fitOX != 0 || win.fitOY != 0 {
		t.Fatalf("офсеты %d,%d, ждали 0,0", win.fitOX, win.fitOY)
	}

	// Широкое окно — та же высота, поля слева/справа по 200.
	win.handleFitResize(2000, 1200)
	if win.scale != 2 {
		t.Fatalf("scale = %v, ждали 2 (лимит по высоте)", win.scale)
	}
	if win.fitOX != 200 || win.fitOY != 0 {
		t.Fatalf("офсеты %d,%d, ждали 200,0", win.fitOX, win.fitOY)
	}
	if b := win.current.Bounds(); b.Dx() != 2000 || b.Dy() != 1200 {
		t.Fatalf("буфер %v, ждали 2000x1200", b)
	}
	// Поля непрозрачно-чёрные.
	if win.current.Pix[3] != 255 {
		t.Error("letterbox-поле прозрачное, ждали A=255")
	}

	// Ввод: координаты окна → координаты контента.
	if x, y := win.toContent(210, 5); x != 10 || y != 5 {
		t.Errorf("toContent(210,5) = %d,%d, ждали 10,5", x, y)
	}

	// Тайлы кадра ложатся в буфер со сдвигом letterbox.
	tile := output.DirtyTile{X: 0, Y: 0, W: 2, H: 1, Data: []byte{1, 2, 3, 255, 4, 5, 6, 255}}
	win.applyFrame(output.Frame{Tiles: []output.DirtyTile{tile}})
	if got := win.current.RGBAAt(200, 0); got.R != 1 || got.G != 2 || got.B != 3 {
		t.Errorf("пиксель тайла в (200,0) = %v, ждали 1,2,3", got)
	}
	want := image.Rect(200, 0, 202, 1)
	if win.pendingDirty != want {
		t.Errorf("pendingDirty = %v, ждали %v", win.pendingDirty, want)
	}

	// Уменьшение окна — масштаб < 1.
	win.handleFitResize(400, 300)
	if win.scale != 0.5 {
		t.Errorf("scale = %v, ждали 0.5", win.scale)
	}
}
