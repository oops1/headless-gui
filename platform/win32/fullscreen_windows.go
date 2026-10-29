package win32

// fullscreen_windows.go — полный экран и мониторы Win32.
//
// Окно главное создаётся как WS_POPUP|WS_THICKFRAME без заголовка: «полный
// экран» — это перестановка окна на rcMonitor его монитора (не rcWork:
// таскбар закрывается, система сама прячет его под окном такого размера без
// заголовка и рамки). На время полного экрана снимается WS_THICKFRAME, иначе
// система зажала бы окно максимальным размером отслеживания (размер основного
// экрана). Положение, размер, стиль и «был развёрнут» запоминаются и
// возвращаются при выходе (арифметика — в platform/native/fullscreen_rect.go).

import (
	"image"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/oops1/headless-gui/v3/platform/native"
)

var (
	shcore                  = windows.NewLazySystemDLL("shcore.dll")
	procGetDpiForMonitor    = shcore.NewProc("GetDpiForMonitor")
	procMonitorFromWindow   = user32.NewProc("MonitorFromWindow")
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
)

const (
	wmDisplaychange    = 0x007E // WM_DISPLAYCHANGE: сменилось разрешение/состав мониторов
	wmWindowposchanged = 0x0047 // WM_WINDOWPOSCHANGED

	gwlStyle = ^uintptr(15) // GWL_STYLE (== -16)

	swpNomove        = 0x0002
	swpNosize        = 0x0001
	swpNozorder      = 0x0004
	swpNoactivate    = 0x0010
	swpFramechanged  = 0x0020
	swpNoownerzorder = 0x0200

	monitorDefaultToNearest = 2
	monitorInfoFPrimary     = 1
	mdtEffectiveDPI         = 0
)

// fullscreenWin32 — состояние полного экрана окна.
type fullscreenWin32 struct {
	on atomic.Bool
	// mu охраняет snap: SetSize/SetPosition во время полного экрана пишут в
	// него с горутины приложения, выход читает на потоке окна.
	mu   sync.Mutex
	snap native.FullscreenSnapshot
	// refitting — идёт перестановка под монитор: вложенное WM_WINDOWPOSCHANGED
	// от неё не должно запускать новую. Только поток окна.
	refitting bool
	onChange  atomic.Pointer[func(bool)]
}

// SetFullscreen просит перейти в полный экран или вернуться. Реализует
// fullscreenBackend. Выполняется на потоке окна (ShowWindow и смена стиля с
// чужого потока Win32 не любит), поэтому результат виден чуть позже вызова.
func (w *Win32Window) SetFullscreen(on bool) bool {
	if w.hwnd == 0 {
		return false
	}
	w.InvokeOnUIThread(func() { w.applyFullscreen(on) })
	return true
}

// SetFullscreenOn встаёт в полный экран на мониторе m (fullscreenOnBackend):
// HMONITOR/имя/границы сверяются с текущим списком мониторов, окно ставится на
// rcMonitor найденного. Окно, уже стоящее в полном экране, переезжает на него
// без выхода из режима и без уведомлений: состояние не меняется.
func (w *Win32Window) SetFullscreenOn(m native.Monitor) bool {
	if w.hwnd == 0 {
		return false
	}
	cur, ok := native.MatchMonitor(EnumMonitors(), m)
	if !ok || cur.Bounds.Empty() {
		return false
	}
	w.InvokeOnUIThread(func() { w.applyFullscreenOn(cur.Bounds) })
	return true
}

// IsFullscreen — окно закрывает монитор. Реализует fullscreenBackend.
func (w *Win32Window) IsFullscreen() bool { return w.fs.on.Load() }

// SetOnFullscreenChanged регистрирует уведомление о смене состояния.
// Реализует fullscreenBackend. Вызывается на потоке окна.
func (w *Win32Window) SetOnFullscreenChanged(fn func(on bool)) {
	if fn == nil {
		w.fs.onChange.Store(nil)
		return
	}
	w.fs.onChange.Store(&fn)
}

func (w *Win32Window) notifyFullscreen(on bool) {
	if fn := w.fs.onChange.Load(); fn != nil {
		(*fn)(on)
	}
}

func (w *Win32Window) windowStyle() uint32 {
	st, _, _ := procGetWindowLongPtrW.Call(uintptr(w.hwnd), gwlStyle)
	return uint32(st)
}

