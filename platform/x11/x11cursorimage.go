package x11

// x11cursorimage.go — X11: курсор из картинки (Engine.SetCursorImage), сборка
// запросов и разбор ответов RENDER.
//
// Файл БЕЗ платформенного суффикса — по той же причине, что x11touch.go:
// раскладку байтов можно проверять на любой машине. Порядок обмена и кэш — в
// x11cursorimage_linux.go.
//
// Как строится курсор с альфой. Ядро протокола знает только 1-битные курсоры;
// курсор с полупрозрачностью умеет расширение RENDER:
//
//  1. CreatePixmap глубины 32 нужного размера и CreateGC на нём;
//  2. PutImage (ZPixmap, глубина 32) — пиксели ARGB с домноженной альфой, байты
//     B,G,R,A, полосами не больше предела запроса;
//  3. RenderCreatePicture(pixmap, формат ARGB32) — картинка поверх пиксмапа;
//  4. RenderCreateCursor(picture, горячая точка);
//  5. картинка, GC и пиксмап больше не нужны (курсор держит свою копию).
//
// Формат ARGB32 (глубина 32, прямой, альфа в старших 8 битах) сервер называет
// сам: его идентификатор берут из QueryPictFormats. Нет RENDER, нет такого
// формата или версия ниже 0.5 — курсор строится 1-битным по порогу альфы
// (pointer.CursorMonoPoints), как «рука» и невидимый курсор в x11cursorbitmap.go.

import (
	"encoding/binary"

	"github.com/oops1/headless-gui/v3/platform/pointer"
	"github.com/oops1/headless-gui/v3/widget/base"
)

// Значения протокола RENDER.
const (
	renderExtensionName = "RENDER"

	renderMinorQueryVersion     = 0
	renderMinorQueryPictFormats = 1
	renderMinorCreatePicture    = 4
	renderMinorFreePicture      = 7
	renderMinorCreateCursor     = 27

	// Версия RENDER, с которой есть CreateCursor.
	renderCursorMajor = 0
	renderCursorMinor = 5

	// Доли запроса PutImage: потолок тела одного запроса. Максимум запроса без
	// BIG-REQUESTS — 262140 байт; берём с запасом.
	x11PutImageMaxData = 200000

	// Порог альфы монохромного курсора: пиксель виден, если альфа не меньше.
	x11MonoAlphaThreshold = 128
)

// x11RenderQueryVersionRequest — RenderQueryVersion(0, 11).
func x11RenderQueryVersionRequest(major byte) []byte {
	buf := make([]byte, 12)
	buf[0] = major
	buf[1] = renderMinorQueryVersion
	binary.LittleEndian.PutUint16(buf[2:4], 3)
	binary.LittleEndian.PutUint32(buf[4:8], 0)
	binary.LittleEndian.PutUint32(buf[8:12], 11)
	return buf
}

// parseRenderVersion разбирает ответ RenderQueryVersion: major@8, minor@12.
func parseRenderVersion(reply []byte) (major, minor int, ok bool) {
	if len(reply) < 16 || reply[0] != 1 {
		return 0, 0, false
	}
	return int(binary.LittleEndian.Uint32(reply[8:12])), int(binary.LittleEndian.Uint32(reply[12:16])), true
}

// renderVersionHasCursor — версия RENDER умеет CreateCursor (0.5 и выше).
func renderVersionHasCursor(major, minor int) bool {
	return major > renderCursorMajor || (major == renderCursorMajor && minor >= renderCursorMinor)
}

// x11RenderQueryPictFormatsRequest — RenderQueryPictFormats.
func x11RenderQueryPictFormatsRequest(major byte) []byte {
	buf := make([]byte, 4)
	buf[0] = major
	buf[1] = renderMinorQueryPictFormats
	binary.LittleEndian.PutUint16(buf[2:4], 1)
	return buf
}

// parseRenderArgbFormat ищет в ответе QueryPictFormats формат ARGB32: прямой
// (Direct), глубина 32, красный сдвиг 16, зелёный 8, синий 0, альфа 24, маски по
// 0xFF. 0 — такого формата нет или ответ обрезан.
//
// Ответ: заголовок 32 байта (num_formats@8), затем PICTFORMINFO по 28 байт:
// id(4) type(1) depth(1) pad(2), red shift/mask, green shift/mask, blue
// shift/mask, alpha shift/mask (по 2 байта каждое), colormap(4).
func parseRenderArgbFormat(reply []byte) uint32 {
	if len(reply) < 32 || reply[0] != 1 {
		return 0
	}
	n := int(binary.LittleEndian.Uint32(reply[8:12]))
	const infoSize = 28
	for i := 0; i < n; i++ {
		off := 32 + i*infoSize
		if off+infoSize > len(reply) {
			return 0
		}
		f := reply[off : off+infoSize]
		if f[4] != 1 /* Direct */ || f[5] != 32 {
			continue
		}
		u16 := func(o int) uint16 { return binary.LittleEndian.Uint16(f[o : o+2]) }
		if u16(8) == 16 && u16(10) == 0xFF && // red shift, mask
			u16(12) == 8 && u16(14) == 0xFF && // green
			u16(16) == 0 && u16(18) == 0xFF && // blue
			u16(20) == 24 && u16(22) == 0xFF { // alpha
			return binary.LittleEndian.Uint32(f[0:4])
		}
	}
	return 0
}

// x11CreatePixmapDepthReq — CreatePixmap заданной глубины.
func x11CreatePixmapDepthReq(pid, drawable uint32, depth byte, w, h int) []byte {
	buf := x11CreatePixmapReq(pid, drawable, w, h)
	buf[1] = depth
	return buf
}

