//go:build linux && !android

// a11y_linux.go — мост доступности AT-SPI 2 (Linux) поверх своего D-Bus.
//
// Как это устроено у всех тулкитов и как сделано здесь:
//
//  1. Адрес ШИНЫ ДОСТУПНОСТИ берётся у сессионной шины:
//     org.a11y.Bus.GetAddress (это отдельный dbus-daemon, поднимаемый
//     at-spi-bus-launcher). Там же спрашивается org.a11y.Status.IsEnabled —
//     без включённой доступности мост не поднимаем (нулевая цена по умолчанию).
//  2. Приложение подключается к этой шине и ЭКСПОРТИРУЕТ дерево объектов
//     /org/a11y/atspi/accessible/<id> с интерфейсами Accessible, Component,
//     Application (+ Value/Action по роли).
//  3. Регистрация в реестре: org.a11y.atspi.Socket.Embed на объекте
//     /org/a11y/atspi/accessible/root сервиса org.a11y.atspi.Registry.
//     После этого скринридер (Orca) видит приложение и обходит его дерево.
//  4. События (фокус, смена имени/значения/состояния) шлются сигналами
//     org.a11y.atspi.Event.Object с пути изменившегося объекта.
//
// Семантика берётся из движка (engine.AccessibilityTree) — виджетам ничего
// делать не нужно. Снимок пересобирается лениво: по уведомлению об изменении
// UI, но не чаще a11yRefreshEvery.
package window