func (w *Win32Window) setWindowStyle(st uint32) {
	procSetWindowLongPtrW.Call(uintptr(w.hwnd), gwlStyle, uintptr(st))
}

func (w *Win32Window) windowRect() image.Rectangle {
	var r rect
	procGetWindowRect.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&r)))
	return image.Rect(int(r.Left), int(r.Top), int(r.Right), int(r.Bottom))
}

// setWindowRect переставляет окно на r (внешний прямоугольник; окно без
// заголовка и рамки, поэтому он же — клиентский). flags — SWP_*.
func (w *Win32Window) setWindowRect(r image.Rectangle, flags uintptr) {
	procSetWindowPos.Call(uintptr(w.hwnd), 0,
		uintptr(r.Min.X), uintptr(r.Min.Y), uintptr(r.Dx()), uintptr(r.Dy()), flags)
}

// monitorRectOfWindow — rcMonitor монитора, на котором лежит большая часть
// окна (или ближайшего).
func (w *Win32Window) monitorRectOfWindow() image.Rectangle {
	hmon, _, _ := procMonitorFromWindow.Call(uintptr(w.hwnd), monitorDefaultToNearest)
	if m, ok := win32MonitorFromHandle(hmon); ok {
		return m.Bounds
	}
	return image.Rectangle{}
}

// applyFullscreen выполняет переход на потоке окна.
func (w *Win32Window) applyFullscreen(on bool) {
	if w.hwnd == 0 || w.fs.on.Load() == on {
		return
	}
	if on {
		w.enterFullscreen(image.Rectangle{})
	} else {
		w.leaveFullscreen()
	}
	w.applyCorners() // полноэкранное окно не скругляется
	w.notifyFullscreen(on)
}

// applyFullscreenOn — полный экран на прямоугольнике монитора target, на
// потоке окна: вход, если окно ещё не в полном экране, иначе перенос.
func (w *Win32Window) applyFullscreenOn(target image.Rectangle) {
	if w.hwnd == 0 || target.Empty() {
		return
	}
	if !w.fs.on.Load() {
		w.enterFullscreen(target)
		w.applyCorners()
		w.notifyFullscreen(true)
		return
	}
	// Уже в полном экране: перенос на другой монитор. Стиль и снимок для
	// выхода остаются прежними; refitting не пускает вложенный refit.
	w.fs.refitting = true
	w.setWindowRect(target, swpNoownerzorder)
	w.fs.refitting = false
}

// enterFullscreen входит в полный экран на прямоугольнике монитора monitor
// (rcMonitor); пустой — на мониторе, где лежит окно.
func (w *Win32Window) enterFullscreen(monitor image.Rectangle) {
	zoomed, _, _ := procIsZoomed.Call(uintptr(w.hwnd))
	if zoomed != 0 {
		// Развёрнутое окно сначала возвращается в нормальное состояние:
		// иначе система продолжает считать его zoomed, держит по рабочей
		// области и не прячет таскбар. Нормальный прямоугольник для выхода
		// берётся уже после этого.
		procShowWindow.Call(uintptr(w.hwnd), uintptr(swRestore))
	}
	snap := native.FullscreenSnapshot{Rect: w.windowRect(), Style: w.windowStyle(), Maximized: zoomed != 0}
	w.fs.mu.Lock()
	w.fs.snap = snap
	w.fs.mu.Unlock()

	if monitor.Empty() {
		monitor = w.monitorRectOfWindow()
	}
	style, target := native.FullscreenEnterPlan(snap.Style, monitor)
	// Флаг — до первого же сообщения от SetWindowPos: WM_SIZE не должен
	// записать «не развёрнуто» поверх запомненного значения.
	w.maximized = snap.Maximized
	w.fs.on.Store(true)
	w.setWindowStyle(style)
	if target.Empty() { // монитор определить не удалось — остаёмся на месте
		target = snap.Rect
	}
	w.fs.refitting = true
	w.setWindowRect(target, swpFramechanged|swpNoownerzorder) // HWND_TOP: поверх остальных
	w.fs.refitting = false
}

