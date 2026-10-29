package base

import (
	"image"
	"image/color"
	"sync/atomic"
	"testing"
)

// Действия по узлам, диалог, прокрутка и меню в дереве доступности (v3.38).

// ─── Помощники ───────────────────────────────────────────────────────────────

// accNodeNamed ищет узел дерева по имени (обход в глубину).
func accNodeNamed(n *AccessNode, name string) *AccessNode {
	if n == nil {
		return nil
	}
	if n.Name == name {
		return n
	}
	for _, c := range n.Children {
		if f := accNodeNamed(c, name); f != nil {
			return f
		}
	}
	return nil
}

// accNodeByRole ищет первый узел заданной роли.
func accNodeByRole(n *AccessNode, role AccessRole) *AccessNode {
	if n == nil {
		return nil
	}
	if n.Role == role {
		return n
	}
	for _, c := range n.Children {
		if f := accNodeByRole(c, role); f != nil {
			return f
		}
	}
	return nil
}

func accHas(n *AccessNode, state string) bool { return accessHasState(n.States, state) }

// accOwner — виджет, рисующий свои узлы сам и принимающий действия над ними.
type accOwner struct {
	Base
	kids  []AccessInfo
	calls []string
	reply bool
}

func (o *accOwner) Draw(DrawContext) {}

func (o *accOwner) AccessChildren() []AccessInfo { return o.kids }

func (o *accOwner) AccessAction(id string, action AccessActionKind) bool {
	o.calls = append(o.calls, id+":"+action.String())
	return o.reply
}

// ─── Идентификаторы узлов и обработчик действий ─────────────────────────────

// Узлы из AccessChildren получают идентификатор: заданный владельцем остаётся,
// пустой заменяется номером в списке. У узлов самих виджетов идентификатора нет.
func TestBuildAccessTree_ActionIDs(t *testing.T) {
	o := &accOwner{kids: []AccessInfo{
		{Role: RoleTextInput, Name: "левая", Bounds: image.Rect(0, 0, 50, 20)},
		{Role: RoleTextInput, Name: "правая", ID: "right", Bounds: image.Rect(50, 0, 100, 20)},
	}}
	o.SetBounds(image.Rect(0, 0, 100, 20))
	btn := NewButton("Кнопка")
	btn.SetBounds(image.Rect(0, 30, 50, 50))
	root := NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 200, 200))
	root.AddChild(o)
	root.AddChild(btn)

	tree := BuildAccessTree(root, nil)
	left, right := accNodeNamed(tree, "левая"), accNodeNamed(tree, "правая")
	if left == nil || right == nil {
		t.Fatal("узлы владельца не найдены")
	}
	if left.ActionID != "0" || right.ActionID != "right" {
		t.Errorf("ActionID: %q и %q, ожидалось \"0\" и \"right\"", left.ActionID, right.ActionID)
	}
	if left.Widget != Widget(o) || right.Widget != Widget(o) {
		t.Error("узел владельца должен ссылаться на владельца")
	}
	if b := accNodeNamed(tree, "Кнопка"); b == nil || b.ActionID != "" {
		t.Errorf("у узла самого виджета ActionID должен быть пуст: %+v", b)
	}
	if o2 := accNodeByRole(tree, RolePanel); o2 == nil || o2.ActionID != "" {
		t.Error("у панели ActionID должен быть пуст")
	}
}

// AccessActionKind называет себя понятно (журналы, сообщения тестов).
func TestAccessActionKind_String(t *testing.T) {
	for k, want := range map[AccessActionKind]string{
		AccessInvoke: "invoke", AccessFocus: "focus", AccessSelect: "select",
		AccessToggle: "toggle", AccessExpand: "expand", AccessCollapse: "collapse",
		0: "unknown",
	} {
		if got := k.String(); got != want {
			t.Errorf("%d: %q, ожидалось %q", int(k), got, want)
		}
	}
}

