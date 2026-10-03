package widget

import (
	"fmt"
	"image/color"
	"math/rand"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// Тесты модели правок RichDocument: без виджета и окна. Ожидаемые значения
// записываются «дампом» документа — строкой вида
//
//	b"Hel" -"lo" ¶ i"x"
//
// (оформление, затем текст в кавычках; абзацы через «¶»), чтобы сравнивать
// содержимое и оформление одним взглядом.

var (
	rdBold   = RichRun{Font: BuiltinFontBold}
	rdItalic = RichRun{Font: BuiltinFontItalic}
	rdRed    = RichRun{Color: color.RGBA{255, 0, 0, 255}}
)

// rdTxt — ран с текстом и оформлением st.
func rdTxt(text string, st RichRun) RichRun {
	st.Text = text
	return st
}

// rdP — абзац из ранов.
func rdP(runs ...RichRun) RichParagraph { return RichParagraph{Runs: runs} }

func rdSty(r RichRun) string {
	var t []string
	switch r.Font {
	case "":
	case BuiltinFontBold:
		t = append(t, "b")
	case BuiltinFontItalic:
		t = append(t, "i")
	case BuiltinFontBoldItalic:
		t = append(t, "bi")
	case BuiltinFontMono:
		t = append(t, "m")
	default:
		t = append(t, r.Font)
	}
	if r.Size > 0 {
		t = append(t, fmt.Sprintf("sz%g", r.Size))
	}
	if r.Color.A != 0 {
		t = append(t, fmt.Sprintf("c%02x%02x%02x", r.Color.R, r.Color.G, r.Color.B))
	}
	if r.BG.A != 0 {
		t = append(t, fmt.Sprintf("bg%02x%02x%02x", r.BG.R, r.BG.G, r.BG.B))
	}
	if r.Underline {
		t = append(t, "u")
	}
	if r.Strike {
		t = append(t, "s")
	}
	if r.Link != "" {
		t = append(t, "link="+r.Link)
	}
	if len(t) == 0 {
		return "-"
	}
	return strings.Join(t, "+")
}

func rdDumpParas(paras []RichParagraph) string {
	var out []string
	for _, p := range paras {
		var parts []string
		var tags []string
		switch p.Align {
		case TextAlignCenter:
			tags = append(tags, "c")
		case TextAlignRight:
			tags = append(tags, "r")
		}
		if p.Indent != 0 {
			tags = append(tags, fmt.Sprintf("ind=%d", p.Indent))
		}
		if p.SpaceBefore != 0 {
			tags = append(tags, fmt.Sprintf("sb=%d", p.SpaceBefore))
		}
		if p.SpaceAfter != 0 {
			tags = append(tags, fmt.Sprintf("sa=%d", p.SpaceAfter))
		}
		if len(tags) > 0 {
			parts = append(parts, "["+strings.Join(tags, " ")+"]")
		}
		if len(p.Runs) == 0 {
			parts = append(parts, "()")
		}
		for _, r := range p.Runs {
			parts = append(parts, fmt.Sprintf("%s%q", rdSty(r), r.Text))
		}
		out = append(out, strings.Join(parts, " "))
	}
	return strings.Join(out, " ¶ ")
}

func rdDump(d *RichDocument) string { return rdDumpParas(d.paras) }

// rdWant сравнивает дамп документа с ожидаемым и проверяет инварианты.
func rdWant(t *testing.T, d *RichDocument, want string) {
	t.Helper()
	if got := rdDump(d); got != want {
		t.Errorf("документ:\n  получили %s\n  ждали    %s", got, want)
	}
	rdCheck(t, d)
}

// rdCheck проверяет инварианты документа.
func rdCheck(t *testing.T, d *RichDocument) {
	t.Helper()
	if len(d.paras) == 0 {
		t.Fatal("в документе нет абзацев")
	}
	for i, p := range d.paras {
		text := richParaLen(p) == 0
		if text && len(p.Runs) > 1 {
			t.Fatalf("абзац %d пуст, но ранов %d: %s", i, len(p.Runs), rdDump(d))
		}
		if len(p.Runs) == 1 && p.Runs[0].Text == "" && p.Runs[0] == (RichRun{}) {
			t.Fatalf("абзац %d хранит безликий пустой ран: %s", i, rdDump(d))
		}
		if !text {
			for j, r := range p.Runs {
				if r.Text == "" {
					t.Fatalf("абзац %d: пустой ран %d среди текста: %s", i, j, rdDump(d))
				}
				if j > 0 && richSameStyle(p.Runs[j-1], r) {
					t.Fatalf("абзац %d: раны %d и %d с одинаковым оформлением не слиты: %s", i, j-1, j, rdDump(d))
				}
			}
		}
	}
	if got, want := d.Len(), utf8.RuneCountInString(d.Text()); got != want {
		t.Fatalf("Len() = %d, а в Text() рун %d", got, want)
	}
}

// ---------------------------------------------------------------------------
// Построение

func TestRichDoc_NewNormalizes(t *testing.T) {
	d := NewRichDocument([]RichParagraph{
		rdP(rdTxt("a", rdBold), rdTxt("", rdItalic), rdTxt("b", rdBold), rdTxt("c", RichRun{})),
		rdP(),
		rdP(rdTxt("", rdItalic), rdTxt("", rdBold)),
		rdP(rdTxt("x\r\ny", RichRun{})),
	})
	// Пустые раны выброшены, одинаковые слиты; в абзаце без текста остался
	// первый пустой ран с оформлением; "\r\n" внутри рана стал "\n".
	rdWant(t, d, `b"ab" -"c" ¶ () ¶ i"" ¶ -"x\ny"`)
}

func TestRichDoc_NewEmpty(t *testing.T) {
	d := NewRichDocument(nil)
	if d.ParagraphCount() != 1 || d.Len() != 0 || d.Text() != "" {
		t.Fatalf("пустой документ: абзацев %d, длина %d", d.ParagraphCount(), d.Len())
	}
	rdCheck(t, d)
	// В пустой документ можно вставлять.
	d.Insert(0, "привет", RichRun{})
	rdWant(t, d, `-"привет"`)
}

func TestRichDoc_NewDoesNotAliasInput(t *testing.T) {
	src := []RichParagraph{rdP(rdTxt("abc", RichRun{}))}
	d := NewRichDocument(src)
	src[0].Runs[0].Text = "ЗАТЁРТО"
	rdWant(t, d, `-"abc"`)
	// И наружу копия: правка копии документ не меняет.
	out := d.Paragraphs()
	out[0].Runs[0].Text = "ЗАТЁРТО"
	rdWant(t, d, `-"abc"`)
	d.Insert(1, "X", RichRun{})
	if out[0].Runs[0].Text != "ЗАТЁРТО" {
		t.Fatal("правка документа изменила ранее выданную копию")
	}
}

func TestRichDoc_FromText(t *testing.T) {
	cases := map[string]string{
		"":             `()`,
		"abc":          `-"abc"`,
		"a\nb":         `-"a" ¶ -"b"`,
		"a\r\nb\r\n":   `-"a" ¶ -"b" ¶ ()`,
		"a\rb":         `-"a" ¶ -"b"`,
		"\n\n":         `() ¶ () ¶ ()`,
		"привет\nмир😀": `-"привет" ¶ -"мир😀"`,
	}
	for in, want := range cases {
		d := NewRichDocumentFromText(in)
		if got := rdDump(d); got != want {
			t.Errorf("%q: %s, ждали %s", in, got, want)
		}
		rdCheck(t, d)
	}
	// Оформление для всех абзацев.
	ps := RichParagraphsFromText("a\n\nb", rdBold)
	if got := rdDumpParas(ps); got != `b"a" ¶ b"" ¶ b"b"` {
		t.Errorf("со стилем: %s", got)
	}
}

// ---------------------------------------------------------------------------
// Insert

func TestRichDoc_InsertPositions(t *testing.T) {
	mk := func() *RichDocument {
		return NewRichDocument([]RichParagraph{rdP(rdTxt("Hello", rdBold), rdTxt("World", RichRun{}))})
	}
	cases := []struct {
		name string
		at   int
		want string
	}{
		{"в начало", 0, `i"X" b"Hello" -"World"`},
		{"в середину рана", 2, `b"He" i"X" b"llo" -"World"`},
		{"на границу ранов", 5, `b"Hello" i"X" -"World"`},
		{"в конец", 10, `b"Hello" -"World" i"X"`},
		{"за концом — в конец", 99, `b"Hello" -"World" i"X"`},
		{"до начала — в начало", -5, `i"X" b"Hello" -"World"`},
	}
	for _, c := range cases {
		d := mk()
		end := d.Insert(c.at, "X", rdItalic)
		rdWant(t, d, c.want)
		if want := min(max(c.at, 0), 10) + 1; end != want {
			t.Errorf("%s: вернули %d, ждали %d", c.name, end, want)
		}
	}
}

func TestRichDoc_InsertSameStyleMerges(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("ac", rdBold))})
	d.Insert(1, "b", rdBold)
	rdWant(t, d, `b"abc"`)
	// Ран на границе: вставка стилем соседа сливается с ним.
	d = NewRichDocument([]RichParagraph{rdP(rdTxt("a", rdBold), rdTxt("c", RichRun{}))})
	d.Insert(1, "b", rdBold)
	rdWant(t, d, `b"ab" -"c"`)
	d = NewRichDocument([]RichParagraph{rdP(rdTxt("a", rdBold), rdTxt("c", RichRun{}))})
	d.Insert(1, "b", RichRun{})
	rdWant(t, d, `b"a" -"bc"`)
}

