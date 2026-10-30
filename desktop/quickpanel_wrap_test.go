package desktop

import (
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/theme"
)

// Подпись плитки: до двух строк с переносом по словам, вторая обрезается
// многоточием. Мера ширины — recCtx (testCharW на знак).
func TestQuickPanel_LabelWrapsToTwoLines(t *testing.T) {
	testreset.Globals(t)
	ctx := &recCtx{}
	s := &theme.Style{}
	const w = 14 * testCharW
	wrap := func(text string) []string { return qsWrapLabel(ctx, text, s, w, 2) }
	width := func(l string) int { return ctx.MeasureText(l, 0) }

	if got := wrap("Wi-Fi"); len(got) != 1 || got[0] != "Wi-Fi" {
		t.Errorf("короткая подпись: %q", got)
	}
	got := wrap("Режим «в самолёте»")
	if len(got) != 2 || got[0] != "Режим «в" || got[1] != "самолёте»" {
		t.Errorf("перенос по словам: %q", got)
	}
	got = wrap("Специальные возможности для людей с ограниченными возможностями")
	if len(got) != 2 {
		t.Fatalf("строк %d, ждали 2: %q", len(got), got)
	}
	if r := []rune(got[1]); r[len(r)-1] != '…' {
		t.Errorf("вторая строка не обрезана многоточием: %q", got[1])
	}
	if width(got[0]) > w || width(got[1]) > w {
		t.Errorf("строки шире поля: %q", got)
	}
	got = wrap("Энергосбережение_энергосбережение")
	if len(got) != 1 || width(got[0]) > w || []rune(got[0])[len([]rune(got[0]))-1] != '…' {
		t.Errorf("слово шире поля: %q", got)
	}
	if got := wrap("   "); len(got) != 0 {
		t.Errorf("пустая подпись: %q", got)
	}
}
