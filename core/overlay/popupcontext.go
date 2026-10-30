// popupcontext.go — транслирующая обёртка DrawContext для рендера оверлея в
// отдельный буфер. Все координаты оверлея задаются в абсолютных логических
// координатах холста; обёртка вычитает (dx,dy) = Rect.Min, чтобы попасть в
// локальную систему координат маленького канваса попапа.

package overlay

import (
	"image"
	"image/color"

	"github.com/oops1/headless-gui/v3/core/raster"
	"github.com/oops1/headless-gui/v3/widget"
)

// Translating сдвигает все координаты на -(dx,dy) и делегирует inner.
// inner — raster.Canvas размером с оверлей (в физическом масштабе движка). Реализует
// widget.DrawContext и widget.AAShapes (виджеты type-assert'ят AAShapes).
type Translating struct {
	Inner  *raster.Canvas
	DX, DY int
}

var (
	_ widget.DrawContext       = (*Translating)(nil)
	_ widget.AAShapes          = (*Translating)(nil)
	_ widget.OffscreenRenderer = (*Translating)(nil)
)

// ─── widget.DrawContext ──────────────────────────────────────────────────────

func (t *Translating) FillRect(x, y, w, h int, col color.RGBA) {
	t.Inner.FillRect(x-t.DX, y-t.DY, w, h, col)
}

func (t *Translating) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	t.Inner.FillRectAlpha(x-t.DX, y-t.DY, w, h, col)
}

func (t *Translating) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	t.Inner.FillRoundRect(x-t.DX, y-t.DY, w, h, r, col)
}

func (t *Translating) DrawBorder(x, y, w, h int, col color.RGBA) {
	t.Inner.DrawBorder(x-t.DX, y-t.DY, w, h, col)
}

func (t *Translating) DrawRoundBorder(x, y, w, h, r int, col color.RGBA) {
	t.Inner.DrawRoundBorder(x-t.DX, y-t.DY, w, h, r, col)
}

func (t *Translating) SetPixel(x, y int, col color.RGBA) {
	t.Inner.SetPixel(x-t.DX, y-t.DY, col)
}

func (t *Translating) DrawHLine(x, y, length int, col color.RGBA) {
	t.Inner.DrawHLine(x-t.DX, y-t.DY, length, col)
}

func (t *Translating) DrawVLine(x, y, length int, col color.RGBA) {
	t.Inner.DrawVLine(x-t.DX, y-t.DY, length, col)
}

func (t *Translating) DrawImage(src image.Image, x, y int) {
	t.Inner.DrawImage(src, x-t.DX, y-t.DY)
}

func (t *Translating) DrawImageScaled(src image.Image, x, y, w, h int) {
	t.Inner.DrawImageScaled(src, x-t.DX, y-t.DY, w, h)
}

func (t *Translating) DrawText(text string, x, y int, col color.RGBA) {
	t.Inner.DrawText(text, x-t.DX, y-t.DY, col)
}

func (t *Translating) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	t.Inner.DrawTextSize(text, x-t.DX, y-t.DY, sizePt, col)
}

func (t *Translating) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
	t.Inner.DrawTextFont(text, x-t.DX, y-t.DY, sizePt, fontName, col)
}

// DrawTextRotated рисует повёрнутую строку в буфере попапа (см.
// widget.RotatedTextDrawer): без этого вертикальная полоса меню «Пуск» в
// отдельном окне пропадала бы.
func (t *Translating) DrawTextRotated(text string, x, y int, sizePt float64, fontName string, angle int, col color.RGBA) bool {
	return t.Inner.DrawTextRotated(text, x-t.DX, y-t.DY, sizePt, fontName, angle, col)
}

func (t *Translating) MeasureText(text string, sizePt float64) int {
	return t.Inner.MeasureText(text, sizePt)
}

func (t *Translating) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return t.Inner.MeasureTextFont(text, sizePt, fontName)
}

