package widget

import (
	"image"
	"testing"
)

// В меню принято писать сочетание клавиш справа от названия — «Ctrl+S». Поля
// для него не было, и приложения дописывали его в сам текст пункта: тогда
// оно выравнивалось по левому краю вслед за названием и выглядело второй
// подписью, а не подсказкой.

func TestMenuItem_ShortcutWidensMenu(t *testing.T) {
	plain := NewPopupMenu()
	plain.SetItems([]MenuItem{{Text: "Сохранить"}})

	withKeys := NewPopupMenu()
	withKeys.SetItems([]MenuItem{{Text: "Сохранить", Shortcut: "Ctrl+Shift+S"}})

	// Меню считает размер само — сравниваем, что место под сочетание
	// отведено: иначе подпись и сочетание налезли бы друг на друга.
	plain.Show(0, 0)
	withKeys.Show(0, 0)
	if withKeys.Bounds().Dx() <= plain.Bounds().Dx() {
		t.Errorf("меню с сочетанием шириной %d, без — %d; ждал шире",
			withKeys.Bounds().Dx(), plain.Bounds().Dx())
	}
}

// Сочетание — только подсказка: меню его показывает, а обрабатывает
// приложение. Проверяем, что оно не стало частью текста пункта.
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

func TestMenuItem_ShortcutFromXAML(t *testing.T) {
	const src = `<Canvas Width="300" Height="200">
  <PopupMenu Name="file">
    <MenuItem Text="Сохранить" InputGestureText="Ctrl+S"/>
    <MenuItem Text="Открыть" Shortcut="Ctrl+O"/>
    <MenuItem Text="Без сочетания"/>
  </PopupMenu>
</Canvas>`
	root, reg, err := LoadUIFromXAML([]byte(src))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	defer ReleaseXAML(root)
	_ = image.Rect(0, 0, 0, 0)

	m, ok := reg["file"].(*PopupMenu)
	if !ok {
		t.Fatalf("file — %T, ждал *PopupMenu", reg["file"])
	}
	items := m.Items()
	if len(items) != 3 {
		t.Fatalf("пунктов %d, ждал 3", len(items))
	}
	if items[0].Shortcut != "Ctrl+S" {
		t.Errorf("InputGestureText не прочитан: %q", items[0].Shortcut)
	}
	if items[1].Shortcut != "Ctrl+O" {
		t.Errorf("Shortcut не прочитан: %q", items[1].Shortcut)
	}
	if items[2].Shortcut != "" {
		t.Errorf("у пункта без сочетания оказалось %q", items[2].Shortcut)
	}
}
