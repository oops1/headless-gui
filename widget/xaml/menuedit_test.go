package xaml_test

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget/base"
	"github.com/oops1/headless-gui/v3/widget/chrome"
	"github.com/oops1/headless-gui/v3/widget/xaml"
)

func TestXAML_MenuItemChecked(t *testing.T) {
	const src = `<Window Width="400" Height="300">
  <Menu Name="bar">
    <MenuItem Header="Вид">
      <MenuItem Header="Тёмная" IsChecked="True" GroupName="theme"/>
      <MenuItem Header="Светлая" GroupName="theme"/>
      <MenuItem Header="Панель" IsCheckable="True"/>
      <MenuItem Header="Обычный"/>
    </MenuItem>
  </Menu>
</Window>`

	_, reg, err := xaml.LoadUIFromXAML([]byte(src))
	if err != nil {
		t.Fatalf("разбор XAML: %v", err)
	}
	mb, ok := reg["bar"].(*base.MenuBar)
	if !ok {
		t.Fatalf("в разметке нет полосы меню: %T", reg["bar"])
	}

	dark, ok := mb.ItemAt(0, 0)
	if !ok {
		t.Fatal("пункт «Тёмная» не найден")
	}
	if !dark.Checked || !dark.Checkable || dark.RadioGroup != "theme" {
		t.Errorf("IsChecked/GroupName не дошли из разметки: %+v", dark)
	}

	panel, _ := mb.ItemAt(0, 2)
	if !panel.Checkable || panel.Checked {
		t.Errorf("IsCheckable без IsChecked дал %+v", panel)
	}

	plain, _ := mb.ItemAt(0, 3)
	if plain.Checkable || plain.Checked {
		t.Errorf("обычный пункт объявлен отмечаемым: %+v", plain)
	}
}

// Горячая клавиша из разметки доходит до привязки — ради этого запрос и подан.
func TestXAML_KeyBindingUnderstandsFunctionKeys(t *testing.T) {
	const src = `<Window Width="400" Height="300">
  <Window.InputBindings>
    <KeyBinding Key="F5" Command="Refresh"/>
    <KeyBinding Key="F12" Modifiers="Ctrl" Command="Tools"/>
  </Window.InputBindings>
  <StackPanel/>
</Window>`

	root, _, err := xaml.LoadUIFromXAML([]byte(src))
	if err != nil {
		t.Fatalf("разбор XAML: %v", err)
	}
	win, ok := root.(*chrome.Window)
	if !ok {
		t.Fatalf("корень разметки — %T, ждали окно", root)
	}
	if len(win.InputBindings) != 2 {
		t.Fatalf("разобрано %d привязок, ждали две", len(win.InputBindings))
	}
	if got := win.InputBindings[0].Key; got != base.KeyF5 {
		t.Errorf("Key=\"F5\" разобран как %d, ждали %d", got, base.KeyF5)
	}
	if got := win.InputBindings[1].Key; got != base.KeyF12 {
		t.Errorf("Key=\"F12\" разобран как %d, ждали %d", got, base.KeyF12)
	}
	if win.InputBindings[1].Mods&base.ModCtrl == 0 {
		t.Error("модификатор Ctrl потерян")
	}
}
