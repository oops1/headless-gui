// Package widget — DockPanel: контейнер с докингом по сторонам (WPF DockPanel).
//
// Дочерние виджеты прикрепляются к сторонам через attached property DockPanel.Dock:
//   - Top: элемент занимает всю ширину сверху
//   - Bottom: элемент занимает всю ширину снизу
//   - Left: элемент занимает всю высоту слева
//   - Right: элемент занимает всю высоту справа
//   - Последний элемент (или без Dock) заполняет оставшееся пространство
//
// WPF-совместимый layout: элементы обрабатываются в порядке добавления,
// каждый "откусывает" место от оставшейся области.
package widget

import (
	"image"
	"image/color"
)

// DockPanel — контейнер с layout по WPF DockPanel.
type DockPanel struct {
	Base

	Background color.RGBA
	UseAlpha   bool
}

// NewDockPanel создаёт пустой DockPanel с прозрачным фоном.
func NewDockPanel() *DockPanel {
	return &DockPanel{
		UseAlpha: true,
	}
}

// SetBounds устанавливает bounds и пересчитывает layout.
func (dp *DockPanel) SetBounds(r image.Rectangle) {
	dp.Base.SetBounds(r)
	dp.layout()
}

// AddChild добавляет дочерний виджет и пересчитывает layout.
func (dp *DockPanel) AddChild(w Widget) {
	dp.Base.AddChild(w)
	dp.layout()
}

// layout расставляет детей по DockPanel.Dock.
// Последний ребёнок заполняет оставшееся пространство (WPF LastChildFill=true).
func (dp *DockPanel) layout() {
	b := dp.bounds
	if b.Empty() {
		return
	}

	// Оставшаяся область (уменьшается по мере размещения детей).
	remaining := b

	children := dp.children
	for i, child := range children {
		isLast := i == len(children)-1

		// Получаем Dock через интерфейс
		dock := DockTop // default
		type dockGetter interface {
			GetDock() DockSide
		}
		if dg, ok := child.(dockGetter); ok {
			dock = dg.GetDock()
		}

		// Margin — внешний отступ ребёнка.
		var m Margin
		type marginGetter interface {
			GetMargin() Margin
		}
		if mg, ok := child.(marginGetter); ok {
			m = mg.GetMargin()
		}

		// Последний элемент заполняет оставшееся пространство
		if isLast {
			child.SetBounds(image.Rect(
				remaining.Min.X+m.Left, remaining.Min.Y+m.Top,
				remaining.Max.X-m.Right, remaining.Max.Y-m.Bottom,
			))
			break
		}

		cb := child.Bounds()
		switch dock {
		case DockTop:
			h := cb.Dy()
			if h <= 0 {
				h = 30 // default
			}
			child.SetBounds(image.Rect(
				remaining.Min.X+m.Left, remaining.Min.Y+m.Top,
				remaining.Max.X-m.Right, remaining.Min.Y+m.Top+h,
			))
			remaining.Min.Y += h + m.Top + m.Bottom

		case DockBottom:
			h := cb.Dy()
			if h <= 0 {
				h = 30
			}
			child.SetBounds(image.Rect(
				remaining.Min.X+m.Left, remaining.Max.Y-m.Bottom-h,
				remaining.Max.X-m.Right, remaining.Max.Y-m.Bottom,
			))
			remaining.Max.Y -= h + m.Top + m.Bottom

		case DockLeft:
			w := cb.Dx()
			if w <= 0 {
				w = 200
			}
			child.SetBounds(image.Rect(
				remaining.Min.X+m.Left, remaining.Min.Y+m.Top,
				remaining.Min.X+m.Left+w, remaining.Max.Y-m.Bottom,
			))
			remaining.Min.X += w + m.Left + m.Right

		case DockRight:
			w := cb.Dx()
			if w <= 0 {
				w = 200
			}
			child.SetBounds(image.Rect(
				remaining.Max.X-m.Right-w, remaining.Min.Y+m.Top,
				remaining.Max.X-m.Right, remaining.Max.Y-m.Bottom,
			))
			remaining.Max.X -= w + m.Left + m.Right

		default:
			// Fill — заполняет оставшееся
			child.SetBounds(image.Rect(
				remaining.Min.X+m.Left, remaining.Min.Y+m.Top,
				remaining.Max.X-m.Right, remaining.Max.Y-m.Bottom,
			))
		}
	}
}

// Draw рисует фон и дочерние виджеты.
func (dp *DockPanel) Draw(ctx DrawContext) {
	b := dp.bounds
	if dp.Background.A > 0 {
		if dp.UseAlpha && dp.Background.A < 255 {
			if ac, ok := ctx.(DrawContextAlpha); ok {
				ac.FillRectAlpha(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), dp.Background)
			}
		} else {
			ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), dp.Background)
		}
	}
	dp.drawChildren(ctx)
}

// ApplyTheme обновляет цвета DockPanel из темы.
func (dp *DockPanel) ApplyTheme(t *Theme) {
	// DockPanel обычно прозрачный — ничего не обновляем
}
