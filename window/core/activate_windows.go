package core

// activate_windows.go — передний план и свёрнутость окна Win32 (задание
// PixGo E14).
//
// Развернуть окно система позволяет любому процессу, а вот SetForegroundWindow
// работает только у процесса, имеющего право на передний план (foreground
// lock): его получает тот, кто обработал последний ввод, либо тот, кому право
// передали явно. Поэтому:
//
//   - свёрнутое окно разворачивается всегда (SW_RESTORE), даже когда право на
//     передний план отказано;
//   - отправитель, запустивший второй процесс, зовёт AllowForeground(pid
//     основного) до передачи файла;
//   - отказ системы не молчаливый: кнопка окна в панели задач мигает, пока
//     пользователь не перейдёт к окну (FlashWindowEx), а Activate возвращает
//     false. Хитрости вроде AttachThreadInput или фальшивого нажатия Alt
//     обходят защиту системы от кражи фокуса — ими не пользуемся.

import (
	"sync/atomic"
	"unsafe"
)

var (
	procIsIconic                 = user32.NewProc("IsIconic")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procFlashWindowEx            = user32.NewProc("FlashWindowEx")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
)

const (
	sizeMinimized = 1 // WM_SIZE: SIZE_MINIMIZED

	flashwAll       = 0x3 // FLASHW_ALL: заголовок и кнопка панели задач
	flashwTimerNoFG = 0xC // FLASHW_TIMERNOFG: мигать, пока окно не станет активным

	asfwAny = 0xFFFFFFFF // ASFW_ANY: право любому процессу
)

// flashWInfo — FLASHWINFO.
type flashWInfo struct {
	CbSize  uint32
	Hwnd    uintptr
	Flags   uint32
	Count   uint32
	Timeout uint32
}

// activateWin32 — состояние свёрнутости окна: последнее известное значение
// (для уведомления о смене) и колбэк.
type activateWin32 struct {
	minimized atomic.Bool
	onChange  atomic.Pointer[func(bool)]
}

// flashRequest собирает FLASHWINFO «мигать, пока окно не станет активным».
// Чистая функция: раскладка и флаги проверяются тестом.
func flashRequest(hwnd uintptr) flashWInfo {
	return flashWInfo{
		CbSize: uint32(unsafe.Sizeof(flashWInfo{})),
		Hwnd:   hwnd,
		Flags:  flashwAll | flashwTimerNoFG,
	}
}

// allowForegroundArg переводит pid в аргумент AllowSetForegroundWindow:
// отрицательный — ASFW_ANY (любой процесс), ноль не процесс.
func allowForegroundArg(pid int) (uintptr, bool) {
	switch {
	case pid < 0:
		return asfwAny, true
	case pid == 0:
		return 0, false
	}
	return uintptr(pid), true
}

// AllowForeground разрешает процессу pid выводить свои окна на передний план
// (AllowSetForegroundWindow). Зовёт тот, у кого сейчас право на передний план
// — обычно процесс, запущенный пользователем из проводника, — перед тем, как
// передать работу уже работающему процессу: тогда Activate у того получит
// право. Право одноразовое и пропадает после следующего ввода пользователя
// в другое окно.
//
// pid < 0 — любой процесс (ASFW_ANY). true — право выдано; false — у вызвавшего
// процесса самого нет права на передний план, или pid неверный.
//
// На других платформах такого механизма нет (там доказательство пользовательского
// действия передаётся токеном, см. TakeActivationToken): всегда false.
func AllowForeground(pid int) bool {
	arg, ok := allowForegroundArg(pid)
	if !ok {
		return false
	}
	ret, _, _ := procAllowSetForegroundWindow.Call(arg)
	return ret != 0
}

// IsMinimized — окно свёрнуто (IsIconic). Реализует minimizedBackend.
func (w *Win32Window) IsMinimized() bool {
	if w.hwnd == 0 {
		return false
	}
	ret, _, _ := procIsIconic.Call(uintptr(w.hwnd))
	return ret != 0
}

// SetOnMinimizedChanged регистрирует уведомление о смене свёрнутости.
// Реализует minimizedBackend. Вызывается на потоке окна (из WM_SIZE).
func (w *Win32Window) SetOnMinimizedChanged(fn func(bool)) {
	if fn == nil {
		w.act.onChange.Store(nil)
		return
	}
	w.act.onChange.Store(&fn)
}

// noteMinimized запоминает свёрнутость из WM_SIZE и сообщает о смене.
func (w *Win32Window) noteMinimized(minimized bool) {
	if w.act.minimized.Swap(minimized) == minimized {
		return
	}
	if fn := w.act.onChange.Load(); fn != nil {
		(*fn)(minimized)
	}
}

// Activate разворачивает окно (если оно свёрнуто или скрыто), поднимает над
// остальными и передаёт ему фокус. true — окно стало активным; false — система
// отказала в праве на передний план, окно развёрнуто, но осталось под другими
// и мигает в панели задач. Реализует native.ActivateBackend.
//
// Выполняется на вызывающей горутине: ShowWindow и SetForegroundWindow к чужому
// потоку обращаются через очередь сообщений окна, а право на передний план
// принадлежит процессу, а не потоку.
func (w *Win32Window) Activate() bool {
	if w.hwnd == 0 {
		return false
	}
	h := uintptr(w.hwnd)
	if ret, _, _ := procIsIconic.Call(h); ret != 0 {
		procShowWindow.Call(h, uintptr(swRestore))
	} else if ret, _, _ := procIsWindowVisible.Call(h); ret == 0 {
		procShowWindow.Call(h, uintptr(swShow)) // убрано в трей — показываем
	}
	procBringWindowToTop.Call(h)
	if ret, _, _ := procSetForegroundWindow.Call(h); ret != 0 {
		return true
	}
	fi := flashRequest(h)
	procFlashWindowEx.Call(uintptr(unsafe.Pointer(&fi)))
	return false
}

// ActivateWithToken на Win32 токен не нужен: право на передний план выдаёт
// AllowForeground, а не строка. Реализует native.ActivateBackend.
func (w *Win32Window) ActivateWithToken(string) bool { return w.Activate() }