import (
	"fmt"
	"image"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

const (
	atspiRegistryName = "org.a11y.atspi.Registry"
	atspiRootPath     = "/org/a11y/atspi/accessible/root"
	atspiNullPath     = "/org/a11y/atspi/null"
	atspiPathPrefix   = "/org/a11y/atspi/accessible/"

	ifaceAccessible  = "org.a11y.atspi.Accessible"
	ifaceComponent   = "org.a11y.atspi.Component"
	ifaceApplication = "org.a11y.atspi.Application"
	ifaceValue       = "org.a11y.atspi.Value"
	ifaceAction      = "org.a11y.atspi.Action"
	ifaceSocket      = "org.a11y.atspi.Socket"
	ifaceCache       = "org.a11y.atspi.Cache"
	atspiCachePath   = "/org/a11y/atspi/cache"

	// Формат записи кэша AT-SPI: ссылка, приложение, родитель, индекс в
	// родителе, число детей, интерфейсы, имя, роль, описание, состояния.
	atspiCacheItemSig  = "((so)(so)(so)iiassusau)"
	atspiCacheItemsSig = "a" + atspiCacheItemSig
	ifaceEventObject   = "org.a11y.atspi.Event.Object"
	ifaceEventFocus    = "org.a11y.atspi.Event.Focus"
	ifaceProps         = "org.freedesktop.DBus.Properties"
	ifaceIntrospect    = "org.freedesktop.DBus.Introspectable"

	// Тип координат в Component (ATSPI_COORD_TYPE_*).
	atspiCoordScreen = 0
	atspiCoordWindow = 1
	atspiCoordParent = 2

	// Как часто пересобираем снимок семантики при активности UI.
	a11yRefreshEvery = 150 * time.Millisecond
)

// ─── Роли и состояния AT-SPI ─────────────────────────────────────────────────
//
// Числа — значения enum'ов ATSPI_ROLE_* / ATSPI_STATE_* из at-spi2-core
// (проверены по atspi-constants.h 2.52).

const (
	atspiRoleInvalid     = 0
	atspiRoleCheckBox    = 7
	atspiRoleComboBox    = 11
	atspiRoleDesktopFrm  = 14
	atspiRoleDialog      = 16
	atspiRoleFiller      = 20
	atspiRoleFrame       = 23
	atspiRoleImage       = 27
	atspiRoleLabel       = 29
	atspiRoleList        = 31
	atspiRoleMenuBar     = 34
	atspiRolePageTabList = 38
	atspiRolePanel       = 39
	atspiRoleProgressBar = 42
	atspiRolePushButton  = 43
	atspiRoleRadioButton = 44
	atspiRoleSlider      = 51
	atspiRoleSpinButton  = 52
	atspiRoleText        = 61
	atspiRoleToggleBtn   = 62
	atspiRoleUnknown     = 67
	atspiRoleApplication = 75
	atspiRoleEntry       = 79
	atspiRoleGrouping    = 99
)

const (
	atspiStateActive    = 1
	atspiStateChecked   = 4
	atspiStateEditable  = 7
	atspiStateEnabled   = 8
	atspiStateExpandble = 9
	atspiStateExpanded  = 10
	atspiStateFocusable = 11
	atspiStateFocused   = 12
	atspiStateModal     = 16
	atspiStatePressed   = 20
	atspiStateSelectbl  = 22
	atspiStateSelected  = 23
	atspiStateSensitive = 24
	atspiStateShowing   = 25
	atspiStateVisible   = 30
	atspiStateCheckable = 41
)

// atspiRoleOf переводит роль движка в роль AT-SPI.
func atspiRoleOf(r widget.AccessRole) uint32 {
	switch r {
	case widget.RoleWindow:
		return atspiRoleFrame
	case widget.RolePanel:
		return atspiRolePanel
	case widget.RoleGroup:
		return atspiRoleGrouping
	case widget.RoleButton:
		return atspiRolePushButton
	case widget.RoleCheckBox:
		return atspiRoleCheckBox
	case widget.RoleRadioButton:
		return atspiRoleRadioButton
	case widget.RoleSwitch:
		return atspiRoleToggleBtn
	case widget.RoleSlider:
		return atspiRoleSlider
	case widget.RoleProgressBar:
		return atspiRoleProgressBar
	case widget.RoleTextInput:
		return atspiRoleEntry
	case widget.RoleLabel:
		return atspiRoleLabel
	case widget.RoleComboBox:
		return atspiRoleComboBox
	case widget.RoleList:
		return atspiRoleList
	case widget.RoleTabControl:
		return atspiRolePageTabList
	case widget.RoleMenuBar:
		return atspiRoleMenuBar
	case widget.RoleSpinner:
		return atspiRoleSpinButton
	case widget.RoleImage:
		return atspiRoleImage
	}
	return atspiRoleUnknown
}

// atspiRoleName — машинное имя роли (нижний регистр, пробелы), как его отдают
// GTK/Qt: скринридеры показывают его, когда нет локализованного варианта.
func atspiRoleName(role uint32) string {
	switch role {
	case atspiRoleFrame:
		return "frame"
	case atspiRolePanel:
		return "panel"
	case atspiRoleGrouping:
		return "grouping"
	case atspiRolePushButton:
		return "push button"
	case atspiRoleCheckBox:
		return "check box"
	case atspiRoleRadioButton:
		return "radio button"
	case atspiRoleToggleBtn:
		return "toggle button"
	case atspiRoleSlider:
		return "slider"
	case atspiRoleProgressBar:
		return "progress bar"
	case atspiRoleEntry:
		return "entry"
	case atspiRoleText:
		return "text"
	case atspiRoleLabel:
		return "label"
	case atspiRoleComboBox:
		return "combo box"
	case atspiRoleList:
		return "list"
	case atspiRolePageTabList:
		return "page tab list"
	case atspiRoleMenuBar:
		return "menu bar"
	case atspiRoleSpinButton:
		return "spin button"
	case atspiRoleImage:
		return "image"
	case atspiRoleApplication:
		return "application"
	case atspiRoleDialog:
		return "dialog"
	}
	return "unknown"
}

// atspiStateSet собирает битовый набор состояний AT-SPI (два uint32:
// биты 0-31 и 32-63) по семантике узла.
func atspiStateSet(info widget.AccessInfo, focused bool) []uint32 {
	var bits [2]uint32
	set := func(b uint) { bits[b/32] |= 1 << (b % 32) }

	set(atspiStateVisible)
	set(atspiStateShowing)
	disabled := a11yHasState(info.States, widget.StateDisabled)
	if !disabled {
		set(atspiStateEnabled)
		set(atspiStateSensitive)
	}
	switch info.Role {
	case widget.RoleButton, widget.RoleCheckBox, widget.RoleRadioButton, widget.RoleSwitch,
		widget.RoleSlider, widget.RoleTextInput, widget.RoleComboBox, widget.RoleList,
		widget.RoleTabControl, widget.RoleSpinner:
		if !disabled {
			set(atspiStateFocusable)
		}
	}
	switch info.Role {
	case widget.RoleCheckBox, widget.RoleRadioButton, widget.RoleSwitch:
		set(atspiStateCheckable)
	case widget.RoleTextInput:
		set(atspiStateEditable)
	}
	if focused {
		set(atspiStateFocused)
		set(atspiStateActive)
	}
	if a11yHasState(info.States, widget.StateChecked) {
		set(atspiStateChecked)
		if info.Role == widget.RoleSwitch {
			set(atspiStatePressed)
		}
	}
	if a11yHasState(info.States, widget.StateSelected) {
		set(atspiStateSelected)
		set(atspiStateSelectbl)
	}
	if a11yHasState(info.States, widget.StateExpanded) {
		set(atspiStateExpanded)
		set(atspiStateExpandble)
	}
	if a11yHasState(info.States, widget.StateModal) {
		set(atspiStateModal)
	}
	if a11yHasState(info.States, widget.StateInactive) {
		bits[atspiStateActive/32] &^= 1 << (atspiStateActive % 32)
	}
	return bits[:]
}

// ─── Мост ────────────────────────────────────────────────────────────────────

func init() { newA11yBridge = func(win *Window) a11yBridge { return &atspiBridge{win: win} } }

// atspiView — снимок семантики вместе с УСТОЙЧИВЫМИ идентификаторами объектов.
//
// Индексы в снимке — позиции в обходе дерева: после перестройки UI один и тот
// же индекс может достаться другому виджету. Клиенты доступности (libatspi,
// а значит и Orca) КЭШИРУЮТ объекты по пути, поэтому такое переиспользование
// показало бы скринридеру чужие имя и роль. Отсюда отдельный слой: каждому
// узлу сопоставляется id, выданный по структурному ключу (путь индексов +
// роль) и живущий, пока живёт сам элемент.
type atspiView struct {
	snap  *a11ySnapshot
	ids   []int32         // индекс снимка → устойчивый id
	index map[int32]int32 // устойчивый id → индекс снимка
}

// node возвращает узел по устойчивому id.
func (v *atspiView) node(id int32) *a11yNode {
	if v == nil {
		return nil
	}
	idx, ok := v.index[id]
	if !ok {
		return nil
	}
	return v.snap.node(idx)
}

// id переводит индекс снимка в устойчивый id (-1 — индекс вне снимка).
func (v *atspiView) id(idx int32) int32 {
	if v == nil || idx < 0 || int(idx) >= len(v.ids) {
		return -1
	}
	return v.ids[idx]
}

// atspiBridge — состояние моста для одного окна.
type atspiBridge struct {
	win  *Window
	conn *dbusConn

	mu      sync.RWMutex
	view    *atspiView
	prev    *atspiView // предыдущий снимок — для деталей событий состояний
	idKeys  map[string]int32
	idNext  int32
	stamp   time.Time
	dirty   bool
	appName string // уникальное имя на шине доступности (:1.N)

	notifier uint64 // дескриптор подписки на изменения UI
	stopCh   chan struct{}
	stopOnce sync.Once
	appID    int32 // id приложения, назначенный реестром
}

// start поднимает мост: проверяет, включена ли доступность, подключается к
// шине a11y, экспортирует дерево и регистрируется в реестре.
func (b *atspiBridge) start() error {
	if !b.enabled() {
		return fmt.Errorf("a11y: доступность выключена")
	}
	addr, err := atspiBusAddress()
	if err != nil {
		return err
	}
	conn, err := dbusDial(addr)
	if err != nil {
		return fmt.Errorf("a11y: шина доступности: %w", err)
	}
	b.conn = conn
	b.appName = conn.unique
	conn.setCallHandler(b.handleCall)

	// Регистрация в реестре: наш корень «вставляется» в рабочий стол.
	reply, err := conn.call(atspiRegistryName, atspiRootPath, ifaceSocket, "Embed",
		"(so)", []any{dbusStruct{Fields: []any{b.appName, dbusObjectPath(atspiRootPath)}}})
	if err != nil {
		conn.Close()
		b.conn = nil
		return fmt.Errorf("a11y: Embed в реестр: %w", err)
	}
	_ = reply // ответ — ссылка на рабочий стол, нам она не нужна

	b.refresh(true)
	b.stopCh = make(chan struct{})
	// Подписка на изменения UI: сама по себе она только взводит флаг —
	// пересборка снимка и рассылка событий идут в своей горутине не чаще
	// a11yRefreshEvery, иначе бурная анимация утопит шину в событиях.
	b.notifier = widget.RegisterUINotifier(b.markDirty, nil)
	go b.eventLoop()
	return nil
}

// stop снимает подписки и закрывает соединение.
func (b *atspiBridge) stop() {
	b.stopOnce.Do(func() {
		if b.stopCh != nil {
			close(b.stopCh)
		}
		widget.UnregisterUINotifier(b.notifier)
		if b.conn != nil {
			// Вежливо выписываемся из реестра — иначе Orca будет держать
			// «мёртвое» приложение до таймаута.
			_ = b.conn.callNoReply(atspiRegistryName, atspiRootPath, ifaceSocket, "Unembed",
				"(so)", []any{dbusStruct{Fields: []any{b.appName, dbusObjectPath(atspiRootPath)}}})
			b.conn.Close()
		}
	})
}

func (b *atspiBridge) markDirty() {
	b.mu.Lock()
	b.dirty = true
	b.mu.Unlock()
}

// enabled решает, поднимать ли мост: явное указание приложения, затем
// переменная окружения, затем org.a11y.Status.
func (b *atspiBridge) enabled() bool {
	if v := os.Getenv("HEADLESS_GUI_A11Y"); v != "" {
		return v != "0" && strings.ToLower(v) != "false"
	}
	if b.win.a11yForce != nil {
		return *b.win.a11yForce
	}
	c, err := dbusSession()
	if err != nil {
		return false
	}
	get := func(prop string) bool {
		reply, err := c.call("org.a11y.Bus", "/org/a11y/bus", ifaceProps, "Get", "ss",
			[]any{"org.a11y.Status", prop})
		if err != nil || len(reply.Body) == 0 {
			return false
		}
		v, ok := reply.Body[0].(dbusVariant)
		if !ok {
			return false
		}
		on, _ := v.Val.(bool)
		return on
	}
	return get("IsEnabled") || get("ScreenReaderEnabled")
}

// atspiBusAddress спрашивает у сессионной шины адрес шины доступности.
func atspiBusAddress() (string, error) {
	c, err := dbusSession()
	if err != nil {
		return "", err
	}
	reply, err := c.call("org.a11y.Bus", "/org/a11y/bus", "org.a11y.Bus", "GetAddress", "", nil)
	if err != nil {
		return "", fmt.Errorf("a11y: org.a11y.Bus.GetAddress: %w", err)
	}
	addr, _ := reply.Body[0].(string)
	if addr == "" {
		return "", fmt.Errorf("a11y: пустой адрес шины доступности")
	}
	return addr, nil
}

// ─── Снимок ──────────────────────────────────────────────────────────────────

// atspiNodeKeys строит структурные ключи узлов снимка: путь индексов от корня
// плюс роль. Обход в глубину гарантирует, что родитель обработан раньше ребёнка.
func atspiNodeKeys(s *a11ySnapshot) []string {
	keys := make([]string, len(s.Nodes))
	for i := range s.Nodes {
		n := &s.Nodes[i]
		if n.Parent < 0 {
			keys[i] = "w:" + string(n.Info.Role)
			continue
		}
		keys[i] = keys[n.Parent] + "/" + strconv.Itoa(int(n.Index)) + ":" + string(n.Info.Role)
	}
	return keys
}

// assignIDs раздаёт узлам устойчивые идентификаторы. Ключи запоминаются на всё
// время жизни моста: элемент, вернувшийся на прежнее место (переключение
// вкладок, повторное открытие панели), получает СВОЙ прежний id.
func (b *atspiBridge) assignIDs(s *a11ySnapshot) *atspiView {
	v := &atspiView{snap: s, ids: make([]int32, len(s.Nodes)), index: make(map[int32]int32, len(s.Nodes))}
	for i, key := range atspiNodeKeys(s) {
		id, ok := b.idKeys[key]
		if !ok {
			id = b.idNext
			b.idNext++
			b.idKeys[key] = id
		}
		v.ids[i] = id
		v.index[id] = int32(i)
	}
	return v
}

// refresh пересобирает снимок семантики (force — не глядя на троттлинг) и
// возвращает изменения относительно прошлого снимка.
func (b *atspiBridge) refresh(force bool) a11yChanges {
	b.mu.Lock()
	if !force && !b.dirty && time.Since(b.stamp) < a11yRefreshEvery {
		b.mu.Unlock()
		return a11yChanges{FocusLost: -1, FocusGained: -1}
	}
	old := b.view
	b.mu.Unlock()

	// Дерево строит движок — под своим замком; наш держать нельзя.
	snap := b.win.accessibilitySnapshot()
	var oldSnap *a11ySnapshot
	if old != nil {
		oldSnap = old.snap
	}
	ch := a11yDiff(oldSnap, snap)

	b.mu.Lock()
	if b.idKeys == nil {
		b.idKeys = map[string]int32{}
	}
	b.prev = b.view
	b.view = b.assignIDs(snap)
	b.stamp = time.Now()
	b.dirty = false
	b.mu.Unlock()
	return ch
}

// current возвращает актуальное представление, при необходимости пересобрав его.
func (b *atspiBridge) current() *atspiView {
	b.mu.RLock()
	v, stamp, dirty := b.view, b.stamp, b.dirty
	b.mu.RUnlock()
	if v == nil || (dirty && time.Since(stamp) >= a11yRefreshEvery) {
		b.refresh(v == nil)
		b.mu.RLock()
		v = b.view
		b.mu.RUnlock()
	}
	return v
}

// eventLoop раз в a11yRefreshEvery пересобирает снимок (если UI менялся) и
// рассылает события доступности.
func (b *atspiBridge) eventLoop() {
	t := time.NewTicker(a11yRefreshEvery)
	defer t.Stop()
	for {
		select {
		case <-b.stopCh:
			return
		case <-t.C:
			b.mu.RLock()
			dirty := b.dirty
			b.mu.RUnlock()
			if !dirty {
				continue
			}
			b.emitChanges(b.refresh(true))
		}
	}
}

// ─── События ─────────────────────────────────────────────────────────────────

// atspiEvent отправляет событие доступности. Формат сигнала AT-SPI —
// "siiv(so)": подробность, два целочисленных параметра, произвольные данные и
// ссылка на ПРИЛОЖЕНИЕ-отправитель. Путь сигнала — объект, с которым событие
// произошло; интерфейс — org.a11y.atspi.Event.<Категория>.
func (b *atspiBridge) atspiEvent(id int32, iface, member, detail string, d1, d2 int32, data dbusVariant) {
	if b.conn == nil {
		return
	}
	_ = b.conn.emit(atspiPath(id), iface, member, "siiv(so)",
		[]any{detail, d1, d2, data, b.ref(atspiAppID)})
}

// emitChanges рассылает события по результатам диффа снимков. Индексы в диффе
// относятся к снимку, наружу же идут устойчивые идентификаторы объектов.
func (b *atspiBridge) emitChanges(ch a11yChanges) {
	b.mu.RLock()
	prevV, curV := b.prev, b.view
	b.mu.RUnlock()
	if curV == nil {
		return
	}
	prev, cur := (*a11ySnapshot)(nil), curV.snap
	if prevV != nil {
		prev = prevV.snap
	}
	emptyStr := dbusVariant{Sig: "s", Val: ""}

	if ch.FocusLost >= 0 && prevV != nil {
		b.atspiEvent(prevV.id(ch.FocusLost), ifaceEventObject, "StateChanged", "focused", 0, 0, emptyStr)
	}
	if ch.FocusGained >= 0 {
		gained := curV.id(ch.FocusGained)
		b.atspiEvent(gained, ifaceEventObject, "StateChanged", "focused", 1, 0, emptyStr)
		// Устаревшее, но всё ещё слушаемое некоторыми клиентами событие фокуса.
		b.atspiEvent(gained, ifaceEventFocus, "Focus", "", 0, 0, emptyStr)
	}
	if ch.Structural {
		b.emitStructural(prevV, curV)
		return
	}
	for _, idx := range ch.NameChanged {
		n := cur.node(idx)
		if n == nil {
			continue
		}
		id := curV.id(idx)
		b.atspiEvent(id, ifaceEventObject, "PropertyChange", "accessible-name", 0, 0,
			dbusVariant{Sig: "s", Val: b.nameOf(id, n)})
	}
	for _, idx := range ch.ValueChanged {
		n := cur.node(idx)
		if n == nil {
			continue
		}
		val, _, _, _ := atspiValueOf(n)
		b.atspiEvent(curV.id(idx), ifaceEventObject, "PropertyChange", "accessible-value", 0, 0,
			dbusVariant{Sig: "d", Val: val})
	}
	if prev == nil {
		return
	}
	for _, idx := range ch.StateChanged {
		id := curV.id(idx)
		oldN, newN := prev.node(idx), cur.node(idx)
		if oldN == nil || newN == nil {
			continue
		}
		for _, st := range atspiStateEvents {
			was := a11yHasState(oldN.Info.States, st.widgetState)
			now := a11yHasState(newN.Info.States, st.widgetState)
			if was == now {
				continue
			}
			on := int32(1)
			if now == st.inverted {
				on = 0
			}
			b.atspiEvent(id, ifaceEventObject, "StateChanged", st.detail, on, 0, emptyStr)
		}
	}
}

// emitStructural сообщает о перестройке дерева: исчезнувшие объекты помечаются
// defunct и удаляются из кэша клиента, появившиеся — добавляются. Благодаря
// устойчивым идентификаторам «исчез» означает именно исчез, а не «сдвинулся
// индекс», поэтому кэш клиента не разъезжается с реальностью.
func (b *atspiBridge) emitStructural(prevV, curV *atspiView) {
	emptyStr := dbusVariant{Sig: "s", Val: ""}
	if prevV != nil {
		for id, idx := range prevV.index {
			if _, alive := curV.index[id]; alive {
				continue
			}
			node := prevV.snap.node(idx)
			parent := int32(-1)
			if node != nil && node.Parent >= 0 {
				parent = prevV.id(node.Parent)
			}
			b.atspiEvent(id, ifaceEventObject, "StateChanged", "defunct", 1, 0, emptyStr)
			if b.conn != nil {
				_ = b.conn.emit(atspiCachePath, ifaceCache, "RemoveAccessible", "(so)",
					[]any{b.ref(id)})
			}
			if parent >= 0 {
				b.atspiEvent(parent, ifaceEventObject, "ChildrenChanged", "remove", 0, 0,
					dbusVariant{Sig: "(so)", Val: b.ref(id)})
			}
		}
	}
	for id, idx := range curV.index {
		if prevV != nil {
			if _, existed := prevV.index[id]; existed {
				continue
			}
		}
		if b.conn != nil {
			_ = b.conn.emit(atspiCachePath, ifaceCache, "AddAccessible", atspiCacheItemSig,
				[]any{b.cacheItem(curV, id)})
		}
		node := curV.snap.node(idx)
		if node == nil {
			continue
		}
		parent := curV.id(node.Parent)
		if node.Parent < 0 {
			parent = atspiAppID
		}
		b.atspiEvent(parent, ifaceEventObject, "ChildrenChanged", "add", node.Index, 0,
			dbusVariant{Sig: "(so)", Val: b.ref(id)})
	}
}

// atspiStateEvents — состояния движка, о смене которых сообщаем клиенту.
// inverted=true: состояние движка означает ОТСУТСТВИЕ состояния AT-SPI
// (disabled → enabled=0).
var atspiStateEvents = []struct {
	widgetState string
	detail      string
	inverted    bool
}{
	{widget.StateChecked, "checked", false},
	{widget.StateSelected, "selected", false},
	{widget.StateExpanded, "expanded", false},
	{widget.StateModal, "modal", false},
	{widget.StateDisabled, "enabled", true},
	{widget.StateDisabled, "sensitive", true},
	{widget.StateInactive, "active", true},
}

// ─── Пути объектов ───────────────────────────────────────────────────────────

// atspiAppID — идентификатор объекта ПРИЛОЖЕНИЯ. Он синтетический: в дереве
// AT-SPI приложение — отдельный узел над окном (рабочий стол → приложение →
// фрейм → виджеты), а снимок движка начинается сразу с окна. Приложение живёт
// по пути /org/a11y/atspi/accessible/root, узлы снимка — по /…/accessible/<id>.
const atspiAppID int32 = -1

// atspiPath возвращает путь объекта по id узла.
func atspiPath(id int32) string {
	if id == atspiAppID {
		return atspiRootPath
	}
	return atspiPathPrefix + strconv.Itoa(int(id))
}

// atspiParsePath разбирает путь объекта в id узла. ok=false — путь не наш.
func atspiParsePath(path string) (int32, bool) {
	if path == atspiRootPath {
		return atspiAppID, true
	}
	if !strings.HasPrefix(path, atspiPathPrefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(path, atspiPathPrefix))
	if err != nil || n < 0 {
		return 0, false
	}
	return int32(n), true
}

// ref собирает ссылку AT-SPI (имя на шине, путь объекта).
func (b *atspiBridge) ref(id int32) dbusStruct {
	return dbusStruct{Fields: []any{b.appName, dbusObjectPath(atspiPath(id))}}
}

// nullRef — «пустая» ссылка (нет родителя/объекта).
func (b *atspiBridge) nullRef() dbusStruct {
	return dbusStruct{Fields: []any{b.appName, dbusObjectPath(atspiNullPath)}}
}

// ─── Геометрия ───────────────────────────────────────────────────────────────

// extents переводит логические границы узла в координаты запрошенного типа.
// Клиенты доступности работают в ФИЗИЧЕСКИХ пикселях экрана, поэтому логические
// координаты движка умножаются на HiDPI-масштаб окна.
func (b *atspiBridge) extents(r image.Rectangle, coord uint32) (x, y, w, h int32) {
	scale := b.win.scale
	if scale <= 0 {
		scale = 1
	}
	px := func(v int) int32 { return int32(float64(v)*scale + 0.5) }
	x, y = px(r.Min.X), px(r.Min.Y)
	w, h = px(r.Dx()), px(r.Dy())
	if coord == atspiCoordScreen && b.win.native != nil {
		ox, oy := b.win.native.GetPosition()
		x += int32(ox)
		y += int32(oy)
	}
	return
}

// pointToLogical переводит точку клиента (в координатах coord) в логические
// координаты окна — для hit-теста по снимку.
func (b *atspiBridge) pointToLogical(x, y int32, coord uint32) (int, int) {
	scale := b.win.scale
	if scale <= 0 {
		scale = 1
	}
	fx, fy := float64(x), float64(y)
	if coord == atspiCoordScreen && b.win.native != nil {
		ox, oy := b.win.native.GetPosition()
		fx -= float64(ox)
		fy -= float64(oy)
	}
	return int(fx/scale + 0.5), int(fy/scale + 0.5)
}

// ─── Обработка вызовов ───────────────────────────────────────────────────────

// appNode — синтетический узел приложения: единственный ребёнок — окно
// (корень снимка). Собирается на лету, чтобы не дублировать состояние.
func (b *atspiBridge) appNode(v *atspiView) *a11yNode {
	n := &a11yNode{Parent: -1, Index: -1}
	n.Info.Name = b.win.title
	if len(v.snap.Nodes) > 0 {
		n.Children = []int32{v.id(a11yRootID)}
	}
	return n
}

// handleCall — точка входа всех вызовов от клиентов доступности.
func (b *atspiBridge) handleCall(msg *dbusMessage) *dbusReply {
	v := b.current()
	if msg.Path == atspiCachePath {
		return b.handleCache(msg, v)
	}
	id, ok := atspiParsePath(msg.Path)
	if !ok {
		return nil
	}
	// node — узел снимка; children в нём хранят индексы снимка, поэтому наружу
	// они отдаются только через v.id(...).
	var node *a11yNode
	if id == atspiAppID {
		node = b.appNode(v)
	} else if node = v.node(id); node == nil {
		return &dbusReply{ErrName: "org.freedesktop.DBus.Error.UnknownObject",
			ErrMsg: "нет объекта " + msg.Path}
	}

	switch msg.Interface {
	case ifaceIntrospect:
		if msg.Member == "Introspect" {
			return &dbusReply{Sig: "s", Body: []any{atspiIntrospectXML(id == atspiAppID)}}
		}
	case ifaceProps:
		return b.handleProps(msg, v, id, node)
	case ifaceAccessible:
		return b.handleAccessible(msg, v, id, node)
	case ifaceComponent:
		return b.handleComponent(msg, v, id, node)
	case ifaceApplication:
		return b.handleApplication(msg)
	case ifaceValue:
		return b.handleValue(msg, node)
	case ifaceAction:
		return b.handleAction(msg, node)
	}
	return nil
}

// handleCache — org.a11y.atspi.Cache: клиент забирает ВСЁ дерево одним
// вызовом вместо сотен точечных запросов. Именно так libatspi (а значит Orca)
// наполняет свой кэш при подключении к приложению.
func (b *atspiBridge) handleCache(msg *dbusMessage, v *atspiView) *dbusReply {
	switch {
	case msg.Interface == ifaceIntrospect && msg.Member == "Introspect":
		return &dbusReply{Sig: "s", Body: []any{atspiCacheXML}}
	case msg.Interface == ifaceCache && msg.Member == "GetItems":
		items := make([]any, 0, len(v.snap.Nodes)+1)
		items = append(items, b.cacheItem(v, atspiAppID))
		for i := range v.snap.Nodes {
			items = append(items, b.cacheItem(v, v.id(int32(i))))
		}
		return &dbusReply{Sig: atspiCacheItemsSig,
			Body: []any{dbusArray{ElemSig: atspiCacheItemSig, Items: items}}}
	}
	return nil
}

// cacheItem собирает запись кэша: ссылка, приложение, родитель, индекс в
// родителе, число детей, интерфейсы, имя, роль, описание, состояния.
func (b *atspiBridge) cacheItem(v *atspiView, id int32) dbusStruct {
	var node *a11yNode
	if id == atspiAppID {
		node = b.appNode(v)
	} else {
		node = v.node(id)
	}
	if node == nil {
		return dbusStruct{}
	}
	index := node.Index
	switch id {
	case atspiAppID:
		index = -1
	case v.id(a11yRootID):
		index = 0 // окно — первый ребёнок приложения
	}
	focused := id != atspiAppID && v.id(v.snap.Focus) == id
	return dbusStruct{Fields: []any{
		b.ref(id),
		b.ref(atspiAppID),
		b.parentRef(id, node),
		index,
		int32(len(node.Children)),
		b.interfacesOf(id, node),
		b.nameOf(id, node),
		b.roleOf(id, node),
		node.Info.Description,
		atspiStateSet(node.Info, focused),
	}}
}

// handleProps — org.freedesktop.DBus.Properties для наших интерфейсов.
func (b *atspiBridge) handleProps(msg *dbusMessage, v *atspiView, id int32, node *a11yNode) *dbusReply {
	prop := func(iface, name string) (dbusVariant, bool) {
		switch iface {
		case ifaceAccessible:
			switch name {
			case "Name":
				return dbusVariant{Sig: "s", Val: b.nameOf(id, node)}, true
			case "Description":
				return dbusVariant{Sig: "s", Val: node.Info.Description}, true
			case "Parent":
				return dbusVariant{Sig: "(so)", Val: b.parentRef(id, node)}, true
			case "ChildCount":
				return dbusVariant{Sig: "i", Val: int32(len(node.Children))}, true
			case "Locale":
				return dbusVariant{Sig: "s", Val: atspiLocale()}, true
			case "AccessibleId":
				if id == atspiAppID {
					return dbusVariant{Sig: "s", Val: "root"}, true
				}
				return dbusVariant{Sig: "s", Val: strconv.Itoa(int(id))}, true
			case "HelpText":
				return dbusVariant{Sig: "s", Val: node.Info.Description}, true
			}
		case ifaceApplication:
			switch name {
			case "ToolkitName":
				return dbusVariant{Sig: "s", Val: "headless-gui"}, true
			case "Version":
				return dbusVariant{Sig: "s", Val: "3"}, true
			case "AtspiVersion":
				return dbusVariant{Sig: "s", Val: "2.1"}, true
			case "Id":
				return dbusVariant{Sig: "i", Val: b.appIDValue()}, true
			}
		case ifaceValue:
			cur, min, max, step := atspiValueOf(node)
			switch name {
			case "CurrentValue":
				return dbusVariant{Sig: "d", Val: cur}, true
			case "MinimumValue":
				return dbusVariant{Sig: "d", Val: min}, true
			case "MaximumValue":
				return dbusVariant{Sig: "d", Val: max}, true
			case "MinimumIncrement":
				return dbusVariant{Sig: "d", Val: step}, true
			}
		}
		return dbusVariant{}, false
	}

	switch msg.Member {
	case "Get":
		if len(msg.Body) < 2 {
			return &dbusReply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs", ErrMsg: "Get(iface, name)"}
		}
		iface, _ := msg.Body[0].(string)
		name, _ := msg.Body[1].(string)
		if val, ok := prop(iface, name); ok {
			return &dbusReply{Sig: "v", Body: []any{val}}
		}
		return &dbusReply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs",
			ErrMsg: "нет свойства " + iface + "." + name}
	case "GetAll":
		iface, _ := msg.Body[0].(string)
		var names []string
		switch iface {
		case ifaceAccessible:
			names = []string{"Name", "Description", "Parent", "ChildCount", "Locale", "AccessibleId", "HelpText"}
		case ifaceApplication:
			names = []string{"ToolkitName", "Version", "AtspiVersion", "Id"}
		case ifaceValue:
			names = []string{"CurrentValue", "MinimumValue", "MaximumValue", "MinimumIncrement"}
		}
		all := map[string]dbusVariant{}
		for _, n := range names {
			if val, ok := prop(iface, n); ok {
				all[n] = val
			}
		}
		return &dbusReply{Sig: "a{sv}", Body: []any{all}}
	case "Set":
		// Реестр присваивает приложению номер: Application.Id — единственное
		// свойство, которое нам действительно пишут.
		if len(msg.Body) >= 3 {
			if iface, _ := msg.Body[0].(string); iface == ifaceApplication {
				if name, _ := msg.Body[1].(string); name == "Id" {
					if v, ok := msg.Body[2].(dbusVariant); ok {
						if n, ok := v.Val.(int32); ok {
							b.mu.Lock()
							b.appID = n
							b.mu.Unlock()
						}
					}
				}
			}
		}
		return &dbusReply{}
	}
	_ = v
	return nil
}

