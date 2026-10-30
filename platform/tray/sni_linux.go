//go:build linux && !android

// sni_linux.go — иконка в системном трее на Linux (X11 и Wayland) по
// протоколу StatusNotifierItem (KDE/freedesktop) + меню com.canonical.dbusmenu.
//
// Старый XEmbed-трей (_NET_SYSTEM_TRAY) умер вместе с GNOME 2, современные
// панели (KDE Plasma, Waybar, xfce4-statusnotifier, sway/ironbar, Cinnamon,
// расширения GNOME) забирают иконку по D-Bus. Схема такая:
//
//	мы  ──RegisterStatusNotifierItem(имя)──▶ org.kde.StatusNotifierWatcher
//	панель ──Properties.GetAll / Activate──▶ наш /StatusNotifierItem
//	панель ──GetLayout / Event───────────▶ наш /MenuBar (com.canonical.dbusmenu)
//
// Меню на Linux рисует САМА панель (мы отдаём только дерево), поэтому наше
// base.PopupMenu по правому клику здесь не показывается — см. интерфейс
// nativeTrayMenu в window/core/tray.go.
//
// ОГРАНИЧЕНИЯ:
//   - GNOME из коробки StatusNotifierWatcher НЕ поднимает: без расширения
//     AppIndicator/AppIndicatorSupport иконки не будет (SetTrayIcon вернёт
//     errNoTrayWatcher). Это свойство среды, а не нашего кода.
//   - Watcher, появившийся ПОЗЖЕ первого SetTrayIcon, сам нас не подхватит:
//     нужен повторный SetTrayIcon (за NameOwnerChanged не следим).
//   - Позиции иконки протокол не сообщает: CursorScreenPos отдаёт (0,0),
//     меню позиционирует панель.
//   - Двойного клика в протоколе нет: Activate — это ОДИНАРНЫЙ левый клик,
//     поэтому дефолт «двойной левый клик восстанавливает окно» (window/core/tray.go) на
//     Linux не срабатывает. Нужен возврат из трея — задайте SetOnTrayClick
//     или пункт в SetTrayMenu.
//   - Wayland не даёт скрыть окно (нет аналога UnmapWindow), HideToTray там
//     no-op; на X11 — честный UnmapWindow.
//
// Соединение с шиной у трея ОТДЕЛЬНОЕ (свой dbus.Dial): обработчик входящих
// вызовов на соединении ровно один, а общее (dbus.Session) уже занято
// уведомлениями и может понадобиться мосту AT-SPI.

package tray

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	xdraw "golang.org/x/image/draw"

	"github.com/oops1/headless-gui/v3/platform/dbus"
	"github.com/oops1/headless-gui/v3/widget/base"
)

const (
	// StatusNotifierWatcher — служба панели, ведущая список иконок.
	sniWatcherName  = "org.kde.StatusNotifierWatcher"
	sniWatcherPath  = "/StatusNotifierWatcher"
	sniWatcherIface = "org.kde.StatusNotifierWatcher"

	// Наш объект-иконка.
	sniItemPath  = "/StatusNotifierItem"
	sniItemIface = "org.kde.StatusNotifierItem"

	// Наше меню.
	dbusmenuPath  = "/MenuBar"
	dbusmenuIface = "com.canonical.dbusmenu"

	// Сигнатура узла дерева меню: id, свойства, дети-варианты.
	dbusmenuNodeSig = "(ia{sv}av)"

	// sniMaxIcon — потолок стороны пиксельной иконки: панель всё равно
	// масштабирует её под свою высоту, гонять мегабайты по шине незачем.
	sniMaxIcon = 128
)

// errNoTrayWatcher — на сессионной шине нет службы трея.
var errNoTrayWatcher = errors.New(
	"window: на сессионной шине нет системного трея (org.kde.StatusNotifierWatcher); " +
		"в GNOME нужно расширение AppIndicator")

// sniItemSeq нумерует иконки внутри процесса: имя на шине по спецификации —
// org.kde.StatusNotifierItem-<pid>-<n>, и два окна одного процесса не должны
// драться за одно и то же имя.
var sniItemSeq atomic.Uint32

