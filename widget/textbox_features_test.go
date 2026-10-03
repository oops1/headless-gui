package widget

import (
	"image"
	"image/color"
	"math/rand"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// Возможности редактора, из-за которых его считали негодным для кода: шрифт,
// Tab и табстопы, стили на диапазонах, горизонтальная полоса, отрисовка без
// копии документа, правки вместо снимков в истории.

// ─── Вспомогательное ─────────────────────────────────────────────────────────

type tbRecText struct {
	s    string
	x, y int
	font string
	col  color.RGBA
}

type tbRecFill struct {
	x, y, w, h int
	col        color.RGBA
	alpha      bool
	round      bool
}

type tbRecLine struct {
	x, y, span int
	horiz      bool
}

// tbRecCtx — контекст отрисовки, записывающий то, что рисует редактор. Замеры
// отдаёт тем же измерителем, что раскладка, — как настоящий холст.
type tbRecCtx struct {
	DrawContext
	texts []tbRecText
	fills []tbRecFill
	lines []tbRecLine
}

func (c *tbRecCtx) FillRect(x, y, w, h int, col color.RGBA) {
	c.fills = append(c.fills, tbRecFill{x: x, y: y, w: w, h: h, col: col})
}
func (c *tbRecCtx) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	c.fills = append(c.fills, tbRecFill{x: x, y: y, w: w, h: h, col: col, alpha: true})
}
func (c *tbRecCtx) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	c.fills = append(c.fills, tbRecFill{x: x, y: y, w: w, h: h, col: col, round: true})
}
func (c *tbRecCtx) DrawBorder(x, y, w, h int, col color.RGBA)         {}
func (c *tbRecCtx) DrawRoundBorder(x, y, w, h, r int, col color.RGBA) {}
func (c *tbRecCtx) SetClip(r image.Rectangle)                         {}
func (c *tbRecCtx) ClearClip()                                        {}

// Clip — без отсечения это вся площадь холста, как у настоящего: поле сужает
// область пересечением с текущей и в конце возвращает её.
func (c *tbRecCtx) Clip() image.Rectangle { return image.Rect(0, 0, 1<<15, 1<<15) }
func (c *tbRecCtx) DrawHLine(x, y, l int, col color.RGBA) {
	c.lines = append(c.lines, tbRecLine{x: x, y: y, span: l, horiz: true})
}
func (c *tbRecCtx) DrawVLine(x, y, l int, col color.RGBA) {
	c.lines = append(c.lines, tbRecLine{x: x, y: y, span: l})
}
func (c *tbRecCtx) DrawTextSize(s string, x, y int, sz float64, col color.RGBA) {
	c.texts = append(c.texts, tbRecText{s: s, x: x, y: y, col: col})
}
func (c *tbRecCtx) DrawTextFont(s string, x, y int, sz float64, font string, col color.RGBA) {
	c.texts = append(c.texts, tbRecText{s: s, x: x, y: y, font: font, col: col})
}
func (c *tbRecCtx) MeasureText(s string, sz float64) int { return MeasureUIText(s, sz) }
func (c *tbRecCtx) MeasureTextFont(s string, sz float64, font string) int {
	return MeasureUITextFont(s, sz, font)
}

// textsOf — только строки выведенного текста.
func (c *tbRecCtx) textsOf() []string {
	out := make([]string, len(c.texts))
	for i, tx := range c.texts {
		out[i] = tx.s
	}
	return out
}

// baseTestWidth — тот же замер, что ставит useTestMeasurer.
func baseTestWidth(text string, sizePt float64) int {
	w := 0
	for _, r := range text {
		w += runeW(r)
	}
	return int(float64(w) * sizePt / 14)
}

// useFontMeasurers ставит измеритель с моноширинным шрифтом "mono" (10 px на
// руну) рядом с обычным тестовым — раскладка тогда видит, чем её меряют.
func useFontMeasurers(t testing.TB) {
	t.Helper()
	useTestMeasurer(t)
	h := RegisterMeasurers(Measurers{
		Text: baseTestWidth,
		TextFont: func(text string, sizePt float64, family string) int {
			if family == "mono" {
				return 10 * len([]rune(text))
			}
			return baseTestWidth(text, sizePt)
		},
	})
	t.Cleanup(func() { UnregisterTextMeasurer(h) })
}

func newTestTB(t testing.TB, w, h int) *TextBox {
	t.Helper()
	useTestMeasurer(t)
	return newTBHere(w, h)
}

func newTBHere(w, h int) *TextBox {
	tb := NewTextBox("")
	tb.SetBounds(image.Rect(0, 0, w, h))
	return tb
}

func key(tb *TextBox, code KeyCode, mod KeyMod) {
	tb.OnKeyEvent(KeyEvent{Code: code, Mod: mod, Pressed: true})
}

func typeRunes(tb *TextBox, s string) {
	for _, r := range s {
		if r == '\n' {
			key(tb, KeyEnter, 0)
			continue
		}
		tb.OnKeyEvent(KeyEvent{Rune: r, Pressed: true})
	}
}

// caretX — x каретки относительно левого края текста.
func caretX(tb *TextBox, pos int) int {
	tb.SetCaretPosition(pos)
	r := tb.IMECaretRect()
	return r.Min.X - tb.bounds.Min.X - tb.PaddingX
}

func draw(tb *TextBox) *tbRecCtx {
	rec := &tbRecCtx{}
	tb.Draw(rec)
	return rec
}

// ─── Выбор шрифта ────────────────────────────────────────────────────────────

