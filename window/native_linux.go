//go:build linux && !android

package window

import (
	"encoding/binary"
	"fmt"
	"image"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"unsafe"
)

// X11Window — реализация NativeWindow через X11 протокол.
// Чистый Go без CGO и внешних зависимостей.
// Общается с X-сервером напрямую через Unix socket.
type X11Window struct {
	// linuxNotifier — системные уведомления через D-Bus (notify_linux.go).
	// Встроен анонимно: его методы showBalloon/setBalloonClickHandler делают
	// окно balloonHost'ом, и Window.ShowBalloon работает без иконки в трее.
	linuxNotifier

	// linuxTray — иконка в системном трее по StatusNotifierItem
	// (tray_sni_linux.go). Вместе с методами hideToTray/restoreFromTray ниже
	// делает окно trayHost'ом (tray.go).
	linuxTray

	conn      net.Conn
	screen    x11Screen
	rootWin   uint32
	wid       uint32 // window ID
	gcID      uint32 // graphics context ID
	width     int
	height    int
	title     string
	maximized bool
	// closed — окно закрыто/уничтожается. Атомарный: главное окно читает его в
	// RunEventLoop, вторичное — в eventPumpLoop, а Close() (для вторичного окна
	// вызывается из горутины teardown) пишет — доступ из разных горутин.
	closed    atomic.Bool

	seqNum    uint16 // sequence number for requests
	mu        sync.Mutex

	// blitBuf — переиспользуемый буфер BGRA-конвертации для PutImage
	// (раньше выделялся полный кадр на каждый блит). blitMu сериализует
	// блиты из framePump и из Expose-обработчика (event loop).
	blitBuf []byte
	blitMu  sync.Mutex

	// shm — состояние MIT-SHM (см. x11shm.go): блит через разделяемую память
	// вместо PutImage по сокету, с фолбэком на PutImage. nil до инициализации
	// (x11ShmInit из Create/CreatePopup); shm.fallback — SHM недоступен/не
	// вышел, PutImage навсегда.
	shm *x11ShmState

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
	atomNetWMStateModal uint32 // _NET_WM_STATE_MODAL (модальность у EWMH-WM)
	atomNetActiveWindow uint32 // _NET_ACTIVE_WINDOW (передача фокуса окну)

	// disabled — окно заблокировано на время модального диалога (аналог
	// Win32 EnableWindow(false)). У X11 нет системного отключения ввода,
	// поэтому обработчик событий сам ДРОПАЕТ ввод (Key/Button/Motion) при
	// взведённом флаге, продолжая обрабатывать Expose/Configure. Атомарный —
	// читается из насоса событий, пишется из произвольной горутины (dialogHost).
	disabled atomic.Bool

	// Позиция клиентской области в ЭКРАННЫХ координатах. GetPosition обязан
	// возвращать правдивое значение из ЛЮБОЙ горутины, но запрос-ответ
	// (TranslateCoordinates) по этому соединению небезопасен: conn читает цикл
	// событий, а SendMouseMove вызывает OnDragMove→GetPosition прямо в горутине
	// насоса событий (блокирующее ожидание reply дало бы дедлок — читать ответ
	// некому). Поэтому позицию КЭШируем: seed из Create (верно без WM сразу),
	// обновление из ConfigureNotify и оптимистично из SetPosition.
	// posSynthetic: у reparenting-WM (openbox) достоверны координаты только
	// SYNTHETIC ConfigureNotify (root-relative, ICCCM 4.2.3); увидев такое,
	// real-события (parent-relative под рамкой) в кэш больше не берём.
	posMu        sync.Mutex
	posX, posY   int
	posCached    bool
	posSynthetic bool

	// Атомы UTF-8 заголовка (EWMH) и Motif-хинтов — интернируются в Create,
	// чтобы SetTitle/borderless работали и после первого показа.
	atomUTF8String    uint32
	atomNetWMName     uint32
	atomNetWMIconName uint32
	atomMotifHints    uint32

	// ── XDND (Drag&Drop файлов из ОС, протокол v5) ──────────────────────────
	atomXdndAware      uint32
	atomXdndEnter      uint32
	atomXdndPosition   uint32
	atomXdndStatus     uint32
	atomXdndLeave      uint32
	atomXdndDrop       uint32
	atomXdndFinished   uint32
	atomXdndSelection  uint32
	atomXdndTypeList   uint32
	atomXdndActionCopy uint32
	atomTextUriList    uint32

	// Состояние текущей DnD-сессии (заполняется из ClientMessage-событий).
	dndSource    uint32 // окно-источник перетаскивания (0 — нет сессии)
	dndVersion   int    // версия протокола источника
	dndAccept    bool   // предложен ли text/uri-list (принимаем ли сброс)
	dndX, dndY   int    // последняя позиция курсора (КОРНЕВЫЕ/экранные координаты)
	dndTime      uint32 // timestamp из XdndDrop (для XConvertSelection)
	dndDropPending bool // ждём SelectionNotify после XConvertSelection

	onFilesDropped func(paths []string, x, y int)
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

// connect подключается к X-серверу (Unix socket из DISPLAY) и выполняет
// протокольный setup. Общий для главного окна (Create) и окна-попапа (CreatePopup).
func (w *X11Window) connect() error {
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
	return nil
}

func (w *X11Window) Create(title string, width, height int) error {
	w.title = title
	w.width = width
	w.height = height

	if err := w.connect(); err != nil {
		return err
	}

	// Создаём окно
	w.wid = w.x11GenID()
	w.gcID = w.x11GenID()

	// Раскладка клавиатуры (до появления событий — reply читается напрямую).
	w.x11LoadKeyboardMapping()

	// MIT-SHM: пробуем активировать блит через разделяемую память ДО
	// CreateWindow/MapWindow — событий на соединении ещё нет, reply читается
	// напрямую (см. x11ShmInit в x11shm.go). Неудача — молчаливый fallback
	// на PutImage.
	w.x11ShmInit()

	// Intern atoms для WM-протоколов
	w.atomWMProtocols = w.x11InternAtom("WM_PROTOCOLS")
	w.atomWMDeleteWindow = w.x11InternAtom("WM_DELETE_WINDOW")
	w.atomNetWMState = w.x11InternAtom("_NET_WM_STATE")
	w.atomWMStateMaxH = w.x11InternAtom("_NET_WM_STATE_MAXIMIZED_HORZ")
	w.atomWMStateMaxV = w.x11InternAtom("_NET_WM_STATE_MAXIMIZED_VERT")
	w.atomNetWMStateModal = w.x11InternAtom("_NET_WM_STATE_MODAL")
	w.atomNetActiveWindow = w.x11InternAtom("_NET_ACTIVE_WINDOW")
	w.atomUTF8String = w.x11InternAtom("UTF8_STRING")
	w.atomNetWMName = w.x11InternAtom("_NET_WM_NAME")
	w.atomNetWMIconName = w.x11InternAtom("_NET_WM_ICON_NAME")
	w.atomMotifHints = w.x11InternAtom("_MOTIF_WM_HINTS")

	// XDND (Drag&Drop файлов из ОС).
	w.atomXdndAware = w.x11InternAtom("XdndAware")
	w.atomXdndEnter = w.x11InternAtom("XdndEnter")
	w.atomXdndPosition = w.x11InternAtom("XdndPosition")
	w.atomXdndStatus = w.x11InternAtom("XdndStatus")
	w.atomXdndLeave = w.x11InternAtom("XdndLeave")
	w.atomXdndDrop = w.x11InternAtom("XdndDrop")
	w.atomXdndFinished = w.x11InternAtom("XdndFinished")
	w.atomXdndSelection = w.x11InternAtom("XdndSelection")
	w.atomXdndTypeList = w.x11InternAtom("XdndTypeList")
	w.atomXdndActionCopy = w.x11InternAtom("XdndActionCopy")
	w.atomTextUriList = w.x11InternAtom("text/uri-list")

	// CreateWindow request
	x := (int(w.screen.WidthInPixels) - width) / 2
	y := (int(w.screen.HeightInPixels) - height) / 2

	// Seed кэша позиции: без WM окно останется ровно здесь, поэтому уже сейчас
	// значение правдиво; под reparenting-WM его скорректирует synthetic
	// ConfigureNotify после карты.
	w.posMu.Lock()
	w.posX, w.posY = x, y
	w.posCached = true
	w.posMu.Unlock()

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

	// Borderless — убираем WM decorations через Motif hints. Свойство ставим
	// ДО MapWindow (иначе WM успевает нарисовать рамку). Структура MwmHints —
	// 5×CARD32 (flags, functions, decorations, input_mode, status); тип
	// property — сам атом _MOTIF_WM_HINTS (НЕ CARDINAL), format 32.
	if w.atomMotifHints != 0 {
		hints := make([]byte, 20)
		binary.LittleEndian.PutUint32(hints[0:4], 2)  // flags = MWM_HINTS_DECORATIONS
		binary.LittleEndian.PutUint32(hints[8:12], 0) // decorations = 0 (нет рамки)
		w.x11ChangeProperty(w.wid, w.atomMotifHints, w.atomMotifHints, 32, hints)
	}

	// XdndAware = 5: объявляем поддержку XDND v5 (тип property — ATOM(4),
	// format 32, значение — версия протокола). Источник читает это свойство,
	// решая, принимает ли окно перетаскивание.
	if w.atomXdndAware != 0 {
		w.x11ChangeProperty(w.wid, w.atomXdndAware, 4 /*ATOM*/, 32, uint32ToBytes(5))
	}

	// Window title
	w.x11SetTitle(w.wid, title)

	// Map (show) window
	w.x11MapWindow(w.wid)

	return nil
}

// CreatePopup создаёт окно-вьюпорт оверлея как override-redirect: WM его не
// трогает (ни рамки, ни фокуса, ни декораций — родное поведение всплывающих
// меню). Позиция задаётся позже (SetPosition в корневых координатах). У окна
// собственное соединение и насос событий (StartEventPump), как у вторичных
// окон диалогов.
func (w *X11Window) CreatePopup(width, height int) error {
	w.width = width
	w.height = height

	if err := w.connect(); err != nil {
		return err
	}

	w.wid = w.x11GenID()
	w.gcID = w.x11GenID()
	w.x11LoadKeyboardMapping()

	// MIT-SHM: та же инициализация, что и в Create (см. комментарий там) —
	// до CreateWindow, событий на соединении ещё нет.
	w.x11ShmInit()

	// Значения в порядке возрастания бит маски: BackPixel(0x02),
	// OverrideRedirect(0x200), EventMask(0x800).
	eventMask := uint32(
		0x00000001 | // KeyPress
			0x00000002 | // KeyRelease
			0x00000004 | // ButtonPress
			0x00000008 | // ButtonRelease
			0x00000040 | // PointerMotion
			0x00008000) // Exposure
	values := []uint32{
		w.screen.BlackPixel, // background
		1,                   // override-redirect = true
		eventMask,
	}
	valueMask := uint32(0x00000002 | 0x00000200 | 0x00000800) // BackPixel|OverrideRedirect|EventMask

	// Создаём в (0,0); реальную позицию выставит хост через SetPosition.
	w.x11CreateWindow(w.wid, w.screen.Root, 0, 0,
		uint16(width), uint16(height), 0, valueMask, values)
	w.x11CreateGC(w.gcID, w.wid)

	// Seed кэша позиции (override-redirect: MoveWindow задаёт корневые координаты).
	w.posMu.Lock()
	w.posX, w.posY = 0, 0
	w.posCached = true
	w.posMu.Unlock()

	w.x11MapWindow(w.wid)
	return nil
}

func (w *X11Window) RunEventLoop() error {
	buf := make([]byte, 32)
	for !w.closed.Load() {
		if !w.readFull(buf) {
			if w.closed.Load() {
				return nil
			}
			return fmt.Errorf("x11: read event: соединение закрыто")
		}
		w.handleX11Event(buf)
	}
	return nil
}

// handleX11Event обрабатывает одно 32-байтовое событие X11. Выделено из
// RunEventLoop, чтобы события, пришедшие между запросом и ответом (например,
// при перечитывании раскладки по MappingNotify), не терялись.
func (w *X11Window) handleX11Event(buf []byte) {
	evType := buf[0] & 0x7F
	// ShmCompletion (MIT-SHM): сервер закончил читать сегмент из предыдущего
	// ShmPutImage, память снова наша (см. x11shm.go). Проверяем ДО switch по
	// core-кодам событий: firstEvent — динамический код за пределами core
	// диапазона, а w.shm нередко nil (SHM не инициализировался/недоступен).
	if w.shm != nil && !w.shm.fallback && evType == w.shm.firstEvent {
		w.x11ShmCompletion()
		return
	}
	// Окно заблокировано модалкой (SetEnabled(false)): дропаем ввод
	// (KeyPress=2, KeyRelease=3, ButtonPress=4, ButtonRelease=5, MotionNotify=6),
	// но НЕ Expose/ConfigureNotify/ClientMessage — окно должно перерисовываться
	// и продолжать реагировать на WM. Этого достаточно для модальности: родитель
	// не отвечает на ввод, пока открыт диалог.
	if w.disabled.Load() {
		switch evType {
		case 2, 3, 4, 5, 6:
			return
		}
	}
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
			// Кэш позиции для GetPosition (см. updatePosFromConfigure).
			cx, cy, synthetic := parseConfigureNotifyPos(buf)
			w.updatePosFromConfigure(cx, cy, synthetic)
			newW := int(binary.LittleEndian.Uint16(buf[20:22]))
			newH := int(binary.LittleEndian.Uint16(buf[22:24]))
			if newW != w.width || newH != w.height {
				w.width = newW
				w.height = newH
				w.x11ShmResize(newW, newH)
				if w.onResize != nil {
					w.onResize(newW, newH)
				}
			}

		case 31: // SelectionNotify — ответ на XConvertSelection (XDND drop)
			w.handleSelectionNotify(buf)

		case 33: // ClientMessage (WM_DELETE_WINDOW / XDND)
			atom := binary.LittleEndian.Uint32(buf[8:12])
			switch atom {
			case w.atomWMDeleteWindow:
				if w.onClose != nil {
					if w.onClose() {
						w.closed.Store(true)
					}
				} else {
					w.closed.Store(true)
				}
			case w.atomXdndEnter:
				w.handleXdndEnter(buf)
			case w.atomXdndPosition:
				w.handleXdndPosition(buf)
			case w.atomXdndLeave:
				w.dndReset()
			case w.atomXdndDrop:
				w.handleXdndDrop(buf)
			}

	case 34: // MappingNotify — раскладка/маппинг клавиатуры изменились
		// (setxkbmap на лету, динамический remap xdotool и т.п.) —
		// перечитываем таблицу keysym'ов.
		w.x11ReloadKeyboardMapping()
	}
}

