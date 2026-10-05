// physical.go — растеризация в ФИЗИЧЕСКОМ размере (HiDPI).
//
// Виджеты живут в логических пикселях, а холст — в физических (логические ×
// масштаб). DrawImage/DrawImageScaled принимают логический размер и сами
// растягивают готовую картинку на масштаб: на 125–200 % значок, растеризованный
// в логическом размере, выходит мягким. Векторный источник (SVG, значок темы)
// надо растеризовать сразу в физическом размере и отдать контексту картинку,
// размер которой совпадает с физическим размером приёмника: тогда растяжения
// нет, а движок кладёт её на холст как есть.
package widget

import (
	"image"
	"image/color"
	"math"

	"github.com/oops1/headless-gui/v3/widget/svg"
)

// ContextScale возвращает HiDPI-масштаб контекста рисования: физических
// пикселей в логическом. Контекст, не сообщающий масштаб (тестовые и
// записывающие), считается обычным — 1.
func ContextScale(ctx DrawContext) float64 {
	if ps, ok := ctx.(physicalScaler); ok {
		if k := ps.Scale(); k > 0 {
			return k
		}
	}
	return 1
}

// PhysicalRect возвращает физический размер логического прямоугольника r на
// контексте ctx. Округление — по краям, как у движка (Canvas.sRect): размер
// совпадает с тем, что получит DrawImageScaled(…, r.Min.X, r.Min.Y, r.Dx(),
// r.Dy()), поэтому картинка такого размера кладётся без растяжения.
func PhysicalRect(ctx DrawContext, r image.Rectangle) image.Point {
	k := ContextScale(ctx)
	if k == 1 {
		return image.Pt(r.Dx(), r.Dy())
	}
	edge := func(v int) int { return int(math.Round(float64(v) * k)) }
	return image.Pt(edge(r.Max.X)-edge(r.Min.X), edge(r.Max.Y)-edge(r.Min.Y))
}

// DrawSVG рисует SVG-документ в логическом прямоугольнике r, растеризуя его в
// физическом размере. Пропорции сохраняются, картинка центрируется (как у
// SVGIcon). current — значение currentColor; tint — перекрасить весь контент в
// current. Растеризация кэшируется самим документом по физическому размеру.
//
// Для документа nil и пустого r ничего не рисуется.
func DrawSVG(ctx DrawContext, doc *svg.Document, r image.Rectangle, current color.RGBA, tint bool) {
	if doc == nil || r.Empty() {
		return
	}
	p := PhysicalRect(ctx, r)
	img := doc.RasterizeCached(p.X, p.Y, current, tint)
	if img == nil {
		return
	}
	ctx.DrawImageScaled(img, r.Min.X, r.Min.Y, r.Dx(), r.Dy())
}
