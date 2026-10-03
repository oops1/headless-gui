package tests

import (
	"image"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Редактор RichText на настоящем движке: события идут тем же путём, что от
// человека (фокус кликом, клавиши, мышь, IME, буфер обмена, контекстное меню),
// а кадр рисуется настоящим холстом.

// editScene — движок с редактором в фокусе.
func editScene(t *testing.T, paras ...widget.RichParagraph) (*engine.Engine, *widget.RichText) {
	t.Helper()
	bounds := image.Rect(20, 20, 420, 140)
	eng, rt := richEngine(t, bounds, paras...)
	rt.Editable = true
	rt.CaretColor = richCaretRed
	eng.RenderOnce()
	eng.SetFocus(rt)
	return eng, rt
}

func richTypeInto(eng *engine.Engine, s string) {
	for _, r := range s {
		eng.SendKeyEvent(widget.KeyEvent{Rune: r, Pressed: true})
	}
}

func richPressKey(eng *engine.Engine, code widget.KeyCode, mod widget.KeyMod) {
	eng.SendKeyEvent(widget.KeyEvent{Code: code, Mod: mod, Pressed: true})
}

func richRunsOf(rt *widget.RichText, i int) []widget.RichRun { return rt.Paragraphs()[i].Runs }

func TestRichEditEngine_TypeUndoRedo(t *testing.T) {
	eng, rt := editScene(t)
	changes := 0
	rt.OnChange = func() { changes++ }

	richTypeInto(eng, "hello world")
	if rt.Text() != "hello world" || rt.CaretPosition() != 11 {
		t.Fatalf("после набора %q, каретка %d", rt.Text(), rt.CaretPosition())
	}
	if changes != 11 {
		t.Errorf("OnChange %d раз, ждали 11", changes)
	}
	// Набор виден на кадре.
	frame := eng.RenderOnce()
	if ink := richInk(frame, image.Rect(20, 20, 420, 140)); ink.Empty() {
		t.Fatal("набранный текст не нарисован")
	}

	richPressKey(eng, widget.KeyZ, widget.ModCtrl)
	if rt.Text() != "" {
		t.Fatalf("слово не отменилось одним Ctrl+Z: %q", rt.Text())
	}
	richPressKey(eng, widget.KeyY, widget.ModCtrl)
	if rt.Text() != "hello world" || rt.CaretPosition() != 11 {
		t.Fatalf("Ctrl+Y: %q каретка %d", rt.Text(), rt.CaretPosition())
	}
	richPressKey(eng, widget.KeyZ, widget.ModCtrl)
	richPressKey(eng, widget.KeyZ, widget.ModCtrl|widget.ModShift)
	if rt.Text() != "hello world" {
		t.Fatalf("Ctrl+Shift+Z: %q", rt.Text())
	}
}

// Каретка редактора рисуется без ShowCaret и стоит там, где её просит IME.
func TestRichEditEngine_CaretDrawnWithoutShowCaret(t *testing.T) {
	eng, rt := editScene(t, widget.RichParagraph{Runs: []widget.RichRun{{Text: "abc"}}})
	if rt.ShowCaret {
		t.Fatal("тест рассчитан на ShowCaret == false")
	}
	richTypeInto(eng, "xy")
	want := rt.IMECaretRect()
	ink := caretFrame(t, eng, image.Rect(20, 20, 420, 140))
	if ink.Empty() {
		t.Fatal("у редактора в фокусе нет каретки")
	}
	if ink.Dx() != 1 || ink.Min.X != want.Min.X || ink.Min.Y != want.Min.Y {
		t.Errorf("каретка %v, ждали у %v", ink, want)
	}
	eng.SetFocus(nil)
	noCaretFor(t, eng, image.Rect(20, 20, 420, 140), "без фокуса")
}

// Щелчок мышью даёт фокус и ставит каретку; набор идёт в это место.
func TestRichEditEngine_ClickFocusesAndPlacesCaret(t *testing.T) {
	bounds := image.Rect(20, 20, 420, 140)
	eng, rt := richEngine(t, bounds, widget.RichParagraph{Runs: []widget.RichRun{{Text: "hello"}}})
	rt.Editable = true
	eng.RenderOnce()
	// Клик правее текста первой строки — каретка в конце абзаца.
	y := 20 + 4 + 6
	eng.SendMouseButton(380, y, widget.MouseLeft, true)
	eng.SendMouseButton(380, y, widget.MouseLeft, false)
	if rt.CaretPosition() != 5 {
		t.Fatalf("каретка после щелчка %d, ждали 5", rt.CaretPosition())
	}
	richTypeInto(eng, "!")
	if rt.Text() != "hello!" {
		t.Fatalf("после щелчка и набора %q — фокус не пришёл", rt.Text())
	}
}

func TestRichEditEngine_EnterShiftEnterBackspace(t *testing.T) {
	eng, rt := editScene(t)
	richTypeInto(eng, "ab")
	richPressKey(eng, widget.KeyEnter, 0)
	richTypeInto(eng, "cd")
	richPressKey(eng, widget.KeyEnter, widget.ModShift)
	richTypeInto(eng, "ef")
	ps := rt.Paragraphs()
	if len(ps) != 2 || rt.Text() != "ab\ncd\nef" || ps[1].Runs[0].Text != "cd\nef" {
		t.Fatalf("Enter и Shift+Enter: %q %+v", rt.Text(), ps)
	}
	// Влево до начала второго абзаца и Backspace — слияние.
	rt.SetCaretPosition(3)
	richPressKey(eng, widget.KeyBackspace, 0)
	if len(rt.Paragraphs()) != 1 || rt.Text() != "abcd\nef" || rt.CaretPosition() != 2 {
		t.Fatalf("Backspace на границе: %q абзацев %d каретка %d",
			rt.Text(), len(rt.Paragraphs()), rt.CaretPosition())
	}
	richPressKey(eng, widget.KeyZ, widget.ModCtrl)
	if len(rt.Paragraphs()) != 2 || rt.Text() != "ab\ncd\nef" {
		t.Fatalf("отмена слияния: %q", rt.Text())
	}
}

// Выделение мышью (двойной щелчок по слову) и набор поверх него: замена и
// дальнейший набор — одна отмена.
func TestRichEditEngine_ReplaceSelectionByDoubleClick(t *testing.T) {
	eng, rt := editScene(t, widget.RichParagraph{Runs: []widget.RichRun{{Text: "hello brave world"}}})
	// Слово «brave»: у шрифта движка ширина символов не известна заранее,
	// поэтому берём точку по самому виджету — смещение 8 (середина слова).
	rt.SetCaretPosition(8)
	rect := rt.IMECaretRect()
	x, y := rect.Min.X, rect.Min.Y+rect.Dy()/2
	for i := 0; i < 2; i++ { // серия щелчков считается движком
		eng.SendMouseButton(x, y, widget.MouseLeft, true)
		eng.SendMouseButton(x, y, widget.MouseLeft, false)
	}
	if rt.SelectedText() != "brave" {
		t.Fatalf("двойной щелчок выделил %q", rt.SelectedText())
	}
	richTypeInto(eng, "calm")
	if rt.Text() != "hello calm world" {
		t.Fatalf("замена выделения: %q", rt.Text())
	}
	richPressKey(eng, widget.KeyZ, widget.ModCtrl)
	if rt.Text() != "hello brave world" {
		t.Fatalf("отмена замены: %q", rt.Text())
	}
}

// Ctrl+B без выделения — стиль набора, движение каретки его сбрасывает;
// на выделении Ctrl+B меняет шрифт, а Ctrl+Z возвращает; жирный курсив.
func TestRichEditEngine_BoldItalicHotkeys(t *testing.T) {
	eng, rt := editScene(t, widget.RichParagraph{Runs: []widget.RichRun{{Text: "abcdef"}}})
	rt.SetCaretPosition(6)
	richPressKey(eng, widget.KeyB, widget.ModCtrl)
	if !rt.SelectionStyle().IsBold() {
		t.Fatal("стиль набора не жирный")
	}
	richTypeInto(eng, "X")
	runs := richRunsOf(rt, 0)
	if len(runs) != 2 || runs[1].Text != "X" || runs[1].Font != widget.BuiltinFontBold {
		t.Fatalf("набор жирным: %+v", runs)
	}
	richPressKey(eng, widget.KeyLeft, 0)
	if rt.SelectionStyle().IsBold() && len(richRunsOf(rt, 0)) == 1 {
		t.Fatal("странное состояние")
	}
	richPressKey(eng, widget.KeyRight, 0)
	richPressKey(eng, widget.KeyB, widget.ModCtrl) // новый стиль набора: жирный
	richPressKey(eng, widget.KeyLeft, 0)           // движение сбрасывает его
	richTypeInto(eng, "_")
	for _, r := range richRunsOf(rt, 0) {
		if strings.Contains(r.Text, "_") && r.IsBold() {
			t.Fatalf("стиль набора пережил движение каретки: %+v", richRunsOf(rt, 0))
		}
	}

	// Выделение: жирный курсив.
	rt.SetParagraphs([]widget.RichParagraph{{Runs: []widget.RichRun{{Text: "abcdef"}}}})
	rt.Select(0, 3)
	richPressKey(eng, widget.KeyI, widget.ModCtrl)
	richPressKey(eng, widget.KeyB, widget.ModCtrl)
	runs = richRunsOf(rt, 0)
	if len(runs) != 2 || runs[0].Font != widget.BuiltinFontBoldItalic || runs[0].Text != "abc" {
		t.Fatalf("жирный курсив: %+v", runs)
	}
	st := rt.SelectionStyle()
	if !st.IsBold() || !st.IsItalic() {
		t.Fatalf("SelectionStyle: %+v", st)
	}
	richPressKey(eng, widget.KeyZ, widget.ModCtrl)
	if rt.SelectionStyle().Font != widget.BuiltinFontItalic {
		t.Fatalf("отмена жирности: %+v", richRunsOf(rt, 0))
	}
	richPressKey(eng, widget.KeyZ, widget.ModCtrl)
	if len(richRunsOf(rt, 0)) != 1 {
		t.Fatalf("отмена курсива: %+v", richRunsOf(rt, 0))
	}
	// Жирное слово рисуется: гарнитура другая, значит и закрашено больше.
	rt.SetParagraphs([]widget.RichParagraph{{Runs: []widget.RichRun{{Text: "WWWW MMMM"}}}})
	rt.SetCaretPosition(0)
	before := brightPixels(eng.RenderOnce(), image.Rect(24, 24, 416, 136))
	rt.SelectAll()
	richPressKey(eng, widget.KeyB, widget.ModCtrl)
	rt.ClearSelection()
	after := brightPixels(eng.RenderOnce(), image.Rect(24, 24, 416, 136))
	if before == 0 || after <= before {
		t.Errorf("жирный не отрисовался толще: было %d ярких точек, стало %d", before, after)
	}
}

// brightPixels — число «чернильных» точек (яркий текст на чёрном фоне) в r.
func brightPixels(img *image.RGBA, r image.Rectangle) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			if int(c.R)+int(c.G)+int(c.B) > 3*100 {
				n++
			}
		}
	}
	return n
}

