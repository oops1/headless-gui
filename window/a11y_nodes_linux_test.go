//go:build linux && !android

package window

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// AT-SPI: действия по узлам, радиокнопки, прокрутка, меню и диалог (v3.38).
// Обработчики вызовов — чистые функции над снимком (как в a11y_activate_linux_test.go),
// поэтому шина доступности не нужна; вызов идёт через handleCall — тот же вход,
// в который попадают сообщения с провода.

// newATSPIBridgeFor собирает мост БЕЗ подключения к шине поверх корня.
func newATSPIBridgeFor(root widget.Widget) (*atspiBridge, *Window) {
	win := newTestWindow(root)
	win.scale = 1
	return &atspiBridge{win: win, appName: ":1.7"}, win
}

// atspiCallNode вызывает метод интерфейса на узле по его устойчивому id —
// через handleCall, как это делает клиент.
func atspiCallNode(b *atspiBridge, id int32, iface, member string, body ...any) *dbusReply {
	return b.handleCall(&dbusMessage{Path: atspiPath(id), Interface: iface, Member: member, Body: body})
}

// idOfNamed возвращает устойчивый id узла с заданным именем в текущем виде.
func idOfNamed(t *testing.T, b *atspiBridge, name string) int32 {
	t.Helper()
	b.refresh(true)
	v := b.current()
	for i := range v.Snap.Nodes {
		if v.Snap.Nodes[i].Info.Name == name {
			return v.id(int32(i))
		}
	}
	t.Fatalf("узел %q не найден", name)
	return -1
}

func boolReply(t *testing.T, rep *dbusReply) bool {
	t.Helper()
	if rep == nil || rep.Sig != "b" || len(rep.Body) != 1 {
		t.Fatalf("ожидался ответ b, получен %#v", rep)
	}
	ok, _ := rep.Body[0].(bool)
	return ok
}

// ─── Роли и состояния ────────────────────────────────────────────────────────

func TestATSPIRoles_DialogMenuMenuItem(t *testing.T) {
	for _, c := range []struct {
		role widget.AccessRole
		want uint32
		name string
	}{
		{widget.RoleDialog, atspiRoleDialog, "dialog"},
		{widget.RoleMenu, atspiRoleMenu, "menu"},
		{widget.RoleMenuItem, atspiRoleMenuItem, "menu item"},
	} {
		if got := atspiRoleOf(c.role); got != c.want {
			t.Errorf("atspiRoleOf(%s) = %d, ожидалось %d", c.role, got, c.want)
		}
		if got := atspiRoleName(c.want); got != c.name {
			t.Errorf("atspiRoleName(%d) = %q, ожидалось %q", c.want, got, c.name)
		}
	}
	if atspiRoleName(atspiRoleCheckMenuIt) != "check menu item" {
		t.Error("имя роли check menu item")
	}
	// Пункт меню с отметкой — check menu item; без — menu item.
	b := &atspiBridge{}
	plain := &a11yNode{Info: widget.AccessInfo{Role: widget.RoleMenuItem}}
	chk := &a11yNode{Info: widget.AccessInfo{Role: widget.RoleMenuItem, States: []string{widget.StateCheckable}}}
	if got := b.roleOf(1, plain); got != atspiRoleMenuItem {
		t.Errorf("обычный пункт: %d", got)
	}
	if got := b.roleOf(1, chk); got != atspiRoleCheckMenuIt {
		t.Errorf("пункт с отметкой: %d", got)
	}
}

