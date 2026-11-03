package dialog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
)

// recModal — заглушка движка, которая запоминает показанные модалки: окно
// подтверждения замены открывается поверх диалога.
type recModal struct {
	shown  []ModalWidget
	closed []ModalWidget
}

func (r *recModal) ShowModal(m ModalWidget)  { r.shown = append(r.shown, m) }
func (r *recModal) CloseModal(m ModalWidget) { r.closed = append(r.closed, m) }

// top — последняя показанная модалка.
func (r *recModal) top() *Dialog {
	if len(r.shown) == 0 {
		return nil
	}
	d, _ := r.shown[len(r.shown)-1].(*Dialog)
	return d
}

// saveTree — каталог с подпапкой, файлом .rdp и файлом .txt.
func saveTree(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"old.rdp", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// saveResult — итог диалога: вызван ли колбэк, путь и ok.
type saveResult struct {
	called bool
	path   string
	ok     bool
}

func openSave(t *testing.T, opts FileDialogOptions) (*FileDialog, *recModal, *saveResult) {
	t.Helper()
	rm := &recModal{}
	res := &saveResult{}
	fd := NewMessageBox(rm).ShowSaveFile(opts, func(p string, ok bool) {
		res.called, res.path, res.ok = true, p, ok
	})
	return fd, rm, res
}

// rdpFilter — фильтр, как у экспорта .rdp в Tandem.
var rdpFilter = []FileFilter{{Label: "RDP", Exts: []string{".rdp"}}, {Label: "*"}}

// По умолчанию Save — обзор: места, список с папками и файлами фильтра; вход
// в подпапку двойным щелчком, результат — выбранная папка плюс имя.
func TestFileSave_BrowserByDefault(t *testing.T) {
	dir := saveTree(t)
	fd, _, res := openSave(t, FileDialogOptions{StartDir: dir, InitialName: "main.home.local.rdp", Filters: rdpFilter})
	if fd.places == nil || fd.table == nil {
		t.Fatal("Save без Compact собран без панели мест или списка")
	}
	hasSave := false
	for _, b := range dialogButtons(fd) {
		hasSave = hasSave || b.GetText() == base.Tr("dlg.save")
	}
	if !hasSave {
		t.Fatal("у обзора Save нет кнопки «Сохранить»")
	}
	if got := fd.VisibleNames(); len(got) != 2 || got[0] != "[sub]" || got[1] != "old.rdp" {
		t.Fatalf("список %v, ожидались [sub] и old.rdp (фильтр .rdp)", got)
	}
	fd.Activate(0) // [sub]
	if fd.CurrentDir() != filepath.Join(dir, "sub") {
		t.Fatalf("после входа в подпапку каталог %q", fd.CurrentDir())
	}
	fd.confirm()
	if want := filepath.Join(dir, "sub", "main.home.local.rdp"); !res.ok || res.path != want {
		t.Fatalf("результат %+v, ожидался %q", *res, want)
	}
}

// Compact — прежняя компактная форма без списка.
func TestFileSave_CompactOption(t *testing.T) {
	fd, _, _ := openSave(t, FileDialogOptions{StartDir: t.TempDir(), InitialName: "a.txt", Compact: true})
	if fd.places != nil || fd.table != nil || fd.VisibleNames() != nil {
		t.Fatal("Compact: у компактной формы появились места или список")
	}
	if got := fd.Dialog().Bounds().Dy(); got != 232 {
		t.Fatalf("Compact: высота %d, ожидалась прежняя 232", got)
	}
	if fd.pathIn == nil {
		t.Fatal("Compact: нет ввода пути — без списка папок в подпапку не попасть")
	}
}

// Щелчок по файлу подставляет имя и показывает предупреждение о перезаписи.
func TestFileSave_SelectFileFillsNameAndWarns(t *testing.T) {
	dir := saveTree(t)
	fd, _, _ := openSave(t, FileDialogOptions{StartDir: dir, InitialName: "new.rdp", Filters: rdpFilter})
	if fd.warnIco.IsVisible() {
		t.Fatal("предупреждение при новом имени")
	}
	fd.selectEntry(1) // old.rdp
	if got := fd.nameIn.GetText(); got != "old.rdp" {
		t.Fatalf("имя после щелчка по файлу %q", got)
	}
	if !fd.warnIco.IsVisible() || fd.warnLbl.Text() == "" {
		t.Fatal("нет предупреждения о перезаписи после щелчка по существующему файлу")
	}
	// В другой папке такого файла нет — предупреждение уходит.
	fd.navigate(filepath.Join(dir, "sub"))
	if fd.warnIco.IsVisible() {
		t.Fatal("предупреждение осталось после перехода в папку без этого файла")
	}
}

// Имени без расширения достаётся расширение выбранного фильтра; своё
// расширение и фильтр «все файлы» имя не меняют.
func TestFileSave_FilterExtension(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name   string
		filter int
		want   string
	}{
		{"conn", 0, "conn.rdp"},
		{"conn.txt", 0, "conn.txt"},
		{"conn", 1, "conn"},
	} {
		fd, _, res := openSave(t, FileDialogOptions{StartDir: dir, Filters: rdpFilter})
		fd.filter = c.filter
		fd.SetFileName(c.name)
		fd.confirm()
		if want := filepath.Join(dir, c.want); res.path != want {
			t.Fatalf("имя %q, фильтр %d: путь %q, ожидался %q", c.name, c.filter, res.path, want)
		}
	}
}

