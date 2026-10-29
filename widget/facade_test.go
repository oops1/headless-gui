package widget

import (
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/facadegen"
)

// Проверки фасада — internal/facadegen (verify.go); глобального состояния
// тесты не трогают — только читают исходники.

// Фасад свеж: файлы *_gen.go совпадают с выводом генератора.
func TestFacade_UpToDate(t *testing.T) {
	facadegen.VerifyUpToDate(t, facadegen.Widget(".."))
}

// Фасад полон: каждое экспортированное имя исходных пакетов объявлено в
// widget тем же видом, лишних имён нет.
func TestFacade_Complete(t *testing.T) {
	facadegen.VerifyComplete(t, facadegen.Widget(".."), ".", 500)
}

// Фасад тождествен исходным пакетам по типам.
func TestFacade_TypesIdentical(t *testing.T) {
	if testing.Short() {
		t.Skip("проверка типов из исходников — около 10 с")
	}
	facadegen.VerifyTypesIdentical(t, facadegen.Widget(".."), "github.com/oops1/headless-gui/v3/widget", 500)
}

var updateAPI = flag.Bool("update-api", false, "переписать testdata/api.txt по фасаду")

// Снимок открытого API фасада: любое изменение набора имён — осознанное, с
// правкой testdata/api.txt (go test ./widget -run APISnapshot -update-api).
func TestFacade_APISnapshot(t *testing.T) {
	facadegen.VerifyAPISnapshot(t, ".", "testdata/api.txt", *updateAPI)
}

// Каждая обёртка xaml, которая строит виджеты (результат с Widget), сперва
// регистрирует теги пакетов виджетов (registerXAML). Prelude генератора
// выбирает их по имени; новый загрузчик с другим именем без регистрации
// падал бы на <TextBox> «неизвестный элемент».
func TestFacade_XAMLEntriesRegister(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "xaml_gen.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Type.Results == nil {
			continue
		}
		builds := false
		for _, r := range fd.Type.Results.List {
			ast.Inspect(r.Type, func(x ast.Node) bool {
				if id, ok := x.(*ast.Ident); ok && id.Name == "Widget" {
					builds = true
				}
				return true
			})
		}
		if !builds {
			continue
		}
		n++
		first := ""
		if len(fd.Body.List) > 0 {
			if es, ok := fd.Body.List[0].(*ast.ExprStmt); ok {
				if c, ok := es.X.(*ast.CallExpr); ok {
					if id, ok := c.Fun.(*ast.Ident); ok {
						first = id.Name
					}
				}
			}
		}
		if first != "registerXAML" {
			t.Errorf("%s строит виджеты, но не зовёт registerXAML() первым (Prelude в internal/facadegen/widget.go)", fd.Name.Name)
		}
	}
	if n < 8 {
		t.Errorf("обёрток загрузки XAML %d — ждали не меньше 8", n)
	}
}
