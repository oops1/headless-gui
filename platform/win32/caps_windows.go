//go:build windows

package win32

// caps_windows.go — возможности версии Windows, от которых зависит поведение
// бэкенда (обзор архитектуры, T1, T2). Определяются один раз при загрузке:
// версия ОС за время работы программы не меняется.

import "golang.org/x/sys/windows"

// Capabilities — что умеет Windows, на которой запущена программа.
type Capabilities struct {
	// Major, Minor, Build — версия ОС по RtlGetVersion (без подмены
	// совместимости манифеста): Windows 10 и 11 — 10.0, различаются сборкой.
	Major, Minor, Build uint32

	// RoundCorners — DWM скругляет углы окна сам (DWMWA_WINDOW_CORNER_PREFERENCE,
	// Windows 11, сборка 22000+), с мягкими углами и системной тенью. Нет —
	// окно с радиусом скругления остаётся прямоугольным, а тень даёт рамка DWM
	// в 1 px: скруглённый регион (SetWindowRgn) отнимал бы у окна тень (T2).
	RoundCorners bool

	// SplitDIBSource — GDI выводит DIB из памяти, лежащей в нескольких
	// резервациях VirtualAlloc (обычный срез Go больше 4 МБ). На Windows 10 —
	// нет: StretchDIBits не копирует ни строки, окно больше ~1024² пустое (T1),
	// поэтому буфер кадра всегда одна резервация (framebuf_windows.go).
	SplitDIBSource bool
}

// Caps — возможности текущей ОС. Тесты подменяют поля, чтобы пройти путь
// другой версии Windows, и возвращают прежнее значение.
var Caps = detectCaps(windows.RtlGetVersion())

// win11Build — первая сборка Windows 11.
const win11Build = 22000

// detectCaps — возможности по версии ОС.
func detectCaps(v *windows.OsVersionInfoEx) Capabilities {
	c := Capabilities{Major: v.MajorVersion, Minor: v.MinorVersion, Build: v.BuildNumber}
	win11 := c.Major > 10 || (c.Major == 10 && c.Build >= win11Build)
	c.RoundCorners = win11
	c.SplitDIBSource = win11
	return c
}
