package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// accessFindByName ищет узел семантического дерева по имени (обход в глубину).
func accessFindByName(n *widget.AccessNode, name string) *widget.AccessNode {
	if n == nil {
		return nil
	}
	if n.Name == name {
		return n
	}
	for _, c := range n.Children {
		if f := accessFindByName(c, name); f != nil {
			return f
		}
	}
	return nil
}

// TestAccessNode_WidgetRef — узлы дерева ссылаются на свои виджеты: без этого
// мост доступности не сможет ничего сделать с найденным элементом.
func TestAccessNode_WidgetRef(t *testing.T) {
	eng := New(400, 300, 20)
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 300))
	btn := widget.NewButton("ОК")
	btn.SetBounds(image.Rect(10, 10, 110, 40))
	root.AddChild(btn)
	eng.SetRoot(root)

	tree := eng.AccessibilityTree()
	if tree.Widget != widget.Widget(root) {
		t.Fatalf("корень дерева ссылается не на root: %#v", tree.Widget)
	}
	n := accessFindByName(tree, "ОК")
	if n == nil {
		t.Fatal("узел кнопки не найден")
	}
	if n.Widget != widget.Widget(btn) {
		t.Fatalf("узел кнопки ссылается не на кнопку: %#v", n.Widget)
	}
}

// TestActivateAccessible_ClicksButton — синтетический клик доходит до OnClick
// и попутно проходит штатный путь ввода (кнопка получает фокус, как от мыши).
func TestActivateAccessible_ClicksButton(t *testing.T) {
	eng := New(400, 300, 20)
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 300))

	clicks := 0
	btn := widget.NewButton("Сохранить")
	btn.SetBounds(image.Rect(10, 10, 110, 40))
	btn.OnClick = func() { clicks++ }
	root.AddChild(btn)
	eng.SetRoot(root)

	if !eng.ActivateAccessible(btn) {
		t.Fatal("ActivateAccessible вернул false для видимой включённой кнопки")
	}
	if clicks != 1 {
		t.Fatalf("OnClick вызван %d раз, ожидался 1", clicks)
	}
	if got := eng.focus.get(); got != widget.Widget(btn) {
		t.Fatalf("после клика фокус на %#v, ожидалась кнопка", got)
	}
	if !btn.IsFocused() {
		t.Fatal("кнопка не считает себя сфокусированной")
	}

	// Повторная активация — ещё один клик (счётчик не залипает).
	eng.ActivateAccessible(btn)
	if clicks != 2 {
		t.Fatalf("после второй активации clicks=%d, ожидалось 2", clicks)
	}
}

// TestActivateAccessible_HiDPI — центр виджета переводится в ФИЗИЧЕСКИЕ
// координаты: при Scale != 1 клик иначе улетел бы мимо.
func TestActivateAccessible_HiDPI(t *testing.T) {
	eng := New(400, 300, 20)
	eng.SetScale(2)

	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 300))

	clicks := 0
	btn := widget.NewButton("Масштаб")
	btn.SetBounds(image.Rect(40, 60, 140, 100))
	btn.OnClick = func() { clicks++ }
	root.AddChild(btn)
	eng.SetRoot(root)

	if !eng.ActivateAccessible(btn) {
		t.Fatal("ActivateAccessible вернул false при scale=2")
	}
	if clicks != 1 {
		t.Fatalf("при scale=2 OnClick вызван %d раз, ожидался 1", clicks)
	}
}

