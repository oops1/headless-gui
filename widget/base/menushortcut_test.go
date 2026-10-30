package base

import (
	"testing"
)

// В меню принято писать сочетание клавиш справа от названия — «Ctrl+S». Поля
// для него не было, и приложения дописывали его в сам текст пункта: тогда
// оно выравнивалось по левому краю вслед за названием и выглядело второй
// подписью, а не подсказкой.

func TestMenuItem_ShortcutWidensMenu(t *testing.T) {
	// Меню считает размер само — сравниваем, что место под сочетание
	// отведено: иначе подпись и сочетание налезли бы друг на друга.
	plain := shownMenuWidth(t, MenuItem{Text: "Сохранить"}, false)
	withKeys := shownMenuWidth(t, MenuItem{Text: "Сохранить", Shortcut: "Ctrl+Shift+S"}, false)
	if withKeys <= plain {
		t.Errorf("меню с сочетанием шириной %d, без — %d; ждал шире", withKeys, plain)
	}
}

// Сочетание — только подсказка: меню его показывает, а обрабатывает
// приложение. Проверяем, что оно не стало частью текста пункта.
// shownMenuWidth — ширина раскрытого меню с единственным пунктом.
//
// Открытым бывает только одно корневое меню сразу: второй Show гасит первое,
// и у погашенного ширины уже не спросить — поэтому замер по одному.
func shownMenuWidth(t *testing.T, item MenuItem, mnemonics bool) int {
	t.Helper()
	m := NewPopupMenu()
	m.UseMnemonics = mnemonics
	m.SetItems([]MenuItem{item})
	m.Show(0, 0)
	defer m.Close()
	w := m.Bounds().Dx()
	if w == 0 {
		t.Fatal("меню не раскрылось")
	}
	return w
}

func TestMenuItem_ShortcutIsSeparateFromText(t *testing.T) {
	m := NewPopupMenu()
	m.SetItems([]MenuItem{{Text: "Открыть", Shortcut: "Ctrl+O"}})

	items := m.Items()
	if len(items) != 1 {
		t.Fatalf("пунктов %d", len(items))
	}
	if items[0].Text != "Открыть" {
		t.Errorf("текст пункта %q — сочетание попало в подпись", items[0].Text)
	}
	if items[0].Shortcut != "Ctrl+O" {
		t.Errorf("сочетание %q", items[0].Shortcut)
	}
}
