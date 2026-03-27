package widget

import (
	"image/color"
	"sync"
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
	ctx.DrawText(text, b.Min.X+l.PaddingX, b.Min.Y+l.PaddingY, l.TextColor)
	l.drawChildren(ctx)
}

// ApplyTheme обновляет цвет текста в соответствии с темой.
func (l *Label) ApplyTheme(t *Theme) {
	l.TextColor = t.LabelText
}
