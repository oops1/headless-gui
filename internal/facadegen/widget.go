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
// ядро window/core и открытые имена слоя ниже бэкендов — platform/native
// (NativeWindow, VK_*, Monitor, HitArea, DragEffect, ErrIconUnsupported) и
// platform/keyboard (LockKeys). platform/pointer и platform/dbus открытых имён
// window не дают.
func Window(root string) []Facade { return windowFacades(root, "window", "window") }

// WindowMinimal — файлы фасада window/minimal (ADR-0013 §3): те же источники
// и те же имена, что у Window, — тип Window один, окна взаимозаменяемы.
// Отличие фасадов — рукописный window/capabilities.go: window подключает
// возможности окна (import _), window/minimal — нет.
func WindowMinimal(root string) []Facade { return windowFacades(root, "window/minimal", "minimal") }

// windowFacades — фасад окна в каталоге dir с именем пакета pkg.
func windowFacades(root, dir, pkgName string) []Facade {
	const mod = "github.com/oops1/headless-gui/v3/"
	pkg := func(src, alias string) Facade {
		return Facade{
			Config: Config{
				Dir:        filepath.Join(root, filepath.FromSlash(src)),
				ImportPath: mod + src,
				Alias:      alias,
				Package:    pkgName,
				Tool:       "internal/facadegen (go run ./internal/facadegen/cmd/facadegen)",
			},
			Out: filepath.Join(root, filepath.FromSlash(dir), alias+"_gen.go"),
		}
	}
	return []Facade{pkg("window/core", "core"), pkg("platform/native", "native"), pkg("platform/keyboard", "keyboard")}
}

// All — все фасады модуля.
func All(root string) []Facade {
	return append(append(Widget(root), Window(root)...), WindowMinimal(root)...)
}
