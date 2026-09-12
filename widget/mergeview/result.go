package mergeview

import "strings"

// Style — как пишется нерешённый конфликт.
type Style int

const (
	// StyleMerge — как `git merge`: наше, «=======», их.
	StyleMerge Style = iota
	// StyleDiff3 — как `git merge --diff3`: между нашим и их добавляется база
	// за «|||||||».
	StyleDiff3
)

// Labels — подписи сторон в маркерах конфликта. Пустая подпись означает
// значение по умолчанию: git пишет туда имя ветки, а без него — HEAD.
type Labels struct {
	Ours, Base, Theirs string
}

// Маркеры конфликта — ровно семь знаков, как у git.
const (
	MarkerOurs   = "<<<<<<<"
	MarkerBase   = "|||||||"
	MarkerSplit  = "======="
	MarkerTheirs = ">>>>>>>"
)

// Result собирает итог: решённые блоки — строками выбранной стороны,
// нерешённые — маркерами конфликта.
//
// res — решения по блокам, по индексу; короткий срез (или nil) означает, что
// решений нет. Неконфликтные блоки решения не спрашивают.
func Result(chunks []Chunk, res []Resolution, st Style, lb Labels) []string {
	var out []string
	for i, c := range chunks {
		if !c.Conflict {
			out = append(out, c.Result()...)
			continue
		}
		var r Resolution
		if i < len(res) {
			r = res[i]
		}
		if r.Resolved() {
			out = append(out, r.Lines(c)...)
			continue
		}
		out = append(out, marker(MarkerOurs, lb.Ours, "HEAD"))
		out = append(out, c.Ours...)
		if st == StyleDiff3 {
			out = append(out, marker(MarkerBase, lb.Base, "base"))
			out = append(out, c.Base...)
		}
		out = append(out, MarkerSplit)
		out = append(out, c.Theirs...)
		out = append(out, marker(MarkerTheirs, lb.Theirs, "merge head"))
	}
	return out
}

// ResultText — то же, что Result, но одной строкой с переводами eol.
// Непустой итог заканчивается переводом строки: так файл пишет git.
func ResultText(chunks []Chunk, res []Resolution, st Style, lb Labels, eol string) string {
	lines := Result(chunks, res, st, lb)
	if len(lines) == 0 {
		return ""
	}
	if eol == "" {
		eol = "\n"
	}
	return strings.Join(lines, eol) + eol
}

// UnresolvedCount считает нерешённые конфликты. Имя с Count, потому что
// Unresolved — это решение «решения нет».
func UnresolvedCount(chunks []Chunk, res []Resolution) int {
	n := 0
	for i, c := range chunks {
		if !c.Conflict {
			continue
		}
		if i < len(res) && res[i].Resolved() {
			continue
		}
		n++
	}
	return n
}

// Conflicts считает конфликтные блоки — решённые тоже.
func Conflicts(chunks []Chunk) int {
	n := 0
	for _, c := range chunks {
		if c.Conflict {
			n++
		}
	}
	return n
}

func marker(mark, label, def string) string {
	if label == "" {
		label = def
	}
	return mark + " " + label
}
