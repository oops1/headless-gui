package base

// depmap_test.go — карта зависимостей пакета по будущим подпакетам
// (ADR-0012, этап 8). Группы — по именам файлов; порядок — целевой граф
// пакетов: всё → base (core); richtext → text; dialog → chrome, text;
// text, richtext, dialog, dock, chrome → xaml (регистрация строителей);
// clipboard — провайдер в ядре, реализация в platform. Остальное —
// обратные рёбра, которые этап разрывает; сегодняшние перечислены в базе:
// проверка падает, если появится новое, и сообщает, когда базовое исчезло.

import (
	"regexp"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/depmap"
)

// Глобальное состояние пакета тест не трогает: только чтение исходников.
func TestDepMap_SubpackageOrder(t *testing.T) {
	re := regexp.MustCompile
	allowed := map[string]map[string]bool{
		"richtext": {"text": true, "xaml": true},
		"dialog":   {"chrome": true, "text": true, "xaml": true},
		"dock":     {"xaml": true},
		"chrome":   {"xaml": true},
		"text":     {"xaml": true},
	}
	depmap.Run(t, depmap.Config{
		Dir:  ".",
		Path: "widget/base",
		Groups: []depmap.Group{
			{Name: "xaml", Match: re(`^xaml`)},
			{Name: "richtext", Match: re(`^richtext`)},
			{Name: "text", Match: re(`^text(box|input)|^ime\.go$`)},
			{Name: "dialog", Match: re(`^dialog|^messagebox`)},
			{Name: "dock", Match: re(`^dock`)},
			{Name: "chrome", Match: re(`^window|^titlebar`)},
			{Name: "clipboard", Match: re(`^clipboard`)},
		},
		Default: "core",
		Allowed: func(from, to string) bool {
			if to == "core" || to == "clipboard" {
				return true
			}
			return allowed[from][to]
		},
		// Сегодняшние обратные рёбра (см. ADR-0012, таблица разрыва). Каждый
		// шаг этапа 8 убирает свои строки.
		Baseline: []string{
			"chrome → dialog",
			"chrome → dock",
			"core → chrome",
			"core → dialog",
			"core → dock",
			"core → richtext",
			"core → text",
			"core → xaml",
			"dock → chrome",
			"dock → dialog",
			"xaml → chrome",
			"xaml → dialog",
			"xaml → dock",
			"xaml → richtext",
			"xaml → text",
		},
		Top: 6,
	})
}
