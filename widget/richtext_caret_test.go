package widget

import (
	"image"
	"image/color"
	"strings"
	"sync"
	"testing"
)

// Тесты каретки RichText без окна. Измеритель подменён (см. newRT): руна —
// 5 px при 10 pt, подъём 8, спуск 2, строка 10 px, отступ виджета 4 px. Поэтому
// смещение off на строке с началом x0 стоит в x = 4 + 5*(off-x0), а строка
// номер n (с нуля, без SpaceBefore) занимает y = 4+10n .. 14+10n.

var caretRed = color.RGBA{R: 255, A: 255}

// newCRT — newRT в режиме с кареткой (ShowCaret): простые ↑/↓/PgUp/PgDn/
// Home/End двигают каретку, а не листают текст.
func newCRT(w, h int, paras ...RichParagraph) *RichText {
	rt := newRT(w, h, paras...)
	rt.ShowCaret = true
	return rt
}

// rtKey — нажатие клавиши с модификаторами.
func rtKey(rt *RichText, code KeyCode, mods ...KeyMod) {
	var m KeyMod
	for _, x := range mods {
		m |= x
	}
	rt.OnKeyEvent(KeyEvent{Code: code, Mod: m, Pressed: true})
}

// rtCaretAt проверяет положение каретки: смещение и (x, y верха строки)
// из IMECaretRect. Высоту проверяют отдельные тесты.
func rtCaretAt(t *testing.T, rt *RichText, off, x, y int) {
	t.Helper()
	if got := rt.CaretPosition(); got != off {
		t.Fatalf("каретка %d, ждали %d", got, off)
	}
	r := rt.IMECaretRect()
	if r.Min.X != x || r.Min.Y != y {
		t.Fatalf("каретка нарисована в (%d,%d), ждали (%d,%d)", r.Min.X, r.Min.Y, x, y)
	}
}

func rtSel(t *testing.T, rt *RichText, want string) {
	t.Helper()
	if got := rt.SelectedText(); got != want {
		t.Fatalf("выделено %q, ждали %q", got, want)
	}
}

// ─── ←/→ ────────────────────────────────────────────────────────────────────

func TestRichCaret_LeftRightCharAndCollapse(t *testing.T) {
	rt := newCRT(200, 100, plainPara("hello"))
	rtKey(rt, KeyRight)
	rtKey(rt, KeyRight)
	rtCaretAt(t, rt, 2, 4+10, 4)
	rtKey(rt, KeyLeft)
	rtCaretAt(t, rt, 1, 4+5, 4)

	// Края: за начало и за конец не уходим.
	rtKey(rt, KeyLeft)
	rtKey(rt, KeyLeft)
	if rt.CaretPosition() != 0 {
		t.Errorf("каретка ушла левее начала: %d", rt.CaretPosition())
	}
	rt.SetCaretPosition(5)
	rtKey(rt, KeyRight)
	if rt.CaretPosition() != 5 {
		t.Errorf("каретка ушла правее конца: %d", rt.CaretPosition())
	}

	// Выделение [1,3): ← схлопывает в левый край, → — в правый, а не шагает
	// на единицу от активного конца.
	rt.Select(1, 3)
	rtKey(rt, KeyLeft)
	rtCaretAt(t, rt, 1, 4+5, 4)
	rtSel(t, rt, "")
	rt.Select(1, 3)
	rtKey(rt, KeyRight)
	rtCaretAt(t, rt, 3, 4+15, 4)
	rtSel(t, rt, "")
	// Каретка слева (выделили справа налево) — всё равно в края.
	rt.Select(1, 3)
	rtKey(rt, KeyRight, ModShift) // caret 4, anchor 1
	rtSel(t, rt, "ell")
	rtKey(rt, KeyLeft)
	rtCaretAt(t, rt, 1, 4+5, 4)
}

func TestRichCaret_ShiftExtendsFromAnchor(t *testing.T) {
	rt := newCRT(200, 100, plainPara("hello world"))
	rt.SetCaretPosition(2)
	rtKey(rt, KeyRight, ModShift)
	rtKey(rt, KeyRight, ModShift)
	rtSel(t, rt, "ll")
	if rt.CaretPosition() != 4 {
		t.Errorf("каретка %d, ждали 4 (активный конец)", rt.CaretPosition())
	}
	// Назад через якорь: выделение сжимается, пересекает якорь и растёт влево.
	rtKey(rt, KeyLeft, ModShift)
	rtKey(rt, KeyLeft, ModShift)
	rtSel(t, rt, "")
	rtKey(rt, KeyLeft, ModShift)
	rtSel(t, rt, "e")
	if rt.CaretPosition() != 1 {
		t.Errorf("каретка %d, ждали 1", rt.CaretPosition())
	}
	// Без Shift выделение пропадает, и якорь забыт: следующее расширение — от
	// новой каретки.
	rtKey(rt, KeyRight)
	rtKey(rt, KeyRight, ModShift)
	rtSel(t, rt, rt.Text()[2:3])
}

