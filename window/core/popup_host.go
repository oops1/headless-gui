package core

import (
	"image"
	"math"
	"sync"
	"sync/atomic"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget"
)

// ── Опциональная возможность бэкенда: окна-попапы для оверлеев ────────────────

// popupWindow — бэкенд умеет создавать окно-вьюпорт оверлея (dropdown/меню):
// без рамки, не активируется, поверх носителя. Реализуется Win32 и X11.
// Wayland/macOS не реализуют — там оверлеи рисуются в холст (in-canvas).
type popupWindow interface {
	CreatePopup(width, height int) error
}

// popupEngine — движок, принимающий проброшенные события мыши попап-окна
// (реализуется *engine.Engine и window.EngineAPI).
type popupEngine interface {
	SendMouseMove(x, y int)
	SendMouseButton(x, y int, btn widget.MouseButton, pressed bool)
}

// ── popupHost ────────────────────────────────────────────────────────────────

// popupHost реализует engine.PopupSink: каждый активный popup-оверлей движка
// показывается в собственном нативном окне-попапе ОС, спозиционированном
// относительно окна-носителя (carrier). Хост создаёт/двигает/закрывает окна по
// ID оверлея и транслирует их события мыши обратно в движок-носитель.
//
// Общий для Win32 и X11; платформенные различия — через опциональные интерфейсы
// (popupWindow/ownedWindow/eventPumper). Модальности/EnableWindow у попапов нет.
type popupHost struct {
	carrier native.NativeWindow // окно-носитель (главное окно или окно диалога)
	invoker uiThreadInvoker     // маршалинг создания/уничтожения окон на UI-поток
	eng     popupEngine         // движок-носитель (для проброса ввода)
	scale   float64             // HiDPI-масштаб носителя (лог × scale = физ)

	// in/run — очередь ввода носителя и постановка в его движок: события
	// попапа выполняются на горутине движка и в одной очереди с событиями
	// носителя, в порядке прихода (GG-68).
	in  *inputQueue
	run func(fn func())

	mu      sync.Mutex
	windows map[uintptr]*hostedPopup // ID оверлея → окно

	// ovrCursor/ovrSet — принудительный курсор движка, как его последний раз
	// получили окна-попапы (cursorOverrideSync). Под mu.
	ovrCursor widget.Cursor
	ovrSet    bool
	// ovrImg — курсор-картинка движка (Engine.SetCursorImage), как её получили
	// окна-попапы. Под mu.
	ovrImg *widget.CursorImage
}

// cursorOverrideSource — движок знает принудительный курсор (реализует
// *engine.Engine).
type cursorOverrideSource interface {
	CursorOverride() (widget.Cursor, bool)
}

// popupCursorSetter — окно ОС умеет менять форму курсора (SetCursor с кодом
// widget.Cursor), как окно-носитель.
type popupCursorSetter interface {
	SetCursor(c int)
}

// hostedPopup — одно окно-попап под конкретный оверлей.
type hostedPopup struct {
	native native.NativeWindow
	rect   image.Rectangle // логический прямоугольник оверлея (в координатах носителя)
	img    *image.RGBA     // последний контент (физические пиксели)
	w, h   int             // физический размер контента
	closed bool

	// Переход появления и исчезновения (engine.PopupTransition, задача 3.4).
	// alpha — непрозрачность окна, offset — сдвиг от rect (логический),
	// fade — как окно умеет прозрачность (до создания — догадка по носителю,
	// после — по окну). gone — окно закрыто раньше конца исчезновения (без
	// прозрачности), а ID ещё в кадрах. forceBlit — кадр в покое: кладётся
	// всегда, как до переходов. Под замком хоста.
	alpha     float64
	offset    image.Point
	fade      popupFadeKind
	gone      bool
	forceBlit bool

	// outMu — вывод в окно (положение, прозрачность, картинка) по одному за
	// раз; out — что в окне уже лежит. Задания вывода приводят окно к
	// последнему состоянию (reconcile), и их порядок не важен: на X11
	// InvokeOnUIThread — горутина на вызов, без порядка. Под замком хоста
	// outMu не берётся; наоборот — можно.
	outMu sync.Mutex
	out   popupOut

	// origin — физическое начало координат оверлея (x в старшем слове, y в
	// младшем). Ввод попапа читает его на потоке окна, и ждать рендер он не
	// должен: замок хоста держит горутина движка, а её нативный вызов может
	// ждать разбора сообщений этим самым потоком (GG-80).
	origin atomic.Uint64
}

