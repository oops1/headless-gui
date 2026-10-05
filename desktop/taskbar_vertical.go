// taskbar_vertical.go — панель задач у бокового края экрана, на мониторе, со
// своими всплывающими панелями.
//
// Нижняя и верхняя панели — ряд: слоты идут слева направо. Панель у левого или
// правого края — столбец: «Пуск» сверху, приложения посередине, трей и часы
// снизу, как в настоящей системе. Горизонтальная раскладка (taskbar.go) от
// этого не менялась: столбец считается отдельно и включается только краем
// EdgeLeft/EdgeRight.
package desktop

import "image"

// VerticalItem — элемент панели, умеющий лежать в столбце.
//
// Панель зовёт SetVertical при добавлении элемента и каждый раз, когда панель
// меняет ориентацию. В столбце PreferredSize(avail) получает avail.X —
// толщину панели (без отступов), avail.Y — высоту, которую можно занять, и
// возвращает размер в обычных осях: X — поперёк панели (не больше толщины),
// Y — вдоль неё.
//
// Элемент, этого не умеющий, остаётся рабочим: панель спрашивает его размер для
// квадрата со стороной в толщину панели (так лежат кнопка «Пуск» и значки),
// даёт ему всю толщину поперёк и высоту, которую он просит, но не больше
// стороны квадрата; всё, что не влезло, обрезается его границами.
type VerticalItem interface {
	Item
	// SetVertical сообщает, лежит ли элемент в столбце. Повторный вызов с тем же
	// значением ничего не меняет.
	SetVertical(vertical bool)
}

// Vertical сообщает, что панель стоит у бокового края и слоты лежат столбцом.
func (t *Taskbar) Vertical() bool { return t.Edge().Vertical() }

// SetEdge назначает край явно, мимо темы: боковые края тема выразить не
// умеет (у неё только флаг taskbar.top). Панель перекладывает слоты и, если
// поставлена на монитор (DockTo), становится к новому краю; привязанные
// всплывающие панели (BindFlyouts) получают новый край.
func (t *Taskbar) SetEdge(e Edge) {
	if t.edgeSet && t.edge == e {
		return
	}
	t.edge, t.edgeSet = e, true
	t.edgeChanged()
}

// ResetEdge возвращает край, выводимый из темы (флаг taskbar.top).
func (t *Taskbar) ResetEdge() {
	if !t.edgeSet {
		return
	}
	t.edgeSet = false
	t.edgeChanged()
}

func (t *Taskbar) edgeChanged() {
	if t.docked {
		t.DockTo(t.monitor)
	} else {
		t.relayout()
		t.syncFlyouts()
	}
	t.Invalidate()
}

// Thickness — толщина панели поперёк её края: высота ряда или ширина столбца
// (метрика taskbar.width, а если тема её не объявила — taskbar.height).
func (t *Taskbar) Thickness() int {
	if t.Vertical() {
		if w := t.metric(KeyTaskbarWidth); w > 0 {
			return w
		}
	}
	return t.Height()
}

// syncOrientation сообщает элементам, лежат ли они в столбце.
func (t *Taskbar) syncOrientation() {
	v, edge := t.Vertical(), t.Edge()
	for _, slot := range t.slots {
		for _, it := range slot {
			if vi, ok := it.(VerticalItem); ok {
				vi.SetVertical(v)
			}
			if ei, ok := it.(EdgeAware); ok {
				ei.SetBarEdge(edge)
			}
		}
	}
}

// sizeOfV спрашивает размер элемента для столбца (см. VerticalItem): X —
// поперёк панели, Y — вдоль неё.
func (t *Taskbar) sizeOfV(it Item, avail image.Point) image.Point {
	if _, ok := it.(VerticalItem); ok {
		sz := it.PreferredSize(avail)
		if sz.X <= 0 || sz.X > avail.X {
			sz.X = avail.X
		}
		if sz.Y < 0 {
			sz.Y = 0
		}
		if sz.Y > avail.Y {
			sz.Y = avail.Y
		}
		return sz
	}
	// Элемент рассчитан на ряд: даём ему квадрат со стороной в толщину панели.
	side := avail.X
	sz := it.PreferredSize(image.Pt(side, side))
	if sz.X <= 0 && sz.Y <= 0 {
		return image.Pt(avail.X, 0)
	}
	h := sz.Y
	if h <= 0 || h > side {
		h = side
	}
	return image.Pt(avail.X, h)
}

