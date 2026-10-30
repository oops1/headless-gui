package wayland

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
)

// Копия помощника из platform/native (clipboard_image_test.go): тестовые
// помощники между пакетами не делятся.

// hugePNGHeader — PNG, у которого заявлен размер w×h, а данных нет: проверка
// защиты от картинок-бомб.
func hugePNGHeader(w, h uint32) []byte {
	var b bytes.Buffer
	b.Write([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 6 // 8 бит, RGBA
	chunk := append([]byte("IHDR"), ihdr...)
	binary.Write(&b, binary.BigEndian, uint32(13))
	b.Write(chunk)
	binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return b.Bytes()
}
