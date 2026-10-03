package widget

import "image"

// dialog_file_choices.go — дополнительные выпадающие списки файлового диалога.
//
// Состав файлового диалога был зашит: путь, список, имя, фильтр типов и
// кнопки. Но у настоящих диалогов бывает то, что знает только приложение:
// в «Открыть»/«Сохранить как» Блокнота Windows справа от типа файла стоит
// список «Кодировка» — в какой читать и в какой записать. Добавить его было
// нечем, и приложению оставалось писать свой файловый диалог целиком (путь,
// места, таблицу, предупреждение о перезаписи) ради одного выпадающего списка.
//
// Здесь — общий механизм, а не поле «кодировка»: приложение описывает списки
// (FileDialogChoice), диалог их рисует и запоминает выбор, а приложение
// забирает его вместе с путём. Что значит выбранный вариант, движок не знает
// и знать не должен: преобразования кодировок, как и само чтение и запись
// файлов, остаются за приложением.

// FileDialogChoice — дополнительный выпадающий список в файловом диалоге.
//
// Выбор приложение получает у самого диалога: колбэк результата
// func(path string, ok bool) менять нельзя (сломались бы все вызывающие), и
// он по-прежнему получает только путь. Зато ShowOpenFile/ShowSaveFile/
// ShowPickFolder возвращают *FileDialog, а у него есть Choice(id) — индекс
// выбранного варианта. Значения живут в диалоге и после его закрытия, поэтому
// к ним можно обратиться прямо из колбэка результата:
//
//	var fd *widget.FileDialog
//	fd = mb.ShowSaveFile(widget.FileDialogOptions{
//		Choices: []widget.FileDialogChoice{widget.EncodingChoice()},
//	}, func(path string, ok bool) {
//		if !ok {
//			return
//		}
//		enc, _ := fd.Choice(widget.EncodingChoiceID)
//		save(path, enc)
//	})
//
// (var fd объявляется отдельно, иначе колбэк не может сослаться на fd — в Go
// переменная не видна в собственном инициализаторе.) Колбэк вызывается после
// выбора пользователя, когда fd уже присвоен: ShowModal не вызывает его
// синхронно.
type FileDialogChoice struct {
	ID      string   // ключ, по которому приложение получит выбор (Choice)
	Label   string   // подпись слева от списка
	Options []string // варианты; список без вариантов в диалоге не показывается
	Default int      // индекс варианта, выбранного изначально (вне диапазона → 0)

	// LabelKey — ключ локализации подписи (Tr). Если задан, он важнее Label и
	// подпись переключается вместе с языком, пока диалог открыт. Без него
	// подпись остаётся такой, какой её задало приложение: оно само отвечает
	// за её язык. Варианты Options не переводятся никогда — приложение
	// передаёт их уже готовыми (кодировки, например, одинаковы на всех языках).
	LabelKey string
}

// Геометрия рядов дополнительных списков.
const (
	choiceRowH   = 38 // шаг ряда: 30 — высота списка, 8 — зазор до следующего ряда
	choiceCtlH   = 30 // высота выпадающего списка (как у фильтра типов)
	choiceLblGap = 6  // зазор между подписью и её списком
	choiceColGap = 14 // зазор между парами «подпись + список» в одном ряду
	choiceMinDD  = 96 // ширина списка не меньше — иначе он выглядит как кнопка
	choiceMaxDD  = 240
)

// choiceCtl — построенный в диалоге список: по нему отвечает Choice.
type choiceCtl struct {
	id       string
	labelKey string
	lbl      *Label
	dd       *Dropdown
}

// choiceSlot — пара «подпись + список» с рассчитанными ширинами.
type choiceSlot struct {
	ch        FileDialogChoice
	lblW, ddW int
}

// choicePlan — раскладка списков по рядам.
type choicePlan struct{ rows [][]choiceSlot }

// height — на сколько диалог выше, чем без списков (0 при пустой раскладке).
func (p choicePlan) height() int { return len(p.rows) * choiceRowH }

// choiceLabel — текущая подпись списка: ключ локализации важнее готового Label.
func choiceLabel(ch FileDialogChoice) string {
	if ch.LabelKey != "" {
		return Tr(ch.LabelKey)
	}
	return ch.Label
}

