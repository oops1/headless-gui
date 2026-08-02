package window

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// buildTestAccessTree — дерево семантики для проверок плоского снимка:
//
//	окно (0,0-200x100)
//	├── кнопка «Да»   (10,10-60x30)
//	├── панель        (0,40-200x60)
//	│   └── чекбокс   (10,50-80x20), отмечен
//	└── поле ввода    (100,10-90x20), в фокусе
func buildTestAccessTree(focusValue string) *widget.AccessNode {
	btn := &widget.AccessNode{AccessInfo: widget.AccessInfo{
		Role: widget.RoleButton, Name: "Да", Bounds: image.Rect(10, 10, 70, 40)}}
	chk := &widget.AccessNode{AccessInfo: widget.AccessInfo{
		Role: widget.RoleCheckBox, Name: "Флажок", Bounds: image.Rect(10, 50, 90, 70),
		States: []string{widget.StateChecked}}}
	panel := &widget.AccessNode{AccessInfo: widget.AccessInfo{
		Role: widget.RolePanel, Bounds: image.Rect(0, 40, 200, 100)},
		Children: []*widget.AccessNode{chk}}
	edit := &widget.AccessNode{AccessInfo: widget.AccessInfo{
		Role: widget.RoleTextInput, Value: focusValue, Bounds: image.Rect(100, 10, 190, 30),
		States: []string{widget.StateFocused}}}
	return &widget.AccessNode{AccessInfo: widget.AccessInfo{
		Role: widget.RoleWindow, Name: "Окно", Bounds: image.Rect(0, 0, 200, 100)},
		Children: []*widget.AccessNode{btn, panel, edit}}
}

// TestA11yFlatten — разворачивание дерева в плоский снимок: связи, индексы, фокус.
func TestA11yFlatten(t *testing.T) {
	s := a11yFlatten(buildTestAccessTree("abc"))
	if len(s.Nodes) != 5 {
		t.Fatalf("узлов %d, want 5", len(s.Nodes))
	}
	root := s.node(a11yRootID)
	if root.Parent != -1 || len(root.Children) != 3 {
		t.Fatalf("корень: parent=%d, детей=%d", root.Parent, len(root.Children))
	}
	// Порядок обхода в глубину: 0 окно, 1 кнопка, 2 панель, 3 чекбокс, 4 поле.
	if s.Nodes[1].Info.Name != "Да" || s.Nodes[3].Info.Role != widget.RoleCheckBox ||
		s.Nodes[4].Info.Role != widget.RoleTextInput {
		t.Fatalf("порядок узлов нарушен: %+v", s.Nodes)
	}
	if s.Nodes[3].Parent != 2 || s.Nodes[3].Index != 0 {
		t.Errorf("чекбокс: parent=%d index=%d, want 2/0", s.Nodes[3].Parent, s.Nodes[3].Index)
	}
	if s.Nodes[4].Index != 2 {
		t.Errorf("поле ввода: index=%d, want 2", s.Nodes[4].Index)
	}
	if s.Focus != 4 {
		t.Errorf("фокус = %d, want 4", s.Focus)
	}
	if a11yFlatten(nil).Focus != -1 || len(a11yFlatten(nil).Nodes) != 0 {
		t.Error("пустое дерево должно давать пустой снимок без фокуса")
	}
}

// TestA11yHitTest — попадание точкой: самый глубокий узел, последний из
// перекрывающихся, промах вне корня.
func TestA11yHitTest(t *testing.T) {
	s := a11yFlatten(buildTestAccessTree("abc"))
	cases := []struct {
		x, y int
		want int32
	}{
		{20, 20, 1},    // кнопка
		{20, 60, 3},    // чекбокс внутри панели
		{150, 60, 2},   // панель (мимо чекбокса)
		{150, 20, 4},   // поле ввода
		{95, 5, 0},     // корень
		{-5, 5, -1},    // мимо окна
		{300, 300, -1}, // мимо окна
	}
	for _, c := range cases {
		if got := s.hitTest(c.x, c.y); got != c.want {
			t.Errorf("hitTest(%d,%d) = %d, want %d", c.x, c.y, got, c.want)
		}
	}
	var empty *a11ySnapshot
	if empty.hitTest(0, 0) != -1 {
		t.Error("hitTest по nil-снимку должен вернуть -1")
	}
}

// TestA11yDiff — диффы снимков: фокус, имя, значение, состояния, структура.
func TestA11yDiff(t *testing.T) {
	old := a11yFlatten(buildTestAccessTree("abc"))

	// Значение поля изменилось.
	cur := a11yFlatten(buildTestAccessTree("abcd"))
	ch := a11yDiff(old, cur)
	if ch.Structural {
		t.Fatal("смена значения — не структурное изменение")
	}
	if len(ch.ValueChanged) != 1 || ch.ValueChanged[0] != 4 {
		t.Errorf("ValueChanged = %v, want [4]", ch.ValueChanged)
	}
	if ch.FocusLost != -1 || ch.FocusGained != -1 {
		t.Errorf("фокус не менялся, а диффом сообщено: %d → %d", ch.FocusLost, ch.FocusGained)
	}

	// Фокус ушёл на кнопку, чекбокс снят, имя кнопки другое.
	tree := buildTestAccessTree("abc")
	tree.Children[2].States = nil
	tree.Children[0].States = []string{widget.StateFocused}
	tree.Children[0].Name = "Нет"
	tree.Children[1].Children[0].States = nil
	cur = a11yFlatten(tree)
	ch = a11yDiff(old, cur)
	if ch.FocusLost != 4 || ch.FocusGained != 1 {
		t.Errorf("фокус: %d → %d, want 4 → 1", ch.FocusLost, ch.FocusGained)
	}
	if len(ch.NameChanged) != 1 || ch.NameChanged[0] != 1 {
		t.Errorf("NameChanged = %v, want [1]", ch.NameChanged)
	}
	// Состояния изменились у кнопки (получила focused) и у чекбокса (снят).
	if len(ch.StateChanged) != 3 {
		t.Errorf("StateChanged = %v, want три узла (кнопка, чекбокс, поле)", ch.StateChanged)
	}

	// Удалили узел — структурное изменение.
	tree = buildTestAccessTree("abc")
	tree.Children = tree.Children[:2]
	ch = a11yDiff(old, a11yFlatten(tree))
	if !ch.Structural {
		t.Error("удаление узла должно давать Structural")
	}

	// Первый снимок (old == nil) — тоже структурное изменение с фокусом.
	ch = a11yDiff(nil, cur)
	if !ch.Structural || ch.FocusGained != 1 {
		t.Errorf("первый снимок: Structural=%v focus=%d", ch.Structural, ch.FocusGained)
	}
}

// TestA11ySnapshotWithoutBridge — окно без движка-провайдера семантики отдаёт
// пустой снимок, а не панику.
func TestA11ySnapshotWithoutBridge(t *testing.T) {
	root := widget.NewWindow("T", 200, 100)
	btn := widget.NewButton("Да")
	btn.SetBounds(image.Rect(10, 10, 70, 40))
	root.AddChild(btn)
	win := newTestWindow(root)
	s := win.accessibilitySnapshot()
	if len(s.Nodes) == 0 {
		t.Fatal("движок должен отдавать семантику")
	}
	if s.Nodes[a11yRootID].Info.Role != widget.RoleWindow {
		t.Errorf("корень снимка: %v", s.Nodes[a11yRootID].Info)
	}
}
