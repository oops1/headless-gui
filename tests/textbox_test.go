package tests

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// newTB создаёт TextBox в движке (точный измеритель текста зарегистрирован).
func newTB(t *testing.T, w, h int) *widget.TextBox {
	t.Helper()
	_ = newDialogEngine() // регистрирует MeasureUIText
	tb := widget.NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, w, h))
	tb.SetFocused(true)
	return tb
}

func tbType(tb *widget.TextBox, s string) {
	for _, r := range s {
		if r == '\n' {
			tb.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
			continue
		}
		tb.OnKeyEvent(widget.KeyEvent{Rune: r, Pressed: true})
	}
}

func tbKey(tb *widget.TextBox, code widget.KeyCode, mod widget.KeyMod) {
	tb.OnKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
}

// Ввод с Enter даёт многострочный текст.
func TestTextBox_MultilineInput(t *testing.T) {
	tb := newTB(t, 300, 100)
	tbType(tb, "Привет\nмир 123")
	if got := tb.GetText(); got != "Привет\nмир 123" {
		t.Fatalf("text=%q", got)
	}
	if n := tb.LineCount(); n != 2 {
		t.Fatalf("lines=%d, want 2", n)
	}
}

// Перенос по словам: длинная строка без \n занимает несколько строк.
func TestTextBox_WordWrap(t *testing.T) {
	tb := newTB(t, 160, 120)
	tb.SetText(strings.Repeat("слово ", 12))
	if n := tb.LineCount(); n < 3 {
		t.Fatalf("wrap: lines=%d, ожидалось ≥3", n)
	}
	// Без переноса — одна строка.
	tb.Wrap = false
	tb.SetText(strings.Repeat("слово ", 12))
	if n := tb.LineCount(); n != 1 {
		t.Fatalf("nowrap: lines=%d, want 1", n)
	}
}

// Стрелки вверх/вниз ходят по строкам, сохраняя целевую колонку.
func TestTextBox_UpDown(t *testing.T) {
	tb := newTB(t, 300, 100)
	tb.SetText("первая строка\nвторая\nтретья строка")
	// Каретка в конце. Вверх дважды — на первую строку.
	tbKey(tb, widget.KeyUp, 0)
	tbKey(tb, widget.KeyUp, 0)
	c := tb.CaretPosition()
	if c > len([]rune("первая строка")) {
		t.Fatalf("после Up caret=%d — не на первой строке", c)
	}
	tbKey(tb, widget.KeyDown, 0)
	c2 := tb.CaretPosition()
	first := len([]rune("первая строка")) + 1
	if c2 < first || c2 > first+len([]rune("вторая")) {
		t.Fatalf("после Down caret=%d — не на второй строке", c2)
	}
}

// Home/End — границы строки; Ctrl+Home/End — границы документа.
func TestTextBox_HomeEnd(t *testing.T) {
	tb := newTB(t, 300, 100)
	tb.SetText("один\nдва\nтри")
	tbKey(tb, widget.KeyUp, 0) // на вторую строку
	tbKey(tb, widget.KeyHome, 0)
	if c := tb.CaretPosition(); c != 5 { // после "один\n"
		t.Fatalf("Home: caret=%d, want 5", c)
	}
	tbKey(tb, widget.KeyEnd, 0)
	if c := tb.CaretPosition(); c != 8 { // "один\nдва"
		t.Fatalf("End: caret=%d, want 8", c)
	}
	tbKey(tb, widget.KeyHome, widget.ModCtrl)
	if c := tb.CaretPosition(); c != 0 {
		t.Fatalf("Ctrl+Home: caret=%d, want 0", c)
	}
	tbKey(tb, widget.KeyEnd, widget.ModCtrl)
	if c := tb.CaretPosition(); c != len([]rune("один\nдва\nтри")) {
		t.Fatalf("Ctrl+End: caret=%d", c)
	}
}

// Ctrl+стрелки — прыжки по словам.
func TestTextBox_CtrlWordJump(t *testing.T) {
	tb := newTB(t, 300, 100)
	tb.SetText("alpha beta gamma")
	tbKey(tb, widget.KeyHome, widget.ModCtrl)
	tbKey(tb, widget.KeyRight, widget.ModCtrl) // → начало "beta"
	if c := tb.CaretPosition(); c != 6 {
		t.Fatalf("Ctrl+Right: caret=%d, want 6", c)
	}
	tbKey(tb, widget.KeyRight, widget.ModCtrl) // → начало "gamma"
	if c := tb.CaretPosition(); c != 11 {
		t.Fatalf("Ctrl+Right 2: caret=%d, want 11", c)
	}
	tbKey(tb, widget.KeyLeft, widget.ModCtrl) // ← начало "beta"
	if c := tb.CaretPosition(); c != 6 {
		t.Fatalf("Ctrl+Left: caret=%d, want 6", c)
	}
}

