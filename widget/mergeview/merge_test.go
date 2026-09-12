package mergeview

import (
	"math/rand"
	"slices"
	"strings"
	"testing"
)

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// ─── Разбор случаев ─────────────────────────────────────────────────────────

func TestMergeOneSideChanged(t *testing.T) {
	base := lines("a\nb\nc")
	ours := lines("a\nB\nc")
	theirs := slices.Clone(base)

	cs := Merge(base, ours, theirs, false)
	if n := Conflicts(cs); n != 0 {
		t.Fatalf("конфликтов %d, а правила одна сторона: %+v", n, cs)
	}
	if got := Result(cs, nil, StyleMerge, Labels{}); !slices.Equal(got, ours) {
		t.Fatalf("итог %q, хочу нашу правку %q", got, ours)
	}

	// Симметрично: правят они.
	cs = Merge(base, base, ours, false)
	if got := Result(cs, nil, StyleMerge, Labels{}); !slices.Equal(got, ours) {
		t.Fatalf("итог %q, хочу их правку %q", got, ours)
	}
}

func TestMergeSameChangeBothSides(t *testing.T) {
	base := lines("a\nb\nc")
	both := lines("a\nB\nc")

	cs := Merge(base, both, both, false)
	if n := Conflicts(cs); n != 0 {
		t.Fatalf("одинаковая правка с двух сторон — не конфликт, получено %d", n)
	}
	if got := Result(cs, nil, StyleMerge, Labels{}); !slices.Equal(got, both) {
		t.Fatalf("итог %q, хочу %q", got, both)
	}
}

func TestMergeConflictAndResolutions(t *testing.T) {
	base := lines("a\nb\nc")
	ours := lines("a\nНАШЕ\nc")
	theirs := lines("a\nИХ\nc")

	cs := Merge(base, ours, theirs, false)
	if n := Conflicts(cs); n != 1 {
		t.Fatalf("конфликтов %d, хочу 1: %+v", n, cs)
	}
	ci := slices.IndexFunc(cs, func(c Chunk) bool { return c.Conflict })
	c := cs[ci]
	if !slices.Equal(c.Ours, lines("НАШЕ")) || !slices.Equal(c.Theirs, lines("ИХ")) || !slices.Equal(c.Base, lines("b")) {
		t.Fatalf("стороны конфликта разошлись: %+v", c)
	}

	res := make([]Resolution, len(cs))
	if n := UnresolvedCount(cs, res); n != 1 {
		t.Fatalf("нерешённых %d, хочу 1", n)
	}

	want := map[Resolution]string{
		TakeOurs:           "a\nНАШЕ\nc",
		TakeTheirs:         "a\nИХ\nc",
		TakeBase:           "a\nb\nc",
		TakeOursThenTheirs: "a\nНАШЕ\nИХ\nc",
		TakeTheirsThenOurs: "a\nИХ\nНАШЕ\nc",
	}
	for r, exp := range want {
		res[ci] = r
		got := Result(cs, res, StyleMerge, Labels{})
		if !slices.Equal(got, lines(exp)) {
			t.Errorf("решение %d: итог %q, хочу %q", r, got, lines(exp))
		}
		if n := UnresolvedCount(cs, res); n != 0 {
			t.Errorf("решение %d: нерешённых %d, хочу 0", r, n)
		}
	}
}

func TestResultMarkers(t *testing.T) {
	base := lines("a\nb\nc")
	ours := lines("a\nНАШЕ\nc")
	theirs := lines("a\nИХ\nc")
	cs := Merge(base, ours, theirs, false)

	got := strings.Join(Result(cs, nil, StyleMerge, Labels{Ours: "main", Theirs: "feature"}), "\n")
	want := "a\n<<<<<<< main\nНАШЕ\n=======\nИХ\n>>>>>>> feature\nc"
	if got != want {
		t.Fatalf("маркеры merge:\n%s\nхочу:\n%s", got, want)
	}

	got = strings.Join(Result(cs, nil, StyleDiff3, Labels{}), "\n")
	want = "a\n<<<<<<< HEAD\nНАШЕ\n||||||| base\nb\n=======\nИХ\n>>>>>>> merge head\nc"
	if got != want {
		t.Fatalf("маркеры diff3:\n%s\nхочу:\n%s", got, want)
	}
}

