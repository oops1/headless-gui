package window

import (
	"image"
	"image/color"
	"sync"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// ── Опциональные возможности бэкенда для нативных модалок ────────────────────

// ownedWindow — поддержка owner-окон и модальности (реализует Win32Window).
// Дочернее окно принадлежит owner'у (всегда поверх него, сворачивается вместе
// с ним), а owner на время модалки отключается (SetEnabled(false)).
type ownedWindow interface {
	SetOwner(parent NativeWindow)
	SetEnabled(v bool)
}

// uiThreadInvoker — маршалинг колбэка на поток цикла сообщений окна.
// Создание/уничтожение нативного окна легально только на этом потоке, а
// модалку можно открыть/закрыть из произвольной горутины (таймер, фон).
type uiThreadInvoker interface {
	InvokeOnUIThread(fn func())
}

// foregrounder — возвращение фокуса ОС окну (Win32 SetForegroundWindow).
type foregrounder interface {
	SetForeground()
}

// eventPumper — бэкенд, вторичное окно которого НЕ обслуживается общим циклом
// сообщений и требует отдельного насоса событий. dialogHost вызывает
// StartEventPump после создания окна и подключения ввода. Win32 не реализует
// (одна очередь сообщений маршрутизирует события всех окон по hwnd); X11
// реализует — у каждого окна собственное соединение, которое иначе никто не
// читает (RunEventLoop для вторичного окна dialogHost не вызывает).
type eventPumper interface {
	StartEventPump()
}

// installModalHost устанавливает хост нативных модалок, если бэкенд
// поддерживает owner-окна и маршалинг на UI-поток, а движок принимает хост.
// На бэкендах без поддержки (X11/Wayland/Cocoa) и в headless — no-op: движок
// показывает модалки в своём холсте (прежнее поведение).
func (win *Window) installModalHost() {
	if _, ok := win.native.(ownedWindow); !ok {
		return
	}
	if _, ok := win.native.(uiThreadInvoker); !ok {
		return
	}
	setter, ok := win.eng.(interface {
		SetModalHost(h engine.ModalHost)
	})
	if !ok {
		return
	}
	setter.SetModalHost(&dialogHost{parent: win})
}

// ── dialogHost ───────────────────────────────────────────────────────────────

// dialogHost реализует engine.ModalHost поверх Win32: каждый модальный диалог
// показывается в собственном нативном окне ОС со своим (вторичным) движком,
// холст которого ровно по размеру диалога. Диалог поверх диалога (FileDialog
// из диалога) даёт стек окон: owner нового окна — верхнее окно стека.
type dialogHost struct {
	parent *Window

	mu    sync.Mutex
	stack []*hostedModal
}

// hostedModal — одна модалка, показанная в собственном окне.
type hostedModal struct {
	dlg    *widget.Dialog
	owner  NativeWindow // окно, отключаемое на время этой модалки

	// Заполняются в create() (на UI-потоке).
	eng       *engine.Engine
	native    NativeWindow
	surf      *surface
	popupHost *popupHost // хост popup-оверлеев диалога (nil без поддержки)

	created  bool // окно фактически создано
	closed   bool // запрошено закрытие (до/после создания)
	tornDown bool // ресурсы освобождены (идемпотентность teardown)
}

// ShowModal — engine.ModalHost. Хостим только *widget.Dialog; прочее отдаём
// движку (false → in-canvas). Возвращаем true сразу: реальное создание окна
// маршалится на UI-поток (безопасно из таймеров/горутин).
func (h *dialogHost) ShowModal(m widget.ModalWidget) bool {
	dlg, ok := m.(*widget.Dialog)
	if !ok {
		return false
	}

	h.mu.Lock()
	var owner NativeWindow
	if n := len(h.stack); n > 0 {
		owner = h.stack[n-1].native
		if owner == nil {
			// Верхняя модалка ещё не создалась — привяжемся к главному окну.
			owner = h.parent.native
		}
	} else {
		owner = h.parent.native
	}
	if _, ok := owner.(ownedWindow); !ok {
		h.mu.Unlock()
		return false
	}
	hm := &hostedModal{dlg: dlg, owner: owner}
	h.stack = append(h.stack, hm)
	h.mu.Unlock()

	h.invoker().InvokeOnUIThread(func() { h.create(hm) })
	return true
}

// CloseModal — engine.ModalHost. Закрываем через вторичный движок (его
// onModalClosed → teardown). Если окно ещё не создано — помечаем closed, и
// create() свернёт его сам. false → модалка не наша.
func (h *dialogHost) CloseModal(m widget.ModalWidget) bool {
	h.mu.Lock()
	var hm *hostedModal
	if m == nil {
		if n := len(h.stack); n > 0 {
			hm = h.stack[n-1]
		}
	} else {
		for _, x := range h.stack {
			if x.dlg == m {
				hm = x
				break
			}
		}
	}
	if hm == nil {
		h.mu.Unlock()
		return false
	}
	already := hm.closed
	hm.closed = true
	created := hm.created
	eng := hm.eng
	dlg := hm.dlg
	h.mu.Unlock()

	if already || !created {
		// Ещё не создано — create() увидит closed и свернёт. Уже закрывается —
		// повторно ничего не делаем.
		return true
	}
	// Закрываем через вторичный движок: он вызовет onModalClosed → teardown.
	eng.CloseModal(dlg)
	return true
}

// create строит вторичный движок и нативное окно для модалки. Выполняется на
// UI-потоке (через InvokeOnUIThread).
func (h *dialogHost) create(hm *hostedModal) {
	h.mu.Lock()
	if hm.closed {
		// Закрыли до создания — просто выкидываем из стека.
		h.removeLocked(hm)
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()

	dlg := hm.dlg
	scale := h.parent.scale
	if scale <= 0 {
		scale = 1
	}
	b := dlg.Bounds()
	dw, dh := b.Dx(), b.Dy()
	if dw <= 0 || dh <= 0 {
		h.mu.Lock()
		h.removeLocked(hm)
		h.mu.Unlock()
		return
	}
	// Вторичный движок ровно по размеру диалога.
	fps := h.parent.maxFPS
	if fps <= 0 {
		fps = 60
	}
	eng := engine.New(dw, dh, fps)
	if scale != 1 {
		eng.SetScale(scale)
	}
	// Синхронизирует глобальный widget.SetScreenBounds(dw,dh) под диалог.
	eng.SetResolution(dw, dh)

	// Физический размер окна/буфера берём у движка — гарантирует попиксельное
	// совпадание с кадрами (тайлами), включая округление HiDPI по краям.
	pw, ph := eng.PhysicalSize()

	// Корень — панель цвета диалога: скруглённые углы корпуса не оставляют
	// «дыр» (за пределами round-rect виден тот же фон), затемнение не нужно.
	root := widget.NewPanel(dlg.Background)
	root.SetBounds(image.Rect(0, 0, dw, dh))
	eng.SetRoot(root)

	// В собственном окне затемнение фона бессмысленно — отключаем (иначе dim
	// перекрыл бы углы скругления). In-canvas путь не затрагивается: это другой
	// объект-сценарий (диалог показывается только здесь, у хоста).
	dlg.Dim = color.RGBA{}

	native := NewNativeWindow()
	// Минимум объявляем ДО Create: иначе окно, меньшее дефолтного минимума
	// платформы, будет создано увеличенным (см. fitMinTrack в native_windows).
	//
	// У обычного диалога минимум — его собственный размер: тянуть диалог с
	// вопросом и двумя кнопками незачем. У растяжимого (Dialog.SetResizable)
	// минимум берётся из его MinSize, иначе окно нельзя было бы сделать ни
	// меньше, ни больше — рамка есть, а предел один и тот же.
	minW, minH := pw, ph
	if dlg.IsResizable() {
		mw, mh := dlg.MinSize()
		minW = int(float64(mw)*scale + 0.5)
		minH = int(float64(mh)*scale + 0.5)
	}
	native.SetMinSize(minW, minH)
	surf := &surface{
		eng:     eng,
		native:  native,
		scale:   scale,
		current: image.NewRGBA(image.Rect(0, 0, pw, ph)),
	}

	if err := native.Create(dlg.Title, pw, ph); err != nil {
		h.mu.Lock()
		h.removeLocked(hm)
		h.mu.Unlock()
		go eng.Stop()
		return
	}
	native.SetResizable(dlg.IsResizable())
	native.SetMinSize(minW, minH)
	if dlg.CornerRadius > 0 {
		native.SetCornerRadius(int(float64(dlg.CornerRadius)*scale + 0.5))
	}

	// Owner + модальность.
	if ow, ok := native.(ownedWindow); ok {
		ow.SetOwner(hm.owner)
	}
	if ow, ok := hm.owner.(ownedWindow); ok {
		ow.SetEnabled(false)
	}
	h.centerOn(native, hm.owner, pw, ph)

	// Drag за заголовок диалога → перемещение нативного окна (лог → физ).
	dlg.OnDragMove = func(dx, dy int) {
		if scale != 1 {
			dx = int(float64(dx)*scale + 0.5)
			dy = int(float64(dy)*scale + 0.5)
		}
		x, y := native.GetPosition()
		native.SetPosition(x+dx, y+dy)
	}

	// Ресайз рамки окна ОС → новый размер холста и диалога.
	//
	// Только у растяжимого: у обычного окно и так не тянется, а лишний
	// обработчик означал бы лишнюю ветку в разборе событий каждого диалога.
	if dlg.IsResizable() {
		native.SetOnResize(func(newW, newH int) {
			if newW <= 0 || newH <= 0 {
				return
			}
			lw, lh := newW, newH
			if scale != 1 {
				lw = int(float64(newW)/scale + 0.5)
				lh = int(float64(newH)/scale + 0.5)
			}
			surf.mu.Lock()
			surf.current = image.NewRGBA(image.Rect(0, 0, newW, newH))
			surf.mu.Unlock()
			eng.SetResolution(lw, lh)
			// Диалог занимает окно целиком: он и есть его содержимое.
			dlg.SetBounds(image.Rect(0, 0, lw, lh))
			eng.Invalidate()
		})
	}

	// Закрытие изнутри вторичного движка (✕/Escape/кнопка → closer → CloseModal).
	eng.SetOnModalClosed(func(widget.ModalWidget) { h.teardown(hm) })
	// Закрытие нативного окна ОС (Alt+F4 / ✕ рамки) — как отмена диалога.
	native.SetOnClose(func() bool {
		dlg.OnCancel()
		h.teardown(hm)
		return false // окно уничтожит teardown (через native.Close)
	})

	// Сохраняем ресурсы в hm ДО запуска окна: teardown (который может сработать
	// от ввода ✕/Esc сразу после setupInput) должен видеть native/eng, чтобы
	// корректно закрыть окно. Флаг created взводим ПОСЛЕДНИМ — только тогда окно
	// «живо» и CloseModal вправе закрывать его через eng.CloseModal.
	h.mu.Lock()
	hm.eng, hm.native, hm.surf = eng, native, surf
	h.mu.Unlock()

	// Показ диалога как модалки во вторичном движке (центрирует/клэмпит по его
	// холсту = размеру диалога → диалог в (0,0)); вешает closer/capture/fade.
	eng.ShowModal(dlg)

	// Насос кадров и ввод вторичного окна.
	surf.setupInput()
	surf.setupExposeRedraw()
	eng.Start()
	go surf.framePump()

	// Хост popup-оверлеев для движка диалога: dropdown'ы/меню внутри диалога
	// открываются в собственных окнах ОС, спозиционированных от окна диалога.
	if _, ok := native.(popupWindow); ok {
		if inv, ok := native.(uiThreadInvoker); ok {
			ph := newPopupHost(native, inv, eng, scale)
			eng.SetPopupSink(ph.apply)
			h.mu.Lock()
			hm.popupHost = ph
			h.mu.Unlock()
			// Деактивация окна диалога (клик в другое приложение) закрывает попапы.
			if an, ok := native.(activationNotifier); ok {
				an.SetOnActivate(func(active bool) {
					if !active {
						eng.CloseAllOverlays()
					}
				})
			}
		}
	}

	// Вторичное окно с собственным соединением (X11) само не обслуживается
	// общим циклом сообщений — запускаем его насос событий. Win32 не реализует
	// eventPumper (одна очередь на все окна): для него это no-op, путь неизменен.
	if ep, ok := native.(eventPumper); ok {
		ep.StartEventPump()
	}

	// created — последним, под тем же мьютексом, что читает CloseModal.
	h.mu.Lock()
	closedDuringCreate := hm.closed
	hm.created = true
	h.mu.Unlock()
	if closedDuringCreate {
		// Закрыли, пока окно поднималось — сворачиваем сейчас.
		h.teardown(hm)
	}
}

// teardown освобождает ресурсы модалки: снимает со стека, восстанавливает
// screenBounds под оставшееся верхнее окно, включает owner и уничтожает окно.
// Идемпотентно (tornDown).
func (h *dialogHost) teardown(hm *hostedModal) {
	h.mu.Lock()
	if hm.tornDown {
		h.mu.Unlock()
		return
	}
	hm.tornDown = true
	hm.closed = true
	h.removeLocked(hm)

	// Экран, к которому вернуться: верхняя оставшаяся модалка или родитель.
	var rw, rh int
	if n := len(h.stack); n > 0 {
		tb := h.stack[n-1].dlg.Bounds()
		rw, rh = tb.Dx(), tb.Dy()
	} else {
		rw, rh = h.parent.w, h.parent.h
	}
	owner := hm.owner
	native := hm.native
	eng := hm.eng
	ph := hm.popupHost
	h.mu.Unlock()

	// Закрываем окна popup-оверлеев диалога (его движок останавливается — sink
	// больше не вызовется, поэтому окна не закроются сами).
	if ph != nil {
		ph.closeAll()
	}

	// Восстанавливаем глобальные screenBounds под оставшееся верхнее окно.
	if rw > 0 && rh > 0 {
		widget.SetScreenBounds(rw, rh)
	}

	// Останавливаем вторичный движок (framePump завершится по закрытию Frames()).
	if eng != nil {
		go eng.Stop()
	}

	// Нативные операции — на UI-потоке.
	h.invoker().InvokeOnUIThread(func() {
		if ow, ok := owner.(ownedWindow); ok {
			ow.SetEnabled(true)
		}
		if fg, ok := owner.(foregrounder); ok {
			fg.SetForeground()
		}
		if native != nil {
			native.Close()
		}
	})
}

// removeLocked убирает hm из стека (при удержанном h.mu).
func (h *dialogHost) removeLocked(hm *hostedModal) {
	for i, x := range h.stack {
		if x == hm {
			h.stack = append(h.stack[:i], h.stack[i+1:]...)
			return
		}
	}
}

// centerOn ставит окно child по центру owner'а (физические координаты экрана).
func (h *dialogHost) centerOn(child, owner NativeWindow, pw, ph int) {
	ox, oy := owner.GetPosition()
	ow, oh := owner.GetSize()
	child.SetPosition(ox+(ow-pw)/2, oy+(oh-ph)/2)
}

// invoker возвращает маршалер UI-потока главного окна.
func (h *dialogHost) invoker() uiThreadInvoker {
	return h.parent.native.(uiThreadInvoker)
}