// SNI — состояние иконки трея одного окна. Встраивается в X11Window и
// WaylandWindow (как Notifier для уведомлений): его методы становятся их
// методами, и окно превращается в trayHost (window/core/tray.go).
//
// Потокобезопасность: SetTrayIcon/RemoveTrayIcon/… зовутся с UI-потока окна,
// входящие вызовы D-Bus обрабатываются в горутинах соединения — всё состояние
// под trayMu. Пользовательские колбэки вызываются вне блокировки, в отдельной
// горутине (иначе долгий обработчик задержал бы ответ панели).
type SNI struct {
	trayMu sync.Mutex

	conn   *dbus.Conn // отдельное соединение с шиной (nil — трея нет)
	name   string     // занятое имя org.kde.StatusNotifierItem-<pid>-<n>
	active bool       // иконка зарегистрирована у Watcher
	status string     // Active / Passive

	iconW, iconH int32  // размер пиксельной иконки
	iconARGB     []byte // пиксели ARGB32 (сетевой порядок байт)
	tooltip      string

	menu     *base.PopupMenu // дерево для dbusmenu (nil — меню нет)
	revision uint32          // ревизия раскладки меню (растёт при смене)

	onTrayClick func(button int, doubleClick bool)
	// menuRun — куда отдавать выбор пункта меню: очередь движка окна
	// (SetTrayMenuRunner). nil — отдельная горутина, как до очереди.
	menuRun func(fn func())
}

// ─── trayHost: иконка ────────────────────────────────────────────────────────

// SetTrayIcon показывает/обновляет иконку в трее. Первый вызов поднимает
// соединение и регистрируется у Watcher; последующие — обновляют пиксели и
// подсказку и будят панель сигналами NewIcon/NewTitle/NewToolTip.
func (t *SNI) SetTrayIcon(icon image.Image, tooltip string) error {
	w, h, argb := sniIconPixmap(icon)

	t.trayMu.Lock()
	t.iconW, t.iconH, t.iconARGB = w, h, argb
	t.tooltip = tooltip
	t.status = "Active"
	c, active := t.conn, t.active
	t.trayMu.Unlock()

	if !active {
		return t.register()
	}
	_ = c.Emit(sniItemPath, sniItemIface, "NewIcon", "", nil)
	_ = c.Emit(sniItemPath, sniItemIface, "NewTitle", "", nil)
	_ = c.Emit(sniItemPath, sniItemIface, "NewToolTip", "", nil)
	return nil
}

// register подключается к шине, экспортирует /StatusNotifierItem и /MenuBar и
// просит Watcher взять иконку. Ошибка — трея в системе нет.
func (t *SNI) register() error {
	c, err := dbus.Dial(dbus.SessionAddress())
	if err != nil {
		return fmt.Errorf("window: трей недоступен: %w", err)
	}
	if !c.NameHasOwner(sniWatcherName) {
		c.Close()
		return errNoTrayWatcher
	}
	name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-%d", os.Getpid(), sniItemSeq.Add(1))
	if err := c.RequestName(name, 0x4 /*DO_NOT_QUEUE*/); err != nil {
		c.Close()
		return fmt.Errorf("window: имя иконки трея: %w", err)
	}
	// Обработчик ставим ДО регистрации: панель читает свойства сразу.
	c.SetCallHandler(t.handleCall)
	if _, err := c.Call(sniWatcherName, sniWatcherPath, sniWatcherIface,
		"RegisterStatusNotifierItem", "s", []any{name}); err != nil {
		c.Close()
		return fmt.Errorf("window: RegisterStatusNotifierItem: %w", err)
	}

	t.trayMu.Lock()
	t.conn, t.name, t.active = c, name, true
	t.trayMu.Unlock()
	return nil
}

// RemoveTrayIcon убирает иконку: объявляем Passive (панель прячет её сразу),
// отдаём имя и закрываем соединение. Повторный SetTrayIcon поднимет всё заново.
func (t *SNI) RemoveTrayIcon() {
	t.trayMu.Lock()
	c, active, name := t.conn, t.active, t.name
	t.conn, t.active, t.name = nil, false, ""
	t.status = "Passive"
	t.trayMu.Unlock()

	if !active || c == nil {
		return
	}
	_ = c.Emit(sniItemPath, sniItemIface, "NewStatus", "s", []any{"Passive"})
	_, _ = c.Call("org.freedesktop.DBus", "/org/freedesktop/DBus",
		"org.freedesktop.DBus", "ReleaseName", "s", []any{name})
	c.Close()
}

