package widget

import (
	"image"
	"strings"
	"sync"
	"testing"
)

// Тесты редактора RichText без окна (измеритель — предсказуемый, как в
// richtext_test.go). Сквозные проверки через настоящий движок — в
// tests/richtext_edit_test.go.

// edRT — редактор с абзацами paras.
func edRT(paras ...RichParagraph) *RichText {
	UseMemoryClipboard()
	rt := newRT(300, 120, paras...)
	rt.Editable = true
	return rt
}

// edType набирает s клавишами: у печатной клавиши Rune заполнен, Code не важен.
func edType(rt *RichText, s string) {
	for _, r := range s {
		rt.OnKeyEvent(KeyEvent{Rune: r, Pressed: true})
	}
}

func edKey(rt *RichText, code KeyCode, mod KeyMod) {
	rt.OnKeyEvent(KeyEvent{Code: code, Mod: mod, Pressed: true})
}

// edRuns — раны абзаца i.
func edRuns(rt *RichText, i int) []RichRun { return rt.Paragraphs()[i].Runs }

func boldRun(text string) RichRun   { return RichRun{Text: text, Font: BuiltinFontBold} }
func italicRun(text string) RichRun { return RichRun{Text: text, Font: BuiltinFontItalic} }

// ─── Режим просмотра ────────────────────────────────────────────────────────

// Без Editable виджет ничего не принимает: ни символы, ни Enter, ни Backspace,
// ни Ctrl+B/V/X/Z — и OnChange молчит.
func TestRichEdit_ViewModeIgnoresInput(t *testing.T) {
	UseMemoryClipboard()
	SetClipboardHTML("<p>pasted</p>", "pasted")
	rt := newRT(300, 120, plainPara("hello"))
	calls := 0
	rt.OnChange = func() { calls++ }
	rt.Select(1, 3)

	edType(rt, "xyz")
	for _, k := range []struct {
		c KeyCode
		m KeyMod
	}{
		{KeyEnter, 0}, {KeyEnter, ModShift}, {KeyBackspace, 0}, {KeyDelete, 0},
		{KeyBackspace, ModCtrl}, {KeyB, ModCtrl}, {KeyI, ModCtrl}, {KeyU, ModCtrl},
		{KeyV, ModCtrl}, {KeyX, ModCtrl}, {KeyZ, ModCtrl}, {KeyY, ModCtrl},
		{KeyInsert, ModShift}, {KeyDelete, ModShift}, {KeyTab, 0},
	} {
		edKey(rt, k.c, k.m)
	}
	if rt.Text() != "hello" {
		t.Errorf("просмотр изменил текст: %q", rt.Text())
	}
	if len(edRuns(rt, 0)) != 1 || edRuns(rt, 0)[0].Font != "" {
		t.Errorf("просмотр изменил оформление: %+v", edRuns(rt, 0))
	}
	if calls != 0 {
		t.Errorf("OnChange вызван %d раз в режиме просмотра", calls)
	}
	// Команды панели тоже не действуют, а чтение работает.
	rt.ToggleBold()
	rt.SetSelectionStyle(func(r *RichRun) { r.Size = 30 })
	rt.SetParagraphFormat(func(p *RichParagraph) { p.Align = TextAlignRight })
	if rt.Undo() || rt.Cut() || rt.Paste() {
		t.Error("команда правки сработала в просмотре")
	}
	if rt.Text() != "hello" || edRuns(rt, 0)[0].Size != 0 || rt.Paragraphs()[0].Align != 0 {
		t.Errorf("просмотр изменён командами панели: %+v", rt.Paragraphs())
	}
	if rt.AccessReadOnly() != true || rt.AcceptsTab() {
		t.Error("просмотр объявил себя редактором")
	}
	if rt.AccessSetText("zzz") || rt.Text() != "hello" {
		t.Error("скринридер переписал текст просмотра")
	}
	// Стрелки вниз/вверх в просмотре по-прежнему листают, а не двигают каретку.
	rt.SetCaretPosition(2)
	edKey(rt, KeyDown, 0)
	if rt.CaretPosition() != 2 {
		t.Errorf("↓ в просмотре сдвинула каретку: %d", rt.CaretPosition())
	}
}

// У редактора каретка рисуется без ShowCaret, а ↑/↓ двигают её, а не листают.
func TestRichEdit_CaretAlwaysShown(t *testing.T) {
	rt := edRT(plainPara("one"), plainPara("two"))
	rt.SetFocused(true)
	rt.mu.Lock()
	shown := rt.caretShownLocked()
	rt.mu.Unlock()
	if !shown || rt.ShowCaret {
		t.Errorf("каретка у редактора: shown=%v ShowCaret=%v", shown, rt.ShowCaret)
	}
	edKey(rt, KeyDown, 0)
	if rt.CaretPosition() != 4 {
		t.Errorf("↓ в редакторе: каретка %d, ждали 4", rt.CaretPosition())
	}
	edKey(rt, KeyHome, 0)
	if rt.CaretPosition() != 4 {
		t.Errorf("Home: каретка %d", rt.CaretPosition())
	}
	edKey(rt, KeyUp, 0)
	if rt.CaretPosition() != 0 {
		t.Errorf("↑: каретка %d, ждали 0", rt.CaretPosition())
	}
}

// ─── Ввод и отмена ──────────────────────────────────────────────────────────

// Набранное слово отменяется одним Ctrl+Z, возвращается Ctrl+Y и
// Ctrl+Shift+Z; каретка встаёт по ответу документа.
func TestRichEdit_TypeUndoRedo(t *testing.T) {
	rt := edRT()
	edType(rt, "hello")
	if rt.Text() != "hello" || rt.CaretPosition() != 5 {
		t.Fatalf("после набора %q, каретка %d", rt.Text(), rt.CaretPosition())
	}
	if n := len(rt.Paragraphs()); n != 1 {
		t.Fatalf("абзацев %d: набор в пустом виджете обязан создать абзац", n)
	}
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "" || rt.CanUndo() || !rt.CanRedo() {
		t.Fatalf("после Ctrl+Z: %q undo=%v redo=%v — слово должно уйти одним шагом",
			rt.Text(), rt.CanUndo(), rt.CanRedo())
	}
	edKey(rt, KeyY, ModCtrl)
	if rt.Text() != "hello" || rt.CaretPosition() != 5 {
		t.Fatalf("после Ctrl+Y %q, каретка %d", rt.Text(), rt.CaretPosition())
	}
	edKey(rt, KeyZ, ModCtrl)
	edKey(rt, KeyZ, ModCtrl|ModShift)
	if rt.Text() != "hello" {
		t.Fatalf("после Ctrl+Shift+Z %q", rt.Text())
	}
}

