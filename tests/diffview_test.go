package tests

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Контрол сравнения DiffView — пункт 0 замечаний difftool: контрол жил в
// приложении и дублировал механизмы движка (свою очередь в поток UI, свои
// шрифты, свои цвета). Здесь — его поведение через публичное API и через
// движок, как им пользуется приложение.

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Файлы, события и сохранение: перевод строки, BOM и отсутствие перевода в
// конце переживают правку — иначе сохранение из сравнения портило бы файл
// целиком, а не одну строку.
func TestDiffView_FilesAndEvents(t *testing.T) {
	dir := t.TempDir()
	lp, rp := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	writeFile(t, lp, "one\r\ntwo\r\nthree\r\n")
	writeFile(t, rp, "\xEF\xBB\xBFone\ntwo!\nthree")

	dv := widget.NewDiffView("", "")
	var loaded, textCh, modCh, saved []widget.DiffSide
	lastChanges := -1
	dv.OnFileLoaded = func(s widget.DiffSide, _ string) { loaded = append(loaded, s) }
	dv.OnTextChanged = func(s widget.DiffSide) { textCh = append(textCh, s) }
	dv.OnModifiedChanged = func(s widget.DiffSide, _ bool) { modCh = append(modCh, s) }
	dv.OnFileSaved = func(s widget.DiffSide, _ string) { saved = append(saved, s) }
	dv.OnDiffChanged = func(n int) { lastChanges = n }

	if err := dv.LoadFile(widget.DiffLeft, lp); err != nil {
		t.Fatal(err)
	}
	if err := dv.LoadFile(widget.DiffRight, rp); err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || dv.ChangeCount() != 1 || lastChanges != 1 {
		t.Fatalf("загрузка: loaded=%v changes=%d/%d", loaded, dv.ChangeCount(), lastChanges)
	}
	ch := dv.Changes()
	if ch[0].Kind != widget.DiffReplace || ch[0].LeftFrom != 1 || ch[0].LeftTo != 2 {
		t.Fatalf("отличие: %+v", ch[0])
	}

	// Правка левой стороны делает файлы равными.
	dv.SetCaret(widget.DiffLeft, 1, 3)
	dv.InsertText("!")
	if !dv.IsModified(widget.DiffLeft) || dv.ChangeCount() != 0 || lastChanges != 0 {
		t.Fatalf("после правки: modified=%v changes=%d", dv.IsModified(widget.DiffLeft), dv.ChangeCount())
	}
	if len(textCh) == 0 || textCh[len(textCh)-1] != widget.DiffLeft || len(modCh) != 1 {
		t.Fatalf("события правки: text=%v mod=%v", textCh, modCh)
	}

	if err := dv.Save(widget.DiffLeft); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(lp); string(got) != "one\r\ntwo!\r\nthree\r\n" {
		t.Fatalf("сохранение потеряло CRLF: %q", got)
	}
	if dv.IsModified(widget.DiffLeft) || len(saved) != 1 {
		t.Fatal("после сохранения сторона всё ещё изменена")
	}

	dv.SetCaret(widget.DiffRight, 2, 5)
	dv.InsertText(".")
	if err := dv.Save(widget.DiffRight); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(rp); string(got) != "\xEF\xBB\xBFone\ntwo!\nthree." {
		t.Fatalf("правый файл: %q", got)
	}

	// Отмена до сохранённого состояния снимает «изменён»: модификация
	// считается по ревизиям, а не флагом.
	dv.InsertText("x")
	if !dv.IsModified(widget.DiffRight) {
		t.Fatal("правка не отмечена")
	}
	dv.Undo()
	if dv.IsModified(widget.DiffRight) {
		t.Fatal("отмена до сохранённого состояния должна снять модификацию")
	}

	if err := widget.NewDiffView("", "").Save(widget.DiffLeft); err != widget.ErrDiffNoPath {
		t.Fatalf("без файла ждали ErrDiffNoPath, получили %v", err)
	}
}

// Двоичный файл не загружается, а ошибка приходит в OnError на языке
// интерфейса.
func TestDiffView_BinaryFileRejected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bin.dat")
	writeFile(t, p, "MZ\x00\x01\x02")
	dv := widget.NewDiffView("", "")
	var got error
	dv.OnError = func(_ widget.DiffSide, err error) { got = err }
	if err := dv.LoadFile(widget.DiffLeft, p); err == nil {
		t.Fatal("двоичный файл загружен")
	}
	if got == nil || !strings.Contains(got.Error(), "bin.dat") {
		t.Fatalf("OnError: %v", got)
	}
	if dv.FilePath(widget.DiffLeft) != "" {
		t.Fatal("сторона запомнила двоичный файл")
	}
}

