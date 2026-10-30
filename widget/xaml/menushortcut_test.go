package xaml

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
)

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

	m, ok := reg["file"].(*base.PopupMenu)
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
