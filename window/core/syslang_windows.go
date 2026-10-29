//go:build windows

package core

// syslang_windows.go — языки интерфейса Windows: GetUserPreferredUILanguages.
//
// Это список «Предпочитаемые языки» из параметров «Язык и регион» (Windows 8+),
// в порядке, который собрал пользователь. Имена приходят уже в BCP-47
// («ru-RU», «en-US», «zh-Hans-CN»); normalizeLanguageTag приводит их к единому
// виду и убирает варианты вроде «de-DE_phoneb».

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32lang                    = windows.NewLazySystemDLL("kernel32.dll")
	procGetUserPreferredUILanguages = kernel32lang.NewProc("GetUserPreferredUILanguages")
	procGetUserDefaultLocaleName    = kernel32lang.NewProc("GetUserDefaultLocaleName")
)

const (
	muiLanguageName = 0x8 // MUI_LANGUAGE_NAME: имена BCP-47, а не шестнадцатеричные LANGID

	localeNameMaxLength = 85 // LOCALE_NAME_MAX_LENGTH
)

// systemLanguages: сначала список предпочтений, при его отказе — язык
// региональных настроек пользователя (старые системы, урезанные образы).
func systemLanguages() []string {
	if l := dedupLanguages(userPreferredUILanguages()); len(l) > 0 {
		return l
	}
	return dedupLanguages(userDefaultLocaleName())
}

// userPreferredUILanguages вызывает GetUserPreferredUILanguages двумя заходами:
// первый узнаёт размер буфера, второй читает многострочный (двойной ноль в
// конце) список.
func userPreferredUILanguages() []string {
	var n, size uint32
	r, _, _ := procGetUserPreferredUILanguages.Call(muiLanguageName,
		uintptr(unsafe.Pointer(&n)), 0, uintptr(unsafe.Pointer(&size)))
	if r == 0 || size == 0 {
		return nil
	}
	buf := make([]uint16, size)
	r, _, _ = procGetUserPreferredUILanguages.Call(muiLanguageName,
		uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return nil
	}
	if int(size) > len(buf) {
		size = uint32(len(buf))
	}
	return splitMultiString(buf[:size])
}

// userDefaultLocaleName — имя региональных настроек пользователя («ru-RU»).
func userDefaultLocaleName() []string {
	buf := make([]uint16, localeNameMaxLength)
	r, _, _ := procGetUserDefaultLocaleName.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if r == 0 {
		return nil
	}
	return []string{windows.UTF16ToString(buf)}
}

// splitMultiString режет список строк UTF-16, разделённых нулями и
// завершённый двойным нулём.
func splitMultiString(buf []uint16) []string {
	var out []string
	start := 0
	for i, c := range buf {
		if c != 0 {
			continue
		}
		if i == start {
			break // пустая строка — конец списка
		}
		out = append(out, windows.UTF16ToString(buf[start:i]))
		start = i + 1
	}
	return out
}
