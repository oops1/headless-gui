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
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"

	"github.com/oops1/headless-gui/v3/widget"
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
	wlShmPoolDestroy      = 1
	wlBufferDestroy       = 0
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

	// wlAxisPixelScale — множитель перевода wl_pointer.axis (уже в пикселях
	// после ÷256) к шагу движка (~40 px/notch; дискретное колесо даёт ~10/notch).
	wlAxisPixelScale = 4.0

	// wl_keyboard events
	wlKeyboardEvKeymap     = 0
	wlKeyboardEvEnter      = 1
	wlKeyboardEvLeave      = 2
	wlKeyboardEvKey        = 3
	wlKeyboardEvModifiers  = 4
	wlKeyboardEvRepeatInfo = 5 // версия 4 протокола

	// wlSeatVersion — версия wl_seat, с которой композитор сообщает частоту
	// автоповтора (wl_keyboard.repeat_info). Выше не берём: следующие версии
	// обязывают принимать события wl_pointer.frame и прочую группировку,
	// которой бэкенд не занимается.
	wlSeatVersion = 4

	xkbModShift = 1 // фиксированные маски реальных модификаторов xkb
	xkbModLock  = 2

	// xdg_wm_base
	xdgWmBaseGetXdgSurface = 2
	xdgWmBasePong          = 3
	xdgWmBaseEvPing        = 0

	// xdg_surface
	xdgSurfaceGetToplevel  = 1
	xdgSurfaceAckConfigure = 4
	xdgSurfaceEvConfigure  = 0

	// xdg_toplevel — опкоды запросов в waylandwire.go (их проверяют тесты),
	// здесь только события.
	xdgToplevelEvConfigure = 0
	xdgToplevelEvClose     = 1

	wlShmFormatXRGB8888 = 1

	// linux input-event-codes
	btnLeft   = 0x110
	btnRight  = 0x111
	btnMiddle = 0x112

	// wl_data_device_manager
	wlDataDevMgrGetDataDevice = 1

	// wl_data_device requests / events
	wlDataDeviceRelease   = 2
	wlDataDeviceEvDataOffer = 0
	wlDataDeviceEvEnter     = 1
	wlDataDeviceEvLeave     = 2
	wlDataDeviceEvMotion    = 3
	wlDataDeviceEvDrop      = 4
	wlDataDeviceEvSelection = 5

	// wl_data_offer requests / events
	wlDataOfferAccept     = 0
	wlDataOfferReceive    = 1
	wlDataOfferDestroy    = 2
	wlDataOfferFinish     = 3
	wlDataOfferSetActions = 4
	wlDataOfferEvOffer    = 0

	// wl_data_device_manager.dnd_action
	wlDndActionCopy = 1
)

// wlBufWaitTimeout — сколько ждать wl_buffer.release перед пропуском кадра.
const wlBufWaitTimeout = 32 * time.Millisecond

// ─── WaylandWindow ───────────────────────────────────────────────────────────

// WaylandWindow — реализация NativeWindow поверх Wayland (xdg-shell + wl_shm).
type WaylandWindow struct {
	// linuxNotifier — системные уведомления через D-Bus (notify_linux.go):
	// путь общий с X11, композитора не касается.
	linuxNotifier

	// linuxTray — иконка в системном трее по StatusNotifierItem
	// (tray_sni_linux.go): тоже чистый D-Bus, композитора не касается.
	linuxTray

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
	gSeatVer                          uint32 // версия wl_seat (repeat_info — с 4-й)
	gCompositorVer                    uint32 // версия wl_compositor (damage_buffer — с 4-й)

	// HiDPI (wayland_scale_linux.go): масштаб поверхности и объекты, через
	// которые о нём договариваются с компоновщиком.
	scale        wlScale
	gViewporter  uint32
	gFracMgr     uint32
	gOutputs     map[uint32]uint32 // имя глобала wl_output → его версия
	viewporterID uint32
	fracMgrID    uint32

	// repeat — автоповтор удерживаемой клавиши (wayland_repeat_linux.go).
	repeat *wlRepeater

	// clip — буфер обмена через wl_data_device (wayland_clipboard_linux.go).
	clip *wlClipboard
	// inputSerial — serial последнего ввода (клавиша, кнопка, вход
	// указателя). Компоновщик принимает set_selection только с ним:
	// доказательство, что действие начал пользователь.
	inputSerial atomic.Uint32
	// onKeyDownRepeat — приёмник нажатий, отличающий повтор от нового
	// нажатия; им пользуется window.surface, если бэкенд умеет (см.
	// keyRepeatSource).
	onKeyDownRepeat func(vk int, repeat bool)

	// wl_data_device_manager (Drag&Drop файлов из ОС).
	gDataDevMgr    uint32 // имя глобала registry
	gDataDevMgrVer uint32 // версия глобала (для finish/set_actions нужна v≥3)
	dataDevMgrID   uint32 // объект после bind
	dataDeviceID   uint32 // объект wl_data_device для seat
	dataDevVersion uint32 // фактическая версия bind (min(advertised,3))

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

	// bufRelease будит блит при wl_buffer.release; dirtyTrack сводит области.
	bufRelease chan struct{}
	dirtyTrack wlDirtyTracker

	width, height int
	title         string
	closed        bool
	configured    bool
	hasFrame      bool // был ли закоммичен хотя бы один кадр
	pendingSerial uint32

	// координаты указателя (motion приходит в fixed 24.8)
	ptrX, ptrY int

	// На какой поверхности указатель: события motion/button/axis приходят
	// без неё, её сообщает только enter. Пока курсор над попапом, ввод
	// принадлежит ему, а не окну. Читает и пишет цикл событий.
	ptrOnPopup bool
	ptrPopupID uintptr

	// Serial'ы указателя. Компоновщик принимает move/resize/set_cursor
	// только с serial'ом недавнего ввода: это доказательство, что действие
	// начал пользователь, а не программа сама себе.
	//
	// Пишет их цикл событий, читает горутина движка (кнопка заголовка, край
	// окна) — отсюда atomic, а не голые поля.
	ptrEnterSerial  atomic.Uint32 // последний wl_pointer.enter — для set_cursor
	ptrButtonSerial atomic.Uint32 // последнее НАЖАТИЕ — для move/resize
	ptrButtons      atomic.Uint32 // маска зажатых кнопок (биты по id движка)

	// Состояния из последнего xdg_toplevel.configure: пишет цикл событий,
	// читает движок (кнопка «развернуть» спрашивает IsMaximized).
	maximized  atomic.Bool
	fullscreen atomic.Bool

	// pendingResize — пришёл новый размер, кадра под него ещё нет. Пока он
	// взведён, ack_configure не сопровождается рекоммитом: старый кадр в
	// новом окне композитор растянул бы, и ресайз шёл бы рывками.
	pendingResize bool

	// cornerR — скругление углов окна в логических точках (SetCornerRadius);
	// не ноль — буфер ARGB с прозрачными углами (cornermask.go).
	cornerR atomic.Int32
	// poolARGB — пул создан в формате ARGB8888. Пишется в setupPool.
	poolARGB bool

	// staleSkipped — сколько кадров прежнего размера не показано, пока ответ
	// на configure с размером был в пути (skipStaleFrame). Счёт на всю жизнь
	// окна: пропуск — мера против окна неверного размера, а не ожидание.
	staleSkipped int

	// resizable — разрешён ли пользователю ресайз. Create фиксирует размер
	// (min == max), SetResizable снимает фиксацию: под Wayland это
	// единственный способ запретить или разрешить растягивание окна.
	resizable atomic.Bool

	// appID — идентификатор для среды рабочего стола (xdg_toplevel.app_id).
	// Пусто до Create — подставится имя исполняемого файла.
	appID string

	// textInput — редактор метода ввода (wayland_ime_linux.go). Расширение
	// необязательное: компоновщик вправе его не предлагать.
	textInput      wlTextInput
	gTextInputMgr  uint32
	textInputMgrID uint32

	// popups — всплывающие окна (wayland_popup_linux.go): меню и списки,
	// которым не хватает места в окне. На Wayland они обязаны быть
	// поверхностями этого же соединения — отдельным окном попап там быть
	// не может.
	popups wlPopups

	// Курсор (wayland_cursor_linux.go): текущая форма, буферы форм и
	// поверхность, которую компоновщик показывает вместо курсора. Форму
	// задаёт движок, применяет — и движок, и цикл событий (на enter),
	// поэтому всё под cursorMu.
	cursorMu         sync.Mutex
	cursorShape      int
	cursorSet        bool
	cursorSurfID     uint32
	cursorPoolID     uint32
	cursorFD         int
	cursorData       []byte
	cursorSlots      map[int]wlCursorSlot
	gCursorShapeMgr  uint32 // имя глобала wp_cursor_shape_manager_v1
	cursorShapeMgrID uint32
	cursorShapeDevID uint32

	rxFDs []int // fd, принятые через SCM_RIGHTS (keymap и т.п.)

	// Клавиатура: распарсенный xkb-keymap и состояние модификаторов
	// (событие modifiers несёт и группу — активную раскладку).
	keymap   *xkbKeymap
	modShift bool
	modCaps  bool
	kbGroup  int

	// minW/minH/minWant — минимальный размер, заданный до создания toplevel
	// (см. SetMinSize): Create фиксирует размер, и отложенный минимум
	// применяется после фиксации, иначе она его затрёт.
	minW, minH int
	minWant    bool

	// Callbacks (интерфейс NativeWindow)
	onResize      func(w, h int)
	onClose       func() bool
	onMouseMove   func(x, y int)
	onMouseButton func(x, y, button int, pressed bool)
	onMouseWheelPixels func(x, y int, dx, dy float64)
	onKeyDown     func(vk int)
	onKeyUp       func(vk int)
	onChar        func(r rune)
	onActivate    func(active bool)
	onFilesDropped func(paths []string, x, y int)

	// ── Состояние Drag&Drop (wl_data_device) ────────────────────────────────
	// offers — известные data_offer'ы и предложен ли в них text/uri-list.
	offers map[uint32]bool
	// textOffers — предложения, в которых есть простой текст (буфер обмена).
	textOffers map[uint32]bool
	// htmlOffers — предложения с HTML: offer → объявленный тип (его же и
	// запрашиваем).
	htmlOffers map[uint32]string
	// gnomeOffers — предложения со списком файлов x-special/gnome-copied-files
	// (там же пометка cut); text/uri-list отмечен в offers.
	gnomeOffers map[uint32]bool
	// dndOffer — активный offer текущего перетаскивания (между enter и drop).
	dndOffer   uint32
	dndSerial  uint32 // serial из enter (для accept)
	dndX, dndY int    // позиция курсора в поверхностных пикселях
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
	w := &WaylandWindow{
		conn:       conn,
		nextID:     2,
		bufRelease: make(chan struct{}, 1),
		repeat:     newWlRepeater(),
	}
	w.clip = newWlClipboard(w)
	return w
}

