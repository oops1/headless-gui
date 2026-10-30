package core

// style.go — вид и поведение окна ОС (Style), владелец, Z-порядок и положение
// после показа (ADR-0013 §4; нужно окнам Tandem RemoteApp: без рамки,
// принадлежат главному окну, сами не появляются на панели задач, управляются
// извне по Z и положению).
//
//	              Win32                    X11                      Wayland  macOS
//	Borderless    без зон ресайза, DWM      без зон ресайза,         да       нет
//	              без скругления и тени     _GTK_FRAME_EXTENTS
//	                                       (тень композитора)
//	Owned         GWLP_HWNDPARENT          WM_TRANSIENT_FOR         нет*     нет
//	Tool          WS_EX_TOOLWINDOW         SKIP_TASKBAR,            нет      нет
//	                                       SKIP_PAGER, SKIP_SWITCHER
//	Raise, Lower  SetWindowPos без         ConfigureWindow          нет      нет
//	              активации                stack-mode
//	SetPosition   да                       да                       нет      нет
//	ответ         по итогу                 «отправлено»: решает     —        —
//	                                       оконный менеджер
//
// * У каждого окна Wayland своё соединение с компоновщиком, а номера объектов
// действуют только внутри соединения: xdg_toplevel.set_parent с окном другого
// соединения — ошибка протокола.
//
// Зоны ресайза у Borderless-окна снимает и ядро: на X11 и Wayland корпус
// виджетного окна заводит их по OnNativeResize (syncEdgeResize). У закрытого
// окна все операции отвечают false.
//
// Каждая операция честно отвечает, выполнена ли она (на X11 — отправлен ли
// запрос: решает оконный менеджер). До Run стиль и владелец запоминаются и
// закладываются в создание окна (окно показывается уже при создании, и
// стиль, применённый после, был бы виден переходом); тогда true значит
// «запомнено» — бэкенд Linux (Wayland или X11) выбирается только в Run,
// фактический итог показывает Style после запуска. На macOS стиль и владелец
// не запоминаются и не применяются: просьба отвечает false (нулевой стиль и
// снятие владельца — true, делать нечего).

import (
	"runtime"
	"sync"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget"
)

// styleState — стиль и владелец окна.
type styleState struct {
	mu sync.Mutex
	// style — Borderless и Tool: до Run — заказанные, после — действующие
	// (ответ бэкенда); Owned — есть владелец.
	style native.Style
	// owner — владелец (SetOwner); до Run — заказанный.
	owner *Window
	// owned — окна, которым это окно — владелец: при закрытии оно их
	// освобождает (releaseOwned).
	owned map[*Window]struct{}
	// live — окно ОС создано: releaseOwned может снять владельца у бэкенда.
	// sent — владелец, которого applyPendingStyle отдал бэкенду до Create;
	// снятый, пока окно создавалось, его снимает settleOwner.
	live bool
	sent *Window
	// moving — владелец в пути: SetOwner уже отдаёт его бэкенду, но ещё не
	// записал. releaseOwned прежнего владельца тогда бэкенд не трогает —
	// снятие стёрло бы нового.
	moving *Window
}

// liveOwnerLocked — владелец, если он не закрыт (под style.mu).
func (win *Window) liveOwnerLocked() *Window {
	if o := win.style.owner; o != nil && !o.closeRequested.Load() {
		return o
	}
	return nil
}

