//go:build windows

package core

import (
	"image"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ─── Win32 API константы ────────────────────────────────────────────────────

const (
	// Window styles
	wsPopup        = 0x80000000
	wsVisible      = 0x10000000
	wsSysmenu      = 0x00080000
	wsMinimizebox  = 0x00020000
	wsMaximizebox  = 0x00010000
	wsThickframe   = 0x00040000
	wsClipchildren = 0x02000000

	// Extended styles
	wsExAppwindow           = 0x00040000
	wsExLayered             = 0x00080000
	wsExNoredirectionbitmap = 0x00200000

	// Messages
	wmDestroy       = 0x0002
	wmActivate      = 0x0006
	wmSize          = 0x0005
	wmClose         = 0x0010
	wmPaint         = 0x000F
	wmErasebkgnd    = 0x0014
	wmMousemove     = 0x0200
	wmLbuttondown   = 0x0201
	wmLbuttonup     = 0x0202
	wmLbuttondblclk = 0x0203
	wmRbuttondown   = 0x0204
	wmRbuttonup     = 0x0205
	wmRbuttondblclk = 0x0206
	wmMbuttondown   = 0x0207
	wmMbuttonup     = 0x0208
	wmMbuttondblclk = 0x0209
	wmMousewheel    = 0x020A
	// Горизонтальное колесо и наклон колеса вбок. Без него прокрутка вширь
	// на Windows не работала вовсе.
	wmMousehwheel = 0x020E

	// wheelDeltaWin — WHEEL_DELTA: единица «одного щелчка» колеса в WM_MOUSEWHEEL.
	// wheelNotchPx — во сколько логических пикселей превращается один щелчок
	// (соответствует шагу тикового колеса в движке — 40 px/notch).
	wheelDeltaWin = 120.0
	wheelNotchPx  = 40.0
	wmKeydown     = 0x0100
	wmKeyup       = 0x0101
	wmChar        = 0x0102
	// Системные клавиатурные сообщения — те, что приходят с зажатым Alt, и
	// F10. Без них ModAlt не выставлялся никогда, а F10 не доходил вовсе.
	wmSyskeydown = 0x0104
	wmSyskeyup   = 0x0105
	wmSyschar    = 0x0106
	wmDropfiles  = 0x0233 // WM_DROPFILES: wParam = HDROP (Drag&Drop файлов из ОС)
	wmSyscommand = 0x0112
	wmNccalcsize = 0x0083
	wmNchittest  = 0x0084
	// Мышь над нерабочей областью: окно объявило там свои кнопки, и рисует
	// их тоже оно — значит и подсветку с нажатием обслуживать ему.
	wmNcmousemove      = 0x00A0
	wmNclbuttondown    = 0x00A1
	wmNclbuttonup      = 0x00A2
	wmNcmouseleave     = 0x02A2
	wmGetminmaxinfo    = 0x0024
	wmNcactivate       = 0x0086
	wmNcpaint          = 0x0085
	wmSettingchange    = 0x001A // WM_SETTINGCHANGE: lParam — строка с названием области параметров
	wmDpichanged       = 0x02E0
	wmGetdpiscaledsize = 0x02E4
	wmEntersizemove    = 0x0231 // начало интерактивного перемещения/ресайза (modal loop)
	wmExitsizemove     = 0x0232 // конец интерактивного перемещения/ресайза

	// ShowWindow commands
	swMinimize       = 6
	swMaximize       = 3
	swRestore        = 9
	swShow           = 5
	swShowNoActivate = 4

	// Extended styles для окон-попапов (оверлеи): не активируется, поверх
	// носителя, без кнопки на панели задач.
	wsExToolwindow = 0x00000080
	wsExTopmost    = 0x00000008
	wsExNoactivate = 0x08000000

	// WM_SIZE params
	sizeMaximized = 2

	// WM_NCHITTEST: коды зон окна (рамка resize).
	htClient = 1
	// Зоны, которые система обслуживает сама: заголовок тащит окно с
	// прилипанием к краям, а кнопка развёртывания показывает макеты
	// привязки Windows 11.
	htCaption     = 2
	htMinButton   = 8
	htMaxButton   = 9
	htClose       = 20
	htLeft        = 10
	htRight       = 11
	htTop         = 12
	htTopLeft     = 13
	htTopRight    = 14
	htBottom      = 15
	htBottomLeft  = 16
	htBottomRight = 17

	// Ширина невидимой resize-рамки borderless-окна (физ. пиксели).
	ncResizeBorder = 8

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

	// Cursors (IDC_*)
	idcArrow    = 32512
	idcIBeam    = 32513
	idcSizeNS   = 32645
	idcSizeWE   = 32644
	idcSizeNWSE = 32642
	idcSizeNESW = 32643
	idcHand     = 32649

	// WM_SETCURSOR
	wmSetcursor = 0x0020

	// WM_APP — базовое сообщение приложения; используем для маршалинга колбэков
	// на UI-поток (InvokeOnUIThread): PostMessage(WM_APP, id) → wndProc вызывает
	// зарегистрированную функцию на потоке цикла сообщений.
	wmApp = 0x8000

	// GWLP_HWNDPARENT — индекс SetWindowLongPtr для смены owner-окна (== -8).
	gwlpHwndParent = ^uintptr(7)
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
	dwmapi = windows.NewLazySystemDLL("dwmapi.dll")

	// Drag&Drop файлов из ОС (WM_DROPFILES). shell32 объявлен в tray_windows.go.
	procDragAcceptFiles = shell32.NewProc("DragAcceptFiles")
	procDragQueryFileW  = shell32.NewProc("DragQueryFileW")
	procDragQueryPoint  = shell32.NewProc("DragQueryPoint")
	procDragFinish      = shell32.NewProc("DragFinish")

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
	procScreenToClient      = user32.NewProc("ScreenToClient")
	procEnableWindow        = user32.NewProc("EnableWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")

	// HiDPI (Win10 1703+; отсутствие процедур не фатально — Call вернёт ошибку).
	procSetProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")
	procGetDpiForSystem               = user32.NewProc("GetDpiForSystem")

	procStretchDIBits      = gdi32.NewProc("StretchDIBits")
	procSetStretchBltMode  = gdi32.NewProc("SetStretchBltMode")
	procCreateRoundRectRgn = gdi32.NewProc("CreateRoundRectRgn")
	// Прямоугольные области и их объединение — выкройка окна-попапа по
	// закрашенной части кадра (popuprgn_windows.go).
	procCreateRectRgn         = gdi32.NewProc("CreateRectRgn")
	procCombineRgn            = gdi32.NewProc("CombineRgn")
	procSetWindowRgn          = user32.NewProc("SetWindowRgn")
	// Прозрачность окна-попапа целиком на переходах (popup_fade.go).
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	// DwmExtendFrameIntoClientArea — рамка DWM в клиентской области: без неё
	// окно без неклиентской части тени не отбрасывает (applyCorners, Win10).
	procDwmExtendFrameIntoClientArea = dwmapi.NewProc("DwmExtendFrameIntoClientArea")
	procSetCapture            = user32.NewProc("SetCapture")
	procReleaseCapture        = user32.NewProc("ReleaseCapture")
	procSetCursor             = user32.NewProc("SetCursor")

	// Мониторы (рабочая область для клэмпа окон-попапов).
	procMonitorFromPoint = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW  = user32.NewProc("GetMonitorInfoW")

	// Раскладка клавиатуры (локаль).
	procGetKeyboardLayout        = user32.NewProc("GetKeyboardLayout")
	procGetKeyboardLayoutList    = user32.NewProc("GetKeyboardLayoutList")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
)

// WM_INPUTLANGCHANGEREQUEST — просьба окну сменить язык ввода (раскладку).
// Обрабатывается DefWindowProc: переключает раскладку потока окна.
const wmInputlangchangerequest = 0x0050

// langidToCode сопоставляет primary LANGID (нижние 10 бит LANGID) → код локали.
var langidToCode = map[uint32]string{
	0x01: "AR", 0x02: "BG", 0x03: "CA", 0x04: "ZH", 0x05: "CS",
	0x06: "DA", 0x07: "DE", 0x08: "EL", 0x09: "EN", 0x0A: "ES",
	0x0B: "FI", 0x0C: "FR", 0x0D: "HE", 0x0E: "HU", 0x0F: "IS",
	0x10: "IT", 0x11: "JA", 0x12: "KO", 0x13: "NL", 0x14: "NO",
	0x15: "PL", 0x16: "PT", 0x18: "RO", 0x19: "RU", 0x1A: "HR",
	0x1B: "SK", 0x1D: "SV", 0x1E: "TH", 0x1F: "TR", 0x22: "UK",
	0x24: "SL", 0x25: "ET", 0x26: "LV", 0x27: "LT", 0x29: "FA",
	0x2A: "VI", 0x2D: "EU", 0x39: "HI",
}

// langidCode возвращает код локали по LANGID (низкое слово HKL) или "".
func langidCode(langid uint32) string {
	return langidToCode[langid&0x3FF]
}

// ─── Win32Window ────────────────────────────────────────────────────────────

// Win32Window — реализация NativeWindow через Win32 API.
// Чистый Go, без CGO. Работает через golang.org/x/sys/windows.
type Win32Window struct {
	// ownIcons — значки окна, созданные нами (WM_SETICON, см. icon_windows.go).
	// Держим хэндлы, чтобы удалить прежний при замене: значок из ресурсов
	// исполняемого файла принадлежит модулю, и уничтожать его нельзя.
	ownIcons     [2]windows.Handle
	pendingIcons []image.Image

	hwnd   windows.HWND
	width  int
	height int
	title  string

	// initial — положение и «развёрнуто» от приложения (initial.go); читает
	// Create. Задаётся до создания окна.
	initial initialPlacement

	maximized bool

	// fs — полный экран (fullscreen_windows.go). Пока он включён, maximized
	// хранит «было ли окно развёрнуто до него» — IsMaximized возвращает это же.
	fs fullscreenWin32

	// act — свёрнутость и её уведомление (activate_windows.go).
	act activateWin32

	// resizable — разрешён ли пользовательский resize за края окна
	// (см. WM_NCHITTEST: у borderless-окна зоны рамки отдаём вручную).
	resizable atomic.Bool

	// cornerRadius — радиус скругления углов окна (0 = прямые). Реализуется
	// через регион окна (SetWindowRgn), переприменяется при resize.
	cornerRadius int
	// popupBands — выкройка окна-попапа, применённая последней. Нужна,
	// чтобы не переустанавливать её на каждый блит: SetWindowRgn перестраивает
	// форму окна, и лишний вызов виден как мигание.
	popupBands []image.Rectangle

	// pendingClose: Close() был вызван из кода (не от ОС).
	// Вместо синхронного DestroyWindow мы отправляем PostMessage(WM_CLOSE),
	// чтобы окно не уничтожалось во время обработки WM_LBUTTONDOWN
	// (иначе WM_LBUTTONUP пролетает в окно ОС под курсором).
	pendingClose bool

	// resizing — идёт интерактивный ресайз за рамку (между WM_ENTERSIZEMOVE и
	// WM_EXITSIZEMOVE). Пока флаг взведён, движку НЕ пересоздаём канвас на каждый
	// WM_SIZE (дорого + мигает): вместо этого растягиваем последний кадр из кэша
	// (frameBuf), а чёткую перерисовку делаем один раз на WM_EXITSIZEMOVE.
	resizing atomic.Bool

	// minW/minH — минимальный размер окна В ФИЗИЧЕСКИХ пикселях (учёт HiDPI-scale
	// делает вызывающая сторона). 0 → дефолт 320×240. Отдаётся в WM_GETMINMAXINFO.
	minW, minH int

	// noQuit — окно НЕ завершает цикл сообщений при своём уничтожении
	// (WM_DESTROY не шлёт PostQuitMessage). Взводится для вторичных окон модалок
	// (owned windows): их закрытие не должно ронять приложение.
	noQuit bool

	// popupAlpha — прозрачность слоистого окна-попапа (SetPopupAlpha); до
	// первого вызова — непрозрачное. Пишет и читает поток окна.
	popupAlpha    float64
	popupAlphaSet bool

	mu         sync.Mutex
	frameBuf   []byte // BGRA пиксели для StretchDIBits (top-down: строка 0 — верхняя)
	bufW, bufH int    // размер кадра, лежащего в frameBuf (для растянутого блита)
	// frameVA — frameBuf выделен VirtualAlloc, а не Go (framebuf_windows.go);
	// frameDead — окно уничтожено, буфер освобождён. blitFails — сколько
	// блитов подряд GDI не вывел; paintMiss — WM_PAINT пришёл, когда
	// перерисовка ещё не была подключена (SetOnExpose).
	frameVA   bool
	frameDead bool
	blitFails atomic.Int32
	paintMiss atomic.Bool

	cursorHandle uintptr         // текущий желаемый курсор (HCURSOR)
	cursorCache  map[int]uintptr // кэш загруженных IDC-курсоров
	cursorHidden bool            // курсор спрятан (CursorNone): WM_SETCURSOR ставит SetCursor(NULL)

	// imgCursors — курсоры из картинок (Engine.SetCursorImage), по ключу
	// содержимого, не более cursorCacheSize; imgCreated — сколько курсоров
	// построено за жизнь окна (в тестах проверяется, что кэш работает).
	// Только поток окна. См. cursorimage_windows.go.
	imgCursors *cursorLRU[uintptr]
	imgCreated int

	// lock — захват указателя (SetPointerLock), см. pointerlock_windows.go.
	lock win32PointerLock

	// Уход указателя: запросы TrackMouseEvent на клиентскую и нерабочую
	// область (pointerleave.go, mouse_windows.go). trackMouse — подмена
	// системного вызова в тестах.
	leave      leaveTracker
	ncLeave    leaveTracker
	trackMouse func(hwnd uintptr, flags uint32) bool

	// Трей/уведомления (Shell_NotifyIcon) — см. tray_windows.go.
	trayAdded      bool           // иконка добавлена (NIM_ADD выполнен)
	trayHIcon      windows.Handle // текущий HICON иконки трея (уничтожаем при замене)
	onTrayClick    func(button int, doubleClick bool)
	onBalloonClick func()
	iconicEnabled  bool // включено iconic-представление окна (DWM превью)

	// onSettingChange — разбор WM_SETTINGCHANGE: зовётся с текстом из lParam
	// (см. systheme_windows.go). Атомарно: подписку ставит и снимает
	// горутина приложения, а читает поток цикла сообщений окна.
	onSettingChange atomic.Pointer[func(area string)]
	// onAccentChange — то же для акцентного цвета (watchSystemAccent).
	onAccentChange atomic.Pointer[func(area string)]

	// kbGrabbed — окно заказало захват клавиатуры (keygrab_windows.go).
	// Атомарно: читает KeyboardGrabActive с любой горутины.
	kbGrabbed atomic.Bool

	// onMonitors — подписка на смену мониторов (monitor_change_windows.go).
	// Атомарно по той же причине, что и onSettingChange.
	onMonitors atomic.Pointer[func()]

	// Callbacks
	onResize           func(w, h int)
	onExpose           func(r image.Rectangle)
	onDpiChanged       func(scale float64)
	onActivate         func(active bool)
	onClose            func() bool
	onMouseMove        func(x, y int)
	onMouseButton      func(x, y, button int, pressed bool)
	onMouseWheelPixels func(x, y int, dx, dy float64)
	onKeyDown          func(vk int)
	// hitTest — зоны окна для системы (hittest_windows.go). Колбэк зовётся
	// из насоса сообщений, а ставится с горутины приложения — отсюда замок.
	hitTestMu sync.Mutex
	hitTest   func(x, y int) HitArea
	// highSurrogate — первая половина суррогатной пары UTF-16, ждущая
	// второго WM_CHAR.
	highSurrogate   rune
	onKeyDownRepeat func(vk int, repeat bool)
	onKeyUp         func(vk int)
	// onRawKey — приёмник клавиш со скан-кодом (rawKeySource): когда задан,
	// onKeyDown/onKeyDownRepeat/onKeyUp не вызываются.
	onRawKey func(k rawKey)
	// onWheelRaw — приёмник сырого колеса (wheelRawSource).
	onWheelRaw func(x, y int, dx, dy float64, precise bool, legacy func())
	onChar     func(r rune)

	// Редактор метода ввода (ime_windows.go): набираемое, готовый текст и
	// место каретки для окна кандидатов.
	onIMEComposition func(text string, caret int)
	onIMECommit      func(text string)
	imeCaret         func() (image.Rectangle, bool)
	onFilesDropped   func(paths []string, x, y int)

	// fileDropEnabled — приём файлов для этого окна уже включён (IDropTarget
	// или, запасным путём, DragAcceptFiles).
	fileDropEnabled bool

	// События приёмника перетаскивания и OLE-регистрация (dnd_windows.go).
	onDragEnter func(x, y int)
	onDragOver  func(x, y int) bool
	onDragLeave func()
	dropTarget  *dropTargetState // нет — окно принимает файлы через WM_DROPFILES
	oleReady    bool             // OLE поднят на потоке окна: годится для DoDragDrop

	// touch — касания (WM_POINTER*, touch_windows.go).
	touch win32TouchState
}

// Реестр окон для WndProc (Win32 callback не может быть методом): маршрутизация
// сообщений к нужному *Win32Window по hwnd. Поддерживает несколько окон на одном
// потоке (главное окно + вторичные окна модалок).
var (
	win32Mu       sync.Mutex
	win32Windows  = map[uintptr]*Win32Window{} // hwnd → окно
	win32Creating *Win32Window                 // окно в процессе Create (hwnd ещё не назначен)
)

func lookupWin32(hwnd uintptr) *Win32Window {
	win32Mu.Lock()
	defer win32Mu.Unlock()
	return win32Windows[hwnd]
}

// ─── Маршалинг колбэков на UI-поток (InvokeOnUIThread) ──────────────────────

// uiCall — отложенный колбэк вместе с окном-адресатом.
type uiCall struct {
	hwnd uintptr
	fn   func()
}

var (
	uiCallMu  sync.Mutex
	uiCallSeq uintptr
	uiCalls   = map[uintptr]uiCall{}
)

// queueUICall кладёт колбэк в очередь и возвращает его id.
func queueUICall(hwnd uintptr, fn func()) uintptr {
	uiCallMu.Lock()
	defer uiCallMu.Unlock()
	uiCallSeq++
	uiCalls[uiCallSeq] = uiCall{hwnd: hwnd, fn: fn}
	return uiCallSeq
}

// takeUICall забирает колбэк по id (или nil, если его уже нет).
func takeUICall(id uintptr) func() {
	uiCallMu.Lock()
	defer uiCallMu.Unlock()
	c := uiCalls[id]
	delete(uiCalls, id)
	return c.fn
}

// dropUICalls выбрасывает колбэки разрушенного окна: их WM_APP уже не придёт.
func dropUICalls(hwnd uintptr) int {
	uiCallMu.Lock()
	defer uiCallMu.Unlock()
	n := 0
	for id, c := range uiCalls {
		if c.hwnd == hwnd {
			delete(uiCalls, id)
			n++
		}
	}
	return n
}

// InvokeOnUIThread выполняет fn на потоке цикла сообщений окна (PostMessage
// WM_APP). Безопасно из любой горутины. Если вызвать с уже-UI-потока (изнутри
// wndProc), fn выполнится после возврата из текущего сообщения.
func (w *Win32Window) InvokeOnUIThread(fn func()) {
	if fn == nil || w.hwnd == 0 {
		return
	}
	hwnd := uintptr(w.hwnd)
	id := queueUICall(hwnd, fn)
	if ret, _, _ := procPostMessageW.Call(hwnd, uintptr(wmApp), id, 0); ret == 0 {
		takeUICall(id) // окно уже мертво — колбэк не доедет
	}
}

// markSecondary помечает окно вторичным (noQuit) без смены владельца: его
// уничтожение не завершает цикл сообщений приложения (secondaryMarker). Звать
// до Create.
func (w *Win32Window) markSecondary() { w.noQuit = true }

// SetOwner делает окно принадлежащим parent (Win32 GWLP_HWNDPARENT): оно всегда
// поверх owner'а и сворачивается вместе с ним. Помечает окно как вторичное
// (noQuit) — его закрытие не завершает цикл сообщений приложения.
func (w *Win32Window) SetOwner(parent NativeWindow) {
	w.noQuit = true
	if w.hwnd == 0 {
		return
	}
	pw, ok := parent.(*Win32Window)
	if !ok || pw.hwnd == 0 {
		return
	}
	procSetWindowLongPtrW.Call(uintptr(w.hwnd), gwlpHwndParent, uintptr(pw.hwnd))
}

// SetEnabled включает/выключает окно (EnableWindow). Отключённое окно не
// получает ввод — так владелец блокируется на время модального диалога.
func (w *Win32Window) SetEnabled(v bool) {
	if w.hwnd == 0 {
		return
	}
	var b uintptr
	if v {
		b = 1
	}
	procEnableWindow.Call(uintptr(w.hwnd), b)
}

// SetForeground выводит окно на передний план и передаёт ему фокус ОС.
func (w *Win32Window) SetForeground() {
	if w.hwnd != 0 {
		procSetForegroundWindow.Call(uintptr(w.hwnd))
	}
}

func NewNativeWindow() NativeWindow {
	return &Win32Window{}
}

// SetResizable включает/выключает пользовательский resize за края
// (реализован через WM_NCHITTEST — см. wndproc). Потокобезопасно.
func (w *Win32Window) SetResizable(v bool) { w.resizable.Store(v) }

func (w *Win32Window) Create(title string, width, height int) error {
	// Borderless popup window с поддержкой resize и minimize/maximize.
	style := uint32(wsPopup | wsVisible | wsMinimizebox | wsMaximizebox | wsThickframe | wsSysmenu | wsClipchildren)
	exStyle := uint32(wsExAppwindow)
	// Развёрнутое окно создаётся скрытым и первым же показом разворачивается
	// (initial.go): положение — в createInternal (место создания).
	_, _, style, show := win32InitialCreate(w.initial, style, swShow, 0, 0)
	if err := w.createInternal(title, width, height, style, exStyle, show, true); err != nil {
		return err
	}
	// Разрешаем приём файлов, перетащенных из проводника (WM_DROPFILES).
	// Только для главного окна — окна-попапы (CreatePopup) его не включают.
	w.enableFileDrop()
	return nil
}

// enableFileDrop включает приём Drag&Drop файлов из ОС: окно регистрируется
// приёмником OLE (IDropTarget — события входа/движения/выхода и сброс), а если
// OLE не поднялся или регистрация отказала — запасным путём DragAcceptFiles
// (WM_DROPFILES, только сброс). Идемпотентно; no-op до создания окна.
//
// Регистрация OLE привязана к потоку, на котором создано окно, поэтому
// реально работает только вызов из Create; поздний вызов из
// SetOnFilesDropped видит уже включённый приём и ничего не делает.
func (w *Win32Window) enableFileDrop() {
	if w.hwnd == 0 || w.fileDropEnabled {
		return
	}
	w.fileDropEnabled = true
	if w.registerDropTarget() {
		return
	}
	procDragAcceptFiles.Call(uintptr(w.hwnd), 1)
}

// CreatePopup создаёт окно-вьюпорт оверлея (dropdown/меню): WS_POPUP без рамки,
// не активируется (WS_EX_NOACTIVATE), поверх носителя (WS_EX_TOPMOST), без
// кнопки на панели задач (WS_EX_TOOLWINDOW). Позиция задаётся позже (SetPosition).
// Помечается noQuit — уничтожение попапа не завершает цикл сообщений приложения.
//
// Окно слоистое (WS_EX_LAYERED): прозрачность целиком задаёт
// SetLayeredWindowAttributes, а рисуется оно как прежде — StretchDIBits в
// WM_PAINT и выкройка SetWindowRgn. Без атрибутов слоистое окно невидимо,
// поэтому они ставятся до показа (createInternal).
func (w *Win32Window) CreatePopup(width, height int) error {
	w.noQuit = true
	style := uint32(wsPopup)
	exStyle := uint32(wsExNoactivate | wsExToolwindow | wsExTopmost | wsExLayered)
	return w.createInternal("", width, height, style, exStyle, swShowNoActivate, false)
}

// SetPopupAlpha задаёт прозрачность окна-попапа целиком (0…1); реализует
// popupAlphaSetter. До создания окна запоминается и ставится при создании.
func (w *Win32Window) SetPopupAlpha(a float64) {
	w.popupAlpha, w.popupAlphaSet = a, true
	if w.hwnd != 0 {
		w.applyLayeredAlpha()
	}
}

// applyLayeredAlpha кладёт прозрачность в слоистое окно.
func (w *Win32Window) applyLayeredAlpha() {
	a := 255
	if w.popupAlphaSet {
		a = int(w.popupAlpha*255 + 0.5)
		if a < 0 {
			a = 0
		}
		if a > 255 {
			a = 255
		}
	}
	const lwaAlpha = 0x2
	procSetLayeredWindowAttributes.Call(uintptr(w.hwnd), 0, uintptr(a), lwaAlpha)
}

func (w *Win32Window) createInternal(title string, width, height int, style, exStyle uint32, showCmd uintptr, centerOnScreen bool) error {
	runtime.LockOSThread() // Win32 UI должен работать в одном потоке

	w.title = title
	w.width = width
	w.height = height
	w.frameDead = false
	// Окно, которое просят создать меньше дефолтного минимума (320×240),
	// иначе вырастет до него: Windows клампит размер по ptMinTrackSize уже
	// при создании. Кадр остаётся прежним, и снизу/справа остаётся
	// непрокрашенная полоса — так «рамка» окна модалки оказывалась больше
	// самого диалога. Явно заданный SetMinSize имеет приоритет.
	w.minW, w.minH = fitMinTrack(w.minW, w.minH, width, height)
	// Пока hwnd не назначен, wndProc маршрутизирует сообщения создаваемого окна
	// (WM_NCCALCSIZE/WM_CREATE приходят синхронно внутри CreateWindowExW) на
	// win32Creating.
	win32Mu.Lock()
	win32Creating = w
	win32Mu.Unlock()

	className, _ := windows.UTF16PtrFromString("HeadlessGUI_WndClass")
	titlePtr, _ := windows.UTF16PtrFromString(title)

	hInst := windows.Handle(0)

	// Загружаем курсор
	cursor, _, _ := procLoadCursorW.Call(0, uintptr(idcArrow))

	// ВАЖНО: без CS_HREDRAW|CS_VREDRAW. Эти флаги заставляют ОС инвалидировать
	// ВСЮ клиентскую область на каждый тик ресайза (WM_SIZE сыпется по пикселю),
	// что даёт мерцание. Мы сами закрашиваем окно в WM_SIZE (растянутый кэш кадра),
	// поэтому полная инвалидация не нужна. CS_OWNDC оставляем (GetDC переиспользует DC).
	// RegisterClassExW повторно (для второго/попап-окна) — no-op: класс уже есть.
	wc := wndClassExW{
		CbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		Style:         csOwndc,
		LpfnWndProc:   windows.NewCallback(wndProc),
		HInstance:     hInst,
		HCursor:       windows.Handle(cursor),
		LpszClassName: className,
	}

	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	// Позиция: по центру экрана (главное окно) или (0,0) — попап позиционирует хост.
	x, y := 0, 0
	if centerOnScreen {
		screenW := getSystemMetrics(0) // SM_CXSCREEN
		screenH := getSystemMetrics(1) // SM_CYSCREEN
		x = (screenW - width) / 2
		y = (screenH - height) / 2
		// Положение от приложения (SetInitialBounds) вместо центра экрана:
		// окно рождается сразу на месте, без прыжка после показа.
		x, y, _, _ = win32InitialCreate(w.initial, 0, 0, x, y)
	}

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
		win32Mu.Lock()
		win32Creating = nil
		win32Mu.Unlock()
		return err
	}
	w.hwnd = windows.HWND(hwnd)
	win32Mu.Lock()
	win32Windows[hwnd] = w
	win32Creating = nil
	win32Mu.Unlock()

	// Значок, заданный до создания окна: SetIcon мог прийти раньше Run.
	w.applyPendingIcon()

	// Слоистое окно (попап) без атрибутов невидимо — ставим до показа.
	if exStyle&wsExLayered != 0 {
		w.applyLayeredAlpha()
	}

	procShowWindow.Call(hwnd, showCmd)
	procUpdateWindow.Call(hwnd)

	// Главное окно (не попап): опциональный iconic-путь превью в панели задач.
	if centerOnScreen {
		w.maybeEnableIconic()
	}

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
	if w.hwnd != 0 && !w.pendingClose {
		// Не вызываем DestroyWindow синхронно — это уничтожило бы окно
		// во время обработки WM_LBUTTONDOWN, и WM_LBUTTONUP пролетел бы
		// в окно ОС под курсором (click-through).
		// PostMessage отложит закрытие: сначала придёт WM_LBUTTONUP,
		// затем WM_CLOSE → DestroyWindow.
		w.pendingClose = true
		procPostMessageW.Call(uintptr(w.hwnd), uintptr(wmClose), 0, 0)
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
	if w.fs.on.Load() {
		// Полноэкранное окно держит размер монитора. Заказанный размер
		// достанется окну при выходе: в снимок его положения.
		w.fs.mu.Lock()
		w.fs.snap.Rect.Max = w.fs.snap.Rect.Min.Add(image.Pt(width, height))
		w.fs.mu.Unlock()
		return
	}
	if w.hwnd != 0 {
		if z, _, _ := procIsZoomed.Call(uintptr(w.hwnd)); z != 0 && w.setNormalSize(width, height) {
			// Развёрнутое окно занимает рабочую область, и MoveWindow для
			// него не имеет смысла: заказанный размер получает нормальный
			// прямоугольник — окно примет его при «восстановить». GetSize
			// отвечает размером развёрнутого окна.
			return
		}
	}
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

// windowPlacement — WINDOWPLACEMENT: состояние показа и нормальный
// прямоугольник окна.
type windowPlacement struct {
	Length           uint32
	Flags            uint32
	ShowCmd          uint32
	PtMinPosition    [2]int32
	PtMaxPosition    [2]int32
	RcNormalPosition rect
}

var (
	procGetWindowPlacement = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement = user32.NewProc("SetWindowPlacement")
)

// setNormalSize меняет размер НОРМАЛЬНОГО прямоугольника окна (того, в который
// окно вернётся из развёрнутого состояния), не трогая сам показ. false —
// система отказала, вызывающий поступает как с обычным окном.
func (w *Win32Window) setNormalSize(width, height int) bool {
	var wp windowPlacement
	wp.Length = uint32(unsafe.Sizeof(wp))
	if r, _, _ := procGetWindowPlacement.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&wp))); r == 0 {
		return false
	}
	wp.RcNormalPosition.Right = wp.RcNormalPosition.Left + int32(width)
	wp.RcNormalPosition.Bottom = wp.RcNormalPosition.Top + int32(height)
	r, _, _ := procSetWindowPlacement.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&wp)))
	return r != 0
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
	if w.fs.on.Load() {
		// То же, что в SetSize: позиция применится при выходе.
		w.fs.mu.Lock()
		w.fs.snap.Rect = image.Rectangle{Min: image.Pt(x, y), Max: image.Pt(x, y).Add(w.fs.snap.Rect.Size())}
		w.fs.mu.Unlock()
		return
	}
	if w.hwnd != 0 {
		// SWP_NOACTIVATE обязателен: перемещение не должно активировать окно.
		// Без него позиционирование окна-попапа (WS_EX_NOACTIVATE защищает
		// только от активации мышью) крало фокус у носителя, тот ловил
		// WM_ACTIVATE(false) → CloseAllOverlays — попап закрывался мгновенно.
		procSetWindowPos.Call(
			uintptr(w.hwnd), 0,
			uintptr(x), uintptr(y), 0, 0,
			0x0001|0x0004|0x0010, // SWP_NOSIZE | SWP_NOZORDER | SWP_NOACTIVATE
		)
	}
}