func diffScene(t *testing.T, dv *widget.DiffView) *engine.Engine {
	t.Helper()
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 900, 500))
	dv.SetBounds(image.Rect(0, 0, 900, 500))
	root.AddChild(dv)
	eng := engine.New(900, 500, 30)
	eng.SetRoot(root)
	return eng
}

// Слежение за файлом: своё сохранение не считается внешней правкой, а
// внешняя приходит в поток движка через его Post — наблюдатель живёт в
// фоновой горутине, и без этого обработчик трогал бы дерево виджетов в обход
// горутины кадра. Движок здесь не запущен, поэтому очередь разбирает
// RenderFrameNow: событие обязано ждать его, а не прийти само.
func TestDiffView_WatchPostsToEngine(t *testing.T) {
	if testing.Short() {
		t.Skip("опрос файлов раз в секунду")
	}
	p := filepath.Join(t.TempDir(), "w.txt")
	writeFile(t, p, "a\n")
	dv := widget.NewDiffView("", "")
	eng := diffScene(t, dv)
	if err := dv.LoadFile(widget.DiffLeft, p); err != nil {
		t.Fatal(err)
	}
	var hits []bool
	dv.OnFileChangedOnDisk = func(_ widget.DiffSide, _ string, deleted bool) { hits = append(hits, deleted) }
	var cmdParam any
	dv.SetCommand("FileChangedCommand", &widget.RelayCommand{ExecuteFn: func(p any) { cmdParam = p }})
	dv.SetWatchFiles(true)
	defer dv.Close()

	dv.InsertText("b")
	if err := dv.Save(widget.DiffLeft); err != nil {
		t.Fatal(err)
	}
	for end := time.Now().Add(1500 * time.Millisecond); time.Now().Before(end); {
		eng.RenderFrameNow()
		time.Sleep(50 * time.Millisecond)
	}
	if len(hits) != 0 {
		t.Fatal("своё сохранение принято за внешнюю правку")
	}

	writeFile(t, p, "changed outside\n")
	// Кадров не просим: наблюдатель заметит правку, но выполнить обработчик
	// сам не вправе.
	time.Sleep(2500 * time.Millisecond)
	if len(hits) != 0 {
		t.Fatal("обработчик вызван из фоновой горутины, минуя Post движка")
	}
	eng.RenderFrameNow()
	if len(hits) != 1 || hits[0] {
		t.Fatalf("внешняя правка: %v", hits)
	}
	if fc, ok := cmdParam.(widget.DiffFileChange); !ok || fc.Path != p || fc.Deleted {
		t.Fatalf("FileChangedCommand: %#v", cmdParam)
	}
	if err := dv.Reload(widget.DiffLeft); err != nil || dv.Lines(widget.DiffLeft)[0] != "changed outside" {
		t.Fatalf("перечитывание: %v %q", err, dv.Lines(widget.DiffLeft))
	}
	// Перечитывание отменяемо: правка до него возвращается.
	dv.Undo()
	if got := dv.Lines(widget.DiffLeft)[0]; got != "ba" {
		t.Fatalf("отмена перечитывания: %q", got)
	}
}

func dvKeys(eng *engine.Engine, keys ...widget.KeyEvent) {
	for _, k := range keys {
		k.Pressed = true
		eng.SendKeyEvent(k)
		k.Pressed = false
		eng.SendKeyEvent(k)
	}
}

func dvRunes(s string) []widget.KeyEvent {
	var out []widget.KeyEvent
	for _, r := range s {
		out = append(out, widget.KeyEvent{Rune: r})
	}
	return out
}

func dvKey(code widget.KeyCode, mod widget.KeyMod) widget.KeyEvent {
	return widget.KeyEvent{Code: code, Mod: mod}
}

