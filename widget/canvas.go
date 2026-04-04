// Package widget — Canvas: контейнер с абсолютным позиционированием (аналог WPF Canvas).
//
// Canvas — это панель, которая размещает дочерние виджеты по абсолютным координатам,
// заданным через attached-свойства Canvas.Left, Canvas.Top, Canvas.Right, Canvas.Bottom.
//
// В отличие от Grid, DockPanel, StackPanel — Canvas не управляет раскладкой автоматически.
// Каждый дочерний элемент должен явно указать свою позицию и размер.
//
// По спецификации WPF:
//   - Canvas.Left / Canvas.Top задают смещение от левого/верхнего края Canvas
//   - Canvas.Right / Canvas.Bottom задают смещение от правого/нижнего края Canvas
//   - Width / Height задают размер дочернего элемента
//   - Если указаны Left и Right одновременно, Left имеет приоритет (ширина = Canvas.Width - Left - Right)
//   - Если указаны Top и Bottom одновременно, Top имеет приоритет (высота = Canvas.Height - Top - Bottom)
//   - Дочерние элементы по умолчанию НЕ обрезаются границами Canvas (ClipToBounds=false)
//   - Canvas сам по себе не имеет размера по содержимому — его bounds задаётся родителем
//   - ZIndex поддерживается порядком добавления: последний добавленный — поверх остальных
//
// XAML:
//
//	<Canvas Width="400" Height="300" Background="#1E1E1E">
//	    <Button Canvas.Left="10" Canvas.Top="20" Width="100" Height="30" Content="OK"/>
//	    <Label Canvas.Right="10" Canvas.Bottom="5" Width="80" Height="20" Text="Status"/>
//	</Canvas>
package widget

import (
	"image"
	"image/color"
)

// CanvasAttached — attached-свойства позиционирования внутри Canvas.
// Значение -1 означает «не задано» (NaN в WPF).
type CanvasAttached struct {
	Left   int // Canvas.Left   — смещение от левого края Canvas (-1 = не задано)
	Top    int // Canvas.Top    — смещение от верхнего края Canvas (-1 = не задано)
	Right  int // Canvas.Right  — смещение от правого края Canvas (-1 = не задано)
	Bottom int // Canvas.Bottom — смещение от нижнего края Canvas (-1 = не задано)
}

// canvasChildInfo хранит информацию о дочернем виджете Canvas:
// attached properties + желаемый размер (Width/Height из XAML).
// Желаемый размер сохраняется при добавлении и не теряется при layout пересчёте.
type canvasChildInfo struct {
	Props        CanvasAttached
	DesiredW     int // желаемая ширина (из Width XAML атрибута или Bounds при добавлении)
	DesiredH     int // желаемая высота (из Height XAML атрибута или Bounds при добавлении)
}

// Canvas — контейнер с абсолютным позиционированием дочерних элементов.
//
// Реализует паттерн WPF Canvas: дочерние виджеты размещаются по координатам,
// заданным через Canvas.Left / Canvas.Top / Canvas.Right / Canvas.Bottom.
type Canvas struct {
	Base
	Background   color.RGBA // фон Canvas (прозрачный по умолчанию)
	UseAlpha     bool       // true = альфа-смешивание для полупрозрачного фона
	ClipToBounds bool       // true = обрезать дочерние элементы границами Canvas (WPF default: false)

	// childInfos хранит Canvas attached properties и желаемый размер для каждого дочернего виджета.
	childInfos []canvasChildInfo
}

// NewCanvas создаёт пустой Canvas с прозрачным фоном.
func NewCanvas() *Canvas {
	return &Canvas{
		Background: color.RGBA{},
		UseAlpha:   true,
	}
}

// AddChild добавляет дочерний виджет с пустыми attached-свойствами.
// Размер берётся из текущих Bounds виджета.
func (c *Canvas) AddChild(w Widget) {
	wb := w.Bounds()
	c.Base.AddChild(w)
	c.childInfos = append(c.childInfos, canvasChildInfo{
		Props:    CanvasAttached{Left: -1, Top: -1, Right: -1, Bottom: -1},
		DesiredW: wb.Dx(),
		DesiredH: wb.Dy(),
	})
}

// AddChildAt добавляет виджет с заданными Canvas attached properties.
// Желаемый размер (desiredW, desiredH) сохраняется и используется при каждом layout.
func (c *Canvas) AddChildAt(w Widget, props CanvasAttached, desiredW, desiredH int) {
	c.Base.AddChild(w)
	c.childInfos = append(c.childInfos, canvasChildInfo{
		Props:    props,
		DesiredW: desiredW,
		DesiredH: desiredH,
	})
	c.layoutChild(len(c.children) - 1)
}

// SetChildCanvasProps задаёт Canvas attached properties для дочернего виджета по индексу.
func (c *Canvas) SetChildCanvasProps(idx int, props CanvasAttached) {
	if idx < 0 || idx >= len(c.childInfos) {
		return
	}
	c.childInfos[idx].Props = props
	c.layoutChild(idx)
}

// GetChildCanvasProps возвращает Canvas attached properties для дочернего виджета.
func (c *Canvas) GetChildCanvasProps(idx int) CanvasAttached {
	if idx < 0 || idx >= len(c.childInfos) {
		return CanvasAttached{Left: -1, Top: -1, Right: -1, Bottom: -1}
	}
	return c.childInfos[idx].Props
}

// SetBounds задаёт bounds Canvas и пересчитывает позиции всех дочерних виджетов.
func (c *Canvas) SetBounds(r image.Rectangle) {
	c.Base.SetBounds(r)
	c.layout()
}