func TestRichDoc_InsertIgnoresStyleText(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("ab", RichRun{}))})
	d.Insert(1, "X", RichRun{Text: "ЧУЖОЙ", Font: BuiltinFontBold})
	rdWant(t, d, `-"a" b"X" -"b"`)
}

func TestRichDoc_InsertIntoEmptyParagraphKeepsRemembered(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("", rdBold))})
	if st := d.StyleAt(0); st != rdBold {
		t.Fatalf("StyleAt в пустом абзаце = %+v", st)
	}
	d.Insert(0, "x", rdBold)
	rdWant(t, d, `b"x"`)
	// Пустой ран другого оформления не остаётся рядом с текстом.
	d = NewRichDocument([]RichParagraph{rdP(rdTxt("", rdItalic))})
	d.Insert(0, "x", rdBold)
	rdWant(t, d, `b"x"`)
}

func TestRichDoc_InsertNewlines(t *testing.T) {
	d := NewRichDocument([]RichParagraph{{
		Runs:  []RichRun{rdTxt("Hello", rdBold), rdTxt("World", RichRun{})},
		Align: TextAlignCenter, Indent: 20, SpaceBefore: 1, SpaceAfter: 2,
	}})
	end := d.Insert(3, "A\nB\n", rdItalic)
	// Все новые абзацы унаследовали выравнивание и отступы; хвост исходного
	// абзаца ушёл в последний.
	rdWant(t, d,
		`[c ind=20 sb=1 sa=2] b"Hel" i"A" ¶ [c ind=20 sb=1 sa=2] i"B" ¶ [c ind=20 sb=1 sa=2] b"lo" -"World"`)
	if end != 3+4 {
		t.Errorf("конец вставки %d, ждали 7", end)
	}
	if d.Text() != "HelA\nB\nloWorld" {
		t.Errorf("текст %q", d.Text())
	}
}

func TestRichDoc_InsertNewlineOnly(t *testing.T) {
	// Enter в конце: новый пустой абзац помнит оформление набираемого.
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("ab", rdBold))})
	d.Insert(2, "\n", rdBold)
	rdWant(t, d, `b"ab" ¶ b""`)
	// Enter в начале: пустой абзац сверху.
	d = NewRichDocument([]RichParagraph{rdP(rdTxt("ab", rdBold))})
	d.Insert(0, "\n", rdBold)
	rdWant(t, d, `b"" ¶ b"ab"`)
	// Enter в пустом абзаце.
	d = NewRichDocument([]RichParagraph{rdP(rdTxt("", rdBold))})
	d.Insert(0, "\n", rdBold)
	rdWant(t, d, `b"" ¶ b""`)
	// Несколько Enter подряд.
	d = NewRichDocument(nil)
	d.Insert(0, "\n\n\n", RichRun{})
	rdWant(t, d, `() ¶ () ¶ () ¶ ()`)
}

func TestRichDoc_InsertCRLF(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("ab", RichRun{}))})
	end := d.Insert(1, "x\r\ny\rz", RichRun{})
	rdWant(t, d, `-"ax" ¶ -"y" ¶ -"zb"`)
	if end != 1+5 { // x \n y \n z
		t.Errorf("конец %d", end)
	}
}

func TestRichDoc_InsertEmptyText(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("ab", RichRun{}))})
	rev := d.Revision()
	if end := d.Insert(1, "", rdBold); end != 1 {
		t.Errorf("пустая вставка вернула %d", end)
	}
	if d.CanUndo() || d.Revision() != rev {
		t.Error("пустая вставка оставила след")
	}
}

func TestRichDoc_InsertInlineKeepsSoftBreak(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("ab", RichRun{}))})
	end := d.InsertInline(1, "x\ny", RichRun{})
	rdWant(t, d, `-"ax\nyb"`)
	if end != 4 || d.ParagraphCount() != 1 {
		t.Errorf("конец %d, абзацев %d", end, d.ParagraphCount())
	}
	// Мягкий '\n' в ране — обычная руна для смещений.
	d.Delete(2, 3)
	rdWant(t, d, `-"axyb"`)
}

func TestRichDoc_OffsetsAreRunes(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("при😀вет", rdBold), rdTxt("мир", RichRun{}))})
	d.Insert(4, "_", rdItalic) // после эмодзи
	rdWant(t, d, `b"при😀" i"_" b"вет" -"мир"`)
	d.Delete(3, 5) // эмодзи и подчёркивание
	rdWant(t, d, `b"привет" -"мир"`)
	if d.Len() != 9 {
		t.Errorf("Len = %d", d.Len())
	}
	d.ApplyStyle(2, 7, func(r *RichRun) { r.Underline = true })
	rdWant(t, d, `b"пр" b+u"ивет" u"м" -"ир"`)
}

func TestRichDoc_InsertParagraphsFragment(t *testing.T) {
	d := NewRichDocument([]RichParagraph{
		{Runs: []RichRun{rdTxt("abcd", RichRun{})}, Align: TextAlignRight},
	})
	frag := []RichParagraph{
		{Runs: []RichRun{rdTxt("X", rdBold)}, Align: TextAlignCenter},
		{Runs: []RichRun{rdTxt("Y", rdItalic)}, Indent: 8},
		{Runs: []RichRun{rdTxt("Z", rdRed)}, Indent: 16},
	}
	end := d.InsertParagraphs(2, frag)
	// Первый вливается в текущий (его выравнивание не меняет), остальные
	// со своим оформлением, хвост "cd" — к последнему.
	rdWant(t, d, `[r] -"ab" b"X" ¶ [ind=8] i"Y" ¶ [ind=16] cff0000"Z" -"cd"`)
	if end != 2+1+1+1+1+1 {
		t.Errorf("конец %d", end)
	}
	// Источник не затронут.
	frag[0].Runs[0].Text = "ЗАТЁРТО"
	if strings.Contains(d.Text(), "ЗАТЁРТО") {
		t.Error("документ разделяет раны с источником")
	}
	// В пустой абзац со списком: формат первого пункта берётся от него.
	d = NewRichDocument(nil)
	d.InsertParagraphs(0, []RichParagraph{
		{Runs: []RichRun{rdTxt("• a", RichRun{})}, Indent: 24},
		{Runs: []RichRun{rdTxt("• b", RichRun{})}, Indent: 24},
	})
	rdWant(t, d, `[ind=24] -"• a" ¶ [ind=24] -"• b"`)
	// Пустой список — ничего.
	d.InsertParagraphs(0, nil)
	rdWant(t, d, `[ind=24] -"• a" ¶ [ind=24] -"• b"`)
}

