package printing

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

// buildDevNames собирает DEVNAMES так, как его отдаёт диалог: заголовок из
// четырёх WORD со смещениями (в символах) и строки UTF-16 с нулями.
func buildDevNames(driver, device, output string) []byte {
	b := make([]byte, 8)
	put := func(s string) int {
		off := len(b) / 2
		for _, u := range utf16.Encode([]rune(s)) {
			b = binary.LittleEndian.AppendUint16(b, u)
		}
		b = binary.LittleEndian.AppendUint16(b, 0)
		return off
	}
	d := put(driver)
	v := put(device)
	o := put(output)
	binary.LittleEndian.PutUint16(b[0:], uint16(d))
	binary.LittleEndian.PutUint16(b[2:], uint16(v))
	binary.LittleEndian.PutUint16(b[4:], uint16(o))
	return b
}

func TestDevNamesDevice(t *testing.T) {
	b := buildDevNames("winspool", `Принтер бухгалтерии (HP)`, "Ne01:")
	got, err := devNamesDevice(b)
	if err != nil || got != "Принтер бухгалтерии (HP)" {
		t.Errorf("got %q, err %v", got, err)
	}
	for name, bad := range map[string][]byte{
		"короткий":         {1, 2, 3},
		"смещение мимо":    func() []byte { x := buildDevNames("a", "b", "c"); binary.LittleEndian.PutUint16(x[2:], 500); return x }(),
		"смещение в шапке": func() []byte { x := buildDevNames("a", "b", "c"); binary.LittleEndian.PutUint16(x[2:], 1); return x }(),
		"без нуля": func() []byte {
			x := buildDevNames("a", "bb", "")
			return x[:len(x)-4] // срезаны завершающие нули device и output
		}(),
	} {
		if _, err := devNamesDevice(bad); err == nil {
			t.Errorf("%s: ошибки нет", name)
		}
	}
}

func buildDevMode(copies uint16, fields uint32, extra int) []byte {
	const size = 156 // sizeof(DEVMODEW) на Windows
	b := make([]byte, size+extra)
	binary.LittleEndian.PutUint16(b[devmodeSizeOff:], size)
	binary.LittleEndian.PutUint16(b[devmodeExtraOff:], uint16(extra))
	binary.LittleEndian.PutUint32(b[devmodeFieldsOff:], fields)
	binary.LittleEndian.PutUint16(b[devmodeCopiesOff:], copies)
	for i := size; i < len(b); i++ {
		b[i] = 0xAB // «закрытая часть драйвера»
	}
	return b
}

func TestDevModeSize(t *testing.T) {
	b := buildDevMode(3, dmCopiesField, 40)
	// Блок GlobalAlloc бывает больше содержимого: берём dmSize+dmDriverExtra.
	padded := append(append([]byte(nil), b...), make([]byte, 100)...)
	n, err := devmodeTotalSize(padded)
	if err != nil || n != 196 {
		t.Errorf("размер %d, err %v", n, err)
	}
	if _, err := devmodeTotalSize(make([]byte, 50)); err == nil {
		t.Error("короткий блок принят")
	}
	if _, err := devmodeTotalSize(b[:150]); err == nil {
		t.Error("блок меньше dmSize+dmDriverExtra принят")
	}
	small := buildDevMode(1, 0, 0)
	binary.LittleEndian.PutUint16(small[devmodeSizeOff:], 10)
	if _, err := devmodeTotalSize(small); err == nil {
		t.Error("dmSize < минимума принят")
	}
}

func TestDevModeSingleCopy(t *testing.T) {
	b := buildDevMode(5, dmCopiesField|0x1, 8)
	devmodeSingleCopy(b)
	if got := binary.LittleEndian.Uint16(b[devmodeCopiesOff:]); got != 1 {
		t.Errorf("dmCopies=%d", got)
	}
	// Закрытая часть драйвера не тронута.
	if b[len(b)-1] != 0xAB {
		t.Error("хвост драйвера изменён")
	}
	// Без флага DM_COPIES поле не определено — не трогаем.
	c := buildDevMode(5, 0x1, 0)
	devmodeSingleCopy(c)
	if got := binary.LittleEndian.Uint16(c[devmodeCopiesOff:]); got != 5 {
		t.Errorf("без DM_COPIES изменено: %d", got)
	}
	devmodeSingleCopy(nil) // не паникует
}