// Раскладка меряет шрифтом виджета: каретка стоит там, где будут буквы.
func TestTextBox_FontNameDrivesLayout(t *testing.T) {
	useFontMeasurers(t)
	tb := newTBHere(300, 100)
	tb.Wrap = false
	tb.SetText("abcde")

	def := caretX(tb, 5)
	tb.FontName = "mono"
	if got := caretX(tb, 5); got != 50 {
		t.Fatalf("каретка в конце 'abcde' моноширинным шрифтом на %d, ждали 50 (было %d без шрифта)", got, def)
	}
	if def == 50 {
		t.Fatal("тест не различает шрифты: у обычного шрифта та же ширина")
	}
	// Вернули шрифт — раскладка вернулась.
	tb.FontName = ""
	if got := caretX(tb, 5); got != def {
		t.Errorf("после сброса шрифта каретка на %d, ждали %d", got, def)
	}
}

// Перенос по словам тоже идёт шрифтом виджета.
func TestTextBox_FontNameDrivesWrap(t *testing.T) {
	useFontMeasurers(t)
	tb := newTBHere(120, 400) // область текста 120-12-7 = 101 px
	tb.SetText("aaaa bbbb cccc dddd")
	def := tb.LineCount()
	tb.FontName = "mono" // слово из 4 знаков = 40 px, влезает два на строку
	if got := tb.LineCount(); got == def {
		t.Errorf("перенос не заметил шрифт: строк %d и там и там", got)
	}
	if got := tb.LineCount(); got != 2 {
		t.Errorf("строк %d, ждали 2 (два слова по 40 px + пробел в 101 px)", got)
	}
}

// Рисуется тем же шрифтом, каким меряется: DrawTextFont, а не DrawTextSize,
// и выделение растёт от ширины ЭТОГО шрифта.
func TestTextBox_FontNameDraws(t *testing.T) {
	useFontMeasurers(t)
	tb := newTBHere(300, 100)
	tb.Wrap = false
	tb.FontName = "mono"
	tb.SetText("hello")
	tb.selAnchor, tb.caret = 1, 4

	rec := draw(tb)
	if len(rec.texts) != 1 || rec.texts[0].font != "mono" || rec.texts[0].s != "hello" {
		t.Fatalf("текст выведен как %+v, ждали одну строку шрифтом mono", rec.texts)
	}
	var sel *tbRecFill
	for i := range rec.fills {
		if rec.fills[i].alpha {
			sel = &rec.fills[i]
		}
	}
	if sel == nil || sel.w != 30 {
		t.Fatalf("выделение %+v, ждали ширину 30 (три знака по 10 px)", sel)
	}
	if want := tb.PaddingX + 10; sel.x != want {
		t.Errorf("выделение начинается в %d, ждали %d", sel.x, want)
	}
}

// Без шрифта — как раньше: DrawTextSize.
func TestTextBox_NoFontNameDrawsDefault(t *testing.T) {
	useFontMeasurers(t)
	tb := newTBHere(300, 100)
	tb.SetText("hello")
	rec := draw(tb)
	if len(rec.texts) != 1 || rec.texts[0].font != "" || rec.texts[0].s != "hello" {
		t.Fatalf("текст выведен как %+v", rec.texts)
	}
	if rec.texts[0].x != tb.PaddingX {
		t.Errorf("текст в x=%d, ждали %d", rec.texts[0].x, tb.PaddingX)
	}
}

// Перенос по словам включают и выключают на лету (пункт меню «Формат»):
// раскладка должна заметить это без правки текста.
func TestTextBox_WrapToggleRelayouts(t *testing.T) {
	tb := newTestTB(t, 120, 300)
	tb.SetText("aaaa bbbb cccc dddd eeee ffff")
	wrapped := tb.LineCount()
	if wrapped < 2 {
		t.Fatalf("тест непригоден: перенос не случился (%d строк)", wrapped)
	}
	tb.Wrap = false
	if got := tb.LineCount(); got != 1 {
		t.Errorf("после выключения переноса строк %d, ждали 1", got)
	}
	tb.Wrap = true
	if got := tb.LineCount(); got != wrapped {
		t.Errorf("после включения переноса строк %d, ждали %d", got, wrapped)
	}
}

// ─── Tab ─────────────────────────────────────────────────────────────────────

func TestTextBox_ImplementsTabAcceptor(t *testing.T) {
	var _ TabAcceptor = (*TextBox)(nil)
	tb := newTestTB(t, 200, 100)
	tab := KeyEvent{Code: KeyTab, Pressed: true}

	if WantsTab(tb, tab) {
		t.Error("по умолчанию Tab должен уходить в смену фокуса — как у приложений, собранных раньше")
	}
	tb.AcceptTab = true
	if !WantsTab(tb, tab) {
		t.Error("с AcceptTab Tab должен доставляться редактору")
	}
	if WantsTab(tb, KeyEvent{Code: KeyTab, Mod: ModCtrl, Pressed: true}) {
		t.Error("Ctrl+Tab остаётся навигацией: иначе из редактора не выйти с клавиатуры")
	}
	tb.ReadOnly = true
	if WantsTab(tb, tab) {
		t.Error("в режиме чтения вставлять некуда — Tab снова должен уходить фокусу")
	}
}