// ─── Ctrl+←/→ ───────────────────────────────────────────────────────────────

func TestRichCaret_CtrlWords(t *testing.T) {
	rt := newCRT(300, 100, plainPara("hello big world"), plainPara("x y"))
	for _, want := range []int{6, 10, 16, 18, 19} { // big, world, 2-й абзац, y, конец
		rtKey(rt, KeyRight, ModCtrl)
		if got := rt.CaretPosition(); got != want {
			t.Fatalf("Ctrl+→: каретка %d, ждали %d", got, want)
		}
	}
	for _, want := range []int{18, 16, 10, 6, 0} {
		rtKey(rt, KeyLeft, ModCtrl)
		if got := rt.CaretPosition(); got != want {
			t.Fatalf("Ctrl+←: каретка %d, ждали %d", got, want)
		}
	}
	// С Shift — выделяет слово; границы те же, что у двойного щелчка.
	rtKey(rt, KeyRight, ModCtrl, ModShift) // 0..6 = "hello "
	rtSel(t, rt, "hello ")
	lo, hi := rt.doc.wordBounds(7) // «big»
	rt.SetCaretPosition(lo)
	rtKey(rt, KeyRight, ModCtrl, ModShift)
	if rt.CaretPosition() != hi+1 { // слово + следующий за ним пробел
		t.Errorf("Ctrl+Shift+→ от начала слова: каретка %d, граница слова %d", rt.CaretPosition(), hi)
	}
}

func TestRichCaret_CtrlWordsEmptyParagraphAndPunct(t *testing.T) {
	rt := newCRT(300, 100, plainPara("a, b"), plainPara(""), plainPara("c"))
	// a(0) ,(1) пробел(2) b(3) | пустой абзац 5 | c 6
	rtKey(rt, KeyRight, ModCtrl)
	rtCaretAt(t, rt, 3, 4+15, 4) // знаки и пробел — «не слово», пропускаются вместе
	rtKey(rt, KeyRight, ModCtrl)
	rtCaretAt(t, rt, 5, 4, 14) // в пустом абзаце слов нет — встаём в него
	rtKey(rt, KeyRight, ModCtrl)
	rtCaretAt(t, rt, 6, 4, 24)
	rtKey(rt, KeyLeft, ModCtrl)
	rtCaretAt(t, rt, 5, 4, 14)
	rtKey(rt, KeyLeft, ModCtrl)
	rtCaretAt(t, rt, 3, 4+15, 4)
}

// ─── ↑/↓ и желаемый X ───────────────────────────────────────────────────────

func TestRichCaret_UpDownDesiredX(t *testing.T) {
	rt := newCRT(300, 100,
		plainPara("abcdefghij"), plainPara("ab"), plainPara("abcdefghij"))
	rt.ShowCaret = true
	rt.SetCaretPosition(6) // x = 4+30
	rtKey(rt, KeyDown)
	// Короткая строка: каретка в её конце.
	rtCaretAt(t, rt, 13, 4+10, 14)
	rtKey(rt, KeyDown)
	// Длинная: X восстановлен — тот же столбец 6, а не 2.
	rtCaretAt(t, rt, 14+6, 4+30, 24)
	rtKey(rt, KeyUp)
	rtKey(rt, KeyUp)
	rtCaretAt(t, rt, 6, 4+30, 4)

	// Горизонтальное движение «желаемый X» сбрасывает.
	rtKey(rt, KeyDown)
	rtKey(rt, KeyLeft) // 12: x = 4+5
	rtKey(rt, KeyDown)
	rtCaretAt(t, rt, 14+1, 4+5, 24)

	// Край: ↑ на первой строке — в начало, ↓ на последней — в конец.
	rtKey(rt, KeyUp)
	rtKey(rt, KeyUp)
	rtKey(rt, KeyUp)
	rtCaretAt(t, rt, 0, 4, 4)
	rt.SetCaretPosition(14 + 3)
	rtKey(rt, KeyDown)
	rtCaretAt(t, rt, 24, 4+50, 24)
}

func TestRichCaret_ShiftUpDownSelectsLines(t *testing.T) {
	rt := newCRT(300, 100, plainPara("abcd"), plainPara("efgh"))
	rt.SetCaretPosition(2)
	rtKey(rt, KeyDown, ModShift)
	rtSel(t, rt, "cd\nef")
	rtKey(rt, KeyUp, ModShift)
	rtSel(t, rt, "")
	rtKey(rt, KeyUp, ModShift)
	if rt.CaretPosition() != 0 {
		t.Errorf("Shift+↑ с первой строки: каретка %d, ждали 0", rt.CaretPosition())
	}
	rtSel(t, rt, "ab")
}

