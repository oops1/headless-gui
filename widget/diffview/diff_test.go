package diffview

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Участки покрывают оба набора подряд, совпадения действительно совпадают, а
// из правых частей собирается правый текст — на тысячах случайных пар.
func TestLinesReconstructs(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	words := []string{"a", "b", "c", "d", "", "e"}
	gen := func() []string {
		out := make([]string, rng.Intn(40))
		for i := range out {
			out[i] = words[rng.Intn(len(words))]
		}
		return out
	}
	for iter := 0; iter < 3000; iter++ {
		a, b := gen(), gen()
		changes := Lines(a, b, false)
		var got []string
		li, ri := 0, 0
		for i, c := range changes {
			if c.LeftFrom != li || c.RightFrom != ri {
				t.Fatalf("разрыв на участке %d: %+v", i, c)
			}
			if i > 0 && c.Kind == Equal && changes[i-1].Kind == Equal {
				t.Fatal("два совпадения подряд")
			}
			if c.Kind == Equal && !slices.Equal(a[c.LeftFrom:c.LeftTo], b[c.RightFrom:c.RightTo]) {
				t.Fatalf("совпадение не совпадает: %+v", c)
			}
			got = append(got, b[c.RightFrom:c.RightTo]...)
			li, ri = c.LeftTo, c.RightTo
		}
		if li != len(a) || ri != len(b) || !slices.Equal(got, b) {
			t.Fatalf("правый текст не собрался: a=%q b=%q участки=%+v", a, b, changes)
		}
	}
}

func TestLinesMinimal(t *testing.T) {
	a := []string{"x", "a", "b", "c", "y"}
	b := []string{"x", "a", "B", "c", "y", "z"}
	want := []Change{
		{Equal, 0, 2, 0, 2},
		{Replace, 2, 3, 2, 3},
		{Equal, 3, 5, 3, 5},
		{Insert, 5, 5, 5, 6},
	}
	if got := Lines(a, b, false); !slices.Equal(got, want) {
		t.Fatalf("получено %+v", got)
	}
}

// Без учёта пробелов строки, отличающиеся только отступом, совпадают.
func TestLinesIgnoreWhitespace(t *testing.T) {
	a := []string{"if x {", "\treturn 1", "}"}
	b := []string{"if x {", "    return  1", "}"}
	if got := Lines(a, b, true); len(got) != 1 || got[0].Kind != Equal {
		t.Errorf("без учёта пробелов нашлись отличия: %+v", got)
	}
	if got := Lines(a, b, false); len(got) == 1 {
		t.Error("с учётом пробелов отличий не нашлось")
	}
}

func readDemo(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// Удалённый блок сдвинут к абзацу, а не стоит там, куда его поставил Майерс.
func TestLinesSlideToParagraph(t *testing.T) {
	l, r := readDemo(t, "demo_left.txt"), readDemo(t, "demo_right.txt")
	for _, c := range Lines(l, r, false) {
		if c.Kind == Delete {
			if c.LeftFrom+1 != 16 || c.LeftTo != 24 {
				t.Fatalf("удалённый блок не сдвинут к абзацу: строки %d..%d", c.LeftFrom+1, c.LeftTo)
			}
			return
		}
	}
	t.Fatal("нет удалённого блока")
}

func TestInlineRange(t *testing.T) {
	a0, a1, b0, b1, ok := InlineRange([]rune("Caption: caption"), []rune("Caption: LocString"))
	if !ok || a0 != 9 || a1 != 16 || b0 != 9 || b1 != 18 {
		t.Errorf("внутристрочная разница: %d..%d / %d..%d ok=%v", a0, a1, b0, b1, ok)
	}
	if _, _, _, _, ok := InlineRange([]rune("abc"), []rune("xyz")); ok {
		t.Error("у совсем разных строк нашлась общая часть")
	}
}

// Файл возвращается байт в байт: CRLF, BOM и перевод в конце переживают
// разбор и сборку. Иначе одна исправленная буква выглядела бы в контроле
// версий как переписанный файл.
func TestTextRoundTrip(t *testing.T) {
	for _, s := range []string{
		"one\r\ntwo\r\nthree\r\n",
		"\xEF\xBB\xBFone\ntwo!\nthree",
		"",
		"\n",
		"single",
	} {
		if got := Decode([]byte(s)).Encode(); !bytes.Equal(got, []byte(s)) {
			t.Errorf("%q вернулся как %q", s, got)
		}
	}
}

func TestReadFileRejectsBinary(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pic.png")
	os.WriteFile(p, []byte{0x89, 'P', 'N', 'G', 0, 0, 1}, 0o644)
	_, err := ReadFile(p)
	if _, ok := err.(*BinaryError); !ok {
		t.Errorf("двоичный файл прочитан как текст: %v", err)
	}
}

func TestDisplayAndRawCol(t *testing.T) {
	rs := []rune("\tab\tc")
	if got := DisplayCol(rs, 1); got != 4 {
		t.Errorf("после табуляции колонка %d", got)
	}
	if got := DisplayCol(rs, 4); got != 8 {
		t.Errorf("после второй табуляции колонка %d", got)
	}
	for col := 0; col <= len(rs); col++ {
		if back := RawCol(rs, DisplayCol(rs, col)); back != col {
			t.Errorf("колонка %d ушла в %d и вернулась в %d", col, DisplayCol(rs, col), back)
		}
	}
}

func TestTokenize(t *testing.T) {
	toks := Tokenize([]rune(`func f() { return "x" // note`))
	kinds := map[TokenKind]bool{}
	for _, tk := range toks {
		kinds[tk.Kind] = true
	}
	for _, k := range []TokenKind{TokenKeyword, TokenFunc, TokenString, TokenComment} {
		if !kinds[k] {
			t.Errorf("не размечено: %v", k)
		}
	}
}
