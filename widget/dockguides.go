// Package widget — dockguides.go: направляющие (guides) докинга и «призрак»
// перетаскиваемой панели для DockManager.
//
// Направляющие рисуются ОВЕРЛЕЕМ в холсте (через OverlayDrawer DockManager'а,
// БЕЗ OverlayBoundsProvider) — они не выносятся в нативный попап-хост, а
// остаются поверх зоны докинга, как контекстное меню локали у Window.
//
// Упрощение фазы 1 (VS Toolbox): четыре стрелки-направляющие по центрам сторон
// менеджера (Left/Right/Top/Bottom). Наведение призрака на стрелку показывает
// предпросмотр целевого региона; отпускание — Dock на эту сторону. Крест по
// центру наведённой стопки (drop-into-stack) — фаза 2; стопки в фазе 1
// набираются повторным доком на ту же сторону.
package widget

import (
	"image"
	"image/color"
)

const (
	// dockGuideSize — сторона квадратной кнопки-направляющей (px).
	dockGuideSize = 38
	// dockGuideInset — отступ центра кнопки-направляющей от края менеджера (px).
	dockGuideInset = 34
)

// dockGuideRect возвращает прямоугольник кнопки-направляющей для стороны side
// в пределах менеджера b. Для DockFill/неизвестных сторон — пустой.
func dockGuideRect(b image.Rectangle, side DockSide) image.Rectangle {
	cx := (b.Min.X + b.Max.X) / 2
	cy := (b.Min.Y + b.Max.Y) / 2
	h := dockGuideSize / 2
	var px, py int
	switch side {
	case DockLeft:
		px, py = b.Min.X+dockGuideInset, cy
	case DockRight:
		px, py = b.Max.X-dockGuideInset, cy
	case DockTop:
		px, py = cx, b.Min.Y+dockGuideInset
	case DockBottom:
		px, py = cx, b.Max.Y-dockGuideInset
	default:
		return image.Rectangle{}
	}
	return image.Rect(px-h, py-h, px+h, py+h)
}

// dockGuideHit возвращает сторону, над кнопкой-направляющей которой находится
// точка (x, y), и ok=false, если точка вне всех направляющих.
func dockGuideHit(b image.Rectangle, x, y int) (DockSide, bool) {
	pt := image.Pt(x, y)
	for _, s := range []DockSide{DockLeft, DockRight, DockTop, DockBottom} {
		if pt.In(dockGuideRect(b, s)) {
			return s, true
		}
	}
	return DockLeft, false
}

// drawDockGuides рисует четыре кнопки-направляющие и (при наведении) предпросмотр
// целевого региона. previewRect — прямоугольник региона, куда встанет панель при
// доке на hovered-сторону; вычисляется менеджером (DockManager.previewRegion) как
// ЕДИНЫЙ источник истины, совпадающий с фактической раскладкой после дропа.
// accent — акцентный цвет темы; face/border — цвета кнопки.
func drawDockGuides(ctx DrawContext, b image.Rectangle, hovered DockSide, hoverOK bool, previewRect image.Rectangle, accent, face, border color.RGBA) {
	// Предпросмотр целевого региона под наведённой направляющей.
	if hoverOK && !previewRect.Empty() {
		pr := previewRect
		hl := color.RGBA{R: accent.R, G: accent.G, B: accent.B, A: 70}
		ctx.FillRectAlpha(pr.Min.X, pr.Min.Y, pr.Dx(), pr.Dy(), hl)
		ctx.DrawBorder(pr.Min.X, pr.Min.Y, pr.Dx(), pr.Dy(), accent)
	}
	for _, s := range []DockSide{DockLeft, DockRight, DockTop, DockBottom} {
		r := dockGuideRect(b, s)
		if r.Empty() {
			continue
		}
		bg := face
		if hoverOK && s == hovered {
			bg = accent
		}
		ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 4, bg)
		ctx.DrawRoundBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 4, border)
		drawDockArrow(ctx, r, s, accent)
	}
}

// drawDockArrow рисует треугольную стрелку внутри кнопки-направляющей,
// указывающую в сторону соответствующего края.
func drawDockArrow(ctx DrawContext, r image.Rectangle, side DockSide, col color.RGBA) {
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2
	const n = 6 // половина основания треугольника
	for i := 0; i <= n; i++ {
		// span уменьшается к вершине.
		span := n - i
		switch side {
		case DockLeft: // вершина слева
			ctx.DrawVLine(cx-n+i, cy-span, span*2+1, col)
		case DockRight: // вершина справа
			ctx.DrawVLine(cx+n-i, cy-span, span*2+1, col)
		case DockTop: // вершина сверху
			ctx.DrawHLine(cx-span, cy-n+i, span*2+1, col)
		case DockBottom: // вершина снизу
			ctx.DrawHLine(cx-span, cy+n-i, span*2+1, col)
		}
	}
}
