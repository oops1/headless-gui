package widget

import (
	"image"
	"path/filepath"
	"testing"
)

// openChoiceDialog показывает диалог нужного режима без движка (stubModal).
// Каталог — временный: путь результата проверяем целиком.
func openChoiceDialog(t *testing.T, mode FileDialogMode, choices []FileDialogChoice, onResult func(string, bool)) (*FileDialog, string) {
	t.Helper()
	dir := t.TempDir()
	mb := NewMessageBox(&stubModal{})
	opts := FileDialogOptions{StartDir: dir, InitialName: "a.txt", Choices: choices}
	switch mode {
	case FileSave:
		return mb.ShowSaveFile(opts, onResult), dir
	case FolderPick:
		return mb.ShowPickFolder(opts, onResult), dir
	default:
		return mb.ShowOpenFile(opts, onResult), dir
	}
}

// dialogDropdowns — все выпадающие списки диалога по порядку добавления.
func dialogDropdowns(fd *FileDialog) []*Dropdown {
	var out []*Dropdown
	for _, c := range fd.Dialog().Children() {
		if dd, ok := c.(*Dropdown); ok {
			out = append(out, dd)
		}
	}
	return out
}

// dialogButtons — кнопки диалога (без ✕ заголовка: он другого типа).
func dialogButtons(fd *FileDialog) []*Button {
	var out []*Button
	for _, c := range fd.Dialog().Children() {
		if b, ok := c.(*Button); ok {
			out = append(out, b)
		}
	}
	return out
}

var allFileModes = []struct {
	name string
	mode FileDialogMode
	h    int // высота диалога без дополнительных списков
}{
	{"Open", FileOpen, 460},
	{"Save", FileSave, 232},
	{"Folder", FolderPick, 460},
}

// Список появляется в обоих видах диалога: полном (Open, FolderPick) и
// компактном (Save), с нужными вариантами и выбранным по умолчанию.
func TestFileDialogChoices_AppearsAndDefault(t *testing.T) {
	for _, m := range allFileModes {
		t.Run(m.name, func(t *testing.T) {
			base, _ := openChoiceDialog(t, m.mode, nil, nil)
			fd, _ := openChoiceDialog(t, m.mode, []FileDialogChoice{{
				ID: "enc", Label: "Кодировка:", Options: []string{"UTF-8", "UTF-16", "CP1251"}, Default: 2,
			}}, nil)

			if n, want := len(dialogDropdowns(fd)), len(dialogDropdowns(base))+1; n != want {
				t.Fatalf("списков в диалоге %d, ожидалось %d (+1 к базовым)", n, want)
			}
			idx, ok := fd.Choice("enc")
			if !ok || idx != 2 {
				t.Fatalf("Choice(enc) = %d, %v; ожидалось значение по умолчанию 2", idx, ok)
			}
			if txt, _ := fd.ChoiceText("enc"); txt != "CP1251" {
				t.Fatalf("ChoiceText = %q, ожидалось CP1251", txt)
			}
		})
	}
}

// Default вне диапазона не должен ни паниковать, ни выбирать «пустоту».
func TestFileDialogChoices_DefaultClamped(t *testing.T) {
	for _, def := range []int{-1, 5} {
		fd, _ := openChoiceDialog(t, FileOpen, []FileDialogChoice{{
			ID: "e", Options: []string{"a", "b"}, Default: def,
		}}, nil)
		if idx, ok := fd.Choice("e"); !ok || idx != 0 {
			t.Fatalf("Default=%d: Choice = %d, %v; ожидалось 0", def, idx, ok)
		}
	}
}

// Выбор доступен в колбэке результата — при открытии, сохранении и выборе
// папки; при отмене колбэк тоже может его прочитать.
func TestFileDialogChoices_ChoiceInResultCallback(t *testing.T) {
	for _, m := range allFileModes {
		t.Run(m.name, func(t *testing.T) {
			var fd *FileDialog
			var gotPath string
			var gotOK bool
			gotIdx, gotFound := -1, false
			fd, dir := openChoiceDialog(t, m.mode, []FileDialogChoice{EncodingChoice()}, func(p string, ok bool) {
				gotPath, gotOK = p, ok
				gotIdx, gotFound = fd.Choice(EncodingChoiceID)
			})
			if !fd.SetChoice(EncodingChoiceID, EncodingWindows1251) {
				t.Fatal("SetChoice вернул false")
			}
			if m.mode != FolderPick {
				fd.SetFileName("a.txt")
			}
			fd.confirm()

			if !gotOK {
				t.Fatal("колбэк результата не вызван с ok=true")
			}
			want := dir
			if m.mode != FolderPick {
				want = filepath.Join(dir, "a.txt")
			}
			if gotPath != want {
				t.Fatalf("путь %q, ожидался %q", gotPath, want)
			}
			if !gotFound || gotIdx != EncodingWindows1251 {
				t.Fatalf("в колбэке Choice = %d, %v; ожидалось Windows-1251 (%d)", gotIdx, gotFound, EncodingWindows1251)
			}
		})
	}
}