// Панель сравнения принимает нажатие, фокус и выбор — и делает сторону
// активной; чужой идентификатор и непринятое действие отвергаются.
func TestDiffView_AccessAction(t *testing.T) {
	d := NewDiffView("", "")
	d.SetBounds(image.Rect(0, 0, 600, 300))

	tree := BuildAccessTree(d, nil)
	var ids []string
	for _, c := range tree.Children {
		if c.ActionID != "" {
			ids = append(ids, c.ActionID)
		}
	}
	if len(ids) != 2 || ids[0] != "0" || ids[1] != "1" {
		t.Fatalf("узлы панелей: %v, ожидалось [0 1]", ids)
	}

	if d.ActiveSide() != DiffLeft {
		t.Fatalf("исходная активная сторона %v", d.ActiveSide())
	}
	for _, act := range []AccessActionKind{AccessInvoke, AccessFocus, AccessSelect} {
		d.SetActiveSide(DiffLeft)
		if !d.AccessAction("1", act) {
			t.Errorf("%v: правая панель отказала", act)
		}
		if d.ActiveSide() != DiffRight {
			t.Errorf("%v: активной осталась %v", act, d.ActiveSide())
		}
	}
	if d.AccessAction("0", AccessExpand) || d.AccessAction("0", AccessToggle) {
		t.Error("панель не раскрывается и не переключается")
	}
	for _, bad := range []string{"", "2", "-1", "x"} {
		if d.AccessAction(bad, AccessInvoke) {
			t.Errorf("идентификатор %q принят", bad)
		}
	}
	d.SetActiveSide(DiffLeft)
	if d.AccessAction("9", AccessFocus); d.ActiveSide() != DiffLeft {
		t.Error("отказное действие сдвинуло активную сторону")
	}
}

// Слияние: идентификатор — номер стороны; скрытая база действий не принимает.
func TestMergeView_AccessAction(t *testing.T) {
	m := NewMergeView("", "")
	m.SetBounds(image.Rect(0, 0, 900, 400))

	tree := BuildAccessTree(m, nil)
	got := map[string]bool{}
	for _, c := range tree.Children {
		got[c.ActionID] = true
	}
	for _, want := range []string{"0", "1", "2", "3"} {
		if !got[want] {
			t.Errorf("нет узла панели %q: %v", want, got)
		}
	}

	if !m.AccessAction("2", AccessFocus) || m.ActiveSide() != MergeTheirs {
		t.Errorf("AccessFocus(их): активна %v", m.ActiveSide())
	}
	if !m.AccessAction("3", AccessInvoke) || m.ActiveSide() != MergeResult {
		t.Errorf("AccessInvoke(итог): активна %v", m.ActiveSide())
	}
	m.SetShowBase(false)
	m.SetActiveSide(MergeOurs)
	if m.AccessAction("1", AccessFocus) || m.ActiveSide() != MergeOurs {
		t.Error("скрытая база приняла действие")
	}
	if m.AccessAction("0", AccessToggle) || m.AccessAction("7", AccessFocus) {
		t.Error("непринимаемое действие или чужая сторона приняты")
	}
}

// ─── Диалог ──────────────────────────────────────────────────────────────────

// ─── Прокрутка: видимая часть ────────────────────────────────────────────────

// scrollScene: окно прокрутки 200×100 с тремя кнопками в содержимом высотой 400.
func scrollScene() (*ScrollView, *Button, *Button, *Button) {
	sv := NewScrollView()
	sv.SetBounds(image.Rect(10, 20, 210, 120))
	sv.ContentHeight = 400
	top := NewButton("верх")
	top.SetBounds(image.Rect(10, 20, 110, 50)) // содержимое: y 20..50
	mid := NewButton("середина")
	mid.SetBounds(image.Rect(10, 100, 110, 130)) // содержимое: y 100..130
	far := NewButton("низ")
	far.SetBounds(image.Rect(10, 300, 110, 330)) // содержимое: y 300..330
	sv.AddChild(top)
	sv.AddChild(mid)
	sv.AddChild(far)
	return sv, top, mid, far
}

