package widget

import (
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/facadegen"
)

// Фасад свеж: файлы *_gen.go совпадают с выводом генератора. Глобального
// состояния тест не трогает — только читает исходники.
func TestFacade_UpToDate(t *testing.T) {
	for _, f := range facadegen.Widget("..") {
		diff, err := facadegen.Check(f.Config, f.Out)
		if err != nil {
			t.Fatal(err)
		}
		if diff != "" {
			t.Errorf("фасад устарел, перегенерируйте: go run ./internal/facadegen/cmd/facadegen\n%s", diff)
		}
	}
}

// Фасад полон: каждое экспортированное имя исходных пакетов объявлено в
// widget тем же видом (алиас — для типа, функция — для функции), лишних
// имён нет. Выборка имён фасада — отдельная от генератора, по всем файлам
// пакета.
func TestFacade_Complete(t *testing.T) {
	want := map[string]string{}
	for _, f := range facadegen.Widget("..") {
		names, err := facadegen.Names(f.Config.Dir)
		if err != nil {
			t.Fatal(err)
		}
		for n, kind := range names {
			if prev, ok := want[n]; ok {
				t.Errorf("имя %s есть в двух исходных пакетах (%s, %s)", n, prev, kind)
			}
			want[n] = kind
		}
	}
	got := facadeNames(t)
	var missing, extra, wrong []string
	for n, kind := range want {
		g, ok := got[n]
		switch {
		case !ok:
			missing = append(missing, n)
		case g != kind:
			wrong = append(wrong, n+": "+kind+" → "+g)
		}
	}
	for n := range got {
		if _, ok := want[n]; !ok {
			extra = append(extra, n)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	sort.Strings(wrong)
	if len(missing) > 0 {
		t.Errorf("в фасаде нет %d имён: %s", len(missing), strings.Join(missing, ", "))
	}
	if len(extra) > 0 {
		t.Errorf("в фасаде лишние имена: %s", strings.Join(extra, ", "))
	}
	if len(wrong) > 0 {
		t.Errorf("вид имени в фасаде другой: %s", strings.Join(wrong, "; "))
	}
	if len(want) < 500 {
		t.Errorf("исходных имён %d — выборка сломана?", len(want))
	}
}

// Фасад тождествен исходным пакетам по типам: алиас — тот же тип, у
// обёртки та же сигнатура (параметр интерфейсного типа компилятор пропустил
// бы и с чужим типом), у константы те же тип и значение, у переменной тот же
// тип. Пакеты проверяются из исходников одним импортёром, поэтому типы base
// и фасада — из одного прохода.
func TestFacade_TypesIdentical(t *testing.T) {
	if testing.Short() {
		t.Skip("проверка типов из исходников — около 10 с")
	}
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "source", nil)
	facade, err := imp.Import("github.com/oops1/headless-gui/v3/widget")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range facadegen.Widget("..") {
		src, err := imp.Import(f.Config.ImportPath)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range src.Scope().Names() {
			b := src.Scope().Lookup(name)
			if !b.Exported() || f.Config.Skip[name] {
				continue
			}
			w := facade.Scope().Lookup(name)
			if w == nil {
				t.Errorf("%s: нет в фасаде", name)
				continue
			}
			n++
			if !types.Identical(w.Type(), b.Type()) {
				t.Errorf("%s: в фасаде %s, в %s %s", name, w.Type(), src.Name(), b.Type())
			}
			switch b := b.(type) {
			case *types.TypeName:
				if wt, ok := w.(*types.TypeName); !ok || !wt.IsAlias() {
					t.Errorf("%s: в фасаде не алиас типа", name)
				}
			case *types.Func:
				if _, ok := w.(*types.Func); !ok {
					t.Errorf("%s: в фасаде не функция", name)
				}
			case *types.Const:
				wc, ok := w.(*types.Const)
				if !ok || !constant.Compare(wc.Val(), token.EQL, b.Val()) {
					t.Errorf("%s: в фасаде не та константа", name)
				}
			case *types.Var:
				if _, ok := w.(*types.Var); !ok {
					t.Errorf("%s: в фасаде не переменная", name)
				}
			}
		}
	}
	if n < 500 {
		t.Errorf("сверено %d имён — импорт сломан?", n)
	}
}

// facadeNames — экспортированные имена пакета widget с видом; тип должен
// быть алиасом.
func facadeNames(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", n), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil {
					t.Errorf("%s: метод %s в фасаде — у фасада нет своих типов", n, d.Name.Name)
				} else if d.Name.IsExported() {
					out[d.Name.Name] = "func"
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						if !s.Assign.IsValid() {
							t.Errorf("%s: тип %s в фасаде не алиас", n, s.Name.Name)
						}
						if s.Name.IsExported() {
							out[s.Name.Name] = "type"
						}
					case *ast.ValueSpec:
						for _, id := range s.Names {
							if id.IsExported() {
								out[id.Name] = strings.ToLower(d.Tok.String())
							}
						}
					}
				}
			}
		}
	}
	return out
}
