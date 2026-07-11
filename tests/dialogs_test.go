package tests

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

func newDialogEngine() *engine.Engine {
	eng := engine.New(600, 400, 20)
	eng.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 30, A: 255})
	root.SetBounds(image.Rect(0, 0, 600, 400))
	eng.SetRoot(root)
	return eng
}

// severity-пресеты дают правильную роль/иконку и локализованный заголовок.
func TestDialog_MessageBoxSeverity(t *testing.T) {
	widget.SetLanguage("RU")
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)

	dlg := mb.ShowError("", "Файл не найден")
	if dlg.Title != "Ошибка" {
		t.Errorf("пустой caption для error должен стать 'Ошибка', got %q", dlg.Title)
	}
	// значок присутствует в дереве
	if !hasWidgetType(dlg, func(w widget.Widget) bool {
		ic, ok := w.(*widget.DialogIcon)
		return ok && ic.Severity == widget.SeverityError
	}) {
		t.Error("нет значка severity=error")
	}
	eng.CloseModal(dlg)
}

// Ctrl+C копирует содержимое в формате Windows MessageBox.
func TestDialog_MessageBoxCtrlC(t *testing.T) {
	widget.SetLanguage("RU")
	widget.UseMemoryClipboard()
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)
	dlg := mb.ShowSeverity("Внимание", "Документ изменён", widget.SeverityWarning, widget.MBYesNoCancel, nil)

	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyC, Mod: widget.ModCtrl, Pressed: true})

	got := widget.ClipboardGetText()
	sep := "---------------------------"
	if strings.Count(got, sep) != 4 {
		t.Errorf("ожидалось 4 разделителя, got %d:\n%s", strings.Count(got, sep), got)
	}
	for _, want := range []string{"Внимание", "Документ изменён", "Да", "Нет", "Отмена"} {
		if !strings.Contains(got, want) {
			t.Errorf("буфер не содержит %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "\r\n") {
		t.Error("ожидались CRLF-переводы строк (формат Windows)")
	}
	eng.CloseModal(dlg)
}

// Enter активирует кнопку по умолчанию (первая accent).
func TestDialog_MessageBoxEnter(t *testing.T) {
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)
	var result widget.MessageBoxResult = -1
	mb.ShowYesNo("Q", "Продолжить?", func(r widget.MessageBoxResult) { result = r })

	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if result != widget.MBResultYes {
		t.Errorf("Enter должен дать Yes (default accent), got %v", result)
	}
}

// Смена языка переводит кнопки и заголовок открытого диалога.
func TestDialog_MessageBoxLiveLocale(t *testing.T) {
	widget.SetLanguage("RU")
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)
	dlg := mb.ShowInfo("", "Готово")

	if dlg.Title != "Информация" {
		t.Fatalf("RU заголовок: %q", dlg.Title)
	}
	okBtn := findButton(dlg)
	if okBtn == nil || okBtn.Text != "OK" {
		t.Fatalf("кнопка OK не найдена")
	}

	widget.SetLanguage("EN")
	if dlg.Title != "Information" {
		t.Errorf("после смены на EN заголовок: %q, ожидался 'Information'", dlg.Title)
	}
	widget.SetLanguage("RU") // вернуть для других тестов
	eng.CloseModal(dlg)
}

// После закрытия подписки на язык сняты (не растут при повторных показах).
func TestDialog_LocaleUnsubscribeOnClose(t *testing.T) {
	widget.SetLanguage("RU")
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)
	for i := 0; i < 5; i++ {
		dlg := mb.ShowInfo("", "x")
		eng.CloseModal(dlg)
	}
	// Смена языка не должна паниковать/утекать; заголовок закрытого диалога
	// больше не подписан — проверяем, что вызов проходит без эффекта.
	widget.SetLanguage("EN")
	widget.SetLanguage("RU")
}

// ─── helpers ────────────────────────────────────────────────────────────────

