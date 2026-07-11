package window

import (
	"image"
	"sync"

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
}

// newPopupHost создаёт хост попапов для окна-носителя.
func newPopupHost(carrier NativeWindow, inv uiThreadInvoker, eng popupEngine, scale float64) *popupHost {
	if scale <= 0 {
		scale = 1
	}
	return &popupHost{
		carrier: carrier,
		invoker: inv,
		eng:     eng,
		scale:   scale,
		windows: map[uintptr]*hostedPopup{},
	}
}

// apply — engine.PopupSink. Создаёт/обновляет/закрывает окна-попапы по составу
// frames. Вызывается из рендер-цикла движка (не UI-поток); нативные операции
// создания/позиционирования маршалятся на UI-поток носителя, блиты
// потокобезопасны.
func (h *popupHost) apply(frames []engine.PopupFrame) {
	h.mu.Lock()
	defer h.mu.Unlock()

	seen := make(map[uintptr]bool, len(frames))
	for _, f := range frames {
		seen[f.ID] = true
		pw := f.Img.Bounds().Dx()
		ph := f.Img.Bounds().Dy()

		hp := h.windows[f.ID]
		if hp == nil {
			// Новый оверлей — создаём окно на UI-потоке.
			h.windows[f.ID] = &hostedPopup{rect: f.Rect, img: f.Img, w: pw, h: ph}
			id := f.ID
			h.invoker.InvokeOnUIThread(func() { h.createPopup(id) })
			continue
		}

		moved := hp.rect != f.Rect || hp.w != pw || hp.h != ph
		hp.rect = f.Rect
		hp.img = f.Img
		hp.w = pw
		hp.h = ph
		if hp.native == nil {
			continue // окно ещё поднимается — createPopup возьмёт свежие rect/img
		}
		if moved {
			id := f.ID
			h.invoker.InvokeOnUIThread(func() { h.repositionAndBlit(id) })
		} else {
			hp.native.BlitRGBA(hp.img) // блит потокобезопасен
		}
	}

	// Закрываем окна оверлеев, которых больше нет в кадре.
	for id, hp := range h.windows {
		if seen[id] {
			continue
		}
		hp.closed = true
		native := hp.native
		delete(h.windows, id)
		if native != nil {
			h.invoker.InvokeOnUIThread(func() { native.Close() })
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
	if img != nil {
		native.BlitRGBA(img)
	}
	h.setupPopupInput(native, id)
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
	if latest != nil {
		native.BlitRGBA(latest)
	}
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
	if img != nil {
		native.BlitRGBA(img)
	}
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
func (h *popupHost) setupPopupInput(native NativeWindow, id uintptr) {
	physOrigin := func() (int, int) {
		h.mu.Lock()
		defer h.mu.Unlock()
		hp := h.windows[id]
		if hp == nil {
			return 0, 0
		}
		return int(float64(hp.rect.Min.X)*h.scale + 0.5),
			int(float64(hp.rect.Min.Y)*h.scale + 0.5)
	}

	native.SetOnMouseMove(func(px, py int) {
		ox, oy := physOrigin()
		h.eng.SendMouseMove(ox+px, oy+py)
	})
	native.SetOnMouseButton(func(px, py, button int, pressed bool) {
		var btn widget.MouseButton
		switch button {
		case 0:
			btn = widget.MouseLeft
		case 1:
			btn = widget.MouseRight
		case 2:
			btn = widget.MouseMiddle
		case 3:
			btn = widget.MouseWheelUp
		case 4:
			btn = widget.MouseWheelDown
		default:
			return
		}
		ox, oy := physOrigin()
		h.eng.SendMouseButton(ox+px, oy+py, btn, pressed)
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