func TestTextBox_TabInsertsTab(t *testing.T) {
	tb := newTestTB(t, 200, 100)

	// Без AcceptTab событие ничего не вставляет (как и раньше).
	key(tb, KeyTab, 0)
	if got := tb.GetText(); got != "" {
		t.Fatalf("без AcceptTab вставилось %q", got)
	}

	tb.AcceptTab = true
	typeRunes(tb, "a")
	key(tb, KeyTab, 0)
	typeRunes(tb, "b")
	if got := tb.GetText(); got != "a\tb" {
		t.Fatalf("текст %q, ждали a<TAB>b", got)
	}
	if got := tb.CaretPosition(); got != 3 {
		t.Errorf("каретка %d, ждали 3", got)
	}

	// Ctrl+Tab и Shift+Tab текст не меняют.
	key(tb, KeyTab, ModCtrl)
	key(tb, KeyTab, ModShift)
	if got := tb.GetText(); got != "a\tb" {
		t.Errorf("Ctrl/Shift+Tab изменили текст: %q", got)
	}

	// Tab заменяет выделение и отменяется как обычная правка.
	key(tb, KeyA, ModCtrl)
	key(tb, KeyTab, 0)
	if got := tb.GetText(); got != "\t" {
		t.Fatalf("Tab поверх выделения: %q", got)
	}
	key(tb, KeyZ, ModCtrl)
	if got := tb.GetText(); got != "a\tb" {
		t.Errorf("Ctrl+Z после Tab: %q", got)
	}

	tb.ReadOnly = true
	key(tb, KeyTab, 0)
	if got := tb.GetText(); got != "a\tb" {
		t.Errorf("в режиме чтения вставилась табуляция: %q", got)
	}
}

// Табуляция выравнивает по сетке, кратной ширине пробела, а не занимает место
// одного знака.
func TestTextBox_TabStopsInLayout(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 100)
	tb.Wrap = false
	tb.TabSize = 8
	tb.SetText("a\tb\t\tc")

	sp := MeasureUIText(" ", tb.fontSize())
	tab := 8 * sp
	aW := MeasureUIText("a", tb.fontSize())
	if aW >= tab {
		t.Fatalf("тест непригоден: 'a' (%d) не уже табстопа (%d)", aW, tab)
	}
	bW := MeasureUIText("b", tb.fontSize())

	if got := caretX(tb, 2); got != tab {
		t.Errorf("после 'a'+Tab каретка на %d, ждали табстоп %d", got, tab)
	}
	if got := caretX(tb, 3); got != tab+bW {
		t.Errorf("после 'b' каретка на %d, ждали %d", got, tab+bW)
	}
	// Две табуляции подряд — две ячейки (b оказывается внутри второй).
	wantAfterTabs := (((tab+bW)/tab)+1)*tab + tab
	if got := caretX(tb, 5); got != wantAfterTabs {
		t.Errorf("после двух табуляций каретка на %d, ждали %d", got, wantAfterTabs)
	}
}

// Щелчок в строку с табуляцией попадает в нужную колонку.
func TestTextBox_TabStopsHitTest(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 100)
	tb.Wrap = false
	tb.TabSize = 8
	tb.SetText("\tx")
	tab := 8 * MeasureUIText(" ", tb.fontSize())

	// Середина табуляции делит её между «до» и «после».
	click := func(dx int) int {
		tb.mu.Lock()
		defer tb.mu.Unlock()
		return tb.charIndexAtPoint(tb.bounds.Min.X+tb.PaddingX+dx, tb.bounds.Min.Y+tb.PaddingY+2)
	}
	if got := click(tab/2 - 2); got != 0 {
		t.Errorf("щелчок в левой половине табуляции дал колонку %d, ждали 0", got)
	}
	if got := click(tab/2 + 2); got != 1 {
		t.Errorf("щелчок в правой половине табуляции дал колонку %d, ждали 1", got)
	}
}

// Табуляция при отрисовке: куски между табуляциями выводятся каждый в своём
// табстопе.
func TestTextBox_TabStopsDraw(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 100)
	tb.Wrap = false
	tb.TabSize = 8
	tb.SetText("a\tb")
	tab := 8 * MeasureUIText(" ", tb.fontSize())

	rec := draw(tb)
	got := rec.textsOf()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("выведено %q, ждали два куска «a» и «b» по обе стороны табуляции", got)
	}
	if rec.texts[0].x != tb.PaddingX {
		t.Errorf("'a' в x=%d, ждали %d", rec.texts[0].x, tb.PaddingX)
	}
	if want := tb.PaddingX + tab; rec.texts[1].x != want {
		t.Errorf("'b' в x=%d, ждали на табстопе %d", rec.texts[1].x, want)
	}
}

// Без TabSize и AcceptTab табуляция рисуется как раньше, одной строкой.
func TestTextBox_TabLegacyWithoutTabSize(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 100)
	tb.Wrap = false
	tb.SetText("a\tb")
	rec := draw(tb)
	if got := rec.textsOf(); len(got) != 1 || got[0] != "a\tb" {
		t.Fatalf("выведено %q, ждали прежнюю одну строку", got)
	}
	if got, want := caretX(tb, 2), MeasureUIText("a\t", tb.fontSize()); got != want {
		t.Errorf("каретка %d, прежний замер %d", got, want)
	}
}

// AcceptTab без TabSize — табстоп в четыре пробела.
func TestTextBox_AcceptTabImpliesDefaultTabSize(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 100)
	tb.Wrap = false
	tb.AcceptTab = true
	tb.SetText("\tx")
	if got, want := caretX(tb, 1), 4*MeasureUIText(" ", tb.fontSize()); got != want {
		t.Errorf("табуляция шириной %d, ждали 4 пробела = %d", got, want)
	}
}

// ─── Стили на диапазонах ─────────────────────────────────────────────────────

var (
	colRed  = color.RGBA{R: 255, A: 255}
	colBlue = color.RGBA{B: 255, A: 255}
	colBG   = color.RGBA{R: 10, G: 20, B: 30, A: 255}
)

// testStyler — Styler на функции, считающий вызовы.
type testStyler struct {
	mu    sync.Mutex
	calls []styleCall
	fn    func(line int, text string) []Span
}

type styleCall struct {
	line int
	text string
}

func (s *testStyler) LineSpans(line int, text string) []Span {
	s.mu.Lock()
	s.calls = append(s.calls, styleCall{line, text})
	s.mu.Unlock()
	if s.fn == nil {
		return nil
	}
	return s.fn(line, text)
}

