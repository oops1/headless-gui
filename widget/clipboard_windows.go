//go:build windows

package widget

// clipboard_windows.go — буфер обмена Windows через Win32 API.
//
// Использует: OpenClipboard, GetClipboardData, SetClipboardData.
// CF_UNICODETEXT = 13 — Unicode текст (UTF-16LE).
// «HTML Format» — зарегистрированный формат оформленного текста; собирается и
// разбирается чистыми функциями из clipboard_html.go.
// CF_HDROP = 15 — список файлов (блок DROPFILES, clipboard_files.go) и рядом
// «Preferred DropEffect» — копировать или переместить при вставке.

import (
	"strings"
	"sync"
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
	procRegisterFormat   = user32Clip.NewProc("RegisterClipboardFormatW")
	procEmptyClipboard   = user32Clip.NewProc("EmptyClipboard")
	procGetClipboardData = user32Clip.NewProc("GetClipboardData")
	procSetClipboardData = user32Clip.NewProc("SetClipboardData")

	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	procGlobalSize   = kernel32.NewProc("GlobalSize")
	procGlobalFree   = kernel32.NewProc("GlobalFree")

	shell32Clip       = syscall.NewLazyDLL("shell32.dll")
	procDragQueryFile = shell32Clip.NewProc("DragQueryFileW")
)

const cfHDrop = 15

// htmlFormat — идентификатор формата «HTML Format». У зарегистрированных
// форматов он выдаётся системой при регистрации, константы для него нет.
var (
	htmlFormatOnce sync.Once
	htmlFormatID   uintptr
)

func htmlFormat() uintptr {
	htmlFormatOnce.Do(func() {
		name, _ := syscall.UTF16PtrFromString("HTML Format")
		htmlFormatID, _, _ = procRegisterFormat.Call(uintptr(unsafe.Pointer(name)))
	})
	return htmlFormatID
}

// dropEffectFormat — «Preferred DropEffect»: по нему Проводник отличает
// вырезанные файлы от скопированных.
var (
	dropEffectOnce sync.Once
	dropEffectID   uintptr
)

func dropEffectFormat() uintptr {
	dropEffectOnce.Do(func() {
		name, _ := syscall.UTF16PtrFromString("Preferred DropEffect")
		dropEffectID, _, _ = procRegisterFormat.Call(uintptr(unsafe.Pointer(name)))
	})
	return dropEffectID
}

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
	setClipboardUnicode(s)
}

// SetHTML кладёт оформленный текст и его простую версию за ОДНО открытие
// буфера. Если открывать дважды, второй EmptyClipboard стёр бы первый формат,
// а между открытиями вклинился бы чужой процесс — и в буфере остался бы либо
// один HTML без текста (Блокнот не вставит ничего), либо чужое содержимое.
func (c *winClipboard) SetHTML(html, plain string) {
	format := htmlFormat()
	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()
	setClipboardUnicode(plain)
	if format != 0 {
		// Завершающий NUL: так принято для «HTML Format», хотя в смещения он
		// не входит.
		data := append(BuildCFHTML(html), 0)
		setClipboardBytes(format, data)
	}
}

// GetHTML читает «HTML Format» и достаёт из него фрагмент.
func (c *winClipboard) GetHTML() (string, bool) {
	format := htmlFormat()
	if format == 0 {
		return "", false
	}
	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return "", false
	}
	defer procCloseClipboard.Call()

	h, _, _ := procGetClipboardData.Call(format)
	if h == 0 {
		return "", false
	}
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return "", false
	}
	defer procGlobalUnlock.Call(h)

	// Размер берём у системы и копируем данные до выхода из блокировки: как
	// и с текстом, содержимое чужое, и сканировать «до нуля» за границу блока
	// нельзя (аудит SEC-16).
	size, _, _ := procGlobalSize.Call(h)
	if size == 0 {
		return "", false
	}
	data := make([]byte, size)
	copy(data, unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size))
	return ParseCFHTML(data)
}

