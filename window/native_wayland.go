//go:build linux && !android

package window

// Wayland-бэкенд: сырой wire-протокол через unix-сокет, без CGO и внешних
// зависимостей (симметрично X11-бэкенду в native_linux.go).
//
// Использует базовые интерфейсы ядра протокола + xdg-shell:
//
//	wl_display, wl_registry, wl_compositor, wl_shm(+pool/buffer),
//	wl_seat(+pointer/keyboard), xdg_wm_base, xdg_surface, xdg_toplevel
//
// Кадры — wl_shm: memfd + mmap, формат XRGB8888 (байты B,G,R,X — тот же
// swizzle, что у X11/Win32). Двойная буферизация: композитор читает буфер
// асинхронно (до wl_buffer.release), пишем в свободный.
//
// Модель окна движка совпадает с Wayland-моделью: chrome (заголовок, кнопки,
// ресайз) рисует клиент, серверных декораций не запрашиваем; expose-событий
// нет (композитор ретейнит содержимое); активность окна приходит состоянием
// activated в xdg_toplevel.configure.
//
// Клавиатура: keymap не парсится (xkb), коды linux evdev транслируются как
// evdev+8 через общий x11KeycodeToVK — раскладко-независимые VK_* совпадают
// с X11-бэкендом.

import (
	"encoding/binary"
	"fmt"
	"image"
	"net"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// ─── Константы протокола ─────────────────────────────────────────────────────

const (
	wlDisplayID = 1 // предопределённый объект wl_display

	// wl_display requests / events
	wlDisplaySync        = 0
	wlDisplayGetRegistry = 1
	wlDisplayEvError     = 0
	wlDisplayEvDeleteID  = 1

	// wl_registry
	wlRegistryBind     = 0
	wlRegistryEvGlobal = 0

	// wl_compositor
	wlCompositorCreateSurface = 0

	// wl_surface
	wlSurfaceAttach = 1
	wlSurfaceDamage = 2
	wlSurfaceCommit = 6

	// wl_shm / pool / buffer
	wlShmCreatePool       = 0
	wlShmPoolCreateBuffer = 0
	wlBufferEvRelease     = 0

	// wl_seat
	wlSeatGetPointer     = 0
	wlSeatGetKeyboard    = 1
	wlSeatEvCapabilities = 0
	seatCapPointer       = 1
	seatCapKeyboard      = 2

	// wl_pointer events
	wlPointerEvEnter  = 0
	wlPointerEvLeave  = 1
	wlPointerEvMotion = 2
	wlPointerEvButton = 3
	wlPointerEvAxis   = 4

	// wl_keyboard events
	wlKeyboardEvKeymap = 0
	wlKeyboardEvKey    = 3

	// xdg_wm_base
	xdgWmBaseGetXdgSurface = 2
	xdgWmBasePong          = 3
	xdgWmBaseEvPing        = 0

	// xdg_surface
	xdgSurfaceGetToplevel  = 1
	xdgSurfaceAckConfigure = 4
	xdgSurfaceEvConfigure  = 0

	// xdg_toplevel
	xdgToplevelSetTitle   = 2
	xdgToplevelSetAppID   = 3
	xdgToplevelSetMinSize = 8
	xdgToplevelSetMaxSize = 7
	xdgToplevelEvConfigure = 0
	xdgToplevelEvClose     = 1

	xdgStateActivated = 4

	wlShmFormatXRGB8888 = 1

	// linux input-event-codes
	btnLeft   = 0x110
	btnRight  = 0x111
	btnMiddle = 0x112
)

// ─── WaylandWindow ───────────────────────────────────────────────────────────

// WaylandWindow — реализация NativeWindow поверх Wayland (xdg-shell + wl_shm).
type WaylandWindow struct {
	conn *net.UnixConn

	mu     sync.Mutex // сериализует запись в сокет и доступ к буферам
	nextID uint32     // следующий client-side object id

	// Глобальные объекты (id после bind)
	registryID   uint32
	compositorID uint32
	shmID        uint32
	seatID       uint32
	wmBaseID     uint32
	// имена глобалов registry (для bind)
	gCompositor, gShm, gSeat, gWmBase uint32

	surfaceID    uint32
	xdgSurfaceID uint32
	toplevelID   uint32
	pointerID    uint32
	keyboardID   uint32

	// shm-пул с двумя кадровыми буферами
	poolID   uint32
	shmFD    int
	shmData  []byte
	bufID    [2]uint32
	bufBusy  [2]bool
	curBuf   int
	stride   int
	poolW    int
	poolH    int

	width, height int
	title         string
	closed        bool
	configured    bool
	hasFrame      bool // был ли закоммичен хотя бы один кадр
	pendingSerial uint32

	// координаты указателя (motion приходит в fixed 24.8)
	ptrX, ptrY int

	rxFDs []int // fd, принятые через SCM_RIGHTS (keymap и т.п.)

	// Callbacks (интерфейс NativeWindow)
	onResize      func(w, h int)
	onClose       func() bool
	onMouseMove   func(x, y int)
	onMouseButton func(x, y, button int, pressed bool)
	onKeyDown     func(vk int)
	onKeyUp       func(vk int)
	onChar        func(r rune)
	onActivate    func(active bool)
}

// waylandSocketPath возвращает путь к сокету композитора или "".
func waylandSocketPath() string {
	disp := os.Getenv("WAYLAND_DISPLAY")
	if disp == "" {
		return ""
	}
	if disp[0] == '/' {
		return disp
	}
	run := os.Getenv("XDG_RUNTIME_DIR")
	if run == "" {
		return ""
	}
	return run + "/" + disp
}

// wlDebug — трассировка протокола (HEADLESS_GUI_WL_DEBUG=1).
var wlDebug = os.Getenv("HEADLESS_GUI_WL_DEBUG") == "1"

func wlLog(format string, args ...any) {
	if wlDebug {
		fmt.Fprintf(os.Stderr, "[wl] "+format+"\n", args...)
	}
}

// newWaylandWindow пробует подключиться к композитору. nil — недоступен.
func newWaylandWindow() *WaylandWindow {
	path := waylandSocketPath()
	if path == "" {
		return nil
	}
	raddr := &net.UnixAddr{Name: path, Net: "unix"}
	conn, err := net.DialUnix("unix", nil, raddr)
	if err != nil {
		return nil
	}
	wlLog("connected: %s", path)
	return &WaylandWindow{conn: conn, nextID: 2}
}

// ─── Отправка запросов ───────────────────────────────────────────────────────

// wlMsg — конструктор исходящего сообщения.
type wlMsg struct {
	buf []byte
}

func newWlMsg(objectID uint32, opcode uint16) *wlMsg {
	m := &wlMsg{buf: make([]byte, 8, 32)}
	binary.LittleEndian.PutUint32(m.buf[0:4], objectID)
	// размер заполним при отправке; opcode — младшие 16 бит второго слова
	binary.LittleEndian.PutUint16(m.buf[4:6], opcode)
	return m
}

func (m *wlMsg) putUint(v uint32) *wlMsg {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	m.buf = append(m.buf, b[:]...)
	return m
}

func (m *wlMsg) putInt(v int32) *wlMsg { return m.putUint(uint32(v)) }

// putString — строка протокола: len (с NUL) + байты + NUL + паддинг до 4.
func (m *wlMsg) putString(s string) *wlMsg {
	n := len(s) + 1
	m.putUint(uint32(n))
	m.buf = append(m.buf, s...)
	m.buf = append(m.buf, 0)
	for len(m.buf)%4 != 0 {
		m.buf = append(m.buf, 0)
	}
	return m
}

// send пишет сообщение (под w.mu). oobFD >= 0 — передать fd через SCM_RIGHTS.
func (w *WaylandWindow) send(m *wlMsg, oobFD int) error {
	binary.LittleEndian.PutUint16(m.buf[6:8], uint16(len(m.buf)))
	w.mu.Lock()
	defer w.mu.Unlock()
	if oobFD >= 0 {
		oob := unix.UnixRights(oobFD)
		_, _, err := w.conn.WriteMsgUnix(m.buf, oob, nil)
		return err
	}
	_, err := w.conn.Write(m.buf)
	return err
}

// newID выделяет id для нового объекта.
func (w *WaylandWindow) newID() uint32 {
	w.mu.Lock()
	id := w.nextID
	w.nextID++
	w.mu.Unlock()
	return id
}

// ─── Приём событий ───────────────────────────────────────────────────────────

// readEvent читает одно событие: (objectID, opcode, тело). fd из ancillary
// складываются в w.rxFDs.
func (w *WaylandWindow) readEvent() (uint32, uint16, []byte, error) {
	hdr := make([]byte, 8)
	oob := make([]byte, 64)
	n, oobn, _, _, err := w.conn.ReadMsgUnix(hdr, oob)
	if err != nil {
		return 0, 0, nil, err
	}
	if oobn > 0 {
		w.collectFDs(oob[:oobn])
	}
	if n < 8 {
		// дочитываем заголовок
		for n < 8 {
			k, err := w.conn.Read(hdr[n:])
			if err != nil {
				return 0, 0, nil, err
			}
			n += k
		}
	}
	obj := binary.LittleEndian.Uint32(hdr[0:4])
	word := binary.LittleEndian.Uint32(hdr[4:8])
	opcode := uint16(word & 0xFFFF)
	size := int(word >> 16)
	body := make([]byte, size-8)
	got := 0
	for got < len(body) {
		k, err := w.conn.Read(body[got:])
		if err != nil {
			return 0, 0, nil, err
		}
		got += k
	}
	return obj, opcode, body, nil
}

// collectFDs разбирает SCM_RIGHTS и запоминает полученные дескрипторы.
func (w *WaylandWindow) collectFDs(oob []byte) {
	msgs, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return
	}
	for _, m := range msgs {
		fds, err := unix.ParseUnixRights(&m)
		if err != nil {
			continue
		}
		w.rxFDs = append(w.rxFDs, fds...)
	}
}

