//go:build linux && !android

package window

import (
	"encoding/binary"
	"fmt"
	"image"
	"net"
	"os"
	"sync"
	"unsafe"
)

// X11Window — реализация NativeWindow через X11 протокол.
// Чистый Go без CGO и внешних зависимостей.
// Общается с X-сервером напрямую через Unix socket.
type X11Window struct {
	conn      net.Conn
	screen    x11Screen
	rootWin   uint32
	wid       uint32 // window ID
	gcID      uint32 // graphics context ID
	width     int
	height    int
	title     string
	maximized bool
	closed    bool

	seqNum    uint16 // sequence number for requests
	mu        sync.Mutex

	// blitBuf — переиспользуемый буфер BGRA-конвертации для PutImage
	// (раньше выделялся полный кадр на каждый блит). blitMu сериализует
	// блиты из framePump и из Expose-обработчика (event loop).
	blitBuf []byte
	blitMu  sync.Mutex

	// Раскладка клавиатуры: GetKeyboardMapping (core-протокол).
	// keysyms — матрица keycode × symsPerCode; колонки 0-1 — группа 1,
	// 2-3 — группа 2 (группа берётся из бит 13-14 поля state события).
	minKeycode  byte
	symsPerCode int
	keysyms     []uint32

	// Диапазон клиентских ID из setup: каждый созданный объект обязан
	// иметь id = ridBase | (n & ridMask), иначе сервер отвечает BadIDChoice.
	ridBase, ridMask uint32
	ridNext          uint32

	// Callbacks
	onResize      func(w, h int)
	onExpose      func(r image.Rectangle)
	onActivate    func(active bool)
	onClose       func() bool
	onMouseMove   func(x, y int)
	onMouseButton func(x, y, button int, pressed bool)
	onKeyDown     func(vk int)
	onKeyUp       func(vk int)
	onChar        func(r rune)

	// Atom IDs для WM протоколов
	atomWMProtocols   uint32
	atomWMDeleteWindow uint32
	atomWMState        uint32
	atomWMStateMaxH    uint32
	atomWMStateMaxV    uint32
	atomNetWMState     uint32
}

type x11Screen struct {
	Root          uint32
	Colormap      uint32
	WhitePixel    uint32
	BlackPixel    uint32
	WidthInPixels uint16
	HeightInPixels uint16
	RootDepth     uint8
	RootVisual    uint32
}

// NewNativeWindow выбирает бэкенд: Wayland при доступном композиторе
// (WAYLAND_DISPLAY + живой сокет), иначе X11 (в т.ч. XWayland).
// HEADLESS_GUI_X11=1 — принудительный X11.
func NewNativeWindow() NativeWindow {
	if os.Getenv("HEADLESS_GUI_X11") != "1" {
		if ww := newWaylandWindow(); ww != nil {
			return ww
		}
	}
	return &X11Window{}
}

