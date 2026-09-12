// Package mergeview — модель трёхстороннего слияния: сведение базы, «нашего» и
// «их» в блоки и сборка итога с маркерами конфликта.
//
// Пакет не зависит от widget и ничего не рисует: сам контрол —
// widget.MergeView, а здесь то, что пригодится и без него, — например,
// приложению, которому нужен только список конфликтов.
//
// Приложение, у которого слияние уже есть (git merge-file и его стили), отдаёт
// контролу готовые блоки: итог обязан совпадать с тем, что запишет git, а
// пересчёт своим алгоритмом дал бы другое разбиение. Merge здесь — для
// остальных: демонстраций и простых случаев.
package mergeview

import (
	"slices"

	"github.com/oops1/headless-gui/v3/widget/diffview"
)

// Chunk — блок слияния: участок, на котором стороны либо согласны, либо нет.
//
// Стороны хранятся как есть — это то, что показывают три панели контрола.
// Итог неконфликтного блока — Merged; если он nil, берётся Ours (так блок
// описывает приложение, которому достаточно сказать «вот строки»).
// У конфликта Merged всегда nil: что писать в итог, решает человек.
type Chunk struct {
	// Conflict — блок требует решения: обе стороны правили одно место
	// по-разному.
	Conflict bool
	// Ours, Base, Theirs — строки сторон на этом участке. У неконфликтного
	// блока стороны могут различаться (правку внесла одна из них) — панели
	// показывают то, что в стороне есть.
	Ours, Base, Theirs []string
	// Merged — итог неконфликтного блока. nil означает «как в Ours».
	Merged []string
}

// Resolution — чем закрыт конфликтный блок.
type Resolution int

const (
	// Unresolved — решения нет: в итог уйдут маркеры конфликта.
	Unresolved Resolution = iota
	// TakeOurs — наша сторона.
	TakeOurs
	// TakeTheirs — их сторона.
	TakeTheirs
	// TakeBase — база: правки обеих сторон отброшены.
	TakeBase
	// TakeOursThenTheirs — обе стороны, наша первой.
	TakeOursThenTheirs
	// TakeTheirsThenOurs — обе стороны, их первой.
	TakeTheirsThenOurs
)

// Resolved сообщает, закрыт ли блок.
func (r Resolution) Resolved() bool { return r != Unresolved }

// Lines возвращает строки, которые уходят в итог по этому решению.
// Unresolved — nil: такой блок пишется маркерами, а не строками.
func (r Resolution) Lines(c Chunk) []string {
	switch r {
	case TakeOurs:
		return c.Ours
	case TakeTheirs:
		return c.Theirs
	case TakeBase:
		return c.Base
	case TakeOursThenTheirs:
		return concat(c.Ours, c.Theirs)
	case TakeTheirsThenOurs:
		return concat(c.Theirs, c.Ours)
	}
	return nil
}

// Result возвращает строки неконфликтного блока.
func (c Chunk) Result() []string {
	if c.Conflict {
		return nil
	}
	if c.Merged != nil {
		return c.Merged
	}
	return c.Ours
}

// Merge сводит базу, «наше» и «их» в блоки.
//
// Обе стороны сравниваются с базой, затем ищутся места, где ОБЕ идут с базой
// вровень, — они и разделяют блоки. Участок между такими местами достаётся
// одной стороне, если вторая его не трогала; если правили обе и вышло разное —
// это конфликт. Одинаковая правка с двух сторон конфликтом не считается.
//
// ignoreWS сравнивает строки без учёта пробелов — как у diffview.Lines; сами
// строки при этом не меняются.
//
// Соседние конфликты, разделённые парой общих строк, НЕ склеиваются: git это
// делает, и приложение, которому важно совпадение с ним байт в байт, отдаёт
// свои блоки через SetChunks, а не считает их здесь.
func Merge(base, ours, theirs []string, ignoreWS bool) []Chunk {
	oc := diffview.Lines(base, ours, ignoreWS)
	tc := diffview.Lines(base, theirs, ignoreWS)

	// Точки синхронизации — участки базы, идущие вровень с ОБЕИМИ сторонами.
	syncs := syncRegions(equalRuns(oc), equalRuns(tc))

	var out []Chunk
	b, o, t := 0, 0, 0
	add := func(c Chunk) {
		if len(c.Ours) == 0 && len(c.Base) == 0 && len(c.Theirs) == 0 && len(c.Merged) == 0 {
			return // пустой участок между двумя соседними точками
		}
		out = append(out, c)
	}
	for _, s := range syncs {
		// Между прошлой точкой и этой стороны шли вразнобой.
		add(unstable(base[b:s.base], ours[o:s.ours], theirs[t:s.theirs]))
		// Сама точка: все три согласны.
		lines := slices.Clone(base[s.base : s.base+s.n])
		add(Chunk{Ours: lines, Base: lines, Theirs: lines})
		b, o, t = s.base+s.n, s.ours+s.n, s.theirs+s.n
	}
	add(unstable(base[b:], ours[o:], theirs[t:]))
	return out
}

// unstable классифицирует участок, на котором стороны разошлись с базой.
func unstable(base, ours, theirs []string) Chunk {
	c := Chunk{
		Ours:   slices.Clone(ours),
		Base:   slices.Clone(base),
		Theirs: slices.Clone(theirs),
	}
	switch {
	case slices.Equal(ours, base):
		c.Merged = c.Theirs // правили только они
	case slices.Equal(theirs, base):
		c.Merged = c.Ours // правили только мы
	case slices.Equal(ours, theirs):
		c.Merged = c.Ours // одна и та же правка с двух сторон — не конфликт
	default:
		c.Conflict = true
	}
	return c
}

// run — участок, на котором сторона идёт с базой вровень: строки
// [base, base+n) базы совпадают со [side, side+n) стороны.
type run struct{ base, side, n int }

// equalRuns выбирает из участков сравнения совпадения. Левая сторона
// сравнения — всегда база.
func equalRuns(cs []diffview.Change) []run {
	var out []run
	for _, c := range cs {
		if c.Kind == diffview.Equal {
			out = append(out, run{base: c.LeftFrom, side: c.RightFrom, n: c.LeftTo - c.LeftFrom})
		}
	}
	return out
}

// sync — участок, идущий вровень во всех трёх текстах.
type sync struct{ base, ours, theirs, n int }

// syncRegions пересекает совпадения двух сторон по координатам базы: общий
// участок — тот, который обе стороны прошли вровень с базой.
func syncRegions(o, t []run) []sync {
	var out []sync
	i, j := 0, 0
	for i < len(o) && j < len(t) {
		a, b := o[i], t[j]
		from := max(a.base, b.base)
		to := min(a.base+a.n, b.base+b.n)
		if from < to {
			out = append(out, sync{
				base:   from,
				ours:   a.side + (from - a.base),
				theirs: b.side + (from - b.base),
				n:      to - from,
			})
		}
		// Дальше идёт тот, чей участок кончился раньше.
		if a.base+a.n <= b.base+b.n {
			i++
		} else {
			j++
		}
	}
	return out
}

func concat(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	out = append(out, a...)
	return append(out, b...)
}