// В режиме просмотра простые ↑/↓ листают, как раньше; Shift+↓ выделяет.
func TestRichCaret_ViewModeKeepsScrollKeys(t *testing.T) {
	rt := newRT(100, 60, manyParas(30)...)
	rtKey(rt, KeyDown)
	if rt.ScrollY() != 10 || rt.CaretPosition() != 0 {
		t.Errorf("↓ в режиме просмотра: прокрутка %d, каретка %d", rt.ScrollY(), rt.CaretPosition())
	}
	rtKey(rt, KeyDown, ModShift)
	if rt.SelectedText() == "" {
		t.Error("Shift+↓ в режиме просмотра не выделил")
	}
	// → двигает каретку и в режиме просмотра (доступность).
	rt.ClearSelection()
	rt.SetCaretPosition(0)
	rtKey(rt, KeyRight)
	if rt.CaretPosition() != 1 {
		t.Errorf("→ в режиме просмотра: каретка %d", rt.CaretPosition())
	}
}

// ─── Home/End по визуальной строке ──────────────────────────────────────────

func TestRichCaret_HomeEndVisualLine(t *testing.T) {
	// 10 рун на строке: "aaaa bbbb " | "cccc".
	rt := newCRT(58, 100, plainPara("aaaa bbbb cccc"))
	rt.ShowCaret = true
	rt.mu.Lock()
	lay := rt.layoutLocked()
	rt.mu.Unlock()
	if len(lay.Lines) != 2 || lay.Lines[0].End != 10 || lay.Lines[1].Start != 10 {
		t.Fatalf("раскладка не та, что ждали: %+v", lay.Lines)
	}
	rt.SetCaretPosition(3)
	rtKey(rt, KeyEnd)
	// Конец первой строки — то же смещение, что начало второй; каретка
	// остаётся на первой строке.
	rtCaretAt(t, rt, 10, 4+50, 4)
	rtKey(rt, KeyEnd)
	rtCaretAt(t, rt, 10, 4+50, 4)
	rtKey(rt, KeyHome)
	rtCaretAt(t, rt, 0, 4, 4)

	// Вторая строка: Home — её начало (то же смещение 10, но на второй строке).
	rt.SetCaretPosition(12)
	rtCaretAt(t, rt, 12, 4+10, 14)
	rtKey(rt, KeyHome)
	rtCaretAt(t, rt, 10, 4, 14)
	rtKey(rt, KeyEnd)
	rtCaretAt(t, rt, 14, 4+20, 14)

	// Шаг вправо с «конца первой строки» уходит на вторую.
	rt.SetCaretPosition(3)
	rtKey(rt, KeyEnd)
	rtKey(rt, KeyRight)
	rtCaretAt(t, rt, 11, 4+5, 14)
	// Шаг влево со второй строки — внутрь первой.
	rtKey(rt, KeyHome)
	rtKey(rt, KeyLeft)
	rtCaretAt(t, rt, 9, 4+45, 4)

	// Shift+End выделяет до конца визуальной строки, а не абзаца.
	rt.SetCaretPosition(5)
	rtKey(rt, KeyEnd, ModShift)
	rtSel(t, rt, "bbbb ")
	rt.SetCaretPosition(12)
	rtKey(rt, KeyHome, ModShift)
	rtSel(t, rt, "cc")
}

func TestRichCaret_CtrlHomeEnd(t *testing.T) {
	rt := newCRT(58, 100, plainPara("aaaa bbbb cccc"), plainPara("dd"))
	rt.SetCaretPosition(3)
	rtKey(rt, KeyEnd, ModCtrl)
	rtCaretAt(t, rt, 17, 4+10, 24)
	rtKey(rt, KeyHome, ModCtrl)
	rtCaretAt(t, rt, 0, 4, 4)
	rtKey(rt, KeyEnd, ModCtrl, ModShift)
	rtSel(t, rt, rt.Text())
	rtKey(rt, KeyHome, ModCtrl, ModShift)
	rtSel(t, rt, "")
}

// Вертикальное движение через мягкий перенос и «хвост» строки.
func TestRichCaret_VerticalThroughWrap(t *testing.T) {
	rt := newCRT(58, 100, plainPara("aaaa bbbb cccc"), plainPara("xyz"))
	rt.SetCaretPosition(2) // x = 14
	rtKey(rt, KeyDown)
	rtCaretAt(t, rt, 12, 4+10, 14)
	rtKey(rt, KeyDown)
	rtCaretAt(t, rt, 15+2, 4+10, 24)
	rtKey(rt, KeyUp)
	rtKey(rt, KeyUp)
	rtCaretAt(t, rt, 2, 4+10, 4)

	// Узкая строка выше: «xyz|» (x = 15) → на «cccc» в столбец 3 → на первую.
	rt.SetCaretPosition(18)
	rtKey(rt, KeyUp)
	rtCaretAt(t, rt, 13, 4+15, 14)
	rtKey(rt, KeyUp)
	rtCaretAt(t, rt, 3, 4+15, 4)
}