func (s *testStyler) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func TestTextBox_StylerColorsAndBackground(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 100)
	tb.SetText("abcdef")
	tb.SetStyler(&testStyler{fn: func(line int, text string) []Span {
		return []Span{
			{From: 1, To: 3, Style: Style{Color: colRed}},
			{From: 3, To: 5, Style: Style{BG: colBG}},
		}
	}})

	rec := draw(tb)
	got := rec.textsOf()
	if strings.Join(got, "|") != "a|bc|de|f" {
		t.Fatalf("строка разрезана как %q, ждали a|bc|de|f", got)
	}
	if rec.texts[0].col != tb.TextColor || rec.texts[3].col != tb.TextColor {
		t.Errorf("куски без стиля должны рисоваться цветом виджета: %v %v", rec.texts[0].col, rec.texts[3].col)
	}
	if rec.texts[1].col != colRed {
		t.Errorf("'bc' цвета %v, ждали красный", rec.texts[1].col)
	}
	if rec.texts[2].col != tb.TextColor {
		t.Errorf("'de' только с фоном — цвет букв должен остаться цветом виджета, а он %v", rec.texts[2].col)
	}
	var bgFill *tbRecFill
	for i := range rec.fills {
		if rec.fills[i].col == colBG {
			bgFill = &rec.fills[i]
		}
	}
	if bgFill == nil {
		t.Fatal("фон диапазона не нарисован")
	}
	// Фон — ровно под 'de': от конца 'abc' до конца 'abcde'.
	x0 := tb.PaddingX + MeasureUIText("abc", tb.fontSize())
	x1 := tb.PaddingX + MeasureUIText("abcde", tb.fontSize())
	if bgFill.x != x0 || bgFill.x+bgFill.w != x1 {
		t.Errorf("фон [%d,%d), ждали [%d,%d)", bgFill.x, bgFill.x+bgFill.w, x0, x1)
	}
}

// Начертание — именем зарегистрированного шрифта.
func TestTextBox_StylerFaceUsesNamedFont(t *testing.T) {
	useFontMeasurers(t)
	tb := newTBHere(300, 100)
	tb.FontName = "mono"
	tb.SetText("func main")
	tb.SetStyler(&testStyler{fn: func(line int, text string) []Span {
		return []Span{{From: 0, To: 4, Style: Style{Face: "mono-bold", Color: colBlue}}}
	}})
	rec := draw(tb)
	if len(rec.texts) != 2 {
		t.Fatalf("выведено %q, ждали два куска", rec.textsOf())
	}
	if rec.texts[0].font != "mono-bold" || rec.texts[0].col != colBlue {
		t.Errorf("'func': шрифт %q цвет %v", rec.texts[0].font, rec.texts[0].col)
	}
	if rec.texts[1].font != "mono" {
		t.Errorf("остаток строки: шрифт %q, ждали шрифт виджета mono", rec.texts[1].font)
	}
	// Позиция второго куска считается шрифтом виджета (4 знака по 10 px).
	if want := tb.PaddingX + 40; rec.texts[1].x != want {
		t.Errorf("второй кусок в x=%d, ждали %d", rec.texts[1].x, want)
	}
}

// Styler получает ЛОГИЧЕСКИЕ строки (по '\n') и их полный текст — перенос по
// словам на него не влияет; диапазоны абзаца попадают на нужную видимую строку.
func TestTextBox_StylerLogicalLinesAcrossWrap(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(110, 400) // область текста 91 px — "aaaa bbbb cccc" не влезает
	tb.SetText("x\naaaa bbbb cccc dddd eeee\ny")
	if tb.LineCount() < 4 {
		t.Fatalf("тест непригоден: перенос не случился (%d строк)", tb.LineCount())
	}
	st := &testStyler{fn: func(line int, text string) []Span {
		if line != 1 {
			return nil
		}
		i := strings.Index(text, "cccc")
		return []Span{{From: i, To: i + 4, Style: Style{Color: colRed}}}
	}}
	tb.SetStyler(st)
	rec := draw(tb)

	seen := map[int]string{}
	for _, c := range st.calls {
		seen[c.line] = c.text
	}
	if len(seen) != 3 || seen[0] != "x" || seen[1] != "aaaa bbbb cccc dddd eeee" || seen[2] != "y" {
		t.Fatalf("Styler спрашивали про %v, ждали три логические строки с полным текстом", seen)
	}
	if len(st.calls) != 3 {
		t.Errorf("Styler позван %d раз, ждали по разу на логическую строку (3)", len(st.calls))
	}
	var red []string
	for _, tx := range rec.texts {
		if tx.col == colRed {
			red = append(red, tx.s)
		}
	}
	if len(red) != 1 || red[0] != "cccc" {
		t.Errorf("красным выведено %q, ждали одно «cccc»", red)
	}
}

// Спрашиваются только видимые строки: разбор языка на весь документ на кадр —
// ровно то, чего кэш и построчный опрос должны избежать.
func TestTextBox_StylerAsksOnlyVisibleLines(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 100)
	tb.Wrap = false
	tb.SetText(strings.Repeat("строка\n", 5000))
	st := &testStyler{}
	tb.SetStyler(st)
	draw(tb)
	if n := st.count(); n == 0 || n > tb.visibleLines()+2 {
		t.Fatalf("Styler позван %d раз при %d видимых строках", n, tb.visibleLines())
	}
}

