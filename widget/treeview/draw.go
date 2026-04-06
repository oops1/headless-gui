package treeview

import (
	"image"
	"image/color"
)

// ─── Draw ──────────────────────────────────────────────────────────────────

// Draw отрисовывает TreeView.
func (tv *TreeView) Draw(ctx DrawContextBridge) {
	b := tv.bounds
	if b.Empty() {
		return
	}

	ih := tv.itemH()
	indent := tv.indentW()
	iconSz := tv.iconSz()

	// Фон
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), tv.Theme.Background)

	// Clip к bounds
	ctx.SetClip(b)
	defer ctx.ClearClip()

	flat := tv.visibleNodes()
	contentW := tv.contentWidth()

	// Вычисляем диапазон видимых строк (виртуализация)
	startIdx := tv.scrollY / ih
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := startIdx + b.Dy()/ih + 2
	if endIdx > len(flat) {
		endIdx = len(flat)
	}

	for i := startIdx; i < endIdx; i++ {
		fi := flat[i]
		y := b.Min.Y + i*ih - tv.scrollY

		// Пропуск за пределами видимости
		if y+ih < b.Min.Y || y >= b.Max.Y {
			continue
		}

		isSelected := fi.item == tv.selectedItem
		isHovered := i == tv.hoverIdx

		// Фон: выделение или hover
		if isSelected {
			ctx.FillRectAlpha(b.Min.X, y, contentW, ih, tv.Theme.SelectColor)
		} else if isHovered {
			ctx.FillRect(b.Min.X, y, contentW, ih, tv.Theme.HoverColor)
		}

		x := b.Min.X + 6 + fi.depth*indent

		// Линии иерархии (опционально)
		if tv.ShowIndentGuides && fi.depth > 0 {
			for d := 0; d < fi.depth; d++ {
				guideX := b.Min.X + 6 + d*indent + indent/2
				ctx.DrawVLine(guideX, y, ih, tv.Theme.IndentGuideColor)
			}
		}

		// Стрелка ▸/▾ (только для узлов с детьми)
		if fi.item.HasChildren() {
			arrowCX := x + 5
			arrowCY := y + ih/2
			if fi.item.Expanded {
				drawArrowDown(ctx, arrowCX, arrowCY, tv.Theme.ArrowColor)
			} else {
				drawArrowRight(ctx, arrowCX, arrowCY, tv.Theme.ArrowColor)
			}
		}

		// Контент после стрелки
		textX := x + arrowZone

		// Кастомный рендерер (через шаблон)
		if tv.itemTemplate != nil && tv.itemTemplate.CustomRenderer != nil {
			rect := image.Rect(textX, y, b.Min.X+contentW, y+ih)
			tv.itemTemplate.CustomRenderer(NodeRendererContext{
				Rect:       rect,
				Item:       fi.item.DataContext,
				IsSelected: isSelected,
				IsHovered:  isHovered,
				Expanded:   fi.item.Expanded,
				DrawCtx:    ctx,
			})
			continue
		}

		// Иконка
		if fi.item.Icon != nil {
			iconY := y + (ih-iconSz)/2
			ctx.DrawImageScaled(fi.item.Icon, textX, iconY, iconSz, iconSz)
			textX += iconSz + 4
		}

		// Текст
		textY := y + (ih-int(tv.fontSize()*1.3))/2
		text := fi.item.DisplayText()
		ctx.DrawTextSize(text, textX, textY, tv.fontSize(), tv.Theme.Foreground)
	}

	// Скроллбар
	if tv.needsScrollbar() {
		tv.drawScrollbar(ctx)
	}

	// Рамка фокуса
	if tv.focused {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), tv.Theme.FocusBorderColor)
	}
}

// drawScrollbar рисует вертикальный скроллбар.
func (tv *TreeView) drawScrollbar(ctx DrawContextBridge) {
	sr := tv.scrollbarRect()

	// Трек
	ctx.FillRect(sr.Min.X, sr.Min.Y, sr.Dx(), sr.Dy(), tv.Theme.ScrollTrackBG)

	// Ползунок
	tr := tv.thumbRect()
	col := tv.Theme.ScrollThumbBG
	if tv.thumbHovered || tv.thumbDragging {
		col = tv.Theme.ScrollThumbHover
	}
	ctx.FillRect(tr.Min.X, tr.Min.Y, tr.Dx(), tr.Dy(), col)
}

// ─── Arrow drawing ─────────────────────────────────────────────────────────

// drawArrowRight рисует треугольник ▸ (свёрнуто).
func drawArrowRight(ctx DrawContextBridge, cx, cy int, col color.RGBA) {
	for dy := -3; dy <= 3; dy++ {
		w := 4 - abs(dy)
		for dx := 0; dx < w; dx++ {
			ctx.SetPixel(cx+dx-1, cy+dy, col)
		}
	}
}

// drawArrowDown рисует треугольник ▾ (раскрыто).
func drawArrowDown(ctx DrawContextBridge, cx, cy int, col color.RGBA) {
	for dx := -3; dx <= 3; dx++ {
		h := 4 - abs(dx)
		for dy := 0; dy < h; dy++ {
			ctx.SetPixel(cx+dx, cy-2+dy, col)
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