// SetFiles кладёт список файлов (CF_HDROP), пометку cut (Preferred DropEffect)
// и пути текстом — за одно открытие буфера, по той же причине, что SetHTML.
//
// Реализует widget.ClipboardFilesProvider. Пути в тексте — через CRLF, как
// принято в Windows: Блокнот иначе показал бы их одной строкой.
func (c *winClipboard) SetFiles(paths []string, cut bool) {
	effect := dropEffectFormat()
	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()
	setClipboardUnicode(strings.Join(paths, "\r\n"))
	setClipboardBytes(cfHDrop, buildDropFiles(paths))
	if effect != 0 {
		setClipboardBytes(effect, dropEffectBytes(cut))
	}
}

// GetFiles читает CF_HDROP и Preferred DropEffect.
//
// Реализует widget.ClipboardFilesProvider. Блок разбирается своими силами с
// проверкой границ (содержимое чужое); пути в однобайтовой кодировке — от
// старых программ — отдаются системе (DragQueryFileW), она знает кодовую
// страницу.
func (c *winClipboard) GetFiles() ([]string, bool, bool) {
	effect := dropEffectFormat()
	ret, _, _ := procOpenClipboard.Call(0)
	if ret == 0 {
		return nil, false, false
	}
	defer procCloseClipboard.Call()

	h, _, _ := procGetClipboardData.Call(cfHDrop)
	if h == 0 {
		return nil, false, false
	}
	data := clipboardBlock(h)
	paths, ok := parseDropFiles(data)
	if !ok {
		paths = dragQueryFiles(h)
		ok = len(paths) > 0
	}
	if !ok {
		return nil, false, false
	}
	cut := false
	if effect != 0 {
		if he, _, _ := procGetClipboardData.Call(effect); he != 0 {
			cut = dropEffectIsCut(clipboardBlock(he))
		}
	}
	return paths, cut, true
}

// clipboardBlock копирует блок буфера обмена целиком, по размеру от системы.
// Буфер должен быть открыт.
func clipboardBlock(h uintptr) []byte {
	ptr, _, _ := procGlobalLock.Call(h)
	if ptr == 0 {
		return nil
	}
	defer procGlobalUnlock.Call(h)
	size, _, _ := procGlobalSize.Call(h)
	if size == 0 {
		return nil
	}
	data := make([]byte, size)
	copy(data, unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size))
	return data
}

// dragQueryFiles достаёт пути из HDROP средствами системы.
func dragQueryFiles(h uintptr) []string {
	n, _, _ := procDragQueryFile.Call(h, 0xFFFFFFFF, 0, 0)
	var out []string
	for i := uintptr(0); i < n && i < 1<<16; i++ {
		l, _, _ := procDragQueryFile.Call(h, i, 0, 0)
		if l == 0 {
			continue
		}
		buf := make([]uint16, l+1)
		procDragQueryFile.Call(h, i, uintptr(unsafe.Pointer(&buf[0])), l+1)
		out = append(out, syscall.UTF16ToString(buf))
	}
	return out
}

// setClipboardUnicode кладёт простой текст как CF_UNICODETEXT. Буфер обмена
// должен быть уже открыт и очищен вызывающим.
func setClipboardUnicode(s string) {
	// Конвертируем в UTF-16LE с нулевым терминатором.
	utf16Str := utf16.Encode([]rune(s))
	utf16Str = append(utf16Str, 0) // NUL terminator.

	size := len(utf16Str) * 2 // 2 bytes per uint16.
	setClipboardBytes(cfUnicodeText, unsafe.Slice((*byte)(unsafe.Pointer(&utf16Str[0])), size))
}

// setClipboardBytes кладёт блок байт в буфер обмена под указанным форматом.
// Буфер должен быть уже открыт вызывающим.
func setClipboardBytes(format uintptr, data []byte) {
	size := uintptr(len(data))
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
	copy(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), size), data)

	procGlobalUnlock.Call(hMem)
	if r, _, _ := procSetClipboardData.Call(format, hMem); r == 0 {
		// Система блок не приняла — он по-прежнему наш.
		procGlobalFree.Call(hMem)
	}
	// Иначе hMem теперь принадлежит системе — НЕ вызываем GlobalFree.
}
