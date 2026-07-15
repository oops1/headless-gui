package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// trayIconSVG — минимальная валидная иконка для проверки загрузки TrayIcon.
const trayIconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">` +
	`<rect x="2" y="2" width="20" height="20" fill="#0078D7"/></svg>`

// trayXAML — <Window> с декларативным треем: иконка, подсказка и меню.
const trayXAML = `<Window Title="Моё приложение" Width="640" Height="480"
        TrayIcon="icons/app.svg" TrayTooltip="Подсказка трея">
  <TrayMenu Name="trayMenu">
    <MenuItem Text="Показать"/>
    <Separator/>
    <MenuItem Text="Выход"/>
  </TrayMenu>
  <Grid/>
</Window>`

// TestXAMLTray_Declaration проверяет, что декларации трея из XAML попадают в
// поля widget.Window (иконка загружена, подсказка задана, меню разобрано и НЕ
// стало ребёнком дерева окна).
func TestXAMLTray_Declaration(t *testing.T) {
	dir := t.TempDir()
	iconsDir := filepath.Join(dir, "icons")
	if err := os.MkdirAll(iconsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(iconsDir, "app.svg"), []byte(trayIconSVG), 0o644); err != nil {
		t.Fatalf("write svg: %v", err)
	}

	root, reg, err := widget.LoadUIFromXAMLWithBase([]byte(trayXAML), dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	win, ok := root.(*widget.Window)
	if !ok {
		t.Fatalf("корень не *widget.Window: %T", root)
	}

	// Иконка загружена и растеризована 32×32.
	if win.TrayIconImage == nil {
		t.Fatal("TrayIconImage == nil (иконка не загружена)")
	}
	if b := win.TrayIconImage.Bounds(); b.Dx() != 32 || b.Dy() != 32 {
		t.Errorf("размер иконки трея = %dx%d, ожидалось 32x32", b.Dx(), b.Dy())
	}

	// Подсказка из атрибута.
	if win.TrayTooltip != "Подсказка трея" {
		t.Errorf("TrayTooltip = %q, ожидалось «Подсказка трея»", win.TrayTooltip)
	}

	// Меню разобрано: «Показать», разделитель, «Выход».
	if win.TrayMenu == nil {
		t.Fatal("TrayMenu == nil (меню не разобрано)")
	}
	items := win.TrayMenu.Items()
	if len(items) != 3 {
		t.Fatalf("пунктов трей-меню = %d, ожидалось 3: %+v", len(items), items)
	}
	if items[0].Text != "Показать" || items[0].Separator {
		t.Errorf("пункт 0 = %+v, ожидался текст «Показать»", items[0])
	}
	if !items[1].Separator {
		t.Errorf("пункт 1 = %+v, ожидался разделитель", items[1])
	}
	if items[2].Text != "Выход" || items[2].Separator {
		t.Errorf("пункт 2 = %+v, ожидался текст «Выход»", items[2])
	}

	// Меню зарегистрировано по Name — код сможет найти его для OnSelect.
	if reg["trayMenu"] != widget.Widget(win.TrayMenu) {
		t.Error("TrayMenu не зарегистрировано в reg под Name=\"trayMenu\"")
	}

	// TrayMenu — ПОЛЕ, а не ребёнок дерева окна (PopupMenu прямым ребёнком Window
	// опасен; window.attachTrayMenu добавит его позже правильно).
	for _, c := range win.Children() {
		if _, isPopup := c.(*widget.PopupMenu); isPopup {
			t.Error("PopupMenu оказался прямым ребёнком Window (должен быть только полем)")
		}
	}
}

// TestXAMLTray_TooltipDefaultsToTitle проверяет, что без TrayTooltip подсказка
// по умолчанию равна Title окна.
func TestXAMLTray_TooltipDefaultsToTitle(t *testing.T) {
	const xaml = `<Window Title="Заголовок" Width="320" Height="240"><Grid/></Window>`
	root, _, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	win := root.(*widget.Window)
	if win.TrayTooltip != "Заголовок" {
		t.Errorf("TrayTooltip = %q, ожидалось = Title «Заголовок»", win.TrayTooltip)
	}
	// Без TrayIcon иконки нет.
	if win.TrayIconImage != nil {
		t.Error("TrayIconImage != nil без атрибута TrayIcon")
	}
	if win.TrayMenu != nil {
		t.Error("TrayMenu != nil без тега <TrayMenu>")
	}
}

// TestXAMLTray_BadIconIsSkipped проверяет, что несуществующая иконка не роняет
// парсинг (log.Printf + пропуск), а поле остаётся nil.
func TestXAMLTray_BadIconIsSkipped(t *testing.T) {
	const xaml = `<Window Title="X" Width="320" Height="240" TrayIcon="nope/missing.png"><Grid/></Window>`
	root, _, err := widget.LoadUIFromXAMLWithBase([]byte(xaml), t.TempDir())
	if err != nil {
		t.Fatalf("load не должен падать при плохой иконке: %v", err)
	}
	win := root.(*widget.Window)
	if win.TrayIconImage != nil {
		t.Error("TrayIconImage должен быть nil при ошибке загрузки")
	}
}

// TestXAMLDockManager_NativeFloating проверяет, что атрибут NativeFloating="True"
// выставляет поле DockManager.NativeFloating (и его отсутствие — false).
func TestXAMLDockManager_NativeFloating(t *testing.T) {
	const xamlOn = `<Canvas Width="400" Height="300">
	  <DockManager Name="dm" Width="400" Height="300" NativeFloating="True">
	    <DockPane Id="tools" Title="Инструменты" Side="Left" Size="120"/>
	    <DockContent><TextBox Text="doc"/></DockContent>
	  </DockManager>
	</Canvas>`
	_, reg, err := widget.LoadUIFromXAML([]byte(xamlOn))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	dm, ok := reg["dm"].(*widget.DockManager)
	if !ok {
		t.Fatalf("dm не *widget.DockManager: %T", reg["dm"])
	}
	if !dm.NativeFloating {
		t.Error("NativeFloating == false, ожидалось true (атрибут NativeFloating=\"True\")")
	}

	const xamlOff = `<Canvas Width="400" Height="300">
	  <DockManager Name="dm2" Width="400" Height="300">
	    <DockContent><TextBox Text="doc"/></DockContent>
	  </DockManager>
	</Canvas>`
	_, reg2, err := widget.LoadUIFromXAML([]byte(xamlOff))
	if err != nil {
		t.Fatalf("load off: %v", err)
	}
	dm2 := reg2["dm2"].(*widget.DockManager)
	if dm2.NativeFloating {
		t.Error("NativeFloating == true без атрибута, ожидалось false")
	}
}
