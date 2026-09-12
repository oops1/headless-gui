package tests

import (
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Контрол трёхстороннего слияния MergeView — GG-57. Три стороны только
// показываются, правится итог; конфликт закрывается решением, а нерешённый
// уходит в итог маркерами git.

const (
	mvBase   = "package ui\n\nfunc New() *Btn {\n\treturn &Btn{}\n}\n"
	mvOurs   = "package ui\n\nfunc New() *Btn {\n\treturn &Btn{Pad: 8}\n}\n"
	mvTheirs = "package ui\n\nfunc New() *Btn {\n\treturn &Btn{Round: 6}\n}\n"
)

func mergeScene(t *testing.T, mv *widget.MergeView) *engine.Engine {
	t.Helper()
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 1000, 600))
	mv.SetBounds(image.Rect(0, 0, 1000, 600))
	root.AddChild(mv)
	eng := engine.New(1000, 600, 30)
	eng.SetRoot(root)
	eng.SetFocus(mv)
	return eng
}

// Конфликт, решения и итог: пока решения нет — маркеры git, после решения —
// строки выбранной стороны.
func TestMergeView_ResolveConflict(t *testing.T) {
	mv := widget.NewMergeView("", "")
	var unresolved []int
	mv.OnResolvedChanged = func(n int) { unresolved = append(unresolved, n) }
	mv.SetTexts(mvBase, mvOurs, mvTheirs)

	if got := mv.ConflictCount(); got != 1 {
		t.Fatalf("конфликтов %d, хочу 1", got)
	}
	if got := mv.Unresolved(); got != 1 {
		t.Fatalf("нерешённых %d, хочу 1", got)
	}
	if !strings.Contains(mv.Result(), "<<<<<<<") {
		t.Fatalf("нерешённый конфликт без маркеров:\n%s", mv.Result())
	}

	ci := -1
	for i, c := range mv.Chunks() {
		if c.Conflict {
			ci = i
		}
	}
	mv.Resolve(ci, widget.MergeTakeTheirs)
	if got := mv.Unresolved(); got != 0 {
		t.Fatalf("после решения нерешённых %d", got)
	}
	res := mv.Result()
	if strings.Contains(res, "<<<<<<<") || !strings.Contains(res, "Round: 6") || strings.Contains(res, "Pad: 8") {
		t.Fatalf("итог после «взять их»:\n%s", res)
	}
	if len(unresolved) == 0 || unresolved[len(unresolved)-1] != 0 {
		t.Fatalf("OnResolvedChanged: %v", unresolved)
	}

	// «Оба» — обе стороны подряд, наша первой.
	mv.Resolve(ci, widget.MergeTakeOursThenTheirs)
	res = mv.Result()
	if !strings.Contains(res, "Pad: 8") || !strings.Contains(res, "Round: 6") ||
		strings.Index(res, "Pad: 8") > strings.Index(res, "Round: 6") {
		t.Fatalf("итог после «взять оба»:\n%s", res)
	}

	// Возврат в нерешённое состояние возвращает и маркеры.
	mv.Resolve(ci, widget.MergeUnresolved)
	if !strings.Contains(mv.Result(), ">>>>>>>") || mv.Unresolved() != 1 {
		t.Fatalf("возврат в нерешённое:\n%s", mv.Result())
	}
}

// Стиль diff3 добавляет в маркеры базу, подписи сторон берутся из SetSides.
func TestMergeView_StyleAndLabels(t *testing.T) {
	mv := widget.NewMergeView("", "")
	mv.SetTexts(mvBase, mvOurs, mvTheirs)
	mv.SetSides(
		widget.MergeSideInfo{Title: "main", Note: "btn.go"},
		widget.MergeSideInfo{Title: "merge-base", Note: "btn.go"},
		widget.MergeSideInfo{Title: "feature", Note: "btn.go"},
	)
	mv.SetStyle(widget.MergeStyleDiff3)

	res := mv.Result()
	for _, want := range []string{"<<<<<<< main", "||||||| merge-base", ">>>>>>> feature"} {
		if !strings.Contains(res, want) {
			t.Fatalf("нет строки %q:\n%s", want, res)
		}
	}
	mv.SetStyle(widget.MergeStyleMerge)
	if strings.Contains(mv.Result(), "|||||||") {
		t.Fatalf("стиль merge оставил базу:\n%s", mv.Result())
	}
}

