package desktop

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
)

// clipCtx — контекст со скруглённым клипом, как у движка: SetRoundClip заменяет
// прямоугольный клип рамкой слоя. FillRoundRect запоминает клип в момент заливки.
type clipCtx struct {
	recCtx
	clip   image.Rectangle
	filled []image.Rectangle // клип на момент каждой заливки скруглённого слоя
}

func (c *clipCtx) Clip() image.Rectangle                 { return c.clip }
func (c *clipCtx) SetClip(r image.Rectangle)             { c.clip = r }
func (c *clipCtx) SetRoundClip(r image.Rectangle, _ int) { c.clip = r }
func (c *clipCtx) ClearRoundClip()                       {}
func (c *clipCtx) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	c.filled = append(c.filled, c.clip)
}

// Скруглённый слой (карточка на краю окна списка) не вылезает за клип
// вызывающего: SetRoundClip подменяет прямоугольный клип рамкой слоя, и без
// возврата пересечения слой ложился поверх соседей.
func TestPaintStyle_RoundedLayerStaysInsideCallerClip(t *testing.T) {
	ctx := &clipCtx{clip: image.Rect(0, 0, 100, 50)}
	st := &theme.Style{Fill: theme.RGB(200, 200, 200), Corner: 4}
	card := image.Rect(10, 30, 90, 120) // выступает за низ клипа
	PaintStyle(ctx, card, st)
	if len(ctx.filled) != 1 {
		t.Fatalf("слой не залит: %d", len(ctx.filled))
	}
	if got := ctx.filled[0]; !got.In(image.Rect(0, 0, 100, 50)) {
		t.Errorf("слой залит в клипе %v, вышедшем за клип вызывающего", got)
	}
	if ctx.clip != image.Rect(0, 0, 100, 50) {
		t.Errorf("клип вызывающего не возвращён: %v", ctx.clip)
	}
}
