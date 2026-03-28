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

	WrapText bool // true — переносить текст по словам в пределах bounds
	FontSize float64

	PaddingX int
	PaddingY int
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
func (l *Label) SetText(text string) {
	l.mu.Lock()
	l.text = text
	l.mu.Unlock()
}

// Text потокобезопасно возвращает текущий текст.
func (l *Label) Text() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.text
}

func (l *Label) Draw(ctx DrawContext) {
	l.mu.RLock()
	text := l.text
	l.mu.RUnlock()

	b := l.bounds
	if l.HasBG {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), l.Background)
	}

	fontSize := l.FontSize
	if fontSize <= 0 {
		fontSize = DefaultFontSizePt
	}

	if !l.WrapText {
		ctx.DrawTextSize(text, b.Min.X+l.PaddingX, b.Min.Y+l.PaddingY, fontSize, l.TextColor)
	} else {
		maxW := b.Dx() - l.PaddingX*2
		lines := wrapTextPixel(ctx, text, fontSize, maxW)
		lineH := int(fontSize*1.5 + 0.5) // межстрочный интервал
		y := b.Min.Y + l.PaddingY
		for _, line := range lines {
			if y+lineH > b.Max.Y {
				break // не вылезаем за границы
			}
			ctx.DrawTextSize(line, b.Min.X+l.PaddingX, y, fontSize, l.TextColor)
			y += lineH
		}
	}
	l.drawChildren(ctx)
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
func (l *Label) ApplyTheme(t *Theme) {
	l.TextColor = t.LabelText
}
