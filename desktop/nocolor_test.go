package desktop

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Компоненты рабочего стола не знают цветов.
//
// Это не стилистическое пожелание, а условие, на котором держится вся
// затея: одна и та же панель задач под профилем Windows 11 — полоса кнопок
// со стеклом, под Windows 2000 — ряд кнопок с фасками, под macOS — док.
// Возможно это ровно до тех пор, пока в отрисовке нет ни одного цвета,
// зашитого в код: первый же литерал переживёт смену темы и останется
// чужеродным пятном.
//
// Тест разбирает исходники пакета и ищет литералы цвета внутри функций
// отрисовки. Проверять глазами это невозможно — цвет добавляется одной
// строкой и не ломает ничего, кроме темизации.
func TestDraw_HasNoColorLiterals(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	found := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		// fakes.go — тестовая оснастка, а не отрисовка: в ней нет Draw.
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !isPaintFunc(fn.Name.Name) {
				return true
			}
			ast.Inspect(fn.Body, func(inner ast.Node) bool {
				lit, ok := inner.(*ast.CompositeLit)
				if !ok {
					return true
				}
				if !isColorLiteral(lit.Type) {
					return true
				}
				pos := fset.Position(lit.Pos())
				t.Errorf("%s: в %s() литерал цвета — цвет обязан приходить из темы",
					pos, fn.Name.Name)
				found++
				return true
			})
			return true
		})
	}
	if found > 0 {
		t.Logf("найдено литералов: %d", found)
	}
}

// isPaintFunc — функция участвует в отрисовке.
func isPaintFunc(name string) bool {
	return name == "Draw" || strings.HasPrefix(name, "draw") || strings.HasPrefix(name, "paint")
}

// isColorLiteral — тип литерала color.RGBA (в любом написании импорта).
func isColorLiteral(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return pkg.Name == "color" && strings.HasPrefix(sel.Sel.Name, "RGBA")
}

// TestDraw_AsksThemeForStyle — компоненты действительно спрашивают тему, а
// не рисуют «как получится». Файл компонента, в котором нет ни одного
// обращения к стилю или метрике, либо ничего не рисует, либо рисует мимо
// темы — и то и другое стоит заметить.
func TestDraw_AsksThemeForStyle(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		switch path {
		case "contract.go", "fakes.go", "paint.go":
			continue // контракт, оснастка и общая отрисовка — не компоненты
		}
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(src)
		if !strings.Contains(text, "func (") || !strings.Contains(text, ") Draw(") {
			continue // в файле нет отрисовки
		}
		if !strings.Contains(text, "GetStyle") && !strings.Contains(text, "GetMetric") &&
			!strings.Contains(text, ".style(") && !strings.Contains(text, ".metric(") {
			t.Errorf("%s рисует, но ни разу не спрашивает тему", path)
		}
	}
}
