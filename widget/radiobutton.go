package widget

import (
	"image"
	"image/color"
	"math"
	"sync"
	"sync/atomic"
)

// RadioButton — переключатель в стиле Windows 10 с поддержкой групп.
//
// Виджеты с одинаковым GroupName автоматически связываются:
// при выборе одного остальные в группе сбрасываются.
type RadioButton struct {
	Base

	Text       string
	GroupName  string
	TextColor  color.RGBA
	CircleBG   color.RGBA
	CircleBord color.RGBA
	DotColor   color.RGBA
	HoverBG    color.RGBA
	AccentBG   color.RGBA

	selected int32 // 0 | 1
	hovered  int32 // 0 | 1
	focused  int32 // 0 | 1

	OnChange func(selected bool)
}

// radioGroups хранит все RadioButton по имени группы.
var (
	radioMu     sync.Mutex
	radioGroups = make(map[string][]*RadioButton)
)

// NewRadioButton создаёт переключатель с меткой и именем группы.
func NewRadioButton(text, group string) *RadioButton {
	rb := &RadioButton{
		Text:       text,
		GroupName:  group,
		TextColor:  win10.CheckText,
		CircleBG:   win10.CheckBG,
		CircleBord: win10.CheckBorder,
		DotColor:   win10.CheckMark,
		HoverBG:    win10.CheckHoverBG,
		AccentBG:   win10.Accent,
	}
	if group != "" {
		radioMu.Lock()
		radioGroups[group] = append(radioGroups[group], rb)
		radioMu.Unlock()
	}
	return rb
}

// SetSelected потокобезопасно выбирает этот RadioButton и сбрасывает остальные в группе.
func (rb *RadioButton) SetSelected(v bool) {
	if v {
		atomic.StoreInt32(&rb.selected, 1)
		rb.deselectOthers()
	} else {
		atomic.StoreInt32(&rb.selected, 0)
	}
}

// IsSelected возвращает true, если RadioButton выбран.
func (rb *RadioButton) IsSelected() bool {
	return atomic.LoadInt32(&rb.selected) == 1
}

func (rb *RadioButton) SetHovered(v bool) {
	if v {
		atomic.StoreInt32(&rb.hovered, 1)
	} else {
		atomic.StoreInt32(&rb.hovered, 0)
	}
}

func (rb *RadioButton) IsHovered() bool {
	return atomic.LoadInt32(&rb.hovered) == 1
}

func (rb *RadioButton) OnMouseMove(x, y int) {
	if !rb.IsEnabled() {
		rb.SetHovered(false)
		return
	}
	rb.SetHovered(image.Pt(x, y).In(rb.bounds))
}

// deselectOthers сбрасывает все RadioButton в той же группе, кроме текущего.
func (rb *RadioButton) deselectOthers() {
	if rb.GroupName == "" {
		return
	}
	radioMu.Lock()
	group := radioGroups[rb.GroupName]
	radioMu.Unlock()
	for _, other := range group {
		if other != rb {
			atomic.StoreInt32(&other.selected, 0)
		}
	}
}

// Draw рисует RadioButton: кружок 16×16 слева + текст справа.
func (rb *RadioButton) Draw(ctx DrawContext) {
	b := rb.bounds
	if b.Empty() {
		return
	}
	const diam = 16
	cx := b.Min.X + diam/2
	cy := b.Min.Y + b.Dy()/2

	// Фон кружка
	bg := rb.CircleBG
	if rb.IsSelected() {
		bg = rb.AccentBG
	} else if rb.IsHovered() {
		bg = rb.HoverBG
	}

	// Рисуем закрашенный круг
	drawFilledCircle(ctx, cx, cy, diam/2, bg)
	drawCircleOutline(ctx, cx, cy, diam/2, rb.CircleBord)

	// Точка выбора (маленький белый кружок внутри)
	if rb.IsSelected() {
		drawFilledCircle(ctx, cx, cy, 4, rb.DotColor)
	}

	// Текст
	const textPad = 6
	textX := b.Min.X + diam + textPad
	textY := b.Min.Y + (b.Dy()-13)/2
	ctx.DrawText(rb.Text, textX, textY, rb.TextColor)

	rb.drawChildren(ctx)
	rb.drawDisabledOverlay(ctx)
}