func hasWidgetType(w widget.Widget, pred func(widget.Widget) bool) bool {
	if pred(w) {
		return true
	}
	for _, c := range w.Children() {
		if hasWidgetType(c, pred) {
			return true
		}
	}
	return false
}

func findButton(w widget.Widget) *widget.Button {
	if b, ok := w.(*widget.Button); ok {
		return b
	}
	for _, c := range w.Children() {
		if b := findButton(c); b != nil {
			return b
		}
	}
	return nil
}

// InputDialog: подтверждение возвращает текст, отмена — ok=false.
func TestDialog_Input(t *testing.T) {
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)

	var got string
	var gotOK bool
	var called bool
	mb.ShowInput("Rename", "New name:", "file.txt", nil, func(text string, ok bool) {
		got, gotOK, called = text, ok, true
	})
	// Enter подтверждает начальный текст.
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if !called || !gotOK || got != "file.txt" {
		t.Fatalf("Enter: called=%v ok=%v text=%q", called, gotOK, got)
	}
}

// Валидация блокирует подтверждение при ошибке.
func TestDialog_InputValidation(t *testing.T) {
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)
	confirmed := false
	mb.ShowInput("T", "V:", "", func(s string) string {
		if s == "" {
			return "пусто"
		}
		return ""
	}, func(text string, ok bool) {
		if ok {
			confirmed = true
		}
	})
	// Пустое значение — Enter не должен подтвердить.
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if confirmed {
		t.Error("валидация не заблокировала подтверждение пустого значения")
	}
}

// Escape отменяет InputDialog (ok=false).
func TestDialog_InputEscapeCancels(t *testing.T) {
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)
	var gotOK = true
	var called bool
	mb.ShowInput("T", "V:", "x", nil, func(text string, ok bool) { gotOK, called = ok, true })
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	if !called || gotOK {
		t.Errorf("Escape должен дать ok=false: called=%v ok=%v", called, gotOK)
	}
}

// ProgressDialog: сеттеры и Close работают, Close идемпотентен.
func TestDialog_Progress(t *testing.T) {
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)
	cancelled := false
	pd := mb.ShowProgress("", "Копирование…", func() { cancelled = true })
	pd.SetProgress(0.5)
	pd.SetStatus("Проверка…")
	pd.SetIndeterminate(true)
	pd.Close()
	pd.Close() // идемпотентно — без паники
	if cancelled {
		t.Error("Close не должен вызывать onCancel")
	}
}

// ─── Файловые диалоги ────────────────────────────────────────────────────────

func makeFileTree(t *testing.T) string {
	dir := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "report.txt"), []byte("hi"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "data.csv"), []byte("a,b"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "image.png"), []byte("x"), 0o644))
	must(os.Mkdir(filepath.Join(dir, "subdir"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "subdir", "inner.txt"), []byte("y"), 0o644))
	return dir
}

// Open: двойной клик по файлу → путь; фильтр скрывает несоответствующие.
func TestDialog_FileOpen(t *testing.T) {
	dir := makeFileTree(t)
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)

	var gotPath string
	var gotOK bool
	fd := mb.ShowOpenFile(widget.FileDialogOptions{
		StartDir: dir,
		Filters:  []widget.FileFilter{{Label: "Текст", Exts: []string{".txt"}}},
	}, func(path string, ok bool) { gotPath, gotOK = path, ok })

	// В списке при фильтре .txt должны быть: [subdir] и report.txt (2 элемента).
	if n := len(fd.VisibleNames()); n != 2 {
		t.Fatalf("при фильтре .txt ожидалось 2 элемента (папка+txt), got %d: %v", n, fd.VisibleNames())
	}

	// Вводим имя и подтверждаем (Enter).
	fd.SetFileName("report.txt")
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if !gotOK || filepath.Base(gotPath) != "report.txt" {
		t.Fatalf("Open: ok=%v path=%q", gotOK, gotPath)
	}
}