// ---------------------------------------------------------------------------
// Delete

func TestRichDoc_DeleteInsideRun(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("Hello", rdBold), rdTxt("World", RichRun{}))})
	d.Delete(1, 3)
	rdWant(t, d, `b"Hlo" -"World"`)
}

func TestRichDoc_DeleteAcrossRuns(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("Hello", rdBold), rdTxt("World", RichRun{}), rdTxt("!!", rdItalic))})
	d.Delete(3, 11)
	rdWant(t, d, `b"Hel" i"!"`)
	// Удаление целого рана, оставляющее одинаковых соседей, сливает их.
	d = NewRichDocument([]RichParagraph{rdP(rdTxt("ab", rdBold), rdTxt("XX", RichRun{}), rdTxt("cd", rdBold))})
	d.Delete(2, 4)
	rdWant(t, d, `b"abcd"`)
}

func TestRichDoc_DeleteAcrossParagraphs(t *testing.T) {
	d := NewRichDocument([]RichParagraph{
		{Runs: []RichRun{rdTxt("Hello", rdBold)}, Align: TextAlignCenter, Indent: 5},
		{Runs: []RichRun{rdTxt("middle", RichRun{})}, Align: TextAlignRight},
		{Runs: []RichRun{rdTxt("World", rdItalic)}, Indent: 9},
	})
	d.Delete(3, 15) // "lo¶middle¶Wo"
	// Оформление абзаца — у первого.
	rdWant(t, d, `[c ind=5] b"Hel" i"rld"`)
	if d.ParagraphCount() != 1 {
		t.Errorf("абзацев %d", d.ParagraphCount())
	}
}

func TestRichDoc_DeleteNewlineOnly(t *testing.T) {
	d := NewRichDocument([]RichParagraph{
		{Runs: []RichRun{rdTxt("ab", rdBold)}, Align: TextAlignCenter},
		{Runs: []RichRun{rdTxt("cd", rdBold)}, Align: TextAlignRight},
	})
	d.Delete(2, 3)
	rdWant(t, d, `[c] b"abcd"`) // оформление первого, раны слиты
}

func TestRichDoc_DeleteWholeParagraphKeepsStyle(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("abc", rdBold))})
	d.Delete(0, 3)
	// Абзац пуст, но помнит, что набор в нём жирный.
	rdWant(t, d, `b""`)
	if d.StyleAt(0) != rdBold {
		t.Errorf("StyleAt = %+v", d.StyleAt(0))
	}
	d.Insert(0, "z", d.StyleAt(0))
	rdWant(t, d, `b"z"`)
}

func TestRichDoc_DeleteMiddleParagraphEmpties(t *testing.T) {
	d := NewRichDocument([]RichParagraph{
		rdP(rdTxt("a", RichRun{})), rdP(rdTxt("bbb", rdBold)), rdP(rdTxt("c", RichRun{})),
	})
	d.Delete(2, 5) // «bbb»
	rdWant(t, d, `-"a" ¶ b"" ¶ -"c"`)
}

func TestRichDoc_DeleteWholeDocument(t *testing.T) {
	d := NewRichDocument([]RichParagraph{
		rdP(rdTxt("ab", rdBold)), rdP(rdTxt("cd", rdItalic)), rdP(rdTxt("ef", RichRun{})),
	})
	d.Delete(0, d.Len())
	rdWant(t, d, `b""`)
	if d.Len() != 0 || d.ParagraphCount() != 1 {
		t.Errorf("Len %d, абзацев %d", d.Len(), d.ParagraphCount())
	}
	// Документ из безликого оформления после удаления — просто пустой.
	d = NewRichDocumentFromText("abc\ndef")
	d.Delete(0, 7)
	rdWant(t, d, `()`)
}

func TestRichDoc_DeleteReversedAndClamped(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("abcdef", RichRun{}))})
	d.Delete(4, 2)
	rdWant(t, d, `-"abef"`)
	d.Delete(2, 100)
	rdWant(t, d, `-"ab"`)
	d.Delete(-5, 1)
	rdWant(t, d, `-"b"`)
}

func TestRichDoc_DeleteEmptyRangeNoop(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("abc", RichRun{}))})
	rev := d.Revision()
	d.Delete(2, 2)
	d.Delete(50, 60) // оба конца упираются в конец документа
	if d.CanUndo() || d.Revision() != rev {
		t.Error("пустое удаление оставило след в истории")
	}
	rdWant(t, d, `-"abc"`)
}

// ---------------------------------------------------------------------------
// ApplyStyle

func rdMakeBold(r *RichRun) { r.Font = BuiltinFontBold }

func TestRichDoc_ApplyStylePartOfRun(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("Hello World", RichRun{}))})
	d.ApplyStyle(2, 7, rdMakeBold)
	rdWant(t, d, `-"He" b"llo W" -"orld"`)
}

func TestRichDoc_ApplyStyleWholeRunAndEdges(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("abc", RichRun{}))})
	d.ApplyStyle(0, 3, rdMakeBold)
	rdWant(t, d, `b"abc"`)
	d = NewRichDocument([]RichParagraph{rdP(rdTxt("abc", RichRun{}))})
	d.ApplyStyle(0, 1, rdMakeBold)
	rdWant(t, d, `b"a" -"bc"`)
	d = NewRichDocument([]RichParagraph{rdP(rdTxt("abc", RichRun{}))})
	d.ApplyStyle(2, 3, rdMakeBold)
	rdWant(t, d, `-"ab" b"c"`)
	// Диапазон шире документа — по границам документа.
	d = NewRichDocument([]RichParagraph{rdP(rdTxt("abc", RichRun{}))})
	d.ApplyStyle(-9, 99, rdMakeBold)
	rdWant(t, d, `b"abc"`)
}

func TestRichDoc_ApplyStyleAcrossRuns(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("aa", RichRun{}), rdTxt("bb", rdItalic), rdTxt("cc", rdBold))})
	d.ApplyStyle(1, 5, func(r *RichRun) { r.Underline = true })
	// fn применилась к каждому рану по отдельности, остальное оформление цело.
	rdWant(t, d, `-"a" u"a" i+u"bb" b+u"c" b"c"`)
}

func TestRichDoc_ApplyStyleMergesNeighbours(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("ab", rdBold), rdTxt("cd", RichRun{}), rdTxt("ef", rdBold))})
	d.ApplyStyle(2, 4, rdMakeBold)
	rdWant(t, d, `b"abcdef"`)
}

func TestRichDoc_ApplyStyleAcrossParagraphs(t *testing.T) {
	d := NewRichDocument([]RichParagraph{
		rdP(rdTxt("abc", RichRun{})), rdP(rdTxt("def", RichRun{})), rdP(rdTxt("ghi", RichRun{})),
	})
	d.ApplyStyle(2, 9, rdMakeBold) // «c¶def¶g»
	rdWant(t, d, `-"ab" b"c" ¶ b"def" ¶ b"g" -"hi"`)
}

func TestRichDoc_ApplyStyleEmptyRange(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("abc", RichRun{}))})
	rev := d.Revision()
	d.ApplyStyle(1, 1, rdMakeBold)
	if d.CanUndo() || d.Revision() != rev {
		t.Error("пустой диапазон оставил след")
	}
	rdWant(t, d, `-"abc"`)
}

