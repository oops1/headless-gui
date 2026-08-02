//go:build linux && !android

package window

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// newATSPIActionBridge собирает мост БЕЗ подключения к шине: обработчики
// вызовов — чистые функции над снимком, реестр AT-SPI для них не нужен.
// Возвращает мост, текущий вид снимка и кнопку с счётчиком нажатий.
func newATSPIActionBridge(t *testing.T) (*atspiBridge, *a11yView, *widget.Button, *int) {
	t.Helper()

	root := widget.NewWindow("Действия", 200, 100)
	clicks := 0
	// Ниже полосы заголовка: клик в заголовок перехватывает сам widget.Window.
	btn := widget.NewButton("Да")
	btn.SetBounds(image.Rect(10, 50, 70, 80))
	btn.OnClick = func() { clicks++ }
	root.AddChild(btn)

	win := newTestWindow(root)
	win.scale = 1
	b := &atspiBridge{win: win, appName: ":1.7"}
	v := b.ids.assign(win.accessibilitySnapshot())
	return b, v, btn, &clicks
}

// findA11yNode ищет в виде узел по имени; возвращает устойчивый id и узел.
func findA11yNode(v *a11yView, name string) (int32, *a11yNode) {
	for i := range v.Snap.Nodes {
		if v.Snap.Nodes[i].Info.Name == name {
			return v.id(int32(i)), &v.Snap.Nodes[i]
		}
	}
	return -1, nil
}

// TestATSPIGrabFocus — Component.GrabFocus переводит фокус ввода на элемент.
func TestATSPIGrabFocus(t *testing.T) {
	b, v, btn, _ := newATSPIActionBridge(t)
	id, node := findA11yNode(v, "Да")
	if node == nil {
		t.Fatal("узел кнопки не найден")
	}

	rep := b.handleComponent(&dbusMessage{Interface: ifaceComponent, Member: "GrabFocus"}, v, id, node)
	if rep == nil || rep.Sig != "b" {
		t.Fatalf("GrabFocus вернул %#v", rep)
	}
	if ok, _ := rep.Body[0].(bool); !ok {
		t.Fatal("GrabFocus вернул false для обычной кнопки")
	}
	if !btn.IsFocused() {
		t.Error("кнопка не получила фокус")
	}

	// Синтетический узел приложения виджета не имеет — действий нет.
	app := b.appNode(v)
	rep = b.handleComponent(&dbusMessage{Interface: ifaceComponent, Member: "GrabFocus"}, v, atspiAppID, app)
	if ok, _ := rep.Body[0].(bool); ok {
		t.Error("GrabFocus по узлу приложения должен возвращать false")
	}
}

// TestATSPIDoAction — Action.DoAction(0) нажимает элемент, прочие номера
// действий вне диапазона.
func TestATSPIDoAction(t *testing.T) {
	b, v, _, clicks := newATSPIActionBridge(t)
	_, node := findA11yNode(v, "Да")
	if node == nil {
		t.Fatal("узел кнопки не найден")
	}

	call := func(idx int32) bool {
		rep := b.handleAction(&dbusMessage{
			Interface: ifaceAction, Member: "DoAction", Body: []any{idx}}, node)
		if rep == nil || rep.Sig != "b" {
			t.Fatalf("DoAction(%d) вернул %#v", idx, rep)
		}
		ok, _ := rep.Body[0].(bool)
		return ok
	}

	if !call(0) {
		t.Fatal("DoAction(0) вернул false для кнопки")
	}
	if *clicks != 1 {
		t.Fatalf("OnClick вызван %d раз, ожидался 1", *clicks)
	}
	if call(1) {
		t.Error("DoAction(1) должен возвращать false — действие одно")
	}
	if *clicks != 1 {
		t.Errorf("после DoAction(1) clicks=%d, ожидался 1", *clicks)
	}

	// Узел без виджета (приложение) нажать нельзя.
	rep := b.handleAction(&dbusMessage{
		Interface: ifaceAction, Member: "DoAction", Body: []any{int32(0)}}, b.appNode(v))
	if ok, _ := rep.Body[0].(bool); ok {
		t.Error("DoAction по узлу приложения должен возвращать false")
	}
}
