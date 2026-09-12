package main

import "github.com/oops1/headless-gui/v3/widget"

// locale.go — строки приложения. Ключи «app.» свои: ключи контрола («merge.»)
// живут в движке и переводятся вместе с ним.

func init() {
	widget.RegisterStrings("RU", map[string]string{
		"app.title":        "Слияние — mergedemo",
		"app.side.ours":    "наше (HEAD)",
		"app.side.base":    "база",
		"app.side.theirs":  "их (сливаемая ветка)",
		"app.ours":         "Взять наше",
		"app.theirs":       "Взять их",
		"app.both":         "Взять оба",
		"app.undo":         "Отменить",
		"app.save":         "Записать итог",
		"app.showBase":     "Показывать базу",
		"app.status":       "нерешённых: %d из %d   ·   итог %d:%d",
		"app.hint":         "Alt+1/2/3 — наше, база, их   ·   F7 — следующий конфликт   ·   Ctrl+S — записать",
		"app.saved":        "записано: %s",
		"app.result.title": "Итог слияния",
		"app.result.hint":  "Файл для записи не задан (ключ -o). Нерешённых конфликтов: %d.",
		"app.err":          "Ошибка",
		"app.close.title":  "Слияние не закончено",
		"app.close.msg":    "Нерешённых конфликтов: %d. Закрыть окно?",
	})
	widget.RegisterStrings("EN", map[string]string{
		"app.title":        "Merge — mergedemo",
		"app.side.ours":    "ours (HEAD)",
		"app.side.base":    "base",
		"app.side.theirs":  "theirs (merged branch)",
		"app.ours":         "Take ours",
		"app.theirs":       "Take theirs",
		"app.both":         "Take both",
		"app.undo":         "Undo",
		"app.save":         "Write result",
		"app.showBase":     "Show base",
		"app.status":       "unresolved: %d of %d   ·   result %d:%d",
		"app.hint":         "Alt+1/2/3 — ours, base, theirs   ·   F7 — next conflict   ·   Ctrl+S — write",
		"app.saved":        "written: %s",
		"app.result.title": "Merge result",
		"app.result.hint":  "No output file given (-o flag). Unresolved conflicts: %d.",
		"app.err":          "Error",
		"app.close.title":  "Merge is not finished",
		"app.close.msg":    "Unresolved conflicts: %d. Close the window?",
	})
}
