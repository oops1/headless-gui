//go:build linux && !android

// tray_sni_linux.go — иконка в системном трее на Linux (X11 и Wayland) по
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
// widget.PopupMenu по правому клику здесь не показывается — см. интерфейс
// nativeTrayMenu в tray.go.
//
// ОГРАНИЧЕНИЯ:
//   - GNOME из коробки StatusNotifierWatcher НЕ поднимает: без расширения
//     AppIndicator/AppIndicatorSupport иконки не будет (setTrayIcon вернёт
//     errNoTrayWatcher). Это свойство среды, а не нашего кода.
//   - Watcher, появившийся ПОЗЖЕ первого setTrayIcon, сам нас не подхватит:
//     нужен повторный SetTrayIcon (за NameOwnerChanged не следим).
//   - Позиции иконки протокол не сообщает: cursorScreenPos отдаёт (0,0),
//     меню позиционирует панель.
//   - Двойного клика в протоколе нет: Activate — это ОДИНАРНЫЙ левый клик,
//     поэтому дефолт «двойной левый клик восстанавливает окно» (tray.go) на
//     Linux не срабатывает. Нужен возврат из трея — задайте SetOnTrayClick
//     или пункт в SetTrayMenu.
//   - Wayland не даёт скрыть окно (нет аналога UnmapWindow), HideToTray там
//     no-op; на X11 — честный UnmapWindow.
//
// Соединение с шиной у трея ОТДЕЛЬНОЕ (свой dbusDial): обработчик входящих
// вызовов на соединении ровно один, а общее (dbusSession) уже занято
// уведомлениями и может понадобиться мосту AT-SPI.
package window

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

	"github.com/oops1/headless-gui/v3/widget"
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

// linuxTray — состояние иконки трея одного окна. Встраивается в X11Window и
// WaylandWindow (как linuxNotifier для уведомлений): его методы становятся их
// методами, и окно превращается в trayHost (tray.go).
//
// Потокобезопасность: setTrayIcon/removeTrayIcon/… зовутся с UI-потока окна,
// входящие вызовы D-Bus обрабатываются в горутинах соединения — всё состояние
// под trayMu. Пользовательские колбэки вызываются вне блокировки, в отдельной
// горутине (иначе долгий обработчик задержал бы ответ панели).
type linuxTray struct {
	trayMu sync.Mutex

	conn   *dbusConn // отдельное соединение с шиной (nil — трея нет)
	name   string    // занятое имя org.kde.StatusNotifierItem-<pid>-<n>
	active bool      // иконка зарегистрирована у Watcher
	status string    // Active / Passive

	iconW, iconH int32  // размер пиксельной иконки
	iconARGB     []byte // пиксели ARGB32 (сетевой порядок байт)
	tooltip      string

	menu     *widget.PopupMenu // дерево для dbusmenu (nil — меню нет)
	revision uint32            // ревизия раскладки меню (растёт при смене)

	onTrayClick func(button int, doubleClick bool)
}

// ─── trayHost: иконка ────────────────────────────────────────────────────────

// setTrayIcon показывает/обновляет иконку в трее. Первый вызов поднимает
// соединение и регистрируется у Watcher; последующие — обновляют пиксели и
// подсказку и будят панель сигналами NewIcon/NewTitle/NewToolTip.
func (t *linuxTray) setTrayIcon(icon image.Image, tooltip string) error {
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
	_ = c.emit(sniItemPath, sniItemIface, "NewIcon", "", nil)
	_ = c.emit(sniItemPath, sniItemIface, "NewTitle", "", nil)
	_ = c.emit(sniItemPath, sniItemIface, "NewToolTip", "", nil)
	return nil
}

// register подключается к шине, экспортирует /StatusNotifierItem и /MenuBar и
// просит Watcher взять иконку. Ошибка — трея в системе нет.
func (t *linuxTray) register() error {
	c, err := dbusDial(dbusSessionAddress())
	if err != nil {
		return fmt.Errorf("window: трей недоступен: %w", err)
	}
	if !c.nameHasOwner(sniWatcherName) {
		c.Close()
		return errNoTrayWatcher
	}
	name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-%d", os.Getpid(), sniItemSeq.Add(1))
	if err := c.requestName(name, 0x4 /*DO_NOT_QUEUE*/); err != nil {
		c.Close()
		return fmt.Errorf("window: имя иконки трея: %w", err)
	}
	// Обработчик ставим ДО регистрации: панель читает свойства сразу.
	c.setCallHandler(t.handleCall)
	if _, err := c.call(sniWatcherName, sniWatcherPath, sniWatcherIface,
		"RegisterStatusNotifierItem", "s", []any{name}); err != nil {
		c.Close()
		return fmt.Errorf("window: RegisterStatusNotifierItem: %w", err)
	}

	t.trayMu.Lock()
	t.conn, t.name, t.active = c, name, true
	t.trayMu.Unlock()
	return nil
}