// Желаемый X правее текста мягко перенесённой строки: каретка у её конца и
// остаётся на ней, а не прыгает на начало следующей.
func TestRichCaret_DesiredXPastWrappedLineEnd(t *testing.T) {
	rt := newCRT(58, 100, plainPara("aaaa bbbb cccc"), plainPara("0123456789"))
	rt.SetCaretPosition(15 + 10) // конец второго абзаца, x = 4+50
	rtKey(rt, KeyUp)
	rtCaretAt(t, rt, 14, 4+20, 14)
	rtKey(rt, KeyUp)
	rtCaretAt(t, rt, 10, 4+50, 4) // конец первой строки, не начало второй
	rtKey(rt, KeyDown)
	rtCaretAt(t, rt, 14, 4+20, 14)
	rtKey(rt, KeyDown)
	rtCaretAt(t, rt, 25, 4+50, 24)
}

// ─── Границы абзацев, раны разного кегля, пустой абзац ─────────────────────

func TestRichCaret_ParagraphBoundaries(t *testing.T) {
	rt := newCRT(200, 100, plainPara("ab"), plainPara("cd"))
	rt.SetCaretPosition(2)
	rtCaretAt(t, rt, 2, 4+10, 4)
	rtKey(rt, KeyRight)
	rtCaretAt(t, rt, 3, 4, 14) // начало второго абзаца
	rtKey(rt, KeyLeft)
	rtCaretAt(t, rt, 2, 4+10, 4)
	rtKey(rt, KeyDown)
	rtCaretAt(t, rt, 5, 4+10, 14)
	rtKey(rt, KeyHome)
	rtCaretAt(t, rt, 3, 4, 14)
	rtKey(rt, KeyLeft, ModShift)
	rtSel(t, rt, "\n")
}

func TestRichCaret_EmptyParagraph(t *testing.T) {
	rt := newCRT(200, 100, plainPara("a"), plainPara(""), plainPara("b"))
	rt.SetCaretPosition(1)
	rtKey(rt, KeyDown)
	rtCaretAt(t, rt, 2, 4, 14)
	if h := rt.IMECaretRect().Dy(); h != 10 {
		t.Errorf("высота каретки в пустом абзаце %d, ждали 10 (шрифт виджета)", h)
	}
	rtKey(rt, KeyDown)
	rtCaretAt(t, rt, 4, 4+5, 24)
	rtKey(rt, KeyUp)
	rtCaretAt(t, rt, 2, 4, 14)
	rtKey(rt, KeyRight)
	rtCaretAt(t, rt, 3, 4, 24)
	rtKey(rt, KeyLeft)
	rtKey(rt, KeyLeft)
	rtCaretAt(t, rt, 1, 4+5, 4)
	// Shift+→ через пустой абзац захватывает оба разделителя.
	rtKey(rt, KeyRight, ModShift)
	rtKey(rt, KeyRight, ModShift)
	rtSel(t, rt, "\n\n")

	// Пустой документ и документ из одного пустого абзаца: каретка стоит.
	empty := newCRT(200, 100)
	rtKey(empty, KeyRight)
	rtKey(empty, KeyDown, ModShift)
	rtKey(empty, KeyEnd)
	if empty.CaretPosition() != 0 {
		t.Errorf("каретка в пустом документе %d", empty.CaretPosition())
	}
	if r := empty.IMECaretRect(); r.Dy() < 1 {
		t.Errorf("прямоугольник каретки пустого документа вырожден: %v", r)
	}
}

func TestRichCaret_MixedSizeRuns(t *testing.T) {
	// "ab" мелким (10 pt: подъём 8, спуск 2), "CD" крупным (20 pt: 16 и 4).
	rt := newCRT(200, 100, para(RichRun{Text: "ab"}, RichRun{Text: "CD", Size: 20}))
	// Базовая линия строки — подъём крупного, 16 px от верха строки (y=4).
	cases := []struct{ off, x, y, h int }{
		{0, 4, 12, 10},  // начало строки — первый ран
		{1, 9, 12, 10},  // внутри мелкого
		{2, 14, 12, 10}, // на границе — по рану слева (мелкому)
		{3, 24, 4, 20},  // внутри крупного: черта от верха строки
		{4, 34, 4, 20},  // конец строки — по рану слева
	}
	for _, c := range cases {
		rt.SetCaretPosition(c.off)
		r := rt.IMECaretRect()
		if r.Min.X != c.x || r.Min.Y != c.y || r.Dy() != c.h || r.Dx() != 1 {
			t.Errorf("смещение %d: прямоугольник %v, ждали x=%d y=%d h=%d", c.off, r, c.x, c.y, c.h)
		}
		// Низ черты — на общей базовой линии с поправкой на спуск рана.
	}
	// Стрелками тоже проходим границу рана без скачков по смещению.
	rt.SetCaretPosition(0)
	for want := 1; want <= 4; want++ {
		rtKey(rt, KeyRight)
		if rt.CaretPosition() != want {
			t.Fatalf("→: %d, ждали %d", rt.CaretPosition(), want)
		}
	}
}

