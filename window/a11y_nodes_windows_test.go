//go:build windows

package window

import (
	"image"
	"image/color"
	"testing"
	"unsafe"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// UIA: радиокнопки, прокрутка, меню, диалог и действия по узлам (v3.38).
// Провайдеры вызываются через настоящие таблицы виртуальных методов.

const (
	slotSelect         = 3 // ISelectionItemProvider::Select
	slotAddToSelection = 4
	slotRemoveFromSel  = 5
	slotIsSelected     = 6
	slotSelContainer   = 7

	slotExpand        = 3 // IExpandCollapseProvider::Expand
	slotCollapse      = 4
	slotExpandCollaps = 5 // get_ExpandCollapseState
)

// uiaScene — мост поверх произвольного корня; элементы ищутся по имени.
type uiaScene struct {
	b   *uiaBridge
	eng *engine.Engine
}

func newUIAScene(t *testing.T, root widget.Widget) *uiaScene {
	t.Helper()
	sc := &uiaScene{eng: engine.New(500, 400, 20)}
	sc.eng.SetRoot(root)
	win := New(sc.eng, "Сцена UIA")
	win.scale = 1
	sc.b = &uiaBridge{win: win, elems: map[int32]*uiaElement{}}
	sc.b.refresh(true)
	t.Cleanup(func() {
		sc.b.mu.Lock()
		for _, e := range sc.b.elems {
			e.forget()
		}
		sc.b.mu.Unlock()
	})
	return sc
}

// named возвращает COM-объект узла с заданным именем (после пересбора снимка).
func (sc *uiaScene) named(t *testing.T, name string) *uiaElement {
	t.Helper()
	sc.b.refresh(true)
	v := sc.b.current()
	for i := range v.Snap.Nodes {
		if v.Snap.Nodes[i].Info.Name == name {
			return sc.b.element(v.id(int32(i)))
		}
	}
	t.Fatalf("узел %q не найден в снимке", name)
	return nil
}

// byRole возвращает COM-объект первого узла заданной роли.
func (sc *uiaScene) byRole(t *testing.T, role widget.AccessRole) *uiaElement {
	t.Helper()
	sc.b.refresh(true)
	v := sc.b.current()
	for i := range v.Snap.Nodes {
		if v.Snap.Nodes[i].Info.Role == role {
			return sc.b.element(v.id(int32(i)))
		}
	}
	t.Fatalf("узла роли %s нет в снимке", role)
	return nil
}

func uiaPattern(t *testing.T, e *uiaElement, id int32) uintptr {
	t.Helper()
	var p uintptr
	if hr := comCall(e.simplePtr(), slotPatternProvider, uintptr(id), uintptr(unsafe.Pointer(&p))); hr != sOK {
		t.Fatalf("GetPatternProvider(%d): hr=%#x", id, hr)
	}
	return p
}

func uiaProp(e *uiaElement, prop int32) comVariant {
	var v comVariant
	comCall(e.simplePtr(), slotPropertyValue, uintptr(prop), uintptr(unsafe.Pointer(&v)))
	return v
}

func uiaBoolProp(t *testing.T, e *uiaElement, prop int32) bool {
	t.Helper()
	v := uiaProp(e, prop)
	if v.vt != vtBool {
		t.Fatalf("свойство %d: vt=%d, ожидался VT_BOOL", prop, v.vt)
	}
	return v.val[0] != 0
}

func uiaRectOf(e *uiaElement) uiaRect {
	var rc uiaRect
	comCall(e.fragmentPtr(), slotBoundingRectangle, uintptr(unsafe.Pointer(&rc)))
	return rc
}

// ─── Радиокнопки: SelectionItem ──────────────────────────────────────────────

// Радиокнопке отдан SelectionItem, а не Invoke: IsSelected читается паттерном и
// свойством, Select выбирает пункт и снимает выбор с соседа по группе.
func TestUIARadio_SelectionItem(t *testing.T) {
	root := widget.NewWindow("Радио", 500, 400)
	a := widget.NewRadioButton("Первая", "uia-radio-1")
	a.SetBounds(image.Rect(10, 50, 160, 80))
	b := widget.NewRadioButton("Вторая", "uia-radio-1")
	b.SetBounds(image.Rect(10, 90, 160, 120))
	root.AddChild(a)
	root.AddChild(b)
	a.SetSelected(true)
	sc := newUIAScene(t, root)
	ea, eb := sc.named(t, "Первая"), sc.named(t, "Вторая")

	if got := uiaPattern(t, ea, uiaPatternSelectionItem); got != ea.selItemPtr() {
		t.Fatalf("SelectionItem у радиокнопки = %#x, ожидался %#x", got, ea.selItemPtr())
	}
	if got := uiaPattern(t, ea, uiaPatternInvoke); got != 0 {
		t.Errorf("Invoke у радиокнопки = %#x: по спецификации ей положен SelectionItem", got)
	}
	if got := uiaPattern(t, ea, uiaPatternToggle); got != 0 {
		t.Errorf("Toggle у радиокнопки = %#x", got)
	}
	var out uintptr
	if hr := comCall(ea.simplePtr(), slotQueryInterface, uintptr(unsafe.Pointer(&iidSelectionItemProvider)),
		uintptr(unsafe.Pointer(&out))); hr != sOK || out != ea.selItemPtr() {
		t.Errorf("QI(SelectionItem): hr=%#x out=%#x", hr, out)
	}
	if hr := comCall(ea.simplePtr(), slotQueryInterface, uintptr(unsafe.Pointer(&iidInvokeProvider)),
		uintptr(unsafe.Pointer(&out))); hr != eNoInterface {
		t.Errorf("QI(Invoke) у радиокнопки: hr=%#x, ожидался E_NOINTERFACE", hr)
	}

	isSel := func(e *uiaElement) bool {
		t.Helper()
		sc.b.refresh(true)
		sel := uiaPattern(t, e, uiaPatternSelectionItem)
		var s int32
		if hr := comCall(sel, slotIsSelected, uintptr(unsafe.Pointer(&s))); hr != sOK {
			t.Fatalf("get_IsSelected: hr=%#x", hr)
		}
		prop := uiaBoolProp(t, e, uiaPropSelectionItemIsSel)
		if prop != (s != 0) {
			t.Errorf("свойство IsSelected=%v не совпадает с паттерном %d", prop, s)
		}
		return s != 0
	}
	if !isSel(ea) || isSel(eb) {
		t.Fatalf("исходное состояние: первая=%v вторая=%v", isSel(ea), isSel(eb))
	}

	// Select второй: выбрана вторая, первая сброшена группой.
	selB := uiaPattern(t, eb, uiaPatternSelectionItem)
	if hr := comCall(selB, slotSelect); hr != sOK {
		t.Fatalf("Select: hr=%#x", hr)
	}
	if !b.IsSelected() || a.IsSelected() {
		t.Fatalf("после Select: первая=%v вторая=%v", a.IsSelected(), b.IsSelected())
	}
	if isSel(ea) || !isSel(eb) {
		t.Error("IsSelected не отразил выбор")
	}

	// AddToSelection выбранной — ничего; невыбранной — выбирает.
	if hr := comCall(selB, slotAddToSelection); hr != sOK || !b.IsSelected() {
		t.Errorf("AddToSelection выбранной: hr=%#x", hr)
	}
	selA := uiaPattern(t, ea, uiaPatternSelectionItem)
	if hr := comCall(selA, slotAddToSelection); hr != sOK || !a.IsSelected() {
		t.Errorf("AddToSelection невыбранной: hr=%#x выбрана=%v", hr, a.IsSelected())
	}
	// Снять выбор нельзя; для невыбранной это не операция.
	if hr := comCall(selA, slotRemoveFromSel); hr != uiaEInvalidOperation {
		t.Errorf("RemoveFromSelection выбранной: hr=%#x", hr)
	}
	if hr := comCall(selB, slotRemoveFromSel); hr != sOK {
		t.Errorf("RemoveFromSelection невыбранной: hr=%#x", hr)
	}
	// Контейнера выбора нет.
	out = 1
	if hr := comCall(selA, slotSelContainer, uintptr(unsafe.Pointer(&out))); hr != sOK || out != 0 {
		t.Errorf("SelectionContainer: hr=%#x out=%#x", hr, out)
	}

	// Выключенную радиокнопку не выбрать.
	b.SetEnabled(false)
	sc.b.refresh(true)
	if hr := comCall(selB, slotSelect); hr != uiaEElementNotEnabled {
		t.Errorf("Select выключенной: hr=%#x", hr)
	}
}

// ─── Диалог ──────────────────────────────────────────────────────────────────

func TestUIADialogControlTypeAndProps(t *testing.T) {
	if got := uiaControlType(widget.RoleDialog); got != uiaCtrlWindow {
		t.Errorf("тип диалога %d, ожидалось окно %d", got, uiaCtrlWindow)
	}
	if got := uiaControlType(widget.RoleMenu); got != uiaCtrlMenu {
		t.Errorf("тип меню %d", got)
	}
	if got := uiaControlType(widget.RoleMenuItem); got != uiaCtrlMenuItem {
		t.Errorf("тип пункта меню %d", got)
	}

	root := widget.NewWindow("Окно", 500, 400)
	sc := newUIAScene(t, root)
	dlg := widget.NewDialog("Подтверждение", 300, 150)
	dlg.Description = "Удалить файл?"
	sc.eng.ShowModal(dlg)

	e := sc.named(t, "Подтверждение")
	if v := uiaProp(e, uiaPropName); variantString(t, &v) != "Подтверждение" {
		t.Error("имя диалога потеряно")
	}
	if v := uiaProp(e, uiaPropHelpText); variantString(t, &v) != "Удалить файл?" {
		t.Error("описание диалога не стало HelpText")
	}
	if v := uiaProp(e, uiaPropControlType); v.val[0] != uintptr(uiaCtrlWindow) {
		t.Errorf("ControlType диалога %d", v.val[0])
	}
	if !uiaBoolProp(t, e, uiaPropIsDialog) {
		t.Error("IsDialog диалога = false")
	}
	if uiaBoolProp(t, sc.byRole(t, widget.RoleWindow), uiaPropIsDialog) {
		t.Error("IsDialog главного окна = true")
	}
}

// ─── Прокрутка: IsOffscreen и границы ────────────────────────────────────────

func TestUIAOffscreenAndVisibleBounds(t *testing.T) {
	root := widget.NewWindow("Прокрутка", 500, 400)
	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(10, 50, 210, 150))
	sv.ContentHeight = 500
	visible := widget.NewButton("целая")
	visible.SetBounds(image.Rect(10, 60, 110, 90))
	partial := widget.NewButton("частичная")
	partial.SetBounds(image.Rect(10, 120, 110, 180))
	hidden := widget.NewButton("скрытая")
	hidden.SetBounds(image.Rect(10, 300, 110, 330))
	sv.AddChild(visible)
	sv.AddChild(partial)
	sv.AddChild(hidden)
	root.AddChild(sv)
	sc := newUIAScene(t, root)

	ev, ep, eh := sc.named(t, "целая"), sc.named(t, "частичная"), sc.named(t, "скрытая")

	if uiaBoolProp(t, ev, uiaPropIsOffscreen) || uiaBoolProp(t, ep, uiaPropIsOffscreen) {
		t.Error("видимые элементы помечены IsOffscreen")
	}
	if !uiaBoolProp(t, eh, uiaPropIsOffscreen) {
		t.Error("прокрученный за край элемент не IsOffscreen")
	}
	if rc := uiaRectOf(ev); rc.left != 10 || rc.top != 60 || rc.width != 100 || rc.height != 30 {
		t.Errorf("границы видимой кнопки: %+v", rc)
	}
	// Частично видимая: пересечение с окном прокрутки (до y=150), а не 60 px.
	if rc := uiaRectOf(ep); rc.left != 10 || rc.top != 120 || rc.width != 100 || rc.height != 30 {
		t.Errorf("границы частично видимой кнопки: %+v, ожидалось 10,120 100x30", rc)
	}
	// Скрытая: пустой прямоугольник.
	if rc := uiaRectOf(eh); rc != (uiaRect{}) {
		t.Errorf("границы скрытой кнопки: %+v, ожидался пустой", rc)
	}

	// После прокрутки видимость переезжает.
	sv.SetScrollY(250)
	sc.b.refresh(true)
	if !uiaBoolProp(t, ev, uiaPropIsOffscreen) {
		t.Error("прокрученная за верх кнопка не IsOffscreen")
	}
	if uiaBoolProp(t, eh, uiaPropIsOffscreen) {
		t.Error("выехавшая кнопка осталась IsOffscreen")
	}
	if rc := uiaRectOf(eh); rc.top != 50 || rc.height != 30 {
		t.Errorf("границы выехавшей кнопки: %+v", rc)
	}
}

