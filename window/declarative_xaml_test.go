package window

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// newTestWindow создаёт headless-окно (без Run/native) поверх движка с корнем
// root — для проверки подхвата деклараций трея/докинга из виджет-дерева.
func newTestWindow(root widget.Widget) *Window {
	eng := engine.New(400, 300, 30)
	eng.SetRoot(root)
	return New(eng, "")
}

// TestPickupDeclarativeDockFloating_InTab проверяет, что обход дерева находит
// DockManager с NativeFloating=true даже в НЕактивной вкладке TabControl
// (как в showcase) и включает нативный отрыв.
func TestPickupDeclarativeDockFloating_InTab(t *testing.T) {
	dm := widget.NewDockManager()
	dm.NativeFloating = true

	tc := widget.NewTabControl()
	tc.AddTab("Первая", widget.NewPanel(color.RGBA{}))
	tc.AddTab("Докинг", dm)
	tc.SetActive(0) // вкладка с DockManager НЕактивна

	root := widget.NewWindow("T", 400, 300)
	root.AddChild(tc)

	win := newTestWindow(root)
	win.pickupDeclarativeDockFloating()
	if win.dockMgr != dm {
		t.Fatalf("dockMgr = %v, ожидался найденный в неактивной вкладке DockManager", win.dockMgr)
	}
}

// TestPickupDeclarativeDockFloating_CodeWins проверяет приоритет явного
// EnableDockFloating: если приложение уже задало менеджер, декларация не
// перетирает его.
func TestPickupDeclarativeDockFloating_CodeWins(t *testing.T) {
	appDM := widget.NewDockManager()
	xamlDM := widget.NewDockManager()
	xamlDM.NativeFloating = true

	root := widget.NewWindow("T", 400, 300)
	root.AddChild(xamlDM)

	win := newTestWindow(root)
	win.EnableDockFloating(appDM) // явный вызов приложения
	win.pickupDeclarativeDockFloating()
	if win.dockMgr != appDM {
		t.Errorf("dockMgr = %v, ожидался заданный приложением appDM (приоритет кода)", win.dockMgr)
	}
}

// TestPickupDeclarativeTray_FillsUnset проверяет перенос декларации трея из
// widget.Window в буферизованные поля window.Window.
func TestPickupDeclarativeTray_FillsUnset(t *testing.T) {
	ww := widget.NewWindow("Моё приложение", 400, 300)
	ww.TrayIconImage = image.NewRGBA(image.Rect(0, 0, 32, 32))
	ww.TrayTooltip = "подсказка"
	ww.TrayMenu = widget.NewPopupMenu()

	win := newTestWindow(ww)
	win.pickupDeclarativeTray()
	if !win.trayIconWant || win.trayIcon == nil {
		t.Error("иконка трея не подхвачена из декларации")
	}
	if win.trayTooltip != "подсказка" {
		t.Errorf("trayTooltip = %q, ожидалось «подсказка»", win.trayTooltip)
	}
	if win.trayMenu != ww.TrayMenu {
		t.Error("trayMenu не подхвачено из декларации")
	}
}

// TestPickupDeclarativeTray_CodeWins проверяет, что явно заданные приложением
// иконка/меню не перетираются XAML-декларацией.
func TestPickupDeclarativeTray_CodeWins(t *testing.T) {
	ww := widget.NewWindow("App", 400, 300)
	ww.TrayIconImage = image.NewRGBA(image.Rect(0, 0, 32, 32))
	ww.TrayMenu = widget.NewPopupMenu()

	win := newTestWindow(ww)
	// Приложение задало своё до Run (native == nil → буферизуется).
	appIcon := image.NewRGBA(image.Rect(0, 0, 16, 16))
	_ = win.SetTrayIcon(appIcon, "app tooltip")
	appMenu := widget.NewPopupMenu()
	win.SetTrayMenu(appMenu)

	win.pickupDeclarativeTray()
	if win.trayIcon != appIcon {
		t.Error("иконка приложения перетёрта XAML-декларацией")
	}
	if win.trayTooltip != "app tooltip" {
		t.Errorf("trayTooltip = %q, ожидалось «app tooltip»", win.trayTooltip)
	}
	if win.trayMenu != appMenu {
		t.Error("меню приложения перетёрто XAML-декларацией")
	}
}
