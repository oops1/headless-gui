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
	FileSave                         // сохранение (компактная форма с именем)
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

// FilePlace — дополнительное «место» боковой панели (например, сетевая шара).
type FilePlace struct {
	Label string
	Path  string
}

// FileDialogOptions — параметры файлового диалога.
type FileDialogOptions struct {
	Mode        FileDialogMode
	Title       string       // пустой → локализованный по режиму
	StartDir    string       // начальный каталог (пустой → домашний)
	InitialName string       // предзаполненное имя (для Save)
	Filters     []FileFilter // фильтры расширений (пустой → «Все файлы»)
	ShowHidden  bool         // показывать скрытые (имена с точкой)
	Places      []FilePlace  // дополнительные места в боковой панели
}

// fileEntry — элемент каталога.
type fileEntry struct {
	name string
	dir  bool
	size int64
	mod  time.Time
}

// FileDialog — модальный файловый диалог (Open/Save/Folder).
// Полностью на движке: работает в headless-режиме и показывает файловую
// систему ПРОЦЕССА (в стриминге — сервера). Темизируется и локализуется.
//
// Open/FolderPick — браузер с панелью мест, breadcrumb-путём и списком
// с колонками; Save — компактная форма (путь + имя + предупреждение),
// как в принятых дизайн-мокапах.
type FileDialog struct {
	eng      ModalShower
	dlg      *Dialog
	opts     FileDialogOptions
	onResult func(path string, ok bool)

	cur     string // текущий каталог
	entries []fileEntry
	filter  int // индекс активного фильтра

	crumb   *crumbBar
	places  *placeList // nil в компактном Save
	table   *fileTable // nil в компактном Save
	nameIn  *TextInput // nil в FolderPick
	warnLbl *Label
	warnIco *DialogIcon
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

	fd := &FileDialog{eng: mb.eng, opts: opts, onResult: onResult}
	if opts.Mode == FileSave {
		fd.buildSaveCompact(mb, title)
	} else {
		fd.buildBrowser(mb, title)
	}

	fd.navigate(start)
	fd.updateWarning() // Save: перезапись при предзаполненном имени
	mb.eng.ShowModal(fd.dlg)
	return fd
}

// buildBrowser — полный браузер (Open / FolderPick), мокап dlg_fileopen:
// breadcrumb + ↑/⟳, панель мест, список с колонками, имя+фильтр, кнопки.
func (fd *FileDialog) buildBrowser(mb *MessageBox, title string) {
	const (
		dlgW   = 640
		dlgH   = 460
		padX   = dlgPad
		crumbY = dlgTitleH + 12
		panelY = crumbY + 42
		panelH = 280
	)
	dlg := NewDialog(title, dlgW, dlgH)
	fd.dlg = dlg

	// ── Breadcrumb-путь + «Вверх» + «Обновить» ───────────────────────────
	fd.crumb = newCrumbBar()
	fd.crumb.SetBounds(image.Rect(padX, crumbY, dlgW-padX-112, crumbY+30))
	fd.crumb.OnNavigate = fd.navigate
	dlg.AddChild(fd.crumb)

	upBtn := NewButton("↑")
	upBtn.SetBounds(image.Rect(dlgW-padX-104, crumbY, dlgW-padX-56, crumbY+30))
	upBtn.SetToolTip(Tr("dlg.file.up"))
	upBtn.OnClick = fd.goUp
	dlg.AddChild(upBtn)

	refBtn := NewButton("⟳")
	refBtn.SetBounds(image.Rect(dlgW-padX-48, crumbY, dlgW-padX, crumbY+30))
	refBtn.SetToolTip(Tr("dlg.file.refresh"))
	refBtn.OnClick = fd.reload
	dlg.AddChild(refBtn)

	// ── Панель мест ──────────────────────────────────────────────────────
	fd.places = newPlaceList(buildPlaces(fd.opts.Places))
	fd.places.SetBounds(image.Rect(padX, panelY, padX+140, panelY+panelH))
	fd.places.OnNavigate = fd.navigate
	dlg.AddChild(fd.places)

	// ── Список файлов с колонками ────────────────────────────────────────
	fd.table = newFileTable()
	fd.table.SetBounds(image.Rect(padX+152, panelY, dlgW-padX, panelY+panelH))
	fd.table.OnSelect = fd.selectEntry
	fd.table.OnActivate = fd.activate
	dlg.AddChild(fd.table)

	// ── Имя файла + фильтр (только Open) ─────────────────────────────────
	nameY := panelY + panelH + 12
	var nameLbl *Label
	var filterDD *Dropdown
	if fd.opts.Mode == FileOpen {
		nameLbl = NewLabel(Tr("dlg.file.name"), win10.LabelText)
		nameLbl.FontSize = 11
		nameLbl.SetBounds(image.Rect(padX, nameY+7, padX+42, nameY+25))
		dlg.AddChild(nameLbl)

		fd.nameIn = NewTextInput("")
		fd.nameIn.SetBounds(image.Rect(padX+44, nameY, padX+44+330, nameY+30))
		dlg.AddChild(fd.nameIn)

		filterDD = fd.newFilterDropdown()
		filterDD.SetBounds(image.Rect(dlgW-padX-224, nameY, dlgW-padX, nameY+30))
		dlg.AddChild(filterDD)
	}

	// ── Кнопки ───────────────────────────────────────────────────────────
	okKey := "dlg.open"
	if fd.opts.Mode == FolderPick {
		okKey = "dlg.select"
	}
	okBtn, cancelBtn := fd.addBottomButtons(mb, dlg, dlgW, dlgH, okKey)

	dlg.OnLanguageChange(func() {
		okBtn.SetText(Tr(okKey))
		cancelBtn.SetText(Tr("dlg.cancel"))
		upBtn.SetToolTip(Tr("dlg.file.up"))
		refBtn.SetToolTip(Tr("dlg.file.refresh"))
		if nameLbl != nil {
			nameLbl.SetText(Tr("dlg.file.name"))
		}
	})
}