// SetTrayClickHandler регистрирует диспетчер кликов по иконке.
func (t *SNI) SetTrayClickHandler(fn func(button int, doubleClick bool)) {
	t.trayMu.Lock()
	t.onTrayClick = fn
	t.trayMu.Unlock()
}

// CursorScreenPos: у StatusNotifierItem нет понятия «позиция курсора» —
// меню размещает панель. Возвращаем (0,0), нашим меню мы здесь не пользуемся.
func (t *SNI) CursorScreenPos() (int, int) { return 0, 0 }

// ─── nativeTrayMenu: меню отдаём панели ──────────────────────────────────────

// SetTrayMenuNative принимает трей-меню для показа СРЕДОЙ (com.canonical.dbusmenu)
// и всегда возвращает true: на Linux панель рисует меню сама, показывать своё
// base.PopupMenu поверх неё не нужно и нечем (позиции иконки мы не знаем).
func (t *SNI) SetTrayMenuNative(menu *base.PopupMenu) bool {
	t.trayMu.Lock()
	t.menu = menu
	t.revision++
	rev, c := t.revision, t.conn
	t.trayMu.Unlock()

	if c != nil {
		// Панель перечитает дерево (LayoutUpdated) и путь к меню (NewMenu).
		_ = c.Emit(dbusmenuPath, dbusmenuIface, "LayoutUpdated", "ui", []any{rev, int32(0)})
		_ = c.Emit(sniItemPath, sniItemIface, "NewMenu", "", nil)
	}
	return true
}

// ─── Входящие вызовы ─────────────────────────────────────────────────────────

// handleCall — мультиплексор входящих вызовов соединения трея по пути объекта.
func (t *SNI) handleCall(msg *dbus.Message) *dbus.Reply {
	switch msg.Path {
	case sniItemPath:
		return t.handleItem(msg)
	case dbusmenuPath:
		return t.handleMenu(msg)
	}
	return nil
}

// handleItem — org.kde.StatusNotifierItem и его свойства.
func (t *SNI) handleItem(msg *dbus.Message) *dbus.Reply {
	switch msg.Interface {
	case dbus.IfaceIntrospectable:
		if msg.Member == "Introspect" {
			return &dbus.Reply{Sig: "s", Body: []any{sniItemIntrospectXML}}
		}
	case dbus.IfaceProperties:
		return dbusPropsReply(msg, sniItemIface, sniItemPropNames, t.itemProp)
	case sniItemIface:
		switch msg.Member {
		case "Activate": // левый клик
			t.dispatchClick(0)
			return &dbus.Reply{}
		case "SecondaryActivate": // средняя кнопка
			t.dispatchClick(2)
			return &dbus.Reply{}
		case "ContextMenu":
			// Правый клик: меню показывает панель через dbusmenu, нам делать
			// нечего. Отвечаем пустым ответом, иначе панель ждёт таймаут.
			return &dbus.Reply{}
		case "Scroll":
			return &dbus.Reply{}
		}
	}
	return nil
}

// sniItemPropNames — состав Properties.GetAll для org.kde.StatusNotifierItem.
var sniItemPropNames = []string{
	"Category", "Id", "Title", "Status", "WindowId",
	"IconName", "IconPixmap", "OverlayIconName", "OverlayIconPixmap",
	"AttentionIconName", "AttentionIconPixmap", "AttentionMovieName",
	"ToolTip", "ItemIsMenu", "Menu",
}