func TestRichEditEngine_CutPasteAndWordHTML(t *testing.T) {
	eng, rt := editScene(t, widget.RichParagraph{Runs: []widget.RichRun{
		{Text: "bold", Font: widget.BuiltinFontBold}, {Text: " tail"}}})
	rt.Select(0, 4)
	richPressKey(eng, widget.KeyX, widget.ModCtrl)
	if rt.Text() != " tail" || widget.ClipboardGetText() != "bold" {
		t.Fatalf("вырезание: %q, буфер %q", rt.Text(), widget.ClipboardGetText())
	}
	richPressKey(eng, widget.KeyEnd, widget.ModCtrl)
	richPressKey(eng, widget.KeyV, widget.ModCtrl)
	runs := richRunsOf(rt, 0)
	if rt.Text() != " tailbold" || !runs[len(runs)-1].IsBold() {
		t.Fatalf("вставка: %q %+v", rt.Text(), runs)
	}
	richPressKey(eng, widget.KeyZ, widget.ModCtrl)
	richPressKey(eng, widget.KeyZ, widget.ModCtrl)
	if rt.Text() != "bold tail" {
		t.Fatalf("две отмены: %q", rt.Text())
	}

	// Фрагмент в духе Word: жирное слово и ссылка.
	rt.Select(0, len([]rune(rt.Text())))
	widget.SetClipboardHTML(`<html><body><!--StartFragment--><p class=MsoNormal>Say <b>hello</b> to `+
		`<a href="https://example.com/x">example</a><o:p></o:p></p><!--EndFragment--></body></html>`,
		"Say hello to example")
	richPressKey(eng, widget.KeyV, widget.ModCtrl)
	if rt.Text() != "Say hello to example" {
		t.Fatalf("вставка Word: %q", rt.Text())
	}
	var bold, link bool
	for _, r := range richRunsOf(rt, 0) {
		bold = bold || (r.Text == "hello" && r.IsBold())
		link = link || (r.Text == "example" && r.Link == "https://example.com/x")
	}
	if !bold || !link {
		t.Fatalf("оформление из Word потеряно: жирный=%v ссылка=%v %+v", bold, link, richRunsOf(rt, 0))
	}
	richPressKey(eng, widget.KeyZ, widget.ModCtrl)
	if rt.Text() != "bold tail" {
		t.Fatalf("вставка поверх выделения отменилась не одним шагом: %q", rt.Text())
	}
}