// ─── Отправка запросов ───────────────────────────────────────────────────────

// send пишет сообщение (под w.mu). oobFD >= 0 — передать fd через SCM_RIGHTS.
func (w *WaylandWindow) send(m *wlMsg, oobFD int) error {
	m.bytes() // проставить длину в заголовке
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

// sendRaw отправляет готовое сообщение (длина уже проставлена).
func (w *WaylandWindow) sendRaw(raw []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := w.conn.Write(raw)
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

	// bind глобалов (версия 1 достаточна для используемых запросов, кроме
	// wl_compositor: с четвёртой версии у поверхности есть damage_buffer —
	// повреждение в пикселях буфера, без которого на дробном масштабе
	// область уезжает).
	compVer := uint32(1)
	if w.gCompositorVer >= wlCompositorVersionBufferDamage {
		compVer = wlCompositorVersionBufferDamage
	}
	w.compositorID = w.bind(w.gCompositor, "wl_compositor", compVer)
	w.shmID = w.bind(w.gShm, "wl_shm", 1)
	w.wmBaseID = w.bind(w.gWmBase, "xdg_wm_base", 1)
	if w.gSeat != 0 {
		ver := w.gSeatVer
		if ver > wlSeatVersion {
			ver = wlSeatVersion
		}
		if ver < 1 {
			ver = 1
		}
		w.seatID = w.bind(w.gSeat, "wl_seat", ver)
	}
	// wl_data_device_manager: Drag&Drop файлов. Версия ≥3 нужна для
	// finish/set_actions; берём min(advertised, 3).
	if w.gDataDevMgr != 0 && w.gSeat != 0 {
		ver := w.gDataDevMgrVer
		if ver > 3 {
			ver = 3
		}
		w.dataDevVersion = ver
		w.dataDevMgrID = w.bind(w.gDataDevMgr, "wl_data_device_manager", ver)
		// get_data_device(new_id, seat)
		w.dataDeviceID = w.newID()
		w.send(newWlMsg(w.dataDevMgrID, wlDataDevMgrGetDataDevice).
			putUint(w.dataDeviceID).putUint(w.seatID), -1)
	}

	// Мониторы и расширения масштаба: без них окно рисуется один к одному,
	// как было до HiDPI.
	for name, ver := range w.gOutputs {
		v := ver
		if v > wlOutputVersionScale {
			v = wlOutputVersionScale
		}
		id := w.bind(name, "wl_output", v)
		w.scale.mu.Lock()
		if w.scale.outputs == nil {
			w.scale.outputs = map[uint32]float64{}
		}
		w.scale.outputs[id] = 0 // масштаб придёт событием scale
		w.scale.mu.Unlock()
	}
	if w.gViewporter != 0 {
		w.viewporterID = w.bind(w.gViewporter, "wp_viewporter", 1)
	}
	if w.gFracMgr != 0 {
		w.fracMgrID = w.bind(w.gFracMgr, "wp_fractional_scale_manager_v1", 1)
	}
	if w.gTextInputMgr != 0 && w.gSeat != 0 {
		w.textInputMgrID = w.bind(w.gTextInputMgr, "zwp_text_input_manager_v3", 1)
		w.setupTextInput()
	}

	// surface + xdg_surface + toplevel
	w.surfaceID = w.newID()
	w.send(newWlMsg(w.compositorID, wlCompositorCreateSurface).putUint(w.surfaceID), -1)
	w.setupScaleObjects()
	w.xdgSurfaceID = w.newID()
	w.send(newWlMsg(w.wmBaseID, xdgWmBaseGetXdgSurface).putUint(w.xdgSurfaceID).putUint(w.surfaceID), -1)
	w.toplevelID = w.newID()
	w.send(newWlMsg(w.xdgSurfaceID, xdgSurfaceGetToplevel).putUint(w.toplevelID), -1)
	w.send(newWlMsg(w.toplevelID, xdgToplevelSetTitle).putString(title), -1)
	w.send(newWlMsg(w.toplevelID, xdgToplevelSetAppID).putString(w.effectiveAppID()), -1)
	// фиксируем размер: движок сам управляет разрешением (в поверхностных
	// единицах — их и ждёт xdg-shell)
	sw, sh := w.toSurface(width), w.toSurface(height)
	w.send(newWlMsg(w.toplevelID, xdgToplevelSetMinSize).putInt(int32(sw)).putInt(int32(sh)), -1)
	w.send(newWlMsg(w.toplevelID, xdgToplevelSetMaxSize).putInt(int32(sw)).putInt(int32(sh)), -1)
	// Минимум, заданный приложением до Create, — ПОСЛЕ фиксации: иначе строка
	// выше затёрла бы его размером окна.
	if w.minWant {
		w.minWant = false
		mw, mh := wlMinSizeArgs(w.minW, w.minH, w.scaleFactor())
		w.send(newWlMsg(w.toplevelID, xdgToplevelSetMinSize).putInt(mw).putInt(mh), -1)
	}
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

	// Буфер обмена: в сессии Wayland внешних утилит может не быть вовсе, а
	// wl_data_device — единственный путь обмена текстом с остальными
	// программами. Регистрируем, только если компоновщик его дал.
	if w.dataDeviceID != 0 {
		widget.SetClipboardProvider(w.clip)
	}
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
// StartEventPump обслуживает окно, которое не крутит общий цикл событий:
// второе окно верхнего уровня (multiwindow.go) или оторванную панель.
// Реализует eventPumper.
//
// У каждого окна Wayland своё соединение с компоновщиком, и события с него
// никто, кроме этой горутины, не читает. Цикл заканчивается сам, когда окно
// закрывают: Close рвёт соединение, и чтение возвращает ошибку.
func (w *WaylandWindow) StartEventPump() {
	if w.conn == nil {
		return
	}
	go w.RunEventLoop()
}

func (w *WaylandWindow) handleEvent(obj uint32, opcode uint16, b []byte) {
	if wlDebug {
		wlLog("event obj=%d opcode=%d len=%d (reg=%d wm=%d xsurf=%d top=%d seat=%d ptr=%d kbd=%d)",
			obj, opcode, len(b), w.registryID, w.wmBaseID, w.xdgSurfaceID, w.toplevelID, w.seatID, w.pointerID, w.keyboardID)
	}
	// Объекты попапов (их несколько и они недолговечны) разбираются
	// отдельной таблицей — перечислять их в общем switch нечем.
	if w.popupEvent(obj, opcode, b) {
		return
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
		iface, off := wlString(b, 4)
		version := binary.LittleEndian.Uint32(b[off : off+4])
		switch iface {
		case "wl_compositor":
			w.gCompositor = name
			w.gCompositorVer = version
		case "wl_shm":
			w.gShm = name
		case "wl_seat":
			w.gSeat = name
			w.gSeatVer = version
		case "xdg_wm_base":
			w.gWmBase = name
		case "wl_data_device_manager":
			w.gDataDevMgr = name
			w.gDataDevMgrVer = version
		case "wl_output":
			// Мониторов может быть несколько; какой из них показывает окно,
			// скажет wl_surface.enter.
			if w.gOutputs == nil {
				w.gOutputs = map[uint32]uint32{}
			}
			w.gOutputs[name] = version
		case "zwp_text_input_manager_v3":
			w.gTextInputMgr = name
		case "wp_viewporter":
			w.gViewporter = name
		case "wp_fractional_scale_manager_v1":
			w.gFracMgr = name
		case "wp_cursor_shape_manager_v1":
			// Необязательное расширение: с ним курсор рисует компоновщик
			// из системной темы, без него — своя картинка.
			w.gCursorShapeMgr = name
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
		// рекоммитим последний кадр сами, иначе окно не замаппится. Кадр под
		// новый размер уже в пути — его и ждём (см. pendingResize).
		if !w.pendingResize {
			w.recommitLast()
		}

	case obj == w.toplevelID && opcode == xdgToplevelEvConfigure:
		// Размер приходит в ПОВЕРХНОСТНЫХ единицах; движок считает в
		// пикселях буфера, и на HiDPI это разные числа.
		nw := w.fromSurface(int(int32(binary.LittleEndian.Uint32(b[0:4]))))
		nh := w.fromSurface(int(int32(binary.LittleEndian.Uint32(b[4:8]))))
		// states: array из uint32
		active, maximized, fullscreen := wlParseStates(b[8:])
		wlLog("toplevel.configure: %dx%d active=%v maximized=%v fullscreen=%v",
			nw, nh, active, maximized, fullscreen)
		w.maximized.Store(maximized)
		w.fullscreen.Store(fullscreen)
		if w.onActivate != nil {
			w.onActivate(active)
		}
		// 0×0 — «решай сам»: размер не трогаем (так приходит configure при
		// первом показе и при выходе из развёрнутого состояния).
		if nw > 0 && nh > 0 && (nw != w.width || nh != w.height) {
			// Под замком: размер читают и горутина кадров (skipStaleFrame),
			// и Run сразу после Create (clientSize).
			w.mu.Lock()
			w.width, w.height = nw, nh
			w.pendingResize = true
			w.mu.Unlock()
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

	case obj == w.surfaceID && opcode == wlSurfaceEvEnter:
		// Окно показалось на мониторе: его масштаб и есть масштаб окна.
		if len(b) >= 4 {
			w.noteSurfaceEnter(binary.LittleEndian.Uint32(b[0:4]))
		}

	case w.isOutputObject(obj) && opcode == wlOutputEvScale:
		if len(b) >= 4 {
			w.noteOutputScale(obj, float64(int32(binary.LittleEndian.Uint32(b[0:4]))))
		}

	case w.isTextInputObject(obj):
		w.handleTextInput(opcode, b)

	case w.isFracObject(obj) && opcode == wpFracEvPreferredScale:
		// Дробный масштаб приходит в 120-х долях: 180 — это 1.5.
		if len(b) >= 4 {
			w.notePreferredScale(binary.LittleEndian.Uint32(b[0:4]))
		}

	case obj == w.pointerID:
		w.handlePointer(opcode, b)

	case obj == w.keyboardID:
		w.handleKeyboard(opcode, b)

	case w.dataDeviceID != 0 && obj == w.dataDeviceID:
		w.handleDataDevice(opcode, b)

	case w.clip.isSource(obj):
		// События нашего источника буфера обмена: у нас просят данные
		// (send) или сообщают, что буфером завладел другой (cancelled).
		switch opcode {
		case wlDataSourceEvSend:
			mime, _ := wlString(b, 0)
			w.clip.handleSourceSend(obj, mime, w.takeFD())
		case wlDataSourceEvCancelled:
			w.clip.handleSourceCancelled(obj)
		}

	case w.isOfferObject(obj):
		// wl_data_offer.offer(mime): фиксируем, что предложено.
		if opcode == wlDataOfferEvOffer {
			mime, _ := wlString(b, 0)
			if mime == mimeTextUriList {
				w.offerSet(obj, true)
			}
			if isTextMime(mime) {
				w.offerSetText(obj)
			}
			if isHTMLMime(mime) {
				w.offerSetHTML(obj, mime)
			}
			if mime == mimeGnomeCopiedFiles {
				w.offerSetGnomeFiles(obj)
			}
		}

	case (obj == w.bufID[0] || obj == w.bufID[1]) && opcode == wlBufferEvRelease:
		w.mu.Lock()
		if obj == w.bufID[0] {
			w.bufBusy[0] = false
		} else {
			w.bufBusy[1] = false
		}
		ch := w.bufRelease
		w.mu.Unlock()
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// handlePointer — события мыши. Координаты enter/motion — fixed 24.8.
func (w *WaylandWindow) handlePointer(opcode uint16, b []byte) {
	switch opcode {
	case wlPointerEvEnter:
		// serial, surface, x(fixed), y(fixed)
		w.ptrEnterSerial.Store(binary.LittleEndian.Uint32(b[0:4]))
		w.inputSerial.Store(binary.LittleEndian.Uint32(b[0:4]))
		surf := binary.LittleEndian.Uint32(b[4:8])
		w.ptrOnPopup = false
		if pop := w.popupForSurface(surf); pop != nil {
			w.ptrOnPopup, w.ptrPopupID = true, pop.id
		}
		w.ptrX = int(int32(binary.LittleEndian.Uint32(b[8:12]))) >> 8
		w.ptrY = int(int32(binary.LittleEndian.Uint32(b[12:16]))) >> 8
		// Форму курсора ставим сразу: без set_cursor после enter курсор над
		// окном остаётся тем, каким его оставил сосед.
		w.applyCursor()
	case wlPointerEvLeave:
		onPopup := w.ptrOnPopup
		w.ptrOnPopup = false
		// Указатель ушёл — в том числе потому, что компоновщик забрал
		// нажатие себе (начались move/resize). Отпускания мы уже не
		// получим, поэтому отпускаем зажатые кнопки сами: иначе движок
		// навсегда остался бы с «нажатой» кнопкой и захватом мыши.
		w.releaseHeldButtons()
		// И снимаем наведение — движением за пределы той поверхности, с
		// которой ушли (pointerleave.go).
		if onPopup {
			if h := w.popupHandlers(); h.Move != nil {
				h.Move(w.ptrPopupID, pointerOutside, pointerOutside)
			}
		} else if w.onMouseMove != nil {
			w.onMouseMove(pointerOutside, pointerOutside)
		}
	case wlPointerEvMotion:
		// time, x(fixed), y(fixed)
		w.ptrX = int(int32(binary.LittleEndian.Uint32(b[4:8]))) >> 8
		w.ptrY = int(int32(binary.LittleEndian.Uint32(b[8:12]))) >> 8
		if w.ptrOnPopup {
			if h := w.popupHandlers(); h.Move != nil {
				h.Move(w.ptrPopupID, w.ptrX, w.ptrY)
			}
			return
		}
		if w.onMouseMove != nil {
			w.onMouseMove(w.ptrX, w.ptrY)
		}
	case wlPointerEvButton:
		// serial, time, button, state
		serial := binary.LittleEndian.Uint32(b[0:4])
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
		if pressed {
			// Serial нажатия — пропуск к xdg_toplevel.move/resize: им
			// клиент доказывает, что действие начал пользователь.
			w.ptrButtonSerial.Store(serial)
		}
		w.inputSerial.Store(serial)
		if id >= 0 {
			// Or/And у atomic.Uint32 появились в go1.23, а модуль держит
			// go1.22 — обходимся CAS-циклом.
			for {
				old := w.ptrButtons.Load()
				next := old | 1<<uint(id)
				if !pressed {
					next = old &^ (1 << uint(id))
				}
				if w.ptrButtons.CompareAndSwap(old, next) {
					break
				}
			}
			if w.ptrOnPopup {
				if h := w.popupHandlers(); h.Button != nil {
					h.Button(w.ptrPopupID, w.ptrX, w.ptrY, id, pressed)
				}
				return
			}
			if w.onMouseButton != nil {
				w.onMouseButton(w.ptrX, w.ptrY, id, pressed)
			}
		}
	case wlPointerEvAxis:
		// time, axis, value(wl_fixed 24.8): axis 0 = вертикаль, 1 = горизонталь;
		// value>0 — вниз/вправо. Тачпады высокой точности шлют дробные значения.
		axis := binary.LittleEndian.Uint32(b[4:8])
		val := int32(binary.LittleEndian.Uint32(b[8:12]))
		if w.ptrOnPopup {
			// Длинный список прокручивают колесом прямо в попапе.
			amt := float64(val) / 256.0 * wlAxisPixelScale
			if h := w.popupHandlers(); h.Wheel != nil {
				if axis == 0 {
					h.Wheel(w.ptrPopupID, w.ptrX, w.ptrY, 0, amt)
				} else if axis == 1 {
					h.Wheel(w.ptrPopupID, w.ptrX, w.ptrY, amt, 0)
				}
			}
			return
		}
		if w.onMouseWheelPixels != nil {
			// Высокоточный путь: wl_fixed → пиксели (÷256), масштаб до «notch»
			// в ~40 px под общий шаг движка.
			amt := float64(val) / 256.0 * wlAxisPixelScale
			if axis == 0 {
				w.onMouseWheelPixels(w.ptrX, w.ptrY, 0, amt)
			} else if axis == 1 {
				w.onMouseWheelPixels(w.ptrX, w.ptrY, amt, 0)
			}
		} else if axis == 0 && w.onMouseButton != nil {
			// Фолбэк на тики.
			id := 3 // wheel up
			if val > 0 {
				id = 4 // wheel down
			}
			w.onMouseButton(w.ptrX, w.ptrY, id, true)
			w.onMouseButton(w.ptrX, w.ptrY, id, false)
		}
	}
}

// handleKeyboard — клавиатура. keymap (fd, формат xkb_v1) парсится в
// таблицу «код → группы → уровни → руна» — полноценный ввод с раскладкой
// (кириллица и пр.) и живым переключением групп через событие modifiers.
func (w *WaylandWindow) handleKeyboard(opcode uint16, b []byte) {
	switch opcode {
	case wlKeyboardEvKeymap:
		// format(uint32), fd(ancillary), size(uint32)
		format := binary.LittleEndian.Uint32(b[0:4])
		size := int(binary.LittleEndian.Uint32(b[4:8]))
		fd := w.takeFD()
		if fd < 0 {
			return
		}
		defer unix.Close(fd)
		if format != 1 || size <= 0 { // 1 = xkb_v1 (текстовый)
			return
		}
		data, err := unix.Mmap(fd, 0, size, unix.PROT_READ, unix.MAP_PRIVATE)
		if err != nil {
			wlLog("keymap mmap: %v", err)
			return
		}
		defer unix.Munmap(data)
		if dump := os.Getenv("HEADLESS_GUI_WL_KEYMAP_DUMP"); dump != "" {
			os.WriteFile(dump, data, 0o644)
		}
		w.keymap = parseXkbKeymap(string(data))
		if w.keymap == nil {
			wlLog("keymap: разобрать не удалось — фолбэк на упрощённый ввод")
		} else {
			wlLog("keymap: %d клавиш", len(w.keymap.keys))
		}

	case wlKeyboardEvModifiers:
		// serial, depressed, latched, locked, group
		depressed := binary.LittleEndian.Uint32(b[4:8])
		latched := binary.LittleEndian.Uint32(b[8:12])
		locked := binary.LittleEndian.Uint32(b[12:16])
		w.modShift = (depressed|latched)&xkbModShift != 0
		w.modCaps = locked&xkbModLock != 0
		w.kbGroup = int(binary.LittleEndian.Uint32(b[16:20]))

	case wlKeyboardEvKey:
		// serial, time, key, state
		w.inputSerial.Store(binary.LittleEndian.Uint32(b[0:4]))
		key := uint32(binary.LittleEndian.Uint32(b[8:12]))
		pressed := binary.LittleEndian.Uint32(b[12:16]) == 1
		if !pressed {
			w.repeat.stopKey(key)
			if vk := w.vkForKey(key); vk != 0 && w.onKeyUp != nil {
				w.onKeyUp(vk)
			}
			return
		}
		w.deliverKey(key, false)
		// Повтор ведёт клиент: композитор между нажатием и отпусканием
		// молчит. Модификаторы не повторяем — повторять нечего.
		if vk := w.vkForKey(key); !isModifierVK(vk) {
			w.repeat.start(key, func() { w.deliverKey(key, true) })
		}

	case wlKeyboardEvLeave:
		// Фокус ушёл — отпускания мы уже не увидим, и повтор завис бы
		// навсегда.
		w.repeat.cancel()

	case wlKeyboardEvRepeatInfo:
		// rate (кл/сек), delay (мс). rate == 0 — композитор выключил повтор.
		rate := int32(binary.LittleEndian.Uint32(b[0:4]))
		delay := int32(binary.LittleEndian.Uint32(b[4:8]))
		w.repeat.setInfo(rate, delay)
		wlLog("repeat_info: rate=%d delay=%d", rate, delay)
	}
}

// vkForKey — виртуальный код клавиши по evdev-коду из события wl_keyboard.key.
// Сначала по физическому месту (x11KeycodeToVK), затем по keymap композитора:
// так находится клавиша Windows, переставленная на нестандартное место.
func (w *WaylandWindow) vkForKey(key uint32) int {
	if vk := x11KeycodeToVK(int(key) + 8); vk != 0 {
		return vk
	}
	return w.keymap.vkFor(key + 8)
}

// deliverKey отдаёт нажатие приложению: сначала код клавиши, затем символ.
//
// repeat=true — это автоповтор, а не новое нажатие: приложению важно знать
// разницу там, где нажатие что-то переключает.
func (w *WaylandWindow) deliverKey(key uint32, repeat bool) {
	if vk := w.vkForKey(key); vk != 0 {
		if w.onKeyDownRepeat != nil {
			w.onKeyDownRepeat(vk, repeat)
		} else if w.onKeyDown != nil {
			w.onKeyDown(vk)
		}
	}
	if w.onChar == nil {
		return
	}
	// Полноценный ввод по keymap (руна с учётом раскладки/Shift/Caps);
	// фолбэк — упрощённый маппинг, как раньше.
	if r := w.keymap.runeFor(key+8, w.kbGroup, w.modShift, w.modCaps); r >= 32 {
		w.onChar(r)
		return
	}
	if w.keymap == nil {
		if r := x11KeycodeToRune(int(key)+8, w.modShift); r != 0 {
			w.onChar(r)
		}
	}
}

// SetOnKeyDownRepeat подписывает приёмник нажатий, отличающий автоповтор от
// нового нажатия. Реализует keyRepeatSource.
func (w *WaylandWindow) SetOnKeyDownRepeat(fn func(vk int, repeat bool)) {
	w.onKeyDownRepeat = fn
}

// ─── SHM-пул и блит ─────────────────────────────────────────────────────────

// setupPool создаёт memfd-пул на два буфера width×height: XRGB8888, а у окна
// со скруглением — ARGB8888 (прозрачные углы, cornermask.go).
func (w *WaylandWindow) setupPool(width, height int) error {
	format := uint32(wlShmFormatXRGB8888)
	argb := w.wantARGB()
	if argb {
		format = wlShmFormatARGB8888
	}
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
	w.poolARGB = argb

	w.poolID = w.newID()
	w.send(newWlMsg(w.shmID, wlShmCreatePool).putUint(w.poolID).putInt(int32(size)), fd)
	for i := 0; i < 2; i++ {
		w.bufID[i] = w.newID()
		w.send(newWlMsg(w.poolID, wlShmPoolCreateBuffer).
			putUint(w.bufID[i]).
			putInt(int32(i*stride*height)).
			putInt(int32(width)).putInt(int32(height)).
			putInt(int32(stride)).
			putUint(format), -1)
	}
	return nil
}

// ensurePool держит кадровые буферы в размер окна.
//
// Компоновщик показывает буфер как есть: после ресайза кадр старого размера
// остаётся островом в новом окне. Поэтому при каждой смене размера пул
// пересоздаётся — так под Wayland поступают все клиенты.
func (w *WaylandWindow) ensurePool(width, height int) error {
	w.mu.Lock()
	same := w.poolID != 0 && w.poolW == width && w.poolH == height && w.poolARGB == w.wantARGB()
	w.mu.Unlock()
	if same {
		return nil
	}
	if width <= 0 || height <= 0 {
		return fmt.Errorf("wayland: размер буфера %dx%d", width, height)
	}
	w.destroyPool()
	return w.setupPool(width, height)
}

// destroyPool освобождает кадровые буферы и их память.
//
// Буферы уничтожаются ДО munmap: память пула, из которой композитор ещё
// читает, освобождать нельзя — сначала он должен узнать, что буферов больше
// нет. Ждать release при этом незачем: wl_buffer.destroy законен и для
// занятого буфера, содержимое композитор уже скопировал или скопирует по
// своим правилам.
func (w *WaylandWindow) destroyPool() {
	w.mu.Lock()
	poolID := w.poolID
	bufs := w.bufID
	data := w.shmData
	fd := w.shmFD
	w.poolID, w.bufID = 0, [2]uint32{}
	w.shmData, w.shmFD = nil, 0
	w.bufBusy = [2]bool{}
	w.curBuf = 0
	w.hasFrame = false
	w.poolW, w.poolH, w.stride = 0, 0, 0
	w.dirtyTrack = wlDirtyTracker{}
	w.mu.Unlock()

	for _, id := range bufs {
		if id != 0 {
			w.send(newWlMsg(id, wlBufferDestroy), -1)
		}
	}
	if poolID != 0 {
		w.send(newWlMsg(poolID, wlShmPoolDestroy), -1)
	}
	if data != nil {
		unix.Munmap(data)
	}
	if fd > 0 {
		unix.Close(fd)
	}
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
	w.applyViewport()
	w.damageSurface(image.Rect(0, 0, w.poolW, w.poolH))
	w.send(newWlMsg(w.surfaceID, wlSurfaceCommit), -1)
}

// attachAndCommit прикрепляет текущий буфер и коммитит damage-область.
func (w *WaylandWindow) attachAndCommit(dirty image.Rectangle) {
	w.send(newWlMsg(w.surfaceID, wlSurfaceAttach).putUint(w.bufID[w.curBuf]).putInt(0).putInt(0), -1)
	// Размер кадра в поверхностных единицах — заново на каждом кадре: после
	// ресайза он меняется, а компоновщик помнит прежний.
	w.applyViewport()
	w.damageSurface(dirty)
	w.send(newWlMsg(w.surfaceID, wlSurfaceCommit), -1)
	w.mu.Lock()
	w.bufBusy[w.curBuf] = true
	w.curBuf ^= 1
	w.hasFrame = true
	w.pendingResize = false
	w.mu.Unlock()
}

func (w *WaylandWindow) BlitRGBA(img *image.RGBA) {
	if img == nil {
		return
	}
	w.BlitRGBADirty(img, img.Bounds())
}

// wlDirtyTracker сводит dirty-области при двойной буферизации: буфер,
// пропустивший кадр, обязан довписать всё, что ушло в соседний.
type wlDirtyTracker struct {
	pending image.Rectangle    // накоплено с пропущенных кадров
	stale   [2]image.Rectangle // устарело в каждом буфере
}

// next возвращает область для записи в буфер idx и обновляет состояние.
func (t *wlDirtyTracker) next(idx int, dirty image.Rectangle) image.Rectangle {
	area := dirty.Union(t.pending).Union(t.stale[idx])
	t.pending = image.Rectangle{}
	t.stale[idx] = image.Rectangle{}
	t.stale[idx^1] = t.stale[idx^1].Union(area)
	return area
}

// skip откладывает область пропущенного кадра до следующего.
func (t *wlDirtyTracker) skip(dirty image.Rectangle) {
	t.pending = t.pending.Union(dirty)
}

// waitBufFree ждёт release целевого буфера. false — кадр надо пропустить.
func (w *WaylandWindow) waitBufFree() bool {
	deadline := time.Now().Add(wlBufWaitTimeout)
	for {
		w.mu.Lock()
		free := !w.bufBusy[w.curBuf]
		ch := w.bufRelease
		w.mu.Unlock()
		if free {
			return true
		}
		left := time.Until(deadline)
		if left <= 0 || ch == nil {
			return false
		}
		t := time.NewTimer(left)
		select {
		case <-ch:
		case <-t.C:
		}
		t.Stop()
	}
}

// BlitRGBADirty конвертирует и коммитит только изменившуюся область. Если
// композитор ещё держит целевой буфер — кадр пропускается, область копится.
func (w *WaylandWindow) BlitRGBADirty(img *image.RGBA, dirty image.Rectangle) {
	if w.surfaceID == 0 || img == nil || w.closed {
		return
	}
	b := img.Bounds()
	if w.skipStaleFrame(b) {
		return
	}
	// Кадр другого размера — это ресайз: пул перестраивается под него, и
	// область рисуется целиком (в новых буферах нет ничего). То же, когда
	// окно получило или потеряло скругление: формат буфера другой.
	if b.Dx() != w.poolW || b.Dy() != w.poolH || w.poolARGB != w.wantARGB() {
		if err := w.ensurePool(b.Dx(), b.Dy()); err != nil {
			wlLog("пул под %dx%d: %v", b.Dx(), b.Dy(), err)
			return
		}
		dirty = b
		w.applyOpaqueRegion(b.Dx(), b.Dy())
	}
	if w.shmData == nil {
		return
	}
	width, height := min(b.Dx(), w.poolW), min(b.Dy(), w.poolH)
	dirty = dirty.Intersect(image.Rect(0, 0, width, height))
	if dirty.Empty() {
		return
	}

	if !w.waitBufFree() {
		w.mu.Lock()
		w.dirtyTrack.skip(dirty)
		w.mu.Unlock()
		return
	}

	w.mu.Lock()
	idx := w.curBuf
	area := w.dirtyTrack.next(idx, dirty).Intersect(image.Rect(0, 0, width, height))
	w.mu.Unlock()

	base := idx * w.stride * w.poolH
	convRectBGRX(w.shmData[base:], w.stride, img.Pix, img.Stride, area)
	if w.poolARGB {
		applyCornerMask(w.shmData[base:], w.stride, w.poolW, w.poolH, w.maskRadius(), area)
	}
	w.attachAndCommit(area)
}

// maxStaleFrames — сколько кадров прежнего размера можно не показать.
//
// Кадр нового размера приходит следом (окно передаёт размер движку, и тот
// перерисовывает всё), так что хватает одного-двух. Больше ждать нельзя:
// окно, которое так и не показалось, хуже окна, на миг показанного не в
// своём размере.
const maxStaleFrames = 2

// skipStaleFrame — не показывать кадр, нарисованный под прежний размер, пока
// ответ на configure с новым размером ещё в пути.
//
// Компоновщик, который в первом configure сразу назначает размер (тайлинг,
// половина экрана), получал первым буфер в размере холста программы: окно
// открывалось не того размера, а дальше рывком менялось. Теперь первым
// коммитится кадр уже нужного размера. Разница в точку — округление
// логического размера на дробном масштабе, такой кадр годится.
func (w *WaylandWindow) skipStaleFrame(b image.Rectangle) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.hasFrame || !w.pendingResize || w.staleSkipped >= maxStaleFrames {
		return false
	}
	if near(b.Dx(), w.width) && near(b.Dy(), w.height) {
		return false
	}
	w.staleSkipped++
	wlLog("кадр %dx%d прежнего размера не показан: жду %dx%d", b.Dx(), b.Dy(), w.width, w.height)
	return true
}

func near(a, b int) bool { return a-b <= 1 && b-a <= 1 }

// clientSize — размер окна в пикселях буфера, каким его назначил
// компоновщик. Окно сверяет его с холстом сразу после Create: первый
// configure приходит ещё внутри Create, когда обработчика ресайза нет.
func (w *WaylandWindow) clientSize() (int, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.width, w.height
}

// ─── Управление окном ────────────────────────────────────────────────────────

func (w *WaylandWindow) Close() {
	w.repeat.cancel()
	w.closeAllPopups()
	w.closeCursor()
	w.closed = true
	// Соединение закрываем ПОСЛЕ памяти: destroyPool ещё пишет в сокет
	// (wl_buffer.destroy, wl_shm_pool.destroy), а Close по протоколу — это
	// разрыв, после которого композитор объекты и так забудет.
	w.destroyPool()
	if w.conn != nil {
		w.conn.Close()
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
func (w *WaylandWindow) SetPosition(x, y int)    {} // Wayland: позицией владеет композитор
func (w *WaylandWindow) GetPosition() (int, int) { return 0, 0 }
// SetCornerRadius задаёт скругление углов окна (логические точки). Со
// скруглением буфер окна — ARGB с прозрачными углами; пул пересоздаётся на
// следующем кадре.
func (w *WaylandWindow) SetCornerRadius(r int) {
	if r < 0 {
		r = 0
	}
	w.cornerR.Store(int32(r))
}

// wantARGB — окну нужен буфер с альфой.
func (w *WaylandWindow) wantARGB() bool { return w.cornerR.Load() > 0 }

// maskRadius — радиус маски в пикселях буфера. Развёрнутое и полноэкранное
// окно не скругляется: оно прилегает к краям экрана и непрозрачно целиком.
func (w *WaylandWindow) maskRadius() int {
	if w.maximized.Load() || w.fullscreen.Load() {
		return 0
	}
	return int(float64(w.cornerR.Load())*w.scaleFactor() + 0.5)
}

// applyOpaqueRegion сообщает компоновщику непрозрачную часть окна, чтобы он
// не смешивал с фоном всё окно ради четырёх углов. Без скругления — всё окно.
// Применяется вместе с ближайшим commit.
func (w *WaylandWindow) applyOpaqueRegion(pw, ph int) {
	if w.compositorID == 0 || w.surfaceID == 0 {
		return
	}
	sw, sh := w.toSurface(pw), w.toSurface(ph)
	sr := 0
	if w.wantARGB() {
		sr = w.toSurface(w.maskRadius())
	}
	sr = min(sr, sw/2, sh/2)
	rid := w.newID()
	w.send(newWlMsg(w.compositorID, wlCompositorCreateRegion).putUint(rid), -1)
	add := func(x, y, rw, rh int) {
		if rw > 0 && rh > 0 {
			w.send(newWlMsg(rid, wlRegionAdd).
				putInt(int32(x)).putInt(int32(y)).putInt(int32(rw)).putInt(int32(rh)), -1)
		}
	}
	if sr == 0 {
		add(0, 0, sw, sh)
	} else {
		add(0, sr, sw, sh-2*sr) // полоса без углов по высоте
		add(sr, 0, sw-2*sr, sh) // и по ширине
	}
	w.send(newWlMsg(w.surfaceID, wlSurfaceSetOpaqueRegion).putUint(rid), -1)
	w.send(newWlMsg(rid, wlRegionDestroy), -1)
}

// ─── Перемещение, размер и состояние окна (xdg_toplevel) ─────────────────────
//
// Под Wayland окном распоряжается компоновщик. Позиции клиент не знает и
// задать её не может — он лишь ПРОСИТ: «тащи меня за курсором», «тяни этот
// край», «сверни», «разверни». Отказ компоновщика — не ошибка: окно просто
// остаётся как было.

// BeginMove просит компоновщик тащить окно за курсором (xdg_toplevel.move).
// Реализует interactiveMover.
func (w *WaylandWindow) BeginMove() bool {
	serial := w.ptrButtonSerial.Load()
	if w.toplevelID == 0 || w.seatID == 0 || serial == 0 {
		return false // нечем доказать, что перемещение начал пользователь
	}
	wlLog("move: seat=%d serial=%d", w.seatID, serial)
	w.send(newWlMsg(w.toplevelID, xdgToplevelMove).
		putUint(w.seatID).putUint(serial), -1)
	// Нажатие ушло компоновщику: он пришлёт leave, а отпускание — нет.
	// Отпускаем кнопки сразу, не дожидаясь leave, иначе окно уедет с
	// «зажатой» кнопкой в состоянии движка.
	w.releaseHeldButtons()
	return true
}

// BeginResize просит компоновщик тянуть край окна (xdg_toplevel.resize).
// edges — биты widget.NativeEdge*. Реализует interactiveMover.
func (w *WaylandWindow) BeginResize(edges int) bool {
	serial := w.ptrButtonSerial.Load()
	if w.toplevelID == 0 || w.seatID == 0 || serial == 0 || !w.resizable.Load() {
		return false
	}
	e := wlResizeEdge(edges)
	if e == xdgResizeEdgeNone {
		return false
	}
	wlLog("resize: seat=%d serial=%d edges=%d", w.seatID, serial, e)
	w.send(newWlMsg(w.toplevelID, xdgToplevelResize).
		putUint(w.seatID).putUint(serial).putUint(e), -1)
	w.releaseHeldButtons()
	return true
}

// releaseHeldButtons отпускает кнопки, которые движок считает зажатыми.
//
// Компоновщик, забравший нажатие под move/resize, отпускания не пришлёт — а
// движок держит по нажатой кнопке захват мыши. Без этого окно после
// перетаскивания вело бы себя так, будто кнопку не отпускали.
func (w *WaylandWindow) releaseHeldButtons() {
	held := w.ptrButtons.Swap(0)
	if held == 0 || w.onMouseButton == nil {
		return
	}
	for id := 0; id < 3; id++ {
		if held&(1<<uint(id)) != 0 {
			w.onMouseButton(w.ptrX, w.ptrY, id, false)
		}
	}
}

// Minimize сворачивает окно (xdg_toplevel.set_minimized). Обратного запроса в
// протоколе нет: разворачивает окно обратно сам пользователь через панель.
func (w *WaylandWindow) Minimize() {
	if w.toplevelID == 0 {
		return
	}
	wlLog("set_minimized")
	w.send(newWlMsg(w.toplevelID, xdgToplevelSetMinimized), -1)
}

// Maximize разворачивает окно (xdg_toplevel.set_maximized). Новый размер
// придёт в configure — его же ждёт и Restore.
func (w *WaylandWindow) Maximize() {
	if w.toplevelID == 0 {
		return
	}
	// Развернуть окно, размер которого зафиксирован (min == max), компоновщик
	// не сможет: снимаем фиксацию так же, как для ресайза за край.
	wlLog("set_maximized")
	w.applySizeLimits(true)
	w.send(newWlMsg(w.toplevelID, xdgToplevelSetMaximized), -1)
}

// Restore возвращает окно из развёрнутого состояния.
func (w *WaylandWindow) Restore() {
	if w.toplevelID == 0 {
		return
	}
	wlLog("unset_maximized")
	w.send(newWlMsg(w.toplevelID, xdgToplevelUnsetMaximized), -1)
	if !w.resizable.Load() {
		// Размер возвращаем под прежний запрет: компоновщик пришлёт
		// configure, и фиксация снова сделает окно нерастяжимым.
		w.applySizeLimits(false)
	}
}

// IsMaximized сообщает состояние из последнего configure. Полноэкранное окно
// для кнопки □ — тоже «развёрнуто»: нажатие на неё должно возвращать окно.
func (w *WaylandWindow) IsMaximized() bool { return w.maximized.Load() || w.fullscreen.Load() }

// Callbacks
func (w *WaylandWindow) SetOnResize(fn func(w, h int))                           { w.onResize = fn }
func (w *WaylandWindow) SetOnClose(fn func() bool)                               { w.onClose = fn }
func (w *WaylandWindow) SetOnMouseMove(fn func(x, y int))                        { w.onMouseMove = fn }
func (w *WaylandWindow) SetOnMouseButton(fn func(x, y, button int, pressed bool)) { w.onMouseButton = fn }

// SetOnMouseWheelPixels регистрирует колбэк точной пиксельной дельты колеса/
// тачпада (wl_pointer.axis). Без него бэкенд шлёт тики через SetOnMouseButton.
func (w *WaylandWindow) SetOnMouseWheelPixels(fn func(x, y int, dx, dy float64)) { w.onMouseWheelPixels = fn }
func (w *WaylandWindow) SetOnKeyDown(fn func(vk int))                            { w.onKeyDown = fn }
func (w *WaylandWindow) SetOnKeyUp(fn func(vk int))                              { w.onKeyUp = fn }
func (w *WaylandWindow) SetOnChar(fn func(r rune))                               { w.onChar = fn }

// SetOnActivate — активность окна (state activated в configure).
func (w *WaylandWindow) SetOnActivate(fn func(active bool)) { w.onActivate = fn }

// SetOnFilesDropped регистрирует колбэк Drag&Drop файлов из ОС
// (wl_data_device). Координаты — поверхностные (клиентские) пиксели.
func (w *WaylandWindow) SetOnFilesDropped(fn func(paths []string, x, y int)) { w.onFilesDropped = fn }

// ─── Drag&Drop (wl_data_device) ──────────────────────────────────────────────
//
// Каркас приёма файлов из файлового менеджера. Поддерживается text/uri-list в
// один проход: enter→(accept)→drop→receive(pipe)→parse→finish. Ограничения:
//   - действие фиксировано copy; альтернативные действия (move/ask) не согласуются;
//   - offer буфера обмена (selection) уничтожается без обработки;
//   - finish/set_actions доступны только при версии wl_data_device_manager ≥ 3
//     (при v<3 — совместимый путь без них);
//   - позиция сброса берётся из последнего enter/motion (поверхностные пиксели).
// Компилируемость и корректность на современных композиторах (v3) обеспечены;
// углублённый torn-drag feedback не реализован.

// isOfferObject сообщает, известен ли объект obj как data_offer.
func (w *WaylandWindow) isOfferObject(id uint32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.offers[id]
	return ok
}

// offerSet фиксирует наличие text/uri-list в offer (или регистрирует новый).
func (w *WaylandWindow) offerSet(id uint32, hasURIList bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.offers == nil {
		w.offers = map[uint32]bool{}
	}
	if hasURIList || !w.offers[id] {
		w.offers[id] = hasURIList
	}
}

// offerSetText отмечает, что offer предлагает простой текст.
//
// Отдельная карта, а не второй флаг в offers: тот говорит про список файлов
// для перетаскивания, и смешивать «тут файлы» с «тут текст» в одном булеве
// значит однажды вставить в редактор список путей вместо скопированной
// строки.
func (w *WaylandWindow) offerSetText(id uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.textOffers == nil {
		w.textOffers = map[uint32]bool{}
	}
	w.textOffers[id] = true
}

// offerHasText сообщает, предлагает ли offer простой текст.
func (w *WaylandWindow) offerHasText(id uint32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.textOffers[id]
}

// offerSetHTML отмечает, что offer предлагает HTML под указанным типом. Если
// объявлено несколько вариантов, остаётся голый text/html: его понимают все.
func (w *WaylandWindow) offerSetHTML(id uint32, mime string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.htmlOffers == nil {
		w.htmlOffers = map[uint32]string{}
	}
	if old, ok := w.htmlOffers[id]; !ok || mime == mimeTextHTML || old == "" {
		w.htmlOffers[id] = mime
	}
}

// offerHTMLMime возвращает тип HTML, предложенный offer'ом, или пустую строку.
func (w *WaylandWindow) offerHTMLMime(id uint32) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.htmlOffers[id]
}

// offerSetGnomeFiles отмечает, что offer предлагает x-special/gnome-copied-files.
func (w *WaylandWindow) offerSetGnomeFiles(id uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.gnomeOffers == nil {
		w.gnomeOffers = map[uint32]bool{}
	}
	w.gnomeOffers[id] = true
}

// offerHasGnomeFiles сообщает, предлагает ли offer x-special/gnome-copied-files.
func (w *WaylandWindow) offerHasGnomeFiles(id uint32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.gnomeOffers[id]
}

// isTextMime — тип, под которым ходит простой текст.
func isTextMime(mime string) bool {
	switch mime {
	case mimeTextUTF8, mimeTextPlain, mimeUTF8Str, mimeTextStr:
		return true
	}
	return false
}

// offerGet возвращает флаг «предложен text/uri-list» для offer.
func (w *WaylandWindow) offerGet(id uint32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.offers[id]
}

// offerDelete удаляет offer из карты.
func (w *WaylandWindow) offerDelete(id uint32) {
	w.mu.Lock()
	delete(w.textOffers, id)
	delete(w.htmlOffers, id)
	delete(w.gnomeOffers, id)
	w.mu.Unlock()
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.offers, id)
}

// handleDataDevice обрабатывает события wl_data_device.
func (w *WaylandWindow) handleDataDevice(opcode uint16, b []byte) {
	switch opcode {
	case wlDataDeviceEvDataOffer:
		// new_id offer — компонент создаёт объект data_offer.
		id := binary.LittleEndian.Uint32(b[0:4])
		w.offerSet(id, false)

	case wlDataDeviceEvEnter:
		// serial, surface, x(fixed), y(fixed), id(offer)
		w.dndSerial = binary.LittleEndian.Uint32(b[0:4])
		w.dndX = int(int32(binary.LittleEndian.Uint32(b[8:12]))) >> 8
		w.dndY = int(int32(binary.LittleEndian.Uint32(b[12:16]))) >> 8
		w.dndOffer = binary.LittleEndian.Uint32(b[16:20])
		w.dndAcceptOffer()

	case wlDataDeviceEvMotion:
		// time, x(fixed), y(fixed)
		w.dndX = int(int32(binary.LittleEndian.Uint32(b[4:8]))) >> 8
		w.dndY = int(int32(binary.LittleEndian.Uint32(b[8:12]))) >> 8

	case wlDataDeviceEvLeave:
		if w.dndOffer != 0 {
			w.send(newWlMsg(w.dndOffer, wlDataOfferDestroy), -1)
			w.offerDelete(w.dndOffer)
			w.dndOffer = 0
		}

	case wlDataDeviceEvDrop:
		w.dndReceive()

	case wlDataDeviceEvSelection:
		// Содержимое буфера обмена: компоновщик отдаёт готовый offer, у
		// которого можно спросить данные (wayland_clipboard_linux.go).
		if len(b) >= 4 {
			w.clip.handleSelection(binary.LittleEndian.Uint32(b[0:4]))
		}
	}
}

// dndAcceptOffer сообщает компоненту, принимаем ли мы предложение (accept +
// set_actions при v≥3).
func (w *WaylandWindow) dndAcceptOffer() {
	if w.dndOffer == 0 {
		return
	}
	if w.offerGet(w.dndOffer) {
		w.send(newWlMsg(w.dndOffer, wlDataOfferAccept).
			putUint(w.dndSerial).putString(mimeTextUriList), -1)
		if w.dataDevVersion >= 3 {
			w.send(newWlMsg(w.dndOffer, wlDataOfferSetActions).
				putUint(wlDndActionCopy).putUint(wlDndActionCopy), -1)
		}
		return
	}
	// Отклоняем: accept с null-строкой (длина 0).
	w.send(newWlMsg(w.dndOffer, wlDataOfferAccept).putUint(w.dndSerial).putUint(0), -1)
}

// dndReceive запрашивает данные (text/uri-list) через pipe и асинхронно их
// читает, парсит и доставляет колбэку, после чего завершает offer.
func (w *WaylandWindow) dndReceive() {
	offer := w.dndOffer
	w.dndOffer = 0
	if offer == 0 || !w.offerGet(offer) {
		w.completeOffer(offer, false)
		return
	}
	r, wr, err := os.Pipe()
	if err != nil {
		w.completeOffer(offer, false)
		return
	}
	// receive(mime, fd): компонент пишет данные в write-конец, мы читаем read-конец.
	w.send(newWlMsg(offer, wlDataOfferReceive).putString(mimeTextUriList), int(wr.Fd()))
	wr.Close() // наш write-конец не нужен — EOF придёт при закрытии компонентом
	x, y := w.dndX, w.dndY
	go func() {
		defer r.Close()
		data, _ := io.ReadAll(io.LimitReader(r, maxDnDBytes))
		paths := parseURIList(string(data))
		if len(paths) > 0 && w.onFilesDropped != nil {
			w.onFilesDropped(paths, x, y)
		}
		w.completeOffer(offer, len(paths) > 0)
	}()
}

// completeOffer завершает работу с offer: finish (v≥3, при успехе) + destroy.
func (w *WaylandWindow) completeOffer(offer uint32, accepted bool) {
	if offer == 0 {
		return
	}
	if accepted && w.dataDevVersion >= 3 {
		w.send(newWlMsg(offer, wlDataOfferFinish), -1)
	}
	w.send(newWlMsg(offer, wlDataOfferDestroy), -1)
	w.offerDelete(offer)
}

// SetResizable разрешает пользователю менять размер окна.
//
// Под Wayland запрет выражается фиксацией размера: set_min_size == set_max_size.
// Create фиксирует окно сразу (движок сам управляет разрешением), и без
// снятия фиксации ни край, ни кнопка «развернуть» ничего бы не дали.
func (w *WaylandWindow) SetResizable(v bool) {
	w.resizable.Store(v)
	if w.toplevelID == 0 {
		return // Create применит: SetResizable зовут и до создания окна
	}
	w.applySizeLimits(v)
}

// applySizeLimits задаёт пару min/max размера под текущий режим.
//
// resizable: максимума нет (0×0 — «сколько угодно»), минимум — заданный
// приложением (или ничего). Иначе окно фиксируется на текущем размере.
func (w *WaylandWindow) applySizeLimits(resizable bool) {
	if w.toplevelID == 0 {
		return
	}
	if !resizable {
		sw, sh := int32(w.toSurface(w.width)), int32(w.toSurface(w.height))
		w.send(newWlMsg(w.toplevelID, xdgToplevelSetMinSize).putInt(sw).putInt(sh), -1)
		w.send(newWlMsg(w.toplevelID, xdgToplevelSetMaxSize).putInt(sw).putInt(sh), -1)
		return
	}
	mw, mh := int32(0), int32(0)
	if w.minW > 0 || w.minH > 0 {
		mw, mh = wlMinSizeArgs(w.minW, w.minH, w.scaleFactor())
	}
	w.send(newWlMsg(w.toplevelID, xdgToplevelSetMinSize).putInt(mw).putInt(mh), -1)
	w.send(newWlMsg(w.toplevelID, xdgToplevelSetMaxSize).putInt(0).putInt(0), -1)
}

// SetAppID задаёт идентификатор приложения (xdg_toplevel.app_id): по нему
// среда рабочего стола ищет значок и <app_id>.desktop. Реализует appIDSetter.
func (w *WaylandWindow) SetAppID(id string) {
	if id == "" {
		return
	}
	w.appID = id
	if w.toplevelID != 0 {
		w.send(newWlMsg(w.toplevelID, xdgToplevelSetAppID).putString(id), -1)
	}
}

// effectiveAppID — заданный приложением идентификатор или имя исполняемого файла.
func (w *WaylandWindow) effectiveAppID() string {
	if w.appID != "" {
		return w.appID
	}
	return defaultAppID(os.Args[0])
}

// SetMinSize задаёт минимальный размер окна через xdg_toplevel.set_min_size
// (опкод xdgToplevelSetMinSize=8 — тот же, что уже шлёт Create при фиксации
// стартового размера, так что нумерация заведомо совпадает с остальным кодом
// этого файла). Композитор не даёт пользователю потянуть край окна меньше
// заданного — раньше запрос не отправлялся вовсе, поэтому ограничения не было
// (GG-18).
//
// Перевод физических пикселей (контракт SetMinSize) в surface-local координаты
// протокола делает wlMinSizeArgs (minsize_linux.go, чистая функция с тестами).
// Scale передаём как 1: этот бэкенд не отслеживает буферный масштаб поверхности
// (ни wl_surface.set_buffer_scale, ни wp_fractional_scale не реализованы —
// см. подробный комментарий у wlMinSizeArgs), так что physical == logical,
// как и у уже существующих вызовов set_min_size/set_max_size в Create.
func (w *WaylandWindow) SetMinSize(width, height int) {
	// Toplevel ещё нет — запоминаем: Create сам зафиксирует размер и следом
	// применит отложенный минимум (иначе он потерялся бы под фиксацией).
	if w.toplevelID == 0 {
		w.minW, w.minH, w.minWant = width, height, true
		return
	}
	w.minW, w.minH = width, height
	mw, mh := wlMinSizeArgs(width, height, w.scaleFactor())
	w.send(newWlMsg(w.toplevelID, xdgToplevelSetMinSize).putInt(mw).putInt(mh), -1)
}