// Движение каретки, щелчок и потеря фокуса рвут набор: следующий символ —
// новая запись отмены.
func TestRichEdit_CaretMoveAndFocusBreakUndoGroup(t *testing.T) {
	rt := edRT()
	edType(rt, "ab")
	edKey(rt, KeyLeft, 0)
	edKey(rt, KeyRight, 0) // снова в конце: без разрыва «c» прилипло бы к «ab»
	edType(rt, "c")
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "ab" {
		t.Fatalf("после движения каретки Ctrl+Z убрал не только «c»: %q", rt.Text())
	}

	rt = edRT()
	rt.SetFocused(true)
	edType(rt, "ab")
	rt.SetFocused(false)
	rt.SetFocused(true)
	edType(rt, "c")
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "ab" {
		t.Fatalf("после потери фокуса Ctrl+Z убрал не только «c»: %q", rt.Text())
	}

	rt = edRT(plainPara("xy"))
	rt.SetCaretPosition(2)
	edType(rt, "a")
	rtPress(rt, 4, 8, 1) // щелчок в начало
	rtRelease(rt, 4, 8, 1)
	rt.SetCaretPosition(3)
	edType(rt, "b")
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "xya" {
		t.Fatalf("после щелчка Ctrl+Z убрал не только «b»: %q", rt.Text())
	}
}

// Enter — новый абзац, Shift+Enter — мягкий перевод строки внутри абзаца.
func TestRichEdit_EnterAndSoftBreak(t *testing.T) {
	rt := edRT(RichParagraph{Runs: []RichRun{{Text: "abcd"}}, Align: TextAlignCenter})
	rt.SetCaretPosition(2)
	edKey(rt, KeyEnter, 0)
	ps := rt.Paragraphs()
	if len(ps) != 2 || ps[0].Runs[0].Text != "ab" || ps[1].Runs[0].Text != "cd" {
		t.Fatalf("Enter: %+v", ps)
	}
	if ps[1].Align != TextAlignCenter {
		t.Error("новый абзац потерял выравнивание")
	}
	if rt.CaretPosition() != 3 {
		t.Errorf("каретка после Enter %d, ждали 3", rt.CaretPosition())
	}
	edKey(rt, KeyEnter, ModShift)
	ps = rt.Paragraphs()
	if len(ps) != 2 {
		t.Fatalf("Shift+Enter разбил абзац: %d абзацев", len(ps))
	}
	if ps[1].Runs[0].Text != "\ncd" || rt.Text() != "ab\n\ncd" {
		t.Errorf("мягкий перевод: %q / %q", ps[1].Runs[0].Text, rt.Text())
	}
	edKey(rt, KeyZ, ModCtrl)
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "abcd" || len(rt.Paragraphs()) != 1 {
		t.Errorf("после двух отмен %q", rt.Text())
	}
}

// Backspace в начале абзаца сливает его с предыдущим, Delete в конце — со
// следующим; обе правки отменяются.
func TestRichEdit_BackspaceAtParagraphBoundary(t *testing.T) {
	rt := edRT(plainPara("ab"), plainPara("cd"))
	rt.SetCaretPosition(3)
	edKey(rt, KeyBackspace, 0)
	if rt.Text() != "abcd" || len(rt.Paragraphs()) != 1 || rt.CaretPosition() != 2 {
		t.Fatalf("Backspace на границе: %q абзацев %d каретка %d",
			rt.Text(), len(rt.Paragraphs()), rt.CaretPosition())
	}
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "ab\ncd" || len(rt.Paragraphs()) != 2 {
		t.Fatalf("отмена слияния: %q", rt.Text())
	}
	rt.SetCaretPosition(2)
	edKey(rt, KeyDelete, 0)
	if rt.Text() != "abcd" || len(rt.Paragraphs()) != 1 {
		t.Fatalf("Delete на границе: %q", rt.Text())
	}
	// В начале документа Backspace, в конце Delete — ничего.
	rt.SetCaretPosition(0)
	edKey(rt, KeyBackspace, 0)
	rt.SetCaretPosition(4)
	edKey(rt, KeyDelete, 0)
	if rt.Text() != "abcd" {
		t.Errorf("края документа: %q", rt.Text())
	}
}

// Ctrl+Backspace и Ctrl+Delete удаляют слово; через границу абзаца слово не
// переходит — убирается только разделитель.
func TestRichEdit_WordDelete(t *testing.T) {
	rt := edRT(plainPara("one two three"))
	rt.SetCaretPosition(7) // после «two»
	edKey(rt, KeyBackspace, ModCtrl)
	if rt.Text() != "one  three" || rt.CaretPosition() != 4 {
		t.Fatalf("Ctrl+Backspace: %q каретка %d", rt.Text(), rt.CaretPosition())
	}
	edKey(rt, KeyDelete, ModCtrl)
	if rt.Text() != "one three" {
		t.Fatalf("Ctrl+Delete: %q", rt.Text())
	}

	rt = edRT(plainPara("one two"), plainPara("three"))
	rt.SetCaretPosition(8) // начало второго абзаца
	edKey(rt, KeyBackspace, ModCtrl)
	if rt.Text() != "one twothree" {
		t.Fatalf("Ctrl+Backspace в начале абзаца: %q", rt.Text())
	}
	rt = edRT(plainPara("one two"), plainPara("three"))
	rt.SetCaretPosition(4) // перед «two»
	edKey(rt, KeyDelete, ModCtrl)
	if rt.Text() != "one \nthree" {
		t.Fatalf("Ctrl+Delete у последнего слова слил абзацы: %q", rt.Text())
	}
}

// Ввод при выделении заменяет его; замена и дальнейший набор — одна отмена,
// которая возвращает выделение.
func TestRichEdit_ReplaceSelection(t *testing.T) {
	rt := edRT(plainPara("hello world"))
	rt.Select(6, 11)
	edType(rt, "X")
	if rt.Text() != "hello X" || rt.CaretPosition() != 7 || rt.SelectedText() != "" {
		t.Fatalf("замена: %q каретка %d", rt.Text(), rt.CaretPosition())
	}
	edType(rt, "yz")
	if rt.Text() != "hello Xyz" {
		t.Fatalf("набор после замены: %q", rt.Text())
	}
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "hello world" {
		t.Fatalf("Ctrl+Z после замены: %q — замена и набор должны отменяться вместе", rt.Text())
	}
	if rt.SelectedText() != "world" {
		t.Errorf("выделение после отмены %q", rt.SelectedText())
	}
	// Backspace по выделению тоже удаляет его целиком.
	rt.Select(0, 6)
	edKey(rt, KeyBackspace, 0)
	if rt.Text() != "world" {
		t.Errorf("Backspace по выделению: %q", rt.Text())
	}
}