// Композиция IME доходит до редактора через движок: видна подчёркнутой,
// заменяется следующей, коммит — одна запись отмены.
func TestRichEditEngine_IME(t *testing.T) {
	eng, rt := editScene(t, widget.RichParagraph{Runs: []widget.RichRun{{Text: "ab"}}})
	rt.SetCaretPosition(2)
	changes := 0
	rt.OnChange = func() { changes++ }

	if !eng.IMEActive() {
		t.Fatal("редактор не принимает незавершённый ввод")
	}
	if _, ok := eng.CaretRect(); !ok {
		t.Fatal("место каретки неизвестно")
	}
	eng.SendComposition("に", 1)
	eng.RenderOnce()
	if rt.Text() != "abに" {
		t.Fatalf("композиция: %q", rt.Text())
	}
	if runs := richRunsOf(rt, 0); len(runs) != 2 || !runs[1].Underline {
		t.Fatalf("композиция не подчёркнута: %+v", runs)
	}
	eng.SendComposition("にほん", 3)
	eng.RenderOnce()
	if rt.Text() != "abにほん" {
		t.Fatalf("вторая композиция: %q", rt.Text())
	}
	eng.CommitComposition("日本")
	eng.RenderOnce()
	if rt.Text() != "ab日本" {
		t.Fatalf("коммит: %q", rt.Text())
	}
	if runs := richRunsOf(rt, 0); len(runs) != 1 || runs[0].Underline {
		t.Fatalf("после коммита: %+v", runs)
	}
	if changes != 1 {
		t.Errorf("OnChange %d раз, ждали 1 (на коммит)", changes)
	}
	richPressKey(eng, widget.KeyZ, widget.ModCtrl)
	if rt.Text() != "ab" || rt.CanUndo() {
		t.Fatalf("отмена ввода: %q undo=%v — одна запись на весь ввод", rt.Text(), rt.CanUndo())
	}

	eng.SendComposition("に", 1)
	eng.CancelComposition()
	eng.RenderOnce()
	if rt.Text() != "ab" {
		t.Fatalf("отмена композиции: %q", rt.Text())
	}
}

