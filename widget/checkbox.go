package widget

import (
	"image"
	"image/color"
	"sync/atomic"
)

// CheckBox — флажок в стиле Windows 10.
// Состояние checked и hovered меняются атомарно — потокобезопасно.
type CheckBox struct {
	Base

	Text       string
	TextColor  color.RGBA
	BoxBG      color.RGBA // фон квадратика
	BoxBorder  color.RGBA
	CheckColor color.RGBA // цвет галочки
	HoverBG    color.RGBA
	AccentBG   color.RGBA // фон квадратика когда checked

	checked int32 // 0 | 1
	hovered int32 // 0 | 1
	focused int32 // 0 | 1

	OnChange func(checked bool)
}

// NewCheckBox создаёт флажок с текстовой меткой.
func NewCheckBox(text string) *CheckBox {
	return &CheckBox{
		Text:       text,
		TextColor:  win10.CheckText,
		BoxBG:      win10.CheckBG,
		BoxBorder:  win10.CheckBorder,
		CheckColor: win10.CheckMark,
		HoverBG:    win10.CheckHoverBG,
		AccentBG:   win10.Accent,
	}
}

// SetChecked потокобезопасно задаёт состояние.
func (cb *CheckBox) SetChecked(v bool) {
	if v {
		atomic.StoreInt32(&cb.checked, 1)
	} else {
		atomic.StoreInt32(&cb.checked, 0)
	}
}

// IsChecked возвращает текущее состояние.
func (cb *CheckBox) IsChecked() bool {
	return atomic.LoadInt32(&cb.checked) == 1
}

func (cb *CheckBox) SetHovered(v bool) {
	if v {
		atomic.StoreInt32(&cb.hovered, 1)
	} else {
		atomic.StoreInt32(&cb.hovered, 0)
	}
}

func (cb *CheckBox) IsHovered() bool {
	return atomic.LoadInt32(&cb.hovered) == 1
}

func (cb *CheckBox) OnMouseMove(x, y int) {
	if !cb.IsEnabled() {
		cb.SetHovered(false)
		return
	}
	cb.SetHovered(image.Pt(x, y).In(cb.bounds))
}

// Draw рисует CheckBox: квадратик 16×16 слева + текст справа.
func (cb *CheckBox) Draw(ctx DrawContext) {
	b := cb.bounds
	boxSize := 16
	boxX := b.Min.X
	boxY := b.Min.Y + (b.Dy()-boxSize)/2

	// Фон квадратика
	bg := cb.BoxBG
	if cb.IsChecked() {
		bg = cb.AccentBG
	} else if cb.IsHovered() {
		bg = cb.HoverBG
	}
	ctx.FillRect(boxX, boxY, boxSize, boxSize, bg)
	if cb.IsFocused() {
		ctx.DrawBorder(boxX, boxY, boxSize, boxSize, cb.AccentBG)
	} else {
		ctx.DrawBorder(boxX, boxY, boxSize, boxSize, cb.BoxBorder)
	}

	// Галочка (рисуем простой чек-марк линиями)
	if cb.IsChecked() {
		cx := boxX + 3
		cy := boxY + 8
		// Рисуем V-образную галочку
		for i := 0; i < 3; i++ {
			ctx.SetPixel(cx+i, cy-i, cb.CheckColor)
			ctx.SetPixel(cx+i, cy-i+1, cb.CheckColor)
		}
		for i := 0; i < 6; i++ {
			ctx.SetPixel(cx+2+i, cy-2+i, cb.CheckColor)
			ctx.SetPixel(cx+2+i, cy-2+i+1, cb.CheckColor)
		}
		// Утолщённая версия для видимости
		for i := 0; i < 3; i++ {
			ctx.SetPixel(cx+i+1, cy-i, cb.CheckColor)
		}
		for i := 0; i < 6; i++ {
			ctx.SetPixel(cx+2+i+1, cy-2+i, cb.CheckColor)
		}
	}

	// Текст
	const textPad = 6
	textX := boxX + boxSize + textPad
	textY := b.Min.Y + (b.Dy()-13)/2
	ctx.DrawText(cb.Text, textX, textY, cb.TextColor)

	cb.drawChildren(ctx)
	cb.drawDisabledOverlay(ctx)
}

// OnMouseButton обрабатывает клик — переключает состояние.
func (cb *CheckBox) OnMouseButton(e MouseEvent) bool {
	if !cb.IsEnabled() {
		return false
	}
	if e.Button == MouseLeft && !e.Pressed {
		newState := !cb.IsChecked()
		cb.SetChecked(newState)
		if cb.OnChange != nil {
			cb.OnChange(newState)
		}
		return true
	}
	return false
}

// ─── Focusable ───────────────────────────────────────────────────────────────

func (cb *CheckBox) SetFocused(v bool) {
	if v {
		atomic.StoreInt32(&cb.focused, 1)
	} else {
		atomic.StoreInt32(&cb.focused, 0)
	}
}

func (cb *CheckBox) IsFocused() bool {
	return atomic.LoadInt32(&cb.focused) == 1
}

// ─── KeyHandler ──────────────────────────────────────────────────────────────

func (cb *CheckBox) OnKeyEvent(e KeyEvent) {
	if !cb.IsEnabled() || !e.Pressed {
		return
	}
	if e.Code == KeySpace {
		newState := !cb.IsChecked()
		cb.SetChecked(newState)
		if cb.OnChange != nil {
			go cb.OnChange(newState)
		}
	}
}

// ApplyTheme обновляет цвета CheckBox.
func (cb *CheckBox) ApplyTheme(t *Theme) {
	cb.TextColor = t.CheckText
	cb.BoxBG = t.CheckBG
	cb.BoxBorder = t.CheckBorder
	cb.CheckColor = t.CheckMark
	cb.HoverBG = t.CheckHoverBG
	cb.AccentBG = t.Accent
}
