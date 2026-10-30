package x11

import "encoding/binary"

// x11argb.go — окна-попапы X11 с альфа-каналом: разбор visual'ов экрана и
// сборка запросов CreateColormap/FreeColormap/CreateWindow.
//
// Файл БЕЗ платформенного суффикса — по той же причине, что x11touch.go:
// раскладку байтов можно проверять на любой машине. Сокет и порядок обмена —
// в x11argb_linux.go.
//
// Зачем. Окно X11 по умолчанию создаётся с глубиной и visual корневого окна —
// 24 бита без альфы. Картинка вынесенного оверлея (меню, списки) несёт
// прозрачную тень и прозрачные точки за скруглением углов; окно без
// альфа-канала показывает их ЧЁРНЫМИ: чёрная рамка вокруг меню и чёрные углы.
// Лечится 32-битным TrueColor visual с масками ARGB: композитор смешивает такое
// окно с тем, что под ним, по альфе (тот же приём, что у xdg_popup в ARGB8888).

const (
	// x11VisualTrueColor — класс visual'а (X.h: TrueColor = 4).
	x11VisualTrueColor = 4
	// Маски цвета ARGB8888: байты в памяти B, G, R, A (little-endian).
	x11ArgbRedMask   = 0x00FF0000
	x11ArgbGreenMask = 0x0000FF00
	x11ArgbBlueMask  = 0x000000FF
	// x11ArgbDepth — глубина окна с альфой.
	x11ArgbDepth = 32

	// Биты value-mask CreateWindow (X.h).
	x11CWBackPixel     = 1 << 1
	x11CWBorderPixel   = 1 << 3
	x11CWOverrideRedir = 1 << 9
	x11CWEventMask     = 1 << 11
	x11CWColormap      = 1 << 13
)

// x11FindArgbVisual ищет среди глубин экрана TrueColor visual глубины 32 с
// масками R=0xFF0000, G=0x00FF00, B=0x0000FF (ARGB). data — список глубин
// (STRUCT DEPTH) сразу за заголовком экрана из ответа на установление
// соединения, n — число глубин (allowed-depths-count экрана). 0 — такого
// visual'а нет (или список обрезан).
//
// Формат: DEPTH — depth(1) pad(1) n-visuals(2) pad(4), затем n-visuals по
// VISUALTYPE: visual-id(4) class(1) bits-per-rgb(1) colormap-entries(2)
// red-mask(4) green-mask(4) blue-mask(4) pad(4) — 24 байта.
func x11FindArgbVisual(data []byte, n int) uint32 {
	off := 0
	for i := 0; i < n; i++ {
		if off+8 > len(data) {
			return 0
		}
		depth := data[off]
		nVis := int(binary.LittleEndian.Uint16(data[off+2 : off+4]))
		off += 8
		for j := 0; j < nVis; j++ {
			if off+24 > len(data) {
				return 0
			}
			v := data[off : off+24]
			off += 24
			if depth != x11ArgbDepth || v[4] != x11VisualTrueColor {
				continue
			}
			if binary.LittleEndian.Uint32(v[8:12]) == x11ArgbRedMask &&
				binary.LittleEndian.Uint32(v[12:16]) == x11ArgbGreenMask &&
				binary.LittleEndian.Uint32(v[16:20]) == x11ArgbBlueMask {
				return binary.LittleEndian.Uint32(v[0:4])
			}
		}
	}
	return 0
}

// x11CreateColormapRequest — запрос CreateColormap (opcode 78, 4 слова):
// alloc(1 байт, 0 = None) в байте 1, mid, window (окно, чей экран определяет
// colormap — корневое), visual. Colormap нужен окну с visual'ом, отличным от
// родительского (иначе BadMatch).
func x11CreateColormapRequest(mid, window, visual uint32) []byte {
	buf := make([]byte, 16)
	buf[0] = 78 // CreateColormap
	buf[1] = 0  // alloc = None
	binary.LittleEndian.PutUint16(buf[2:4], 4)
	binary.LittleEndian.PutUint32(buf[4:8], mid)
	binary.LittleEndian.PutUint32(buf[8:12], window)
	binary.LittleEndian.PutUint32(buf[12:16], visual)
	return buf
}

// x11FreeColormapRequest — запрос FreeColormap (opcode 79, 2 слова).
func x11FreeColormapRequest(cmap uint32) []byte {
	buf := make([]byte, 8)
	buf[0] = 79 // FreeColormap
	binary.LittleEndian.PutUint16(buf[2:4], 2)
	binary.LittleEndian.PutUint32(buf[4:8], cmap)
	return buf
}

// x11ArgbPopupAttrs возвращает value-mask и значения CreateWindow для
// override-redirect попапа с альфой. Значения идут строго по возрастанию бит
// маски: BackPixel, BorderPixel, OverrideRedirect, EventMask, Colormap.
//
// BackPixel=0 — прозрачный чёрный (на 32 битах это прозрачность, а не
// чернота). BorderPixel обязателен: при visual, отличном от родительского,
// без него CreateWindow даёт BadMatch (граница по умолчанию — CopyFromParent
// — берёт пиксель родителя другой глубины).
func x11ArgbPopupAttrs(eventMask, colormap uint32) (valueMask uint32, values []uint32) {
	valueMask = x11CWBackPixel | x11CWBorderPixel | x11CWOverrideRedir | x11CWEventMask | x11CWColormap
	values = []uint32{
		0, // BackPixel
		0, // BorderPixel
		1, // OverrideRedirect = true
		eventMask,
		colormap,
	}
	return
}
