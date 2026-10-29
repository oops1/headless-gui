package facadegen

import "path/filepath"

// Facade — один файл фасада: из какого пакета и куда.
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
	return []Facade{pkg("base", "base"), pkg("xaml", "xaml")}
}
