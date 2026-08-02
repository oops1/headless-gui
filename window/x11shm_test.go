//go:build linux && !android

package window

import (
	"encoding/binary"
	"image"
	"testing"
)

// TestX11QueryShmExtensionRequest проверяет длину и поля QueryExtension("MIT-SHM"):
// opcode=98, name-length=7, само имя без искажений, паддинг до кратности 4.
func TestX11QueryShmExtensionRequest(t *testing.T) {
	buf := x11QueryShmExtensionRequest()

	if len(buf)%4 != 0 {
		t.Fatalf("длина запроса должна быть кратна 4, got %d", len(buf))
	}
	if buf[0] != 98 {
		t.Fatalf("opcode: got %d, want 98 (QueryExtension)", buf[0])
	}
	reqLen := binary.LittleEndian.Uint16(buf[2:4])
	if int(reqLen)*4 != len(buf) {
		t.Fatalf("length-поле не совпадает с фактической длиной: length=%d слов, буфер=%d байт", reqLen, len(buf))
	}
	nameLen := binary.LittleEndian.Uint16(buf[4:6])
	if nameLen != 7 {
		t.Fatalf("name-length: got %d, want 7 (\"MIT-SHM\")", nameLen)
	}
	name := string(buf[8 : 8+nameLen])
	if name != "MIT-SHM" {
		t.Fatalf("имя расширения: got %q, want %q", name, "MIT-SHM")
	}
}

// TestParseQueryExtensionReply проверяет разбор reply: present/major/first-event
// и отказ на коротком/не-reply пакете.
func TestParseQueryExtensionReply(t *testing.T) {
	reply := make([]byte, 32)
	reply[0] = 1  // Reply
	reply[8] = 1  // present = true
	reply[9] = 130 // major-opcode
	reply[10] = 87 // first-event

	major, firstEvent, present := parseQueryExtensionReply(reply)
	if !present || major != 130 || firstEvent != 87 {
		t.Fatalf("got (major=%d, firstEvent=%d, present=%v), want (130, 87, true)", major, firstEvent, present)
	}

	reply[8] = 0 // расширение отсутствует
	if _, _, present := parseQueryExtensionReply(reply); present {
		t.Fatalf("present=false ожидался при отсутствующем расширении")
	}

	errPkt := make([]byte, 32) // type=0 — Error, не reply
	if _, _, present := parseQueryExtensionReply(errPkt); present {
		t.Fatalf("Error-пакет не должен парситься как успешный reply")
	}

	if _, _, present := parseQueryExtensionReply(make([]byte, 4)); present {
		t.Fatalf("короткий буфер не должен парситься")
	}
}

// TestX11ShmQueryVersionRequest — тело запроса из одного 4-байтового слова
// (заголовок расширения, без дополнительных полей).
func TestX11ShmQueryVersionRequest(t *testing.T) {
	buf := x11ShmQueryVersionRequest(130)
	if len(buf) != 4 {
		t.Fatalf("длина: got %d, want 4", len(buf))
	}
	if buf[0] != 130 {
		t.Fatalf("major-opcode: got %d, want 130", buf[0])
	}
	if buf[1] != shmMinorQueryVersion {
		t.Fatalf("minor-opcode: got %d, want %d (ShmQueryVersion)", buf[1], shmMinorQueryVersion)
	}
	if l := binary.LittleEndian.Uint16(buf[2:4]); l != 1 {
		t.Fatalf("length: got %d слов, want 1", l)
	}
}

// TestX11ShmAttachRequest проверяет длину (4 слова = 16 байт), порядок полей
// shmseg/shmid и байт read-only.
func TestX11ShmAttachRequest(t *testing.T) {
	buf := x11ShmAttachRequest(130, 0xABCDEF01, 0x12345678, false)
	if len(buf) != 16 {
		t.Fatalf("длина: got %d, want 16 (4 слова)", len(buf))
	}
	if buf[0] != 130 || buf[1] != shmMinorAttach {
		t.Fatalf("opcode/minor: got (%d,%d), want (130,%d)", buf[0], buf[1], shmMinorAttach)
	}
	if l := binary.LittleEndian.Uint16(buf[2:4]); l != 4 {
		t.Fatalf("length: got %d слов, want 4", l)
	}
	if v := binary.LittleEndian.Uint32(buf[4:8]); v != 0xABCDEF01 {
		t.Fatalf("shmseg: got %#x, want %#x", v, uint32(0xABCDEF01))
	}
	if v := binary.LittleEndian.Uint32(buf[8:12]); v != 0x12345678 {
		t.Fatalf("shmid: got %#x, want %#x", v, uint32(0x12345678))
	}
	if buf[12] != 0 {
		t.Fatalf("read-only: got %d, want 0", buf[12])
	}

	buf = x11ShmAttachRequest(130, 1, 2, true)
	if buf[12] != 1 {
		t.Fatalf("read-only=true: got byte %d, want 1", buf[12])
	}
}

