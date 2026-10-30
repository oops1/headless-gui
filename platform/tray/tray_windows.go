//go:build windows

package tray

// tray_windows.go — иконка в трее и уведомления на Windows: Shell_NotifyIcon
// (иконка, подсказка, balloon-уведомления NIF_INFO) на окне бэкенда Win32.
// О событиях иконки Windows сообщает окну сообщением wmTrayCallback; его, как и
// WM_DESTROY (иконка не переживает окно), пакет ловит через
// win32.AddMessageHook (register_windows.go). Чистый Go, без CGO.

import (
	"image"
	"sync"
	"unsafe"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/sys/windows"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/platform/win32"
	"github.com/oops1/headless-gui/v4/widget/base"
)

const (
	// Shell_NotifyIcon: dwMessage
	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	// NOTIFYICONDATA.uFlags
	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010

	// wmTrayCallback — сообщение, которым Windows сообщает окну о событиях
	// иконки (NOTIFYICONDATA.uCallbackMessage). WM_APP (0x8000) занят
	// InvokeOnUIThread бэкенда — берём следующее, бэкенд держит его за треем.
	wmTrayCallback = 0x8000 + 1

	// wmDestroy — WM_DESTROY: окно гибнет, иконку снимаем.
	wmDestroy = 0x0002

	// GetSystemMetrics: размер маленькой системной иконки.
	smCXSmIcon = 49
	smCYSmIcon = 50
)

var (
	shell32 = windows.NewLazySystemDLL("shell32.dll")
	user32  = windows.NewLazySystemDLL("user32.dll")

	procShellNotifyIconW = shell32.NewProc("Shell_NotifyIconW")
	procDestroyIcon      = user32.NewProc("DestroyIcon")
	procGetCursorPos     = user32.NewProc("GetCursorPos")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
)

// shellNotifyIcon — Shell_NotifyIconW. Переменная: тесты подменяют её, чтобы
// иконка не появлялась в настоящем трее.
var shellNotifyIcon = func(msg uint32, nid *notifyIconDataW) bool {
	ret, _, _ := procShellNotifyIconW.Call(uintptr(msg), uintptr(unsafe.Pointer(nid)))
	return ret != 0
}

// notifyIconDataW — NOTIFYICONDATAW (модерн-версия, Vista+).
type notifyIconDataW struct {
	CbSize           uint32
	HWnd             windows.HWND
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32 // union: uTimeout / uVersion
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     windows.Handle
}

// point — POINT для GetCursorPos.
type point struct{ X, Y int32 }

// winTray — иконка в трее одного окна Win32.
//
// Замков два. opMu упорядочивает вызовы Shell_NotifyIcon и держит added и
// hicon: методы зовут с горутины движка и приложения. mu — короткий, для
// колбэков и closed: его берёт и обработчик уведомлений иконки на потоке окна,
// а Shell_NotifyIcon под ним не зовётся — пока он ждёт Проводника, поток окна
// разбирает присланные ему сообщения, и обработчик не должен встать на замке.
// Колбэки зовутся вне замков.
type winTray struct {
	w *win32.Win32Window

	opMu  sync.Mutex
	added bool           // иконка добавлена (NIM_ADD выполнен)
	hicon windows.Handle // текущий HICON иконки (уничтожаем при замене)

	mu        sync.Mutex
	onClick   func(button int, doubleClick bool)
	onBalloon func()
	closed    bool
}

// trays — трей по окну: так обработчики сообщений находят его по hwnd
// (win32.WindowOf).
var (
	traysMu sync.Mutex
	trays   = map[*win32.Win32Window]*winTray{}
)

// newTray — трей окна Win32 (фабрика native.RegisterTray); у прочих окон трея
// нет. Прежний трей того же окна снимается.
func newTray(nw native.NativeWindow) native.Tray {
	w, ok := nw.(*win32.Win32Window)
	if !ok || w == nil {
		return nil
	}
	t := &winTray{w: w}
	traysMu.Lock()
	old := trays[w]
	trays[w] = t
	traysMu.Unlock()
	if old != nil {
		old.Close()
	}
	return t
}