// relayoutVertical расставляет слоты столбцом: «Пуск» сверху, трей снизу,
// приложения — в оставшемся промежутке.
//
// Устроена как горизонтальная раскладка, повёрнутая на девяносто градусов;
// отступ taskbar.pad.x и зазор taskbar.gap берутся те же, но отсчитываются
// вдоль столбца. Группа не центруется (taskbar.centered — свойство ряда).
func (t *Taskbar) relayoutVertical(b image.Rectangle) {
	pad := t.metric(KeyTaskbarPadX)
	gap := t.metric(KeyTaskbarGap)
	inner := image.Rect(b.Min.X, b.Min.Y+pad, b.Max.X, b.Max.Y-pad)
	if inner.Empty() {
		return
	}
	avail := image.Pt(inner.Dx(), inner.Dy())

	// Трей и часы внизу: считаются первыми, их верхний край — граница для
	// остальных.
	bottom := inner.Max.Y
	for i := len(t.slots[SlotTray]) - 1; i >= 0; i-- {
		it := t.slots[SlotTray][i]
		sz := t.sizeOfV(it, avail)
		placeV(it, image.Rect(inner.Min.X, bottom-sz.Y, inner.Min.X+sz.X, bottom), inner)
		bottom -= sz.Y + gap
	}
	trayStart := bottom
	t.trayEdge = trayStart

	start, apps := t.slots[SlotStart], t.slots[SlotApps]

	y := inner.Min.Y
	// Слот виджетов — самый верх столбца, над «Пуском».
	for _, it := range t.slots[SlotWidgets] {
		sz := t.sizeOfV(it, avail)
		placeV(it, image.Rect(inner.Min.X, y, inner.Min.X+sz.X, y+sz.Y), inner)
		if sz.Y > 0 {
			y += sz.Y + gap
		}
	}
	for _, it := range start {
		sz := t.sizeOfV(it, avail)
		placeV(it, image.Rect(inner.Min.X, y, inner.Min.X+sz.X, y+sz.Y), inner)
		y += sz.Y + gap
	}
	t.startEdge = y
	if len(apps) == 0 {
		return
	}

	// Приложениям — промежуток до трея; не влезают — сжимаются пропорционально,
	// как в ряду.
	midAvail := trayStart - y
	if midAvail < 0 {
		midAvail = 0
	}
	hs := make([]int, len(apps))
	total := 0
	for i, it := range apps {
		hs[i] = t.sizeOfV(it, image.Pt(avail.X, midAvail)).Y
		total += hs[i]
	}
	if len(apps) > 0 {
		total += gap * (len(apps) - 1)
	}
	scale := 1.0
	if total > midAvail && total > 0 {
		gaps := gap * (len(apps) - 1)
		if total > gaps {
			scale = float64(midAvail-gaps) / float64(total-gaps)
		}
		if scale < 0 {
			scale = 0
		}
	}
	for i, it := range apps {
		h := int(float64(hs[i]) * scale)
		placeV(it, image.Rect(inner.Min.X, y, inner.Max.X, y+h), inner)
		y += h + gap
	}
}

// placeV ставит элемент в прямоугольник столбца: узкий элемент центруется по
// ширине панели (как в ряду значок центруется по высоте).
func placeV(it Item, r, inner image.Rectangle) {
	if w := r.Dx(); w > 0 && w < inner.Dx() {
		left := inner.Min.X + (inner.Dx()-w)/2
		r = image.Rect(left, r.Min.Y, left+w, r.Max.Y)
	}
	r = r.Intersect(inner)
	if r.Empty() {
		it.SetBounds(image.Rectangle{})
		return
	}
	it.SetBounds(r)
}

// ─── Монитор ────────────────────────────────────────────────────────────────

