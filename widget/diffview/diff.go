// Package diffview — модель сравнения двух текстов: алгоритм различий, работа с
// текстом и файлами, грубая подсветка синтаксиса.
//
// Пакет не зависит от widget и ничего не рисует: сам контрол — widget.DiffView,
// а здесь то, что пригодится и без него, — например, приложению, которому
// нужны только различия строк (журнал коммитов, предпросмотр слияния).
package diffview

import "strings"

// ChangeKind — вид участка сравнения.
type ChangeKind int

const (
	// Equal — строки совпадают.
	Equal ChangeKind = iota
	// Replace — строки слева заменены строками справа.
	Replace
	// Delete — строки есть только слева.
	Delete
	// Insert — строки есть только справа.
	Insert
)

func (k ChangeKind) String() string {
	switch k {
	case Equal:
		return "equal"
	case Replace:
		return "replace"
	case Delete:
		return "delete"
	case Insert:
		return "insert"
	}
	return "unknown"
}

// Change — участок сравнения: строки [LeftFrom, LeftTo) слева против
// [RightFrom, RightTo) справа. Номера строк — от нуля, конец не включён.
type Change struct {
	Kind               ChangeKind
	LeftFrom, LeftTo   int
	RightFrom, RightTo int
}

type op byte

const (
	opEqual op = iota
	opDelete
	opInsert
)

// MaxEditDistance — предел числа правок для алгоритма Майерса. Память растёт
// как D², и сравнение двух совершенно разных больших файлов без предела
// съело бы её всю; сверх предела файлы считаются заменёнными целиком.
const MaxEditDistance = 2000

// normWS — строки для сравнения без учёта пробелов: пробелы схлопнуты,
// края обрезаны.
func normWS(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.Join(strings.Fields(l), " ")
	}
	return out
}

// Lines сравнивает два набора строк и возвращает участки, покрывающие оба
// набора целиком и подряд: совпадения чередуются с отличиями, соседних
// совпадений не бывает.
//
// Сравнение — Майерс O(ND) с отсечением общего начала и конца. Результат
// минимален, но минимальный не значит читаемый: вставка внутри повторяющихся
// строк может встать куда угодно, и после алгоритма блоки сдвигаются к
// границам абзацев — так же поступает git. ignoreWS сравнивает строки без
// учёта пробелов; сами строки при этом не меняются.
func Lines(a, b []string, ignoreWS bool) []Change {
	cmpA, cmpB := a, b
	if ignoreWS {
		cmpA, cmpB = normWS(a), normWS(b)
	}
	ids := make(map[string]int, len(a)+len(b))
	intern := func(s string) int {
		if id, ok := ids[s]; ok {
			return id
		}
		id := len(ids)
		ids[s] = id
		return id
	}
	ai := make([]int, len(cmpA))
	for i, s := range cmpA {
		ai[i] = intern(s)
	}
	bi := make([]int, len(cmpB))
	for i, s := range cmpB {
		bi[i] = intern(s)
	}

	pre := 0
	for pre < len(ai) && pre < len(bi) && ai[pre] == bi[pre] {
		pre++
	}
	suf := 0
	for suf < len(ai)-pre && suf < len(bi)-pre && ai[len(ai)-1-suf] == bi[len(bi)-1-suf] {
		suf++
	}
	ops := myers(ai[pre:len(ai)-suf], bi[pre:len(bi)-suf])

	var out []Change
	add := func(c Change) {
		if c.LeftFrom == c.LeftTo && c.RightFrom == c.RightTo {
			return
		}
		if n := len(out); n > 0 && c.Kind == Equal && out[n-1].Kind == Equal {
			out[n-1].LeftTo, out[n-1].RightTo = c.LeftTo, c.RightTo
			return
		}
		out = append(out, c)
	}
	add(Change{Equal, 0, pre, 0, pre})
	x, y := pre, pre
	for i := 0; i < len(ops); {
		if ops[i] == opEqual {
			j := i
			for j < len(ops) && ops[j] == opEqual {
				j++
			}
			n := j - i
			add(Change{Equal, x, x + n, y, y + n})
			x, y, i = x+n, y+n, j
			continue
		}
		x0, y0 := x, y
		for i < len(ops) && ops[i] != opEqual {
			if ops[i] == opDelete {
				x++
			} else {
				y++
			}
			i++
		}
		kind := Replace
		switch {
		case y == y0:
			kind = Delete
		case x == x0:
			kind = Insert
		}
		add(Change{kind, x0, x, y0, y})
	}
	add(Change{Equal, x, x + suf, y, y + suf})
	slideChanges(out, cmpA, cmpB)
	return out
}