// itemProp отдаёт значение свойства иконки.
func (t *SNI) itemProp(name string) (dbus.Variant, bool) {
	t.trayMu.Lock()
	defer t.trayMu.Unlock()

	empty := dbus.Array{ElemSig: "(iiay)"}
	switch name {
	case "Category":
		return dbus.Variant{Sig: "s", Val: "ApplicationStatus"}, true
	case "Id":
		return dbus.Variant{Sig: "s", Val: notifyAppName()}, true
	case "Title":
		return dbus.Variant{Sig: "s", Val: t.tooltip}, true
	case "Status":
		st := t.status
		if st == "" {
			st = "Active"
		}
		return dbus.Variant{Sig: "s", Val: st}, true
	case "WindowId":
		// В XML KStatusNotifierItem это INT32; мы окна не отдаём — 0.
		return dbus.Variant{Sig: "i", Val: int32(0)}, true
	case "IconName", "OverlayIconName", "AttentionIconName", "AttentionMovieName":
		// Иконку отдаём пикселями, именами из темы не пользуемся.
		return dbus.Variant{Sig: "s", Val: ""}, true
	case "IconPixmap":
		return dbus.Variant{Sig: "a(iiay)", Val: t.pixmapLocked()}, true
	case "OverlayIconPixmap", "AttentionIconPixmap":
		return dbus.Variant{Sig: "a(iiay)", Val: empty}, true
	case "ToolTip":
		// (значок из темы, пиксели значка, заголовок, текст) — панели показывают
		// заголовок, поэтому подсказка живёт именно там.
		return dbus.Variant{Sig: "(sa(iiay)ss)", Val: dbus.Struct{
			Fields: []any{"", empty, t.tooltip, ""},
		}}, true
	case "ItemIsMenu":
		// false — левый клик приходит нам как Activate, а не открывает меню.
		return dbus.Variant{Sig: "b", Val: false}, true
	case "Menu":
		if t.menu == nil {
			return dbus.Variant{Sig: "o", Val: dbus.ObjectPath("/")}, true
		}
		return dbus.Variant{Sig: "o", Val: dbus.ObjectPath(dbusmenuPath)}, true
	}
	return dbus.Variant{}, false
}

// pixmapLocked собирает a(iiay) с текущей иконкой. Вызывать под trayMu.
func (t *SNI) pixmapLocked() dbus.Array {
	out := dbus.Array{ElemSig: "(iiay)"}
	if t.iconW <= 0 || t.iconH <= 0 || len(t.iconARGB) == 0 {
		return out
	}
	out.Items = append(out.Items, dbus.Struct{Fields: []any{t.iconW, t.iconH, t.iconARGB}})
	return out
}

// dispatchClick доставляет клик по иконке диспетчеру окна. Колбэк уходит в
// отдельную горутину: он крутит UI, а мы обязаны быстро ответить панели.
func (t *SNI) dispatchClick(button int) {
	t.trayMu.Lock()
	fn := t.onTrayClick
	t.trayMu.Unlock()
	if fn != nil {
		go fn(button, false)
	}
}

// ─── com.canonical.dbusmenu ──────────────────────────────────────────────────

// handleMenu — меню трея: раскладка, свойства и события пунктов.
func (t *SNI) handleMenu(msg *dbus.Message) *dbus.Reply {
	switch msg.Interface {
	case dbus.IfaceIntrospectable:
		if msg.Member == "Introspect" {
			return &dbus.Reply{Sig: "s", Body: []any{dbusmenuIntrospectXML}}
		}
	case dbus.IfaceProperties:
		return dbusPropsReply(msg, dbusmenuIface, dbusmenuPropNames, dbusmenuProp)
	case dbusmenuIface:
		return t.handleMenuMember(msg)
	}
	return nil
}

// dbusmenuPropNames / dbusmenuProp — свойства самого меню (не пунктов).
var dbusmenuPropNames = []string{"Version", "TextDirection", "Status", "IconThemePath"}

func dbusmenuProp(name string) (dbus.Variant, bool) {
	switch name {
	case "Version":
		return dbus.Variant{Sig: "u", Val: uint32(3)}, true
	case "TextDirection":
		return dbus.Variant{Sig: "s", Val: "ltr"}, true
	case "Status":
		return dbus.Variant{Sig: "s", Val: "normal"}, true
	case "IconThemePath":
		return dbus.Variant{Sig: "as", Val: []string{}}, true
	}
	return dbus.Variant{}, false
}