// monitorInfo — MONITORINFO (GetMonitorInfoW).
type monitorInfo struct {
	CbSize    uint32
	RcMonitor rect
	RcWork    rect
	DwFlags   uint32
}

// WorkAreaAt возвращает рабочую область монитора, содержащего точку (x, y):
// экран минус таскбар. Используется popupHost для вписывания окон-попапов
// (меню у трея иначе раскрывалось под таскбар). Пустой Rect при ошибке.
func (w *Win32Window) WorkAreaAt(x, y int) image.Rectangle {
	const monitorDefaultToNearest = 2
	pt := uintptr(uint32(x)) | uintptr(uint32(y))<<32
	hmon, _, _ := procMonitorFromPoint.Call(pt, monitorDefaultToNearest)
	if hmon == 0 {
		return image.Rectangle{}
	}
	var mi monitorInfo
	mi.CbSize = uint32(unsafe.Sizeof(mi))
	ret, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi)))
	if ret == 0 {
		return image.Rectangle{}
	}
	return image.Rect(int(mi.RcWork.Left), int(mi.RcWork.Top), int(mi.RcWork.Right), int(mi.RcWork.Bottom))
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
	if w.fs.on.Load() {
		// Развернуть полноэкранное окно значило бы вернуть таскбар. Просьба
		// запоминается: после выхода окно будет развёрнуто.
		w.fs.mu.Lock()
		w.fs.snap.Maximized, w.maximized = true, true
		w.fs.mu.Unlock()
		return
	}
	if w.hwnd != 0 {
		procShowWindow.Call(uintptr(w.hwnd), uintptr(swMaximize))
		w.maximized = true
	}
}

