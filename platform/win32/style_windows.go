package win32

// style_windows.go — стиль окна (native.Styler), владелец (native.OwnerSetter),
// Z-порядок (native.Stacker) и положение с честным ответом (native.Positioner),
// ADR-0013 §4.
//
//	Borderless  зоны ресайза не отдаются (WM_NCHITTEST), DWM не рисует
//	            неклиентскую часть (DWMWA_NCRENDERING_POLICY): ни скругления,
//	            ни тени, ни рамки в 1 px Windows 10 (applyCorners)
//	Tool        WS_EX_TOOLWINDOW вместо WS_EX_APPWINDOW: без кнопки на панели
//	            задач и вне Alt+Tab
//	владелец    GWLP_HWNDPARENT (до Create — hWndParent), без модальности;
//	            у владельца «поверх всех» окно тоже становится «поверх всех»
//	Z-порядок   SetWindowPos(HWND_TOP / HWND_BOTTOM) без активации
//
// Ответы — по итогу, а не по коду возврата: окно фонового процесса Windows
// молча оставляет под активным окном другого процесса, хотя SetWindowPos
// отвечает успехом.

import (
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/oops1/headless-gui/v4/platform/native"
)

const (
	hwndTop    = 0 // HWND_TOP
	hwndBottom = 1 // HWND_BOTTOM
	gwHwndprev = 3 // GW_HWNDPREV — окно над данным в Z-порядке
	gwOwner    = 4 // GW_OWNER — владелец
	swShowNA   = 8 // SW_SHOWNA — показать, не активируя

	dwmwaNcrenderingPolicy = 2 // DWMWA_NCRENDERING_POLICY
	dwmncrpUseWindowStyle  = 0 // DWMNCRP_USEWINDOWSTYLE
	dwmncrpDisabled        = 1 // DWMNCRP_DISABLED
)

var (
	procGetWindow             = user32.NewProc("GetWindow")
	procIsWindow              = user32.NewProc("IsWindow")
	procDwmGetWindowAttribute = dwmapi.NewProc("DwmGetWindowAttribute")
)

// styleUITimeout — сколько ApplyStyle и SetOwnerWindow (callOnUI) ждут поток
// окна (как kbGrabUITimeout у захвата клавиатуры); styleUIPoll — как часто
// ожидание проверяет, живо ли окно (вызов уничтоженного окна выбрасывается, и
// ответа не будет).
// Переменные — для тестов.
var (
	styleUITimeout = 3 * time.Second
	styleUIPoll    = 10 * time.Millisecond
)

// hwndAlive — окно с этим номером существует (IsWindow): у закрытого окна
// Win32Window.hwnd остаётся прежним.
func hwndAlive(h uintptr) bool {
	r, _, _ := procIsWindow.Call(h)
	return r != 0
}

// win32Style — стиль и владелец окна.
type win32Style struct {
	// borderless — Style.Borderless. Атомарно: WM_NCHITTEST читает поток
	// окна, RoundsCorners — горутина движка.
	borderless atomic.Bool
	// tool — Style.Tool, заказанный до Create; у живого окна его хранит
	// WS_EX_TOOLWINDOW.
	tool atomic.Bool
	// owner — владелец, заказанный до Create: Create кладёт его в hWndParent.
	owner *Win32Window
	// hiddenByOwner — окно спрятал свёрнутый владелец (WM_SHOWWINDOW с
	// SW_PARENTCLOSING): показать его обратно может только этот же владелец,
	// и окно, у которого его сняли, показывается само (revealIfHiddenByOwner).
	hiddenByOwner atomic.Bool
	// ownerSet — владельца поставил SetOwnerWindow (живому окну или в Create —
	// hWndParent), а не SetOwner модального окна: WM_CLOSE владельца снимает
	// его перед DestroyWindow.
	ownerSet atomic.Bool
}