func TestResultTextEOL(t *testing.T) {
	cs := Merge(lines("a"), lines("a"), lines("a"), false)
	if got := ResultText(cs, nil, StyleMerge, Labels{}, "\r\n"); got != "a\r\n" {
		t.Fatalf("итог %q, хочу \"a\\r\\n\"", got)
	}
	if got := ResultText(nil, nil, StyleMerge, Labels{}, "\n"); got != "" {
		t.Fatalf("пустое слияние дало %q", got)
	}
}

func TestMergeInsertsAtEdges(t *testing.T) {
	base := lines("b")
	ours := lines("сверху\nb")
	theirs := lines("b\nснизу")

	cs := Merge(base, ours, theirs, false)
	if n := Conflicts(cs); n != 0 {
		t.Fatalf("вставки в разные концы конфликтовать не должны: %+v", cs)
	}
	if got := Result(cs, nil, StyleMerge, Labels{}); !slices.Equal(got, lines("сверху\nb\nснизу")) {
		t.Fatalf("итог %q", got)
	}
}

func TestMergeEmptySides(t *testing.T) {
	base := lines("a\nb")
	// Одна сторона вычистила файл, вторая не трогала — берётся пустая.
	cs := Merge(base, nil, base, false)
	if n := Conflicts(cs); n != 0 {
		t.Fatalf("конфликтов %d: %+v", n, cs)
	}
	if got := Result(cs, nil, StyleMerge, Labels{}); len(got) != 0 {
		t.Fatalf("итог %q, хочу пусто", got)
	}
}

// ─── Property ───────────────────────────────────────────────────────────────

// Блоки покрывают все три текста подряд и без разрывов; сторона, не правившая
// файл, из блоков собирается обратно; при отсутствии конфликтов итог — это
// правка второй стороны.
func TestMergeCoversAllThree(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	words := []string{"a", "b", "c", "d", "", "e"}
	gen := func(n int) []string {
		out := make([]string, rng.Intn(n))
		for i := range out {
			out[i] = words[rng.Intn(len(words))]
		}
		return out
	}
	// edit — случайная правка текста: так стороны остаются похожими на базу,
	// а не случайными наборами строк.
	edit := func(src []string) []string {
		out := slices.Clone(src)
		for k := rng.Intn(3); k > 0; k-- {
			if len(out) == 0 {
				out = append(out, words[rng.Intn(len(words))])
				continue
			}
			i := rng.Intn(len(out))
			switch rng.Intn(3) {
			case 0:
				out[i] = words[rng.Intn(len(words))]
			case 1:
				out = slices.Delete(out, i, i+1)
			default:
				out = slices.Insert(out, i, words[rng.Intn(len(words))])
			}
		}
		return out
	}

	for iter := 0; iter < 3000; iter++ {
		base := gen(20)
		ours, theirs := edit(base), edit(base)
		cs := Merge(base, ours, theirs, false)

		var gb, go_, gt []string
		for _, c := range cs {
			gb = append(gb, c.Base...)
			go_ = append(go_, c.Ours...)
			gt = append(gt, c.Theirs...)
		}
		if !slices.Equal(gb, base) || !slices.Equal(go_, ours) || !slices.Equal(gt, theirs) {
			t.Fatalf("блоки не покрыли стороны:\nbase=%q → %q\nours=%q → %q\ntheirs=%q → %q\nблоки=%+v",
				base, gb, ours, go_, theirs, gt, cs)
		}

		// Если сторона равна базе, итог обязан быть второй стороной целиком.
		if slices.Equal(theirs, base) {
			if got := Result(cs, nil, StyleMerge, Labels{}); !slices.Equal(got, ours) {
				t.Fatalf("их не правили, а итог %q вместо %q", got, ours)
			}
		}
		if slices.Equal(ours, base) {
			if got := Result(cs, nil, StyleMerge, Labels{}); !slices.Equal(got, theirs) {
				t.Fatalf("мы не правили, а итог %q вместо %q", got, theirs)
			}
		}
	}
}