func TestRichDoc_ApplyStyleEmptyParagraph(t *testing.T) {
	// Каретка в пустом абзаце: стиль запоминается абзацем.
	d := NewRichDocumentFromText("a\n\nb")
	d.ApplyStyle(2, 2, rdMakeBold)
	rdWant(t, d, `-"a" ¶ b"" ¶ -"b"`)
	if d.StyleAt(2) != rdBold {
		t.Errorf("StyleAt = %+v", d.StyleAt(2))
	}
	// Выделение, проходящее сквозь пустой абзац, красит и его.
	d = NewRichDocumentFromText("a\n\nb")
	d.ApplyStyle(0, 4, rdMakeBold)
	rdWant(t, d, `b"a" ¶ b"" ¶ b"b"`)
	// Выделение, лишь доходящее до начала пустого абзаца, его не красит.
	d = NewRichDocumentFromText("a\n\nb")
	d.ApplyStyle(0, 2, rdMakeBold)
	rdWant(t, d, `b"a" ¶ () ¶ -"b"`)
	// Пустой абзац последний, выделение до конца документа.
	d = NewRichDocumentFromText("a\n")
	d.ApplyStyle(0, 2, rdMakeBold)
	rdWant(t, d, `b"a" ¶ b""`)
}

func TestRichDoc_ApplyStyleNoChangeNoHistory(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("abc", rdBold))})
	d.ApplyStyle(0, 3, rdMakeBold) // уже жирный
	if d.CanUndo() {
		t.Error("правка без изменений попала в историю")
	}
	d.ApplyStyle(0, 3, nil)
	if d.CanUndo() {
		t.Error("nil-функция попала в историю")
	}
}

func TestRichDoc_ApplyStyleCannotChangeText(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("abc", RichRun{}))})
	d.ApplyStyle(0, 3, func(r *RichRun) { r.Text = "ПОДМЕНА"; r.Underline = true })
	rdWant(t, d, `u"abc"`)
}

func TestRichDoc_ApplyStyleLinkAndSize(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("see docs here", RichRun{}))})
	d.ApplyStyle(4, 8, func(r *RichRun) { r.Link = "https://x"; r.Size = 14 })
	rdWant(t, d, `-"see " sz14+link=https://x"docs" -" here"`)
	d.ApplyStyle(4, 8, func(r *RichRun) { r.Link = ""; r.Size = 0 })
	rdWant(t, d, `-"see docs here"`)
}

// ---------------------------------------------------------------------------
// StyleAt

func TestRichDoc_StyleAt(t *testing.T) {
	d := NewRichDocument([]RichParagraph{
		rdP(rdTxt("ab", rdBold), rdTxt("cd", rdItalic)),
		rdP(rdTxt("ef", rdRed)),
		rdP(rdTxt("", rdBold)),
		rdP(),
	})
	cases := []struct {
		pos  int
		want RichRun
	}{
		{0, rdBold},     // начало абзаца — первый символ
		{1, rdBold},     // внутри первого рана
		{2, rdBold},     // граница ранов — символ слева
		{3, rdItalic},   // внутри второго
		{4, rdItalic},   // конец абзаца
		{5, rdRed},      // начало второго абзаца
		{7, rdRed},      // конец второго
		{8, rdBold},     // пустой абзац помнит
		{9, RichRun{}},  // пустой без ранов
		{99, RichRun{}}, // за концом — последний абзац
		{-3, rdBold},    // до начала — начало
	}
	for _, c := range cases {
		if got := d.StyleAt(c.pos); got != c.want {
			t.Errorf("StyleAt(%d) = %+v, ждали %+v", c.pos, got, c.want)
		}
	}
	// Text в результате всегда пуст.
	if d.StyleAt(1).Text != "" {
		t.Error("StyleAt вернул текст")
	}
}

// ---------------------------------------------------------------------------
// SetParagraphFormat

func TestRichDoc_SetParagraphFormat(t *testing.T) {
	mk := func() *RichDocument {
		return NewRichDocumentFromText("aaa\nbbb\nccc")
	}
	center := func(p *RichParagraph) { p.Align = TextAlignCenter; p.Indent = 10 }

	d := mk()
	d.SetParagraphFormat(5, 5, center) // каретка в «bbb»
	rdWant(t, d, `-"aaa" ¶ [c ind=10] -"bbb" ¶ -"ccc"`)

	d = mk()
	d.SetParagraphFormat(2, 9, center) // задевает все три
	rdWant(t, d, `[c ind=10] -"aaa" ¶ [c ind=10] -"bbb" ¶ [c ind=10] -"ccc"`)

	// Диапазон, оканчивающийся ровно на начале абзаца, его не задевает.
	d = mk()
	d.SetParagraphFormat(0, 4, center)
	rdWant(t, d, `[c ind=10] -"aaa" ¶ -"bbb" ¶ -"ccc"`)

	// Но один символ следующего абзаца уже задевает.
	d = mk()
	d.SetParagraphFormat(0, 5, center)
	rdWant(t, d, `[c ind=10] -"aaa" ¶ [c ind=10] -"bbb" ¶ -"ccc"`)

	// Раны fn изменить не может.
	d = mk()
	d.SetParagraphFormat(0, 0, func(p *RichParagraph) { p.Runs = nil; p.SpaceAfter = 3 })
	rdWant(t, d, `[sa=3] -"aaa" ¶ -"bbb" ¶ -"ccc"`)

	// Без изменений — без истории.
	d = mk()
	d.SetParagraphFormat(0, 11, func(p *RichParagraph) {})
	if d.CanUndo() {
		t.Error("пустая правка формата попала в историю")
	}
	d.SetParagraphFormat(0, 3, nil)
	if d.CanUndo() {
		t.Error("nil-функция попала в историю")
	}
}

func TestRichDoc_ParagraphFormatAt(t *testing.T) {
	d := NewRichDocument([]RichParagraph{
		{Runs: []RichRun{rdTxt("ab", RichRun{})}, Align: TextAlignRight, Indent: 4},
		rdP(rdTxt("cd", RichRun{})),
	})
	if p := d.ParagraphFormatAt(1); p.Align != TextAlignRight || p.Indent != 4 || p.Runs != nil {
		t.Errorf("%+v", p)
	}
	if p := d.ParagraphFormatAt(4); p.Align != TextAlignLeft || p.Indent != 0 {
		t.Errorf("%+v", p)
	}
}

func TestRichDoc_Fragment(t *testing.T) {
	d := NewRichDocument([]RichParagraph{
		{Runs: []RichRun{rdTxt("Hello", rdBold)}, Align: TextAlignCenter},
		rdP(rdTxt("mid", RichRun{})),
		{Runs: []RichRun{rdTxt("World", rdItalic)}, Indent: 3},
	})
	if got := rdDumpParas(d.Fragment(3, 12)); got != `[c] b"lo" ¶ -"mid" ¶ [ind=3] i"Wo"` {
		t.Errorf("через абзацы: %s", got)
	}
	if got := rdDumpParas(d.Fragment(1, 3)); got != `[c] b"el"` {
		t.Errorf("внутри абзаца: %s", got)
	}
	if got := rdDumpParas(d.Fragment(2, 2)); got != `[c] ()` {
		t.Errorf("пустой: %s", got)
	}
	// Вставка фрагмента воспроизводит копируемое.
	frag := d.Fragment(1, 12)
	d2 := NewRichDocument(nil)
	d2.InsertParagraphs(0, frag)
	if d2.Text() != "ello\nmid\nWo" {
		t.Errorf("текст %q", d2.Text())
	}
}

// ---------------------------------------------------------------------------
// Undo / redo

