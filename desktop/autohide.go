// autohide.go — панель, уезжающая за край экрана.
//
// Панель занимает место, которого на маленьком экране жалко: удалённый стол
// в окне 1024×768 отдаёт ей пятнадцатую часть высоты навсегда. Скрытая
// панель оставляет у края чувствительную полоску в пару точек: курсор
// подведён — панель выезжает, курсор ушёл — уезжает обратно.
//
// Это не было в задании, но без этого панель нельзя убрать вовсе, а именно
// так с ней обычно и живут на тесных экранах.
package desktop

import (
	"image"
	"sync/atomic"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

const (
	// KeyTaskbarAutoHideBand — высота чувствительной полоски у края, по
	// которой скрытая панель узнаёт о подведённом курсоре.
	//
	// Слишком тонкая полоска не ловится мышью, слишком толстая выдёргивает
	// панель, когда её не звали. Две точки — то, чем обходятся настоящие
	// системы.
	KeyTaskbarAutoHideBand theme.Key = "taskbar.autohide.band"

	// AnimTaskbarSlide — имя анимации выезда в теме.
	AnimTaskbarSlide theme.Key = "taskbar.slide"
)

// defaultSlide — длительность выезда, если тема своей не назвала.
const defaultSlide = 140 * time.Millisecond

// SetAutoHide включает или выключает автоскрытие.
//
// Включение сразу убирает панель за край: иначе она осталась бы на экране до
// первого движения мыши, и настройка выглядела бы не сработавшей.
func (t *Taskbar) SetAutoHide(v bool) {
	if t.autoHide == v {
		return
	}
	t.autoHide = v
	if v {
		t.conceal(false)
		return
	}
	t.reveal(false)
}

// AutoHide сообщает, включено ли автоскрытие.
func (t *Taskbar) AutoHide() bool { return t.autoHide }

// IsRevealed сообщает, показана ли панель сейчас.
func (t *Taskbar) IsRevealed() bool { return atomic.LoadInt32(&t.revealed) == 1 }

// Reveal выдвигает панель (плавно). Оболочке нужно, когда панель показывают
// не мышью: горячей клавишей, из меню, при открытии меню «Пуск».
func (t *Taskbar) Reveal() { t.reveal(true) }

// Conceal убирает панель за край (плавно).
func (t *Taskbar) Conceal() { t.conceal(true) }

// reveal и conceal отличаются только целью смещения: 0 — панель на месте,
// её высота — панель полностью за краем.
func (t *Taskbar) reveal(animated bool) {
	atomic.StoreInt32(&t.revealed, 1)
	t.slideTo(0, animated)
}

func (t *Taskbar) conceal(animated bool) {
	atomic.StoreInt32(&t.revealed, 0)
	h := t.fullBounds.Dy()
	if h <= 0 {
		h = t.Height()
	}
	t.slideTo(h, animated)
}

// slideTo двигает панель к смещению want (в точках от её места).
func (t *Taskbar) slideTo(want int, animated bool) {
	if t.fullBounds.Empty() {
		t.offset = want
		return
	}
	if !animated {
		t.offset = want
		t.applyOffset()
		return
	}

	from := t.offset
	if from == want {
		return
	}
	dur := defaultSlide
	if t.tm != nil {
		if a := t.tm.GetAnimation(AnimTaskbarSlide); a.Duration > 0 {
			dur = a.Duration
		}
	}
	// AnimateOwned, а не Animate: подведённый и тут же убранный курсор иначе
	// оставил бы две анимации, тянущие панель в разные стороны.
	widget.AnimateOwned(t, "slide", dur, widget.EaseOutCubic, func(k float64) {
		t.offset = from + int(float64(want-from)*k)
		t.applyOffset()
	})
}

// applyOffset ставит панель со смещением и перекладывает элементы.
func (t *Taskbar) applyOffset() {
	if t.fullBounds.Empty() {
		return
	}
	r := t.shiftedBounds()
	t.Base.SetBounds(r)
	t.relayout()
	t.Invalidate()
}

// shiftedBounds — границы панели с учётом смещения.
//
// Уезжает панель В СВОЙ край: нижняя вниз, верхняя вверх. Если бы смещение
// всегда шло вниз, строка меню macOS выезжала бы на середину экрана.
func (t *Taskbar) shiftedBounds() image.Rectangle {
	r := t.fullBounds
	if t.offset == 0 {
		return r
	}
	if t.Edge() == EdgeTop {
		return r.Sub(image.Pt(0, t.offset))
	}
	return r.Add(image.Pt(0, t.offset))
}

// revealBand — чувствительная полоска у края экрана, по которой скрытая
// панель узнаёт о курсоре.
func (t *Taskbar) revealBand() image.Rectangle {
	if t.fullBounds.Empty() {
		return image.Rectangle{}
	}
	h := t.metric(KeyTaskbarAutoHideBand)
	if h <= 0 {
		h = 2
	}
	r := t.fullBounds
	if t.Edge() == EdgeTop {
		return image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+h)
	}
	return image.Rect(r.Min.X, r.Max.Y-h, r.Max.X, r.Max.Y)
}

// handleAutoHideMove решает, выехать панели или уехать.
//
// Возвращает true, если событие относится к автоскрытию и разбирать его
// дальше не нужно.
func (t *Taskbar) handleAutoHideMove(x, y int) bool {
	if !t.autoHide || t.fullBounds.Empty() {
		return false
	}
	pt := image.Pt(x, y)
	if t.IsRevealed() {
		// Ушёл за пределы выдвинутой панели — убираем. С запасом в полоску:
		// иначе панель дёргается, стоит курсору коснуться самого её края.
		if !pt.In(t.fullBounds) {
			t.conceal(true)
		}
		return false
	}
	if pt.In(t.revealBand()) {
		t.reveal(true)
		return true
	}
	return false
}
