// separator.go — тонкая линия между группами содержимого.
//
// Разделитель существовал только внутри меню, панели инструментов и вкладок —
// каждый раз своей приватной реализацией. Отдельно, в произвольной раскладке,
// его было не поставить: приходилось класть панель высотой в пиксель и красить
// её вручную, а при смене темы — перекрашивать самому, потому что панель про
// тему разделителя ничего не знает.
package widget

import (
	"image"
	"image/color"
)

// SeparatorOrientation — вдоль какой оси идёт линия.
type SeparatorOrientation int

const (
	// SeparatorHorizontal — горизонтальная линия (разделяет строки).
	SeparatorHorizontal SeparatorOrientation = iota
	// SeparatorVertical — вертикальная линия (разделяет колонки).
	SeparatorVertical
)

// Separator — линия, разделяющая группы содержимого.
type Separator struct {
	Base

	// Orientation — направление линии. По умолчанию горизонтальная: строк в
	// формах и списках настроек несравнимо больше, чем колонок.
	Orientation SeparatorOrientation

	// Color — цвет линии. Берётся из темы (Theme.Border) и обновляется
	// ApplyTheme, поэтому обычно трогать его не нужно.
	Color color.RGBA

	// Thickness — толщина линии в пикселях. 0 означает один пиксель:
	// разделитель тем и хорош, что не претендует на внимание.
	Thickness int

	// Margin — отступ от краёв виджета ВДОЛЬ линии.
	//
	// Разделитель в списке настроек обычно не доходит до самого края — иначе
	// он читается как граница панели, а не как деление внутри неё.
	Margin int
}

// NewSeparator создаёт горизонтальный разделитель цвета текущей темы.
func NewSeparator() *Separator {
	return &Separator{Color: win10.Border}
}

// NewVerticalSeparator создаёт вертикальный разделитель.
func NewVerticalSeparator() *Separator {
	return &Separator{Orientation: SeparatorVertical, Color: win10.Border}
}

// thickness возвращает фактическую толщину линии.
func (s *Separator) thickness() int {
	if s.Thickness > 0 {
		return s.Thickness
	}
	return 1
}

// Draw рисует линию по центру отведённой области.
//
// По центру, а не по краю: разделителю обычно отводят строку в несколько
// пикселей высотой, и линия, прижатая к верхнему краю, зрительно принадлежала
// бы предыдущей группе, а не промежутку между группами.
func (s *Separator) Draw(ctx DrawContext) {
	b := s.bounds
	if b.Empty() || s.Color.A == 0 {
		return
	}
	t := s.thickness()

	if s.Orientation == SeparatorVertical {
		x := b.Min.X + (b.Dx()-t)/2
		y0, y1 := b.Min.Y+s.Margin, b.Max.Y-s.Margin
		if y1 <= y0 {
			return
		}
		ctx.FillRect(x, y0, t, y1-y0, s.Color)
		return
	}

	y := b.Min.Y + (b.Dy()-t)/2
	x0, x1 := b.Min.X+s.Margin, b.Max.X-s.Margin
	if x1 <= x0 {
		return
	}
	ctx.FillRect(x0, y, x1-x0, t, s.Color)
}

// ApplyTheme обновляет цвет линии из темы.
func (s *Separator) ApplyTheme(t *Theme) { s.Color = t.Border }

// DesiredSize сообщает раскладке толщину линии по своей оси.
//
// Нужен контейнерам, которые считают размер по детям (StackPanel, ScrollView):
// разделителю не нужно ни задавать высоту вручную, ни угадывать её.
func (s *Separator) DesiredSize() (int, int) {
	t := s.thickness()
	if s.Orientation == SeparatorVertical {
		return t, 0
	}
	return 0, t
}

// SetBounds задаёт границы разделителя.
func (s *Separator) SetBounds(r image.Rectangle) { s.Base.SetBounds(r) }
