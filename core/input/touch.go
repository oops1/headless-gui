package input

// touch.go — доставка касаний виджетам (widget.TouchEvent, widget.TouchHandler).
//
// Маршрутизация устроена как у нажатия мыши, но владелец касания определяется
// не захватом по заявке (WantsCapture), а ответом самого виджета:
//
//  1. Began: виджеты пути hit-теста под точкой, от самого глубокого к корню (как
//     всплытие нажатия), получают OnTouch; первый вернувший true становится
//     ВЛАДЕЛЬЦЕМ этого касания. Открытый оверлей (меню, список) старше дерева:
//     если точка в нём, касание достаётся ему, а если оверлей его не взял —
//     никому: виджет под меню не должен перехватить касание, предназначенное меню.
//  2. Moved / Ended / Cancelled: идут только владельцу, куда бы палец ни ушёл
//     (захват по ID на всё время касания). У каждого пальца свой владелец, поэтому
//     два пальца на двух виджетах ведут два независимых разговора, а два пальца на
//     одном виджете приходят ему обоими потоками с разными ID — на этом строится
//     щипок (widget.GestureRecognizer).
//  3. Касание, которого не взял никто, движок НЕ превращает в мышь: SendTouch
//     возвращает false. Превращение «ничьего» касания в левую кнопку — дело окна
//     (window.Window.SetTouchEnabled), а для окна, которое касаниями не
//     занимается, — дело ОС: она сама порождает мышь из касания, и движок не
//     вправе дублировать её (двойной щелчок на каждый тап).
//
// Координаты на входе — ФИЗИЧЕСКИЕ пиксели окна, как у SendMouse*; виджету
// касание уходит в ЛОГИЧЕСКИХ и в кадре самого виджета (frames.go): ребёнок
// прокрутки видит точку в координатах содержимого, как и при мыши.

