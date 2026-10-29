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
// получают псевдонимы. Обобщённые объявления не поддерживаются (их в
// пакетах нет); экспортированная функция с неэкспортированным типом в
// сигнатуре и одно имя в двух файлах — ошибка генерации.
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
				if d.Recv == nil && d.Name.IsExported() {
					out[d.Name.Name] = "func"
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							out[s.Name.Name] = "type"
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							if n.IsExported() {
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

// Generate возвращает исходный текст фасада (отформатированный gofmt).
func Generate(cfg Config) ([]byte, error) {
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
	where := map[string]string{} // экспортированное имя → файл
	var errs []string
	for _, f := range files {
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
						note(where, &errs, s.Name, f.name)
					case *ast.ValueSpec:
						for _, n := range s.Names {
							note(where, &errs, n, f.name)
						}
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil {
					note(where, &errs, d.Name, f.name)
				}
			}
		}
	}

	g := &gen{cfg: cfg, fset: fset, localTypes: localTypes, imports: map[string]string{}, names: map[string]string{}}
	g.names[cfg.Alias] = cfg.ImportPath
	g.imports[cfg.ImportPath] = cfg.Alias
	var body bytes.Buffer
	for _, f := range files {
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
	errs = append(errs, g.errs...)
	if len(errs) > 0 {
		return nil, fmt.Errorf("facadegen: %s", strings.Join(errs, "; "))
	}

	var hdr bytes.Buffer
	tool := cfg.Tool
	if tool == "" {
		tool = "internal/facadegen"
	}
	fmt.Fprintf(&hdr, "// Code generated by %s; DO NOT EDIT.\n\n", tool)
	fmt.Fprintf(&hdr, "// Фасад %s: имена пакета %s (ADR-0012).\n\n", cfg.Package, cfg.ImportPath)
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
		return nil, fmt.Errorf("facadegen: gofmt: %v", err)
	}
	return src, nil
}

// Check сравнивает файл фасада с выводом генератора. Пустая строка — совпадает.
func Check(cfg Config, file string) (string, error) {
	want, err := Generate(cfg)
	if err != nil {
		return "", err
	}
	got, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	got = bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n"))
	if bytes.Equal(got, want) {
		return "", nil
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
			return fmt.Sprintf("%s:%d: в файле %q, генератор даёт %q", file, i+1, a, b), nil
		}
	}
	return file + ": отличается", nil
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

func (g *gen) skip(id *ast.Ident) bool {
	return !id.IsExported() || g.cfg.Skip[id.Name]
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
	if d.Recv != nil || g.skip(d.Name) {
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
	fmt.Fprintf(out, "func %s(%s)%s { %s }\n\n", d.Name.Name, strings.Join(params, ", "), res, call)
}

func (g *gen) genDecl(out *bytes.Buffer, d *ast.GenDecl) {
	switch d.Tok {
	case token.TYPE:
		groupDoc := len(d.Specs) > 1 && d.Doc != nil
		for _, s := range d.Specs {
			ts := s.(*ast.TypeSpec)
			if g.skip(ts.Name) {
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
				if g.skip(id) {
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
