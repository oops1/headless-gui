package widget

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// FileDialogMode — режим файлового диалога.
type FileDialogMode int

const (
	FileOpen   FileDialogMode = iota // выбор существующего файла
	FileSave                         // сохранение (имя + предупреждение перезаписи)
	FolderPick                       // выбор папки
)

// FileFilter — фильтр расширений: подпись + список расширений (".txt").
// Пустой Exts или {"*"} — все файлы.
type FileFilter struct {
	Label string
	Exts  []string
}

func (f FileFilter) match(name string) bool {
	if len(f.Exts) == 0 {
		return true
	}
	lower := strings.ToLower(name)
	for _, e := range f.Exts {
		if e == "*" || e == ".*" {
			return true
		}
		if strings.HasSuffix(lower, strings.ToLower(e)) {
			return true
		}
	}
	return false
}

// FileDialogOptions — параметры файлового диалога.
type FileDialogOptions struct {
	Mode        FileDialogMode
	Title       string       // пустой → локализованный по режиму
	StartDir    string       // начальный каталог (пустой → домашний)
	InitialName string       // предзаполненное имя (для Save)
	Filters     []FileFilter // фильтры расширений (пустой → «Все файлы»)
	ShowHidden  bool         // показывать скрытые (имена с точкой)
}

// fileEntry — элемент каталога.
type fileEntry struct {
	name  string
	dir   bool
	size  int64
	mod   time.Time
}

// FileDialog — модальный файловый браузер (Open/Save/Folder).
// Полностью на движке: работает в headless-режиме и показывает файловую
// систему ПРОЦЕССА (в стриминге — сервера). Темизируется и локализуется.
type FileDialog struct {
	eng      ModalShower
	dlg      *Dialog
	opts     FileDialogOptions
	onResult func(path string, ok bool)

	cur      string // текущий каталог
	entries  []fileEntry
	filter   int // индекс активного фильтра

	pathLbl  *Label
	list     *ListView
	nameIn   *TextInput
	warnLbl  *Label
}

// ShowOpenFile показывает диалог выбора файла.
func (mb *MessageBox) ShowOpenFile(opts FileDialogOptions, onResult func(path string, ok bool)) *FileDialog {
	opts.Mode = FileOpen
	return mb.showFileDialog(opts, onResult)
}

// ShowSaveFile показывает диалог сохранения файла.
func (mb *MessageBox) ShowSaveFile(opts FileDialogOptions, onResult func(path string, ok bool)) *FileDialog {
	opts.Mode = FileSave
	return mb.showFileDialog(opts, onResult)
}

// ShowPickFolder показывает диалог выбора папки.
func (mb *MessageBox) ShowPickFolder(opts FileDialogOptions, onResult func(path string, ok bool)) *FileDialog {
	opts.Mode = FolderPick
	return mb.showFileDialog(opts, onResult)
}

