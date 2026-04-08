package widget

import (
	"image"
	"image/color"
)

// HorizontalAlignment — WPF HorizontalAlignment (позиционирование внутри родителя).
type HorizontalAlignment int

const (
	HAlignStretch HorizontalAlignment = iota // растянуть (default)
	HAlignLeft                               // прижать к левому краю
	HAlignCenter                             // по центру
	HAlignRight                              // прижать к правому краю
)

// VerticalAlignment — WPF VerticalAlignment (позиционирование внутри родителя).
type VerticalAlignment int

const (
	VAlignStretch VerticalAlignment = iota // растянуть (default)
	VAlignTop                              // прижать к верхнему краю
	VAlignCenter                           // по центру
	VAlignBottom                           // прижать к нижнему краю
)

// DockSide определяет сторону прикрепления в DockPanel (WPF DockPanel.Dock).
type DockSide int

const (
	DockLeft   DockSide = iota // 0 — по умолчанию (WPF standard: DockPanel.Dock default = Left)
	DockTop
	DockBottom
	DockRight
	DockFill // последний элемент заполняет оставшееся пространство
)

// Margin — отступы виджета (WPF Thickness: Left, Top, Right, Bottom).
type Margin struct {
	Left, Top, Right, Bottom int
}

// Base — общие поля и тривиальные реализации интерфейса Widget.
// Встраивается во все конкретные виджеты.
type Base struct {
	bounds   image.Rectangle
	children []Widget

	// disabled=true → виджет отключён (WPF IsEnabled="False").
	// По умолчанию false (т.е. виджет включён), что соответствует WPF IsEnabled=True.
	disabled bool

	// Grid layout (attached properties, как в WPF).
	GridRow     int // Grid.Row     (0-based)
	GridColumn  int // Grid.Column  (0-based)
	GridRowSpan int // Grid.RowSpan (по умолчанию 1)
	GridColSpan int // Grid.ColumnSpan (по умолчанию 1)

	// DockPanel attached property.
	Dock DockSide

	// Margin — внешние отступы (WPF Margin).
	WidgetMargin Margin

	// HorizontalAlignment — WPF HorizontalAlignment (Left, Center, Right, Stretch).
	HAlign HorizontalAlignment
	// VerticalAlignment — WPF VerticalAlignment (Top, Center, Bottom, Stretch).
	VAlign VerticalAlignment

	// XAMLWidth / XAMLHeight — явно заданные Width/Height из XAML.
	// Используются applyAlignmentRect когда bounds ещё не установлены контейнером.
	XAMLWidth  int
	XAMLHeight int
}

func (b *Base) Bounds() image.Rectangle     { return b.bounds }
func (b *Base) SetBounds(r image.Rectangle) { b.bounds = r }
func (b *Base) Children() []Widget          { return b.children }
func (b *Base) AddChild(w Widget)           { b.children = append(b.children, w) }

// RemoveChild удаляет дочерний виджет из контейнера (по указателю).
// Возвращает true, если виджет был найден и удалён.
// Используется, например, при закрытии Panel-«окна» внутри Canvas.
func (b *Base) RemoveChild(w Widget) bool {
	for i, child := range b.children {
		if child == w {
			b.children = append(b.children[:i], b.children[i+1:]...)
			return true
		}
	}
	return false
}

// IsEnabled возвращает true, если виджет включён (WPF IsEnabled).
// По умолчанию все виджеты включены.
func (b *Base) IsEnabled() bool { return !b.disabled }

// SetEnabled включает/выключает виджет (WPF IsEnabled).
func (b *Base) SetEnabled(v bool) { b.disabled = !v }

// ── Grid attached properties ────────────────────────────────────────────────

func (b *Base) SetGridProps(row, col, rowSpan, colSpan int) {
	b.GridRow = row
	b.GridColumn = col
	b.GridRowSpan = rowSpan
	b.GridColSpan = colSpan
}