// x11ReloadKeyboardMapping перечитывает GetKeyboardMapping во время работы
// event loop: пакеты, оказавшиеся между запросом и ответом, — это обычные
// события, они обрабатываются на месте (reply отличается типом 1).
func (w *X11Window) x11ReloadKeyboardMapping() {
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

	pkt := make([]byte, 32)
	for i := 0; i < 4096; i++ { // предохранитель от бесконечного цикла
		if !w.readFull(pkt) {
			return
		}
		switch pkt[0] {
		case 1: // наш reply
			symsPer := int(pkt[1])
			length := int(binary.LittleEndian.Uint32(pkt[4:8])) * 4
			data := make([]byte, length)
			if !w.readFull(data) || symsPer == 0 {
				return
			}
			syms := make([]uint32, length/4)
			for j := range syms {
				syms[j] = binary.LittleEndian.Uint32(data[j*4 : j*4+4])
			}
			w.minKeycode = first
			w.symsPerCode = symsPer
			w.keysyms = syms
			return
		case 0: // error-пакет (32 байта) — запрос отвергнут
			return
		default: // обычное событие — обрабатываем на месте
			w.handleX11Event(pkt)
		}
	}
}

func (w *X11Window) Close() {
	w.closed.Store(true)
	// Освобождаем локальный маппинг SHM-сегмента (см. x11ShmClose): серверную
	// половину X-сервер снимает сам при закрытии соединения.
	w.x11ShmClose()
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
		// Программный ресайз не проходит через ConfigureNotify-ветку (width/
		// height уже записаны выше, и проверка «размер изменился» там не
		// сработает), поэтому SHM-сегмент пересоздаём здесь сами — иначе блит
		// тихо жил бы на PutImage-фолбэке до первого ресайза мышью.
		w.x11ShmResize(width, height)
	}
}

