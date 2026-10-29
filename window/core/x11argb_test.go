package core

import (
	"encoding/binary"
	"testing"
)

// argbDepth собирает STRUCT DEPTH с заданными visual'ами (по 24 байта).
func argbDepth(depth byte, visuals ...[]byte) []byte {
	out := make([]byte, 8)
	out[0] = depth
	binary.LittleEndian.PutUint16(out[2:4], uint16(len(visuals)))
	for _, v := range visuals {
		out = append(out, v...)
	}
	return out
}

// argbVisual собирает STRUCT VISUALTYPE.
func argbVisual(id uint32, class byte, r, g, b uint32) []byte {
	v := make([]byte, 24)
	binary.LittleEndian.PutUint32(v[0:4], id)
	v[4] = class
	v[5] = 8
	binary.LittleEndian.PutUint16(v[6:8], 256)
	binary.LittleEndian.PutUint32(v[8:12], r)
	binary.LittleEndian.PutUint32(v[12:16], g)
	binary.LittleEndian.PutUint32(v[16:20], b)
	return v
}

func catBytes(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// TestX11FindArgbVisual_PicksTrueColorArgb: из списка глубин берётся именно
// TrueColor глубины 32 с масками ARGB — не другой класс, не другие маски, не
// тот же набор масков на глубине 24.
func TestX11FindArgbVisual_PicksTrueColorArgb(t *testing.T) {
	data := catBytes(
		argbDepth(1),
		argbDepth(24,
			argbVisual(0x21, 4, 0xFF0000, 0x00FF00, 0x0000FF), // маски ARGB, но глубина 24
		),
		argbDepth(32,
			argbVisual(0x50, 5, 0xFF0000, 0x00FF00, 0x0000FF), // DirectColor
			argbVisual(0x60, 4, 0x0000FF, 0x00FF00, 0xFF0000), // TrueColor, но ABGR
			argbVisual(0x77, 4, 0xFF0000, 0x00FF00, 0x0000FF), // то, что нужно
			argbVisual(0x78, 4, 0xFF0000, 0x00FF00, 0x0000FF), // второй не берём
		),
	)
	if got := x11FindArgbVisual(data, 3); got != 0x77 {
		t.Fatalf("visual = %#x, want 0x77", got)
	}
}

// TestX11FindArgbVisual_NoneCases: нет подходящего — 0 (откат на 24 бита).
func TestX11FindArgbVisual_NoneCases(t *testing.T) {
	only24 := catBytes(argbDepth(24, argbVisual(0x21, 4, 0xFF0000, 0x00FF00, 0x0000FF)))
	if got := x11FindArgbVisual(only24, 1); got != 0 {
		t.Errorf("только глубина 24: visual = %#x, want 0", got)
	}
	wrongMasks := catBytes(argbDepth(32, argbVisual(0x60, 4, 0x0000FF, 0x00FF00, 0xFF0000)))
	if got := x11FindArgbVisual(wrongMasks, 1); got != 0 {
		t.Errorf("маски не ARGB: visual = %#x, want 0", got)
	}
	if got := x11FindArgbVisual(nil, 0); got != 0 {
		t.Errorf("пустой список: visual = %#x, want 0", got)
	}
	// Обрезанный список (счётчик больше данных) — без паники и 0.
	argb := catBytes(argbDepth(32, argbVisual(0x77, 4, 0xFF0000, 0x00FF00, 0x0000FF)))
	if got := x11FindArgbVisual(argb[:len(argb)-1], 1); got != 0 {
		t.Errorf("обрезанный visual: %#x, want 0", got)
	}
	if got := x11FindArgbVisual(argb[:5], 1); got != 0 {
		t.Errorf("обрезанный заголовок: %#x, want 0", got)
	}
	if got := x11FindArgbVisual(argb, 5); got != 0x77 {
		t.Errorf("счётчик глубин больше списка не мешает найденному: %#x, want 0x77", got)
	}
}

func TestX11CreateColormapRequest(t *testing.T) {
	b := x11CreateColormapRequest(0x400002, 0x100, 0x77)
	if len(b) != 16 || b[0] != 78 || b[1] != 0 || binary.LittleEndian.Uint16(b[2:4]) != 4 {
		t.Fatalf("заголовок CreateColormap: % x", b)
	}
	if binary.LittleEndian.Uint32(b[4:8]) != 0x400002 ||
		binary.LittleEndian.Uint32(b[8:12]) != 0x100 ||
		binary.LittleEndian.Uint32(b[12:16]) != 0x77 {
		t.Fatalf("mid/window/visual: % x", b)
	}
}

func TestX11FreeColormapRequest(t *testing.T) {
	b := x11FreeColormapRequest(0x400002)
	if len(b) != 8 || b[0] != 79 || binary.LittleEndian.Uint16(b[2:4]) != 2 ||
		binary.LittleEndian.Uint32(b[4:8]) != 0x400002 {
		t.Fatalf("FreeColormap: % x", b)
	}
}

// TestX11ArgbPopupAttrs: маска и порядок значений. Сервер сопоставляет
// значения битам маски по возрастанию: перепутанный порядок даёт BadMatch/
// BadValue, а отсутствие BorderPixel при visual≠родительского — BadMatch.
func TestX11ArgbPopupAttrs(t *testing.T) {
	const ev, cm = 0x8067, 0x400009
	mask, vals := x11ArgbPopupAttrs(ev, cm)
	if want := uint32(0x2 | 0x8 | 0x200 | 0x800 | 0x2000); mask != want {
		t.Fatalf("маска %#x, want %#x", mask, want)
	}
	want := []uint32{0, 0, 1, ev, cm} // BackPixel, BorderPixel, OverrideRedirect, EventMask, Colormap
	if len(vals) != len(want) {
		t.Fatalf("значений %d, want %d", len(vals), len(want))
	}
	for i := range want {
		if vals[i] != want[i] {
			t.Errorf("значение %d = %#x, want %#x", i, vals[i], want[i])
		}
	}
}
