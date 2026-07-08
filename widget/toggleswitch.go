package widget

import (
	"image"
	"image/color"
	"math"
	"sync/atomic"
	"time"
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

	on       int32 // 0 | 1
	hovered  int32 // 0 | 1
	focused  int32 // 0 | 1
	pressing int32 // 1 — press был на этом виджете

	// knobPos — визуальная позиция ручки в [0,1] (0=выкл, 1=вкл), хранится как
	// uint32 (0 = 0.0, math.MaxUint32 = 1.0) по образцу ProgressBar.value.
	// Используется в Draw ТОЛЬКО пока анимация реально тикала (см. animating)
	// — иначе Draw опирается на каноническое IsOn(), чтобы поведение без
	// работающего движка (тесты, вызовы SetOn "в лоб", т.е. без единого
	// StepAnimations) совпадало с прежним: движок анимации ленивый — часы
	// стартуют только на ПЕРВОМ StepAnimations, поэтому между регистрацией
	// AnimateOwned и первым тиком knobPos ещё не сдвинут никуда.
	knobPos atomic.Uint32
	// animating — установлен true, только когда тик анимации ручки РЕАЛЬНО
	// вызвался хотя бы раз (см. tick в animateKnobTo). Пока движок не тикнул
	// ни разу, Draw не должен доверять knobPos — он ещё содержит "from", а
	// не текущий прогресс. Снимается по завершении анимации (t>=1.0) или при
	// мгновенном Classic3D-переходе (там анимаций не будет вовсе).
	animating atomic.Bool

	OnChange func(on bool)
}

const (
	toggleW      = 44 // ширина капсулы
	toggleH      = 22 // высота капсулы
	toggleThumbR = 7  // радиус кружка
	togglePad    = 4  // отступ кружка от края

	toggleAnimDur = 120 * time.Millisecond // длительность скольжения ручки
)

// SetText задаёт текст метки переключателя.
func (t *ToggleSwitch) SetText(s string) {
	if t.Text == s {
		return
	}
	t.Text = s
	t.Invalidate()
}

// GetText возвращает текущий текст метки.
func (t *ToggleSwitch) GetText() string { return t.Text }

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
// При фактическом изменении инвалидирует область виджета (авто-damage) и
// запускает плавное скольжение ручки к новой позиции (кроме темы Classic3D,
// где переключение мгновенное — аутентичное поведение Win2000).
func (ts *ToggleSwitch) SetOn(v bool) {
	if atomic.SwapInt32(&ts.on, b2i(v)) == b2i(v) {
		return
	}
	ts.Invalidate()
	ts.animateKnobTo(v)
}

// IsOn возвращает текущее состояние.
func (ts *ToggleSwitch) IsOn() bool {
	return atomic.LoadInt32(&ts.on) == 1
}

// setKnobPos потокобезопасно задаёт визуальную позицию ручки [0,1] и
// инвалидирует область виджета. Публичный сеттер — это единственный способ,
// которым тик анимации трогает состояние виджета (без доступа к приватным
// полям напрямую), как и требует контракт AnimateOwned/tick.
func (ts *ToggleSwitch) setKnobPos(v float64) {
	nv := uint32(math.Round(max01(v) * float64(math.MaxUint32)))
	if ts.knobPos.Swap(nv) != nv {
		ts.Invalidate()
	}
}

// knobPosVal возвращает текущую визуальную позицию ручки [0,1].
func (ts *ToggleSwitch) knobPosVal() float64 {
	return float64(ts.knobPos.Load()) / float64(math.MaxUint32)
}

// animateKnobTo запускает (или, в Classic3D, мгновенно завершает) скольжение
// ручки к канонической позиции, соответствующей on.
func (ts *ToggleSwitch) animateKnobTo(on bool) {
	target := 0.0
	if on {
		target = 1.0
	}
	if currentStyle().Classic3D {
		// Классика Win2000: анимации отключены целиком — мгновенный переход.
		ts.animating.Store(false)
		ts.setKnobPos(target)
		return
	}
	// from — текущая ВИЗУАЛЬНАЯ позиция ручки: пока прошлая анимация реально
	// тикала, это knobPos; иначе на экране каноническое положение СТАРОГО
	// состояния (Draw в покое рисует по IsOn, а knobPos может хранить
	// устаревший "from" анимации, не получившей ни одного тика — например,
	// после StopAllAnimations или серии SetOn без работающего движка).
	from := ts.knobPosVal()
	if !ts.animating.Load() {
		from = 1.0 - target // SetOn гарантирует фактическую смену состояния
	}
	// animating взводится ВНУТРИ тика (а не до регистрации анимации): часы
	// анимации ленивые (см. widget/anim.go — старт на первом StepAnimations),
	// поэтому между AnimateOwned и первым реальным тиком должно пройти Draw
	// по-прежнему как канонический IsOn() — иначе тест/вызов "SetOn без
	// работающего движка" увидит устаревший (ещё не сдвинутый) knobPos.
	AnimateOwned(ts, "knob", toggleAnimDur, EaseOutCubic, func(t float64) {
		ts.animating.Store(true)
		ts.setKnobPos(LerpF(from, target, t))
		if t >= 1.0 {
			// Штатное завершение анимации: дальше Draw полагается на
			// каноническое IsOn(), а не на knobPos (см. Draw).
			ts.animating.Store(false)
		}
	})
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
	if b.Empty() {
		return
	}
	capY := b.Min.Y + (b.Dy()-toggleH)/2

	// Фон капсулы
	bg := ts.OffBG
	if ts.IsOn() {
		bg = ts.OnBG
	}
	ctx.FillRoundRect(b.Min.X, capY, toggleW, toggleH, toggleH/2, bg)
	ctx.DrawRoundBorder(b.Min.X, capY, toggleW, toggleH, toggleH/2, ts.BorderCol)

	// Кружок. Пока анимация ручки активна — используем визуальную позицию
	// (интерполированную тиками); иначе рисуем каноническую позицию по
	// IsOn() напрямую — так конечное состояние всегда совпадает с тем, что
	// было бы без анимации вообще (например, если движок не тикает).
	onOff := b.Min.X + togglePad + toggleThumbR
	onOn := b.Min.X + toggleW - togglePad - toggleThumbR
	var cx int
	if ts.animating.Load() {
		cx = LerpInt(onOff, onOn, ts.knobPosVal())
	} else if ts.IsOn() {
		cx = onOn
	} else {
		cx = onOff
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
//
// WPF-совместимое поведение: переключение только если press был
// на этом же ToggleSwitch (защита от «пролётного» release).
func (ts *ToggleSwitch) OnMouseButton(e MouseEvent) bool {
	if !ts.IsEnabled() {
		return false
	}
	if e.Button == MouseLeft {
		if e.Pressed {
			atomic.StoreInt32(&ts.pressing, 1)
			return true
		}
		wasDown := atomic.SwapInt32(&ts.pressing, 0) != 0
		if wasDown {
			newState := !ts.IsOn()
			ts.SetOn(newState)
			if ts.OnChange != nil {
				ts.OnChange(newState)
			}
		}
		return wasDown
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
			ts.OnChange(newState) // синхронно — единая модель с mouse-path
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