// Без прокрутки видимые элементы не помечаются, а выступающий за окно получает
// видимую часть — пересечение, и не считается спрятанным; целиком вне окна —
// offscreen.
func TestBuildAccessTree_Offscreen(t *testing.T) {
	sv, _, _, _ := scrollScene()
	root := NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 300, 200))
	root.AddChild(sv)

	tree := BuildAccessTree(root, nil)
	top, mid, far := accNodeNamed(tree, "верх"), accNodeNamed(tree, "середина"), accNodeNamed(tree, "низ")
	if accHas(top, StateOffscreen) || !top.Visible.Empty() {
		t.Errorf("верхняя кнопка видна целиком: %+v", top.AccessInfo)
	}
	// Окно прокрутки — экранные y 20..120; кнопка «середина» в содержимом
	// 100..130 при нулевой прокрутке лежит на экране так же и обрезается по 120.
	if accHas(mid, StateOffscreen) {
		t.Error("частично видимая кнопка помечена offscreen")
	}
	if mid.Bounds != image.Rect(10, 100, 110, 130) {
		t.Errorf("границы частично видимой кнопки %v: должны остаться полными", mid.Bounds)
	}
	if want := image.Rect(10, 100, 110, 120); mid.Visible != want {
		t.Errorf("видимая часть %v, ожидалось %v", mid.Visible, want)
	}
	if !accHas(far, StateOffscreen) {
		t.Errorf("кнопка за краем окна без offscreen: %v", far.States)
	}
	if !far.Visible.Empty() {
		t.Errorf("у невидимой кнопки Visible=%v", far.Visible)
	}
}

// Прокрутка переносит границы на экран и меняет, кто виден: верхняя кнопка
// уезжает за край, нижняя выезжает.
func TestBuildAccessTree_OffscreenFollowsScroll(t *testing.T) {
	sv, _, _, _ := scrollScene()
	root := NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 300, 200))
	root.AddChild(sv)
	sv.SetScrollY(250)

	tree := BuildAccessTree(root, nil)
	top, far := accNodeNamed(tree, "верх"), accNodeNamed(tree, "низ")
	if !accHas(top, StateOffscreen) {
		t.Errorf("прокрученная за верх кнопка без offscreen: %v", top.States)
	}
	if accHas(far, StateOffscreen) {
		t.Errorf("выехавшая кнопка помечена offscreen: %v", far.States)
	}
	// content y 300..330 минус прокрутка 250 = экранные 50..80.
	if far.Bounds != image.Rect(10, 50, 110, 80) {
		t.Errorf("экранные границы %v, ожидалось 10,50 110,80", far.Bounds)
	}
	if !far.Visible.Empty() {
		t.Errorf("целиком видимая кнопка получила Visible %v", far.Visible)
	}
}

// Вложенные узлы владельца обрезаются так же, как виджеты.
func TestBuildAccessTree_OffscreenVirtualNodes(t *testing.T) {
	sv := NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 200, 100))
	sv.ContentHeight = 500
	o := &accOwner{kids: []AccessInfo{
		{Role: RoleTextInput, Name: "виден", Bounds: image.Rect(0, 10, 100, 40)},
		{Role: RoleTextInput, Name: "скрыт", Bounds: image.Rect(0, 300, 100, 340)},
	}}
	o.SetBounds(image.Rect(0, 0, 200, 500))
	sv.AddChild(o)

	tree := BuildAccessTree(sv, nil)
	if n := accNodeNamed(tree, "виден"); accHas(n, StateOffscreen) {
		t.Errorf("видимый узел владельца помечен offscreen")
	}
	if n := accNodeNamed(tree, "скрыт"); !accHas(n, StateOffscreen) {
		t.Errorf("узел владельца за краем без offscreen")
	}
}

// ─── Меню ────────────────────────────────────────────────────────────────────

func menuScene(t *testing.T) *PopupMenu {
	t.Helper()
	w, h := getScreenBounds()
	SetScreenBounds(1000, 800)
	t.Cleanup(func() { SetScreenBounds(w, h) })
	m := NewPopupMenu()
	m.SetItems([]MenuItem{
		{Text: "Открыть", Shortcut: "Ctrl+O"},
		{Separator: true},
		{Text: "Закрепить", Checkable: true, Checked: true},
		{Text: "Недоступный", Disabled: true},
		{Text: "Ещё", SubItems: []MenuItem{{Text: "Раз"}, {Text: "Два"}}},
	})
	return m
}

// Закрытое меню — узел «меню» без пунктов и со состоянием offscreen.
func TestPopupMenu_ClosedInTree(t *testing.T) {
	m := menuScene(t)
	root := NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 1000, 800))
	root.AddChild(m)

	tree := BuildAccessTree(root, nil)
	menu := accNodeByRole(tree, RoleMenu)
	if menu == nil {
		t.Fatal("закрытого меню нет в дереве")
	}
	if !accHas(menu, StateOffscreen) || accHas(menu, StatePopup) {
		t.Errorf("закрытое меню: %v", menu.States)
	}
	if len(menu.Children) != 0 {
		t.Errorf("у закрытого меню %d детей", len(menu.Children))
	}
}

