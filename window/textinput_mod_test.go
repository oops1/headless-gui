package window

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Windows сообщает AltGr как Ctrl+Alt. Виджеты при любом Ctrl уходят в ветку
// сочетаний клавиш, поэтому символ, набранный через AltGr (на немецкой
// раскладке AltGr+Q даёт «@»), пропадал: приложение видело «сочетание
// Ctrl+Alt+@», а не ввод символа.

func TestTextMod_AltGrIsTyping(t *testing.T) {
	altGr := widget.ModCtrl | widget.ModAlt
	if got := textMod(altGr); got != 0 {
		t.Errorf("textMod(Ctrl+Alt) = %d, ждал пусто: это ввод символа", got)
	}
	// Shift при AltGr остаётся: AltGr+Shift — другой символ, но всё ещё ввод.
	if got := textMod(altGr | widget.ModShift); got != widget.ModShift {
		t.Errorf("textMod(Ctrl+Alt+Shift) = %d, ждал ModShift", got)
	}
}

// Одиночные Ctrl и Alt — настоящие сочетания, и путать их с вводом нельзя:
// Ctrl+V вставляет, а не печатает «v».
func TestTextMod_KeepsRealCombinations(t *testing.T) {
	cases := []widget.KeyMod{
		widget.ModCtrl,
		widget.ModAlt,
		widget.ModCtrl | widget.ModShift,
		widget.ModAlt | widget.ModShift,
		widget.ModShift,
		0,
	}
	for _, mod := range cases {
		if got := textMod(mod); got != mod {
			t.Errorf("textMod(%d) = %d, ждал без изменений", mod, got)
		}
	}
}
