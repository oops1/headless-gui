package core

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Действия по узлам и слой меню в общем слое доступности (v3.38): маршрутизация
// действия к узлу, поиск по точке, фокус меню, видимые границы.

// nodeOwner — виджет, у которого два узла рисует он сам. Принимает действия по
// узлам; без обработчика (plain=true) ведёт себя как прежде.
type nodeOwner struct {
	widget.Base
	calls   []string
	focused bool
	reply   bool
}

func (o *nodeOwner) Draw(widget.DrawContext) {}

func (o *nodeOwner) AccessChildren() []widget.AccessInfo {
	b := o.Bounds()
	mid := (b.Min.X + b.Max.X) / 2
	return []widget.AccessInfo{
		{Role: widget.RoleTextInput, Name: "левая", ID: "L", Bounds: image.Rect(b.Min.X, b.Min.Y, mid, b.Max.Y)},
		{Role: widget.RoleTextInput, Name: "правая", ID: "R", Bounds: image.Rect(mid, b.Min.Y, b.Max.X, b.Max.Y)},
	}
}

func (o *nodeOwner) AccessAction(id string, action widget.AccessActionKind) bool {
	o.calls = append(o.calls, id+":"+action.String())
	return o.reply
}

func (o *nodeOwner) SetFocused(v bool) { o.focused = v }
func (o *nodeOwner) IsFocused() bool   { return o.focused }

// plainOwner — тот же владелец узлов, но без обработчика действий.
type plainOwner struct {
	widget.Base
	clicks int
}

func (o *plainOwner) Draw(widget.DrawContext) {}

func (o *plainOwner) AccessChildren() []widget.AccessInfo {
	b := o.Bounds()
	return []widget.AccessInfo{{Role: widget.RoleTextInput, Name: "узел", Bounds: b}}
}

func (o *plainOwner) OnMouseButton(e widget.MouseEvent) bool {
	if e.Button == widget.MouseLeft && !e.Pressed {
		o.clicks++
	}
	return true
}

func (o *plainOwner) SetFocused(bool) {}
func (o *plainOwner) IsFocused() bool { return false }

// nodeNamed ищет узел снимка по имени.
func nodeNamed(s *a11ySnapshot, name string) (int32, *a11yNode) {
	for i := range s.Nodes {
		if s.Nodes[i].Info.Name == name {
			return int32(i), &s.Nodes[i]
		}
	}
	return -1, nil
}

// Плоский снимок несёт идентификатор узла владельца, а устойчивые id узлов,
// различающихся только им, не совпадают.
func TestA11yFlatten_KeepsActionID(t *testing.T) {
	root := widget.NewWindow("Узлы", 400, 300)
	o := &nodeOwner{}
	o.SetBounds(image.Rect(10, 40, 210, 140))
	root.AddChild(o)

	win := newTestWindow(root)
	s := win.accessibilitySnapshot()
	_, l := nodeNamed(s, "левая")
	_, r := nodeNamed(s, "правая")
	if l == nil || r == nil {
		t.Fatal("узлы владельца не найдены в снимке")
	}
	if l.ActionID != "L" || r.ActionID != "R" || l.Widget != widget.Widget(o) {
		t.Errorf("ActionID: %q и %q, Widget=%v", l.ActionID, r.ActionID, l.Widget)
	}
	keys := a11yNodeKeys(s)
	seen := map[string]bool{}
	for _, k := range keys {
		if seen[k] {
			t.Errorf("повторяющийся структурный ключ %q", k)
		}
		seen[k] = true
	}
	// Узел самого виджета — без идентификатора.
	if s.Nodes[a11yRootID].ActionID != "" {
		t.Errorf("у окна ActionID=%q", s.Nodes[a11yRootID].ActionID)
	}
}

// Действие по узлу владельца с обработчиком доходит до обработчика с id узла и
// не превращается в клик по центру владельца.
func TestAccessibilityNodeAction_RoutesToOwnerHandler(t *testing.T) {
	root := widget.NewWindow("Узлы", 400, 300)
	o := &nodeOwner{reply: true}
	o.SetBounds(image.Rect(10, 40, 210, 140))
	root.AddChild(o)
	win := newTestWindow(root)
	s := win.accessibilitySnapshot()
	_, r := nodeNamed(s, "правая")

	for _, tc := range []struct {
		kind widget.AccessActionKind
		want string
	}{
		{widget.AccessInvoke, "R:invoke"},
		{widget.AccessFocus, "R:focus"},
		{widget.AccessSelect, "R:select"},
		{widget.AccessToggle, "R:toggle"},
		{widget.AccessExpand, "R:expand"},
		{widget.AccessCollapse, "R:collapse"},
	} {
		o.calls = nil
		if !win.accessibilityNodeAction(r, tc.kind) {
			t.Errorf("%v: действие отвергнуто", tc.kind)
		}
		if len(o.calls) != 1 || o.calls[0] != tc.want {
			t.Errorf("%v: обработчик получил %v, ожидалось %q", tc.kind, o.calls, tc.want)
		}
	}

	// Отказ обработчика окончателен: запасного клика в центр нет.
	o.reply = false
	o.calls = nil
	if win.accessibilityNodeAction(r, widget.AccessInvoke) {
		t.Error("отказ обработчика превращён в успех")
	}
	if len(o.calls) != 1 {
		t.Errorf("повторных вызовов быть не должно: %v", o.calls)
	}
}

