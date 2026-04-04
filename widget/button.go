package widget

import (
	"image"
	"image/color"
	"sync/atomic"
)

// IconPosition определяет расположение иконки относительно текста.
type IconPosition int

const (
	// IconLeft — иконка слева от текста (по умолчанию).
	IconLeft IconPosition = iota
	// IconTop — иконка над текстом.
	IconTop
	// IconOnly — только иконка, текст не отображается.
	IconOnly
)

// Button — кнопка в стиле Windows 10 Dark.
// Pressed и Hovered меняются атомарно — можно вызывать из любой горутины.
type Button struct {
	Base

	Text          string
	TextColor     color.RGBA
	Background    color.RGBA
	HoverBG       color.RGBA // фон при наведении курсора
	PressedBG     color.RGBA
	BorderColor   color.RGBA
	HighlightTop  color.RGBA // 1-пиксельная акцентная линия сверху
	ShowHighlight bool

	// Icon — иконка кнопки (PNG/JPEG). Если не nil, рисуется рядом с текстом.
	Icon     image.Image
	IconPath string       // путь к файлу (заполняется из XAML атрибута Icon=)
	IconPos  IconPosition // расположение иконки (Left, Top, IconOnly)
	IconSize int          // размер иконки в пикселях (0 = авто: высота кнопки - 8)

	pressed int32 // 0 | 1, атомарно
	hovered int32 // 0 | 1, атомарно
	focused int32 // 0 | 1, атомарно
	OnClick func()
}

// NewButton создаёт кнопку в стиле Windows 10 Dark.
func NewButton(text string) *Button {
	return &Button{
		Text:         text,
		TextColor:    win10.BtnText,
		Background:   win10.BtnBG,
		HoverBG:      color.RGBA{R: 62, G: 62, B: 64, A: 255},
		PressedBG:    win10.BtnPressedBG,
		BorderColor:  win10.BtnBorder,
		HighlightTop: win10.Accent,
	}
}

// NewWin10AccentButton создаёт кнопку с синим акцентным фоном («primary action»).
func NewWin10AccentButton(text string) *Button {
	return &Button{
		Text:        text,
		TextColor:   color.RGBA{R: 255, G: 255, B: 255, A: 255},
		Background:  win10.Accent,
		HoverBG:     color.RGBA{R: 0, G: 99, B: 177, A: 255},
		PressedBG:   color.RGBA{R: 0, G: 84, B: 153, A: 255},
		BorderColor: color.RGBA{R: 0, G: 84, B: 153, A: 255},
	}
}

// SetPressed потокобезопасно меняет состояние нажатия.
func (btn *Button) SetPressed(v bool) {
	if v {
		atomic.StoreInt32(&btn.pressed, 1)
	} else {
		atomic.StoreInt32(&btn.pressed, 0)
	}
}

// IsPressed возвращает текущее состояние нажатия.
func (btn *Button) IsPressed() bool {
	return atomic.LoadInt32(&btn.pressed) == 1
}

// SetHovered потокобезопасно меняет состояние наведения.
func (btn *Button) SetHovered(v bool) {
	if v {
		atomic.StoreInt32(&btn.hovered, 1)
	} else {
		atomic.StoreInt32(&btn.hovered, 0)
	}
}

// IsHovered возвращает true если курсор над кнопкой.
func (btn *Button) IsHovered() bool {
	return atomic.LoadInt32(&btn.hovered) == 1
}

// OnMouseMove реализует MouseMoveHandler — обновляет hover-состояние.
func (btn *Button) OnMouseMove(x, y int) {
	btn.SetHovered(image.Pt(x, y).In(btn.bounds))
}