// popupOut — что лежит в окне-попапе (под outMu).
type popupOut struct {
	placed bool
	rect   image.Rectangle // где окно стоит (логический, со сдвигом)
	w, h   int             // размер окна
	alpha  float64         // прозрачность окна (popupFadeWindow) или заложенная в картинку (popupFadeCopy)
	img    *image.RGBA     // кадр оверлея, по которому положена картинка
}

// effOffset — сдвиг, который окно применяет: у окна без прозрачности его нет
// — непрозрачное меню, проезжающее путь, вместо показа сразу выглядело бы
// рывками.
func (hp *hostedPopup) effOffset() image.Point {
	if hp.fade == popupFadeNone {
		return image.Point{}
	}
	return hp.offset
}

// setOrigin запоминает начало координат оверлея в физических пикселях.
func (hp *hostedPopup) setOrigin(rect image.Rectangle, scale float64) {
	x := int32(float64(rect.Min.X)*scale + 0.5)
	y := int32(float64(rect.Min.Y)*scale + 0.5)
	hp.origin.Store(uint64(uint32(x))<<32 | uint64(uint32(y)))
}

// originXY возвращает начало координат оверлея без блокировок.
func (hp *hostedPopup) originXY() (int, int) {
	v := hp.origin.Load()
	return int(int32(v >> 32)), int(int32(v))
}

// newPopupHost создаёт хост попапов для окна-носителя. in — очередь ввода
// носителя (nil — своя).
func newPopupHost(carrier native.NativeWindow, inv uiThreadInvoker, eng popupEngine, scale float64, in *inputQueue) *popupHost {
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
	return &popupHost{
		carrier: carrier,
		invoker: inv,
		eng:     eng,
		scale:   scale,
		in:      in,
		run:     run,
		windows: map[uintptr]*hostedPopup{},
	}
}

// apply — engine.PopupSink. Создаёт/обновляет/закрывает окна-попапы по составу
// frames. Вызывается из рендер-цикла движка (не UI-поток); нативные операции
// создания и позиционирования маршалятся на UI-поток носителя.
//
// Под замком хоста — только состав окон; всё нативное выполняется ПОСЛЕ него
// (GG-80). Блит перестал быть чистой записью пикселей: с выкройкой окна по
// закрашенной части он зовёт SetWindowRgn, а тот синхронно шлёт сообщения окну
// и ждёт, пока их разберёт поток окна. Если этот поток в тот же момент ждёт
// замок хоста — в обработчике движения мыши над попапом, — оба ждут друг друга
// навсегда: окно «не отвечает», Windows предлагает закрыть программу.
//
// Вывод — reconcile: окно приводится к последнему состоянию. Перенос и шаги
// перехода — на UI-потоке, обычный новый кадр — сразу, на горутине движка.
func (h *popupHost) apply(frames []engine.PopupFrame) {
	var (
		created []uintptr
		onUI    []*hostedPopup
		direct  []*hostedPopup
		closed  []native.NativeWindow
	)

	h.mu.Lock()
	seen := make(map[uintptr]bool, len(frames))
	for _, f := range frames {
		seen[f.ID] = true
		pw := f.Img.Bounds().Dx()
		ph := f.Img.Bounds().Dy()
		alpha, offset, closing := 1.0, image.Point{}, false
		if t := f.Transition; t != nil {
			alpha, offset, closing = t.Opacity, t.Offset, t.Closing
		}

		hp := h.windows[f.ID]
		if hp != nil && hp.gone && !closing {
			// Окно закрыли раньше конца исчезновения, а оверлей открыт снова.
			delete(h.windows, f.ID)
			hp = nil
		}
		if hp == nil {
			if closing {
				continue // исчезающий оверлей, которого здесь не показывали
			}
			// Новый оверлей — окно создаётся на UI-потоке.
			hp = &hostedPopup{rect: f.Rect, img: f.Img, w: pw, h: ph,
				alpha: alpha, offset: offset, fade: h.fadeHint()}
			hp.setOrigin(f.Rect, h.scale)
			h.windows[f.ID] = hp
			created = append(created, f.ID)
			continue
		}
		if hp.gone {
			continue
		}

		prev := hp.rect.Add(hp.effOffset())
		resized := hp.w != pw || hp.h != ph
		hp.rect, hp.img, hp.w, hp.h = f.Rect, f.Img, pw, ph
		hp.alpha, hp.offset = alpha, offset
		if f.Transition == nil {
			hp.forceBlit = true
		}
		move := resized || prev != hp.rect.Add(hp.effOffset())
		// Исчезающий оверлей ввод не держит: события под ним уходят тому, что
		// видно под ним, — в точке, где окно стоит сейчас, со сдвигом.
		if closing {
			hp.setOrigin(f.Rect.Add(hp.effOffset()), h.scale)
		} else {
			hp.setOrigin(f.Rect, h.scale)
		}
		if hp.native == nil {
			continue // окно ещё поднимается — createPopup возьмёт свежее состояние
		}
		if closing && hp.fade == popupFadeNone {
			// Исчезать нечем — окно уходит сразу, ID досматриваем до конца.
			closed = append(closed, hp.native)
			hp.native, hp.gone, hp.closed = nil, true, true
			continue
		}
		if move || f.Transition != nil {
			onUI = append(onUI, hp)
		} else {
			direct = append(direct, hp)
		}
	}

	// Окна оверлеев, которых больше нет в кадре.
	for id, hp := range h.windows {
		if seen[id] {
			continue
		}
		hp.closed = true
		if hp.native != nil {
			closed = append(closed, hp.native)
		}
		delete(h.windows, id)
	}
	h.mu.Unlock()

	for _, id := range created {
		h.invoker.InvokeOnUIThread(func() { h.createPopup(id) })
	}
	for _, hp := range onUI {
		h.invoker.InvokeOnUIThread(func() { h.reconcile(hp, true) })
	}
	for _, hp := range direct {
		h.reconcile(hp, false)
	}
	for _, nw := range closed {
		h.invoker.InvokeOnUIThread(func() { nw.Close() })
	}
}

