package window

import (
	"image"
	"sync"
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
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
	carrier NativeWindow    // окно-носитель (главное окно или окно диалога)
	invoker uiThreadInvoker // маршалинг создания/уничтожения окон на UI-поток
	eng     popupEngine     // движок-носитель (для проброса ввода)
	scale   float64         // HiDPI-масштаб носителя (лог × scale = физ)

	// in/run — очередь ввода носителя и постановка в его движок: события
	// попапа выполняются на горутине движка и в одной очереди с событиями
	// носителя, в порядке прихода (GG-68).
	in  *inputQueue
	run func(fn func())

	mu      sync.Mutex
	windows map[uintptr]*hostedPopup // ID оверлея → окно
}

// hostedPopup — одно окно-попап под конкретный оверлей.
type hostedPopup struct {
	native NativeWindow
	rect   image.Rectangle // логический прямоугольник оверлея (в координатах носителя)
	img    *image.RGBA     // последний контент (физические пиксели)
	w, h   int             // физический размер контента
	closed bool

	// origin — физическое начало координат оверлея (x в старшем слове, y в
	// младшем). Ввод попапа читает его на потоке окна, и ждать рендер он не
	// должен: замок хоста держит горутина движка, а её нативный вызов может
	// ждать разбора сообщений этим самым потоком (GG-80).
	origin atomic.Uint64
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
func newPopupHost(carrier NativeWindow, inv uiThreadInvoker, eng popupEngine, scale float64, in *inputQueue) *popupHost {
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

// blitJob — окно и кадр, которые нужно положить в него после снятия замка.
type blitJob struct {
	native NativeWindow
	img    *image.RGBA
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
func (h *popupHost) apply(frames []engine.PopupFrame) {
	var (
		created []uintptr
		moved   []uintptr
		blits   []blitJob
		closed  []NativeWindow
	)

	h.mu.Lock()
	seen := make(map[uintptr]bool, len(frames))
	for _, f := range frames {
		seen[f.ID] = true
		pw := f.Img.Bounds().Dx()
		ph := f.Img.Bounds().Dy()

		hp := h.windows[f.ID]
		if hp == nil {
			// Новый оверлей — окно создаётся на UI-потоке.
			hp = &hostedPopup{rect: f.Rect, img: f.Img, w: pw, h: ph}
			hp.setOrigin(f.Rect, h.scale)
			h.windows[f.ID] = hp
			created = append(created, f.ID)
			continue
		}

		move := hp.rect != f.Rect || hp.w != pw || hp.h != ph
		hp.rect = f.Rect
		hp.img = f.Img
		hp.w = pw
		hp.h = ph
		hp.setOrigin(f.Rect, h.scale)
		if hp.native == nil {
			continue // окно ещё поднимается — createPopup возьмёт свежие rect/img
		}
		if move {
			moved = append(moved, f.ID)
		} else {
			blits = append(blits, blitJob{hp.native, hp.img})
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
	for _, id := range moved {
		h.invoker.InvokeOnUIThread(func() { h.repositionAndBlit(id) })
	}
	for _, j := range blits {
		blitPopup(j.native, j.img)
	}
	for _, native := range closed {
		h.invoker.InvokeOnUIThread(func() { native.Close() })
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
	rect := hp.rect
	img := hp.img
	pw, ph := hp.w, hp.h
	carrier := h.carrier
	h.mu.Unlock()

	native := NewNativeWindow()
	pop, ok := native.(popupWindow)
	if !ok {
		return // бэкенд без поддержки окон-попапов (host не должен был ставиться)
	}
	if err := pop.CreatePopup(pw, ph); err != nil {
		return
	}
	if ow, ok := native.(ownedWindow); ok {
		ow.SetOwner(carrier)
	}
	h.positionPopup(native, carrier, rect, pw, ph)
	blitPopup(native, img)
	h.setupPopupInput(native, hp)
	// Вторичное окно с собственным соединением (X11) — запускаем его насос событий.
	if ep, ok := native.(eventPumper); ok {
		ep.StartEventPump()
	}

	h.mu.Lock()
	if hp.closed {
		h.mu.Unlock()
		native.Close()
		return
	}
	hp.native = native
	latest := hp.img // мог обновиться, пока окно поднималось
	h.mu.Unlock()
	blitPopup(native, latest)
}

// popupRegionSetter — бэкенд умеет выкроить окно по закрашенной части кадра.
// Реализуется Win32; остальные просто блитят.
type popupRegionSetter interface {
	blitPopupRegion(img *image.RGBA, bands []image.Rectangle)
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
func blitPopup(native NativeWindow, img *image.RGBA) {
	if native == nil || img == nil {
		return
	}
	if rs, ok := native.(popupRegionSetter); ok {
		rs.blitPopupRegion(img, engine.OpaqueBands(img))
		return
	}
	native.BlitRGBA(img)
}

// repositionAndBlit переносит окно под новый Rect/размер и блитит контент.
func (h *popupHost) repositionAndBlit(id uintptr) {
	h.mu.Lock()
	hp := h.windows[id]
	if hp == nil || hp.closed || hp.native == nil {
		h.mu.Unlock()
		return
	}
	native := hp.native
	carrier := h.carrier
	rect := hp.rect
	img := hp.img
	pw, ph := hp.w, hp.h
	h.mu.Unlock()

	h.positionPopup(native, carrier, rect, pw, ph)
	blitPopup(native, img)
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
func (h *popupHost) positionPopup(native, carrier NativeWindow, rect image.Rectangle, pw, ph int) {
	cx, cy := carrier.GetPosition()
	sx := int(float64(rect.Min.X)*h.scale + 0.5)
	sy := int(float64(rect.Min.Y)*h.scale + 0.5)
	x, y := cx+sx, cy+sy
	if wp, ok := native.(workAreaProvider); ok {
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
	native.SetSize(pw, ph)
	native.SetPosition(x, y)
}

// setupPopupInput пробрасывает события мыши окна-попапа в движок-носитель:
// локальные физические координаты попапа → физические координаты носителя
// (Rect.Min×scale + локальные). Клавиатуру не трогаем — фокус у носителя.
// Начало координат берётся из атомарного снимка окна, а не из-под замка хоста:
// эти колбэки приходят на потоке окна, а замок держит горутина движка — ждать
// её здесь значит рисковать взаимоблокировкой (GG-80).
func (h *popupHost) setupPopupInput(native NativeWindow, hp *hostedPopup) {
	// Начало координат снимается в момент события: к исполнению оверлей мог
	// переехать, а щёлкнули по тому месту, где он стоял.
	native.SetOnMouseMove(func(px, py int) {
		ox, oy := hp.originXY()
		h.in.move(h.run, ox+px, oy+py, h.eng.SendMouseMove)
	})
	native.SetOnMouseButton(func(px, py, button int, pressed bool) {
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
	natives := make([]NativeWindow, 0, len(h.windows))
	for id, hp := range h.windows {
		hp.closed = true
		if hp.native != nil {
			natives = append(natives, hp.native)
		}
		delete(h.windows, id)
	}
	h.mu.Unlock()
	for _, n := range natives {
		native := n
		h.invoker.InvokeOnUIThread(func() { native.Close() })
	}
}