// Правка с клавиатуры через движок: набор, автоотступ, Tab внутрь текста
// (TabAcceptor), слово назад, буфер обмена, отмена и повтор.
func TestDiffView_KeyboardEditing(t *testing.T) {
	dv := widget.NewDiffView("", "")
	dv.SetText(widget.DiffLeft, "L", "", "alpha beta\n\tgamma\n")
	dv.SetText(widget.DiffRight, "R", "", "alpha beta\n\tgamma\n")
	eng := diffScene(t, dv)
	eng.SetFocus(dv)

	dv.SetCaret(widget.DiffLeft, 1, 6)
	dvKeys(eng, dvKey(widget.KeyEnter, 0))
	if l := dv.Lines(widget.DiffLeft); l[2] != "\t" {
		t.Fatalf("автоотступ: %q", l)
	}
	dvKeys(eng, dvKey(widget.KeyTab, 0))
	dvKeys(eng, dvRunes("x")...)
	if l := dv.Lines(widget.DiffLeft); l[2] != "\t\tx" {
		t.Fatalf("Tab не вставился в текст: %q", l)
	}
	if dv.ChangeCount() != 1 {
		t.Fatalf("отличий %d, ожидалось 1", dv.ChangeCount())
	}

	// Набор группируется: одна отмена снимает всё набранное подряд (Tab и
	// «x»), а перевод строки с автоотступом — отдельный шаг.
	dvKeys(eng, dvKey(widget.KeyZ, widget.ModCtrl))
	if l := dv.Lines(widget.DiffLeft); len(l) != 3 || l[2] != "\t" {
		t.Fatalf("отмена набора: %q", l)
	}
	dvKeys(eng, dvKey(widget.KeyY, widget.ModCtrl))
	if l := dv.Lines(widget.DiffLeft); l[2] != "\t\tx" {
		t.Fatalf("повтор: %q", l)
	}

	dv.SetCaret(widget.DiffLeft, 0, 10)
	dvKeys(eng, dvKey(widget.KeyBackspace, widget.ModCtrl))
	if l := dv.Lines(widget.DiffLeft); l[0] != "alpha " {
		t.Fatalf("Ctrl+Backspace: %q", l)
	}

	// Выделение Shift+Home и копирование; вставка в другую сторону.
	dvKeys(eng, dvKey(widget.KeyHome, widget.ModShift), dvKey(widget.KeyC, widget.ModCtrl))
	if got := widget.ClipboardGetText(); got != "alpha " {
		t.Fatalf("буфер обмена: %q", got)
	}
	dv.SetCaret(widget.DiffRight, 0, 0)
	dvKeys(eng, dvKey(widget.KeyV, widget.ModCtrl))
	if l := dv.Lines(widget.DiffRight); l[0] != "alpha alpha beta" {
		t.Fatalf("вставка: %q", l)
	}
	if dv.ActiveSide() != widget.DiffRight {
		t.Fatal("активная сторона не сменилась")
	}
}

// Копирование переносит строки с переводом строки файла: CRLF-файл отдаёт в
// буфер обмена CRLF, иначе вставка в Блокнот склеила бы строки.
func TestDiffView_CopyUsesFileEOL(t *testing.T) {
	dv := widget.NewDiffView("", "")
	dv.SetText(widget.DiffLeft, "L", "", "a\r\nb\r\nc\r\n")
	dv.SetCaret(widget.DiffLeft, 0, 0)
	dv.SelectAll()
	dv.Copy()
	// Перевод в конце файла — свойство файла (FinalNL), а не пустая строка
	// буфера, поэтому выделение всего кончается на «c».
	if got := widget.ClipboardGetText(); got != "a\r\nb\r\nc" {
		t.Fatalf("буфер обмена: %q", got)
	}
}

// Сторона только для чтения: правки не проходят, Tab снова уходит обходу
// фокуса, а перенос всего файла в другую сторону отменяем.
func TestDiffView_ReadOnlyAndCopyAll(t *testing.T) {
	dv := widget.NewDiffView("", "")
	dv.SetText(widget.DiffLeft, "L", "", "one\ntwo\n")
	dv.SetText(widget.DiffRight, "R", "", "one\nTWO\nthree\n")
	dv.SetReadOnly(widget.DiffRight, true)

	dv.SetCaret(widget.DiffRight, 0, 0)
	dv.InsertText("zzz")
	if l := dv.Lines(widget.DiffRight); l[0] != "one" {
		t.Fatalf("правка стороны только для чтения: %q", l)
	}
	if dv.AcceptsTab() {
		t.Fatal("сторона только для чтения забирает Tab")
	}
	dv.SetActiveSide(widget.DiffLeft)
	if !dv.AcceptsTab() {
		t.Fatal("редактируемая сторона не забирает Tab")
	}

	// Перенос в сторону только для чтения запрещён, из неё — разрешён.
	dv.CopyAll(true)
	if dv.Lines(widget.DiffRight)[1] != "TWO" {
		t.Fatal("перенос в сторону только для чтения")
	}
	dv.CopyAll(false)
	if dv.ChangeCount() != 0 || dv.Text(widget.DiffLeft) != "one\nTWO\nthree\n" {
		t.Fatalf("перенос всего файла: %d %q", dv.ChangeCount(), dv.Text(widget.DiffLeft))
	}
	dv.Undo()
	if dv.ChangeCount() == 0 {
		t.Fatal("отмена переноса всего файла")
	}
}

