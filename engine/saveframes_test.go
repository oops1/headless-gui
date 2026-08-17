package engine

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// SaveFrames не пишет больше кадров, чем разрешено лимитом.
func TestSaveFrames_LimitStops(t *testing.T) {
	dir := t.TempDir()
	e := New(64, 64, 20)
	e.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 10, G: 10, B: 10, A: 255})
	root.SetBounds(image.Rect(0, 0, 64, 64))
	e.SetRoot(root)
	e.SaveFrames(dir)
	e.SaveFramesLimit(3)
	// Без Start: кадры и правки виджета — из одной горутины.
	go e.saveWorker()

	for i := 0; i < 12; i++ {
		root.Background = color.RGBA{R: uint8(i * 20), G: 40, B: 60, A: 255}
		e.Invalidate()
		e.renderFrame()
	}
	close(e.saveCh)
	<-e.saveDone

	files, err := filepath.Glob(filepath.Join(dir, "*.png"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > 3 {
		t.Fatalf("сохранено %d PNG при лимите 3", len(files))
	}
	if len(files) == 0 {
		t.Fatal("не сохранено ни одного кадра")
	}
	if _, err := os.Stat(files[0]); err != nil {
		t.Fatalf("файл кадра недоступен: %v", err)
	}
}

// Лимит по умолчанию выставляется в SaveFrames.
func TestSaveFrames_DefaultLimit(t *testing.T) {
	e := New(32, 32, 20)
	e.SaveFrames(t.TempDir())
	if got := e.saveLimit.Load(); got != DefaultSaveFramesLimit {
		t.Fatalf("лимит по умолчанию %d, ожидался %d", got, DefaultSaveFramesLimit)
	}
	e.SaveFramesLimit(0) // без предела
	for i := 0; i < 5; i++ {
		if !e.saveAllowed() {
			t.Fatal("нулевой лимит должен пропускать все кадры")
		}
	}
}
