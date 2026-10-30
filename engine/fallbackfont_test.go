package engine

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

func newLabelRoot(text string) widget.Widget {
	l := widget.NewLabel(text, color255)
	l.SetBounds(image.Rect(0, 0, 300, 40))
	return l
}

// ─── E16: ленивая загрузка шрифтов запасной цепочки ──────────────────────────

// engine.New не читает ни одного файла запасной цепочки — какие бы шрифты ни
// стояли в системе.
func TestEngineNew_ReadsNoFallbackFiles(t *testing.T) {
	testreset.Globals(t)
	reads := hookFallbackReads(t)
	eng := New(320, 200, 30)
	defer eng.Stop()
	if n := reads.count(); n != 0 {
		t.Fatalf("New прочитал запасные шрифты: %v", reads.list())
	}
	// Обычный текст не читает их и в кадре.
	eng.SetRoot(newLabelRoot("Hello 123 Привет"))
	eng.renderFrame()
	if n := reads.count(); n != 0 {
		t.Fatalf("кадр с обычным текстом прочитал запасные шрифты: %v", reads.list())
	}
}
