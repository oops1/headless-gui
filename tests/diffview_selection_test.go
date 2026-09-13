package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// GG-70: DiffView отдаёт выделенный диапазон строк стороны. Текст выделения
// не говорит, какие это строки — одинаковые строки встречаются много раз, — а
// приложению, добавляющему в индекс выделенные строки, нужны номера.
func TestDiffView_Selection(t *testing.T) {
	dv := widget.NewDiffView("", "")
	dv.SetBounds(image.Rect(0, 0, 900, 500))
	dv.SetText(widget.DiffLeft, "old", "", "l0\nl1\nl2\nl3\n")
	dv.SetText(widget.DiffRight, "new", "", "l0\nr1\nr2\nl3\n")
	dv.SetFocused(true)

	if _, _, ok := dv.Selection(widget.DiffRight); ok {
		t.Fatal("без выделения ok=true")
	}

	key := func(code widget.KeyCode, mod widget.KeyMod) {
		dv.OnKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
	}
	want := func(from, to int) {
		t.Helper()
		gf, gt, ok := dv.Selection(widget.DiffRight)
		if !ok || gf != from || gt != to {
			t.Fatalf("Selection(правая) = %d, %d, %v; ждал %d, %d", gf, gt, ok, from, to)
		}
	}

	dv.SetActiveSide(widget.DiffRight)
	dv.SetCaret(widget.DiffRight, 1, 0)

	// Shift+↓ от начала строки выделяет ОДНУ строку: выделение кончается в
	// начале следующей и её не задевает.
	key(widget.KeyDown, widget.ModShift)
	want(1, 2)
	key(widget.KeyDown, widget.ModShift)
	want(1, 3)
	// Конец внутри строки её задевает.
	key(widget.KeyRight, widget.ModShift)
	want(1, 4)

	// Выделение вверх: начало и конец упорядочиваются.
	dv.SetCaret(widget.DiffRight, 2, 1)
	key(widget.KeyUp, widget.ModShift)
	key(widget.KeyUp, widget.ModShift)
	want(0, 3)

	// Выделение одной стороны другую не задевает; неверная сторона — не паника.
	if _, _, ok := dv.Selection(widget.DiffLeft); ok {
		t.Fatal("у левой стороны без выделения ok=true")
	}
	if _, _, ok := dv.Selection(widget.DiffSide(7)); ok {
		t.Fatal("для неверной стороны ok=true")
	}
}