func TestRichDoc_UndoRedoEachOperation(t *testing.T) {
	base := func() *RichDocument {
		return NewRichDocument([]RichParagraph{
			{Runs: []RichRun{rdTxt("Hello", rdBold), rdTxt("World", RichRun{})}, Align: TextAlignCenter},
			{Runs: []RichRun{rdTxt("second", rdItalic)}, Indent: 7},
			rdP(),
		})
	}
	ops := []struct {
		name string
		do   func(d *RichDocument)
	}{
		{"Insert", func(d *RichDocument) { d.Insert(3, "XY", rdRed) }},
		{"Insert с абзацами", func(d *RichDocument) { d.Insert(3, "X\nY\nZ", rdRed) }},
		{"Insert в пустой абзац", func(d *RichDocument) { d.Insert(d.Len(), "q", rdBold) }},
		{"InsertInline", func(d *RichDocument) { d.InsertInline(3, "X\nY", rdRed) }},
		{"InsertParagraphs", func(d *RichDocument) {
			d.InsertParagraphs(7, []RichParagraph{{Runs: []RichRun{rdTxt("p", rdRed)}, Indent: 1}, {Runs: []RichRun{rdTxt("q", rdRed)}, Indent: 2}})
		}},
		{"Delete внутри", func(d *RichDocument) { d.Delete(1, 3) }},
		{"Delete через раны", func(d *RichDocument) { d.Delete(3, 8) }},
		{"Delete через абзацы", func(d *RichDocument) { d.Delete(7, 14) }},
		{"Delete всего", func(d *RichDocument) { d.Delete(0, d.Len()) }},
		{"Delete только \\n", func(d *RichDocument) { d.Delete(10, 11) }},
		{"ApplyStyle", func(d *RichDocument) { d.ApplyStyle(3, 14, rdMakeBold) }},
		{"ApplyStyle пустой абзац", func(d *RichDocument) { d.ApplyStyle(d.Len(), d.Len(), func(r *RichRun) { r.Size = 20 }) }},
		{"SetParagraphFormat", func(d *RichDocument) {
			d.SetParagraphFormat(2, 14, func(p *RichParagraph) { p.Align = TextAlignRight; p.Indent = 99 })
		}},
		{"Replace", func(d *RichDocument) { d.Replace(3, 14, "заменили\nтут", rdRed) }},
	}
	for _, op := range ops {
		d := base()
		orig := d.Paragraphs()
		op.do(d)
		after := d.Paragraphs()
		rdCheck(t, d)
		if richParasEqual(orig, after) {
			t.Errorf("%s: операция ничего не изменила — тест бессмыслен", op.name)
			continue
		}
		if !d.CanUndo() || d.CanRedo() {
			t.Errorf("%s: CanUndo=%v CanRedo=%v", op.name, d.CanUndo(), d.CanRedo())
		}
		if _, ok := d.Undo(); !ok {
			t.Errorf("%s: Undo не сработал", op.name)
		}
		if !richParasEqual(orig, d.paras) {
			t.Errorf("%s: после Undo\n  получили %s\n  ждали    %s", op.name, rdDump(d), rdDumpParas(orig))
		}
		rdCheck(t, d)
		if d.CanUndo() || !d.CanRedo() {
			t.Errorf("%s: после Undo CanUndo=%v CanRedo=%v", op.name, d.CanUndo(), d.CanRedo())
		}
		if _, ok := d.Redo(); !ok {
			t.Errorf("%s: Redo не сработал", op.name)
		}
		if !richParasEqual(after, d.paras) {
			t.Errorf("%s: после Redo\n  получили %s\n  ждали    %s", op.name, rdDump(d), rdDumpParas(after))
		}
		rdCheck(t, d)
	}
}

func TestRichDoc_UndoNothing(t *testing.T) {
	d := NewRichDocumentFromText("abc")
	if _, ok := d.Undo(); ok {
		t.Error("Undo на пустой истории вернул ok")
	}
	if _, ok := d.Redo(); ok {
		t.Error("Redo на пустой истории вернул ok")
	}
	rdWant(t, d, `-"abc"`)
}

func TestRichDoc_UndoSelection(t *testing.T) {
	d := NewRichDocumentFromText("hello world")

	d.Insert(5, "XYZ", RichRun{})
	sel, _ := d.Undo()
	if sel != (RichDocSel{5, 5}) {
		t.Errorf("отмена вставки: %+v", sel)
	}
	sel, _ = d.Redo()
	if sel != (RichDocSel{8, 8}) {
		t.Errorf("возврат вставки: %+v", sel)
	}
	d.Undo()

	d.Delete(2, 7)
	sel, _ = d.Undo()
	if sel != (RichDocSel{2, 7}) {
		t.Errorf("отмена удаления должна выделить возвращённое: %+v", sel)
	}
	sel, _ = d.Redo()
	if sel != (RichDocSel{2, 2}) {
		t.Errorf("возврат удаления: %+v", sel)
	}
	d.Undo()

	d.ApplyStyle(1, 4, rdMakeBold)
	sel, _ = d.Undo()
	if sel != (RichDocSel{1, 4}) {
		t.Errorf("отмена стиля: %+v", sel)
	}
	sel, _ = d.Redo()
	if sel != (RichDocSel{1, 4}) {
		t.Errorf("возврат стиля: %+v", sel)
	}
}

func TestRichDoc_NewEditClearsRedo(t *testing.T) {
	d := NewRichDocumentFromText("abc")
	d.Insert(3, "d", RichRun{})
	d.Undo()
	if !d.CanRedo() {
		t.Fatal("нет Redo после Undo")
	}
	d.Insert(0, "X", RichRun{})
	if d.CanRedo() {
		t.Error("новая правка не сбросила Redo")
	}
	rdWant(t, d, `-"Xabc"`)
}

func TestRichDoc_UndoSequence(t *testing.T) {
	d := NewRichDocumentFromText("one")
	states := []string{rdDump(d)}
	steps := []func(){
		func() { d.Insert(3, " two", rdBold) },
		func() { d.Insert(0, "zero\n", RichRun{}) },
		func() { d.Delete(2, 9) },
		func() { d.ApplyStyle(0, 3, rdMakeBold) },
		func() { d.SetParagraphFormat(0, 0, func(p *RichParagraph) { p.Indent = 4 }) },
	}
	for _, s := range steps {
		s()
		states = append(states, rdDump(d))
	}
	for i := len(steps) - 1; i >= 0; i-- {
		d.Undo()
		if got := rdDump(d); got != states[i] {
			t.Fatalf("после отмены шага %d: %s, ждали %s", i, got, states[i])
		}
	}
	for i := 0; i < len(steps); i++ {
		d.Redo()
		if got := rdDump(d); got != states[i+1] {
			t.Fatalf("после возврата шага %d: %s, ждали %s", i, got, states[i+1])
		}
	}
}

func TestRichDoc_UndoDepthLimit(t *testing.T) {
	d := NewRichDocumentFromText("")
	d.SetUndoDepth(3)
	for i := 0; i < 5; i++ {
		d.Insert(d.Len(), string(rune('a'+i)), RichRun{})
	}
	n := 0
	for {
		if _, ok := d.Undo(); !ok {
			break
		}
		n++
	}
	if n != 3 {
		t.Errorf("отмен %d, ждали 3", n)
	}
	rdWant(t, d, `-"ab"`)
	// Уменьшение глубины обрезает уже накопленное.
	d = NewRichDocumentFromText("")
	for i := 0; i < 5; i++ {
		d.Insert(d.Len(), "x", RichRun{})
	}
	d.SetUndoDepth(2)
	n = 0
	for {
		if _, ok := d.Undo(); !ok {
			break
		}
		n++
	}
	if n != 2 {
		t.Errorf("после SetUndoDepth(2) отмен %d", n)
	}
	// Глубина 0 — истории нет, документ работает.
	d = NewRichDocumentFromText("")
	d.SetUndoDepth(0)
	d.Insert(0, "abc", RichRun{})
	d.Type(3, "d", RichRun{})
	d.Delete(0, 1)
	if d.CanUndo() {
		t.Error("история при глубине 0")
	}
	rdWant(t, d, `-"bcd"`)
}