// OnMouseButton обрабатывает клик — выбирает этот RadioButton.
func (rb *RadioButton) OnMouseButton(e MouseEvent) bool {
	if !rb.IsEnabled() {
		return false
	}
	if e.Button == MouseLeft && !e.Pressed {
		if !rb.IsSelected() {
			rb.SetSelected(true)
			if rb.OnChange != nil {
				rb.OnChange(true)
			}
		}
		return true
	}
	return false
}

// ─── Focusable ───────────────────────────────────────────────────────────────

func (rb *RadioButton) SetFocused(v bool) {
	if v {
		atomic.StoreInt32(&rb.focused, 1)
	} else {
		atomic.StoreInt32(&rb.focused, 0)
	}
}

func (rb *RadioButton) IsFocused() bool {
	return atomic.LoadInt32(&rb.focused) == 1
}

// ─── KeyHandler ──────────────────────────────────────────────────────────────

func (rb *RadioButton) OnKeyEvent(e KeyEvent) {
	if !rb.IsEnabled() || !e.Pressed {
		return
	}
	if e.Code == KeySpace {
		if !rb.IsSelected() {
			rb.SetSelected(true)
			if rb.OnChange != nil {
				go rb.OnChange(true)
			}
		}
	}
}

// ApplyTheme обновляет цвета RadioButton.
func (rb *RadioButton) ApplyTheme(t *Theme) {
	rb.TextColor = t.CheckText
	rb.CircleBG = t.CheckBG
	rb.CircleBord = t.CheckBorder
	rb.DotColor = t.CheckMark
	rb.HoverBG = t.CheckHoverBG
	rb.AccentBG = t.Accent
}

// RemoveFromGroup удаляет RadioButton из глобального реестра групп.
// Вызывать при удалении виджета из дерева.
func (rb *RadioButton) RemoveFromGroup() {
	if rb.GroupName == "" {
		return
	}
	radioMu.Lock()
	defer radioMu.Unlock()
	group := radioGroups[rb.GroupName]
	for i, r := range group {
		if r == rb {
			radioGroups[rb.GroupName] = append(group[:i], group[i+1:]...)
			break
		}
	}
}

// ─── Вспомогательные функции рисования кругов ────────────────────────────────

// drawFilledCircle рисует закрашенный круг (Midpoint circle fill).
func drawFilledCircle(ctx DrawContext, cx, cy, r int, col color.RGBA) {
	for dy := -r; dy <= r; dy++ {
		halfW := int(math.Sqrt(float64(r*r - dy*dy)))
		ctx.DrawHLine(cx-halfW, cy+dy, halfW*2+1, col)
	}
}

// drawCircleOutline рисует контур круга (Midpoint circle algorithm).
func drawCircleOutline(ctx DrawContext, cx, cy, r int, col color.RGBA) {
	x, y := r, 0
	d := 1 - r
	for x >= y {
		ctx.SetPixel(cx+x, cy+y, col)
		ctx.SetPixel(cx-x, cy+y, col)
		ctx.SetPixel(cx+x, cy-y, col)
		ctx.SetPixel(cx-x, cy-y, col)
		ctx.SetPixel(cx+y, cy+x, col)
		ctx.SetPixel(cx-y, cy+x, col)
		ctx.SetPixel(cx+y, cy-x, col)
		ctx.SetPixel(cx-y, cy-x, col)
		y++
		if d < 0 {
			d += 2*y + 1
		} else {
			x--
			d += 2*(y-x) + 1
		}
	}
}