// removeTrayIcon убирает иконку: объявляем Passive (панель прячет её сразу),
// отдаём имя и закрываем соединение. Повторный setTrayIcon поднимет всё заново.
func (t *linuxTray) removeTrayIcon() {
	t.trayMu.Lock()
	c, active, name := t.conn, t.active, t.name
	t.conn, t.active, t.name = nil, false, ""
	t.status = "Passive"
	t.trayMu.Unlock()

	if !active || c == nil {
		return
	}
	_ = c.emit(sniItemPath, sniItemIface, "NewStatus", "s", []any{"Passive"})
	_, _ = c.call("org.freedesktop.DBus", "/org/freedesktop/DBus",
		"org.freedesktop.DBus", "ReleaseName", "s", []any{name})
	c.Close()
}

// setTrayClickHandler регистрирует диспетчер кликов по иконке.
func (t *linuxTray) setTrayClickHandler(fn func(button int, doubleClick bool)) {
	t.trayMu.Lock()
	t.onTrayClick = fn
	t.trayMu.Unlock()
}

// cursorScreenPos: у StatusNotifierItem нет понятия «позиция курсора» —
// меню размещает панель. Возвращаем (0,0), нашим меню мы здесь не пользуемся.
func (t *linuxTray) cursorScreenPos() (int, int) { return 0, 0 }

// ─── nativeTrayMenu: меню отдаём панели ──────────────────────────────────────

// setTrayMenuNative принимает трей-меню для показа СРЕДОЙ (com.canonical.dbusmenu)
// и всегда возвращает true: на Linux панель рисует меню сама, показывать своё
// widget.PopupMenu поверх неё не нужно и нечем (позиции иконки мы не знаем).
func (t *linuxTray) setTrayMenuNative(menu *widget.PopupMenu) bool {
	t.trayMu.Lock()
	t.menu = menu
	t.revision++
	rev, c := t.revision, t.conn
	t.trayMu.Unlock()

	if c != nil {
		// Панель перечитает дерево (LayoutUpdated) и путь к меню (NewMenu).
		_ = c.emit(dbusmenuPath, dbusmenuIface, "LayoutUpdated", "ui", []any{rev, int32(0)})
		_ = c.emit(sniItemPath, sniItemIface, "NewMenu", "", nil)
	}
	return true
}

// ─── Входящие вызовы ─────────────────────────────────────────────────────────

// handleCall — мультиплексор входящих вызовов соединения трея по пути объекта.
func (t *linuxTray) handleCall(msg *dbusMessage) *dbusReply {
	switch msg.Path {
	case sniItemPath:
		return t.handleItem(msg)
	case dbusmenuPath:
		return t.handleMenu(msg)
	}
	return nil
}