// x11PutImageRequests режет картинку (ZPixmap, строка за строкой, 4 байта на
// пиксель, глубина depth) на запросы PutImage так, чтобы тело каждого не
// превышало x11PutImageMaxData.
func x11PutImageRequests(drawable, gc uint32, w, h int, depth byte, data []byte) [][]byte {
	rowBytes := w * 4
	if w <= 0 || h <= 0 || len(data) < rowBytes*h {
		return nil
	}
	maxRows := x11PutImageMaxData / rowBytes
	if maxRows < 1 {
		maxRows = 1
	}
	var reqs [][]byte
	for y := 0; y < h; y += maxRows {
		rows := maxRows
		if y+rows > h {
			rows = h - y
		}
		dataLen := rows * rowBytes // кратно 4: выравнивание не нужно
		buf := make([]byte, 24+dataLen)
		buf[0] = 72 // PutImage
		buf[1] = 2  // ZPixmap
		binary.LittleEndian.PutUint16(buf[2:4], uint16(len(buf)/4))
		binary.LittleEndian.PutUint32(buf[4:8], drawable)
		binary.LittleEndian.PutUint32(buf[8:12], gc)
		binary.LittleEndian.PutUint16(buf[12:14], uint16(w))
		binary.LittleEndian.PutUint16(buf[14:16], uint16(rows))
		binary.LittleEndian.PutUint16(buf[18:20], uint16(y)) // dst-y; dst-x = 0
		buf[21] = depth
		copy(buf[24:], data[y*rowBytes:y*rowBytes+dataLen])
		reqs = append(reqs, buf)
	}
	return reqs
}

// x11RenderCreatePictureRequest — RenderCreatePicture без значений атрибутов.
func x11RenderCreatePictureRequest(major byte, pid, drawable, format uint32) []byte {
	buf := make([]byte, 20)
	buf[0] = major
	buf[1] = renderMinorCreatePicture
	binary.LittleEndian.PutUint16(buf[2:4], 5)
	binary.LittleEndian.PutUint32(buf[4:8], pid)
	binary.LittleEndian.PutUint32(buf[8:12], drawable)
	binary.LittleEndian.PutUint32(buf[12:16], format)
	// value-mask (buf[16:20]) = 0
	return buf
}

// x11RenderCreateCursorRequest — RenderCreateCursor: курсор cid из картинки src
// с горячей точкой (x, y).
func x11RenderCreateCursorRequest(major byte, cid, src uint32, x, y int) []byte {
	buf := make([]byte, 16)
	buf[0] = major
	buf[1] = renderMinorCreateCursor
	binary.LittleEndian.PutUint16(buf[2:4], 4)
	binary.LittleEndian.PutUint32(buf[4:8], cid)
	binary.LittleEndian.PutUint32(buf[8:12], src)
	binary.LittleEndian.PutUint16(buf[12:14], uint16(x))
	binary.LittleEndian.PutUint16(buf[14:16], uint16(y))
	return buf
}

// x11RenderFreePictureRequest — RenderFreePicture.
func x11RenderFreePictureRequest(major byte, pid uint32) []byte {
	buf := make([]byte, 8)
	buf[0] = major
	buf[1] = renderMinorFreePicture
	binary.LittleEndian.PutUint16(buf[2:4], 2)
	binary.LittleEndian.PutUint32(buf[4:8], pid)
	return buf
}

// x11RenderCursorIDs — идентификаторы ресурсов, нужных на время сборки курсора
// с альфой.
type x11RenderCursorIDs struct {
	cursor   uint32 // итоговый курсор
	drawable uint32 // окно: по нему сервер узнаёт экран для пиксмапа
	pixmap   uint32
	gc       uint32
	picture  uint32
}

// x11RenderCursorRequests собирает запросы курсора с альфой из картинки ci
// (RENDER major — код расширения, argb — формат ARGB32 сервера).
func x11RenderCursorRequests(ids x11RenderCursorIDs, major byte, argb uint32, ci *base.CursorImage) [][]byte {
	reqs := [][]byte{
		x11CreatePixmapDepthReq(ids.pixmap, ids.drawable, 32, ci.W, ci.H),
		x11CreateGCReq(ids.gc, ids.pixmap, 0),
	}
	reqs = append(reqs, x11PutImageRequests(ids.pixmap, ids.gc, ci.W, ci.H, 32, pointer.CursorPremulBGRA(ci))...)
	reqs = append(reqs,
		x11RenderCreatePictureRequest(major, ids.picture, ids.pixmap, argb),
		x11RenderCreateCursorRequest(major, ids.cursor, ids.picture, ci.HotX, ci.HotY),
		x11RenderFreePictureRequest(major, ids.picture),
		x11FreeReq(x11OpFreeGC, ids.gc),
		x11FreeReq(x11OpFreePixmap, ids.pixmap),
	)
	return reqs
}

// x11MonoImageCursorRequests собирает запросы 1-битного курсора из картинки —
// запасной путь без RENDER: пиксель виден при альфе не меньше порога, тёмный
// рисуется чёрным, светлый — белым.
func x11MonoImageCursorRequests(ids x11CursorIDs, ci *base.CursorImage) [][]byte {
	src, mask := pointer.CursorMonoPoints(ci, x11MonoAlphaThreshold)
	return x11MonoCursorRequests(ids, ci.W, ci.H, src, mask, ci.HotX, ci.HotY)
}