var (
	_ native.Styler      = (*Win32Window)(nil)
	_ native.OwnerSetter = (*Win32Window)(nil)
	_ native.Stacker     = (*Win32Window)(nil)
	_ native.Positioner  = (*Win32Window)(nil)
)

// createExStyle — расширенный стиль главного окна при создании (Create).
func (w *Win32Window) createExStyle() uint32 {
	if w.style.tool.Load() {
		return wsExToolwindow
	}
	return wsExAppwindow
}

// createOwner — hWndParent для CreateWindowExW: владелец, заказанный до
// Create (ноль — без владельца, и у попапов). Владелец, закрытый с тех пор,
// не передаётся: с недействительным hWndParent окно не создалось бы вовсе.
func (w *Win32Window) createOwner() uintptr {
	o := w.style.owner
	w.style.owner = nil
	if o == nil || !hwndAlive(uintptr(o.hwnd)) {
		return 0
	}
	return uintptr(o.hwnd)
}

// ApplyStyle реализует native.Styler. Живое окно меняется на своём потоке;
// Borderless и Tool Windows умеет оба, поэтому ответ — то, что действует
// после вызова (Tool — по WS_EX_TOOLWINDOW, который окно на деле получило).
//
// Поток окна не ответил за styleUITimeout, или окно уничтожено, пока вызов
// ждал очереди: вызов, ещё не начатый, отменяется (позже стиль не сменится
// вопреки ответу), начатый — дожидается своего итога.
func (w *Win32Window) ApplyStyle(s native.Style) native.Style {
	if w.hwnd == 0 {
		w.style.borderless.Store(s.Borderless)
		w.style.tool.Store(s.Tool)
		return native.Style{Borderless: s.Borderless, Tool: s.Tool}
	}
	return callOnUI(w, func() native.Style { return w.applyStyleLive(s.Borderless, s.Tool) }, w.liveStyle)
}

// callOnUI выполняет fn на потоке окна ui и дожидается итога. Поток не
// ответил за styleUITimeout, или окно ui уничтожено, пока вызов ждал очереди
// (вызовы мёртвого окна выбрасываются): вызов, ещё не начатый, отменяется —
// позже он не выполнится вопреки ответу, и ответ тогда cancelled(); начатый
// дожидается своего итога.
func callOnUI[T any](ui *Win32Window, fn func() T, cancelled func() T) T {
	res := make(chan T, 1)
	var state atomic.Int32 // 0 — ждёт очереди, 1 — выполняется, 2 — отменён
	run := func() {
		if state.CompareAndSwap(0, 1) {
			res <- fn()
		}
	}
	if ui.onUIThread() {
		run()
		return <-res
	}
	if !ui.postUICall(run) {
		return cancelled()
	}
	deadline := time.NewTimer(styleUITimeout)
	defer deadline.Stop()
	poll := time.NewTicker(styleUIPoll)
	defer poll.Stop()
	for {
		select {
		case got := <-res:
			return got
		case <-poll.C:
			if hwndAlive(uintptr(ui.hwnd)) {
				continue
			}
		case <-deadline.C:
		}
		if state.CompareAndSwap(0, 2) {
			return cancelled()
		}
		return <-res
	}
}

// liveStyle — стиль, который действует у живого окна.
func (w *Win32Window) liveStyle() native.Style {
	ex, _, _ := procGetWindowLongPtrW.Call(uintptr(w.hwnd), gwlExstyleAOT)
	return native.Style{Borderless: w.style.borderless.Load(), Tool: ex&wsExToolwindow != 0}
}