// Набор над выделением берёт оформление его первого символа.
func TestRichEdit_ReplaceKeepsStyle(t *testing.T) {
	rt := edRT(RichParagraph{Runs: []RichRun{{Text: "aa "}, boldRun("bold"), {Text: " zz"}}})
	rt.Select(3, 7)
	edType(rt, "Q")
	runs := edRuns(rt, 0)
	if len(runs) != 3 || runs[1].Text != "Q" || runs[1].Font != BuiltinFontBold {
		t.Fatalf("замена жирного слова: %+v", runs)
	}
}

// Tab вставляет отступ только при AcceptTab, а TabAcceptor сообщает то же.
func TestRichEdit_Tab(t *testing.T) {
	rt := edRT(plainPara("x"))
	if rt.AcceptsTab() {
		t.Error("без AcceptTab редактор забирает Tab")
	}
	edKey(rt, KeyTab, 0)
	if rt.Text() != "x" {
		t.Errorf("Tab без AcceptTab изменил текст: %q", rt.Text())
	}
	rt.AcceptTab = true
	if !rt.AcceptsTab() {
		t.Error("с AcceptTab редактор не забирает Tab")
	}
	edKey(rt, KeyTab, 0)
	if rt.Text() != "    x" {
		t.Errorf("Tab: %q", rt.Text())
	}
	edKey(rt, KeyTab, ModShift)
	if rt.Text() != "    x" {
		t.Errorf("Shift+Tab изменил текст: %q", rt.Text())
	}
	rt.Editable = false
	if rt.AcceptsTab() {
		t.Error("просмотр забирает Tab")
	}
}

// Alt без Ctrl не печатает, Ctrl+Alt (AltGr) печатает, управляющие — нет.
func TestRichEdit_PrintableRules(t *testing.T) {
	rt := edRT()
	rt.OnKeyEvent(KeyEvent{Rune: 'a', Mod: ModAlt, Pressed: true})
	rt.OnKeyEvent(KeyEvent{Rune: '@', Mod: ModCtrl | ModAlt, Pressed: true})
	rt.OnKeyEvent(KeyEvent{Rune: 0x7f, Pressed: true})
	rt.OnKeyEvent(KeyEvent{Rune: 'b', Mod: ModCtrl, Pressed: true})
	rt.OnKeyEvent(KeyEvent{Rune: 'я', Pressed: true})
	rt.OnKeyEvent(KeyEvent{Rune: 'c', Pressed: false})
	if rt.Text() != "@я" {
		t.Errorf("набрано %q, ждали «@я»", rt.Text())
	}
}

// ─── Оформление ─────────────────────────────────────────────────────────────

// Ctrl+B без выделения меняет стиль набора: следующая буква жирная, а
// движение каретки сбрасывает его.
func TestRichEdit_PendingStyleAndReset(t *testing.T) {
	rt := edRT(plainPara("abc"))
	rt.SetCaretPosition(3)
	edKey(rt, KeyB, ModCtrl)
	if !rt.SelectionStyle().IsBold() {
		t.Fatal("панель не видит стиль набора: кнопка «Ж» не нажалась")
	}
	if len(edRuns(rt, 0)) != 1 || edRuns(rt, 0)[0].Font != "" {
		t.Fatalf("Ctrl+B без выделения изменил текст: %+v", edRuns(rt, 0))
	}
	edType(rt, "de")
	runs := edRuns(rt, 0)
	if len(runs) != 2 || runs[1].Text != "de" || runs[1].Font != BuiltinFontBold {
		t.Fatalf("набор жирным: %+v", runs)
	}
	// Второе нажатие выключает стиль набора.
	edKey(rt, KeyB, ModCtrl)
	edType(rt, "f")
	runs = edRuns(rt, 0)
	if len(runs) != 3 || runs[2].Text != "f" || runs[2].Font != "" {
		t.Fatalf("Ctrl+B ещё раз: %+v", runs)
	}

	rt = edRT(plainPara("abc"))
	rt.SetCaretPosition(1)
	edKey(rt, KeyB, ModCtrl)
	edKey(rt, KeyRight, 0)
	if rt.SelectionStyle().IsBold() {
		t.Error("движение каретки не сбросило стиль набора")
	}
	edType(rt, "X")
	if runs := edRuns(rt, 0); len(runs) != 1 || runs[0].Text != "abXc" {
		t.Errorf("после движения набралось не обычным: %+v", runs)
	}

	// Щелчок и программная установка каретки тоже сбрасывают.
	rt = edRT(plainPara("abc"))
	edKey(rt, KeyI, ModCtrl)
	rt.SetCaretPosition(2)
	if rt.SelectionStyle().IsItalic() {
		t.Error("SetCaretPosition не сбросил стиль набора")
	}
	edKey(rt, KeyI, ModCtrl)
	rtPress(rt, 4, 8, 1)
	rtRelease(rt, 4, 8, 1)
	if rt.SelectionStyle().IsItalic() {
		t.Error("щелчок не сбросил стиль набора")
	}
}

// Стиль набора переживает Backspace, но не Enter-разрыв оформления: в новом
// абзаце оформление несёт сам документ.
func TestRichEdit_PendingStyleSurvivesTypingAndEnter(t *testing.T) {
	rt := edRT()
	edKey(rt, KeyB, ModCtrl)
	edType(rt, "ab")
	edKey(rt, KeyBackspace, 0)
	edType(rt, "c")
	if runs := edRuns(rt, 0); len(runs) != 1 || runs[0].Text != "ac" || runs[0].Font != BuiltinFontBold {
		t.Fatalf("набор с Backspace: %+v", runs)
	}
	edKey(rt, KeyEnter, 0)
	edType(rt, "z")
	ps := rt.Paragraphs()
	if len(ps) != 2 || ps[1].Runs[0].Font != BuiltinFontBold {
		t.Fatalf("новый абзац после Enter потерял жирный: %+v", ps)
	}
}

// Ctrl+B на выделении делает его жирным, отмена возвращает, второе нажатие
// на полностью жирном выключает.
func TestRichEdit_ToggleBoldOnSelectionAndUndo(t *testing.T) {
	rt := edRT(plainPara("abcdef"))
	rt.Select(0, 3)
	edKey(rt, KeyB, ModCtrl)
	runs := edRuns(rt, 0)
	if len(runs) != 2 || runs[0].Text != "abc" || !runs[0].IsBold() || runs[1].IsBold() {
		t.Fatalf("жирное выделение: %+v", runs)
	}
	if rt.SelectedText() != "abc" {
		t.Errorf("выделение потерялось: %q", rt.SelectedText())
	}
	edKey(rt, KeyZ, ModCtrl)
	if runs := edRuns(rt, 0); len(runs) != 1 || runs[0].Font != "" {
		t.Fatalf("отмена: %+v", runs)
	}
	edKey(rt, KeyY, ModCtrl)
	if !edRuns(rt, 0)[0].IsBold() {
		t.Fatalf("возврат: %+v", edRuns(rt, 0))
	}
	edKey(rt, KeyB, ModCtrl)
	if runs := edRuns(rt, 0); len(runs) != 1 || runs[0].Font != "" {
		t.Fatalf("повторное Ctrl+B не сняло жирный: %+v", runs)
	}
}