// ─── Кириллица и эмодзи: смещения в рунах ───────────────────────────────────

func TestRichCaret_CyrillicAndEmojiOffsetsInRunes(t *testing.T) {
	rt := newCRT(300, 100, plainPara("привет 😀 мир"))
	if got := len([]rune(rt.Text())); got != 12 {
		t.Fatalf("в тексте %d рун, ждали 12", got)
	}
	rtKey(rt, KeyRight, ModCtrl) // «привет » → начало эмодзи
	rtCaretAt(t, rt, 7, 4+35, 4)
	rtKey(rt, KeyRight, ModShift)
	rtSel(t, rt, "😀") // одна руна — не разрезается пополам
	if rt.CaretPosition() != 8 {
		t.Errorf("каретка %d, ждали 8 (руны, не байты)", rt.CaretPosition())
	}
	rtKey(rt, KeyRight)
	rtKey(rt, KeyRight)
	rtCaretAt(t, rt, 9, 4+45, 4)
	rtKey(rt, KeyEnd)
	rtCaretAt(t, rt, 12, 4+60, 4)
	rtKey(rt, KeyLeft, ModCtrl)
	rtCaretAt(t, rt, 9, 4+45, 4) // «мир»
	rtKey(rt, KeyLeft)
	rtKey(rt, KeyLeft, ModShift)
	rtSel(t, rt, "😀")

	// Программная установка и скринридер — тоже в рунах.
	rt.SetCaretPosition(3)
	if rt.AccessCaret() != 3 {
		t.Errorf("AccessCaret %d", rt.AccessCaret())
	}
}

// ─── Мышь ───────────────────────────────────────────────────────────────────

func TestRichCaret_ClickPlacesAndShiftClickExtends(t *testing.T) {
	rt := newCRT(300, 100, plainPara("hello world"))
	rtClick(rt, 4+10+1, 8, 1) // ближайшая граница — 2
	if rt.CaretPosition() != 2 {
		t.Fatalf("щелчок поставил каретку на %d", rt.CaretPosition())
	}
	rtSel(t, rt, "")

	// Shift+щелчок: якорь — прежняя каретка.
	e := MouseEvent{X: 4 + 25, Y: 8, Button: MouseLeft, Pressed: true, Clicks: 1, Mod: ModShift}
	rt.OnMouseButton(e)
	e.Pressed = false
	rt.OnMouseButton(e)
	rtSel(t, rt, "llo")
	if rt.CaretPosition() != 5 {
		t.Errorf("каретка %d, ждали 5", rt.CaretPosition())
	}
	// Ещё один Shift+щелчок левее якоря: выделение переворачивается.
	e = MouseEvent{X: 4, Y: 8, Button: MouseLeft, Pressed: true, Clicks: 1, Mod: ModShift}
	rt.OnMouseButton(e)
	e.Pressed = false
	rt.OnMouseButton(e)
	rtSel(t, rt, "he")
	if rt.CaretPosition() != 0 {
		t.Errorf("каретка %d, ждали 0", rt.CaretPosition())
	}
	// Простой щелчок снимает выделение и ставит каретку.
	rtClick(rt, 4+30, 8, 1)
	rtSel(t, rt, "")
	if rt.CaretPosition() != 6 {
		t.Errorf("каретка %d, ждали 6", rt.CaretPosition())
	}
	// Протяжка двигает каретку вместе с выделением.
	rtPress(rt, 4+10, 8, 1)
	rt.OnMouseMove(4+40, 8)
	rtRelease(rt, 4+40, 8, 1)
	rtSel(t, rt, "llo wo")
	if rt.CaretPosition() != 8 {
		t.Errorf("каретка после протяжки %d, ждали 8", rt.CaretPosition())
	}
}

func TestRichCaret_ShiftClickDoesNotFollowLink(t *testing.T) {
	rt := newCRT(300, 100, para(RichRun{Text: "see "}, RichRun{Text: "here", Link: "u://1"}))
	var got []string
	rt.OnLinkClick = func(u string) { got = append(got, u) }
	rt.SetCaretPosition(4)
	e := MouseEvent{X: 4 + 25, Y: 8, Button: MouseLeft, Pressed: true, Clicks: 1, Mod: ModShift}
	rt.OnMouseButton(e)
	e.Pressed = false
	rt.OnMouseButton(e)
	if len(got) != 0 {
		t.Errorf("Shift+щелчок открыл ссылку: %v", got)
	}
	rtSel(t, rt, "h")
}

