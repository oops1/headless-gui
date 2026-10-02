package widget

import (
	"image"
	"image/color"
	"strings"
	"sync"
	"testing"
)

// Тесты виджета RichText без окна: измеритель подменён предсказуемым (5 px на
// символ при 10 pt, подъём 8, спуск 2), поэтому координаты считаются в уме.
// Отступ виджета 4 px, добавка к высоте строки 0 — строка нулевого абзаца
// занимает y = 4..14.

func newRT(w, h int, paras ...RichParagraph) *RichText {
	rt := NewRichText()
	rt.measurer = fakeRichMeasurer{}
	rt.LineSpacing = 0
	rt.SetBounds(image.Rect(0, 0, w, h))
	rt.SetParagraphs(paras)
	return rt
}

// rtRec — контекст, записывающий примитивы с координатами и цветом.
type rtRec struct {
	DrawContext
	texts []rtText
	rects []rtRect
	clip  image.Rectangle
	has   bool
}

type rtText struct {
	text       string
	x, y       int
	size       float64
	font       string
	col        color.RGBA
	clipAtDraw image.Rectangle
}

type rtRect struct {
	name string
	r    image.Rectangle
	col  color.RGBA
}

func (c *rtRec) FillRect(x, y, w, h int, col color.RGBA) {
	c.rects = append(c.rects, rtRect{"FillRect", image.Rect(x, y, x+w, y+h), col})
}
func (c *rtRec) FillRectAlpha(x, y, w, h int, col color.RGBA) {
	c.rects = append(c.rects, rtRect{"FillRectAlpha", image.Rect(x, y, x+w, y+h), col})
}
func (c *rtRec) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	c.rects = append(c.rects, rtRect{"FillRoundRect", image.Rect(x, y, x+w, y+h), col})
}
func (c *rtRec) DrawBorder(x, y, w, h int, col color.RGBA) {
	c.rects = append(c.rects, rtRect{"DrawBorder", image.Rect(x, y, x+w, y+h), col})
}
func (c *rtRec) DrawTextFont(text string, x, y int, size float64, font string, col color.RGBA) {
	c.texts = append(c.texts, rtText{text, x, y, size, font, col, c.Clip()})
}
func (c *rtRec) SetClip(r image.Rectangle) { c.clip, c.has = r, true }
func (c *rtRec) ClearClip()                { c.has = false }
func (c *rtRec) Clip() image.Rectangle {
	if !c.has {
		return image.Rect(0, 0, 1<<15, 1<<15)
	}
	return c.clip
}

func (c *rtRec) textOf(s string) *rtText {
	for i := range c.texts {
		if c.texts[i].text == s {
			return &c.texts[i]
		}
	}
	return nil
}

func rtClick(rt *RichText, x, y, clicks int) {
	rt.OnMouseButton(MouseEvent{X: x, Y: y, Button: MouseLeft, Pressed: true, Clicks: clicks})
	rt.OnMouseButton(MouseEvent{X: x, Y: y, Button: MouseLeft, Pressed: false, Clicks: clicks})
}

func rtPress(rt *RichText, x, y, clicks int) {
	rt.OnMouseButton(MouseEvent{X: x, Y: y, Button: MouseLeft, Pressed: true, Clicks: clicks})
}

func rtRelease(rt *RichText, x, y, clicks int) {
	rt.OnMouseButton(MouseEvent{X: x, Y: y, Button: MouseLeft, Pressed: false, Clicks: clicks})
}

func plainPara(text string) RichParagraph { return RichParagraph{Runs: []RichRun{{Text: text}}} }

// ─── Выделение ──────────────────────────────────────────────────────────────

func TestRichText_DragSelectsRange(t *testing.T) {
	rt := newRT(200, 100, plainPara("hello world"))
	// Нажатие у левого края текста (x=4) и протяжка до пятой границы (x=4+25).
	rtPress(rt, 4, 8, 1)
	rt.OnMouseMove(4+25, 8)
	rtRelease(rt, 4+25, 8, 1)
	if got := rt.SelectedText(); got != "hello" {
		t.Fatalf("выделено %q, ждали %q", got, "hello")
	}
	// Протяжка справа налево даёт то же выделение.
	rtPress(rt, 4+55, 8, 1)
	rt.OnMouseMove(4+30, 8)
	rtRelease(rt, 4+30, 8, 1)
	if got := rt.SelectedText(); got != "world" {
		t.Fatalf("выделено %q, ждали %q", got, "world")
	}
	// Простой щелчок без протяжки снимает выделение.
	rtClick(rt, 20, 8, 1)
	if got := rt.SelectedText(); got != "" {
		t.Fatalf("после щелчка выделено %q", got)
	}
}

