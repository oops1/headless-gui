package window

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/output"
)

func redTile() output.DirtyTile {
	return output.DirtyTile{X: 0, Y: 0, W: 1, H: 1, Data: []byte{255, 0, 0, 255}}
}

// Кадр, снятый под прежний размер холста, в новый буфер не ложится: окно
// сменило размер, а кадр уже стоял в очереди. Раньше его тайлы ложились
// обрезанными по краю, и в окне оставалась раскладка прежнего размера.
func TestApplyFrame_SkipsFrameOfOtherSize(t *testing.T) {
	eng := engine.New(851, 570, 30)
	win := New(eng, "size")
	win.current = image.NewRGBA(image.Rect(0, 0, 851, 570))

	stale := output.Frame{Tiles: []output.DirtyTile{redTile()}, Width: 1920, Height: 1080}
	if win.applyFrame(stale) {
		t.Fatal("кадр 1920x1080 принят буфером 851x570")
	}
	if win.current.Pix[0] != 0 || !win.pendingDirty.Empty() {
		t.Fatal("тайл прежнего размера попал в буфер")
	}

	fresh := output.Frame{Tiles: []output.DirtyTile{redTile()}, Width: 851, Height: 570}
	if !win.applyFrame(fresh) || win.current.Pix[0] != 255 {
		t.Fatal("кадр своего размера не наложен")
	}

	// Кадр без размера (собран не движком) — как прежде.
	if !win.applyFrame(output.Frame{Tiles: []output.DirtyTile{redTile()}}) {
		t.Error("кадр без размера отвергнут")
	}
}

// FitScale: буфер окна больше холста на поля — сверка идёт с холстом.
func TestApplyFrame_FitScaleComparesWithCanvas(t *testing.T) {
	eng := engine.New(800, 600, 30)
	win := New(eng, "fit")
	win.SetContentFit(FitScale)
	win.fitBaseW, win.fitBaseH = 800, 600
	win.handleFitResize(2000, 1200) // холст 1600x1200, поля по 200

	pw, ph := eng.PhysicalSize()
	if win.applyFrame(output.Frame{Tiles: []output.DirtyTile{redTile()}, Width: 2000, Height: 1200}) {
		t.Error("кадр размера буфера с полями принят за кадр холста")
	}
	if !win.applyFrame(output.Frame{Tiles: []output.DirtyTile{redTile()}, Width: pw, Height: ph}) {
		t.Errorf("кадр холста %dx%d отвергнут", pw, ph)
	}
	// Масштаб сменился — кадр прежнего холста больше не годится.
	win.handleFitResize(400, 300)
	if win.applyFrame(output.Frame{Tiles: []output.DirtyTile{redTile()}, Width: pw, Height: ph}) {
		t.Error("кадр прежнего масштаба принят после смены размера")
	}
}