func TestRichDoc_ClearHistoryAndSetParagraphs(t *testing.T) {
	d := NewRichDocumentFromText("abc")
	d.Insert(0, "x", RichRun{})
	d.Undo()
	d.ClearHistory()
	if d.CanUndo() || d.CanRedo() {
		t.Error("ClearHistory не очистила")
	}
	d.Insert(0, "x", RichRun{})
	rev := d.Revision()
	d.SetParagraphs([]RichParagraph{rdP(rdTxt("новый", RichRun{}))})
	if d.CanUndo() || d.Revision() == rev {
		t.Error("SetParagraphs не сбросила историю или не изменила ревизию")
	}
	rdWant(t, d, `-"новый"`)
}

func TestRichDoc_RevisionGrows(t *testing.T) {
	d := NewRichDocumentFromText("abc")
	r0 := d.Revision()
	d.Insert(0, "x", RichRun{})
	r1 := d.Revision()
	d.Undo()
	r2 := d.Revision()
	d.Redo()
	r3 := d.Revision()
	if !(r0 < r1 && r1 < r2 && r2 < r3) {
		t.Errorf("ревизии не растут: %d %d %d %d", r0, r1, r2, r3)
	}
}

// ---------------------------------------------------------------------------
// Группировка

func TestRichDoc_TypingGroupsIntoOneUndo(t *testing.T) {
	d := NewRichDocumentFromText("")
	at := 0
	for _, ch := range "привет" {
		at = d.Type(at, string(ch), RichRun{})
	}
	rdWant(t, d, `-"привет"`)
	if len(d.undo) != 1 {
		t.Fatalf("записей истории %d, ждали 1", len(d.undo))
	}
	sel, _ := d.Undo()
	rdWant(t, d, `()`)
	if sel != (RichDocSel{0, 0}) {
		t.Errorf("каретка после отмены набора %+v", sel)
	}
	sel, _ = d.Redo()
	rdWant(t, d, `-"привет"`)
	if sel != (RichDocSel{6, 6}) {
		t.Errorf("каретка после возврата набора %+v", sel)
	}
}

func TestRichDoc_TypingGroupBreaks(t *testing.T) {
	// Прыжок каретки: следующий символ не там, где закончился предыдущий.
	d := NewRichDocumentFromText("")
	d.Type(0, "a", RichRun{})
	d.Type(1, "b", RichRun{})
	d.Type(0, "c", RichRun{})
	if len(d.undo) != 2 {
		t.Errorf("после прыжка записей %d, ждали 2", len(d.undo))
	}
	rdWant(t, d, `-"cab"`)

	// Смена оформления набора.
	d = NewRichDocumentFromText("")
	d.Type(0, "a", RichRun{})
	d.Type(1, "b", rdBold)
	d.Type(2, "c", rdBold)
	if len(d.undo) != 2 {
		t.Errorf("после смены стиля записей %d, ждали 2", len(d.undo))
	}

	// BreakUndoGroup.
	d = NewRichDocumentFromText("")
	d.Type(0, "a", RichRun{})
	d.BreakUndoGroup()
	d.Type(1, "b", RichRun{})
	if len(d.undo) != 2 {
		t.Errorf("после BreakUndoGroup записей %d", len(d.undo))
	}

	// Enter — отдельная запись и разрыв набора.
	d = NewRichDocumentFromText("")
	d.Type(0, "a", RichRun{})
	d.Type(1, "\n", RichRun{})
	d.Type(2, "b", RichRun{})
	d.Type(3, "c", RichRun{})
	if len(d.undo) != 3 {
		t.Errorf("после Enter записей %d, ждали 3", len(d.undo))
	}
	rdWant(t, d, `-"a" ¶ -"bc"`)
	d.Undo()
	rdWant(t, d, `-"a" ¶ ()`)
	d.Undo()
	rdWant(t, d, `-"a"`)

	// Другая правка между набором — разрыв.
	d = NewRichDocumentFromText("")
	d.Type(0, "a", RichRun{})
	d.ApplyStyle(0, 1, rdMakeBold)
	d.Type(1, "b", RichRun{})
	if len(d.undo) != 3 {
		t.Errorf("после ApplyStyle записей %d, ждали 3", len(d.undo))
	}

	// Вставка не склеивается с набором, даже в нужном месте и тем же стилем.
	d = NewRichDocumentFromText("")
	d.Type(0, "a", RichRun{})
	d.Insert(1, "b", RichRun{})
	d.Type(2, "c", RichRun{})
	if len(d.undo) != 3 {
		t.Errorf("вставка склеилась с набором: записей %d", len(d.undo))
	}
}

func TestRichDoc_TypingAfterUndoStartsNewGroup(t *testing.T) {
	d := NewRichDocumentFromText("")
	d.Type(0, "a", RichRun{})
	d.Type(1, "b", RichRun{})
	d.Undo()
	d.Type(0, "x", RichRun{})
	d.Type(1, "y", RichRun{})
	if len(d.undo) != 1 || d.CanRedo() {
		t.Errorf("записей %d, CanRedo=%v", len(d.undo), d.CanRedo())
	}
	rdWant(t, d, `-"xy"`)
}

func TestRichDoc_TypingInsideRunSplitsAndRestores(t *testing.T) {
	d := NewRichDocument([]RichParagraph{rdP(rdTxt("Hello", rdBold), rdTxt("World", RichRun{}))})
	orig := d.Paragraphs()
	at := 2
	for _, ch := range "abc" {
		at = d.Type(at, string(ch), rdItalic)
	}
	rdWant(t, d, `b"He" i"abc" b"llo" -"World"`)
	d.Undo()
	if !richParasEqual(orig, d.paras) {
		t.Errorf("после отмены %s", rdDump(d))
	}
}

func TestRichDoc_GroupAndReplace(t *testing.T) {
	d := NewRichDocument([]RichParagraph{
		{Runs: []RichRun{rdTxt("Hello", rdBold)}, Indent: 3},
		rdP(rdTxt("World", RichRun{})),
	})
	orig := d.Paragraphs()
	d.Replace(2, 8, "Z", rdItalic)
	rdWant(t, d, `[ind=3] b"He" i"Z" -"rld"`)
	if len(d.undo) != 1 {
		t.Fatalf("Replace записал %d записей", len(d.undo))
	}
	d.Undo()
	if !richParasEqual(orig, d.paras) {
		t.Errorf("после отмены Replace: %s", rdDump(d))
	}
	d.Redo()
	rdWant(t, d, `[ind=3] b"He" i"Z" -"rld"`)

	// Replace с пустым диапазоном — просто вставка; с пустым текстом — удаление.
	d = NewRichDocumentFromText("abc")
	if end := d.Replace(1, 1, "X", RichRun{}); end != 2 {
		t.Errorf("end %d", end)
	}
	rdWant(t, d, `-"aXbc"`)
	d.Replace(0, 2, "", RichRun{})
	rdWant(t, d, `-"bc"`)
	if len(d.undo) != 2 {
		t.Errorf("записей %d", len(d.undo))
	}
}

func TestRichDoc_BeginEndGroupSpansParagraphs(t *testing.T) {
	d := NewRichDocument([]RichParagraph{
		rdP(rdTxt("aaa", RichRun{})), rdP(rdTxt("bbb", RichRun{})), rdP(rdTxt("ccc", RichRun{})),
		rdP(rdTxt("ddd", RichRun{})), rdP(rdTxt("eee", RichRun{})),
	})
	orig := d.Paragraphs()
	d.BeginGroup()
	d.ApplyStyle(16, 18, rdMakeBold) // последний абзац
	d.Insert(0, "X\nY", rdRed)       // первый: +1 абзац
	d.BeginGroup()                   // вложенная
	d.Delete(8, 14)                  // слияние посередине
	d.EndGroup()
	d.SetParagraphFormat(0, 0, func(p *RichParagraph) { p.Indent = 5 })
	d.EndGroup()
	if len(d.undo) != 1 {
		t.Fatalf("записей %d, ждали 1", len(d.undo))
	}
	after := d.Paragraphs()
	d.Undo()
	if !richParasEqual(orig, d.paras) {
		t.Errorf("после отмены группы:\n  %s\n  ждали %s", rdDump(d), rdDumpParas(orig))
	}
	d.Redo()
	if !richParasEqual(after, d.paras) {
		t.Errorf("после возврата группы:\n  %s\n  ждали %s", rdDump(d), rdDumpParas(after))
	}
	// После закрытия группы следующая правка — новая запись.
	d.Insert(0, "q", RichRun{})
	if len(d.undo) != 2 {
		t.Errorf("записей %d, ждали 2", len(d.undo))
	}
	// Лишний EndGroup безвреден.
	d.EndGroup()
	d.Insert(0, "w", RichRun{})
	if len(d.undo) != 3 {
		t.Errorf("записей %d, ждали 3", len(d.undo))
	}
}