// layout пересчитывает позиции всех дочерних виджетов по их Canvas attached properties.
func (c *Canvas) layout() {
	for i := range c.children {
		c.layoutChild(i)
	}
}

// layoutChild рассчитывает bounds одного дочернего виджета по Canvas attached properties.
//
// Алгоритм (по спецификации WPF):
//  1. Если задан Left — x = Canvas.Min.X + Left
//  2. Иначе если задан Right — x = Canvas.Max.X - Right - childWidth
//  3. Иначе — x = Canvas.Min.X (элемент в левом верхнем углу)
//  4. Аналогично для Y (Top / Bottom)
//  5. Если задан Left и Right одновременно — Left приоритет, ширина = canvasWidth - Left - Right
//  6. Аналогично для Top и Bottom
func (c *Canvas) layoutChild(idx int) {
	if idx < 0 || idx >= len(c.children) || idx >= len(c.childInfos) {
		return
	}

	cb := c.Bounds()
	if cb.Empty() {
		return
	}

	child := c.children[idx]
	info := c.childInfos[idx]
	props := info.Props

	// Желаемый размер ребёнка (сохранён при добавлении)
	cw := info.DesiredW
	ch := info.DesiredH

	// Если размер не задан — используем дефолтный
	if cw <= 0 {
		cw = 80
	}
	if ch <= 0 {
		ch = 30
	}

	canvasW := cb.Dx()
	canvasH := cb.Dy()

	// ── Горизонтальная позиция ──────────────────────────────────────────────

	var x int
	hasLeft := props.Left >= 0
	hasRight := props.Right >= 0

	switch {
	case hasLeft && hasRight:
		// Оба заданы: Left имеет приоритет, ширина вычисляется
		x = cb.Min.X + props.Left
		cw = canvasW - props.Left - props.Right
		if cw < 0 {
			cw = 0
		}
	case hasLeft:
		x = cb.Min.X + props.Left
	case hasRight:
		x = cb.Min.X + canvasW - props.Right - cw
	default:
		x = cb.Min.X // по умолчанию — левый край
	}

	// ── Вертикальная позиция ────────────────────────────────────────────────

	var y int
	hasTop := props.Top >= 0
	hasBottom := props.Bottom >= 0

	switch {
	case hasTop && hasBottom:
		// Оба заданы: Top имеет приоритет, высота вычисляется
		y = cb.Min.Y + props.Top
		ch = canvasH - props.Top - props.Bottom
		if ch < 0 {
			ch = 0
		}
	case hasTop:
		y = cb.Min.Y + props.Top
	case hasBottom:
		y = cb.Min.Y + canvasH - props.Bottom - ch
	default:
		y = cb.Min.Y // по умолчанию — верхний край
	}

	// Margin — дополнительные отступы (WPF Margin работает и внутри Canvas)
	type marginGetter interface {
		GetMargin() Margin
	}
	if mg, ok := child.(marginGetter); ok {
		m := mg.GetMargin()
		x += m.Left
		y += m.Top
		cw -= m.Left + m.Right
		ch -= m.Top + m.Bottom
		if cw < 0 {
			cw = 0
		}
		if ch < 0 {
			ch = 0
		}
	}

	oldBounds := child.Bounds()
	newBounds := image.Rect(x, y, x+cw, y+ch)
	child.SetBounds(newBounds)

	// Если позиция виджета изменилась — рекурсивно сдвигаем всех потомков.
	// Это необходимо для контейнеров (Panel и т.д.), чьи дочерние виджеты
	// используют абсолютные координаты и не пересчитываются автоматически
	// при вызове SetBounds на родителе.
	dx := newBounds.Min.X - oldBounds.Min.X
	dy := newBounds.Min.Y - oldBounds.Min.Y
	if (dx != 0 || dy != 0) && !oldBounds.Empty() {
		shiftDescendants(child, dx, dy)
	}
}

// shiftDescendants рекурсивно сдвигает bounds всех потомков виджета на (dx, dy).
// Для виджетов с собственным layout (Canvas, Grid, DockPanel, TabControl, StackPanel)
// вызов SetBounds уже перестраивает дочерние позиции, поэтому рекурсия не нужна.
func shiftDescendants(w Widget, dx, dy int) {
	delta := image.Pt(dx, dy)
	for _, child := range w.Children() {
		child.SetBounds(child.Bounds().Add(delta))
		if !HasOwnLayout(child) {
			shiftDescendants(child, dx, dy)
		}
	}
}

// HasOwnLayout возвращает true для контейнеров, которые сами пересчитывают
// позиции дочерних виджетов при вызове SetBounds (через layout / layoutContent).
func HasOwnLayout(w Widget) bool {
	switch w.(type) {
	case *Canvas, *Grid, *DockPanel, *TabControl, *StackPanel, *Window:
		return true
	}
	return false
}

// Draw рисует фон Canvas и все дочерние виджеты.
func (c *Canvas) Draw(ctx DrawContext) {
	b := c.Bounds()

	// Фон
	if c.Background.A > 0 {
		if c.UseAlpha && c.Background.A < 255 {
			if ac, ok := ctx.(DrawContextAlpha); ok {
				ac.FillRectAlpha(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), c.Background)
			} else {
				ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), c.Background)
			}
		} else {
			ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), c.Background)
		}
	}

	// Clipping (WPF Canvas по умолчанию ClipToBounds=false)
	if c.ClipToBounds {
		ctx.SetClip(b)
		defer ctx.ClearClip()
	}

	c.drawChildren(ctx)
}

// ApplyTheme — Canvas не имеет темизируемых элементов (только фон).
func (c *Canvas) ApplyTheme(t *Theme) {}