// fadeHint — как окна-попапы этого носителя умеют прозрачность, пока окно
// ещё не создано: Win32 — прозрачностью окна (носитель того же рода); X11
// узнаёт про композитор только при создании попапа.
func (h *popupHost) fadeHint() popupFadeKind {
	if _, ok := h.carrier.(popupAlphaSetter); ok {
		return popupFadeWindow
	}
	return popupFadeNone
}

// reconcile приводит окно-попап к последнему состоянию: положение (place —
// можно двигать окно: задание на UI-потоке), прозрачность, картинку. Делается
// только недостающее, и задания можно выполнять в любом порядке — последнее
// всегда кладёт итог.
func (h *popupHost) reconcile(hp *hostedPopup, place bool) {
	hp.outMu.Lock()
	defer hp.outMu.Unlock()

	h.mu.Lock()
	nw := hp.native
	if nw == nil || hp.closed {
		h.mu.Unlock()
		return
	}
	rest, off := hp.rect, hp.effOffset()
	pw, ph := hp.w, hp.h
	alpha, src, fade := hp.alpha, hp.img, hp.fade
	force := hp.forceBlit
	hp.forceBlit = false
	carrier := h.carrier
	h.mu.Unlock()

	o := &hp.out
	at := rest.Add(off)
	if place && (!o.placed || o.rect != at || o.w != pw || o.h != ph) {
		h.positionPopup(nw, carrier, rest, off, pw, ph, !o.placed || o.w != pw || o.h != ph)
		o.placed, o.rect, o.w, o.h = true, at, pw, ph
	}
	if fade == popupFadeWindow && o.alpha != alpha {
		if as, ok := nw.(popupAlphaSetter); ok {
			as.SetPopupAlpha(alpha)
		}
		o.alpha = alpha
	}
	baked := fade == popupFadeCopy && o.alpha != alpha
	if force || o.img != src || baked {
		img := src
		if fade == popupFadeCopy && alpha < 1 {
			img = fadedCopy(src, alpha)
		}
		blitPopup(nw, img)
		o.img = src
		if fade == popupFadeCopy {
			o.alpha = alpha
		}
	}
}

