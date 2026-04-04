package widget

import (
	"image"
	"image/color"
	"math"
	"sync"
)

// Slider — горизонтальный ползунок значения в стиле Windows 10.
//
// Значение меняется в диапазоне [Min, Max] (по умолчанию 0.0–1.0).
// Поддерживает drag мышью и клавиатурную навигацию (←/→).
type Slider struct {
	Base

	Min float64
	Max float64

	TrackBG     color.RGBA
	FillColor   color.RGBA
	ThumbColor  color.RGBA
	ThumbHover  color.RGBA
	BorderColor color.RGBA

	TrackHeight int // высота дорожки (по умолчанию 4)
	ThumbRadius int // радиус ползунка (по умолчанию 8)

	mu       sync.Mutex
	value    float64
	dragging bool
	hovered  bool
	focused  bool

	OnChange func(value float64)
}

// NewSlider создаёт ползунок [0.0, 1.0].
func NewSlider() *Slider {
	return &Slider{
		Min:         0,
		Max:         1,
		TrackBG:     win10.SliderTrackBG,
		FillColor:   win10.SliderFill,
		ThumbColor:  win10.SliderThumb,
		ThumbHover:  win10.Accent,
		BorderColor: win10.SliderBorder,
		TrackHeight: 4,
		ThumbRadius: 8,
	}
}

// NewSliderRange создаёт ползунок с заданным диапазоном.
func NewSliderRange(min, max float64) *Slider {
	s := NewSlider()
	s.Min = min
	s.Max = max
	s.value = min
	return s
}

// SetValue задаёт значение с ограничением [Min, Max].
func (s *Slider) SetValue(v float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value = s.clamp(v)
}

// Value возвращает текущее значение.
func (s *Slider) Value() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.value
}

func (s *Slider) clamp(v float64) float64 {
	if v < s.Min {
		return s.Min
	}
	if v > s.Max {
		return s.Max
	}
	return v
}

// ratio возвращает нормализованное значение [0, 1].
func (s *Slider) ratio() float64 {
	rng := s.Max - s.Min
	if rng <= 0 {
		return 0
	}
	return (s.value - s.Min) / rng
}

// trackRect возвращает прямоугольник дорожки.
func (s *Slider) trackRect() image.Rectangle {
	b := s.bounds
	trackY := b.Min.Y + (b.Dy()-s.TrackHeight)/2
	return image.Rect(b.Min.X+s.ThumbRadius, trackY,
		b.Max.X-s.ThumbRadius, trackY+s.TrackHeight)
}

// thumbCenter возвращает центр ползунка.
func (s *Slider) thumbCenter() (int, int) {
	tr := s.trackRect()
	r := s.ratio()
	cx := tr.Min.X + int(math.Round(r*float64(tr.Dx())))
	cy := s.bounds.Min.Y + s.bounds.Dy()/2
	return cx, cy
}

// Draw рисует Slider: дорожка + заливка + ползунок.
func (s *Slider) Draw(ctx DrawContext) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tr := s.trackRect()

	// Дорожка (фон)
	ctx.FillRoundRect(tr.Min.X, tr.Min.Y, tr.Dx(), tr.Dy(), 2, s.TrackBG)

	// Заполненная часть
	fillW := int(math.Round(s.ratio() * float64(tr.Dx())))
	if fillW > 0 {
		ctx.FillRoundRect(tr.Min.X, tr.Min.Y, fillW, tr.Dy(), 2, s.FillColor)
	}

	// Ползунок
	cx, cy := s.thumbCenter()
	thumbCol := s.ThumbColor
	if s.hovered || s.dragging {
		thumbCol = s.ThumbHover
	}
	drawFilledCircle(ctx, cx, cy, s.ThumbRadius, thumbCol)
	drawCircleOutline(ctx, cx, cy, s.ThumbRadius, s.BorderColor)

	// Меньший белый кружок в центре для красоты
	innerR := s.ThumbRadius - 3
	if innerR > 2 {
		drawFilledCircle(ctx, cx, cy, innerR, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	}

	s.drawChildren(ctx)
	s.drawDisabledOverlay(ctx)
}

// valueFromX вычисляет значение по позиции мыши.
func (s *Slider) valueFromX(x int) float64 {
	tr := s.trackRect()
	if tr.Dx() <= 0 {
		return s.Min
	}
	ratio := float64(x-tr.Min.X) / float64(tr.Dx())
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	return s.Min + ratio*(s.Max-s.Min)
}

// OnMouseButton обрабатывает клик — начало/конец перетаскивания.
func (s *Slider) OnMouseButton(e MouseEvent) bool {
	if !s.IsEnabled() {
		return false
	}
	if e.Button != MouseLeft {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if e.Pressed {
		s.dragging = true
		newVal := s.valueFromX(e.X)
		if newVal != s.value {
			s.value = s.clamp(newVal)
			if s.OnChange != nil {
				go s.OnChange(s.value)
			}
		}
		return true
	}

	s.dragging = false
	return true
}

// OnMouseMove обрабатывает drag и hover.
func (s *Slider) OnMouseMove(x, y int) {
	if !s.IsEnabled() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dragging {
		newVal := s.valueFromX(x)
		if newVal != s.value {
			s.value = s.clamp(newVal)
			if s.OnChange != nil {
				go s.OnChange(s.value)
			}
		}
		return
	}

	// Hover на ползунке
	cx, cy := s.thumbCenter()
	dx := x - cx
	dy := y - cy
	s.hovered = dx*dx+dy*dy <= s.ThumbRadius*s.ThumbRadius
}

// OnKeyEvent обрабатывает клавиши ←/→ для изменения значения.
func (s *Slider) OnKeyEvent(e KeyEvent) {
	if !s.IsEnabled() || !e.Pressed {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	step := (s.Max - s.Min) / 20 // 5% шаг
	if e.Mod&ModShift != 0 {
		step = (s.Max - s.Min) / 100 // мелкий шаг
	}

	switch e.Code {
	case KeyLeft:
		s.value = s.clamp(s.value - step)
	case KeyRight:
		s.value = s.clamp(s.value + step)
	case KeyHome:
		s.value = s.Min
	case KeyEnd:
		s.value = s.Max
	default:
		return
	}

	if s.OnChange != nil {
		go s.OnChange(s.value)
	}
}

// SetFocused реализует Focusable.
func (s *Slider) SetFocused(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.focused = v
}

// IsFocused реализует Focusable.
func (s *Slider) IsFocused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.focused
}

// ApplyTheme обновляет цвета Slider.
func (s *Slider) ApplyTheme(t *Theme) {
	s.TrackBG = t.SliderTrackBG
	s.FillColor = t.SliderFill
	s.ThumbColor = t.SliderThumb
	s.ThumbHover = t.Accent
	s.BorderColor = t.SliderBorder
}
