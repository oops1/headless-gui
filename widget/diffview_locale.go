package widget

// diffview_locale.go — строки контрола сравнения.
//
// Ключи «diff.» регистрируются тем же механизмом, что «dlg.» у стандартных
// диалогов: приложение переопределяет любой ключ или добавляет язык теми же
// вызовами RegisterStrings, а SetLanguage подхватывается на лету.
//
// Только символы, которые есть во встроенных Go-шрифтах: «⋯» (U+22EF) там
// нет, и его рисовал бы системный запасной шрифт — у каждой ОС свой, и кадр
// расходился бы между платформами. Поэтому в свёртке «…» (U+2026).

func init() {
	RegisterStrings("EN", map[string]string{
		"diff.empty":        "(empty)",
		"diff.modified":     "● modified",
		"diff.readonly":     "read-only",
		"diff.fold":         "…  %d identical lines hidden  …",
		"diff.ctx.undo":     "Undo",
		"diff.ctx.redo":     "Redo",
		"diff.ctx.cut":      "Cut",
		"diff.ctx.copy":     "Copy",
		"diff.ctx.paste":    "Paste",
		"diff.ctx.delete":   "Delete",
		"diff.ctx.selall":   "Select all",
		"diff.ctx.toRight":  "Copy block to the right  →",
		"diff.ctx.toLeft":   "←  Copy block to the left",
		"diff.ctx.next":     "Next change",
		"diff.ctx.prev":     "Previous change",
		"diff.ctx.allRight": "Copy whole file to the right",
		"diff.ctx.allLeft":  "Copy whole file to the left",
		"diff.err.binary":   "%s: binary file, comparison is not supported",
		"diff.a11y.view":    "File comparison",
		"diff.a11y.left":    "Left: %s",
		"diff.a11y.right":   "Right: %s",
	})
	RegisterStrings("RU", map[string]string{
		"diff.empty":        "(пусто)",
		"diff.modified":     "● изменён",
		"diff.readonly":     "только чтение",
		"diff.fold":         "…  скрыто одинаковых строк: %d  …",
		"diff.ctx.undo":     "Отменить",
		"diff.ctx.redo":     "Повторить",
		"diff.ctx.cut":      "Вырезать",
		"diff.ctx.copy":     "Копировать",
		"diff.ctx.paste":    "Вставить",
		"diff.ctx.delete":   "Удалить",
		"diff.ctx.selall":   "Выделить всё",
		"diff.ctx.toRight":  "Перенести блок вправо  →",
		"diff.ctx.toLeft":   "←  Перенести блок влево",
		"diff.ctx.next":     "Следующее изменение",
		"diff.ctx.prev":     "Предыдущее изменение",
		"diff.ctx.allRight": "Перенести весь файл вправо",
		"diff.ctx.allLeft":  "Перенести весь файл влево",
		"diff.err.binary":   "%s: двоичный файл, сравнение не поддерживается",
		"diff.a11y.view":    "Сравнение файлов",
		"diff.a11y.left":    "Слева: %s",
		"diff.a11y.right":   "Справа: %s",
	})
}