func (w *X11Window) GetSize() (int, int) {
	return w.width, w.height
}

func (w *X11Window) SetPosition(x, y int) {
	if w.wid != 0 {
		w.x11MoveWindow(w.wid, x, y)
		// Оптимистично обновляем кэш: под reparenting-WM ConfigureWindow
		// трактуется как запрос корневой позиции (WM двигает рамку), поэтому
		// x,y ≈ будущие экранные координаты; synthetic ConfigureNotify уточнит.
		// Даёт плавный drag (GetPosition сразу после SetPosition не отстаёт).
		w.posMu.Lock()
		w.posX, w.posY = x, y
		w.posCached = true
		w.posMu.Unlock()
	}
}

// GetPosition возвращает экранные координаты клиентской области окна.
// Значение берётся из кэша (см. поле posX/posY): запрос-ответ по этому
// соединению здесь недопустим — его читает цикл событий, а вызов приходит и из
// чужой горутины (dialogHost.centerOn), и прямо из горутины насоса событий
// (OnDragMove). Кэш заполняется seed'ом в Create, событиями ConfigureNotify
// (у reparenting-WM — по достоверным synthetic) и оптимистично в SetPosition.
func (w *X11Window) GetPosition() (int, int) {
	w.posMu.Lock()
	defer w.posMu.Unlock()
	if !w.posCached {
		return 0, 0
	}
	return w.posX, w.posY
}