// Выделение, где жирное только частично, Ctrl+B делает жирным целиком.
func TestRichEdit_ToggleMixedSelection(t *testing.T) {
	rt := edRT(RichParagraph{Runs: []RichRun{boldRun("ab"), {Text: "cd"}}})
	rt.SelectAll()
	rt.ToggleBold()
	if runs := edRuns(rt, 0); len(runs) != 1 || !runs[0].IsBold() {
		t.Fatalf("смешанное выделение: %+v", runs)
	}
	rt.ToggleBold()
	if runs := edRuns(rt, 0); len(runs) != 1 || runs[0].IsBold() {
		t.Fatalf("второе нажатие: %+v", runs)
	}
}

// Жирность у курсива даёт жирный курсив, снятие курсива — жирный.
func TestRichEdit_BoldItalicCombination(t *testing.T) {
	rt := edRT(RichParagraph{Runs: []RichRun{italicRun("text")}})
	rt.SelectAll()
	rt.ToggleBold()
	r := edRuns(rt, 0)[0]
	if r.Font != BuiltinFontBoldItalic {
		t.Fatalf("жирность у курсива: шрифт %q, ждали жирный курсив", r.Font)
	}
	st := rt.SelectionStyle()
	if !st.IsBold() || !st.IsItalic() {
		t.Fatalf("SelectionStyle жирного курсива: %+v", st)
	}
	rt.ToggleItalic()
	if r := edRuns(rt, 0)[0]; r.Font != BuiltinFontBold {
		t.Fatalf("снятие курсива: шрифт %q", r.Font)
	}
	rt.ToggleBold()
	if r := edRuns(rt, 0)[0]; r.Font != "" {
		t.Fatalf("снятие жирного: шрифт %q", r.Font)
	}
	// Стиль набора комбинируется так же.
	rt = edRT()
	edKey(rt, KeyI, ModCtrl)
	edKey(rt, KeyB, ModCtrl)
	edType(rt, "w")
	if r := edRuns(rt, 0)[0]; r.Font != BuiltinFontBoldItalic {
		t.Fatalf("набор жирным курсивом: %q", r.Font)
	}
}

// Чужой шрифт жирным не становится: утолщения без отдельного шрифта нет.
func TestRichEdit_ToggleBoldKeepsForeignFont(t *testing.T) {
	rt := edRT(RichParagraph{Runs: []RichRun{{Text: "code", Font: "Consolas"}}})
	rt.SelectAll()
	rt.ToggleBold()
	if r := edRuns(rt, 0)[0]; r.Font != "Consolas" {
		t.Errorf("шрифт приложения подменён: %q", r.Font)
	}
}

func TestRichEdit_UnderlineStrikeAndHotkeys(t *testing.T) {
	rt := edRT(plainPara("abc"))
	rt.SelectAll()
	edKey(rt, KeyU, ModCtrl)
	if !edRuns(rt, 0)[0].Underline {
		t.Fatal("Ctrl+U не подчеркнул")
	}
	rt.ToggleStrike()
	if !edRuns(rt, 0)[0].Strike {
		t.Fatal("ToggleStrike не зачеркнул")
	}
	rt.ToggleUnderline()
	r := edRuns(rt, 0)[0]
	if r.Underline || !r.Strike {
		t.Fatalf("ToggleUnderline: %+v", r)
	}
	// Автоповтор удерживаемой Ctrl+B не мигает жирным.
	rt.OnKeyEvent(KeyEvent{Code: KeyB, Mod: ModCtrl, Pressed: true, Repeat: true})
	if edRuns(rt, 0)[0].IsBold() {
		t.Error("автоповтор Ctrl+B переключил жирный")
	}
}

// SetSelectionStyle: общий путь (цвет, кегль), на выделении и в каретке.
func TestRichEdit_SetSelectionStyle(t *testing.T) {
	rt := edRT(plainPara("abcdef"))
	rt.Select(1, 3)
	rt.SetSelectionStyle(func(r *RichRun) { r.Size = 20; r.Link = "u"; r.Text = "HACK" })
	runs := edRuns(rt, 0)
	if len(runs) != 3 || runs[1].Text != "bc" || runs[1].Size != 20 || runs[1].Link != "u" {
		t.Fatalf("стиль на выделении: %+v", runs)
	}
	if rt.Text() != "abcdef" {
		t.Errorf("fn изменила текст: %q", rt.Text())
	}
	// В каретке — стиль набора.
	rt.SetCaretPosition(6)
	rt.SetSelectionStyle(func(r *RichRun) { r.Size = 30; r.Link = "" })
	edType(rt, "Z")
	runs = edRuns(rt, 0)
	if last := runs[len(runs)-1]; last.Text != "Z" || last.Size != 30 {
		t.Fatalf("стиль набора через SetSelectionStyle: %+v", runs)
	}
	if st := rt.SelectionStyle(); st.Size != 30 {
		t.Errorf("SelectionStyle в каретке: %+v", st)
	}
	rt.SetSelectionStyle(nil)
	rt.SetParagraphFormat(nil)
}

// SetParagraphFormat — выравнивание и отступ абзацев выделения.
func TestRichEdit_ParagraphFormat(t *testing.T) {
	rt := edRT(plainPara("a"), plainPara("b"), plainPara("c"))
	rt.Select(0, 3) // абзацы «a» и «b»
	rt.SetParagraphFormat(func(p *RichParagraph) { p.Align = TextAlignCenter; p.Indent = 12 })
	ps := rt.Paragraphs()
	if ps[0].Align != TextAlignCenter || ps[1].Align != TextAlignCenter || ps[2].Align != 0 || ps[0].Indent != 12 {
		t.Fatalf("формат абзацев: %+v", ps)
	}
	if got := rt.SelectionParagraphFormat(); got.Align != TextAlignCenter || got.Indent != 12 || got.Runs != nil {
		t.Errorf("SelectionParagraphFormat: %+v", got)
	}
	edKey(rt, KeyZ, ModCtrl)
	if rt.Paragraphs()[0].Align != 0 {
		t.Error("отмена формата абзацев не сработала")
	}
}