func TestRichText_DoubleClickWordTripleClickParagraph(t *testing.T) {
	rt := newRT(300, 100, plainPara("hello big world"), plainPara("second"))
	// «big» — рунное смещение 6..9, x = 4+6*5 .. 4+9*5; середина — 4+37.
	rtClick(rt, 4+37, 8, 2)
	if got := rt.SelectedText(); got != "big" {
		t.Fatalf("двойной щелчок выделил %q, ждали big", got)
	}
	rtClick(rt, 4+37, 8, 3)
	if got := rt.SelectedText(); got != "hello big world" {
		t.Fatalf("тройной щелчок выделил %q, ждали абзац", got)
	}
	// Тройной щелчок во втором абзаце — только он, без разделителя.
	rtClick(rt, 10, 4+10+4, 3)
	if got := rt.SelectedText(); got != "second" {
		t.Fatalf("тройной щелчок выделил %q, ждали второй абзац", got)
	}
	// Двойной щелчок на пробеле выделяет пробел, а не соседнее слово: пробел
	// «hello big» — смещение 5; левая половина пробела ближе к границе 5.
	rtClick(rt, 4+5*5+1, 8, 2)
	if got := rt.SelectedText(); got != " " {
		t.Errorf("двойной щелчок на пробеле выделил %q", got)
	}
}

// Выделение через несколько абзацев и строк с переносом.
func TestRichText_SelectionAcrossParagraphs(t *testing.T) {
	rt := newRT(200, 100, plainPara("one two"), plainPara("three"))
	rt.SelectAll()
	if got := rt.SelectedText(); got != "one two\nthree" {
		t.Fatalf("выделено %q", got)
	}
	// Протяжка вниз за последнюю строку — до конца текста.
	rtPress(rt, 4, 8, 1)
	rt.OnMouseMove(150, 90)
	rtRelease(rt, 150, 90, 1)
	if got := rt.SelectedText(); got != "one two\nthree" {
		t.Fatalf("протяжка вниз выделила %q", got)
	}
}

// ─── Копирование ────────────────────────────────────────────────────────────

func TestRichText_CopyPutsPlainAndHTML(t *testing.T) {
	UseMemoryClipboard()
	red := color.RGBA{R: 255, A: 255}
	rt := newRT(400, 200,
		RichParagraph{Runs: []RichRun{
			{Text: "Hello "},
			{Text: "bold", Font: BuiltinFontBold, Color: red},
			{Text: " "},
			{Text: "link", Link: "http://x/?a=1&b=2"},
		}},
		RichParagraph{Runs: []RichRun{{Text: "second", Size: 20, Strike: true}},
			Align: TextAlignCenter, Indent: 6},
	)
	rt.SelectAll()
	rt.OnKeyEvent(KeyEvent{Code: KeyC, Mod: ModCtrl, Pressed: true})

	if got := ClipboardGetText(); got != "Hello bold link\nsecond" {
		t.Errorf("простой текст %q", got)
	}
	html, ok := ClipboardHTML()
	if !ok {
		t.Fatal("HTML в буфер не попал")
	}
	for _, want := range []string{
		`font-weight:bold`, `color:#ff0000`,
		`<a href="http://x/?a=1&amp;b=2">`,
		`text-align:center`, `margin-left:6px`,
		`font-size:20pt`, `text-decoration:line-through`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("в HTML нет %q:\n%s", want, html)
		}
	}
	if n := strings.Count(html, "<p "); n != 2 {
		t.Errorf("абзацев в HTML %d, ждали 2", n)
	}
	// Цвет по умолчанию не выгружается: тёмная тема дала бы белый текст,
	// невидимый на странице Word.
	if strings.Contains(html, "Hello</span>") || strings.Contains(html, `style="color`) && !strings.Contains(html, "#ff0000") {
		t.Errorf("в HTML попало оформление по умолчанию:\n%s", html)
	}
}

func TestRichText_CopyPartialAndEscaping(t *testing.T) {
	UseMemoryClipboard()
	rt := newRT(400, 200, RichParagraph{Runs: []RichRun{
		{Text: "ab<c>"}, {Text: "&d\ne", Font: BuiltinFontItalic},
	}})
	rt.Select(1, 7) // "b<c>&d"
	if !rt.Copy() {
		t.Fatal("Copy вернул false при непустом выделении")
	}
	if got := ClipboardGetText(); got != "b<c>&d" {
		t.Errorf("простой текст %q", got)
	}
	html, _ := ClipboardHTML()
	if !strings.Contains(html, "b&lt;c&gt;") || !strings.Contains(html, "&amp;d") {
		t.Errorf("разметка не экранирована:\n%s", html)
	}
	if strings.Contains(html, "<c>") || strings.Contains(html, "a") && strings.Contains(html, ">a") {
		t.Errorf("в HTML попало невыделенное или сырая разметка:\n%s", html)
	}
	if !strings.Contains(html, "font-style:italic") {
		t.Errorf("курсив потерян:\n%s", html)
	}

	// Переводы строки внутри рана — <br>.
	rt.Select(5, 9)
	rt.Copy()
	if html, _ = ClipboardHTML(); !strings.Contains(html, "<br>") {
		t.Errorf("перевод строки не стал <br>:\n%s", html)
	}
}

