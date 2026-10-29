package engine

// lockorder_test.go — статическая проверка порядка замков движка (ADR-0011).
//
// Правило: frameMu — единственный замок, под которым можно брать другие;
// остальные замки листовые — под ними не берут ни другой замок, ни frameMu
// повторно, и не зовут функции, которые их берут. Проверка читает исходники
// пакета (go/parser, без типов) и для каждой функции ведёт список удержанных
// замков по порядку операторов: Lock/RLock добавляет, Unlock/RUnlock снимает,
// defer-Unlock держит до конца функции, ветка, кончающаяся return, не влияет
// на то, что после неё. На каждом захвате и на каждом вызове функции пакета
// (транзитивно по её собственным захватам) сверяется правило.
//
// Что проверка не видит (сознательно, чтобы не давать ложных тревог):
// тела функций-литералов (они выполняются не там, где написаны: AfterFunc,
// Post, guardCall), вызовы через интерфейсы и чужие пакеты (Draw виджетов,
// хосты окна) и локальные переменные, кроме «x := e.поле» и «x := &e.поле».
// Обратные вызовы виджетов под frameMu и из обработчиков ввода остаются на
// совести вызывающего: под листовым замком их звать нельзя, см. ADR.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"sort"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
)

// rootLock — замок-корень: под ним берутся остальные.
const rootLock = "frameMu"

type lockFunc struct {
	key      string // Type.Method или .Func
	recv     string // имя приёмника метода ("" у функции)
	recvType string
	decl     *ast.FuncDecl
	acquires map[string]bool // прямые захваты
	callees  map[string]bool // ключи вызываемых функций пакета (вне литералов)
}

type lockModel struct {
	fset   *token.FileSet
	funcs  map[string]*lockFunc
	fields map[string][]string // имя поля структуры → типы (у разных структур)
	memo   map[string]map[string]bool
}

func loadLockModel(t *testing.T) *lockModel {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	m := &lockModel{fset: fset, funcs: map[string]*lockFunc{}, fields: map[string][]string{}, memo: map[string]map[string]bool{}}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.GenDecl:
					for _, sp := range d.Specs {
						ts, ok := sp.(*ast.TypeSpec)
						if !ok {
							continue
						}
						st, ok := ts.Type.(*ast.StructType)
						if !ok {
							continue
						}
						for _, fld := range st.Fields.List {
							tn := typeName(fld.Type)
							if tn == "" {
								continue
							}
							for _, n := range fld.Names {
								m.fields[n.Name] = append(m.fields[n.Name], tn)
							}
						}
					}
				case *ast.FuncDecl:
					lf := &lockFunc{decl: d, acquires: map[string]bool{}, callees: map[string]bool{}}
					if d.Recv != nil && len(d.Recv.List) == 1 {
						lf.recvType = typeName(d.Recv.List[0].Type)
						if len(d.Recv.List[0].Names) == 1 {
							lf.recv = d.Recv.List[0].Names[0].Name
						}
					}
					lf.key = lf.recvType + "." + d.Name.Name
					m.funcs[lf.key] = lf
				}
			}
		}
	}
	// Прямые захваты и вызовы каждой функции (без литералов).
	for _, lf := range m.funcs {
		if lf.decl.Body == nil {
			continue
		}
		locals := m.localTypes(lf)
		ast.Inspect(lf.decl.Body, func(n ast.Node) bool {
			if _, ok := n.(*ast.FuncLit); ok {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if lk, op := lockOp(call, lf); lk != "" && (op == "Lock" || op == "RLock") {
				lf.acquires[lk] = true
			}
			for _, k := range m.calleeKeys(call, lf, locals) {
				lf.callees[k] = true
			}
			return true
		})
	}
	return m
}

// typeName — имя типа без указателя. Тип чужого пакета (time.Timer) остаётся с
// префиксом пакета и ни с одной функцией пакета engine не совпадает: иначе
// t.tm.Stop() у поля tm *time.Timer считался бы нашим Timer.Stop.
func typeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return typeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if pkg, ok := t.X.(*ast.Ident); ok {
			return pkg.Name + "." + t.Sel.Name
		}
		return ""
	case *ast.IndexExpr: // atomic.Pointer[Canvas]
		return typeName(t.X)
	}
	return ""
}