// takeFD забирает самый старый принятый fd (или -1).
func (w *WaylandWindow) takeFD() int {
	if len(w.rxFDs) == 0 {
		return -1
	}
	fd := w.rxFDs[0]
	w.rxFDs = w.rxFDs[1:]
	return fd
}

// wlString читает строку из тела события; возвращает строку и новый offset.
func wlString(b []byte, off int) (string, int) {
	n := int(binary.LittleEndian.Uint32(b[off : off+4]))
	off += 4
	s := ""
	if n > 0 {
		s = string(b[off : off+n-1]) // без NUL
	}
	off += (n + 3) &^ 3
	return s, off
}

// ─── NativeWindow: Create / RunEventLoop ────────────────────────────────────

func (w *WaylandWindow) Create(title string, width, height int) error {
	w.title = title
	w.width = width
	w.height = height

	// registry + начальный roundtrip: собираем глобалы.
	w.registryID = w.newID()
	if err := w.send(newWlMsg(wlDisplayID, wlDisplayGetRegistry).putUint(w.registryID), -1); err != nil {
		return fmt.Errorf("wayland: get_registry: %w", err)
	}
	if err := w.roundtrip(); err != nil {
		return fmt.Errorf("wayland: registry roundtrip: %w", err)
	}
	if w.gCompositor == 0 || w.gShm == 0 || w.gWmBase == 0 {
		return fmt.Errorf("wayland: композитор не предоставил compositor/shm/xdg_wm_base")
	}

	// bind глобалов (версия 1 достаточна для используемых запросов).
	w.compositorID = w.bind(w.gCompositor, "wl_compositor", 1)
	w.shmID = w.bind(w.gShm, "wl_shm", 1)
	w.wmBaseID = w.bind(w.gWmBase, "xdg_wm_base", 1)
	if w.gSeat != 0 {
		w.seatID = w.bind(w.gSeat, "wl_seat", 1)
	}

	// surface + xdg_surface + toplevel
	w.surfaceID = w.newID()
	w.send(newWlMsg(w.compositorID, wlCompositorCreateSurface).putUint(w.surfaceID), -1)
	w.xdgSurfaceID = w.newID()
	w.send(newWlMsg(w.wmBaseID, xdgWmBaseGetXdgSurface).putUint(w.xdgSurfaceID).putUint(w.surfaceID), -1)
	w.toplevelID = w.newID()
	w.send(newWlMsg(w.xdgSurfaceID, xdgSurfaceGetToplevel).putUint(w.toplevelID), -1)
	w.send(newWlMsg(w.toplevelID, xdgToplevelSetTitle).putString(title), -1)
	w.send(newWlMsg(w.toplevelID, xdgToplevelSetAppID).putString("headless-gui"), -1)
	// фиксируем размер: движок сам управляет разрешением
	w.send(newWlMsg(w.toplevelID, xdgToplevelSetMinSize).putInt(int32(width)).putInt(int32(height)), -1)
	w.send(newWlMsg(w.toplevelID, xdgToplevelSetMaxSize).putInt(int32(width)).putInt(int32(height)), -1)
	w.send(newWlMsg(w.surfaceID, wlSurfaceCommit), -1)

	// Ждём первый configure (обязателен до attach).
	wlLog("жду первый configure…")
	if err := w.roundtrip(); err != nil {
		return fmt.Errorf("wayland: configure roundtrip: %w", err)
	}
	if !w.configured {
		// дочитываем события до configure
		for !w.configured && !w.closed {
			if err := w.dispatchOne(); err != nil {
				return fmt.Errorf("wayland: ожидание configure: %w", err)
			}
		}
	}
	wlLog("configured; создаю shm-пул %dx%d", width, height)

	// shm-пул на два буфера
	if err := w.setupPool(width, height); err != nil {
		return err
	}

	// Первый кадр: чёрный буфер, чтобы окно появилось сразу.
	w.attachAndCommit(image.Rect(0, 0, width, height))
	wlLog("первый буфер закоммичен")
	return nil
}