// Открытое меню отдаёт пункты с границами, именами, состояниями и путями.
func TestPopupMenu_OpenItems(t *testing.T) {
	m := menuScene(t)
	root := NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 1000, 800))
	root.AddChild(m)
	m.Show(100, 100)
	m.OnMouseMove(120, 100+m.padY()+m.ItemHeight/2) // над «Открыть»

	tree := BuildAccessTree(root, nil)
	menu := accNodeByRole(tree, RoleMenu)
	if menu == nil || accHas(menu, StateOffscreen) || !accHas(menu, StatePopup) {
		t.Fatalf("открытое меню: %+v", menu)
	}
	if menu.Bounds.Empty() || !menu.Bounds.In(image.Rect(0, 0, 1000, 800)) {
		t.Errorf("границы меню %v", menu.Bounds)
	}

	open := accNodeNamed(menu, "Открыть")
	pinned := accNodeNamed(menu, "Закрепить")
	dis := accNodeNamed(menu, "Недоступный")
	more := accNodeNamed(menu, "Ещё")
	if open == nil || pinned == nil || dis == nil || more == nil {
		t.Fatalf("пункты не найдены: %v %v %v %v", open, pinned, dis, more)
	}
	if len(menu.Children) != 4 {
		t.Errorf("пунктов в дереве %d, ожидалось 4 (разделитель не в счёт)", len(menu.Children))
	}
	for _, n := range []*AccessNode{open, pinned, dis, more} {
		if n.Role != RoleMenuItem || !accHas(n, StatePopup) {
			t.Errorf("%q: роль %q состояния %v", n.Name, n.Role, n.States)
		}
		if !n.Bounds.In(menu.Bounds) {
			t.Errorf("%q: границы %v вне меню %v", n.Name, n.Bounds, menu.Bounds)
		}
		if n.Widget != Widget(m) {
			t.Errorf("%q: владелец не меню", n.Name)
		}
	}
	if open.ActionID != "0" || pinned.ActionID != "2" || dis.ActionID != "3" || more.ActionID != "4" {
		t.Errorf("пути пунктов: %q %q %q %q", open.ActionID, pinned.ActionID, dis.ActionID, more.ActionID)
	}
	if open.Description != "Ctrl+O" {
		t.Errorf("сочетание клавиш %q", open.Description)
	}
	if !accHas(open, StateFocused) {
		t.Errorf("пункт под курсором без focused: %v", open.States)
	}
	if accHas(pinned, StateFocused) {
		t.Error("пункт без плашки наведения считает себя в фокусе")
	}
	if !accHas(pinned, StateCheckable) || !accHas(pinned, StateChecked) {
		t.Errorf("пункт с отметкой: %v", pinned.States)
	}
	if !accHas(dis, StateDisabled) {
		t.Errorf("недоступный пункт: %v", dis.States)
	}
	if !accHas(more, StateHasPopup) || accHas(more, StateExpanded) {
		t.Errorf("пункт с подменю: %v", more.States)
	}
	// Пункты идут сверху вниз, не перекрываясь.
	if !(open.Bounds.Max.Y <= pinned.Bounds.Min.Y && pinned.Bounds.Max.Y <= dis.Bounds.Min.Y &&
		dis.Bounds.Max.Y <= more.Bounds.Min.Y) {
		t.Errorf("пункты перекрываются: %v %v %v %v", open.Bounds, pinned.Bounds, dis.Bounds, more.Bounds)
	}
}