// lockOp распознаёт X.Lock()/RLock()/Unlock()/RUnlock() и даёт имя замка:
// путь селекторов без приёмника метода; у метода не на Engine — с именем
// типа приёмника (fm.mu → focusManager.mu).
func lockOp(call *ast.CallExpr, lf *lockFunc) (lock, op string) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || len(call.Args) != 0 {
		return "", ""
	}
	switch sel.Sel.Name {
	case "Lock", "RLock", "Unlock", "RUnlock":
	default:
		return "", ""
	}
	parts := selectorPath(sel.X)
	if len(parts) == 0 {
		return "", ""
	}
	if parts[0] == lf.recv && lf.recv != "" {
		parts = parts[1:]
		if lf.recvType != "Engine" {
			parts = append([]string{lf.recvType}, parts...)
		}
	}
	if len(parts) == 0 {
		return "", ""
	}
	return strings.Join(parts, "."), sel.Sel.Name
}

func selectorPath(e ast.Expr) []string {
	switch x := e.(type) {
	case *ast.Ident:
		return []string{x.Name}
	case *ast.SelectorExpr:
		p := selectorPath(x.X)
		if p == nil {
			return nil
		}
		return append(p, x.Sel.Name)
	case *ast.ParenExpr:
		return selectorPath(x.X)
	case *ast.StarExpr:
		return selectorPath(x.X)
	case *ast.UnaryExpr:
		return selectorPath(x.X)
	}
	return nil
}

// localTypes — локальные переменные вида x := e.поле / x := &e.поле и их типы.
func (m *lockModel) localTypes(lf *lockFunc) map[string][]string {
	out := map[string][]string{}
	if lf.recv == "" {
		return out
	}
	ast.Inspect(lf.decl.Body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		id, ok := as.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		p := selectorPath(as.Rhs[0])
		if len(p) == 2 && p[0] == lf.recv {
			out[id.Name] = m.fields[p[1]]
		}
		return true
	})
	return out
}

// calleeKeys — ключи функций пакета, которые может звать call: e.m(),
// recv.m() у методов, e.field.m(), x.m() у локальной x := e.field, f().
func (m *lockModel) calleeKeys(call *ast.CallExpr, lf *lockFunc, locals map[string][]string) []string {
	var keys []string
	has := func(k string) bool { _, ok := m.funcs[k]; return ok }
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		if has("." + fn.Name) {
			keys = append(keys, "."+fn.Name)
		}
	case *ast.SelectorExpr:
		p := selectorPath(fn.X)
		switch {
		case len(p) == 1 && p[0] == lf.recv:
			if k := lf.recvType + "." + fn.Sel.Name; has(k) {
				keys = append(keys, k)
			}
		case len(p) == 2 && p[0] == lf.recv:
			for _, tn := range m.fields[p[1]] {
				if k := tn + "." + fn.Sel.Name; has(k) {
					keys = append(keys, k)
				}
			}
		case len(p) == 1:
			for _, tn := range locals[p[0]] {
				if k := tn + "." + fn.Sel.Name; has(k) {
					keys = append(keys, k)
				}
			}
		}
	}
	return keys
}

// transitive — все замки, которые функция key берёт сама или через вызовы.
func (m *lockModel) transitive(key string, stack map[string]bool) map[string]bool {
	if r, ok := m.memo[key]; ok {
		return r
	}
	if stack[key] {
		return map[string]bool{}
	}
	stack[key] = true
	out := map[string]bool{}
	lf := m.funcs[key]
	for lk := range lf.acquires {
		out[lk] = true
	}
	for c := range lf.callees {
		for lk := range m.transitive(c, stack) {
			out[lk] = true
		}
	}
	delete(stack, key)
	m.memo[key] = out
	return out
}

// violation — одно нарушение порядка.
type violation struct {
	where, held, lock, via string
}

func (v violation) String() string {
	s := fmt.Sprintf("%s: под {%s} берётся %s", v.where, v.held, v.lock)
	if v.via != "" {
		s += " (через " + v.via + ")"
	}
	return s
}

