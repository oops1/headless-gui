//go:build windows

package core

// pointerlock_windows.go — Win32: захват указателя (Window.SetPointerLock).
//
// Сдвиги берутся из Raw Input: RegisterRawInputDevices(мышь, usage page 1,
// usage 2) на окно, дальше система присылает ему WM_INPUT, а в нём
// RAWMOUSE.lLastX/lLastY — сдвиг в единицах устройства, без ускорения указателя
// (MOUSE_MOVE_RELATIVE). RIDEV_INPUTSINK не нужен: сдвиги нужны, только пока
// окно на переднем плане, а при потере фокуса захват всё равно снимается.
//
// Указатель при захвате прячется (WM_SETCURSOR отдаёт «нет курсора») и
// зажимается ClipCursor. Зажимается в ОДНУ точку — центр клиентской области,
// а не во всю область: окна здесь без рамки, зоны изменения размера лежат по
// краям самой клиентской области (WM_NCHITTEST), и указатель, упёршийся в край
// при зажатии во всю область, начинал бы тянуть размер окна. Сырой ввод от
// зажима не зависит — сдвиги идут сколько бы устройство ни двигали.
//
// Фокус. WM_ACTIVATE (неактивно) снимает зажим и возвращает курсор, активация —
// ставит обратно; желание приложения (want) при этом не теряется.

import (
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/oops1/headless-gui/v3/platform/pointer"
)

const (
	wmInput = 0x00FF // WM_INPUT
	wmMove  = 0x0003 // WM_MOVE

	ridInput        = 0x10000003 // RID_INPUT
	ridevRemove     = 0x00000001 // RIDEV_REMOVE
	smCxScreen      = 0
	smCyScreen      = 1
	hidUsagePageGen = 0x01 // generic desktop
	hidUsageMouse   = 0x02
)

var (
	procRegisterRawInputDevices      = user32.NewProc("RegisterRawInputDevices")
	procGetRegisteredRawInputDevices = user32.NewProc("GetRegisteredRawInputDevices")
	procGetRawInputData              = user32.NewProc("GetRawInputData")
	procClipCursor                   = user32.NewProc("ClipCursor")
	procGetClipCursor                = user32.NewProc("GetClipCursor")
	procLockSetCursorPos             = user32.NewProc("SetCursorPos")
	procClientToScreen               = user32.NewProc("ClientToScreen")
)

// rawInputDevice — RAWINPUTDEVICE.
type rawInputDevice struct {
	UsagePage  uint16
	Usage      uint16
	Flags      uint32
	HwndTarget uintptr
}

// win32PointerLock — состояние захвата указателя окна Win32. Всё, кроме
// обратных вызовов и hide, трогает только поток окна.
type win32PointerLock struct {
	want       bool // приложение просило захват
	registered bool // Raw Input зарегистрирован на окно
	engaged    bool // указатель сейчас зажат и спрятан

	// Последнее абсолютное положение устройства (MOUSE_MOVE_ABSOLUTE: планшет,
	// удалённый сеанс): из него считается сдвиг.
	absX, absY int32
	haveAbs    bool

	// hide читает WM_SETCURSOR (поток окна), пишет он же; atomic — на случай
	// чтения из тестов с другого потока.
	hide atomic.Bool

	// foreground — подмена проверки «окно на переднем плане» в тестах.
	foreground func() bool

	onMotion  atomic.Pointer[func(dx, dy float64)]
	onChanged atomic.Pointer[func(active bool)]
}

// SetOnRelativeMotion реализует native.PointerLocker.
func (w *Win32Window) SetOnRelativeMotion(fn func(dx, dy float64)) {
	if fn == nil {
		w.lock.onMotion.Store(nil)
		return
	}
	w.lock.onMotion.Store(&fn)
}

// SetOnPointerLockChanged реализует native.PointerLocker.
func (w *Win32Window) SetOnPointerLockChanged(fn func(active bool)) {
	if fn == nil {
		w.lock.onChanged.Store(nil)
		return
	}
	w.lock.onChanged.Store(&fn)
}

// SetPointerLock реализует native.PointerLocker. Работа идёт на потоке окна: курсор,
// зажим и сырой ввод привязаны к нему; вызывающий ждёт результата.
func (w *Win32Window) SetPointerLock(on bool) bool {
	if w.hwnd == 0 {
		return !on
	}
	if w.onUIThread() {
		return w.lock.set(w, on)
	}
	done := make(chan bool, 1)
	w.InvokeOnUIThread(func() { done <- w.lock.set(w, on) })
	select {
	case ok := <-done:
		return ok
	case <-time.After(2 * time.Second):
		return !on
	}
}

// onUIThread — в keygrab_windows.go (общий с захватом клавиатуры).

// set включает и выключает захват. Поток окна.
func (l *win32PointerLock) set(w *Win32Window, on bool) bool {
	if !on {
		l.want = false
		l.disengage(w)
		if l.registered {
			registerRawMouse(0, true)
			l.registered = false
		}
		return true
	}
	if !l.registered {
		if !registerRawMouse(uintptr(w.hwnd), false) {
			return false
		}
		l.registered = true
	}
	l.want = true
	if w.isForeground() {
		l.engage(w)
	}
	return true
}

// isForeground — окно на переднем плане: сырой ввод без INPUTSINK приходит
// только ему.
func (w *Win32Window) isForeground() bool {
	if w.lock.foreground != nil {
		return w.lock.foreground()
	}
	fg, _, _ := procGetForegroundWindow.Call()
	return fg == uintptr(w.hwnd)
}