// bind отправляет wl_registry.bind и возвращает id нового объекта.
func (w *WaylandWindow) bind(name uint32, iface string, version uint32) uint32 {
	id := w.newID()
	m := newWlMsg(w.registryID, wlRegistryBind).
		putUint(name).
		putString(iface).
		putUint(version).
		putUint(id)
	w.send(m, -1)
	return id
}

// roundtrip — wl_display.sync: гарантирует обработку всех предыдущих запросов.
func (w *WaylandWindow) roundtrip() error {
	cbID := w.newID()
	if err := w.send(newWlMsg(wlDisplayID, wlDisplaySync).putUint(cbID), -1); err != nil {
		return err
	}
	for {
		obj, opcode, body, err := w.readEvent()
		if err != nil {
			return err
		}
		if obj == cbID { // wl_callback.done
			return nil
		}
		w.handleEvent(obj, opcode, body)
	}
}

// dispatchOne читает и обрабатывает одно событие.
func (w *WaylandWindow) dispatchOne() error {
	obj, opcode, body, err := w.readEvent()
	if err != nil {
		return err
	}
	w.handleEvent(obj, opcode, body)
	return nil
}

func (w *WaylandWindow) RunEventLoop() error {
	for !w.closed {
		if err := w.dispatchOne(); err != nil {
			if w.closed {
				return nil
			}
			return err
		}
	}
	return nil
}