// TestX11ShmDetachRequest — тело запроса из 2 слов (заголовок + shmseg).
func TestX11ShmDetachRequest(t *testing.T) {
	buf := x11ShmDetachRequest(130, 0xDEADBEEF)
	if len(buf) != 8 {
		t.Fatalf("длина: got %d, want 8 (2 слова)", len(buf))
	}
	if buf[0] != 130 || buf[1] != shmMinorDetach {
		t.Fatalf("opcode/minor: got (%d,%d), want (130,%d)", buf[0], buf[1], shmMinorDetach)
	}
	if l := binary.LittleEndian.Uint16(buf[2:4]); l != 2 {
		t.Fatalf("length: got %d слов, want 2", l)
	}
	if v := binary.LittleEndian.Uint32(buf[4:8]); v != 0xDEADBEEF {
		t.Fatalf("shmseg: got %#x, want %#x", v, uint32(0xDEADBEEF))
	}
}

// TestX11ShmPutImageRequest проверяет длину (10 слов = 40 байт) и порядок ВСЕХ
// полей, перечисленных в протоколе MIT-SHM 1.2 для ShmPutImage (minor=3).
func TestX11ShmPutImageRequest(t *testing.T) {
	buf := x11ShmPutImageRequest(130, 0x01000001, 0x01000002,
		640, 480, // totalWidth, totalHeight
		10, 20, 100, 50, // srcX, srcY, srcW, srcH
		15, 25, // dstX, dstY
		true, 0x01000003, 0x1000)

	if len(buf) != 40 {
		t.Fatalf("длина: got %d, want 40 (10 слов)", len(buf))
	}
	if buf[0] != 130 || buf[1] != shmMinorPutImage {
		t.Fatalf("opcode/minor: got (%d,%d), want (130,%d)", buf[0], buf[1], shmMinorPutImage)
	}
	if l := binary.LittleEndian.Uint16(buf[2:4]); l != 10 {
		t.Fatalf("length: got %d слов, want 10", l)
	}
	if v := binary.LittleEndian.Uint32(buf[4:8]); v != 0x01000001 {
		t.Fatalf("drawable: got %#x, want %#x", v, uint32(0x01000001))
	}
	if v := binary.LittleEndian.Uint32(buf[8:12]); v != 0x01000002 {
		t.Fatalf("gc: got %#x, want %#x", v, uint32(0x01000002))
	}
	if v := binary.LittleEndian.Uint16(buf[12:14]); v != 640 {
		t.Fatalf("totalWidth: got %d, want 640", v)
	}
	if v := binary.LittleEndian.Uint16(buf[14:16]); v != 480 {
		t.Fatalf("totalHeight: got %d, want 480", v)
	}
	if v := binary.LittleEndian.Uint16(buf[16:18]); v != 10 {
		t.Fatalf("srcX: got %d, want 10", v)
	}
	if v := binary.LittleEndian.Uint16(buf[18:20]); v != 20 {
		t.Fatalf("srcY: got %d, want 20", v)
	}
	if v := binary.LittleEndian.Uint16(buf[20:22]); v != 100 {
		t.Fatalf("srcW: got %d, want 100", v)
	}
	if v := binary.LittleEndian.Uint16(buf[22:24]); v != 50 {
		t.Fatalf("srcH: got %d, want 50", v)
	}
	if v := int16(binary.LittleEndian.Uint16(buf[24:26])); v != 15 {
		t.Fatalf("dstX: got %d, want 15", v)
	}
	if v := int16(binary.LittleEndian.Uint16(buf[26:28])); v != 25 {
		t.Fatalf("dstY: got %d, want 25", v)
	}
	if buf[28] != 24 {
		t.Fatalf("depth: got %d, want 24", buf[28])
	}
	if buf[29] != 2 {
		t.Fatalf("format: got %d, want 2 (ZPixmap)", buf[29])
	}
	if buf[30] != 1 {
		t.Fatalf("send-event: got %d, want 1", buf[30])
	}
	if v := binary.LittleEndian.Uint32(buf[32:36]); v != 0x01000003 {
		t.Fatalf("shmseg: got %#x, want %#x", v, uint32(0x01000003))
	}
	if v := binary.LittleEndian.Uint32(buf[36:40]); v != 0x1000 {
		t.Fatalf("offset: got %#x, want %#x", v, uint32(0x1000))
	}

	// sendEvent=false — байт 30 обязан остаться нулевым.
	buf = x11ShmPutImageRequest(130, 1, 2, 1, 1, 0, 0, 1, 1, 0, 0, false, 3, 0)
	if buf[30] != 0 {
		t.Fatalf("send-event=false: байт 30 = %d, want 0", buf[30])
	}

	// Отрицательные dstX/dstY (окно частично докрашивается за левым/верхним
	// краем) обязаны кодироваться как INT16, а не переполняться в CARD16.
	buf = x11ShmPutImageRequest(130, 1, 2, 100, 100, 0, 0, 10, 10, -5, -7, false, 3, 0)
	if v := int16(binary.LittleEndian.Uint16(buf[24:26])); v != -5 {
		t.Fatalf("отрицательный dstX: got %d, want -5", v)
	}
	if v := int16(binary.LittleEndian.Uint16(buf[26:28])); v != -7 {
		t.Fatalf("отрицательный dstY: got %d, want -7", v)
	}
}