// normChoices готовит списки приложения к показу и возвращает копию: выбрасывает
// списки без вариантов (пустой выпадающий список ни к чему — выбирать нечего,
// а Default некуда указывать) и приводит Default в диапазон. Копия нужна,
// чтобы не менять приложению срезы, которые оно может переиспользовать для
// следующего диалога.
func normChoices(in []FileDialogChoice) []FileDialogChoice {
	var out []FileDialogChoice
	for _, c := range in {
		if len(c.Options) == 0 {
			continue
		}
		c.Options = append([]string(nil), c.Options...)
		if c.Default < 0 || c.Default >= len(c.Options) {
			c.Default = 0
		}
		out = append(out, c)
	}
	return out
}

// planChoices раскладывает списки по рядам шириной avail пикселей.
//
// Ряд заполняется слева направо, пока пары помещаются; не поместилась —
// начинается следующий ряд. Ширина списка — по самому длинному варианту:
// фиксированная обрезала бы «Windows-1251» в узком и раздувала бы «UTF-8» в
// широком. В типичном случае (один список «Кодировка») ряд один.
func planChoices(choices []FileDialogChoice, avail int) choicePlan {
	var p choicePlan
	var row []choiceSlot
	used := 0
	sz := fontSizeOrDefault(0)
	for _, c := range choices {
		// Подпись рисуется кеглем 11, как остальные подписи диалога; +4 — её
		// внутренние отступы (PaddingX Label равен 2 с каждой стороны).
		lblW := MeasureUIText(choiceLabel(c), 11) + 4
		ddW := choiceMinDD
		for _, o := range c.Options {
			// Пункт + отступ текста слева + место под стрелку справа.
			if w := MeasureUIText(o, sz) + 6 + 26; w > ddW {
				ddW = w
			}
		}
		if ddW > choiceMaxDD {
			ddW = choiceMaxDD
		}
		// Одна пара шире всей строки (очень длинная подпись) — ужимаем
		// список, но не подпись: без подписи непонятно, что выбирают.
		if over := lblW + choiceLblGap + ddW - avail; over > 0 {
			ddW -= over
			if ddW < choiceMinDD/2 {
				ddW = choiceMinDD / 2
			}
		}
		w := lblW + choiceLblGap + ddW
		if len(row) > 0 && used+choiceColGap+w > avail {
			p.rows = append(p.rows, row)
			row, used = nil, 0
		}
		if len(row) > 0 {
			used += choiceColGap
		}
		row = append(row, choiceSlot{ch: c, lblW: lblW, ddW: ddW})
		used += w
	}
	if len(row) > 0 {
		p.rows = append(p.rows, row)
	}
	return p
}

// addChoices ставит списки в диалог, начиная с вертикали y0.
//
// Каждый ряд выравнивается по ПРАВОМУ краю (как фильтр типов выше него):
// единственный список «Кодировка» оказывается под фильтром и сливается с ним
// в один столбец, а не висит отдельно у левого края.
func (fd *FileDialog) addChoices(dlg *Dialog, dlgW, y0 int, plan choicePlan) {
	hasKey := false
	for r, row := range plan.rows {
		total := 0
		for i, s := range row {
			if i > 0 {
				total += choiceColGap
			}
			total += s.lblW + choiceLblGap + s.ddW
		}
		x := dlgW - dlgPad - total
		y := y0 + r*choiceRowH
		for _, s := range row {
			lbl := NewLabel(choiceLabel(s.ch), win10.LabelText)
			lbl.FontSize = 11
			lbl.SetBounds(image.Rect(x, y+7, x+s.lblW, y+25))
			dlg.AddChild(lbl)
			x += s.lblW + choiceLblGap

			dd := NewDropdown(s.ch.Options...)
			// SetSelected помечает выбор «сделанным» — для диалога это верно:
			// у списка всегда есть выбранный вариант, пусть и по умолчанию.
			dd.SetSelected(s.ch.Default)
			dd.SetBounds(image.Rect(x, y, x+s.ddW, y+choiceCtlH))
			dlg.AddChild(dd)
			x += s.ddW + choiceColGap

			fd.choices = append(fd.choices, &choiceCtl{id: s.ch.ID, labelKey: s.ch.LabelKey, lbl: lbl, dd: dd})
			hasKey = hasKey || s.ch.LabelKey != ""
		}
	}
	if !hasKey {
		return
	}
	// Подпись по ключу меняется вместе с языком. Ширина при этом остаётся
	// посчитанной для языка на момент показа: перекладывать открытый диалог
	// из-за смены языка не стоит усилий.
	dlg.OnLanguageChange(func() {
		for _, c := range fd.choices {
			if c.labelKey != "" {
				c.lbl.SetText(Tr(c.labelKey))
			}
		}
	})
}

