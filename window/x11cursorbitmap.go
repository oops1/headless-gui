package window

// x11cursorbitmap.go — курсоры X11 из 1-битных карт, без шрифта «cursor».
//
// В шрифте нет ни невидимого курсора, ни руки (раскрытой и сжатой), поэтому
// такие формы строятся протоколом ядра: две карты глубины 1 (рисунок и маска),
// из них CreateCursor. Расширение RENDER не нужно — работает на любом сервере.
//
// Файл без платформенного суффикса: запросы собираются чистой функцией, и
// тесты идут в общем прогоне.

import "encoding/binary"

// x11CursorIDs — идентификаторы ресурсов, нужных на время сборки курсора.
type x11CursorIDs struct {
	cursor   uint32 // итоговый курсор
	drawable uint32 // окно: по нему сервер узнаёт экран для карт
	pixSrc   uint32 // карта рисунка: 1 — пиксель закрашен цветом «fore»
	pixMask  uint32 // карта маски: 1 — пиксель курсора виден
	gc0, gc1 uint32 // контексты с цветом 0 (очистка) и 1 (рисование)
}

// Коды запросов ядра X11.
const (
	x11OpCreatePixmap      = 53
	x11OpFreePixmap        = 54
	x11OpCreateGC          = 55
	x11OpFreeGC            = 60
	x11OpPolyPoint         = 64
	x11OpPolyFillRectangle = 70
	x11OpCreateCursor      = 93
)

// x11BitmapCursorRequests собирает запросы, создающие курсор формы c из
// 1-битных карт. CursorNone — карта 1×1 с нулевой маской, то есть невидимый
// курсор; нарисованные формы берут рисунок из wlCursorImage: чёрный пиксель —
// контур (fore), белый — заливка (back), прозрачный — вне маски.
func x11BitmapCursorRequests(ids x11CursorIDs, c int) [][]byte {
	w, h := 1, 1
	var src, mask [][2]int16
	hotX, hotY := 0, 0
	if c != wlCursorNone {
		pic, hx, hy := wlCursorImage(c)
		b := pic.Bounds()
		w, h, hotX, hotY = b.Dx(), b.Dy(), hx, hy
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := pic.PixOffset(b.Min.X+x, b.Min.Y+y)
				if pic.Pix[i+3] == 0 {
					continue
				}
				mask = append(mask, [2]int16{int16(x), int16(y)})
				if pic.Pix[i] == 0 && pic.Pix[i+1] == 0 && pic.Pix[i+2] == 0 {
					src = append(src, [2]int16{int16(x), int16(y)})
				}
			}
		}
	}

	reqs := [][]byte{
		x11CreatePixmapReq(ids.pixSrc, ids.drawable, w, h),
		x11CreatePixmapReq(ids.pixMask, ids.drawable, w, h),
		x11CreateGCReq(ids.gc0, ids.pixSrc, 0),
		x11CreateGCReq(ids.gc1, ids.pixSrc, 1),
		// Содержимое новой карты не определено: сначала очищаем.
		x11PolyFillRectangleReq(ids.pixSrc, ids.gc0, w, h),
		x11PolyFillRectangleReq(ids.pixMask, ids.gc0, w, h),
	}
	if len(src) > 0 {
		reqs = append(reqs, x11PolyPointReq(ids.pixSrc, ids.gc1, src))
	}
	if len(mask) > 0 {
		reqs = append(reqs, x11PolyPointReq(ids.pixMask, ids.gc1, mask))
	}
	reqs = append(reqs,
		x11CreateCursorReq(ids.cursor, ids.pixSrc, ids.pixMask, hotX, hotY),
		// Курсор держит свои копии: карты и контексты больше не нужны.
		x11FreeReq(x11OpFreeGC, ids.gc0),
		x11FreeReq(x11OpFreeGC, ids.gc1),
		x11FreeReq(x11OpFreePixmap, ids.pixSrc),
		x11FreeReq(x11OpFreePixmap, ids.pixMask),
	)
	return reqs
}

