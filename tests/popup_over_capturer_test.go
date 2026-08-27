// Всплывающее меню, открытое поверх виджета, который просит захват мыши
// (титлбар окна, вьюха терминала), должно получать клики само: открытый
// overlay старше обычного Z-порядка и старше заявки на захват.
//
// Сценарий заказчика (WinLine, ENGINE_ISSUES «A press over a window never
// reaches the menu open above it»): меню «Пуск» и меню вкладок открываются
// над окном, и ни один их пункт не срабатывал.
package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// greedyView — виджет, который забирает захват на ЛЮБОЕ нажатие в своих
// границах. Так ведёт себя вьюха терминала (выделение текста) и титлбар
// окна (перетаскивание): им нужен захват до того, как выяснится, потянут
// ли мышь на самом деле.
type greedyView struct {
	widget.Base
	presses int
}

func (g *greedyView) WantsCapture(e widget.MouseEvent) bool {
	return e.Button == widget.MouseLeft && e.Pressed &&
		image.Pt(e.X, e.Y).In(g.Bounds())
}

func (g *greedyView) OnMouseButton(e widget.MouseEvent) bool {
	if e.Pressed {
		g.presses++
	}
	return true
}

func (g *greedyView) Draw(ctx widget.DrawContext) {}

// newMenuOverView собирает сцену: жадный виджет во весь корень и меню,
// открытое поверх него. Возвращает движок, виджет, меню и точку внутри
// первого пункта меню.
func newMenuOverView(t *testing.T, items ...string) (*engine.Engine, *greedyView, *widget.PopupMenu, image.Point, *int) {
	t.Helper()
	eng := engine.New(800, 600, 30)
	root := widget.NewPanel(widget.DarkTheme().WindowBG)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 800, 600))

	view := &greedyView{}
	view.SetBounds(image.Rect(0, 0, 800, 600)) // окно во весь корень
	root.AddChild(view)

	clicked := 0
	menu := widget.NewPopupMenu()
	for _, it := range items {
		menu.AddItem(it, func() { clicked++ })
	}
	root.AddChild(menu)
	eng.SetRoot(root)

	// Меню открыто ВНУТРИ жадного виджета — именно этот случай ломался.
	menu.Show(200, 150)
	mb := menu.Bounds()
	if mb.Dx() == 0 || mb.Dy() == 0 {
		t.Fatal("меню не открылось: пустые bounds")
	}
	// Центр первого пункта: сверху вниз, отступ в половину высоты пункта.
	pt := image.Pt(mb.Min.X+mb.Dx()/2, mb.Min.Y+menu.ItemHeight/2)
	return eng, view, menu, pt, &clicked
}

// TestPopupOverCapturer_ItemReceivesClick — пункт меню, лежащего над
// виджетом с заявкой на захват, срабатывает.
func TestPopupOverCapturer_ItemReceivesClick(t *testing.T) {
	eng, view, menu, pt, clicked := newMenuOverView(t, "Открыть", "Закрыть")

	eng.SendMouseButton(pt.X, pt.Y, widget.MouseLeft, true)
	eng.SendMouseButton(pt.X, pt.Y, widget.MouseLeft, false)

	if *clicked != 1 {
		t.Errorf("пункт меню сработал %d раз, ждали 1", *clicked)
	}
	if view.presses != 0 {
		t.Errorf("нажатие ушло виджету под меню (%d раз) — меню его не получило", view.presses)
	}
	if menu.IsOpen() {
		t.Error("меню осталось открытым после выбора пункта")
	}
}

// TestPopupOverCapturer_ClickOutsideStillCaptures — клик МИМО меню работает
// по-прежнему: захват достаётся виджету под курсором, меню гаснет.
func TestPopupOverCapturer_ClickOutsideStillCaptures(t *testing.T) {
	eng, view, menu, _, clicked := newMenuOverView(t, "Открыть", "Закрыть")

	// Точка заведомо вне меню (меню открыто в 200,150).
	eng.SendMouseButton(600, 500, widget.MouseLeft, true)
	eng.SendMouseButton(600, 500, widget.MouseLeft, false)

	if view.presses == 0 {
		t.Error("клик мимо меню не дошёл до виджета — захват сломан")
	}
	if *clicked != 0 {
		t.Errorf("сработал пункт меню (%d), хотя кликали мимо", *clicked)
	}
	if menu.IsOpen() {
		t.Error("меню не погасло при клике в стороне")
	}
}