// Решение не трогает правки руками в соседних блоках: строки блока заменяются
// на месте, а не пересобирается весь итог.
func TestMergeView_ResolveKeepsManualEdits(t *testing.T) {
	mv := widget.NewMergeView("", "")
	mv.SetTexts(mvBase, mvOurs, mvTheirs)

	// Правим первую строку итога (она вне конфликта).
	mv.SetCaret(0, len("package ui"))
	mv.InsertText(" // правка руками")
	if !strings.Contains(mv.Result(), "// правка руками") {
		t.Fatal("правка не попала в итог")
	}

	ci := -1
	for i, c := range mv.Chunks() {
		if c.Conflict {
			ci = i
		}
	}
	mv.Resolve(ci, widget.MergeTakeOurs)
	res := mv.Result()
	if !strings.Contains(res, "// правка руками") {
		t.Fatalf("решение стёрло правку руками:\n%s", res)
	}
	if strings.Contains(res, "<<<<<<<") || !strings.Contains(res, "Pad: 8") {
		t.Fatalf("решение не применилось:\n%s", res)
	}
}

// Правка итога с клавиатуры через движок: набор, Tab внутрь текста, отмена и
// повтор; стороны при этом только для чтения.
func TestMergeView_KeyboardEditing(t *testing.T) {
	mv := widget.NewMergeView("", "")
	mv.SetTexts(mvBase, mvOurs, mvTheirs)
	eng := mergeScene(t, mv)
	_ = eng

	edited := 0
	mv.OnResultEdited = func() { edited++ }
	mv.SetCaret(0, 0)
	dvKeys(eng, dvRunes("X")...)
	if got := mv.ResultLines()[0]; !strings.HasPrefix(got, "X") {
		t.Fatalf("набор не дошёл до итога: %q", got)
	}
	if edited == 0 {
		t.Fatal("OnResultEdited не вызван")
	}

	// Tab вставляется в текст: контрол объявляет AcceptsTab.
	dvKeys(eng, dvKey(widget.KeyTab, 0))
	if got := mv.ResultLines()[0]; !strings.HasPrefix(got, "X\t") {
		t.Fatalf("Tab не вставился: %q", got)
	}

	mv.Undo()
	mv.Undo()
	if got := mv.ResultLines()[0]; got != "package ui" {
		t.Fatalf("после отмены: %q", got)
	}
	mv.Redo()
	if got := mv.ResultLines()[0]; !strings.HasPrefix(got, "X") {
		t.Fatalf("после повтора: %q", got)
	}

	// Стороны не правятся: ввод в панель «наше» текст не меняет.
	before := strings.Join(mv.Chunks()[0].Ours, "\n")
	mv.SetActiveSide(widget.MergeOurs)
	dvKeys(eng, dvRunes("Z")...)
	if after := strings.Join(mv.Chunks()[0].Ours, "\n"); after != before {
		t.Fatalf("сторона «наше» изменилась: %q → %q", before, after)
	}
}

// Отмена возвращает и решение по конфликту, а не только правку текста.
func TestMergeView_UndoResolve(t *testing.T) {
	mv := widget.NewMergeView("", "")
	mv.SetTexts(mvBase, mvOurs, mvTheirs)
	ci := -1
	for i, c := range mv.Chunks() {
		if c.Conflict {
			ci = i
		}
	}
	mv.Resolve(ci, widget.MergeTakeOurs)
	if mv.Unresolved() != 0 {
		t.Fatal("подготовка: конфликт не решён")
	}
	mv.Undo()
	if mv.Unresolved() != 1 {
		t.Fatalf("отмена не вернула конфликт: нерешённых %d", mv.Unresolved())
	}
	if !strings.Contains(mv.Result(), "<<<<<<<") {
		t.Fatalf("отмена не вернула маркеры:\n%s", mv.Result())
	}
	if mv.Resolution(ci) != widget.MergeUnresolved {
		t.Fatalf("решение осталось: %v", mv.Resolution(ci))
	}
}