// Набор у границы ссылки не продолжает её; внутри ссылки — продолжает.
func TestRichEdit_TypingNearLink(t *testing.T) {
	link := RichRun{Text: "link", Link: "https://x.y"}
	rt := edRT(RichParagraph{Runs: []RichRun{{Text: "pre "}, link, {Text: " post"}}})
	rt.SetCaretPosition(8) // сразу за ссылкой
	edType(rt, "!")
	for _, r := range edRuns(rt, 0) {
		if strings.Contains(r.Text, "!") && r.Link != "" {
			t.Fatalf("символ, набранный за ссылкой, стал её частью: %+v", edRuns(rt, 0))
		}
	}
	rt = edRT(RichParagraph{Runs: []RichRun{{Text: "pre "}, link, {Text: " post"}}})
	rt.SetCaretPosition(6) // внутри ссылки
	edType(rt, "!")
	found := false
	for _, r := range edRuns(rt, 0) {
		if r.Link != "" && strings.Contains(r.Text, "li!nk") {
			found = true
		}
	}
	if !found {
		t.Fatalf("символ внутри ссылки вышел из неё: %+v", edRuns(rt, 0))
	}
}

// ─── Буфер обмена ───────────────────────────────────────────────────────────

func TestRichEdit_CutPasteOneUndo(t *testing.T) {
	rt := edRT(RichParagraph{Runs: []RichRun{boldRun("hello"), {Text: " world"}}})
	rt.Select(0, 5)
	edKey(rt, KeyX, ModCtrl)
	if rt.Text() != " world" || rt.CaretPosition() != 0 {
		t.Fatalf("после Ctrl+X %q каретка %d", rt.Text(), rt.CaretPosition())
	}
	if ClipboardGetText() != "hello" {
		t.Fatalf("в буфере %q", ClipboardGetText())
	}
	if _, ok := ClipboardHTML(); !ok {
		t.Error("вырезание не положило HTML")
	}
	rt.SetCaretPosition(6)
	edKey(rt, KeyV, ModCtrl)
	runs := edRuns(rt, 0)
	if rt.Text() != " worldhello" || runs[len(runs)-1].Text != "hello" || !runs[len(runs)-1].IsBold() {
		t.Fatalf("вставка вернула не жирное: %q %+v", rt.Text(), runs)
	}
	if rt.CaretPosition() != 11 {
		t.Errorf("каретка после вставки %d", rt.CaretPosition())
	}
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != " world" {
		t.Fatalf("отмена вставки: %q", rt.Text())
	}
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "hello world" {
		t.Fatalf("отмена вырезания: %q — одна запись на вырезание", rt.Text())
	}
	// Shift+Delete и Shift+Insert — то же, что Ctrl+X и Ctrl+V.
	rt.Select(0, 5)
	edKey(rt, KeyDelete, ModShift)
	rt.SetCaretPosition(len([]rune(rt.Text())))
	edKey(rt, KeyInsert, ModShift)
	if rt.Text() != " worldhello" {
		t.Fatalf("Shift+Delete / Shift+Insert: %q", rt.Text())
	}
	// Без выделения вырезать нечего.
	rt.SetCaretPosition(1)
	if rt.Cut() {
		t.Error("Cut без выделения")
	}
}

// Вставка поверх выделения заменяет его одной записью отмены.
func TestRichEdit_PasteReplacesSelection(t *testing.T) {
	rt := edRT(plainPara("hello world"))
	SetClipboardHTML("<p>big</p>", "big")
	rt.Select(6, 11)
	edKey(rt, KeyV, ModCtrl)
	if rt.Text() != "hello big" {
		t.Fatalf("вставка поверх выделения: %q", rt.Text())
	}
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "hello world" {
		t.Fatalf("одна отмена должна вернуть и удалённое: %q", rt.Text())
	}
}

// Вставка HTML из «Word» сохраняет жирный, ссылку и абзацы.
func TestRichEdit_PasteWordHTML(t *testing.T) {
	rt := edRT()
	word := `<html xmlns:o="urn:schemas-microsoft-com:office:office"><body>` +
		`<!--StartFragment--><p class=MsoNormal>Plain <b>bold</b> and ` +
		`<a href="https://example.com/doc">the link</a><o:p></o:p></p>` +
		`<p class=MsoNormal align=center>Second</p><!--EndFragment--></body></html>`
	SetClipboardHTML(word, "Plain bold and the link\nSecond")
	edKey(rt, KeyV, ModCtrl)
	ps := rt.Paragraphs()
	if len(ps) != 2 {
		t.Fatalf("абзацев %d: %+v", len(ps), ps)
	}
	var bold, link bool
	for _, r := range ps[0].Runs {
		if r.Text == "bold" && r.IsBold() {
			bold = true
		}
		if r.Text == "the link" && r.Link == "https://example.com/doc" {
			link = true
		}
	}
	if !bold || !link {
		t.Errorf("оформление не доехало: жирный=%v ссылка=%v %+v", bold, link, ps[0].Runs)
	}
	if ps[1].Align != TextAlignCenter || ps[1].Runs[0].Text != "Second" {
		t.Errorf("второй абзац: %+v", ps[1])
	}
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "" {
		t.Errorf("вставка не отменилась одной записью: %q", rt.Text())
	}
}

// Без HTML в буфере вставляется простой текст оформлением каретки.
func TestRichEdit_PastePlainTextUsesCaretStyle(t *testing.T) {
	rt := edRT(RichParagraph{Runs: []RichRun{boldRun("ab")}})
	rt.SetCaretPosition(2)
	ClipboardSetText("x\r\ny")
	edKey(rt, KeyV, ModCtrl)
	ps := rt.Paragraphs()
	if len(ps) != 2 || ps[0].Runs[0].Text != "abx" || ps[1].Runs[0].Text != "y" ||
		!ps[0].Runs[0].IsBold() || !ps[1].Runs[0].IsBold() {
		t.Fatalf("простая вставка: %+v", ps)
	}
	// Пустой буфер ничего не делает.
	ClipboardSetText("")
	if rt.Paste() {
		t.Error("пустой буфер что-то вставил")
	}
}

// ─── HTML целиком ───────────────────────────────────────────────────────────

func TestRichEdit_HTMLRoundTrip(t *testing.T) {
	rt := edRT()
	rt.SetParagraphs([]RichParagraph{
		{Runs: []RichRun{{Text: "Title "}, boldRun("bold"), {Text: "x", Link: "https://a.b"}}, Align: TextAlignCenter},
		{},
		{Runs: []RichRun{{Text: "tail", Underline: true}}},
		{},
	})
	html := rt.HTML()
	rt2 := edRT()
	rt2.SetHTML(html)
	if got, want := rt2.Text(), rt.Text(); got != want {
		t.Fatalf("круг HTML: %q, ждали %q\n%s", got, want, html)
	}
	a, b := rt.Paragraphs(), rt2.Paragraphs()
	if len(a) != len(b) {
		t.Fatalf("абзацев %d и %d\n%s", len(a), len(b), html)
	}
	if b[0].Align != TextAlignCenter || !b[0].Runs[1].IsBold() || b[0].Runs[2].Link != "https://a.b" ||
		!b[2].Runs[0].Underline {
		t.Errorf("оформление после круга: %+v", b)
	}
	if rt2.CanUndo() || rt2.CaretPosition() != 0 {
		t.Error("SetHTML оставил историю или каретку")
	}
	if NewRichText().HTML() != "" {
		t.Error("HTML пустого виджета непуст")
	}
}