// handleMenuMember разбирает методы com.canonical.dbusmenu.
func (t *SNI) handleMenuMember(msg *dbus.Message) *dbus.Reply {
	t.trayMu.Lock()
	menu, rev := t.menu, t.revision
	t.trayMu.Unlock()

	var items []base.MenuItem
	if menu != nil {
		items = menu.Items()
	}
	root, byID := dbusmenuBuild(items)

	switch msg.Member {
	case "GetLayout":
		parent, depth := int32(0), int32(-1)
		var names []string
		if len(msg.Body) > 0 {
			parent, _ = msg.Body[0].(int32)
		}
		if len(msg.Body) > 1 {
			depth, _ = msg.Body[1].(int32)
		}
		if len(msg.Body) > 2 {
			names, _ = msg.Body[2].([]string)
		}
		node, ok := byID[parent]
		if !ok {
			return &dbus.Reply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs",
				ErrMsg: fmt.Sprintf("нет пункта меню %d", parent)}
		}
		return &dbus.Reply{Sig: "u" + dbusmenuNodeSig,
			Body: []any{rev, dbusmenuLayout(node, depth, names)}}

	case "GetGroupProperties":
		var ids []any
		var names []string
		if len(msg.Body) > 0 {
			ids, _ = msg.Body[0].([]any)
		}
		if len(msg.Body) > 1 {
			names, _ = msg.Body[1].([]string)
		}
		out := dbus.Array{ElemSig: "(ia{sv})"}
		add := func(e *dbusmenuEntry) {
			out.Items = append(out.Items, dbus.Struct{
				Fields: []any{e.id, dbusmenuFilter(e.props(), names)}})
		}
		if len(ids) == 0 {
			// Пустой список — «все пункты» (так делает libdbusmenu).
			dbusmenuEach(root, add)
		} else {
			for _, v := range ids {
				id, _ := v.(int32)
				if e, ok := byID[id]; ok {
					add(e)
				}
			}
		}
		return &dbus.Reply{Sig: "a(ia{sv})", Body: []any{out}}

	case "GetProperty":
		if len(msg.Body) < 2 {
			return &dbus.Reply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs", ErrMsg: "GetProperty(id, name)"}
		}
		id, _ := msg.Body[0].(int32)
		name, _ := msg.Body[1].(string)
		e, ok := byID[id]
		if !ok {
			return &dbus.Reply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs", ErrMsg: "нет пункта меню"}
		}
		v, ok := e.props()[name]
		if !ok {
			return &dbus.Reply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs", ErrMsg: "нет свойства " + name}
		}
		return &dbus.Reply{Sig: "v", Body: []any{v}}

	case "Event":
		if len(msg.Body) < 2 {
			return &dbus.Reply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs", ErrMsg: "Event(id, eventId, data, timestamp)"}
		}
		id, _ := msg.Body[0].(int32)
		ev, _ := msg.Body[1].(string)
		if ev == "clicked" {
			t.activateItem(menu, byID[id])
		}
		return &dbus.Reply{}

	case "AboutToShow":
		// Дерево строим на каждый GetLayout — обновлять панели нечего.
		return &dbus.Reply{Sig: "b", Body: []any{false}}

	case "AboutToShowGroup":
		return &dbus.Reply{Sig: "aiai", Body: []any{[]int32{}, []int32{}}}
	}
	return nil
}

// activateItem выполняет выбор пункта меню: OnClick пункта и OnSelect меню
// (индекс — в СВОЁМ списке, как у нашего base.PopupMenu: у вложенного пункта
// это индекс внутри подменю). Разделители и выключенные пункты игнорируются.
// Ответ панели не ждёт колбэков: они уходят в очередь движка окна — там же
// выполняются обработчики виджетов (GG-68), — а без неё в отдельную горутину.
func (t *SNI) activateItem(menu *base.PopupMenu, e *dbusmenuEntry) {
	if e == nil || e.id == 0 || e.item.Separator || e.item.Disabled {
		return
	}
	onClick, idx, text := e.item.OnClick, e.index, e.item.Text
	t.trayMu.Lock()
	run := t.menuRun
	t.trayMu.Unlock()
	if run == nil {
		run = func(fn func()) { go fn() }
	}
	run(func() {
		if onClick != nil {
			onClick()
		}
		if menu != nil && menu.OnSelect != nil {
			menu.OnSelect(idx, text)
		}
	})
}

// SetTrayMenuRunner задаёт, где выполнять выбор пункта меню (очередь движка).
func (t *SNI) SetTrayMenuRunner(run func(fn func())) {
	t.trayMu.Lock()
	t.menuRun = run
	t.trayMu.Unlock()
}

// ─── Дерево меню ─────────────────────────────────────────────────────────────