func TestRichText_CopyWithoutSelectionKeepsClipboard(t *testing.T) {
	UseMemoryClipboard()
	ClipboardSetText("prior")
	rt := newRT(200, 100, plainPara("text"))
	if rt.Copy() {
		t.Error("Copy без выделения вернул true")
	}
	rt.OnKeyEvent(KeyEvent{Code: KeyC, Mod: ModCtrl, Pressed: true})
	if got := ClipboardGetText(); got != "prior" {
		t.Errorf("буфер затёрт: %q", got)
	}
}

// Ссылка с исполняемой схемой в HTML не выгружается.
func TestRichText_CopyDropsDangerousLinks(t *testing.T) {
	UseMemoryClipboard()
	rt := newRT(300, 100, RichParagraph{Runs: []RichRun{
		{Text: "x", Link: "java\tscript:alert(1)"},
		{Text: "y", Link: "  JavaScript:alert(1)"},
		{Text: "z", Link: "data:text/html,hi"},
		{Text: "ok", Link: "https://a/b"},
	}})
	rt.SelectAll()
	rt.Copy()
	html, _ := ClipboardHTML()
	if strings.Contains(strings.ToLower(html), "script") || strings.Contains(html, "data:") {
		t.Errorf("опасная ссылка попала в буфер:\n%s", html)
	}
	if !strings.Contains(html, `href="https://a/b"`) {
		t.Errorf("обычная ссылка потеряна:\n%s", html)
	}
}

func TestRichText_CtrlAAndKeysNeedPress(t *testing.T) {
	rt := newRT(200, 100, plainPara("abc"))
	rt.OnKeyEvent(KeyEvent{Code: KeyA, Mod: ModCtrl, Pressed: false})
	if rt.SelectedText() != "" {
		t.Error("отпускание клавиши выделило текст")
	}
	rt.OnKeyEvent(KeyEvent{Code: KeyA, Mod: ModCtrl, Pressed: true})
	if rt.SelectedText() != "abc" {
		t.Error("Ctrl+A не выделил всё")
	}
}

// ─── Ссылки ─────────────────────────────────────────────────────────────────

func TestRichText_LinkClickCallsBack(t *testing.T) {
	rt := newRT(300, 100, RichParagraph{Runs: []RichRun{
		{Text: "see "}, {Text: "here", Link: "u://1"}, {Text: " end"},
	}})
	var got []string
	rt.OnLinkClick = func(u string) { got = append(got, u) }

	// «see » — 20 px; ссылка — x = 4+20 .. 4+40.
	rtClick(rt, 4+30, 8, 1)
	if len(got) != 1 || got[0] != "u://1" {
		t.Fatalf("щелчок по ссылке: вызовы %v", got)
	}
	got = nil

	rtClick(rt, 4+5, 8, 1) // мимо ссылки
	rtClick(rt, 4+60, 8, 1)
	if len(got) != 0 {
		t.Errorf("щелчок мимо ссылки вызвал %v", got)
	}

	// Протяжка с ссылки — выделение, а не щелчок.
	rtPress(rt, 4+25, 8, 1)
	rt.OnMouseMove(4+60, 8)
	rtRelease(rt, 4+60, 8, 1)
	if len(got) != 0 {
		t.Errorf("протяжка вызвала %v", got)
	}
	if rt.SelectedText() == "" {
		t.Error("протяжка с ссылки не выделила текст")
	}

	// Нажали на ссылке, отпустили в другом месте — не щелчок.
	rtPress(rt, 4+25, 8, 1)
	rtRelease(rt, 4+5, 8, 1)
	if len(got) != 0 {
		t.Errorf("отпускание вне ссылки вызвало %v", got)
	}

	// Двойной щелчок по ссылке выделяет слово, а не открывает ссылку дважды.
	rtClick(rt, 4+30, 8, 2)
	if len(got) != 0 {
		t.Errorf("двойной щелчок вызвал %v", got)
	}

	// Отключённый виджет ссылок не открывает.
	rt.SetEnabled(false)
	rtClick(rt, 4+30, 8, 1)
	if len(got) != 0 {
		t.Errorf("отключённый виджет вызвал %v", got)
	}
}

func TestRichText_LinkClickWithoutHandlerIsHarmless(t *testing.T) {
	rt := newRT(300, 100, RichParagraph{Runs: []RichRun{{Text: "a", Link: "u"}}})
	rtClick(rt, 6, 8, 1) // OnLinkClick == nil — не паника
}

