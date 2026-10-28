package engine

// popuptrans.go — переходы хостовых попапов (задача 3.4 плана 4.0):
// появление и исчезновение оверлея, вынесенного в окно ОС (SetPopupSink).
//
// В холсте переход — смешение снимков (overlaytrans.go). У окна-попапа
// пиксели свои: картинка оверлея лежит в кэше (popupEntry.img) на прозрачном
// фоне, и переход — это непрозрачность и сдвиг ОКНА, которые прикладывает
// хост. Движок шлёт тот же Img с PopupFrame.Transition на каждом шаге; DrawOverlay
// на шагах не зовётся, и холст не перерисовывается — шаг доходит только до
// renderPopups (renderFrame, popupSteps).
//
// Исчезающий оверлей остаётся в кадрах (Transition.Closing) до конца
// перехода, с последней картинкой и на прежнем месте; хост закрывает окно,
// когда ID пропадает. Хост, не умеющий прозрачности окна, получает те же
// кадры и показывает окно сразу, а исчезающее закрывает сразу.
//
// Включается двумя выключателями: SetOverlayTransitions (общий для всех
// оверлеев) и SetPopupFade (хост понимает переходы). Длительность, кривая и
// сдвиг — те же токены темы, что у оверлеев в холсте.

import (
	"image"
	"math"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// popupTrans — параметры переходов кадра; dur 0 — переходов нет.
// closeDur/closeCurve — исчезновение (theme.AnimMenuClose или, без него,
// то же, что появление).
type popupTrans struct {
	dur        time.Duration
	curve      widget.Easing
	slide      int // логические точки
	closeDur   time.Duration
	closeCurve widget.Easing
}

// popupTransSpec — параметры переходов попапов для этого кадра.
func (e *Engine) popupTransSpec(canvas *Canvas) popupTrans {
	if !e.popupFade.Load() || !e.ovt.on.Load() {
		return popupTrans{}
	}
	m := e.themeMgr.Load()
	if m == nil {
		return popupTrans{}
	}
	a := m.GetAnimation(theme.AnimMenuOpen)
	if a.Duration <= 0 {
		return popupTrans{}
	}
	c := m.GetAnimationFirst(theme.AnimMenuClose, theme.AnimMenuOpen)
	return popupTrans{
		dur:        a.Duration,
		curve:      widget.EasingByName(a.Curve),
		slide:      int(math.Round(m.GetMetric(theme.KeyMenuOpenOffset))),
		closeDur:   c.Duration,
		closeCurve: widget.EasingByName(c.Curve),
	}
}

func (ent *popupEntry) val() float64     { return math.Float64frombits(ent.tv.Load()) }
func (ent *popupEntry) setVal(v float64) { ent.tv.Store(math.Float64bits(v)) }

// popupOpenStep — открытый оверлей кадра: начинает переход появления (новый
// или исчезавший), заканчивает дошедший до конца.
func (e *Engine) popupOpenStep(ent *popupEntry, it popupItem, fresh bool, tr popupTrans) {
	switch {
	case ent.closing:
		// Открыт снова посреди исчезновения — разворачиваемся с текущей доли.
		ent.closing = false
		if tr.dur <= 0 {
			ent.stopTrans()
			return
		}
		ent.slide = popupSlideDir(it, tr.slide)
		e.popupAnimate(ent, ent.val(), 1, tr)
	case fresh && tr.dur > 0:
		ent.slide = popupSlideDir(it, tr.slide)
		ent.setVal(0)
		e.popupAnimate(ent, 0, 1, tr)
	case ent.trans && ent.tdone.Load():
		ent.stopTrans() // появление закончено: окно в покое
	}
}

// popupCloseStep — оверлей, которого нет в кадре. true — он исчезает
// переходом и остаётся в кадрах; false — уходит сразу.
func (e *Engine) popupCloseStep(ent *popupEntry, tr popupTrans) bool {
	if ent.closing {
		if ent.tdone.Load() {
			return false // исчезновение закончено
		}
		return true
	}
	from := 1.0
	if ent.trans {
		from = ent.val()
	}
	if tr.dur <= 0 || tr.closeDur <= 0 || ent.img == nil || from <= 0 {
		return false
	}
	ent.closing = true
	e.popupAnimate(ent, from, 0, popupTrans{dur: tr.closeDur, curve: tr.closeCurve})
	return true
}

// popupAnimate ведёт долю от from к to за часть длительности, пропорциональную
// пути. Кривая — в тике, к линейному прогрессу: конец — прогресс 1.
func (e *Engine) popupAnimate(ent *popupEntry, from, to float64, tr popupTrans) {
	ent.stopAnim()
	ent.trans = true
	ent.tdone.Store(false)
	d := time.Duration(float64(tr.dur) * math.Abs(to-from))
	if d <= 0 {
		ent.setVal(to)
		ent.tdone.Store(true)
		e.popupWake()
		return
	}
	curve := tr.curve
	ent.anim = widget.AnimateOwned(ent, "popup-trans", d, nil, func(p float64) {
		if p >= 1 {
			ent.setVal(to)
			ent.tdone.Store(true)
			e.popupWake()
			return
		}
		k := p
		if curve != nil {
			k = curve(p)
		}
		ent.setVal(from + (to-from)*k)
		e.popupWake()
	})
}

// popupWake просит кадр под шаг попапа: холст не меняется, кадр доходит
// только до renderPopups.
func (e *Engine) popupWake() {
	e.popupTicked.Store(true)
	e.invGen.Add(1)
}

// stopAnim снимает анимацию перехода (под popupMu).
func (ent *popupEntry) stopAnim() {
	if ent.anim != nil {
		ent.anim.Stop()
		ent.anim = nil
	}
}

// stopTrans снимает переход совсем: окно в покое.
func (ent *popupEntry) stopTrans() {
	ent.stopAnim()
	ent.trans, ent.closing = false, false
	ent.tdone.Store(false)
}

// transition — шаг перехода кадра или nil в покое.
func (ent *popupEntry) transition() *PopupTransition {
	if !ent.trans {
		return nil
	}
	v := ent.val()
	return &PopupTransition{
		Opacity: math.Max(0, math.Min(1, v)),
		Offset:  image.Pt(int(math.Round(float64(ent.slide.X)*(1-v))), int(math.Round(float64(ent.slide.Y)*(1-v)))),
		Closing: ent.closing,
	}
}

// sig — сигнатура попапа с шагом перехода.
func (ent *popupEntry) sig(id uintptr) popupSig {
	s := popupSig{id: id, rect: ent.rect, hash: ent.hash}
	if t := ent.transition(); t != nil {
		s.trans, s.closing, s.off = true, t.Closing, t.Offset
		s.op = int(math.Round(t.Opacity * 1000))
	}
	return s
}

// frame — кадр попапа для хоста.
func (ent *popupEntry) frame(id uintptr) PopupFrame {
	return PopupFrame{ID: id, Rect: ent.rect, Img: ent.img, Transition: ent.transition()}
}

// isSelfAnimated — анимирует ли оверлей себя сам.
func isSelfAnimated(od widget.OverlayDrawer) bool {
	sa, ok := od.(widget.SelfAnimatedOverlay)
	return ok && sa.AnimatesOverlay()
}

// popupOwners — ID владельцев оверлеев в дереве и открытых модальных
// окнах, открытых и погашенных: исчезнуть переходом может только оверлей,
// чей владелец ещё здесь.
func (e *Engine) popupOwners(root widget.Widget, modals []widget.ModalWidget) map[uintptr]bool {
	set := map[uintptr]bool{}
	var walk func(w widget.Widget, depth int)
	walk = func(w widget.Widget, depth int) {
		if w == nil || tooDeep(depth) {
			return
		}
		if _, ok := w.(widget.OverlayDrawer); ok {
			if id := widgetID(w); id != 0 {
				set[id] = true
			}
		}
		for _, c := range w.Children() {
			walk(c, depth+1)
		}
	}
	walk(root, 0)
	for _, m := range modals {
		if m.IsModal() {
			walk(m, 0)
		}
	}
	return set
}

// popupSlideDir — сдвиг в начале появления: оверлей выше точки привязки
// (или верха владельца) выезжает снизу, остальные — сверху.
func popupSlideDir(it popupItem, d int) image.Point {
	if d <= 0 {
		return image.Point{}
	}
	w, ok := it.od.(widget.Widget)
	if !ok {
		return image.Pt(0, -d)
	}
	return ovtSlideDir(w, it.rect.Add(it.off), d)
}