// Щелчок правее мягко перенесённой строки оставляет каретку на ней.
func TestRichCaret_ClickRightOfWrappedLineStaysOnIt(t *testing.T) {
	rt := newCRT(58, 100, plainPara("aaaa bbbb cccc"))
	rtClick(rt, 56, 8, 1)
	rtCaretAt(t, rt, 10, 4+50, 4)
	rtClick(rt, 6, 18, 1)
	rtCaretAt(t, rt, 10, 4, 14)
}

// ─── Прокрутка к каретке ────────────────────────────────────────────────────

func TestRichCaret_ScrollsToCaret(t *testing.T) {
	rt := newCRT(100, 60, manyParas(30)...)
	rt.ShowCaret = true
	// В пределах видимой области (5 строк, одна из них — отступ) не листаем.
	for i := 0; i < 4; i++ {
		rtKey(rt, KeyDown)
	}
	if rt.ScrollY() != 0 {
		t.Errorf("прокрутка %d, ждали 0 пока каретка на виду", rt.ScrollY())
	}
	// Дальше — каретка упирается в низ и текст едет вместе с ней.
	for i := 0; i < 10; i++ {
		rtKey(rt, KeyDown)
	}
	r := rt.IMECaretRect()
	if r.Min.Y < 0 || r.Max.Y > 60 {
		t.Errorf("каретка вне видимой области: %v (прокрутка %d)", r, rt.ScrollY())
	}
	if rt.ScrollY() == 0 {
		t.Error("текст не поехал за кареткой")
	}
	// Ctrl+End — в самый низ, Ctrl+Home — в самый верх.
	rtKey(rt, KeyEnd, ModCtrl)
	if got := rt.ScrollY(); got != 308-60 {
		t.Errorf("Ctrl+End: прокрутка %d, ждали %d", got, 308-60)
	}
	rtKey(rt, KeyHome, ModCtrl)
	if rt.ScrollY() != 0 {
		t.Errorf("Ctrl+Home: прокрутка %d", rt.ScrollY())
	}
	// Колесо увело каретку за экран; любое движение возвращает её.
	rt.SetScrollY(200)
	rtKey(rt, KeyRight)
	if r := rt.IMECaretRect(); r.Min.Y < 0 || r.Max.Y > 60 {
		t.Errorf("после → каретка вне экрана: %v", r)
	}
	// Программная установка тоже показывает каретку.
	rt.SetCaretPosition(len([]rune(rt.Text())))
	if r := rt.IMECaretRect(); r.Min.Y < 0 || r.Max.Y > 60 {
		t.Errorf("SetCaretPosition: каретка вне экрана: %v", r)
	}
}

func TestRichCaret_PageUpDown(t *testing.T) {
	rt := newCRT(100, 60, manyParas(30)...)
	rt.ShowCaret = true
	// Высота страницы — 60 минус строка = 50 px = 5 строк.
	rtKey(rt, KeyPageDown)
	if got := rt.CaretPosition(); got != 5*7 { // «line a\n» — 7 рун на строку
		t.Errorf("PgDn: каретка %d, ждали %d (5 строк)", got, 5*7)
	}
	if rt.ScrollY() != 50 {
		t.Errorf("PgDn: прокрутка %d, ждали 50", rt.ScrollY())
	}
	r := rt.IMECaretRect()
	if r.Min.Y != 4 {
		t.Errorf("каретка осталась на той же высоте экрана? y=%d, ждали 4", r.Min.Y)
	}
	rtKey(rt, KeyPageUp)
	if rt.CaretPosition() != 0 || rt.ScrollY() != 0 {
		t.Errorf("PgUp: каретка %d, прокрутка %d", rt.CaretPosition(), rt.ScrollY())
	}
	// PgUp с первой страницы — в начало документа, PgDn с последней — в конец.
	rtKey(rt, KeyPageUp)
	if rt.CaretPosition() != 0 {
		t.Errorf("PgUp у начала: %d", rt.CaretPosition())
	}
	rtKey(rt, KeyEnd, ModCtrl)
	rtKey(rt, KeyPageDown)
	if got := len([]rune(rt.Text())); rt.CaretPosition() != got {
		t.Errorf("PgDn у конца: %d, ждали %d", rt.CaretPosition(), got)
	}
	// Shift+PgDn выделяет страницу.
	rtKey(rt, KeyHome, ModCtrl)
	rtKey(rt, KeyPageDown, ModShift)
	if n := strings.Count(rt.SelectedText(), "\n"); n != 5 {
		t.Errorf("Shift+PgDn: в выделении %d переводов строки, ждали 5", n)
	}
}