func (b *Base) GetGridRow() int     { return b.GridRow }
func (b *Base) GetGridColumn() int  { return b.GridColumn }
func (b *Base) GetGridRowSpan() int {
	if b.GridRowSpan < 1 {
		return 1
	}
	return b.GridRowSpan
}
func (b *Base) GetGridColSpan() int {
	if b.GridColSpan < 1 {
		return 1
	}
	return b.GridColSpan
}

// ── DockPanel attached property ─────────────────────────────────────────────

func (b *Base) GetDock() DockSide    { return b.Dock }
func (b *Base) SetDock(d DockSide)   { b.Dock = d }

// ── Margin ──────────────────────────────────────────────────────────────────

func (b *Base) GetMargin() Margin      { return b.WidgetMargin }
func (b *Base) SetMargin(m Margin)     { b.WidgetMargin = m }

// ── Alignment ───────────────────────────────────────────────────────────────

func (b *Base) GetHAlign() HorizontalAlignment { return b.HAlign }
func (b *Base) SetHAlign(a HorizontalAlignment) { b.HAlign = a }
func (b *Base) GetVAlign() VerticalAlignment   { return b.VAlign }
func (b *Base) SetVAlign(a VerticalAlignment)  { b.VAlign = a }

// GetXAMLSize возвращает явно заданные Width/Height из XAML.
func (b *Base) GetXAMLSize() (int, int) { return b.XAMLWidth, b.XAMLHeight }

// SetXAMLSize сохраняет явные Width/Height из XAML.
func (b *Base) SetXAMLSize(w, h int) { b.XAMLWidth = w; b.XAMLHeight = h }

// applyAlignmentRect корректирует прямоугольник r на основе
// HorizontalAlignment / VerticalAlignment виджета и его текущего размера.
// Если alignment = Stretch — возвращает r без изменений.
// Если alignment = Left/Center/Right — использует текущий Dx() виджета как ширину.
func applyAlignmentRect(w Widget, r image.Rectangle) image.Rectangle {
	type alignGetter interface {
		GetHAlign() HorizontalAlignment
		GetVAlign() VerticalAlignment
	}
	ag, ok := w.(alignGetter)
	if !ok {
		return r
	}

	ha := ag.GetHAlign()
	va := ag.GetVAlign()

	// Текущий размер виджета: сначала пробуем XAMLWidth/XAMLHeight,
	// затем текущие bounds, затем desiredWidth/desiredHeight.
	type xamlSizeGetter interface {
		GetXAMLSize() (int, int)
	}
	wb := w.Bounds()
	ww := wb.Dx()
	wh := wb.Dy()
	if xsg, ok2 := w.(xamlSizeGetter); ok2 {
		xw, xh := xsg.GetXAMLSize()
		if xw > 0 {
			ww = xw
		}
		if xh > 0 {
			wh = xh
		}
	}

	// Горизонтальное выравнивание
	switch ha {
	case HAlignLeft:
		if ww > 0 && ww < r.Dx() {
			r.Max.X = r.Min.X + ww
		}
	case HAlignCenter:
		if ww > 0 && ww < r.Dx() {
			cx := r.Min.X + (r.Dx()-ww)/2
			r.Min.X = cx
			r.Max.X = cx + ww
		}
	case HAlignRight:
		if ww > 0 && ww < r.Dx() {
			r.Min.X = r.Max.X - ww
		}
	}

	// Вертикальное выравнивание
	switch va {
	case VAlignTop:
		if wh > 0 && wh < r.Dy() {
			r.Max.Y = r.Min.Y + wh
		}
	case VAlignCenter:
		if wh > 0 && wh < r.Dy() {
			cy := r.Min.Y + (r.Dy()-wh)/2
			r.Min.Y = cy
			r.Max.Y = cy + wh
		}
	case VAlignBottom:
		if wh > 0 && wh < r.Dy() {
			r.Min.Y = r.Max.Y - wh
		}
	}

	return r
}

// ── Desired size (Auto-измерение) ───────────────────────────────────────────