// checkLock сверяет захват lock при удержанных held с правилом.
func checkLock(held []string, lock string) bool {
	if len(held) == 0 {
		return true
	}
	if len(held) == 1 && held[0] == rootLock {
		return lock != rootLock
	}
	return false
}

// walk идёт по телу функции, ведя список удержанных замков, и собирает
// нарушения и пары «удержан → берётся» (для отчёта).
type lockWalker struct {
	m      *lockModel
	lf     *lockFunc
	locals map[string][]string
	viol   []violation
	edges  map[string]bool
}

func (w *lockWalker) pos(n ast.Node) string {
	p := w.m.fset.Position(n.Pos())
	return fmt.Sprintf("%s:%d %s", p.Filename, p.Line, w.lf.key)
}

func (w *lockWalker) note(held []string, lock, via string, n ast.Node) {
	for _, h := range held {
		if h != lock {
			w.edges[h+" -> "+lock] = true
		}
	}
	if !checkLock(held, lock) {
		w.viol = append(w.viol, violation{where: w.pos(n), held: strings.Join(held, ", "), lock: lock, via: via})
	}
}

// exprLocks обрабатывает выражение: захваты и вызовы в порядке обхода.
func (w *lockWalker) expr(e ast.Expr, held []string) []string {
	if e == nil {
		return held
	}
	ast.Inspect(e, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if lk, op := lockOp(call, w.lf); lk != "" {
			switch op {
			case "Lock", "RLock":
				w.note(held, lk, "", call)
				held = append(held, lk)
			default:
				held = removeLock(held, lk)
			}
			return false
		}
		for _, k := range w.m.calleeKeys(call, w.lf, w.locals) {
			if len(held) == 0 {
				continue
			}
			tr := w.m.transitive(k, map[string]bool{})
			names := make([]string, 0, len(tr))
			for lk := range tr {
				names = append(names, lk)
			}
			sort.Strings(names)
			for _, lk := range names {
				w.note(held, lk, k, call)
			}
		}
		return true
	})
	return held
}

func removeLock(held []string, lk string) []string {
	for i := len(held) - 1; i >= 0; i-- {
		if held[i] == lk {
			return append(append([]string{}, held[:i]...), held[i+1:]...)
		}
	}
	return held
}

func terminates(s ast.Stmt) bool {
	switch x := s.(type) {
	case *ast.ReturnStmt, *ast.BranchStmt:
		return true
	case *ast.ExprStmt:
		if c, ok := x.X.(*ast.CallExpr); ok {
			if id, ok := c.Fun.(*ast.Ident); ok && id.Name == "panic" {
				return true
			}
		}
	case *ast.BlockStmt:
		return len(x.List) > 0 && terminates(x.List[len(x.List)-1])
	}
	return false
}

// block обрабатывает блок и отдаёт удержанные замки после него; terminated —
// блок кончается выходом (его состояние не идёт дальше).
func (w *lockWalker) block(list []ast.Stmt, held []string) (after []string, terminated bool) {
	held = append([]string{}, held...)
	for _, s := range list {
		held = w.stmt(s, held)
		if terminates(s) {
			return held, true
		}
	}
	return held, false
}

// branch выбирает состояние после развилки: первая ветка без выхода, иначе
// как до развилки.
func branch(before []string, results [][]string, terms []bool) []string {
	for i, r := range results {
		if !terms[i] {
			return r
		}
	}
	return before
}

