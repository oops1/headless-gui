package core

// depmap_test.go — карта зависимостей пакета по бэкендам окна (ADR-0013,
// этап 9) и сторож того, что бэкендов в ядре больше нет.
//
// С 9.5 все бэкенды — свои пакеты (platform/win32, x11, wayland, cocoa). Файлы
// ядра под конкретную ОС — только прослойки (backend_*.go: выбор бэкенда,
// возможности ОС теми же именами на всех платформах, псевдонимы открытых имён
// фасада; monitor_*.go: мониторы без окна; activate_other.go и
// doubleclick_other.go — пара к функциям backend_windows.go). Возможности окна
// (доступность, оболочка, трей, перетаскивание) — свои пакеты platform/*
// (9.8), и их файлов под ОС в ядре нет. Файл с суффиксом ОС или с именем
// бэкенда (x11*, wayland*, native_<ОС>) попадает в группу backend, и она
// обязана быть пустой: бэкенд с 9.2b к ядру не обращается, так что
// вернувшийся файл бэкенда (или возможности) рёбер бы не дал — ловит его
// именно пустота группы. Группы выбираются по имени, поэтому файл, выбранный
// только тегом //go:build, ловит отдельный TestDepMap_NoTaggedOSFiles.
//
// Правило рёбер (с 9.2b): бэкенд не зависит ни от ядра, ни от другого
// бэкенда — общее у них в platform/native, keyboard, pointer, dbus.
// База нарушений своя под каждую ОС (файлы берутся по тегам сборки); с 9.3
// она пуста везде.
//
// ОС берётся из go/build (переменная GOOS), а не из runtime.GOOS: собранный
// под Windows тест проверяет и карту Linux —
// GOOS=linux CGO_ENABLED=0 ./core.test -test.run DepMap.

import (
	"go/build"
	"os"
	"regexp"
	"sort"
	"strings"
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

// Файл под ОС выбирает не только суффикс, но и //go:build: файл без суффикса,
// который собирается не на всех трёх ОС, — тоже файл под ОС, а группы
// TestDepMap_BackendOrder выбираются по имени и его не видят. В ядре такие
// файлы — только прослойки: backend_*.go, monitor_*.go и поимённо
// перечисленные ниже. Новый — ошибка теста (место ему — в бэкенде или
// возможности platform/*), исключение, которого больше нет, — тоже.
func TestDepMap_NoTaggedOSFiles(t *testing.T) {
	shim := regexp.MustCompile(`^(backend|monitor)_`)
	// Пара к функциям backend_windows.go под прочими ОС.
	listed := map[string]bool{"activate_other.go": false, "doubleclick_other.go": false}
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		var in []string
		for _, goos := range []string{"windows", "linux", "darwin"} {
			ctx := build.Default
			ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled = goos, "amd64", false
			ok, err := ctx.MatchFile(".", n)
			if err != nil {
				t.Fatalf("%s: %v", n, err)
			}
			if ok {
				in = append(in, goos)
			}
		}
		if len(in) == 3 || shim.MatchString(n) {
			continue
		}
		if _, ok := listed[n]; ok {
			listed[n] = true
			continue
		}
		t.Errorf("%s собирается только под %v: файл под ОС в ядре (место ему — в бэкенде или возможности platform/*)", n, in)
	}
	var stale []string
	for n, seen := range listed {
		if !seen {
			stale = append(stale, n)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("исключения устарели (файла нет или он собирается на всех ОС): %v", stale)
	}
}