func (btn *Button) Draw(ctx DrawContext) {
	b := btn.bounds

	bg := btn.Background
	switch {
	case btn.IsPressed():
		bg = btn.PressedBG
	case btn.IsHovered() && btn.HoverBG.A > 0:
		bg = btn.HoverBG
	}

	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), bg)
	if btn.IsFocused() {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), btn.HighlightTop)
	} else {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), btn.BorderColor)
	}

	if btn.ShowHighlight && !btn.IsPressed() {
		ctx.DrawHLine(b.Min.X+1, b.Min.Y, b.Dx()-2, btn.HighlightTop)
	}

	// ── Размер иконки ──────────────────────────────────────────────────────
	iconSz := btn.IconSize
	if iconSz <= 0 {
		iconSz = b.Dy() - 8 // авто: высота кнопки − 8px padding
		if iconSz < 12 {
			iconSz = 12
		}
	}
	hasIcon := btn.Icon != nil
	hasText := btn.Text != "" && btn.IconPos != IconOnly

	const textH = 13
	textW := 0
	if hasText {
		textW = ctx.MeasureText(btn.Text, DefaultFontSizePt)
	}
	iconGap := 4 // зазор между иконкой и текстом

	// ── Расположение контента ───────────────────────────────────────────
	switch {
	case hasIcon && hasText && btn.IconPos == IconLeft:
		// Иконка слева, текст справа — оба центрированы по вертикали
		totalW := iconSz + iconGap + textW
		startX := b.Min.X + (b.Dx()-totalW)/2
		if startX < b.Min.X+4 {
			startX = b.Min.X + 4
		}
		iconY := b.Min.Y + (b.Dy()-iconSz)/2
		ctx.DrawImageScaled(btn.Icon, startX, iconY, iconSz, iconSz)
		textX := startX + iconSz + iconGap
		textY := b.Min.Y + (b.Dy()-textH)/2
		ctx.DrawText(btn.Text, textX, textY, btn.TextColor)

	case hasIcon && hasText && btn.IconPos == IconTop:
		// Иконка сверху, текст снизу — оба центрированы по горизонтали
		totalH := iconSz + iconGap + textH
		startY := b.Min.Y + (b.Dy()-totalH)/2
		if startY < b.Min.Y+2 {
			startY = b.Min.Y + 2
		}
		iconX := b.Min.X + (b.Dx()-iconSz)/2
		ctx.DrawImageScaled(btn.Icon, iconX, startY, iconSz, iconSz)
		textX := b.Min.X + (b.Dx()-textW)/2
		textY := startY + iconSz + iconGap
		ctx.DrawText(btn.Text, textX, textY, btn.TextColor)

	case hasIcon && !hasText:
		// Только иконка — центрирована
		iconX := b.Min.X + (b.Dx()-iconSz)/2
		iconY := b.Min.Y + (b.Dy()-iconSz)/2
		ctx.DrawImageScaled(btn.Icon, iconX, iconY, iconSz, iconSz)

	default:
		// Только текст (или нет ничего)
		textX := b.Min.X + (b.Dx()-textW)/2
		textY := b.Min.Y + (b.Dy()-textH)/2
		if textX < b.Min.X+4 {
			textX = b.Min.X + 4
		}
		if textY < b.Min.Y+2 {
			textY = b.Min.Y + 2
		}
		if hasText {
			ctx.DrawText(btn.Text, textX, textY, btn.TextColor)
		}
	}

	btn.drawChildren(ctx)
}

// OnMouseButton реализует widget.MouseClickHandler — вызывает OnClick при нажатии.
func (btn *Button) OnMouseButton(e MouseEvent) bool {
	if e.Button == MouseLeft {
		btn.SetPressed(e.Pressed)
		if !e.Pressed && btn.OnClick != nil {
			btn.OnClick()
		}
		return true
	}
	return false
}

// ─── Focusable ───────────────────────────────────────────────────────────────

func (btn *Button) SetFocused(v bool) {
	if v {
		atomic.StoreInt32(&btn.focused, 1)
	} else {
		atomic.StoreInt32(&btn.focused, 0)
	}
}

func (btn *Button) IsFocused() bool {
	return atomic.LoadInt32(&btn.focused) == 1
}

// ─── KeyHandler ──────────────────────────────────────────────────────────────

func (btn *Button) OnKeyEvent(e KeyEvent) {
	if !e.Pressed {
		return
	}
	// Enter или Space активируют кнопку
	if e.Code == KeyEnter || e.Code == KeySpace {
		if btn.OnClick != nil {
			go btn.OnClick()
		}
	}
}

// ApplyTheme обновляет цвета кнопки в соответствии с темой.
func (btn *Button) ApplyTheme(t *Theme) {
	btn.TextColor = t.BtnText
	btn.Background = t.BtnBG
	btn.HoverBG = t.BtnHoverBG
	btn.PressedBG = t.BtnPressedBG
	btn.BorderColor = t.BtnBorder
	btn.HighlightTop = t.Accent
}
