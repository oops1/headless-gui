package widget

import (
	"math/rand"
	"strings"
	"testing"
)

// fakeRichMeasurer — измеритель с предсказуемыми числами: ширина руны —
// половина кегля (10 pt → 5 px), шрифт "wide" вдвое шире; подъём — 80% кегля,
// спуск — 20%. Раскладку так можно проверять арифметикой, без движка.
type fakeRichMeasurer struct{}

func (fakeRichMeasurer) TextWidth(text string, size float64, font string) int {
	w := int(size/2) * len([]rune(text))
	if font == "wide" {
		w *= 2
	}
	return w
}

func (fakeRichMeasurer) FontMetrics(size float64, font string) FontMetrics {
	a, d := int(size*0.8), int(size*0.2)
	return FontMetrics{Ascent: a, Descent: d, Height: a + d}
}

func layoutOf(width int, paras ...RichParagraph) *richLayout {
	return layoutRich(paras, richLayoutOpts{Width: width, Size: 10, M: fakeRichMeasurer{}})
}

func para(runs ...RichRun) RichParagraph { return RichParagraph{Runs: runs} }

// lineTexts — тексты строк (склейка сегментов) для сравнения.
func lineTexts(l *richLayout) []string {
	var out []string
	for _, ln := range l.Lines {
		var sb strings.Builder
		for _, s := range ln.Segments {
			sb.WriteString(s.Run.Text)
		}
		out = append(out, sb.String())
	}
	return out
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Перенос по словам: слово, не влезшее в остаток строки, уходит на следующую
// целиком; пробел остаётся «висеть» на конце прежней строки.
func TestRichLayout_WordWrap(t *testing.T) {
	// 5 px на символ, ширина 40 px = 8 символов.
	l := layoutOf(40, para(RichRun{Text: "aaa bbb ccc ddd"}))
	want := []string{"aaa bbb ", "ccc ddd"}
	if got := lineTexts(l); !eqStrings(got, want) {
		t.Fatalf("строки %q, ждали %q", got, want)
	}
	// Смещения полуинтервалами в рунах документа.
	if l.Lines[0].Start != 0 || l.Lines[0].End != 8 || l.Lines[1].Start != 8 || l.Lines[1].End != 15 {
		t.Errorf("смещения строк: %d..%d, %d..%d", l.Lines[0].Start, l.Lines[0].End,
			l.Lines[1].Start, l.Lines[1].End)
	}
	if l.Lines[0].HardEnd {
		t.Error("строка, оборванная переносом, не должна считаться оканчивающейся переводом")
	}
	if !l.Lines[1].HardEnd {
		t.Error("последняя строка абзаца заканчивается абзацем")
	}
	// Хвостовой пробел не входит в ширину строки: 7 символов, а не 8.
	if l.Lines[0].Width != 35 {
		t.Errorf("ширина первой строки %d, ждали 35 (без хвостового пробела)", l.Lines[0].Width)
	}
	if l.TextLen != 15 {
		t.Errorf("длина текста %d, ждали 15", l.TextLen)
	}
}

// Слово, разорванное границей ранов, остаётся одним словом: на границе
// строку не рвут.
func TestRichLayout_WordAcrossRuns(t *testing.T) {
	// "abcd" (два рана) влезает в 25 px (5 символов), "abcd ef" — нет.
	l := layoutOf(25, para(RichRun{Text: "ab"}, RichRun{Text: "cd ef", Font: "bold"}))
	want := []string{"abcd ", "ef"}
	if got := lineTexts(l); !eqStrings(got, want) {
		t.Fatalf("строки %q, ждали %q", got, want)
	}
	if n := len(l.Lines[0].Segments); n != 2 {
		t.Fatalf("в первой строке %d сегментов, ждали 2 (по одному на ран)", n)
	}
	if l.Lines[0].Segments[0].RunIdx != 0 || l.Lines[0].Segments[1].RunIdx != 1 {
		t.Error("сегменты перепутали раны")
	}
	// Второй сегмент начинается там, где кончился первый.
	s0, s1 := l.Lines[0].Segments[0], l.Lines[0].Segments[1]
	if s1.X != s0.X+s0.W {
		t.Errorf("сегменты не стыкуются: %d+%d != %d", s0.X, s0.W, s1.X)
	}
}

// Главное: высота строки — по самому высокому рану СТРОКИ, а не по базовому
// кеглю виджета, и все сегменты стоят на одной базовой линии.
func TestRichLayout_MixedSizesLineHeightAndBaseline(t *testing.T) {
	l := layoutOf(500, para(
		RichRun{Text: "small "},         // 10 pt: подъём 8, спуск 2
		RichRun{Text: "BIG", Size: 30},  // 30 pt: подъём 24, спуск 6
		RichRun{Text: " tiny", Size: 5}, // 5 pt: подъём 4, спуск 1
	))
	if len(l.Lines) != 1 {
		t.Fatalf("строк %d, ждали 1", len(l.Lines))
	}
	ln := l.Lines[0]
	if ln.Height != 30 {
		t.Errorf("высота строки %d, ждали 30 (подъём 24 + спуск 6 крупного рана); "+
			"по базовому кеглю вышло бы 10", ln.Height)
	}
	if ln.Baseline != 24 {
		t.Errorf("базовая линия на %d от верха строки, ждали 24", ln.Baseline)
	}
	// Верх каждого сегмента при рисовании = Baseline - Ascent; низ базовой линии
	// у всех один: верх + подъём == Baseline.
	wantTop := map[int]int{0: 16, 1: 0, 2: 20}
	for _, s := range ln.Segments {
		if top := ln.Baseline - s.Ascent; top != wantTop[s.RunIdx] {
			t.Errorf("ран %d: верх на %d, ждали %d", s.RunIdx, top, wantTop[s.RunIdx])
		}
		if (ln.Baseline-s.Ascent)+s.Ascent != ln.Baseline {
			t.Errorf("ран %d не стоит на общей базовой линии", s.RunIdx)
		}
		if s.Run.Size == 0 {
			t.Errorf("ран %d: действующий кегль не проставлен", s.RunIdx)
		}
	}
	// Строка из одних мелких ранов остаётся невысокой: растёт только та строка,
	// где есть крупный ран.
	l2 := layoutOf(500, para(RichRun{Text: "big", Size: 30}), para(RichRun{Text: "small"}))
	if l2.Lines[0].Height != 30 || l2.Lines[1].Height != 10 {
		t.Errorf("высоты строк %d и %d, ждали 30 и 10", l2.Lines[0].Height, l2.Lines[1].Height)
	}
	if l2.Lines[1].Y != 30 || l2.Height != 40 {
		t.Errorf("Y второй строки %d, высота %d; ждали 30 и 40", l2.Lines[1].Y, l2.Height)
	}
}

// Крупный ран, перенесённый на вторую строку, раздвигает ТОЛЬКО её.
func TestRichLayout_BigRunOnWrappedLine(t *testing.T) {
	// "aaa bbb " = 8*5 = 40 px, "XX" = 2*10 = 20 px: вместе 60 > 50.
	l := layoutOf(50, para(RichRun{Text: "aaa bbb "}, RichRun{Text: "XX", Size: 20}))
	if len(l.Lines) != 2 {
		t.Fatalf("строк %d, ждали 2", len(l.Lines))
	}
	if l.Lines[0].Height != 10 || l.Lines[1].Height != 20 {
		t.Errorf("высоты %d и %d, ждали 10 и 20", l.Lines[0].Height, l.Lines[1].Height)
	}
	if l.Lines[1].Y != 10 {
		t.Errorf("вторая строка на Y=%d, ждали 10", l.Lines[1].Y)
	}
}

// Шрифт по умолчанию виджета подставляется только там, где ран своего не задал.
func TestRichLayout_DefaultsAndEffectiveFace(t *testing.T) {
	l := layoutRich([]RichParagraph{para(RichRun{Text: "a"}, RichRun{Text: "b", Font: "wide", Size: 20})},
		richLayoutOpts{Width: 100, Font: "mono", Size: 12, M: fakeRichMeasurer{}})
	s := l.Lines[0].Segments
	if s[0].Run.Font != "mono" || s[0].Run.Size != 12 {
		t.Errorf("умолчания не подставлены: %q %v", s[0].Run.Font, s[0].Run.Size)
	}
	if s[1].Run.Font != "wide" || s[1].Run.Size != 20 {
		t.Errorf("заданное раном затёрто: %q %v", s[1].Run.Font, s[1].Run.Size)
	}
	// "wide" вдвое шире: 20/2*2 = 20 px на руну.
	if s[1].W != 20 {
		t.Errorf("ширина сегмента %d, ждали 20", s[1].W)
	}
}

// Жёсткий перенос: слово длиннее строки режется по символам и строка
// заполняется до отказа.
func TestRichLayout_HardBreak(t *testing.T) {
	// Ширина 50 px = 10 символов; слово — 25 символов.
	l := layoutOf(50, para(RichRun{Text: strings.Repeat("x", 25)}))
	want := []string{strings.Repeat("x", 10), strings.Repeat("x", 10), strings.Repeat("x", 5)}
	if got := lineTexts(l); !eqStrings(got, want) {
		t.Fatalf("строки %q, ждали %q", got, want)
	}
	if l.Lines[1].Start != 10 || l.Lines[2].Start != 20 || l.Lines[2].End != 25 {
		t.Errorf("смещения после жёсткого переноса: %d, %d..%d",
			l.Lines[1].Start, l.Lines[2].Start, l.Lines[2].End)
	}

	// Хвост жёсткого переноса делит строку со следующим словом.
	l = layoutOf(50, para(RichRun{Text: strings.Repeat("x", 12) + " yy"}))
	want = []string{strings.Repeat("x", 10), "xx yy"}
	if got := lineTexts(l); !eqStrings(got, want) {
		t.Fatalf("строки %q, ждали %q", got, want)
	}

	// Слово, начинающееся в середине строки и не помещающееся даже на пустой:
	// сначала уходит на новую строку, потом режется.
	l = layoutOf(50, para(RichRun{Text: "ab " + strings.Repeat("y", 15)}))
	want = []string{"ab ", strings.Repeat("y", 10), strings.Repeat("y", 5)}
	if got := lineTexts(l); !eqStrings(got, want) {
		t.Fatalf("строки %q, ждали %q", got, want)
	}
}

// Область уже одного символа не должна зациклить раскладку: по символу на
// строку, но каждый символ попадает в документ.
func TestRichLayout_HardBreakNarrowerThanGlyph(t *testing.T) {
	l := layoutOf(3, para(RichRun{Text: "abc"})) // символ 5 px > 3 px
	if got := lineTexts(l); !eqStrings(got, []string{"a", "b", "c"}) {
		t.Fatalf("строки %q", got)
	}
}

// Жёсткий перенос внутри слова, склеенного из нескольких ранов с разными
// кеглями: режутся куски своим шрифтом.
func TestRichLayout_HardBreakAcrossRuns(t *testing.T) {
	l := layoutOf(50, para(RichRun{Text: "aaaaaaa"}, RichRun{Text: "BBBBB", Size: 20}))
	// 7*5 = 35 px, затем по 10 px на «B»: на первой строке ещё 1 символ (45), остальные — дальше.
	got := lineTexts(l)
	if strings.Join(got, "") != "aaaaaaaBBBBB" {
		t.Fatalf("потеряны символы: %q", got)
	}
	for i, ln := range l.Lines {
		if w := ln.Width; w > 50 && len(ln.Segments) > 1 {
			t.Errorf("строка %d шире области: %d", i, w)
		}
	}
	if got[0] != "aaaaaaaB" {
		t.Errorf("первая строка %q, ждали aaaaaaaB", got[0])
	}
}

// Явный перевод строки внутри рана: тот же абзац, новая строка; пустая строка
// между переводами тоже строка, а её высота — по рану.
func TestRichLayout_ExplicitNewline(t *testing.T) {
	l := layoutOf(500, para(RichRun{Text: "ab\n\ncd"}))
	if got := lineTexts(l); !eqStrings(got, []string{"ab", "", "cd"}) {
		t.Fatalf("строки %q", got)
	}
	if l.Lines[0].End != 2 || l.Lines[1].Start != 3 || l.Lines[1].End != 3 || l.Lines[2].Start != 4 {
		t.Errorf("смещения строк: %d..%d, %d..%d, %d..",
			l.Lines[0].Start, l.Lines[0].End, l.Lines[1].Start, l.Lines[1].End, l.Lines[2].Start)
	}
	if !l.Lines[0].HardEnd || !l.Lines[1].HardEnd {
		t.Error("строка, оканчивающаяся переводом, должна быть HardEnd")
	}
	if l.Lines[1].Height != 10 {
		t.Errorf("высота пустой строки %d, ждали 10", l.Lines[1].Height)
	}

	// Пустая строка берёт высоту у рана, в котором стоит перевод.
	l = layoutOf(500, para(RichRun{Text: "a"}, RichRun{Text: "\n\n", Size: 30}, RichRun{Text: "b"}))
	if len(l.Lines) != 3 {
		t.Fatalf("строк %d, ждали 3", len(l.Lines))
	}
	if l.Lines[1].Height != 30 {
		t.Errorf("пустая строка в крупном ране: высота %d, ждали 30", l.Lines[1].Height)
	}

	// Текст, оканчивающийся переводом, оставляет пустую строку в конце.
	l = layoutOf(500, para(RichRun{Text: "a\n"}))
	if got := lineTexts(l); !eqStrings(got, []string{"a", ""}) {
		t.Errorf("строки %q, ждали [a ]", got)
	}
}

// Абзацы: разделитель занимает позицию, пустой абзац — строка, отступы
// складываются в Y.
func TestRichLayout_ParagraphsSpacingAndEmpty(t *testing.T) {
	l := layoutOf(500,
		RichParagraph{Runs: []RichRun{{Text: "one"}}, SpaceAfter: 4},
		RichParagraph{},
		RichParagraph{Runs: []RichRun{{Text: "two"}}, SpaceBefore: 6},
	)
	if len(l.Lines) != 3 {
		t.Fatalf("строк %d, ждали 3", len(l.Lines))
	}
	// one: Y=0 h=10, +4 после; пустой: Y=14 h=10; two: +6 до → Y=30.
	ys := []int{l.Lines[0].Y, l.Lines[1].Y, l.Lines[2].Y}
	if ys[0] != 0 || ys[1] != 14 || ys[2] != 30 {
		t.Errorf("Y строк %v, ждали [0 14 30]", ys)
	}
	if l.Height != 40 {
		t.Errorf("высота %d, ждали 40", l.Height)
	}
	// Смещения: "one" 0..3, разделитель, пустой 4..4, разделитель, "two" 5..8.
	if l.ParaStart[0] != 0 || l.ParaStart[1] != 4 || l.ParaStart[2] != 5 || l.TextLen != 8 {
		t.Errorf("начала абзацев %v, длина %d", l.ParaStart, l.TextLen)
	}
	if l.Lines[1].Start != 4 || l.Lines[1].End != 4 {
		t.Errorf("пустой абзац: %d..%d", l.Lines[1].Start, l.Lines[1].End)
	}

	// Нет абзацев — нет строк.
	if e := layoutOf(100); len(e.Lines) != 0 || e.Height != 0 || e.TextLen != 0 {
		t.Errorf("пустой документ: строк %d, высота %d", len(e.Lines), e.Height)
	}
}

// Выравнивание и отступ: сдвиг считается по ширине без хвостовых пробелов.
func TestRichLayout_AlignAndIndent(t *testing.T) {
	text := RichRun{Text: "abcd  "} // 4 символа + два висящих пробела
	cases := []struct {
		name   string
		align  TextAlign
		indent int
		wantX  int
	}{
		{"слева", TextAlignLeft, 0, 0},
		{"слева с отступом", TextAlignLeft, 12, 12},
		{"по центру", TextAlignCenter, 0, (100 - 20) / 2},
		{"вправо", TextAlignRight, 0, 100 - 20},
		{"вправо с отступом", TextAlignRight, 10, 10 + (90 - 20)},
		{"по центру с отступом", TextAlignCenter, 10, 10 + (90-20)/2},
	}
	for _, c := range cases {
		l := layoutOf(100, RichParagraph{Runs: []RichRun{text}, Align: c.align, Indent: c.indent})
		ln := l.Lines[0]
		if ln.X != c.wantX || ln.Segments[0].X != c.wantX {
			t.Errorf("%s: X=%d (сегмент %d), ждали %d", c.name, ln.X, ln.Segments[0].X, c.wantX)
		}
		if ln.Segments[0].W != 20 {
			t.Errorf("%s: ширина сегмента %d, ждали 20 без пробелов", c.name, ln.Segments[0].W)
		}
		if ln.Segments[0].Adv != 30 {
			t.Errorf("%s: полная ширина %d, ждали 30", c.name, ln.Segments[0].Adv)
		}
	}

	// Каждая строка выравнивается по-своему: у короткой больше сдвиг.
	// 7 символов = 35 px не влезают в 30 px: "aaaa " (20 px) и "bb" (10 px).
	l := layoutOf(30, RichParagraph{Runs: []RichRun{{Text: "aaaa bb"}}, Align: TextAlignRight})
	if len(l.Lines) != 2 {
		t.Fatalf("строк %d, ждали 2", len(l.Lines))
	}
	if l.Lines[0].X != 30-20 || l.Lines[1].X != 30-10 {
		t.Errorf("сдвиги строк %d и %d, ждали 10 и 20", l.Lines[0].X, l.Lines[1].X)
	}
}

// Ширина не задана — переносов нет, только явные переводы.
func TestRichLayout_NoWidthNoWrap(t *testing.T) {
	l := layoutOf(0, para(RichRun{Text: strings.Repeat("word ", 50) + "\nnext"}))
	if len(l.Lines) != 2 {
		t.Fatalf("строк %d, ждали 2", len(l.Lines))
	}
}

// Свойство: что бы ни пришло, текст строк восстанавливает текст абзацев
// (кроме переводов строки), смещения непрерывны, сегменты стыкуются, а строка
// не шире области — кроме одного неделимого символа.
func TestRichLayout_RandomInvariants(t *testing.T) {
	rnd := rand.New(rand.NewSource(20261003))
	words := []string{"слово", "a", "ii", "длинноеслово", "мама", "x", "переносимоеслововнутристроки",
		"WWW", "", " ", "  ", "\n"}
	sizes := []float64{0, 8, 10, 14, 24}
	fonts := []string{"", "wide", "bold"}
	for tc := 0; tc < 300; tc++ {
		var paras []RichParagraph
		for p := 0; p < 1+rnd.Intn(4); p++ {
			var runs []RichRun
			for r := 0; r < rnd.Intn(6); r++ {
				var sb strings.Builder
				for w := 0; w < 1+rnd.Intn(8); w++ {
					sb.WriteString(words[rnd.Intn(len(words))])
					if rnd.Intn(2) == 0 {
						sb.WriteByte(' ')
					}
				}
				runs = append(runs, RichRun{Text: sb.String(), Size: sizes[rnd.Intn(len(sizes))],
					Font: fonts[rnd.Intn(len(fonts))]})
			}
			paras = append(paras, RichParagraph{Runs: runs, Indent: rnd.Intn(20),
				Align: TextAlign(rnd.Intn(3)), SpaceBefore: rnd.Intn(5)})
		}
		width := 30 + rnd.Intn(200)
		l := layoutOf(width, paras...)
		doc := richBuildDoc(paras)

		if l.TextLen != len(doc.runes) {
			t.Fatalf("случай %d: длина %d, документ %d", tc, l.TextLen, len(doc.runes))
		}
		prevEnd := -1
		for li, ln := range l.Lines {
			if ln.Height <= 0 {
				t.Fatalf("случай %d, строка %d: высота %d", tc, li, ln.Height)
			}
			// Смещения строки совпадают с текстом документа.
			var sb strings.Builder
			for si, s := range ln.Segments {
				sb.WriteString(s.Run.Text)
				if si > 0 && s.X != ln.Segments[si-1].X+ln.Segments[si-1].W {
					t.Fatalf("случай %d, строка %d: сегменты не стыкуются", tc, li)
				}
				if got := string(doc.runes[s.Start:s.End]); got != s.Run.Text {
					t.Fatalf("случай %d, строка %d: сегмент %q, документ %q", tc, li, s.Run.Text, got)
				}
				if s.Ascent > ln.Baseline {
					t.Fatalf("случай %d: подъём сегмента %d выше базовой линии строки %d",
						tc, s.Ascent, ln.Baseline)
				}
			}
			if got := string(doc.runes[ln.Start:ln.End]); got != sb.String() {
				t.Fatalf("случай %d, строка %d: строка %q, документ %q", tc, li, sb.String(), got)
			}
			// Строки идут подряд: следующая начинается там, где кончилась
			// прошлая, либо через один разделитель (перевод строки/абзац).
			if prevEnd >= 0 && ln.Start != prevEnd && ln.Start != prevEnd+1 {
				t.Fatalf("случай %d, строка %d: разрыв, прошлая кончилась на %d, эта с %d",
					tc, li, prevEnd, ln.Start)
			}
			prevEnd = ln.End
			// Строка с несколькими символами обязана влезать в область: шире
			// может быть только неделимый символ.
			avail := width - paras[ln.Para].Indent
			if avail < 1 {
				avail = 1
			}
			if runesIn(sb.String()) > 1 && ln.Width > avail {
				t.Fatalf("случай %d, строка %d: ширина %d > %d (%q)", tc, li, ln.Width, avail, sb.String())
			}
		}
	}
}

func runesIn(s string) int { return len([]rune(strings.TrimRight(s, " "))) }

// Попадание мышью: ближайшая граница между символами, края и промежутки.
func TestRichLayout_HitTest(t *testing.T) {
	m := fakeRichMeasurer{}
	// Две строки по 5 px на символ: "abcd " и "efgh".
	l := layoutOf(25, para(RichRun{Text: "abcd efgh"}))
	if got := lineTexts(l); !eqStrings(got, []string{"abcd ", "efgh"}) {
		t.Fatalf("строки %q", got)
	}
	cases := []struct {
		x, y, want int
		name       string
	}{
		{0, 0, 0, "левый край"},
		{2, 0, 0, "ближе к левой границе буквы"},
		{3, 0, 1, "ближе к правой границе"},
		{7, 0, 1, "середина «b» — левее центра"},
		{8, 0, 2, "середина «b» — правее центра"},
		{19, 0, 4, "правый край «d»"},
		{200, 0, 5, "правее строки — её конец"},
		{-10, 0, 0, "левее текста — начало строки"},
		{12, 12, 7, "вторая строка"},
		{500, 12, 9, "конец второй строки"},
		{2, 5000, 5, "ниже всего — последняя строка"},
		{2, -50, 0, "выше всего — первая строка"},
	}
	for _, c := range cases {
		if got := l.offsetAt(m, c.x, c.y); got != c.want {
			t.Errorf("%s: (%d,%d) → %d, ждали %d", c.name, c.x, c.y, got, c.want)
		}
	}
}

// Попадание в сегмент с ссылкой — только над буквами, а не рядом.
func TestRichLayout_SegmentAt(t *testing.T) {
	l := layoutOf(500, para(RichRun{Text: "ab "}, RichRun{Text: "link", Link: "http://x"}, RichRun{Text: " end"}))
	if s := l.segmentAt(2, 3); s == nil || s.Run.Link != "" {
		t.Errorf("над «ab»: %+v", s)
	}
	// «ab » — 15 px, «link» — 15..35.
	if s := l.segmentAt(20, 3); s == nil || s.Run.Link != "http://x" {
		t.Errorf("над ссылкой: %+v", s)
	}
	if s := l.segmentAt(20, 50); s != nil {
		t.Errorf("под строкой нашлась ссылка: %+v", s)
	}
	if s := l.segmentAt(20, -1); s != nil {
		t.Errorf("над строкой нашлась ссылка: %+v", s)
	}
	if s := l.segmentAt(900, 3); s != nil {
		t.Errorf("правее текста нашлась ссылка: %+v", s)
	}
}

// Геометрия выделения: отрезок строки в пикселях.
func TestRichLayout_SelectionX(t *testing.T) {
	m := fakeRichMeasurer{}
	l := layoutOf(500, para(RichRun{Text: "abc"}, RichRun{Text: "def", Size: 20}))
	ln := &l.Lines[0]
	// «abc» — 15 px (по 5), «def» — по 10 px.
	cases := []struct {
		lo, hi, x0, x1 int
		ok             bool
	}{
		{0, 6, 0, 45, true},
		{1, 2, 5, 10, true},
		{2, 4, 10, 25, true}, // конец «abc» и первая «d»
		{4, 6, 25, 45, true},
		{6, 9, 0, 0, false},
	}
	for _, c := range cases {
		x0, x1, ok := l.selectionX(m, ln, c.lo, c.hi)
		if ok != c.ok || (ok && (x0 != c.x0 || x1 != c.x1)) {
			t.Errorf("[%d,%d): (%d,%d,%v), ждали (%d,%d,%v)", c.lo, c.hi, x0, x1, ok, c.x0, c.x1, c.ok)
		}
	}
}

// Подчёркивание и зачёркивание следуют метрикам шрифта: крупный шрифт — линия
// толще и дальше от базовой, и всегда внутри строки.
func TestRichDecoration_ScalesWithFont(t *testing.T) {
	prevThick, prevUl := 0, 0
	for _, size := range []float64{8, 10, 16, 24, 48, 96} {
		fm := fakeRichMeasurer{}.FontMetrics(size, "")
		ul, strike, thick := richDecoration(fm.Ascent, fm.Descent)
		if thick < 1 || ul < 1 {
			t.Errorf("кегль %v: толщина %d, подчёркивание на %d", size, thick, ul)
		}
		if ul+thick > fm.Descent && fm.Descent >= 2 {
			t.Errorf("кегль %v: подчёркивание (%d+%d) вышло за спуск %d", size, ul, thick, fm.Descent)
		}
		if strike >= 0 || -strike > fm.Ascent {
			t.Errorf("кегль %v: зачёркивание на %d вне подъёма %d", size, strike, fm.Ascent)
		}
		if thick < prevThick || ul < prevUl {
			t.Errorf("кегль %v: линия стала тоньше или ближе при росте кегля", size)
		}
		prevThick, prevUl = thick, ul
	}
	// Конкретные числа для обычного кегля, чтобы константы не поплыли.
	if ul, strike, thick := richDecoration(8, 2); ul != 1 || thick != 1 || strike != -2 {
		t.Errorf("10 pt: ul=%d strike=%d thick=%d", ul, strike, thick)
	}
}