// Перенос одного блока и навигация по отличиям.
func TestDiffView_BlockCopyAndNavigation(t *testing.T) {
	dv := widget.NewDiffView("", "")
	dv.SetBounds(image.Rect(0, 0, 900, 500))
	dv.SetText(widget.DiffLeft, "L", "", "a\nb\nc\nd\ne\nf\n")
	dv.SetText(widget.DiffRight, "R", "", "a\nB\nc\nd\nE\nf\n")
	var copied []int
	dv.OnBlockCopied = func(i int, toRight bool) {
		if toRight {
			copied = append(copied, i)
		}
	}
	if dv.ChangeCount() != 2 {
		t.Fatalf("отличий %d", dv.ChangeCount())
	}
	dv.NextChange()
	if dv.CurrentChange() != 0 {
		t.Fatalf("первое отличие: %d", dv.CurrentChange())
	}
	dv.NextChange()
	if dv.CurrentChange() != 1 {
		t.Fatalf("второе отличие: %d", dv.CurrentChange())
	}
	dv.CopyCurrent(true)
	if dv.Lines(widget.DiffRight)[4] != "e" || dv.ChangeCount() != 1 {
		t.Fatalf("перенос блока: %q", dv.Lines(widget.DiffRight))
	}
	if len(copied) != 1 || copied[0] != 1 {
		t.Fatalf("OnBlockCopied: %v", copied)
	}
	dv.CopyBlock(0, false)
	if dv.Lines(widget.DiffLeft)[1] != "B" || dv.ChangeCount() != 0 {
		t.Fatalf("перенос влево: %q", dv.Lines(widget.DiffLeft))
	}
}

// Разметка: файлы относительно каталога разметки, флаги, команды из
// DataContext. Путь за пределы каталога разметки не открывается (SEC-8).
type diffVM struct {
	Save  *widget.RelayCommand
	Count *widget.RelayCommand
}

func TestDiffView_XAML(t *testing.T) {
	dir := t.TempDir()
	ui := filepath.Join(dir, "ui")
	if err := os.MkdirAll(ui, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(ui, "old.txt"), "x\ny\n")
	writeFile(t, filepath.Join(ui, "new.txt"), "x\nY\n")
	writeFile(t, filepath.Join(dir, "secret.txt"), "secret\n")
	xamlPath := filepath.Join(ui, "diff.xaml")
	writeFile(t, xamlPath, `<Canvas Width="900" Height="500">
  <DiffView x:Name="diff" LeftFile="old.txt" RightFile="new.txt"
            ReadOnlyLeft="True" HideUnchanged="True" ContextLines="1"
            FontSize="11" SaveCommand="{Binding Save}" DiffChangedCommand="{Binding Count}"/>
  <DiffView x:Name="escape" LeftFile="../secret.txt"/>
</Canvas>`)

	var savedSide any = "нет"
	counts := []int{}
	vm := &diffVM{
		Save:  &widget.RelayCommand{ExecuteFn: func(p any) { savedSide = p }},
		Count: &widget.RelayCommand{ExecuteFn: func(p any) { counts = append(counts, p.(int)) }},
	}
	_, reg, _, err := widget.LoadUIFromXAMLFileBindings(xamlPath, vm)
	if err != nil {
		t.Fatal(err)
	}
	dv, ok := reg["diff"].(*widget.DiffView)
	if !ok {
		t.Fatalf("diff собрался как %T", reg["diff"])
	}
	if dv.ChangeCount() != 1 || !dv.IsReadOnly(widget.DiffLeft) || !dv.HideUnchanged() {
		t.Fatalf("атрибуты: changes=%d ro=%v hide=%v", dv.ChangeCount(), dv.IsReadOnly(widget.DiffLeft), dv.HideUnchanged())
	}
	if got := dv.FilePath(widget.DiffRight); filepath.Base(got) != "new.txt" {
		t.Fatalf("файл правой стороны: %q", got)
	}

	dv.SetCaret(widget.DiffRight, 1, 1)
	dv.OnKeyEvent(widget.KeyEvent{Code: widget.KeyS, Mod: widget.ModCtrl, Pressed: true})
	if savedSide != widget.DiffRight {
		t.Fatalf("SaveCommand получил %v", savedSide)
	}
	dv.CopyAll(false) // левая только для чтения — ничего не происходит
	dv.CopyAll(true)
	if len(counts) == 0 || counts[len(counts)-1] != 0 {
		t.Fatalf("DiffChangedCommand: %v", counts)
	}

	esc := reg["escape"].(*widget.DiffView)
	if esc.FilePath(widget.DiffLeft) != "" || strings.Contains(esc.Text(widget.DiffLeft), "secret") {
		t.Fatal("разметка открыла файл за пределами своего каталога")
	}
}