// В режиме просмотра ввод не принимается ни клавишами, ни IME, а Tab уходит
// обходу фокуса.
func TestRichEditEngine_ViewModeRejectsInput(t *testing.T) {
	bounds := image.Rect(20, 20, 420, 140)
	eng, rt := richEngine(t, bounds, widget.RichParagraph{Runs: []widget.RichRun{{Text: "view"}}})
	eng.RenderOnce()
	eng.SetFocus(rt)
	changes := 0
	rt.OnChange = func() { changes++ }
	richTypeInto(eng, "abc")
	richPressKey(eng, widget.KeyEnter, 0)
	richPressKey(eng, widget.KeyBackspace, 0)
	richPressKey(eng, widget.KeyB, widget.ModCtrl)
	richPressKey(eng, widget.KeyX, widget.ModCtrl)
	richPressKey(eng, widget.KeyV, widget.ModCtrl)
	eng.SendComposition("に", 1)
	eng.CommitComposition("日")
	eng.RenderOnce()
	if rt.Text() != "view" || changes != 0 {
		t.Fatalf("просмотр изменился: %q, OnChange %d", rt.Text(), changes)
	}
	if runs := richRunsOf(rt, 0); len(runs) != 1 || runs[0].Font != "" {
		t.Fatalf("просмотр изменил оформление: %+v", runs)
	}
	// Выделение и копирование по-прежнему работают.
	richPressKey(eng, widget.KeyA, widget.ModCtrl)
	richPressKey(eng, widget.KeyC, widget.ModCtrl)
	if widget.ClipboardGetText() != "view" {
		t.Errorf("копирование из просмотра: %q", widget.ClipboardGetText())
	}
}

