// Тесты режима вкладок в заголовке окна (стиль Windows 11 Terminal):
// содержимое активной вкладки в клиентской области, переключение/закрытие,
// XAML-декларация TitleTabs + TabItem + TitleTabsMenu.
package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

func newTabPanel() *widget.Panel {
	p := widget.NewPanel(color.RGBA{R: 40, G: 40, B: 40, A: 255})
	p.ShowHeader = false
	return p
}

// TestWindowTitleTabs_ContentSwap — активная вкладка держит Content в
// клиентской области; переключение меняет ребёнка окна.
func TestWindowTitleTabs_ContentSwap(t *testing.T) {
	w := widget.NewWindow("term", 600, 400)
	a, b := newTabPanel(), newTabPanel()

	w.AddTitleTab("таб 1", a)
	w.AddTitleTab("таб 2", b)

	if got := w.ActiveTitleTab(); got != 0 {
		t.Fatalf("ActiveTitleTab = %d, ждали 0 (первая — активная)", got)
	}
	if a.Bounds() != w.ContentBounds() {
		t.Errorf("контент вкладки 0: %v, ждали %v", a.Bounds(), w.ContentBounds())
	}
	if hasChild(w, b) {
		t.Error("контент неактивной вкладки не должен быть ребёнком окна")
	}

	w.SetActiveTitleTab(1)
	if !hasChild(w, b) || hasChild(w, a) {
		t.Error("после переключения ребёнком должен быть контент вкладки 1")
	}
	if b.Bounds() != w.ContentBounds() {
		t.Errorf("контент вкладки 1: %v, ждали %v", b.Bounds(), w.ContentBounds())
	}

	// Ресайз окна растягивает активный контент (штатный Window.SetBounds).
	w.SetBounds(image.Rect(0, 0, 800, 500))
	if b.Bounds() != w.ContentBounds() {
		t.Errorf("после ресайза: %v, ждали %v", b.Bounds(), w.ContentBounds())
	}
}

// TestWindowTitleTabs_Close — закрытие активной вкладки активирует соседнюю,
// закрытие последней вызывает OnClose.
func TestWindowTitleTabs_Close(t *testing.T) {
	w := widget.NewWindow("term", 600, 400)
	a, b := newTabPanel(), newTabPanel()
	w.AddTitleTab("first", a)
	w.AddTitleTab("second", b)
	w.SetActiveTitleTab(1)

	closedIdx, closedHeader := -1, ""
	w.OnTitleTabClosed = func(idx int, header string) { closedIdx, closedHeader = idx, header }
	winClosed := false
	w.OnClose = func() { winClosed = true }

	w.CloseTitleTab(1)
	if closedIdx != 1 || closedHeader != "second" {
		t.Errorf("OnTitleTabClosed(%d,%q), ждали (1,second)", closedIdx, closedHeader)
	}
	if got := w.ActiveTitleTab(); got != 0 {
		t.Errorf("после закрытия активной: ActiveTitleTab = %d, ждали 0", got)
	}
	if !hasChild(w, a) || hasChild(w, b) {
		t.Error("ребёнком должен остаться контент вкладки 0")
	}
	if winClosed {
		t.Error("OnClose не должен вызываться, пока есть вкладки")
	}

	w.CloseTitleTab(0)
	if !winClosed {
		t.Error("закрытие последней вкладки должно вызвать OnClose")
	}
	if w.TitleTabCount() != 0 {
		t.Errorf("TitleTabCount = %d, ждали 0", w.TitleTabCount())
	}
}

// TestWindowTitleTabs_XAML — декларация из XAML: TitleTabs, TabItem,
// TitleTabsMenu; Title при вкладках не мешает, контент активной — ребёнок.
func TestWindowTitleTabs_XAML(t *testing.T) {
	const xaml = `<Window Title="app" Width="600" Height="400" TitleTabs="True">
		<TabItem Header="одна" ToolTip="первая вкладка">
			<Panel Name="p1" Background="#202020"/>
		</TabItem>
		<TabItem Header="две">
			<Panel Name="p2" Background="#303030"/>
		</TabItem>
		<TitleTabsMenu>
			<MenuItem Header="Параметры"/>
			<MenuItem Header="О программе"/>
		</TitleTabsMenu>
	</Window>`
	root, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatal(err)
	}
	w, ok := root.(*widget.Window)
	if !ok {
		t.Fatalf("корень %T, ждали *widget.Window", root)
	}
	if !w.TitleTabsEnabled() || w.TitleTabCount() != 2 {
		t.Fatalf("TitleTabs: enabled=%v count=%d", w.TitleTabsEnabled(), w.TitleTabCount())
	}
	if got := w.TitleTabHeader(0); got != "одна" {
		t.Errorf("заголовок вкладки 0 = %q", got)
	}
	if w.TitleTabsMenu() == nil {
		t.Error("TitleTabsMenu не прикрепилось")
	}
	p1 := reg["p1"]
	if p1 == nil || !hasChild(w, p1) {
		t.Error("контент первой вкладки должен быть ребёнком окна")
	}
	if p1.Bounds() != w.ContentBounds() {
		t.Errorf("контент вкладки: %v, ждали %v", p1.Bounds(), w.ContentBounds())
	}
	if p2 := reg["p2"]; p2 != nil && hasChild(w, p2) {
		t.Error("контент второй (неактивной) вкладки не должен быть в дереве")
	}
}

func hasChild(w widget.Widget, target widget.Widget) bool {
	for _, c := range w.Children() {
		if c == target {
			return true
		}
	}
	return false
}
