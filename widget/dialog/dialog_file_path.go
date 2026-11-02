package dialog

// dialog_file_path.go — путь, введённый руками, в файловом диалоге.
//
// Строка пути (crumbBar) умеет только переходить к родительским папкам. Щелчок
// по её пустому месту, как в Проводнике, кладёт на её место поле ввода: Enter
// переходит по пути, Escape и потеря фокуса возвращают строку пути. Здесь же —
// разбор поля «Имя файла» в Save: путь от текущей папки, расширение выбранного
// фильтра и подтверждение замены существующего файла.

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/oops1/headless-gui/v4/internal/focusreq"
	"github.com/oops1/headless-gui/v4/widget/base"
	"github.com/oops1/headless-gui/v4/widget/text"
)

// pathInput — поле ввода пути поверх строки пути. Потерю фокуса (щелчок мимо,
// Tab) диалог узнаёт из SetFocused: своего события у TextInput нет.
type pathInput struct {
	*text.TextInput
	onBlur func()
}

func (p *pathInput) SetFocused(focused bool) {
	p.TextInput.SetFocused(focused)
	if !focused && p.onBlur != nil {
		p.onBlur()
	}
}

// addPathInput кладёт скрытое поле ввода пути на место строки пути и включает
// переход в него щелчком по пустому месту строки.
func (fd *FileDialog) addPathInput(dlg *Dialog) {
	in := &pathInput{TextInput: text.NewTextInput("")}
	in.SetBounds(fd.crumb.Bounds())
	in.SetVisible(false)
	in.SetTabIndex(-1) // скрытое поле — не остановка Tab
	in.onBlur = func() { fd.endPathEdit(false) }
	dlg.AddChild(in)
	fd.pathIn = in
	fd.crumb.OnEdit = fd.beginPathEdit

	// Escape закрывает модалку раньше, чем клавиша дойдёт до поля: во время
	// ввода он отменяет ввод, а диалог остаётся.
	dlg.OnClosing = func() bool {
		if fd.pathEditing {
			fd.endPathEdit(true)
			return false
		}
		return true
	}
}

// beginPathEdit показывает поле ввода с текущей папкой, выделенной целиком:
// набранный путь заменяет её.
func (fd *FileDialog) beginPathEdit() {
	if fd.pathIn == nil || fd.pathEditing {
		return
	}
	fd.pathEditing = true
	fd.pathIn.SetText(fd.cur)
	fd.pathIn.AccessSetSelection(0, len([]rune(fd.cur)))
	fd.pathIn.SetValidationError("")
	fd.crumb.SetVisible(false)
	fd.pathIn.SetVisible(true)
	focusreq.Request(fd.pathIn)
	fd.dlg.Invalidate()
}

// endPathEdit возвращает строку пути. returnFocus — вернуть фокус тому, у
// кого его взяло поле (Enter, Escape); при потере фокуса он уже ушёл.
func (fd *FileDialog) endPathEdit(returnFocus bool) {
	if !fd.pathEditing {
		return
	}
	fd.pathEditing = false
	fd.pathIn.SetVisible(false)
	fd.crumb.SetVisible(true)
	if returnFocus {
		focusreq.Return(fd.pathIn)
	}
	fd.dlg.Invalidate()
}

// commitPathEdit переходит по введённому пути: папка — в неё; файл — в его
// папку, а имя — в поле «Имя файла». Путь, которого нет или который вне
// разрешённых корней, оставляет поле открытым с ошибкой.
func (fd *FileDialog) commitPathEdit() {
	p := fd.resolvePath(fd.pathIn.GetText())
	if p == "" {
		fd.endPathEdit(true)
		return
	}
	st, err := os.Stat(p)
	dir := p
	if err == nil && !st.IsDir() {
		dir = filepath.Dir(p)
	}
	if err != nil || !withinRoots(fd.roots, dir) {
		fd.pathIn.SetValidationError(base.Tr("dlg.file.badpath"))
		return
	}
	fd.endPathEdit(true)
	fd.navigate(dir)
	if !st.IsDir() && fd.nameIn != nil {
		fd.SetFileName(filepath.Base(p))
	}
}