// handleEvent — диспетчер входящих событий по объекту/опкоду.
func (w *WaylandWindow) handleEvent(obj uint32, opcode uint16, b []byte) {
	if wlDebug {
		wlLog("event obj=%d opcode=%d len=%d (reg=%d wm=%d xsurf=%d top=%d seat=%d ptr=%d kbd=%d)",
			obj, opcode, len(b), w.registryID, w.wmBaseID, w.xdgSurfaceID, w.toplevelID, w.seatID, w.pointerID, w.keyboardID)
	}
	switch {
	case obj == wlDisplayID && opcode == wlDisplayEvError:
		// object, code, message — фатальная ошибка протокола
		_, off := binary.LittleEndian.Uint32(b[0:4]), 8
		msg, _ := wlString(b, off)
		fmt.Fprintf(os.Stderr, "wayland: protocol error: %s\n", msg)
		w.closed = true

	case obj == wlDisplayID && opcode == wlDisplayEvDeleteID:
		// подтверждение удаления объекта — освобождать нечего (id не переиспользуем)

	case obj == w.registryID && opcode == wlRegistryEvGlobal:
		name := binary.LittleEndian.Uint32(b[0:4])
		iface, _ := wlString(b, 4)
		switch iface {
		case "wl_compositor":
			w.gCompositor = name
		case "wl_shm":
			w.gShm = name
		case "wl_seat":
			w.gSeat = name
		case "xdg_wm_base":
			w.gWmBase = name
		}

	case obj == w.wmBaseID && opcode == xdgWmBaseEvPing:
		serial := binary.LittleEndian.Uint32(b[0:4])
		w.send(newWlMsg(w.wmBaseID, xdgWmBasePong).putUint(serial), -1)

	case obj == w.xdgSurfaceID && opcode == xdgSurfaceEvConfigure:
		serial := binary.LittleEndian.Uint32(b[0:4])
		w.send(newWlMsg(w.xdgSurfaceID, xdgSurfaceAckConfigure).putUint(serial), -1)
		w.configured = true
		// Состояние из configure применяется композитором на СЛЕДУЮЩЕМ
		// commit после ack. Движок on-demand может молчать (UI статичен) —
		// рекоммитим последний кадр сами, иначе окно не замаппится.
		w.recommitLast()

	case obj == w.toplevelID && opcode == xdgToplevelEvConfigure:
		nw := int(int32(binary.LittleEndian.Uint32(b[0:4])))
		nh := int(int32(binary.LittleEndian.Uint32(b[4:8])))
		// states: array из uint32
		alen := int(binary.LittleEndian.Uint32(b[8:12]))
		active := false
		for i := 0; i+4 <= alen; i += 4 {
			if binary.LittleEndian.Uint32(b[12+i:16+i]) == xdgStateActivated {
				active = true
			}
		}
		if w.onActivate != nil {
			w.onActivate(active)
		}
		if nw > 0 && nh > 0 && (nw != w.width || nh != w.height) {
			w.width, w.height = nw, nh
			if w.onResize != nil {
				w.onResize(nw, nh)
			}
		}

	case obj == w.toplevelID && opcode == xdgToplevelEvClose:
		if w.onClose == nil || w.onClose() {
			w.closed = true
		}

	case obj == w.seatID && opcode == wlSeatEvCapabilities:
		caps := binary.LittleEndian.Uint32(b[0:4])
		if caps&seatCapPointer != 0 && w.pointerID == 0 {
			w.pointerID = w.newID()
			w.send(newWlMsg(w.seatID, wlSeatGetPointer).putUint(w.pointerID), -1)
		}
		if caps&seatCapKeyboard != 0 && w.keyboardID == 0 {
			w.keyboardID = w.newID()
			w.send(newWlMsg(w.seatID, wlSeatGetKeyboard).putUint(w.keyboardID), -1)
		}

	case obj == w.pointerID:
		w.handlePointer(opcode, b)

	case obj == w.keyboardID:
		w.handleKeyboard(opcode, b)

	case (obj == w.bufID[0] || obj == w.bufID[1]) && opcode == wlBufferEvRelease:
		w.mu.Lock()
		if obj == w.bufID[0] {
			w.bufBusy[0] = false
		} else {
			w.bufBusy[1] = false
		}
		w.mu.Unlock()
	}
}

