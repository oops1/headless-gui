// scrollbar_thin.go — тонкая полоса прокрутки ScrollView с автоскрытием.
//
// Обычная полоса ScrollView — фиксированные 10 px, всегда на виду, и она отнимает
// ширину у содержимого. Тонкая — как в современных оболочках: лежит поверх
// содержимого (ничего у него не отнимает), в покое скрыта, появляется при
// движении мыши над областью и при прокрутке (колесо, перетаскивание, клавиши,
// программный ScrollBy), через паузу плавно гаснет. Под курсором, наведённым на
// саму полосу, она расширяется и не гаснет, пока курсор на ней или идёт
// перетаскивание.
//
// Включение — SetScrollbarStyle(ScrollbarThin) либо флаг темы (ThemeStyle.
// ScrollbarThin, токен профиля scrollbar.thin). Ширина и цвета берутся из
// темы: ThemeStyle.ScrollbarThinWidth / ScrollbarThinHoverWidth и обычные
// цвета полосы (ScrollThumbBG, Accent, ScrollTrackBG); явные значения экземпляра
// (SetThinScrollbar, SetThinColors) главнее. По умолчанию ScrollView остаётся
// прежним: полоса 10 px, всегда видна.
//
// Анимация — на часах движка (Animate*): ни горутин, ни таймеров ОС. Пока полоса
// скрыта и ничего не происходит, анимаций нет и кадры не готовятся; каждый шаг
// перерисовывает только саму полосу (узкие полосы у правого и нижнего краёв), а
// не всё содержимое.
package widget

import (
	"image"
	"image/color"
	"sync"
	"sync/atomic"
	"time"
)

// ScrollbarStyle — вид полосы прокрутки ScrollView.
type ScrollbarStyle int

const (
	// ScrollbarFixed — прежняя полоса: фиксированная ширина, всегда видна,
	// отнимает ширину у содержимого. Значение по умолчанию.
	ScrollbarFixed ScrollbarStyle = iota
	// ScrollbarThin — тонкая полоса поверх содержимого с автоскрытием.
	ScrollbarThin
)

// Умолчания тонкой полосы. Ширину и цвета задаёт тема; это значения на случай,
// когда она молчит.
const (
	// defaultThinScrollbarWidth — ширина тонкой полосы в покое.
	defaultThinScrollbarWidth = 4
	// defaultThinHoverFactor — во сколько раз полоса шире под курсором, если
	// ширину под курсором тема не задала.
	defaultThinHoverFactor = 2
	// defaultAutoHideDelay — пауза без активности до начала затухания.
	defaultAutoHideDelay = 1200 * time.Millisecond
	// defaultFadeDuration — длительность появления и затухания.
	defaultFadeDuration = 200 * time.Millisecond
	// expandDuration — длительность расширения полосы под курсором.
	expandDuration = 120 * time.Millisecond
	// thinTrackAlpha — доля непрозрачности трека (подложки полосы) при
	// расширении: трек лишь намечает дорожку, а не закрашивает её.
	thinTrackAlpha = 0.5
	// thinMinVisible — прозрачность, ниже которой полоса не ловит мышь:
	// невидимая полоса не должна перехватывать нажатия у содержимого.
	thinMinVisible = 0.05
)

// thinBar — состояние показа тонкой полосы. Свой замок: им пользуются тики
// анимации (часы движка) и события мыши, а замок ScrollView держится вокруг
// геометрии. Порядок захвата один: sv.mu → thin.mu; обратного нет.
type thinBar struct {
	mu sync.Mutex

	alpha  float64 // прозрачность полосы [0,1]
	expand float64 // расширение под курсором [0,1]

	showing    bool // цель затухания: полоса должна быть видна
	expanded   bool // цель расширения: курсор на полосе или идёт перетаскивание
	hold       bool // не гасить: курсор на полосе или идёт перетаскивание
	timerArmed bool // таймер скрытия уже заведён (один на серию событий)
	lastPoke   time.Time

	delay, fade time.Duration // настройки; 0 — умолчание

	// stripW/horiz — ширина колонки полосы и наличие горизонтальной, как их
	// видел последний кадр. Нужны, чтобы тик анимации заявлял перерисовку, не
	// беря замок ScrollView (тик может прийти, пока тот занят).
	stripW atomic.Int32
	horiz  atomic.Bool
}