// Разметка из fs.FS кладёт содержимое без пути на диске.
func TestDiffView_XAMLFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"a.txt": {Data: []byte("1\n2\n")},
		"b.txt": {Data: []byte("1\n3\n")},
	}
	_, reg, err := widget.LoadUIFromXAMLFS([]byte(`<Canvas Width="400" Height="300">
  <DiffView x:Name="d" LeftFile="a.txt" RightFile="b.txt"/>
</Canvas>`), fsys)
	if err != nil {
		t.Fatal(err)
	}
	dv := reg["d"].(*widget.DiffView)
	if dv.ChangeCount() != 1 || dv.FilePath(widget.DiffLeft) != "" || dv.Text(widget.DiffRight) != "1\n3\n" {
		t.Fatalf("fs.FS: changes=%d path=%q text=%q", dv.ChangeCount(), dv.FilePath(widget.DiffLeft), dv.Text(widget.DiffRight))
	}
	if err := dv.Save(widget.DiffLeft); err != widget.ErrDiffNoPath {
		t.Fatalf("сохранение вкомпилированного файла: %v", err)
	}
}

// Доступность: скринридер видит две текстовые панели с содержимым, а не одну
// безымянную группу.
func TestDiffView_AccessibilityChildren(t *testing.T) {
	dv := widget.NewDiffView("", "")
	dv.SetBounds(image.Rect(0, 0, 900, 500))
	dv.SetText(widget.DiffLeft, "old.go", "", "package a\n")
	dv.SetText(widget.DiffRight, "new.go", "", "package b\n")
	dv.SetReadOnly(widget.DiffLeft, true)

	tree := widget.BuildAccessTree(dv, nil)
	if tree == nil || tree.Role != widget.RoleGroup {
		t.Fatalf("корень: %+v", tree)
	}
	var panes []widget.AccessInfo
	for _, c := range tree.Children {
		if c.Role == widget.RoleTextInput {
			panes = append(panes, c.AccessInfo)
		}
	}
	if len(panes) != 2 {
		t.Fatalf("текстовых панелей %d, ожидалось 2", len(panes))
	}
	if !strings.Contains(panes[0].Name, "old.go") || panes[1].Value != "package b" {
		t.Fatalf("панели: %+v", panes)
	}
	ro := false
	for _, s := range panes[0].States {
		ro = ro || s == widget.StateReadOnly
	}
	if !ro {
		t.Fatal("левая панель не помечена только для чтения")
	}
	if panes[0].Bounds.Empty() || panes[0].Bounds.Max.X > panes[1].Bounds.Min.X {
		t.Fatalf("границы панелей: %v %v", panes[0].Bounds, panes[1].Bounds)
	}
}

// Два файла из проводника раскладываются по сторонам.
func TestDiffView_DropTwoFiles(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	writeFile(t, a, "1\n")
	writeFile(t, b, "2\n")
	dv := widget.NewDiffView("", "")
	dv.SetBounds(image.Rect(0, 0, 900, 500))
	if !dv.OnFilesDropped(10, 10, []string{a, b}) {
		t.Fatal("файлы не приняты")
	}
	if dv.FilePath(widget.DiffLeft) != a || dv.FilePath(widget.DiffRight) != b {
		t.Fatalf("стороны: %q %q", dv.FilePath(widget.DiffLeft), dv.FilePath(widget.DiffRight))
	}
}