// Радиокнопка в AT-SPI «отмечена», а не «выбрана»: Orca читает checked.
func TestATSPIStateSet_RadioChecked(t *testing.T) {
	sel := widget.AccessInfo{Role: widget.RoleRadioButton, States: []string{widget.StateSelected}}
	set := atspiStateSet(sel, false)
	if !hasBit(set, atspiStateChecked) || !hasBit(set, atspiStateCheckable) {
		t.Errorf("выбранная радиокнопка без checked/checkable: %v", set)
	}
	if hasBit(set, atspiStateSelected) {
		t.Error("у радиокнопки не должно быть selected: так читается строка списка")
	}
	off := atspiStateSet(widget.AccessInfo{Role: widget.RoleRadioButton}, false)
	if hasBit(off, atspiStateChecked) {
		t.Error("невыбранная радиокнопка отмечена")
	}
	// Выбранная строка списка — по-прежнему selected, а не checked.
	row := atspiStateSet(widget.AccessInfo{Role: widget.RoleList, States: []string{widget.StateSelected}}, false)
	if !hasBit(row, atspiStateSelected) || hasBit(row, atspiStateChecked) {
		t.Errorf("выбранный список: %v", row)
	}
	// Событие о смене выбора радиокнопки — checked; для прочих — как раньше.
	if got := atspiStateDetail(widget.StateSelected, "selected", widget.RoleRadioButton); got != "checked" {
		t.Errorf("событие радиокнопки: %q", got)
	}
	if got := atspiStateDetail(widget.StateSelected, "selected", widget.RoleList); got != "selected" {
		t.Errorf("событие списка: %q", got)
	}
}

// Вне видимой области — не showing; пункт меню с подменю — expandable,
// has-popup и collapsed/expanded; пункт с отметкой — checkable.
func TestATSPIStateSet_OffscreenMenuItem(t *testing.T) {
	vis := atspiStateSet(widget.AccessInfo{Role: widget.RoleButton}, false)
	if !hasBit(vis, atspiStateShowing) || !hasBit(vis, atspiStateVisible) {
		t.Errorf("видимая кнопка: %v", vis)
	}
	off := atspiStateSet(widget.AccessInfo{Role: widget.RoleButton, States: []string{widget.StateOffscreen}}, false)
	if hasBit(off, atspiStateShowing) {
		t.Error("прокрученная за край кнопка showing")
	}
	if !hasBit(off, atspiStateVisible) {
		t.Error("прокрученная за край кнопка перестала быть visible")
	}

	sub := atspiStateSet(widget.AccessInfo{Role: widget.RoleMenuItem,
		States: []string{widget.StateHasPopup}}, false)
	if !hasBit(sub, atspiStateHasPopup) || !hasBit(sub, atspiStateExpandble) ||
		!hasBit(sub, atspiStateCollapsed) || hasBit(sub, atspiStateExpanded) || !hasBit(sub, atspiStateFocusable) {
		t.Errorf("закрытый пункт с подменю: %v", sub)
	}
	exp := atspiStateSet(widget.AccessInfo{Role: widget.RoleMenuItem,
		States: []string{widget.StateHasPopup, widget.StateExpanded}}, false)
	if !hasBit(exp, atspiStateExpanded) || hasBit(exp, atspiStateCollapsed) {
		t.Errorf("раскрытый пункт: %v", exp)
	}
	chk := atspiStateSet(widget.AccessInfo{Role: widget.RoleMenuItem,
		States: []string{widget.StateCheckable, widget.StateChecked}}, false)
	if !hasBit(chk, atspiStateCheckable) || !hasBit(chk, atspiStateChecked) {
		t.Errorf("пункт с отметкой: %v", chk)
	}
	if hasBit(atspiStateSet(widget.AccessInfo{Role: widget.RoleMenuItem}, false), atspiStateCheckable) {
		t.Error("обычный пункт checkable")
	}
}

// ─── Действия по узлам ───────────────────────────────────────────────────────

