// Package depmap — карта зависимостей внутри одного пакета по группам файлов
// (ADR-0012, этап 8: `widget` → подпакеты).
//
// Пакет проверяется типами (go/types, импортёр из исходников), поэтому
// обращение к методу, полю, типу или функции другого файла находится точно,
// а не по совпадению имён. Файлы распределяются по группам — будущим
// подпакетам — по регулярным выражениям на имя файла; не попавшие — группа
// Config.Default. Между группами считаются рёбра «откуда → куда» с перечнем
// идентификаторов.
//
// Правило: ребро допустимо, если его разрешает Config.Allowed; прочие рёбра
// — нарушения. Чтобы этап шёл шагами, нарушения, перечисленные в
// Config.Baseline, терпятся: проверка падает на новом нарушении и сообщает
// о базовых, которые исчезли (их пора убрать из базы). Так карта в ADR и
// код не расходятся, а связность не растёт, пока подпакеты ещё не вырезаны.
//
// Файлы берутся по тегам сборки текущей ОС (как у компилятора); тесты не
// читаются.
package depmap

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Group — группа файлов (будущий подпакет).
type Group struct {
	Name  string
	Match *regexp.Regexp
}

// Config — что и по какому правилу проверять.
type Config struct {
	// Dir — каталог пакета (обычно ".").
	Dir string
	// Path — путь пакета для проверки типов (любая строка, одна на пакет).
	Path string
	// Groups — группы по порядку: файл попадает в первую подошедшую.
	Groups []Group
	// Default — группа файлов, не подошедших ни одной (ядро).
	Default string
	// Allowed — разрешено ли ребро from → to между группами.
	Allowed func(from, to string) bool
	// Baseline — терпимые нарушения «from → to» (на время переноса).
	Baseline []string
	// Top — сколько идентификаторов печатать на ребро в отчёте.
	Top int
}

// Edge — ребро между группами с идентификаторами и числом обращений.
type Edge struct {
	From, To string
	Refs     int
	Idents   map[string]int // «вид Имя (файл) ← файл» → обращений
}

// Report — результат построения карты.
type Report struct {
	Files  map[string][]string // группа → файлы
	Lines  map[string]int      // группа → строк
	Edges  map[string]*Edge    // «from → to» → ребро
	InDeg  map[string]int      // файл → входящих обращений из других файлов
	Errors []error             // ошибки проверки типов (частичные)
}

// Build строит карту.
func Build(cfg Config) (*Report, error) {
	if cfg.Dir == "" {
		cfg.Dir = "."
	}
	if cfg.Path == "" {
		cfg.Path = "pkg"
	}
	if cfg.Default == "" {
		cfg.Default = "core"
	}
	groupOf := func(file string) string {
		for _, g := range cfg.Groups {
			if g.Match.MatchString(file) {
				return g.Name
			}
		}
		return cfg.Default
	}

	fset := token.NewFileSet()
	entries, err := os.ReadDir(cfg.Dir)
	if err != nil {
		return nil, err
	}
	rep := &Report{Files: map[string][]string{}, Lines: map[string]int{}, Edges: map[string]*Edge{}, InDeg: map[string]int{}}
	var files []*ast.File
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		if ok, _ := build.Default.MatchFile(cfg.Dir, n); !ok {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(cfg.Dir, n), nil, 0)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
		g := groupOf(n)
		rep.Files[g] = append(rep.Files[g], n)
		rep.Lines[g] += fset.File(f.Pos()).LineCount()
	}
	for g := range rep.Files {
		sort.Strings(rep.Files[g])
	}
	conf := types.Config{
		Importer: importer.ForCompiler(fset, "source", nil),
		Error:    func(err error) { rep.Errors = append(rep.Errors, err) },
	}
	info := &types.Info{
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	pkg, _ := conf.Check(cfg.Path, fset, files, info)
	if pkg == nil {
		return nil, fmt.Errorf("depmap: пакет %s не проверен типами", cfg.Dir)
	}
	fileOf := func(pos token.Pos) string {
		if !pos.IsValid() {
			return ""
		}
		return filepath.Base(fset.Position(pos).Filename)
	}
	add := func(from, to, what string) {
		if from == "" || to == "" || from == to {
			return
		}
		rep.InDeg[to]++
		gf, gt := groupOf(from), groupOf(to)
		if gf == gt {
			return
		}
		key := gf + " → " + gt
		e := rep.Edges[key]
		if e == nil {
			e = &Edge{From: gf, To: gt, Idents: map[string]int{}}
			rep.Edges[key] = e
		}
		e.Refs++
		e.Idents[what+" ("+to+") ← "+from]++
	}
	for id, obj := range info.Uses {
		if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != cfg.Path || obj.Parent() != pkg.Scope() {
			continue
		}
		kind := "func"
		switch obj.(type) {
		case *types.TypeName:
			kind = "type"
		case *types.Var:
			kind = "var"
		case *types.Const:
			kind = "const"
		}
		add(fileOf(id.Pos()), fileOf(obj.Pos()), kind+" "+obj.Name())
	}
	for sel, s := range info.Selections {
		obj := s.Obj()
		if obj == nil || obj.Pkg() == nil || obj.Pkg().Path() != cfg.Path {
			continue
		}
		recv := ""
		if t := s.Recv(); t != nil {
			if p, ok := t.(*types.Pointer); ok {
				t = p.Elem()
			}
			if n, ok := t.(*types.Named); ok {
				recv = n.Obj().Name() + "."
			}
		}
		kind := "method"
		if s.Kind() == types.FieldVal {
			kind = "field"
		}
		add(fileOf(sel.Pos()), fileOf(obj.Pos()), kind+" "+recv+obj.Name())
	}
	return rep, nil
}

