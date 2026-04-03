//go:build windows

package window

import (
	"image"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ─── Win32 API константы ────────────────────────────────────────────────────

const (
	// Window styles
	wsPopup       = 0x80000000
	wsVisible     = 0x10000000
	wsSysmenu     = 0x00080000
	wsMinimizebox = 0x00020000
	wsMaximizebox = 0x00010000
	wsThickframe  = 0x00040000
	wsClipchildren = 0x02000000

	// Extended styles
	wsExAppwindow   = 0x00040000
	wsExLayered     = 0x00080000
	wsExNoredirectionbitmap = 0x00200000

	// Messages
	wmDestroy     = 0x0002
	wmSize        = 0x0005
	wmClose       = 0x0010
	wmPaint       = 0x000F
	wmErasebkgnd  = 0x0014
	wmMousemove   = 0x0200
	wmLbuttondown = 0x0201
	wmLbuttonup   = 0x0202
	wmRbuttondown = 0x0204
	wmRbuttonup   = 0x0205
	wmMbuttondown = 0x0207
	wmMbuttonup   = 0x0208
	wmMousewheel  = 0x020A
	wmKeydown     = 0x0100
	wmKeyup       = 0x0101
	wmChar        = 0x0102
	wmSyscommand  = 0x0112
	wmNccalcsize  = 0x0083
	wmNchittest   = 0x0084
	wmNcactivate  = 0x0086
	wmNcpaint     = 0x0085
	wmDpichanged  = 0x02E0
	wmGetdpiscaledsize = 0x02E4

	// ShowWindow commands
	swMinimize  = 6
	swMaximize  = 3
	swRestore   = 9
	swShow      = 5

	// WM_SIZE params
	sizeMaximized = 2

	// WM_SYSCOMMAND
	scMinimize = 0xF020
	scMaximize = 0xF030
	scRestore  = 0xF120

	// BitBlt raster ops
	srccopy = 0x00CC0020

	// DIB types
	dibRgbColors = 0

	// Color depth
	biRgb = 0

	// CS_* class styles
	csHredraw = 0x0002
	csVredraw = 0x0001
	csOwndc   = 0x0020

	// Cursor
	idcArrow = 32512
)

// ─── Win32 API структуры ────────────────────────────────────────────────────

type wndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       windows.Handle
}

type point struct {
	X, Y int32
}