// ─── Меню ────────────────────────────────────────────────────────────────────

func menuUIAScene(t *testing.T) (*uiaScene, *widget.PopupMenu, *[]string) {
	t.Helper()
	root := widget.NewWindow("Меню", 500, 400)
	m := widget.NewPopupMenu()
	var log []string
	m.SetItems([]widget.MenuItem{
		{Text: "Открыть", Shortcut: "Ctrl+O", OnClick: func() { log = append(log, "Открыть") }},
		{Text: "Закрепить", Checkable: true, Checked: true, OnClick: func() { log = append(log, "Закрепить") }},
		{Text: "Ещё", SubItems: []widget.MenuItem{{Text: "Раз", OnClick: func() { log = append(log, "Раз") }}}},
	})
	root.AddChild(m)
	sc := newUIAScene(t, root)
	return sc, m, &log
}

// Пункт меню — MenuItem с Invoke; пункт с отметкой — Toggle; пункт с подменю —
// ExpandCollapse; сочетание клавиш — AcceleratorKey.
func TestUIAMenuItems_PatternsAndProps(t *testing.T) {
	sc, m, _ := menuUIAScene(t)
	m.Show(100, 100)

	menu := sc.byRole(t, widget.RoleMenu)
	if got := uiaProp(menu, uiaPropControlType); got.val[0] != uintptr(uiaCtrlMenu) {
		t.Errorf("ControlType меню %d", got.val[0])
	}
	if uiaBoolProp(t, menu, uiaPropIsOffscreen) {
		t.Error("открытое меню IsOffscreen")
	}
	open, pin, more := sc.named(t, "Открыть"), sc.named(t, "Закрепить"), sc.named(t, "Ещё")

	if v := uiaProp(open, uiaPropControlType); v.val[0] != uintptr(uiaCtrlMenuItem) {
		t.Errorf("ControlType пункта %d", v.val[0])
	}
	if uiaPattern(t, open, uiaPatternInvoke) != open.invokePtr() {
		t.Error("у пункта меню нет Invoke")
	}
	if uiaPattern(t, open, uiaPatternToggle) != 0 || uiaPattern(t, open, uiaPatternExpandCollapse) != 0 {
		t.Error("у обычного пункта лишние паттерны")
	}
	if v := uiaProp(open, uiaPropAcceleratorKey); variantString(t, &v) != "Ctrl+O" {
		t.Error("сочетание клавиш не отдано как AcceleratorKey")
	}
	if v := uiaProp(open, uiaPropHelpText); v.vt != vtEmpty {
		t.Errorf("сочетание клавиш попало и в HelpText: vt=%d", v.vt)
	}
	if !uiaFocusable(sc.b.current().node(open.id).Info) {
		t.Error("пункт меню не фокусируемый")
	}

	// Пункт с отметкой: Toggle вместо Invoke, ToggleState On.
	tog := uiaPattern(t, pin, uiaPatternToggle)
	if tog != pin.togglePtr() || uiaPattern(t, pin, uiaPatternInvoke) != 0 {
		t.Fatal("пункт с отметкой: Toggle обязан быть, Invoke — нет")
	}
	var ts int32
	comCall(tog, slotToggleState, uintptr(unsafe.Pointer(&ts)))
	if ts != uiaToggleOn {
		t.Errorf("ToggleState пункта с отметкой = %d", ts)
	}
	if v := uiaProp(pin, uiaPropToggleState); v.vt != vtI4 || v.val[0] != uiaToggleOn {
		t.Errorf("свойство ToggleState: %+v", v)
	}

	// Пункт с подменю: ExpandCollapse, состояние Collapsed.
	ec := uiaPattern(t, more, uiaPatternExpandCollapse)
	if ec != more.expandPtr() {
		t.Fatal("у пункта с подменю нет ExpandCollapse")
	}
	var st int32 = 99
	comCall(ec, slotExpandCollaps, uintptr(unsafe.Pointer(&st)))
	if st != uiaExpandCollapsed {
		t.Errorf("исходное состояние подменю = %d", st)
	}
	var out uintptr
	if hr := comCall(more.simplePtr(), slotQueryInterface, uintptr(unsafe.Pointer(&iidExpandCollapseProvider)),
		uintptr(unsafe.Pointer(&out))); hr != sOK || out != more.expandPtr() {
		t.Errorf("QI(ExpandCollapse): hr=%#x", hr)
	}
	if hr := comCall(open.simplePtr(), slotQueryInterface, uintptr(unsafe.Pointer(&iidExpandCollapseProvider)),
		uintptr(unsafe.Pointer(&out))); hr != eNoInterface {
		t.Errorf("QI(ExpandCollapse) у обычного пункта: hr=%#x", hr)
	}
}