// applyStyleLive меняет стиль живого окна. Только поток окна.
func (w *Win32Window) applyStyleLive(borderless, tool bool) native.Style {
	if w.style.borderless.Swap(borderless) != borderless {
		w.applyBorderless(borderless)
	}
	hwnd := uintptr(w.hwnd)
	ex, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlExstyleAOT)
	want := ex &^ (wsExToolwindow | wsExAppwindow)
	if tool {
		want |= wsExToolwindow
	} else {
		want |= wsExAppwindow
	}
	if want != ex {
		// Панель задач замечает смену стиля только при показе окна: видимое
		// окно прячется на время смены (так советует MSDN, «Managing Taskbar
		// Buttons»). Активное окно показывается активным, остальные — нет и
		// возвращаются на своё место в Z-порядке: показ ставит окно наверх.
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		fg, _, _ := procGetForegroundWindow.Call()
		prev := foreignAbove(hwnd, hwnd)
		owned := ownedAbove(hwnd, prev)
		if visible != 0 {
			procShowWindow.Call(hwnd, swHide)
		}
		procSetWindowLongPtrW.Call(hwnd, gwlExstyleAOT, want)
		procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0,
			swpNomove|swpNosize|swpNozorder|swpNoactivate|swpFramechanged)
		switch {
		case visible == 0:
		case fg == hwnd:
			procShowWindow.Call(hwnd, swShow)
		default:
			procShowWindow.Call(hwnd, swShowNA)
			after := prev
			if after == 0 || isTopmostHWND(after) && !isTopmostHWND(hwnd) {
				after = hwndTop // окно было наверху своего слоя
			}
			insertGroup(hwnd, after, owned)
		}
	}
	w.style.tool.Store(tool)
	return w.liveStyle()
}

// foreignAbove — ближайшее окно над start, которое не hwnd и не принадлежит
// hwnd (его окна с владельцем, окно IME в том числе, переставляются вместе с
// ним: insertGroup); 0 — таких нет.
func foreignAbove(start, hwnd uintptr) uintptr {
	for h := start; ; {
		if h, _, _ = procGetWindow.Call(h, gwHwndprev); h == 0 || h != hwnd && !ownedBy(h, hwnd) {
			return h
		}
	}
}

// ownedAbove — окна слоя hwnd, принадлежащие ему и лежащие над ним до окна
// stop (не включая; 0 — до самого верха), от ближнего к hwnd.
func ownedAbove(hwnd, stop uintptr) []uintptr {
	top := isTopmostHWND(hwnd)
	var list []uintptr
	for h := hwnd; ; {
		if h, _, _ = procGetWindow.Call(h, gwHwndprev); h == 0 || h == stop {
			return list
		}
		if ownedBy(h, hwnd) && isTopmostHWND(h) == top {
			list = append(list, h)
		}
	}
}

// insertGroup ставит hwnd сразу под after, а над ним — его окна owned (от
// ближнего к hwnd), в прежнем порядке. SWP_NOOWNERZORDER: владельца самого
// hwnd перестановка не трогает, а окна, принадлежащие hwnd, с этим флагом
// Windows сама не поднимает — их ставит этот же вызов.
func insertGroup(hwnd, after uintptr, owned []uintptr) {
	const flags = swpNomove | swpNosize | swpNoactivate | swpNoownerzorder
	procSetWindowPos.Call(hwnd, after, 0, 0, 0, 0, flags)
	for _, o := range owned {
		procSetWindowPos.Call(o, after, 0, 0, 0, 0, flags)
	}
}

// applyBorderless включает или выключает неклиентскую отрисовку DWM: без неё
// у окна нет ни тени, ни скругления, ни рамки. Только поток окна (или до
// показа, в createInternal).
func (w *Win32Window) applyBorderless(on bool) {
	policy := int32(dwmncrpUseWindowStyle)
	if on {
		policy = dwmncrpDisabled
		// Рамка в 1 px, которой applyCorners включает тень на Windows 10.
		var m dwmMargins
		procDwmExtendFrameIntoClientArea.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&m)))
	}
	procDwmSetWindowAttribute.Call(uintptr(w.hwnd), dwmwaNcrenderingPolicy,
		uintptr(unsafe.Pointer(&policy)), unsafe.Sizeof(policy))
	w.applyCorners()
}