// childID переводит ребёнка узла в устойчивый id. У приложения ребёнок уже
// хранится как id, у прочих узлов — как индекс снимка.
func (b *atspiBridge) childID(v *atspiView, id int32, child int32) int32 {
	if id == atspiAppID {
		return child
	}
	return v.id(child)
}

// handleAccessible — org.a11y.atspi.Accessible.
func (b *atspiBridge) handleAccessible(msg *dbusMessage, v *atspiView, id int32, node *a11yNode) *dbusReply {
	snap := v.snap
	switch msg.Member {
	case "GetChildAtIndex":
		idx, _ := msg.Body[0].(int32)
		if idx < 0 || int(idx) >= len(node.Children) {
			return &dbusReply{Sig: "(so)", Body: []any{b.nullRef()}}
		}
		return &dbusReply{Sig: "(so)", Body: []any{b.ref(b.childID(v, id, node.Children[idx]))}}
	case "GetChildren":
		items := make([]any, 0, len(node.Children))
		for _, c := range node.Children {
			items = append(items, b.ref(b.childID(v, id, c)))
		}
		return &dbusReply{Sig: "a(so)", Body: []any{dbusArray{ElemSig: "(so)", Items: items}}}
	case "GetIndexInParent":
		if id == atspiAppID {
			return &dbusReply{Sig: "i", Body: []any{int32(-1)}}
		}
		if id == a11yRootID {
			return &dbusReply{Sig: "i", Body: []any{int32(0)}} // окно — первый ребёнок приложения
		}
		return &dbusReply{Sig: "i", Body: []any{node.Index}}
	case "GetRelationSet":
		return &dbusReply{Sig: "a(ua(so))", Body: []any{dbusArray{ElemSig: "(ua(so))"}}}
	case "GetRole":
		return &dbusReply{Sig: "u", Body: []any{b.roleOf(id, node)}}
	case "GetRoleName", "GetLocalizedRoleName":
		return &dbusReply{Sig: "s", Body: []any{atspiRoleName(b.roleOf(id, node))}}
	case "GetState":
		states := atspiStateSet(node.Info, snap.Focus == id)
		return &dbusReply{Sig: "au", Body: []any{states}}
	case "GetAttributes":
		attrs := map[string]string{"toolkit": "headless-gui"}
		return &dbusReply{Sig: "a{ss}", Body: []any{attrs}}
	case "GetApplication":
		return &dbusReply{Sig: "(so)", Body: []any{b.ref(atspiAppID)}}
	case "GetInterfaces":
		return &dbusReply{Sig: "as", Body: []any{b.interfacesOf(id, node)}}
	}
	return nil
}

