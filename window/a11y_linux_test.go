//go:build linux && !android

package window

import (
	"image"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// TestATSPIRoleMapping — роли и состояния движка переводятся в коды AT-SPI.
func TestATSPIRoleMapping(t *testing.T) {
	cases := []struct {
		role widget.AccessRole
		want uint32
		name string
	}{
		{widget.RoleButton, atspiRolePushButton, "push button"},
		{widget.RoleCheckBox, atspiRoleCheckBox, "check box"},
		{widget.RoleRadioButton, atspiRoleRadioButton, "radio button"},
		{widget.RoleSwitch, atspiRoleToggleBtn, "toggle button"},
		{widget.RoleTextInput, atspiRoleEntry, "entry"},
		{widget.RoleWindow, atspiRoleFrame, "frame"},
		{widget.RoleUnknown, atspiRoleUnknown, "unknown"},
	}
	for _, c := range cases {
		if got := atspiRoleOf(c.role); got != c.want {
			t.Errorf("atspiRoleOf(%s) = %d, want %d", c.role, got, c.want)
		}
		if got := atspiRoleName(c.want); got != c.name {
			t.Errorf("atspiRoleName(%d) = %q, want %q", c.want, got, c.name)
		}
	}
}

// hasBit — установлен ли бит состояния в наборе AT-SPI (два uint32).
func hasBit(set []uint32, bit uint) bool {
	if int(bit/32) >= len(set) {
		return false
	}
	return set[bit/32]&(1<<(bit%32)) != 0
}

// TestATSPIStateSet — набор состояний собирается по семантике узла.
func TestATSPIStateSet(t *testing.T) {
	btn := widget.AccessInfo{Role: widget.RoleButton}
	set := atspiStateSet(btn, true)
	for _, b := range []uint{atspiStateVisible, atspiStateShowing, atspiStateEnabled,
		atspiStateSensitive, atspiStateFocusable, atspiStateFocused} {
		if !hasBit(set, b) {
			t.Errorf("у кнопки в фокусе нет состояния %d", b)
		}
	}
	if hasBit(set, atspiStateChecked) {
		t.Error("у кнопки не должно быть checked")
	}

	dis := widget.AccessInfo{Role: widget.RoleButton, States: []string{widget.StateDisabled}}
	set = atspiStateSet(dis, false)
	if hasBit(set, atspiStateEnabled) || hasBit(set, atspiStateSensitive) ||
		hasBit(set, atspiStateFocusable) || hasBit(set, atspiStateFocused) {
		t.Errorf("выключенная кнопка: %v", set)
	}

	cb := widget.AccessInfo{Role: widget.RoleCheckBox, States: []string{widget.StateChecked}}
	set = atspiStateSet(cb, false)
	if !hasBit(set, atspiStateCheckable) || !hasBit(set, atspiStateChecked) {
		t.Errorf("отмеченный чекбокс: %v", set)
	}

	// Состояние 41 (checkable) живёт во ВТОРОМ слове набора — проверяем, что
	// разрядность не потерялась.
	if len(set) != 2 {
		t.Fatalf("набор состояний должен быть из двух uint32, получено %d", len(set))
	}
	if set[1] == 0 {
		t.Error("второе слово набора пустое, хотя checkable=41 должен быть в нём")
	}
}

// TestATSPIPaths — пути объектов и их разбор.
func TestATSPIPaths(t *testing.T) {
	if got := atspiPath(atspiAppID); got != atspiRootPath {
		t.Errorf("путь приложения = %q", got)
	}
	if got := atspiPath(7); got != atspiPathPrefix+"7" {
		t.Errorf("путь узла = %q", got)
	}
	for _, c := range []struct {
		path string
		id   int32
		ok   bool
	}{
		{atspiRootPath, atspiAppID, true},
		{atspiPathPrefix + "0", 0, true},
		{atspiPathPrefix + "42", 42, true},
		{atspiPathPrefix + "-1", 0, false},
		{"/org/other/thing", 0, false},
		{atspiPathPrefix + "abc", 0, false},
	} {
		id, ok := atspiParsePath(c.path)
		if ok != c.ok || (ok && id != c.id) {
			t.Errorf("atspiParsePath(%q) = %d,%v; want %d,%v", c.path, id, ok, c.id, c.ok)
		}
	}
}

// TestATSPIStableIDs — устойчивые идентификаторы: пережившие перестройку узлы
// сохраняют свои id, исчезнувшие их не отдают новым элементам.
func TestATSPIStableIDs(t *testing.T) {
	b := &atspiBridge{}

	full := a11yFlatten(buildTestAccessTree("abc"))
	v1 := b.ids.assign(full)

	// Убираем ПЕРВОГО ребёнка (кнопку): индексы всех последующих узлов
	// сдвигаются, но их устойчивые id обязаны сохраниться.
	tree := buildTestAccessTree("abc")
	tree.Children = tree.Children[1:]
	short := a11yFlatten(tree)
	v2 := b.ids.assign(short)

	if v1.id(0) != v2.id(0) {
		t.Errorf("id окна изменился: %d → %d", v1.id(0), v2.id(0))
	}
	// Панель: в полном снимке индекс 2, в укороченном — 1.
	if full.Nodes[2].Info.Role != widget.RolePanel || short.Nodes[1].Info.Role != widget.RolePanel {
		t.Fatalf("тестовые деревья изменились, индексы панели неверны")
	}
	// Ключ узла включает индекс в родителе, поэтому у сдвинувшейся панели id
	// новый — но СТАРЫЙ id при этом не переиспользован.
	if v2.id(1) == v1.id(1) {
		t.Errorf("панель получила id исчезнувшей кнопки (%d) — кэш клиента разъедется", v2.id(1))
	}
	// Возврат к прежней структуре возвращает прежние идентификаторы.
	v3 := b.ids.assign(a11yFlatten(buildTestAccessTree("abc")))
	for i := range full.Nodes {
		if v3.id(int32(i)) != v1.id(int32(i)) {
			t.Errorf("узел %d: id %d, ожидался прежний %d", i, v3.id(int32(i)), v1.id(int32(i)))
		}
	}
	// Обратное соответствие id → узел.
	if n := v3.node(v3.id(1)); n == nil || n.Info.Name != "Да" {
		t.Errorf("поиск узла по id сломан: %#v", n)
	}
	if v3.node(9999) != nil {
		t.Error("неизвестный id должен давать nil")
	}
}

// TestATSPICacheItems — записи кэша: приложение + все узлы, правильные роли,
// имена, индексы и число детей.
func TestATSPICacheItems(t *testing.T) {
	win := newTestWindow(widget.NewWindow("Кэш", 200, 100))
	b := &atspiBridge{win: win, appName: ":1.7"}
	win.title = "Кэш"
	v := b.ids.assign(a11yFlatten(buildTestAccessTree("abc")))

	rep := b.handleCache(&dbusMessage{Path: atspiCachePath, Interface: ifaceCache, Member: "GetItems"}, v)
	if rep == nil || rep.Sig != atspiCacheItemsSig {
		t.Fatalf("GetItems вернул %#v", rep)
	}
	arr, ok := rep.Body[0].(dbusArray)
	if !ok || len(arr.Items) != len(v.Snap.Nodes)+1 {
		t.Fatalf("записей кэша %d, want %d", len(arr.Items), len(v.Snap.Nodes)+1)
	}
	// Кодируемость записей — гарантия, что сигнатура и типы совпали.
	e := dbusEnc{}
	if err := e.encodeAs(atspiCacheItemsSig, arr); err != nil {
		t.Fatalf("запись кэша не кодируется: %v", err)
	}

	app := arr.Items[0].(dbusStruct)
	if app.Fields[3] != int32(-1) || app.Fields[4] != int32(1) {
		t.Errorf("приложение: индекс %v, детей %v", app.Fields[3], app.Fields[4])
	}
	if app.Fields[6] != "Кэш" || app.Fields[7] != uint32(atspiRoleApplication) {
		t.Errorf("приложение: имя %v, роль %v", app.Fields[6], app.Fields[7])
	}
	frame := arr.Items[1].(dbusStruct)
	if frame.Fields[7] != uint32(atspiRoleFrame) || frame.Fields[3] != int32(0) {
		t.Errorf("окно: роль %v, индекс %v", frame.Fields[7], frame.Fields[3])
	}
	btn := arr.Items[2].(dbusStruct)
	if btn.Fields[6] != "Да" || btn.Fields[7] != uint32(atspiRolePushButton) {
		t.Errorf("кнопка: имя %v, роль %v", btn.Fields[6], btn.Fields[7])
	}
	// Родитель кнопки — окно, а приложение у всех одно.
	parent := btn.Fields[2].(dbusStruct)
	if string(parent.Fields[1].(dbusObjectPath)) != atspiPath(v.id(0)) {
		t.Errorf("родитель кнопки = %v", parent.Fields[1])
	}
	if string(btn.Fields[1].(dbusStruct).Fields[1].(dbusObjectPath)) != atspiRootPath {
		t.Errorf("приложение в записи кнопки = %v", btn.Fields[1])
	}
}

// newATSPITestBridge собирает окно с деревом виджетов и поднимает мост.
// Пропускает тест, если шина доступности или реестр недоступны.
func newATSPITestBridge(t *testing.T) (*atspiBridge, *engine.Engine, *widget.TextInput) {
	t.Helper()
	t.Setenv("HEADLESS_GUI_A11Y", "1")
	if _, err := atspiBusAddress(); err != nil {
		t.Skipf("нет шины доступности: %v", err)
	}

	eng := engine.New(400, 300, 20)
	root := widget.NewWindow("Окно доступности", 400, 300)

	btn := widget.NewButton("Сохранить")
	btn.SetBounds(image.Rect(10, 10, 120, 40))
	root.AddChild(btn)

	cb := widget.NewCheckBox("Запомнить")
	cb.SetBounds(image.Rect(10, 50, 160, 72))
	cb.SetChecked(true)
	root.AddChild(cb)

	ti := widget.NewTextInput("")
	ti.SetBounds(image.Rect(10, 80, 200, 108))
	ti.SetText("привет")
	root.AddChild(ti)

	eng.SetRoot(root)
	eng.SetFocus(ti)

	win := New(eng, "Окно доступности")
	win.scale = 1
	b := &atspiBridge{win: win}
	if err := b.start(); err != nil {
		t.Skipf("мост не поднялся (нет реестра AT-SPI?): %v", err)
	}
	t.Cleanup(b.stop)
	return b, eng, ti
}

// TestATSPIBridgeLive — живой мост на шине доступности: клиент обходит дерево,
// читает свойства, роли, состояния, геометрию и находит узел по точке.
func TestATSPIBridgeLive(t *testing.T) {
	b, _, _ := newATSPITestBridge(t)

	addr, err := atspiBusAddress()
	if err != nil {
		t.Fatal(err)
	}
	cl, err := dbusDial(addr)
	if err != nil {
		t.Fatalf("клиент шины доступности: %v", err)
	}
	defer cl.Close()

	call := func(path, iface, member, sig string, args []any) *dbusMessage {
		t.Helper()
		reply, err := cl.call(b.appName, path, iface, member, sig, args)
		if err != nil {
			t.Fatalf("%s %s.%s: %v", path, iface, member, err)
		}
		return reply
	}
	getProp := func(path, iface, name string) any {
		t.Helper()
		reply := call(path, ifaceProps, "Get", "ss", []any{iface, name})
		v, ok := reply.Body[0].(dbusVariant)
		if !ok {
			t.Fatalf("свойство %s.%s: %#v", iface, name, reply.Body[0])
		}
		return v.Val
	}

	// ── Приложение ───────────────────────────────────────────────────────────
	if got := call(atspiRootPath, ifaceAccessible, "GetRole", "", nil).Body[0]; got != uint32(atspiRoleApplication) {
		t.Errorf("роль приложения = %v", got)
	}
	if got := getProp(atspiRootPath, ifaceAccessible, "Name"); got != "Окно доступности" {
		t.Errorf("имя приложения = %v", got)
	}
	if got := getProp(atspiRootPath, ifaceAccessible, "ChildCount"); got != int32(1) {
		t.Errorf("детей у приложения = %v, want 1", got)
	}
	if got := getProp(atspiRootPath, ifaceApplication, "ToolkitName"); got != "headless-gui" {
		t.Errorf("ToolkitName = %v", got)
	}
	// Родитель приложения — рабочий стол реестра.
	parent, ok := getProp(atspiRootPath, ifaceAccessible, "Parent").(dbusStruct)
	if !ok || parent.Fields[0] != atspiRegistryName {
		t.Errorf("родитель приложения = %#v", parent)
	}

	// ── Окно (корень снимка) ─────────────────────────────────────────────────
	winPath := atspiPathPrefix + "0"
	if got := call(winPath, ifaceAccessible, "GetRole", "", nil).Body[0]; got != uint32(atspiRoleFrame) {
		t.Errorf("роль окна = %v, want frame", got)
	}
	children := call(winPath, ifaceAccessible, "GetChildren", "", nil).Body[0].([]any)
	if len(children) != 3 {
		t.Fatalf("детей окна = %d, want 3", len(children))
	}

	// ── Кнопка ───────────────────────────────────────────────────────────────
	btnRef := children[0].(dbusStruct)
	btnPath := string(btnRef.Fields[1].(dbusObjectPath))
	if got := call(btnPath, ifaceAccessible, "GetRole", "", nil).Body[0]; got != uint32(atspiRolePushButton) {
		t.Errorf("роль кнопки = %v", got)
	}
	if got := call(btnPath, ifaceAccessible, "GetRoleName", "", nil).Body[0]; got != "push button" {
		t.Errorf("имя роли кнопки = %v", got)
	}
	if got := getProp(btnPath, ifaceAccessible, "Name"); got != "Сохранить" {
		t.Errorf("имя кнопки = %v", got)
	}
	ifaces := call(btnPath, ifaceAccessible, "GetInterfaces", "", nil).Body[0].([]string)
	var hasAction bool
	for _, s := range ifaces {
		hasAction = hasAction || s == ifaceAction
	}
	if !hasAction {
		t.Errorf("кнопка без интерфейса Action: %v", ifaces)
	}
	// Геометрия: логические координаты × scale (=1) + позиция окна (native нет → 0).
	ext := call(btnPath, ifaceComponent, "GetExtents", "u", []any{uint32(atspiCoordWindow)}).Body[0].(dbusStruct)
	if ext.Fields[0] != int32(10) || ext.Fields[1] != int32(10) ||
		ext.Fields[2] != int32(110) || ext.Fields[3] != int32(30) {
		t.Errorf("границы кнопки = %v, want 10,10,110,30", ext.Fields)
	}

	// ── Чекбокс: состояние checked ───────────────────────────────────────────
	cbPath := string(children[1].(dbusStruct).Fields[1].(dbusObjectPath))
	states := call(cbPath, ifaceAccessible, "GetState", "", nil).Body[0].([]any)
	set := make([]uint32, 0, len(states))
	for _, s := range states {
		set = append(set, s.(uint32))
	}
	if !hasBit(set, atspiStateChecked) || !hasBit(set, atspiStateCheckable) {
		t.Errorf("состояния чекбокса = %v", set)
	}

	// ── Поле ввода: значение и фокус ─────────────────────────────────────────
	tiPath := string(children[2].(dbusStruct).Fields[1].(dbusObjectPath))
	states = call(tiPath, ifaceAccessible, "GetState", "", nil).Body[0].([]any)
	set = set[:0]
	for _, s := range states {
		set = append(set, s.(uint32))
	}
	if !hasBit(set, atspiStateFocused) || !hasBit(set, atspiStateEditable) {
		t.Errorf("состояния поля ввода = %v", set)
	}

	// ── Поиск узла по точке ──────────────────────────────────────────────────
	hit := call(winPath, ifaceComponent, "GetAccessibleAtPoint", "iiu",
		[]any{int32(20), int32(20), uint32(atspiCoordWindow)}).Body[0].(dbusStruct)
	if string(hit.Fields[1].(dbusObjectPath)) != btnPath {
		t.Errorf("точка (20,20) попала в %v, ожидалась кнопка %s", hit.Fields[1], btnPath)
	}
	miss := call(winPath, ifaceComponent, "GetAccessibleAtPoint", "iiu",
		[]any{int32(390), int32(290), uint32(atspiCoordWindow)}).Body[0].(dbusStruct)
	if string(miss.Fields[1].(dbusObjectPath)) != winPath {
		t.Errorf("точка в пустом месте окна должна дать само окно, а дала %v", miss.Fields[1])
	}

	// ── Кэш: клиент забирает всё дерево одним вызовом ────────────────────────
	items := call(atspiCachePath, ifaceCache, "GetItems", "", nil).Body[0].([]any)
	if len(items) != 5 { // приложение + окно + три виджета
		t.Fatalf("записей кэша %d, want 5", len(items))
	}
	var sawButton bool
	for _, it := range items {
		st := it.(dbusStruct)
		if st.Fields[6] == "Сохранить" && st.Fields[7] == uint32(atspiRolePushButton) {
			sawButton = true
		}
	}
	if !sawButton {
		t.Error("в кэше нет кнопки «Сохранить»")
	}

	// ── Несуществующий объект — ошибка, а не тишина ──────────────────────────
	if _, err := cl.call(b.appName, atspiPathPrefix+"9999", ifaceAccessible, "GetRole", "", nil); err == nil {
		t.Error("запрос несуществующего узла должен возвращать ошибку")
	}
}

// TestATSPIHoldForManualCheck — ручная проверка НАСТОЯЩИМ клиентом доступности
// (Orca, Accerciser, pyatspi): поднимает мост и держит его заданное число
// секунд, чтобы приложение можно было найти в дереве рабочего стола.
//
//	HEADLESS_GUI_A11Y_HOLD=30 go test ./window -run TestATSPIHold
//
// Без переменной окружения тест пропускается.
func TestATSPIHoldForManualCheck(t *testing.T) {
	hold := os.Getenv("HEADLESS_GUI_A11Y_HOLD")
	if hold == "" {
		t.Skip("HEADLESS_GUI_A11Y_HOLD не задана")
	}
	sec, err := strconv.Atoi(hold)
	if err != nil || sec <= 0 {
		t.Fatalf("HEADLESS_GUI_A11Y_HOLD=%q — ожидались секунды", hold)
	}
	b, eng, _ := newATSPITestBridge(t)
	t.Logf("мост поднят как %s, держим %d с", b.appName, sec)

	// Пока держим — двигаем фокус по кругу: клиент увидит поток событий.
	kids := eng.Root().Children()
	deadline := time.Now().Add(time.Duration(sec) * time.Second)
	for i := 0; time.Now().Before(deadline); i++ {
		eng.SetFocus(kids[i%len(kids)])
		time.Sleep(time.Second)
	}
}

// TestATSPIFocusEvent — смена фокуса рассылает событие StateChanged(focused).
func TestATSPIFocusEvent(t *testing.T) {
	b, eng, ti := newATSPITestBridge(t)

	addr, _ := atspiBusAddress()
	cl, err := dbusDial(addr)
	if err != nil {
		t.Fatalf("клиент: %v", err)
	}
	defer cl.Close()

	type ev struct {
		path   string
		detail string
		d1     int32
	}
	events := make(chan ev, 16)
	cl.onSignal(func(msg *dbusMessage) {
		if msg.Interface != ifaceEventObject || msg.Member != "StateChanged" || msg.Sender != b.appName {
			return
		}
		e := ev{path: msg.Path}
		e.detail, _ = msg.Body[0].(string)
		e.d1, _ = msg.Body[1].(int32)
		select {
		case events <- e:
		default:
		}
	})
	if err := cl.addMatch("type='signal',interface='" + ifaceEventObject + "'"); err != nil {
		t.Fatalf("AddMatch: %v", err)
	}

	// Переносим фокус с поля ввода на кнопку — мост должен заметить это по
	// уведомлению об изменении UI и разослать события.
	root := eng.Root()
	btn := root.Children()[0]
	eng.SetFocus(btn)
	_ = ti

	deadline := time.After(5 * time.Second)
	var gotLost, gotGained bool
	for !gotLost || !gotGained {
		select {
		case e := <-events:
			if e.detail != "focused" {
				continue
			}
			if e.d1 == 0 {
				gotLost = true
			} else {
				gotGained = true
			}
		case <-deadline:
			t.Fatalf("события фокуса не пришли (lost=%v gained=%v)", gotLost, gotGained)
		}
	}
}