func (w *Win32Window) Restore() {
	if w.fs.on.Load() {
		w.fs.mu.Lock()
		w.fs.snap.Maximized, w.maximized = false, false
		w.fs.mu.Unlock()
		return
	}
	if w.hwnd != 0 {
		procShowWindow.Call(uintptr(w.hwnd), uintptr(swRestore))
		w.maximized = false
	}
}

func (w *Win32Window) IsMaximized() bool {
	if w.fs.on.Load() {
		// Полноэкранное окно системе не «развёрнуто» (IsZoomed ложно), а
		// приложение спрашивает про прежнее значение.
		w.fs.mu.Lock()
		defer w.fs.mu.Unlock()
		return w.fs.snap.Maximized
	}
	if w.hwnd != 0 {
		ret, _, _ := procIsZoomed.Call(uintptr(w.hwnd))
		w.maximized = ret != 0
	}
	return w.maximized
}

func (w *Win32Window) BlitRGBA(img *image.RGBA) {
	if img == nil {
		return
	}
	w.BlitRGBADirty(img, img.Bounds())
}

// convertFrameToBGRA конвертирует прямоугольник dirty кадра img в BGRA-буфер
// dst шириной width пикселей, БЕЗ переворота по Y (top-down DIB, BiHeight<0).
// Строка y кадра ложится в строку y буфера — порядок строк совпадает, поэтому
// переворот не нужен, а конвертация идёт построчно быстрым 32-битным swap'ом.
func convertFrameToBGRA(dst []byte, img *image.RGBA, dirty image.Rectangle, width int) {
	x0, x1 := dirty.Min.X*4, dirty.Max.X*4
	rowLen := x1 - x0
	if rowLen <= 0 {
		return
	}
	src := img.Pix
	stride := img.Stride
	for y := dirty.Min.Y; y < dirty.Max.Y; y++ {
		s := y*stride + x0
		d := y*width*4 + x0
		swapRBRow(dst[d:d+rowLen], src[s:s+rowLen])
	}
}

