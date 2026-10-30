//go:build windows

package win32

// keygrab_windows.go — захват клавиатуры Win32: низкоуровневый хук
// WH_KEYBOARD_LL (описание поведения — в keygrab.go).
//
// Почему именно хук, а не RegisterHotKey: Win и Alt+Tab как горячие клавиши
// не регистрируются, а хук видит клавишу до оболочки и вправе её съесть.
//
// Что важно знать про WH_KEYBOARD_LL:
//   - он глобальный: вызывается для клавиш ЛЮБОГО приложения, поэтому фильтр
//     (keyboard.GrabFilter) пропускает всё, пока активно не наше окно;
//   - вызывается на потоке, который его поставил, из его же цикла сообщений —
//     то есть на потоке окна, где живёт и wndProc: колбэки приложения идут
//     оттуда же, что и для обычных клавиш;
//   - должен вернуться быстро (LowLevelHooksTimeout, по умолчанию 300 мс —
//     дольше система молча снимает хук); колбэк окна лишь ставит событие в
//     очередь движка, так что это безопасно;
//   - хук один на процесс (HOOKPROC не знает, чей он): окон с захватом может
//     быть несколько, хук ставится при первом и снимается с последним;
//   - синтетические нажатия (LLKHF_INJECTED: SendInput, AutoHotkey, экранная
//     клавиатура) не трогаются: перехват чужих подстановок ломал бы их.

import (
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oops1/headless-gui/v4/platform/keyboard"
	"github.com/oops1/headless-gui/v4/platform/native"
)

var (
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procGetAsyncKeyState    = user32.NewProc("GetAsyncKeyState")
	procGetModuleHandleW    = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW")
)

const (
	whKeyboardLL = 13 // WH_KEYBOARD_LL

	llkhfExtended = 0x01 // LLKHF_EXTENDED: расширенная клавиша (E0-префикс)
	llkhfAltdown  = 0x20 // LLKHF_ALTDOWN: Alt зажат
	llkhfInjected = 0x10 // LLKHF_INJECTED: событие синтетическое

	// kbGrabUITimeout — сколько вызывающая горутина ждёт, пока поток окна
	// поставит или снимет хук. Если цикл сообщений не крутится, хук всё равно
	// не заработал бы.
	kbGrabUITimeout = 3 * time.Second
)

// kbdLLHookStruct — KBDLLHOOKSTRUCT.
type kbdLLHookStruct struct {
	VkCode    uint32
	ScanCode  uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

// kbGrabHub — единственный хук процесса и окна, которым он служит.
type kbGrabHub struct {
	mu     sync.Mutex
	hook   uintptr                  // HHOOK; 0 — не стоит
	wins   map[uintptr]*Win32Window // hwnd → окно с захватом
	owner  *Win32Window             // окно, получившее последнюю перехваченную клавишу
	filter keyboard.GrabFilter
}

var kbGrab kbGrabHub

var (
	kbHookCbOnce sync.Once
	kbHookCb     uintptr
)

// Подмены системных вызовов в тестах.
var (
	kbForegroundWindow = func() uintptr {
		h, _, _ := procGetForegroundWindow.Call()
		return h
	}
	kbAsyncKeyDown = func(vk int) bool {
		r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
		return r&0x8000 != 0
	}
)

// onUIThread — вызов идёт на потоке, владеющем окном (там же, где wndProc и
// цикл сообщений).
func (w *Win32Window) onUIThread() bool {
	if w.hwnd == 0 {
		return false
	}
	var pid uint32
	tid, _, _ := procGetWindowThreadProcessId.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&pid)))
	return uint32(tid) == windows.GetCurrentThreadId()
}

// SetKeyboardGrab включает или выключает захват клавиатуры (keyboardGrabBackend).
//
// Хук ставится на потоке окна; из другого потока вызов дожидается его
// (kbGrabUITimeout) и возвращает настоящий итог — удалось ли поставить хук.
// Выключение всегда успешно.
func (w *Win32Window) SetKeyboardGrab(on bool) bool {
	if w.hwnd == 0 {
		return false
	}
	if !on && !w.kbGrabbed.Load() {
		// Нечего снимать (захват не включали или окно уже уничтожено, а
		// WM_DESTROY его снял): ждать поток окна, которого может уже не быть,
		// незачем — так закрытие окна не задерживалось бы на kbGrabUITimeout.
		return true
	}
	if w.onUIThread() {
		return kbGrab.set(w, on)
	}
	res := make(chan bool, 1)
	w.InvokeOnUIThread(func() { res <- kbGrab.set(w, on) })
	select {
	case ok := <-res:
		return ok
	case <-time.After(kbGrabUITimeout):
		return !on
	}
}

// KeyboardGrabActive — захват действует прямо сейчас: заказан, хук стоит и
// окно активно (keyboardGrabQuerier).
func (w *Win32Window) KeyboardGrabActive() bool {
	if !w.kbGrabbed.Load() {
		return false
	}
	return kbForegroundWindow() == uintptr(w.hwnd)
}

