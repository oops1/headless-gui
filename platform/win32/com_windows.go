//go:build windows

package win32

// com_windows.go — основа COM без библиотек: HRESULT, GUID, таблицы
// виртуальных методов. Общая для UIA (доступность) и OLE (перетаскивание).

import (
	"fmt"
	"sync"
	"unsafe"
)

// HRESULT, общие для любого COM-объекта.
const (
	SOK          = uintptr(0)
	ENoInterface = uintptr(0x80004002)
	EInvalidArg  = uintptr(0x80070057)
	EFail        = uintptr(0x80004005)
	ENotImpl     = uintptr(0x80004001)
)

// IIDIUnknown — IUnknown {00000000-0000-0000-C000-000000000046}.
var IIDIUnknown = GUID{0x00000000, 0x0000, 0x0000, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}

// GUID — структура GUID в раскладке Windows.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// Equals — GUID совпадают; nil не равен ничему.
func (g *GUID) Equals(o *GUID) bool {
	if g == nil || o == nil {
		return false
	}
	return *g == *o
}

// String форматирует GUID как {XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX} —
// нужно, чтобы в журнале было видно, какой интерфейс просит клиент.
func (g *GUID) String() string {
	if g == nil {
		return "<nil>"
	}
	return fmt.Sprintf("{%08X-%04X-%04X-%02X%02X-%02X%02X%02X%02X%02X%02X}",
		g.Data1, g.Data2, g.Data3, g.Data4[0], g.Data4[1],
		g.Data4[2], g.Data4[3], g.Data4[4], g.Data4[5], g.Data4[6], g.Data4[7])
}

var (
	vtableMu   sync.Mutex
	vtableKeep [][]uintptr // держим таблицы живыми: их адреса ушли в чужой код
)

// NewVTable собирает vtable из указателей на функции и возвращает адрес
// первого элемента. Слайс сохраняется глобально — сборщик мусора не должен
// освободить память, на которую смотрит UIA.
func NewVTable(fns ...uintptr) uintptr {
	tbl := make([]uintptr, len(fns))
	copy(tbl, fns)
	vtableMu.Lock()
	vtableKeep = append(vtableKeep, tbl)
	vtableMu.Unlock()
	return uintptr(unsafe.Pointer(&tbl[0]))
}