// Invoke по пункту меню выполняет ЕГО, а не клик в центр меню; Expand открывает
// подменю, Collapse закрывает.
func TestUIAMenuItems_ActionsReachTheItem(t *testing.T) {
	sc, m, log := menuUIAScene(t)
	m.Show(100, 100)

	// Раскрыть подменю через ExpandCollapse, прочитать состояние, нажать «Раз».
	more := sc.named(t, "Ещё")
	ec := uiaPattern(t, more, uiaPatternExpandCollapse)
	if hr := comCall(ec, slotExpand); hr != sOK {
		t.Fatalf("Expand: hr=%#x", hr)
	}
	sc.b.refresh(true)
	var st int32
	comCall(ec, slotExpandCollaps, uintptr(unsafe.Pointer(&st)))
	if st != uiaExpanded {
		t.Errorf("после Expand состояние = %d", st)
	}
	raz := sc.named(t, "Раз")
	if hr := comCall(uiaPattern(t, raz, uiaPatternInvoke), slotInvoke); hr != sOK {
		t.Fatalf("Invoke пункта подменю: hr=%#x", hr)
	}
	if len(*log) != 1 || (*log)[0] != "Раз" {
		t.Fatalf("выполнено %v, ожидалось [Раз]", *log)
	}
	if m.IsOpen() {
		t.Error("меню осталось открытым после выбора пункта")
	}

	// Новое открытие: Invoke по второму пункту — только он, и Collapse подменю.
	*log = nil
	m.Show(100, 100)
	sc.b.refresh(true)
	pin := sc.named(t, "Закрепить")
	if hr := comCall(uiaPattern(t, pin, uiaPatternToggle), slotToggle); hr != sOK {
		t.Fatalf("Toggle пункта: hr=%#x", hr)
	}
	if len(*log) != 1 || (*log)[0] != "Закрепить" {
		t.Errorf("выполнено %v, ожидалось [Закрепить]", *log)
	}

	m.Show(100, 100)
	more = sc.named(t, "Ещё")
	ec = uiaPattern(t, more, uiaPatternExpandCollapse)
	comCall(ec, slotExpand)
	if c := comCall(ec, slotCollapse); c != sOK {
		t.Fatalf("Collapse: hr=%#x", c)
	}
	sc.b.refresh(true)
	comCall(ec, slotExpandCollaps, uintptr(unsafe.Pointer(&st)))
	if st != uiaExpandCollapsed {
		t.Errorf("после Collapse состояние = %d", st)
	}
	// Collapse закрытого подменю — штатный отказ, не падение.
	if hr := comCall(ec, slotCollapse); hr != uiaEElementNotEnabled {
		t.Errorf("повторный Collapse: hr=%#x", hr)
	}
}