// handlePointer — события мыши. Координаты enter/motion — fixed 24.8.
func (w *WaylandWindow) handlePointer(opcode uint16, b []byte) {
	switch opcode {
	case wlPointerEvEnter:
		// serial, surface, x(fixed), y(fixed)
		w.ptrX = int(int32(binary.LittleEndian.Uint32(b[8:12]))) >> 8
		w.ptrY = int(int32(binary.LittleEndian.Uint32(b[12:16]))) >> 8
	case wlPointerEvMotion:
		// time, x(fixed), y(fixed)
		w.ptrX = int(int32(binary.LittleEndian.Uint32(b[4:8]))) >> 8
		w.ptrY = int(int32(binary.LittleEndian.Uint32(b[8:12]))) >> 8
		if w.onMouseMove != nil {
			w.onMouseMove(w.ptrX, w.ptrY)
		}
	case wlPointerEvButton:
		// serial, time, button, state
		btn := binary.LittleEndian.Uint32(b[8:12])
		pressed := binary.LittleEndian.Uint32(b[12:16]) == 1
		id := -1
		switch btn {
		case btnLeft:
			id = 0
		case btnRight:
			id = 1
		case btnMiddle:
			id = 2
		}
		if id >= 0 && w.onMouseButton != nil {
			w.onMouseButton(w.ptrX, w.ptrY, id, pressed)
		}
	case wlPointerEvAxis:
		// time, axis, value(fixed): 0 = вертикаль; >0 — вниз
		axis := binary.LittleEndian.Uint32(b[4:8])
		val := int32(binary.LittleEndian.Uint32(b[8:12]))
		if axis == 0 && w.onMouseButton != nil {
			id := 3 // wheel up
			if val > 0 {
				id = 4 // wheel down
			}
			w.onMouseButton(w.ptrX, w.ptrY, id, true)
			w.onMouseButton(w.ptrX, w.ptrY, id, false)
		}
	}
}

