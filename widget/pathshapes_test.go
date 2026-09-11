package widget

import (
	"image"
	"image/color"
	"testing"
)

// Откат дробных путей для контекста без PathShapes: кривая всё равно ложится —
// той же раскладкой, округлённой до точек.

type pathRecorder struct {
	DrawContext
	cubics, paths, fills int
}

func (r *pathRecorder) StrokeCubicAA(_, _, _, _, _, _, _, _, _ float64, _ color.RGBA) { r.cubics++ }
func (r *pathRecorder) StrokePathAA([]Point2F, float64, bool, color.RGBA)             { r.paths++ }
func (r *pathRecorder) FillPathAA([]Point2F, color.RGBA)                              { r.fills++ }

type rectCounter struct {
	DrawContext
	rects int
}

func (c *rectCounter) FillRect(int, int, int, int, color.RGBA) { c.rects++ }

// С PathShapes помощники отдают путь холсту целиком, без округления.
func TestPathHelpers_ForwardToCanvas(t *testing.T) {
	r := &pathRecorder{}
	col := color.RGBA{A: 255}
	strokeCubic(r, 0, 0, 10, 0, 10, 20, 20, 20, 1.5, col)
	strokePath(r, []Point2F{{0, 0}, {5.5, 3.25}}, 1.5, false, col)
	fillPath(r, []Point2F{{0, 0}, {5, 0}, {2.5, 4}}, col)
	if r.cubics != 1 || r.paths != 1 || r.fills != 1 {
		t.Errorf("до холста дошло: кривых %d, путей %d, заливок %d", r.cubics, r.paths, r.fills)
	}
}

// Без PathShapes кривая рисуется ступеньками, но рисуется.
func TestPathHelpers_FallbackDraws(t *testing.T) {
	c := &rectCounter{}
	strokeCubic(c, 0, 0, 40, 0, 40, 60, 80, 60, 2, color.RGBA{A: 255})
	if c.rects == 0 {
		t.Error("без PathShapes кривая не нарисована")
	}
}

// Округление выбрасывает совпавшие подряд точки: отрезок нулевой длины
// ступенчатому откату рисовать нечего.
func TestPathHelpers_RoundDropsDuplicates(t *testing.T) {
	got := roundPoints([]Point2F{{0, 0}, {0.2, 0.1}, {0.4, 0.3}, {1.6, 0}})
	want := []image.Point{{0, 0}, {2, 0}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("округлено в %v, ожидалось %v", got, want)
	}
}