// Подменю: пункты открытого подменю идут после пунктов меню, путь содержит
// индекс родителя, а родитель отмечен раскрытым.
func TestPopupMenu_Submenu(t *testing.T) {
	m := menuScene(t)
	root := NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 1000, 800))
	root.AddChild(m)
	m.Show(100, 100)
	if !m.AccessAction("4", AccessExpand) {
		t.Fatal("раскрытие подменю отклонено")
	}

	tree := BuildAccessTree(root, nil)
	one, two := accNodeNamed(tree, "Раз"), accNodeNamed(tree, "Два")
	if one == nil || two == nil {
		t.Fatal("пункты подменю не найдены")
	}
	if one.ActionID != "4/0" || two.ActionID != "4/1" {
		t.Errorf("пути подменю: %q %q", one.ActionID, two.ActionID)
	}
	if more := accNodeNamed(tree, "Ещё"); !accHas(more, StateExpanded) {
		t.Errorf("родитель раскрытого подменю: %v", more.States)
	}
	menu := accNodeByRole(tree, RoleMenu)
	if !one.Bounds.In(menu.Bounds) {
		t.Errorf("границы меню %v не охватывают подменю %v", menu.Bounds, one.Bounds)
	}
	if one.Bounds.Overlaps(accNodeNamed(tree, "Ещё").Bounds) && one.Bounds.Min.X < accNodeNamed(tree, "Ещё").Bounds.Min.X {
		t.Errorf("подменю %v слева от родителя %v", one.Bounds, accNodeNamed(tree, "Ещё").Bounds)
	}

	// Свернуть подменю.
	if !m.AccessAction("4", AccessCollapse) {
		t.Fatal("сворачивание подменю отклонено")
	}
	if accNodeNamed(BuildAccessTree(root, nil), "Раз") != nil {
		t.Error("пункты свёрнутого подменю остались в дереве")
	}
	if m.AccessAction("4", AccessCollapse) {
		t.Error("повторное сворачивание принято")
	}
}

// Действия над пунктами меню: подсветка, нажатие, отказ для недоступного,
// разделителя и пути, которого нет.
func TestPopupMenu_AccessAction(t *testing.T) {
	m := menuScene(t)
	var opened, selected []string
	items := m.Items()
	items[0].OnClick = func() { opened = append(opened, "Открыть") }
	items[2].OnClick = func() { opened = append(opened, "Закрепить") }
	m.SetItems(items)
	m.OnSelect = func(i int, text string) { selected = append(selected, text) }

	if m.AccessAction("0", AccessInvoke) {
		t.Fatal("закрытое меню приняло действие")
	}
	m.Show(100, 100)

	// Подсветка пункта — как стрелка клавиатуры; меню остаётся открытым.
	if !m.AccessAction("2", AccessFocus) {
		t.Fatal("подсветка отклонена")
	}
	if h := atomic.LoadInt32(&m.hoverIdx); !m.IsOpen() || h != 2 {
		t.Errorf("после фокуса: открыто=%v hover=%d", m.IsOpen(), h)
	}
	if h := atomic.LoadInt32(&m.hoverIdx); !m.AccessAction("0", AccessSelect) ||
		atomic.LoadInt32(&m.hoverIdx) != 0 {
		t.Errorf("AccessSelect: hover=%d", h)
	}
	tree := BuildAccessTree(accPanelWith(m), nil)
	if n := accNodeNamed(tree, "Открыть"); !accHas(n, StateFocused) {
		t.Error("подсвеченный пункт без focused в дереве")
	}

	// Отказы.
	for _, id := range []string{"1", "3", "9", "-1", "x", "", "0/0", "4/0"} {
		if m.AccessAction(id, AccessInvoke) {
			t.Errorf("путь %q принят", id)
		}
	}
	if len(opened) != 0 || !m.IsOpen() {
		t.Fatalf("отказы выполнили пункты: %v, открыто=%v", opened, m.IsOpen())
	}
	if m.AccessAction("0", AccessCollapse) || m.AccessAction("0", AccessExpand) {
		t.Error("раскрытие пункта без подменю принято")
	}

	// Нажатие выполняет пункт и закрывает меню.
	if !m.AccessAction("2", AccessInvoke) {
		t.Fatal("нажатие отклонено")
	}
	if len(opened) != 1 || opened[0] != "Закрепить" {
		t.Errorf("выполнено %v", opened)
	}
	if m.IsOpen() {
		t.Error("меню осталось открытым после выбора пункта")
	}
	if len(selected) != 1 || selected[0] != "Закрепить" {
		t.Errorf("OnSelect получил %v", selected)
	}
}

// accPanelWith — панель с одним ребёнком (помощник тестов).
func accPanelWith(w Widget) *Panel {
	p := NewPanel(color.RGBA{A: 255})
	p.SetBounds(image.Rect(0, 0, 1000, 800))
	p.AddChild(w)
	return p
}