func TestRichText_CursorOverLinkIsHand(t *testing.T) {
	rt := newRT(300, 100, RichParagraph{Runs: []RichRun{
		{Text: "see "}, {Text: "here", Link: "u://1"},
	}})
	if c := rt.Cursor(4+30, 8); c != CursorHand {
		t.Errorf("над ссылкой курсор %v, ждали руку", c)
	}
	if c := rt.Cursor(4+5, 8); c != CursorIBeam {
		t.Errorf("над текстом курсор %v, ждали I-образный", c)
	}
	// Под строкой ссылки нет: рядом с буквами рука не нужна.
	if c := rt.Cursor(4+30, 80); c != CursorIBeam {
		t.Errorf("под текстом курсор %v", c)
	}
	rt.SetEnabled(false)
	if c := rt.Cursor(4+30, 8); c != CursorArrow {
		t.Errorf("у отключённого виджета курсор %v", c)
	}
}

// ─── Прокрутка ──────────────────────────────────────────────────────────────

func manyParas(n int) []RichParagraph {
	out := make([]RichParagraph, n)
	for i := range out {
		out[i] = plainPara("line " + string(rune('a'+i%26)))
	}
	return out
}

func TestRichText_ScrollbarAppearsAndWheelScrolls(t *testing.T) {
	rt := newRT(100, 60, manyParas(30)...)
	// 30 строк по 10 px + 8 px отступов = 308 > 60: полоса есть.
	if got := rt.ContentHeight(); got != 308 {
		t.Fatalf("высота содержимого %d, ждали 308", got)
	}
	rt.mu.Lock()
	rt.layoutLocked()
	bar := rt.barOn
	w := rt.lay.Width
	rt.mu.Unlock()
	if !bar {
		t.Fatal("полоса прокрутки не включилась")
	}
	if w != 100-8-richBarW {
		t.Errorf("ширина раскладки %d, ждали %d (без полосы)", w, 100-8-richBarW)
	}

	rt.OnMouseButton(MouseEvent{X: 20, Y: 20, Button: MouseWheelDown, Pressed: true})
	if got := rt.ScrollY(); got != 30 {
		t.Errorf("после тика колеса прокрутка %d, ждали 30 (три строки)", got)
	}
	rt.OnMouseButton(MouseEvent{X: 20, Y: 20, Button: MouseWheelUp, Pressed: true})
	rt.OnMouseButton(MouseEvent{X: 20, Y: 20, Button: MouseWheelUp, Pressed: true})
	if got := rt.ScrollY(); got != 0 {
		t.Errorf("вверх за предел: %d, ждали 0", got)
	}
	rt.SetScrollY(100000)
	if got := rt.ScrollY(); got != 308-60 {
		t.Errorf("вниз за предел: %d, ждали %d", got, 308-60)
	}

	// Колесо вне виджета не принимается.
	if rt.OnMouseButton(MouseEvent{X: 500, Y: 20, Button: MouseWheelDown, Pressed: true}) {
		t.Error("колесо вне виджета поглощено")
	}
}

// Нечего прокручивать — колесо уходит родителю.
func TestRichText_WheelBubblesWhenContentFits(t *testing.T) {
	rt := newRT(200, 200, plainPara("short"))
	if rt.OnMouseButton(MouseEvent{X: 20, Y: 20, Button: MouseWheelDown, Pressed: true}) {
		t.Error("колесо поглощено, хотя прокручивать нечего")
	}
	if rt.OnMouseWheelPixels(20, 20, 0, 10) {
		t.Error("точная дельта поглощена, хотя прокручивать нечего")
	}
	rt.mu.Lock()
	rt.layoutLocked()
	bar := rt.barOn
	rt.mu.Unlock()
	if bar {
		t.Error("полоса показана при помещающемся тексте")
	}
}

func TestRichText_PixelWheelAndEdges(t *testing.T) {
	rt := newRT(100, 60, manyParas(30)...)
	if !rt.OnMouseWheelPixels(20, 20, 0, 7.5) {
		t.Fatal("дельта не принята")
	}
	if rt.OnMouseWheelPixels(20, 20, 0, -7.5) && rt.ScrollY() > 1 {
		t.Errorf("прокрутка %d после возврата", rt.ScrollY())
	}
	rt.SetScrollY(0)
	if rt.OnMouseWheelPixels(20, 20, 0, -5) {
		t.Error("у верхнего края дельта вверх должна уйти родителю")
	}
	rt.ScrollToEnd()
	if rt.OnMouseWheelPixels(20, 20, 0, 5) {
		t.Error("у нижнего края дельта вниз должна уйти родителю")
	}
}