// buildSaveCompact — компактная форма «Сохранить как» (мокап dlg_filesave):
// путь, имя + фильтр, строка предупреждения о перезаписи, кнопки.
func (fd *FileDialog) buildSaveCompact(mb *MessageBox, title string) {
	const (
		dlgW   = 540
		dlgH   = 232
		padX   = dlgPad
		crumbY = dlgTitleH + 12
	)
	dlg := NewDialog(title, dlgW, dlgH)
	fd.dlg = dlg

	fd.crumb = newCrumbBar()
	fd.crumb.SetBounds(image.Rect(padX, crumbY, dlgW-padX, crumbY+30))
	fd.crumb.OnNavigate = fd.navigate
	dlg.AddChild(fd.crumb)

	lblY := crumbY + 42
	nameLbl := newMutedLabel(Tr("dlg.file.filename"))
	nameLbl.FontSize = 11
	nameLbl.SetBounds(image.Rect(padX, lblY, dlgW-padX, lblY+18))
	dlg.AddChild(nameLbl)

	nameY := lblY + 22
	fd.nameIn = NewTextInput("")
	fd.nameIn.SetText(fd.opts.InitialName)
	fd.nameIn.OnChange = func(string) { fd.updateWarning() }
	fd.nameIn.SetBounds(image.Rect(padX, nameY, dlgW-padX-132, nameY+30))
	dlg.AddChild(fd.nameIn)

	filterDD := fd.newFilterDropdown()
	filterDD.SetBounds(image.Rect(dlgW-padX-120, nameY, dlgW-padX, nameY+30))
	dlg.AddChild(filterDD)

	// Предупреждение о перезаписи: треугольник + приглушённый оранжевый текст.
	warnY := nameY + 42
	fd.warnIco = NewDialogIcon(SeverityWarning)
	fd.warnIco.SetBounds(image.Rect(padX+2, warnY-2, padX+24, warnY+20))
	fd.warnIco.SetVisible(false)
	dlg.AddChild(fd.warnIco)

	fd.warnLbl = NewLabel("", severityColor(SeverityWarning))
	fd.warnLbl.FontSize = 10
	fd.warnLbl.SetBounds(image.Rect(padX+30, warnY+2, dlgW-padX, warnY+18))
	dlg.AddChild(fd.warnLbl)

	okBtn, cancelBtn := fd.addBottomButtons(mb, dlg, dlgW, dlgH, "dlg.save")

	dlg.OnLanguageChange(func() {
		okBtn.SetText(Tr("dlg.save"))
		cancelBtn.SetText(Tr("dlg.cancel"))
		nameLbl.SetText(Tr("dlg.file.filename"))
		fd.updateWarning()
	})
}

// newFilterDropdown создаёт выпадающий список фильтров расширений.
func (fd *FileDialog) newFilterDropdown() *Dropdown {
	items := make([]string, len(fd.opts.Filters))
	for i, f := range fd.opts.Filters {
		items[i] = fileFilterLabel(f)
	}
	dd := NewDropdown(items...)
	dd.OnChange = func(i int, _ string) {
		fd.filter = i
		fd.reload()
		fd.updateWarning()
	}
	return dd
}

