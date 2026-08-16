//go:build windows

package widget

// clipboard_windows.go — буфер обмена Windows через Win32 API.
//
// Использует: OpenClipboard, GetClipboardData, SetClipboardData.
// CF_UNICODETEXT = 13 — Unicode текст (UTF-16LE).

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

var (
	user32Clip = syscall.NewLazyDLL("user32.dll")
	kernel32   = syscall.NewLazyDLL("kernel32.dll")

	procOpenClipboard    = user32Clip.NewProc("OpenClipboard")
	procCloseClipboard   = user32Clip.NewProc("CloseClipboard")
	procEmptyClipboard   = user32Clip.NewProc("EmptyClipboard")
	procGetClipboardData = user32Clip.NewProc("GetClipboardData")
	procSetClipboardData = user32Clip.NewProc("SetClipboardData")

	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	procGlobalSize   = kernel32.NewProc("GlobalSize")
	procGlobalFree   = kernel32.NewProc("GlobalFree")
)

// winClipboard — Windows системный буфер обмена.
type winClipboard struct{}

func init() {
	defaultClipboard = &winClipboard{}
}

func (c *winClipboard) GetText() string {
	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return ""
	}
	defer procCloseClipboard.Call()

	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return ""
	}

	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return ""
	}
	defer procGlobalUnlock.Call(h)

	// Читаем UTF-16LE строку до нулевого терминатора, НЕ выходя за размер
	// блока: CF_UNICODETEXT обязан быть NUL-терминирован, но чужое приложение
	// могло положить в буфер обмена что угодно, и скан «до нуля» ушёл бы за
	// границу выделения (аудит SEC-16). Размер блока даёт GlobalSize; если она
	// вернула 0 — доверять нечему, читаем пусто.
	size, _, _ := procGlobalSize.Call(h)
	if size < 2 {
		return ""
	}
	maxChars := int(size / 2)
	utf16Chars := make([]uint16, 0, min(maxChars, 4096))
	for i := 0; i < maxChars; i++ {
		ch := *(*uint16)(unsafe.Pointer(ptr + uintptr(i)*2))
		if ch == 0 {
			break
		}
		utf16Chars = append(utf16Chars, ch)
	}

	return string(utf16.Decode(utf16Chars))
}

func (c *winClipboard) SetText(s string) {
	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()

	// Конвертируем в UTF-16LE с нулевым терминатором.
	utf16Str := utf16.Encode([]rune(s))
	utf16Str = append(utf16Str, 0) // NUL terminator.

	size := uintptr(len(utf16Str) * 2) // 2 bytes per uint16.
	hMem, _, _ := procGlobalAlloc.Call(gmemMoveable, size)
	if hMem == 0 {
		return
	}

	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		// Блок ещё наш (системе он передаётся только в SetClipboardData) —
		// освобождаем, иначе HGLOBAL утекает при каждой неудаче.
		procGlobalFree.Call(hMem)
		return
	}

	// Копируем данные.
	src := unsafe.Pointer(&utf16Str[0])
	dst := unsafe.Pointer(ptr)
	copy(
		unsafe.Slice((*byte)(dst), size),
		unsafe.Slice((*byte)(src), size),
	)

	procGlobalUnlock.Call(hMem)
	procSetClipboardData.Call(cfUnicodeText, hMem)
	// hMem теперь принадлежит системе — НЕ вызываем GlobalFree.
}
