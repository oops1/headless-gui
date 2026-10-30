package layers

import (
	"go/build"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const mod = "github.com/oops1/headless-gui/v4"

// Возможности окна (ADR-0013 §3): их подключает window, ни ядро, ни бэкенды,
// ни window/minimal их не тянут.
var features = []string{"platform/a11y", "platform/dnd", "platform/shell", "platform/tray"}

// wired — возможности, уже подключённые к window пустым импортом
// (window/capabilities.go). Пополняется по шагам 9.8.
var wired = []string{"platform/a11y", "platform/dnd", "platform/shell", "platform/tray"}

// backends — бэкенды окна.
var backends = []string{"platform/win32", "platform/x11", "platform/wayland", "platform/cocoa"}

// baseline — рёбра, временно нарушающие правило, по ОС: «субъект → запрет».
// База убывает по шагам 9.8; ребро, которого больше нет, — ошибка теста
// (база устарела, её надо сократить).
var baseline = map[string]map[string][]string{
	"windows": {},
	"linux":   {},
	"darwin":  {},
}

// graph — рабочие импорты пакетов модуля (относительные пути) под одной ОС.
type graph map[string][]string

func load(t *testing.T, goos string) graph {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx := build.Default
	ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled = goos, "amd64", false
	g := graph{}
	err = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		name := d.Name()
		if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor") {
			return filepath.SkipDir
		}
		pkg, err := ctx.ImportDir(p, 0)
		if err != nil {
			return nil // нет файлов под эту ОС или каталог без Go
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		var imps []string
		for _, imp := range pkg.Imports {
			if strings.HasPrefix(imp, mod+"/") {
				imps = append(imps, strings.TrimPrefix(imp, mod+"/"))
			}
		}
		g[rel] = imps
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// deps — все пакеты модуля, которые pkg тянет (транзитивно).
func (g graph) deps(pkg string) map[string]bool {
	seen := map[string]bool{}
	var walk func(p string)
	walk = func(p string) {
		for _, d := range g[p] {
			if !seen[d] {
				seen[d] = true
				walk(d)
			}
		}
	}
	walk(pkg)
	return seen
}

// check — subject не тянет ни одного из forbidden, кроме рёбер базы; ребро
// базы, которого нет, — ошибка (база устарела).
func check(t *testing.T, goos string, g graph, subject string, forbidden []string) {
	t.Helper()
	if _, ok := g[subject]; !ok {
		return // пакета нет под эту ОС
	}
	deps := g.deps(subject)
	allowed := map[string]bool{}
	for _, b := range baseline[goos][subject] {
		allowed[b] = true
	}
	for _, f := range forbidden {
		switch {
		case deps[f] && !allowed[f]:
			t.Errorf("%s: %s тянет %s (ADR-0013 §1, §3)", goos, subject, f)
		case !deps[f] && allowed[f]:
			t.Errorf("%s: %s уже не тянет %s — уберите ребро из базы", goos, subject, f)
		}
	}
}

func TestLayers(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			g := load(t, goos)
			withDBus := append(append([]string{}, features...), "platform/dbus")

			// Ядро и window/minimal не тянут ни возможностей, ни D-Bus (он
			// нужен только возможностям Linux).
			check(t, goos, g, "window/core", withDBus)
			check(t, goos, g, "window/minimal", withDBus)

			// Бэкенды не тянут возможностей: возможности стоят на бэкендах.
			for _, b := range backends {
				check(t, goos, g, b, features)
			}

			// Слой platform не тянет ни движка, ни фасада widget, ни window.
			var platform []string
			for p := range g {
				if strings.HasPrefix(p, "platform/") {
					platform = append(platform, p)
				}
			}
			sort.Strings(platform)
			upper := []string{"engine", "widget", "window", "window/core", "window/minimal"}
			for _, p := range platform {
				check(t, goos, g, p, upper)
			}

			// window подключает каждую возможность, уже вынесенную из ядра.
			deps := g.deps("window")
			for _, f := range wired {
				if !deps[f] {
					t.Errorf("%s: window не подключает %s (window/capabilities.go)", goos, f)
				}
			}
		})
	}
}

// Сторож самого теста: граф не пуст и ядро в нём есть под каждой ОС.
func TestLayers_GraphSane(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		g := load(t, goos)
		if len(g["window/core"]) == 0 || len(g["window"]) == 0 {
			t.Errorf("%s: граф без window/core или window (%d пакетов)", goos, len(g))
		}
	}
}