// choiceByID находит список по ключу (первый при повторе ID).
func (fd *FileDialog) choiceByID(id string) *choiceCtl {
	for _, c := range fd.choices {
		if c.id == id {
			return c
		}
	}
	return nil
}

// Choice возвращает индекс варианта, выбранного в дополнительном списке id
// (FileDialogOptions.Choices). Второе значение — false, если такого списка в
// диалоге нет (неизвестный ID или список без вариантов отброшен).
//
// Работает и после закрытия диалога, в том числе при отмене: значение — это
// то, что стояло в списке в момент закрытия; приложение, которому выбор нужен
// только при успехе, смотрит на ok в колбэке результата. Потокобезопасно.
// При повторяющихся ID отвечает первый список.
func (fd *FileDialog) Choice(id string) (int, bool) {
	c := fd.choiceByID(id)
	if c == nil {
		return 0, false
	}
	return c.dd.Selected(), true
}

// ChoiceText возвращает текст выбранного варианта списка id — ровно ту строку,
// что приложение передало в Options.
func (fd *FileDialog) ChoiceText(id string) (string, bool) {
	c := fd.choiceByID(id)
	if c == nil {
		return "", false
	}
	return c.dd.SelectedText(), true
}

// SetChoice выбирает вариант idx в списке id (программно/для автоматизации,
// как SetFileName). false — списка нет или idx вне диапазона.
func (fd *FileDialog) SetChoice(id string, idx int) bool {
	c := fd.choiceByID(id)
	if c == nil || idx < 0 || idx >= len(c.dd.Items()) {
		return false
	}
	c.dd.SetSelected(idx)
	return true
}

// ─── Готовый набор: кодировки ────────────────────────────────────────────────

// EncodingChoiceID — ключ списка кодировок из EncodingChoice.
const EncodingChoiceID = "encoding"

// Индексы вариантов EncodingChoice — их возвращает FileDialog.Choice. Порядок
// закреплён: приложение может сравнивать с константами, а не с номерами.
const (
	EncodingUTF8        = iota // UTF-8 без BOM (выбран по умолчанию)
	EncodingUTF8BOM            // UTF-8 с BOM
	EncodingUTF16LE            // UTF-16 little-endian
	EncodingUTF16BE            // UTF-16 big-endian
	EncodingWindows1251        // Windows-1251
)

// EncodingChoice — готовый список «Кодировка», как в Блокноте Windows: при
// открытии — в какой кодировке читать файл, при сохранении — в какой записать.
//
// Движок только спрашивает: он НЕ читает и не пишет файл, не преобразует
// текст и не определяет кодировку по содержимому — это дело приложения,
// которое смотрит на Choice(EncodingChoiceID) и сравнивает с константами
// Encoding*. Подпись переводится (ключ dlg.file.encoding); названия вариантов
// — технические имена, они одинаковы на всех языках, поэтому не переводятся.
// Нужны другие кодировки — дополните Options в КОНЕЦ, чтобы константы остались
// верными.
func EncodingChoice() FileDialogChoice {
	return FileDialogChoice{
		ID:       EncodingChoiceID,
		LabelKey: "dlg.file.encoding",
		Options:  []string{"UTF-8", "UTF-8 (BOM)", "UTF-16 LE", "UTF-16 BE", "Windows-1251"},
		Default:  EncodingUTF8,
	}
}
