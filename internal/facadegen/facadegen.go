// Package facadegen генерирует фасад пакета: для каждого экспортированного
// имени исходного пакета — алиас типа, обёртку функции с копией
// документации, повторное объявление константы и переменной (ADR-0012,
// этап 8: `widget` — фасад над `widget/base` и подпакетами).
//
// Генератор работает по синтаксису (go/ast), а не по проверке типов: читает
// все файлы пакета, кроме тестов, независимо от тегов сборки, поэтому вывод
// один на всех ОС. Имя в сигнатуре — либо встроенный тип, либо тип того же
// пакета (в фасаде у него алиас с тем же именем), либо `pkg.Name` из импорта
// файла; импорты переносятся в фасад, совпадающие имена разных путей
// получают псевдонимы. Имя с директивой //facadegen:skip в документации в
// фасад не идёт (служебное, для соседних подпакетов). Имена из файлов с
// условием сборки (//go:build, суффикс ОС или архитектуры) — в отдельном
// файле фасада с тем же условием (TagFile). Обобщённые объявления не
// поддерживаются (их в пакетах нет); экспортированная функция с
// неэкспортированным типом в сигнатуре и одно имя в двух файлах с одинаковым
// условием сборки — ошибка генерации.
//
// Свежесть проверяет тест фасада — он сравнивает файл с выводом генератора
// (Check); полноту и тождество — тот же тест по типам: сигнатура обёртки
// тождественна исходной, алиас — тот же тип, у константы те же тип и
// значение. Компилятора для этого мало: параметр интерфейсного типа
// принял бы и чужой тип.
package facadegen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Config — какой пакет оборачивать и как назвать фасад.
type Config struct {
	// Dir — каталог исходного пакета.
	Dir string
	// ImportPath — путь импорта исходного пакета.
	ImportPath string
	// Alias — имя, под которым фасад импортирует исходный пакет.
	Alias string
	// Package — имя пакета фасада.
	Package string
	// Skip — экспортированные имена, которые фасад объявляет сам (не
	// генерируются).
	Skip map[string]bool
	// Prelude — оператор, который обёртка функции name выполняет перед
	// вызовом (например, ленивая регистрация); "" — ничего. nil — нигде.
	Prelude func(name string) string
	// Tool — как перегенерировать (в шапку файла).
	Tool string
}

// Names — экспортированные имена верхнего уровня пакета в каталоге dir (без
// тестов, по всем тегам сборки), по видам: "type", "func", "const", "var".
// Независимая от генератора выборка — для теста полноты фасада.
func Names(dir string) (map[string]string, error) {
	files, _, err := parseDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, f := range files {
		for _, d := range f.file.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() && !Skipped(d.Doc) {
					out[d.Name.Name] = "func"
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() && !Skipped(d.Doc, s.Doc) {
							out[s.Name.Name] = "type"
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							if n.IsExported() && !Skipped(d.Doc, s.Doc) {
								out[n.Name] = strings.ToLower(d.Tok.String())
							}
						}
					}
				}
			}
		}
	}
	return out, nil
}

type srcFile struct {
	name string
	file *ast.File
}

func parseDir(dir string) ([]srcFile, *token.FileSet, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	fset := token.NewFileSet()
	var files []srcFile
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, parser.ParseComments)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, srcFile{n, f})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return files, fset, nil
}

// Generate возвращает исходный текст основного файла фасада — имена из
// файлов пакета без условий сборки (отформатированный gofmt).
func Generate(cfg Config) ([]byte, error) {
	all, err := GenerateAll(cfg)
	if err != nil {
		return nil, err
	}
	return all[""], nil
}

