package testreset

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// RequireInEveryTest падает, если какой-нибудь тест пакета (файлы *_test.go
// в текущем каталоге) начинается не с testreset.Globals(t).
//
// Договорённость пакета: общее состояние процесса сбрасывается первой строкой
// каждого теста. Эта проверка следит, чтобы новые тесты её не забывали:
// падение здесь дешевле, чем «плавающий» отказ чужого теста через неделю.
// minTests — нижняя граница числа найденных тестов: защита от молчаливо
// сломанного разбора файлов.
func RequireInEveryTest(t *testing.T, minTests int) {
	t.Helper()
	files, err := filepath.Glob("*_test.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("тестовые файлы не найдены: %v", err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, name := range files {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") || !isTestSignature(fn) {
				continue
			}
			checked++
			if !startsWithGlobalsReset(fn) {
				t.Errorf("%s: %s не начинается с testreset.Globals(t) — общее состояние процесса "+
					"протечёт в соседние тесты и сломает их при -shuffle=on",
					name, fn.Name.Name)
			}
		}
	}
	if checked < minTests {
		t.Errorf("проверено всего %d тестов (ждали не меньше %d) — разбор файлов сломан?", checked, minTests)
	}
}

// isTestSignature — func TestXxx(t *testing.T).
func isTestSignature(fn *ast.FuncDecl) bool {
	ps := fn.Type.Params.List
	if len(ps) != 1 || len(ps[0].Names) != 1 {
		return false
	}
	star, ok := ps[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "T"
}

// startsWithGlobalsReset сообщает, что первое выражение тела —
// testreset.Globals(<параметр теста>).
func startsWithGlobalsReset(fn *ast.FuncDecl) bool {
	if fn.Body == nil || len(fn.Body.List) == 0 {
		return false
	}
	es, ok := fn.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := es.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Globals" {
		return false
	}
	if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "testreset" {
		return false
	}
	arg, ok := call.Args[0].(*ast.Ident)
	return ok && arg.Name == fn.Type.Params.List[0].Names[0].Name
}