// BlitRGBADirty выводит только изменившуюся область dirty: RGBA→BGRA
// конвертация и StretchDIBits ограничиваются этой областью.
// Буфер кадра переиспользуется между вызовами (полный кадр, top-down DIB).
func (w *Win32Window) BlitRGBADirty(img *image.RGBA, dirty image.Rectangle) {
	if w.hwnd == 0 || img == nil {
		return
	}
	b := img.Bounds()
	width := b.Dx()
	height := b.Dy()
	dirty = dirty.Intersect(b)
	if dirty.Empty() {
		return
	}

	// Конвертируем RGBA → BGRA (Win32 DIB формат) — только строки/столбцы
	// dirty-области. Переворота по Y нет: DIB объявлен top-down (BiHeight<0).
	w.mu.Lock()
	if w.frameDead {
		w.mu.Unlock()
		return
	}
	needed := width * height * 4
	if len(w.frameBuf) < needed {
		w.allocFrameBuf(needed)
		dirty = b // новый буфер — заполняем целиком
	}
	convertFrameToBGRA(w.frameBuf, img, dirty, width)
	w.bufW = width
	w.bufH = height
	// Мьютекс удерживается до конца StretchDIBits: блит может прийти
	// одновременно из framePump и из WM_PAINT (event loop).

	// Во время интерактивного ресайза размер окна ОС уже больше/меньше нашего
	// буфера. Обычный dirty-блит покрыл бы лишь старый прямоугольник, оставив
	// непокрытые полосы мигать фоном. Поэтому растягиваем весь кадр на всю
	// клиентскую область — плавно, без вспышек, до готовности чёткого кадра.
	if w.resizing.Load() {
		w.blitStretchedLocked()
		w.mu.Unlock()
		return
	}

	hdc, _, _ := procGetDC.Call(uintptr(w.hwnd))
	if hdc == 0 {
		w.mu.Unlock()
		return
	}
	defer procReleaseDC.Call(uintptr(w.hwnd), hdc)

	// HALFTONE для качественного масштабирования
	procSetStretchBltMode.Call(hdc, 4) // HALFTONE

	bi := bitmapInfo{
		BmiHeader: bitmapInfoHeader{
			BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			BiWidth:       int32(width),
			BiHeight:      -int32(height), // negative = top-down (переворот не нужен)
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: biRgb,
		},
	}

	dw := dirty.Dx()
	dh := dirty.Dy()
	// ВНИМАНИЕ (проверено визуально, PERF-2): знак BiHeight задаёт только
	// ПОРЯДОК ХРАНЕНИЯ строк в буфере. Система координат ИСТОЧНИКА у
	// StretchDIBits остаётся bottom-up и для top-down DIB: YSrc по-прежнему
	// отсчитывается от НИЖНЕГО края изображения. С YSrc = dirty.Min.Y частичные
	// (dirty) блиты брали строки из зеркального места кадра — окно показывало
	// чужой кусок интерфейса на месте перерисованного виджета.
	ySrc := height - dirty.Max.Y

	lines, _, _ := procStretchDIBits.Call(
		hdc,
		uintptr(dirty.Min.X), uintptr(dirty.Min.Y), uintptr(dw), uintptr(dh), // dst rect
		uintptr(dirty.Min.X), uintptr(ySrc), uintptr(dw), uintptr(dh), // src rect
		uintptr(unsafe.Pointer(&w.frameBuf[0])),
		uintptr(unsafe.Pointer(&bi)),
		uintptr(dibRgbColors),
		uintptr(srccopy),
	)
	w.mu.Unlock()
	// Кадр, который GDI не вывел, иначе считался бы показанным: интерфейс в
	// покое новых кадров не шлёт, и окно оставалось бы пустым навсегда.
	if lines == 0 {
		w.blitFailed()
	} else {
		w.blitFails.Store(0)
	}
}