// desiredHeight возвращает желаемую высоту виджета для Auto-измерения в Grid/DockPanel.
// Для Label — высота текста + padding. Для контейнеров — максимум из детей.
// Если не можем определить — возвращаем дефолт 26px.
func desiredHeight(w Widget) int {
	// Если явно задан Height в XAML — используем его.
	type xamlSizeGetter interface {
		GetXAMLSize() (int, int)
	}
	if xsg, ok := w.(xamlSizeGetter); ok {
		_, xh := xsg.GetXAMLSize()
		if xh > 0 {
			return xh
		}
	}

	switch v := w.(type) {
	case *Label:
		fs := v.FontSize
		if fs <= 0 {
			fs = DefaultFontSizePt
		}
		return int(fs*1.5+0.5) + v.PaddingY*2
	case *Button:
		return 32
	case *TextInput:
		return 26
	case *MenuBar:
		return 28
	case *StackPanel:
		// StackPanel: максимальная высота ребёнка + margin + padding (для горизонтального)
		pad := v.Padding
		type marginGetter interface {
			GetMargin() Margin
		}
		children := w.Children()
		if len(children) > 0 {
			maxH := 0
			for _, ch := range children {
				h := ch.Bounds().Dy()
				if h <= 0 {
					h = desiredHeight(ch)
				}
				if mg, ok := ch.(marginGetter); ok {
					m := mg.GetMargin()
					h += m.Top + m.Bottom
				}
				if h > maxH {
					maxH = h
				}
			}
			if maxH > 0 {
				return maxH + pad*2
			}
		}
		return 30 + pad*2
	default:
		// Для контейнеров — максимальная высота среди детей + их margin.
		children := w.Children()
		if len(children) > 0 {
			type marginGetter interface {
				GetMargin() Margin
			}
			maxH := 0
			for _, ch := range children {
				h := ch.Bounds().Dy()
				if h <= 0 {
					h = desiredHeight(ch)
				}
				// Учитываем margin дочернего элемента.
				if mg, ok := ch.(marginGetter); ok {
					m := mg.GetMargin()
					h += m.Top + m.Bottom
				}
				if h > maxH {
					maxH = h
				}
			}
			if maxH > 0 {
				return maxH
			}
		}
		return 26
	}
}

// desiredWidth возвращает желаемую ширину виджета для Auto-измерения.
// Для Label/TextBlock — ширина текста (приблизительно), для остальных — дефолт.
func desiredWidth(w Widget) int {
	// Если явно задан Width в XAML — используем его.
	type xamlSizeGetter interface {
		GetXAMLSize() (int, int)
	}
	if xsg, ok := w.(xamlSizeGetter); ok {
		xw, _ := xsg.GetXAMLSize()
		if xw > 0 {
			return xw
		}
	}

	switch v := w.(type) {
	case *Label:
		// Примерная ширина: длина текста * средняя ширина символа + padding
		text := v.Text()
		charW := 7 // средняя ширина символа при дефолтном шрифте
		return len(text)*charW + v.PaddingX*2
	case *Button:
		return 80
	case *TextInput:
		return 120
	default:
		return 80
	}
}

// drawChildren рендерит всех потомков в тот же контекст.
// Вызывается конкретными виджетами в конце своего Draw.
func (b *Base) drawChildren(ctx DrawContext) {
	for _, child := range b.children {
		if child.Bounds().Empty() {
			continue
		}
		child.Draw(ctx)
	}
}

// DrawChildren рендерит всех потомков в тот же контекст.
// Экспортированная версия для использования во внешних виджетах.
func (b *Base) DrawChildren(ctx DrawContext) {
	b.drawChildren(ctx)
}

// drawDisabledOverlay рисует полупрозрачный серый оверлей поверх виджета,
// визуально показывая что он отключён (аналог WPF IsEnabled=False).
func (b *Base) drawDisabledOverlay(ctx DrawContext) {
	if b.disabled {
		r := b.bounds
		ctx.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(),
			color.RGBA{R: 30, G: 30, B: 30, A: 140})
	}
}

// DrawDisabledOverlay рисует полупрозрачный серый оверлей если виджет отключён.
// Экспортированная версия для использования во внешних виджетах.
func (b *Base) DrawDisabledOverlay(ctx DrawContext) {
	b.drawDisabledOverlay(ctx)
}