// Вертикальный шаг при прокрутке виджета с полосой: полоса не ломает
// положение каретки.
func TestRichCaret_CtrlA(t *testing.T) {
	rt := newCRT(200, 100, plainPara("abc"), plainPara("de"))
	rtKey(rt, KeyA, ModCtrl)
	rtSel(t, rt, "abc\nde")
	if rt.CaretPosition() != 6 {
		t.Errorf("после Ctrl+A каретка %d, ждали 6", rt.CaretPosition())
	}
	rtKey(rt, KeyRight)
	rtCaretAt(t, rt, 6, 4+10, 14) // схлопнулась в правый край
}

// ─── Доступность ────────────────────────────────────────────────────────────

func TestRichCaret_Access(t *testing.T) {
	rt := newCRT(200, 100, plainPara("привет"), plainPara("мир"))
	if !rt.AccessSetCaret(4) {
		t.Fatal("AccessSetCaret отказал")
	}
	if rt.AccessCaret() != 4 || rt.CaretPosition() != 4 {
		t.Errorf("каретка %d/%d, ждали 4", rt.AccessCaret(), rt.CaretPosition())
	}
	if a, b := rt.AccessSelection(); a != 4 || b != 4 {
		t.Errorf("выделение (%d,%d) при отсутствии выделения", a, b)
	}
	rtKey(rt, KeyRight, ModShift)
	if a, b := rt.AccessSelection(); a != 4 || b != 5 || rt.AccessCaret() != 5 {
		t.Errorf("выделение (%d,%d), каретка %d", a, b, rt.AccessCaret())
	}
	rt.AccessSetCaret(100) // усечение: два абзаца — 10 рун
	if rt.AccessCaret() != 10 {
		t.Errorf("каретка после усечения %d, ждали 10", rt.AccessCaret())
	}
	rt.AccessSetSelection(1, 3)
	if rt.CaretPosition() != 3 {
		t.Errorf("каретка после AccessSetSelection %d, ждали 3", rt.CaretPosition())
	}
	// Замена содержимого возвращает каретку в начало.
	rt.SetText("x")
	if rt.CaretPosition() != 0 {
		t.Errorf("после SetText каретка %d", rt.CaretPosition())
	}
}

// ─── Отрисовка и мигание ────────────────────────────────────────────────────

// withClock подменяет часы мигания и возвращает функцию-«стрелку».
func withClock(t *testing.T, start int64) *int64 {
	t.Helper()
	now := start
	old := richNowMs
	richNowMs = func() int64 { return now }
	t.Cleanup(func() { richNowMs = old })
	return &now
}

// caretRects — прямоугольники цвета каретки в записи кадра.
func caretRects(c *rtRec) []image.Rectangle {
	var out []image.Rectangle
	for _, r := range c.rects {
		if r.col == caretRed {
			out = append(out, r.r)
		}
	}
	return out
}

func TestRichCaret_DrawOnlyWhenShownAndFocused(t *testing.T) {
	now := withClock(t, 1000)
	rt := newRT(200, 100, plainPara("hello"))
	rt.CaretColor = caretRed
	rt.SetCaretPosition(2)

	draw := func() []image.Rectangle {
		c := &rtRec{}
		rt.Draw(c)
		return caretRects(c)
	}
	// Режим просмотра: каретки нет даже в фокусе — поведение этапа 1.
	rt.SetFocused(true)
	if r := draw(); len(r) != 0 {
		t.Errorf("в режиме просмотра нарисована каретка: %v", r)
	}
	rt.ShowCaret = true
	// Без фокуса каретки нет.
	rt.SetFocused(false)
	if r := draw(); len(r) != 0 {
		t.Errorf("без фокуса нарисована каретка: %v", r)
	}
	// В фокусе — одна черта: x=14, строка 4..14, ширина 1.
	rt.SetFocused(true)
	r := draw()
	if len(r) != 1 || r[0] != image.Rect(14, 4, 15, 14) {
		t.Fatalf("каретка нарисована как %v, ждали [(14,4)-(15,14)]", r)
	}
	// Тёмная фаза мигания — черты нет, светлая — есть.
	*now += caretBlinkHalfPeriodMs
	if r := draw(); len(r) != 0 {
		t.Errorf("в тёмной фазе нарисована каретка: %v", r)
	}
	*now += caretBlinkHalfPeriodMs
	if r := draw(); len(r) != 1 {
		t.Errorf("в светлой фазе каретки нет: %v", r)
	}
	// Движение каретки начинает мигание заново: сразу видна.
	*now += caretBlinkHalfPeriodMs
	rtKey(rt, KeyRight)
	if r := draw(); len(r) != 1 || r[0].Min.X != 19 {
		t.Errorf("после движения каретка не видна сразу: %v", r)
	}
}