// updatePosFromConfigure обновляет кэш позиции по данным ConfigureNotify.
// synthetic — событие, посланное WM через SendEvent (ICCCM 4.2.3): его x,y —
// корневые (экранные) координаты клиентской области. У reparenting-WM только
// такие координаты достоверны; real-события несут координаты относительно рамки.
// Без WM reparent'а нет и координаты real-события тоже корневые — их принимаем,
// пока не увидели ни одного synthetic.
func (w *X11Window) updatePosFromConfigure(x, y int, synthetic bool) {
	w.posMu.Lock()
	if synthetic {
		w.posX, w.posY = x, y
		w.posSynthetic = true
		w.posCached = true
	} else if !w.posSynthetic {
		w.posX, w.posY = x, y
		w.posCached = true
	}
	w.posMu.Unlock()
}

// parseConfigureNotifyPos извлекает x,y и признак synthetic из 32-байтового
// события ConfigureNotify. Формат: код@0 (бит 7 — send_event/synthetic),
// x@16, y@18 (INT16). Вынесено для юнит-тестов без X-сервера.
func parseConfigureNotifyPos(buf []byte) (x, y int, synthetic bool) {
	x = int(int16(binary.LittleEndian.Uint16(buf[16:18])))
	y = int(int16(binary.LittleEndian.Uint16(buf[18:20])))
	synthetic = buf[0]&0x80 != 0
	return
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

	// MIT-SHM: если расширение активно и сервер уже подтвердил предыдущий
	// кадр (ShmCompletion), шлём этот через разделяемую память — короткий
	// ShmPutImage вместо копирования пикселей в тело запроса. Если SHM
	// недоступен или занят предыдущим кадром — падаем на обычный PutImage
	// за этот кадр, не блокируясь (см. x11ShmBlit в x11shm.go).
	if w.x11ShmBlit(img, dirty) {
		return
	}

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

// SetOnFilesDropped регистрирует колбэк Drag&Drop файлов из ОС (XDND).
// Координаты — клиентские физические пиксели.
func (w *X11Window) SetOnFilesDropped(fn func(paths []string, x, y int)) { w.onFilesDropped = fn }

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

// x11UnmapWindow убирает окно с экрана (UnmapWindow). У EWMH-WM оно при этом
// пропадает и из панели задач — полный аналог Win32 SW_HIDE, нужен HideToTray.
func (w *X11Window) x11UnmapWindow(wid uint32) {
	buf := make([]byte, 8)
	buf[0] = 10 // UnmapWindow
	binary.LittleEndian.PutUint16(buf[2:4], 2)
	binary.LittleEndian.PutUint32(buf[4:8], wid)
	w.x11Send(buf)
}

func (w *X11Window) x11SetTitle(wid uint32, title string) {
	data := []byte(title)
	// WM_NAME/WM_ICON_NAME (тип STRING = latin1) — совместимость со старыми WM.
	// Кириллица/UTF-8 в них отображается мусором, поэтому это лишь fallback.
	w.x11ChangeProperty(wid, 39 /*WM_NAME*/, 31 /*STRING*/, 8, data)
	w.x11ChangeProperty(wid, 37 /*WM_ICON_NAME*/, 31 /*STRING*/, 8, data)
	// _NET_WM_NAME/_NET_WM_ICON_NAME (тип UTF8_STRING, format 8) — EWMH-путь,
	// который openbox и прочие современные WM показывают корректно (UTF-8).
	if w.atomUTF8String != 0 {
		if w.atomNetWMName != 0 {
			w.x11ChangeProperty(wid, w.atomNetWMName, w.atomUTF8String, 8, data)
		}
		if w.atomNetWMIconName != 0 {
			w.x11ChangeProperty(wid, w.atomNetWMIconName, w.atomUTF8String, 8, data)
		}
	}
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

	// Читаем ответ (32 байта) ЦЕЛИКОМ: одиночный Read может вернуть меньше,
	// тогда reply[8:12] — мусор, а следующий InternAtom прочитает хвост и
	// рассинхронизирует ВСЕ последующие атомы (в т.ч. _MOTIF_WM_HINTS → WM
	// игнорирует borderless и рисует рамку). Интерн идёт до MapWindow, события
	// не вклиниваются, поэтому readFull здесь достаточно.
	reply := make([]byte, 32)
	if !w.readFull(reply) {
		return 0
	}
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
	case 118: // evdev KEY_INSERT=110, +8 = 118 (не путать с 112/117 PgUp/PgDn)
		return VK_INSERT
	case 110:
		return VK_HOME
	case 115:
		return VK_END
	case 112:
		return VK_PRIOR // Page Up
	case 117:
		return VK_NEXT // Page Down
	case 38:
		return VK_A
	case 54:
		return VK_C
	case 55:
		return VK_V
	case 53:
		return VK_X
	case 29:
		return VK_Y
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

// SetResizable — no-op: пользовательский resize за края borderless-окна
// на этой платформе пока не реализован.
func (w *X11Window) SetResizable(v bool) {}

// SetMinSize — no-op: минимальный размер окна на X11 пока не ограничиваем.
func (w *X11Window) SetMinSize(width, height int) {}

// ─── Поддержка нативных модальных диалогов (dialogHost) ─────────────────────
//
// X11-бэкенд реализует опциональные интерфейсы window-пакета:
//   - uiThreadInvoker (InvokeOnUIThread) — маршалинг колбэков;
//   - ownedWindow      (SetOwner/SetEnabled) — owner + блокировка родителя;
//   - foregrounder     (SetForeground) — возврат фокуса;
//   - eventPumper      (StartEventPump) — насос событий вторичного окна.
//
// В отличие от Win32 (одна очередь сообщений на все окна, привязка к потоку),
// у каждого X11-окна СВОЁ соединение и thread-affinity нет: нативные запросы
// (x11Send) сериализуются мьютексом соединения и легальны из любой горутины.

// InvokeOnUIThread выполняет fn. На X11 привязки к потоку нет, поэтому запуск
// в отдельной горутине безопасен и, в отличие от синхронного вызова, НЕ блокирует
// чтение событий родителя (dialogHost зовёт это из UI-колбэка цикла событий
// родителя для create() и из горутин движка для teardown()). Все нативные
// операции внутри fn идут через собственные соединения окон и защищены мьютексом.
func (w *X11Window) InvokeOnUIThread(fn func()) {
	if fn == nil {
		return
	}
	go fn()
}

// SetOwner делает окно transient-for parent (WM_TRANSIENT_FOR): WM держит его
// поверх родителя и группирует с ним. Дополнительно (bonus) помечает окно
// модальным через _NET_WM_STATE_MODAL для EWMH-совместимых WM.
//
// Аналог Win32 noQuit здесь не нужен: у вторичного окна СВОЙ насос событий
// (StartEventPump), а не общий RunEventLoop, поэтому его закрытие никогда не
// завершает цикл событий главного окна и не роняет приложение.
func (w *X11Window) SetOwner(parent NativeWindow) {
	px, ok := parent.(*X11Window)
	if !ok || w.wid == 0 || px.wid == 0 {
		return
	}
	// WM_TRANSIENT_FOR — предопределённый атом 68; тип XA_WINDOW (33), формат 32.
	// ID окон глобальны на сервере, поэтому разные соединения не мешают.
	w.x11ChangeProperty(w.wid, 68, 33, 32, uint32ToBytes(px.wid))
	// _NET_WM_STATE_MODAL: окно уже отображено (Create делает MapWindow), поэтому
	// изменяем состояние через ClientMessage _NET_WM_STATE ADD к root.
	if w.atomNetWMState != 0 && w.atomNetWMStateModal != 0 {
		w.x11NetWMStateAdd(w.atomNetWMStateModal)
	}
}

// SetEnabled включает/выключает реакцию окна на ввод. У X11 нет системного
// EnableWindow — реализуем флагом disabled: обработчик событий дропает ввод,
// см. handleX11Event.
func (w *X11Window) SetEnabled(v bool) { w.disabled.Store(!v) }

// SetForeground поднимает окно (ConfigureWindow stack-mode=Above), просит WM
// активировать его (_NET_ACTIVE_WINDOW) и переносит фокус ввода (SetInputFocus).
func (w *X11Window) SetForeground() {
	if w.wid == 0 {
		return
	}
	w.x11RaiseWindow()
	w.x11NetActiveWindow()
	w.x11SetInputFocus()
}

// StartEventPump запускает насос событий вторичного окна в отдельной горутине.
// На Win32 не нужен (одна очередь сообщений на все окна) — там интерфейс не
// реализован. На X11 у окна собственное соединение, которое иначе никто не
// читает: dialogHost НЕ вызывает RunEventLoop для вторичного окна.
func (w *X11Window) StartEventPump() {
	if w.conn == nil {
		return
	}
	go w.eventPumpLoop()
}

// eventPumpLoop читает события соединения этого окна, пока оно не закрыто.
// Переиспользует логику RunEventLoop/handleX11Event. Завершается корректно при
// уничтожении окна: Close() выставляет closed и закрывает conn → readFull
// возвращает false (без паники на закрытом соединении) → цикл выходит. Насос
// главного окна (RunEventLoop) при этом не затрагивается — у него своё conn.
func (w *X11Window) eventPumpLoop() {
	buf := make([]byte, 32)
	for !w.closed.Load() {
		if !w.readFull(buf) {
			return
		}
		w.handleX11Event(buf)
	}
}

// x11RaiseWindow поднимает окно на вершину стека (ConfigureWindow, stack-mode=Above).
func (w *X11Window) x11RaiseWindow() {
	buf := make([]byte, 16)
	buf[0] = 12 // ConfigureWindow
	binary.LittleEndian.PutUint16(buf[2:4], 4)     // length (16 байт)
	binary.LittleEndian.PutUint32(buf[4:8], w.wid)
	binary.LittleEndian.PutUint16(buf[8:10], 0x40) // value-mask: stack-mode
	// buf[10:12] — паддинг
	binary.LittleEndian.PutUint32(buf[12:16], 0)   // Above
	w.x11Send(buf)
}

// x11SetInputFocus передаёт фокус ввода окну (SetInputFocus, revert-to=Parent).
func (w *X11Window) x11SetInputFocus() {
	buf := make([]byte, 12)
	buf[0] = 42 // SetInputFocus
	buf[1] = 2  // revert-to = Parent
	binary.LittleEndian.PutUint16(buf[2:4], 3)   // length
	binary.LittleEndian.PutUint32(buf[4:8], w.wid)
	binary.LittleEndian.PutUint32(buf[8:12], 0)  // time = CurrentTime
	w.x11Send(buf)
}

// x11NetActiveWindow просит WM активировать окно (EWMH _NET_ACTIVE_WINDOW).
func (w *X11Window) x11NetActiveWindow() {
	if w.atomNetActiveWindow == 0 {
		return
	}
	buf := make([]byte, 32)
	buf[0] = 33                                              // ClientMessage
	buf[1] = 32                                              // format
	binary.LittleEndian.PutUint16(buf[2:4], 8)
	binary.LittleEndian.PutUint32(buf[4:8], w.wid)          // window
	binary.LittleEndian.PutUint32(buf[8:12], w.atomNetActiveWindow) // type
	binary.LittleEndian.PutUint32(buf[12:16], 1)           // data[0]: source = application
	binary.LittleEndian.PutUint32(buf[16:20], 0)           // data[1]: timestamp = 0
	binary.LittleEndian.PutUint32(buf[20:24], 0)           // data[2]: текущее активное окно

	sendBuf := make([]byte, 44)
	sendBuf[0] = 25 // SendEvent
	sendBuf[1] = 0  // propagate = false
	binary.LittleEndian.PutUint16(sendBuf[2:4], 11)
	binary.LittleEndian.PutUint32(sendBuf[4:8], w.rootWin)
	binary.LittleEndian.PutUint32(sendBuf[8:12], 0x00180000) // SubstructureRedirect|SubstructureNotify
	copy(sendBuf[12:], buf)
	w.x11Send(sendBuf)
}

// x11NetWMStateAdd добавляет одно состояние _NET_WM_STATE отображённому окну
// (ClientMessage к root, action = _NET_WM_STATE_ADD).
func (w *X11Window) x11NetWMStateAdd(prop uint32) {
	buf := make([]byte, 32)
	buf[0] = 33 // ClientMessage
	buf[1] = 32 // format
	binary.LittleEndian.PutUint16(buf[2:4], 8)
	binary.LittleEndian.PutUint32(buf[4:8], w.wid)
	binary.LittleEndian.PutUint32(buf[8:12], w.atomNetWMState)
	binary.LittleEndian.PutUint32(buf[12:16], 1)   // _NET_WM_STATE_ADD
	binary.LittleEndian.PutUint32(buf[16:20], prop) // первое свойство
	binary.LittleEndian.PutUint32(buf[20:24], 0)   // второе свойство — нет
	binary.LittleEndian.PutUint32(buf[24:28], 1)   // source = application

	sendBuf := make([]byte, 44)
	sendBuf[0] = 25
	sendBuf[1] = 0
	binary.LittleEndian.PutUint16(sendBuf[2:4], 11)
	binary.LittleEndian.PutUint32(sendBuf[4:8], w.rootWin)
	binary.LittleEndian.PutUint32(sendBuf[8:12], 0x00180000)
	copy(sendBuf[12:], buf)
	w.x11Send(sendBuf)
}
