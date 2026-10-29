package core

// depmap_test.go — карта зависимостей пакета по будущим пакетам бэкендов
// (ADR-0013, этап 9). Группы — по именам файлов: бэкенды Win32, X11,
// Wayland, Cocoa (с файлами, которые по смыслу их, хотя названы иначе:
// activate_x11_linux.go, icon_linux.go, initial_wayland_linux.go…);
// остальное — ядро (обвязка и пока не разнесённые общие типы).
//
// Правило: ни ядро, ни бэкенд не зависят от другого бэкенда — только от
// ядра (до 9.2b; потом — от platform/native). Сегодняшние нарушения — в
// базе, своей под каждую ОС (файлы берутся по тегам сборки текущей ОС):
// проверка падает на новом ребре и сообщает, когда базовое исчезло. Каждый
// шаг этапа 9 убирает свои строки.

import (
	"regexp"
	"runtime"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/depmap"
)

// Глобальное состояние пакета тест не трогает: только чтение исходников.
func TestDepMap_BackendOrder(t *testing.T) {
	re := regexp.MustCompile
	baseline := map[string][]string{
		"windows": {
			"other → win32",
			"win32 → wayland",
			"win32 → x11",
			"x11 → wayland",
		},
		"linux": {
			"other → wayland",
			"other → x11",
			"wayland → x11",
			"x11 → wayland",
		},
		"darwin": {
			"other → cocoa",
			"x11 → wayland",
		},
	}
	depmap.Run(t, depmap.Config{
		Dir:  ".",
		Path: "window/core",
		Groups: []depmap.Group{
			{Name: "win32", Match: re(`_windows\.go$`)},
			{Name: "x11", Match: re(`^(x11|native_linux)|_x11_linux\.go$|^icon_linux\.go$`)},
			{Name: "wayland", Match: re(`^(wayland|native_wayland)|_wayland_linux\.go$`)},
			{Name: "cocoa", Match: re(`_darwin\.go$`)},
		},
		Default: "other",
		Allowed: func(from, to string) bool {
			return to == "other"
		},
		Baseline: baseline[runtime.GOOS],
		Top:      6,
	})
}