// blitStretchedLocked растягивает кэшированный кадр (frameBuf, bufW×bufH) на
// всю текущую клиентскую область окна. Вызывается С УДЕРЖАННЫМ w.mu. Якорь —
// левый-верхний угол, масштаб COLORONCOLOR (быстрый, для интерактивного resize).
// Непокрытых областей не остаётся: кадр растянут на всё окно, поэтому фон
// нигде не вспыхивает, пока движок не отрисует чёткий кадр нового размера.
func (w *Win32Window) blitStretchedLocked() {
	if w.hwnd == 0 || len(w.frameBuf) == 0 || w.bufW <= 0 || w.bufH <= 0 {
		return
	}
	var cr rect
	procGetClientRect.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&cr)))
	cw := int(cr.Right - cr.Left)
	ch := int(cr.Bottom - cr.Top)
	if cw <= 0 || ch <= 0 {
		return
	}
	hdc, _, _ := procGetDC.Call(uintptr(w.hwnd))
	if hdc == 0 {
		return
	}
	defer procReleaseDC.Call(uintptr(w.hwnd), hdc)

	procSetStretchBltMode.Call(hdc, 3) // COLORONCOLOR — дешёвый скейл на каждый тик

	bi := bitmapInfo{
		BmiHeader: bitmapInfoHeader{
			BiSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			BiWidth:       int32(w.bufW),
			BiHeight:      -int32(w.bufH), // negative = top-down (буфер не перевёрнут)
			BiPlanes:      1,
			BiBitCount:    32,
			BiCompression: biRgb,
		},
	}
	procStretchDIBits.Call(
		hdc,
		0, 0, uintptr(cw), uintptr(ch), // dst = вся клиентская область
		0, 0, uintptr(w.bufW), uintptr(w.bufH), // src = весь кэшированный кадр
		uintptr(unsafe.Pointer(&w.frameBuf[0])),
		uintptr(unsafe.Pointer(&bi)),
		uintptr(dibRgbColors),
		uintptr(srccopy),
	)
}

// blitCachedStretched — обёртка blitStretchedLocked с захватом мьютекса.
func (w *Win32Window) blitCachedStretched() {
	w.mu.Lock()
	w.blitStretchedLocked()
	w.mu.Unlock()
}

// SetMinSize задаёт минимальный размер окна В ФИЗИЧЕСКИХ пикселях (вызывающая
// сторона умножает логические на HiDPI-scale). Значение отдаётся ОС в
// WM_GETMINMAXINFO (ptMinTrackSize). 0 → дефолт 320×240.
func (w *Win32Window) SetMinSize(width, height int) {
	w.minW = width
	w.minH = height
}

// SetOnExpose — колбэк перерисовки области по WM_PAINT (см. exposeNotifier).
//
// WM_PAINT, пришедший раньше (первый показ внутри Create — до того, как окно
// подключит перерисовку), BeginPaint уже провалидировал, и система больше не
// попросила бы перерисовать окно: просим заново.
func (w *Win32Window) SetOnExpose(fn func(r image.Rectangle)) {
	w.onExpose = fn
	if fn != nil && w.hwnd != 0 && w.paintMiss.Swap(false) {
		procInvalidateRect.Call(uintptr(w.hwnd), 0, 0)
	}
}

// SetOnDpiChanged — колбэк смены DPI монитора (см. dpiChangeNotifier).
func (w *Win32Window) SetOnDpiChanged(fn func(scale float64)) { w.onDpiChanged = fn }

// SetOnActivate — колбэк смены активности окна (см. activationNotifier).
func (w *Win32Window) SetOnActivate(fn func(active bool)) { w.onActivate = fn }

// DetectScale включает per-monitor DPI awareness (v2) и возвращает
// системный масштаб (DPI/96). На Windows до 1703 или при ошибке — 1.0.
// Без awareness Windows растягивала бы наш кадр bitmap-скейлом (мыло).
func (w *Win32Window) DetectScale() float64 {
	// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 = -4 (псевдо-хендл).
	const dpiAwarenessPerMonitorV2 = ^uintptr(3) // == uintptr(-4)
	if procSetProcessDpiAwarenessContext.Find() == nil {
		procSetProcessDpiAwarenessContext.Call(dpiAwarenessPerMonitorV2)
	}
	if procGetDpiForSystem.Find() != nil {
		return 1
	}
	dpi, _, _ := procGetDpiForSystem.Call()
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96.0
}

// Callbacks
// DWM-атрибут предпочтения формы углов (Windows 11+).
const (
	dwmwaWindowCornerPreference = 33 // DWMWA_WINDOW_CORNER_PREFERENCE
	dwmwcpDoNotRound            = 1  // DWMWCP_DONOTROUND
	dwmwcpRound                 = 2  // DWMWCP_ROUND (мягкие углы + тень)
)

// setDwmCorner просит DWM скруглить/не скруглять углы (Win11). Возвращает true,
// если вызов успешен (S_OK) — значит ОС поддерживает атрибут и применила его.
// На Windows 10 атрибут не поддерживается → возвращает false (нужен фолбэк).
func (w *Win32Window) setDwmCorner(round bool) bool {
	if w.hwnd == 0 {
		return false
	}
	pref := int32(dwmwcpDoNotRound)
	if round {
		pref = dwmwcpRound
	}
	ret, _, _ := procDwmSetWindowAttribute.Call(
		uintptr(w.hwnd), uintptr(dwmwaWindowCornerPreference),
		uintptr(unsafe.Pointer(&pref)), unsafe.Sizeof(pref))
	return ret == 0 // S_OK
}

// SetCornerRadius задаёт радиус скругления углов окна (0 = прямые).
// На Windows 11 используется DWM (мягкие сглаженные углы + системная тень);
// на Windows 10 — фолбэк через регион окна (чёткие углы). Безопасно в рантайме.
func (w *Win32Window) SetCornerRadius(r int) {
	w.cornerRadius = r
	w.applyCorners()
}

// applyCorners применяет текущее скругление: сперва пробует DWM (Win11),
// иначе — round-rect регион (Win10). Переприменяется при resize.
func (w *Win32Window) applyCorners() {
	if w.hwnd == 0 {
		return
	}
	if w.cornerRadius <= 0 || w.maximized || w.fs.on.Load() {
		// Прямые углы (или развёрнутое окно): DWM → не скруглять, регион снять.
		w.setDwmCorner(false)
		procSetWindowRgn.Call(uintptr(w.hwnd), 0, 1)
		return
	}
	// Win11: мягкие углы средствами DWM — без жёсткого региона.
	if w.setDwmCorner(true) {
		procSetWindowRgn.Call(uintptr(w.hwnd), 0, 1) // снимаем возможный старый регион
		return
	}
	// Win10: DWM углы не скругляет, а скруглённый регион (SetWindowRgn), как
	// раньше, отнимал у окна системную тень: светлое окно сливалось с тем, что
	// позади (отчёт RDP-клиента на v3.39.0). Углы остаются прямыми, как у всех
	// окон Windows 10, — и корпус окна рисуется прямоугольным (RoundsCorners,
	// widget.Window.SquareCorners), — а тень включает рамка DWM в 1 px снизу:
	// у окна без неклиентской области DWM иначе её не рисует.
	m := dwmMargins{Bottom: 1}
	procDwmExtendFrameIntoClientArea.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&m)))
	procSetWindowRgn.Call(uintptr(w.hwnd), 0, 1)
}

// dwmMargins — MARGINS для DwmExtendFrameIntoClientArea.
type dwmMargins struct{ Left, Right, Top, Bottom int32 }

// RoundsCorners — скруглены ли сейчас углы этого окна средствами ОС
// (cornerRounder): только у окна со скруглением, не развёрнутого и не на весь
// экран, и только на Windows 11 (DWMWA_WINDOW_CORNER_PREFERENCE). На Windows 10
// и у окна с прямыми углами — нет: корпус тогда рисуется прямоугольным.
func (w *Win32Window) RoundsCorners() bool {
	if w.hwnd == 0 {
		return true
	}
	if w.cornerRadius <= 0 || w.maximized || w.fs.on.Load() {
		return false
	}
	return w.setDwmCorner(true)
}

func (w *Win32Window) SetOnResize(fn func(w, h int))                            { w.onResize = fn }
func (w *Win32Window) SetOnClose(fn func() bool)                                { w.onClose = fn }
func (w *Win32Window) SetOnMouseMove(fn func(x, y int))                         { w.onMouseMove = fn }
func (w *Win32Window) SetOnMouseButton(fn func(x, y, button int, pressed bool)) { w.onMouseButton = fn }

// SetOnMouseWheelPixels регистрирует колбэк точной пиксельной дельты колеса
// (высокоточные тачпады шлют WM_MOUSEWHEEL с delta, не кратной WHEEL_DELTA).
func (w *Win32Window) SetOnMouseWheelPixels(fn func(x, y int, dx, dy float64)) {
	w.onMouseWheelPixels = fn
}
func (w *Win32Window) SetOnKeyDown(fn func(vk int)) { w.onKeyDown = fn }