func (t *Translating) MeasureRunePositions(text string, sizePt float64) []int {
	return t.Inner.MeasureRunePositions(text, sizePt)
}

func (t *Translating) SetClip(r image.Rectangle) {
	t.Inner.SetClip(r.Sub(image.Pt(t.DX, t.DY)))
}

func (t *Translating) ClearClip() { t.Inner.ClearClip() }

// Clip возвращает область отсечения в АБСОЛЮТНЫХ логических координатах
// (как ожидает виджет): локальный клип inner + (dx,dy).
func (t *Translating) Clip() image.Rectangle {
	return t.Inner.Clip().Add(image.Pt(t.DX, t.DY))
}

// Scale сообщает HiDPI-масштаб буфера попапа (widget.ContextScale): векторные
// значки внутри всплывающей области растеризуются в физическом размере, а не
// растягиваются движком.
func (t *Translating) Scale() float64 { return t.Inner.Scale() }

// ─── widget.AAShapes ─────────────────────────────────────────────────────────

func (t *Translating) FillEllipseAA(cx, cy, rx, ry int, col color.RGBA) {
	t.Inner.FillEllipseAA(cx-t.DX, cy-t.DY, rx, ry, col)
}

func (t *Translating) StrokeEllipseAA(cx, cy, rx, ry int, thickness float64, col color.RGBA) {
	t.Inner.StrokeEllipseAA(cx-t.DX, cy-t.DY, rx, ry, thickness, col)
}

func (t *Translating) FillPolygonAA(pts []image.Point, col color.RGBA) {
	t.Inner.FillPolygonAA(t.shift(pts), col)
}

func (t *Translating) StrokePolylineAA(pts []image.Point, thickness float64, closed bool, col color.RGBA) {
	t.Inner.StrokePolylineAA(t.shift(pts), thickness, closed, col)
}

func (t *Translating) DrawLineAA(x1, y1, x2, y2 int, thickness float64, col color.RGBA) {
	t.Inner.DrawLineAA(x1-t.DX, y1-t.DY, x2-t.DX, y2-t.DY, thickness, col)
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
	_ widget.BackdropDrawer    = (*Translating)(nil)
	_ widget.ShadowDrawer      = (*Translating)(nil)
	_ widget.ShadowParamDrawer = (*Translating)(nil)
	_ widget.MicaDrawer        = (*Translating)(nil)
	_ widget.RoundClipper      = (*Translating)(nil)
	_ widget.OpacityDrawer     = (*Translating)(nil)

	_ widget.RotatedTextDrawer = (*Translating)(nil)

	_ widget.ImageRectDrawer       = (*Translating)(nil)
	_ widget.ImageCacheInvalidator = (*Translating)(nil)
	_ widget.PixelBlitter          = (*Translating)(nil)
)

// BlitPixels кладёт часть чужого буфера в буфер попапа пиксель в пиксель
// (widget.PixelBlitter): без пробрасывания внешняя поверхность внутри попапа
// шла бы запасным путём через DrawImage.
func (t *Translating) BlitPixels(src *image.RGBA, sr image.Rectangle, x, y int, opaque bool) {
	t.Inner.BlitPixels(src, sr, x-t.DX, y-t.DY, opaque)
}

// DrawImageRect рисует часть изображения с выбором фильтра в буфер попапа
// (widget.ImageRectDrawer): без пробрасывания картинка внутри попапа шла бы
// запасным путём — целиком и билинейно.
func (t *Translating) DrawImageRect(src image.Image, sr, dr image.Rectangle, f widget.ImageFilter) {
	t.Inner.DrawImageRect(src, sr, dr.Sub(image.Pt(t.DX, t.DY)), f)
}

// HasGlyph — есть ли глиф в шрифте по умолчанию (widget.GlyphCoverage).
func (t *Translating) HasGlyph(r rune) bool { return t.Inner.HasGlyph(r) }

