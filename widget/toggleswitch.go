package widget

import (
	"image"
	"image/color"
	"sync/atomic"
)

// ToggleSwitch — переключатель вкл/выкл в стиле Windows 10.
//
// Визуально: горизонтальная капсула с кружком-ползунком.
// Размер капсулы: 44×22. Кружок перемещается слева (выкл) направо (вкл).
type ToggleSwitch struct {
	Base

	Text       string     // текст метки справа от переключателя
	TextColor  color.RGBA
	OffBG      color.RGBA // фон выключенного состояния
	OnBG       color.RGBA // фон включённого
	ThumbColor color.RGBA // кружок
	BorderCol  color.RGBA // рамка капсулы

	on      int32 // 0 | 1
	hovered int32 // 0 | 1
	focused int32 // 0 | 1

	OnChange func(on bool)
}

const (
	toggleW      = 44 // ширина капсулы
	toggleH      = 22 // высота капсулы
	toggleThumbR = 7  // радиус кружка
	togglePad    = 4  // отступ кружка от края
)

// NewToggleSwitch создаёт переключатель с текстовой меткой.
func NewToggleSwitch(text string) *ToggleSwitch {
	return &ToggleSwitch{
		Text:       text,
		TextColor:  win10.CheckText,
		OffBG:      win10.ToggleBG,
		OnBG:       win10.ToggleOnBG,
		ThumbColor: win10.ToggleThumb,
		BorderCol:  win10.ToggleBorder,
	}
}

// SetOn потокобезопасно задаёт состояние.
func (ts *ToggleSwitch) SetOn(v bool) {
	if v {
		atomic.StoreInt32(&ts.on, 1)
	} else {
		atomic.StoreInt32(&ts.on, 0)
	}
}

// IsOn возвращает текущее состояние.
func (ts *ToggleSwitch) IsOn() bool {
	return atomic.LoadInt32(&ts.on) == 1
}

func (ts *ToggleSwitch) SetHovered(v bool) {
	if v {
		atomic.StoreInt32(&ts.hovered, 1)
	} else {
		atomic.StoreInt32(&ts.hovered, 0)
	}
}

func (ts *ToggleSwitch) IsHovered() bool {
	return atomic.LoadInt32(&ts.hovered) == 1
}

func (ts *ToggleSwitch) OnMouseMove(x, y int) {
	if !ts.IsEnabled() {
		ts.SetHovered(false)
		return
	}
	ts.SetHovered(image.Pt(x, y).In(ts.bounds))
}

// Draw рисует ToggleSwitch: капсула + кружок + текст.
func (ts *ToggleSwitch) Draw(ctx DrawContext) {
	b := ts.bounds
	capY := b.Min.Y + (b.Dy()-toggleH)/2

	// Фон капсулы
	bg := ts.OffBG
	if ts.IsOn() {
		bg = ts.OnBG
	}
	ctx.FillRoundRect(b.Min.X, capY, toggleW, toggleH, toggleH/2, bg)
	ctx.DrawRoundBorder(b.Min.X, capY, toggleW, toggleH, toggleH/2, ts.BorderCol)

	// Кружок
	var cx int
	if ts.IsOn() {
		cx = b.Min.X + toggleW - togglePad - toggleThumbR
	} else {
		cx = b.Min.X + togglePad + toggleThumbR
	}
	cy := capY + toggleH/2
	drawFilledCircle(ctx, cx, cy, toggleThumbR, ts.ThumbColor)

	// Текст
	if ts.Text != "" {
		textX := b.Min.X + toggleW + 8
		textY := b.Min.Y + (b.Dy()-13)/2
		ctx.DrawText(ts.Text, textX, textY, ts.TextColor)
	}

	ts.drawChildren(ctx)
	ts.drawDisabledOverlay(ctx)
}

// OnMouseButton обрабатывает клик — переключает состояние.
func (ts *ToggleSwitch) OnMouseButton(e MouseEvent) bool {
	if !ts.IsEnabled() {
		return false
	}
	if e.Button == MouseLeft && !e.Pressed {
		newState := !ts.IsOn()
		ts.SetOn(newState)
		if ts.OnChange != nil {
			ts.OnChange(newState)
		}
		return true
	}
	return false
}

// ─── Focusable ───────────────────────────────────────────────────────────────

func (ts *ToggleSwitch) SetFocused(v bool) {
	if v {
		atomic.StoreInt32(&ts.focused, 1)
	} else {
		atomic.StoreInt32(&ts.focused, 0)
	}
}

func (ts *ToggleSwitch) IsFocused() bool {
	return atomic.LoadInt32(&ts.focused) == 1
}

// ─── KeyHandler ──────────────────────────────────────────────────────────────

func (ts *ToggleSwitch) OnKeyEvent(e KeyEvent) {
	if !ts.IsEnabled() || !e.Pressed {
		return
	}
	if e.Code == KeySpace {
		newState := !ts.IsOn()
		ts.SetOn(newState)
		if ts.OnChange != nil {
			go ts.OnChange(newState)
		}
	}
}

// ApplyTheme обновляет цвета ToggleSwitch.
func (ts *ToggleSwitch) ApplyTheme(t *Theme) {
	ts.TextColor = t.CheckText
	ts.OffBG = t.ToggleBG
	ts.OnBG = t.ToggleOnBG
	ts.ThumbColor = t.ToggleThumb
	ts.BorderCol = t.ToggleBorder
}