func (w *lockWalker) stmt(s ast.Stmt, held []string) []string {
	switch x := s.(type) {
	case nil:
		return held
	case *ast.ExprStmt:
		return w.expr(x.X, held)
	case *ast.AssignStmt:
		for _, r := range x.Rhs {
			held = w.expr(r, held)
		}
		for _, l := range x.Lhs {
			held = w.expr(l, held)
		}
		return held
	case *ast.DeferStmt:
		// defer X.Unlock() держит замок до конца; остальные defer — литералы
		// и вызовы, которые идут при выходе: их не трогаем.
		if lk, op := lockOp(x.Call, w.lf); lk != "" && (op == "Lock" || op == "RLock") {
			w.note(held, lk, "", x.Call)
			return append(held, lk)
		}
		return held
	case *ast.GoStmt:
		return held
	case *ast.ReturnStmt:
		for _, r := range x.Results {
			held = w.expr(r, held)
		}
		return held
	case *ast.IncDecStmt:
		return w.expr(x.X, held)
	case *ast.SendStmt:
		held = w.expr(x.Chan, held)
		return w.expr(x.Value, held)
	case *ast.DeclStmt:
		if gd, ok := x.Decl.(*ast.GenDecl); ok {
			for _, sp := range gd.Specs {
				if vs, ok := sp.(*ast.ValueSpec); ok {
					for _, v := range vs.Values {
						held = w.expr(v, held)
					}
				}
			}
		}
		return held
	case *ast.BlockStmt:
		after, _ := w.block(x.List, held)
		return after
	case *ast.IfStmt:
		held = w.stmt(x.Init, held)
		held = w.expr(x.Cond, held)
		r1, t1 := w.block(x.Body.List, held)
		results, terms := [][]string{r1}, []bool{t1}
		if x.Else != nil {
			var r2 []string
			var t2 bool
			if eb, ok := x.Else.(*ast.BlockStmt); ok {
				r2, t2 = w.block(eb.List, held)
			} else {
				r2 = w.stmt(x.Else, held)
				t2 = terminates(x.Else)
			}
			results, terms = append(results, r2), append(terms, t2)
		}
		return branch(held, results, terms)
	case *ast.ForStmt:
		held = w.stmt(x.Init, held)
		held = w.expr(x.Cond, held)
		r, t := w.block(x.Body.List, held)
		return branch(held, [][]string{r}, []bool{t})
	case *ast.RangeStmt:
		held = w.expr(x.X, held)
		r, t := w.block(x.Body.List, held)
		return branch(held, [][]string{r}, []bool{t})
	case *ast.SwitchStmt:
		held = w.stmt(x.Init, held)
		held = w.expr(x.Tag, held)
		return w.cases(x.Body, held)
	case *ast.TypeSwitchStmt:
		held = w.stmt(x.Init, held)
		held = w.stmt(x.Assign, held)
		return w.cases(x.Body, held)
	case *ast.SelectStmt:
		return w.cases(x.Body, held)
	case *ast.LabeledStmt:
		return w.stmt(x.Stmt, held)
	}
	return held
}

func (w *lockWalker) cases(body *ast.BlockStmt, held []string) []string {
	var results [][]string
	var terms []bool
	for _, c := range body.List {
		var list []ast.Stmt
		switch cc := c.(type) {
		case *ast.CaseClause:
			for _, e := range cc.List {
				held = w.expr(e, held)
			}
			list = cc.Body
		case *ast.CommClause:
			if cc.Comm != nil {
				held = w.stmt(cc.Comm, held)
			}
			list = cc.Body
		}
		r, t := w.block(list, held)
		results, terms = append(results, r), append(terms, t)
	}
	return branch(held, results, terms)
}

// TestLockOrder_FrameMuIsTheOnlyNestingLock — правило ADR-0011 по исходникам.
func TestLockOrder_FrameMuIsTheOnlyNestingLock(t *testing.T) {
	testreset.Globals(t)
	m := loadLockModel(t)
	if len(m.funcs) < 100 {
		t.Fatalf("разобрано %d функций — пакет не прочитан", len(m.funcs))
	}
	var all []violation
	edges := map[string]bool{}
	keys := make([]string, 0, len(m.funcs))
	for k := range m.funcs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		lf := m.funcs[k]
		if lf.decl.Body == nil {
			continue
		}
		w := &lockWalker{m: m, lf: lf, locals: m.localTypes(lf), edges: edges}
		w.block(lf.decl.Body.List, nil)
		all = append(all, w.viol...)
	}
	names := make([]string, 0, len(edges))
	for e := range edges {
		names = append(names, e)
	}
	sort.Strings(names)
	t.Logf("вложения (удержан -> берётся), %d:\n  %s", len(names), strings.Join(names, "\n  "))
	for _, v := range all {
		t.Error(v)
	}
}