// x11CreatePixmapReq — CreatePixmap глубины 1.
func x11CreatePixmapReq(pid, drawable uint32, w, h int) []byte {
	buf := make([]byte, 16)
	buf[0] = x11OpCreatePixmap
	buf[1] = 1 // depth
	binary.LittleEndian.PutUint16(buf[2:4], 4)
	binary.LittleEndian.PutUint32(buf[4:8], pid)
	binary.LittleEndian.PutUint32(buf[8:12], drawable)
	binary.LittleEndian.PutUint16(buf[12:14], uint16(w))
	binary.LittleEndian.PutUint16(buf[14:16], uint16(h))
	return buf
}

// x11CreateGCReq — CreateGC с одним значением: foreground.
func x11CreateGCReq(gc, drawable, foreground uint32) []byte {
	const gcForeground = 0x00000004
	buf := make([]byte, 20)
	buf[0] = x11OpCreateGC
	binary.LittleEndian.PutUint16(buf[2:4], 5)
	binary.LittleEndian.PutUint32(buf[4:8], gc)
	binary.LittleEndian.PutUint32(buf[8:12], drawable)
	binary.LittleEndian.PutUint32(buf[12:16], gcForeground)
	binary.LittleEndian.PutUint32(buf[16:20], foreground)
	return buf
}

// x11PolyFillRectangleReq закрашивает карту целиком цветом контекста.
func x11PolyFillRectangleReq(drawable, gc uint32, w, h int) []byte {
	buf := make([]byte, 20)
	buf[0] = x11OpPolyFillRectangle
	binary.LittleEndian.PutUint16(buf[2:4], 5)
	binary.LittleEndian.PutUint32(buf[4:8], drawable)
	binary.LittleEndian.PutUint32(buf[8:12], gc)
	// x, y = 0 (buf[12:16])
	binary.LittleEndian.PutUint16(buf[16:18], uint16(w))
	binary.LittleEndian.PutUint16(buf[18:20], uint16(h))
	return buf
}

// x11PolyPointReq рисует точки цветом контекста (координаты от начала карты).
func x11PolyPointReq(drawable, gc uint32, pts [][2]int16) []byte {
	buf := make([]byte, 12+4*len(pts))
	buf[0] = x11OpPolyPoint
	buf[1] = 0 // CoordModeOrigin
	binary.LittleEndian.PutUint16(buf[2:4], uint16(len(buf)/4))
	binary.LittleEndian.PutUint32(buf[4:8], drawable)
	binary.LittleEndian.PutUint32(buf[8:12], gc)
	for i, p := range pts {
		binary.LittleEndian.PutUint16(buf[12+4*i:], uint16(p[0]))
		binary.LittleEndian.PutUint16(buf[14+4*i:], uint16(p[1]))
	}
	return buf
}

// x11CreateCursorReq — CreateCursor (opcode 93): рисунок чёрный (fore),
// фон белый (back), как у курсоров шрифта «cursor».
func x11CreateCursorReq(cid, src, mask uint32, hotX, hotY int) []byte {
	buf := make([]byte, 32)
	buf[0] = x11OpCreateCursor
	binary.LittleEndian.PutUint16(buf[2:4], 8)
	binary.LittleEndian.PutUint32(buf[4:8], cid)
	binary.LittleEndian.PutUint32(buf[8:12], src)
	binary.LittleEndian.PutUint32(buf[12:16], mask)
	// fore-red/green/blue = 0 (чёрный), back-* = 0xFFFF (белый)
	binary.LittleEndian.PutUint16(buf[22:24], 0xFFFF)
	binary.LittleEndian.PutUint16(buf[24:26], 0xFFFF)
	binary.LittleEndian.PutUint16(buf[26:28], 0xFFFF)
	binary.LittleEndian.PutUint16(buf[28:30], uint16(hotX))
	binary.LittleEndian.PutUint16(buf[30:32], uint16(hotY))
	return buf
}

// x11FreeReq — FreePixmap / FreeGC: запрос из двух слов с одним ресурсом.
func x11FreeReq(opcode byte, id uint32) []byte {
	buf := make([]byte, 8)
	buf[0] = opcode
	binary.LittleEndian.PutUint16(buf[2:4], 2)
	binary.LittleEndian.PutUint32(buf[4:8], id)
	return buf
}