import (
	"image"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// touchEpoch — начало отсчёта штампов, которые движок ставит событиям без
// собственного времени (TouchEvent.Time == 0).
var touchEpoch = time.Now()

// touchOwner — виджет, которому принадлежит касание, и цепочка контейнеров со
// сдвигом над ним: прокрутка может сдвинуться посреди касания (палец, тянущий
// содержимое, как раз её и сдвигает), и запомненное смещение стало бы ложью — как
// и у захвата мыши (captureOffset).
type touchOwner struct {
	w       widget.Widget
	chain   []widget.ContentOffsetter
	chainOK bool
	// x, y — где касание было в последний раз (логические экранные пиксели):
	// отмена без события пальца (CancelTouches) называет эту точку. Под
	// touchState.mu.
	x, y int
}

// touchState — владельцы активных касаний. Под mu: SendTouch идёт на горутине
// ввода, а CancelTouches могут позвать из любой.
type touchState struct {
	mu     sync.Mutex
	owners map[int]*touchOwner
}

// deliverTouch отдаёт касание виджету с координатами его кадра off (см.
// deliverButton).
func deliverTouch(th widget.TouchHandler, ev widget.TouchEvent, off image.Point) bool {
	if off == (image.Point{}) {
		return th.OnTouch(ev)
	}
	prev := widget.SetEventFrame(off)
	ev.X += off.X
	ev.Y += off.Y
	ok := th.OnTouch(ev)
	widget.SetEventFrame(prev)
	return ok
}

// ownerOffset — смещение кадра владельца касания прямо сейчас.
func (d *Dispatcher) ownerOffset(o *touchOwner) image.Point {
	if !o.chainOK {
		for _, r := range d.treeRoots() {
			if c, found := offsetChainIn(r, o.w, nil, 0); found {
				o.chain = c
				break
			}
		}
		o.chainOK = true
	}
	var off image.Point
	for _, oc := range o.chain {
		off = off.Add(oc.ContentOffset())
	}
	return off
}

// SendTouch доставляет событие касания и сообщает, принято ли оно.
//
// ev.X, ev.Y — физические пиксели окна, как у SendMouseMove; нулевое ev.Time
// движок заменяет временем прихода события. true — касание принадлежит виджету
// (на Began: нашёлся владелец; на остальных фазах: владелец есть и получил
// событие); false — касание ничьё, и решать его судьбу вызывающему (окно
// превращает такое в мышь, см. window.Window.SetTouchEnabled). События, чей
// палец движку неизвестен (Moved без Began), тоже дают false.
//
// Вход для бэкендов окна и для тестов: headless-приложение кормит этим движок
// так же, как SendMouseButton.
func (d *Dispatcher) SendTouch(ev widget.TouchEvent) bool {
	defer d.host.GuardInput("SendTouch")()
	defer d.serveFocusRequests()()

	ev.X, ev.Y = d.toLogical(ev.X, ev.Y)
	if k := d.host.Scale(); k != 1 && k > 0 {
		// Оси контакта — те же пиксели окна, что и точка: виджету тоже логические.
		ev.Major /= k
		ev.Minor /= k
	}
	if ev.Time == 0 {
		ev.Time = time.Since(touchEpoch) + 1
	}
	if ev.Phase == widget.TouchBegan {
		return d.touchBegan(ev)
	}
	return d.touchFollow(ev)
}

// touchBegan подбирает владельца новому касанию.
func (d *Dispatcher) touchBegan(ev widget.TouchEvent) bool {
	// Тот же ID без отрыва: ОС потеряла конец прежнего касания. Старому владельцу
	// сообщаем об отмене — иначе он остался бы с зажатым жестом навсегда.
	d.touch.mu.Lock()
	old := d.touch.owners[ev.ID]
	delete(d.touch.owners, ev.ID)
	d.touch.mu.Unlock()
	if old != nil {
		if th, ok := old.w.(widget.TouchHandler); ok {
			c := ev
			c.Phase = widget.TouchCancelled
			deliverTouch(th, c, d.ownerOffset(old))
		}
	}

	var dispatchRoot widget.Widget
	if m := d.host.TopModal(); m != nil {
		dispatchRoot = m
	} else {
		dispatchRoot = d.host.Root()
	}
	if dispatchRoot == nil {
		return false
	}

	x, y := ev.X, ev.Y
	path := HitPath(dispatchRoot, x, y)

	// Оверлей, на который пришлась точка, старше дерева (см. SendMouseButton).
	overlayW, overlayOff := findOverlayStep(dispatchRoot, x, y)

	// Как и нажатие мыши, касание закрывает всплывающие панели вне своего пути.
	// Делаем это до доставки: виджет, который в Began сам откроет меню, не должен
	// увидеть, как его только что открытое меню гасят.
	keep := pathSet(path)
	if overlayW != nil {
		keep[overlayW] = struct{}{}
	}
	if len(path) > 0 || overlayW != nil {
		DismissOutside(dispatchRoot, keep, x, y)
	}

	var owner widget.Widget
	if overlayW != nil {
		// Оверлей или берёт касание сам, или оно ничьё: дерево под ним не в счёт.
		if th, ok := overlayW.(widget.TouchHandler); ok && deliverTouch(th, ev, overlayOff) {
			owner = overlayW
		}
	} else {
		for i := len(path) - 1; i >= 0; i-- {
			th, ok := path[i].W.(widget.TouchHandler)
			if !ok {
				continue
			}
			if deliverTouch(th, ev, path[i].Off) {
				owner = path[i].W
				break
			}
		}
	}
	if owner == nil {
		return false
	}

	if _, ok := owner.(widget.Focusable); ok {
		d.focus.set(owner)
	}
	d.touch.mu.Lock()
	if d.touch.owners == nil {
		d.touch.owners = make(map[int]*touchOwner)
	}
	d.touch.owners[ev.ID] = &touchOwner{w: owner, x: ev.X, y: ev.Y}
	d.touch.mu.Unlock()

	// Касание, как и клик, может открыть оверлей или сменить фокус.
	d.host.Invalidate()
	return true
}

// touchFollow доставляет Moved / Ended / Cancelled владельцу касания.
func (d *Dispatcher) touchFollow(ev widget.TouchEvent) bool {
	d.touch.mu.Lock()
	o := d.touch.owners[ev.ID]
	if o != nil {
		o.x, o.y = ev.X, ev.Y
		if ev.Phase.Terminal() {
			delete(d.touch.owners, ev.ID)
		}
	}
	d.touch.mu.Unlock()
	if o == nil {
		return false
	}
	th, ok := o.w.(widget.TouchHandler)
	if !ok {
		return false
	}
	off := d.ownerOffset(o)
	deliverTouch(th, ev, off)
	d.invalidateWidget(o.w, off)
	return true
}

// CancelTouches отменяет все активные касания: каждому владельцу уходит
// TouchCancelled в точке, где касание было в последний раз. Зовут, когда ввод у
// окна отобрали — потеря фокуса, системный жест, — и конец касаний не придёт.
func (d *Dispatcher) CancelTouches() {
	d.touch.mu.Lock()
	owners := d.touch.owners
	d.touch.owners = nil
	d.touch.mu.Unlock()
	for id, o := range owners {
		th, ok := o.w.(widget.TouchHandler)
		if !ok {
			continue
		}
		deliverTouch(th, widget.TouchEvent{ID: id, X: o.x, Y: o.y, Phase: widget.TouchCancelled,
			Time: time.Since(touchEpoch) + 1}, d.ownerOffset(o))
	}
}

// TouchActive — сколько касаний сейчас принадлежат виджетам.
func (d *Dispatcher) TouchActive() int {
	d.touch.mu.Lock()
	defer d.touch.mu.Unlock()
	return len(d.touch.owners)
}
