package widget

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// NumericUpDown — числовое поле со «спиннером» (стрелки вверх/вниз),
// аналог WPF Extended Toolkit IntegerUpDown / DoubleUpDown.
//
// Возможности:
//   - Min / Max / Step / Decimals
//   - клик по стрелкам ▲/▼ и колесо мыши изменяют значение
//   - стрелки клавиатуры Up/Down при фокусе
//   - прямой ввод числа (цифры, '.', '-'); фиксация по Enter
//   - значение всегда зажимается в [Min, Max]
type NumericUpDown struct {
	Base

	Min      float64
	Max      float64
	Step     float64
	Decimals int // количество знаков после запятой при отображении

	TextColor color.RGBA
	FieldBG   color.RGBA
	Border    color.RGBA
	AccentBG  color.RGBA
	HoverBG   color.RGBA

	OnChange func(value float64)

	mu          sync.Mutex
	value       float64
	editing     string // буфер редактирования (когда в фокусе)
	isEditing   bool
	focused     int32 // 0 | 1, атомарно
	upHovered   bool
	downHovered bool
}

const nudSpinnerWidth = 18

// NewNumericUpDown создаёт числовое поле. По умолчанию [0..100], шаг 1.
func NewNumericUpDown() *NumericUpDown {
	return &NumericUpDown{
		Min:       0,
		Max:       100,
		Step:      1,
		Decimals:  0,
		TextColor: win10.InputText,
		FieldBG:   win10.InputBG,
		Border:    win10.InputBorder,
		AccentBG:  win10.Accent,
		HoverBG:   win10.CheckHoverBG,
	}
}

// Value возвращает текущее значение.
func (n *NumericUpDown) Value() float64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.value
}

// SetValue задаёт значение (зажимается в [Min, Max]) и вызывает OnChange,
// если значение изменилось.
func (n *NumericUpDown) SetValue(v float64) {
	n.mu.Lock()
	old := n.value
	n.value = n.clamp(v)
	n.isEditing = false
	n.editing = ""
	changed := n.value != old
	nv := n.value
	cb := n.OnChange
	n.mu.Unlock()
	if changed && cb != nil {
		cb(nv)
	}
}

// clamp зажимает значение в допустимый диапазон (вызывать как угодно).
func (n *NumericUpDown) clamp(v float64) float64 {
	if n.Max > n.Min {
		if v > n.Max {
			v = n.Max
		}
		if v < n.Min {
			v = n.Min
		}
	}
	return v
}

func (n *NumericUpDown) format() string {
	return strconv.FormatFloat(n.value, 'f', n.Decimals, 64)
}

// step изменяет значение на ±Step; commit редактирования сначала.
func (n *NumericUpDown) step(dir float64) {
	n.mu.Lock()
	n.commitLocked()
	old := n.value
	step := n.Step
	if step == 0 {
		step = 1
	}
	n.value = n.clamp(n.value + dir*step)
	changed := n.value != old
	nv := n.value
	cb := n.OnChange
	n.mu.Unlock()
	if changed && cb != nil {
		cb(nv)
	}
}

// commitLocked парсит буфер редактирования в значение (под n.mu).
func (n *NumericUpDown) commitLocked() {
	if !n.isEditing {
		return
	}
	s := strings.TrimSpace(n.editing)
	if s != "" && s != "-" && s != "." && s != "-." {
		if v, err := strconv.ParseFloat(s, 64); err == nil {
			n.value = n.clamp(v)
		}
	}
	n.isEditing = false
	n.editing = ""
}

// commit фиксирует ввод и вызывает OnChange при изменении.
func (n *NumericUpDown) commit() {
	n.mu.Lock()
	old := n.value
	n.commitLocked()
	changed := n.value != old
	nv := n.value
	cb := n.OnChange
	n.mu.Unlock()
	if changed && cb != nil {
		cb(nv)
	}
}

// ─── Draw ─────────────────────────────────────────────────────────────────────

func (n *NumericUpDown) Draw(ctx DrawContext) {
	b := n.bounds
	if b.Empty() {
		return
	}

	spinX := b.Max.X - nudSpinnerWidth
	if spinX < b.Min.X {
		spinX = b.Min.X
	}

	// Поле ввода.
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), n.FieldBG)
	border := n.Border
	if n.IsFocused() {
		border = n.AccentBG
	}
	ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), border)

	// Текст значения.
	n.mu.Lock()
	var txt string
	if n.isEditing {
		txt = n.editing
	} else {
		txt = n.format()
	}
	n.mu.Unlock()
	textY := b.Min.Y + (b.Dy()-13)/2
	ctx.DrawText(txt, b.Min.X+6, textY, n.TextColor)

	// Спиннер: две половинки.
	midY := b.Min.Y + b.Dy()/2
	// Разделитель по вертикали и горизонтали.
	ctx.FillRect(spinX, b.Min.Y, 1, b.Dy(), n.Border)
	if n.upHovered {
		ctx.FillRect(spinX+1, b.Min.Y+1, nudSpinnerWidth-2, midY-b.Min.Y-1, n.HoverBG)
	}
	if n.downHovered {
		ctx.FillRect(spinX+1, midY, nudSpinnerWidth-2, b.Max.Y-midY-1, n.HoverBG)
	}
	ctx.FillRect(spinX, midY, nudSpinnerWidth, 1, n.Border)

	// Стрелки ▲ / ▼ (маленькие треугольники).
	cx := spinX + nudSpinnerWidth/2
	upCy := b.Min.Y + (midY-b.Min.Y)/2
	downCy := midY + (b.Max.Y-midY)/2
	n.drawTriangle(ctx, cx, upCy, true, n.TextColor)
	n.drawTriangle(ctx, cx, downCy, false, n.TextColor)

	n.drawChildren(ctx)
	n.drawDisabledOverlay(ctx)
}