// dbusmenuEntry — пункт меню с присвоенным идентификатором dbusmenu.
//
// Нумерация ПЛОСКАЯ и сквозная в порядке обхода в глубину: корень — 0, дальше
// 1, 2, 3… по мере спуска (пункт, его подпункты, следующий пункт…). Так она
// стабильна между вызовами (дерево пересобирается из тех же Items()), не
// ограничивает вложенность и переживает подменю любой глубины. Меняется меню —
// SetTrayMenu поднимает revision и шлёт LayoutUpdated, панель перечитывает всё.
type dbusmenuEntry struct {
	id    int32
	item  base.MenuItem
	index int // индекс пункта в своём списке (для PopupMenu.OnSelect)
	kids  []*dbusmenuEntry
}

// dbusmenuBuild строит дерево с идентификаторами и индекс «id → пункт».
// Корень (id 0) присутствует всегда, даже у пустого меню.
func dbusmenuBuild(items []base.MenuItem) (*dbusmenuEntry, map[int32]*dbusmenuEntry) {
	root := &dbusmenuEntry{id: 0, index: -1}
	byID := map[int32]*dbusmenuEntry{0: root}
	next := int32(0)

	var walk func(parent *dbusmenuEntry, list []base.MenuItem)
	walk = func(parent *dbusmenuEntry, list []base.MenuItem) {
		for i, it := range list {
			next++
			e := &dbusmenuEntry{id: next, item: it, index: i}
			byID[e.id] = e
			parent.kids = append(parent.kids, e)
			if len(it.SubItems) > 0 {
				walk(e, it.SubItems)
			}
		}
	}
	walk(root, items)
	return root, byID
}

// dbusmenuEach обходит дерево в глубину, включая корень.
func dbusmenuEach(e *dbusmenuEntry, fn func(*dbusmenuEntry)) {
	if e == nil {
		return
	}
	fn(e)
	for _, k := range e.kids {
		dbusmenuEach(k, fn)
	}
}

// props собирает a{sv} пункта в терминах dbusmenu.
func (e *dbusmenuEntry) props() map[string]dbus.Variant {
	p := map[string]dbus.Variant{}
	if e.id == 0 {
		p["children-display"] = dbus.Variant{Sig: "s", Val: "submenu"}
		return p
	}
	if e.item.Separator {
		p["type"] = dbus.Variant{Sig: "s", Val: "separator"}
		p["visible"] = dbus.Variant{Sig: "b", Val: true}
		return p
	}
	p["label"] = dbus.Variant{Sig: "s", Val: dbusmenuLabel(e.item.Text)}
	p["enabled"] = dbus.Variant{Sig: "b", Val: !e.item.Disabled}
	p["visible"] = dbus.Variant{Sig: "b", Val: true}
	if len(e.kids) > 0 {
		p["children-display"] = dbus.Variant{Sig: "s", Val: "submenu"}
	}
	return p
}

// dbusmenuLabel экранирует подчёркивания: в dbusmenu '_' помечает мнемонику
// («_Файл» подчеркнёт Ф), а в наших пунктах это обычный символ.
func dbusmenuLabel(s string) string { return strings.ReplaceAll(s, "_", "__") }

// dbusmenuFilter оставляет только запрошенные свойства (пустой список — все).
func dbusmenuFilter(p map[string]dbus.Variant, names []string) map[string]dbus.Variant {
	if len(names) == 0 {
		return p
	}
	out := map[string]dbus.Variant{}
	for _, n := range names {
		if v, ok := p[n]; ok {
			out[n] = v
		}
	}
	return out
}

// dbusmenuLayout сериализует узел в (ia{sv}av). depth: -1 — все уровни,
// 0 — без детей, n — не глубже n уровней (как требует спецификация GetLayout).
func dbusmenuLayout(e *dbusmenuEntry, depth int32, names []string) dbus.Struct {
	children := dbus.Array{ElemSig: "v"}
	if depth != 0 {
		sub := depth
		if sub > 0 {
			sub--
		}
		for _, k := range e.kids {
			children.Items = append(children.Items,
				dbus.Variant{Sig: dbusmenuNodeSig, Val: dbusmenuLayout(k, sub, names)})
		}
	}
	return dbus.Struct{Fields: []any{e.id, dbusmenuFilter(e.props(), names), children}}
}

// ─── org.freedesktop.DBus.Properties ─────────────────────────────────────────

