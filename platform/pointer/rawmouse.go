package pointer

// rawmouse.go — разбор RAWINPUT мыши (захват указателя Win32).

import (
	"encoding/binary"
)

// Значения Raw Input (winuser.h), нужные разбору RAWINPUT.
const (
	RIMTypeMouse      = 0    // RIM_TYPEMOUSE
	MouseMoveAbsolute = 0x01 // MOUSE_MOVE_ABSOLUTE
)

// ParseRawMouse разбирает RAWINPUT мыши: lLastX, lLastY и признак «абсолютное
// положение» (MOUSE_MOVE_ABSOLUTE). ptrSize — размер указателя (8 или 4):
// заголовок RAWINPUTHEADER содержит два слова размером с указатель. ok=false —
// это не мышь или буфер короче структуры.
//
// Раскладка RAWMOUSE после заголовка: usFlags (2) + 2 байта выравнивания,
// ulButtons (4), ulRawButtons (4), lLastX (4), lLastY (4), ulExtraInformation (4).
func ParseRawMouse(buf []byte, ptrSize int) (x, y int32, absolute, ok bool) {
	hdr := 8 + 2*ptrSize
	if len(buf) < hdr+24 {
		return 0, 0, false, false
	}
	if binary.LittleEndian.Uint32(buf[0:4]) != RIMTypeMouse {
		return 0, 0, false, false
	}
	flags := binary.LittleEndian.Uint16(buf[hdr : hdr+2])
	x = int32(binary.LittleEndian.Uint32(buf[hdr+12 : hdr+16]))
	y = int32(binary.LittleEndian.Uint32(buf[hdr+16 : hdr+20]))
	return x, y, flags&MouseMoveAbsolute != 0, true
}
