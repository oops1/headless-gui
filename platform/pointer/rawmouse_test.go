package pointer

import (
	"encoding/binary"
	"testing"
)

// plRawMouse собирает RAWINPUT мыши с заголовком под указатель размером ptrSize.
func plRawMouse(ptrSize int, typ uint32, flags uint16, x, y int32) []byte {
	hdr := 8 + 2*ptrSize
	b := make([]byte, hdr+24)
	binary.LittleEndian.PutUint32(b[0:], typ)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)))
	binary.LittleEndian.PutUint16(b[hdr:], flags)
	binary.LittleEndian.PutUint32(b[hdr+12:], uint32(x))
	binary.LittleEndian.PutUint32(b[hdr+16:], uint32(y))
	return b
}

func TestParseRawMouse(t *testing.T) {
	for _, ptr := range []int{8, 4} {
		x, y, abs, ok := ParseRawMouse(plRawMouse(ptr, RIMTypeMouse, 0, 7, -3), ptr)
		if !ok || x != 7 || y != -3 || abs {
			t.Errorf("указатель %d: (%d,%d) abs=%v ok=%v, ждал (7,-3) относительное", ptr, x, y, abs, ok)
		}
		x, y, abs, ok = ParseRawMouse(plRawMouse(ptr, RIMTypeMouse, MouseMoveAbsolute, 30000, 65535), ptr)
		if !ok || x != 30000 || y != 65535 || !abs {
			t.Errorf("указатель %d: абсолютное (%d,%d) abs=%v ok=%v", ptr, x, y, abs, ok)
		}
	}
	// Не мышь (RIM_TYPEKEYBOARD = 1) и обрезанный буфер отвергаются.
	if _, _, _, ok := ParseRawMouse(plRawMouse(8, 1, 0, 1, 1), 8); ok {
		t.Error("клавиатурный RAWINPUT принят как мышь")
	}
	if _, _, _, ok := ParseRawMouse(plRawMouse(8, RIMTypeMouse, 0, 1, 1)[:30], 8); ok {
		t.Error("обрезанный буфер принят")
	}
	// Заголовок другого размера читает другие байты: проверка, что ptrSize учтён.
	b := plRawMouse(8, RIMTypeMouse, 0, 5, 6)
	if x, _, _, ok := ParseRawMouse(b, 4); ok && x == 5 {
		t.Error("размер заголовка не учитывается")
	}
}
