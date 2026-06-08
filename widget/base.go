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

	// hidden=true → виджет скрыт (WPF Visibility="Collapsed"/"Hidden").
	// Хранится инвертированно, чтобы zero value (false) означал «видим» —
	// все виджеты по умолчанию видимы, как в WPF.
	hidden bool

	// ToolTip — всплывающая подсказка (WPF ToolTip). Пустая строка = нет подсказки.
	// Единый источник для всех виджетов; движок рисует её при наведении курсора.
	ToolTip string

	// ContextMenu — контекстное меню (WPF ContextMenu), показывается движком
	// по правому клику над виджетом. nil = нет меню.
	ContextMenu *PopupMenu

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

	// hAlignSet/vAlignSet — было ли выравнивание задано явно (через XAML/код).
	// Нужно, чтобы отличить явный Stretch от значения по умолчанию: Canvas
	// растягивает потомка только при ЯВНОМ HorizontalAlignment="Stretch" (BUG-6).
	hAlignSet bool
	vAlignSet bool

	// TabIndexValue — порядок обхода Tab (WPF TabIndex). 0 по умолчанию.
	// Отрицательное значение исключает виджет из Tab-навигации.
	TabIndexValue int

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

// ClearChildren удаляет всех потомков (используется при перестроении ItemsControl).
func (b *Base) ClearChildren() { b.children = nil }

// IsEnabled возвращает true, если виджет включён (WPF IsEnabled).
// По умолчанию все виджеты включены.
func (b *Base) IsEnabled() bool { return !b.disabled }

// SetEnabled включает/выключает виджет (WPF IsEnabled).
func (b *Base) SetEnabled(v bool) { b.disabled = !v }

// ── Visibility (WPF Visibility) ─────────────────────────────────────────────

// IsVisible возвращает true, если виджет видим (по умолчанию true).
// Скрытые виджеты не рисуются и не участвуют в hit-тесте.
func (b *Base) IsVisible() bool { return !b.hidden }

// SetVisible показывает (true) или скрывает (false) виджет.
// Аналог WPF Visibility: true ↔ Visible, false ↔ Collapsed.
func (b *Base) SetVisible(v bool) { b.hidden = !v }

// ── ToolTip (WPF ToolTip) ───────────────────────────────────────────────────

// GetToolTip возвращает текст всплывающей подсказки (пустая строка = нет).
func (b *Base) GetToolTip() string { return b.ToolTip }

// SetToolTip задаёт текст всплывающей подсказки.
func (b *Base) SetToolTip(s string) { b.ToolTip = s }

// GetContextMenu возвращает контекстное меню виджета (или nil).
func (b *Base) GetContextMenu() *PopupMenu { return b.ContextMenu }

// SetContextMenu задаёт контекстное меню (показывается по правому клику).
func (b *Base) SetContextMenu(m *PopupMenu) { b.ContextMenu = m }

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

func (b *Base) GetHAlign() HorizontalAlignment  { return b.HAlign }
func (b *Base) SetHAlign(a HorizontalAlignment) { b.HAlign = a; b.hAlignSet = true }
func (b *Base) GetVAlign() VerticalAlignment    { return b.VAlign }
func (b *Base) SetVAlign(a VerticalAlignment)   { b.VAlign = a; b.vAlignSet = true }

// HAlignExplicit / VAlignExplicit сообщают, было ли выравнивание задано явно.
func (b *Base) HAlignExplicit() bool { return b.hAlignSet }
func (b *Base) VAlignExplicit() bool { return b.vAlignSet }

// TabIndex возвращает порядок Tab-навигации (WPF TabIndex).
func (b *Base) TabIndex() int { return b.TabIndexValue }

// SetTabIndex задаёт порядок Tab-навигации (отрицательный — исключить из обхода).
func (b *Base) SetTabIndex(i int) { b.TabIndexValue = i }

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
// Скрытые потомки (SetVisible(false) / Visibility=Collapsed) пропускаются.
func (b *Base) drawChildren(ctx DrawContext) {
	for _, child := range b.children {
		if child.Bounds().Empty() {
			continue
		}
		if !IsWidgetVisible(child) {
			continue
		}
		child.Draw(ctx)
	}
}

// IsWidgetVisible возвращает false, если виджет реализует IsVisible() и скрыт.
// Виджеты без этого метода считаются видимыми.
func IsWidgetVisible(w Widget) bool {
	if v, ok := w.(interface{ IsVisible() bool }); ok {
		return v.IsVisible()
	}
	return true
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
