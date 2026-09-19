package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// GG-83: пустой итог держит одну служебную пустую строку — каретке нужно где-то
// стоять. Строки следующего решения вставлялись ПЕРЕД ней, и в тексте оставался
// лишний перевод строки в конце: файл, которого не было в индексе, записывался
// с пустой строкой на конце.

// Путь ошибки: решение пустой стороной схлопывает итог до служебной строки,
// следующее решение вставляет текст перед ней.
func TestMergeView_PadLineDroppedAfterEmptySide(t *testing.T) {
	m := widget.NewMergeView("", "")
	m.SetChunks([]widget.MergeChunk{{Conflict: true, Theirs: []string{"new", "file"}}})
	m.SetResultEOL("\n", false, true)

	m.Resolve(0, widget.MergeTakeOurs) // нашей стороны нет — итог пуст
	if got := m.Result(); got != "" {
		t.Fatalf("итог после пустой стороны %q, ждал пустой", got)
	}

	m.Resolve(0, widget.MergeTakeTheirs)
	if got, want := m.Result(), "new\nfile\n"; got != want {
		t.Fatalf("итог %q, ждал %q", got, want)
	}
	if got := m.ResultLines(); len(got) != 2 || got[0] != "new" || got[1] != "file" {
		t.Fatalf("строки итога %q, ждал [new file]", got)
	}
}

// Взять сторону целиком у нерешённого конфликта: маркеры заменяются текстом,
// лишней строки не появляется.
func TestMergeView_ResolveFromMarkers(t *testing.T) {
	m := widget.NewMergeView("", "")
	m.SetChunks([]widget.MergeChunk{{Conflict: true, Theirs: []string{"new", "file"}}})
	m.SetResultEOL("\n", false, true)

	m.Resolve(0, widget.MergeTakeTheirs)

	if got, want := m.Result(), "new\nfile\n"; got != want {
		t.Fatalf("итог %q, ждал %q", got, want)
	}
}

// Пустой итог остаётся пустым файлом, а не одиноким переводом строки.
func TestMergeView_EmptyResultStaysEmpty(t *testing.T) {
	m := widget.NewMergeView("", "")
	m.SetChunks(nil)
	m.SetResultEOL("\n", false, true)

	if got := m.Result(); got != "" {
		t.Fatalf("пустой итог %q, ждал пустую строку", got)
	}
}

// Возврат блока в нерешённое состояние возвращает маркеры и не теряет перевод
// строки в конце.
func TestMergeView_UnresolveKeepsFinalNewline(t *testing.T) {
	m := widget.NewMergeView("", "")
	m.SetChunks([]widget.MergeChunk{{Conflict: true, Theirs: []string{"new"}}})
	m.SetResultEOL("\n", false, true)

	m.Resolve(0, widget.MergeTakeOurs)
	m.Resolve(0, widget.MergeUnresolved)

	got := m.Result()
	if got == "" || got[len(got)-1] != '\n' {
		t.Fatalf("после возврата в нерешённое итог %q — нет перевода строки в конце", got)
	}
	if lines := m.ResultLines(); len(lines) == 0 || lines[len(lines)-1] == "" {
		t.Fatalf("маркеры конфликта не вернулись или осталась служебная строка: %q", lines)
	}
}

// Собственная пустая строка итога (она принадлежит блоку) остаётся на месте.
func TestMergeView_NonEmptyResultKeepsOwnBlankLine(t *testing.T) {
	m := widget.NewMergeView("", "")
	m.SetChunks([]widget.MergeChunk{{Ours: []string{"a", ""}, Base: []string{"a", ""}, Theirs: []string{"a", ""}}})
	m.SetResultEOL("\n", false, true)

	if got, want := m.Result(), "a\n\n"; got != want {
		t.Fatalf("итог %q, ждал %q — пустая строка принадлежит блоку", got, want)
	}
}