// Нажатие на пункт подменю выполняет его и закрывает всю цепочку.
func TestPopupMenu_AccessActionSubmenuItem(t *testing.T) {
	m := menuScene(t)
	clicked := ""
	items := m.Items()
	items[4].SubItems[1].OnClick = func() { clicked = "Два" }
	m.SetItems(items)
	m.Show(100, 100)

	if m.AccessAction("4/1", AccessInvoke) {
		t.Fatal("путь в закрытое подменю принят")
	}
	if !m.AccessAction("4", AccessInvoke) { // пункт с подменю нажатием раскрывается
		t.Fatal("нажатие на пункт с подменю отклонено")
	}
	if c, _ := m.openChildOf(); c == nil {
		t.Fatal("подменю не раскрылось")
	}
	// Подменю открыто для пункта 4: путь через другой пункт устарел и в него
	// не попадает, даже если такой индекс в подменю есть.
	if m.AccessAction("2/1", AccessInvoke) || clicked != "" {
		t.Errorf("путь через чужой пункт принят: clicked=%q", clicked)
	}
	if !m.AccessAction("4/1", AccessInvoke) {
		t.Fatal("нажатие на пункт подменю отклонено")
	}
	if clicked != "Два" {
		t.Errorf("выполнен %q", clicked)
	}
	if m.IsOpen() {
		t.Error("цепочка меню осталась открытой")
	}
}

// Длинное меню с прокруткой: пункты за краем видимой части — offscreen.
func TestPopupMenu_ScrolledItemsOffscreen(t *testing.T) {
	w, h := getScreenBounds()
	SetScreenBounds(800, 200)
	t.Cleanup(func() { SetScreenBounds(w, h) })
	m := NewPopupMenu()
	var items []MenuItem
	for i := 0; i < 20; i++ {
		items = append(items, MenuItem{Text: "Пункт " + string(rune('A'+i))})
	}
	m.SetItems(items)
	m.Show(10, 10)
	if !m.scrollable {
		t.Skip("меню не стало прокручиваемым")
	}

	infos := m.AccessChildren()
	var visible, hidden int
	for _, in := range infos {
		if accessHasState(in.States, StateOffscreen) {
			hidden++
		} else {
			visible++
		}
	}
	if visible == 0 || hidden == 0 {
		t.Errorf("видимых %d, скрытых %d: оба должны быть", visible, hidden)
	}
	// Прокрутка меняет, кто виден.
	m.AccessAction("19", AccessFocus)
	last := m.AccessChildren()[19]
	if accessHasState(last.States, StateOffscreen) {
		t.Errorf("подсвеченный последний пункт остался за краем: %v", last.States)
	}
}

// ─── Строка меню ─────────────────────────────────────────────────────────────