// Существующий файл: сначала подтверждение; «Нет» оставляет диалог, «Да»
// (Enter) — выбор. Подтверждение учитывает и расширение фильтра.
func TestFileSave_ConfirmReplace(t *testing.T) {
	dir := saveTree(t)
	fd, rm, res := openSave(t, FileDialogOptions{StartDir: dir, Filters: rdpFilter})
	fd.SetFileName("old") // станет old.rdp
	fd.confirm()
	ask := rm.top()
	if res.called || ask == nil || ask == fd.Dialog() {
		t.Fatalf("замена без вопроса: результат %+v, модалка %v", *res, ask)
	}
	var no *base.Button
	for _, c := range ask.Children() {
		if b, ok := c.(*base.Button); ok && b.GetText() == base.Tr("dlg.no") {
			no = b
		}
	}
	if no == nil {
		t.Fatal("в вопросе нет кнопки «Нет»")
	}
	no.OnClick()
	if res.called {
		t.Fatalf("«Нет» закрыло диалог: %+v", *res)
	}

	fd.confirm()
	rm.top().DefaultAction() // Enter — «Да»
	if want := filepath.Join(dir, "old.rdp"); !res.ok || res.path != want {
		t.Fatalf("«Да»: результат %+v, ожидался %q", *res, want)
	}
}

// Имя существующей папки в поле — вход в неё, в поле возвращается имя по
// умолчанию; файл не выбирается.
func TestFileSave_NameOfFolderEnters(t *testing.T) {
	dir := saveTree(t)
	fd, _, res := openSave(t, FileDialogOptions{StartDir: dir, InitialName: "a.rdp", Filters: rdpFilter})
	fd.SetFileName("sub")
	fd.confirm()
	if res.called || fd.CurrentDir() != filepath.Join(dir, "sub") || fd.nameIn.GetText() != "a.rdp" {
		t.Fatalf("имя папки: результат %+v, каталог %q, имя %q", *res, fd.CurrentDir(), fd.nameIn.GetText())
	}
}

// Ввод пути: щелчок мимо сегментов строки пути открывает поле с текущей
// папкой; Enter переходит; несуществующий путь — ошибка, поле открыто;
// Escape и потеря фокуса возвращают строку пути, диалог не закрывается.
func TestFileDialog_PathEdit(t *testing.T) {
	dir := saveTree(t)
	other := t.TempDir()
	modes := []fileMode{
		{name: "Open", mode: FileOpen},
		{name: "Save", mode: FileSave},
		{name: "SaveCompact", mode: FileSave, compact: true},
		{name: "Folder", mode: FolderPick},
	}
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			rm := &recModal{}
			opts := FileDialogOptions{StartDir: dir, InitialName: "a.txt", Compact: m.compact}
			var fd *FileDialog
			switch m.mode {
			case FileOpen:
				fd = NewMessageBox(rm).ShowOpenFile(opts, nil)
			case FolderPick:
				fd = NewMessageBox(rm).ShowPickFolder(opts, nil)
			default:
				fd = NewMessageBox(rm).ShowSaveFile(opts, nil)
			}
			click := func() {
				b := fd.crumb.Bounds()
				fd.crumb.OnMouseButton(base.MouseEvent{X: b.Max.X - 3, Y: b.Min.Y + 3, Button: base.MouseLeft, Pressed: true})
			}

			click()
			if !fd.pathEditing || !fd.pathIn.IsVisible() || fd.crumb.IsVisible() || fd.pathIn.GetText() != dir {
				t.Fatalf("после щелчка: ввод %v, поле %v, строка %v, текст %q",
					fd.pathEditing, fd.pathIn.IsVisible(), fd.crumb.IsVisible(), fd.pathIn.GetText())
			}
			fd.pathIn.SetText(filepath.Join(dir, "nope"))
			fd.confirm()
			if !fd.pathEditing || fd.pathIn.ValidationError() == "" || fd.CurrentDir() != dir {
				t.Fatalf("несуществующий путь: ввод %v, ошибка %q, каталог %q",
					fd.pathEditing, fd.pathIn.ValidationError(), fd.CurrentDir())
			}
			fd.pathIn.SetText(other)
			fd.confirm()
			if fd.pathEditing || !fd.crumb.IsVisible() || fd.CurrentDir() != other {
				t.Fatalf("Enter: ввод %v, строка %v, каталог %q", fd.pathEditing, fd.crumb.IsVisible(), fd.CurrentDir())
			}

			// Относительный путь — от текущей папки.
			fd.navigate(dir)
			click()
			fd.pathIn.SetText("sub")
			fd.confirm()
			if fd.CurrentDir() != filepath.Join(dir, "sub") {
				t.Fatalf("относительный путь: каталог %q", fd.CurrentDir())
			}

			// Escape: диалог не закрывается, ввод отменён; второй — закрывает.
			click()
			if fd.Dialog().CanClose() || fd.pathEditing {
				t.Fatal("Escape во время ввода закрыл бы диалог или не отменил ввод")
			}
			if !fd.Dialog().CanClose() {
				t.Fatal("Escape без ввода не закрывает диалог")
			}

			// Потеря фокуса — тоже отмена.
			click()
			fd.pathIn.SetFocused(true)
			fd.pathIn.SetFocused(false)
			if fd.pathEditing || !fd.crumb.IsVisible() {
				t.Fatal("потеря фокуса не вернула строку пути")
			}
		})
	}
}