// Save: возвращает путь с введённым именем в текущем каталоге.
func TestDialog_FileSave(t *testing.T) {
	dir := makeFileTree(t)
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)
	var gotPath string
	var gotOK bool
	fd := mb.ShowSaveFile(widget.FileDialogOptions{
		StartDir:    dir,
		InitialName: "new-file.txt",
	}, func(path string, ok bool) { gotPath, gotOK = path, ok })
	_ = fd
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if !gotOK || gotPath != filepath.Join(dir, "new-file.txt") { //nolint
		t.Fatalf("Save: ok=%v path=%q, ожидался %q", gotOK, gotPath, filepath.Join(dir, "new-file.txt"))
	}
}

// FolderPick: подтверждение возвращает текущий каталог, файлы скрыты.
func TestDialog_FolderPick(t *testing.T) {
	dir := makeFileTree(t)
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)
	var gotPath string
	var gotOK bool
	fd := mb.ShowPickFolder(widget.FileDialogOptions{StartDir: dir},
		func(path string, ok bool) { gotPath, gotOK = path, ok })

	if n := len(fd.VisibleNames()); n != 1 { // только [subdir]
		t.Fatalf("FolderPick: ожидалась 1 папка, got %d: %v", n, fd.VisibleNames())
	}
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if !gotOK || gotPath != dir {
		t.Fatalf("FolderPick: ok=%v path=%q, ожидался %q", gotOK, gotPath, dir)
	}
}

// Кнопка ✕ в заголовке закрывает диалог с семантикой отмены (как Escape).
func TestDialog_CloseButtonCancels(t *testing.T) {
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)
	gotOK := true
	called := false
	id := mb.ShowInput("", "name:", "abc", nil, func(_ string, ok bool) { called = true; gotOK = ok })
	// ✕ расположен у правого края заголовка.
	dlgB := id.Dialog().Bounds()
	x := dlgB.Max.X - 18
	y := dlgB.Min.Y + 17
	eng.SendMouseButton(x, y, widget.MouseLeft, true)
	eng.SendMouseButton(x, y, widget.MouseLeft, false)
	if !called || gotOK {
		t.Fatalf("close-btn должен отменять: called=%v ok=%v", called, gotOK)
	}
}

// Диалог таскается за заголовок: press на титлбаре + move сдвигает диалог
// вместе с детьми; кнопка ✕ после перетаскивания работает по новой позиции.
func TestDialog_DragByTitleBar(t *testing.T) {
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)
	canceled := false
	id := mb.ShowInput("", "name:", "abc", nil, func(_ string, ok bool) { canceled = !ok })
	dlg := id.Dialog()
	before := dlg.Bounds()
	okBtnBefore := findButton(dlg).Bounds() // первая *Button = OK

	// Тянем за середину заголовка на (+60, +40).
	sx, sy := before.Min.X+40, before.Min.Y+12
	eng.SendMouseButton(sx, sy, widget.MouseLeft, true)
	eng.SendMouseMove(sx+60, sy+40)
	eng.SendMouseButton(sx+60, sy+40, widget.MouseLeft, false)

	after := dlg.Bounds()
	if after.Min.X != before.Min.X+60 || after.Min.Y != before.Min.Y+40 {
		t.Fatalf("диалог не сдвинулся: %v → %v", before, after)
	}
	okBtnAfter := findButton(dlg).Bounds()
	if okBtnAfter.Min.X != okBtnBefore.Min.X+60 || okBtnAfter.Min.Y != okBtnBefore.Min.Y+40 {
		t.Fatalf("дети не сдвинулись: %v → %v", okBtnBefore, okBtnAfter)
	}

	// ✕ по НОВОЙ позиции закрывает с отменой.
	cx, cy := after.Max.X-18, after.Min.Y+17
	eng.SendMouseButton(cx, cy, widget.MouseLeft, true)
	eng.SendMouseButton(cx, cy, widget.MouseLeft, false)
	if !canceled {
		t.Fatal("✕ по новой позиции не сработал")
	}
}