// SetOwnerWindow реализует native.OwnerSetter: GWLP_HWNDPARENT, без
// модальности и без пометки вторичным (в отличие от SetOwner модального
// диалога). Окно, оказавшееся ниже нового владельца, встаёт сразу над ним;
// не встало — владелец возвращается прежний, ответ false. Закрытый владелец
// не принимается.
//
// Владелец ставится на его потоке: там же WM_CLOSE владельца снимает его у
// своих окон перед DestroyWindow (releaseOwnedOnClose), и установка не
// вклинится между снятием и уничтожением — окно не погибнет вместе с
// закрывающимся владельцем. Поток владельца не ответил за styleUITimeout, или
// владелец уничтожен, пока вызов ждал очереди: не начатый вызов отменяется,
// ответ false.
func (w *Win32Window) SetOwnerWindow(owner native.NativeWindow) bool {
	var pw *Win32Window
	if owner != nil {
		var ok bool
		if pw, ok = owner.(*Win32Window); !ok || pw == w || pw.hwnd == 0 || !hwndAlive(uintptr(pw.hwnd)) {
			return false
		}
	}
	if w.hwnd == 0 {
		w.style.owner = pw
		return true
	}
	if pw == nil {
		return w.setOwnerLive(nil)
	}
	return callOnUI(pw, func() bool { return w.setOwnerLive(pw) }, func() bool { return false })
}

// setOwnerLive ставит владельца живому окну (nil — снимает). С владельцем —
// на потоке владельца (SetOwnerWindow).
func (w *Win32Window) setOwnerLive(pw *Win32Window) bool {
	hwnd := uintptr(w.hwnd)
	var ph uintptr
	if pw != nil {
		if !hwndAlive(uintptr(pw.hwnd)) {
			return false // закрылся, пока вызов ждал его потока
		}
		ph = uintptr(pw.hwnd)
	}
	was, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlpHwndParent)
	wasSet := w.style.ownerSet.Load()
	w.style.ownerSet.Store(ph != 0)
	procSetWindowLongPtrW.Call(hwnd, gwlpHwndParent, ph)
	if got, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlpHwndParent); got != ph {
		w.style.ownerSet.Store(wasSet)
		return false
	}
	if ph != 0 && !w.placeAboveOwner(ph) {
		procSetWindowLongPtrW.Call(hwnd, gwlpHwndParent, was)
		w.style.ownerSet.Store(wasSet)
		return false
	}
	w.postUICall(w.revealIfHiddenByOwner)
	return true
}

// releaseOwnedOnClose снимает это окно владельцем у окон, которым его
// поставил SetOwnerWindow, прямо перед DestroyWindow (WM_CLOSE, поток окна):
// иначе Windows уничтожила бы их вместе с ним без WM_CLOSE. Ядро снимает их
// раньше (releaseOwned), а здесь — страховка от SetOwner, пришедшего в то же
// время: он выполняется на этом же потоке. Окна модальных диалогов (SetOwner
// модального окна) гибнут вместе с владельцем, как прежде.
func (w *Win32Window) releaseOwnedOnClose() {
	hwnd := uintptr(w.hwnd)
	win32Mu.Lock()
	var list []*Win32Window
	for _, o := range win32Windows {
		if o != w && o.style.ownerSet.Load() {
			list = append(list, o)
		}
	}
	win32Mu.Unlock()
	for _, o := range list {
		if cur, _, _ := procGetWindow.Call(uintptr(o.hwnd), gwOwner); cur != hwnd {
			continue
		}
		procSetWindowLongPtrW.Call(uintptr(o.hwnd), gwlpHwndParent, 0)
		o.style.ownerSet.Store(false)
		o.postUICall(o.revealIfHiddenByOwner)
	}
}