func (mb *MessageBox) showFileDialog(opts FileDialogOptions, onResult func(path string, ok bool)) *FileDialog {
	const (
		dlgW    = 640
		dlgH    = 460
		padX    = 14
		titleH  = 32
	)
	title := opts.Title
	if title == "" {
		switch opts.Mode {
		case FileSave:
			title = Tr("dlg.title.save")
		case FolderPick:
			title = Tr("dlg.title.folder")
		default:
			title = Tr("dlg.title.open")
		}
	}
	if len(opts.Filters) == 0 {
		opts.Filters = []FileFilter{{Label: "*", Exts: nil}}
	}
	start := opts.StartDir
	if start == "" {
		start, _ = os.UserHomeDir()
	}
	if start == "" {
		start = "."
	}

	dlg := NewDialog(title, dlgW, dlgH)
	fd := &FileDialog{eng: mb.eng, dlg: dlg, opts: opts, onResult: onResult}

	// ── Путь (breadcrumb-строка) + кнопка «Вверх» ────────────────────────
	pathY := titleH + 10
	fd.pathLbl = NewLabel("", dlg.TitleColor)
	fd.pathLbl.SetBounds(image.Rect(padX, pathY+6, dlgW-padX-100, pathY+24))
	dlg.AddChild(fd.pathLbl)

	upBtn := NewButton("↑ " + Tr("dlg.file.up"))
	upBtn.SetBounds(image.Rect(dlgW-padX-90, pathY, dlgW-padX, pathY+28))
	upBtn.OnClick = fd.goUp
	dlg.AddChild(upBtn)

	// ── Список файлов ────────────────────────────────────────────────────
	listY := pathY + 38
	listH := dlgH - listY - 96
	fd.list = NewListView()
	fd.list.SetBounds(image.Rect(padX, listY, dlgW-padX, listY+listH))
	fd.list.OnActivate = func(idx int, _ string) { fd.activate(idx) }
	fd.list.OnSelect = func(idx int, _ string) { fd.selectEntry(idx) }
	dlg.AddChild(fd.list)

	// ── Имя файла + фильтр ───────────────────────────────────────────────
	nameY := listY + listH + 10
	nameLbl := NewLabel(Tr("dlg.file.name"), dlg.TitleColor)
	nameLbl.SetBounds(image.Rect(padX, nameY+6, padX+56, nameY+24))
	dlg.AddChild(nameLbl)

	fd.nameIn = NewTextInput("")
	fd.nameIn.SetText(opts.InitialName)
	fd.nameIn.SetBounds(image.Rect(padX+60, nameY, dlgW-padX-210, nameY+30))
	if opts.Mode == FolderPick {
		fd.nameIn.SetEnabled(false)
	}
	dlg.AddChild(fd.nameIn)

	filterItems := make([]string, len(opts.Filters))
	for i, f := range opts.Filters {
		filterItems[i] = fileFilterLabel(f)
	}
	filterDD := NewDropdown(filterItems...)
	filterDD.SetBounds(image.Rect(dlgW-padX-196, nameY, dlgW-padX, nameY+30))
	filterDD.OnChange = func(i int, _ string) { fd.filter = i; fd.reload() }
	dlg.AddChild(filterDD)

	// ── Предупреждение (Save: перезапись) ────────────────────────────────
	fd.warnLbl = NewLabel("", severityColor(SeverityWarning))
	fd.warnLbl.FontSize = 10
	fd.warnLbl.SetBounds(image.Rect(padX, nameY+36, dlgW-padX-200, nameY+52))
	dlg.AddChild(fd.warnLbl)

	// ── Кнопки ───────────────────────────────────────────────────────────
	const btnW, btnH, btnGap = 100, 30, 10
	btnY := dlgH - 14 - btnH
	okKey := "dlg.open"
	if opts.Mode == FileSave {
		okKey = "dlg.save"
	} else if opts.Mode == FolderPick {
		okKey = "dlg.select"
	}
	okBtn := trBtn(okKey, true)
	okBtn.SetBounds(image.Rect(dlgW-padX-btnW*2-btnGap, btnY, dlgW-padX-btnW-btnGap, btnY+btnH))
	okBtn.OnClick = fd.confirm
	dlg.AddChild(okBtn)

	cancelBtn := trBtn("dlg.cancel", false)
	cancelBtn.SetBounds(image.Rect(dlgW-padX-btnW, btnY, dlgW-padX, btnY+btnH))
	cancelBtn.OnClick = func() {
		mb.eng.CloseModal(dlg)
		if onResult != nil {
			onResult("", false)
		}
	}
	dlg.AddChild(cancelBtn)

	dlg.OnLanguageChange(func() {
		okBtn.SetText(Tr(okKey))
		cancelBtn.SetText(Tr("dlg.cancel"))
		upBtn.SetText("↑ " + Tr("dlg.file.up"))
		nameLbl.SetText(Tr("dlg.file.name"))
	})
	dlg.DefaultAction = fd.confirm
	dlg.CancelAction = func() {
		if onResult != nil {
			onResult("", false)
		}
	}

	fd.navigate(start)
	mb.eng.ShowModal(dlg)
	return fd
}

// navigate переходит в каталог dir и перечитывает содержимое.
func (fd *FileDialog) navigate(dir string) {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	fd.cur = dir
	fd.reload()
}

// reload перечитывает текущий каталог с учётом фильтра.
func (fd *FileDialog) reload() {
	fd.pathLbl.SetText(fd.cur)
	fd.entries = readDirEntries(fd.cur, fd.opts.ShowHidden)

	f := fd.opts.Filters[fd.filter]
	items := make([]string, 0, len(fd.entries)+1)
	kept := make([]fileEntry, 0, len(fd.entries))
	for _, e := range fd.entries {
		if !e.dir {
			if fd.opts.Mode == FolderPick {
				continue // в режиме папок файлы не показываем
			}
			if !f.match(e.name) {
				continue
			}
		}
		kept = append(kept, e)
		items = append(items, formatEntry(e))
	}
	fd.entries = kept
	fd.list.SetItems(items)
}

// selectEntry — одиночный клик: для файла кладём имя в поле.
func (fd *FileDialog) selectEntry(idx int) {
	if idx < 0 || idx >= len(fd.entries) {
		return
	}
	e := fd.entries[idx]
	if !e.dir && fd.opts.Mode != FolderPick {
		fd.nameIn.SetText(e.name)
		fd.updateWarning()
	}
}