// При установленном OnDragMove (нативный режим: диалог в собственном окне ОС)
// перетаскивание за заголовок вызывает колбэк с дельтой и НЕ сдвигает bounds
// самого диалога — двигается нативное окно, а виджет в своём холсте неподвижен.
func TestDialog_OnDragMove_NativeMode(t *testing.T) {
	eng := newDialogEngine()
	mb := widget.NewMessageBox(eng)
	id := mb.ShowInput("", "name:", "abc", nil, nil)
	dlg := id.Dialog()
	before := dlg.Bounds()

	var gotDX, gotDY int
	var calls int
	dlg.OnDragMove = func(dx, dy int) { gotDX += dx; gotDY += dy; calls++ }

	sx, sy := before.Min.X+40, before.Min.Y+12
	eng.SendMouseButton(sx, sy, widget.MouseLeft, true)
	eng.SendMouseMove(sx+60, sy+40)
	eng.SendMouseButton(sx+60, sy+40, widget.MouseLeft, false)

	if calls == 0 {
		t.Fatal("OnDragMove не вызван")
	}
	if gotDX != 60 || gotDY != 40 {
		t.Fatalf("неверная дельта: got (%d,%d), want (60,40)", gotDX, gotDY)
	}
	if after := dlg.Bounds(); after != before {
		t.Fatalf("bounds диалога не должны меняться в нативном режиме: %v → %v", before, after)
	}
}

// ─── Вписывание модалок в холст ──────────────────────────────────────────────

// TestShowModal_LargerThanCanvas_PinnedTopLeft — диалог больше холста
// прижимается к левому/верхнему краю, а не центрируется в отрицательные
// координаты: титлбар и ✕ должны оставаться видимыми и достижимыми.
func TestShowModal_LargerThanCanvas_PinnedTopLeft(t *testing.T) {
	eng := newDialogEngine() // холст 600×400
	dlg := widget.NewDialog("Огромный", 700, 500)
	eng.ShowModal(dlg)
	defer eng.CloseModal(dlg)

	if b := dlg.Bounds(); b.Min.X != 0 || b.Min.Y != 0 {
		t.Fatalf("диалог больше холста должен прижиматься к (0,0), а он %v", b)
	}
}

// TestSetResolution_ClampsOpenModal — открытый диалог был отцентрирован под
// прежний холст; при уменьшении разрешения он обязан быть вписан в новый
// (раньше уезжал за край и обрезался). Дети сдвигаются вместе с ним.
func TestSetResolution_ClampsOpenModal(t *testing.T) {
	eng := newDialogEngine() // холст 600×400
	dlg := widget.NewDialog("Диалог", 300, 200)
	btn := widget.NewButton("OK")
	btn.SetBounds(image.Rect(20, 160, 120, 190))
	dlg.AddChild(btn)
	eng.ShowModal(dlg)
	defer eng.CloseModal(dlg)

	before := dlg.Bounds() // центр 600×400 → (150,100)-(450,300)
	if before.Min.X != 150 || before.Min.Y != 100 {
		t.Fatalf("диалог не отцентрирован: %v", before)
	}
	btnOff := btn.Bounds().Min.Sub(before.Min)

	eng.SetResolution(320, 240)

	after := dlg.Bounds()
	if after.Dx() != 300 || after.Dy() != 200 {
		t.Fatalf("размер диалога изменился: %v", after)
	}
	if after.Min.X < 0 || after.Min.Y < 0 || after.Max.X > 320 || after.Max.Y > 240 {
		t.Fatalf("диалог не вписан в холст 320×240: %v", after)
	}
	if got := btn.Bounds().Min.Sub(after.Min); got != btnOff {
		t.Fatalf("ребёнок отстал от диалога: смещение %v, want %v", got, btnOff)
	}

	// Увеличение холста обратно не трогает уже видимый целиком диалог.
	moved := after
	eng.SetResolution(600, 400)
	if got := dlg.Bounds(); got != moved {
		t.Fatalf("видимый целиком диалог не должен переезжать: %v → %v", moved, got)
	}
}
