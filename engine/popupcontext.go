// popupcontext.go — транслирующая обёртка DrawContext для рендера оверлея в
// отдельный буфер. Все координаты оверлея задаются в абсолютных логических
// координатах холста; обёртка вычитает (dx,dy) = Rect.Min, чтобы попасть в
// локальную систему координат маленького канваса попапа.
package engine

import (
	"image"
	"image/color"

	"github.com/oops1/headless-gui/v3/widget"
)

// translatingContext сдвигает все координаты на -(dx,dy) и делегирует inner.
// inner — Canvas размером с оверлей (в физическом масштабе движка). Реализует
// widget.DrawContext и widget.AAShapes (виджеты type-assert'ят AAShapes).
type translatingContext struct {
	inner  *Canvas
	dx, dy int
}

var (
	_ widget.DrawContext = (*translatingContext)(nil)
	_ widget.AAShapes    = (*translatingContext)(nil)
)

// ─── widget.DrawContext ──────────────────────────────────────────────────────

func (t *translatingContext) FillRect(x, y, w, h int, col color.RGBA) {
	t.inner.FillRect(x-t.dx, y-t.dy, w, h, col)
}

func (t *translatingContext) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	t.inner.FillRectAlpha(x-t.dx, y-t.dy, w, h, col)
}

func (t *translatingContext) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	t.inner.FillRoundRect(x-t.dx, y-t.dy, w, h, r, col)
}

func (t *translatingContext) DrawBorder(x, y, w, h int, col color.RGBA) {
	t.inner.DrawBorder(x-t.dx, y-t.dy, w, h, col)
}

func (t *translatingContext) DrawRoundBorder(x, y, w, h, r int, col color.RGBA) {
	t.inner.DrawRoundBorder(x-t.dx, y-t.dy, w, h, r, col)
}

func (t *translatingContext) SetPixel(x, y int, col color.RGBA) {
	t.inner.SetPixel(x-t.dx, y-t.dy, col)
}

func (t *translatingContext) DrawHLine(x, y, length int, col color.RGBA) {
	t.inner.DrawHLine(x-t.dx, y-t.dy, length, col)
}

func (t *translatingContext) DrawVLine(x, y, length int, col color.RGBA) {
	t.inner.DrawVLine(x-t.dx, y-t.dy, length, col)
}

func (t *translatingContext) DrawImage(src image.Image, x, y int) {
	t.inner.DrawImage(src, x-t.dx, y-t.dy)
}

func (t *translatingContext) DrawImageScaled(src image.Image, x, y, w, h int) {
	t.inner.DrawImageScaled(src, x-t.dx, y-t.dy, w, h)
}

func (t *translatingContext) DrawText(text string, x, y int, col color.RGBA) {
	t.inner.DrawText(text, x-t.dx, y-t.dy, col)
}

func (t *translatingContext) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	t.inner.DrawTextSize(text, x-t.dx, y-t.dy, sizePt, col)
}

func (t *translatingContext) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
	t.inner.DrawTextFont(text, x-t.dx, y-t.dy, sizePt, fontName, col)
}

func (t *translatingContext) MeasureText(text string, sizePt float64) int {
	return t.inner.MeasureText(text, sizePt)
}

func (t *translatingContext) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return t.inner.MeasureTextFont(text, sizePt, fontName)
}

func (t *translatingContext) MeasureRunePositions(text string, sizePt float64) []int {
	return t.inner.MeasureRunePositions(text, sizePt)
}

func (t *translatingContext) SetClip(r image.Rectangle) {
	t.inner.SetClip(r.Sub(image.Pt(t.dx, t.dy)))
}

func (t *translatingContext) ClearClip() { t.inner.ClearClip() }

// Clip возвращает область отсечения в АБСОЛЮТНЫХ логических координатах
// (как ожидает виджет): локальный клип inner + (dx,dy).
func (t *translatingContext) Clip() image.Rectangle {
	return t.inner.Clip().Add(image.Pt(t.dx, t.dy))
}

// ─── widget.AAShapes ─────────────────────────────────────────────────────────

func (t *translatingContext) FillEllipseAA(cx, cy, rx, ry int, col color.RGBA) {
	t.inner.FillEllipseAA(cx-t.dx, cy-t.dy, rx, ry, col)
}

func (t *translatingContext) StrokeEllipseAA(cx, cy, rx, ry int, thickness float64, col color.RGBA) {
	t.inner.StrokeEllipseAA(cx-t.dx, cy-t.dy, rx, ry, thickness, col)
}

func (t *translatingContext) FillPolygonAA(pts []image.Point, col color.RGBA) {
	t.inner.FillPolygonAA(t.shift(pts), col)
}

func (t *translatingContext) StrokePolylineAA(pts []image.Point, thickness float64, closed bool, col color.RGBA) {
	t.inner.StrokePolylineAA(t.shift(pts), thickness, closed, col)
}

func (t *translatingContext) DrawLineAA(x1, y1, x2, y2 int, thickness float64, col color.RGBA) {
	t.inner.DrawLineAA(x1-t.dx, y1-t.dy, x2-t.dx, y2-t.dy, thickness, col)
}

// shift возвращает копию точек, сдвинутых на -(dx,dy).
func (t *translatingContext) shift(pts []image.Point) []image.Point {
	if len(pts) == 0 {
		return pts
	}
	off := image.Pt(t.dx, t.dy)
	out := make([]image.Point, len(pts))
	for i, p := range pts {
		out[i] = p.Sub(off)
	}
	return out
}
