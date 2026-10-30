package win32

import (
	"encoding/binary"
	"unicode/utf16"
)

// dnd_hdrop.go — чистая часть Win32-перетаскивания: сборка блока CF_HDROP и
// решения IDropSource. Без тега платформы намеренно: раскладку байтов и
// таблицу решений проверяет общий прогон тестов на любой машине, а не только
// Windows. Сами COM-объекты — в dnd_windows.go.

// hdropHeaderSize — sizeof(DROPFILES): DWORD pFiles, POINT pt, BOOL fNC,
// BOOL fWide.
const hdropHeaderSize = 20

// buildHDrop собирает блок CF_HDROP (DROPFILES + список путей в UTF-16, каждый
// путь с завершающим нулём и ещё один ноль в конце списка) — то, что ждут
// Проводник и прочие приёмники от источника файлов.
//
// Поле pt и fNC нулевые: приёмники берут точку сброса из параметров
// IDropTarget::Drop, а не из блока.
func buildHDrop(paths []string) []byte {
	var u []uint16
	for _, p := range paths {
		u = append(u, utf16.Encode([]rune(p))...)
		u = append(u, 0)
	}
	u = append(u, 0)
	if len(paths) == 0 {
		u = append(u, 0) // пустой список — два нуля подряд
	}
	b := make([]byte, hdropHeaderSize+len(u)*2)
	binary.LittleEndian.PutUint32(b[0:4], hdropHeaderSize) // pFiles
	binary.LittleEndian.PutUint32(b[16:20], 1)             // fWide
	for i, v := range u {
		binary.LittleEndian.PutUint16(b[hdropHeaderSize+i*2:], v)
	}
	return b
}

// parseHDrop разбирает блок CF_HDROP с путями в UTF-16 (обратная к buildHDrop).
// Однобайтовые списки (fWide == 0) не поддерживаются: ok == false. Блок
// чужой, поэтому каждое смещение сверяется с его размером.
func parseHDrop(b []byte) (paths []string, ok bool) {
	if len(b) < hdropHeaderSize {
		return nil, false
	}
	off := int(binary.LittleEndian.Uint32(b[0:4]))
	if off < hdropHeaderSize || off > len(b) || binary.LittleEndian.Uint32(b[16:20]) == 0 {
		return nil, false
	}
	var cur []uint16
	for i := off; i+2 <= len(b); i += 2 {
		c := binary.LittleEndian.Uint16(b[i:])
		if c != 0 {
			cur = append(cur, c)
			continue
		}
		if len(cur) == 0 {
			break // пустая строка — конец списка
		}
		paths = append(paths, string(utf16.Decode(cur)))
		cur = nil
	}
	return paths, true
}

// Результаты IDropSource и значения OLE, нужные чистой части.
const (
	dragdropSDrop   = 0x00040100 // DRAGDROP_S_DROP
	dragdropSCancel = 0x00040101 // DRAGDROP_S_CANCEL
	dragdropSUseDef = 0x00040102 // DRAGDROP_S_USEDEFAULTCURSORS

	mkLButton = 0x0001
	mkRButton = 0x0002
	mkShift   = 0x0004
	mkControl = 0x0008
	mkMButton = 0x0010
	mkAlt     = 0x0020
)

// dragContinueResult — решение IDropSource::QueryContinueDrag: Esc — отмена;
// кнопка, с которой начали (mask, биты MK_*), отпущена — сброс; иначе
// перетаскивание продолжается (S_OK == 0).
func dragContinueResult(escape bool, keyState, mask uint32) uint32 {
	switch {
	case escape:
		return dragdropSCancel
	case keyState&mask == 0:
		return dragdropSDrop
	}
	return 0
}
