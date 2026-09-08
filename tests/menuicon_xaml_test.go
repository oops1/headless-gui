package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Значок пункта меню из разметки — часть запроса GG-53.
//
// Значки Go.Git вкомпилированы, но разметке отставать незачем: у Button и
// TextInput путь в атрибуте уже читается, а у пункта меню — нет.
func TestMenuIcon_FromXAML(t *testing.T) {
	dir := t.TempDir()
	uiDir := filepath.Join(dir, "ui")
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTinyPNG(t, filepath.Join(uiDir, "copy.png"))

	xamlPath := filepath.Join(uiDir, "menu.xaml")
	const xaml = `<Canvas Width="300" Height="200">
  <ContextMenu x:Name="menu">
    <MenuItem Text="Копировать" Icon="copy.png" IconSize="14"/>
    <MenuItem Text="Вставить"/>
  </ContextMenu>
</Canvas>`
	if err := os.WriteFile(xamlPath, []byte(xaml), 0o644); err != nil {
		t.Fatal(err)
	}

	_, reg, err := widget.LoadUIFromXAMLFile(xamlPath)
	if err != nil {
		t.Fatalf("LoadUIFromXAMLFile: %v", err)
	}
	menu, ok := reg["menu"].(*widget.PopupMenu)
	if !ok {
		t.Fatalf("menu собрался как %T", reg["menu"])
	}

	items := menu.Items()
	if len(items) != 2 {
		t.Fatalf("пунктов %d, ожидалось 2", len(items))
	}
	if items[0].Icon == nil {
		t.Error("значок пункта не загружен из разметки")
	}
	if items[0].IconSize != 14 {
		t.Errorf("размер значка %d, в разметке 14", items[0].IconSize)
	}
	if items[0].IconPath != "copy.png" {
		t.Errorf("путь значка %q", items[0].IconPath)
	}
	if items[1].Icon != nil {
		t.Error("у пункта без атрибута появился значок")
	}
}

// Недоступный значок не ломает загрузку разметки: меню без картинки понятнее,
// чем окно, не открывшееся из-за неё.
func TestMenuIcon_MissingIconIsSkipped(t *testing.T) {
	dir := t.TempDir()
	xamlPath := filepath.Join(dir, "menu.xaml")
	const xaml = `<Canvas Width="300" Height="200">
  <ContextMenu x:Name="menu">
    <MenuItem Text="Копировать" Icon="nope/missing.png"/>
  </ContextMenu>
</Canvas>`
	if err := os.WriteFile(xamlPath, []byte(xaml), 0o644); err != nil {
		t.Fatal(err)
	}

	_, reg, err := widget.LoadUIFromXAMLFile(xamlPath)
	if err != nil {
		t.Fatalf("разметка со значком-пустышкой не загрузилась: %v", err)
	}
	menu := reg["menu"].(*widget.PopupMenu)
	if items := menu.Items(); len(items) != 1 || items[0].Icon != nil {
		t.Errorf("пункт получил значок из несуществующего файла: %+v", items)
	}
}