// addBottomButtons добавляет пару OK/Отмена в правый нижний угол.
func (fd *FileDialog) addBottomButtons(mb *MessageBox, dlg *Dialog, dlgW, dlgH int, okKey string) (okBtn, cancelBtn *Button) {
	const (
		btnH   = 30
		btnGap = 8
		padX   = dlgPad
		btnPad = 12
	)
	btnY := dlgH - btnPad - btnH
	okW := mbBtnWidth(Tr(okKey))
	cancelW := mbBtnWidth(Tr("dlg.cancel"))

	okBtn = trBtn(okKey, true)
	okBtn.SetBounds(image.Rect(dlgW-padX-cancelW-btnGap-okW, btnY, dlgW-padX-cancelW-btnGap, btnY+btnH))
	okBtn.OnClick = fd.confirm
	dlg.AddChild(okBtn)

	cancelBtn = trBtn("dlg.cancel", false)
	cancelBtn.SetBounds(image.Rect(dlgW-padX-cancelW, btnY, dlgW-padX, btnY+btnH))
	cancelBtn.OnClick = func() {
		mb.eng.CloseModal(dlg)
		if fd.onResult != nil {
			fd.onResult("", false)
		}
	}
	dlg.AddChild(cancelBtn)

	dlg.DefaultAction = fd.confirm
	dlg.CancelAction = func() {
		if fd.onResult != nil {
			fd.onResult("", false)
		}
	}
	return okBtn, cancelBtn
}

// buildPlaces собирает панель мест: пользовательские, домашняя,
// стандартные подпапки и корни дисков.
func buildPlaces(extra []FilePlace) []placeItem {
	var items []placeItem
	for _, p := range extra {
		items = append(items, placeItem{label: p.Label, path: p.Path})
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		items = append(items, placeItem{localeKey: "dlg.place.home", path: home})
		for _, sub := range []struct{ key, name string }{
			{"dlg.place.docs", "Documents"},
			{"dlg.place.downloads", "Downloads"},
		} {
			p := filepath.Join(home, sub.name)
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				items = append(items, placeItem{localeKey: sub.key, path: p})
			}
		}
	}
	for _, root := range systemRoots() {
		label := root
		if root == "/" {
			items = append(items, placeItem{localeKey: "dlg.place.root", path: root})
			continue
		}
		items = append(items, placeItem{label: label, path: root})
	}
	return items
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
	fd.crumb.SetPath(fd.cur)
	if fd.places != nil {
		fd.places.SetCurrent(fd.cur)
	}
	if fd.table == nil {
		return // компактный Save: списка нет
	}

	all := readDirEntries(fd.cur, fd.opts.ShowHidden)
	f := fd.opts.Filters[fd.filter]
	kept := make([]fileEntry, 0, len(all))
	for _, e := range all {
		if !e.dir {
			if fd.opts.Mode == FolderPick {
				continue // в режиме папок файлы не показываем
			}
			if !f.match(e.name) {
				continue
			}
		}
		kept = append(kept, e)
	}
	fd.entries = kept
	fd.table.SetEntries(kept)
}

// selectEntry — одиночный клик: для файла кладём имя в поле.
func (fd *FileDialog) selectEntry(idx int) {
	if idx < 0 || idx >= len(fd.entries) {
		return
	}
	e := fd.entries[idx]
	if !e.dir && fd.nameIn != nil {
		fd.nameIn.SetText(e.name)
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
	if fd.nameIn != nil {
		fd.nameIn.SetText(e.name)
	}
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
	if fd.opts.Mode != FileSave || fd.warnLbl == nil {
		return
	}
	name := strings.TrimSpace(fd.nameIn.GetText())
	warn := false
	if name != "" {
		if st, err := os.Stat(filepath.Join(fd.cur, name)); err == nil && !st.IsDir() {
			warn = true
		}
	}
	if warn {
		fd.warnLbl.SetText(Tr("dlg.file.overwrite"))
	} else {
		fd.warnLbl.SetText("")
	}
	fd.warnIco.SetVisible(warn)
	fd.dlg.Invalidate()
}

// confirm подтверждает выбор согласно режиму.
func (fd *FileDialog) confirm() {
	switch fd.opts.Mode {
	case FolderPick:
		// Выделенная папка приоритетнее текущего каталога.
		if fd.table != nil {
			if i := fd.table.Selected(); i >= 0 && i < len(fd.entries) {
				fd.finish(filepath.Join(fd.cur, fd.entries[i].name))
				return
			}
		}
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
			// Без имени: Enter по выделенной папке — вход в неё.
			if fd.table != nil {
				if i := fd.table.Selected(); i >= 0 && i < len(fd.entries) && fd.entries[i].dir {
					fd.activate(i)
				}
			}
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

// VisibleNames возвращает имена текущего списка (папки как «[имя]»).
// В компактном Save возвращает nil.
func (fd *FileDialog) VisibleNames() []string {
	if fd.table == nil {
		return nil
	}
	out := make([]string, len(fd.entries))
	for i, e := range fd.entries {
		if e.dir {
			out[i] = "[" + e.name + "]"
		} else {
			out[i] = e.name
		}
	}
	return out
}

// SetFileName задаёт имя в поле (программно/для автоматизации).
func (fd *FileDialog) SetFileName(name string) {
	if fd.nameIn == nil {
		return
	}
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
		if !showHidden && (strings.HasPrefix(name, ".") || isHiddenFSEntry(dir, name)) {
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
