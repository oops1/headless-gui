package widget

import "image"

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
	DockTop    DockSide = iota // по умолчанию
	DockBottom
	DockLeft
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
}

func (b *Base) Bounds() image.Rectangle     { return b.bounds }
func (b *Base) SetBounds(r image.Rectangle) { b.bounds = r }
func (b *Base) Children() []Widget          { return b.children }
func (b *Base) AddChild(w Widget)           { b.children = append(b.children, w) }

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

	// Текущий размер виджета (из XAML Width/Height)
	wb := w.Bounds()
	ww := wb.Dx()
	wh := wb.Dy()

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
	default:
		// Для контейнеров — максимальная высота среди детей
		children := w.Children()
		if len(children) > 0 {
			maxH := 0
			for _, ch := range children {
				h := ch.Bounds().Dy()
				if h <= 0 {
					h = desiredHeight(ch)
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
		child.Draw(ctx)
	}
}
