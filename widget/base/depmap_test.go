package base

// depmap_test.go — карта зависимостей пакета по группам файлов (ADR-0012,
// этап 8). XAML (8.2b), ввод текста (8.3), RichText (8.4), окно (8.5), док и
// диалоги (8.6) — отдельные пакеты widget/xaml, widget/text,
// widget/richtext, widget/chrome, widget/dock, widget/dialog: их рёбра
// проверяет компилятор (они → base, обратного импорта нет). В ядре осталась
// одна группа — переносимая часть буфера обмена (провайдер, буфер в памяти,
// CF_HTML; реализации по ОС с 8.7 — platform/clipboard): ядро может им
// пользоваться, а он ядром, других рёбер нет. База нарушений пуста:
// проверка падает на любом новом.

import (
	"regexp"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/depmap"
)

// Глобальное состояние пакета тест не трогает: только чтение исходников.
func TestDepMap_SubpackageOrder(t *testing.T) {
	re := regexp.MustCompile
	depmap.Run(t, depmap.Config{
		Dir:  ".",
		Path: "widget/base",
		Groups: []depmap.Group{
			{Name: "clipboard", Match: re(`^clipboard`)},
		},
		Default: "core",
		Allowed: func(from, to string) bool {
			return to == "core" || to == "clipboard"
		},
		Top: 6,
	})
}