// handleKeyboard — клавиатура: keymap (fd закрываем — xkb не парсим),
// key: linux evdev; та же таблица, что X11 (keycode = evdev + 8).
func (w *WaylandWindow) handleKeyboard(opcode uint16, b []byte) {
	switch opcode {
	case wlKeyboardEvKeymap:
		if fd := w.takeFD(); fd >= 0 {
			unix.Close(fd)
		}
	case wlKeyboardEvKey:
		// serial, time, key, state
		key := int(binary.LittleEndian.Uint32(b[8:12]))
		pressed := binary.LittleEndian.Uint32(b[12:16]) == 1
		vk := x11KeycodeToVK(key + 8)
		if vk == 0 {
			return
		}
		if pressed {
			if w.onKeyDown != nil {
				w.onKeyDown(vk)
			}
			// Упрощённый символьный ввод (как в X11-бэкенде): без xkb-раскладки.
			if w.onChar != nil {
				if r := x11KeycodeToRune(key+8, false); r != 0 {
					w.onChar(r)
				}
			}
		} else if w.onKeyUp != nil {
			w.onKeyUp(vk)
		}
	}
}

// ─── SHM-пул и блит ─────────────────────────────────────────────────────────

// setupPool создаёт memfd-пул на два буфера width×height (XRGB8888).
func (w *WaylandWindow) setupPool(width, height int) error {
	stride := width * 4
	size := stride * height * 2 // два кадровых буфера

	fd, err := unix.MemfdCreate("headless-gui-shm", unix.MFD_CLOEXEC)
	if err != nil {
		return fmt.Errorf("wayland: memfd_create: %w", err)
	}
	if err := unix.Ftruncate(fd, int64(size)); err != nil {
		unix.Close(fd)
		return fmt.Errorf("wayland: ftruncate: %w", err)
	}
	data, err := unix.Mmap(fd, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		unix.Close(fd)
		return fmt.Errorf("wayland: mmap: %w", err)
	}

	w.shmFD, w.shmData = fd, data
	w.stride, w.poolW, w.poolH = stride, width, height

	w.poolID = w.newID()
	w.send(newWlMsg(w.shmID, wlShmCreatePool).putUint(w.poolID).putInt(int32(size)), fd)
	for i := 0; i < 2; i++ {
		w.bufID[i] = w.newID()
		w.send(newWlMsg(w.poolID, wlShmPoolCreateBuffer).
			putUint(w.bufID[i]).
			putInt(int32(i*stride*height)).
			putInt(int32(width)).putInt(int32(height)).
			putInt(int32(stride)).
			putUint(wlShmFormatXRGB8888), -1)
	}
	return nil
}

// recommitLast повторно коммитит последний закоммиченный буфер (полный
// damage) — применяет состояние после ack_configure без нового кадра движка.
func (w *WaylandWindow) recommitLast() {
	w.mu.Lock()
	ready := w.poolID != 0 && w.hasFrame
	last := w.curBuf ^ 1 // последний отправленный буфер
	w.mu.Unlock()
	if !ready {
		return
	}
	wlLog("recommit последнего кадра (buf=%d)", last)
	w.send(newWlMsg(w.surfaceID, wlSurfaceAttach).putUint(w.bufID[last]).putInt(0).putInt(0), -1)
	w.send(newWlMsg(w.surfaceID, wlSurfaceDamage).
		putInt(0).putInt(0).putInt(int32(w.poolW)).putInt(int32(w.poolH)), -1)
	w.send(newWlMsg(w.surfaceID, wlSurfaceCommit), -1)
}

