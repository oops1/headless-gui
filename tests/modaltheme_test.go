package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

// Тема диалога — запрос GG-37.
//
// SetTheme красит дерево и УЖЕ показанные модалки, а диалог, созданный после
// смены темы, оставался с цветами, полученными при создании. У большинства
// виджетов это незаметно — они читают палитру во время отрисовки, — но виджет,
// кеширующий цвета у себя (DataGridWidget), оставался с умолчаниями: в светлой
// теме таблица внутри диалога рисовалась тёмной.

func themedModalScene(t *testing.T, th *widget.Theme) (*engine.Engine, *widget.DataGridWidget) {
	t.Helper()
	root := widget.NewPanel(th.WindowBG)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 300))

	eng := engine.New(400, 300, 30)
	eng.SetRoot(root)
	eng.SetTheme(th) // тема сменилась ДО создания диалога
	eng.RenderOnce()

	dlg := widget.NewDialog("Внешние репозитории", 300, 200)
	grid := widget.NewDataGridWidget()
	col := datagrid.NewTextColumn("Имя", "Name")
	col.SetActualWidth(200)
	grid.Grid.AddColumn(col)
	oc := datagrid.NewObservableCollection()
	oc.Add(&tipRow{Name: "origin", State: "—"})
	grid.Grid.SetItemsSource(oc)
	grid.SetBounds(image.Rect(20, 60, 280, 200))
	dlg.AddChild(grid)

	eng.ShowModal(dlg)
	eng.RenderOnce()
	return eng, grid
}

// Таблица в диалоге, показанном после смены темы, красится этой темой.
func TestShowModal_AppliesCurrentTheme(t *testing.T) {
	th := widget.Win11LightTheme()
	_, grid := themedModalScene(t, th)

	if got := grid.Grid.Background; got != th.WindowBG {
		t.Errorf("фон таблицы в диалоге %v, тема даёт %v — диалог не покрашен", got, th.WindowBG)
	}
}

// То же в тёмной теме: проверка не привязана к одному набору цветов.
func TestShowModal_AppliesDarkTheme(t *testing.T) {
	th := widget.Win11DarkTheme()
	_, grid := themedModalScene(t, th)

	if got := grid.Grid.Background; got != th.WindowBG {
		t.Errorf("фон таблицы в диалоге %v, тема даёт %v", got, th.WindowBG)
	}
}

// Поддерево со своей темой диалог не перекрашивает — как и SetTheme.
func TestShowModal_KeepsOwnThemeScope(t *testing.T) {
	root := widget.NewPanel(widget.Theme{}.WindowBG)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 300))

	eng := engine.New(400, 300, 30)
	eng.SetRoot(root)
	eng.SetTheme(widget.Win11LightTheme())

	own := widget.Win11DarkTheme()
	scope := widget.NewThemeScope(own)
	scope.SetBounds(image.Rect(0, 0, 200, 100))
	grid := widget.NewDataGridWidget()
	grid.SetBounds(image.Rect(0, 0, 200, 100))
	scope.AddChild(grid)

	dlg := widget.NewDialog("Диалог", 300, 200)
	dlg.AddChild(scope)
	eng.ShowModal(dlg)
	eng.RenderOnce()

	if got := grid.Grid.Background; got == widget.Win11LightTheme().WindowBG {
		t.Error("показ диалога перекрасил поддерево с собственной темой")
	}
}
