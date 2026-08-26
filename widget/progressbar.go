package widget

import (
	"image"
	"image/color"
	"math"
	"sync/atomic"
	"time"
)

// ProgressBar — горизонтальный прогресс-бар в стиле Windows 10 Dark.
// Значение [0.0, 1.0] задаётся атомарно через SetValue.
type ProgressBar struct {
	Base

	Background  color.RGBA
	FillColor   color.RGBA
	BorderColor color.RGBA
	ShowBorder  bool

	// Style — манера отрисовки: штатная полоса темы или светящаяся голова
	// со следом (см. progressbar_glow.go).
	Style ProgressBarStyle

	// GlowTail / GlowHead — концы градиента при ProgressStyleGlow
	// (A=0 → из темы, а если и там не заданы — от FillColor).
	GlowTail color.RGBA
	GlowHead color.RGBA

	// glowAnim — зацикленная анимация, пока светящаяся полоса на экране;
	// lastDrawn — момент последней отрисовки, по нему она себя снимает.
	glowAnim  *Animation
	lastDrawn atomic.Int64

	// value хранится как uint32: 0 = 0.0, math.MaxUint32 = 1.0
	value atomic.Uint32

	// indeterminate — режим бегущей полосы (значение неизвестно). Пока
	// включён, виджет анимируется (NeedsAnimation → движок не пропускает кадры).
	indeterminate atomic.Bool
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

// SetValue задаёт значение [0.0, 1.0] МГНОВЕННО. Потокобезопасно.
// При фактическом изменении инвалидирует область виджета (авто-damage).
// Остаётся синхронным ради обратной совместимости и тестов — для плавного
// перехода используйте AnimateValue.
func (pb *ProgressBar) SetValue(v float64) {
	v = max01(v)
	nv := uint32(math.Round(v * float64(math.MaxUint32)))
	if pb.value.Swap(nv) != nv {
		pb.Invalidate()
	}
}

// Value возвращает текущее значение [0.0, 1.0]. Потокобезопасно.
func (pb *ProgressBar) Value() float64 {
	return float64(pb.value.Load()) / float64(math.MaxUint32)
}

// progressAnimDur — длительность плавного перехода AnimateValue.
const progressAnimDur = 200 * time.Millisecond

// AnimateValue плавно анимирует отображаемое значение к newValue за ~200ms
// (EaseOutCubic). В теме Classic3D — как и SetValue, мгновенно (анимации в
// классике отключены целиком). Использует AnimateOwned(pb, "value", ...):
// повторный вызов до завершения предыдущей анимации останавливает её и
// продолжает лерп от ТЕКУЩЕГО отображаемого значения — без "дёргания".
func (pb *ProgressBar) AnimateValue(newValue float64) {
	newValue = max01(newValue)
	if currentStyle().Classic3D {
		pb.SetValue(newValue)
		return
	}
	from := pb.Value()
	AnimateOwned(pb, "value", progressAnimDur, EaseOutCubic, func(t float64) {
		pb.SetValue(LerpF(from, newValue, t))
	})
}

// SetIndeterminate включает/выключает режим бегущей полосы (потокобезопасно).
func (pb *ProgressBar) SetIndeterminate(on bool) {
	if pb.indeterminate.Swap(on) != on {
		pb.Invalidate()
	}
}

// IsIndeterminate сообщает, включён ли режим бегущей полосы.
func (pb *ProgressBar) IsIndeterminate() bool { return pb.indeterminate.Load() }

// NeedsAnimation реализует widget.Animated: пока индикатор неопределённый,
// движок не пропускает кадры (полоса едет). Работает, только если виджет
// в фокусе либо принудительно; для диалога прогресса достаточно
// периодической перерисовки от SetProgress/SetStatus.
func (pb *ProgressBar) NeedsAnimation() bool {
	return pb.indeterminate.Load() || pb.glowEnabled()
}

// indetOffset вычисляет позицию бегунка от времени (без хранения состояния).
func indetOffset(trackW int) int {
	const period = 1200 * time.Millisecond
	blockW := trackW / 3
	span := trackW + blockW
	phase := float64(time.Now().UnixNano()%int64(period)) / float64(period)
	return int(phase*float64(span)) - blockW
}

func (pb *ProgressBar) Draw(ctx DrawContext) {
	b := pb.Bounds()
	if b.Empty() {
		return
	}
	v := pb.Value()

	if pb.glowEnabled() {
		pb.drawGlow(ctx, b, v)
		pb.drawChildren(ctx)
		return
	}
	pb.drawThemed(ctx, b, v)
	pb.drawChildren(ctx)
}

// drawThemed рисует штатную для темы полосу: классика Win2000 — утопленная
// дорожка с блоками, Mac/Win11 — скруглённая пилюля, Win10 — прямоугольник.
func (pb *ProgressBar) drawThemed(ctx DrawContext, b image.Rectangle, v float64) {
	st := currentStyle()
	if pb.indeterminate.Load() {
		// Бегущая полоса живёт от кадра к кадру, а фокуса у неё нет (диалог
		// ожидания) — держим кадры той же зациклённой анимацией, что и
		// свечение. Она снимет себя, когда полосу перестанут рисовать.
		pb.markDrawn()
		pb.ensureGlowAnim()
	}

	// Неопределённый режим: бегущий блок ~1/3 ширины (кроме классики Win2000,
	// где indeterminate рисуется как заполнение блоками).
	if pb.indeterminate.Load() && !st.Classic3D {
		cr := 0
		if st.ControlCorner > 0 {
			cr = st.ControlCorner
			if cr > b.Dy()/2 {
				cr = b.Dy() / 2
			}
			ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, pb.Background)
		} else {
			ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), pb.Background)
		}
		trackW := b.Dx()
		blockW := trackW / 3
		off := indetOffset(trackW)
		x0 := b.Min.X + off
		x1 := x0 + blockW
		if x0 < b.Min.X {
			x0 = b.Min.X
		}
		if x1 > b.Max.X {
			x1 = b.Max.X
		}
		if x1 > x0 {
			if cr > 0 {
				ctx.FillRoundRect(x0, b.Min.Y, x1-x0, b.Dy(), cr, pb.FillColor)
			} else {
				ctx.FillRect(x0, b.Min.Y, x1-x0, b.Dy(), pb.FillColor)
			}
		}
		if pb.ShowBorder {
			if cr > 0 {
				ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, pb.BorderColor)
			} else {
				ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), pb.BorderColor)
			}
		}
		return
	}

	switch {
	case st.Classic3D:
		// Классика: утопленная дорожка, заполнение «блоками» Win2000.
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), pb.Background)
		drawBevelSunken(ctx, b.Min.X, b.Min.Y, b.Dx(), b.Dy(), st)
		innerW := b.Dx() - 6
		blockW, gap := 8, 2
		if pb.indeterminate.Load() {
			// Marquee Win2000: группа блоков ползёт по дорожке и уходит за
			// край. Без этой ветки классика в неопределённом режиме просто
			// показывала бы пустую дорожку.
			const group = 5
			step := blockW + gap
			span := innerW + group*step
			off := int(marqueePhase()*float64(span)) - group*step
			for i := 0; i < group; i++ {
				x := off + i*step
				if x < 0 || x+blockW > innerW {
					continue
				}
				ctx.FillRect(b.Min.X+3+x, b.Min.Y+3, blockW, b.Dy()-6, pb.FillColor)
			}
			break
		}
		fillW := int(math.Round(float64(innerW) * v))
		for x := 0; x+blockW <= fillW; x += blockW + gap {
			ctx.FillRect(b.Min.X+3+x, b.Min.Y+3, blockW, b.Dy()-6, pb.FillColor)
		}
	case st.ControlCorner > 0:
		cr := st.ControlCorner
		if cr > b.Dy()/2 {
			cr = b.Dy() / 2
		}
		ctx.FillRoundRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, pb.Background)
		fillW := int(math.Round(float64(b.Dx()) * v))
		if fillW > cr*2 {
			ctx.FillRoundRect(b.Min.X, b.Min.Y, fillW, b.Dy(), cr, pb.FillColor)
		} else if fillW > 0 {
			ctx.FillRect(b.Min.X+1, b.Min.Y+1, fillW, b.Dy()-2, pb.FillColor)
		}
		if pb.ShowBorder {
			ctx.DrawRoundBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), cr, pb.BorderColor)
		}
	default:
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), pb.Background)
		fillW := int(math.Round(float64(b.Dx()) * v))
		if fillW > 0 {
			ctx.FillRect(b.Min.X, b.Min.Y, fillW, b.Dy(), pb.FillColor)
		}
		if pb.ShowBorder {
			ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), pb.BorderColor)
		}
	}
}

// ApplyTheme обновляет цвета прогресс-бара.
func (pb *ProgressBar) ApplyTheme(t *Theme) {
	pb.Background = t.ProgressBG
	pb.FillColor = t.ProgressFill
	pb.GlowTail = color.RGBA{}
	pb.GlowHead = color.RGBA{}
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