// attachAndCommit прикрепляет текущий буфер и коммитит damage-область.
func (w *WaylandWindow) attachAndCommit(dirty image.Rectangle) {
	w.send(newWlMsg(w.surfaceID, wlSurfaceAttach).putUint(w.bufID[w.curBuf]).putInt(0).putInt(0), -1)
	w.send(newWlMsg(w.surfaceID, wlSurfaceDamage).
		putInt(int32(dirty.Min.X)).putInt(int32(dirty.Min.Y)).
		putInt(int32(dirty.Dx())).putInt(int32(dirty.Dy())), -1)
	w.send(newWlMsg(w.surfaceID, wlSurfaceCommit), -1)
	w.mu.Lock()
	w.bufBusy[w.curBuf] = true
	w.curBuf ^= 1
	w.hasFrame = true
	w.mu.Unlock()
}

func (w *WaylandWindow) BlitRGBA(img *image.RGBA) {
	if img == nil {
		return
	}
	w.BlitRGBADirty(img, img.Bounds())
}

// BlitRGBADirty пишет кадр в свободный shm-буфер (BGRX) и коммитит.
// Из-за чередования буферов копируется весь кадр, damage — только dirty.
func (w *WaylandWindow) BlitRGBADirty(img *image.RGBA, dirty image.Rectangle) {
	if w.surfaceID == 0 || img == nil || w.closed {
		return
	}
	b := img.Bounds()
	width, height := b.Dx(), b.Dy()
	if width > w.poolW || height > w.poolH {
		width, height = min(width, w.poolW), min(height, w.poolH)
	}
	dirty = dirty.Intersect(image.Rect(0, 0, width, height))
	if dirty.Empty() {
		return
	}

	base := w.curBuf * w.stride * w.poolH
	dst := w.shmData[base:]
	src := img.Pix
	stride := img.Stride
	for y := 0; y < height; y++ {
		so := y * stride
		do := y * w.stride
		for x := 0; x < width; x++ {
			si, di := so+x*4, do+x*4
			dst[di+0] = src[si+2] // B
			dst[di+1] = src[si+1] // G
			dst[di+2] = src[si+0] // R
			dst[di+3] = 0xFF      // X
		}
	}
	w.attachAndCommit(dirty)
}

// ─── Управление окном ────────────────────────────────────────────────────────

func (w *WaylandWindow) Close() {
	w.closed = true
	if w.conn != nil {
		w.conn.Close()
	}
	if w.shmData != nil {
		unix.Munmap(w.shmData)
		w.shmData = nil
	}
	if w.shmFD > 0 {
		unix.Close(w.shmFD)
		w.shmFD = 0
	}
}

func (w *WaylandWindow) SetTitle(title string) {
	w.title = title
	if w.toplevelID != 0 {
		w.send(newWlMsg(w.toplevelID, xdgToplevelSetTitle).putString(title), -1)
	}
}

func (w *WaylandWindow) SetSize(width, height int)  { w.width, w.height = width, height }
func (w *WaylandWindow) GetSize() (int, int)        { return w.width, w.height }
func (w *WaylandWindow) SetPosition(x, y int)       {} // Wayland: позицией владеет композитор
func (w *WaylandWindow) GetPosition() (int, int)    { return 0, 0 }
func (w *WaylandWindow) Minimize()                  {} // set_minimized — при необходимости
func (w *WaylandWindow) Maximize()                  {}
func (w *WaylandWindow) Restore()                   {}
func (w *WaylandWindow) IsMaximized() bool          { return false }
func (w *WaylandWindow) SetCornerRadius(int)        {}

// Callbacks
func (w *WaylandWindow) SetOnResize(fn func(w, h int))                           { w.onResize = fn }
func (w *WaylandWindow) SetOnClose(fn func() bool)                               { w.onClose = fn }
func (w *WaylandWindow) SetOnMouseMove(fn func(x, y int))                        { w.onMouseMove = fn }
func (w *WaylandWindow) SetOnMouseButton(fn func(x, y, button int, pressed bool)) { w.onMouseButton = fn }
func (w *WaylandWindow) SetOnKeyDown(fn func(vk int))                            { w.onKeyDown = fn }
func (w *WaylandWindow) SetOnKeyUp(fn func(vk int))                              { w.onKeyUp = fn }
func (w *WaylandWindow) SetOnChar(fn func(r rune))                               { w.onChar = fn }

// SetOnActivate — активность окна (state activated в configure).
func (w *WaylandWindow) SetOnActivate(fn func(active bool)) { w.onActivate = fn }
