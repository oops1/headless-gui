package win32

// framebuf_windows.go — память буфера кадра для GDI.
//
// На Windows 10 окно больше примерно 1024×1024 оставалось пустым (отчёт
// RDP-клиента Tandem на v3.38.0, две машины с Windows 10 22H2). Буфер кадра
// был обычным срезом Go, а куча Go на windows/amd64 резервирует память
// аренами по 4 МБ отдельными вызовами VirtualAlloc(MEM_RESERVE): буфер больше
// 4 МБ почти всегда лежит в двух резервациях. StretchDIBits и
// SetDIBitsToDevice на Windows 10 с таким источником не копируют ни строки и
// возвращают 0 при ERROR_SUCCESS (по-видимому, win32k закрепляет буфер DIB
// целиком и не может закрепить диапазон из нескольких резерваций). Windows 11
// так не делает. Копия того же кадра в буфер из одного VirtualAlloc выводится
// всегда. Поэтому буфер — одна резервация на любой версии, хотя Windows 11
// выводит и срез Go (Caps.SplitDIBSource): одна дорога проще двух.

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// allocFrameBuf заводит буфер кадра на n байт одной резервацией VirtualAlloc.
// Не вышло — обычный срез Go: на Windows 11 и при малых окнах он работает.
// Вызывать под w.mu.
func (w *Win32Window) allocFrameBuf(n int) {
	w.freeFrameBuf()
	p, err := windows.VirtualAlloc(0, uintptr(n), windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil || p == 0 {
		w.frameBuf = make([]byte, n)
		return
	}
	w.frameBuf, w.frameVA = unsafe.Slice((*byte)(unsafe.Pointer(p)), n), true
}

// freeFrameBuf возвращает буфер кадра системе. Вызывать под w.mu.
func (w *Win32Window) freeFrameBuf() {
	if w.frameVA && len(w.frameBuf) > 0 {
		windows.VirtualFree(uintptr(unsafe.Pointer(&w.frameBuf[0])), 0, windows.MEM_RELEASE)
	}
	w.frameBuf, w.frameVA = nil, false
}

// blitFailed — GDI не вывел кадр. Окно просит у системы перерисовку
// (WM_PAINT → блит из кэша), но не больше трёх раз подряд: постоянный отказ
// не должен превратиться в бесконечный цикл перерисовок.
func (w *Win32Window) blitFailed() {
	if w.blitFails.Add(1) <= 3 {
		procInvalidateRect.Call(uintptr(w.hwnd), 0, 0)
	}
}