// SetFocus на пункте меню подсвечивает пункт; GetFocus отдаёт его же, а
// поиск по точке находит пункт под курсором (меню рисуется оверлеем).
func TestUIAMenuItems_FocusAndPoint(t *testing.T) {
	sc, m, _ := menuUIAScene(t)
	m.Show(100, 100)
	pin := sc.named(t, "Закрепить")

	if hr := comCall(pin.fragmentPtr(), slotSetFocus); hr != sOK {
		t.Fatalf("SetFocus: hr=%#x", hr)
	}
	sc.b.refresh(true)
	if f := sc.b.focusElement(); f == nil || f.id != pin.id {
		t.Fatalf("GetFocus вернул %v, ожидался подсвеченный пункт %d", f, pin.id)
	}
	if !uiaBoolProp(t, pin, uiaPropHasKeyboardFocus) {
		t.Error("HasKeyboardFocus подсвеченного пункта = false")
	}
	if uiaBoolProp(t, sc.named(t, "Открыть"), uiaPropHasKeyboardFocus) {
		t.Error("HasKeyboardFocus неподсвеченного пункта = true")
	}

	// Поиск по точке (экранные координаты = логические при scale=1, окна нет).
	c := uiaRectOf(sc.named(t, "Открыть"))
	hit := sc.b.hitTestScreen(int32(c.left+c.width/2), int32(c.top+c.height/2))
	if hit == nil || hit.id != sc.named(t, "Открыть").id {
		t.Errorf("поиск по точке центра пункта: %v", hit)
	}
}

