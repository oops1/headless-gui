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
	pressing int32 // 1 — press был на этом виджете

	OnChange func(selected bool)
}

// radioGroups хранит все RadioButton по имени группы.
var (
	radioMu     sync.Mutex
	radioGroups = make(map[string][]*RadioButton)
)

// SetText задаёт текст метки переключателя.
func (r *RadioButton) SetText(s string) {
	if r.Text == s {
		return
	}
	r.Text = s
	r.Invalidate()
}

// GetText возвращает текущий текст метки.
func (r *RadioButton) GetText() string { return r.Text }

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
// При фактическом изменении инвалидирует область виджета (авто-damage).
func (rb *RadioButton) SetSelected(v bool) {
	if atomic.SwapInt32(&rb.selected, b2i(v)) != b2i(v) {
		rb.Invalidate()
	}
	if v {
		rb.deselectOthers()
	}
}

// IsSelected возвращает true, если RadioButton выбран.
func (rb *RadioButton) IsSelected() bool {
	return atomic.LoadInt32(&rb.selected) == 1
}

func (rb *RadioButton) SetHovered(v bool) {
	if atomic.SwapInt32(&rb.hovered, b2i(v)) != b2i(v) {
		rb.Invalidate()
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
			// Сброс выбора меняет отрисовку соседа — инвалидируем его область.
			if atomic.SwapInt32(&other.selected, 0) != 0 {
				other.Invalidate()
			}
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

	st := currentStyle()
	if st.Classic3D {
		// Классика Win2000: утопленный белый круг (как чекбокс) — двойное
		// направленное кольцо: тёмные дуги сверху-слева, светлые снизу-справа,
		// чёрная точка выбора.
		drawFilledCircle(ctx, cx, cy, diam/2-1, rb.CircleBG)
		drawSunkenRing(ctx, cx, cy, diam/2, st.BevelShadow, st.BevelLight)
		drawSunkenRing(ctx, cx, cy, diam/2-1, st.BevelDark, win10.WindowBG)
		if rb.IsSelected() {
			drawFilledCircle(ctx, cx, cy, 3, win10.CheckMark) // классика: тёмная точка
		}
	} else {
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
	}

	// Текст
	const textPad = 6
	textX := b.Min.X + diam + textPad
	textY := b.Min.Y + (b.Dy()-13)/2
	ctx.DrawText(rb.Text, textX, textY, rb.TextColor)

	// Классика Win2000: пунктирная рамка фокуса вокруг текста метки.
	if st.Classic3D && rb.IsFocused() && rb.Text != "" {
		tw := ctx.MeasureText(rb.Text, DefaultFontSizePt)
		drawDottedRect(ctx, textX-2, textY-2, tw+5, 17, st.BevelDark)
	}

	rb.drawChildren(ctx)
	rb.drawDisabledOverlay(ctx)
}

// OnMouseButton обрабатывает клик — выбирает этот RadioButton.
//
// WPF-совместимое поведение: переключение только если press был
// на этом же RadioButton (защита от «пролётного» release).
func (rb *RadioButton) OnMouseButton(e MouseEvent) bool {
	if !rb.IsEnabled() {
		return false
	}
	if e.Button == MouseLeft {
		if e.Pressed {
			atomic.StoreInt32(&rb.pressing, 1)
			return true
		}
		wasDown := atomic.SwapInt32(&rb.pressing, 0) != 0
		if wasDown && !rb.IsSelected() {
			rb.SetSelected(true)
			if rb.OnChange != nil {
				rb.OnChange(true)
			}
		}
		return wasDown
	}
	return false
}

// ─── Focusable ───────────────────────────────────────────────────────────────

func (rb *RadioButton) SetFocused(v bool) {
	if atomic.SwapInt32(&rb.focused, b2i(v)) != b2i(v) {
		rb.Invalidate()
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
				rb.OnChange(true) // синхронно — единая модель с mouse-path
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

// drawFilledCircle рисует закрашенный круг (AA при поддержке, иначе Midpoint fill).
func drawFilledCircle(ctx DrawContext, cx, cy, r int, col color.RGBA) {
	if aa, ok := ctx.(AAShapes); ok {
		aa.FillEllipseAA(cx, cy, r, r, col)
		return
	}
	for dy := -r; dy <= r; dy++ {
		halfW := int(math.Sqrt(float64(r*r - dy*dy)))
		ctx.DrawHLine(cx-halfW, cy+dy, halfW*2+1, col)
	}
}

// drawCircleOutline рисует контур круга (AA при поддержке, иначе Midpoint).
func drawCircleOutline(ctx DrawContext, cx, cy, r int, col color.RGBA) {
	if aa, ok := ctx.(AAShapes); ok {
		aa.StrokeEllipseAA(cx, cy, r, r, 1.2, col)
		return
	}
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