// set включает или выключает захват для окна. Только поток окна.
func (h *kbGrabHub) set(w *Win32Window, on bool) bool {
	if !on {
		h.release(w, true)
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	hwnd := uintptr(w.hwnd)
	if _, ok := h.wins[hwnd]; ok {
		return true
	}
	if h.hook == 0 {
		kbHookCbOnce.Do(func() { kbHookCb = windows.NewCallback(kbHookProc) })
		mod, _, _ := procGetModuleHandleW.Call(0)
		hk, _, _ := procSetWindowsHookExW.Call(whKeyboardLL, kbHookCb, mod, 0)
		if hk == 0 {
			return false
		}
		h.hook = hk
		h.filter.Reset()
	}
	if h.wins == nil {
		h.wins = map[uintptr]*Win32Window{}
	}
	h.wins[hwnd] = w
	w.kbGrabbed.Store(true)
	return true
}

// release снимает захват окна. notify — показать окну отпускание клавиш,
// перехваченных и ещё зажатых: система отпускания теперь увидит без нажатия,
// а приложение иначе решило бы, что Win всё ещё зажата. При уничтожении окна
// notify=false.
func (h *kbGrabHub) release(w *Win32Window, notify bool) {
	var held []int
	h.mu.Lock()
	hwnd := uintptr(w.hwnd)
	if _, ok := h.wins[hwnd]; !ok {
		h.mu.Unlock()
		return
	}
	delete(h.wins, hwnd)
	w.kbGrabbed.Store(false)
	if h.owner == w {
		if notify {
			held = h.filter.Held()
		}
		h.owner = nil
	}
	if len(h.wins) == 0 && h.hook != 0 {
		procUnhookWindowsHookEx.Call(h.hook)
		h.hook = 0
		h.filter.Reset()
	}
	h.mu.Unlock()
	// Колбэки приложения — вне замка: из них можно снова звать SetKeyboardGrab.
	for _, vk := range held {
		scan, ext := vkScanCode(vk)
		w.deliverGrabbedKey(vk, scan, ext, true, false)
	}
}

// deliverGrabbedKey отдаёт перехваченную клавишу окну тем же путём, что и
// обычную из wndProc — со скан-кодом, если окно его принимает (rawKeySource).
// Без этого как раз перехваченные Win и Alt+Tab приходили бы без скан-кода
// (а RDP передаёт серверу именно его) и мимо учёта зажатых клавиш, который
// отпускает их при потере окном фокуса.
func (w *Win32Window) deliverGrabbedKey(vk int, scan uint16, ext, up, repeat bool) {
	if w.onRawKey != nil {
		w.onRawKey(keyboard.RawKey{VK: vk, Scan: scan, Extended: ext, Pressed: !up, Repeat: repeat && !up})
		return
	}
	switch {
	case up:
		if w.onKeyUp != nil {
			w.onKeyUp(vk)
		}
	case w.onKeyDownRepeat != nil:
		w.onKeyDownRepeat(vk, repeat)
	case w.onKeyDown != nil:
		w.onKeyDown(vk)
	}
}

// kbHookProc — HOOKPROC хука WH_KEYBOARD_LL.
func kbHookProc(nCode, wparam, lparam uintptr) uintptr {
	if kbGrab.handle(nCode, wparam, lparam) {
		return 1
	}
	ret, _, _ := procCallNextHookEx.Call(kbGrab.hookHandle(), nCode, wparam, lparam)
	return ret
}

func (h *kbGrabHub) hookHandle() uintptr {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.hook
}

// handle разбирает событие хука; true — клавишу съели (хук вернёт 1).
func (h *kbGrabHub) handle(nCode, wparam, lparam uintptr) bool {
	if int32(nCode) < 0 || lparam == 0 { // HC_ACTION == 0
		return false
	}
	// Указатель берём через адрес переменной: память принадлежит системе, и
	// vet не любит прямое uintptr → Pointer.
	p := *(*unsafe.Pointer)(unsafe.Pointer(&lparam))
	kb := (*kbdLLHookStruct)(p)
	if kb.Flags&llkhfInjected != 0 {
		return false
	}
	var up bool
	switch wparam {
	case wmKeydown, wmSyskeydown:
	case wmKeyup, wmSyskeyup:
		up = true
	default:
		return false
	}

	fg := kbForegroundWindow()
	h.mu.Lock()
	w := h.wins[fg]
	act := h.filter.Event(int(kb.VkCode), up, kb.Flags&llkhfAltdown != 0,
		kbAsyncKeyDown(native.VK_CONTROL), w != nil)
	target := w
	if target == nil {
		target = h.owner // отпускание перехваченной клавиши после ухода фокуса
	}
	if act.Swallow && act.Deliver && w != nil {
		h.owner = w
	}
	h.mu.Unlock()

	if !act.Swallow {
		return false
	}
	if act.Deliver && target != nil {
		target.deliverGrabbedKey(int(kb.VkCode), uint16(kb.ScanCode), kb.Flags&llkhfExtended != 0, up, act.Repeat)
	}
	return true
}

var procMapVirtualKeyW = user32.NewProc("MapVirtualKeyW")

// vkScanCode — скан-код клавиши по виртуальному коду (MAPVK_VK_TO_VSC_EX):
// старший байт 0xE0 означает расширенную клавишу.
func vkScanCode(vk int) (scan uint16, ext bool) {
	r, _, _ := procMapVirtualKeyW.Call(uintptr(vk), 4) // MAPVK_VK_TO_VSC_EX
	return uint16(r & 0xFF), r&0xFF00 == 0xE000
}