// snapshot отдаёт прозрачность и расширение для кадра.
func (t *thinBar) snapshot() (alpha, expand float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.alpha, t.expand
}

// visible — полоса достаточно видна, чтобы ловить мышь.
func (t *thinBar) visible() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.alpha > thinMinVisible
}

func (t *thinBar) delayLocked() time.Duration {
	if t.delay > 0 {
		return t.delay
	}
	return defaultAutoHideDelay
}

func (t *thinBar) fadeLocked() time.Duration {
	if t.fade > 0 {
		return t.fade
	}
	return defaultFadeDuration
}

// ─── Включение и настройка ──────────────────────────────────────────────────

// SetScrollbarStyle выбирает вид полосы. Явный выбор главнее темы: тема,
// применённая позже, его не меняет.
func (sv *ScrollView) SetScrollbarStyle(s ScrollbarStyle) {
	sv.mu.Lock()
	changed := sv.sbStyle != s
	sv.sbStyle = s
	sv.sbStyleSet = true
	sv.mu.Unlock()
	if changed {
		sv.Invalidate()
	}
}

// ScrollbarStyle возвращает вид полосы.
func (sv *ScrollView) ScrollbarStyle() ScrollbarStyle {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	return sv.sbStyle
}

// SetScrollbarWidth задаёт ширину обычной (ScrollbarFixed) полосы. Явное
// значение главнее темы. Ноль и меньше — вернуть умолчание (10 px) и снова
// слушать тему.
func (sv *ScrollView) SetScrollbarWidth(w int) {
	sv.mu.Lock()
	changed := false
	if w <= 0 {
		changed = sv.scrollbarWidth != defaultScrollbarWidth
		sv.scrollbarWidth, sv.sbWidthSet = defaultScrollbarWidth, false
	} else {
		changed = sv.scrollbarWidth != w
		sv.scrollbarWidth, sv.sbWidthSet = w, true
	}
	sv.mu.Unlock()
	if changed {
		sv.Invalidate()
	}
}

// SetThinScrollbar задаёт ширину тонкой полосы: idle — в покое, hover — под
// курсором (и при перетаскивании). Ноль — значение темы либо умолчание.
func (sv *ScrollView) SetThinScrollbar(idle, hover int) {
	sv.mu.Lock()
	sv.thinIdle, sv.thinHover = idle, hover
	sv.mu.Unlock()
	sv.Invalidate()
}

// SetThinColors задаёт цвета тонкой полосы: ползунок, ползунок под курсором и
// дорожка. Нулевой цвет (A=0) — взять обычный цвет полосы (ThumbColor,
// ThumbHoverBG, TrackColor), то есть цвета темы.
func (sv *ScrollView) SetThinColors(thumb, thumbHover, track color.RGBA) {
	sv.mu.Lock()
	sv.thinThumb, sv.thinThumbHover, sv.thinTrack = thumb, thumbHover, track
	sv.mu.Unlock()
	sv.Invalidate()
}

// SetAutoHide задаёт паузу до скрытия тонкой полосы и длительность появления и
// затухания. Ноль — умолчание (1,2 с и 200 мс). Длительность затухания,
// равная нулю в теме без анимации, не выбирается: для мгновенного скрытия
// передайте fade = 1 нс.
func (sv *ScrollView) SetAutoHide(delay, fade time.Duration) {
	sv.thin.mu.Lock()
	sv.thin.delay, sv.thin.fade = delay, fade
	sv.thin.mu.Unlock()
}

// isThin — выбрана ли тонкая полоса. Под sv.mu.
func (sv *ScrollView) isThin() bool { return sv.sbStyle == ScrollbarThin }