// revealIfHiddenByOwner показывает окно (без активации), если его спрятал
// свёрнутый владелец, а нынешний этого уже не сделает: его нет или он не
// свёрнут. Окно возвращается в прежнем состоянии (свёрнутое — свёрнутым), а
// его собственные окна Windows ведёт сама: свёрнутый владелец их не прячет,
// свёрнутое окно прячет и вернёт при своём восстановлении. Поток окна (через
// postUICall — после WM_SHOWWINDOW, которые уже в пути): решение и показ не
// расходятся со сворачиванием владельца.
func (w *Win32Window) revealIfHiddenByOwner() {
	if !w.style.hiddenByOwner.Load() || w.hwnd == 0 {
		return
	}
	hwnd := uintptr(w.hwnd)
	if v, _, _ := procIsWindowVisible.Call(hwnd); v != 0 {
		w.style.hiddenByOwner.Store(false)
		return
	}
	if o, _, _ := procGetWindow.Call(hwnd, gwOwner); o != 0 {
		if r, _, _ := procIsIconic.Call(o); r != 0 {
			return // покажет восстановление владельца
		}
	}
	w.style.hiddenByOwner.Store(false)
	procShowWindow.Call(hwnd, swShowNA)
}

// noteShowWindow запоминает, спрятал ли окно его владелец (WM_SHOWWINDOW):
// lParam SW_PARENTCLOSING — владелец свёрнут; любой показ снимает отметку.
func (w *Win32Window) noteShowWindow(show bool, reason uintptr) {
	const swParentClosing = 1
	switch {
	case show:
		w.style.hiddenByOwner.Store(false)
	case reason == swParentClosing:
		w.style.hiddenByOwner.Store(true)
	}
}

// placeAboveOwner ставит окно в Z-порядке сразу над владельцем, если оно
// ниже: окно с владельцем Windows держит над ним, но порядок, сложившийся до
// смены владельца, исправляет лишь при следующем подъёме владельца. Ответ —
// окно над владельцем.
func (w *Win32Window) placeAboveOwner(owner uintptr) bool {
	hwnd := uintptr(w.hwnd)
	if zAbove(hwnd, owner) {
		return true
	}
	// Новое место — под окном, что над владельцем, то есть прямо над ним, и
	// туда же — окна, принадлежащие самому hwnd (над ним). Обычный владелец
	// под окном «поверх всех» — наверху обычных окон: туда (HWND_TOP) и
	// встаёт окно. У владельца «поверх всех» окно тоже становится «поверх
	// всех», как Windows поступает с окнами такого владельца сама.
	owned := ownedAbove(hwnd, 0)
	after := foreignAbove(owner, hwnd)
	ownerTop := isTopmostHWND(owner)
	switch {
	case after == 0 && ownerTop:
		after = hwndTopmost
	case after == 0, isTopmostHWND(after) && !ownerTop:
		after = hwndTop
	}
	insertGroup(hwnd, after, owned)
	return zAbove(hwnd, owner)
}

// zAbove — upper выше lower в Z-порядке.
func zAbove(upper, lower uintptr) bool {
	for h := lower; ; {
		if h, _, _ = procGetWindow.Call(h, gwHwndprev); h == 0 {
			return false
		}
		if h == upper {
			return true
		}
	}
}

// isTopmostHWND — у окна WS_EX_TOPMOST.
func isTopmostHWND(h uintptr) bool {
	ex, _, _ := procGetWindowLongPtrW.Call(h, gwlExstyleAOT)
	return ex&wsExTopmost != 0
}

// RaiseWindow реализует native.Stacker: наверх Z-порядка (окно «поверх
// всех» — наверх своего слоя) без активации. Ответ — по тому, где окно
// оказалось: окно фонового процесса Windows молча оставляет под активным
// окном другого процесса, хотя SetWindowPos отвечает успехом.
func (w *Win32Window) RaiseWindow() bool {
	if w.hwnd == 0 {
		return false
	}
	hwnd := uintptr(w.hwnd)
	r, _, _ := procSetWindowPos.Call(hwnd, hwndTop, 0, 0, 0, 0,
		swpNomove|swpNosize|swpNoactivate)
	return r != 0 && topOfLayer(hwnd)
}