// Владелец без обработчика — прежнее поведение: нажатие кликом в центр
// владельца, фокус — фокусом владельца, а раскрытие не выполняется.
func TestAccessibilityNodeAction_OwnerWithoutHandlerKeepsOldBehaviour(t *testing.T) {
	root := widget.NewWindow("Узлы", 400, 300)
	o := &plainOwner{}
	o.SetBounds(image.Rect(10, 60, 210, 140))
	root.AddChild(o)
	btn := widget.NewButton("Да")
	btn.SetBounds(image.Rect(220, 60, 280, 90))
	clicks := 0
	btn.OnClick = func() { clicks++ }
	root.AddChild(btn)
	win := newTestWindow(root)
	s := win.accessibilitySnapshot()

	_, n := nodeNamed(s, "узел")
	if n == nil || n.ActionID == "" {
		t.Fatalf("узел владельца без обработчика: %+v", n)
	}
	if !win.accessibilityNodeAction(n, widget.AccessInvoke) || o.clicks != 1 {
		t.Errorf("прежний путь: нажатие владельца, clicks=%d", o.clicks)
	}
	if win.accessibilityNodeAction(n, widget.AccessExpand) {
		t.Error("раскрытие без обработчика владельца принято")
	}

	// Обычный виджет: Invoke/Select/Toggle — клик, Focus — фокус.
	_, b := nodeNamed(s, "Да")
	if !win.accessibilityNodeAction(b, widget.AccessInvoke) || clicks != 1 {
		t.Errorf("Invoke кнопки: clicks=%d", clicks)
	}
	if !win.accessibilityNodeAction(b, widget.AccessSelect) || clicks != 2 {
		t.Errorf("Select кнопки: clicks=%d", clicks)
	}
	if !win.accessibilityNodeAction(b, widget.AccessToggle) || clicks != 3 {
		t.Errorf("Toggle кнопки: clicks=%d", clicks)
	}
	if !win.accessibilityNodeAction(b, widget.AccessFocus) || !btn.IsFocused() {
		t.Error("Focus кнопки не перевёл фокус")
	}
	if win.accessibilityNodeAction(b, widget.AccessCollapse) {
		t.Error("Collapse кнопки принят")
	}
	if win.accessibilityNodeAction(nil, widget.AccessInvoke) {
		t.Error("nil-узел принят")
	}
	if win.accessibilityNodeAction(&a11yNode{}, widget.AccessInvoke) {
		t.Error("узел без виджета принят")
	}
}

// ─── Поиск по точке ──────────────────────────────────────────────────────────

func snapOf(nodes ...a11yNode) *a11ySnapshot {
	s := &a11ySnapshot{Nodes: nodes, Focus: -1}
	for i := range s.Nodes {
		if p := s.Nodes[i].Parent; p >= 0 {
			s.Nodes[p].Children = append(s.Nodes[p].Children, int32(i))
		}
	}
	return s
}

func inf(role widget.AccessRole, r image.Rectangle, states ...string) widget.AccessInfo {
	return widget.AccessInfo{Role: role, Bounds: r, States: states}
}

// Открытое меню выступает за границы окна и владельца: точка в пункте
// возвращает пункт, а не то, что лежит под меню.
func TestA11yHitTest_PopupLayerWins(t *testing.T) {
	s := snapOf(
		a11yNode{Info: inf(widget.RoleWindow, image.Rect(0, 0, 200, 100)), Parent: -1},
		a11yNode{Info: inf(widget.RoleButton, image.Rect(10, 10, 190, 90)), Parent: 0},
		// Меню — ребёнок кнопки, но стоит за пределами и кнопки, и окна.
		a11yNode{Info: inf(widget.RoleMenu, image.Rect(150, 60, 260, 130), widget.StatePopup), Parent: 1},
		a11yNode{Info: inf(widget.RoleMenuItem, image.Rect(150, 60, 260, 90), widget.StatePopup), Parent: 2},
		a11yNode{Info: inf(widget.RoleMenuItem, image.Rect(150, 90, 260, 120), widget.StatePopup), Parent: 2},
	)
	for _, tc := range []struct {
		x, y int
		want int32
		note string
	}{
		{20, 20, 1, "кнопка вне меню"},
		{160, 70, 3, "первый пункт над кнопкой"},
		{250, 100, 4, "второй пункт за границей окна"},
		{300, 100, -1, "вне окна и вне меню"},
	} {
		if got := s.hitTest(tc.x, tc.y); got != tc.want {
			t.Errorf("%s: hitTest(%d,%d)=%d, ожидалось %d", tc.note, tc.x, tc.y, got, tc.want)
		}
	}
	// Закрытое меню (offscreen, без popup) точку не перехватывает.
	s.Nodes[2].Info.States = []string{widget.StateOffscreen}
	s.Nodes[3].Info.States = nil
	s.Nodes[4].Info.States = nil
	if got := s.hitTest(160, 70); got != 1 {
		t.Errorf("закрытое меню перехватило точку: %d", got)
	}
}