func (w *X11Window) Create(title string, width, height int) error {
	w.title = title
	w.width = width
	w.height = height

	// Подключаемся к X-серверу
	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0"
	}

	// Парсим DISPLAY — обычно :0 → /tmp/.X11-unix/X0
	var sockPath string
	if len(display) > 0 && display[0] == ':' {
		num := display[1:]
		// Убираем .screen если есть
		for i, c := range num {
			if c == '.' {
				num = num[:i]
				break
			}
		}
		sockPath = "/tmp/.X11-unix/X" + num
	} else {
		return fmt.Errorf("x11: unsupported DISPLAY format: %s", display)
	}

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return fmt.Errorf("x11: connect to %s: %w", sockPath, err)
	}
	w.conn = conn

	// X11 connection setup
	if err := w.x11Setup(); err != nil {
		conn.Close()
		return fmt.Errorf("x11: setup: %w", err)
	}

	// Создаём окно
	w.wid = w.x11GenID()
	w.gcID = w.x11GenID()

	// Раскладка клавиатуры (до появления событий — reply читается напрямую).
	w.x11LoadKeyboardMapping()

	// Intern atoms для WM-протоколов
	w.atomWMProtocols = w.x11InternAtom("WM_PROTOCOLS")
	w.atomWMDeleteWindow = w.x11InternAtom("WM_DELETE_WINDOW")
	w.atomNetWMState = w.x11InternAtom("_NET_WM_STATE")
	w.atomWMStateMaxH = w.x11InternAtom("_NET_WM_STATE_MAXIMIZED_HORZ")
	w.atomWMStateMaxV = w.x11InternAtom("_NET_WM_STATE_MAXIMIZED_VERT")

	// CreateWindow request
	x := (int(w.screen.WidthInPixels) - width) / 2
	y := (int(w.screen.HeightInPixels) - height) / 2

	eventMask := uint32(
		0x00000001 | // KeyPress
			0x00000002 | // KeyRelease
			0x00000004 | // ButtonPress
			0x00000008 | // ButtonRelease
			0x00000040 | // PointerMotion
			0x00008000 | // ExposureMask
			0x00020000 | // StructureNotifyMask (resize/close)
			0x00400000) // FocusChangeMask

	values := []uint32{
		w.screen.BlackPixel,                    // background
		eventMask,                               // event-mask
	}
	valueMask := uint32(0x00000002 | 0x00000800) // BackPixel | EventMask

	w.x11CreateWindow(w.wid, w.screen.Root, int16(x), int16(y),
		uint16(width), uint16(height), 0, valueMask, values)

	// Graphics Context
	w.x11CreateGC(w.gcID, w.wid)

	// WM_PROTOCOLS — чтобы получать WM_DELETE_WINDOW
	w.x11ChangeProperty(w.wid, w.atomWMProtocols, 4 /*ATOM*/, 32,
		uint32ToBytes(w.atomWMDeleteWindow))

	// Borderless — убираем WM decorations через Motif hints
	motifHints := w.x11InternAtom("_MOTIF_WM_HINTS")
	// flags=2 (decorations), decorations=0
	hints := make([]byte, 20)
	binary.LittleEndian.PutUint32(hints[0:4], 2)    // flags = MWM_HINTS_DECORATIONS
	binary.LittleEndian.PutUint32(hints[8:12], 0)   // decorations = 0
	w.x11ChangeProperty(w.wid, motifHints, motifHints, 32, hints)

	// Window title
	w.x11SetTitle(w.wid, title)

	// Map (show) window
	w.x11MapWindow(w.wid)

	return nil
}