// Навигация: F7 переходит к конфликту, ResolveAll закрывает всё разом.
func TestMergeView_NavigationAndResolveAll(t *testing.T) {
	base := "a\nb\nc\nd\ne\nf\ng\n"
	ours := "a\nНАШЕ1\nc\nd\ne\nНАШЕ2\ng\n"
	theirs := "a\nИХ1\nc\nd\ne\nИХ2\ng\n"

	mv := widget.NewMergeView("", "")
	var current []int
	mv.OnCurrentConflict = func(i int) { current = append(current, i) }
	mv.SetTexts(base, ours, theirs)
	if got := mv.ConflictCount(); got != 2 {
		t.Fatalf("конфликтов %d, хочу 2", got)
	}

	mv.GoToConflict(0)
	if got := mv.CurrentConflict(); got != 0 {
		t.Fatalf("текущий конфликт %d", got)
	}
	mv.NextConflict()
	if got := mv.CurrentConflict(); got != 1 {
		t.Fatalf("после NextConflict текущий %d", got)
	}
	mv.PrevConflict()
	if got := mv.CurrentConflict(); got != 0 {
		t.Fatalf("после PrevConflict текущий %d", got)
	}
	if len(current) == 0 {
		t.Fatal("OnCurrentConflict не вызывался")
	}

	mv.ResolveAll(widget.MergeTakeTheirs)
	if got := mv.Unresolved(); got != 0 {
		t.Fatalf("после ResolveAll нерешённых %d", got)
	}
	res := mv.Result()
	if !strings.Contains(res, "ИХ1") || !strings.Contains(res, "ИХ2") || strings.Contains(res, "НАШЕ") {
		t.Fatalf("итог после ResolveAll:\n%s", res)
	}
}

// Панель базы прячется, и её отсутствие не ломает раскладку и доступность.
func TestMergeView_AccessibilityChildren(t *testing.T) {
	mv := widget.NewMergeView("", "")
	mv.SetTexts(mvBase, mvOurs, mvTheirs)
	mv.SetSides(
		widget.MergeSideInfo{Title: "main"},
		widget.MergeSideInfo{Title: "base"},
		widget.MergeSideInfo{Title: "feature"},
	)
	mv.SetBounds(image.Rect(0, 0, 1000, 600))

	panes := mergePanes(widget.BuildAccessTree(mv, mv))
	if len(panes) != 4 {
		t.Fatalf("текстовых панелей %d, хочу 4 (три стороны и итог)", len(panes))
	}
	for _, n := range panes[:3] {
		if !hasState(n.States, widget.StateReadOnly) {
			t.Fatalf("сторона %q должна быть только для чтения: %v", n.Name, n.States)
		}
	}
	if hasState(panes[3].States, widget.StateReadOnly) {
		t.Fatal("итог помечен только для чтения")
	}
	if !strings.Contains(panes[0].Name, "main") || panes[3].Value == "" {
		t.Fatalf("подписи и содержимое панелей: %+v", panes)
	}
	// Панели стоят слева направо и не налезают друг на друга.
	if panes[0].Bounds.Max.X > panes[1].Bounds.Min.X || panes[1].Bounds.Max.X > panes[2].Bounds.Min.X {
		t.Fatalf("границы панелей: %v %v %v", panes[0].Bounds, panes[1].Bounds, panes[2].Bounds)
	}

	mv.SetShowBase(false)
	if got := len(mergePanes(widget.BuildAccessTree(mv, mv))); got != 3 {
		t.Fatalf("без базы панелей %d, хочу 3", got)
	}
}