// Выбор мышью/клавиатурой (сам Dropdown), а не SetChoice, тоже доходит до
// приложения: Choice читает состояние реального виджета.
func TestFileDialogChoices_UserSelectionVisible(t *testing.T) {
	fd, _ := openChoiceDialog(t, FileSave, []FileDialogChoice{EncodingChoice()}, nil)
	dds := dialogDropdowns(fd)
	dds[len(dds)-1].SetSelected(EncodingUTF16BE) // список приложения добавлен последним
	if idx, _ := fd.Choice(EncodingChoiceID); idx != EncodingUTF16BE {
		t.Fatalf("Choice = %d, ожидалось %d", idx, EncodingUTF16BE)
	}
}

// Неизвестный ID, список без вариантов и вне диапазона — аккуратные отказы.
func TestFileDialogChoices_UnknownAndInvalid(t *testing.T) {
	fd, _ := openChoiceDialog(t, FileOpen, []FileDialogChoice{
		{ID: "empty", Label: "Пусто"},
		{ID: "e", Options: []string{"a", "b"}},
	}, nil)
	if _, ok := fd.Choice("nope"); ok {
		t.Fatal("неизвестный ID вернул ok=true")
	}
	if _, ok := fd.Choice("empty"); ok {
		t.Fatal("список без вариантов не должен появляться в диалоге")
	}
	if fd.SetChoice("e", 2) || fd.SetChoice("e", -1) || fd.SetChoice("nope", 0) {
		t.Fatal("SetChoice принял недопустимые аргументы")
	}
	if _, ok := fd.ChoiceText("nope"); ok {
		t.Fatal("ChoiceText неизвестного ID вернул ok=true")
	}
}

// Без Choices диалог ровно прежний: тот же размер, тот же набор виджетов,
// никаких списков приложения. Размеры 460/232 — исходные, зашитые раньше.
func TestFileDialogChoices_EmptyKeepsDialog(t *testing.T) {
	for _, m := range allFileModes {
		t.Run(m.name, func(t *testing.T) {
			for _, choices := range [][]FileDialogChoice{nil, {}, {{ID: "x"}}} { // последний — отброшен как пустой
				fd, _ := openChoiceDialog(t, m.mode, choices, nil)
				if got := fd.Dialog().Bounds().Dy(); got != m.h {
					t.Fatalf("choices=%v: высота диалога %d, исходная %d", choices, got, m.h)
				}
				if _, ok := fd.Choice("x"); ok {
					t.Fatal("Choice нашёл список, которого нет")
				}
			}
			// Число виджетов: сравниваем с диалогом, собранным теми же вызовами
			// без поля — различие дали бы только лишние виджеты списков.
			a, _ := openChoiceDialog(t, m.mode, nil, nil)
			b, _ := openChoiceDialog(t, m.mode, []FileDialogChoice{}, nil)
			if x, y := len(a.Dialog().Children()), len(b.Dialog().Children()); x != y {
				t.Fatalf("число виджетов %d и %d", x, y)
			}
		})
	}
}