func TestRichText_ThumbDragAndTrackClick(t *testing.T) {
	rt := newRT(100, 60, manyParas(30)...)
	rt.mu.Lock()
	rt.layoutLocked()
	track, _, thumb := rt.barGeomLocked()
	maxS := rt.maxScrollLocked()
	rt.mu.Unlock()
	if thumb.Empty() || track.Empty() {
		t.Fatal("нет полосы или ползунка")
	}
	if thumb.Min.Y != track.Min.Y {
		t.Errorf("ползунок при нулевой прокрутке не у верха: %v / %v", thumb, track)
	}

	// Тянем за середину ползунка вниз на 10 px.
	x := thumb.Min.X + 2
	y0 := (thumb.Min.Y + thumb.Max.Y) / 2
	rtPress(rt, x, y0, 1)
	rt.OnMouseMove(x, y0+10)
	rtRelease(rt, x, y0+10, 1)
	span := track.Dy() - thumb.Dy()
	want := 10 * maxS / span
	if got := rt.ScrollY(); got < want-2 || got > want+2 {
		t.Errorf("после протяжки прокрутка %d, ждали около %d", got, want)
	}
	// Протяжка по полосе не должна выделять текст.
	if rt.SelectedText() != "" {
		t.Errorf("протяжка ползунка выделила текст %q", rt.SelectedText())
	}

	// Щелчок по треку ниже ползунка — прыжок вниз.
	rt.SetScrollY(0)
	rtClick(rt, x, track.Max.Y-3, 1)
	if got := rt.ScrollY(); got < maxS-maxS/5 {
		t.Errorf("щелчок у низа трека дал прокрутку %d из %d", got, maxS)
	}
}

func TestRichText_KeyboardScroll(t *testing.T) {
	rt := newRT(100, 60, manyParas(30)...)
	rt.OnKeyEvent(KeyEvent{Code: KeyDown, Pressed: true})
	if got := rt.ScrollY(); got != 10 {
		t.Errorf("Down: %d, ждали 10", got)
	}
	rt.OnKeyEvent(KeyEvent{Code: KeyPageDown, Pressed: true})
	if got := rt.ScrollY(); got != 10+50 {
		t.Errorf("PgDn: %d, ждали 60 (высота минус строка)", got)
	}
	rt.OnKeyEvent(KeyEvent{Code: KeyEnd, Pressed: true})
	if got := rt.ScrollY(); got != 308-60 {
		t.Errorf("End: %d", got)
	}
	rt.OnKeyEvent(KeyEvent{Code: KeyHome, Pressed: true})
	if got := rt.ScrollY(); got != 0 {
		t.Errorf("Home: %d", got)
	}
}

// Прокрутка зажимается, когда содержимое укоротили.
func TestRichText_ScrollClampedWhenContentShrinks(t *testing.T) {
	rt := newRT(100, 60, manyParas(30)...)
	rt.ScrollToEnd()
	rt.SetParagraphs(manyParas(8)) // 88 px при окне 60: ещё прокручивается, но не дальше 28
	if got := rt.ScrollY(); got != 28 {
		t.Errorf("прокрутка %d после замены более коротким текстом, ждали 28", got)
	}
	rt.SetParagraphs(manyParas(3)) // 38 px — помещается целиком
	if got := rt.ScrollY(); got != 0 {
		t.Errorf("прокрутка %d после замены помещающимся текстом, ждали 0", got)
	}
}

// Протяжка выделения за нижний край прокручивает текст сама.
func TestRichText_DragBelowViewAutoScrolls(t *testing.T) {
	rt := newRT(100, 60, manyParas(30)...)
	rtPress(rt, 6, 8, 1)
	for i := 0; i < 5; i++ {
		rt.OnMouseMove(30, 100+i*20)
	}
	rtRelease(rt, 30, 200, 1)
	if rt.ScrollY() == 0 {
		t.Error("текст не прокрутился при протяжке за нижний край")
	}
	if sel := rt.SelectedText(); !strings.HasPrefix(sel, "line a") || strings.Count(sel, "\n") < 5 {
		t.Errorf("выделено слишком мало: %q", sel)
	}
}

// ─── Отрисовка ──────────────────────────────────────────────────────────────

// Раны разных кеглей стоят на одной базовой линии: baseline = y + Ascent
// одинаков у всех, а верх строки — по самому высокому рану.
func TestRichText_DrawSharesBaseline(t *testing.T) {
	rt := newRT(400, 100, RichParagraph{Runs: []RichRun{
		{Text: "small"}, {Text: "BIG", Size: 30}, {Text: "tiny", Size: 5},
	}})
	rec := &rtRec{}
	rt.Draw(rec)
	small, big, tiny := rec.textOf("small"), rec.textOf("BIG"), rec.textOf("tiny")
	if small == nil || big == nil || tiny == nil {
		t.Fatalf("не все раны нарисованы: %+v", rec.texts)
	}
	baseline := func(tx *rtText) int {
		return tx.y + int(tx.size*0.8) // подъём fake-шрифта
	}
	if baseline(small) != baseline(big) || baseline(big) != baseline(tiny) {
		t.Errorf("базовые линии разные: %d, %d, %d", baseline(small), baseline(big), baseline(tiny))
	}
	// Верх строки (y=4) совпадает с верхом самого высокого рана: он не обрезан.
	if big.y != 4 {
		t.Errorf("крупный ран нарисован на y=%d, ждали 4 (верх строки)", big.y)
	}
	if baseline(big) != 4+24 {
		t.Errorf("базовая линия на %d, ждали %d", baseline(big), 4+24)
	}
	// Шаги по X — ширины предыдущих сегментов: 25, затем 3*15=45.
	if small.x != 4 || big.x != 4+25 || tiny.x != 4+25+45 {
		t.Errorf("X сегментов: %d, %d, %d", small.x, big.x, tiny.x)
	}
	if small.size != 10 || big.size != 30 {
		t.Errorf("кегли: %v, %v", small.size, big.size)
	}
}