// dbusPropsReply отвечает на Get/GetAll/Set для одного интерфейса: get отдаёт
// значение свойства, names — состав GetAll.
func dbusPropsReply(msg *dbus.Message, iface string, names []string,
	get func(string) (dbus.Variant, bool)) *dbus.Reply {

	switch msg.Member {
	case "Get":
		if len(msg.Body) < 2 {
			return &dbus.Reply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs", ErrMsg: "Get(iface, name)"}
		}
		want, _ := msg.Body[0].(string)
		name, _ := msg.Body[1].(string)
		// Пустой интерфейс шлют нестрогие клиенты — отвечаем как за свой.
		if want != "" && want != iface {
			return &dbus.Reply{ErrName: "org.freedesktop.DBus.Error.UnknownInterface", ErrMsg: want}
		}
		if v, ok := get(name); ok {
			return &dbus.Reply{Sig: "v", Body: []any{v}}
		}
		return &dbus.Reply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs",
			ErrMsg: "нет свойства " + iface + "." + name}
	case "GetAll":
		all := map[string]dbus.Variant{}
		var want string
		if len(msg.Body) > 0 {
			want, _ = msg.Body[0].(string)
		}
		if want == "" || want == iface {
			for _, n := range names {
				if v, ok := get(n); ok {
					all[n] = v
				}
			}
		}
		return &dbus.Reply{Sig: "a{sv}", Body: []any{all}}
	case "Set":
		return &dbus.Reply{ErrName: "org.freedesktop.DBus.Error.PropertyReadOnly", ErrMsg: "только чтение"}
	}
	return nil
}

// ─── Иконка: image.Image → ARGB32 ────────────────────────────────────────────

// sniIconPixmap готовит иконку для StatusNotifierItem: приводит к RGBA,
// ужимает слишком крупные картинки (панель всё равно масштабирует) и переводит
// пиксели в ARGB32. Возвращает (0,0,nil) для пустой иконки.
func sniIconPixmap(icon image.Image) (int32, int32, []byte) {
	if icon == nil {
		return 0, 0, nil
	}
	b := icon.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return 0, 0, nil
	}
	tw, th := sniFitIcon(w, h)
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	if tw == w && th == h {
		draw.Draw(dst, dst.Bounds(), icon, b.Min, draw.Src)
	} else {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), icon, b, xdraw.Src, nil)
	}
	return int32(tw), int32(th), sniARGB32(dst)
}

// sniFitIcon ужимает размер до sniMaxIcon по большей стороне, сохраняя пропорции.
func sniFitIcon(w, h int) (int, int) {
	if w <= sniMaxIcon && h <= sniMaxIcon {
		return w, h
	}
	if w >= h {
		nh := h * sniMaxIcon / w
		if nh < 1 {
			nh = 1
		}
		return sniMaxIcon, nh
	}
	nw := w * sniMaxIcon / h
	if nw < 1 {
		nw = 1
	}
	return nw, sniMaxIcon
}

// sniARGB32 переводит RGBA в пиксели StatusNotifierItem: ARGB32 в СЕТЕВОМ
// порядке байт (A,R,G,B), строки сверху вниз, без выравнивания.
//
// Внимание: это НЕ тот же буфер, что native.IconColorBuffer (platform/native/tray.go, там BGRA
// little-endian для Windows-DIB). Кроме порядка байт есть второе отличие:
// image.RGBA в Go премультиплицирован, а хосты трея читают пиксели как
// QImage::Format_ARGB32, то есть с ОБЫЧНОЙ альфой — поэтому здесь она
// разворачивается обратно (иначе полупрозрачные края выходят тёмными).
func sniARGB32(img *image.RGBA) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			si := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			r, g, bl, a := img.Pix[si], img.Pix[si+1], img.Pix[si+2], img.Pix[si+3]
			if a != 0 && a != 0xFF {
				r, g, bl = sniUnpremul(r, a), sniUnpremul(g, a), sniUnpremul(bl, a)
			}
			di := (y*w + x) * 4
			out[di+0] = a
			out[di+1] = r
			out[di+2] = g
			out[di+3] = bl
		}
	}
	return out
}

// sniUnpremul делит канал на альфу (с округлением) — обратная премультипликация.
func sniUnpremul(c, a byte) byte {
	v := (int(c)*255 + int(a)/2) / int(a)
	if v > 255 {
		v = 255
	}
	return byte(v)
}

// ─── Introspection ───────────────────────────────────────────────────────────

