package facadegen

import (
	"path/filepath"
	"strings"
)

// Facade — фасад одного исходного пакета: из какого пакета и куда (Out —
// основной файл; файлы условий сборки — рядом, TagFile).
type Facade struct {
	Config Config
	Out    string
}

// Widget — файлы фасада widget относительно корня модуля root. Каждый шаг
// этапа 8 добавляет сюда свой подпакет.
func Widget(root string) []Facade {
	const mod = "github.com/oops1/headless-gui/v3/"
	pkg := func(dir, alias string) Facade {
		return Facade{
			Config: Config{
				Dir:        filepath.Join(root, "widget", dir),
				ImportPath: mod + "widget/" + dir,
				Alias:      alias,
				Package:    "widget",
				Tool:       "internal/facadegen (go run ./internal/facadegen/cmd/facadegen)",
			},
			Out: filepath.Join(root, "widget", alias+"_gen.go"),
		}
	}
	x := pkg("xaml", "xaml")
	// Загрузка разметки сперва регистрирует теги пакетов виджетов
	// (widget/xaml_register.go): программа без XAML их строителей не держит.
	x.Config.Prelude = func(name string) string {
		if strings.HasPrefix(name, "LoadUIFromXAML") {
			return "registerXAML()"
		}
		return ""
	}
	return []Facade{pkg("base", "base"), x, pkg("text", "text"), pkg("richtext", "richtext"), pkg("chrome", "chrome"), pkg("dock", "dock"), pkg("dialog", "dialog")}
}

// Window — файлы фасада window (ADR-0013) относительно корня модуля root:
// ядро window/core; каждый шаг этапа 9 добавляет свой пакет.
func Window(root string) []Facade {
	const mod = "github.com/oops1/headless-gui/v3/"
	return []Facade{{
		Config: Config{
			Dir:        filepath.Join(root, "window", "core"),
			ImportPath: mod + "window/core",
			Alias:      "core",
			Package:    "window",
			Tool:       "internal/facadegen (go run ./internal/facadegen/cmd/facadegen)",
		},
		Out: filepath.Join(root, "window", "core_gen.go"),
	}}
}

// All — все фасады модуля.
func All(root string) []Facade {
	return append(Widget(root), Window(root)...)
}
