package window

import (
	"image"
	"log"
	"sync"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// dock_host.go — фаза 2 докинг-панелей: отрыв DockPane в отдельное НЕМОДАЛЬНОЕ
// нативное окно ОС и возврат доком.
//
// По образцу dialogHost (modal_host.go), но без модальности: каждая оторванная
// панель показывается в собственном borderless-окне, принадлежащем главному
// (owned window: поверх него, сворачивается вместе, НЕ блокирует его —
// SetEnabled(false) не вызывается). Корень вторичного движка — сама панель p:
// её титлбар работает как титлбар окна (drag двигает нативное окно через
// DockPane.OnDragMove; кнопка dock/✕ возвращает/закрывает).
//
// Установка: window.Window.EnableDockFloating(dm). На бэкендах без owner-окон и
// маршалинга на UI-поток (Wayland/macOS) и в headless — no-op: панель флоатит в
// холсте менеджера как в фазе 1 (OnFloatNative не навешивается).
//
// Ограничения фазы 2:
//   - drag-возврат перетаскиванием окна на направляющие главного окна НЕ
//     поддержан (возврат — кнопкой dock панели);
//   - размер плавающего окна фиксирован (ресайз рамкой не поддержан);
//   - double-click титлбара для возврата не реализован (кнопка dock его заменяет).

const (
	dockFloatMinW = 200 // мин. ширина оторванного окна панели (лог. пиксели)
	dockFloatMinH = 150 // мин. высота
)

// dockFloatHost вешает на панели менеджера хук OnFloatNative и ведёт жизненный
// цикл вторичных окон оторванных панелей.
type dockFloatHost struct {
	parent *Window
	mgr    *widget.DockManager

	mu     sync.Mutex
	floats map[*widget.DockPane]*floatingPane
}

// floatingPane — одна панель, показанная в собственном окне ОС.
type floatingPane struct {
	pane *widget.DockPane

	// Заполняются в create() (на UI-потоке).
	eng       *engine.Engine
	native    NativeWindow
	surf      *surface
	popupHost *popupHost

	// userOnState — пользовательский OnStateChanged панели, обёрнутый на время
	// показа в окне (восстанавливается при teardown).
	userOnState func(p *widget.DockPane)

	tornDown bool // ресурсы освобождены (идемпотентность teardown)
}

// EnableDockFloating включает отрыв панелей менеджера dm в отдельные нативные
// окна ОС. Вызывать ДО Run(). На бэкендах без поддержки owner-окон / маршалинга
// на UI-поток (Wayland/macOS) и в headless — no-op (панель флоатит в холсте).
func (win *Window) EnableDockFloating(dm *widget.DockManager) {
	win.dockMgr = dm
}

// pickupDeclarativeDockFloating подхватывает декларацию нативного отрыва из XAML
// (<DockManager NativeFloating="True">): обходит дерево корня, и если приложение
// НЕ вызвало EnableDockFloating явно (win.dockMgr == nil), включает отрыв для
// ПЕРВОГО найденного менеджера с NativeFloating=true. Хост отрыва (dock_host)
// держит один менеджер, поэтому при нескольких таких менеджерах остальные лишь
// логируются. Вызывается в Run() ДО installDockFloating (которая читает dockMgr).
func (win *Window) pickupDeclarativeDockFloating() {
	if win.dockMgr != nil {
		return // приложение уже задало явным EnableDockFloating — приоритет
	}
	root := win.eng.Root()
	if root == nil {
		return
	}
	var found []*widget.DockManager
	collectNativeFloatingDockManagers(root, &found)
	if len(found) == 0 {
		return
	}
	win.EnableDockFloating(found[0])
	if len(found) > 1 {
		log.Printf("window: %d DockManager с NativeFloating=true — нативный отрыв "+
			"включён только для первого (хост держит один менеджер)", len(found))
	}
}

// collectNativeFloatingDockManagers рекурсивно собирает по дереву все
// *widget.DockManager с NativeFloating=true (в порядке обхода сверху вниз).
func collectNativeFloatingDockManagers(w widget.Widget, out *[]*widget.DockManager) {
	if w == nil {
		return
	}
	if dm, ok := w.(*widget.DockManager); ok && dm.NativeFloating {
		*out = append(*out, dm)
	}
	// TabControl.Children() отдаёт только АКТИВНУЮ вкладку — обходим все вкладки
	// явно, иначе менеджер в неактивной вкладке (как в showcase) не был бы найден.
	if tc, ok := w.(*widget.TabControl); ok {
		for i := 0; i < tc.TabCount(); i++ {
			collectNativeFloatingDockManagers(tc.TabContent(i), out)
		}
		return
	}
	for _, c := range w.Children() {
		collectNativeFloatingDockManagers(c, out)
	}
}

// installDockFloating ставит хост отрыва панелей, если бэкенд поддерживает
// owner-окна и маршалинг на UI-поток. Вызывается из Run() после installPopupHost.
func (win *Window) installDockFloating() {
	if win.dockMgr == nil {
		return
	}
	if _, ok := win.native.(ownedWindow); !ok {
		return
	}
	if _, ok := win.native.(uiThreadInvoker); !ok {
		return
	}
	h := &dockFloatHost{
		parent: win,
		mgr:    win.dockMgr,
		floats: map[*widget.DockPane]*floatingPane{},
	}
	win.dockHost = h
	h.install()
}

// install навешивает OnFloatNative на текущие панели и подписывается на добавление
// новых через менеджерный OnPaneAdded (сохраняя ранее заданный колбэк).
func (h *dockFloatHost) install() {
	for _, p := range h.mgr.Panes() {
		h.arm(p)
	}
	prev := h.mgr.OnPaneAdded
	h.mgr.OnPaneAdded = func(p *widget.DockPane) {
		h.arm(p)
		if prev != nil {
			prev(p)
		}
	}
}

// arm вешает на панель хук отрыва в нативное окно.
func (h *dockFloatHost) arm(p *widget.DockPane) {
	if p == nil {
		return
	}
	p.OnFloatNative = func(pane *widget.DockPane) { h.onFloat(pane) }
}

// onFloat — DockPane.OnFloatNative: помечаем панель как оторванную и маршалим
// создание окна на UI-поток (безопасно из горутины движка).
func (h *dockFloatHost) onFloat(p *widget.DockPane) {
	h.mu.Lock()
	if _, exists := h.floats[p]; exists {
		h.mu.Unlock()
		return
	}
	fp := &floatingPane{pane: p}
	h.floats[p] = fp
	h.mu.Unlock()

	h.invoker().InvokeOnUIThread(func() { h.create(fp) })
}

// create строит вторичный движок и нативное окно для панели (на UI-потоке).
func (h *dockFloatHost) create(fp *floatingPane) {
	p := fp.pane
	scale := h.parent.scale
	if scale <= 0 {
		scale = 1
	}

	// Размер окна: последний размер панели (докнутый регион) с минимумом.
	b := p.Bounds()
	dw, dh := b.Dx(), b.Dy()
	if dw < dockFloatMinW {
		dw = dockFloatMinW
	}
	if dh < dockFloatMinH {
		dh = dockFloatMinH
	}

	fps := h.parent.maxFPS
	if fps <= 0 {
		fps = 60
	}
	eng := engine.New(dw, dh, fps)
	if scale != 1 {
		eng.SetScale(scale)
	}
	eng.SetResolution(dw, dh)
	pw, ph := eng.PhysicalSize()

	// Панель — корень вторичного движка (её титлбар = титлбар окна).
	p.SetBounds(image.Rect(0, 0, dw, dh))

	native := NewNativeWindow()
	surf := &surface{
		eng:     eng,
		native:  native,
		scale:   scale,
		current: image.NewRGBA(image.Rect(0, 0, pw, ph)),
	}

	if err := native.Create(p.Title, pw, ph); err != nil {
		// Окно не поднялось — снимаем регистрацию, останавливаем движок и
		// возвращаем панель в док (на её последнюю сторону).
		h.mu.Lock()
		delete(h.floats, p)
		h.mu.Unlock()
		go eng.Stop()
		p.OnDragMove = nil
		p.Dock(p.Side())
		return
	}
	native.SetResizable(false)
	native.SetMinSize(pw, ph)

	// Owner БЕЗ модальности: панель поверх главного окна и сворачивается с ним,
	// но НЕ блокирует его (SetEnabled(false) не вызываем).
	if ow, ok := native.(ownedWindow); ok {
		ow.SetOwner(h.parent.native)
	}
	h.positionNear(native, b, scale)

	// Сохраняем ресурсы в fp ДО подключения ввода/закрытия: teardown (который
	// может сработать сразу от ввода dock/✕) должен видеть native/eng.
	h.mu.Lock()
	fp.eng, fp.native, fp.surf = eng, native, surf
	h.mu.Unlock()

	// Drag за титлбар панели → перемещение нативного окна (лог → физ).
	p.OnDragMove = func(dx, dy int) {
		if scale != 1 {
			dx = int(float64(dx)*scale + 0.5)
			dy = int(float64(dy)*scale + 0.5)
		}
		x, y := native.GetPosition()
		native.SetPosition(x+dx, y+dy)
	}

	// Перехват выхода из плавающего состояния (кнопка dock → Dock, кнопка ✕ →
	// Close): менеджер уже провёл dockPane/closePane и вызвал OnStateChanged —
	// нам остаётся снести окно. Пользовательский OnStateChanged сохраняем и зовём.
	fp.userOnState = p.OnStateChanged
	p.OnStateChanged = func(pp *widget.DockPane) {
		if pp.State() != widget.PaneFloating {
			h.teardown(fp)
		}
		if fp.userOnState != nil {
			fp.userOnState(pp)
		}
	}

	// Закрытие окна ОС (Alt+F4 / ✕ рамки) — как закрытие панели.
	native.SetOnClose(func() bool {
		p.Close() // → mgr.closePane → OnStateChanged → teardown (снесёт окно)
		return false
	})

	// Панель как корень вторичного движка (bounds уже (0,0,dw,dh) — SetRoot их
	// не перезапишет). Инжектит вторичный движок как CaptureManager панели.
	eng.SetRoot(p)

	// Насос кадров и ввод вторичного окна.
	surf.setupInput()
	surf.setupExposeRedraw()
	eng.Start()
	go surf.framePump()

	// Хост popup-оверлеев вторичного движка: dropdown'ы/меню внутри панели
	// открываются в собственных окнах ОС, спозиционированных от окна панели.
	if _, ok := native.(popupWindow); ok {
		if inv, ok := native.(uiThreadInvoker); ok {
			pph := newPopupHost(native, inv, eng, scale)
			eng.SetPopupSink(pph.apply)
			h.mu.Lock()
			fp.popupHost = pph
			h.mu.Unlock()
			if an, ok := native.(activationNotifier); ok {
				an.SetOnActivate(func(active bool) {
					if !active {
						eng.CloseAllOverlays()
					}
				})
			}
		}
	}

	// Вторичное окно с собственным соединением (X11) — запускаем его насос
	// событий. Win32 не реализует eventPumper (одна очередь на все окна) — no-op.
	if ep, ok := native.(eventPumper); ok {
		ep.StartEventPump()
	}
}

// teardown освобождает ресурсы оторванной панели: снимает нативные хуки,
// восстанавливает главный CaptureManager, закрывает popup'ы, останавливает
// вторичный движок и уничтожает окно. К моменту вызова панель уже возвращена в
// док / закрыта менеджером (teardown вызывается ИЗ обёртки OnStateChanged).
// Идемпотентно (tornDown).
func (h *dockFloatHost) teardown(fp *floatingPane) {
	h.mu.Lock()
	if fp.tornDown {
		h.mu.Unlock()
		return
	}
	fp.tornDown = true
	delete(h.floats, fp.pane)
	p := fp.pane
	eng := fp.eng
	native := fp.native
	pph := fp.popupHost
	userOnState := fp.userOnState
	h.mu.Unlock()

	// Снимаем нативные хуки панели и восстанавливаем пользовательский колбэк
	// состояния (обёртка больше не нужна).
	p.OnDragMove = nil
	p.OnStateChanged = userOnState

	// Вторичный движок при eng.SetRoot(p) перезаписал CaptureManager панели на
	// себя. Вернём главный — иначе после возврата capture в главном холсте чинился
	// бы лишь при следующем клике (движок инжектит его захватчику лениво).
	if cm, ok := h.parent.eng.(widget.CaptureManager); ok {
		p.SetCaptureManager(cm)
	}

	// Окна popup-оверлеев панели (её движок останавливается — sink больше не
	// вызовется, сами они не закроются).
	if pph != nil {
		pph.closeAll()
	}

	// Останавливаем вторичный движок (framePump завершится по закрытию Frames();
	// Stop снимает регистрацию в реестре нотификаторов — без утечки).
	if eng != nil {
		go eng.Stop()
	}

	// Нативные операции — на UI-потоке.
	h.invoker().InvokeOnUIThread(func() {
		if native != nil {
			native.Close()
		}
		if fg, ok := h.parent.native.(foregrounder); ok {
			fg.SetForeground()
		}
	})
}

// teardownAll сносит все оторванные окна (при закрытии главного окна). Вызывается
// после возврата RunEventLoop — движки останавливаются, чтобы не текли горутины
// и реестр нотификаторов; owned-окна ОС уничтожает вместе с owner'ом.
func (h *dockFloatHost) teardownAll() {
	h.mu.Lock()
	fps := make([]*floatingPane, 0, len(h.floats))
	for _, fp := range h.floats {
		fps = append(fps, fp)
	}
	h.mu.Unlock()
	for _, fp := range fps {
		h.teardown(fp)
	}
}

// positionNear ставит окно панели туда, где она была в главном окне: клиентский
// угол главного окна + позиция панели (лог × scale).
func (h *dockFloatHost) positionNear(native NativeWindow, paneBounds image.Rectangle, scale float64) {
	px, py := h.parent.native.GetPosition()
	x := px + int(float64(paneBounds.Min.X)*scale+0.5)
	y := py + int(float64(paneBounds.Min.Y)*scale+0.5)
	native.SetPosition(x, y)
}

// invoker возвращает маршалер UI-потока главного окна.
func (h *dockFloatHost) invoker() uiThreadInvoker {
	return h.parent.native.(uiThreadInvoker)
}
