//go:build linux && !android

package window

import (
	"encoding/binary"
	"image"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

// ─── MIT-SHM: блит через разделяемую память вместо PutImage по сокету ──────
//
// Идея: PutImage копирует весь блитуемый прямоугольник в тело X11-запроса и
// шлёт его через Unix-сокет — при частых кадрах это лишняя копия и системные
// вызовы write() на каждый кадр. MIT-SHM даёт X-серверу прямой доступ к
// SysV-сегменту разделяемой памяти: клиент один раз аттачит сегмент
// (ShmAttach), а дальше на каждый кадр шлёт короткий ShmPutImage (адрес +
// прямоугольник) вместо самих пикселей.
//
// Протокол не гарантирует, что сервер закончил ЧИТАТЬ сегмент сразу после
// ShmPutImage — до события ShmCompletion переиспользовать память нельзя.
// Поэтому пока предыдущий кадр не подтверждён, новый кадр уходит обычным
// PutImage (см. x11ShmBlit/busy) — рендер никогда не блокируется в ожидании
// сервера.
//
// Инициализация (QueryExtension → ShmQueryVersion → ShmAttach) идёт ДО
// CreateWindow, пока на соединении заведомо нет событий (тот же приём, что и
// у x11LoadKeyboardMapping/x11InternAtom в native_linux.go) — поэтому reply
// читается напрямую, без петли "событие или ответ". Любая неудача на любом
// шаге — необратимый fallback на PutImage до конца жизни окна.

// Minor-опкоды расширения MIT-SHM (относительно её major-opcode).
const (
	shmMinorQueryVersion = 0
	shmMinorAttach       = 1
	shmMinorDetach       = 2
	shmMinorPutImage     = 3
)

// x11ShmState — состояние MIT-SHM для одного X11Window. nil-указатель на
// уровне X11Window (поле shm) означает «инициализация ещё не запускалась»;
// fallback=true — «пробовали и не вышло/не поддерживается», PutImage навсегда.
type x11ShmState struct {
	major      byte // major-opcode расширения на этом сервере (0 — неизвестен)
	firstEvent byte // ShmCompletion = firstEvent+0

	shmid  int    // System V shm-идентификатор текущего сегмента
	shmseg uint32 // XID сегмента на стороне X-сервера (ShmAttach/ShmPutImage)
	data   []byte // маппинг сегмента в память процесса (nil, если не аттачен)

	width, height int // размер сегмента в пикселях (совпадает с размером окна)

	// busy — предыдущий кадр отправлен (sendEvent=1), ShmCompletion ещё не
	// пришёл. Пишется из горутины блита (true) и из горутины цикла событий
	// (false при ShmCompletion) — атомарный.
	busy atomic.Bool

	fallback bool // true — SHM не используется, BlitRGBADirty всегда шлёт PutImage
}

// x11ShmInit пытается активировать MIT-SHM для окна: QueryExtension →
// ShmQueryVersion → выделение сегмента под текущий размер окна → раунд-трип
// на предмет ошибки ShmAttach. Вызывается из Create/CreatePopup ДО
// CreateWindow (см. комментарий выше про порядок). Ничего не паникует и не
// возвращает ошибку — при любой неудаче w.shm.fallback остаётся true, и
// BlitRGBADirty работает как раньше, через PutImage.
func (w *X11Window) x11ShmInit() {
	w.shm = &x11ShmState{fallback: true}

	major, firstEvent, ok := w.x11QueryShmExtension()
	if !ok {
		return
	}
	if !w.x11ShmQueryVersion(major) {
		return
	}
	w.shm.major = major
	w.shm.firstEvent = firstEvent

	shmid, data, shmseg, ok := w.x11ShmAllocSegment(w.width, w.height)
	if !ok {
		return
	}
	// Единственное место, где синхронная проверка ошибки ShmAttach безопасна:
	// событий на соединении ещё нет (окно не создано, MapWindow не вызывался).
	if !w.x11SyncCheckError() {
		unix.SysvShmDetach(data)
		unix.SysvShmCtl(shmid, unix.IPC_RMID, nil) // на случай, если Attach уже отклонён сервером
		return
	}

	w.shm.shmid = shmid
	w.shm.data = data
	w.shm.shmseg = shmseg
	w.shm.width = w.width
	w.shm.height = w.height
	w.shm.fallback = false
}

// x11ShmAllocSegment создаёт SysV-сегмент под width×height пикселей (BGRA,
// 4 байта/пиксель), маппит его локально (shmat) и шлёт серверу ShmAttach.
// Сегмент сразу помечается к удалению (IPC_RMID): ядро уничтожит его, как
// только сегмент отсоединят все прикреплённые стороны (мы и X-сервер), даже
// если процесс упадёт раньше явного Detach — утечки нет.
//
// Общая часть для первичной инициализации (x11ShmInit, с синхронной проверкой
// ошибки следом) и пересоздания при ресайзе (x11ShmResize, best-effort — см.
// его комментарий, почему там синхронная проверка недопустима).
func (w *X11Window) x11ShmAllocSegment(width, height int) (shmid int, data []byte, shmseg uint32, ok bool) {
	if width <= 0 || height <= 0 {
		return 0, nil, 0, false
	}
	size := width * height * 4

	shmid, err := unix.SysvShmGet(unix.IPC_PRIVATE, size, unix.IPC_CREAT|0600)
	if err != nil {
		return 0, nil, 0, false
	}
	data, err = unix.SysvShmAttach(shmid, 0, 0)
	if err != nil {
		unix.SysvShmCtl(shmid, unix.IPC_RMID, nil)
		return 0, nil, 0, false
	}

	shmseg = w.x11GenID()
	w.x11Send(x11ShmAttachRequest(w.shm.major, shmseg, uint32(shmid), false))
	unix.SysvShmCtl(shmid, unix.IPC_RMID, nil)

	return shmid, data, shmseg, true
}

// x11ShmResize пересоздаёт SHM-сегмент под новый размер окна (вызывается из
// handleX11Event при ConfigureNotify). В отличие от x11ShmInit, здесь НЕЛЬЗЯ
// сделать синхронную round-trip проверку ошибки (x11SyncCheckError): это
// читало бы следующий пакет с сокета прямо из цикла обработки событий,
// воруя чужое событие. Поэтому поведение best-effort: если сервер на самом
// деле отверг ShmAttach, ShmCompletion для этого сегмента никогда не придёт,
// busy останется взведённым навсегда, и x11ShmBlit будет молча возвращать
// false — BlitRGBADirty будет каждый кадр использовать PutImage. Итог тот же,
// что и явный fallback, просто без явного флага.
func (w *X11Window) x11ShmResize(width, height int) {
	if w.shm == nil || w.shm.fallback {
		return
	}
	if width <= 0 || height <= 0 {
		return // окно свёрнуто/нулевого размера — существующий сегмент не трогаем
	}

	w.blitMu.Lock()
	defer w.blitMu.Unlock()

	if w.shm.shmseg != 0 {
		w.x11Send(x11ShmDetachRequest(w.shm.major, w.shm.shmseg))
	}
	if w.shm.data != nil {
		unix.SysvShmDetach(w.shm.data)
		w.shm.data = nil
	}
	w.shm.shmseg = 0

	shmid, data, shmseg, ok := w.x11ShmAllocSegment(width, height)
	if !ok {
		w.shm.fallback = true
		return
	}
	w.shm.shmid = shmid
	w.shm.data = data
	w.shm.shmseg = shmseg
	w.shm.width = width
	w.shm.height = height
	// Новый сегмент ничего ещё не отправлял — ждать ShmCompletion от старого
	// (уже отсоединённого) сегмента незачем.
	w.shm.busy.Store(false)
}

// x11ShmClose освобождает локальный маппинг сегмента. Серверную половину
// (аттач ShmAttach) X-сервер снимает сам при закрытии соединения клиента —
// отдельный ShmDetach-запрос перед закрытием сокета не нужен.
func (w *X11Window) x11ShmClose() {
	if w.shm == nil || w.shm.data == nil {
		return
	}
	unix.SysvShmDetach(w.shm.data)
	w.shm.data = nil
}

// x11ShmBlit пытается отправить dirty-область текущего кадра через MIT-SHM.
// Возвращает false, если SHM недоступен, сервер ещё не подтвердил предыдущий
// кадр (busy) или размер img разошёлся с размером сегмента (гонка с
// ресайзом) — тогда BlitRGBADirty обязан сам отправить этот кадр PutImage.
// Вызывается под w.blitMu (см. BlitRGBADirty).
func (w *X11Window) x11ShmBlit(img *image.RGBA, dirty image.Rectangle) bool {
	shm := w.shm
	if shm == nil || shm.fallback || shm.data == nil {
		return false
	}
	bounds := img.Bounds()
	if bounds.Dx() != shm.width || bounds.Dy() != shm.height {
		return false
	}
	if !shm.busy.CompareAndSwap(false, true) {
		return false // предыдущий кадр ещё не подтверждён сервером (ShmCompletion)
	}

	// BGRA-конвертация только dirty-строк, но НА МЕСТЕ в полнокадровом
	// сегменте (totalWidth/Height у ShmPutImage — размер всего изображения,
	// offset=0, поэтому смещение строки считается по полной ширине сегмента,
	// а не по ширине dirty-области — иначе плывёт весь кадр).
	dw := dirty.Dx()
	dh := dirty.Dy()
	stride := shm.width
	src := img.Pix
	srcStride := img.Stride
	rowLen := dw * 4
	for y := 0; y < dh; y++ {
		srcOff := (dirty.Min.Y+y)*srcStride + dirty.Min.X*4
		dstOff := ((dirty.Min.Y+y)*stride + dirty.Min.X) * 4
		// RGBA → BGRA построчным 32-битным swap'ом (см. pixconv.go).
		swapRBRow(shm.data[dstOff:dstOff+rowLen], src[srcOff:srcOff+rowLen])
	}

	req := x11ShmPutImageRequest(shm.major, w.wid, w.gcID,
		shm.width, shm.height,
		dirty.Min.X, dirty.Min.Y, dw, dh,
		dirty.Min.X, dirty.Min.Y,
		true, shm.shmseg, 0)
	w.x11Send(req)
	return true
}

// x11ShmCompletion — обработка события ShmCompletion (firstEvent+0): сервер
// закончил читать сегмент из предыдущего ShmPutImage, память снова наша.
// Вызывается из handleX11Event.
func (w *X11Window) x11ShmCompletion() {
	if w.shm != nil {
		w.shm.busy.Store(false)
	}
}

// ─── Протокол: сборка запросов (чистые функции — см. x11shm_test.go) ───────

// x11QueryShmExtensionRequest — тело core-запроса QueryExtension("MIT-SHM")
// (opcode 98). Формат идентичен x11InternAtom: opcode, unused, length,
// name-length, unused, name (+ паддинг до кратности 4).
func x11QueryShmExtensionRequest() []byte {
	name := []byte("MIT-SHM")
	nameLen := len(name)
	pad := (4 - nameLen%4) % 4
	reqLen := 2 + (nameLen+pad)/4

	buf := make([]byte, reqLen*4)
	buf[0] = 98 // QueryExtension
	binary.LittleEndian.PutUint16(buf[2:4], uint16(reqLen))
	binary.LittleEndian.PutUint16(buf[4:6], uint16(nameLen))
	copy(buf[8:], name)
	return buf
}

// parseQueryExtensionReply разбирает 32-байтовый reply QueryExtension:
// present@8 (BOOL), major-opcode@9, first-event@10, first-error@11.
func parseQueryExtensionReply(reply []byte) (major, firstEvent byte, present bool) {
	if len(reply) < 12 || reply[0] != 1 {
		return 0, 0, false
	}
	present = reply[8] != 0
	major = reply[9]
	firstEvent = reply[10]
	return major, firstEvent, present
}

// x11QueryShmExtension посылает QueryExtension("MIT-SHM") и читает reply
// напрямую (см. комментарий x11ShmInit — событий на соединении ещё нет).
func (w *X11Window) x11QueryShmExtension() (major, firstEvent byte, ok bool) {
	w.x11Send(x11QueryShmExtensionRequest())

	reply := make([]byte, 32)
	if !w.readFull(reply) {
		return 0, 0, false
	}
	major, firstEvent, present := parseQueryExtensionReply(reply)
	if !present {
		return 0, 0, false
	}
	return major, firstEvent, true
}

// x11ShmQueryVersionRequest — тело запроса ShmQueryVersion (minor=0,
// length=1 слово: только 4-байтовый заголовок расширения, без доп. полей).
func x11ShmQueryVersionRequest(major byte) []byte {
	buf := make([]byte, 4)
	buf[0] = major
	buf[1] = shmMinorQueryVersion
	binary.LittleEndian.PutUint16(buf[2:4], 1)
	return buf
}

// x11ShmQueryVersion посылает ShmQueryVersion и проверяет, что сервер
// ответил успешным reply (тип 1). Сама версия/shared-pixmaps не используются:
// нужен только ZPixmap-путь ShmPutImage.
func (w *X11Window) x11ShmQueryVersion(major byte) bool {
	w.x11Send(x11ShmQueryVersionRequest(major))

	reply := make([]byte, 32)
	if !w.readFull(reply) {
		return false
	}
	return reply[0] == 1
}

// x11ShmAttachRequest — тело запроса ShmAttach (minor=1, length=4 слова):
// заголовок(1) + shmseg(1) + shmid(1) + readOnly(1 байт)+pad(3 байта)(1).
func x11ShmAttachRequest(major byte, shmseg, shmid uint32, readOnly bool) []byte {
	buf := make([]byte, 16)
	buf[0] = major
	buf[1] = shmMinorAttach
	binary.LittleEndian.PutUint16(buf[2:4], 4)
	binary.LittleEndian.PutUint32(buf[4:8], shmseg)
	binary.LittleEndian.PutUint32(buf[8:12], shmid)
	if readOnly {
		buf[12] = 1
	}
	// buf[13:16] — паддинг, остаётся нулевым
	return buf
}

// x11ShmDetachRequest — тело запроса ShmDetach (minor=2, length=2 слова):
// заголовок(1) + shmseg(1).
func x11ShmDetachRequest(major byte, shmseg uint32) []byte {
	buf := make([]byte, 8)
	buf[0] = major
	buf[1] = shmMinorDetach
	binary.LittleEndian.PutUint16(buf[2:4], 2)
	binary.LittleEndian.PutUint32(buf[4:8], shmseg)
	return buf
}

// x11ShmPutImageRequest — тело запроса ShmPutImage (minor=3, length=10 слов):
// drawable(u32) gc(u32) totalWidth(u16) totalHeight(u16) srcX(u16) srcY(u16)
// srcW(u16) srcH(u16) dstX(i16) dstY(i16) depth(u8) format(u8) sendEvent(u8)
// bpad(u8) shmseg(u32) offset(u32). depth/format зафиксированы под тот же
// формат, что и у существующего x11PutImage (ZPixmap, depth=24).
func x11ShmPutImageRequest(major byte, drawable, gc uint32, totalW, totalH, srcX, srcY, srcW, srcH, dstX, dstY int, sendEvent bool, shmseg, offset uint32) []byte {
	buf := make([]byte, 40)
	buf[0] = major
	buf[1] = shmMinorPutImage
	binary.LittleEndian.PutUint16(buf[2:4], 10)
	binary.LittleEndian.PutUint32(buf[4:8], drawable)
	binary.LittleEndian.PutUint32(buf[8:12], gc)
	binary.LittleEndian.PutUint16(buf[12:14], uint16(totalW))
	binary.LittleEndian.PutUint16(buf[14:16], uint16(totalH))
	binary.LittleEndian.PutUint16(buf[16:18], uint16(srcX))
	binary.LittleEndian.PutUint16(buf[18:20], uint16(srcY))
	binary.LittleEndian.PutUint16(buf[20:22], uint16(srcW))
	binary.LittleEndian.PutUint16(buf[22:24], uint16(srcH))
	binary.LittleEndian.PutUint16(buf[24:26], uint16(int16(dstX)))
	binary.LittleEndian.PutUint16(buf[26:28], uint16(int16(dstY)))
	buf[28] = 24 // depth
	buf[29] = 2  // format = ZPixmap
	if sendEvent {
		buf[30] = 1
	}
	// buf[31] — паддинг
	binary.LittleEndian.PutUint32(buf[32:36], shmseg)
	binary.LittleEndian.PutUint32(buf[36:40], offset)
	return buf
}

// x11SyncCheckError шлёт GetInputFocus (core-запрос с гарантированным reply)
// сразу вслед за потенциально отклоняемым fire-and-forget запросом (здесь —
// ShmAttach) и читает РОВНО один пакет: X-сервер гарантированно доставляет
// ошибки более раннего запроса раньше reply на более поздний (тот же приём,
// что и XSync). Если пришла ошибка — она относится к ShmAttach; ещё
// вычитываем следом сам reply GetInputFocus, чтобы не рассинхронизировать
// поток (см. предупреждение в x11InternAtom про readFull).
//
// Безопасно вызывать только там, где на соединении заведомо нет посторонних
// событий (см. x11ShmInit) — иначе можно перехватить чужое событие.
func (w *X11Window) x11SyncCheckError() bool {
	req := make([]byte, 4)
	req[0] = 43 // GetInputFocus
	binary.LittleEndian.PutUint16(req[2:4], 1)
	w.x11Send(req)

	pkt := make([]byte, 32)
	if !w.readFull(pkt) {
		return false
	}
	if pkt[0] == 0 { // Error — относится к предшествующему запросу (ShmAttach)
		w.readFull(pkt) // добираем reply GetInputFocus
		return false
	}
	return true
}