// sniItemIntrospectXML — описание /StatusNotifierItem (некоторые панели
// проверяют интерфейс интроспекцией, прежде чем читать свойства).
const sniItemIntrospectXML = `<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN" "http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">
<node>
 <interface name="org.freedesktop.DBus.Introspectable">
  <method name="Introspect"><arg name="xml" type="s" direction="out"/></method>
 </interface>
 <interface name="org.freedesktop.DBus.Properties">
  <method name="Get"><arg type="s" direction="in"/><arg type="s" direction="in"/><arg type="v" direction="out"/></method>
  <method name="GetAll"><arg type="s" direction="in"/><arg type="a{sv}" direction="out"/></method>
 </interface>
 <interface name="org.kde.StatusNotifierItem">
  <property name="Category" type="s" access="read"/>
  <property name="Id" type="s" access="read"/>
  <property name="Title" type="s" access="read"/>
  <property name="Status" type="s" access="read"/>
  <property name="WindowId" type="i" access="read"/>
  <property name="IconName" type="s" access="read"/>
  <property name="IconPixmap" type="a(iiay)" access="read"/>
  <property name="OverlayIconName" type="s" access="read"/>
  <property name="OverlayIconPixmap" type="a(iiay)" access="read"/>
  <property name="AttentionIconName" type="s" access="read"/>
  <property name="AttentionIconPixmap" type="a(iiay)" access="read"/>
  <property name="AttentionMovieName" type="s" access="read"/>
  <property name="ToolTip" type="(sa(iiay)ss)" access="read"/>
  <property name="ItemIsMenu" type="b" access="read"/>
  <property name="Menu" type="o" access="read"/>
  <method name="Activate"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
  <method name="SecondaryActivate"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
  <method name="ContextMenu"><arg name="x" type="i" direction="in"/><arg name="y" type="i" direction="in"/></method>
  <method name="Scroll"><arg name="delta" type="i" direction="in"/><arg name="orientation" type="s" direction="in"/></method>
  <signal name="NewIcon"/>
  <signal name="NewTitle"/>
  <signal name="NewToolTip"/>
  <signal name="NewMenu"/>
  <signal name="NewStatus"><arg name="status" type="s"/></signal>
 </interface>
</node>`

// dbusmenuIntrospectXML — описание /MenuBar.
const dbusmenuIntrospectXML = `<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN" "http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">
<node>
 <interface name="org.freedesktop.DBus.Introspectable">
  <method name="Introspect"><arg name="xml" type="s" direction="out"/></method>
 </interface>
 <interface name="org.freedesktop.DBus.Properties">
  <method name="Get"><arg type="s" direction="in"/><arg type="s" direction="in"/><arg type="v" direction="out"/></method>
  <method name="GetAll"><arg type="s" direction="in"/><arg type="a{sv}" direction="out"/></method>
 </interface>
 <interface name="com.canonical.dbusmenu">
  <property name="Version" type="u" access="read"/>
  <property name="TextDirection" type="s" access="read"/>
  <property name="Status" type="s" access="read"/>
  <property name="IconThemePath" type="as" access="read"/>
  <method name="GetLayout">
   <arg name="parentId" type="i" direction="in"/>
   <arg name="recursionDepth" type="i" direction="in"/>
   <arg name="propertyNames" type="as" direction="in"/>
   <arg name="revision" type="u" direction="out"/>
   <arg name="layout" type="(ia{sv}av)" direction="out"/>
  </method>
  <method name="GetGroupProperties">
   <arg name="ids" type="ai" direction="in"/>
   <arg name="propertyNames" type="as" direction="in"/>
   <arg name="properties" type="a(ia{sv})" direction="out"/>
  </method>
  <method name="GetProperty">
   <arg name="id" type="i" direction="in"/><arg name="name" type="s" direction="in"/>
   <arg name="value" type="v" direction="out"/>
  </method>
  <method name="Event">
   <arg name="id" type="i" direction="in"/><arg name="eventId" type="s" direction="in"/>
   <arg name="data" type="v" direction="in"/><arg name="timestamp" type="u" direction="in"/>
  </method>
  <method name="AboutToShow">
   <arg name="id" type="i" direction="in"/><arg name="needUpdate" type="b" direction="out"/>
  </method>
  <signal name="LayoutUpdated"><arg name="revision" type="u"/><arg name="parent" type="i"/></signal>
 </interface>
</node>`