// thinWidths возвращает ширину тонкой полосы в покое и под курсором. Под sv.mu.
func (sv *ScrollView) thinWidths() (idle, hover int) {
	idle = sv.thinIdle
	if idle <= 0 {
		idle = sv.themeThinIdle
	}
	if idle <= 0 {
		idle = defaultThinScrollbarWidth
	}
	hover = sv.thinHover
	if hover <= 0 {
		hover = sv.themeThinHover
	}
	if hover <= 0 {
		hover = idle * defaultThinHoverFactor
	}
	if hover < idle {
		hover = idle
	}
	return idle, hover
}

// sbW — ширина области полосы (трека): у обычной — её ширина, у тонкой — самая
// большая (под курсором): по ней считаются геометрия и попадание мыши. Под sv.mu.
func (sv *ScrollView) sbW() int {
	if sv.isThin() {
		_, hover := sv.thinWidths()
		return hover
	}
	return sv.scrollbarWidth
}

// reserve — сколько полоса отнимает у содержимого. Тонкая лежит поверх и не
// отнимает ничего. Под sv.mu.
func (sv *ScrollView) reserve() int {
	if sv.isThin() {
		return 0
	}
	return sv.scrollbarWidth
}

// workArea — верх и высота рабочей зоны вертикальной полосы: у тонкой — вся
// высота (кнопок ▲▼ классики нет), у обычной — как у sbWorkArea. Под sv.mu.
func (sv *ScrollView) workArea(vb image.Rectangle) (top, h int) {
	if sv.isThin() {
		return vb.Min.Y, vb.Dy()
	}
	return sbWorkArea(vb, sv.scrollbarWidth)
}

// interactive — полоса может ловить мышь: обычная — всегда, тонкая — пока видна.
// Под sv.mu.
func (sv *ScrollView) interactive() bool {
	return !sv.isThin() || sv.thin.visible()
}

// ─── Показ и скрытие ────────────────────────────────────────────────────────

// thinClock — часы активности полосы; в тестах подменяются. Таймер скрытия идёт
// на часах движка (анимация), а «сколько прошло с последнего движения» меряют
// эти.
var thinClock = time.Now

// animateQuiet — AnimateOwned без полной инвалидации при заведении.
//
// Animate/AnimateOwned будят движок полной перерисовкой (notifyUIChanged →
// Engine.Invalidate), и это верно для анимации, которая что-то меняет на весь
// экран. Здесь анимация меняет только узкую полосу и сама заявляет её
// область (thinInvalidate), а таймер скрытия не меняет ничего: полная
// перерисовка кадра на каждое движение мыши над прокруткой была бы расточительна.
// Реестр и семантика (owner,tag) — те же; шагает анимацию тот же цикл движка.
func animateQuiet(owner any, tag string, dur time.Duration, curve Easing, tick func(t float64)) *Animation {
	a := &Animation{duration: dur, curve: curve, tick: tick, owner: owner, tag: tag}
	anim.mu.Lock()
	for _, other := range anim.active {
		if !other.stopped && !other.done && other.owner == owner && other.tag == tag {
			other.stopped = true
		}
	}
	a.loop = currentAnimLoop()
	anim.active = append(anim.active, a)
	anim.mu.Unlock()
	return a
}

// thinPoke показывает тонкую полосу и откладывает её скрытие: зовётся на
// движение мыши над областью и на любую прокрутку. Можно звать из любого места,
// в том числе под sv.mu (берёт только свой замок). У обычной полосы и у
// содержимого, которое помещается, ничего не делает.
//
// Дёшево: серия событий лишь обновляет метку времени. Новая анимация заводится
// только при переходе «скрыта → видна» и при заведении таймера скрытия (один на
// серию); таймер, сработав раньше времени, переносится на остаток паузы.
func (sv *ScrollView) thinPoke() {
	if sv.sbStyle != ScrollbarThin || !sv.hasBars() {
		return
	}
	t := &sv.thin
	now := thinClock()
	t.mu.Lock()
	t.lastPoke = now
	needFade := !t.showing
	t.showing = true
	arm := !t.timerArmed
	if arm {
		t.timerArmed = true
	}
	from, dur := t.alpha, time.Duration(float64(t.fadeLocked())*(1-t.alpha))
	delay := t.delayLocked()
	t.mu.Unlock()

	if needFade {
		sv.thinFadeTo(from, 1, dur)
	}
	if arm {
		sv.thinArm(delay)
	}
}

