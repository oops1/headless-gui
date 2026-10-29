package facadegen

// verify.go — проверки фасада для тестов пакета-фасада (widget, window):
// свежесть, полнота, тождество по типам, снимок открытого API. Тест зовёт
// их из каталога фасада.

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
)

// VerifyUpToDate — файлы *_gen.go фасада совпадают с выводом генератора,
// лишних файлов условий сборки нет.
func VerifyUpToDate(t testing.TB, facades []Facade) {
	t.Helper()
	for _, f := range facades {
		diff, err := Check(f)
		if err != nil {
			t.Fatal(err)
		}
		if diff != "" {
			t.Errorf("фасад устарел, перегенерируйте: go run ./internal/facadegen/cmd/facadegen\n%s", diff)
		}
	}
}

// VerifyComplete — каждое экспортированное имя исходных пакетов объявлено в
// фасаде (каталог dir) тем же видом (алиас — для типа, функция — для
// функции), лишних имён, кроме рукописных файлов фасада, нет. Выборка имён
// фасада — отдельная от генератора, по всем файлам пакета. min — нижняя
// граница числа исходных имён (защита от сломанной выборки).
func VerifyComplete(t testing.TB, facades []Facade, dir string, min int) {
	t.Helper()
	want := map[string]string{}
	for _, f := range facades {
		names, err := Names(f.Config.Dir)
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
	got, hand := FacadeNames(t, dir)
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
		if _, ok := want[n]; !ok && !hand[n] {
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
	if len(want) < min {
		t.Errorf("исходных имён %d — выборка сломана?", len(want))
	}
}

// VerifyTypesIdentical — фасад facadePath тождествен исходным пакетам по
// типам (под текущей ОС): алиас — тот же тип, у обёртки та же сигнатура
// (параметр интерфейсного типа компилятор пропустил бы и с чужим типом), у
// константы те же тип и значение, у переменной тот же тип. Пакеты
// проверяются из исходников одним импортёром.
func VerifyTypesIdentical(t testing.TB, facades []Facade, facadePath string, min int) {
	t.Helper()
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "source", nil)
	facade, err := imp.Import(facadePath)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range facades {
		src, err := imp.Import(f.Config.ImportPath)
		if err != nil {
			t.Fatal(err)
		}
		// Имена фасада — экспортированные без //facadegen:skip.
		names, err := Names(f.Config.Dir)
		if err != nil {
			t.Fatal(err)
		}
		for name := range names {
			b := src.Scope().Lookup(name)
			if b == nil || f.Config.Skip[name] {
				continue // имя другой ОС или объявленное фасадом
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
	if n < min {
		t.Errorf("сверено %d имён — импорт сломан?", n)
	}
}

// VerifyAPISnapshot — имена и виды фасада (каталог dir) совпадают со
// снимком file. Проверки выше сверяют фасад с исходными пакетами тем же
// разбором, что и генератор, поэтому лишняя директива //facadegen:skip на
// публичном имени прошла бы их молча; снимок ловит это: любое изменение
// набора имён — осознанное, с update.
func VerifyAPISnapshot(t testing.TB, dir, file string, update bool) {
	t.Helper()
	got, _ := FacadeNames(t, dir)
	lines := make([]string, 0, len(got))
	for n, kind := range got {
		lines = append(lines, kind+" "+n)
	}
	sort.Strings(lines)
	if update {
		if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, l := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if l != "" {
			want[l] = true
		}
	}
	have := map[string]bool{}
	for _, l := range lines {
		have[l] = true
		if !want[l] {
			t.Errorf("новое имя в фасаде: %s (если осознанно — -update-api)", l)
		}
	}
	for l := range want {
		if !have[l] {
			t.Errorf("из фасада пропало: %s", l)
		}
	}
}

// FacadeNames — экспортированные имена пакета-фасада в каталоге dir (все
// файлы, по всем условиям сборки) с видом и множество имён из рукописных
// файлов (не *_gen.go). В сгенерированных файлах тип должен быть алиасом,
// методов нет.
func FacadeNames(t testing.TB, dir string) (map[string]string, map[string]bool) {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	hand := map[string]bool{}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		gen := strings.HasSuffix(n, "_gen.go")
		mark := func(name string) {
			if !gen {
				hand[name] = true
			}
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil {
					if gen {
						t.Errorf("%s: метод %s в сгенерированном фасаде", n, d.Name.Name)
					}
				} else if d.Name.IsExported() {
					out[d.Name.Name] = "func"
					mark(d.Name.Name)
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						if gen && !s.Assign.IsValid() {
							t.Errorf("%s: тип %s в фасаде не алиас", n, s.Name.Name)
						}
						if s.Name.IsExported() {
							out[s.Name.Name] = "type"
							mark(s.Name.Name)
						}
					case *ast.ValueSpec:
						for _, id := range s.Names {
							if id.IsExported() {
								out[id.Name] = strings.ToLower(d.Tok.String())
								mark(id.Name)
							}
						}
					}
				}
			}
		}
	}
	return out, hand
}
