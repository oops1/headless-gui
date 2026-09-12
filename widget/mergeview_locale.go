package widget

// mergeview_locale.go — строки контрола слияния.
//
// Ключи «merge.» регистрируются тем же механизмом, что «diff.» у контрола
// сравнения: приложение переопределяет любой ключ или добавляет язык теми же
// вызовами RegisterStrings, а SetLanguage подхватывается на лету.
//
// Только символы, которые есть во встроенных Go-шрифтах: иначе строку рисовал
// бы системный запасной шрифт — у каждой ОС свой, и эталонные кадры расходились
// бы между платформами.

func init() {
	RegisterStrings("EN", map[string]string{
		"merge.empty":          "(empty)",
		"merge.readonly":       "read-only",
		"merge.side.ours":      "ours",
		"merge.side.base":      "base",
		"merge.side.theirs":    "theirs",
		"merge.side.result":    "result",
		"merge.left":           "%d unresolved",
		"merge.done":           "%d conflicts resolved",
		"merge.ctx.ours":       "Take ours",
		"merge.ctx.theirs":     "Take theirs",
		"merge.ctx.both":       "Take both: ours, then theirs",
		"merge.ctx.base":       "Take base",
		"merge.ctx.unresolve":  "Mark as unresolved",
		"merge.ctx.copy":       "Copy",
		"merge.ctx.paste":      "Paste",
		"merge.ctx.next":       "Next conflict",
		"merge.ctx.prev":       "Previous conflict",
		"merge.a11y.view":      "Three-way merge",
		"merge.a11y.ours":      "Ours: %s",
		"merge.a11y.base":      "Base: %s",
		"merge.a11y.theirs":    "Theirs: %s",
		"merge.a11y.result":    "Result: %s",
		"merge.err.binary":     "%s: binary file, merging is not supported",
		"merge.err.sidesCount": "three-way merge needs three sides",
	})
	RegisterStrings("RU", map[string]string{
		"merge.empty":          "(пусто)",
		"merge.readonly":       "только чтение",
		"merge.side.ours":      "наше",
		"merge.side.base":      "база",
		"merge.side.theirs":    "их",
		"merge.side.result":    "итог",
		"merge.left":           "нерешённых: %d",
		"merge.done":           "конфликтов решено: %d",
		"merge.ctx.ours":       "Взять наше",
		"merge.ctx.theirs":     "Взять их",
		"merge.ctx.both":       "Взять оба: наше, потом их",
		"merge.ctx.base":       "Взять базу",
		"merge.ctx.unresolve":  "Снова считать нерешённым",
		"merge.ctx.copy":       "Копировать",
		"merge.ctx.paste":      "Вставить",
		"merge.ctx.next":       "Следующий конфликт",
		"merge.ctx.prev":       "Предыдущий конфликт",
		"merge.a11y.view":      "Трёхстороннее слияние",
		"merge.a11y.ours":      "Наше: %s",
		"merge.a11y.base":      "База: %s",
		"merge.a11y.theirs":    "Их: %s",
		"merge.a11y.result":    "Итог: %s",
		"merge.err.binary":     "%s: двоичный файл, слияние не поддерживается",
		"merge.err.sidesCount": "для слияния нужны три стороны",
	})
}