// Ответ приложения кэшируется: без этого разбор языка шёл бы на каждый кадр.
func TestTextBox_StylerCacheAndInvalidation(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 100)
	tb.SetText("one\ntwo")
	st := &testStyler{}
	tb.SetStyler(st)

	draw(tb)
	first := st.count()
	if first != 2 {
		t.Fatalf("первый кадр: %d вызовов, ждали 2", first)
	}
	draw(tb)
	draw(tb)
	if st.count() != first {
		t.Fatalf("повторные кадры без правок опрашивают Styler: %d вызовов вместо %d", st.count(), first)
	}

	// Правка текста сбрасывает кэш.
	tb.SetCaretPosition(0)
	typeRunes(tb, "z")
	draw(tb)
	if st.count() == first {
		t.Fatal("после правки текста Styler не спрошен заново")
	}
	got := map[string]bool{}
	for _, c := range st.calls[first:] {
		got[c.text] = true
	}
	if !got["zone"] || !got["two"] {
		t.Errorf("после правки Styler получил %v, ждали «zone» и «two»", got)
	}

	// Приложение сообщает об изменении правил.
	n := st.count()
	draw(tb)
	if st.count() != n {
		t.Fatal("кадр без перемен опросил Styler")
	}
	tb.InvalidateStyles()
	draw(tb)
	if st.count() == n {
		t.Error("после InvalidateStyles Styler не спрошен заново")
	}

	// nil снимает подсветку.
	tb.SetStyler(nil)
	n = st.count()
	rec := draw(tb)
	if st.count() != n || len(rec.texts) != 2 {
		t.Errorf("со снятым Styler: вызовов %d (было %d), строк выведено %d", st.count(), n, len(rec.texts))
	}
}

// Styler — чужой код: его зовут без замка, и он вправе заглянуть в сам виджет.
// Под замком это был бы тупик.
func TestTextBox_StylerMayCallBackIntoWidget(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 100)
	tb.SetText("hello")
	tb.SetStyler(&testStyler{fn: func(line int, text string) []Span {
		_ = tb.GetText()
		_ = tb.CaretPosition()
		_ = tb.LineCount()
		return nil
	}})
	done := make(chan struct{})
	go func() {
		draw(tb)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Draw завис: Styler вызван под замком виджета")
	}
}

// Ответ, полученный для прежнего текста, в кэш не попадает: Styler зовётся без
// замка, и текст успевает смениться.
func TestTextBox_StaleStylerAnswerIsNotCached(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 100)
	tb.SetText("old")
	edited := false
	st := &testStyler{}
	st.fn = func(line int, text string) []Span {
		if !edited {
			edited = true
			tb.SetText("a new text") // правка, пока Draw ждёт ответа
			_ = tb.LineCount()       // и раскладка успела пройти: кэш уже сброшен
		}
		return []Span{{From: 0, To: 1, Style: Style{Color: colRed}}}
	}
	tb.SetStyler(st)
	draw(tb)
	n := st.count()

	rec := draw(tb)
	if st.count() == n {
		t.Fatal("устаревший ответ осел в кэше: новый текст нарисован без вопроса к Styler")
	}
	if got := rec.textsOf(); strings.Join(got, "|") != "a| new text" {
		t.Errorf("новый текст выведен как %q", got)
	}
}

// ─── Горизонтальная полоса ───────────────────────────────────────────────────

const hbarLong = "mmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmm"

func newHBarTB(t testing.TB) *TextBox {
	tb := newTestTB(t, 200, 100)
	tb.Wrap = false
	tb.SetText(hbarLong + "\nshort")
	return tb
}

func thumbOf(tb *TextBox) image.Rectangle {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.ensureLayout()
	return tb.hbarThumbLocked()
}

func scrollXOf(tb *TextBox) int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.scrollX
}

func maxScrollXOf(tb *TextBox) int {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.ensureLayout()
	return tbScrollXMax(tb.contentWidth(), tb.textAreaW())
}

func TestTextBox_HBarShownOnlyWhenOverflowing(t *testing.T) {
	tb := newHBarTB(t)
	linesNoBar := func(tb *TextBox) int { return (tb.bounds.Dy() - 2*tb.PaddingY) / tb.lineHeight() }

	tb.mu.Lock()
	tb.ensureLayout()
	shown, vis := tb.hbarShown(), tb.visibleLines()
	tb.mu.Unlock()
	if !shown {
		t.Fatal("длинная строка без переноса, а полосы нет")
	}
	if want := (tb.bounds.Dy() - 2*tb.PaddingY - tbHBarH) / tb.lineHeight(); vis != want {
		t.Errorf("видимых строк %d, ждали %d (полоса отнимает высоту)", vis, want)
	}
	if thumbOf(tb).Empty() {
		t.Error("у показанной полосы нет ползунка")
	}

	// Короткий текст — полосы нет, высота прежняя.
	tb.SetText("short\nlines")
	tb.mu.Lock()
	tb.ensureLayout()
	shown, vis = tb.hbarShown(), tb.visibleLines()
	tb.mu.Unlock()
	if shown || vis != linesNoBar(tb) {
		t.Errorf("короткий текст: полоса %v, строк %d (ждали без полосы, %d)", shown, vis, linesNoBar(tb))
	}

	// С переносом по словам полоса никогда не нужна.
	tb.Wrap = true
	tb.SetText(hbarLong)
	tb.mu.Lock()
	tb.ensureLayout()
	shown = tb.hbarShown()
	tb.mu.Unlock()
	if shown {
		t.Error("с переносом по словам показана горизонтальная полоса")
	}
}

func TestTextBox_HBarDrawn(t *testing.T) {
	tb := newHBarTB(t)
	rec := draw(tb)
	th := thumbOf(tb)
	found := false
	for _, f := range rec.fills {
		if f.round && f.x == th.Min.X && f.y == th.Min.Y && f.w == th.Dx() && f.h == th.Dy() {
			found = true
		}
	}
	if !found {
		t.Fatalf("ползунок %v не нарисован; заливки: %+v", th, rec.fills)
	}

	tb.SetText("short")
	rec = draw(tb)
	for _, f := range rec.fills {
		if f.round && f.h == tbHBarH-4 {
			t.Fatalf("полоса нарисована у короткого текста: %+v", f)
		}
	}
}

