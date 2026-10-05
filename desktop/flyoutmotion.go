// flyoutmotion.go — появление и исчезновение всплывающей панели.
//
// Панель не возникает и не пропадает мгновенно: она выезжает от края привязки
// («Пуск» — снизу, центр уведомлений — справа) и проявляется, а закрываясь —
// уезжает и гаснет. Длительность и кривую берёт тема (токен AnimMenuOpen);
// нулевая длительность — мгновенно, как в Windows 2000.
//
// Состояние — одно число, «присутствие» панели: 0 — её нет, 1 — стоит на месте.
// Оно идёт за флагом «открыта» по анимации движка (widget.AnimateOwned,
// часы у движка). Из него выводятся и сдвиг, и прозрачность, так что
// открытие, закрытие и переоткрытие посреди закрытия — один механизм, без
// скачков.
//
// Перерисовывается только область, в которой панель движется: её
// прямоугольник в покое плюс тень и путь от края. Кадр целиком анимация не
// будит.
package desktop

import (
	"image"
	"math"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Presence возвращает присутствие панели: 0 — не показана, 1 — на месте, между —
// выезжает или уезжает. У кривых с выбросом (out-back) бывает чуть больше
// единицы.
func (f *Flyout) Presence() float64 {
	return math.Float64frombits(f.presence.Load())
}

func (f *Flyout) setPresence(v float64) {
	f.presence.Store(math.Float64bits(v))
}

// IsAnimating сообщает, идёт ли сейчас появление или исчезновение.
func (f *Flyout) IsAnimating() bool {
	want := 0.0
	if f.IsOpen() {
		want = 1
	}
	return f.Presence() != want
}

// Settle обрывает анимацию и ставит панель в конечное положение: открытая
// встаёт на место, закрытая пропадает. Нужен там, где ждать нечего: снимок
// экрана, тест, смена темы посреди движения.
func (f *Flyout) Settle() {
	f.amu.Lock()
	prev := f.anim
	f.anim = nil
	f.amu.Unlock()
	if prev != nil {
		prev.Stop()
	}
	was := f.paintedRegion()
	if f.IsOpen() {
		f.setPresence(1)
	} else {
		f.setPresence(0)
	}
	widget.InvalidateRect(was)
	widget.InvalidateRect(f.paintedRegion())
}

// visible — панель показана: открыта или ещё уезжает.
func (f *Flyout) visible() bool {
	return f.IsOpen() || f.Presence() > 0
}

// animKey — имя анимации темы для этой панели.
func (f *Flyout) animKey() theme.Key {
	if f.AnimKey != "" {
		return f.AnimKey
	}
	return AnimMenuOpen
}

// animateTo ведёт присутствие к to (1 — открытие, 0 — закрытие).
func (f *Flyout) animateTo(to float64) {
	dur, curve := animation(f.tm, f.animKey())
	from := f.Presence()

	f.amu.Lock()
	prev := f.anim
	f.anim = nil
	f.amu.Unlock()

	if dur <= 0 || from == to {
		if prev != nil {
			prev.Stop()
		}
		f.setPresence(to)
		return
	}

	// AnimateOwned сам снимает прежнюю анимацию панели: открытие посреди
	// закрытия продолжает с того места, где панель была, а не с нуля.
	a := widget.AnimateOwned(f, "present", dur, curve, func(t float64) {
		v := widget.LerpF(from, to, t)
		if v < 0 {
			v = 0
		}
		f.setPresence(v)
		widget.InvalidateRect(f.paintedRegion())
	})
	f.amu.Lock()
	f.anim = a
	f.amu.Unlock()
}

// slideFrom — откуда выезжает панель, с учётом SlideAuto.
func (f *Flyout) slideFrom() SlideFrom {
	if f.Slide != SlideAuto {
		return f.Slide
	}
	// Прижатая к краю монитора панель выезжает из-за этого края, остальные — от
	// края панели задач, к которой привязаны.
	e := f.Edge
	if f.pinned {
		e = f.pinEdge
	}
	switch e {
	case EdgeTop:
		return SlideTop
	case EdgeLeft:
		return SlideLeft
	case EdgeRight:
		return SlideRight
	}
	return SlideBottom
}

// slideDistance — на сколько точек панель сдвинута в самом начале появления.
func (f *Flyout) slideDistance(rest image.Rectangle) int {
	d := f.SlideDistance
	if d > 0 {
		return d
	}
	if d == 0 {
		if m := f.metric(KeyFlyoutSlideDistance); m > 0 {
			return m
		}
		return flyoutSlideDefault
	}
	// До края экрана: панель стартует целиком за ним.
	scr := f.Screen
	switch f.slideFrom() {
	case SlideRight:
		if !scr.Empty() {
			return scr.Max.X - rest.Min.X
		}
		return rest.Dx()
	case SlideLeft:
		if !scr.Empty() {
			return rest.Max.X - scr.Min.X
		}
		return rest.Dx()
	case SlideTop:
		if !scr.Empty() {
			return rest.Max.Y - scr.Min.Y
		}
		return rest.Dy()
	default:
		if !scr.Empty() {
			return scr.Max.Y - rest.Min.Y
		}
		return rest.Dy()
	}
}

// slideOffset — на сколько панель сейчас смещена от места в покое.
func (f *Flyout) slideOffset() image.Point {
	p := f.Presence()
	if p >= 1 {
		return image.Point{}
	}
	from := f.slideFrom()
	if from == SlideNone {
		return image.Point{}
	}
	v := int(math.Round(float64(f.slideDistance(f.restRect())) * (1 - p)))
	switch from {
	case SlideBottom:
		return image.Pt(0, v)
	case SlideTop:
		return image.Pt(0, -v)
	case SlideLeft:
		return image.Pt(-v, 0)
	case SlideRight:
		return image.Pt(v, 0)
	}
	return image.Point{}
}

// dirtyRect — область, которую панель занимает на экране: в покое — её
// прямоугольник, пока движется — область движения.
func (f *Flyout) dirtyRect() image.Rectangle {
	rest := f.restRect()
	if rest.Empty() || f.Presence() >= 1 {
		return rest
	}
	return f.motionRegion(rest)
}

// paintedRegion — область, которую надо перерисовать на шаге анимации: то, что
// панель занимает сейчас, а пока она закрыта и уезжает — ещё и место, где она
// стояла в покое на момент Close.
//
// Одного dirtyRect мало: на последнем шаге закрытия restRect() вправе уже быть
// пустым (размер панели зависит от содержимого, которое при закрытии
// освобождается), и тогда освободившаяся область не заявлялась — предыдущий
// кадр, на котором почти прозрачная панель ещё видна, оставался последним,
// что получал потребитель, собирающий кадр по повреждениям.
func (f *Flyout) paintedRegion() image.Rectangle {
	r := f.dirtyRect()
	if f.IsOpen() {
		return r
	}
	f.amu.Lock()
	rest := f.closeRest
	f.amu.Unlock()
	if rest.Empty() {
		return r
	}
	rest = rest.Inset(-f.shadowPad())
	if !f.Screen.Empty() {
		rest = rest.Intersect(f.Screen)
	}
	return r.Union(rest)
}

// shadowPad — запас вокруг окна под тень стиля (0, если тени нет).
func (f *Flyout) shadowPad() int {
	s := f.style(theme.StateNormal)
	if s == nil {
		return 0
	}
	if sp, ok := s.ExplicitShadow(); ok {
		// Мягкая тень по токенам: размытие, умноженное на два, плюс смещение.
		return sp.Extent()
	}
	if s.Elevation > 0 && s.Shadow.A > 0 {
		return int(s.Elevation*2.5) + 1
	}
	return 0
}

// motionRegion — всё, что панель способна закрасить, пока движется: место в
// покое, тень вокруг него и путь от края привязки.
//
// Со стороны, откуда панель выезжает, область упирается в источник движения —
// край панели задач или экрана: панель вырастает из-за него и не залезает
// поверх. С остальных сторон добавляется запас под тень.
func (f *Flyout) motionRegion(rest image.Rectangle) image.Rectangle {
	r := rest.Inset(-f.shadowPad())

	switch f.slideFrom() {
	case SlideBottom:
		r.Max.Y = rest.Max.Y
		if !f.Anchor.Empty() && f.Anchor.Min.Y > rest.Max.Y {
			r.Max.Y = f.Anchor.Min.Y
		}
	case SlideTop:
		r.Min.Y = rest.Min.Y
		if !f.Anchor.Empty() && f.Anchor.Max.Y < rest.Min.Y {
			r.Min.Y = f.Anchor.Max.Y
		}
	case SlideRight:
		r.Max.X = rest.Max.X
		if !f.Screen.Empty() {
			r.Max.X = f.Screen.Max.X
		}
		// Боковая панель справа: окно вырастает из-за неё и не залезает на неё.
		if !f.pinned && f.Edge == EdgeRight && !f.Anchor.Empty() && f.Anchor.Min.X >= rest.Max.X {
			r.Max.X = f.Anchor.Min.X
		}
	case SlideLeft:
		r.Min.X = rest.Min.X
		if !f.Screen.Empty() {
			r.Min.X = f.Screen.Min.X
		}
		if !f.pinned && f.Edge == EdgeLeft && !f.Anchor.Empty() && f.Anchor.Max.X <= rest.Min.X {
			r.Min.X = f.Anchor.Max.X
		}
	}
	if !f.Screen.Empty() {
		r = r.Intersect(f.Screen)
	}
	return r
}
