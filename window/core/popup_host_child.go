package core

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
	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/widget"
)

// childPopupHost — бэкенд, показывающий попапы сам (Wayland).
//
// Координаты и размеры — физические пиксели в системе окна-носителя: то же,
// в чём бэкенд получает кадры и отдаёт события.
type childPopupHost interface {
	OpenChildPopup(id uintptr, x, y, w, h int) error
	MoveChildPopup(id uintptr, x, y, w, h int)
	BlitChildPopup(id uintptr, img *image.RGBA, bands []image.Rectangle)
	CloseChildPopup(id uintptr)
	SetChildPopupHandlers(h native.ChildPopupHandlers)
}

// childPopupCursor — бэкенд умеет выбирать форму курсора над своими попапами.
// У xdg_popup курсор общий с указателем, и над попапом он остаётся тем, что
// выбрало окно-носитель, пока указатель был на нём; SetPopupCursor задаёт
// форму для времени, когда указатель на попапе: set — принудительный курсор
// движка (c), иначе стрелка.
type childPopupCursor interface {
	SetPopupCursor(c int, set bool)
}

// childPopupCursorImage — бэкенд умеет показывать над своими попапами
// курсор-картинку (Engine.SetCursorImage). Следующий SetPopupCursor заменяет
// её формой.
type childPopupCursorImage interface {
	SetPopupCursorImage(ci *widget.CursorImage)
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

	// ovrCursor/ovrSet — принудительный курсор движка, как его последний раз
	// получил бэкенд (syncCursorOverride). Под mu.
	ovrCursor widget.Cursor
	ovrSet    bool
	// ovrImg — курсор-картинка движка, как её последний раз получил бэкенд.
	ovrImg *widget.CursorImage
}

// childPopup — один показанный оверлей.
type childPopup struct {
	rect   image.Rectangle // логические координаты в холсте носителя
	w, h   int             // физический размер кадра
	origin image.Point     // физическое начало координат (rect.Min × scale)

	// Переход появления и исчезновения (engine.PopupTransition, задача 3.4).
	// img и alpha — кадр движка и непрозрачность, по которым сделан
	// последний отданный бэкенду кадр. closing — оверлей исчезает: попап
	// держится, пока ID в кадрах. gone — попап убран раньше конца
	// исчезновения (открылся другой), а ID ещё в кадрах.
	img     *image.RGBA
	alpha   float64
	closing bool
	gone    bool
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
	be.SetChildPopupHandlers(native.ChildPopupHandlers{
		Move:   c.onMove,
		Button: c.onButton,
		Wheel:  c.onWheel,
		Done:   c.onDone,
	})
	return c
}