// TestX11ShmBlitFallsBackWhenNil — без инициализированного SHM (shm == nil,
// как сразу после zero-value X11Window) x11ShmBlit обязан вернуть false, не
// трогая никакие поля и не паникуя, чтобы BlitRGBADirty ушёл в PutImage.
func TestX11ShmBlitFallsBackWhenNil(t *testing.T) {
	w := &X11Window{}
	img := newTestRGBA(4, 4)
	if w.x11ShmBlit(img, img.Bounds()) {
		t.Fatalf("x11ShmBlit(shm=nil) должен вернуть false")
	}
}

// TestX11ShmBlitFallsBackWhenFallback — shm != nil, но fallback=true (SHM не
// прошёл инициализацию) — тоже должен молча отказать.
func TestX11ShmBlitFallsBackWhenFallback(t *testing.T) {
	w := &X11Window{shm: &x11ShmState{fallback: true}}
	img := newTestRGBA(4, 4)
	if w.x11ShmBlit(img, img.Bounds()) {
		t.Fatalf("x11ShmBlit(fallback=true) должен вернуть false")
	}
}

// TestX11ShmBlitBusySkipsFrame — пока предыдущий кадр не подтверждён сервером
// (busy=true), x11ShmBlit не должен пытаться отправлять новый — вызывающий
// (BlitRGBADirty) обязан сам сделать PutImage за этот кадр, без ожидания.
func TestX11ShmBlitBusySkipsFrame(t *testing.T) {
	shm := &x11ShmState{width: 4, height: 4, data: make([]byte, 4*4*4)}
	shm.busy.Store(true)
	w := &X11Window{shm: shm}
	img := newTestRGBA(4, 4)

	if w.x11ShmBlit(img, img.Bounds()) {
		t.Fatalf("x11ShmBlit при busy=true должен вернуть false и не слать запрос")
	}
}

// TestX11ShmBlitSizeMismatchFallsBack — размер кадра разошёлся с размером
// сегмента (гонка с ресайзом, сегмент ещё не пересоздан/пересоздаётся) —
// нельзя писать в сегмент по чужой геометрии, обязан быть fallback на PutImage.
func TestX11ShmBlitSizeMismatchFallsBack(t *testing.T) {
	shm := &x11ShmState{width: 4, height: 4, data: make([]byte, 4*4*4)}
	w := &X11Window{shm: shm}
	img := newTestRGBA(8, 8) // не совпадает с shm.width/height

	if w.x11ShmBlit(img, img.Bounds()) {
		t.Fatalf("x11ShmBlit при несовпадении размеров должен вернуть false")
	}
	if shm.busy.Load() {
		t.Fatalf("busy не должен взводиться, если кадр отклонён до отправки")
	}
}

// newTestRGBA — маленькое тестовое RGBA-изображение размером w×h.
func newTestRGBA(w, h int) *image.RGBA {
	return image.NewRGBA(image.Rect(0, 0, w, h))
}
