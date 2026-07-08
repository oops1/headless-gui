package engine

import (
	"encoding/json"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// findByRole возвращает первый узел с заданной ролью (обход в глубину).
func findByRole(n *widget.AccessNode, role widget.AccessRole) *widget.AccessNode {
	if n == nil {
		return nil
	}
	if n.Role == role {
		return n
	}
	for _, c := range n.Children {
		if f := findByRole(c, role); f != nil {
			return f
		}
	}
	return nil
}

func hasState(n *widget.AccessNode, s string) bool {
	for _, st := range n.States {
		if st == s {
			return true
		}
	}
	return false
}

func TestAccessibility_Tree(t *testing.T) {
	eng := New(400, 300, 20)

	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 300))

	btn := widget.NewButton("Сохранить")
	btn.SetBounds(image.Rect(10, 10, 120, 40))
	btn.ToolTip = "Сохраняет документ"
	root.AddChild(btn)

	cb := widget.NewCheckBox("Запомнить")
	cb.SetBounds(image.Rect(10, 50, 160, 72))
	cb.SetChecked(true)
	root.AddChild(cb)

	ti := widget.NewTextInput("")
	ti.SetBounds(image.Rect(10, 80, 200, 108))
	ti.SetText("hello")
	root.AddChild(ti)

	sl := widget.NewSlider()
	sl.SetBounds(image.Rect(10, 120, 200, 144))
	sl.SetValue(0.5)
	root.AddChild(sl)

	hidden := widget.NewButton("Скрытая")
	hidden.SetBounds(image.Rect(10, 150, 120, 180))
	hidden.SetVisible(false)
	root.AddChild(hidden)

	dis := widget.NewButton("Выключенная")
	dis.SetBounds(image.Rect(10, 190, 140, 220))
	dis.SetEnabled(false)
	root.AddChild(dis)

	eng.SetRoot(root)
	eng.SetFocus(ti)

	tree := eng.AccessibilityTree()
	if tree == nil {
		t.Fatal("дерево пустое")
	}
	if tree.Role != widget.RolePanel {
		t.Errorf("корень: роль %q, ожидалась panel", tree.Role)
	}

	b := findByRole(tree, widget.RoleButton)
	if b == nil || b.Name != "Сохранить" {
		t.Fatalf("кнопка не найдена или имя неверно: %+v", b)
	}
	if b.Description != "Сохраняет документ" {
		t.Errorf("Description из ToolTip: %q", b.Description)
	}

	c := findByRole(tree, widget.RoleCheckBox)
	if c == nil || !hasState(c, widget.StateChecked) {
		t.Errorf("чекбокс без состояния checked: %+v", c)
	}

	in := findByRole(tree, widget.RoleTextInput)
	if in == nil || in.Value != "hello" {
		t.Errorf("textinput value: %+v", in)
	}
	if !hasState(in, widget.StateFocused) {
		t.Errorf("фокусированный textinput без состояния focused: %+v", in)
	}

	s := findByRole(tree, widget.RoleSlider)
	if s == nil || s.Value != "0.5" {
		t.Errorf("slider value: %+v", s)
	}

	// Скрытый виджет исключён из дерева.
	count := 0
	var walk func(n *widget.AccessNode)
	walk = func(n *widget.AccessNode) {
		if n.Role == widget.RoleButton {
			count++
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(tree)
	if count != 2 { // Сохранить + Выключенная (скрытая — нет)
		t.Errorf("кнопок в дереве %d, ожидалось 2 (скрытая исключена)", count)
	}

	// Disabled-состояние.
	found := false
	walk = func(n *widget.AccessNode) {
		if n.Name == "Выключенная" && hasState(n, widget.StateDisabled) {
			found = true
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(tree)
	if !found {
		t.Error("выключенная кнопка без состояния disabled")
	}
}

func TestAccessibility_ModalAndJSON(t *testing.T) {
	// Диалог остаётся открытым до конца теста — ShowModal запускает fade-in
	// затемнения (AnimateOwned); подчищаем глобальный реестр анимаций, иначе
	// "зависшая" анимация тикает в других тестах пакета (движок здесь не
	// Start()-нут, StepAnimations её не продвигает сам).
	defer widget.StopAllAnimations()
	eng := New(300, 200, 20)
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 300, 200))
	eng.SetRoot(root)

	dlg := widget.NewDialog("Подтверждение", 200, 100)
	eng.ShowModal(dlg)

	tree := eng.AccessibilityTree()
	if tree == nil || len(tree.Children) == 0 {
		t.Fatal("модалка не попала в дерево")
	}
	modal := tree.Children[len(tree.Children)-1]
	if !hasState(modal, widget.StateModal) {
		t.Errorf("модальный узел без состояния modal: %+v", modal)
	}

	// JSON-сериализация (side-channel для стриминга).
	data, err := json.Marshal(tree)
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	if !strings.Contains(string(data), `"role"`) || !strings.Contains(string(data), `"modal"`) {
		t.Errorf("json не содержит ожидаемых полей: %s", data[:min(len(data), 200)])
	}
}

// Кастомный виджет с собственной семантикой (интерфейс Accessible).
type customAccessWidget struct {
	widget.Base
}

func (c *customAccessWidget) Draw(widget.DrawContext) {}
func (c *customAccessWidget) AccessInfo() widget.AccessInfo {
	return widget.AccessInfo{Role: widget.RoleButton, Name: "кастомный", Bounds: c.Bounds()}
}

func TestAccessibility_CustomWidget(t *testing.T) {
	w := &customAccessWidget{}
	w.SetBounds(image.Rect(0, 0, 10, 10))
	tree := widget.BuildAccessTree(w, nil)
	if tree == nil || tree.Role != widget.RoleButton || tree.Name != "кастомный" {
		t.Errorf("кастомная семантика не применилась: %+v", tree)
	}
}