// Run строит карту, пишет отчёт в лог теста и сверяет рёбра с правилом и
// базой: новое нарушение — ошибка, исчезнувшее базовое — сообщение.
func Run(t testing.TB, cfg Config) *Report {
	t.Helper()
	rep, err := Build(cfg)
	if err != nil {
		t.Fatal(err)
	}
	top := cfg.Top
	if top <= 0 {
		top = 8
	}
	var sb strings.Builder
	groups := make([]string, 0, len(rep.Files))
	for g := range rep.Files {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	fmt.Fprintln(&sb, "группы:")
	for _, g := range groups {
		fmt.Fprintf(&sb, "  %-10s %3d файлов %6d строк\n", g, len(rep.Files[g]), rep.Lines[g])
	}
	keys := make([]string, 0, len(rep.Edges))
	for k := range rep.Edges {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintln(&sb, "рёбра (откуда → куда: обращений / идентификаторов):")
	base := map[string]bool{}
	for _, b := range cfg.Baseline {
		base[b] = true
	}
	var bad []string
	seen := map[string]bool{}
	for _, k := range keys {
		e := rep.Edges[k]
		ok := cfg.Allowed == nil || cfg.Allowed(e.From, e.To)
		mark := ""
		if !ok {
			seen[k] = true
			if base[k] {
				mark = "  [база]"
			} else {
				mark = "  [НАРУШЕНИЕ]"
				bad = append(bad, k)
			}
		}
		fmt.Fprintf(&sb, "  %-22s %5d / %-4d%s\n", k+":", e.Refs, len(e.Idents), mark)
		if !ok {
			type kv struct {
				k string
				c int
			}
			var list []kv
			for id, c := range e.Idents {
				list = append(list, kv{id, c})
			}
			sort.Slice(list, func(i, j int) bool {
				if list[i].c != list[j].c {
					return list[i].c > list[j].c
				}
				return list[i].k < list[j].k
			})
			for i, it := range list {
				if i >= top {
					fmt.Fprintf(&sb, "        … ещё %d\n", len(list)-top)
					break
				}
				fmt.Fprintf(&sb, "        %4d  %s\n", it.c, it.k)
			}
		}
	}
	t.Log(sb.String())
	if len(rep.Errors) > 0 {
		t.Logf("ошибок проверки типов: %d (первая: %v)", len(rep.Errors), rep.Errors[0])
	}
	for _, b := range cfg.Baseline {
		if !seen[b] {
			t.Logf("базовое нарушение %q исчезло — уберите его из Baseline", b)
		}
	}
	for _, k := range bad {
		t.Errorf("новая зависимость между группами: %s (вне порядка подпакетов и вне базы)", k)
	}
	return rep
}