// Цвета: ран своего цвета не задал — цвет виджета; ссылка — цвет ссылки.
func TestRichText_DrawColorsAndLinkUnderline(t *testing.T) {
	custom := color.RGBA{G: 200, A: 255}
	rt := newRT(400, 100, RichParagraph{Runs: []RichRun{
		{Text: "plain"}, {Text: "colored", Color: custom}, {Text: "link", Link: "u"},
	}})
	rt.TextColor = color.RGBA{R: 10, G: 20, B: 30, A: 255}
	rt.LinkColor = color.RGBA{B: 250, A: 255}
	rec := &rtRec{}
	rt.Draw(rec)
	if c := rec.textOf("plain").col; c != rt.TextColor {
		t.Errorf("цвет обычного рана %v", c)
	}
	if c := rec.textOf("colored").col; c != custom {
		t.Errorf("цвет заданного рана %v", c)
	}
	lk := rec.textOf("link")
	if lk.col != rt.LinkColor {
		t.Errorf("цвет ссылки %v", lk.col)
	}
	// Подчёркивание ссылки: заливка под базовой линией цветом ссылки.
	baseline := lk.y + 8
	var found bool
	for _, r := range rec.rects {
		if r.name == "FillRect" && r.col == rt.LinkColor && r.r.Min.Y > baseline && r.r.Dy() >= 1 && r.r.Dx() == 20 {
			found = true
		}
	}
	if !found {
		t.Errorf("подчёркивания ссылки нет: %+v", rec.rects)
	}
	rt.LinkUnderline = false
	rec = &rtRec{}
	rt.Draw(rec)
	for _, r := range rec.rects {
		if r.name == "FillRect" && r.col == rt.LinkColor {
			t.Errorf("подчёркивание нарисовано при LinkUnderline=false: %+v", r)
		}
	}
}

// Подчёркивание и зачёркивание: положение и толщина — по метрикам шрифта рана.
func TestRichText_DrawDecorationsFollowFontMetrics(t *testing.T) {
	for _, size := range []float64{10, 40} {
		rt := newRT(600, 200, RichParagraph{Runs: []RichRun{
			{Text: "word", Size: size, Underline: true, Strike: true},
		}})
		rec := &rtRec{}
		rt.Draw(rec)
		tx := rec.texts[0]
		fm := fakeRichMeasurer{}.FontMetrics(size, "")
		baseline := tx.y + fm.Ascent
		ul, strike, thick := richDecoration(fm.Ascent, fm.Descent)
		wantUL := image.Rect(tx.x, baseline+ul, tx.x+int(size/2)*4, baseline+ul+thick)
		wantST := image.Rect(tx.x, baseline+strike, tx.x+int(size/2)*4, baseline+strike+thick)
		var gotUL, gotST bool
		for _, r := range rec.rects {
			if r.name != "FillRect" {
				continue
			}
			gotUL = gotUL || r.r == wantUL
			gotST = gotST || r.r == wantST
		}
		if !gotUL || !gotST {
			t.Errorf("кегль %v: линии не там (ждали %v и %v): %+v", size, wantUL, wantST, rec.rects)
		}
	}
	// Крупный шрифт — линия толще: константой так не получилось бы.
	_, _, t10 := richDecoration(8, 2)
	_, _, t40 := richDecoration(32, 8)
	if t40 <= t10 {
		t.Errorf("толщина линии не растёт с кеглем: %d и %d", t10, t40)
	}
}

// Фон рана — полоса высотой в сам ран, а не в строку.
func TestRichText_DrawRunBackground(t *testing.T) {
	bg := color.RGBA{R: 200, G: 200, A: 255}
	rt := newRT(400, 100, RichParagraph{Runs: []RichRun{
		{Text: "BIG", Size: 30}, {Text: "mark", BG: bg},
	}})
	rec := &rtRec{}
	rt.Draw(rec)
	for _, r := range rec.rects {
		if r.col == bg {
			// Мелкий ран: подъём 8 + спуск 2 = 10 px, а строка — 30.
			if r.r.Dy() != 10 || r.r.Dx() != 20 {
				t.Errorf("фон рана %v, ждали 20×10", r.r)
			}
			if r.r.Min.Y != 4+24-8 {
				t.Errorf("фон рана начинается на %d, ждали %d", r.r.Min.Y, 4+24-8)
			}
			return
		}
	}
	t.Error("фон рана не нарисован")
}