func (w *X11Window) RunEventLoop() error {
	buf := make([]byte, 32)
	for !w.closed {
		_, err := w.conn.Read(buf)
		if err != nil {
			if w.closed {
				return nil
			}
			return fmt.Errorf("x11: read event: %w", err)
		}

		evType := buf[0] & 0x7F
		switch evType {
		case 2: // KeyPress
			keycode := buf[1]
			state := binary.LittleEndian.Uint16(buf[28:30])
			vk := x11KeycodeToVK(int(keycode))
			if w.onKeyDown != nil && vk != 0 {
				w.onKeyDown(vk)
			}
			// Символьный ввод: полноценный маппинг по GetKeyboardMapping
			// (раскладка/Shift/Caps/группа); фолбэк — упрощённая таблица.
			if w.onChar != nil {
				if r := w.x11RuneForKey(keycode, state); r >= 32 {
					w.onChar(r)
				} else if w.keysyms == nil {
					if r := x11KeycodeToRune(int(keycode), state&1 != 0); r != 0 {
						w.onChar(r)
					}
				}
			}

		case 3: // KeyRelease
			keycode := buf[1]
			vk := x11KeycodeToVK(int(keycode))
			if w.onKeyUp != nil && vk != 0 {
				w.onKeyUp(vk)
			}

		case 4: // ButtonPress
			x := int(int16(binary.LittleEndian.Uint16(buf[24:26])))
			y := int(int16(binary.LittleEndian.Uint16(buf[26:28])))
			button := int(buf[1]) - 1 // X11: 1=left, 2=mid, 3=right → 0,1,2
			if button == 2 {
				button = 1 // right
			} else if button == 1 {
				button = 2 // middle
			}
			if w.onMouseButton != nil {
				w.onMouseButton(x, y, button, true)
			}

		case 5: // ButtonRelease
			x := int(int16(binary.LittleEndian.Uint16(buf[24:26])))
			y := int(int16(binary.LittleEndian.Uint16(buf[26:28])))
			button := int(buf[1]) - 1
			if button == 2 {
				button = 1
			} else if button == 1 {
				button = 2
			}
			if w.onMouseButton != nil {
				w.onMouseButton(x, y, button, false)
			}

		case 6: // MotionNotify
			x := int(int16(binary.LittleEndian.Uint16(buf[24:26])))
			y := int(int16(binary.LittleEndian.Uint16(buf[26:28])))
			if w.onMouseMove != nil {
				w.onMouseMove(x, y)
			}

		case 9: // FocusIn
			if w.onActivate != nil {
				w.onActivate(true)
			}

		case 10: // FocusOut
			if w.onActivate != nil {
				w.onActivate(false)
			}

		case 12: // Expose
			// ОС просит восстановить область окна — блитим из кэша кадра.
			// Формат события: x@8, y@10, width@12, height@14 (little-endian).
			if w.onExpose != nil {
				ex := int(binary.LittleEndian.Uint16(buf[8:10]))
				ey := int(binary.LittleEndian.Uint16(buf[10:12]))
				ew := int(binary.LittleEndian.Uint16(buf[12:14]))
				eh := int(binary.LittleEndian.Uint16(buf[14:16]))
				if ew > 0 && eh > 0 {
					w.onExpose(image.Rect(ex, ey, ex+ew, ey+eh))
				}
			}

		case 22: // ConfigureNotify
			newW := int(binary.LittleEndian.Uint16(buf[20:22]))
			newH := int(binary.LittleEndian.Uint16(buf[22:24]))
			if newW != w.width || newH != w.height {
				w.width = newW
				w.height = newH
				if w.onResize != nil {
					w.onResize(newW, newH)
				}
			}

		case 33: // ClientMessage (WM_DELETE_WINDOW)
			atom := binary.LittleEndian.Uint32(buf[8:12])
			if atom == w.atomWMDeleteWindow {
				if w.onClose != nil {
					if w.onClose() {
						w.closed = true
						return nil
					}
				} else {
					w.closed = true
					return nil
				}
			}
		}
	}
	return nil
}

func (w *X11Window) Close() {
	w.closed = true
	if w.conn != nil {
		w.conn.Close()
	}
}

func (w *X11Window) SetTitle(title string) {
	w.title = title
	if w.wid != 0 {
		w.x11SetTitle(w.wid, title)
	}
}

func (w *X11Window) SetSize(width, height int) {
	w.width = width
	w.height = height
	if w.wid != 0 {
		w.x11ConfigureWindow(w.wid, width, height)
	}
}

func (w *X11Window) GetSize() (int, int) {
	return w.width, w.height
}

func (w *X11Window) SetPosition(x, y int) {
	if w.wid != 0 {
		w.x11MoveWindow(w.wid, x, y)
	}
}

func (w *X11Window) GetPosition() (int, int) {
	// Упрощённо — возвращаем 0,0 (полная реализация требует GetGeometry)
	return 0, 0
}

func (w *X11Window) Minimize() {
	// XIconifyWindow — отправляем ClientMessage с UnmapNotify
	if w.wid != 0 {
		// Используем WM_CHANGE_STATE protocol
		w.x11IconifyWindow()
	}
}

func (w *X11Window) Maximize() {
	if w.wid != 0 {
		w.x11ToggleMaximize(true)
		w.maximized = true
	}
}

func (w *X11Window) Restore() {
	if w.wid != 0 {
		w.x11ToggleMaximize(false)
		w.maximized = false
	}
}

func (w *X11Window) IsMaximized() bool {
	return w.maximized
}

// SetCornerRadius — скругление окна на X11 (пока no-op).
func (w *X11Window) SetCornerRadius(int) {}

func (w *X11Window) BlitRGBA(img *image.RGBA) {
	if img == nil {
		return
	}
	w.BlitRGBADirty(img, img.Bounds())
}

