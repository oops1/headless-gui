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

// DrawTextRotated рисует повёрнутую строку в буфере попапа (см.
// widget.RotatedTextDrawer): без этого вертикальная полоса меню «Пуск» в
// отдельном окне пропадала бы.
func (t *translatingContext) DrawTextRotated(text string, x, y int, sizePt float64, fontName string, angle int, col color.RGBA) bool {
	return t.inner.DrawTextRotated(text, x-t.dx, y-t.dy, sizePt, fontName, angle, col)
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

// Scale сообщает HiDPI-масштаб буфера попапа (widget.ContextScale): векторные
// значки внутри всплывающей области растеризуются в физическом размере, а не
// растягиваются движком.
func (t *translatingContext) Scale() float64 { return t.inner.Scale() }

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

// ─── Опциональные возможности контекста ─────────────────────────────────────
//
// Размытие подложки, мягкая тень и скруглённый клип — не часть DrawContext, а
// возможности, о которых виджет спрашивает приведением типа (BackdropDrawer,
// ShadowDrawer, RoundClipper). Обёртка, не реализующая их, делает слой
// «глухим»: PaintStyle не находит ни тени, ни скругления и рисует панель
// плоским прямоугольником — именно это случалось с оверлеями, вынесенными в
// отдельное окно. Здесь они пробрасываются в нижележащий канвас с тем же
// переводом координат, что и остальное рисование.

var (
	_ widget.BackdropDrawer    = (*translatingContext)(nil)
	_ widget.ShadowDrawer      = (*translatingContext)(nil)
	_ widget.ShadowParamDrawer = (*translatingContext)(nil)
	_ widget.MicaDrawer        = (*translatingContext)(nil)
	_ widget.RoundClipper      = (*translatingContext)(nil)
	_ widget.OpacityDrawer     = (*translatingContext)(nil)

	_ widget.RotatedTextDrawer = (*translatingContext)(nil)
)

// BlurBehind размывает уже нарисованное в r (в буфере попапа) и подкрашивает.
func (t *translatingContext) BlurBehind(r image.Rectangle, radius int, tint color.RGBA) {
	t.inner.BlurBehind(r.Sub(image.Pt(t.dx, t.dy)), radius, tint)
}

// MicaBehind кладёт размытые обои в r. Буфер попапа — окошко основного
// холста, поэтому обои берутся у него со сдвигом (dx, dy).
func (t *translatingContext) MicaBehind(r image.Rectangle, radius int, tint color.RGBA) bool {
	return t.inner.micaAt(r.Sub(image.Pt(t.dx, t.dy)), image.Pt(t.dx, t.dy), radius, tint)
}

// DrawShadow рисует мягкую тень с явными размытием и смещением.
func (t *translatingContext) DrawShadow(r image.Rectangle, corner int, blur, offsetX, offsetY float64, col color.RGBA) {
	t.inner.DrawShadow(r.Sub(image.Pt(t.dx, t.dy)), corner, blur, offsetX, offsetY, col)
}

// DrawSoftShadow рисует мягкую тень под r.
func (t *translatingContext) DrawSoftShadow(r image.Rectangle, corner int, elevation float64, col color.RGBA) {
	t.inner.DrawSoftShadow(r.Sub(image.Pt(t.dx, t.dy)), corner, elevation, col)
}

// SetRoundClip включает отсечение по скруглённому контуру r.
func (t *translatingContext) SetRoundClip(r image.Rectangle, radius int) {
	t.inner.SetRoundClip(r.Sub(image.Pt(t.dx, t.dy)), radius)
}

// ClearRoundClip снимает скруглённое отсечение (прямоугольное остаётся).
func (t *translatingContext) ClearRoundClip() { t.inner.ClearRoundClip() }

// DrawWithOpacity рисует draw в слое с прозрачностью alpha (см.
// Canvas.DrawWithOpacity). Нужно панелям, которые выезжают с проявлением: в
// отдельном окне прозрачность слоя означает смесь с прозрачным буфером окна.
func (t *translatingContext) DrawWithOpacity(r image.Rectangle, alpha float64, draw func()) {
	t.inner.DrawWithOpacity(r.Sub(image.Pt(t.dx, t.dy)), alpha, draw)
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