// handleComponent — org.a11y.atspi.Component (геометрия и попадание точкой).
func (b *atspiBridge) handleComponent(msg *dbusMessage, v *atspiView, id int32, node *a11yNode) *dbusReply {
	arg := func(i int) int32 {
		if i < len(msg.Body) {
			if v, ok := msg.Body[i].(int32); ok {
				return v
			}
		}
		return 0
	}
	coordAt := func(i int) uint32 {
		if i < len(msg.Body) {
			if v, ok := msg.Body[i].(uint32); ok {
				return v
			}
		}
		return atspiCoordScreen
	}

	switch msg.Member {
	case "GetExtents":
		x, y, w, h := b.extents(node.Info.Bounds, coordAt(0))
		return &dbusReply{Sig: "(iiii)", Body: []any{dbusStruct{Fields: []any{x, y, w, h}}}}
	case "GetPosition":
		x, y, _, _ := b.extents(node.Info.Bounds, coordAt(0))
		return &dbusReply{Sig: "ii", Body: []any{x, y}}
	case "GetSize":
		_, _, w, h := b.extents(node.Info.Bounds, atspiCoordWindow)
		return &dbusReply{Sig: "ii", Body: []any{w, h}}
	case "Contains":
		lx, ly := b.pointToLogical(arg(0), arg(1), coordAt(2))
		return &dbusReply{Sig: "b", Body: []any{image.Pt(lx, ly).In(node.Info.Bounds)}}
	case "GetAccessibleAtPoint":
		lx, ly := b.pointToLogical(arg(0), arg(1), coordAt(2))
		hit := v.snap.hitTest(lx, ly)
		if hit < 0 {
			return &dbusReply{Sig: "(so)", Body: []any{b.nullRef()}}
		}
		return &dbusReply{Sig: "(so)", Body: []any{b.ref(v.id(hit))}}
	case "GetLayer":
		// ATSPI_LAYER_WIDGET = 3.
		return &dbusReply{Sig: "u", Body: []any{uint32(3)}}
	case "GetMDIZOrder":
		return &dbusReply{Sig: "n", Body: []any{int16(0)}}
	case "GetAlpha":
		return &dbusReply{Sig: "d", Body: []any{1.0}}
	case "GrabFocus":
		// Программная передача фокуса из скринридера пока не поддержана:
		// у движка нет публичного «сфокусировать узел по семантическому id».
		return &dbusReply{Sig: "b", Body: []any{false}}
	case "ScrollTo", "ScrollToPoint", "SetExtents", "SetPosition", "SetSize":
		return &dbusReply{Sig: "b", Body: []any{false}}
	}
	return nil
}

