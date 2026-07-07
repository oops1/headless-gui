package widget

import (
	"image/color"
	"strings"
	"sync"
	"unicode"
)

// Label — текстовая метка.
// Текст можно менять из любой горутины через SetText.
type Label struct {
	Base

	mu        sync.RWMutex
	text      string
	TextColor color.RGBA

	HasBG      bool
	Background color.RGBA

	WrapText bool    // true — переносить текст по словам в пределах bounds
	Muted    bool    // true — вторичный текст: тема красит в приглушённый цвет
	FontSize float64 // размер шрифта в pt (0 → DefaultFontSizePt)
	FontName string  // именованный шрифт (зарегистрированный через RegisterFont); "" → default

	Bold      bool // FontWeight="Bold"
	Italic    bool // FontStyle="Italic"
	Underline bool // TextDecorations="Underline"

	PaddingX int
	PaddingY int
}

// Встроенные имена шрифтов для жирного/курсива (регистрируются движком из gofont).
const (
	BuiltinFontBold       = "$hg_bold"
	BuiltinFontItalic     = "$hg_italic"
	BuiltinFontBoldItalic = "$hg_bolditalic"
)

// effectiveFont выбирает шрифт: явный FontName важнее; иначе встроенный
// жирный/курсивный вариант по флагам Bold/Italic.
func (l *Label) effectiveFont() string {
	if l.FontName != "" {
		return l.FontName
	}
	switch {
	case l.Bold && l.Italic:
		return BuiltinFontBoldItalic
	case l.Bold:
		return BuiltinFontBold
	case l.Italic:
		return BuiltinFontItalic
	}
	return ""
}

// NewLabel создаёт метку с явным цветом текста.
func NewLabel(text string, col color.RGBA) *Label {
	return &Label{
		text:      text,
		TextColor: col,
		PaddingX:  2,
		PaddingY:  2,
	}
}

// NewWin10Label создаёт метку с цветом текста Win10 Dark.
func NewWin10Label(text string) *Label {
	return &Label{
		text:      text,
		TextColor: win10.LabelText,
		PaddingX:  2,
		PaddingY:  2,
	}
}

// SetText потокобезопасно обновляет текст.
// При фактическом изменении инвалидирует область виджета (авто-damage).
func (l *Label) SetText(text string) {
	l.mu.Lock()
	if l.text == text {
		l.mu.Unlock()
		return
	}
	l.text = text
	l.mu.Unlock()
	l.Invalidate() // вне l.mu
}

// Text потокобезопасно возвращает текущий текст.
func (l *Label) Text() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.text
}

func (l *Label) Draw(ctx DrawContext) {
	b := l.bounds
	if b.Empty() {
		return
	}

	l.mu.RLock()
	text := l.text
	l.mu.RUnlock()

	if l.HasBG {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), l.Background)
	}

	fontSize := l.FontSize
	if fontSize <= 0 {
		fontSize = DefaultFontSizePt
	}

	font := l.effectiveFont()
	if !l.WrapText {
		l.drawLine(ctx, text, b.Min.X+l.PaddingX, b.Min.Y+l.PaddingY, fontSize, font)
	} else {
		maxW := b.Dx() - l.PaddingX*2
		lines := wrapTextPixelFont(ctx, text, fontSize, font, maxW)
		lineH := int(fontSize*1.5 + 0.5) // межстрочный интервал
		y := b.Min.Y + l.PaddingY
		for _, line := range lines {
			if y+lineH > b.Max.Y {
				break // не вылезаем за границы
			}
			l.drawLine(ctx, line, b.Min.X+l.PaddingX, y, fontSize, font)
			y += lineH
		}
	}
	l.drawChildren(ctx)
}

// drawLine рисует одну строку нужным шрифтом, при необходимости — подчёркивание.
func (l *Label) drawLine(ctx DrawContext, text string, x, y int, sizePt float64, font string) {
	if font != "" {
		ctx.DrawTextFont(text, x, y, sizePt, font, l.TextColor)
	} else {
		ctx.DrawTextSize(text, x, y, sizePt, l.TextColor)
	}
	if l.Underline && text != "" {
		var tw int
		if font != "" {
			tw = ctx.MeasureTextFont(text, sizePt, font)
		} else {
			tw = ctx.MeasureText(text, sizePt)
		}
		uy := y + int(sizePt*1.35+0.5)
		ctx.DrawHLine(x, uy, tw, l.TextColor)
	}
}

// wrapTextPixelFont разбивает text на строки с именованным шрифтом.
func wrapTextPixelFont(ctx DrawContext, text string, sizePt float64, fontName string, maxW int) []string {
	if fontName == "" {
		return wrapTextPixel(ctx, text, sizePt, maxW)
	}
	if maxW <= 0 {
		return []string{text}
	}
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		words := splitWords(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		var line string
		for _, word := range words {
			candidate := line
			if candidate != "" {
				candidate += " "
			}
			candidate += word
			if ctx.MeasureTextFont(candidate, sizePt, fontName) > maxW && line != "" {
				result = append(result, line)
				line = word
			} else {
				line = candidate
			}
		}
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

// wrapTextPixel разбивает text на строки, чтобы каждая влезала в maxW пикселей.
func wrapTextPixel(ctx DrawContext, text string, sizePt float64, maxW int) []string {
	if maxW <= 0 {
		return []string{text}
	}
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		words := splitWords(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		var line string
		for _, word := range words {
			candidate := line
			if candidate != "" {
				candidate += " "
			}
			candidate += word
			if ctx.MeasureText(candidate, sizePt) > maxW && line != "" {
				result = append(result, line)
				line = word
			} else {
				line = candidate
			}
		}
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

// splitWords разбивает строку по пробелам, сохраняя слова.
func splitWords(s string) []string {
	var words []string
	var cur strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) {
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
		} else {
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	return words
}

// ApplyTheme обновляет цвет текста в соответствии с темой.
// Непрозрачный фон следует за темой (как у Panel/Canvas) — иначе после
// переключения остаются «островки» прежней палитры.
func (l *Label) ApplyTheme(t *Theme) {
	if l.Muted {
		l.TextColor = t.InputPlaceholder
	} else {
		l.TextColor = t.LabelText
	}
	if l.Background.A > 0 {
		l.Background = t.PanelBG
	}
}
