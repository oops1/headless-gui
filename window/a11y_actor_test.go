package window

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestA11yFlattenKeepsWidget — плоский снимок сохраняет ссылку на виджет.
// Без неё мост доступности знает про элемент всё, кроме того, КОГО нажимать.
func TestA11yFlattenKeepsWidget(t *testing.T) {
	root := widget.NewWindow("Действия", 200, 100)
	btn := widget.NewButton("Да")
	btn.SetBounds(image.Rect(10, 10, 70, 40))
	root.AddChild(btn)

	win := newTestWindow(root)
	s := win.accessibilitySnapshot()
	if s.Nodes[a11yRootID].Widget != widget.Widget(root) {
		t.Errorf("корень снимка ссылается не на окно: %#v", s.Nodes[a11yRootID].Widget)
	}
	found := false
	for i := range s.Nodes {
		if s.Nodes[i].Info.Name == "Да" {
			found = true
			if s.Nodes[i].Widget != widget.Widget(btn) {
				t.Errorf("узел кнопки ссылается не на кнопку: %#v", s.Nodes[i].Widget)
			}
		}
	}
	if !found {
		t.Fatal("узел кнопки не найден в снимке")
	}
	// Синтетические узлы (построенные мостом вручную) виджета не имеют.
	if a11yFlatten(buildTestAccessTree("abc")).Nodes[1].Widget != nil {
		t.Error("у узла без виджета Widget должен быть nil")
	}
}

// TestWindowAccessibilityActions — окно проксирует действия доступности в
// движок: фокус и нажатие. Это то, что дёргают GrabFocus/DoAction из мостов.
func TestWindowAccessibilityActions(t *testing.T) {
	root := widget.NewWindow("Действия", 200, 100)

	// Кнопки ниже заголовка окна: клик в полосу заголовка перехватывает
	// сам widget.Window (drag), и до кнопки событие не дошло бы.
	clicks := 0
	btn := widget.NewButton("Да")
	btn.SetBounds(image.Rect(10, 50, 70, 80))
	btn.OnClick = func() { clicks++ }
	root.AddChild(btn)

	off := widget.NewButton("Нет")
	off.SetBounds(image.Rect(80, 50, 140, 80))
	off.SetEnabled(false)
	root.AddChild(off)

	win := newTestWindow(root)

	if !win.accessibilityFocus(btn) {
		t.Fatal("accessibilityFocus вернул false для включённой кнопки")
	}
	if !btn.IsFocused() {
		t.Error("кнопка не получила фокус")
	}
	if !win.accessibilityActivate(btn) || clicks != 1 {
		t.Errorf("activate: clicks=%d, ожидался 1", clicks)
	}

	// Виджета нет (синтетический узел приложения) — действий нет.
	if win.accessibilityFocus(nil) || win.accessibilityActivate(nil) {
		t.Error("действия над nil-виджетом должны возвращать false")
	}
	// Выключенная кнопка не фокусируется и не нажимается.
	if win.accessibilityFocus(off) || win.accessibilityActivate(off) {
		t.Error("действия над выключенным виджетом должны возвращать false")
	}
}