// BlitRGBADirty выводит только изменившуюся область dirty: BGRA-конвертация
// и PutImage ограничены этой областью. Буфер конвертации переиспользуется.
func (w *X11Window) BlitRGBADirty(img *image.RGBA, dirty image.Rectangle) {
	if w.wid == 0 || img == nil {
		return
	}
	dirty = dirty.Intersect(img.Bounds())
	if dirty.Empty() {
		return
	}
	dw := dirty.Dx()
	dh := dirty.Dy()

	w.blitMu.Lock()
	defer w.blitMu.Unlock()

	// X11 PutImage: формат ZPixmap, depth=24, BGRA order.
	// Буфер содержит только dirty-область (строки шириной dw).
	pixLen := dw * dh * 4
	if cap(w.blitBuf) < pixLen {
		w.blitBuf = make([]byte, pixLen)
	}
	data := w.blitBuf[:pixLen]

	src := img.Pix
	stride := img.Stride
	for y := 0; y < dh; y++ {
		srcOff := (dirty.Min.Y+y)*stride + dirty.Min.X*4
		dstOff := y * dw * 4
		for x := 0; x < dw; x++ {
			si := srcOff + x*4
			di := dstOff + x*4
			data[di+0] = src[si+2] // B
			data[di+1] = src[si+1] // G
			data[di+2] = src[si+0] // R
			data[di+3] = src[si+3] // A
		}
	}

	w.x11PutImage(w.wid, w.gcID, dirty.Min.X, dirty.Min.Y, dw, dh, data)
}

// Callbacks
func (w *X11Window) SetOnResize(fn func(w, h int))                              { w.onResize = fn }

// SetOnExpose — колбэк перерисовки области по Expose (см. exposeNotifier).
func (w *X11Window) SetOnExpose(fn func(r image.Rectangle)) { w.onExpose = fn }

// SetOnActivate — колбэк смены активности окна (FocusIn/FocusOut).
func (w *X11Window) SetOnActivate(fn func(active bool)) { w.onActivate = fn }
func (w *X11Window) SetOnClose(fn func() bool)                                   { w.onClose = fn }
func (w *X11Window) SetOnMouseMove(fn func(x, y int))                            { w.onMouseMove = fn }
func (w *X11Window) SetOnMouseButton(fn func(x, y, button int, pressed bool))    { w.onMouseButton = fn }
func (w *X11Window) SetOnKeyDown(fn func(vk int))                                { w.onKeyDown = fn }
func (w *X11Window) SetOnKeyUp(fn func(vk int))                                  { w.onKeyUp = fn }
func (w *X11Window) SetOnChar(fn func(r rune))                                   { w.onChar = fn }

// ─── X11 протокол (низкоуровневые запросы) ──────────────────────────────────

func (w *X11Window) x11Send(data []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.conn.Write(data)
	w.seqNum++
}

