// winparse.go — разбор двоичных структур Windows, которые возвращает диалог
// печати (DEVNAMES и DEVMODE). Чистые функции над байтами: без них проверить
// разбор можно было бы только на настоящей Windows и с настоящим диалогом.
package printing

import (
	"encoding/binary"
	"errors"
	"unicode/utf16"
)

// devNamesDevice достаёт имя устройства (принтера) из DEVNAMES.
//
// DEVNAMES — это заголовок из четырёх WORD со смещениями строк (драйвер,
// устройство, порт, флаг по умолчанию), за которым в том же блоке лежат сами
// строки UTF-16. Смещения — в символах от начала блока, а не в байтах: путаница
// в единицах даёт обрезанное или пустое имя принтера.
func devNamesDevice(b []byte) (string, error) {
	if len(b) < 8 {
		return "", errors.New("printing: блок DEVNAMES короче заголовка")
	}
	off := int(binary.LittleEndian.Uint16(b[2:])) * 2 // wDeviceOffset
	if off < 8 || off+2 > len(b) {
		return "", errors.New("printing: смещение имени принтера в DEVNAMES вне блока")
	}
	var u []uint16
	for p := off; p+2 <= len(b); p += 2 {
		c := binary.LittleEndian.Uint16(b[p:])
		if c == 0 {
			return string(utf16.Decode(u)), nil
		}
		u = append(u, c)
	}
	return "", errors.New("printing: имя принтера в DEVNAMES не завершено нулём")
}

// Смещения в DEVMODEW (wingdi.h): имя устройства 32 WCHAR, затем версии, размер
// структуры и её «хвоста» драйвера, поля-флаги и — для принтеров — ориентация,
// размер бумаги, масштаб, число копий.
const (
	devmodeSizeOff   = 68
	devmodeExtraOff  = 70
	devmodeFieldsOff = 72
	devmodeCopiesOff = 86
	devmodeMinSize   = 92 // до dmColor включительно: всё, что мы читаем
	dmCopiesField    = 0x00000100
)

// devmodeTotalSize — полный размер DEVMODE вместе с закрытой частью драйвера
// (dmSize + dmDriverExtra). Копировать надо весь блок: настройки драйвера
// (двусторонняя печать, лоток) лежат именно в хвосте, и без него выбор
// пользователя в диалоге пропал бы.
func devmodeTotalSize(b []byte) (int, error) {
	if len(b) < devmodeMinSize {
		return 0, errors.New("printing: блок DEVMODE короче минимального")
	}
	size := int(binary.LittleEndian.Uint16(b[devmodeSizeOff:])) + int(binary.LittleEndian.Uint16(b[devmodeExtraOff:]))
	if size < devmodeMinSize || size > len(b) {
		return 0, errors.New("printing: размер DEVMODE не сходится с блоком")
	}
	return size, nil
}

// devmodeSingleCopy выставляет в DEVMODE одну копию. Число копий диалог отдаёт
// отдельным полем, и пакет печатает копии сам (в нужном порядке по комплектам);
// если бы драйвер при этом ещё и сам размножал страницы по dmCopies, пользователь
// получил бы копий в квадрате.
func devmodeSingleCopy(dm []byte) {
	if len(dm) < devmodeMinSize {
		return
	}
	if binary.LittleEndian.Uint32(dm[devmodeFieldsOff:])&dmCopiesField != 0 {
		binary.LittleEndian.PutUint16(dm[devmodeCopiesOff:], 1)
	}
}
