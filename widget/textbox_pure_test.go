package widget

import (
	"image"
	"image/color"
	"math/rand"
	"testing"
)

// Чистая логика редактора: табстопы, куски по стилям, выделение, геометрия
// полосы. Без отрисовки и без виджета — ошибка здесь ломает все кадры сразу.

// ─── Табстопы ────────────────────────────────────────────────────────────────

func TestTabStopAfter(t *testing.T) {
	for _, tc := range []struct{ x, tabW, want int }{
		{0, 32, 32},  // с нуля — к первой ячейке
		{5, 32, 32},  // внутри ячейки — к её концу
		{31, 32, 32}, // за пиксель до границы
		{32, 32, 64}, // ровно на границе — всё равно к следующей: табуляция не нулевая
		{70, 32, 96},
		{7, 0, 7}, // без табстопов x не меняется
	} {
		if got := tabStopAfter(tc.x, tc.tabW); got != tc.want {
			t.Errorf("tabStopAfter(%d, %d) = %d, ждали %d", tc.x, tc.tabW, got, tc.want)
		}
	}
}

// measure8 — измеритель «восемь пикселей на руну».
func measure8(s string) int { return 8 * len([]rune(s)) }

func TestMeasureTabbed(t *testing.T) {
	const tabW = 32
	cases := []struct {
		name string
		text string
		tabW int
		want int
	}{
		{"пусто", "", tabW, 0},
		{"без табуляции", "abc", tabW, 24},
		{"буква и табуляция", "a\tb", tabW, 32 + 8},
		{"одна табуляция", "\t", tabW, 32},
		{"две подряд", "\t\t", tabW, 64},
		{"ровно на границе сетки", "abcd\tx", tabW, 64 + 8}, // 'abcd' = 32 = граница -> следующая 64
		{"внутри ячейки", "abc\tx", tabW, 32 + 8},
		{"две табуляции с текстом", "a\tb\tc", tabW, 64 + 8},
		{"без табстопов", "a\tb", 0, 24},
		{"табуляция в конце", "ab\t", tabW, 32},
	}
	for _, tc := range cases {
		if got := measureTabbed(tc.text, tc.tabW, measure8); got != tc.want {
			t.Errorf("%s: measureTabbed(%q, %d) = %d, ждали %d", tc.name, tc.text, tc.tabW, got, tc.want)
		}
	}
}

// Ширина префикса не убывает с длиной: иначе деление пополам в раскладке
// (firstOverflow, colAtX) промахивается.
func TestMeasureTabbed_MonotonicPrefix(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	alphabet := []rune("ab \t\tcd")
	for iter := 0; iter < 200; iter++ {
		n := rnd.Intn(30)
		rs := make([]rune, n)
		for i := range rs {
			rs[i] = alphabet[rnd.Intn(len(alphabet))]
		}
		prev := 0
		for k := 0; k <= n; k++ {
			w := measureTabbed(string(rs[:k]), 24, measure8)
			if w < prev {
				t.Fatalf("%q: ширина префикса %d (%d) меньше предыдущей %d", string(rs), k, w, prev)
			}
			prev = w
		}
	}
}

func TestSplitTabRuns(t *testing.T) {
	rs := []rune("\ta\t\tbc\t")
	got := splitTabRuns(rs, 0, len(rs))
	want := []tabRun{{1, 2}, {4, 6}}
	if len(got) != len(want) {
		t.Fatalf("отрезки %v, ждали %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("отрезки %v, ждали %v", got, want)
		}
	}
	// Подотрезок строки режется по тем же правилам.
	if got := splitTabRuns(rs, 2, 5); len(got) != 1 || got[0] != (tabRun{4, 5}) {
		t.Errorf("отрезки %v, ждали [{4 5}]", got)
	}
	if got := splitTabRuns(rs, 3, 4); len(got) != 0 {
		t.Errorf("между двумя табуляциями отрезков быть не должно, а есть %v", got)
	}
}

// ─── Куски по стилям ─────────────────────────────────────────────────────────

var (
	stRed  = Style{Color: color.RGBA{R: 255, A: 255}}
	stBlue = Style{Color: color.RGBA{B: 255, A: 255}}
	stBG   = Style{BG: color.RGBA{R: 1, G: 2, B: 3, A: 255}}
)

// refPieces — эталон: стиль каждой руны считается в лоб, затем склеивается.
func refPieces(n, off int, spans []Span) []tbPiece {
	if n <= 0 {
		return nil
	}
	per := make([]Style, n)
	for _, sp := range spans {
		for i := 0; i < n; i++ {
			if p := i + off; p >= sp.From && p < sp.To {
				per[i] = mergeStyle(per[i], sp.Style)
			}
		}
	}
	var out []tbPiece
	for i := 0; i < n; i++ {
		if m := len(out); m > 0 && out[m-1].Style == per[i] {
			out[m-1].To = i + 1
			continue
		}
		out = append(out, tbPiece{From: i, To: i + 1, Style: per[i]})
	}
	return out
}

