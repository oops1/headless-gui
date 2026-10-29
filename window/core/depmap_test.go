package core

// depmap_test.go — карта зависимостей пакета по будущим пакетам бэкендов
// (ADR-0013, этап 9). Группы — по именам файлов: бэкенды Win32, X11,
// Wayland, Cocoa (с файлами, которые по смыслу их, хотя названы иначе:
// activate_x11_linux.go, icon_linux.go, initial_wayland_linux.go…);
// возможности под ОС (доступность, трей, DnD, оболочка — feature: их
// отделяет 9.8); остальное — ядро (обвязка окна).
//
// Win32 с 9.2 — свой пакет platform/win32 (здесь только прослойка ядра
// backend_windows.go: возможности ОС теми же именами, что на других
// платформах, — это ядро, как и выбор бэкенда backend_linux.go).
//
// Правило (с 9.2b): бэкенд не зависит ни от ядра, ни от другого бэкенда —
// общее у них в platform/native, platform/keyboard, platform/pointer,
// platform/dbus. Ядро и возможности зависят от кого угодно. Сегодняшние
// нарушения — в базе, своей под каждую ОС (файлы берутся по тегам сборки):
// проверка падает на новом ребре и сообщает, когда базовое исчезло. Каждый
// шаг этапа 9 убирает свои строки.
//
// ОС берётся из go/build (переменная GOOS), а не из runtime.GOOS: собранный
// под Windows тест проверяет и карту Linux —
// GOOS=linux CGO_ENABLED=0 ./core.test -test.run DepMap.

import (
	"go/build"
	"regexp"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/depmap"
)

// Глобальное состояние пакета тест не трогает: только чтение исходников.
func TestDepMap_BackendOrder(t *testing.T) {
	re := regexp.MustCompile
	baseline := map[string][]string{
		"windows": {},
		"linux": {
			"wayland → feature",
			"x11 → feature",
		},
		"darwin": {},
	}
	depmap.Run(t, depmap.Config{
		Dir:  ".",
		Path: "window/core",
		Groups: []depmap.Group{
			{Name: "other", Match: re(`^backend_`)},
			{Name: "feature", Match: re(`^(a11y|tray|dnd|openurl|systheme|syslang|notify)\w*_(windows|linux|darwin)\.go$`)},
			{Name: "win32", Match: re(`_windows\.go$`)},
			{Name: "x11", Match: re(`^(x11|native_linux)|_x11_linux\.go$|^icon_linux\.go$`)},
			{Name: "wayland", Match: re(`^(wayland|native_wayland)|_wayland_linux\.go$`)},
			{Name: "cocoa", Match: re(`_darwin\.go$`)},
		},
		Default: "other",
		Allowed: func(from, to string) bool {
			return from == "other" || from == "feature"
		},
		Baseline: baseline[build.Default.GOOS],
		Top:      12,
	})
}