// handleApplication — org.a11y.atspi.Application.
func (b *atspiBridge) handleApplication(msg *dbusMessage) *dbusReply {
	switch msg.Member {
	case "GetLocale":
		return &dbusReply{Sig: "s", Body: []any{atspiLocale()}}
	case "RegisterEventListener", "DeregisterEventListener":
		// Устаревшие методы: события мы шлём безусловно.
		return &dbusReply{}
	}
	return nil
}

// handleValue — org.a11y.atspi.Value (слайдер, прогресс, спиннер).
func (b *atspiBridge) handleValue(msg *dbusMessage, node *a11yNode) *dbusReply {
	switch msg.Member {
	case "SetCurrentValue":
		return &dbusReply{Sig: "b", Body: []any{false}}
	case "GetCurrentValue":
		cur, _, _, _ := atspiValueOf(node)
		return &dbusReply{Sig: "d", Body: []any{cur}}
	}
	return nil
}

// handleAction — org.a11y.atspi.Action: одно действие «click» у кнопок.
func (b *atspiBridge) handleAction(msg *dbusMessage, node *a11yNode) *dbusReply {
	switch msg.Member {
	case "GetNActions":
		return &dbusReply{Sig: "i", Body: []any{int32(1)}}
	case "GetName", "GetLocalizedName":
		return &dbusReply{Sig: "s", Body: []any{"click"}}
	case "GetDescription":
		return &dbusReply{Sig: "s", Body: []any{""}}
	case "GetKeyBinding":
		return &dbusReply{Sig: "s", Body: []any{""}}
	case "DoAction":
		// Нажатие из скринридера — отдельная задача (нужен путь «сфокусировать
		// и активировать узел» в движке).
		return &dbusReply{Sig: "b", Body: []any{false}}
	}
	return nil
}