func (w *Win32Window) leaveFullscreen() {
	w.fs.mu.Lock()
	snap := w.fs.snap
	w.fs.mu.Unlock()

	style, target := native.FullscreenLeavePlan(w.windowStyle(), snap, win32MonitorRects())
	w.fs.on.Store(false)
	w.setWindowStyle(style)
	w.fs.refitting = true
	w.setWindowRect(target, swpFramechanged|swpNozorder|swpNoactivate)
	w.fs.refitting = false
	if snap.Maximized {
		procShowWindow.Call(uintptr(w.hwnd), uintptr(swMaximize))
	}
}

// refitFullscreen переставляет полноэкранное окно под его монитор: сменилось
// разрешение (WM_DISPLAYCHANGE), окно перенесли на другой монитор, сменился DPI.
func (w *Win32Window) refitFullscreen() {
	if w.hwnd == 0 || !w.fs.on.Load() || w.fs.refitting {
		return
	}
	target, ok := native.FullscreenRefit(w.windowRect(), w.monitorRectOfWindow())
	if !ok {
		return
	}
	w.fs.refitting = true
	w.setWindowRect(target, swpNozorder|swpNoactivate)
	w.fs.refitting = false
}

// ─── Мониторы ────────────────────────────────────────────────────────────────

// monitorInfoEx — MONITORINFOEXW: MONITORINFO и имя устройства.
type monitorInfoEx struct {
	monitorInfo
	SzDevice [32]uint16
}

// win32MonitorFromHandle описывает монитор по HMONITOR.
func win32MonitorFromHandle(hmon uintptr) (native.Monitor, bool) {
	if hmon == 0 {
		return native.Monitor{}, false
	}
	var mi monitorInfoEx
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	if ret, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi))); ret == 0 {
		return native.Monitor{}, false
	}
	m := native.MonitorWithKey(native.Monitor{
		Name:    windows.UTF16ToString(mi.SzDevice[:]),
		Bounds:  image.Rect(int(mi.RcMonitor.Left), int(mi.RcMonitor.Top), int(mi.RcMonitor.Right), int(mi.RcMonitor.Bottom)),
		Work:    image.Rect(int(mi.RcWork.Left), int(mi.RcWork.Top), int(mi.RcWork.Right), int(mi.RcWork.Bottom)),
		Scale:   1,
		Primary: mi.DwFlags&monitorInfoFPrimary != 0,
	}, uint64(hmon))
	// GetDpiForMonitor — Windows 8.1+; без неё (или при ошибке) масштаб 1.
	if procGetDpiForMonitor.Find() == nil {
		var dx, dy uint32
		if hr, _, _ := procGetDpiForMonitor.Call(hmon, mdtEffectiveDPI,
			uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); hr == 0 && dx > 0 {
			m.Scale = float64(dx) / 96.0
		}
	}
	return m, true
}

var (
	monEnumMu  sync.Mutex
	monEnumOut []uintptr
	// monEnumProc создаётся один раз: windows.NewCallback ни при каком вызове
	// не освобождает слот, а их число в процессе ограничено.
	monEnumProc = windows.NewCallback(func(hmon, hdc, rc, lparam uintptr) uintptr {
		monEnumOut = append(monEnumOut, hmon)
		return 1 // продолжать перечисление
	})
)

// EnumMonitors — все мониторы (EnumDisplayMonitors).
func EnumMonitors() []native.Monitor {
	monEnumMu.Lock()
	monEnumOut = monEnumOut[:0]
	procEnumDisplayMonitors.Call(0, 0, monEnumProc, 0)
	handles := append([]uintptr(nil), monEnumOut...)
	monEnumMu.Unlock()

	var out []native.Monitor
	for _, h := range handles {
		if m, ok := win32MonitorFromHandle(h); ok {
			out = append(out, m)
		}
	}
	return out
}

// win32MonitorRects — прямоугольники мониторов (для возврата окна на экран).
func win32MonitorRects() []image.Rectangle {
	var out []image.Rectangle
	for _, m := range EnumMonitors() {
		out = append(out, m.Bounds)
	}
	return out
}

// MonitorOfWindow — монитор окна (MonitorFromWindow). Реализует monitorSource.
func (w *Win32Window) MonitorOfWindow() (native.Monitor, bool) {
	if w.hwnd == 0 {
		return native.Monitor{}, false
	}
	hmon, _, _ := procMonitorFromWindow.Call(uintptr(w.hwnd), monitorDefaultToNearest)
	return win32MonitorFromHandle(hmon)
}
