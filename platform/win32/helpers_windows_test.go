//go:build windows

package win32

// helpers_windows_test.go — копии помощников тестов platform/a11y
// (a11y_windows_test.go) и window/core (pointerlock_test.go, rawkey_test.go): тестовые
// помощники между пакетами не делятся.

import (
	"encoding/binary"
	"syscall"
	"unsafe"
)

// comCall вызывает метод COM-объекта по номеру слота в vtable — ровно так, как
// это делает клиент UIA. Проверяет не только логику провайдера, но и саму
// раскладку таблицы: перепутанный слот виден сразу.
//
// Объекты тестов живут в Go-куче — checkptr здесь неприменим.
//
// Выходные параметры передаются адресом, превращённым в uintptr в самом
// вызове (uintptr(unsafe.Pointer(&x))). Без директивы uintptrescapes такой
// адрес для компилятора — просто число: переменная остаётся на стеке, стек
// горутины может переехать, пока идёт вызов, и метод запишет ответ по
// старому адресу (тесты UIA падали так в ~15% прогонов, когда куча стала
// маленькой и сборки — частыми). Директива — та же, что у syscall.Proc.Call:
// аргументы-uintptr считаются указателями, их адресаты уходят в кучу и живут
// до конца вызова.
//
//go:uintptrescapes
//go:nocheckptr
func comCall(iface uintptr, slot int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(unsafe.Pointer(iface))
	fn := *(*uintptr)(unsafe.Add(vtbl, slot*int(unsafe.Sizeof(uintptr(0)))))
	all := append([]uintptr{iface}, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

// Слоты IUnknown.
const (
	slotQueryInterface = 0
	slotAddRef         = 1
	slotRelease        = 2
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

// wheelCall — сырое или пиксельное колесо, как его видит приложение (тесты
// трёх бэкендов).
type wheelCall struct {
	x, y    int
	dx, dy  float64
	precise bool
}