// GenerateAll возвращает все файлы фасада: ключ "" — основной (имена из
// файлов без условий сборки), остальные ключи — условие сборки (//go:build
// или суффикс ОС в имени файла), под которым объявлены имена: фасад кладёт
// их в отдельный файл с тем же условием (TagFile). Имя, объявленное в двух
// файлах с разными условиями (своё на каждую ОС), — по файлу на условие.
func GenerateAll(cfg Config) (map[string][]byte, error) {
	files, fset, err := parseDir(cfg.Dir)
	if err != nil {
		return nil, err
	}
	if cfg.Alias == "" {
		cfg.Alias = path.Base(cfg.ImportPath)
	}

	// Типы пакета (все, в том числе неэкспортированные): имя в сигнатуре,
	// объявленное здесь, — тип пакета.
	localTypes := map[string]bool{}
	groups := map[string][]srcFile{}
	var keys []string
	where := map[string]map[string]string{} // условие → экспортированное имя → файл
	var errs []string
	for _, f := range files {
		c := constraintOf(f)
		if _, ok := groups[c]; !ok {
			keys = append(keys, c)
			where[c] = map[string]string{}
		}
		groups[c] = append(groups[c], f)
		for _, d := range f.file.Decls {
			switch d := d.(type) {
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						localTypes[s.Name.Name] = true
						if s.TypeParams != nil && s.Name.IsExported() {
							errs = append(errs, fmt.Sprintf("%s: обобщённый тип %s не поддерживается", f.name, s.Name.Name))
						}
						note(where[c], &errs, s.Name, f.name)
					case *ast.ValueSpec:
						for _, n := range s.Names {
							note(where[c], &errs, n, f.name)
						}
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil {
					note(where[c], &errs, d.Name, f.name)
				}
			}
		}
	}
	if _, ok := groups[""]; !ok {
		keys = append(keys, "")
	}
	out := map[string][]byte{}
	for _, c := range keys {
		src, gerrs := generateGroup(cfg, fset, localTypes, groups[c], c)
		errs = append(errs, gerrs...)
		if src != nil {
			out[c] = src
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("facadegen: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

// generateGroup — один файл фасада из файлов group с условием сборки c
// ("" — без условия). Пустой файл с условием не нужен: nil.
func generateGroup(cfg Config, fset *token.FileSet, localTypes map[string]bool, group []srcFile, c string) ([]byte, []string) {
	g := &gen{cfg: cfg, fset: fset, localTypes: localTypes, imports: map[string]string{}, names: map[string]string{}}
	g.names[cfg.Alias] = cfg.ImportPath
	g.imports[cfg.ImportPath] = cfg.Alias
	var body bytes.Buffer
	for _, f := range group {
		g.file = f
		g.fileImports = map[string]string{}
		for _, im := range f.file.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			name := path.Base(p)
			if im.Name != nil {
				name = im.Name.Name
			}
			if name == "_" || name == "." {
				continue
			}
			g.fileImports[name] = p
		}
		var out bytes.Buffer
		for _, d := range f.file.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				g.funcDecl(&out, d)
			case *ast.GenDecl:
				g.genDecl(&out, d)
			}
		}
		if out.Len() > 0 {
			fmt.Fprintf(&body, "\n// ── %s ──\n\n", f.name)
			body.Write(out.Bytes())
		}
	}
	if len(g.errs) > 0 {
		return nil, g.errs
	}
	if c != "" && body.Len() == 0 {
		return nil, nil
	}

	var hdr bytes.Buffer
	tool := cfg.Tool
	if tool == "" {
		tool = "internal/facadegen"
	}
	if c != "" {
		fmt.Fprintf(&hdr, "//go:build %s\n\n", c)
	}
	fmt.Fprintf(&hdr, "// Code generated by %s; DO NOT EDIT.\n\n", tool)
	fmt.Fprintf(&hdr, "// Фасад %s: имена пакета %s.\n\n", cfg.Package, cfg.ImportPath)
	fmt.Fprintf(&hdr, "package %s\n\nimport (\n", cfg.Package)
	paths := make([]string, 0, len(g.imports))
	for p := range g.imports {
		paths = append(paths, p)
	}
	// Стандартная библиотека первой группой, как у goimports.
	std := func(p string) bool { return !strings.Contains(strings.SplitN(p, "/", 2)[0], ".") }
	sort.Slice(paths, func(i, j int) bool {
		if std(paths[i]) != std(paths[j]) {
			return std(paths[i])
		}
		return paths[i] < paths[j]
	})
	for i, p := range paths {
		if i > 0 && std(paths[i-1]) && !std(p) {
			hdr.WriteByte('\n')
		}
		if g.imports[p] == path.Base(p) {
			fmt.Fprintf(&hdr, "\t%q\n", p)
		} else {
			fmt.Fprintf(&hdr, "\t%s %q\n", g.imports[p], p)
		}
	}
	hdr.WriteString(")\n")
	hdr.Write(body.Bytes())
	src, err := format.Source(hdr.Bytes())
	if err != nil {
		return nil, []string{fmt.Sprintf("gofmt: %v", err)}
	}
	return src, nil
}

// goosSuffix и goarchSuffix — ОС и архитектуры, которые Go распознаёт по
// суффиксу имени файла (_GOOS, _GOARCH, _GOOS_GOARCH; список go tool dist).
var (
	goosSuffix   = []string{"aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos", "ios", "js", "linux", "nacl", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows", "zos"}
	goarchSuffix = []string{"386", "amd64", "amd64p32", "arm", "armbe", "arm64", "arm64be", "loong64", "mips", "mipsle", "mips64", "mips64le", "mips64p32", "mips64p32le", "ppc", "ppc64", "ppc64le", "riscv", "riscv64", "s390", "s390x", "sparc", "sparc64", "wasm"}
)

// constraintOf — условие сборки файла: строка //go:build и (или) суффикс ОС
// в имени ("" — без условия).
func constraintOf(f srcFile) string {
	var parts []string
	for _, cg := range f.file.Comments {
		if cg.Pos() >= f.file.Package {
			break
		}
		for _, c := range cg.List {
			if t := strings.TrimSpace(c.Text); strings.HasPrefix(t, "//go:build ") {
				parts = append(parts, strings.TrimSpace(strings.TrimPrefix(t, "//go:build ")))
			}
		}
	}
	// Суффиксы имени: _GOOS, _GOARCH, _GOOS_GOARCH (как у go/build).
	elems := strings.Split(strings.TrimSuffix(f.name, ".go"), "_")
	var tags []string
	if n := len(elems); n >= 3 && in(goosSuffix, elems[n-2]) && in(goarchSuffix, elems[n-1]) {
		tags = []string{elems[n-2], elems[n-1]}
	} else if n >= 2 && (in(goosSuffix, elems[n-1]) || in(goarchSuffix, elems[n-1])) {
		tags = []string{elems[n-1]}
	}
	for i := len(tags) - 1; i >= 0; i-- {
		// Тег из суффикса уже стоит в выражении конъюнктом (//go:build
		// linux && !android у x_linux.go) — второй раз не пишем: одинаковые
		// условия — один файл. При || в выражении — пишем всегда.
		if !conjunct(parts, tags[i]) {
			parts = append([]string{tags[i]}, parts...)
		}
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	for i, p := range parts {
		if strings.Contains(p, "||") {
			parts[i] = "(" + p + ")"
		}
	}
	return strings.Join(parts, " && ")
}

func in(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// conjunct — tag стоит конъюнктом верхнего уровня в одном из выражений
// (выражение без ||: a && b && tag).
func conjunct(parts []string, tag string) bool {
	for _, p := range parts {
		if strings.Contains(p, "||") {
			continue
		}
		for _, t := range strings.Split(p, "&&") {
			if strings.Trim(strings.TrimSpace(t), "()") == tag {
				return true
			}
		}
	}
	return false
}

// TagFile — имя файла фасада для условия сборки c рядом с основным out.
// Суффикс _gen.go не похож на ОС, поэтому условие задаёт только //go:build.
func TagFile(out, alias, c string) string {
	r := strings.NewReplacer("!", "not_", "&&", "and", "||", "or", "(", "", ")", "", " ", "_")
	name := r.Replace(c)
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	return filepath.Join(filepath.Dir(out), alias+"_tag_"+strings.Trim(name, "_")+"_gen.go")
}

// Files — все файлы фасада f: путь → содержимое.
func Files(f Facade) (map[string][]byte, error) {
	all, err := GenerateAll(f.Config)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for c, src := range all {
		if c == "" {
			out[f.Out] = src
		} else {
			out[TagFile(f.Out, f.Config.Alias, c)] = src
		}
	}
	return out, nil
}

// Stale — файлы условий сборки фасада f на диске, которых генератор больше не
// даёт (их надо удалить).
func Stale(f Facade, want map[string][]byte) ([]string, error) {
	pattern := filepath.Join(filepath.Dir(f.Out), f.Config.Alias+"_tag_*_gen.go")
	have, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	var stale []string
	for _, h := range have {
		if _, ok := want[h]; !ok {
			stale = append(stale, h)
		}
	}
	return stale, nil
}

// Check сравнивает файлы фасада f с выводом генератора. Пустая строка —
// совпадают; иначе — первое различие.
func Check(f Facade) (string, error) {
	want, err := Files(f)
	if err != nil {
		return "", err
	}
	stale, err := Stale(f, want)
	if err != nil {
		return "", err
	}
	if len(stale) > 0 {
		return "лишние файлы фасада: " + strings.Join(stale, ", "), nil
	}
	names := make([]string, 0, len(want))
	for n := range want {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, file := range names {
		got, err := os.ReadFile(file)
		if err != nil {
			return file + ": нет файла", nil
		}
		got = bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n"))
		if d := diff(file, got, want[file]); d != "" {
			return d, nil
		}
	}
	return "", nil
}

func diff(file string, got, want []byte) string {
	if bytes.Equal(got, want) {
		return ""
	}
	gl, wl := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
	for i := 0; i < len(gl) || i < len(wl); i++ {
		var a, b string
		if i < len(gl) {
			a = gl[i]
		}
		if i < len(wl) {
			b = wl[i]
		}
		if a != b {
			return fmt.Sprintf("%s:%d: в файле %q, генератор даёт %q", file, i+1, a, b)
		}
	}
	return file + ": отличается"
}

func note(where map[string]string, errs *[]string, id *ast.Ident, file string) {
	if !id.IsExported() {
		return
	}
	if prev, ok := where[id.Name]; ok {
		*errs = append(*errs, fmt.Sprintf("имя %s объявлено в %s и %s", id.Name, prev, file))
		return
	}
	where[id.Name] = file
}

type gen struct {
	cfg         Config
	fset        *token.FileSet
	localTypes  map[string]bool
	imports     map[string]string // путь → имя в фасаде
	names       map[string]string // имя в фасаде → путь
	file        srcFile
	fileImports map[string]string // имя в файле → путь
	errs        []string
}

func (g *gen) skip(id *ast.Ident, docs ...*ast.CommentGroup) bool {
	return !id.IsExported() || g.cfg.Skip[id.Name] || Skipped(docs...)
}

// SkipDirective — строка документации, которая убирает имя из фасада:
// служебное имя пакета, экспортированное для соседних подпакетов (ADR-0012).
// Как директива (без пробела после //) она не видна в go doc.
const SkipDirective = "//facadegen:skip"

// Skipped сообщает, что в документации есть SkipDirective.
func Skipped(docs ...*ast.CommentGroup) bool {
	for _, cg := range docs {
		if cg == nil {
			continue
		}
		for _, c := range cg.List {
			if strings.TrimSpace(c.Text) == SkipDirective {
				return true
			}
		}
	}
	return false
}

// importName — имя пакета p в фасаде; при совпадении имён разных путей —
// псевдоним из двух последних элементов пути.
func (g *gen) importName(p, preferred string) string {
	if n, ok := g.imports[p]; ok {
		return n
	}
	n := preferred
	if q, taken := g.names[n]; taken && q != p {
		parts := strings.Split(p, "/")
		n = ""
		if len(parts) > 1 {
			n = parts[len(parts)-2]
		}
		n = strings.NewReplacer(".", "", "-", "", "_", "").Replace(n + parts[len(parts)-1])
		for i := 2; ; i++ {
			if q, taken := g.names[n]; !taken || q == p {
				break
			}
			n = fmt.Sprintf("%s%d", preferred, i)
		}
	}
	g.imports[p] = n
	g.names[n] = p
	return n
}

// expr печатает выражение типа из сигнатуры в пространстве имён фасада.
func (g *gen) expr(e ast.Expr) string {
	var walk func(ast.Node) bool
	walk = func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Field:
			// Имена параметров и полей внутри типа — не типы.
			ast.Inspect(n.Type, walk)
			return false
		case *ast.SelectorExpr:
			if x, ok := n.X.(*ast.Ident); ok {
				p, ok := g.fileImports[x.Name]
				if !ok {
					g.errs = append(g.errs, fmt.Sprintf("%s: пакет %s в сигнатуре не найден среди импортов (имя пакета не совпадает с последним элементом пути — дайте импорту имя)", g.file.name, x.Name))
					return false
				}
				x.Name = g.importName(p, x.Name)
				return false
			}
		case *ast.Ident:
			if g.localTypes[n.Name] && !n.IsExported() {
				g.errs = append(g.errs, fmt.Sprintf("%s: неэкспортированный тип %s в сигнатуре экспортированного имени", g.file.name, n.Name))
			}
		case *ast.FuncLit, *ast.BlockStmt:
			return false
		}
		return true
	}
	ast.Inspect(e, walk)
	var b bytes.Buffer
	if err := printer.Fprint(&b, g.fset, e); err != nil {
		g.errs = append(g.errs, err.Error())
	}
	return b.String()
}

func (g *gen) doc(out *bytes.Buffer, cg *ast.CommentGroup) {
	if cg == nil {
		return
	}
	for _, c := range cg.List {
		out.WriteString(c.Text)
		out.WriteByte('\n')
	}
}

func (g *gen) funcDecl(out *bytes.Buffer, d *ast.FuncDecl) {
	if d.Recv != nil || g.skip(d.Name, d.Doc) {
		return
	}
	if d.Type.TypeParams != nil {
		g.errs = append(g.errs, fmt.Sprintf("%s: обобщённая функция %s не поддерживается", g.file.name, d.Name.Name))
		return
	}
	g.doc(out, d.Doc)
	var params, args []string
	used := map[string]bool{}
	for _, fld := range d.Type.Params.List {
		for _, n := range fld.Names {
			used[n.Name] = true
		}
	}
	if r := d.Type.Results; r != nil {
		for _, fld := range r.List {
			for _, n := range fld.Names {
				used[n.Name] = true
			}
		}
	}
	i := 0
	for _, fld := range d.Type.Params.List {
		typ := g.expr(fld.Type)
		_, variadic := fld.Type.(*ast.Ellipsis)
		names := fld.Names
		if len(names) == 0 {
			names = []*ast.Ident{{Name: "_"}}
		}
		var ns []string
		for _, n := range names {
			name := n.Name
			if name == "_" || name == g.cfg.Alias {
				name = fmt.Sprintf("a%d", i)
				for used[name] {
					name += "_"
				}
				used[name] = true
			}
			i++
			ns = append(ns, name)
			if variadic {
				args = append(args, name+"...")
			} else {
				args = append(args, name)
			}
		}
		params = append(params, strings.Join(ns, ", ")+" "+typ)
	}
	res := ""
	if r := d.Type.Results; r != nil && len(r.List) > 0 {
		var rs []string
		named := false
		for _, fld := range r.List {
			typ := g.expr(fld.Type)
			if len(fld.Names) == 0 {
				rs = append(rs, typ)
				continue
			}
			named = true
			var ns []string
			for _, n := range fld.Names {
				name := n.Name
				if name == g.cfg.Alias {
					name = "r" + name
					for used[name] {
						name += "_"
					}
					used[name] = true
				}
				ns = append(ns, name)
			}
			rs = append(rs, strings.Join(ns, ", ")+" "+typ)
		}
		if len(rs) == 1 && !named {
			res = " " + rs[0]
		} else {
			res = " (" + strings.Join(rs, ", ") + ")"
		}
	}
	call := fmt.Sprintf("%s.%s(%s)", g.cfg.Alias, d.Name.Name, strings.Join(args, ", "))
	if res != "" {
		call = "return " + call
	}
	if g.cfg.Prelude != nil {
		if pre := g.cfg.Prelude(d.Name.Name); pre != "" {
			call = pre + "\n" + call
		}
	}
	fmt.Fprintf(out, "func %s(%s)%s { %s }\n\n", d.Name.Name, strings.Join(params, ", "), res, call)
}

func (g *gen) genDecl(out *bytes.Buffer, d *ast.GenDecl) {
	switch d.Tok {
	case token.TYPE:
		groupDoc := len(d.Specs) > 1 && d.Doc != nil
		for _, s := range d.Specs {
			ts := s.(*ast.TypeSpec)
			if g.skip(ts.Name, d.Doc, ts.Doc) {
				continue
			}
			if groupDoc {
				// Комментарий группы type (...) — отдельным абзацем перед
				// первым алиасом группы.
				g.doc(out, d.Doc)
				out.WriteByte('\n')
				groupDoc = false
			}
			doc := ts.Doc
			if doc == nil && len(d.Specs) == 1 {
				doc = d.Doc
			}
			g.doc(out, doc)
			fmt.Fprintf(out, "type %s = %s.%s\n\n", ts.Name.Name, g.cfg.Alias, ts.Name.Name)
		}
	case token.CONST, token.VAR:
		var specs bytes.Buffer
		n := 0
		for _, s := range d.Specs {
			vs := s.(*ast.ValueSpec)
			var names, vals []string
			for _, id := range vs.Names {
				if g.skip(id, d.Doc, vs.Doc) {
					continue
				}
				names = append(names, id.Name)
				vals = append(vals, g.cfg.Alias+"."+id.Name)
			}
			if len(names) == 0 {
				continue
			}
			if len(d.Specs) > 1 {
				g.doc(&specs, vs.Doc)
			}
			fmt.Fprintf(&specs, "%s = %s", strings.Join(names, ", "), strings.Join(vals, ", "))
			if vs.Comment != nil {
				for _, c := range vs.Comment.List {
					specs.WriteString(" " + c.Text)
				}
			}
			specs.WriteByte('\n')
			n++
		}
		if n == 0 {
			return
		}
		kw := d.Tok.String()
		if len(d.Specs) == 1 {
			doc := d.Specs[0].(*ast.ValueSpec).Doc
			if doc == nil {
				doc = d.Doc
			}
			g.doc(out, doc)
			fmt.Fprintf(out, "%s %s\n", kw, specs.String())
			return
		}
		g.doc(out, d.Doc)
		fmt.Fprintf(out, "%s (\n%s)\n\n", kw, specs.String())
	}
}