// Путь к файлу: переход в его папку, имя — в поле «Имя файла».
func TestFileDialog_PathEditFile(t *testing.T) {
	dir := saveTree(t)
	fd, _, res := openSave(t, FileDialogOptions{StartDir: t.TempDir(), Filters: rdpFilter})
	fd.beginPathEdit()
	fd.pathIn.SetText(`"` + filepath.Join(dir, "old.rdp") + `"`) // в кавычках, как копирует Проводник
	fd.confirm()
	if res.called || fd.CurrentDir() != dir || fd.nameIn.GetText() != "old.rdp" || !fd.warnIco.IsVisible() {
		t.Fatalf("путь к файлу: результат %+v, каталог %q, имя %q, предупреждение %v",
			*res, fd.CurrentDir(), fd.nameIn.GetText(), fd.warnIco.IsVisible())
	}
}

// Путь вне разрешённых корней — ошибка, а не переход.
func TestFileDialog_PathEditRoots(t *testing.T) {
	root := saveTree(t)
	fd, _, _ := openSave(t, FileDialogOptions{StartDir: root, AllowedRoots: []string{root}})
	fd.beginPathEdit()
	fd.pathIn.SetText(t.TempDir())
	fd.confirm()
	if !fd.pathEditing || fd.pathIn.ValidationError() == "" || fd.CurrentDir() != root {
		t.Fatalf("вне корней: ввод %v, ошибка %q, каталог %q", fd.pathEditing, fd.pathIn.ValidationError(), fd.CurrentDir())
	}
}

// Open: полный путь в поле имени — этот файл, а не «текущая папка + путь».
func TestFileOpen_AbsoluteName(t *testing.T) {
	dir := saveTree(t)
	var got string
	fd := NewMessageBox(&recModal{}).ShowOpenFile(FileDialogOptions{StartDir: t.TempDir()},
		func(p string, ok bool) { got = p })
	fd.SetFileName(filepath.Join(dir, "notes.txt"))
	fd.confirm()
	if want := filepath.Join(dir, "notes.txt"); got != want {
		t.Fatalf("Open с полным путём: %q, ожидался %q", got, want)
	}
}

// Строка пути отбрасывает головные сегменты так, чтобы вместе с «… › »
// поместилось всё остальное; последний сегмент не отбрасывается.
func TestCrumbFirst(t *testing.T) {
	const sep, ell = 5, 8 // сегмент с разделителем — ширина + 17, «… › » — 25
	for _, c := range []struct {
		widths []int
		avail  int
		want   int
	}{
		{[]int{50, 50}, 200, 0},
		{[]int{100, 100, 100}, 334, 0},
		{[]int{100, 100, 100}, 333, 1},
		{[]int{100, 100, 100}, 242, 1}, // 217 + 25 — впритык
		{[]int{100, 100, 100}, 241, 2}, // без «… › » хватило бы и одного
		{[]int{500}, 100, 0},
		{[]int{100, 500}, 100, 1},
	} {
		if got := crumbFirst(c.widths, sep, ell, c.avail); got != c.want {
			t.Errorf("crumbFirst(%v, avail %d) = %d, ожидалось %d", c.widths, c.avail, got, c.want)
		}
	}
}