// ─── OnChange ───────────────────────────────────────────────────────────────

func TestRichEdit_OnChange(t *testing.T) {
	rt := edRT(plainPara("hello"))
	calls := 0
	rt.OnChange = func() {
		calls++
		_ = rt.Text() // обработчик вправе звать виджет: вызов — вне замка
		_ = rt.SelectionStyle()
	}
	want := func(n int, what string) {
		t.Helper()
		if calls != n {
			t.Fatalf("%s: OnChange вызван %d раз, ждали %d", what, calls, n)
		}
	}
	rt.SetCaretPosition(5)
	edKey(rt, KeyLeft, 0)
	want(0, "движение")
	edType(rt, "ab")
	want(2, "ввод двух символов")
	edKey(rt, KeyBackspace, 0)
	want(3, "Backspace")
	edKey(rt, KeyEnter, 0)
	want(4, "Enter")
	rt.SelectAll()
	rt.ToggleBold()
	want(5, "жирный на выделении")
	rt.ToggleBold()
	want(6, "снятие жирного")
	rt.SetCaretPosition(0)
	rt.ToggleBold()
	want(6, "жирный без выделения (стиль набора) — текст не менялся")
	edKey(rt, KeyZ, ModCtrl)
	want(7, "отмена")
	edKey(rt, KeyY, ModCtrl)
	want(8, "возврат")
	rt.SetCaretPosition(0)
	rt.Select(0, 2)
	rt.Copy()
	want(8, "копирование")
	edKey(rt, KeyX, ModCtrl)
	want(9, "вырезание")
	edKey(rt, KeyV, ModCtrl)
	want(10, "вставка")
	rt.SetSelectionStyle(func(r *RichRun) {})
	want(10, "стиль без видимых изменений")
	rt.SetParagraphs([]RichParagraph{plainPara("x")})
	rt.SetText("y")
	rt.SetHTML("<p>z</p>")
	rt.AppendRun(RichRun{Text: "!"})
	rt.AppendParagraph(plainPara("p"))
	rt.Clear()
	want(10, "программная замена содержимого")
	if edKey(rt, KeyZ, ModCtrl); calls != 10 {
		t.Errorf("пустая отмена вызвала OnChange: %d", calls)
	}
}

// ─── IME ────────────────────────────────────────────────────────────────────

func TestRichEdit_IMECompositionCommitOneUndo(t *testing.T) {
	rt := edRT(plainPara("ab"))
	rt.SetCaretPosition(2)
	calls := 0
	rt.OnChange = func() { calls++ }

	rt.IMESetComposition("に", 1)
	if rt.Text() != "abに" || rt.CaretPosition() != 3 {
		t.Fatalf("композиция: %q каретка %d", rt.Text(), rt.CaretPosition())
	}
	runs := edRuns(rt, 0)
	if len(runs) != 2 || !runs[1].Underline || runs[0].Underline {
		t.Fatalf("композиция не подчёркнута отдельным раном: %+v", runs)
	}
	rt.IMESetComposition("にほん", 3)
	if rt.Text() != "abにほん" {
		t.Fatalf("вторая композиция должна заменить первую: %q", rt.Text())
	}
	rt.IMESetComposition("にほ", 1) // курсор внутри композиции
	if rt.Text() != "abにほ" || rt.CaretPosition() != 3 {
		t.Fatalf("курсор внутри композиции: %q каретка %d", rt.Text(), rt.CaretPosition())
	}
	if calls != 0 {
		t.Fatalf("OnChange на промежуточных композициях: %d", calls)
	}

	rt.IMECommit("日本")
	if rt.Text() != "ab日本" || rt.CaretPosition() != 4 {
		t.Fatalf("коммит: %q каретка %d", rt.Text(), rt.CaretPosition())
	}
	if runs := edRuns(rt, 0); len(runs) != 1 || runs[0].Underline {
		t.Fatalf("после коммита осталось подчёркивание: %+v", runs)
	}
	if calls != 1 {
		t.Fatalf("OnChange на коммите: %d, ждали 1", calls)
	}
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "ab" {
		t.Fatalf("отмена коммита: %q", rt.Text())
	}
	if rt.CanUndo() {
		t.Fatal("в истории больше одной записи на весь ввод")
	}
	edKey(rt, KeyY, ModCtrl)
	if rt.Text() != "ab日本" {
		t.Fatalf("возврат коммита: %q", rt.Text())
	}
}

// Набор перед IME и после него — отдельные записи отмены.
func TestRichEdit_IMEKeepsSurroundingHistory(t *testing.T) {
	rt := edRT()
	edType(rt, "x")
	rt.IMESetComposition("あ", 1)
	rt.IMECommit("亜")
	edType(rt, "y")
	if rt.Text() != "x亜y" {
		t.Fatalf("текст %q", rt.Text())
	}
	for _, want := range []string{"x亜", "x", ""} {
		edKey(rt, KeyZ, ModCtrl)
		if rt.Text() != want {
			t.Fatalf("после отмены %q, ждали %q", rt.Text(), want)
		}
	}
	if rt.CanUndo() {
		t.Error("лишние записи в истории")
	}
}

// Композиция поверх выделения замещает его; отмена композиции возвращает
// выделение и не трогает ни историю, ни стек возврата.
func TestRichEdit_IMEOverSelectionAndCancel(t *testing.T) {
	rt := edRT(plainPara("abc"))
	edType(rt, "q") // запись в истории
	edKey(rt, KeyZ, ModCtrl)
	if !rt.CanRedo() {
		t.Fatal("нет возврата для проверки")
	}
	rt.Select(0, 2)
	rt.IMESetComposition("に", 1)
	if rt.Text() != "にc" || rt.SelectedText() != "" {
		t.Fatalf("композиция поверх выделения: %q", rt.Text())
	}
	rt.IMESetComposition("にほ", 2)
	rt.IMECancel()
	if rt.Text() != "abc" {
		t.Fatalf("отмена композиции: %q", rt.Text())
	}
	if rt.SelectedText() != "ab" {
		t.Errorf("выделение после отмены %q", rt.SelectedText())
	}
	if rt.CanUndo() || !rt.CanRedo() {
		t.Errorf("отмена композиции тронула историю: undo=%v redo=%v", rt.CanUndo(), rt.CanRedo())
	}

	rt.IMESetComposition("に", 1)
	rt.IMECommit("日")
	if rt.Text() != "日c" {
		t.Fatalf("коммит поверх выделения: %q", rt.Text())
	}
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "abc" || rt.CanUndo() {
		t.Fatalf("одна отмена должна вернуть выделенное: %q undo=%v", rt.Text(), rt.CanUndo())
	}
	// Пустая композиция — то же, что отмена.
	rt.IMESetComposition("に", 1)
	rt.IMESetComposition("", 0)
	if rt.Text() != "abc" {
		t.Errorf("пустая композиция: %q", rt.Text())
	}
}