// activate — двойной клик / Enter: вход в папку или подтверждение файла.
func (fd *FileDialog) activate(idx int) {
	if idx < 0 || idx >= len(fd.entries) {
		return
	}
	e := fd.entries[idx]
	if e.dir {
		fd.navigate(filepath.Join(fd.cur, e.name))
		return
	}
	fd.nameIn.SetText(e.name)
	fd.confirm()
}

// goUp поднимается на уровень выше.
func (fd *FileDialog) goUp() {
	parent := filepath.Dir(fd.cur)
	if parent != fd.cur {
		fd.navigate(parent)
	}
}

// updateWarning показывает/скрывает предупреждение о перезаписи (Save).
func (fd *FileDialog) updateWarning() {
	if fd.opts.Mode != FileSave {
		return
	}
	name := strings.TrimSpace(fd.nameIn.GetText())
	if name == "" {
		fd.warnLbl.SetText("")
		return
	}
	if st, err := os.Stat(filepath.Join(fd.cur, name)); err == nil && !st.IsDir() {
		fd.warnLbl.SetText(Tr("dlg.file.overwrite"))
	} else {
		fd.warnLbl.SetText("")
	}
}

// confirm подтверждает выбор согласно режиму.
func (fd *FileDialog) confirm() {
	switch fd.opts.Mode {
	case FolderPick:
		fd.finish(fd.cur)
	case FileSave:
		name := strings.TrimSpace(fd.nameIn.GetText())
		if name == "" {
			return
		}
		fd.finish(filepath.Join(fd.cur, name))
	default: // FileOpen
		name := strings.TrimSpace(fd.nameIn.GetText())
		if name == "" {
			return
		}
		p := filepath.Join(fd.cur, name)
		// Если имя оказалось папкой — входим в неё.
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			fd.navigate(p)
			return
		}
		fd.finish(p)
	}
}

func (fd *FileDialog) finish(path string) {
	fd.eng.CloseModal(fd.dlg)
	if fd.onResult != nil {
		fd.onResult(path, true)
	}
}

// ─── Автоматизация / тестирование ────────────────────────────────────────────

// Dialog возвращает базовый модальный диалог (для темизации/интроспекции).
func (fd *FileDialog) Dialog() *Dialog { return fd.dlg }

// CurrentDir возвращает текущий каталог браузера.
func (fd *FileDialog) CurrentDir() string { return fd.cur }

// VisibleNames возвращает строки текущего списка (папки как «[имя]»).
func (fd *FileDialog) VisibleNames() []string { return fd.list.Items() }

// SetFileName задаёт имя в поле (программно/для автоматизации).
func (fd *FileDialog) SetFileName(name string) {
	fd.nameIn.SetText(name)
	fd.updateWarning()
}

// Activate имитирует двойной клик/Enter по элементу idx (вход в папку и т.п.).
func (fd *FileDialog) Activate(idx int) { fd.activate(idx) }

// ─── Модель файловой системы ─────────────────────────────────────────────────

// readDirEntries читает каталог: папки первыми, затем файлы, по алфавиту.
func readDirEntries(dir string, showHidden bool) []fileEntry {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]fileEntry, 0, len(des))
	for _, de := range des {
		name := de.Name()
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		fe := fileEntry{name: name, dir: de.IsDir()}
		if info, err := de.Info(); err == nil {
			fe.size = info.Size()
			fe.mod = info.ModTime()
		}
		out = append(out, fe)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].dir != out[j].dir {
			return out[i].dir // папки первыми
		}
		return strings.ToLower(out[i].name) < strings.ToLower(out[j].name)
	})
	return out
}

// formatEntry форматирует строку списка: «📁 имя» / «имя  размер  дата».
func formatEntry(e fileEntry) string {
	if e.dir {
		return "[" + e.name + "]"
	}
	return fmt.Sprintf("%s    %s    %s", e.name, humanSize(e.size), e.mod.Format("2006-01-02 15:04"))
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func fileFilterLabel(f FileFilter) string {
	if f.Label != "" && f.Label != "*" {
		if len(f.Exts) > 0 {
			return f.Label + " (" + strings.Join(f.Exts, ", ") + ")"
		}
		return f.Label
	}
	if len(f.Exts) == 0 {
		return "*.*"
	}
	return strings.Join(f.Exts, ", ")
}

// systemRoots возвращает корневые точки: диски (Windows) или «/» (Unix).
// Экспортируется для панели мест приложений; в v1 диалог использует
// только breadcrumb-навигацию и «Вверх».
func systemRoots() []string {
	if runtime.GOOS == "windows" {
		var roots []string
		for c := 'A'; c <= 'Z'; c++ {
			p := string(c) + ":\\"
			if _, err := os.Stat(p); err == nil {
				roots = append(roots, p)
			}
		}
		return roots
	}
	return []string{"/"}
}