// SetOnKeyDownRepeat подписывает приёмник нажатий, отличающий автоповтор от
// нового нажатия. Реализует keyRepeatSource.
func (w *Win32Window) SetOnKeyDownRepeat(fn func(vk int, repeat bool)) {
	w.onKeyDownRepeat = fn
}
func (w *Win32Window) SetOnKeyUp(fn func(vk int)) { w.onKeyUp = fn }

// SetOnRawKey подписывает приёмник клавиш со скан-кодом. Реализует rawKeySource.
func (w *Win32Window) SetOnRawKey(fn func(k rawKey)) { w.onRawKey = fn }

// SetOnWheelRaw подписывает приёмник сырого колеса. Реализует wheelRawSource.
func (w *Win32Window) SetOnWheelRaw(fn func(x, y int, dx, dy float64, precise bool, legacy func())) {
	w.onWheelRaw = fn
}
func (w *Win32Window) SetOnChar(fn func(r rune)) { w.onChar = fn }

// SetOnFilesDropped регистрирует колбэк Drag&Drop файлов из ОС (WM_DROPFILES).
// Координаты — клиентские физические пиксели. Гарантирует включённый приём
// файлов (DragAcceptFiles), даже если окно уже создано.
func (w *Win32Window) SetOnFilesDropped(fn func(paths []string, x, y int)) {
	w.onFilesDropped = fn
	w.enableFileDrop()
}

// ─── WndProc ────────────────────────────────────────────────────────────────

// charFromUTF16 собирает руну из потока WM_CHAR.
//
// Windows шлёт текст в UTF-16: символ вне основной плоскости — эмодзи,
// редкие письменности — приходит ДВУМЯ сообщениями, старшим и младшим
// суррогатом. По одному они не значат ничего, и раньше в приложение уходила
// пара мусорных рун вместо одного символа.
//
// ok=false — руны пока нет: либо ждём вторую половину пары, либо пара не
// сложилась (непарный суррогат лучше отбросить, чем показать знак-замену).
func (w *Win32Window) charFromUTF16(u rune) (rune, bool) {
	switch {
	case utf16.IsSurrogate(u) && w.highSurrogate == 0:
		w.highSurrogate = u
		return 0, false
	case w.highSurrogate != 0:
		pair := utf16.DecodeRune(w.highSurrogate, u)
		w.highSurrogate = 0
		if pair == utf8.RuneError {
			return 0, false
		}
		return pair, true
	}
	return u, true
}