type msg struct {
	HWnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type rect struct {
	Left, Top, Right, Bottom int32
}

type paintstruct struct {
	HDC         uintptr
	FErase      int32
	RcPaint     rect
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

type bitmapInfoHeader struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfo struct {
	BmiHeader bitmapInfoHeader
	BmiColors [1]uint32
}

// ─── Win32 API процедуры ────────────────────────────────────────────────────

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
	procUpdateWindow        = user32.NewProc("UpdateWindow")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procSetWindowTextW      = user32.NewProc("SetWindowTextW")
	procMoveWindow          = user32.NewProc("MoveWindow")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procGetClientRect       = user32.NewProc("GetClientRect")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procIsZoomed            = user32.NewProc("IsZoomed")
	procLoadCursorW         = user32.NewProc("LoadCursorW")
	procBeginPaint          = user32.NewProc("BeginPaint")
	procEndPaint            = user32.NewProc("EndPaint")
	procInvalidateRect      = user32.NewProc("InvalidateRect")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procSetWindowLongPtrW   = user32.NewProc("SetWindowLongPtrW")
	procGetWindowLongPtrW   = user32.NewProc("GetWindowLongPtrW")

	procStretchDIBits = gdi32.NewProc("StretchDIBits")
	procSetStretchBltMode = gdi32.NewProc("SetStretchBltMode")
)

// ─── Win32Window ────────────────────────────────────────────────────────────

// Win32Window — реализация NativeWindow через Win32 API.
// Чистый Go, без CGO. Работает через golang.org/x/sys/windows.
type Win32Window struct {
	hwnd   windows.HWND
	width  int
	height int
	title  string

	maximized bool

	mu      sync.Mutex
	frameBuf []byte // BGRA пиксели для StretchDIBits (перевёрнуто по Y)

	// Callbacks
	onResize      func(w, h int)
	onClose       func() bool
	onMouseMove   func(x, y int)
	onMouseButton func(x, y, button int, pressed bool)
	onKeyDown     func(vk int)
	onKeyUp       func(vk int)
	onChar        func(r rune)
}

// Глобальный указатель на окно для WndProc (Win32 callback не может быть методом).
var globalWin32 *Win32Window

func NewNativeWindow() NativeWindow {
	return &Win32Window{}
}

func (w *Win32Window) Create(title string, width, height int) error {
	runtime.LockOSThread() // Win32 UI должен работать в одном потоке

	w.title = title
	w.width = width
	w.height = height
	globalWin32 = w

	className, _ := windows.UTF16PtrFromString("HeadlessGUI_WndClass")
	titlePtr, _ := windows.UTF16PtrFromString(title)

	hInst := windows.Handle(0)

	// Загружаем курсор
	cursor, _, _ := procLoadCursorW.Call(0, uintptr(idcArrow))

	wc := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		Style:         csHredraw | csVredraw | csOwndc,
		LpfnWndProc:   windows.NewCallback(wndProc),
		HInstance:     hInst,
		HCursor:       windows.Handle(cursor),
		LpszClassName: className,
	}

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Borderless popup window с поддержкой resize и minimize/maximize
	style := uint32(wsPopup | wsVisible | wsMinimizebox | wsMaximizebox | wsThickframe | wsSysmenu | wsClipchildren)
	exStyle := uint32(wsExAppwindow)

	// Вычисляем позицию по центру экрана
	screenW := getSystemMetrics(0) // SM_CXSCREEN
	screenH := getSystemMetrics(1) // SM_CYSCREEN
	x := (screenW - width) / 2
	y := (screenH - height) / 2

	hwnd, _, err := procCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(style),
		uintptr(x), uintptr(y),
		uintptr(width), uintptr(height),
		0, 0,
		uintptr(hInst),
		0,
	)
	if hwnd == 0 {
		return err
	}
	w.hwnd = windows.HWND(hwnd)

	procShowWindow.Call(hwnd, uintptr(swShow))
	procUpdateWindow.Call(hwnd)

	return nil
}

func (w *Win32Window) RunEventLoop() error {
	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(
			uintptr(unsafe.Pointer(&m)),
			0, 0, 0,
		)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	return nil
}

func (w *Win32Window) Close() {
	if w.hwnd != 0 {
		procDestroyWindow.Call(uintptr(w.hwnd))
		w.hwnd = 0
	}
}

func (w *Win32Window) SetTitle(title string) {
	w.title = title
	if w.hwnd != 0 {
		ptr, _ := windows.UTF16PtrFromString(title)
		procSetWindowTextW.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(ptr)))
	}
}

func (w *Win32Window) SetSize(width, height int) {
	w.width = width
	w.height = height
	if w.hwnd != 0 {
		var r rect
		procGetWindowRect.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&r)))
		procMoveWindow.Call(
			uintptr(w.hwnd),
			uintptr(r.Left), uintptr(r.Top),
			uintptr(width), uintptr(height),
			1, // repaint
		)
	}
}

func (w *Win32Window) GetSize() (int, int) {
	if w.hwnd != 0 {
		var r rect
		procGetClientRect.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&r)))
		return int(r.Right - r.Left), int(r.Bottom - r.Top)
	}
	return w.width, w.height
}

func (w *Win32Window) SetPosition(x, y int) {
	if w.hwnd != 0 {
		procSetWindowPos.Call(
			uintptr(w.hwnd), 0,
			uintptr(x), uintptr(y), 0, 0,
			0x0001|0x0004, // SWP_NOSIZE | SWP_NOZORDER
		)
	}
}