// setOwnerRef запоминает владельца и переносит окно из списка прежнего
// владельца в список нового. Замки окон берутся по одному. false — новый
// владелец закрылся, пока окно в него записывалось: его releaseOwned уже
// прошёл, и окно не запомнено.
func (win *Window) setOwnerRef(owner *Window) bool {
	win.style.mu.Lock()
	old := win.style.owner
	win.style.owner = owner
	win.style.moving = nil
	win.style.mu.Unlock()
	if old != nil && old != owner {
		old.style.mu.Lock()
		delete(old.style.owned, win)
		old.style.mu.Unlock()
	}
	if owner == nil {
		return true
	}
	owner.style.mu.Lock()
	if owner.style.owned == nil {
		owner.style.owned = map[*Window]struct{}{}
	}
	owner.style.owned[win] = struct{}{}
	// Close ставит признак до того, как releaseOwned берёт список: либо окно
	// уже в его снимке, либо признак виден здесь.
	closed := owner.closeRequested.Load()
	if closed {
		delete(owner.style.owned, win)
	}
	owner.style.mu.Unlock()
	if closed {
		win.style.mu.Lock()
		if win.style.owner == owner {
			win.style.owner = nil
		}
		win.style.mu.Unlock()
		return false
	}
	return true
}

// releaseOwned освобождает окна, которыми владеет закрывающееся окно (Close и
// закрытие средствами ОС, до уничтожения окна ОС): они остаются открытыми, уже
// без владельца. Иначе Windows уничтожила бы их вместе с владельцем без
// WM_CLOSE — мимо ядра и SetOnCloseRequest, а на X11 их WM_TRANSIENT_FOR
// указывал бы на уничтоженное окно, номер которого сервер может отдать новому.
func (win *Window) releaseOwned() {
	win.style.mu.Lock()
	list := make([]*Window, 0, len(win.style.owned))
	for o := range win.style.owned {
		list = append(list, o)
	}
	win.style.owned = nil
	win.style.mu.Unlock()
	for _, o := range list {
		o.style.mu.Lock()
		mine := o.style.owner == win
		if mine {
			o.style.owner = nil
		}
		// Окно ОС ещё создаётся: бэкенд трогать нельзя (поток окна читает
		// заказанного владельца), снимет settleOwner после Create. Новый
		// владелец в пути (SetOwner): снятие у бэкенда стёрло бы его.
		live := o.style.live
		moving := o.style.moving
		o.style.mu.Unlock()
		if !mine || !live || moving != nil && moving != win || o.closeRequested.Load() {
			continue
		}
		if setter, ok := o.native.(native.OwnerSetter); ok {
			setter.SetOwnerWindow(nil)
		}
	}
}

// settleOwner — после Create: окно ОС создано (live). Владелец, отданный
// бэкенду до создания, мог закрыться, пока окно создавалось (releaseOwned
// бэкенд тогда не трогал): снимаем его у живого окна.
func (win *Window) settleOwner() {
	win.style.mu.Lock()
	win.style.live = true
	lost := win.style.sent != nil && win.liveOwnerLocked() != win.style.sent
	win.style.sent = nil
	win.style.mu.Unlock()
	if !lost {
		return
	}
	win.setOwnerRef(nil)
	if setter, ok := win.native.(native.OwnerSetter); ok {
		setter.SetOwnerWindow(nil)
	}
}

// styleBeforeRun — ответ на просьбу, сделанную до Run: «запомнено» везде,
// кроме macOS, где ни стиля, ни владельца бэкенд не умеет.
func styleBeforeRun() bool { return runtime.GOOS != "darwin" }

// SetStyle задаёт вид и поведение окна ОС (Style). true — стиль применён
// целиком; false — часть стиля оконная система не умеет (её нет в Style после
// вызова), Owned=true, а владельца нет (владельца задаёт SetOwner;
// Owned=false снимает его), или окно уже закрыто.
func (win *Window) SetStyle(s native.Style) bool {
	if win.native != nil && win.closeRequested.Load() {
		return false
	}
	ok := true
	win.style.mu.Lock()
	owned := win.liveOwnerLocked() != nil
	win.style.mu.Unlock()
	if s.Owned != owned {
		if s.Owned || !win.SetOwner(nil) {
			ok = false
		}
	}
	want := native.Style{Borderless: s.Borderless, Tool: s.Tool}
	if win.native == nil {
		got := want
		if !styleBeforeRun() {
			got = native.Style{} // macOS: запоминать нечего, Style не обещает
		}
		win.style.mu.Lock()
		win.style.style.Borderless, win.style.style.Tool = got.Borderless, got.Tool
		win.style.mu.Unlock()
		return ok && got == want
	}
	got := native.Style{}
	if st, has := win.native.(native.Styler); has {
		got = st.ApplyStyle(want)
	}
	got.Owned = false
	win.style.mu.Lock()
	win.style.style.Borderless, win.style.style.Tool = got.Borderless, got.Tool
	win.style.mu.Unlock()
	// Скругление ОС и зоны ресайза зависят от Borderless: корпус виджетного
	// окна должен знать, рисоваться ли ему с прямыми углами и есть ли у него
	// края (горутина движка).
	win.post(func() {
		win.syncSquareCorners()
		win.syncEdgeResize()
	})
	return ok && got == want
}