// trayOfWindow — трей окна hwnd; nil — у окна трея нет.
func trayOfWindow(hwnd uintptr) *winTray {
	w := win32.WindowOf(hwnd)
	if w == nil {
		return nil
	}
	traysMu.Lock()
	defer traysMu.Unlock()
	return trays[w]
}

// onTrayCallback — уведомление иконки (wmTrayCallback). Сообщение целиком
// наше: отвечаем handled, как прежде отвечала оконная процедура бэкенда.
func onTrayCallback(hwnd, _, lparam uintptr) (uintptr, bool) {
	if t := trayOfWindow(hwnd); t != nil {
		t.handleCallback(lparam)
	}
	return 0, true
}

// onDestroy — окно гибнет (WM_DESTROY): иконка не переживает окно — ни у
// главного, ни у дочернего, ни у закрытого без WM_CLOSE вместе с владельцем.
// Отвечает handled=false: уборку окна делает бэкенд.
func onDestroy(hwnd, _, _ uintptr) (uintptr, bool) {
	if t := trayOfWindow(hwnd); t != nil {
		t.Close()
	}
	return 0, false
}

// baseNotifyData заполняет общую часть NOTIFYICONDATAW (окно, uID, cbSize).
func baseNotifyData(hwnd uintptr) notifyIconDataW {
	return notifyIconDataW{
		CbSize: uint32(unsafe.Sizeof(notifyIconDataW{})),
		HWnd:   windows.HWND(hwnd),
		UID:    1,
	}
}

// isClosed — окно закрыто, трей снят.
func (t *winTray) isClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

// SetTrayIcon добавляет или обновляет иконку в трее.
func (t *winTray) SetTrayIcon(icon image.Image, tooltip string) error {
	t.opMu.Lock()
	defer t.opMu.Unlock()
	hwnd := t.w.HWND()
	if hwnd == 0 || t.isClosed() {
		return native.ErrTrayUnsupported
	}
	hicon := imageToHICON(icon)

	nid := baseNotifyData(hwnd)
	nid.UFlags = nifMessage | nifIcon | nifTip
	nid.UCallbackMessage = wmTrayCallback
	nid.HIcon = hicon
	copyUTF16(nid.SzTip[:], tooltip)

	msg := uint32(nimAdd)
	if t.added {
		msg = nimModify
	}
	ok := shellNotifyIcon(msg, &nid)

	keep, drop := trayIconAfterNotify(ok, t.hicon, hicon)
	if drop != 0 {
		procDestroyIcon.Call(uintptr(drop))
	}
	t.hicon = keep
	if !ok {
		return native.ErrTrayUnsupported
	}
	t.added = true
	return nil
}

// trayIconAfterNotify выбирает, какую иконку оставить, а какую уничтожить:
// при успехе живёт новая, при неудаче — прежняя.
func trayIconAfterNotify(ok bool, prev, next windows.Handle) (keep, drop windows.Handle) {
	if ok {
		return next, prev
	}
	return prev, next
}

// RemoveTrayIcon убирает иконку из трея. HICON уничтожается всегда, даже если
// иконка так и не была добавлена.
func (t *winTray) RemoveTrayIcon() {
	t.opMu.Lock()
	defer t.opMu.Unlock()
	t.removeLocked()
}

// removeLocked — RemoveTrayIcon под opMu.
func (t *winTray) removeLocked() {
	if hwnd := t.w.HWND(); hwnd != 0 && t.added {
		nid := baseNotifyData(hwnd)
		shellNotifyIcon(nimDelete, &nid)
	}
	if t.hicon != 0 {
		procDestroyIcon.Call(uintptr(t.hicon))
		t.hicon = 0
	}
	t.added = false
}

