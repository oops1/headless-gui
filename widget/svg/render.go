package svg

import (
	"image"
	"image/color"
)

// defaultCurrentColor — во что резолвится fill/stroke="currentColor", когда
// вызывающий Render просит цвета самого документа (tint.A == 0) и явно не
// передал цвет темы. Совпадает с дефолтной заливкой SVG-спеки (см.
// defaultInherited в parse.go) — так вызов Render с прозрачным tint не
// отличается от «отрисовать документ как он есть, без темизации».
var defaultCurrentColor = color.RGBA{0, 0, 0, 255}

// Render — растеризация SVG-документа как публичная операция пакета, без
// привязки к виджету SVGIcon. Инструментам темы (тулбар, дерево, значки
// статуса файлов) она нужна напрямую: иконки лежат в embed.FS, разбираются
// Parse и перекрашиваются в цвет активной темы вне какого-либо виджета.
//
// tint управляет и подстановкой currentColor, и перекраской:
//   - tint.A == 0 (полностью прозрачный цвет) — ЛОВУШКА: это НЕ «ничего не
//     рисовать», а «не перекрашивать» — сохраняются цвета, заданные в самом
//     документе атрибутами fill/stroke. Фигуры с fill="currentColor" при
//     этом не остаются неопределёнными: они резолвятся в чёрный
//     (defaultCurrentColor), как в самой SVG-спеке при отсутствии внешнего
//     цвета. Если нужно вместо этого подставить в currentColor конкретный
//     цвет темы, не перекрашивая остальные фигуры, — используйте
//     doc.Rasterize(w, h, current, false)/doc.RasterizeCached(...) напрямую
//     (Render такую комбинацию одним вызовом не выражает).
//   - tint.A != 0 — весь контент документа (заливки, обводки и currentColor)
//     перекрашивается в tint, как SVGIcon с Tint=true; альфа tint участвует
//     в смешивании вместе с fill-opacity/stroke-opacity самого документа.
//
// doc == nil, w <= 0 или h <= 0 — не паника, результат nil.
//
// Растеризация идёт через кэш документа (RasterizeCached): повторный Render
// с теми же (w, h, tint) отдаёт ту же закэшированную картинку. Возвращаемый
// *image.RGBA — из кэша документа, вызывающий не должен его изменять.
func Render(doc *Document, w, h int, tint color.RGBA) *image.RGBA {
	if doc == nil || w <= 0 || h <= 0 {
		return nil
	}
	if tint.A == 0 {
		return doc.RasterizeCached(w, h, defaultCurrentColor, false)
	}
	return doc.RasterizeCached(w, h, tint, true)
}