func (w *Win32Window) GetPosition() (int, int) {
	if w.hwnd != 0 {
		var r rect
		procGetWindowRect.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&r)))
		return int(r.Left), int(r.Top)
	}
	return 0, 0
}

func (w *Win32Window) Minimize() {
	if w.hwnd != 0 {
		procShowWindow.Call(uintptr(w.hwnd), uintptr(swMinimize))
	}
}

func (w *Win32Window) Maximize() {
	if w.hwnd != 0 {
		procShowWindow.Call(uintptr(w.hwnd), uintptr(swMaximize))
		w.maximized = true
	}
}

func (w *Win32Window) Restore() {
	if w.hwnd != 0 {
		procShowWindow.Call(uintptr(w.hwnd), uintptr(swRestore))
		w.maximized = false
	}
}

func (w *Win32Window) IsMaximized() bool {
	if w.hwnd != 0 {
		ret, _, _ := procIsZoomed.Call(uintptr(w.hwnd))
		w.maximized = ret != 0
	}
	return w.maximized
}

func (w *Win32Window) BlitRGBA(img *image.RGBA) {
	if w.hwnd == 0 || img == nil {
		return
	}
	b := img.Bounds()
	width := b.Dx()
	height := b.Dy()

	// Конвертируем RGBA → BGRA (Win32 DIB формат) и переворачиваем по Y.
	w.mu.Lock()
	needed := width * height * 4
	if len(w.frameBuf) < needed {
		w.frameBuf = make([]byte, needed)
	}
	src := img.Pix
	dst := w.frameBuf
	stride := img.Stride
	for y := 0; y < height; y++ {
		srcRow := src[(height-1-y)*stride:]
		dstOff := y * width * 4
		for x := 0; x < width; x++ {
			si := x * 4
			di := dstOff + x*4
			dst[di+0] = srcRow[si+2] // B
			dst[di+1] = srcRow[si+1] // G
			dst[di+2] = srcRow[si+0] // R
			dst[di+3] = srcRow[si+3] // A
		}
	}
	w.mu.Unlock()

	hdc, _, _ := procGetDC.Call(uintptr(w.hwnd))
	if hdc == 0 {
		return
	}
	defer procReleaseDC.Call(uintptr(w.hwnd), hdc)

	// HALFTONE для качественного масштабирования
	procSetStretchBltMode.Call(hdc, 4) // HALFTONE

	bi := bitmapInfo{
		BmiHeader: bitmapInfoHeader{
			BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			BiWidth:       int32(width),
			BiHeight:      int32(height), // positive = bottom-up (мы перевернули)
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: biRgb,
		},
	}

	w.mu.Lock()
	procStretchDIBits.Call(
		hdc,
		0, 0, uintptr(width), uintptr(height), // dst rect
		0, 0, uintptr(width), uintptr(height), // src rect
		uintptr(unsafe.Pointer(&w.frameBuf[0])),
		uintptr(unsafe.Pointer(&bi)),
		uintptr(dibRgbColors),
		uintptr(srccopy),
	)
	w.mu.Unlock()
}

// Callbacks
func (w *Win32Window) SetOnResize(fn func(w, h int))                              { w.onResize = fn }
func (w *Win32Window) SetOnClose(fn func() bool)                                   { w.onClose = fn }
func (w *Win32Window) SetOnMouseMove(fn func(x, y int))                            { w.onMouseMove = fn }
func (w *Win32Window) SetOnMouseButton(fn func(x, y, button int, pressed bool))    { w.onMouseButton = fn }
func (w *Win32Window) SetOnKeyDown(fn func(vk int))                                { w.onKeyDown = fn }
func (w *Win32Window) SetOnKeyUp(fn func(vk int))                                  { w.onKeyUp = fn }
func (w *Win32Window) SetOnChar(fn func(r rune))                                   { w.onChar = fn }

// ─── WndProc ────────────────────────────────────────────────────────────────