// ─── Вспомогательное ─────────────────────────────────────────────────────────

// parentRef — ссылка на родителя: у приложения это рабочий стол реестра,
// у окна — приложение, у прочих — узел-родитель снимка.
func (b *atspiBridge) parentRef(id int32, node *a11yNode) dbusStruct {
	switch {
	case id == atspiAppID:
		return dbusStruct{Fields: []any{atspiRegistryName, dbusObjectPath(atspiRootPath)}}
	case id == a11yRootID || node.Parent < 0:
		return b.ref(atspiAppID)
	}
	return b.ref(node.Parent)
}

// roleOf — роль узла; синтетический корень — приложение.
func (b *atspiBridge) roleOf(id int32, node *a11yNode) uint32 {
	if id == atspiAppID {
		return atspiRoleApplication
	}
	return atspiRoleOf(node.Info.Role)
}

// nameOf — имя узла; у приложения и безымянного окна — заголовок окна.
func (b *atspiBridge) nameOf(id int32, node *a11yNode) string {
	if node.Info.Name == "" && (id == atspiAppID || id == a11yRootID) {
		return b.win.title
	}
	return node.Info.Name
}

// interfacesOf перечисляет интерфейсы, которые узел реально поддерживает.
func (b *atspiBridge) interfacesOf(id int32, node *a11yNode) []string {
	if id == atspiAppID {
		return []string{ifaceAccessible, ifaceApplication}
	}
	out := []string{ifaceAccessible, ifaceComponent}
	switch node.Info.Role {
	case widget.RoleSlider, widget.RoleProgressBar, widget.RoleSpinner:
		out = append(out, ifaceValue)
	case widget.RoleButton, widget.RoleCheckBox, widget.RoleRadioButton, widget.RoleSwitch:
		out = append(out, ifaceAction)
	}
	return out
}

