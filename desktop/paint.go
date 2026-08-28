package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Общая отрисовка по стилю темы.
//
// Компоненты рабочего стола не знают цветов и размеров: они спрашивают у
// темы стиль для своего состояния и передают его сюда. Из-за этого «как
// выглядит наведённая кнопка» решается в одном месте, а не в каждом
// компоненте, и профиль темы, добавивший скругление или фаску, меняет вид
// сразу у всех.

// PaintStyle рисует подложку по стилю: размытую, если тема просит стекло,
// затем заливку, затем рамку. Скругление, толщина рамки и радиус размытия —
// из стиля.
func PaintStyle(ctx widget.DrawContext, r image.Rectangle, s *theme.Style) {
	if s == nil || r.Empty() {
		return
	}
	corner := int(s.Corner)

	// Тень — до всего остального: она лежит под слоем.
	if s.Elevation > 0 && s.Shadow.A > 0 {
		if sd, ok := ctx.(widget.ShadowDrawer); ok {
			sd.DrawSoftShadow(r, corner, s.Elevation, s.Shadow)
		}
	}

	// Стекло: размытая подложка, если тема просит и контекст умеет.
	if s.Backdrop.Mode == theme.BackdropBlur {
		if bd, ok := ctx.(widget.BackdropDrawer); ok {
			bd.BlurBehind(r, int(s.Backdrop.Radius), s.Backdrop.Tint)
		} else if s.Backdrop.Tint.A > 0 {
			fillAlpha(ctx, r, s.Backdrop.Tint)
		}
	} else if s.Backdrop.Mode == theme.BackdropAlpha && s.Backdrop.Tint.A > 0 {
		fillAlpha(ctx, r, s.Backdrop.Tint)
	}

	if s.Fill.A > 0 {
		switch {
		case corner > 0:
			ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), corner, s.Fill)
		case s.Fill.A < 255:
			// Полупрозрачная заливка — это плёнка поверх фона (подсветка
			// кнопки под курсором), и класть её надо смешиванием: обычный
			// FillRect записал бы цвет вместе с чужой альфой прямо в буфер.
			ctx.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), s.Fill)
		default:
			ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), s.Fill)
		}
	}

	// Объёмная рамка Windows 2000 вместо плоской, если профиль её объявил.
	if s.Bevel != nil {
		widget.DrawBevel(ctx, r, s.Bevel.Light, s.Bevel.Shadow, s.Bevel.Dark, s.Bevel.Sunken)
		return
	}

	if s.Border.A > 0 && s.BorderWidth > 0 {
		if corner > 0 {
			ctx.DrawRoundBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), corner, s.Border)
		} else {
			ctx.DrawBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), s.Border)
		}
	}
}

// DrawUnderline рисует метку под кнопкой окна: так Windows 10 и 11 отмечают
// открытое окно вместо вдавленной кнопки классических тем.
//
// Длина метки — доля ширины кнопки (ratio), и она же различает темы: в
// Windows 10 полоса идёт почти во всю ширину, в Windows 11 это короткая
// чёрточка под значком. Если тема просит короткую метку, то у активного окна
// она вдвое длиннее, чем у просто открытого — именно этим Windows 11 их и
// различает. Тема с полной полосой рисует её только активному.
func DrawUnderline(ctx widget.DrawContext, r image.Rectangle, thickness int, ratio float64, active bool, s *theme.Style) {
	if thickness <= 0 || s == nil || r.Empty() {
		return
	}
	if ratio <= 0 {
		ratio = 1
	}
	if ratio >= 1 && !active {
		return // полная полоса — примета активного окна, остальным её не рисуют
	}
	col := s.Border
	if col.A == 0 {
		col = s.Text
	}
	if col.A == 0 {
		return
	}

	k := ratio
	if !active {
		k /= 2
	}
	w := int(float64(r.Dx()) * k)
	if w < thickness {
		w = thickness
	}
	if w > r.Dx() {
		w = r.Dx()
	}
	x := r.Min.X + (r.Dx()-w)/2
	ctx.FillRect(x, r.Max.Y-thickness, w, thickness, col)
}