func wndProc(hwnd uintptr, umsg uint32, wparam, lparam uintptr) uintptr {
	w := globalWin32
	if w == nil {
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
		return ret
	}

	switch umsg {
	case wmNccalcsize:
		// Borderless окно: вся область окна = client area.
		// Убираем non-client frame (рамку от WS_THICKFRAME),
		// чтобы наш контент рисовался от самого верха без зазоров.
		if wparam != 0 {
			// wparam=TRUE → lparam указывает на NCCALCSIZE_PARAMS.
			// Возвращаем 0 — client rect = window rect (без инсетов).
			return 0
		}
		return 0

	case wmNcactivate:
		// Перехватываем отрисовку non-client area при смене фокуса.
		// Возвращаем TRUE (1) чтобы Windows считала, что мы обработали,
		// но передаём lParam = -1 чтобы DWM не перерисовывал NC-область.
		return 1

	case wmNcpaint:
		// Подавляем отрисовку non-client рамки полностью.
		// У нас borderless окно — NC-области нет, рисовать нечего.
		return 0

	case wmClose:
		if w.onClose != nil {
			if w.onClose() {
				procDestroyWindow.Call(hwnd)
			}
			return 0
		}
		procDestroyWindow.Call(hwnd)
		return 0

	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0

	case wmSize:
		newW := int(lparam & 0xFFFF)
		newH := int((lparam >> 16) & 0xFFFF)
		if newW > 0 && newH > 0 {
			w.width = newW
			w.height = newH
			if w.onResize != nil {
				w.onResize(newW, newH)
			}
		}
		return 0

	case wmPaint:
		var ps paintstruct
		procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0

	case wmErasebkgnd:
		return 1 // Не стираем фон (мы рисуем сами)

	case wmMousemove:
		x := int(int16(lparam & 0xFFFF))
		y := int(int16((lparam >> 16) & 0xFFFF))
		if w.onMouseMove != nil {
			w.onMouseMove(x, y)
		}
		return 0

	case wmLbuttondown:
		x := int(int16(lparam & 0xFFFF))
		y := int(int16((lparam >> 16) & 0xFFFF))
		if w.onMouseButton != nil {
			w.onMouseButton(x, y, 0, true)
		}
		return 0

	case wmLbuttonup:
		x := int(int16(lparam & 0xFFFF))
		y := int(int16((lparam >> 16) & 0xFFFF))
		if w.onMouseButton != nil {
			w.onMouseButton(x, y, 0, false)
		}
		return 0

	case wmRbuttondown:
		x := int(int16(lparam & 0xFFFF))
		y := int(int16((lparam >> 16) & 0xFFFF))
		if w.onMouseButton != nil {
			w.onMouseButton(x, y, 1, true)
		}
		return 0

	case wmRbuttonup:
		x := int(int16(lparam & 0xFFFF))
		y := int(int16((lparam >> 16) & 0xFFFF))
		if w.onMouseButton != nil {
			w.onMouseButton(x, y, 1, false)
		}
		return 0

	case wmMbuttondown:
		x := int(int16(lparam & 0xFFFF))
		y := int(int16((lparam >> 16) & 0xFFFF))
		if w.onMouseButton != nil {
			w.onMouseButton(x, y, 2, true)
		}
		return 0

	case wmMbuttonup:
		x := int(int16(lparam & 0xFFFF))
		y := int(int16((lparam >> 16) & 0xFFFF))
		if w.onMouseButton != nil {
			w.onMouseButton(x, y, 2, false)
		}
		return 0

	case wmKeydown:
		if w.onKeyDown != nil {
			w.onKeyDown(int(wparam))
		}
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
		return ret

	case wmKeyup:
		if w.onKeyUp != nil {
			w.onKeyUp(int(wparam))
		}
		return 0

	case wmChar:
		r := rune(wparam)
		if r >= 32 && w.onChar != nil {
			w.onChar(r)
		}
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
	return ret
}

// getSystemMetrics вызывает GetSystemMetrics.
func getSystemMetrics(index int) int {
	proc := user32.NewProc("GetSystemMetrics")
	ret, _, _ := proc.Call(uintptr(index))
	return int(ret)
}