// apply — engine.PopupSink: привести состав попапов к составу кадра.
//
// Переходы: непрозрачность закладывается в копию кадра (fadedCopy — буфер
// попапа с альфой точек, компоновщик смешивает её с тем, что под ним);
// исчезающий оверлей держит попап, пока его ID в кадрах. Сдвиг (Offset) не
// прикладывается: переставить xdg_popup можно, только пересоздав его
// (MoveChildPopup), — на каждом шаге это мигание, а не выезд.
func (c *childPopups) apply(frames []engine.PopupFrame) {
	type job struct {
		id      uintptr
		x, y    int
		w, h    int
		img     *image.RGBA
		alpha   float64
		reopen  bool
		created bool
	}
	var jobs []job
	var closed []uintptr

	c.mu.Lock()
	seen := make(map[uintptr]bool, len(frames))
	fresh := false // открывается или пересоздаётся попап живого оверлея
	for _, f := range frames {
		alpha, closing := 1.0, false
		if t := f.Transition; t != nil {
			alpha, closing = t.Opacity, t.Closing
		}
		cp := c.open[f.ID]
		if cp != nil && cp.gone && !closing {
			// Попап убрали раньше конца исчезновения, а оверлей открыт снова.
			delete(c.open, f.ID)
			cp = nil
		}
		if cp == nil && closing {
			continue // исчезающий оверлей, которого здесь не показывали
		}
		seen[f.ID] = true
		if cp != nil && cp.gone {
			continue
		}
		pw, ph := f.Img.Bounds().Dx(), f.Img.Bounds().Dy()
		origin := image.Pt(
			int(float64(f.Rect.Min.X)*c.scale+0.5),
			int(float64(f.Rect.Min.Y)*c.scale+0.5),
		)
		j := job{id: f.ID, x: origin.X, y: origin.Y, w: pw, h: ph,
			img: f.Img, alpha: alpha}
		if cp == nil {
			j.created = true
			cp = &childPopup{rect: f.Rect, w: pw, h: ph, origin: origin}
			c.open[f.ID] = cp
		} else {
			j.reopen = cp.origin != origin || cp.w != pw || cp.h != ph
			cp.rect, cp.w, cp.h, cp.origin = f.Rect, pw, ph, origin
		}
		// Кадр — только если в попапе лежит не он: шаг перехода, сменивший
		// один сдвиг, картинку не меняет.
		blit := j.created || j.reopen || cp.img != f.Img || cp.alpha != alpha
		cp.img, cp.alpha, cp.closing = f.Img, alpha, closing
		if !closing && (j.created || j.reopen) {
			fresh = true
		}
		if blit {
			jobs = append(jobs, j)
		}
	}
	if fresh {
		// xdg_popup уничтожаются только сверху вниз. Новый попап стал бы
		// потомком исчезающего (тот держит захват ввода), и исчезающий, уйдя
		// в конце перехода раньше потомка, разорвал бы соединение
		// протокольной ошибкой. Поэтому исчезающие уходят сразу, до открытия.
		for id, cp := range c.open {
			if cp.closing && !cp.gone {
				cp.gone = true
				closed = append(closed, id)
			}
		}
		kept := jobs[:0]
		for _, j := range jobs {
			if !c.open[j.id].gone {
				kept = append(kept, j)
			}
		}
		jobs = kept
	}
	for id, cp := range c.open {
		if !seen[id] {
			if !cp.gone {
				closed = append(closed, id)
			}
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
	opened := false
	for _, j := range jobs {
		switch {
		case j.created:
			if err := c.be.OpenChildPopup(j.id, j.x, j.y, j.w, j.h); err != nil {
				continue
			}
			opened = true
		case j.reopen:
			c.be.MoveChildPopup(j.id, j.x, j.y, j.w, j.h)
		}
		// Область ввода — по кадру в покое: у погашенной копии альфа точек
		// ниже, а ловить клики попап должен там же.
		img := j.img
		if j.alpha < 1 {
			img = fadedCopy(j.img, j.alpha)
		}
		c.be.BlitChildPopup(j.id, img, engine.OpaqueBands(j.img))
	}
	if opened {
		c.syncCursorOverride() // попап открылся под действующим принудительным курсором
	}
}

// syncCursorOverride отдаёт бэкенду принудительный курсор движка (форма над
// попапами). Зовётся на горутине движка при смене override
// (surface.refreshCursor) и после открытия попапа; повтор того же значения
// бэкенда не беспокоит.
func (c *childPopups) syncCursorOverride() {
	cb, ok := c.be.(childPopupCursor)
	if !ok {
		return
	}
	src, ok := c.eng.(cursorOverrideSource)
	if !ok {
		return
	}
	cur, set := src.CursorOverride()
	img := cursorImageOf(c.eng)
	c.mu.Lock()
	same := c.ovrSet == set && (!set || c.ovrCursor == cur) && sameCursorImage(c.ovrImg, img)
	c.ovrCursor, c.ovrSet, c.ovrImg = cur, set, img
	c.mu.Unlock()
	if same {
		return
	}
	if img != nil {
		if ci, ok := c.be.(childPopupCursorImage); ok {
			ci.SetPopupCursorImage(img)
			return
		}
	}
	cb.SetPopupCursor(int(cur), set)
}

// closeAll убирает все попапы (окно-носитель разбирается).
func (c *childPopups) closeAll() {
	c.mu.Lock()
	ids := make([]uintptr, 0, len(c.open))
	for id, cp := range c.open {
		if !cp.gone {
			ids = append(ids, id)
		}
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
//
// Исчезающий попап ввод пересылает, как popupHost: оверлей движок уже
// закрыл, и событие уходит тому, что в этой точке холста под ним.
func (c *childPopups) originOf(id uintptr) (image.Point, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := c.open[id]
	if cp == nil || cp.gone {
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
	cp := c.open[id]
	// Исчезающий оверлей движок уже закрыл: закрывать нечего, а живые
	// оверлеи, открытые поверх, трогать нельзя.
	known := cp != nil && !cp.closing && !cp.gone
	c.mu.Unlock()
	if !known {
		return
	}
	c.in.post(c.run, func() { oc.CloseAllOverlays() })
}