// DrawTextCentered рисует строку по центру области цветом и шрифтом стиля.
// Возвращает ширину строки — вызывающему она нужна для раскладки.
func DrawTextCentered(ctx widget.DrawContext, r image.Rectangle, text string, s *theme.Style) int {
	if text == "" || s == nil || r.Empty() {
		return 0
	}
	size := s.Font.Size
	if size <= 0 {
		size = widget.DefaultFontSizePt
	}
	w := measure(ctx, text, size, s.Font)
	x := r.Min.X + (r.Dx()-w)/2
	y := r.Min.Y + (r.Dy()-int(size*1.4))/2
	drawText(ctx, text, x, y, size, s)
	return w
}

// DrawTextLeft рисует строку у левого края с отступом стиля.
func DrawTextLeft(ctx widget.DrawContext, r image.Rectangle, text string, s *theme.Style) int {
	if text == "" || s == nil || r.Empty() {
		return 0
	}
	size := s.Font.Size
	if size <= 0 {
		size = widget.DefaultFontSizePt
	}
	x := r.Min.X + int(s.PadX)
	y := r.Min.Y + (r.Dy()-int(size*1.4))/2
	drawText(ctx, text, x, y, size, s)
	return measure(ctx, text, size, s.Font)
}

// DrawTextLeftElided рисует строку у левого края, усекая её многоточием,
// если она не помещается.
//
// Просто обрезать строку клипом мало: получается обрывок вроде «п», который
// читается как мусор, а не как укороченный заголовок. Если не помещается
// даже одна буква с многоточием, не рисуется ничего — пустое место честнее
// огрызка.
func DrawTextLeftElided(ctx widget.DrawContext, r image.Rectangle, text string, s *theme.Style) int {
	if text == "" || s == nil || r.Empty() {
		return 0
	}
	avail := r.Dx() - 2*int(s.PadX)
	if avail <= 0 {
		return 0
	}
	shown := Elide(ctx, text, s, avail)
	if shown == "" {
		return 0
	}
	return DrawTextLeft(ctx, r, shown, s)
}

// Elide укорачивает строку до ширины maxW, добавляя многоточие. Возвращает
// пустую строку, если не помещается даже одна буква с многоточием.
func Elide(ctx widget.DrawContext, text string, s *theme.Style, maxW int) string {
	if text == "" || maxW <= 0 {
		return ""
	}
	if MeasureText(ctx, text, s) <= maxW {
		return text
	}
	const ellipsis = "…"
	runes := []rune(text)
	// Отрезаем с конца по руне, пока строка с многоточием не влезет.
	for n := len(runes) - 1; n > 0; n-- {
		if MeasureText(ctx, string(runes[:n])+ellipsis, s) <= maxW {
			return string(runes[:n]) + ellipsis
		}
	}
	return ""
}

// MeasureText возвращает ширину строки шрифтом стиля.
func MeasureText(ctx widget.DrawContext, text string, s *theme.Style) int {
	if text == "" || s == nil {
		return 0
	}
	size := s.Font.Size
	if size <= 0 {
		size = widget.DefaultFontSizePt
	}
	return measure(ctx, text, size, s.Font)
}

func measure(ctx widget.DrawContext, text string, size float64, f theme.FontSpec) int {
	if f.Family != "" {
		return ctx.MeasureTextFont(text, size, f.Family)
	}
	return ctx.MeasureText(text, size)
}

func drawText(ctx widget.DrawContext, text string, x, y int, size float64, s *theme.Style) {
	if s.Font.Family != "" {
		ctx.DrawTextFont(text, x, y, size, s.Font.Family, s.Text)
		return
	}
	ctx.DrawTextSize(text, x, y, size, s.Text)
}

// StateOf собирает состояние компонента из признаков, которые есть у всех:
// наведение, нажатие, отключённость, выбранность.
//
// Компоненту остаётся передать свои булевы поля — а какое состояние сильнее
// и как оно ложится на стиль, решает тема.
func StateOf(hover, pressed, active, disabled, focused bool) theme.State {
	var st theme.State
	if hover {
		st |= theme.StateHover
	}
	if pressed {
		st |= theme.StatePressed
	}
	if active {
		st |= theme.StateActive
	}
	if disabled {
		st |= theme.StateDisabled
	}
	if focused {
		st |= theme.StateFocused
	}
	return st
}