// DoAction и GrabFocus на узле из AccessChildren доходят до владельца с id узла.
func TestATSPINodeActions_ReachOwner(t *testing.T) {
	root := widget.NewWindow("Узлы", 400, 300)
	o := &nodeOwner{reply: true}
	o.SetBounds(image.Rect(10, 50, 210, 150))
	root.AddChild(o)
	b, _ := newATSPIBridgeFor(root)
	right := idOfNamed(t, b, "правая")

	// У узла владельца есть Action.
	ifaces := atspiCallNode(b, right, ifaceAccessible, "GetInterfaces").Body[0].([]string)
	has := false
	for _, s := range ifaces {
		has = has || s == ifaceAction
	}
	if !has {
		t.Errorf("у узла владельца нет Action: %v", ifaces)
	}

	if !boolReply(t, atspiCallNode(b, right, ifaceAction, "DoAction", int32(0))) {
		t.Fatal("DoAction(0) отвергнут")
	}
	if !boolReply(t, atspiCallNode(b, right, ifaceComponent, "GrabFocus")) {
		t.Fatal("GrabFocus отвергнут")
	}
	if len(o.calls) != 2 || o.calls[0] != "R:invoke" || o.calls[1] != "R:focus" {
		t.Fatalf("обработчик владельца получил %v, ожидалось [R:invoke R:focus]", o.calls)
	}

	// Ответ владельца окончателен.
	o.reply = false
	if boolReply(t, atspiCallNode(b, right, ifaceAction, "DoAction", int32(0))) {
		t.Error("отказ владельца превращён в успех")
	}
	// Второго действия у узла без подменю нет.
	if boolReply(t, atspiCallNode(b, right, ifaceAction, "DoAction", int32(1))) {
		t.Error("DoAction(1) принят у узла без подменю")
	}
	if n := len(o.calls); n != 3 {
		t.Errorf("лишние вызовы владельца: %v", o.calls)
	}
}

// Меню: пункты — узлы menu item с Action; нажатие выполняет пункт, а
// «expand or contract» раскрывает и закрывает подменю.
func TestATSPIMenu_ActionsOnItems(t *testing.T) {
	root := widget.NewWindow("Меню", 500, 400)
	m := widget.NewPopupMenu()
	var log []string
	m.SetItems([]widget.MenuItem{
		{Text: "Открыть", OnClick: func() { log = append(log, "Открыть") }},
		{Text: "Ещё", SubItems: []widget.MenuItem{{Text: "Раз", OnClick: func() { log = append(log, "Раз") }}}},
	})
	root.AddChild(m)
	b, _ := newATSPIBridgeFor(root)
	m.Show(100, 100)

	open := idOfNamed(t, b, "Открыть")
	more := idOfNamed(t, b, "Ещё")
	if role := atspiCallNode(b, open, ifaceAccessible, "GetRole").Body[0]; role != uint32(atspiRoleMenuItem) {
		t.Errorf("роль пункта %v", role)
	}
	if layer := atspiCallNode(b, open, ifaceComponent, "GetLayer").Body[0]; layer != uint32(atspiLayerPopup) {
		t.Errorf("слой пункта меню %v, ожидался popup", layer)
	}
	if layer := atspiCallNode(b, 0, ifaceComponent, "GetLayer").Body[0]; layer != uint32(atspiLayerWidget) {
		t.Errorf("слой окна %v", layer)
	}

	// Число действий: у пункта с подменю два.
	nact := func(id int32) int32 {
		return atspiCallNode(b, id, ifaceProps, "Get", ifaceAction, "NActions").Body[0].(dbusVariant).Val.(int32)
	}
	if nact(open) != 1 || nact(more) != 2 {
		t.Errorf("NActions: %d и %d, ожидалось 1 и 2", nact(open), nact(more))
	}
	if n := atspiCallNode(b, more, ifaceAction, "GetName", int32(1)).Body[0]; n != "expand or contract" {
		t.Errorf("второе действие: %v", n)
	}

	// Раскрыть, увидеть пункт подменю, закрыть.
	if !boolReply(t, atspiCallNode(b, more, ifaceAction, "DoAction", int32(1))) {
		t.Fatal("раскрытие подменю отвергнуто")
	}
	raz := idOfNamed(t, b, "Раз")
	if raz < 0 {
		t.Fatal("пункт подменю не появился")
	}
	if !boolReply(t, atspiCallNode(b, more, ifaceAction, "DoAction", int32(1))) {
		t.Fatal("закрытие подменю отвергнуто")
	}
	b.refresh(true)
	for i := range b.current().Snap.Nodes {
		if b.current().Snap.Nodes[i].Info.Name == "Раз" {
			t.Error("пункт закрытого подменю остался в дереве")
		}
	}

	// Нажатие выполняет ИМЕННО этот пункт и закрывает меню.
	if !boolReply(t, atspiCallNode(b, open, ifaceAction, "DoAction", int32(0))) {
		t.Fatal("нажатие на пункт отвергнуто")
	}
	if len(log) != 1 || log[0] != "Открыть" {
		t.Errorf("выполнено %v, ожидалось [Открыть]", log)
	}
	if m.IsOpen() {
		t.Error("меню осталось открытым")
	}

	// GrabFocus на пункте подсвечивает его, не закрывая меню.
	m.Show(100, 100)
	open = idOfNamed(t, b, "Открыть")
	if !boolReply(t, atspiCallNode(b, open, ifaceComponent, "GrabFocus")) || !m.IsOpen() {
		t.Error("GrabFocus на пункте меню")
	}
	b.refresh(true)
	if b.current().Snap.Focus < 0 || b.current().id(b.current().Snap.Focus) != open {
		t.Errorf("фокус снимка не на подсвеченном пункте: %d", b.current().Snap.Focus)
	}
}