// Коммит без композиции вставляет текст; в просмотре IME не принимается.
func TestRichEdit_IMECommitWithoutCompositionAndViewMode(t *testing.T) {
	rt := edRT(plainPara("ab"))
	rt.SetCaretPosition(1)
	rt.IMECommit("é")
	if rt.Text() != "aéb" {
		t.Fatalf("коммит без композиции: %q", rt.Text())
	}
	rt.IMECommit("")
	rt.IMECancel()
	if rt.Text() != "aéb" {
		t.Fatalf("пустой коммит и отмена без композиции: %q", rt.Text())
	}
	rt.Editable = false
	rt.IMESetComposition("に", 1)
	rt.IMECommit("日")
	if rt.Text() != "aéb" {
		t.Fatalf("просмотр принял IME: %q", rt.Text())
	}
	if !isIMEComposer(rt) {
		t.Error("RichText не реализует IMEComposer")
	}
}

func isIMEComposer(w any) bool { _, ok := w.(IMEComposer); return ok }

// Клавиша, щелчок и потеря фокуса принимают композицию как текст — она не
// остаётся висеть подчёркнутой в документе, а история получает одну запись.
func TestRichEdit_IMEFinishedByOtherInput(t *testing.T) {
	rt := edRT(plainPara("ab"))
	rt.SetFocused(true)
	rt.SetCaretPosition(2)
	rt.IMESetComposition("に", 1)
	edKey(rt, KeyEnter, 0)
	if rt.Text() != "abに\n" {
		t.Fatalf("Enter поверх композиции: %q", rt.Text())
	}
	if runs := edRuns(rt, 0); runs[len(runs)-1].Underline {
		t.Errorf("композиция осталась подчёркнутой: %+v", runs)
	}

	rt = edRT(plainPara("ab"))
	rt.SetFocused(true)
	rt.SetCaretPosition(0)
	rt.IMESetComposition("に", 1)
	rtPress(rt, 4+5*3, 8, 1)
	rtRelease(rt, 4+5*3, 8, 1)
	if rt.Text() != "にab" || edRuns(rt, 0)[0].Underline {
		t.Fatalf("щелчок во время композиции: %q %+v", rt.Text(), edRuns(rt, 0))
	}

	rt = edRT(plainPara("ab"))
	rt.SetFocused(true)
	rt.SetCaretPosition(2)
	rt.IMESetComposition("に", 1)
	rt.SetFocused(false)
	if runs := edRuns(rt, 0); rt.Text() != "abに" || len(runs) != 1 || runs[0].Underline {
		t.Fatalf("потеря фокуса во время композиции: %q %+v", rt.Text(), runs)
	}
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "ab" {
		t.Errorf("отмена принятой при потере фокуса композиции: %q", rt.Text())
	}
}

func TestRichEdit_IMECaretRectFollowsCaret(t *testing.T) {
	rt := edRT(plainPara("ab"))
	rt.SetCaretPosition(2)
	r0 := rt.IMECaretRect()
	rt.IMESetComposition("にほん", 3)
	r1 := rt.IMECaretRect()
	if r1.Min.X <= r0.Min.X {
		t.Errorf("каретка не ушла за композицию: %v → %v", r0, r1)
	}
}

// ─── Содержимое и история ───────────────────────────────────────────────────

// Допись в журнал историю не засоряет; допись абзаца её не ломает, допись
// рана — сбрасывает (иначе отмена вернула бы абзац без дописанного).
func TestRichEdit_AppendAndHistory(t *testing.T) {
	rt := edRT(plainPara("a"))
	rt.SetCaretPosition(1)
	edType(rt, "b")
	rt.AppendParagraph(plainPara("tail"))
	if rt.CanRedo() {
		t.Error("допись создала возврат")
	}
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "a\ntail" {
		t.Fatalf("отмена после AppendParagraph: %q", rt.Text())
	}
	edKey(rt, KeyY, ModCtrl)
	if rt.Text() != "ab\ntail" {
		t.Fatalf("возврат после AppendParagraph: %q", rt.Text())
	}
	rt.AppendRun(RichRun{Text: "!"})
	if rt.CanUndo() || rt.CanRedo() {
		t.Error("AppendRun не сбросил историю, ссылающуюся на абзац")
	}
	if rt.Text() != "ab\ntail!" {
		t.Fatalf("после AppendRun: %q", rt.Text())
	}
	// И дальше редактор работает.
	rt.SetCaretPosition(len([]rune(rt.Text())))
	edType(rt, "?")
	if rt.Text() != "ab\ntail!?" {
		t.Fatalf("набор после AppendRun: %q", rt.Text())
	}
}

// «Нет абзацев» — по-прежнему пусто, а не один пустой абзац; первая правка
// создаёт абзац.
func TestRichEdit_EmptyWidgetSemantics(t *testing.T) {
	rt := edRT()
	if len(rt.Paragraphs()) != 0 {
		t.Fatalf("новый виджет: %d абзацев", len(rt.Paragraphs()))
	}
	rt.AppendParagraph(plainPara("x"))
	if ps := rt.Paragraphs(); len(ps) != 1 || rt.Text() != "x" {
		t.Fatalf("AppendParagraph в пустой: %+v", ps)
	}
	rt.Clear()
	if len(rt.Paragraphs()) != 0 || rt.Text() != "" {
		t.Fatalf("Clear: %+v", rt.Paragraphs())
	}
	rt.AppendRun(RichRun{Text: "r"})
	if ps := rt.Paragraphs(); len(ps) != 1 || rt.Text() != "r" {
		t.Fatalf("AppendRun в пустой: %+v", ps)
	}
	rt.Clear()
	edType(rt, "q")
	if ps := rt.Paragraphs(); len(ps) != 1 || rt.Text() != "q" {
		t.Fatalf("набор в пустой: %+v", ps)
	}
	// Каретка пустого виджета есть и ходит.
	rt.Clear()
	if r := rt.IMECaretRect(); r.Empty() {
		t.Error("у пустого редактора нет каретки")
	}
}