// IsBlank сообщает, что строка пуста или из одних пробелов.
func IsBlank(s string) bool { return strings.TrimSpace(s) == "" }

// slideChanges двигает чистые вставки и удаления внутри повторов так, чтобы
// блок кончался перед пустой строкой: Майерс минимален, но не читаем.
func slideChanges(chunks []Change, a, b []string) {
	for i, c := range chunks {
		if c.Kind != Delete && c.Kind != Insert {
			continue
		}
		lines, s, e := a, c.LeftFrom, c.LeftTo
		if c.Kind == Insert {
			lines, s, e = b, c.RightFrom, c.RightTo
		}
		prevLen, nextLen := 0, 0
		if i > 0 && chunks[i-1].Kind == Equal {
			prevLen = chunks[i-1].LeftTo - chunks[i-1].LeftFrom
		}
		if i+1 < len(chunks) && chunks[i+1].Kind == Equal {
			nextLen = chunks[i+1].LeftTo - chunks[i+1].LeftFrom
		}
		up := 0
		for up < prevLen-1 && lines[s-up-1] == lines[e-up-1] {
			up++
		}
		down := 0
		for down < nextLen-1 && lines[s+down] == lines[e+down] {
			down++
		}
		score := func(off int) int {
			ns, ne := s+off, e+off
			sc := 0
			if ne >= len(lines) || IsBlank(lines[ne]) {
				sc += 2
			}
			if ns == 0 || IsBlank(lines[ns-1]) {
				sc++
			}
			return sc
		}
		best, bestSc := 0, score(0)
		for off := -up; off <= down; off++ {
			if sc := score(off); sc > bestSc {
				best, bestSc = off, sc
			}
		}
		if best == 0 {
			continue
		}
		chunks[i].LeftFrom += best
		chunks[i].LeftTo += best
		chunks[i].RightFrom += best
		chunks[i].RightTo += best
		if i > 0 {
			chunks[i-1].LeftTo += best
			chunks[i-1].RightTo += best
		}
		if i+1 < len(chunks) {
			chunks[i+1].LeftFrom += best
			chunks[i+1].RightFrom += best
		}
	}
}

// myers — классический O(ND) с сохранением фронта на каждом шаге.
func myers(a, b []int) []op {
	n, m := len(a), len(b)
	ops := make([]op, 0, n+m)
	whole := func() []op {
		for i := 0; i < n; i++ {
			ops = append(ops, opDelete)
		}
		for i := 0; i < m; i++ {
			ops = append(ops, opInsert)
		}
		return ops
	}
	if n == 0 || m == 0 {
		return whole()
	}
	maxD := n + m
	limit := min(maxD, MaxEditDistance)
	off := maxD + 1
	v := make([]int, 2*maxD+3)
	var trace [][]int
	found := -1
	for d := 0; d <= limit && found < 0; d++ {
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1]
			} else {
				x = v[off+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[off+k] = x
			if x >= n && y >= m {
				found = d
				break
			}
		}
		snap := make([]int, 2*d+1)
		copy(snap, v[off-d:off+d+1])
		trace = append(trace, snap)
	}
	if found < 0 {
		return whole()
	}

	x, y := n, m
	for d := found; d > 0; d-- {
		prev := trace[d-1]
		get := func(k int) int { return prev[k+d-1] }
		k := x - y
		pk := k - 1
		if k == -d || (k != d && get(k-1) < get(k+1)) {
			pk = k + 1
		}
		px := get(pk)
		py := px - pk
		for x > px && y > py {
			ops = append(ops, opEqual)
			x--
			y--
		}
		if pk == k+1 {
			ops = append(ops, opInsert)
		} else {
			ops = append(ops, opDelete)
		}
		x, y = px, py
	}
	for x > 0 && y > 0 {
		ops = append(ops, opEqual)
		x--
		y--
	}
	for i, j := 0, len(ops)-1; i < j; i, j = i+1, j-1 {
		ops[i], ops[j] = ops[j], ops[i]
	}
	return ops
}

// InlineRange — отличающийся средний участок пары строк (в рунах): общее
// начало и общий конец отсекаются, остаток и есть внутристрочная разница.
// ok=false — у строк нет ни общего начала, ни общего конца.
func InlineRange(a, b []rune) (a0, a1, b0, b1 int, ok bool) {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	if p+s == 0 {
		return 0, 0, 0, 0, false
	}
	return p, len(a) - s, p, len(b) - s, true
}