func TestRichCaret_BlinkNeedsAnimationInvalidatesOnlyCaretRect(t *testing.T) {
	now := withClock(t, 5000)
	rt := newCRT(200, 100, plainPara("hello"))
	rt.CaretColor = caretRed
	rt.ShowCaret = true
	rt.SetFocused(true)
	rt.SetCaretPosition(1)

	var mu sync.Mutex
	var rects []image.Rectangle
	var fulls int
	h := RegisterUINotifier(func() { mu.Lock(); fulls++; mu.Unlock() },
		func(r image.Rectangle) { mu.Lock(); rects = append(rects, r); mu.Unlock() })
	defer UnregisterUINotifier(h)
	reset := func() { mu.Lock(); rects, fulls = nil, 0; mu.Unlock() }

	rt.Draw(&rtRec{}) // нарисована светлая фаза
	reset()
	if rt.NeedsAnimation() {
		t.Error("NeedsAnimation вернул true: кадр ради мигания не нужен, нужна инвалидация")
	}
	mu.Lock()
	n := len(rects) + fulls
	mu.Unlock()
	if n != 0 {
		t.Error("фаза не менялась, а виджет инвалидирован")
	}
	*now += caretBlinkHalfPeriodMs // фаза сменилась
	rt.NeedsAnimation()
	mu.Lock()
	got := append([]image.Rectangle(nil), rects...)
	f := fulls
	mu.Unlock()
	if len(got) != 1 || got[0] != image.Rect(9, 4, 10, 14) || f != 0 {
		t.Errorf("инвалидация %v (полных %d), ждали ровно прямоугольник каретки", got, f)
	}
	// Повторный вызов в той же фазе — тишина.
	reset()
	rt.NeedsAnimation()
	mu.Lock()
	n = len(rects) + fulls
	mu.Unlock()
	if n != 0 {
		t.Error("повторная инвалидация в той же фазе")
	}
	// Без фокуса и без ShowCaret — всегда false и ничего не инвалидирует.
	rt.SetFocused(false)
	*now += caretBlinkHalfPeriodMs
	reset()
	rt.NeedsAnimation()
	rt.ShowCaret = false
	rt.SetFocused(true)
	reset() // SetFocused сам инвалидирует виджет — это не анимация
	rt.NeedsAnimation()
	mu.Lock()
	n = len(rects) + fulls
	mu.Unlock()
	if n != 0 {
		t.Errorf("анимация заявлена без фокуса или без ShowCaret: %v", rects)
	}
}

// Каретка рисуется в отсечении виджета: строка, уехавшая под край, не оставляет
// черту поверх соседей.
func TestRichCaret_DrawnInsideClip(t *testing.T) {
	withClock(t, 0)
	rt := newCRT(100, 60, manyParas(30)...)
	rt.ShowCaret = true
	rt.CaretColor = caretRed
	rt.SetFocused(true)
	rt.SetCaretPosition(0)
	rt.SetScrollY(100) // каретка осталась на первой строке, она за кадром
	c := &rtClipRec{}
	rt.Draw(c)
	for _, r := range c.carets {
		if !r.clip.Eq(image.Rect(0, 0, 90, 60)) {
			t.Errorf("каретка рисуется с отсечением %v", r.clip)
		}
	}
}

type rtClipRec struct {
	rtRec
	carets []struct{ clip image.Rectangle }
}

func (c *rtClipRec) FillRect(x, y, w, h int, col color.RGBA) {
	if col == caretRed {
		c.carets = append(c.carets, struct{ clip image.Rectangle }{c.Clip()})
	}
	c.rtRec.FillRect(x, y, w, h, col)
}

// Гонка: каретка двигается из одних горутин, рисуется и опрашивается из других.
func TestRichCaret_Race(t *testing.T) {
	rt := newCRT(100, 60, manyParas(30)...)
	rt.ShowCaret = true
	rt.SetFocused(true)
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
	run(func(i int) { rt.Draw(&rtRec{}); rt.NeedsAnimation() })
	run(func(i int) {
		rtKey(rt, KeyDown)
		rtKey(rt, KeyRight, ModShift)
		rtKey(rt, KeyPageUp)
		rtKey(rt, KeyHome, ModCtrl)
	})
	run(func(i int) { _ = rt.IMECaretRect(); _ = rt.AccessCaret(); rt.SetCaretPosition(i % 50) })
	run(func(i int) { rtClick(rt, 10, 10+i%40, 1) })
	for i := 0; i < 200; i++ {
		rt.AppendParagraph(plainPara("tail"))
		if i%50 == 49 {
			rt.SetParagraphs(manyParas(5))
		}
	}
	close(stop)
	wg.Wait()
}
