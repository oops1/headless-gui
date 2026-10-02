package window

// popup_host_child.go — хост всплывающих окон для бэкендов, где попап не
// самостоятельное окно ОС, а поверхность-потомок носителя.
//
// popupHost (popup_host.go) поднимает под каждый оверлей отдельное окно и
// ставит его по ЭКРАННЫМ координатам: угол окна-носителя плюс место оверлея.
// На Win32 и X11 это верно, на Wayland — невозможно: клиент не знает, где
// стоит его окно, и адресовать точку экрана нечем. Там попап создаётся как
// xdg_popup — потомок носителя, чьё место задано относительно родителя, —
// и управлять им умеет только сам бэкенд, внутри своего соединения.
//
// Отсюда второй хост: тот же engine.PopupSink, но вместо создания окон он
// зовёт бэкенд, а тот решает, как показать попап у себя.

import (
	"image"
	"sync"

	"github.com/oops1/headless-gui/v3/engine"
)

// childPopupHandlers — приёмники ввода попапов. Координаты локальные для
// попапа и физические, как и всюду на границе с бэкендом.
type childPopupHandlers struct {
	Move   func(id uintptr, x, y int)
	Button func(id uintptr, x, y, button int, pressed bool)
	Wheel  func(id uintptr, x, y int, dx, dy float64)
	// Done — попап закрыл не движок, а система: щелчок снаружи при захвате
	// ввода. Оверлей в дереве виджетов надо убрать, иначе он останется
	// открытым без окна.
	Done func(id uintptr)
}

// childPopupHost — бэкенд, показывающий попапы сам (Wayland).
//
// Координаты и размеры — физические пиксели в системе окна-носителя: то же,
// в чём бэкенд получает кадры и отдаёт события.
type childPopupHost interface {
	OpenChildPopup(id uintptr, x, y, w, h int) error
	MoveChildPopup(id uintptr, x, y, w, h int)
	BlitChildPopup(id uintptr, img *image.RGBA, bands []image.Rectangle)
	CloseChildPopup(id uintptr)
	SetChildPopupHandlers(h childPopupHandlers)
}

// overlayCloser — движок умеет закрыть все вынесенные оверлеи (реализует
// *engine.Engine).
type overlayCloser interface {
	CloseAllOverlays()
}

// childPopups — состав открытых попапов и перевод их ввода в движок.
type childPopups struct {
	be    childPopupHost
	eng   popupEngine
	scale float64

	// in/run — очередь ввода носителя и постановка в его движок: события
	// попапа исполняются на горутине движка и в одной очереди с событиями
	// носителя, в порядке прихода.
	in  *inputQueue
	run func(fn func())

	mu   sync.Mutex
	open map[uintptr]*childPopup
}

// childPopup — один показанный оверлей.
type childPopup struct {
	rect   image.Rectangle // логические координаты в холсте носителя
	w, h   int             // физический размер кадра
	origin image.Point     // физическое начало координат (rect.Min × scale)
}

// newChildPopups создаёт хост попапов-потомков.
func newChildPopups(be childPopupHost, eng popupEngine, scale float64, in *inputQueue) *childPopups {
	if scale <= 0 {
		scale = 1
	}
	if in == nil {
		in = &inputQueue{}
	}
	run := func(fn func()) { fn() }
	if p, ok := eng.(poster); ok {
		run = p.Post
	}
	c := &childPopups{be: be, eng: eng, scale: scale, in: in, run: run,
		open: map[uintptr]*childPopup{}}
	be.SetChildPopupHandlers(childPopupHandlers{
		Move:   c.onMove,
		Button: c.onButton,
		Wheel:  c.onWheel,
		Done:   c.onDone,
	})
	return c
}