func (w *X11Window) x11Setup() error {
	// Connection setup: byte-order=0x6C (little-endian), protocol 11.0
	setup := []byte{0x6C, 0, 11, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	w.conn.Write(setup)

	// Читаем ответ
	hdr := make([]byte, 8)
	if _, err := w.conn.Read(hdr); err != nil {
		return err
	}

	if hdr[0] == 0 { // Failed
		return fmt.Errorf("x11 connection refused")
	}

	// Успех или authenticate — читаем дополнительные данные
	addLen := int(binary.LittleEndian.Uint16(hdr[6:8])) * 4
	addData := make([]byte, addLen)
	total := 0
	for total < addLen {
		n, err := w.conn.Read(addData[total:])
		if err != nil {
			return err
		}
		total += n
	}

	// resource-id-base/mask — база для клиентских ID (см. x11GenID).
	w.ridBase = binary.LittleEndian.Uint32(addData[4:8])
	w.ridMask = binary.LittleEndian.Uint32(addData[8:12])

	// min-keycode — байт 26 (для GetKeyboardMapping).
	if len(addData) > 27 {
		w.minKeycode = addData[26]
	}

	// Находим информацию об экране (после vendor и pixmap formats)
	vendorLen := int(binary.LittleEndian.Uint16(addData[16:18]))
	vendorPad := (4 - vendorLen%4) % 4
	numFormats := addData[21]

	screenOff := 32 + vendorLen + vendorPad + int(numFormats)*8
	if screenOff+40 > len(addData) {
		return fmt.Errorf("x11: setup data too short")
	}

	sd := addData[screenOff:]
	w.screen.Root = binary.LittleEndian.Uint32(sd[0:4])
	w.screen.Colormap = binary.LittleEndian.Uint32(sd[4:8])
	w.screen.WhitePixel = binary.LittleEndian.Uint32(sd[8:12])
	w.screen.BlackPixel = binary.LittleEndian.Uint32(sd[12:16])
	// sd[16:20] = current input masks
	w.screen.WidthInPixels = binary.LittleEndian.Uint16(sd[20:22])
	w.screen.HeightInPixels = binary.LittleEndian.Uint16(sd[22:24])
	w.screen.RootDepth = sd[38]
	w.screen.RootVisual = binary.LittleEndian.Uint32(sd[32:36])

	w.rootWin = w.screen.Root
	return nil
}

// x11GenID выделяет клиентский ID ресурса: обязательно в диапазоне
// resource-id-base|mask из setup — произвольные значения сервер отвергает
// (BadIDChoice). Из-за этого бага (счётчик с единицы) CreateWindow не
// работал до первой живой проверки бэкенда.
func (w *X11Window) x11GenID() uint32 {
	w.ridNext++
	return w.ridBase | (w.ridNext & w.ridMask)
}

func (w *X11Window) x11CreateWindow(wid, parent uint32, x, y int16, width, height uint16, borderWidth uint16, valueMask uint32, values []uint32) {
	// Opcode 1: CreateWindow.
	// Фиксированная часть тела (после 4-байтового заголовка) — 28 байт:
	// wid(4) parent(4) x(2) y(2) w(2) h(2) border(2) class(2) visual(4) mask(4).
	bodyLen := 28 + len(values)*4
	buf := make([]byte, 4+bodyLen)
	buf[0] = 1                                                    // opcode
	buf[1] = w.screen.RootDepth                                   // depth
	binary.LittleEndian.PutUint16(buf[2:4], uint16((4+bodyLen)/4)) // length in 4-byte units
	binary.LittleEndian.PutUint32(buf[4:8], wid)
	binary.LittleEndian.PutUint32(buf[8:12], parent)
	binary.LittleEndian.PutUint16(buf[12:14], uint16(x))
	binary.LittleEndian.PutUint16(buf[14:16], uint16(y))
	binary.LittleEndian.PutUint16(buf[16:18], width)
	binary.LittleEndian.PutUint16(buf[18:20], height)
	binary.LittleEndian.PutUint16(buf[20:22], borderWidth)
	binary.LittleEndian.PutUint16(buf[22:24], 1)                      // class = InputOutput
	binary.LittleEndian.PutUint32(buf[24:28], w.screen.RootVisual)    // visual
	binary.LittleEndian.PutUint32(buf[28:32], valueMask)
	for i, v := range values {
		binary.LittleEndian.PutUint32(buf[32+i*4:36+i*4], v)
	}
	w.x11Send(buf)
}

func (w *X11Window) x11CreateGC(gcid, drawable uint32) {
	buf := make([]byte, 16)
	buf[0] = 55 // CreateGC
	binary.LittleEndian.PutUint16(buf[2:4], 4) // length
	binary.LittleEndian.PutUint32(buf[4:8], gcid)
	binary.LittleEndian.PutUint32(buf[8:12], drawable)
	binary.LittleEndian.PutUint32(buf[12:16], 0) // value-mask = 0
	w.x11Send(buf)
}

func (w *X11Window) x11MapWindow(wid uint32) {
	buf := make([]byte, 8)
	buf[0] = 8 // MapWindow
	binary.LittleEndian.PutUint16(buf[2:4], 2)
	binary.LittleEndian.PutUint32(buf[4:8], wid)
	w.x11Send(buf)
}

func (w *X11Window) x11SetTitle(wid uint32, title string) {
	// ChangeProperty: WM_NAME
	data := []byte(title)
	w.x11ChangeProperty(wid, 39 /*WM_NAME*/, 31 /*STRING*/, 8, data)
}

func (w *X11Window) x11ChangeProperty(wid, property, propType uint32, format int, data []byte) {
	dataLen := len(data)
	pad := (4 - dataLen%4) % 4
	reqLen := 6 + (dataLen+pad)/4

	buf := make([]byte, reqLen*4)
	buf[0] = 18 // ChangeProperty
	buf[1] = 0  // mode = Replace
	binary.LittleEndian.PutUint16(buf[2:4], uint16(reqLen))
	binary.LittleEndian.PutUint32(buf[4:8], wid)
	binary.LittleEndian.PutUint32(buf[8:12], property)
	binary.LittleEndian.PutUint32(buf[12:16], propType)
	buf[16] = byte(format)
	nElements := dataLen
	if format == 32 {
		nElements = dataLen / 4
	} else if format == 16 {
		nElements = dataLen / 2
	}
	binary.LittleEndian.PutUint32(buf[20:24], uint32(nElements))
	copy(buf[24:], data)
	w.x11Send(buf)
}

// x11LoadKeyboardMapping запрашивает таблицу keysym'ов (GetKeyboardMapping).
// Вызывается при инициализации, до подписки на события — ответ читается
// последовательно, как в x11InternAtom. Ошибки не фатальны: без таблицы
// ввод символов деградирует до упрощённого маппинга.
func (w *X11Window) x11LoadKeyboardMapping() {
	first := w.minKeycode
	if first == 0 {
		first = 8
	}
	count := 255 - int(first) + 1

	req := make([]byte, 8)
	req[0] = 101 // GetKeyboardMapping
	binary.LittleEndian.PutUint16(req[2:4], 2)
	req[4] = first
	req[5] = byte(count)
	w.mu.Lock()
	w.conn.Write(req)
	w.seqNum++
	w.mu.Unlock()

	// Ответ: 32-байтовый заголовок + length*4 байт keysym'ов.
	hdr := make([]byte, 32)
	if !w.readFull(hdr) || hdr[0] != 1 {
		return
	}
	symsPer := int(hdr[1])
	length := int(binary.LittleEndian.Uint32(hdr[4:8])) * 4
	data := make([]byte, length)
	if !w.readFull(data) || symsPer == 0 {
		return
	}
	syms := make([]uint32, length/4)
	for i := range syms {
		syms[i] = binary.LittleEndian.Uint32(data[i*4 : i*4+4])
	}
	w.minKeycode = first
	w.symsPerCode = symsPer
	w.keysyms = syms
}

// readFull дочитывает buf целиком (false — ошибка соединения).
func (w *X11Window) readFull(buf []byte) bool {
	got := 0
	for got < len(buf) {
		n, err := w.conn.Read(buf[got:])
		if err != nil {
			return false
		}
		got += n
	}
	return true
}

// x11RuneForKey возвращает руну для keycode с учётом state события
// (бит 0 — Shift, бит 1 — CapsLock, биты 13-14 — группа/раскладка XKB).
// 0 — непечатаемая клавиша или таблица не загружена.
func (w *X11Window) x11RuneForKey(keycode byte, state uint16) rune {
	if w.keysyms == nil || keycode < w.minKeycode {
		return 0
	}
	row := int(keycode-w.minKeycode) * w.symsPerCode
	if row+w.symsPerCode > len(w.keysyms) {
		return 0
	}
	shift := state&1 != 0
	caps := state&2 != 0
	group := int(state>>13) & 3

	col := group * 2
	if col+1 >= w.symsPerCode { // группа за пределами таблицы — первая
		col = 0
	}
	level := 0
	if shift {
		level = 1
	}
	sym := w.keysyms[row+col+level]
	if sym == 0 { // NoSymbol на уровне Shift — базовый symbol
		sym = w.keysyms[row+col]
	}
	r := keysymToRune(sym)
	// CapsLock: инверсия регистра для букв.
	if caps && isLetterRune(r) {
		alt := w.keysyms[row+col]
		if level == 0 {
			alt = w.keysyms[row+col+1]
		}
		if ar := keysymToRune(alt); isLetterRune(ar) {
			r = ar
		}
	}
	return r
}

func (w *X11Window) x11InternAtom(name string) uint32 {
	nameBytes := []byte(name)
	nameLen := len(nameBytes)
	pad := (4 - nameLen%4) % 4
	reqLen := 2 + (nameLen+pad)/4

	buf := make([]byte, reqLen*4)
	buf[0] = 16 // InternAtom
	buf[1] = 0  // only_if_exists = false
	binary.LittleEndian.PutUint16(buf[2:4], uint16(reqLen))
	binary.LittleEndian.PutUint16(buf[4:6], uint16(nameLen))
	copy(buf[8:], nameBytes)

	w.mu.Lock()
	w.conn.Write(buf)
	w.seqNum++
	w.mu.Unlock()

	// Читаем ответ (32 байта)
	reply := make([]byte, 32)
	w.conn.Read(reply)
	return binary.LittleEndian.Uint32(reply[8:12])
}

func (w *X11Window) x11ConfigureWindow(wid uint32, width, height int) {
	buf := make([]byte, 20)
	buf[0] = 12 // ConfigureWindow
	binary.LittleEndian.PutUint16(buf[2:4], 5)    // length
	binary.LittleEndian.PutUint32(buf[4:8], wid)
	binary.LittleEndian.PutUint16(buf[8:10], 0x0C) // value-mask: width|height
	binary.LittleEndian.PutUint32(buf[12:16], uint32(width))
	binary.LittleEndian.PutUint32(buf[16:20], uint32(height))
	w.x11Send(buf)
}

func (w *X11Window) x11MoveWindow(wid uint32, x, y int) {
	buf := make([]byte, 20)
	buf[0] = 12 // ConfigureWindow
	binary.LittleEndian.PutUint16(buf[2:4], 5)
	binary.LittleEndian.PutUint32(buf[4:8], wid)
	binary.LittleEndian.PutUint16(buf[8:10], 0x03) // value-mask: x|y
	binary.LittleEndian.PutUint32(buf[12:16], uint32(x))
	binary.LittleEndian.PutUint32(buf[16:20], uint32(y))
	w.x11Send(buf)
}

func (w *X11Window) x11IconifyWindow() {
	// WM_CHANGE_STATE client message
	wmChangeState := w.x11InternAtom("WM_CHANGE_STATE")
	buf := make([]byte, 32)
	buf[0] = 33                                                    // ClientMessage
	buf[1] = 32                                                    // format
	binary.LittleEndian.PutUint16(buf[2:4], 8)                    // length
	binary.LittleEndian.PutUint32(buf[4:8], w.rootWin)            // window = root
	binary.LittleEndian.PutUint32(buf[8:12], wmChangeState)       // type
	binary.LittleEndian.PutUint32(buf[12:16], 3)                  // IconicState

	// Отправляем SendEvent к root window
	sendBuf := make([]byte, 44)
	sendBuf[0] = 25                                                // SendEvent
	sendBuf[1] = 0                                                 // propagate
	binary.LittleEndian.PutUint16(sendBuf[2:4], 11)               // length
	binary.LittleEndian.PutUint32(sendBuf[4:8], w.rootWin)
	binary.LittleEndian.PutUint32(sendBuf[8:12], 0x00180000)      // SubstructureRedirect|SubstructureNotify
	copy(sendBuf[12:], buf)
	w.x11Send(sendBuf)
}

func (w *X11Window) x11ToggleMaximize(maximize bool) {
	// _NET_WM_STATE client message to root
	action := uint32(0) // _NET_WM_STATE_REMOVE
	if maximize {
		action = 1 // _NET_WM_STATE_ADD
	}

	buf := make([]byte, 32)
	buf[0] = 33                                                    // ClientMessage
	buf[1] = 32                                                    // format
	binary.LittleEndian.PutUint16(buf[2:4], 8)
	binary.LittleEndian.PutUint32(buf[4:8], w.wid)
	binary.LittleEndian.PutUint32(buf[8:12], w.atomNetWMState)
	binary.LittleEndian.PutUint32(buf[12:16], action)
	binary.LittleEndian.PutUint32(buf[16:20], w.atomWMStateMaxH)
	binary.LittleEndian.PutUint32(buf[20:24], w.atomWMStateMaxV)

	sendBuf := make([]byte, 44)
	sendBuf[0] = 25
	sendBuf[1] = 0
	binary.LittleEndian.PutUint16(sendBuf[2:4], 11)
	binary.LittleEndian.PutUint32(sendBuf[4:8], w.rootWin)
	binary.LittleEndian.PutUint32(sendBuf[8:12], 0x00180000)
	copy(sendBuf[12:], buf)
	w.x11Send(sendBuf)
}

func (w *X11Window) x11PutImage(drawable, gc uint32, dstX, dstY, width, height int, data []byte) {
	// PutImage opcode=72
	// Для больших изображений отправляем полосами (X11 max request = 262140 bytes)
	maxDataPerReq := 262140 - 24 // оставляем место для заголовка
	rowBytes := width * 4
	maxRows := maxDataPerReq / rowBytes
	if maxRows < 1 {
		maxRows = 1
	}

	for yOff := 0; yOff < height; yOff += maxRows {
		rows := maxRows
		if yOff+rows > height {
			rows = height - yOff
		}

		dataLen := rows * rowBytes
		pad := (4 - dataLen%4) % 4
		reqLen := 6 + (dataLen+pad)/4

		buf := make([]byte, reqLen*4)
		buf[0] = 72 // PutImage
		buf[1] = 2  // ZPixmap format
		binary.LittleEndian.PutUint16(buf[2:4], uint16(reqLen))
		binary.LittleEndian.PutUint32(buf[4:8], drawable)
		binary.LittleEndian.PutUint32(buf[8:12], gc)
		binary.LittleEndian.PutUint16(buf[12:14], uint16(width))
		binary.LittleEndian.PutUint16(buf[14:16], uint16(rows))
		binary.LittleEndian.PutUint16(buf[16:18], uint16(dstX))      // dst-x
		binary.LittleEndian.PutUint16(buf[18:20], uint16(dstY+yOff)) // dst-y
		buf[20] = 0                                                  // left-pad
		buf[21] = 24                                                 // depth
		// buf[22:24] = padding (0)

		srcOff := yOff * rowBytes
		copy(buf[24:], data[srcOff:srcOff+dataLen])
		w.x11Send(buf)
	}
}

// ─── Маппинг клавиш X11 → VK ───────────────────────────────────────────────

func x11KeycodeToVK(keycode int) int {
	// X11 keycodes (стандартная раскладка evdev)
	switch keycode {
	case 22:
		return VK_BACKSPACE
	case 23:
		return VK_TAB
	case 36:
		return VK_ENTER
	case 9:
		return VK_ESCAPE
	case 65:
		return VK_SPACE
	case 113:
		return VK_LEFT
	case 111:
		return VK_UP
	case 114:
		return VK_RIGHT
	case 116:
		return VK_DOWN
	case 119:
		return VK_DELETE
	case 110:
		return VK_HOME
	case 115:
		return VK_END
	case 38:
		return VK_A
	case 54:
		return VK_C
	case 55:
		return VK_V
	case 53:
		return VK_X
	case 52:
		return VK_Z
	case 50, 62:
		return VK_SHIFT
	case 37, 105:
		return VK_CONTROL
	case 64, 108:
		return VK_ALT
	}
	return 0
}

func x11KeycodeToRune(keycode int, shift bool) rune {
	// Упрощённый маппинг для ASCII (полная реализация через XKB/xkbcommon)
	if keycode >= 10 && keycode <= 19 {
		// Number row: keycode 10='1', 19='0'
		if keycode == 19 {
			return '0'
		}
		return rune('1' + keycode - 10)
	}
	// Letters a-z: keycodes 38-58 (evdev)
	letters := "asdfghjkl;'qwertyuiop[]\\zxcvbnm,./"
	idx := keycode - 38
	if idx >= 0 && idx < len(letters) {
		r := rune(letters[idx])
		if shift && r >= 'a' && r <= 'z' {
			r -= 32
		}
		return r
	}
	if keycode == 65 {
		return ' '
	}
	return 0
}

func uint32ToBytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

// Ensure unused import doesn't cause error
var _ = unsafe.Sizeof(0)