// Рисуются только видимые строки, и отсечение возвращается прежним.
func TestRichText_DrawsOnlyVisibleAndRestoresClip(t *testing.T) {
	rt := newRT(100, 60, manyParas(5000)...)
	rec := &rtRec{}
	outer := image.Rect(0, 0, 1000, 1000)
	rec.SetClip(outer)
	rt.Draw(rec)
	if n := len(rec.texts); n == 0 || n > 8 {
		t.Errorf("нарисовано %d строк, ждали не больше 8 из 5000", n)
	}
	if rec.Clip() != outer {
		t.Errorf("отсечение не возвращено: %v", rec.Clip())
	}
	// Всё нарисованное — внутри области отсечения виджета минус полоса.
	for _, tx := range rec.texts {
		if !tx.clipAtDraw.In(image.Rect(0, 0, 100-richBarW, 60)) {
			t.Errorf("текст %q рисовался с отсечением %v", tx.text, tx.clipAtDraw)
		}
	}

	// После прокрутки видны другие строки.
	rt.SetScrollY(2000)
	rec = &rtRec{}
	rt.Draw(rec)
	if len(rec.texts) == 0 || rec.texts[0].text == "line a" {
		t.Errorf("после прокрутки видны не те строки: %+v", rec.texts)
	}
}

// Выделение рисуется полосой на высоту строки, под буквами.
func TestRichText_DrawSelection(t *testing.T) {
	rt := newRT(400, 100, plainPara("hello world"), plainPara("next"))
	sel := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	rt.SelColor = sel
	rt.Select(2, 7) // «llo w»
	rec := &rtRec{}
	rt.Draw(rec)
	var got image.Rectangle
	for _, r := range rec.rects {
		if r.col == sel {
			got = r.r
		}
	}
	want := image.Rect(4+10, 4, 4+35, 14)
	if got != want {
		t.Errorf("полоса выделения %v, ждали %v", got, want)
	}

	// Выделение, захватывающее перевод строки, даёт хвост.
	rt.Select(6, 12) // «world\n»
	rec = &rtRec{}
	rt.Draw(rec)
	got = image.Rectangle{}
	for _, r := range rec.rects {
		if r.col == sel && r.r.Min.Y == 4 {
			got = r.r
		}
	}
	if want := image.Rect(4+30, 4, 4+55+richSpill, 14); got != want {
		t.Errorf("полоса выделения с переводом %v, ждали %v", got, want)
	}
}

// ─── Содержимое и API ───────────────────────────────────────────────────────

func TestRichText_AppendAndText(t *testing.T) {
	rt := newRT(200, 100)
	rt.AppendRun(RichRun{Text: "Hello "})
	rt.AppendRun(RichRun{Text: "world", Font: BuiltinFontBold})
	rt.AppendParagraph(RichParagraph{Runs: []RichRun{{Text: "second\r\nline"}}})
	if got := rt.Text(); got != "Hello world\nsecond\nline" {
		t.Errorf("текст %q", got)
	}
	// Простое добавление совпадает с пересборкой с нуля.
	want := richBuildDoc(rt.Paragraphs())
	rt.mu.Lock()
	got := rt.doc
	rt.mu.Unlock()
	if string(got.runes) != string(want.runes) || len(got.paraStart) != len(want.paraStart) {
		t.Errorf("инкрементальный документ расходится с пересобранным")
	}
	for i := range want.paraStart {
		if got.paraStart[i] != want.paraStart[i] {
			t.Errorf("начало абзаца %d: %d, ждали %d", i, got.paraStart[i], want.paraStart[i])
		}
	}
	rt.Clear()
	if rt.Text() != "" {
		t.Error("Clear оставил текст")
	}
}

// SetParagraphs копирует: правка исходных срезов не меняет виджет.
func TestRichText_SetParagraphsCopies(t *testing.T) {
	runs := []RichRun{{Text: "keep"}}
	paras := []RichParagraph{{Runs: runs}}
	rt := newRT(200, 100)
	rt.SetParagraphs(paras)
	runs[0].Text = "changed"
	paras[0].Runs = nil
	if rt.Text() != "keep" {
		t.Errorf("виджет изменился вслед за исходным срезом: %q", rt.Text())
	}
	if rt.Paragraphs()[0].Runs[0].Text != "keep" {
		t.Error("Paragraphs отдал изменённое")
	}
}

func TestRichText_SetTextSplitsParagraphs(t *testing.T) {
	rt := newRT(200, 100)
	rt.SetText("a\r\nb\n\nc")
	if n := len(rt.Paragraphs()); n != 4 {
		t.Errorf("абзацев %d, ждали 4", n)
	}
	if rt.Text() != "a\nb\n\nc" {
		t.Errorf("текст %q", rt.Text())
	}
}