// mergePanes — текстовые панели контрола из дерева доступности (меню контрола
// живёт там же отдельным узлом).
func mergePanes(tree *widget.AccessNode) []widget.AccessInfo {
	var out []widget.AccessInfo
	for _, c := range tree.Children {
		if c.Role == widget.RoleTextInput {
			out = append(out, c.AccessInfo)
		}
	}
	return out
}

func hasState(states []string, want string) bool {
	for _, s := range states {
		if s == want {
			return true
		}
	}
	return false
}

// Разметка из fs.FS: тег, три стороны вкомпилированными файлами, ShowBase и
// стиль маркеров.
func TestMergeView_XAMLFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"base.txt":   {Data: []byte(mvBase)},
		"ours.txt":   {Data: []byte(mvOurs)},
		"theirs.txt": {Data: []byte(mvTheirs)},
	}
	_, reg, err := widget.LoadUIFromXAMLFS([]byte(`<Canvas Width="900" Height="600">
  <MergeView x:Name="mv" BaseFile="base.txt" OursFile="ours.txt" TheirsFile="theirs.txt"
             ShowBase="False" ConflictStyle="diff3"/>
</Canvas>`), fsys)
	if err != nil {
		t.Fatalf("разметка: %v", err)
	}
	mv, ok := reg["mv"].(*widget.MergeView)
	if !ok {
		t.Fatalf("тег MergeView не собрался: %T", reg["mv"])
	}
	if mv.ShowBase() {
		t.Fatal("ShowBase=False не применился")
	}
	if mv.ConflictCount() != 1 {
		t.Fatalf("файлы из fs.FS не слились: конфликтов %d", mv.ConflictCount())
	}
	if !strings.Contains(mv.Result(), "|||||||") {
		t.Fatalf("ConflictStyle=diff3 не применился:\n%s", mv.Result())
	}
}

// Команда разметки: ResolvedCommand получает число оставшихся конфликтов.
func TestMergeView_XAMLCommand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "base.txt"), mvBase)
	writeFile(t, filepath.Join(dir, "ours.txt"), mvOurs)
	writeFile(t, filepath.Join(dir, "theirs.txt"), mvTheirs)
	xp := filepath.Join(dir, "ui.xaml")
	writeFile(t, xp, `<Canvas Width="900" Height="600">
  <MergeView x:Name="mv" BaseFile="base.txt" OursFile="ours.txt" TheirsFile="theirs.txt"
             ResolvedCommand="{Binding Left}"/>
</Canvas>`)

	left := -1
	vm := &mergeVM{cmd: &widget.RelayCommand{ExecuteFn: func(p any) {
		if n, ok := p.(int); ok {
			left = n
		}
	}}}
	_, reg, _, err := widget.LoadUIFromXAMLFileBindings(xp, vm)
	if err != nil {
		t.Fatalf("разметка: %v", err)
	}
	mv := reg["mv"].(*widget.MergeView)
	ci := -1
	for i, c := range mv.Chunks() {
		if c.Conflict {
			ci = i
		}
	}
	mv.Resolve(ci, widget.MergeTakeOurs)
	if left != 0 {
		t.Fatalf("ResolvedCommand: параметр %d, хочу 0", left)
	}

	// Путь за пределы каталога разметки не читается (SEC-8): стороны пусты.
	writeFile(t, filepath.Join(dir, "bad.xaml"), `<Canvas Width="400" Height="300">
  <MergeView x:Name="mv" BaseFile="../secret.txt" OursFile="ours.txt" TheirsFile="theirs.txt"/>
</Canvas>`)
	_, reg2, err := widget.LoadUIFromXAMLFile(filepath.Join(dir, "bad.xaml"))
	if err != nil {
		t.Fatalf("разметка с выходом за каталог: %v", err)
	}
	if got := reg2["mv"].(*widget.MergeView).ConflictCount(); got != 0 {
		t.Fatalf("путь за каталог разметки прочитан: конфликтов %d", got)
	}
}

type mergeVM struct{ cmd *widget.RelayCommand }

func (v *mergeVM) Left() widget.ICommand { return v.cmd }
