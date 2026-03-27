package widget

import (
	"image/color"
	"math"
	"sync/atomic"
)

// ProgressBar — горизонтальный прогресс-бар в стиле Windows 10 Dark.
// Значение [0.0, 1.0] задаётся атомарно через SetValue.
type ProgressBar struct {
	Base

	Background  color.RGBA
	FillColor   color.RGBA
	BorderColor color.RGBA
	ShowBorder  bool

	// value хранится как uint32: 0 = 0.0, math.MaxUint32 = 1.0
	value atomic.Uint32
}

// NewProgressBar создаёт прогресс-бар с цветами Windows 10 Dark.
func NewProgressBar() *ProgressBar {
	return &ProgressBar{
		Background:  win10.ProgressBG,
		FillColor:   win10.ProgressFill,
		BorderColor: win10.Border,
		ShowBorder:  true,
	}
}

// NewProgressBarColor создаёт прогресс-бар с произвольным цветом заливки.
func NewProgressBarColor(fill color.RGBA) *ProgressBar {
	pb := NewProgressBar()
	pb.FillColor = fill
	return pb
}

// SetValue задаёт значение [0.0, 1.0]. Потокобезопасно.
func (pb *ProgressBar) SetValue(v float64) {
	v = max01(v)
	pb.value.Store(uint32(math.Round(v * float64(math.MaxUint32))))
}

// Value возвращает текущее значение [0.0, 1.0]. Потокобезопасно.
func (pb *ProgressBar) Value() float64 {
	return float64(pb.value.Load()) / float64(math.MaxUint32)
}

func (pb *ProgressBar) Draw(ctx DrawContext) {
	b := pb.bounds
	v := pb.Value()

	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), pb.Background)

	fillW := int(math.Round(float64(b.Dx()) * v))
	if fillW > 0 {
		ctx.FillRect(b.Min.X, b.Min.Y, fillW, b.Dy(), pb.FillColor)
	}

	if pb.ShowBorder {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), pb.BorderColor)
	}

	pb.drawChildren(ctx)
}

// ApplyTheme обновляет цвета прогресс-бара.
func (pb *ProgressBar) ApplyTheme(t *Theme) {
	pb.Background = t.ProgressBG
	pb.FillColor = t.ProgressFill
	pb.BorderColor = t.Border
}

func max01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