func mouse(tb *TextBox, x, y int, pressed bool) {
	tb.OnMouseButton(MouseEvent{X: x, Y: y, Button: MouseLeft, Pressed: pressed})
}

// Щелчок мимо ползунка — прыжок; перетаскивание ползунка — плавный сдвиг, и
// ни то ни другое не ставит каретку в текст под полосой.
func TestTextBox_HBarClickAndDrag(t *testing.T) {
	tb := newHBarTB(t)
	tb.SetCaretPosition(0)
	max := maxScrollXOf(tb)
	if max <= 0 {
		t.Fatalf("тест непригоден: прокручивать нечего (max=%d)", max)
	}
	th := thumbOf(tb)
	y := th.Min.Y + th.Dy()/2

	// Щелчок правее ползунка — прыжок вправо.
	mouse(tb, th.Max.X+60, y, true)
	jumped := scrollXOf(tb)
	if jumped <= 0 {
		t.Fatalf("щелчок по треку не сдвинул текст (scrollX=%d)", jumped)
	}
	mouse(tb, th.Max.X+60, y, false)
	if got := tb.CaretPosition(); got != 0 {
		t.Errorf("щелчок по полосе сдвинул каретку в %d", got)
	}

	// Перетаскивание ползунка: взялись за него — текст едет вслед без скачка.
	tb.SetCaretPosition(0)
	tb.mu.Lock()
	tb.scrollX = 0
	tb.mu.Unlock()
	th = thumbOf(tb)
	grabX := th.Min.X + 3
	mouse(tb, grabX, y, true)
	if got := scrollXOf(tb); got != 0 {
		t.Fatalf("нажатие на ползунок сдвинуло текст на %d", got)
	}
	tb.OnMouseMove(grabX+40, y)
	mid := scrollXOf(tb)
	if mid <= 0 {
		t.Fatalf("перетаскивание не сдвинуло текст")
	}
	tb.OnMouseMove(grabX+5000, y) // далеко за правым краем
	if got := scrollXOf(tb); got != max {
		t.Errorf("перетаскивание за край дало scrollX=%d, ждали упор %d", got, max)
	}
	mouse(tb, grabX+5000, y, false)
	tb.OnMouseMove(grabX, y) // после отпускания ползунок не ведётся
	if got := scrollXOf(tb); got != max {
		t.Errorf("после отпускания текст сдвинулся на %d", got)
	}
	if got := tb.CaretPosition(); got != 0 {
		t.Errorf("перетаскивание ползунка сдвинуло каретку в %d", got)
	}
	if tb.selAnchor >= 0 {
		t.Error("перетаскивание ползунка начало выделение текста")
	}
}

// Горизонтальная дельта колеса двигает текст; у края событие отдаётся
// родителю; Shift уводит вертикальное колесо вбок.
func TestTextBox_HorizontalWheel(t *testing.T) {
	tb := newHBarTB(t)
	max := maxScrollXOf(tb)

	if !tb.OnMouseWheelPixels(50, 50, 30, 0) {
		t.Fatal("горизонтальная дельта не принята")
	}
	if got := scrollXOf(tb); got != 30 {
		t.Fatalf("scrollX=%d, ждали 30", got)
	}
	tb.OnMouseWheelPixels(50, 50, -100, 0)
	if got := scrollXOf(tb); got != 0 {
		t.Errorf("scrollX=%d после прокрутки влево до упора", got)
	}
	if tb.OnMouseWheelPixels(50, 50, -5, 0) {
		t.Error("у левого края дельта влево принята — должна уйти родителю")
	}
	tb.OnMouseWheelPixels(50, 50, 99999, 0)
	if got := scrollXOf(tb); got != max {
		t.Errorf("scrollX=%d, ждали упор %d", got, max)
	}
	if tb.OnMouseWheelPixels(50, 50, 5, 0) {
		t.Error("у правого края дельта вправо принята")
	}

	// Shift + вертикальная дельта = вбок.
	tb.mu.Lock()
	tb.scrollX = 0
	tb.mu.Unlock()
	if !tb.OnMouseWheelPixelsMod(50, 50, 0, 25, ModShift) {
		t.Fatal("Shift+колесо не принято")
	}
	if got := scrollXOf(tb); got != 25 {
		t.Errorf("Shift+колесо: scrollX=%d, ждали 25", got)
	}

	// Тиковое колесо с Shift.
	tb.mu.Lock()
	tb.scrollX = 0
	tb.mu.Unlock()
	tb.OnMouseButton(MouseEvent{X: 50, Y: 50, Button: MouseWheelDown, Pressed: true, Mod: ModShift})
	if got := scrollXOf(tb); got <= 0 {
		t.Errorf("тиковое Shift+колесо не сдвинуло текст (scrollX=%d)", got)
	}
	if tb.ScrollTop() != 0 {
		t.Error("тиковое Shift+колесо прокрутило и по вертикали")
	}

	// С переносом по словам горизонтальной прокрутки нет.
	tb.Wrap = true
	tb.SetText(hbarLong)
	if tb.OnMouseWheelPixels(50, 50, 30, 0) {
		t.Error("с переносом по словам горизонтальная дельта принята")
	}
}

// Смещение не уходит за конец текста: раньше предела не было.
func TestTextBox_ScrollXClamped(t *testing.T) {
	tb := newHBarTB(t)
	max := maxScrollXOf(tb)
	tb.mu.Lock()
	tb.scrollX = 100000
	tb.clampScrollX()
	got := tb.scrollX
	tb.mu.Unlock()
	if got != max {
		t.Errorf("scrollX=%d, ждали зажим в %d", got, max)
	}
	// Каретка в конце самой длинной строки видна, и это максимум.
	tb.SetCaretPosition(len([]rune(hbarLong)))
	if got := scrollXOf(tb); got != max {
		t.Errorf("каретка в конце длинной строки: scrollX=%d, ждали %d", got, max)
	}
	// Удалили длинную строку — смещение вернулось.
	key(tb, KeyA, ModCtrl)
	key(tb, KeyDelete, 0)
	if got := scrollXOf(tb); got != 0 {
		t.Errorf("после удаления текста scrollX=%d", got)
	}
}

