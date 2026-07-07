package widget

import (
	"image"
	"image/color"
)

// DialogSeverity — тип значка стандартного диалога (влияет на иконку и
// её семантический цвет; не зависит от темы — как в системных MessageBox).
type DialogSeverity int

const (
	SeverityNone DialogSeverity = iota
	SeverityInfo
	SeverityQuestion
	SeverityWarning
	SeverityError
)

// severityColor — семантический цвет значка (постоянный, узнаваемый).
func severityColor(s DialogSeverity) color.RGBA {
	switch s {
	case SeverityInfo:
		return color.RGBA{R: 0, G: 120, B: 212, A: 255} // синий
	case SeverityQuestion:
		return color.RGBA{R: 60, G: 170, B: 100, A: 255} // зелёный
	case SeverityWarning:
		return color.RGBA{R: 235, G: 160, B: 40, A: 255} // оранжевый
	case SeverityError:
		return color.RGBA{R: 220, G: 80, B: 80, A: 255} // красный
	}
	return color.RGBA{}
}

// severityGlyph — символ внутри значка.
func severityGlyph(s DialogSeverity) string {
	switch s {
	case SeverityInfo:
		return "i"
	case SeverityQuestion:
		return "?"
	case SeverityWarning:
		return "!"
	case SeverityError:
		return "✕" // ✕
	}
	return ""
}

// DialogIcon — значок severity (круг для info/question/error, треугольник
// для warning) со сглаживанием, если DrawContext его поддерживает.
type DialogIcon struct {
	Base
	Severity DialogSeverity
}

// NewDialogIcon создаёт значок заданного типа.
func NewDialogIcon(s DialogSeverity) *DialogIcon {
	return &DialogIcon{Severity: s}
}

func (ic *DialogIcon) Draw(ctx DrawContext) {
	if ic.Severity == SeverityNone {
		return
	}
	b := ic.bounds
	cx := b.Min.X + b.Dx()/2
	cy := b.Min.Y + b.Dy()/2
	r := b.Dx() / 2
	if b.Dy()/2 < r {
		r = b.Dy() / 2
	}
	col := severityColor(ic.Severity)
	glyphCol := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	aa, hasAA := ctx.(AAShapes)
	if ic.Severity == SeverityWarning {
		// Треугольник вершиной вверх.
		pts := []image.Point{
			{X: cx, Y: cy - r},
			{X: cx + r, Y: cy + r - 1},
			{X: cx - r, Y: cy + r - 1},
		}
		if hasAA {
			aa.FillPolygonAA(pts, col)
		} else {
			fillPolygon(ctx, pts, col)
		}
		glyphCol = color.RGBA{R: 40, G: 30, B: 0, A: 255} // тёмный «!» на оранжевом
	} else {
		if hasAA {
			aa.FillEllipseAA(cx, cy, r, r, col)
		} else {
			drawFilledCircle(ctx, cx, cy, r, col)
		}
	}

	// Символ по центру (жирным встроенным шрифтом).
	g := severityGlyph(ic.Severity)
	sz := float64(r)
	gw := ctx.MeasureTextFont(g, sz, BuiltinFontBold)
	gy := cy - int(sz*0.62)
	if ic.Severity == SeverityWarning {
		gy = cy - int(sz*0.5)
	}
	ctx.DrawTextFont(g, cx-gw/2, gy, sz, BuiltinFontBold, glyphCol)
}

func (ic *DialogIcon) ApplyTheme(*Theme) {} // цвета значка семантические

// trBtn создаёт кнопку с подписью из локализованного ключа (напр. "dlg.ok").
// accent=true — акцентная (primary) кнопка. Живое переобновление подписи при
// смене языка обеспечивает вызывающий диалог (см. Dialog.bindLocale).
func trBtn(key string, accent bool) *Button {
	if accent {
		return NewWin10AccentButton(Tr(key))
	}
	return NewButton(Tr(key))
}