// Смена содержимого сбрасывает выделение, а добавление в конец — сохраняет.
func TestRichText_SelectionAcrossContentChanges(t *testing.T) {
	rt := newRT(200, 100, plainPara("abcdef"))
	rt.Select(1, 4)
	rt.AppendParagraph(plainPara("more"))
	if rt.SelectedText() != "bcd" {
		t.Errorf("добавление сбросило выделение: %q", rt.SelectedText())
	}
	rt.SetParagraphs([]RichParagraph{plainPara("xy")})
	if rt.SelectedText() != "" {
		t.Errorf("замена содержимого оставила выделение: %q", rt.SelectedText())
	}
	// Скринридер вправе промахнуться за конец текста.
	rt.AccessSetSelection(-5, 1000)
	if lo, hi := rt.AccessSelection(); lo != 0 || hi != 2 {
		t.Errorf("выделение скринридера %d..%d, ждали 0..2", lo, hi)
	}
}

// Раскладка не пересчитывается без нужды и пересчитывается при смене ширины,
// шрифта по умолчанию и содержимого.
func TestRichText_LayoutCache(t *testing.T) {
	rt := newRT(200, 100, plainPara("alpha beta gamma delta epsilon zeta"))
	rt.mu.Lock()
	l1 := rt.layoutLocked()
	l2 := rt.layoutLocked()
	rt.mu.Unlock()
	if l1 != l2 {
		t.Error("раскладка пересчитана без причины")
	}

	rt.SetBounds(image.Rect(0, 0, 60, 100))
	rt.mu.Lock()
	l3 := rt.layoutLocked()
	rt.mu.Unlock()
	if l3 == l1 || len(l3.Lines) <= len(l1.Lines) {
		t.Errorf("узкий виджет: строк %d (было %d)", len(l3.Lines), len(l1.Lines))
	}

	rt.FontSize = 20
	rt.mu.Lock()
	l4 := rt.layoutLocked()
	rt.mu.Unlock()
	if l4 == l3 || l4.Height <= l3.Height {
		t.Error("смена кегля по умолчанию не пересчитала раскладку")
	}

	rt.AppendRun(RichRun{Text: " more"})
	rt.mu.Lock()
	l5 := rt.layoutLocked()
	rt.mu.Unlock()
	if l5 == l4 {
		t.Error("смена содержимого не пересчитала раскладку")
	}
}

// ─── Доступность ────────────────────────────────────────────────────────────

func TestRichText_Accessibility(t *testing.T) {
	rt := newRT(200, 100, plainPara("one"), plainPara("two"))
	tp, ok := AccessTextOf(rt)
	if !ok {
		t.Fatal("RichText не отдаёт AccessTextProvider")
	}
	if tp.AccessText() != "one\ntwo" {
		t.Errorf("текст для скринридера %q", tp.AccessText())
	}
	if !tp.AccessReadOnly() {
		t.Error("текст должен быть только для чтения")
	}
	if a, b := tp.AccessSelection(); a != b {
		t.Errorf("выделение до выделения: %d..%d", a, b)
	}
	rt.Select(2, 5)
	if a, b := tp.AccessSelection(); a != 2 || b != 5 {
		t.Errorf("выделение %d..%d, ждали 2..5", a, b)
	}

	node := BuildAccessTree(rt, nil)
	if node.Role != RoleDocument {
		t.Errorf("роль %q, ждали документ", node.Role)
	}
	hasRO := false
	for _, s := range node.States {
		hasRO = hasRO || s == StateReadOnly
	}
	if !hasRO {
		t.Errorf("нет состояния «только чтение»: %v", node.States)
	}
	rt.SetEnabled(false)
	node = BuildAccessTree(rt, nil)
	dis := false
	for _, s := range node.States {
		dis = dis || s == StateDisabled
	}
	if !dis {
		t.Errorf("выключенный виджет без состояния disabled: %v", node.States)
	}
}

// ─── Потоки ─────────────────────────────────────────────────────────────────

// Отрисовка, события мыши, копирование и дописывание идут из разных горутин —
// под -race это должно быть тихо.
func TestRichText_ConcurrentUse(t *testing.T) {
	UseMemoryClipboard()
	rt := newRT(160, 80, manyParas(10)...)
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
		rtPress(rt, 6+i%50, 8, 1)
		rt.OnMouseMove(60, 40+i%30)
		rtRelease(rt, 60, 40, 1)
	})
	run(func(i int) { rt.SelectAll(); rt.Copy(); _ = rt.SelectedText() })
	run(func(i int) {
		rt.OnMouseButton(MouseEvent{X: 20, Y: 20, Button: MouseWheelDown, Pressed: true})
		rt.OnKeyEvent(KeyEvent{Code: KeyHome, Pressed: true})
		_ = rt.Cursor(30, 30)
	})
	run(func(i int) { rt.SetBounds(image.Rect(0, 0, 120+i%80, 80)) })

	for i := 0; i < 300; i++ {
		rt.AppendParagraph(RichParagraph{Runs: []RichRun{{Text: "log line"}, {Text: " tail", Link: "u"}}})
		rt.AppendRun(RichRun{Text: "!"})
		if i%100 == 99 {
			rt.SetParagraphs(manyParas(5))
		}
		rt.ScrollToEnd()
	}
	close(stop)
	wg.Wait()
}