// Курсор над полосой — стрелка, над текстом — I-beam.
func TestTextBox_CursorOverHBar(t *testing.T) {
	tb := newHBarTB(t)
	th := thumbOf(tb)
	if got := tb.Cursor(th.Min.X+1, th.Min.Y+1); got != CursorArrow {
		t.Errorf("над полосой курсор %v", got)
	}
	if got := tb.Cursor(50, 20); got != CursorIBeam {
		t.Errorf("над текстом курсор %v", got)
	}
}

// ─── Отрисовка без копии документа ───────────────────────────────────────────

func bigDoc(lines int) string {
	var sb strings.Builder
	for i := 0; i < lines; i++ {
		sb.WriteString("строка кода номер ")
		sb.WriteString("x")
		sb.WriteByte('\n')
	}
	return sb.String()
}

// Кадр не копирует документ: раньше Draw под замком копировал весь срез рун и
// все строки на каждый кадр.
func TestTextBox_DrawDoesNotCopyDocument(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 200)
	tb.Wrap = false
	tb.SetText(bigDoc(150000)) // ~3 млн рун — копия была бы в 12+ МБ
	draw(tb)                   // раскладка и ширина самой длинной строки — один раз

	allocated := func(f func()) uint64 {
		var a, b runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&a)
		f()
		runtime.ReadMemStats(&b)
		return b.TotalAlloc - a.TotalAlloc
	}
	got := allocated(func() {
		for i := 0; i < 5; i++ {
			draw(tb)
		}
	})
	if per := got / 5; per > 256<<10 {
		t.Fatalf("кадр выделил %d КБ — документ копируется целиком", per>>10)
	}
}

// Нажатие клавиши без обработчика OnChange не копирует документ и не строит
// снимок для истории.
func TestTextBox_KeystrokeDoesNotCopyDocument(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 200)
	tb.Wrap = false
	tb.SetText(bigDoc(60000))
	tb.SetCaretPosition(100)
	typeRunes(tb, "ab") // прогрев буферов раскладки

	var a, b runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&a)
	for i := 0; i < 5; i++ {
		typeRunes(tb, "x")
	}
	runtime.ReadMemStats(&b)
	if per := (b.TotalAlloc - a.TotalAlloc) / 5; per > 512<<10 {
		t.Fatalf("нажатие клавиши выделило %d КБ — копируется документ", per>>10)
	}
}

// Нарисованный по видимому диапазону кадр содержит те же строки, что и раньше.
func TestTextBox_DrawVisibleRangeOnly(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 100)
	tb.Wrap = false
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString("L")
		sb.WriteString(strings.Repeat("0", i%7))
		sb.WriteByte('\n')
	}
	tb.SetText(sb.String())
	tb.mu.Lock()
	tb.scrollY = 500 * tb.lineHeight()
	tb.mu.Unlock()
	rec := draw(tb)

	first := 500
	lh := tb.lineHeight()
	for _, tx := range rec.texts {
		row := (tx.y - 2 - tb.PaddingY) / lh // относительный номер строки в кадре
		want := "L" + strings.Repeat("0", (first+row)%7)
		if tx.s != want {
			t.Fatalf("строка кадра %d: %q, ждали %q", row, tx.s, want)
		}
	}
	if len(rec.texts) == 0 || len(rec.texts) > tb.visibleLines()+2 {
		t.Errorf("нарисовано %d строк при %d видимых", len(rec.texts), tb.visibleLines())
	}
}

// ─── История правок ──────────────────────────────────────────────────────────

// Правка хранится как замена, а не снимок: размер записи не зависит от
// размера документа.
func TestTextBox_UndoStoresEditsNotSnapshots(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 100)
	tb.Wrap = false
	tb.SetText(strings.Repeat("x", 400000))
	tb.SetCaretPosition(1000)
	typeRunes(tb, strings.Repeat("a", 50))
	key(tb, KeyBackspace, 0)
	key(tb, KeyDelete, 0)

	total := 0
	tb.mu.Lock()
	for _, e := range tb.undoStack {
		for _, ed := range e.edits {
			total += len(ed.removed) + len(ed.inserted)
		}
	}
	n := len(tb.undoStack)
	tb.mu.Unlock()
	if n != 52 {
		t.Fatalf("записей истории %d, ждали 52", n)
	}
	if total > 200 {
		t.Fatalf("история держит %d рун ради 52 однознаковых правок — это снимки", total)
	}
}

func TestTextBox_UndoDepthLimited(t *testing.T) {
	tb := newTestTB(t, 300, 100)
	for i := 0; i < 250; i++ {
		typeRunes(tb, "a")
	}
	tb.mu.Lock()
	n := len(tb.undoStack)
	tb.mu.Unlock()
	if n != tbUndoDepth {
		t.Fatalf("глубина истории %d, ждали %d", n, tbUndoDepth)
	}
	for i := 0; i < 300; i++ {
		key(tb, KeyZ, ModCtrl)
	}
	if got := len(tb.GetText()); got != 50 {
		t.Errorf("после отмены всей истории осталось %d знаков, ждали 50 (250-200)", got)
	}
}