// createPopup поднимает нативное окно-попап для оверлея id (на UI-потоке).
func (h *popupHost) createPopup(id uintptr) {
	h.mu.Lock()
	hp := h.windows[id]
	if hp == nil || hp.closed {
		h.mu.Unlock()
		return
	}
	pw, ph := hp.w, hp.h
	alpha := hp.alpha
	carrier := h.carrier
	h.mu.Unlock()

	nw := newNativeWindow() // хук: тесты подставляют окно ОС
	pop, ok := nw.(popupWindow)
	if !ok {
		return // бэкенд без поддержки окон-попапов (host не должен был ставиться)
	}
	// Прозрачность окна — до создания: окно, рождённое непрозрачным и
	// погашенное следом, мигнуло бы целиком.
	if as, ok := nw.(popupAlphaSetter); ok {
		as.SetPopupAlpha(alpha)
	}
	if err := pop.CreatePopup(pw, ph); err != nil {
		return
	}
	fade := popupFadeOf(nw)
	if ow, ok := nw.(ownedWindow); ok {
		ow.SetOwner(carrier)
	}
	h.mu.Lock()
	hp.fade = fade
	rest, off := hp.rect, hp.effOffset()
	pw, ph = hp.w, hp.h
	h.mu.Unlock()
	hp.outMu.Lock()
	h.positionPopup(nw, carrier, rest, off, pw, ph, true)
	hp.out = popupOut{placed: true, rect: rest.Add(off), w: pw, h: ph, alpha: 1}
	if fade == popupFadeWindow {
		hp.out.alpha = alpha
	}
	hp.outMu.Unlock()
	h.setupPopupInput(nw, hp)
	// Вторичное окно с собственным соединением (X11) — запускаем его насос событий.
	if ep, ok := nw.(eventPumper); ok {
		ep.StartEventPump()
	}

	h.mu.Lock()
	if hp.closed {
		h.mu.Unlock()
		nw.Close()
		return
	}
	hp.native = nw
	hp.forceBlit = true
	ovr, ovrSet, ovrImg := h.ovrCursor, h.ovrSet, h.ovrImg
	h.mu.Unlock()
	// Первая картинка и всё, что сменилось, пока окно поднималось.
	h.reconcile(hp, true)
	// Принудительный курсор, заданный до открытия попапа (или пока он
	// поднимался), действует и над ним. Мы на потоке окна.
	if ovrImg != nil {
		if ic, ok := nw.(cursorImageSetter); ok {
			ic.SetCursorImage(ovrImg)
			return
		}
	}
	if ovrSet {
		if sc, ok := nw.(popupCursorSetter); ok {
			sc.SetCursor(int(ovr))
		}
	}
}

// syncCursorOverride переносит принудительный курсор движка на окна-попапы.
// Зовётся на горутине движка при смене override (surface.refreshCursor) и
// после открытия попапа.
//
// Курсор окна-носителя до попапов не доходит: у каждого окна ОС свой. Поэтому
// заданная Engine.SetCursorOverride форма ставится и им; снятая — возвращает им
// стрелку (курсор виджетов в попапах не определяется, там стрелка, как и была).
func (h *popupHost) syncCursorOverride() {
	src, ok := h.eng.(cursorOverrideSource)
	if !ok {
		return
	}
	c, set := src.CursorOverride()
	img := cursorImageOf(h.eng)

	h.mu.Lock()
	if h.ovrSet == set && (!set || h.ovrCursor == c) && sameCursorImage(h.ovrImg, img) {
		h.mu.Unlock()
		return // ничего не изменилось
	}
	h.ovrCursor, h.ovrSet, h.ovrImg = c, set, img
	natives := make([]native.NativeWindow, 0, len(h.windows))
	for _, hp := range h.windows {
		if hp.native != nil {
			natives = append(natives, hp.native)
		}
	}
	h.mu.Unlock()

	shape := int(widget.CursorArrow)
	if set {
		shape = int(c)
	}
	for _, n := range natives {
		if img != nil {
			// Курсор-картинка: тот же указатель, что над носителем. Окно без
			// поддержки картинок показывает стрелку.
			if ic, ok := n.(cursorImageSetter); ok {
				h.invoker.InvokeOnUIThread(func() { ic.SetCursorImage(img) })
				continue
			}
		}
		sc, ok := n.(popupCursorSetter)
		if !ok {
			continue
		}
		h.invoker.InvokeOnUIThread(func() { sc.SetCursor(shape) })
	}
}

// popupRegionSetter — бэкенд умеет выкроить окно по закрашенной части кадра.
// Реализуется Win32; остальные просто блитят.
type popupRegionSetter interface {
	BlitPopupRegion(img *image.RGBA, bands []image.Rectangle)
}

// blitPopup кладёт кадр в окно-попап и, если бэкенд умеет, выкраивает окно по
// закрашенной части.
//
// Зачем выкройка. Вынесенный оверлей занимает ПРЯМОУГОЛЬНИК — объединение
// всего, что он рисует. У каскадного меню это полоса, раскрытое подменю и его
// дочернее подменю, стоящее правее и ниже; между ними остаётся площадь,
// которую не закрашивает никто. В холсте сквозь неё виден рабочий стол, а в
// отдельном окне видна чернота: окно непрозрачно, и «ничего» показывается
// чёрным прямоугольником.
func blitPopup(nw native.NativeWindow, img *image.RGBA) {
	if nw == nil || img == nil {
		return
	}
	if rs, ok := nw.(popupRegionSetter); ok {
		rs.BlitPopupRegion(img, engine.OpaqueBands(img))
		return
	}
	nw.BlitRGBA(img)
}