// Раскладка: диалог вырос ровно на ряд, списки внутри диалога, ниже имени и
// фильтра, выше кнопок и не пересекаются ни с чем.
func TestFileDialogChoices_Layout(t *testing.T) {
	for _, m := range allFileModes {
		t.Run(m.name, func(t *testing.T) {
			base, _ := openChoiceDialog(t, m.mode, nil, nil)
			fd, _ := openChoiceDialog(t, m.mode, []FileDialogChoice{EncodingChoice()}, nil)

			wantGrow := choiceRowH
			if m.mode == FolderPick {
				wantGrow = 0 // первый ряд встаёт в пустой ряд «имя + фильтр»
			}
			if got := fd.Dialog().Bounds().Dy() - base.Dialog().Bounds().Dy(); got != wantGrow {
				t.Fatalf("диалог вырос на %d, ожидалось %d", got, wantGrow)
			}

			dlgR := fd.Dialog().Bounds()
			dds := dialogDropdowns(fd)
			ours := dds[len(dds)-1].Bounds()
			if !ours.In(dlgR) {
				t.Fatalf("список %v вне диалога %v", ours, dlgR)
			}
			// Подпись списка — Label, добавленный сразу перед ним.
			var lblR image.Rectangle
			ch := fd.Dialog().Children()
			for i, c := range ch {
				if c == Widget(dds[len(dds)-1]) {
					lblR = ch[i-1].Bounds()
				}
			}
			if lblR.Empty() || lblR.Overlaps(ours) {
				t.Fatalf("подпись %v пуста или налезает на список %v", lblR, ours)
			}

			for _, b := range dialogButtons(fd) {
				if br := b.Bounds(); br.Overlaps(ours) || br.Overlaps(lblR) {
					t.Fatalf("кнопка %v налезает на список %v / подпись %v", br, ours, lblR)
				}
			}
			// Ни с одним из прежних виджетов диалога (кроме содержимого, которое
			// рисуется целиком вне ряда списков) пересечений нет.
			for _, c := range ch {
				if c == Widget(dds[len(dds)-1]) || c.Bounds() == lblR {
					continue
				}
				if r := c.Bounds(); r.Overlaps(ours) || r.Overlaps(lblR) {
					t.Fatalf("%T %v пересекается со списком %v / подписью %v", c, r, ours, lblR)
				}
			}
		})
	}
}

// Много списков не вылезают за диалог, а переносятся на следующие ряды,
// увеличивая высоту ровно на число рядов.
func TestFileDialogChoices_ManyWrapRows(t *testing.T) {
	var list []FileDialogChoice
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		list = append(list, FileDialogChoice{ID: id, Label: "Подпись " + id + ":", Options: []string{"Первый вариант", "Второй длинный вариант"}})
	}
	fd, _ := openChoiceDialog(t, FileSave, list, nil)
	plan := planChoices(normChoices(list), 540-2*dlgPad)
	if len(plan.rows) < 2 {
		t.Fatalf("шесть списков уместились в %d ряд — перенос не проверен", len(plan.rows))
	}
	if got, want := fd.Dialog().Bounds().Dy(), 232+plan.height(); got != want {
		t.Fatalf("высота %d, ожидалось %d", got, want)
	}
	dlgR := fd.Dialog().Bounds()
	dds := dialogDropdowns(fd)[1:] // первый — фильтр типов
	if len(dds) != len(list) {
		t.Fatalf("списков %d, ожидалось %d", len(dds), len(list))
	}
	for i, dd := range dds {
		if !dd.Bounds().In(dlgR) {
			t.Fatalf("список %d %v вне диалога %v", i, dd.Bounds(), dlgR)
		}
		for j := i + 1; j < len(dds); j++ {
			if dd.Bounds().Overlaps(dds[j].Bounds()) {
				t.Fatalf("списки %d и %d пересекаются", i, j)
			}
		}
	}
}

// Готовый набор кодировок: порядок вариантов совпадает с константами.
func TestEncodingChoice_MatchesConstants(t *testing.T) {
	c := EncodingChoice()
	if c.ID != EncodingChoiceID || c.LabelKey == "" || c.Default != EncodingUTF8 {
		t.Fatalf("EncodingChoice() = %+v", c)
	}
	want := map[int]string{
		EncodingUTF8: "UTF-8", EncodingUTF8BOM: "UTF-8 (BOM)", EncodingUTF16LE: "UTF-16 LE",
		EncodingUTF16BE: "UTF-16 BE", EncodingWindows1251: "Windows-1251",
	}
	if len(c.Options) != len(want) {
		t.Fatalf("вариантов %d, ожидалось %d", len(c.Options), len(want))
	}
	for i, w := range want {
		if c.Options[i] != w {
			t.Fatalf("вариант %d = %q, ожидалось %q", i, c.Options[i], w)
		}
	}
	// Подпись переведена и на русском, и на английском: ключ заведён.
	for _, lang := range []string{"EN", "RU"} {
		if _, ok := Translation(lang, c.LabelKey); !ok {
			t.Fatalf("нет перевода %s для %s", c.LabelKey, lang)
		}
	}
}