func wndProc(hwnd uintptr, umsg uint32, wparam, lparam uintptr) uintptr {
	w := lookupWin32(hwnd)
	if w == nil {
		// Сообщения, пришедшие до регистрации hwnd (внутри CreateWindowExW),
		// относятся к создаваемому окну.
		win32Mu.Lock()
		w = win32Creating
		win32Mu.Unlock()
	}
	if w == nil {
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
		return ret
	}

	switch umsg {
	case wmGetObject:
		// Запрос клиента доступности (скринридер, Инспектор UIA): отдаём
		// корневой провайдер UI Automation. Если мост не поднят, сообщение
		// уходит дальше в DefWindowProc (см. a11y_windows.go).
		if ret, ok := uiaHandleGetObject(hwnd, wparam, lparam); ok {
			return ret
		}

	case wmApp:
		// Маршалинг колбэка на UI-поток (см. InvokeOnUIThread).
		if fn := takeUICall(wparam); fn != nil {
			fn()
		}
		return 0

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
		if w.pendingClose {
			// Close() был вызван из кода (напр. из OnClose виджета) —
			// колбэк уже отработал, просто уничтожаем окно.
			procDestroyWindow.Call(hwnd)
			return 0
		}
		// Закрытие от ОС (Alt+F4, taskbar, etc.) — спрашиваем колбэк.
		if w.onClose != nil {
			if w.onClose() {
				procDestroyWindow.Call(hwnd)
			}
			return 0
		}
		procDestroyWindow.Call(hwnd)
		return 0

	case wmDestroy:
		kbGrab.release(w, false) // хук захвата клавиатуры не должен пережить окно
		w.revokeDropTarget()     // IDropTarget окна (dnd_windows.go)
		win32Mu.Lock()
		delete(win32Windows, hwnd)
		win32Mu.Unlock()
		dropUICalls(hwnd) // WM_APP этому окну уже не придёт
		w.destroyArtCursors()
		w.destroyImageCursors()
		w.lock.release(w) // не оставить курсор зажатым и сырой ввод — на мёртвое окно
		w.mu.Lock()
		w.frameDead = true
		w.freeFrameBuf() // память VirtualAlloc сборщик мусора не вернёт
		w.mu.Unlock()
		// Вторичные окна модалок (noQuit) не завершают цикл сообщений — иначе
		// закрытие диалога уронило бы всё приложение. Главное окно — завершает.
		if !w.noQuit {
			procPostQuitMessage.Call(0)
		}
		return 0

	case wmEntersizemove:
		// Пользователь схватил рамку — входим в модальный цикл ресайза.
		// Дальше WM_SIZE не трогает движок, а лишь растягивает кэш кадра.
		w.resizing.Store(true)
		break // → DefWindowProc (штатная механика modal loop)

	case wmExitsizemove:
		// Ресайз завершён — применяем реальный размер к движку ОДИН раз
		// (пересоздание канваса дорогое). Затем «мост» из растянутого кэша,
		// пока движок не отдаст чёткий кадр нового размера.
		w.resizing.Store(false)
		if w.onResize != nil && w.width > 0 && w.height > 0 {
			w.onResize(w.width, w.height)
		}
		w.blitCachedStretched()
		break // → DefWindowProc

	case wmSize:
		w.noteMinimized(wparam == sizeMinimized) // SIZE_MINIMIZED: размер клиента нулевой
		newW := int(lparam & 0xFFFF)
		newH := int((lparam >> 16) & 0xFFFF)
		if newW > 0 && newH > 0 {
			w.width = newW
			w.height = newH
			// wparam: 2 = SIZE_MAXIMIZED, 0 = SIZE_RESTORED. Полноэкранное
			// окно не меняет запомненное «было развёрнуто» (fullscreen_windows.go).
			if !w.fs.on.Load() {
				w.maximized = wparam == 2
			}
			w.applyCorners() // регион в координатах окна — пересоздаём под новый размер
			if w.resizing.Load() {
				// Интерактивный ресайз: немедленно закрашиваем НОВЫЙ размер
				// окна растянутым кэшем — не дожидаясь кадра от движка.
				// Так непокрытые полосы не мигают фоном. SetResolution
				// отложен до WM_EXITSIZEMOVE.
				w.blitCachedStretched()
			} else if w.onResize != nil {
				// Не-интерактивный путь (maximize/restore/программный SetSize):
				// сразу применяем размер к движку.
				w.onResize(newW, newH)
			}
			w.lock.refit(w) // зажим указателя — в центр нового размера
		}
		return 0

	case wmPaint:
		// BeginPaint валидирует область и сообщает повреждённый прямоугольник;
		// содержимое восстанавливается блитом из кэша кадра (см. window.go).
		var ps paintstruct
		procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if w.resizing.Load() {
			// Во время modal resize loop свежего кадра ещё нет — рисуем из кэша
			// растянуто на всю область (НИКОГДА не отдаём пустую отрисовку в Def).
			w.blitCachedStretched()
			return 0
		}
		if w.onExpose == nil {
			w.paintMiss.Store(true) // перерисовку подключат позже — см. SetOnExpose
		}
		if w.onExpose != nil {
			r := image.Rect(int(ps.RcPaint.Left), int(ps.RcPaint.Top),
				int(ps.RcPaint.Right), int(ps.RcPaint.Bottom))
			if !r.Empty() {
				w.onExpose(r)
			}
		}
		return 0

	case wmActivate:
		// LOWORD(wparam): WA_INACTIVE=0, WA_ACTIVE=1, WA_CLICKACTIVE=2.
		// lparam — hwnd окна, к которому уходит/от которого приходит фокус.
		// Деактивация в пользу ДРУГОГО НАШЕГО окна (окно-попап меню, окно
		// диалога) — не деактивация приложения: показ окна-попапа иначе сам
		// себя закрывал (носитель ловил false → CloseAllOverlays), а титлбар
		// носителя мигал «неактивным» при открытом меню — системные меню так
		// не делают.
		active := wparam&0xFFFF != 0
		if !active && lparam != 0 && lookupWin32(lparam) != nil {
			return 0
		}
		w.lock.onActivate(w, active) // захват указателя: снять при потере фокуса, вернуть при получении
		if w.onActivate != nil {
			w.onActivate(active)
		}
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
		return ret

	case wmInput:
		// Сырой ввод мыши (захват указателя). DefWindowProc обязателен: по
		// документации система освобождает данные сообщения в нём.
		w.lock.handleInput(lparam)
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
		return ret

	case wmMove:
		w.lock.refit(w) // зажим указателя следует за окном
		break           // → DefWindowProc

	case wmSettingchange:
		// Сообщение приходит по любому поводу; нужное нам различает строка
		// в lParam (см. systheme_windows.go). Дальше уходит в DefWindowProc —
		// как и положено широковещательному сообщению.
		w.handleSettingChange(lparam)
		if monitorsChangedBySetting(wparam) {
			w.noteMonitorsChanged() // SPI_SETWORKAREA: сдвинули панель задач
		}

	case wmDpichanged:
		w.noteMonitorsChanged() // масштаб монитора мог измениться
		// Окно перенесли на монитор с другим DPI. wparam: LOWORD = новый DPI;
		// lparam → RECT с рекомендованным размером/позицией окна.
		newDpi := wparam & 0xFFFF
		if w.fs.on.Load() {
			// Рекомендованный прямоугольник — для обычного окна; полноэкранное
			// встаёт на монитор, на который переехало.
			w.refitFullscreen()
		} else if r := (*rect)(unsafe.Pointer(lparam)); r != nil {
			procSetWindowPos.Call(hwnd, 0,
				uintptr(r.Left), uintptr(r.Top),
				uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
				0x0004|0x0010) // SWP_NOZORDER | SWP_NOACTIVATE
		}
		if w.onDpiChanged != nil && newDpi > 0 {
			w.onDpiChanged(float64(newDpi) / 96.0)
		}
		return 0

	case wmNchittest:
		if w.fs.on.Load() {
			// Полный экран: ни краёв ресайза, ни заголовка, за который можно
			// тащить окно (и привязывать его к краям экрана).
			return htClient
		}
		// Borderless-окно: рамки ОС нет, зоны resize отдаём вручную.
		// lParam — ЭКРАННЫЕ координаты курсора (signed 16-bit слова).
		sx := int(int16(lparam & 0xFFFF))
		sy := int(int16((lparam >> 16) & 0xFFFF))
		var wr rect
		procGetWindowRect.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&wr)))

		// Края важнее кнопок: полоса ресайза идёт поверх заголовка, иначе
		// верхний край окна с кнопками было бы не ухватить.
		edge := w.resizable.Load() && !w.maximized &&
			(sx-int(wr.Left) < ncResizeBorder || int(wr.Right)-sx <= ncResizeBorder ||
				sy-int(wr.Top) < ncResizeBorder || int(wr.Bottom)-sy <= ncResizeBorder)
		if !edge {
			// Что здесь — знает приложение: оно эти кнопки и нарисовало.
			if area, ok := w.hitAreaAt(sx-int(wr.Left), sy-int(wr.Top)); ok {
				return area
			}
		}
		if !w.resizable.Load() || w.maximized {
			break // → DefWindowProc (HTCLIENT и т.п.)
		}
		left := sx-int(wr.Left) < ncResizeBorder
		right := int(wr.Right)-sx <= ncResizeBorder
		top := sy-int(wr.Top) < ncResizeBorder
		bottom := int(wr.Bottom)-sy <= ncResizeBorder
		switch {
		case top && left:
			return htTopLeft
		case top && right:
			return htTopRight
		case bottom && left:
			return htBottomLeft
		case bottom && right:
			return htBottomRight
		case left:
			return htLeft
		case right:
			return htRight
		case top:
			return htTop
		case bottom:
			return htBottom
		}
		return htClient

	case wmNcmousemove, wmNclbuttondown, wmNclbuttonup:
		// Кнопку заголовка нарисовало приложение, и подсветку с нажатием
		// ведёт тоже оно: переводим событие в клиентские координаты и
		// отдаём как обычную мышь. Иначе кнопка «развернуть», объявленная
		// системе ради макетов привязки, перестала бы подсвечиваться и
		// нажиматься.
		if _, ours := ncButtonArea(wparam); !ours {
			break // прочая нерабочая область — системе
		}
		sx := int(int16(lparam & 0xFFFF))
		sy := int(int16((lparam >> 16) & 0xFFFF))
		pt := point{X: int32(sx), Y: int32(sy)}
		procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
		switch umsg {
		case wmNcmousemove:
			w.ncButtonMouseMove(hwnd, int(pt.X), int(pt.Y))
		case wmNclbuttondown:
			if w.onMouseButton != nil {
				w.onMouseButton(int(pt.X), int(pt.Y), 0, true)
			}
		case wmNclbuttonup:
			if w.onMouseButton != nil {
				w.onMouseButton(int(pt.X), int(pt.Y), 0, false)
			}
		}
		return 0

	case wmNcmouseleave:
		// Курсор ушёл с кнопки заголовка — снимаем подсветку, уведя мышь
		// за пределы окна.
		w.ncMouseLeft()
		return 0

	case wmMouseleave:
		// Указатель покинул клиентскую область (TrackMouseEvent, см.
		// clientMouseMove): тот же путь, что LeaveNotify на X11. Переход на
		// кнопку заголовка уходом не считается — mouseLeaveReal.
		w.handleMouseLeave(hwnd)
		return 0

	case wmDisplaychange:
		// Сменилось разрешение или состав мониторов: полноэкранное окно
		// пересчитывает прямоугольник (иначе остаётся с прежним размером).
		w.refitFullscreen()
		w.noteMonitorsChanged()

	case wmWindowposchanged:
		// Окно перенесли (Win+Shift+стрелка) — на другом мониторе тот же
		// прямоугольник ему не годится.
		if w.fs.on.Load() && !w.fs.refitting {
			w.refitFullscreen()
		}

	case wmGetminmaxinfo:
		// Значения минимума согласованы с fitMinTrack (см. createInternal).
		// Минимальный размер окна при пользовательском resize.
		// MINMAXINFO: 5×POINT; ptMinTrackSize — смещение 24 (int32-индексы 6,7).
		// Значения — в физических пикселях (SetMinSize уже учёл HiDPI-scale).
		if lparam != 0 {
			minW, minH := int32(320), int32(240)
			if w.minW > 0 {
				minW = int32(w.minW)
			}
			if w.minH > 0 {
				minH = int32(w.minH)
			}
			mm := (*[10]int32)(unsafe.Pointer(lparam))
			mm[6] = minW // ptMinTrackSize.x
			mm[7] = minH // ptMinTrackSize.y
			if w.fs.on.Load() {
				// Полный экран: потолок размера — сам монитор, даже если он
				// больше основного (по умолчанию потолок — основной экран).
				if mon := w.monitorRectOfWindow(); !mon.Empty() {
					mm[2], mm[3] = int32(mon.Min.X), int32(mon.Min.Y) // ptMaxPosition
					mm[4], mm[5] = int32(mon.Dx()), int32(mon.Dy())   // ptMaxSize
					mm[8], mm[9] = int32(mon.Dx()), int32(mon.Dy())   // ptMaxTrackSize
				}
			}
			return 0
		}

	case wmSetcursor:
		// Удерживаем выбранную форму курсора в клиентской области.
		// Младшее слово lParam = hit-test; HTCLIENT=1.
		if h, ok := w.setCursorMessage(lparam & 0xFFFF); ok {
			procSetCursor.Call(h) // h == 0 прячет курсор (CursorNone)
			return 1
		}
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
		return ret

	case wmErasebkgnd:
		return 1 // Не стираем фон (мы рисуем сами)

	case wmMousemove:
		x := int(int16(lparam & 0xFFFF))
		y := int(int16((lparam >> 16) & 0xFFFF))
		w.clientMouseMove(hwnd, x, y)
		return 0

	case wmXbuttondown, wmXbuttonup, wmXbuttondblclk:
		// Боковые кнопки «назад/вперёд» (XBUTTON1/2). Обработчик обязан
		// вернуть TRUE, иначе DefWindowProc отправит окну WM_APPCOMMAND.
		if w.handleXButton(umsg, wparam, lparam) {
			return 1
		}
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
		return ret

	case wmLbuttondown, wmLbuttondblclk:
		// Захватываем мышь — гарантируем, что WM_LBUTTONUP придёт к нам,
		// даже если окно будет уничтожено/скрыто во время обработки press.
		//
		// WM_LBUTTONDBLCLK обрабатывается идентично WM_LBUTTONDOWN:
		// Windows присылает dblclk вместо второго down при быстром повторном клике.
		// Если не обработать — движок не увидит press, pressConsumer останется nil,
		// и последующий WM_LBUTTONUP пролетит на виджет под закрытым окном.
		procSetCapture.Call(uintptr(hwnd))
		x := int(int16(lparam & 0xFFFF))
		y := int(int16((lparam >> 16) & 0xFFFF))
		if w.onMouseButton != nil {
			w.onMouseButton(x, y, 0, true)
		}
		return 0

	case wmLbuttonup:
		procReleaseCapture.Call()
		x := int(int16(lparam & 0xFFFF))
		y := int(int16((lparam >> 16) & 0xFFFF))
		if w.onMouseButton != nil {
			w.onMouseButton(x, y, 0, false)
		}
		return 0

	case wmRbuttondown, wmRbuttondblclk:
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

	case wmMbuttondown, wmMbuttondblclk:
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

	case wmMousehwheel:
		// Горизонтальное колесо: delta>0 — вправо (в отличие от
		// вертикального, где положительное значение означает «от себя»).
		pt := point{
			X: int32(int16(lparam & 0xFFFF)),
			Y: int32(int16((lparam >> 16) & 0xFFFF)),
		}
		procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
		delta := int16((wparam >> 16) & 0xFFFF)
		if delta != 0 {
			x, y := int(pt.X), int(pt.Y)
			legacy := func() {
				if w.onMouseWheelPixels != nil {
					dx := float64(delta) / wheelDeltaWin * wheelNotchPx
					w.onMouseWheelPixels(x, y, dx, 0)
				}
			}
			if w.onWheelRaw != nil {
				dx, dy, precise := win32WheelRaw(delta, true)
				w.onWheelRaw(x, y, dx, dy, precise, legacy)
			} else {
				legacy()
			}
		}
		return 0

	case wmMousewheel:
		// Для WM_MOUSEWHEEL координаты в lparam заданы в экранных координатах.
		// Конвертируем их в клиентские, чтобы hit-test виджетов был корректным.
		pt := point{
			X: int32(int16(lparam & 0xFFFF)),
			Y: int32(int16((lparam >> 16) & 0xFFFF)),
		}
		procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))

		delta := int16((wparam >> 16) & 0xFFFF)
		x, y := int(pt.X), int(pt.Y)
		legacy := func() {
			if w.onMouseWheelPixels != nil {
				// Высокоточный путь: delta кратна 120 (WHEEL_DELTA) для обычной мыши,
				// но у прецизионных тачпадов приходит дробной. Один «notch» (120) =
				// wheelNotchPx пикселей; знак: delta>0 = вверх ⇒ dy<0.
				if delta != 0 {
					dy := -float64(delta) / wheelDeltaWin * wheelNotchPx
					w.onMouseWheelPixels(x, y, 0, dy)
				}
			} else if w.onMouseButton != nil {
				// Фолбэк на тики (например, в режиме popup-хоста).
				if delta > 0 {
					w.onMouseButton(x, y, 3, true)
					w.onMouseButton(x, y, 3, false)
				} else if delta < 0 {
					w.onMouseButton(x, y, 4, true)
					w.onMouseButton(x, y, 4, false)
				}
			}
		}
		if w.onWheelRaw != nil && delta != 0 {
			dx, dy, precise := win32WheelRaw(delta, false)
			w.onWheelRaw(x, y, dx, dy, precise, legacy)
		} else {
			legacy()
		}
		return 0

	case wmPointerDown, wmPointerUpdate, wmPointerUp, wmPointerCaptureChanged:
		// Касание, взятое окном, в DefWindowProc не отдаём: иначе система
		// породит ещё и мышь (touch_windows.go). Перо и окно без касаний —
		// как раньше, через DefWindowProc ниже.
		if w.handlePointerMsg(hwnd, umsg, wparam) {
			return 0
		}

	case wmImeSetContext, wmImeStartComposition, wmImeComposition, wmImeEndComposition:
		if ret, done := w.handleIMEMessage(hwnd, umsg, wparam, lparam); done {
			return ret
		}
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
		return ret

	case wmKeydown, wmSyskeydown:
		// Нажатие Caps/Num/Scroll Lock, которое мы сами послали через
		// SendInput (SetLockKeys): приложению оно не нужно, оно его же и
		// вызвало (lockkeys_windows.go).
		if lkSyntheticKey(int(wparam)) {
			return 0
		}
		// Бит 30 lParam — «клавиша уже была нажата», то есть это автоповтор,
		// а не новое нажатие (WM_KEYDOWN, previous key state).
		scan, ext, repeat := win32KeyLParam(lparam)
		if w.onRawKey != nil {
			w.onRawKey(rawKey{VK: int(wparam), Scan: scan, Extended: ext, Pressed: true, Repeat: repeat})
		} else if w.onKeyDownRepeat != nil {
			w.onKeyDownRepeat(int(wparam), repeat)
		} else if w.onKeyDown != nil {
			w.onKeyDown(int(wparam))
		}
		// Системные сочетания отдаём системе: Alt+F4 закрывает окно, а
		// Alt+Space открывает оконное меню, и подменять их собой нельзя.
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
		return ret

	case wmKeyup, wmSyskeyup:
		if lkSyntheticKey(int(wparam)) {
			return 0
		}
		if w.onRawKey != nil {
			scan, ext, _ := win32KeyLParam(lparam)
			w.onRawKey(rawKey{VK: int(wparam), Scan: scan, Extended: ext})
		} else if w.onKeyUp != nil {
			w.onKeyUp(int(wparam))
		}
		if umsg == wmSyskeyup {
			// То же, что и с нажатием: системные сочетания доигрывает ОС.
			ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
			return ret
		}
		return 0

	case wmSyscommand:
		// Вход в меню окна по F10 или одиночному Alt забирал бы клавиатуру у
		// приложения, которое само рисует строку меню (syskeymenu.go).
		if swallowKeyboardMenu(wparam, lparam) {
			return 0
		}
		ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
		return ret

	case wmSyschar:
		// Символ, набранный с зажатым Alt. По умолчанию система отвечает на
		// него звуком «такого пункта меню нет» — своей строки меню у окна
		// нет, а приложение сочетание уже получило отдельным событием.
		// Alt+Space (0x20) оставляем системе: это оконное меню.
		if rune(wparam) == ' ' {
			ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
			return ret
		}
		return 0

	case wmChar:
		if r, ok := w.charFromUTF16(rune(wparam)); ok && r >= 32 && w.onChar != nil {
			w.onChar(r)
		}
		return 0

	case wmDropfiles:
		// wParam — HDROP: список путей + точка сброса. Раскрываем и завершаем
		// (DragFinish обязателен — иначе ОС не освободит структуру).
		w.handleDropFiles(wparam)
		return 0

	case wmTrayCallback:
		// Событие иконки трея (см. NOTIFYICONDATA.uCallbackMessage).
		w.handleTrayCallback(lparam)
		return 0

	case wmPrintClient, wmPrint:
		// Превью окна (таскбар/Aero Peek): блитим кэш кадра в переданный HDC
		// (wParam), иначе PrintWindow/DWM показывают чёрное.
		w.handlePrintClient(wparam)
		return 0

	case wmDwmSendIconicThumbnail:
		// Iconic-миниатюра из кэша кадра (только при HEADLESS_GUI_ICONIC_PREVIEW=1).
		if w.handleIconicThumbnail(lparam) {
			return 0
		}

	case wmDwmSendIconicLivePreviewBitmap:
		if w.handleIconicLivePreview() {
			return 0
		}
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(umsg), wparam, lparam)
	return ret
}