func TestRichDoc_GroupStartsAfterTyping(t *testing.T) {
	d := NewRichDocumentFromText("")
	d.Type(0, "a", RichRun{})
	d.BeginGroup()
	d.Type(1, "b", RichRun{}) // не должна склеиться с набором до группы
	d.EndGroup()
	if len(d.undo) != 2 {
		t.Errorf("записей %d, ждали 2", len(d.undo))
	}
}

// ---------------------------------------------------------------------------
// Случайные проверки

// rdCell — один символ документа в эталонной модели: руна и оформление.
// Разделитель абзацев — отдельный вид ячейки.
type rdCell struct {
	r   rune
	st  RichRun
	sep bool
}

func rdFlatten(d *RichDocument) []rdCell {
	var out []rdCell
	for i, p := range d.paras {
		if i > 0 {
			out = append(out, rdCell{r: '\n', sep: true})
		}
		for _, r := range p.Runs {
			st := r
			st.Text = ""
			for _, c := range r.Text {
				out = append(out, rdCell{r: c, st: st})
			}
		}
	}
	return out
}

var rdAlphabet = []string{"a", "б", "ж", "z", " ", "😀", "x", "я", "\n", "\n", "y", "—"}

func rdRandText(rng *rand.Rand, maxLen int) string {
	n := 1 + rng.Intn(maxLen)
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString(rdAlphabet[rng.Intn(len(rdAlphabet))])
	}
	return sb.String()
}

var rdStyles = []RichRun{
	{}, rdBold, rdItalic, rdRed,
	{Font: BuiltinFontBold, Size: 14},
	{Underline: true},
	{Link: "https://e.x", Color: color.RGBA{0, 0, 255, 255}},
}

func rdRandDoc(rng *rand.Rand) *RichDocument {
	n := 1 + rng.Intn(4)
	var paras []RichParagraph
	for i := 0; i < n; i++ {
		p := RichParagraph{Align: TextAlign(rng.Intn(3)), Indent: rng.Intn(3) * 8, SpaceAfter: rng.Intn(2)}
		for k := rng.Intn(4); k > 0; k-- {
			text := rdRandText(rng, 6)
			text = strings.ReplaceAll(text, "\n", "·") // абзацы — структурой, не символом
			p.Runs = append(p.Runs, rdTxt(text, rdStyles[rng.Intn(len(rdStyles))]))
		}
		paras = append(paras, p)
	}
	return NewRichDocument(paras)
}

// rdFns — детерминированные правки оформления: нужны и документу, и модели.
var rdFns = []func(*RichRun){
	func(r *RichRun) { r.Font = BuiltinFontBold },
	func(r *RichRun) { r.Underline = !r.Underline },
	func(r *RichRun) { r.Color = color.RGBA{0, 255, 0, 255} },
	func(r *RichRun) { *r = RichRun{Text: r.Text} },
	func(r *RichRun) { r.Size = 18 },
	func(r *RichRun) { r.Link = "https://l.k" },
}

func rdClamp(v, hi int) int { return min(max(v, 0), hi) }

// rdRandOp выполняет случайную правку над документом и над моделью. typing
// разрешает Type/группы (там промежуточные состояния не сверяются).
func rdRandOp(rng *rand.Rand, d *RichDocument, model *[]rdCell, typing bool) string {
	n := len(*model)
	pos := func() int { return rng.Intn(n+3) - 1 } // с выходом за границы
	insertModel := func(at int, text string, st RichRun, inline bool) {
		at = rdClamp(at, len(*model))
		st.Text = ""
		var cells []rdCell
		for _, r := range richNormalizeText(text) {
			if r == '\n' && !inline {
				cells = append(cells, rdCell{r: '\n', sep: true})
			} else {
				cells = append(cells, rdCell{r: r, st: st})
			}
		}
		*model = slices.Insert(*model, at, cells...)
	}
	deleteModel := func(a, b int) {
		a, b = richOrdered(rdClamp(a, len(*model)), rdClamp(b, len(*model)))
		*model = slices.Delete(*model, a, b)
	}
	kinds := 9
	if typing {
		kinds = 12
	}
	switch k := rng.Intn(kinds); k {
	case 0, 1:
		at, text, st := pos(), rdRandText(rng, 5), rdStyles[rng.Intn(len(rdStyles))]
		d.Insert(at, text, st)
		insertModel(at, text, st, false)
		return fmt.Sprintf("Insert(%d,%q)", at, text)
	case 2:
		at, text, st := pos(), rdRandText(rng, 5), rdStyles[rng.Intn(len(rdStyles))]
		d.InsertInline(at, text, st)
		insertModel(at, text, st, true)
		return fmt.Sprintf("InsertInline(%d,%q)", at, text)
	case 3, 4:
		a, b := pos(), pos()
		d.Delete(a, b)
		deleteModel(a, b)
		return fmt.Sprintf("Delete(%d,%d)", a, b)
	case 5, 6:
		a, b := pos(), pos()
		fi := rng.Intn(len(rdFns))
		d.ApplyStyle(a, b, rdFns[fi])
		lo, hi := richOrdered(rdClamp(a, len(*model)), rdClamp(b, len(*model)))
		for i := lo; i < hi; i++ {
			if !(*model)[i].sep {
				rdFns[fi](&(*model)[i].st)
			}
		}
		return fmt.Sprintf("ApplyStyle(%d,%d,fn%d)", a, b, fi)
	case 7:
		a, b := pos(), pos()
		al, ind := TextAlign(rng.Intn(3)), rng.Intn(5)
		d.SetParagraphFormat(a, b, func(p *RichParagraph) { p.Align, p.Indent = al, ind })
		return fmt.Sprintf("SetParagraphFormat(%d,%d)", a, b)
	case 8:
		a, b := pos(), pos()
		text, st := rdRandText(rng, 5), rdStyles[rng.Intn(len(rdStyles))]
		d.Replace(a, b, text, st)
		lo, hi := richOrdered(a, b)
		deleteModel(lo, hi)
		insertModel(lo, text, st, false)
		return fmt.Sprintf("Replace(%d,%d,%q)", a, b, text)
	case 9, 10:
		// Набор подряд: часто продолжаем с места, где закончили.
		at := pos()
		if rng.Intn(3) > 0 {
			at = d.typeEnd
		}
		text := string([]rune(rdRandText(rng, 1))[:1])
		d.Type(at, text, RichRun{})
		insertModel(at, text, RichRun{}, false)
		return fmt.Sprintf("Type(%d,%q)", at, text)
	default:
		d.BeginGroup()
		for i := 0; i < 1+rng.Intn(3); i++ {
			rdRandOp(rng, d, model, false)
		}
		d.EndGroup()
		return "группа"
	}
}