// drawTriangle рисует маленький треугольник вверх (up=true) или вниз.
func (n *NumericUpDown) drawTriangle(ctx DrawContext, cx, cy int, up bool, col color.RGBA) {
	const h = 3 // высота треугольника
	for dy := 0; dy <= h; dy++ {
		// y идёт сверху вниз; w — полуширина строки.
		y := cy - h/2 + dy
		var w int
		if up {
			// Вершина сверху (узко), основание снизу (широко).
			w = dy
		} else {
			// Основание сверху (широко), вершина снизу (узко).
			w = h - dy
		}
		ctx.FillRect(cx-w, y, 2*w+1, 1, col)
	}
}

// ─── Mouse ────────────────────────────────────────────────────────────────────

func (n *NumericUpDown) OnMouseMove(x, y int) {
	if !n.IsEnabled() {
		n.upHovered, n.downHovered = false, false
		return
	}
	b := n.bounds
	spinX := b.Max.X - nudSpinnerWidth
	midY := b.Min.Y + b.Dy()/2
	in := image.Pt(x, y).In(b)
	n.upHovered = in && x >= spinX && y < midY
	n.downHovered = in && x >= spinX && y >= midY
}

func (n *NumericUpDown) OnMouseButton(e MouseEvent) bool {
	if !n.IsEnabled() {
		return false
	}
	b := n.bounds

	// Колесо мыши.
	if e.Button == MouseWheelUp {
		n.step(+1)
		return true
	}
	if e.Button == MouseWheelDown {
		n.step(-1)
		return true
	}

	if e.Button != MouseLeft || !e.Pressed {
		return false
	}
	if !image.Pt(e.X, e.Y).In(b) {
		return false
	}

	spinX := b.Max.X - nudSpinnerWidth
	midY := b.Min.Y + b.Dy()/2

	if e.X >= spinX {
		// Клик по спиннеру.
		if e.Y < midY {
			n.step(+1)
		} else {
			n.step(-1)
		}
		return true
	}

	// Клик в поле — переходим в режим редактирования с текущим значением.
	n.mu.Lock()
	if !n.isEditing {
		n.isEditing = true
		n.editing = n.format()
	}
	n.mu.Unlock()
	return true
}

// ─── Focusable ──────────────────────────────────────────────────────────────

func (n *NumericUpDown) SetFocused(v bool) {
	var i int32
	if v {
		i = 1
	}
	atomic.StoreInt32(&n.focused, i)
	if !v {
		n.commit() // фиксируем ввод при потере фокуса
	}
}

func (n *NumericUpDown) IsFocused() bool {
	return atomic.LoadInt32(&n.focused) == 1
}

// ─── KeyHandler ──────────────────────────────────────────────────────────────

func (n *NumericUpDown) OnKeyEvent(e KeyEvent) {
	if !n.IsEnabled() || !e.Pressed {
		return
	}
	switch e.Code {
	case KeyUp:
		n.step(+1)
		return
	case KeyDown:
		n.step(-1)
		return
	case KeyEnter:
		n.commit()
		return
	case KeyBackspace:
		n.mu.Lock()
		if !n.isEditing {
			n.isEditing = true
			n.editing = n.format()
		}
		if len(n.editing) > 0 {
			r := []rune(n.editing)
			n.editing = string(r[:len(r)-1])
		}
		n.mu.Unlock()
		return
	}

	// Ввод цифр / точки / минуса.
	if e.Rune == '-' || e.Rune == '.' || (e.Rune >= '0' && e.Rune <= '9') {
		n.mu.Lock()
		if !n.isEditing {
			n.isEditing = true
			n.editing = ""
		}
		// '-' допустим только в начале; '.' — единожды.
		ok := true
		if e.Rune == '-' && len(n.editing) != 0 {
			ok = false
		}
		if e.Rune == '.' && strings.ContainsRune(n.editing, '.') {
			ok = false
		}
		if ok {
			n.editing += string(e.Rune)
		}
		n.mu.Unlock()
	}
}

// Cursor — I-beam над текстовой частью, стрелка над спиннером (CursorProvider).
func (n *NumericUpDown) Cursor(x, y int) Cursor {
	b := n.Bounds()
	if x >= b.Max.X-nudSpinnerWidth {
		return CursorArrow
	}
	return CursorIBeam
}

// ApplyTheme обновляет цвета.
func (n *NumericUpDown) ApplyTheme(t *Theme) {
	n.TextColor = t.InputText
	n.FieldBG = t.InputBG
	n.Border = t.InputBorder
	n.AccentBG = t.Accent
	n.HoverBG = t.CheckHoverBG
}