// workAreaProvider — бэкенд умеет сообщить рабочую область монитора,
// содержащего точку (экран минус таскбар). Реализуется Win32.
type workAreaProvider interface {
	WorkAreaAt(x, y int) image.Rectangle
}

// positionPopup ставит окно-попап в экранную позицию: угол клиентской области
// носителя + Rect.Min×scale. Размер — физические пиксели контента.
// Позиция вписывается в рабочую область монитора: меню у трея (низ экрана)
// иначе раскрывалось под таскбар, и нижние пункты («Выход») были недостижимы.
// Сдвиг окна безопасен для проброса ввода: клики транслируются в ЛОКАЛЬНЫХ
// координатах окна-попапа, которые не зависят от его экранной позиции.
//
// rest — место в покое: по нему выбирается и вписывается рабочая область,
// и лишь потом прибавляется сдвиг перехода off — выезд у края монитора не
// перебрасывает окно на соседний. resize — размер сменился (SetSize на
// каждом шаге перехода у X11 пересоздавал бы сегмент SHM).
func (h *popupHost) positionPopup(nw, carrier native.NativeWindow, rest image.Rectangle, off image.Point, pw, ph int, resize bool) {
	cx, cy := carrier.GetPosition()
	sx := int(float64(rest.Min.X)*h.scale + 0.5)
	sy := int(float64(rest.Min.Y)*h.scale + 0.5)
	x, y := cx+sx, cy+sy
	if wp, ok := nw.(workAreaProvider); ok {
		if wa := wp.WorkAreaAt(x, y); !wa.Empty() {
			if x+pw > wa.Max.X {
				x = wa.Max.X - pw
			}
			if y+ph > wa.Max.Y {
				y = wa.Max.Y - ph
			}
			if x < wa.Min.X {
				x = wa.Min.X
			}
			if y < wa.Min.Y {
				y = wa.Min.Y
			}
		}
	}
	// math.Round, а не +0.5: сдвиг вверх отрицательный, и усечение к нулю
	// недодавало бы точку.
	x += int(math.Round(float64(off.X) * h.scale))
	y += int(math.Round(float64(off.Y) * h.scale))
	if resize {
		nw.SetSize(pw, ph)
	}
	nw.SetPosition(x, y)
}

// setupPopupInput пробрасывает события мыши окна-попапа в движок-носитель:
// локальные физические координаты попапа → физические координаты носителя
// (Rect.Min×scale + локальные). Клавиатуру не трогаем — фокус у носителя.
// Начало координат берётся из атомарного снимка окна, а не из-под замка хоста:
// эти колбэки приходят на потоке окна, а замок держит горутина движка — ждать
// её здесь значит рисковать взаимоблокировкой (GG-80).
func (h *popupHost) setupPopupInput(nw native.NativeWindow, hp *hostedPopup) {
	// Начало координат снимается в момент события: к исполнению оверлей мог
	// переехать, а щёлкнули по тому месту, где он стоял.
	nw.SetOnMouseMove(func(px, py int) {
		ox, oy := hp.originXY()
		h.in.move(h.run, ox+px, oy+py, h.eng.SendMouseMove)
	})
	nw.SetOnMouseButton(func(px, py, button int, pressed bool) {
		btn, ok := nativeButton(button)
		if !ok {
			return
		}
		ox, oy := hp.originXY()
		h.in.post(h.run, func() {
			h.eng.SendMouseButton(ox+px, oy+py, btn, pressed)
		})
	})
}

// closeAll закрывает все окна-попапы (при разборе носителя, например teardown
// диалога, чей движок останавливается и sink больше не вызовется).
func (h *popupHost) closeAll() {
	h.mu.Lock()
	natives := make([]native.NativeWindow, 0, len(h.windows))
	for id, hp := range h.windows {
		hp.closed = true
		if hp.native != nil {
			natives = append(natives, hp.native)
		}
		delete(h.windows, id)
	}
	h.mu.Unlock()
	for _, n := range natives {
		nw := n
		h.invoker.InvokeOnUIThread(func() { nw.Close() })
	}
}