// SetTrayClickHandler / SetBalloonClickHandler регистрируют колбэки.
func (t *winTray) SetTrayClickHandler(fn func(button int, doubleClick bool)) {
	t.mu.Lock()
	if !t.closed {
		t.onClick = fn
	}
	t.mu.Unlock()
}

func (t *winTray) SetBalloonClickHandler(fn func()) {
	t.mu.Lock()
	if !t.closed {
		t.onBalloon = fn
	}
	t.mu.Unlock()
}

// SetTrayMenuNative — меню трея на Windows рисует ядро (widget.PopupMenu у
// курсора), система его не показывает.
func (t *winTray) SetTrayMenuNative(*base.PopupMenu) bool { return false }

// SetTrayMenuRunner — системного меню нет, исполнять нечего.
func (t *winTray) SetTrayMenuRunner(func(fn func())) {}

// ShowBalloon показывает системное balloon-уведомление. Требует установленной
// иконки трея.
func (t *winTray) ShowBalloon(title, text string, infoFlag uint32) error {
	t.opMu.Lock()
	defer t.opMu.Unlock()
	hwnd := t.w.HWND()
	if hwnd == 0 || !t.added || t.isClosed() {
		return native.ErrTrayUnsupported
	}
	nid := baseNotifyData(hwnd)
	nid.UFlags = nifInfo
	nid.DwInfoFlags = infoFlag
	copyUTF16(nid.SzInfo[:], text)
	copyUTF16(nid.SzInfoTitle[:], title)
	if !shellNotifyIcon(nimModify, &nid) {
		return native.ErrTrayUnsupported
	}
	return nil
}

// CursorScreenPos возвращает позицию курсора (экранные координаты).
func (t *winTray) CursorScreenPos() (int, int) {
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	return int(pt.X), int(pt.Y)
}

// Close снимает иконку и колбэки; повторный вызов ничего не делает. Зовут
// ядро при закрытии окна (tearDown) и обработчик WM_DESTROY.
func (t *winTray) Close() {
	t.opMu.Lock()
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		t.opMu.Unlock()
		return
	}
	t.closed = true
	t.onClick, t.onBalloon = nil, nil
	t.mu.Unlock()
	t.removeLocked()
	t.opMu.Unlock()

	traysMu.Lock()
	if trays[t.w] == t {
		delete(trays, t.w)
	}
	traysMu.Unlock()
}

// handleCallback разбирает уведомление иконки трея (поток окна).
func (t *winTray) handleCallback(lparam uintptr) {
	button, dbl, balloon, ok := trayEventToClick(uint32(lparam))
	if !ok {
		return
	}
	t.mu.Lock()
	onClick, onBalloon := t.onClick, t.onBalloon
	t.mu.Unlock()
	if balloon {
		if onBalloon != nil {
			onBalloon()
		}
		return
	}
	if onClick != nil {
		onClick(button, dbl)
	}
}

// imageToHICON масштабирует icon до размера маленькой системной иконки и строит
// HICON (цветной DIB с альфой + AND-маска из альфы).
func imageToHICON(icon image.Image) windows.Handle {
	sz := getSystemMetrics(smCXSmIcon)
	szY := getSystemMetrics(smCYSmIcon)
	if sz <= 0 {
		sz = 16
	}
	if szY <= 0 {
		szY = 16
	}
	dst := image.NewRGBA(image.Rect(0, 0, sz, szY))
	if icon != nil {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), icon, icon.Bounds(), xdraw.Over, nil)
	}
	return win32.NewIcon(dst)
}

// getSystemMetrics — GetSystemMetrics.
func getSystemMetrics(index int) int {
	r, _, _ := procGetSystemMetrics.Call(uintptr(index))
	return int(int32(r))
}

// copyUTF16 кодирует s в dst (UTF-16), обрезая до len(dst)-1 и завершая NUL.
func copyUTF16(dst []uint16, s string) {
	u := windows.StringToUTF16(s)
	n := len(u)
	if n > len(dst) {
		n = len(dst)
		u[n-1] = 0
	}
	copy(dst[:n], u[:n])
}