// DockTo ставит панель к её краю на мониторе m: вдоль всего края, толщиной
// Thickness. Возвращает занятый прямоугольник (пустой, если тема не задала
// толщину). Привязанные всплывающие панели (BindFlyouts) получают этот монитор
// и рабочую область без панели.
//
// Повторный вызов с изменившимся монитором (поменялось разрешение, панель
// переехала) переставляет панель на месте, ничего не пересоздавая. Смена темы,
// меняющая край или толщину, переставляет панель сама.
func (t *Taskbar) DockTo(m Monitor) image.Rectangle {
	t.monitor, t.docked = m, true
	th := t.Thickness()
	bd := m.Bounds
	var r image.Rectangle
	switch t.Edge() {
	case EdgeTop:
		r = image.Rect(bd.Min.X, bd.Min.Y, bd.Max.X, bd.Min.Y+th)
	case EdgeLeft:
		r = image.Rect(bd.Min.X, bd.Min.Y, bd.Min.X+th, bd.Max.Y)
	case EdgeRight:
		r = image.Rect(bd.Max.X-th, bd.Min.Y, bd.Max.X, bd.Max.Y)
	default:
		r = image.Rect(bd.Min.X, bd.Max.Y-th, bd.Max.X, bd.Max.Y)
	}
	if th <= 0 {
		r = image.Rectangle{}
	}
	t.SetBounds(r)
	t.syncFlyouts()
	return r
}

// Monitor возвращает монитор, на который панель поставлена (DockTo); второе
// значение false — не ставилась.
func (t *Taskbar) Monitor() (Monitor, bool) { return t.monitor, t.docked }

// WorkArea — рабочая область монитора панели без полосы самой панели
// (автоскрытая панель места не занимает). Без монитора — пустой прямоугольник.
func (t *Taskbar) WorkArea() image.Rectangle {
	if !t.docked {
		return image.Rectangle{}
	}
	w := t.monitor.Work()
	r := t.ReservedArea().Intersect(w)
	if r.Empty() {
		return w
	}
	switch t.Edge() {
	case EdgeTop:
		w.Min.Y = r.Max.Y
	case EdgeLeft:
		w.Min.X = r.Max.X
	case EdgeRight:
		w.Max.X = r.Min.X
	default:
		w.Max.Y = r.Min.Y
	}
	return w
}

// BindFlyouts привязывает всплывающие панели к краю и монитору этой панели:
// панель раскрывает их от своего края (Flyout.Edge) и держит в границах своего
// монитора с рабочей областью без полосы панели (Flyout.SetMonitor). Привязка
// действует и на будущие смены края, темы и монитора — оболочка ничего не
// переставляет вручную.
//
// Принимает *Flyout, а также StartMenu, NotificationCenter и любой тип,
// встраивающий *Flyout. Повторная привязка той же панели ничего не дублирует.
func (t *Taskbar) BindFlyouts(panels ...FlyoutPanel) {
	for _, p := range panels {
		if p == nil {
			continue
		}
		f := p.AsFlyout()
		if f == nil {
			continue
		}
		dup := false
		for _, b := range t.bound {
			if b == f {
				dup = true
				break
			}
		}
		if !dup {
			t.bound = append(t.bound, f)
		}
	}
	t.syncFlyouts()
}

// UnbindFlyouts снимает привязку; панели остаются на последнем краю и мониторе.
func (t *Taskbar) UnbindFlyouts(panels ...FlyoutPanel) {
	for _, p := range panels {
		if p == nil {
			continue
		}
		f := p.AsFlyout()
		for i, b := range t.bound {
			if b == f {
				t.bound = append(t.bound[:i], t.bound[i+1:]...)
				break
			}
		}
	}
}

// BoundFlyouts возвращает привязанные панели.
func (t *Taskbar) BoundFlyouts() []*Flyout { return append([]*Flyout(nil), t.bound...) }

// syncFlyouts раздаёт привязанным панелям край и монитор.
func (t *Taskbar) syncFlyouts() {
	if len(t.bound) == 0 {
		return
	}
	edge := t.Edge()
	var m Monitor
	if t.docked {
		m = t.monitor
		m.WorkArea = t.WorkArea()
	}
	for _, f := range t.bound {
		f.Edge = edge
		if t.docked {
			f.SetMonitor(m)
		}
	}
}