// hasBars — есть ли что показывать (содержимое не помещается).
func (sv *ScrollView) hasBars() bool {
	b := sv.Bounds()
	return sv.ContentHeight > b.Dy() || (sv.ContentWidth > 0 && sv.ContentWidth > b.Dx())
}

// thinFadeTo ведёт прозрачность от from к to за dur.
func (sv *ScrollView) thinFadeTo(from, to float64, dur time.Duration) {
	t := &sv.thin
	if dur <= 0 {
		t.mu.Lock()
		t.alpha = to
		t.mu.Unlock()
		sv.thinInvalidate()
		return
	}
	animateQuiet(sv, "sb-fade", MotionDur(dur), EaseOutQuad, func(p float64) {
		t.mu.Lock()
		t.alpha = LerpF(from, to, p)
		t.mu.Unlock()
		sv.thinInvalidate()
	})
	sv.thinInvalidate() // разбудить цикл: первый шаг нарисует начало перехода
}

// thinArm заводит таймер скрытия на d; предыдущий заменяется (тот же тег).
func (sv *ScrollView) thinArm(d time.Duration) {
	a := animateQuiet(sv, "sb-hide", d, nil, nil)
	a.OnDone = sv.thinHideDue
}

// thinHideDue — таймер сработал: гасим полосу, если с последнего движения
// прошла вся пауза и курсор не на полосе; иначе переносим таймер на остаток.
func (sv *ScrollView) thinHideDue() {
	t := &sv.thin
	now := thinClock()
	t.mu.Lock()
	wait := t.delayLocked() // курсор на полосе или тянут ползунок — ждём полную паузу
	if !t.hold {
		wait = t.delayLocked() - now.Sub(t.lastPoke)
	}
	if wait > 0 {
		t.mu.Unlock()
		sv.thinArm(wait)
		return
	}
	t.timerArmed = false
	t.showing = false
	from, dur := t.alpha, time.Duration(float64(t.fadeLocked())*t.alpha)
	t.mu.Unlock()
	sv.thinFadeTo(from, 0, dur)
}