// InvalidateImageCache сбрасывает кэш масштабирования в канвасе попапа и, через
// общий журнал движка, во всех остальных (widget.ImageCacheInvalidator).
func (t *Translating) InvalidateImageCache(src image.Image) {
	t.Inner.InvalidateImageCache(src)
}

// BlurBehind размывает уже нарисованное в r (в буфере попапа) и подкрашивает.
func (t *Translating) BlurBehind(r image.Rectangle, radius int, tint color.RGBA) {
	t.Inner.BlurBehind(r.Sub(image.Pt(t.DX, t.DY)), radius, tint)
}

// MicaBehind кладёт размытые обои в r. Буфер попапа — окошко основного
// холста, поэтому обои берутся у него со сдвигом (dx, dy).
func (t *Translating) MicaBehind(r image.Rectangle, radius int, tint color.RGBA) bool {
	return t.Inner.MicaAt(r.Sub(image.Pt(t.DX, t.DY)), image.Pt(t.DX, t.DY), radius, tint)
}

// DrawShadow рисует мягкую тень с явными размытием и смещением.
func (t *Translating) DrawShadow(r image.Rectangle, corner int, blur, offsetX, offsetY float64, col color.RGBA) {
	t.Inner.DrawShadow(r.Sub(image.Pt(t.DX, t.DY)), corner, blur, offsetX, offsetY, col)
}

// DrawSoftShadow рисует мягкую тень под r.
func (t *Translating) DrawSoftShadow(r image.Rectangle, corner int, elevation float64, col color.RGBA) {
	t.Inner.DrawSoftShadow(r.Sub(image.Pt(t.DX, t.DY)), corner, elevation, col)
}

// SetRoundClip включает отсечение по скруглённому контуру r.
func (t *Translating) SetRoundClip(r image.Rectangle, radius int) {
	t.Inner.SetRoundClip(r.Sub(image.Pt(t.DX, t.DY)), radius)
}

// ClearRoundClip снимает скруглённое отсечение (прямоугольное остаётся).
func (t *Translating) ClearRoundClip() { t.Inner.ClearRoundClip() }

// DrawWithOpacity рисует draw в слое с прозрачностью alpha (см.
// raster.Canvas.DrawWithOpacity). Нужно панелям, которые выезжают с проявлением: в
// отдельном окне прозрачность слоя означает смесь с прозрачным буфером окна.
func (t *Translating) DrawWithOpacity(r image.Rectangle, alpha float64, draw func()) {
	t.Inner.DrawWithOpacity(r.Sub(image.Pt(t.DX, t.DY)), alpha, draw)
}

// RenderOffscreen рисует draw в картинку вне буфера попапа
// (widget.OffscreenRenderer): область сдвигается в систему буфера, а draw
// получает контекст картинки с тем же сдвигом — рисует в абсолютных
// координатах, как и весь оверлей. Без этого хостовый оверлей («Пуск» в окне
// ОС) не строил бы кэши перехода живой плитки (ADR-0010).
func (t *Translating) RenderOffscreen(r image.Rectangle, draw func(ctx widget.DrawContext)) *image.RGBA {
	if draw == nil {
		return nil
	}
	return t.Inner.RenderOffscreen(r.Sub(image.Pt(t.DX, t.DY)), func(c widget.DrawContext) {
		inner, ok := c.(*raster.Canvas)
		if !ok {
			return
		}
		draw(&Translating{Inner: inner, DX: t.DX, DY: t.DY})
	})
}

// shift возвращает копию точек, сдвинутых на -(dx,dy).
func (t *Translating) shift(pts []image.Point) []image.Point {
	if len(pts) == 0 {
		return pts
	}
	off := image.Pt(t.DX, t.DY)
	out := make([]image.Point, len(pts))
	for i, p := range pts {
		out[i] = p.Sub(off)
	}
	return out
}
