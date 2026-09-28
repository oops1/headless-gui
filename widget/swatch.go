package widget

import (
	"image/color"
)

// swatch.go — прямоугольник заданного цвета с рамкой темы.
//
// Самый простой из виджетов и потому нужный: показать цвет рядом с его
// названием, сложить легенду диаграммы, отметить ветку в списке. Панель
// нужного цвета делала то же самое, но без рамки: белый образец на белом поле
// и чёрный на тёмном пропадали вовсе, и приложение рисовало рамку само —
// каждое по-своему.

// Swatch — образец цвета.
type Swatch struct {
	Base

	// Color — показываемый цвет.
	Color color.RGBA

	// Border — цвет рамки. Нулевая альфа — рамка из темы: у образца она не
	// украшение, а единственное, что отделяет его от фона того же цвета.
	Border color.RGBA

	// CornerRadius — скругление углов; 0 — из темы (как у полей ввода).
	CornerRadius int

	pal cpPalette
}

// NewSwatch создаёт образец заданного цвета.
func NewSwatch(c color.RGBA) *Swatch {
	s := &Swatch{Color: c}
	if t := CurrentTheme(); t != nil {
		s.pal = cpPaletteFrom(t)
	} else {
		s.pal = cpPaletteFrom(Win11LightTheme())
	}
	return s
}

// SetColor меняет цвет образца.
func (s *Swatch) SetColor(c color.RGBA) {
	if s.Color == c {
		return
	}
	s.Color = c
	s.Invalidate()
}

// Draw рисует образец.
func (s *Swatch) Draw(ctx DrawContext) {
	b := s.Bounds()
	if b.Empty() {
		return
	}
	st := currentStyle()
	if s.CornerRadius > 0 {
		st.ControlCorner = s.CornerRadius
	}
	border := s.Border
	if border.A == 0 {
		border = s.pal.border
	}
	cpDrawSwatch(ctx, b, s.Color, border, st)
	s.drawChildren(ctx)
	s.drawDisabledOverlay(ctx)
}

// ApplyTheme обновляет цвет рамки по умолчанию.
func (s *Swatch) ApplyTheme(t *Theme) { s.pal = cpPaletteFrom(t) }