// Закрытое меню — IsOffscreen, без границ.
func TestUIAMenu_ClosedIsOffscreen(t *testing.T) {
	sc, _, _ := menuUIAScene(t)
	menu := sc.byRole(t, widget.RoleMenu)
	if !uiaBoolProp(t, menu, uiaPropIsOffscreen) {
		t.Error("закрытое меню не IsOffscreen")
	}
	if rc := uiaRectOf(menu); rc != (uiaRect{}) {
		t.Errorf("границы закрытого меню: %+v", rc)
	}
}

// ─── Действия по узлам владельца ─────────────────────────────────────────────

// Invoke и SetFocus на узле из AccessChildren доходят до владельца с id узла.
func TestUIANodeActions_ReachOwnerHandler(t *testing.T) {
	root := widget.NewWindow("Узлы", 500, 400)
	o := &nodeOwner{reply: true}
	o.SetBounds(image.Rect(10, 50, 210, 150))
	root.AddChild(o)
	sc := newUIAScene(t, root)
	right := sc.named(t, "правая")

	if hr := comCall(right.fragmentPtr(), slotSetFocus); hr != sOK {
		t.Fatalf("SetFocus: hr=%#x", hr)
	}
	if len(o.calls) != 1 || o.calls[0] != "R:focus" {
		t.Fatalf("SetFocus: обработчик получил %v", o.calls)
	}

	// У текстовой панели нет Invoke — действие по узлу идёт через Select у
	// паттернов; проверим общий путь uiaDoNode напрямую.
	if hr := uiaDoNode(right.invokePtr(), "Invoke", widget.AccessInvoke); hr != sOK {
		t.Fatalf("uiaDoNode: hr=%#x", hr)
	}
	if o.calls[len(o.calls)-1] != "R:invoke" {
		t.Errorf("обработчик получил %v", o.calls)
	}
	// Отказ владельца — UIA_E_ELEMENTNOTENABLED, а не тихий успех.
	o.reply = false
	if hr := uiaDoNode(right.invokePtr(), "Invoke", widget.AccessInvoke); hr != uiaEElementNotEnabled {
		t.Errorf("отказ владельца: hr=%#x", hr)
	}
}

// ─── Фон: события не поднимаются без клиентов, но выбор попадает в снимок ───

func TestUIARadio_StateInSnapshot(t *testing.T) {
	root := widget.NewWindow("Радио", 500, 400)
	r := widget.NewRadioButton("Единственная", "uia-radio-2")
	r.SetBounds(image.Rect(10, 50, 160, 80))
	root.AddChild(r)
	sc := newUIAScene(t, root)
	e := sc.named(t, "Единственная")
	if uiaBoolProp(t, e, uiaPropSelectionItemIsSel) {
		t.Error("невыбранная радиокнопка IsSelected")
	}
	r.SetSelected(true)
	sc.b.refresh(true)
	if !uiaBoolProp(t, e, uiaPropSelectionItemIsSel) {
		t.Error("выбранная радиокнопка не IsSelected")
	}
	_ = color.RGBA{}
}