func TestMenuBar_AccessTreeAndActions(t *testing.T) {
	w, h := getScreenBounds()
	SetScreenBounds(1000, 800)
	t.Cleanup(func() { SetScreenBounds(w, h) })

	mb := NewMenuBar()
	mb.SetBounds(image.Rect(0, 0, 400, 28))
	var picked []string
	mb.AddMenu("Файл", MenuItem{Text: "Создать", OnClick: func() { picked = append(picked, "Создать") }},
		MenuItem{Text: "Выход"})
	mb.AddMenu("Правка", MenuItem{Text: "Копировать"})
	mb.AddTopItem("Справка", func() { picked = append(picked, "Справка") })
	root := accPanelWith(mb)

	tree := BuildAccessTree(root, nil)
	bar := accNodeByRole(tree, RoleMenuBar)
	if bar == nil || len(bar.Children) != 3 {
		t.Fatalf("строка меню: %+v", bar)
	}
	file, edit, help := bar.Children[0], bar.Children[1], bar.Children[2]
	if file.Name != "Файл" || file.Role != RoleMenuItem || file.ActionID != "t0" {
		t.Errorf("пункт «Файл»: %+v", file.AccessInfo)
	}
	if !accHas(file, StateHasPopup) || accHas(file, StateExpanded) || accHas(file, StateFocused) {
		t.Errorf("закрытый пункт с подменю: %v", file.States)
	}
	if accHas(help, StateHasPopup) {
		t.Errorf("пункт без подменю: %v", help.States)
	}
	if file.Bounds.Empty() || file.Bounds.Max.X > edit.Bounds.Min.X {
		t.Errorf("границы пунктов %v %v", file.Bounds, edit.Bounds)
	}

	// Раскрыть «Файл» программно: появляются пункты подменю.
	if !mb.AccessAction("t0", AccessExpand) {
		t.Fatal("раскрытие «Файл» отклонено")
	}
	tree = BuildAccessTree(root, nil)
	bar = accNodeByRole(tree, RoleMenuBar)
	if !accHas(bar.Children[0], StateExpanded) || !accHas(bar.Children[0], StateFocused) {
		t.Errorf("раскрытый пункт: %v", bar.Children[0].States)
	}
	create := accNodeNamed(bar, "Создать")
	if create == nil || create.ActionID != "t0/0" || !accHas(create, StatePopup) {
		t.Fatalf("пункт подменю: %+v", create)
	}
	if create.Bounds.Min.Y < bar.Children[0].Bounds.Max.Y {
		t.Errorf("подменю %v не под строкой %v", create.Bounds, bar.Children[0].Bounds)
	}
	// Строка охватывает раскрытое подменю — иначе поиск по точке до него не дойдёт.
	if !create.Bounds.In(bar.Bounds) {
		t.Errorf("границы строки %v не охватывают подменю %v", bar.Bounds, create.Bounds)
	}

	// Пункт подменю выполняется и закрывает меню.
	if !mb.AccessAction("t0/0", AccessInvoke) {
		t.Fatal("нажатие на «Создать» отклонено")
	}
	if len(picked) != 1 || picked[0] != "Создать" {
		t.Errorf("выполнено %v", picked)
	}
	if mb.popup.IsOpen() || atomic.LoadInt32(&mb.activeIdx) != -1 {
		t.Error("меню осталось открытым")
	}
	// Путь в закрытое подменю — устаревший.
	if mb.AccessAction("t0/1", AccessInvoke) {
		t.Error("путь в закрытое подменю принят")
	}

	// Нажатие на пункт открывает и закрывает; на пункт без подменю — выполняет.
	if !mb.AccessAction("t1", AccessInvoke) || !mb.popup.IsOpen() {
		t.Error("нажатие на «Правка» не открыло меню")
	}
	if !mb.AccessAction("t1", AccessInvoke) || mb.popup.IsOpen() {
		t.Error("повторное нажатие не закрыло меню")
	}
	if !mb.AccessAction("t2", AccessInvoke) || len(picked) != 2 || picked[1] != "Справка" {
		t.Errorf("пункт без подменю: %v", picked)
	}
	// Фокус подсвечивает, не раскрывая; Collapse без раскрытия отклоняется.
	if !mb.AccessAction("t0", AccessFocus) || mb.popup.IsOpen() {
		t.Error("фокус на пункте строки открыл меню")
	}
	if mb.AccessAction("t0", AccessCollapse) || mb.AccessAction("t2", AccessExpand) {
		t.Error("нелепые Collapse/Expand приняты")
	}
	for _, bad := range []string{"", "x", "t", "t9", "t-1", "0"} {
		if mb.AccessAction(bad, AccessInvoke) {
			t.Errorf("идентификатор %q принят", bad)
		}
	}
}

// Меню рисуется оверлеем и окном прокрутки не обрезается: пункты меню, открытого
// владельцем внутри прокрутки, лежат вне её окна — и не offscreen.
func TestPopupMenu_NotClippedByScrollView(t *testing.T) {
	w, h := getScreenBounds()
	SetScreenBounds(1000, 800)
	t.Cleanup(func() { SetScreenBounds(w, h) })

	sv := NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 100, 60))
	sv.ContentHeight = 200
	m := NewPopupMenu()
	m.SetItems([]MenuItem{{Text: "Раз"}, {Text: "Два"}})
	m.SetBounds(image.Rect(0, 0, 10, 10))
	sv.AddChild(m)
	m.Show(300, 300) // далеко за окном прокрутки 100×60

	tree := BuildAccessTree(sv, nil)
	menu := accNodeByRole(tree, RoleMenu)
	if menu == nil || accHas(menu, StateOffscreen) {
		t.Fatalf("открытое меню спрятано окном прокрутки: %+v", menu)
	}
	for _, name := range []string{"Раз", "Два"} {
		n := accNodeNamed(tree, name)
		if n == nil || accHas(n, StateOffscreen) || !n.Visible.Empty() {
			t.Errorf("пункт %q обрезан окном прокрутки: %+v", name, n)
		}
	}
}
