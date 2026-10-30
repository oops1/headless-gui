package core

// depmap_test.go — карта зависимостей пакета по бэкендам окна (ADR-0013,
// этап 9) и сторож того, что бэкендов в ядре больше нет.
//
// С 9.5 все бэкенды — свои пакеты (platform/win32, x11, wayland, cocoa). Файлы
// ядра под конкретную ОС — либо прослойки (backend_*.go: выбор бэкенда,
// возможности ОС теми же именами на всех платформах, псевдонимы открытых имён
// фасада; monitor_*.go: мониторы без окна), либо возможности (feature:
// доступность и оболочка, их отделяет 9.8). Любой другой файл под ОС и любой
// файл с именем бэкенда (x11*, wayland*, native_<ОС>) попадает в группу
// backend, и она обязана быть пустой: бэкенд с 9.2b к ядру не обращается, так
// что вернувшийся файл бэкенда рёбер бы не дал — ловит его именно пустота
// группы.
//
// Правило рёбер (с 9.2b): бэкенд не зависит ни от ядра, ни от другого
// бэкенда — общее у них в platform/native, keyboard, pointer, dbus, tray.
// База нарушений своя под каждую ОС (файлы берутся по тегам сборки); с 9.3
// она пуста везде.
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
		"linux":   {},
		"darwin":  {},
	}
	rep := depmap.Run(t, depmap.Config{
		Dir:  ".",
		Path: "window/core",
		Groups: []depmap.Group{
			{Name: "other", Match: re(`^(backend|monitor)_`)},
			{Name: "feature", Match: re(`^(a11y|tray|dnd|openurl|systheme|syslang|notify)\w*_(windows|linux|darwin)\.go$`)},
			{Name: "backend", Match: re(`_(windows|linux|darwin)\.go$|^(x11|wayland|win32|cocoa)|^native_(windows|linux|wayland|darwin)`)},
		},
		Default: "other",
		Allowed: func(from, to string) bool {
			return from != "backend"
		},
		Baseline: baseline[build.Default.GOOS],
		Top:      12,
	})
	if fs := rep.Files["backend"]; len(fs) > 0 {
		t.Errorf("файлы бэкенда в ядре (место им — platform/*): %v", fs)
	}
}