// Прокрученный за край элемент под курсором не лежит, частично видимый —
// лежит только своей видимой частью, даже если родитель точку содержит.
func TestA11yHitTest_UsesVisibleRect(t *testing.T) {
	vis := image.Rect(10, 80, 110, 100) // видимая часть: обрезана окном прокрутки
	s := snapOf(
		a11yNode{Info: inf(widget.RoleWindow, image.Rect(0, 0, 200, 200)), Parent: -1},
		a11yNode{Info: inf(widget.RoleList, image.Rect(0, 0, 200, 200)), Parent: 0},
		a11yNode{Info: widget.AccessInfo{Role: widget.RoleButton, Bounds: image.Rect(10, 80, 110, 130), Visible: vis}, Parent: 1},
		a11yNode{Info: inf(widget.RoleButton, image.Rect(10, 150, 110, 180), widget.StateOffscreen), Parent: 1},
	)
	if got := s.hitTest(50, 90); got != 2 {
		t.Errorf("точка в видимой части: %d, ожидалась кнопка 2", got)
	}
	// Полные границы кнопки доходят до y=130, но видна она только до y=100:
	// ниже под курсором список, а не кнопка.
	if got := s.hitTest(50, 110); got != 1 {
		t.Errorf("точка в обрезанной части: %d, ожидался список 1", got)
	}
	// Целиком вне видимой области: под курсором список, а не offscreen-кнопка.
	if got := s.hitTest(50, 160); got != 1 {
		t.Errorf("точка под offscreen-кнопкой: %d, ожидался список 1", got)
	}
	if r := a11yVisibleRect(s.Nodes[3].Info); !r.Empty() {
		t.Errorf("видимая часть offscreen-узла %v", r)
	}
	if r := a11yVisibleRect(s.Nodes[2].Info); r != vis {
		t.Errorf("видимая часть выступающего узла %v", r)
	}
	if r := a11yVisibleRect(s.Nodes[1].Info); r != s.Nodes[1].Info.Bounds {
		t.Errorf("видимая часть целого узла %v", r)
	}
}

// Подсвеченный пункт меню перебивает фокус виджетов: у меню фокус — это плашка.
func TestA11yFlatten_MenuItemFocusWins(t *testing.T) {
	tree := &widget.AccessNode{
		AccessInfo: widget.AccessInfo{Role: widget.RoleWindow, Bounds: image.Rect(0, 0, 200, 200)},
		Children: []*widget.AccessNode{
			{AccessInfo: widget.AccessInfo{Role: widget.RoleMenu, Bounds: image.Rect(0, 0, 100, 60),
				States: []string{widget.StatePopup}},
				Children: []*widget.AccessNode{
					{AccessInfo: widget.AccessInfo{Role: widget.RoleMenuItem, Name: "Один",
						Bounds: image.Rect(0, 0, 100, 30), States: []string{widget.StatePopup, widget.StateFocused}}},
					{AccessInfo: widget.AccessInfo{Role: widget.RoleMenuItem, Name: "Два",
						Bounds: image.Rect(0, 30, 100, 60), States: []string{widget.StatePopup}}},
				}},
			// Поле ввода идёт ПОСЛЕ меню в обходе и тоже в фокусе.
			{AccessInfo: widget.AccessInfo{Role: widget.RoleTextInput, Name: "Поле",
				Bounds: image.Rect(0, 100, 100, 130), States: []string{widget.StateFocused}}},
		},
	}
	s := a11yFlatten(tree)
	id, _ := nodeNamed(s, "Один")
	if s.Focus != id {
		t.Errorf("фокус %d, ожидался пункт меню %d", s.Focus, id)
	}
	// Без подсвеченного пункта фокус у поля, как раньше.
	tree.Children[0].Children[0].States = []string{widget.StatePopup}
	s = a11yFlatten(tree)
	if fid, _ := nodeNamed(s, "Поле"); s.Focus != fid {
		t.Errorf("без меню фокус %d, ожидалось поле %d", s.Focus, fid)
	}
}