// topOfLayer — над окном нет видимых окон его слоя (обычных или «поверх
// всех»), кроме принадлежащих ему самому: те Windows держит над владельцем.
func topOfLayer(hwnd uintptr) bool {
	for h := hwnd; ; {
		if h, _, _ = procGetWindow.Call(h, gwHwndprev); h == 0 {
			return true
		}
		if blocksTop(hwnd, h) {
			return false
		}
	}
}

// blocksTop — окно h, лежащее над hwnd, мешает ему быть верхним: видимое,
// того же слоя и не принадлежит hwnd.
func blocksTop(hwnd, h uintptr) bool {
	return isTopmostHWND(h) == isTopmostHWND(hwnd) && shownHWND(h) && !ownedBy(h, hwnd)
}

// shownHWND — окно видно на экране: показано и не спрятано DWM (окна других
// рабочих столов и приостановленных приложений Магазина показаны, но
// «закрыты» — DWMWA_CLOAKED).
func shownHWND(h uintptr) bool {
	if v, _, _ := procIsWindowVisible.Call(h); v == 0 {
		return false
	}
	const dwmwaCloaked = 14
	var cloaked uint32
	r, _, _ := procDwmGetWindowAttribute.Call(h, dwmwaCloaked,
		uintptr(unsafe.Pointer(&cloaked)), unsafe.Sizeof(cloaked))
	return r != 0 || cloaked == 0
}

// ownedBy — h принадлежит owner (прямо или через цепочку владельцев).
func ownedBy(h, owner uintptr) bool {
	for {
		if h, _, _ = procGetWindow.Call(h, gwOwner); h == 0 {
			return false
		}
		if h == owner {
			return true
		}
	}
}

// LowerWindow реализует native.Stacker: вниз Z-порядка без активации. Окно
// «поверх всех» не опускается (false): HWND_BOTTOM молча снял бы с него
// WS_EX_TOPMOST.
func (w *Win32Window) LowerWindow() bool {
	if w.hwnd == 0 || isTopmostHWND(uintptr(w.hwnd)) {
		return false
	}
	r, _, _ := procSetWindowPos.Call(uintptr(w.hwnd), hwndBottom, 0, 0, 0, 0,
		swpNomove|swpNosize|swpNoactivate)
	return r != 0
}

// MoveTo реализует native.Positioner. У развёрнутого, свёрнутого и
// полноэкранного окна место задаёт его состояние — false (SetPosition в этих
// случаях откладывает положение до выхода из состояния или двигает окно,
// которое тут же вернётся на место).
func (w *Win32Window) MoveTo(x, y int) bool {
	if w.hwnd == 0 || w.fs.on.Load() {
		return false
	}
	hwnd := uintptr(w.hwnd)
	if r, _, _ := procIsIconic.Call(hwnd); r != 0 {
		return false
	}
	if r, _, _ := procIsZoomed.Call(hwnd); r != 0 {
		return false
	}
	r, _, _ := procSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0,
		swpNosize|swpNozorder|swpNoactivate)
	return r != 0
}

// ScreenPosition реализует native.Positioner. У свёрнутого окна положения
// нет: GetWindowRect отвечает служебным (-32000, -32000).
func (w *Win32Window) ScreenPosition() (x, y int, ok bool) {
	if w.hwnd == 0 {
		return 0, 0, false
	}
	hwnd := uintptr(w.hwnd)
	if r, _, _ := procIsIconic.Call(hwnd); r != 0 {
		return 0, 0, false
	}
	var r rect
	if ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ret == 0 {
		return 0, 0, false
	}
	return int(r.Left), int(r.Top), true
}