// ─── localeProvider: раскладка клавиатуры ОС ────────────────────────────────

// CurrentLocaleCode возвращает код активной раскладки потока окна ("EN","RU",…).
// Работает из любого потока: берём раскладку через thread id окна, поэтому
// переключение системной комбинацией клавиш корректно отражается.
func (w *Win32Window) CurrentLocaleCode() string {
	if w.hwnd == 0 {
		return ""
	}
	tid, _, _ := procGetWindowThreadProcessId.Call(uintptr(w.hwnd), 0)
	hkl, _, _ := procGetKeyboardLayout.Call(tid)
	return langidCode(uint32(hkl) & 0xFFFF)
}

// AvailableLocaleCodes возвращает установленные в системе раскладки.
func (w *Win32Window) AvailableLocaleCodes() []string {
	list := w.keyboardLayoutList()
	var out []string
	seen := map[string]bool{}
	for _, hkl := range list {
		c := langidCode(uint32(hkl) & 0xFFFF)
		if c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// ActivateLocaleCode переключает раскладку окна на соответствующую коду.
// Используем PostMessage(WM_INPUTLANGCHANGEREQUEST) — корректно работает
// межпоточно (поток окна сам переключит язык ввода).
func (w *Win32Window) ActivateLocaleCode(code string) bool {
	if w.hwnd == 0 {
		return false
	}
	want := strings.ToUpper(strings.TrimSpace(code))
	for _, hkl := range w.keyboardLayoutList() {
		if langidCode(uint32(hkl)&0xFFFF) == want {
			procPostMessageW.Call(uintptr(w.hwnd), uintptr(wmInputlangchangerequest), 0, hkl)
			return true
		}
	}
	return false
}

// keyboardLayoutList возвращает список HKL установленных раскладок.
func (w *Win32Window) keyboardLayoutList() []uintptr {
	n, _, _ := procGetKeyboardLayoutList.Call(0, 0)
	if n == 0 {
		return nil
	}
	list := make([]uintptr, int(n))
	procGetKeyboardLayoutList.Call(n, uintptr(unsafe.Pointer(&list[0])))
	return list
}

// SetCursor задаёт форму курсора по коду widget.Cursor (0=Arrow,1=IBeam,
// 2=Hand,3=SizeWE,4=SizeNS,5=SizeNWSE,6=SizeNESW,7=None,8=Grab,9=Grabbing,
// 10=SizeAll,11=Crosshair,12=Wait; см. win32CursorSpecFor). Реальное
// применение — в WndProc (WM_SETCURSOR).
func (w *Win32Window) SetCursor(c int) {
	h, hidden := w.cursorFor(c)
	if w.cursorHandle == h && w.cursorHidden == hidden {
		return
	}
	w.cursorHandle, w.cursorHidden = h, hidden
	// Применяем сразу, не дожидаясь движения мыши (и далее держим через
	// WM_SETCURSOR) — но только если под указателем наше окно: курсор
	// ставится глобально.
	if w.pointerOverWindow() {
		procSetCursor.Call(h) // h == 0 прячет курсор (CursorNone)
	}
}

// handleDropFiles раскрывает HDROP из WM_DROPFILES: перечисляет пути
// (DragQueryFileW), берёт клиентскую точку сброса (DragQueryPoint) и
// освобождает структуру (DragFinish). Пути — UTF-16 → Go string.
func (w *Win32Window) handleDropFiles(hDrop uintptr) {
	// Гарантируем DragFinish даже при раннем выходе.
	defer procDragFinish.Call(hDrop)

	// Количество файлов: индекс 0xFFFFFFFF.
	n, _, _ := procDragQueryFileW.Call(hDrop, 0xFFFFFFFF, 0, 0)
	count := int(n)
	if count <= 0 {
		return
	}

	paths := make([]string, 0, count)
	for i := 0; i < count; i++ {
		// Требуемая длина (без завершающего NUL) — вызов с нулевым буфером.
		l, _, _ := procDragQueryFileW.Call(hDrop, uintptr(i), 0, 0)
		if l == 0 {
			continue
		}
		buf := make([]uint16, int(l)+1)
		procDragQueryFileW.Call(hDrop, uintptr(i),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		paths = append(paths, windows.UTF16ToString(buf))
	}

	// Точка сброса в клиентских координатах (физические пиксели).
	var pt point
	procDragQueryPoint.Call(hDrop, uintptr(unsafe.Pointer(&pt)))

	if w.onFilesDropped != nil && len(paths) > 0 {
		w.onFilesDropped(paths, int(pt.X), int(pt.Y))
	}
}

// getSystemMetrics вызывает GetSystemMetrics.
func getSystemMetrics(index int) int {
	proc := user32.NewProc("GetSystemMetrics")
	ret, _, _ := proc.Call(uintptr(index))
	return int(ret)
}