// Набор у нижнего края прокручивает к каретке.
func TestRichEdit_ScrollsToCaret(t *testing.T) {
	rt := edRT()
	rt.SetBounds(image.Rect(0, 0, 300, 60))
	for i := 0; i < 12; i++ {
		edType(rt, "line")
		edKey(rt, KeyEnter, 0)
	}
	if rt.ScrollY() == 0 {
		t.Fatal("текст ушёл за нижний край, а прокрутки нет")
	}
	rt.mu.Lock()
	r := rt.caretRectLocked()
	rt.mu.Unlock()
	if !r.In(rt.Bounds()) {
		t.Errorf("каретка %v вне виджета %v", r, rt.Bounds())
	}
}

// Скринридер: правка текста — через AccessSetText, одной отменяемой записью.
func TestRichEdit_Accessibility(t *testing.T) {
	rt := edRT(plainPara("old"))
	calls := 0
	rt.OnChange = func() { calls++ }
	if rt.AccessReadOnly() {
		t.Error("редактор объявлен только для чтения")
	}
	for _, s := range rt.AccessInfo().States {
		if s == StateReadOnly {
			t.Error("у редактора состояние «только чтение»")
		}
	}
	var setter AccessTextSetter = rt
	if !setter.AccessSetText("new\ntext") || rt.Text() != "new\ntext" || calls != 1 {
		t.Fatalf("AccessSetText: %q calls=%d", rt.Text(), calls)
	}
	edKey(rt, KeyZ, ModCtrl)
	if rt.Text() != "old" {
		t.Errorf("отмена AccessSetText: %q", rt.Text())
	}
	rt.SetEnabled(false)
	if rt.AccessSetText("x") {
		t.Error("выключенный виджет принял AccessSetText")
	}
}

// ─── Контекстное меню ───────────────────────────────────────────────────────

func TestRichEdit_ContextMenu(t *testing.T) {
	rt := edRT(plainPara("hello world"))
	if m := rt.ContextMenuAt(10, 8); m == nil {
		t.Fatal("у редактора нет контекстного меню")
	}
	if m := rt.ContextMenuAt(5000, 8); m != nil {
		t.Error("меню вне виджета")
	}
	// Правый щелчок вне выделения переносит каретку под курсор.
	rt.Select(0, 2)
	rt.ContextMenuAt(4+5*8, 8)
	if rt.CaretPosition() != 8 || rt.SelectedText() != "" {
		t.Errorf("щелчок вне выделения: каретка %d, выделено %q", rt.CaretPosition(), rt.SelectedText())
	}
	// Внутри выделения — сохраняет.
	rt.Select(2, 6)
	rt.ContextMenuAt(4+5*4, 8)
	if rt.SelectedText() != "llo " {
		t.Errorf("щелчок внутри выделения снял его: %q", rt.SelectedText())
	}
	// Своё меню приложения важнее.
	rt.SetContextMenu(NewPopupMenu())
	if m := rt.ContextMenuAt(10, 8); m != nil {
		t.Error("встроенное меню перебило меню приложения")
	}
	// Просмотру встроенное меню не положено.
	rt = newRT(300, 120, plainPara("x"))
	if m := rt.ContextMenuAt(10, 8); m != nil {
		t.Error("у просмотра появилось меню")
	}
	if len(rt.Children()) != 0 {
		t.Errorf("меню стало ребёнком виджета: %d", len(rt.Children()))
	}
}

// ─── TypeReplace (документ) ─────────────────────────────────────────────────

func TestRichDocument_TypeReplaceContinuesTyping(t *testing.T) {
	d := NewRichDocumentFromText("hello world")
	end := d.TypeReplace(6, 11, "X", RichRun{})
	end = d.Type(end, "y", RichRun{})
	end = d.Type(end, "z", RichRun{})
	if d.Text() != "hello Xyz" || end != 9 {
		t.Fatalf("текст %q, конец %d", d.Text(), end)
	}
	sel, ok := d.Undo()
	if !ok || d.Text() != "hello world" || sel != (RichDocSel{6, 11}) {
		t.Fatalf("одна отмена: %q %v ok=%v", d.Text(), sel, ok)
	}
	if d.CanUndo() {
		t.Error("в истории лишние записи")
	}
	// Без выделения — обычный Type, с абзацем — набор не продолжается.
	d = NewRichDocumentFromText("abc")
	d.TypeReplace(3, 3, "d", RichRun{})
	d.TypeReplace(4, 4, "e", RichRun{})
	d.Undo()
	if d.Text() != "abc" {
		t.Errorf("TypeReplace без выделения не склеился: %q", d.Text())
	}
	d = NewRichDocumentFromText("abc def")
	end = d.TypeReplace(0, 3, "x\ny", RichRun{})
	d.Type(end, "z", RichRun{})
	d.Undo()
	if d.Text() != "x\ny def" {
		t.Errorf("набор после замены с абзацем склеился: %q", d.Text())
	}
}

// ─── Потоки ─────────────────────────────────────────────────────────────────

// Ввод, команды панели, IME, отрисовка, дописывание и чтение — из разных
// горутин; под -race это должно быть тихо, а OnChange не должен держать замок.
func TestRichEdit_ConcurrentUse(t *testing.T) {
	rt := edRT(manyParas(6)...)
	rt.OnChange = func() { _ = rt.Text() }
	var wg sync.WaitGroup
	stop := make(chan struct{})
	run := func(f func(i int)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
					f(i)
				}
			}
		}()
	}
	run(func(i int) { rt.Draw(&rtRec{}) })
	run(func(i int) {
		edType(rt, "ab")
		edKey(rt, KeyEnter, 0)
		edKey(rt, KeyBackspace, 0)
		edKey(rt, KeyZ, ModCtrl)
		edKey(rt, KeyY, ModCtrl)
		// Меню открывает и разбирает ввод поток событий — тот же, что клавиши;
		// из потока кадра его только рисуют.
		rt.ContextMenuAt(20, 20)
	})
	run(func(i int) {
		rt.ToggleBold()
		rt.SetSelectionStyle(func(r *RichRun) { r.Underline = !r.Underline })
		rt.SetParagraphFormat(func(p *RichParagraph) { p.Indent = i % 5 })
		_ = rt.SelectionStyle()
		_ = rt.SelectionParagraphFormat()
	})
	run(func(i int) {
		rt.IMESetComposition("にほん", 2)
		rt.IMECommit("日本")
		rt.IMESetComposition("x", 1)
		rt.IMECancel()
		_ = rt.IMECaretRect()
	})
	run(func(i int) {
		rt.SelectAll()
		rt.Cut()
		rt.Paste()
		_ = rt.HTML()
		_ = rt.CanUndo()
		_ = rt.HasOverlay()
		rt.DrawOverlay(&rtRec{})
	})
	for i := 0; i < 200; i++ {
		rt.AppendParagraph(plainPara("log"))
		rt.AppendRun(RichRun{Text: "!"})
		if i%50 == 49 {
			rt.SetParagraphs(manyParas(4))
		}
	}
	close(stop)
	wg.Wait()
}