// handleItem — org.kde.StatusNotifierItem и его свойства.
func (t *linuxTray) handleItem(msg *dbusMessage) *dbusReply {
	switch msg.Interface {
	case ifaceIntrospect:
		if msg.Member == "Introspect" {
			return &dbusReply{Sig: "s", Body: []any{sniItemIntrospectXML}}
		}
	case ifaceProps:
		return dbusPropsReply(msg, sniItemIface, sniItemPropNames, t.itemProp)
	case sniItemIface:
		switch msg.Member {
		case "Activate": // левый клик
			t.dispatchClick(0)
			return &dbusReply{}
		case "SecondaryActivate": // средняя кнопка
			t.dispatchClick(2)
			return &dbusReply{}
		case "ContextMenu":
			// Правый клик: меню показывает панель через dbusmenu, нам делать
			// нечего. Отвечаем пустым ответом, иначе панель ждёт таймаут.
			return &dbusReply{}
		case "Scroll":
			return &dbusReply{}
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
func (t *linuxTray) itemProp(name string) (dbusVariant, bool) {
	t.trayMu.Lock()
	defer t.trayMu.Unlock()

	empty := dbusArray{ElemSig: "(iiay)"}
	switch name {
	case "Category":
		return dbusVariant{Sig: "s", Val: "ApplicationStatus"}, true
	case "Id":
		return dbusVariant{Sig: "s", Val: notifyAppName()}, true
	case "Title":
		return dbusVariant{Sig: "s", Val: t.tooltip}, true
	case "Status":
		st := t.status
		if st == "" {
			st = "Active"
		}
		return dbusVariant{Sig: "s", Val: st}, true
	case "WindowId":
		// В XML KStatusNotifierItem это INT32; мы окна не отдаём — 0.
		return dbusVariant{Sig: "i", Val: int32(0)}, true
	case "IconName", "OverlayIconName", "AttentionIconName", "AttentionMovieName":
		// Иконку отдаём пикселями, именами из темы не пользуемся.
		return dbusVariant{Sig: "s", Val: ""}, true
	case "IconPixmap":
		return dbusVariant{Sig: "a(iiay)", Val: t.pixmapLocked()}, true
	case "OverlayIconPixmap", "AttentionIconPixmap":
		return dbusVariant{Sig: "a(iiay)", Val: empty}, true
	case "ToolTip":
		// (значок из темы, пиксели значка, заголовок, текст) — панели показывают
		// заголовок, поэтому подсказка живёт именно там.
		return dbusVariant{Sig: "(sa(iiay)ss)", Val: dbusStruct{
			Fields: []any{"", empty, t.tooltip, ""},
		}}, true
	case "ItemIsMenu":
		// false — левый клик приходит нам как Activate, а не открывает меню.
		return dbusVariant{Sig: "b", Val: false}, true
	case "Menu":
		if t.menu == nil {
			return dbusVariant{Sig: "o", Val: dbusObjectPath("/")}, true
		}
		return dbusVariant{Sig: "o", Val: dbusObjectPath(dbusmenuPath)}, true
	}
	return dbusVariant{}, false
}

// pixmapLocked собирает a(iiay) с текущей иконкой. Вызывать под trayMu.
func (t *linuxTray) pixmapLocked() dbusArray {
	out := dbusArray{ElemSig: "(iiay)"}
	if t.iconW <= 0 || t.iconH <= 0 || len(t.iconARGB) == 0 {
		return out
	}
	out.Items = append(out.Items, dbusStruct{Fields: []any{t.iconW, t.iconH, t.iconARGB}})
	return out
}

// dispatchClick доставляет клик по иконке диспетчеру окна. Колбэк уходит в
// отдельную горутину: он крутит UI, а мы обязаны быстро ответить панели.
func (t *linuxTray) dispatchClick(button int) {
	t.trayMu.Lock()
	fn := t.onTrayClick
	t.trayMu.Unlock()
	if fn != nil {
		go fn(button, false)
	}
}

// ─── com.canonical.dbusmenu ──────────────────────────────────────────────────

// handleMenu — меню трея: раскладка, свойства и события пунктов.
func (t *linuxTray) handleMenu(msg *dbusMessage) *dbusReply {
	switch msg.Interface {
	case ifaceIntrospect:
		if msg.Member == "Introspect" {
			return &dbusReply{Sig: "s", Body: []any{dbusmenuIntrospectXML}}
		}
	case ifaceProps:
		return dbusPropsReply(msg, dbusmenuIface, dbusmenuPropNames, dbusmenuProp)
	case dbusmenuIface:
		return t.handleMenuMember(msg)
	}
	return nil
}

// dbusmenuPropNames / dbusmenuProp — свойства самого меню (не пунктов).
var dbusmenuPropNames = []string{"Version", "TextDirection", "Status", "IconThemePath"}

func dbusmenuProp(name string) (dbusVariant, bool) {
	switch name {
	case "Version":
		return dbusVariant{Sig: "u", Val: uint32(3)}, true
	case "TextDirection":
		return dbusVariant{Sig: "s", Val: "ltr"}, true
	case "Status":
		return dbusVariant{Sig: "s", Val: "normal"}, true
	case "IconThemePath":
		return dbusVariant{Sig: "as", Val: []string{}}, true
	}
	return dbusVariant{}, false
}

// handleMenuMember разбирает методы com.canonical.dbusmenu.
func (t *linuxTray) handleMenuMember(msg *dbusMessage) *dbusReply {
	t.trayMu.Lock()
	menu, rev := t.menu, t.revision
	t.trayMu.Unlock()

	var items []widget.MenuItem
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
			return &dbusReply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs",
				ErrMsg: fmt.Sprintf("нет пункта меню %d", parent)}
		}
		return &dbusReply{Sig: "u" + dbusmenuNodeSig,
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
		out := dbusArray{ElemSig: "(ia{sv})"}
		add := func(e *dbusmenuEntry) {
			out.Items = append(out.Items, dbusStruct{
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
		return &dbusReply{Sig: "a(ia{sv})", Body: []any{out}}

	case "GetProperty":
		if len(msg.Body) < 2 {
			return &dbusReply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs", ErrMsg: "GetProperty(id, name)"}
		}
		id, _ := msg.Body[0].(int32)
		name, _ := msg.Body[1].(string)
		e, ok := byID[id]
		if !ok {
			return &dbusReply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs", ErrMsg: "нет пункта меню"}
		}
		v, ok := e.props()[name]
		if !ok {
			return &dbusReply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs", ErrMsg: "нет свойства " + name}
		}
		return &dbusReply{Sig: "v", Body: []any{v}}

	case "Event":
		if len(msg.Body) < 2 {
			return &dbusReply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs", ErrMsg: "Event(id, eventId, data, timestamp)"}
		}
		id, _ := msg.Body[0].(int32)
		ev, _ := msg.Body[1].(string)
		if ev == "clicked" {
			t.activateItem(menu, byID[id])
		}
		return &dbusReply{}

	case "AboutToShow":
		// Дерево строим на каждый GetLayout — обновлять панели нечего.
		return &dbusReply{Sig: "b", Body: []any{false}}

	case "AboutToShowGroup":
		return &dbusReply{Sig: "aiai", Body: []any{[]int32{}, []int32{}}}
	}
	return nil
}

// activateItem выполняет выбор пункта меню: OnClick пункта и OnSelect меню
// (индекс — в СВОЁМ списке, как у нашего widget.PopupMenu: у вложенного пункта
// это индекс внутри подменю). Разделители и выключенные пункты игнорируются.
// Колбэки уходят в отдельную горутину — ответ панели не должен ждать UI.
func (t *linuxTray) activateItem(menu *widget.PopupMenu, e *dbusmenuEntry) {
	if e == nil || e.id == 0 || e.item.Separator || e.item.Disabled {
		return
	}
	onClick, idx, text := e.item.OnClick, e.index, e.item.Text
	go func() {
		if onClick != nil {
			onClick()
		}
		if menu != nil && menu.OnSelect != nil {
			menu.OnSelect(idx, text)
		}
	}()
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
	item  widget.MenuItem
	index int // индекс пункта в своём списке (для PopupMenu.OnSelect)
	kids  []*dbusmenuEntry
}

// dbusmenuBuild строит дерево с идентификаторами и индекс «id → пункт».
// Корень (id 0) присутствует всегда, даже у пустого меню.
func dbusmenuBuild(items []widget.MenuItem) (*dbusmenuEntry, map[int32]*dbusmenuEntry) {
	root := &dbusmenuEntry{id: 0, index: -1}
	byID := map[int32]*dbusmenuEntry{0: root}
	next := int32(0)

	var walk func(parent *dbusmenuEntry, list []widget.MenuItem)
	walk = func(parent *dbusmenuEntry, list []widget.MenuItem) {
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
func (e *dbusmenuEntry) props() map[string]dbusVariant {
	p := map[string]dbusVariant{}
	if e.id == 0 {
		p["children-display"] = dbusVariant{Sig: "s", Val: "submenu"}
		return p
	}
	if e.item.Separator {
		p["type"] = dbusVariant{Sig: "s", Val: "separator"}
		p["visible"] = dbusVariant{Sig: "b", Val: true}
		return p
	}
	p["label"] = dbusVariant{Sig: "s", Val: dbusmenuLabel(e.item.Text)}
	p["enabled"] = dbusVariant{Sig: "b", Val: !e.item.Disabled}
	p["visible"] = dbusVariant{Sig: "b", Val: true}
	if len(e.kids) > 0 {
		p["children-display"] = dbusVariant{Sig: "s", Val: "submenu"}
	}
	return p
}

// dbusmenuLabel экранирует подчёркивания: в dbusmenu '_' помечает мнемонику
// («_Файл» подчеркнёт Ф), а в наших пунктах это обычный символ.
func dbusmenuLabel(s string) string { return strings.ReplaceAll(s, "_", "__") }

// dbusmenuFilter оставляет только запрошенные свойства (пустой список — все).
func dbusmenuFilter(p map[string]dbusVariant, names []string) map[string]dbusVariant {
	if len(names) == 0 {
		return p
	}
	out := map[string]dbusVariant{}
	for _, n := range names {
		if v, ok := p[n]; ok {
			out[n] = v
		}
	}
	return out
}

// dbusmenuLayout сериализует узел в (ia{sv}av). depth: -1 — все уровни,
// 0 — без детей, n — не глубже n уровней (как требует спецификация GetLayout).
func dbusmenuLayout(e *dbusmenuEntry, depth int32, names []string) dbusStruct {
	children := dbusArray{ElemSig: "v"}
	if depth != 0 {
		sub := depth
		if sub > 0 {
			sub--
		}
		for _, k := range e.kids {
			children.Items = append(children.Items,
				dbusVariant{Sig: dbusmenuNodeSig, Val: dbusmenuLayout(k, sub, names)})
		}
	}
	return dbusStruct{Fields: []any{e.id, dbusmenuFilter(e.props(), names), children}}
}

// ─── org.freedesktop.DBus.Properties ─────────────────────────────────────────

// dbusPropsReply отвечает на Get/GetAll/Set для одного интерфейса: get отдаёт
// значение свойства, names — состав GetAll.
func dbusPropsReply(msg *dbusMessage, iface string, names []string,
	get func(string) (dbusVariant, bool)) *dbusReply {

	switch msg.Member {
	case "Get":
		if len(msg.Body) < 2 {
			return &dbusReply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs", ErrMsg: "Get(iface, name)"}
		}
		want, _ := msg.Body[0].(string)
		name, _ := msg.Body[1].(string)
		// Пустой интерфейс шлют нестрогие клиенты — отвечаем как за свой.
		if want != "" && want != iface {
			return &dbusReply{ErrName: "org.freedesktop.DBus.Error.UnknownInterface", ErrMsg: want}
		}
		if v, ok := get(name); ok {
			return &dbusReply{Sig: "v", Body: []any{v}}
		}
		return &dbusReply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs",
			ErrMsg: "нет свойства " + iface + "." + name}
	case "GetAll":
		all := map[string]dbusVariant{}
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
		return &dbusReply{Sig: "a{sv}", Body: []any{all}}
	case "Set":
		return &dbusReply{ErrName: "org.freedesktop.DBus.Error.PropertyReadOnly", ErrMsg: "только чтение"}
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
// Внимание: это НЕ тот же буфер, что iconColorBuffer в tray.go (там BGRA
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

// ─── Привязка к бэкендам ─────────────────────────────────────────────────────

// hideToTray прячет окно X11: UnmapWindow убирает его и с экрана, и из панели
// задач EWMH-WM (полный аналог SW_HIDE на Windows).
func (w *X11Window) hideToTray() {
	if w.wid != 0 {
		w.x11UnmapWindow(w.wid)
	}
}

// restoreFromTray показывает окно обратно и просит WM его активировать.
func (w *X11Window) restoreFromTray() {
	if w.wid == 0 {
		return
	}
	w.x11MapWindow(w.wid)
	w.x11RaiseWindow()
	w.x11NetActiveWindow()
}

// hideToTray на Wayland — no-op: xdg_toplevel скрыть нельзя (нет ни unmap, ни
// «спрятать» в протоколе), а set_minimized лишь просит композитор свернуть окно
// и на многих композиторах не реализован. Иконка в трее при этом работает.
func (w *WaylandWindow) hideToTray() {}

// restoreFromTray на Wayland — no-op: скрывать было нечего (см. hideToTray).
func (w *WaylandWindow) restoreFromTray() {}

// SetForeground на Wayland — no-op: активацию окна раздаёт композитор
// (xdg-activation-v1 у нас не реализован). Метод нужен, чтобы окно
// удовлетворяло интерфейсу trayHost.
func (w *WaylandWindow) SetForeground() {}

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