// thinHold отмечает, что курсор на полосе или идёт перетаскивание: полоса
// расширяется и не гаснет. Отпускание возвращает обычный отсчёт паузы.
func (sv *ScrollView) thinHold(on bool) {
	if sv.sbStyle != ScrollbarThin {
		return
	}
	t := &sv.thin
	t.mu.Lock()
	if t.hold == on {
		t.mu.Unlock()
		return
	}
	t.hold = on
	t.expanded = on
	from, to := t.expand, 0.0
	if on {
		to = 1
	}
	t.mu.Unlock()

	animateQuiet(sv, "sb-expand", MotionDur(time.Duration(float64(expandDuration)*absF(to-from))), EaseOutQuad, func(p float64) {
		t.mu.Lock()
		t.expand = LerpF(from, to, p)
		t.mu.Unlock()
		sv.thinInvalidate()
	})
	sv.thinInvalidate()
	if !on {
		sv.thinPoke()
	}
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// thinInvalidate перерисовывает только полосы: правую и (если есть) нижнюю
// колонки, а не всю прокрутку — содержимое под ними кадр не пересчитывает.
func (sv *ScrollView) thinInvalidate() {
	b := sv.Bounds()
	if b.Empty() {
		return
	}
	w := int(sv.thin.stripW.Load())
	if w <= 0 {
		w = defaultThinScrollbarWidth * defaultThinHoverFactor
	}
	horiz := sv.thin.horiz.Load() || sv.ContentWidth > 0
	sv.invalidateRect(image.Rect(b.Max.X-w, b.Min.Y, b.Max.X, b.Max.Y))
	if horiz {
		sv.invalidateRect(image.Rect(b.Min.X, b.Max.Y-w, b.Max.X, b.Max.Y))
	}
}

// ─── Отрисовка ──────────────────────────────────────────────────────────────

// scaleAlpha ослабляет цвет в k раз. Цвета предумножены на альфу, поэтому
// множатся все четыре канала.
func scaleAlpha(c color.RGBA, k float64) color.RGBA {
	if k >= 1 {
		return c
	}
	if k <= 0 {
		return color.RGBA{}
	}
	return color.RGBA{
		R: uint8(float64(c.R) * k), G: uint8(float64(c.G) * k),
		B: uint8(float64(c.B) * k), A: uint8(float64(c.A) * k),
	}
}

// thinColors возвращает цвета ползунка, ползунка под курсором и дорожки.
func (sv *ScrollView) thinColors() (thumb, hover, track color.RGBA) {
	thumb, hover, track = sv.thinThumb, sv.thinThumbHover, sv.thinTrack
	if thumb.A == 0 {
		thumb = sv.ThumbColor
	}
	if hover.A == 0 {
		hover = sv.ThumbHoverBG
	}
	if track.A == 0 {
		track = sv.TrackColor
	}
	return thumb, hover, track
}

// drawThinBars рисует тонкие полосы поверх содержимого. Вызывается из Draw
// после детей, без замка: все геометрические величины переданы снимком.
func (sv *ScrollView) drawThinBars(ctx DrawContext, vert, horiz bool, vbar, thumb, hstrip, hthumb image.Rectangle,
	vActive, hActive bool, idleW, hoverW int, thumbCol, hoverCol, trackCol color.RGBA) {

	sv.thin.stripW.Store(int32(hoverW))
	sv.thin.horiz.Store(horiz)
	alpha, expand := sv.thin.snapshot()
	if alpha <= 0 {
		return
	}
	// Ширина плавно растёт от покоя к расширенной.
	w := idleW + int(float64(hoverW-idleW)*expand+0.5)
	trackA := alpha * expand * thinTrackAlpha

	if vert {
		if expand > 0 {
			t := scaleAlpha(trackCol, trackA)
			ctx.FillRectAlpha(vbar.Max.X-w, vbar.Min.Y, w, vbar.Dy(), t)
		}
		col := thumbCol
		if vActive {
			col = hoverCol
		}
		x := thumb.Max.X - w
		ctx.FillRoundRect(x, thumb.Min.Y, w, thumb.Dy(), w/2, scaleAlpha(col, alpha))
	}
	if horiz {
		if expand > 0 {
			t := scaleAlpha(trackCol, trackA)
			ctx.FillRectAlpha(hstrip.Min.X, hstrip.Max.Y-w, hstrip.Dx(), w, t)
		}
		col := thumbCol
		if hActive {
			col = hoverCol
		}
		y := hstrip.Max.Y - w
		ctx.FillRoundRect(hthumb.Min.X, y, hthumb.Dx(), w, w/2, scaleAlpha(col, alpha))
	}
}

// defaultScrollbarWidth — ширина обычной полосы ScrollView по умолчанию.
const defaultScrollbarWidth = 10

// applyScrollbarTheme переносит в ScrollView вид и ширину полосы из темы.
// Явно заданное экземпляром (SetScrollbarStyle, SetScrollbarWidth) главнее. Тема
// без этих полей (все пресеты) даёт прежнее: фиксированная полоса 10 px.
func (sv *ScrollView) applyScrollbarTheme(t *Theme) {
	sv.mu.Lock()
	if !sv.sbStyleSet {
		if t.Style.ScrollbarThin {
			sv.sbStyle = ScrollbarThin
		} else {
			sv.sbStyle = ScrollbarFixed
		}
	}
	if !sv.sbWidthSet {
		switch {
		case t.Style.ScrollbarWidth > 0:
			sv.scrollbarWidth, sv.sbWidthFromTheme = t.Style.ScrollbarWidth, true
		case sv.sbWidthFromTheme:
			sv.scrollbarWidth, sv.sbWidthFromTheme = defaultScrollbarWidth, false
		}
	}
	sv.themeThinIdle, sv.themeThinHover = t.Style.ScrollbarThinWidth, t.Style.ScrollbarThinHoverWidth
	sv.mu.Unlock()
}