// Shift+навигация выделяет; Ctrl+C/X/V работают с буфером обмена.
func TestTextBox_SelectionClipboard(t *testing.T) {
	tb := newTB(t, 300, 100)
	tb.SetText("hello world")
	tbKey(tb, widget.KeyHome, widget.ModCtrl)
	for i := 0; i < 5; i++ {
		tbKey(tb, widget.KeyRight, widget.ModShift)
	}
	if s := tb.SelectedText(); s != "hello" {
		t.Fatalf("selection=%q", s)
	}
	tbKey(tb, widget.KeyC, widget.ModCtrl)
	if c := widget.ClipboardGetText(); c != "hello" {
		t.Fatalf("clipboard=%q", c)
	}
	// Вырезание и вставка в конец.
	tbKey(tb, widget.KeyX, widget.ModCtrl)
	if got := tb.GetText(); got != " world" {
		t.Fatalf("после Cut: %q", got)
	}
	tbKey(tb, widget.KeyEnd, widget.ModCtrl)
	tbKey(tb, widget.KeyV, widget.ModCtrl)
	if got := tb.GetText(); got != " worldhello" {
		t.Fatalf("после Paste: %q", got)
	}
}

// Ctrl+A + Delete очищают документ; undo возвращает.
func TestTextBox_SelectAllDeleteUndo(t *testing.T) {
	tb := newTB(t, 300, 100)
	tb.SetText("раз\nдва")
	tbKey(tb, widget.KeyA, widget.ModCtrl)
	tbKey(tb, widget.KeyDelete, 0)
	if got := tb.GetText(); got != "" {
		t.Fatalf("после Ctrl+A+Del: %q", got)
	}
	tbKey(tb, widget.KeyZ, widget.ModCtrl)
	if got := tb.GetText(); got != "раз\nдва" {
		t.Fatalf("после Undo: %q", got)
	}
	tbKey(tb, widget.KeyY, widget.ModCtrl)
	if got := tb.GetText(); got != "" {
		t.Fatalf("после Redo: %q", got)
	}
}

// PgDn/PgUp листают постранично, каретка остаётся видимой (скролл едет).
func TestTextBox_PageScroll(t *testing.T) {
	tb := newTB(t, 300, 80) // мало строк на экран
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("line\n")
	}
	tb.SetText(b.String())
	tbKey(tb, widget.KeyHome, widget.ModCtrl)
	if tb.ScrollTop() != 0 {
		t.Fatalf("после Ctrl+Home scroll=%d", tb.ScrollTop())
	}
	tbKey(tb, widget.KeyPageDown, 0)
	tbKey(tb, widget.KeyPageDown, 0)
	if tb.ScrollTop() == 0 {
		t.Fatal("PgDn не прокрутил")
	}
	c := tb.CaretPosition()
	if c == 0 {
		t.Fatal("PgDn не сдвинул каретку")
	}
	tbKey(tb, widget.KeyPageUp, 0)
	if tb.CaretPosition() >= c {
		t.Fatal("PgUp не вернул каретку вверх")
	}
}

// ReadOnly: правка блокируется, копирование работает.
func TestTextBox_ReadOnly(t *testing.T) {
	tb := newTB(t, 300, 100)
	tb.SetText("const")
	tb.ReadOnly = true
	tbType(tb, "XYZ")
	tbKey(tb, widget.KeyBackspace, 0)
	if got := tb.GetText(); got != "const" {
		t.Fatalf("ReadOnly нарушен: %q", got)
	}
	tbKey(tb, widget.KeyA, widget.ModCtrl)
	tbKey(tb, widget.KeyC, widget.ModCtrl)
	if c := widget.ClipboardGetText(); c != "const" {
		t.Fatalf("копирование в ReadOnly: %q", c)
	}
}

// Клик мышью ставит каретку, drag выделяет.
func TestTextBox_MouseSelect(t *testing.T) {
	tb := newTB(t, 300, 100)
	tb.SetText("abc def")
	// Клик в начало.
	tb.OnMouseButton(widget.MouseEvent{X: 7, Y: 8, Button: widget.MouseLeft, Pressed: true})
	// Тянем вправо на ~30px (за "abc").
	tb.OnMouseMove(40, 8)
	tb.OnMouseButton(widget.MouseEvent{X: 40, Y: 8, Button: widget.MouseLeft, Pressed: false})
	if s := tb.SelectedText(); s == "" {
		t.Fatal("drag не выделил текст")
	}
}

// Сквозной headless-тест: фокус через движок, ввод через SendKeyEvent.
func TestTextBox_EngineInput(t *testing.T) {
	eng := newDialogEngine()
	tb := widget.NewTextBox("")
	tb.SetBounds(image.Rect(10, 10, 310, 110))
	root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 30, A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 200))
	root.AddChild(tb)
	eng.SetRoot(root)
	eng.SetFocus(tb)

	for _, r := range "go " {
		eng.SendKeyEvent(widget.KeyEvent{Rune: r, Pressed: true})
	}
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	for _, r := range "wasm" {
		eng.SendKeyEvent(widget.KeyEvent{Rune: r, Pressed: true})
	}
	if got := tb.GetText(); got != "go \nwasm" {
		t.Fatalf("engine input: %q", got)
	}
}