// resolvePath — путь из поля ввода: абсолютный как есть, относительный — от
// текущей папки. Кавычки по краям (путь, скопированный из Проводника)
// снимаются; «D:» на Windows — корень диска, а не текущая папка на нём.
func (fd *FileDialog) resolvePath(s string) string {
	s = strings.Trim(strings.TrimSpace(s), `"`)
	if s == "" {
		return ""
	}
	if vol := filepath.VolumeName(s); vol != "" && vol == s {
		s += string(filepath.Separator)
	}
	if !filepath.IsAbs(s) {
		s = filepath.Join(fd.cur, s)
	}
	return filepath.Clean(s)
}

// saveTarget разбирает поле «Имя файла» в Save. isDir — введено имя
// существующей папки (в неё переходят). Иначе path — файл; имени без
// расширения достаётся расширение выбранного фильтра. ok=false — имя пустое.
func (fd *FileDialog) saveTarget() (path string, isDir, ok bool) {
	if fd.nameIn == nil {
		return "", false, false
	}
	name := strings.TrimSpace(fd.nameIn.GetText())
	p := fd.resolvePath(name)
	if p == "" {
		return "", false, false
	}
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return p, true, true
	}
	if filepath.Ext(name) == "" {
		p += fd.filterExt()
	}
	return p, false, true
}

// filterExt — первое расширение выбранного фильтра («.rdp»); у фильтра «все
// файлы» его нет.
func (fd *FileDialog) filterExt() string {
	if fd.filter < 0 || fd.filter >= len(fd.opts.Filters) {
		return ""
	}
	for _, e := range fd.opts.Filters[fd.filter].Exts {
		if e = normExt(e); e != "" {
			return e
		}
	}
	return ""
}

// normExt приводит расширение фильтра к виду «.rdp» («*.rdp», «rdp» — тоже);
// у «*», «.*» и пустого — "".
func normExt(e string) string {
	e = strings.TrimPrefix(strings.TrimSpace(e), "*")
	if e == "" || e == "." || e == ".*" {
		return ""
	}
	if !strings.HasPrefix(e, ".") {
		e = "." + e
	}
	return e
}

// hasExt — есть ли расширение ext (без учёта регистра) у какого-нибудь из
// фильтров.
func hasExt(filters []FileFilter, ext string) bool {
	for _, f := range filters {
		for _, e := range f.Exts {
			if n := normExt(e); n != "" && strings.EqualFold(n, ext) {
				return true
			}
		}
	}
	return false
}

// matchNameToFilter — смена фильтра в Save меняет расширение в поле имени на
// расширение нового фильтра, как в Windows: «cert.cer» → «cert.pem». Меняется
// только расширение, которое есть в списке фильтров; имя без расширения, с
// чужим расширением («отчёт.v2») или уже подходящее новому фильтру остаётся,
// фильтр «все файлы» имя не трогает. Предупреждение обновляет вызывающий.
func (fd *FileDialog) matchNameToFilter() {
	if fd.opts.Mode != FileSave || fd.nameIn == nil {
		return
	}
	name := strings.TrimSpace(fd.nameIn.GetText())
	ext := filepath.Ext(name)
	newExt := fd.filterExt() // "" — «все файлы» или номер вне списка
	// Имя без расширения отсеивает hasExt: пустого расширения у фильтров нет.
	if newExt == "" || !hasExt(fd.opts.Filters, ext) ||
		hasExt(fd.opts.Filters[fd.filter:fd.filter+1], ext) {
		return
	}
	fd.nameIn.SetText(strings.TrimSuffix(name, ext) + newExt)
}

// confirmReplace спрашивает, заменить ли существующий файл: «Да» — выбор
// сделан, «Нет» — диалог остаётся открытым.
func (fd *FileDialog) confirmReplace(p string) {
	NewMessageBox(fd.eng).ShowSeverity(base.Tr("dlg.file.replace.title"),
		base.Trf("dlg.file.replace", filepath.Base(p)),
		SeverityWarning, MBYesNo, func(r MessageBoxResult) {
			if r == MBResultYes {
				fd.finish(p)
			}
		})
}