// Настоящее меню: снимок окна видит его пункты, поиск по точке находит пункт
// под курсором, подсвеченный пункт — в фокусе, действия идут в меню.
func TestA11ySnapshot_PopupMenuEndToEnd(t *testing.T) {
	root := widget.NewWindow("Меню", 400, 300)
	m := widget.NewPopupMenu()
	picked := ""
	m.SetItems([]widget.MenuItem{
		{Text: "Первый"},
		{Text: "Второй", OnClick: func() { picked = "Второй" }},
	})
	root.AddChild(m)
	win := newTestWindow(root)
	m.Show(50, 80)

	s := win.accessibilitySnapshot()
	_, first := nodeNamed(s, "Первый")
	secondID, second := nodeNamed(s, "Второй")
	if first == nil || second == nil {
		t.Fatal("пункты меню не попали в снимок")
	}
	if first.Info.Role != widget.RoleMenuItem || first.ActionID != "0" || second.ActionID != "1" {
		t.Errorf("пункты: %+v / %+v", first, second)
	}

	// Центр второго пункта: поиск по точке возвращает его.
	c := second.Info.Bounds
	if got := s.hitTest(c.Min.X+c.Dx()/2, c.Min.Y+c.Dy()/2); got != secondID {
		t.Errorf("hitTest центра пункта = %d, ожидался %d", got, secondID)
	}

	// Действие «фокус» подсвечивает пункт, и он же становится фокусом снимка.
	if !win.accessibilityNodeAction(second, widget.AccessFocus) {
		t.Fatal("фокус на пункте меню отклонён")
	}
	s = win.accessibilitySnapshot()
	if s.Focus != secondID {
		t.Errorf("фокус снимка %d, ожидался подсвеченный пункт %d", s.Focus, secondID)
	}
	// Нажатие выполняет пункт и закрывает меню.
	if !win.accessibilityNodeAction(second, widget.AccessInvoke) || picked != "Второй" {
		t.Errorf("нажатие на пункт: picked=%q", picked)
	}
	if m.IsOpen() {
		t.Error("меню осталось открытым")
	}
	if _, gone := nodeNamed(win.accessibilitySnapshot(), "Второй"); gone != nil {
		t.Error("пункты закрытого меню остались в снимке")
	}
}

// Диалог сообщения виден снимку: роль, имя, описание и модальность.
func TestA11ySnapshot_DialogNamed(t *testing.T) {
	root := widget.NewWindow("Окно", 400, 300)
	eng := engine.New(400, 300, 30)
	eng.SetRoot(root)
	win := New(eng, "")
	dlg := widget.NewDialog("Подтверждение", 300, 150)
	dlg.Description = "Удалить файл?"
	eng.ShowModal(dlg)

	s := win.accessibilitySnapshot()
	for i := range s.Nodes {
		n := &s.Nodes[i]
		if n.Info.Role != widget.RoleDialog {
			continue
		}
		if n.Info.Name != "Подтверждение" || n.Info.Description != "Удалить файл?" {
			t.Errorf("диалог: имя %q, описание %q", n.Info.Name, n.Info.Description)
		}
		if !a11yHasState(n.Info.States, widget.StateModal) {
			t.Errorf("диалог без modal: %v", n.Info.States)
		}
		return
	}
	t.Fatal("диалога нет в снимке")
}

// showDialogForTest показывает модальный диалог в движке окна.
func showDialogForTest(t *testing.T, win *Window, dlg widget.ModalWidget) {
	t.Helper()
	e, ok := win.eng.(interface{ ShowModal(widget.ModalWidget) })
	if !ok {
		t.Fatal("движок окна не умеет ShowModal")
	}
	e.ShowModal(dlg)
}

// Устойчивый ключ узла владельца включает его идентификатор: пункт «2» остаётся
// «2», даже если из списка исчез разделитель перед ним и его порядковый номер
// среди детей сдвинулся.
func TestA11yNodeKeys_IncludeActionID(t *testing.T) {
	mk := func(id string) *a11ySnapshot {
		return snapOf(
			a11yNode{Info: inf(widget.RoleMenu, image.Rect(0, 0, 10, 10)), Parent: -1},
			a11yNode{Info: inf(widget.RoleMenuItem, image.Rect(0, 0, 10, 5)), Parent: 0, Index: 0, ActionID: id},
		)
	}
	a, b := a11yNodeKeys(mk("2")), a11yNodeKeys(mk("1"))
	if a[1] == b[1] {
		t.Errorf("узлы с разными идентификаторами получили один ключ %q", a[1])
	}
	if a[1] != a11yNodeKeys(mk("2"))[1] {
		t.Error("ключ узла неустойчив")
	}
}