func samePieces(a, b []tbPiece) bool {
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

func TestSplitStylePieces_Basic(t *testing.T) {
	// Нет диапазонов — строка одним куском со стилем «как у виджета».
	if got := splitStylePieces(5, 0, nil); !samePieces(got, []tbPiece{{0, 5, Style{}}}) {
		t.Errorf("без диапазонов: %v", got)
	}
	// Пустая строка кусков не даёт.
	if got := splitStylePieces(0, 0, []Span{{0, 3, stRed}}); got != nil {
		t.Errorf("пустая строка: %v", got)
	}
	// Диапазон посередине даёт три куска, промежутки — нулевой стиль.
	got := splitStylePieces(10, 0, []Span{{3, 6, stRed}})
	want := []tbPiece{{0, 3, Style{}}, {3, 6, stRed}, {6, 10, Style{}}}
	if !samePieces(got, want) {
		t.Errorf("диапазон посередине: %v, ждали %v", got, want)
	}
	// Выходящий за строку обрезается, пустой и перевёрнутый игнорируются.
	got = splitStylePieces(5, 0, []Span{{-4, 2, stRed}, {4, 99, stBlue}, {3, 3, stBG}, {4, 1, stBG}})
	want = []tbPiece{{0, 2, stRed}, {2, 4, Style{}}, {4, 5, stBlue}}
	if !samePieces(got, want) {
		t.Errorf("обрезка и мусор: %v, ждали %v", got, want)
	}
	// Соседние равные стили склеиваются.
	got = splitStylePieces(6, 0, []Span{{0, 2, stRed}, {2, 4, stRed}})
	want = []tbPiece{{0, 4, stRed}, {4, 6, Style{}}}
	if !samePieces(got, want) {
		t.Errorf("склейка: %v, ждали %v", got, want)
	}
}

// Цвет от одного диапазона и фон от другого складываются в одном куске, а при
// споре за одно поле побеждает более поздний в срезе.
func TestSplitStylePieces_OverlapMerges(t *testing.T) {
	got := splitStylePieces(8, 0, []Span{
		{0, 8, stRed},  // синтаксис: красный на всю строку
		{2, 5, stBG},   // поиск: фон поверх
		{4, 6, stBlue}, // более поздний перекрывает цвет красного
	})
	merged := mergeStyle(stRed, stBG)
	over := mergeStyle(merged, stBlue)
	bluePlain := mergeStyle(stRed, stBlue)
	want := []tbPiece{
		{0, 2, stRed},
		{2, 4, merged},
		{4, 5, over},
		{5, 6, bluePlain},
		{6, 8, stRed},
	}
	if !samePieces(got, want) {
		t.Errorf("перекрытие:\n got  %v\n want %v", got, want)
	}
}

// Видимая строка — кусок логической: диапазоны сдвигаются на её начало.
func TestSplitStylePieces_LineOffset(t *testing.T) {
	// Абзац "aaaa bbbb cccc", диапазон на "bbbb" = [5,9); видимая строка
	// "bbbb" начинается с 5-й руны абзаца.
	got := splitStylePieces(4, 5, []Span{{5, 9, stRed}})
	if !samePieces(got, []tbPiece{{0, 4, stRed}}) {
		t.Errorf("со сдвигом: %v", got)
	}
	// А соседняя строка этого же абзаца диапазона не касается.
	got = splitStylePieces(4, 10, []Span{{5, 9, stRed}})
	if !samePieces(got, []tbPiece{{0, 4, Style{}}}) {
		t.Errorf("чужая строка абзаца: %v", got)
	}
}

func TestSplitStylePieces_RandomMatchesReference(t *testing.T) {
	rnd := rand.New(rand.NewSource(7))
	styles := []Style{stRed, stBlue, stBG, {Face: "b"}, mergeStyle(stRed, stBG), {}}
	for iter := 0; iter < 2000; iter++ {
		n := rnd.Intn(25)
		off := rnd.Intn(10)
		spans := make([]Span, rnd.Intn(8))
		for i := range spans {
			f := rnd.Intn(n+12) - 4
			spans[i] = Span{f, f + rnd.Intn(10) - 1, styles[rnd.Intn(len(styles))]}
		}
		got := splitStylePieces(n, off, spans)
		want := refPieces(n, off, spans)
		if n > 0 && len(spans) == 0 {
			want = []tbPiece{{0, n, Style{}}}
		}
		if !samePieces(got, want) {
			t.Fatalf("n=%d off=%d spans=%v\n got  %v\n want %v", n, off, spans, got, want)
		}
		// Куски покрывают строку целиком и без дыр.
		pos := 0
		for _, p := range got {
			if p.From != pos || p.To <= p.From {
				t.Fatalf("дыра или пустой кусок в %v", got)
			}
			pos = p.To
		}
		if n > 0 && pos != n {
			t.Fatalf("куски кончаются на %d, строка %d: %v", pos, n, got)
		}
	}
}

func TestMergeStyle(t *testing.T) {
	base := Style{Color: color.RGBA{R: 9, A: 255}, Face: "mono"}
	got := mergeStyle(base, Style{BG: stBG.BG})
	if got.Color != base.Color || got.Face != "mono" || got.BG != stBG.BG {
		t.Errorf("пустые поля поверх не должны затирать: %+v", got)
	}
	got = mergeStyle(base, Style{Face: "bold"})
	if got.Face != "bold" || got.Color != base.Color {
		t.Errorf("гарнитура поверх: %+v", got)
	}
}

// ─── Выделение на строке ─────────────────────────────────────────────────────

func TestTbSelOnLine(t *testing.T) {
	ln := tbLine{start: 10, end: 20}
	for _, tc := range []struct {
		name         string
		lo, hi       int
		wantLo, wHi  int
		spill, shown bool
	}{
		{"целиком внутри", 12, 15, 12, 15, false, true},
		{"слева от строки", 2, 10, 0, 0, false, false},
		{"начинается раньше", 2, 14, 10, 14, false, true},
		{"уходит на следующую", 15, 30, 15, 20, true, true},
		{"ровно с конца строки: только перевод", 20, 25, 20, 20, true, true},
		{"после перевода", 21, 25, 0, 0, false, false},
		{"нет выделения", -1, -1, 0, 0, false, false},
	} {
		lo, hi, spill, ok := tbSelOnLine(ln, tc.lo, tc.hi)
		if ok != tc.shown || (ok && (lo != tc.wantLo || hi != tc.wHi || spill != tc.spill)) {
			t.Errorf("%s: (%d,%d,%v,%v), ждали (%d,%d,%v,%v)", tc.name, lo, hi, spill, ok, tc.wantLo, tc.wHi, tc.spill, tc.shown)
		}
	}
}

// ─── Полоса прокрутки ────────────────────────────────────────────────────────

func TestTbScrollXMax(t *testing.T) {
	for _, tc := range []struct{ content, view, want int }{
		{100, 200, 0},    // помещается — смещать нечего
		{200, 200, 4},    // ровно по ширине: остаётся запас под каретку в конце
		{1000, 200, 804}, // пролистывается весь текст плюс запас под каретку
	} {
		if got := tbScrollXMax(tc.content, tc.view); got != tc.want {
			t.Errorf("tbScrollXMax(%d, %d) = %d, ждали %d", tc.content, tc.view, got, tc.want)
		}
	}
	if got := tbClampScrollX(-5, 1000, 200); got != 0 {
		t.Errorf("отрицательное смещение зажато в %d", got)
	}
	if got := tbClampScrollX(5000, 1000, 200); got != 804 {
		t.Errorf("слишком большое смещение зажато в %d", got)
	}
	if got := tbClampScrollX(300, 1000, 200); got != 300 {
		t.Errorf("допустимое смещение изменено на %d", got)
	}
}

func TestTbNeedHBar(t *testing.T) {
	if tbNeedHBar(true, 5000, 200) {
		t.Error("с переносом по словам полоса не нужна")
	}
	if tbNeedHBar(false, 200, 200) {
		t.Error("строка по ширине области помещается")
	}
	if !tbNeedHBar(false, 201, 200) {
		t.Error("строка шире области — полоса нужна")
	}
}

func TestTbHBarGeometry(t *testing.T) {
	b := image.Rect(10, 20, 210, 120)
	tr := tbHBarTrack(b, 6, 181)
	if tr.Min.X != 16 || tr.Dx() != 181 {
		t.Errorf("трек %v: должен стоять под текстом, слева с отступом, шириной в область", tr)
	}
	if tr.Max.Y > b.Max.Y-1 || tr.Min.Y < b.Max.Y-tbHBarH {
		t.Errorf("трек %v выходит за отведённую полосу высотой %d", tr, tbHBarH)
	}
	hit := tbHBarHit(b, 6, 181)
	if !tr.In(hit) || hit.Max.Y != b.Max.Y {
		t.Errorf("зона нажатия %v не накрывает трек %v", hit, tr)
	}
	if !image.Pt(tr.Min.X-2, tr.Min.Y).In(hit) {
		t.Error("зона нажатия должна иметь запас по сторонам трека")
	}
}

// ─── Поиск строки по позиции ─────────────────────────────────────────────────

func TestLineOfPos_MatchesLinearScan(t *testing.T) {
	rnd := rand.New(rand.NewSource(3))
	linear := func(lines []tbLine, pos int) int {
		for i, ln := range lines {
			if pos >= ln.start && pos <= ln.end {
				return i
			}
		}
		return len(lines) - 1
	}
	for iter := 0; iter < 500; iter++ {
		// Строки, как их строит раскладка: стык по пробелу (+1), по '\n' (+1)
		// и разрез длинного слова (стык без зазора).
		var lines []tbLine
		pos := 0
		for k := 0; k < 1+rnd.Intn(30); k++ {
			ln := tbLine{start: pos, end: pos + rnd.Intn(12)}
			lines = append(lines, ln)
			pos = ln.end
			if rnd.Intn(3) > 0 {
				pos++
			}
		}
		for p := 0; p <= pos+2; p++ {
			if got, want := lineOfPos(lines, p), linear(lines, p); got != want {
				t.Fatalf("lines=%v pos=%d: бинарный %d, обход %d", lines, p, got, want)
			}
		}
	}
}