// registerRawMouse регистрирует (remove=false) или снимает мышь в Raw Input.
// При снятии hwnd должен быть 0, как требует RIDEV_REMOVE.
func registerRawMouse(hwnd uintptr, remove bool) bool {
	d := rawInputDevice{UsagePage: hidUsagePageGen, Usage: hidUsageMouse, HwndTarget: hwnd}
	if remove {
		d.Flags = ridevRemove
	}
	r, _, _ := procRegisterRawInputDevices.Call(uintptr(unsafe.Pointer(&d)), 1, unsafe.Sizeof(d))
	return r != 0
}

// clientCenter — центр клиентской области в экранных координатах.
func (w *Win32Window) clientCenter() (point, bool) {
	var cr rect
	if r, _, _ := procGetClientRect.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&cr))); r == 0 {
		return point{}, false
	}
	p := point{X: cr.Right / 2, Y: cr.Bottom / 2}
	if r, _, _ := procClientToScreen.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&p))); r == 0 {
		return point{}, false
	}
	return p, true
}

// engage зажимает указатель в центре окна и прячет его. Поток окна.
func (l *win32PointerLock) engage(w *Win32Window) {
	if !l.refit(w) {
		return
	}
	if l.engaged {
		return
	}
	l.engaged = true
	l.hide.Store(true)
	procSetCursor.Call(0) // прячем сразу, не дожидаясь WM_SETCURSOR
	l.notify(true)
}

// refit переставляет зажим на центр клиентской области (окно двигали или
// меняли размер). false — геометрию получить не удалось. Поток окна.
func (l *win32PointerLock) refit(w *Win32Window) bool {
	if !l.want || (!l.engaged && !w.isForeground()) {
		return false
	}
	c, ok := w.clientCenter()
	if !ok {
		return false
	}
	clip := rect{Left: c.X, Top: c.Y, Right: c.X + 1, Bottom: c.Y + 1}
	procClipCursor.Call(uintptr(unsafe.Pointer(&clip)))
	procLockSetCursorPos.Call(uintptr(c.X), uintptr(c.Y))
	return true
}

// disengage снимает зажим и возвращает курсор. Поток окна.
func (l *win32PointerLock) disengage(w *Win32Window) {
	if !l.engaged {
		return
	}
	l.engaged = false
	l.haveAbs = false
	procClipCursor.Call(0)
	l.hide.Store(false)
	if w.pointerOverWindow() {
		procSetCursor.Call(w.cursorHandle) // вернуть форму окна (0 — если спрятана приложением)
	}
	l.notify(false)
}

// release вызывается при уничтожении окна: зажим и сырой ввод не должны
// пережить его.
func (l *win32PointerLock) release(w *Win32Window) {
	l.want = false
	l.disengage(w)
	if l.registered {
		registerRawMouse(0, true)
		l.registered = false
	}
}

// onActivate — окно получило или потеряло активность (WM_ACTIVATE).
func (l *win32PointerLock) onActivate(w *Win32Window, active bool) {
	if !l.want {
		return
	}
	if active {
		l.engage(w)
	} else {
		l.disengage(w)
	}
}

func (l *win32PointerLock) notify(active bool) {
	if fn := l.onChanged.Load(); fn != nil {
		(*fn)(active)
	}
}

// handleInput разбирает WM_INPUT: берёт RAWINPUT у системы и отдаёт сдвиг
// приложению. lparam — HRAWINPUT.
func (l *win32PointerLock) handleInput(lparam uintptr) {
	if !l.engaged {
		return
	}
	var buf [128]byte
	size := uint32(len(buf))
	hdr := rawInputHeaderSize()
	n, _, _ := procGetRawInputData.Call(lparam, ridInput,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), uintptr(hdr))
	if int32(n) <= 0 {
		return
	}
	l.handleRaw(buf[:n])
}

// handleRaw отдаёт приложению сдвиг из RAWINPUT мыши (без заголовка WM_INPUT):
// отдельно от чтения системного буфера, чтобы разбор проверялся на готовых
// байтах.
func (l *win32PointerLock) handleRaw(buf []byte) {
	if !l.engaged {
		return
	}
	x, y, abs, ok := pointer.ParseRawMouse(buf, int(unsafe.Sizeof(uintptr(0))))
	if !ok {
		return
	}
	dx, dy := float64(x), float64(y)
	if abs {
		dx, dy = l.absDelta(x, y)
	}
	if dx == 0 && dy == 0 {
		return
	}
	if fn := l.onMotion.Load(); fn != nil {
		(*fn)(dx, dy)
	}
}

// absDelta превращает абсолютное положение устройства (0..65535 на весь
// экран) в сдвиг в пикселях экрана относительно предыдущего.
func (l *win32PointerLock) absDelta(x, y int32) (float64, float64) {
	if !l.haveAbs {
		l.absX, l.absY, l.haveAbs = x, y, true
		return 0, 0
	}
	sw, sh := getSystemMetrics(smCxScreen), getSystemMetrics(smCyScreen)
	dx := float64(x-l.absX) * float64(sw) / 65535
	dy := float64(y-l.absY) * float64(sh) / 65535
	l.absX, l.absY = x, y
	return dx, dy
}

// rawInputHeaderSize — sizeof(RAWINPUTHEADER): dwType, dwSize, hDevice, wParam.
func rawInputHeaderSize() int { return 8 + 2*int(unsafe.Sizeof(uintptr(0))) }