// Tab: при AcceptTab идёт в текст, иначе уводит фокус; Ctrl+Tab — всегда
// навигация.
func TestRichEditEngine_AcceptTab(t *testing.T) {
	eng, rt := editScene(t, widget.RichParagraph{Runs: []widget.RichRun{{Text: "x"}}})
	richPressKey(eng, widget.KeyTab, 0)
	if rt.Text() != "x" {
		t.Fatalf("Tab без AcceptTab изменил текст: %q", rt.Text())
	}
	rt.AcceptTab = true
	eng.SetFocus(rt)
	richPressKey(eng, widget.KeyTab, 0)
	if rt.Text() != "    x" {
		t.Fatalf("Tab с AcceptTab: %q", rt.Text())
	}
	richPressKey(eng, widget.KeyTab, widget.ModCtrl)
	if rt.Text() != "    x" {
		t.Fatalf("Ctrl+Tab вставил отступ: %q", rt.Text())
	}
}

// Правая кнопка открывает меню редактора; пункты работают с клавиатуры;
// своё ContextMenu приложения важнее.
func TestRichEditEngine_ContextMenu(t *testing.T) {
	eng, rt := editScene(t, widget.RichParagraph{Runs: []widget.RichRun{{Text: "hello"}}})
	widget.ClipboardSetText("PASTED")
	x, y := 60, 20+4+6

	eng.SendMouseButton(x, y, widget.MouseRight, true)
	eng.SendMouseButton(x, y, widget.MouseRight, false)
	if !rt.HasOverlay() {
		t.Fatal("правая кнопка не открыла меню")
	}
	if r := rt.OverlayBounds(); r.Empty() {
		t.Error("у открытого меню нет границ")
	}
	eng.RenderOnce()
	// Первый доступный пункт — «Paste» (Cut и Copy без выделения отключены).
	richPressKey(eng, widget.KeyDown, 0)
	richPressKey(eng, widget.KeyEnter, 0)
	if !strings.Contains(rt.Text(), "PASTED") {
		t.Fatalf("пункт «Вставить» не вставил: %q", rt.Text())
	}
	if rt.HasOverlay() {
		t.Error("меню осталось после выбора пункта")
	}

	// Escape закрывает меню, не трогая текст.
	before := rt.Text()
	eng.SendMouseButton(x, y, widget.MouseRight, true)
	eng.SendMouseButton(x, y, widget.MouseRight, false)
	if !rt.HasOverlay() {
		t.Fatal("меню не открылось второй раз")
	}
	richPressKey(eng, widget.KeyEscape, 0)
	if rt.HasOverlay() || rt.Text() != before {
		t.Fatalf("Esc: меню=%v текст %q", rt.HasOverlay(), rt.Text())
	}

	// Меню приложения.
	app := widget.NewPopupMenu()
	app.SetItems([]widget.MenuItem{{Text: "App"}})
	rt.SetContextMenu(app)
	eng.SendMouseButton(x, y, widget.MouseRight, true)
	eng.SendMouseButton(x, y, widget.MouseRight, false)
	if rt.HasOverlay() {
		t.Error("встроенное меню перебило меню приложения")
	}
}

// Выгрузка и загрузка HTML целиком и событие изменения от команд панели.
func TestRichEditEngine_HTMLAndPanelCommands(t *testing.T) {
	eng, rt := editScene(t)
	changes := 0
	rt.OnChange = func() { changes++ }
	richTypeInto(eng, "abc def")
	rt.Select(0, 3)
	rt.ToggleBold()
	rt.SetSelectionStyle(func(r *widget.RichRun) { r.Underline = true })
	rt.SetParagraphFormat(func(p *widget.RichParagraph) { p.Align = widget.TextAlignCenter })
	if changes != 7+3 {
		t.Errorf("OnChange %d, ждали 10", changes)
	}
	html := rt.HTML()
	if !strings.Contains(html, "font-weight:bold") || !strings.Contains(html, "text-align:center") {
		t.Fatalf("HTML без оформления: %s", html)
	}
	other := widget.NewRichText()
	other.SetHTML(html)
	if other.Text() != "abc def" || !other.Paragraphs()[0].Runs[0].IsBold() ||
		other.Paragraphs()[0].Align != widget.TextAlignCenter {
		t.Fatalf("загрузка HTML: %q %+v", other.Text(), other.Paragraphs())
	}
}