// Style — вид и поведение окна ОС: до Run — заказанные, после — те, что
// действуют.
func (win *Window) Style() native.Style {
	win.style.mu.Lock()
	defer win.style.mu.Unlock()
	s := win.style.style
	s.Owned = win.liveOwnerLocked() != nil
	return s
}

// SetOwner делает окно принадлежащим owner (Style.Owned): оно всегда над
// owner по Z, сворачивается и восстанавливается вместе с ним, а владелец
// остаётся доступен (в отличие от владельца модального диалога). nil снимает
// владельца. false — оконная система этого не умеет (Wayland, macOS), owner —
// само окно или его потомок по владению, или owner ещё не показан, когда
// показано окно, одно из окон уже закрыто или (Windows) окно не встало над
// владельцем либо поток окна-владельца не ответил вовремя. На Windows у владельца «поверх всех» окно само становится
// «поверх всех»: так устроены окна с владельцем.
//
// Закрытие владельца окно освобождает: оно остаётся открытым, уже без
// владельца (Owned=false), на всех ОС одинаково. Дочерние окна OpenWindow
// закрываются вместе с открывшим их окном и так.
//
// До Run (и до OpenWindow для дочернего окна) владелец запоминается и
// закладывается в создание окна; owner к этому времени должен быть показан.
func (win *Window) SetOwner(owner *Window) bool {
	for o := owner; o != nil; {
		if o == win {
			return false
		}
		o.style.mu.Lock()
		next := o.style.owner
		o.style.mu.Unlock()
		o = next
	}
	if owner != nil && owner.closeRequested.Load() {
		return false
	}
	if win.native == nil {
		if owner != nil && !styleBeforeRun() {
			return false // macOS: запоминать нечего
		}
		return win.setOwnerRef(owner)
	}
	if win.closeRequested.Load() {
		return false
	}
	setter, has := win.native.(native.OwnerSetter)
	if !has {
		// Снимать нечего: владельца бэкенд без возможности не получал.
		return owner == nil
	}
	var on native.NativeWindow
	if owner != nil {
		if on = owner.native; on == nil {
			return false
		}
	}
	win.style.mu.Lock()
	prev := win.style.owner
	win.style.moving = owner
	win.style.mu.Unlock()
	if !setter.SetOwnerWindow(on) {
		win.style.mu.Lock()
		win.style.moving = nil
		// Прежний владелец закрылся, пока новый ставился: его releaseOwned
		// бэкенд не тронул (новый был в пути), а новый не встал — снимаем
		// прежнего сами.
		stale := prev != nil && win.style.owner == nil
		win.style.mu.Unlock()
		if stale {
			setter.SetOwnerWindow(nil)
		}
		return false
	}
	if !win.setOwnerRef(owner) {
		setter.SetOwnerWindow(nil) // владелец закрылся, пока его ставили
		return false
	}
	return true
}

