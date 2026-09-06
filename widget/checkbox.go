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

	Text      string
	TextColor color.RGBA

	// FontSize — кегль подписи в пунктах (0 = общий размер).
	FontSize float64

	BoxBG      color.RGBA // фон квадратика
	BoxBorder  color.RGBA
	CheckColor color.RGBA // цвет галочки
	HoverBG    color.RGBA
	AccentBG   color.RGBA // фон квадратика когда checked

	checked  int32 // 0 | 1
	hovered  int32 // 0 | 1
	focused  int32 // 0 | 1
	pressing int32 // 1 — press был на этом виджете, ожидаем release

	OnChange func(checked bool)
}

// SetText задаёт текст метки флажка.
func (c *CheckBox) SetText(s string) {
	if c.Text == s {
		return
	}
	c.Text = s
	c.Invalidate()
}

// GetText возвращает текущий текст метки.
func (c *CheckBox) GetText() string { return c.Text }

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
// При фактическом изменении инвалидирует область виджета (авто-damage).
func (cb *CheckBox) SetChecked(v bool) {
	if atomic.SwapInt32(&cb.checked, b2i(v)) != b2i(v) {
		cb.Invalidate()
	}
}

// IsChecked возвращает текущее состояние.
func (cb *CheckBox) IsChecked() bool {
	return atomic.LoadInt32(&cb.checked) == 1
}

func (cb *CheckBox) SetHovered(v bool) {
	if atomic.SwapInt32(&cb.hovered, b2i(v)) != b2i(v) {
		cb.Invalidate()
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
	if b.Empty() {
		return
	}
	boxSize := 16
	boxX := b.Min.X
	boxY := b.Min.Y + (b.Dy()-boxSize)/2
	st := currentStyle()

	if st.Classic3D {
		// Классика Win2000: белый утопленный квадратик, тёмная галочка.
		ctx.FillRect(boxX, boxY, boxSize, boxSize, cb.BoxBG)
		drawBevelSunken(ctx, boxX, boxY, boxSize, boxSize, st)
	} else {
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
	}

	// Галочка ✓: короткое плечо идёт ВНИЗ-вправо к нижней вершине,
	// затем длинное плечо — ВВЕРХ-вправо (а не «домиком», как было раньше).
	if cb.IsChecked() {
		col := cb.CheckColor
		if aa, ok := ctx.(AAShapes); ok {
			// Сглаженная ломаная той же геометрии.
			aa.StrokePolylineAA([]image.Point{
				{X: boxX + 4, Y: boxY + 7},
				{X: boxX + 7, Y: boxY + 10},
				{X: boxX + 12, Y: boxY + 5},
			}, 1.8, false, col)
		} else {
			// Короткое плечо: (boxX+4, boxY+7) → (boxX+7, boxY+10), вниз-вправо.
			for i := 0; i <= 3; i++ {
				x := boxX + 4 + i
				y := boxY + 7 + i
				ctx.SetPixel(x, y, col)
				ctx.SetPixel(x, y+1, col) // утолщение
			}
			// Длинное плечо: (boxX+7, boxY+10) → (boxX+12, boxY+5), вверх-вправо.
			for i := 0; i <= 5; i++ {
				x := boxX + 7 + i
				y := boxY + 10 - i
				ctx.SetPixel(x, y, col)
				ctx.SetPixel(x, y+1, col) // утолщение
			}
		}
	}

	// Текст
	const textPad = 6
	textX := boxX + boxSize + textPad
	textY := b.Min.Y + (b.Dy()-13)/2
	sizePt := fontSizeOrDefault(cb.FontSize)
	ctx.DrawTextSize(cb.Text, textX, textY, sizePt, cb.TextColor)

	// Классика Win2000: пунктирная рамка фокуса вокруг текста метки.
	if st.Classic3D && cb.IsFocused() && cb.Text != "" {
		tw := ctx.MeasureText(cb.Text, sizePt)
		drawDottedRect(ctx, textX-2, textY-2, tw+5, 17, st.BevelDark)
	}

	cb.drawChildren(ctx)
	cb.drawDisabledOverlay(ctx)
}

// OnMouseButton обрабатывает клик — переключает состояние.
//
// WPF-совместимое поведение: переключение срабатывает только если
// press был на этом же CheckBox (защита от «пролётного» release).
func (cb *CheckBox) OnMouseButton(e MouseEvent) bool {
	if !cb.IsEnabled() {
		return false
	}
	if e.Button == MouseLeft {
		if e.Pressed {
			atomic.StoreInt32(&cb.pressing, 1)
			return true
		}
		wasDown := atomic.SwapInt32(&cb.pressing, 0) != 0
		if wasDown {
			newState := !cb.IsChecked()
			cb.SetChecked(newState)
			if cb.OnChange != nil {
				cb.OnChange(newState)
			}
		}
		return wasDown
	}
	return false
}

// ─── Focusable ───────────────────────────────────────────────────────────────

func (cb *CheckBox) SetFocused(v bool) {
	if atomic.SwapInt32(&cb.focused, b2i(v)) != b2i(v) {
		cb.Invalidate()
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
			cb.OnChange(newState) // синхронно — единая модель с mouse-path
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