// Замена всего текста снаружи обнуляет историю: записанные позиции указывают
// в прежний документ.
func TestTextBox_SetTextResetsHistory(t *testing.T) {
	tb := newTestTB(t, 300, 100)
	typeRunes(tb, "hello")
	tb.SetText("совсем другой текст")
	key(tb, KeyZ, ModCtrl)
	if got := tb.GetText(); got != "совсем другой текст" {
		t.Errorf("Ctrl+Z после SetText испортил текст: %q", got)
	}
	// Тот же текст историю не трогает (так делают привязки на каждое изменение).
	typeRunes(tb, "!")
	tb.SetText(tb.GetText())
	key(tb, KeyZ, ModCtrl)
	if got := tb.GetText(); got != "совсем другой текст" {
		t.Errorf("SetText тем же текстом сбросил историю: %q", got)
	}
}

// Историю правок нельзя отличить от прежней историю снимков: случайные
// последовательности правок, отмен и возвратов дают тот же текст и ту же
// каретку, что эталон на снимках.
func TestTextBox_UndoRedoMatchesSnapshotModel(t *testing.T) {
	useTestMeasurer(t)
	type snap struct {
		text  string
		caret int
	}
	for seed := int64(1); seed <= 150; seed++ {
		rnd := rand.New(rand.NewSource(seed))
		tb := newTBHere(300, 200)
		var undo, redo []snap
		now := func() snap { return snap{tb.GetText(), tb.CaretPosition()} }

		for step := 0; step < 80; step++ {
			before := now()
			op := rnd.Intn(14)
			isEdit := false
			isUndo, isRedo := false, false
			switch op {
			case 0, 1, 2:
				tb.OnKeyEvent(KeyEvent{Rune: rune("abc de"[rnd.Intn(6)]), Pressed: true})
				isEdit = true
			case 3:
				key(tb, KeyEnter, 0)
				isEdit = true
			case 4:
				key(tb, KeyBackspace, 0)
			case 5:
				key(tb, KeyDelete, 0)
			case 6:
				key(tb, KeyBackspace, ModCtrl)
			case 7:
				key(tb, KeyDelete, ModCtrl)
			case 8:
				key(tb, KeyLeft, KeyMod(rnd.Intn(2))*ModShift)
			case 9:
				key(tb, KeyRight, KeyMod(rnd.Intn(2))*ModShift)
			case 10:
				tb.InsertAtCaret("xyz\nq")
				isEdit = true
			case 11:
				key(tb, KeyA, ModCtrl)
			case 12:
				key(tb, KeyZ, ModCtrl)
				isUndo = true
			case 13:
				key(tb, KeyY, ModCtrl)
				isRedo = true
			}
			after := now()

			switch {
			case isUndo:
				want := before
				if n := len(undo); n > 0 {
					want = undo[n-1]
					undo = undo[:n-1]
					redo = append(redo, before)
				}
				if after != want {
					t.Fatalf("seed %d шаг %d: отмена дала %+v, снимочная модель %+v", seed, step, after, want)
				}
			case isRedo:
				want := before
				if n := len(redo); n > 0 {
					want = redo[n-1]
					redo = redo[:n-1]
					undo = append(undo, before)
				}
				if after != want {
					t.Fatalf("seed %d шаг %d: возврат дал %+v, снимочная модель %+v", seed, step, after, want)
				}
			case isEdit || after.text != before.text:
				undo = append(undo, before)
				if len(undo) > 200 {
					undo = undo[1:]
				}
				redo = nil
			}
		}
	}
}

// Набор через IME становится одной отменяемой правкой, а промежуточные виды
// композиции в историю не попадают.
func TestTextBox_IMECommitIsUndoable(t *testing.T) {
	tb := newTestTB(t, 300, 100)
	typeRunes(tb, "ab")
	tb.IMESetComposition("н", 1)
	tb.IMESetComposition("ни", 2)
	tb.IMESetComposition("нин", 3)
	tb.IMECommit("你")
	if got := tb.GetText(); got != "ab你" {
		t.Fatalf("текст %q", got)
	}
	tb.mu.Lock()
	n := len(tb.undoStack)
	tb.mu.Unlock()
	if n != 3 {
		t.Errorf("записей %d: ждали 3 (два знака и один IME-ввод)", n)
	}
	key(tb, KeyZ, ModCtrl)
	if got := tb.GetText(); got != "ab" {
		t.Errorf("после отмены ввода IME: %q", got)
	}
	key(tb, KeyY, ModCtrl)
	if got := tb.GetText(); got != "ab你" {
		t.Errorf("после возврата: %q", got)
	}
	key(tb, KeyZ, ModCtrl)
	key(tb, KeyZ, ModCtrl)
	if got := tb.GetText(); got != "a" {
		t.Errorf("отмена прежних правок после IME: %q", got)
	}
}

// ─── Потокобезопасность ──────────────────────────────────────────────────────

// Отрисовка, ввод и смена стилей из разных горутин (проверяется -race).
func TestTextBox_ConcurrentDrawEditStyle(t *testing.T) {
	useTestMeasurer(t)
	tb := newTBHere(300, 150)
	tb.Wrap = false
	tb.AcceptTab = true
	tb.SetText(strings.Repeat("alpha beta\tgamma\n", 200))
	tb.SetStyler(&testStyler{fn: func(line int, text string) []Span {
		_ = tb.GetText()
		return []Span{{From: 0, To: 2, Style: Style{Color: colRed}}}
	}})

	stop := make(chan struct{})
	var wg sync.WaitGroup
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
	run(func(int) { draw(tb) })
	run(func(i int) {
		tb.OnKeyEvent(KeyEvent{Rune: 'q', Pressed: true})
		if i%7 == 0 {
			key(tb, KeyTab, 0)
		}
		if i%5 == 0 {
			key(tb, KeyZ, ModCtrl)
		}
	})
	run(func(i int) {
		if i%2 == 0 {
			tb.InvalidateStyles()
		} else {
			tb.OnMouseWheelPixels(50, 50, 7, 3)
		}
	})
	time.Sleep(300 * time.Millisecond)
	close(stop)
	wg.Wait()
}