// Модель документа (руны и оформление каждой руны) совпадает с реальным
// документом после каждой правки; отмены возвращают в точности прежние
// состояния, возвраты — в точности следующие.
func TestRichDoc_RandomEditsMatchModelAndUndoStepByStep(t *testing.T) {
	for seed := int64(1); seed <= 300; seed++ {
		rng := rand.New(rand.NewSource(seed))
		d := rdRandDoc(rng)
		d.SetUndoDepth(100000)
		model := rdFlatten(d)
		orig := d.Paragraphs()
		var states [][]RichParagraph
		var log []string

		for i := 0; i < 40; i++ {
			before := d.Paragraphs()
			nUndo := len(d.undo)
			log = append(log, rdRandOp(rng, d, &model, false))
			rdCheck(t, d)
			if got := rdFlatten(d); !slices.Equal(got, model) {
				t.Fatalf("seed %d, шаг %d (%s): документ разошёлся с моделью\n  документ %s\n  журнал %v",
					seed, i, log[len(log)-1], rdDump(d), log)
			}
			// Одно действие — не более одной записи истории.
			switch len(d.undo) - nUndo {
			case 0:
			case 1:
				states = append(states, before)
			default:
				t.Fatalf("seed %d: действие %s добавило %d записей", seed, log[len(log)-1], len(d.undo)-nUndo)
			}
		}
		final := d.Paragraphs()

		for i := len(states) - 1; i >= 0; i-- {
			if _, ok := d.Undo(); !ok {
				t.Fatalf("seed %d: Undo закончился раньше времени (осталось %d)", seed, i+1)
			}
			if !richParasEqual(states[i], d.paras) {
				t.Fatalf("seed %d: после отмены шага %d\n  получили %s\n  ждали    %s\n  журнал %v",
					seed, i, rdDump(d), rdDumpParas(states[i]), log)
			}
			rdCheck(t, d)
		}
		if _, ok := d.Undo(); ok {
			t.Fatalf("seed %d: лишняя запись истории", seed)
		}
		if !richParasEqual(orig, d.paras) {
			t.Fatalf("seed %d: документ после всех отмен не равен исходному", seed)
		}
		for i := range states {
			if _, ok := d.Redo(); !ok {
				t.Fatalf("seed %d: Redo закончился раньше времени", seed)
			}
			var want []RichParagraph
			if i+1 < len(states) {
				want = states[i+1]
			} else {
				want = final
			}
			if !richParasEqual(want, d.paras) {
				t.Fatalf("seed %d: после возврата шага %d\n  получили %s\n  ждали    %s", seed, i, rdDump(d), rdDumpParas(want))
			}
		}
	}
}

// Набор и группы склеивают записи, поэтому промежуточные состояния не
// сверяются — но полный откат обязан вернуть исходный документ, полный
// возврат — конечный, а текст и оформление рун — совпасть с моделью.
func TestRichDoc_RandomTypingGroupsFullUndo(t *testing.T) {
	for seed := int64(1000); seed < 1300; seed++ {
		rng := rand.New(rand.NewSource(seed))
		d := rdRandDoc(rng)
		d.SetUndoDepth(100000)
		model := rdFlatten(d)
		orig := d.Paragraphs()
		var log []string
		for i := 0; i < 60; i++ {
			log = append(log, rdRandOp(rng, d, &model, true))
			rdCheck(t, d)
			if rng.Intn(6) == 0 {
				d.BreakUndoGroup()
			}
			if got := rdFlatten(d); !slices.Equal(got, model) {
				t.Fatalf("seed %d, шаг %d (%s): документ разошёлся с моделью\n  документ %s",
					seed, i, log[len(log)-1], rdDump(d))
			}
		}
		final := d.Paragraphs()
		steps := 0
		for {
			if _, ok := d.Undo(); !ok {
				break
			}
			steps++
			rdCheck(t, d)
		}
		if !richParasEqual(orig, d.paras) {
			t.Fatalf("seed %d: после %d отмен документ не равен исходному\n  получили %s\n  ждали    %s\n  журнал %v",
				seed, steps, rdDump(d), rdDumpParas(orig), log)
		}
		for i := 0; i < steps; i++ {
			if _, ok := d.Redo(); !ok {
				t.Fatalf("seed %d: Redo закончился раньше времени", seed)
			}
			rdCheck(t, d)
		}
		if !richParasEqual(final, d.paras) {
			t.Fatalf("seed %d: после всех возвратов документ не равен конечному", seed)
		}
	}
}

// Те же правки при ограниченной глубине: старые записи теряются, но то, что
// осталось, откатывается без порчи документа.
func TestRichDoc_RandomLimitedDepthStaysConsistent(t *testing.T) {
	for seed := int64(2000); seed < 2100; seed++ {
		rng := rand.New(rand.NewSource(seed))
		d := rdRandDoc(rng)
		d.SetUndoDepth(5)
		model := rdFlatten(d)
		for i := 0; i < 50; i++ {
			rdRandOp(rng, d, &model, true)
			if len(d.undo) > 5 {
				t.Fatalf("seed %d: записей %d при глубине 5", seed, len(d.undo))
			}
		}
		for {
			if _, ok := d.Undo(); !ok {
				break
			}
			rdCheck(t, d)
		}
		for {
			if _, ok := d.Redo(); !ok {
				break
			}
			rdCheck(t, d)
		}
		if got := rdFlatten(d); !slices.Equal(got, model) {
			t.Fatalf("seed %d: после отмен и возвратов документ разошёлся с моделью", seed)
		}
	}
}

func TestRichDoc_InsertAtEveryPositionOfEveryRunLayout(t *testing.T) {
	// Исчерпывающе: все позиции вставки в документ с несколькими ранами,
	// абзацами и кириллицей с эмодзи; текст и оформление сверяются с моделью.
	d0 := NewRichDocument([]RichParagraph{
		rdP(rdTxt("ай😀", rdBold), rdTxt("бэ", RichRun{}), rdTxt("ц", rdItalic)),
		rdP(),
		rdP(rdTxt("я", rdRed)),
	})
	n := d0.Len()
	for _, ins := range []string{"Ж", "ЖЗ", "\n", "Ж\nЗ", "\nЖ"} {
		for at := -1; at <= n+1; at++ {
			d := NewRichDocument(d0.Paragraphs())
			model := rdFlatten(d)
			d.Insert(at, ins, rdRed)
			cells := []rdCell{}
			for _, r := range ins {
				if r == '\n' {
					cells = append(cells, rdCell{r: '\n', sep: true})
				} else {
					cells = append(cells, rdCell{r: r, st: rdRed})
				}
			}
			model = slices.Insert(model, rdClamp(at, n), cells...)
			if got := rdFlatten(d); !slices.Equal(got, model) {
				t.Fatalf("Insert(%d,%q): %s", at, ins, rdDump(d))
			}
			rdCheck(t, d)
			d.Undo()
			if !richParasEqual(d0.paras, d.paras) {
				t.Fatalf("Insert(%d,%q): отмена не вернула документ: %s", at, ins, rdDump(d))
			}
		}
	}
}

func TestRichDoc_DeleteEveryRange(t *testing.T) {
	d0 := NewRichDocument([]RichParagraph{
		{Runs: []RichRun{rdTxt("ай😀", rdBold), rdTxt("бэ", RichRun{})}, Indent: 1},
		rdP(),
		{Runs: []RichRun{rdTxt("я", rdRed), rdTxt("ю", rdItalic)}, Indent: 2},
	})
	n := d0.Len()
	for a := 0; a <= n; a++ {
		for b := a; b <= n; b++ {
			d := NewRichDocument(d0.Paragraphs())
			model := rdFlatten(d)
			d.Delete(a, b)
			model = slices.Delete(model, a, b)
			if got := rdFlatten(d); !slices.Equal(got, model) {
				t.Fatalf("Delete(%d,%d): %s", a, b, rdDump(d))
			}
			rdCheck(t, d)
			d.Undo()
			if !richParasEqual(d0.paras, d.paras) {
				t.Fatalf("Delete(%d,%d): отмена не вернула документ: %s", a, b, rdDump(d))
			}
		}
	}
}