// Raise поднимает окно наверх Z-порядка без активации (фокус остаётся, где
// был). false — окна ещё нет, оконная система этого не умеет или окно
// наверх не встало: Windows не пускает окно фонового процесса над активным
// окном другого процесса. На X11 решает оконный менеджер: mutter подъём окна
// другого приложения при активном чужом отклоняет и вместо него помечает окно
// «требует внимания» (у окна без Tool GNOME Shell покажет уведомление), а
// ответ остаётся true — запрос отправлен.
func (win *Window) Raise() bool {
	if sk, ok := win.native.(native.Stacker); ok && !win.closeRequested.Load() {
		return sk.RaiseWindow()
	}
	return false
}

// Lower опускает окно вниз Z-порядка без активации. false — окна ещё нет,
// оконная система этого не умеет или окно поверх всех (AlwaysOnTop) на
// Windows: опустить его значило бы молча снять этот режим. Окно с владельцем
// остаётся над владельцем.
func (win *Window) Lower() bool {
	if sk, ok := win.native.(native.Stacker); ok && !win.closeRequested.Load() {
		return sk.LowerWindow()
	}
	return false
}

// SetPosition ставит показанное окно левым верхним углом в (x, y) — экранные
// физические координаты, как у SetInitialBounds и Monitor.Bounds. false —
// окно ещё не показано (до Run положение задаёт SetInitialBounds), развёрнуто,
// на весь экран или свёрнуто, оконная система положения окна не задаёт
// (Wayland) или окно уже закрыто.
func (win *Window) SetPosition(x, y int) bool {
	if p, ok := win.native.(native.Positioner); ok && !win.closeRequested.Load() {
		return p.MoveTo(x, y)
	}
	return false
}

// Position — положение показанного окна (левый верхний угол, экранные
// физические координаты). ok=false — окно ещё не показано, свёрнуто, уже
// закрыто, или оконная система положения не сообщает (Wayland).
func (win *Window) Position() (x, y int, ok bool) {
	if p, has := win.native.(native.Positioner); has && !win.closeRequested.Load() {
		return p.ScreenPosition()
	}
	return 0, 0, false
}

// applyPendingStyle отдаёт бэкенду стиль и владельца, заказанные до Run, ДО
// создания окна ОС: окно показывается уже в Create.
func (win *Window) applyPendingStyle() {
	win.style.mu.Lock()
	want := native.Style{Borderless: win.style.style.Borderless, Tool: win.style.style.Tool}
	owner := win.style.owner
	win.style.mu.Unlock()
	got := native.Style{}
	if st, ok := win.native.(native.Styler); ok && want != (native.Style{}) {
		got = st.ApplyStyle(want)
	}
	if owner != nil {
		setter, ok := win.native.(native.OwnerSetter)
		if !ok || owner.native == nil || owner.closeRequested.Load() || !setter.SetOwnerWindow(owner.native) {
			win.setOwnerRef(nil)
		} else {
			win.style.mu.Lock()
			win.style.sent = owner
			win.style.mu.Unlock()
		}
	}
	win.style.mu.Lock()
	win.style.style.Borderless, win.style.style.Tool = got.Borderless, got.Tool
	win.style.mu.Unlock()
}

// syncEdgeResize даёт корпусу виджетного окна зоны ресайза по краям, только
// пока окно не Borderless. Зоны виджет заводит по наличию OnNativeResize (его
// ставят бэкенды с interactiveMover — X11, Wayland), и отказа BeginResize
// мало: у края остались бы курсор ресайза и нажатие, которое ничего не
// делает. Горутина движка; из bringUp — до setupInput и насоса событий:
// движок уже может работать, но ввода, читающего OnNativeResize, ещё нет.
func (win *Window) syncEdgeResize() {
	mv, ok := win.native.(interactiveMover)
	if !ok || win.eng == nil {
		return
	}
	ww, ok := win.eng.Root().(*widget.Window)
	if !ok {
		return
	}
	switch {
	case win.Style().Borderless && ww.OnNativeResize != nil:
		ww.OnNativeResize = nil
	case !win.Style().Borderless && ww.OnNativeResize == nil:
		ww.OnNativeResize = mv.BeginResize
	default:
		return
	}
	ww.Invalidate()
}