// TestActivateAccessible_Rejects — скрытый, выключенный, пустой и nil-виджет
// не активируются: скринридер не должен уметь того, чего не может мышь.
func TestActivateAccessible_Rejects(t *testing.T) {
	eng := New(400, 300, 20)
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 300))

	hiddenClicks, disabledClicks, emptyClicks := 0, 0, 0

	hidden := widget.NewButton("Скрытая")
	hidden.SetBounds(image.Rect(10, 10, 110, 40))
	hidden.SetVisible(false)
	hidden.OnClick = func() { hiddenClicks++ }
	root.AddChild(hidden)

	disabled := widget.NewButton("Выключенная")
	disabled.SetBounds(image.Rect(10, 50, 110, 80))
	disabled.SetEnabled(false)
	disabled.OnClick = func() { disabledClicks++ }
	root.AddChild(disabled)

	empty := widget.NewButton("Без размера")
	empty.SetBounds(image.Rect(200, 200, 200, 200))
	empty.OnClick = func() { emptyClicks++ }
	root.AddChild(empty)

	eng.SetRoot(root)

	cases := []struct {
		name string
		w    widget.Widget
		hits *int
	}{
		{"скрытый", hidden, &hiddenClicks},
		{"выключенный", disabled, &disabledClicks},
		{"пустые границы", empty, &emptyClicks},
	}
	for _, c := range cases {
		if eng.ActivateAccessible(c.w) {
			t.Errorf("%s виджет активирован, ожидался false", c.name)
		}
		if *c.hits != 0 {
			t.Errorf("%s виджет получил %d кликов, ожидалось 0", c.name, *c.hits)
		}
	}
	if eng.ActivateAccessible(nil) {
		t.Error("ActivateAccessible(nil) вернул true")
	}
}

// TestFocusAccessible — фокус переходит на виджет, и это видно в снапшоте
// семантики (состояние focused) — ровно то, что читает скринридер.
func TestFocusAccessible(t *testing.T) {
	eng := New(400, 300, 20)
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 300))

	first := widget.NewButton("Первая")
	first.SetBounds(image.Rect(10, 10, 110, 40))
	root.AddChild(first)

	second := widget.NewButton("Вторая")
	second.SetBounds(image.Rect(10, 50, 110, 80))
	root.AddChild(second)

	eng.SetRoot(root)
	eng.SetFocus(first)

	if !eng.FocusAccessible(second) {
		t.Fatal("FocusAccessible вернул false для видимой включённой кнопки")
	}
	if got := eng.focus.get(); got != widget.Widget(second) {
		t.Fatalf("фокус на %#v, ожидалась вторая кнопка", got)
	}
	if first.IsFocused() {
		t.Error("первая кнопка сохранила фокус")
	}

	tree := eng.AccessibilityTree()
	n := accessFindByName(tree, "Вторая")
	if n == nil {
		t.Fatal("узел второй кнопки не найден")
	}
	if !hasState(n, widget.StateFocused) {
		t.Errorf("в снапшоте у второй кнопки нет состояния focused: %v", n.States)
	}
	if p := accessFindByName(tree, "Первая"); p != nil && hasState(p, widget.StateFocused) {
		t.Error("в снапшоте у первой кнопки осталось состояние focused")
	}
}

// TestFocusAccessible_Rejects — скрытый/выключенный/нефокусируемый виджет
// фокус не забирает.
func TestFocusAccessible_Rejects(t *testing.T) {
	eng := New(400, 300, 20)
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 300))

	ok := widget.NewButton("Годная")
	ok.SetBounds(image.Rect(10, 10, 110, 40))
	root.AddChild(ok)

	hidden := widget.NewButton("Скрытая")
	hidden.SetBounds(image.Rect(10, 50, 110, 80))
	hidden.SetVisible(false)
	root.AddChild(hidden)

	disabled := widget.NewButton("Выключенная")
	disabled.SetBounds(image.Rect(10, 90, 110, 120))
	disabled.SetEnabled(false)
	root.AddChild(disabled)

	// Label фокус не принимает (не реализует widget.Focusable).
	lbl := widget.NewLabel("Подпись", color.RGBA{R: 255, G: 255, B: 255, A: 255})
	lbl.SetBounds(image.Rect(10, 130, 110, 150))
	root.AddChild(lbl)

	eng.SetRoot(root)
	eng.SetFocus(ok)

	for _, c := range []struct {
		name string
		w    widget.Widget
	}{
		{"скрытый", hidden},
		{"выключенный", disabled},
		{"нефокусируемый", lbl},
		{"nil", nil},
	} {
		if eng.FocusAccessible(c.w) {
			t.Errorf("FocusAccessible(%s) вернул true", c.name)
		}
		if got := eng.focus.get(); got != widget.Widget(ok) {
			t.Fatalf("после отказа (%s) фокус уехал на %#v", c.name, got)
		}
	}
}