// apply — engine.PopupSink: привести состав попапов к составу кадра.
func (c *childPopups) apply(frames []engine.PopupFrame) {
	type job struct {
		id      uintptr
		x, y    int
		w, h    int
		img     *image.RGBA
		bands   []image.Rectangle
		reopen  bool
		created bool
	}
	var jobs []job
	var closed []uintptr

	c.mu.Lock()
	seen := make(map[uintptr]bool, len(frames))
	for _, f := range frames {
		seen[f.ID] = true
		pw, ph := f.Img.Bounds().Dx(), f.Img.Bounds().Dy()
		origin := image.Pt(
			int(float64(f.Rect.Min.X)*c.scale+0.5),
			int(float64(f.Rect.Min.Y)*c.scale+0.5),
		)
		cp := c.open[f.ID]
		j := job{id: f.ID, x: origin.X, y: origin.Y, w: pw, h: ph,
			img: f.Img, bands: engine.OpaqueBands(f.Img)}
		if cp == nil {
			j.created = true
			c.open[f.ID] = &childPopup{rect: f.Rect, w: pw, h: ph, origin: origin}
		} else {
			j.reopen = cp.origin != origin || cp.w != pw || cp.h != ph
			cp.rect, cp.w, cp.h, cp.origin = f.Rect, pw, ph, origin
		}
		jobs = append(jobs, j)
	}
	for id := range c.open {
		if !seen[id] {
			closed = append(closed, id)
			delete(c.open, id)
		}
	}
	c.mu.Unlock()

	// Нативные вызовы — вне замка: бэкенд пишет в сокет компоновщика, а
	// ответы на него разбирает другая горутина, которой замок может
	// понадобиться.
	for _, id := range closed {
		c.be.CloseChildPopup(id)
	}
	for _, j := range jobs {
		switch {
		case j.created:
			if err := c.be.OpenChildPopup(j.id, j.x, j.y, j.w, j.h); err != nil {
				continue
			}
		case j.reopen:
			c.be.MoveChildPopup(j.id, j.x, j.y, j.w, j.h)
		}
		c.be.BlitChildPopup(j.id, j.img, j.bands)
	}
}

// closeAll убирает все попапы (окно-носитель разбирается).
func (c *childPopups) closeAll() {
	c.mu.Lock()
	ids := make([]uintptr, 0, len(c.open))
	for id := range c.open {
		ids = append(ids, id)
	}
	c.open = map[uintptr]*childPopup{}
	c.mu.Unlock()
	for _, id := range ids {
		c.be.CloseChildPopup(id)
	}
}

// originOf — физическое начало координат попапа в системе носителя.
//
// Берётся место, куда движок САМ нарисовал оверлей, а не то, куда попап
// поставил компоновщик: картинка одна и та же, и точка под курсором в ней
// одна и та же, где бы окно ни стояло на экране.
func (c *childPopups) originOf(id uintptr) (image.Point, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := c.open[id]
	if cp == nil {
		return image.Point{}, false
	}
	return cp.origin, true
}

func (c *childPopups) onMove(id uintptr, x, y int) {
	o, ok := c.originOf(id)
	if !ok {
		return
	}
	c.in.move(c.run, o.X+x, o.Y+y, c.eng.SendMouseMove)
}

func (c *childPopups) onButton(id uintptr, x, y, button int, pressed bool) {
	btn, ok := nativeButton(button)
	if !ok {
		return
	}
	o, found := c.originOf(id)
	if !found {
		return
	}
	c.in.post(c.run, func() {
		c.eng.SendMouseButton(o.X+x, o.Y+y, btn, pressed)
	})
}

func (c *childPopups) onWheel(id uintptr, x, y int, dx, dy float64) {
	we, ok := c.eng.(interface {
		SendMouseWheelPixels(x, y int, dx, dy float64)
	})
	if !ok {
		return
	}
	o, found := c.originOf(id)
	if !found {
		return
	}
	c.in.post(c.run, func() { we.SendMouseWheelPixels(o.X+x, o.Y+y, dx, dy) })
}

// onDone — попап закрыла система (щелчок снаружи при захвате ввода). Движку
// нужно убрать оверлей из дерева: окна у него больше нет.
func (c *childPopups) onDone(id uintptr) {
	oc, ok := c.eng.(overlayCloser)
	if !ok {
		return
	}
	c.mu.Lock()
	_, known := c.open[id]
	c.mu.Unlock()
	if !known {
		return
	}
	c.in.post(c.run, func() { oc.CloseAllOverlays() })
}