// GetAccessibleAtPoint находит пункт открытого меню, выступающего за границы
// владельца; Contains не считает точку в скрытой части элемента его точкой.
func TestATSPIComponent_PointInPopupAndVisible(t *testing.T) {
	root := widget.NewWindow("Меню", 500, 400)
	m := widget.NewPopupMenu()
	m.SetItems([]widget.MenuItem{{Text: "Первый"}, {Text: "Второй"}})
	root.AddChild(m)

	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(10, 200, 210, 300))
	sv.ContentHeight = 500
	partial := widget.NewButton("частичная")
	partial.SetBounds(image.Rect(10, 270, 110, 330)) // экранные 270..330, окно кончается на 300
	sv.AddChild(partial)
	root.AddChild(sv)

	b, _ := newATSPIBridgeFor(root)
	m.Show(100, 60)
	second := idOfNamed(t, b, "Второй")
	node := b.current().node(second)
	c := node.Info.Bounds
	rep := atspiCallNode(b, 0, ifaceComponent, "GetAccessibleAtPoint",
		int32(c.Min.X+c.Dx()/2), int32(c.Min.Y+c.Dy()/2), uint32(atspiCoordWindow))
	ref := rep.Body[0].(dbusStruct)
	if got := string(ref.Fields[1].(dbusObjectPath)); got != atspiPath(second) {
		t.Errorf("под курсором %s, ожидался пункт меню %s", got, atspiPath(second))
	}

	pid := idOfNamed(t, b, "частичная")
	in := func(x, y int32) bool {
		return boolReply(t, atspiCallNode(b, pid, ifaceComponent, "Contains", x, y, uint32(atspiCoordWindow)))
	}
	if !in(50, 285) {
		t.Error("точка в видимой части кнопки не принята")
	}
	if in(50, 315) {
		t.Error("точка в части кнопки за окном прокрутки принята")
	}
}

// Диалог в кэше AT-SPI: роль dialog, имя и описание.
func TestATSPIDialog_NamedAndDescribed(t *testing.T) {
	root := widget.NewWindow("Окно", 400, 300)
	b, win := newATSPIBridgeFor(root)
	dlg := widget.NewDialog("Подтверждение", 300, 150)
	dlg.Description = "Удалить файл?"
	showDialogForTest(t, win, dlg)

	id := idOfNamed(t, b, "Подтверждение")
	v := b.current()
	node := v.node(id)
	if node == nil {
		t.Fatal("узел диалога не найден")
	}
	if got := b.roleOf(id, node); got != atspiRoleDialog {
		t.Errorf("роль диалога %d, ожидалась dialog (%d)", got, atspiRoleDialog)
	}
	item := b.cacheItem(v, id)
	if item.Fields[6] != "Подтверждение" || item.Fields[8] != "Удалить файл?" {
		t.Errorf("запись кэша: имя %v, описание %v", item.Fields[6], item.Fields[8])
	}
	set := item.Fields[9].([]uint32)
	if !hasBit(set, atspiStateModal) {
		t.Errorf("диалог не modal: %v", set)
	}
}