// appIDValue — номер приложения, присвоенный реестром.
func (b *atspiBridge) appIDValue() int32 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.appID
}

// atspiValueOf вытаскивает числовое значение узла (Value-интерфейс).
// Диапазон движок не публикует, поэтому 0..1 для прогресса и 0..100 иначе —
// значение всё равно передаётся как есть.
func atspiValueOf(node *a11yNode) (cur, min, max, step float64) {
	cur, _ = strconv.ParseFloat(node.Info.Value, 64)
	switch node.Info.Role {
	case widget.RoleProgressBar:
		return cur, 0, 1, 0.01
	}
	return cur, 0, 100, 1
}

// atspiLocale — локаль приложения в формате POSIX.
func atspiLocale() string {
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return "C"
}

// atspiIntrospectXML — описание объекта для org.freedesktop.DBus.Introspectable.
// Клиенты доступности его не требуют, но с ним объект видно в d-feet/busctl —
// это главный инструмент отладки моста.
func atspiIntrospectXML(root bool) string {
	var sb strings.Builder
	sb.WriteString(`<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN" "http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">` + "\n<node>\n")
	sb.WriteString(`<interface name="org.a11y.atspi.Accessible">
<property name="Name" type="s" access="read"/>
<property name="Description" type="s" access="read"/>
<property name="Parent" type="(so)" access="read"/>
<property name="ChildCount" type="i" access="read"/>
<method name="GetChildAtIndex"><arg direction="in" type="i"/><arg direction="out" type="(so)"/></method>
<method name="GetChildren"><arg direction="out" type="a(so)"/></method>
<method name="GetIndexInParent"><arg direction="out" type="i"/></method>
<method name="GetRelationSet"><arg direction="out" type="a(ua(so))"/></method>
<method name="GetRole"><arg direction="out" type="u"/></method>
<method name="GetRoleName"><arg direction="out" type="s"/></method>
<method name="GetLocalizedRoleName"><arg direction="out" type="s"/></method>
<method name="GetState"><arg direction="out" type="au"/></method>
<method name="GetAttributes"><arg direction="out" type="a{ss}"/></method>
<method name="GetApplication"><arg direction="out" type="(so)"/></method>
<method name="GetInterfaces"><arg direction="out" type="as"/></method>
</interface>
<interface name="org.a11y.atspi.Component">
<method name="Contains"><arg direction="in" type="i"/><arg direction="in" type="i"/><arg direction="in" type="u"/><arg direction="out" type="b"/></method>
<method name="GetAccessibleAtPoint"><arg direction="in" type="i"/><arg direction="in" type="i"/><arg direction="in" type="u"/><arg direction="out" type="(so)"/></method>
<method name="GetExtents"><arg direction="in" type="u"/><arg direction="out" type="(iiii)"/></method>
<method name="GetPosition"><arg direction="in" type="u"/><arg direction="out" type="i"/><arg direction="out" type="i"/></method>
<method name="GetSize"><arg direction="out" type="i"/><arg direction="out" type="i"/></method>
<method name="GetLayer"><arg direction="out" type="u"/></method>
<method name="GrabFocus"><arg direction="out" type="b"/></method>
</interface>
`)
	if root {
		sb.WriteString(`<interface name="org.a11y.atspi.Application">
<property name="ToolkitName" type="s" access="read"/>
<property name="Version" type="s" access="read"/>
<property name="AtspiVersion" type="s" access="read"/>
<property name="Id" type="i" access="readwrite"/>
<method name="GetLocale"><arg direction="in" type="u"/><arg direction="out" type="s"/></method>
</interface>
`)
	}
	sb.WriteString("</node>\n")
	return sb.String()
}

// atspiCacheXML — описание объекта кэша.
const atspiCacheXML = `<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN" "http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">
<node>
<interface name="org.a11y.atspi.Cache">
<method name="GetItems"><arg direction="out" type="` + atspiCacheItemsSig + `"/></method>
<signal name="AddAccessible"><arg name="nodeAdded" type="` + atspiCacheItemSig + `"/></signal>
<signal name="RemoveAccessible"><arg name="nodeRemoved" type="(so)"/></signal>
</interface>
</node>
`
