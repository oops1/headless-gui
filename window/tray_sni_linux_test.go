//go:build linux && !android

package window

import (
	"errors"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// ─── Иконка: ARGB32 ──────────────────────────────────────────────────────────

// TestSNIARGB32 — конвертер пикселей: порядок байт A,R,G,B и разворот
// премультиплицированной альфы (image.RGBA премультиплицирован, а панели
// читают пиксели как QImage::Format_ARGB32).
func TestSNIARGB32(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	// (0,0) непрозрачный зелёный, (1,0) полупрозрачный красный (премультипл.),
	// (0,1) полностью прозрачный, (1,1) непрозрачный синий.
	img.SetRGBA(0, 0, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	img.SetRGBA(1, 0, color.RGBA{R: 128, G: 0, B: 0, A: 128})
	img.SetRGBA(0, 1, color.RGBA{})
	img.SetRGBA(1, 1, color.RGBA{R: 0, G: 0, B: 255, A: 255})

	got := sniARGB32(img)
	want := []byte{
		255, 0, 255, 0, // A,R,G,B — зелёный
		128, 255, 0, 0, // альфа развёрнута: 128/128 → R=255
		0, 0, 0, 0, // прозрачный
		255, 0, 0, 255, // синий
	}
	if len(got) != len(want) {
		t.Fatalf("длина %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("байт %d = %d, want %d (всё: %v)", i, got[i], want[i], got)
		}
	}

	// Порядок байт ОТЛИЧАЕТСЯ от буфера Windows-DIB (BGRA little-endian) —
	// переиспользовать iconColorBuffer нельзя, проверяем это явно.
	if bgra := iconColorBuffer(img); bgra[0] == got[0] && bgra[1] == got[1] &&
		bgra[2] == got[2] && bgra[3] == got[3] {
		t.Error("ARGB32 совпал с BGRA-буфером Windows — один из конвертеров неверен")
	}
}

// TestSNIIconPixmap — подготовка иконки: размер, приведение типа, ужимание.
func TestSNIIconPixmap(t *testing.T) {
	if w, h, px := sniIconPixmap(nil); w != 0 || h != 0 || px != nil {
		t.Errorf("nil-иконка дала %dx%d (%d байт)", w, h, len(px))
	}

	// Не-RGBA источник (NRGBA, НЕ премультиплицирован) должен доехать без
	// искажения цвета: 50% красного остаётся чистым красным с альфой 128.
	src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 255, A: 128})
		}
	}
	w, h, px := sniIconPixmap(src)
	if w != 4 || h != 4 || len(px) != 4*4*4 {
		t.Fatalf("размер %dx%d, %d байт", w, h, len(px))
	}
	if px[0] != 128 || px[1] != 255 || px[2] != 0 || px[3] != 0 {
		t.Errorf("первый пиксель %v, want [128 255 0 0]", px[:4])
	}

	// Крупная картинка ужимается с сохранением пропорций.
	big := image.NewRGBA(image.Rect(0, 0, 256, 128))
	w, h, px = sniIconPixmap(big)
	if w != sniMaxIcon || h != sniMaxIcon/2 {
		t.Errorf("масштаб дал %dx%d, want %dx%d", w, h, sniMaxIcon, sniMaxIcon/2)
	}
	if len(px) != int(w)*int(h)*4 {
		t.Errorf("буфер %d байт при %dx%d", len(px), w, h)
	}
	if gw, gh := sniFitIcon(64, 32); gw != 64 || gh != 32 {
		t.Errorf("маленькую иконку не трогаем: %dx%d", gw, gh)
	}
}

// ─── dbusmenu: дерево из widget.PopupMenu ────────────────────────────────────

// testTrayMenu — меню на все случаи: обычный пункт, разделитель, выключенный
// пункт и вложенное подменю.
func testTrayMenu(clicks chan<- string) *widget.PopupMenu {
	m := widget.NewPopupMenu()
	m.SetItems([]widget.MenuItem{
		{Text: "Показать", OnClick: func() { clicks <- "Показать" }},
		{Separator: true},
		{Text: "Не_доступно", Disabled: true, OnClick: func() { clicks <- "Не_доступно" }},
		{Text: "Язык", SubItems: []widget.MenuItem{
			{Text: "Русский", OnClick: func() { clicks <- "Русский" }},
			{Text: "English", OnClick: func() { clicks <- "English" }},
		}},
		{Text: "Выход", OnClick: func() { clicks <- "Выход" }},
	})
	return m
}

// layoutKids разбирает детей узла (ia{sv}av), пришедших как массив вариантов.
func layoutKids(t *testing.T, node dbusStruct) []dbusStruct {
	t.Helper()
	raw, ok := node.Fields[2].([]any)
	if !ok {
		if arr, isArr := node.Fields[2].(dbusArray); isArr {
			for _, it := range arr.Items {
				raw = append(raw, it)
			}
		}
	}
	out := make([]dbusStruct, 0, len(raw))
	for _, v := range raw {
		vr, ok := v.(dbusVariant)
		if !ok {
			t.Fatalf("ребёнок не variant: %#v", v)
		}
		st, ok := vr.Val.(dbusStruct)
		if !ok {
			t.Fatalf("вариант ребёнка не (ia{sv}av): %#v", vr.Val)
		}
		out = append(out, st)
	}
	return out
}

// layoutProps достаёт свойства узла (у собранного НАМИ дерева это
// map[string]dbusVariant, у раскодированного с шины — map[string]any).
func layoutProps(t *testing.T, node dbusStruct) map[string]any {
	t.Helper()
	switch p := node.Fields[1].(type) {
	case map[string]dbusVariant:
		out := map[string]any{}
		for k, v := range p {
			out[k] = v.Val
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k, v := range p {
			if vr, ok := v.(dbusVariant); ok {
				out[k] = vr.Val
			} else {
				out[k] = v
			}
		}
		return out
	}
	t.Fatalf("свойства узла: %#v", node.Fields[1])
	return nil
}

// TestDBusMenuLayout — сборка дерева com.canonical.dbusmenu из PopupMenu:
// нумерация, разделитель, выключенный пункт, подменю, глубина рекурсии.
func TestDBusMenuLayout(t *testing.T) {
	menu := testTrayMenu(make(chan string, 8))
	root, byID := dbusmenuBuild(menu.Items())

	// Плоская сквозная нумерация в порядке обхода в глубину:
	// 1 Показать, 2 разделитель, 3 Недоступно, 4 Язык, 5 Русский, 6 English, 7 Выход.
	if len(byID) != 8 { // 7 пунктов + корень
		t.Fatalf("узлов %d, want 8", len(byID))
	}
	for id, want := range map[int32]string{1: "Показать", 3: "Не_доступно", 4: "Язык", 5: "Русский", 6: "English", 7: "Выход"} {
		e, ok := byID[id]
		if !ok {
			t.Fatalf("нет узла %d", id)
		}
		if e.item.Text != want {
			t.Errorf("узел %d = %q, want %q", id, e.item.Text, want)
		}
	}
	// Индекс для OnSelect — в СВОЁМ списке (как у widget.PopupMenu).
	if byID[6].index != 1 {
		t.Errorf("English index = %d, want 1", byID[6].index)
	}
	if byID[7].index != 4 {
		t.Errorf("Выход index = %d, want 4", byID[7].index)
	}

	node := dbusmenuLayout(root, -1, nil)
	if id, _ := node.Fields[0].(int32); id != 0 {
		t.Errorf("корень имеет id %v", node.Fields[0])
	}
	if layoutProps(t, node)["children-display"] != "submenu" {
		t.Error("у корня нет children-display=submenu")
	}
	kids := layoutKids(t, node)
	if len(kids) != 5 {
		t.Fatalf("детей корня %d, want 5", len(kids))
	}

	// Обычный пункт: подчёркивание экранировано (в dbusmenu это мнемоника).
	p := layoutProps(t, kids[0])
	if p["label"] != "Показать" || p["enabled"] != true || p["visible"] != true {
		t.Errorf("пункт 0: %#v", p)
	}
	if _, ok := p["children-display"]; ok {
		t.Error("у листа не должно быть children-display")
	}

	// Разделитель.
	if p = layoutProps(t, kids[1]); p["type"] != "separator" {
		t.Errorf("разделитель: %#v", p)
	}

	// Выключенный пункт + экранирование '_'.
	if p = layoutProps(t, kids[2]); p["enabled"] != false || p["label"] != "Не__доступно" {
		t.Errorf("выключенный пункт: %#v", p)
	}

	// Подменю.
	p = layoutProps(t, kids[3])
	if p["children-display"] != "submenu" {
		t.Errorf("у пункта с подменю нет children-display: %#v", p)
	}
	sub := layoutKids(t, kids[3])
	if len(sub) != 2 {
		t.Fatalf("подпунктов %d, want 2", len(sub))
	}
	if layoutProps(t, sub[1])["label"] != "English" {
		t.Errorf("второй подпункт: %#v", layoutProps(t, sub[1]))
	}

	// recursionDepth: 0 — без детей, 1 — только верхний уровень.
	if k := layoutKids(t, dbusmenuLayout(root, 0, nil)); len(k) != 0 {
		t.Errorf("depth=0 дал %d детей", len(k))
	}
	k1 := layoutKids(t, dbusmenuLayout(root, 1, nil))
	if len(k1) != 5 {
		t.Fatalf("depth=1 дал %d детей верхнего уровня", len(k1))
	}
	if s := layoutKids(t, k1[3]); len(s) != 0 {
		t.Errorf("depth=1 всё же раскрыл подменю (%d)", len(s))
	}

	// propertyNames фильтрует свойства.
	only := layoutProps(t, layoutKids(t, dbusmenuLayout(root, 1, []string{"label"}))[0])
	if len(only) != 1 || only["label"] != "Показать" {
		t.Errorf("фильтр по label дал %#v", only)
	}

	// Пустое меню: корень есть, детей нет.
	emptyRoot, emptyByID := dbusmenuBuild(nil)
	if len(emptyByID) != 1 {
		t.Errorf("у пустого меню %d узлов", len(emptyByID))
	}
	if k := layoutKids(t, dbusmenuLayout(emptyRoot, -1, nil)); len(k) != 0 {
		t.Errorf("у пустого меню %d детей", len(k))
	}
}

// TestDBusMenuEventClicked — Event(clicked) доезжает до MenuItem.OnClick и
// PopupMenu.OnSelect; разделители и выключенные пункты игнорируются.
func TestDBusMenuEventClicked(t *testing.T) {
	clicks := make(chan string, 8)
	selects := make(chan string, 8)
	menu := testTrayMenu(clicks)
	menu.OnSelect = func(idx int, text string) { selects <- text }

	tray := &linuxTray{menu: menu}
	click := func(id int32) *dbusReply {
		return tray.handleCall(&dbusMessage{
			Path: dbusmenuPath, Interface: dbusmenuIface, Member: "Event",
			Sig:  "isvu",
			Body: []any{id, "clicked", dbusVariant{Sig: "s", Val: ""}, uint32(0)},
		})
	}
	waitClick := func(what string) {
		t.Helper()
		select {
		case got := <-clicks:
			if got != what {
				t.Errorf("OnClick дал %q, want %q", got, what)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("OnClick %q не сработал", what)
		}
		select {
		case got := <-selects:
			if got != what {
				t.Errorf("OnSelect дал %q, want %q", got, what)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("OnSelect %q не сработал", what)
		}
	}

	if rep := click(1); rep == nil || rep.ErrName != "" {
		t.Fatalf("Event вернул %#v", rep)
	}
	waitClick("Показать")

	// Вложенный пункт: OnSelect получает индекс ВНУТРИ подменю (так же ведёт
	// себя наш widget.PopupMenu — child.OnSelect = m.OnSelect).
	menu.OnSelect = func(idx int, text string) {
		if idx != 1 {
			t.Errorf("OnSelect подпункта: idx = %d, want 1", idx)
		}
		selects <- text
	}
	click(6)
	waitClick("English")

	// Разделитель и выключенный пункт молчат.
	click(2)
	click(3)
	click(999) // несуществующий id
	select {
	case s := <-clicks:
		t.Errorf("сработал запрещённый пункт: %q", s)
	case <-time.After(300 * time.Millisecond):
	}
}

// TestDBusMenuGetLayoutHandler — метод GetLayout целиком (ревизия, ошибка на
// неизвестный parentId) и GetGroupProperties.
func TestDBusMenuGetLayoutHandler(t *testing.T) {
	menu := testTrayMenu(make(chan string, 8))
	tray := &linuxTray{menu: menu, revision: 7}

	rep := tray.handleCall(&dbusMessage{
		Path: dbusmenuPath, Interface: dbusmenuIface, Member: "GetLayout",
		Sig: "iias", Body: []any{int32(0), int32(-1), []string{}},
	})
	if rep == nil || rep.ErrName != "" {
		t.Fatalf("GetLayout вернул %#v", rep)
	}
	if rep.Sig != "u(ia{sv}av)" {
		t.Errorf("сигнатура ответа %q", rep.Sig)
	}
	if rev, _ := rep.Body[0].(uint32); rev != 7 {
		t.Errorf("ревизия %v, want 7", rep.Body[0])
	}
	if k := layoutKids(t, rep.Body[1].(dbusStruct)); len(k) != 5 {
		t.Errorf("детей в ответе %d", len(k))
	}

	// Поддерево по id пункта с подменю.
	rep = tray.handleCall(&dbusMessage{
		Path: dbusmenuPath, Interface: dbusmenuIface, Member: "GetLayout",
		Sig: "iias", Body: []any{int32(4), int32(-1), []string{}},
	})
	if rep == nil || rep.ErrName != "" {
		t.Fatalf("GetLayout(4) вернул %#v", rep)
	}
	if k := layoutKids(t, rep.Body[1].(dbusStruct)); len(k) != 2 {
		t.Errorf("детей у подменю %d, want 2", len(k))
	}

	// Неизвестный parentId — ошибка, а не паника.
	rep = tray.handleCall(&dbusMessage{
		Path: dbusmenuPath, Interface: dbusmenuIface, Member: "GetLayout",
		Sig: "iias", Body: []any{int32(4242), int32(-1), []string{}},
	})
	if rep == nil || rep.ErrName == "" {
		t.Errorf("GetLayout несуществующего вернул %#v", rep)
	}

	// GetGroupProperties: пустой список ids — все узлы.
	rep = tray.handleCall(&dbusMessage{
		Path: dbusmenuPath, Interface: dbusmenuIface, Member: "GetGroupProperties",
		Sig: "aias", Body: []any{[]any{}, []string{}},
	})
	if rep == nil || rep.ErrName != "" {
		t.Fatalf("GetGroupProperties вернул %#v", rep)
	}
	arr, ok := rep.Body[0].(dbusArray)
	if !ok || len(arr.Items) != 8 {
		t.Errorf("GetGroupProperties отдал %#v", rep.Body[0])
	}

	// AboutToShow не просит перечитать дерево.
	rep = tray.handleCall(&dbusMessage{
		Path: dbusmenuPath, Interface: dbusmenuIface, Member: "AboutToShow",
		Sig: "i", Body: []any{int32(0)},
	})
	if rep == nil || rep.Body[0] != false {
		t.Errorf("AboutToShow вернул %#v", rep)
	}

	// Свойства самого меню.
	rep = tray.handleCall(&dbusMessage{
		Path: dbusmenuPath, Interface: ifaceProps, Member: "Get",
		Sig: "ss", Body: []any{dbusmenuIface, "Version"},
	})
	if rep == nil || rep.ErrName != "" {
		t.Fatalf("Get Version вернул %#v", rep)
	}
	if v, _ := rep.Body[0].(dbusVariant); v.Val != uint32(3) {
		t.Errorf("Version = %#v", rep.Body[0])
	}
}

// ─── Сквозная проверка на живой шине с фейковым Watcher'ом ───────────────────

// TestTrayAgainstFakeWatcher — трей целиком, без настоящего рабочего стола:
// тест поднимает СВОЙ org.kde.StatusNotifierWatcher на сессионной шине,
// принимает нашу регистрацию, затем вторым соединением («панель») читает
// свойства иконки, забирает дерево меню, кликает по пункту и по самой иконке.
func TestTrayAgainstFakeWatcher(t *testing.T) {
	watcher := dialTestBus(t)
	de := dialTestBus(t)

	registered := make(chan string, 4)
	watcher.setCallHandler(func(msg *dbusMessage) *dbusReply {
		if msg.Path != sniWatcherPath {
			return nil
		}
		switch {
		case msg.Interface == sniWatcherIface && msg.Member == "RegisterStatusNotifierItem":
			s, _ := msg.Body[0].(string)
			registered <- s
			return &dbusReply{}
		case msg.Interface == ifaceProps && msg.Member == "Get":
			return &dbusReply{Sig: "v", Body: []any{dbusVariant{Sig: "b", Val: true}}}
		}
		return nil
	})
	if err := watcher.requestName(sniWatcherName, 0x4 /*DO_NOT_QUEUE*/); err != nil {
		t.Skipf("настоящий трей уже держит %s, подменять не будем: %v", sniWatcherName, err)
	}

	clicks := make(chan string, 8)
	selects := make(chan string, 8)
	menu := testTrayMenu(clicks)
	menu.OnSelect = func(idx int, text string) { selects <- text }

	trayClicks := make(chan int, 8)
	tray := &linuxTray{}
	t.Cleanup(tray.removeTrayIcon)
	tray.setTrayClickHandler(func(button int, doubleClick bool) { trayClicks <- button })
	if !tray.setTrayMenuNative(menu) {
		t.Fatal("setTrayMenuNative вернул false — Window показал бы своё меню")
	}

	// Иконка 8×8: непрозрачный синий.
	icon := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < len(icon.Pix); i += 4 {
		icon.Pix[i+2], icon.Pix[i+3] = 255, 255 // B, A
	}
	if err := tray.setTrayIcon(icon, "headless-gui трей"); err != nil {
		t.Fatalf("setTrayIcon: %v", err)
	}

	var itemName string
	select {
	case itemName = <-registered:
	case <-time.After(3 * time.Second):
		t.Fatal("Watcher не получил RegisterStatusNotifierItem")
	}
	tray.trayMu.Lock()
	own := tray.name
	tray.trayMu.Unlock()
	if itemName != own || itemName == "" {
		t.Fatalf("зарегистрировано имя %q, а занято %q", itemName, own)
	}

	// ── «Панель» читает свойства иконки ──────────────────────────────────────
	rep, err := de.call(itemName, sniItemPath, ifaceProps, "GetAll", "s", []any{sniItemIface})
	if err != nil {
		t.Fatalf("Properties.GetAll: %v", err)
	}
	props, ok := rep.Body[0].(map[string]any)
	if !ok {
		t.Fatalf("GetAll вернул %#v", rep.Body[0])
	}
	strProp := func(name string) string {
		v, _ := props[name].(dbusVariant)
		s, _ := v.Val.(string)
		return s
	}
	if got := strProp("Category"); got != "ApplicationStatus" {
		t.Errorf("Category = %q", got)
	}
	if got := strProp("Status"); got != "Active" {
		t.Errorf("Status = %q", got)
	}
	if got := strProp("Title"); got != "headless-gui трей" {
		t.Errorf("Title = %q", got)
	}
	if v, _ := props["ItemIsMenu"].(dbusVariant); v.Val != false {
		t.Errorf("ItemIsMenu = %#v", props["ItemIsMenu"])
	}
	if v, _ := props["Menu"].(dbusVariant); v.Val != dbusObjectPath(dbusmenuPath) {
		t.Errorf("Menu = %#v, want %s", props["Menu"], dbusmenuPath)
	}

	// IconPixmap: одна картинка 8×8, первые байты — ARGB непрозрачного синего.
	pixVar, _ := props["IconPixmap"].(dbusVariant)
	pixArr, ok := pixVar.Val.([]any)
	if !ok || len(pixArr) != 1 {
		t.Fatalf("IconPixmap = %#v", pixVar.Val)
	}
	pix, ok := pixArr[0].(dbusStruct)
	if !ok || len(pix.Fields) != 3 {
		t.Fatalf("элемент IconPixmap = %#v", pixArr[0])
	}
	if pix.Fields[0] != int32(8) || pix.Fields[1] != int32(8) {
		t.Errorf("размер иконки %v×%v, want 8×8", pix.Fields[0], pix.Fields[1])
	}
	raw, _ := pix.Fields[2].([]byte)
	if len(raw) != 8*8*4 {
		t.Fatalf("пикселей %d байт, want %d", len(raw), 8*8*4)
	}
	if raw[0] != 255 || raw[1] != 0 || raw[2] != 0 || raw[3] != 255 {
		t.Errorf("первый пиксель %v, want [255 0 0 255] (A,R,G,B)", raw[:4])
	}

	// ToolTip читается точечным Get.
	rep, err = de.call(itemName, sniItemPath, ifaceProps, "Get", "ss", []any{sniItemIface, "ToolTip"})
	if err != nil {
		t.Fatalf("Get ToolTip: %v", err)
	}
	tip, _ := rep.Body[0].(dbusVariant)
	tipSt, ok := tip.Val.(dbusStruct)
	if !ok || len(tipSt.Fields) != 4 || tipSt.Fields[2] != "headless-gui трей" {
		t.Errorf("ToolTip = %#v", tip.Val)
	}

	// ── «Панель» забирает меню ───────────────────────────────────────────────
	rep, err = de.call(itemName, dbusmenuPath, dbusmenuIface, "GetLayout", "iias",
		[]any{int32(0), int32(-1), []string{}})
	if err != nil {
		t.Fatalf("GetLayout: %v", err)
	}
	root, ok := rep.Body[1].(dbusStruct)
	if !ok {
		t.Fatalf("layout = %#v", rep.Body[1])
	}
	kids := layoutKids(t, root)
	if len(kids) != 5 {
		t.Fatalf("панель увидела %d пунктов, want 5", len(kids))
	}
	if p := layoutProps(t, kids[0]); p["label"] != "Показать" {
		t.Errorf("первый пункт: %#v", p)
	}
	if p := layoutProps(t, kids[1]); p["type"] != "separator" {
		t.Errorf("второй пункт не разделитель: %#v", p)
	}
	if p := layoutProps(t, kids[2]); p["enabled"] != false {
		t.Errorf("третий пункт не выключен: %#v", p)
	}
	subKids := layoutKids(t, kids[3])
	if len(subKids) != 2 || layoutProps(t, subKids[0])["label"] != "Русский" {
		t.Errorf("подменю: %#v", subKids)
	}
	subID, _ := subKids[0].Fields[0].(int32)

	// ── Клик по пункту подменю ───────────────────────────────────────────────
	if _, err := de.call(itemName, dbusmenuPath, dbusmenuIface, "Event", "isvu",
		[]any{subID, "clicked", dbusVariant{Sig: "s", Val: ""}, uint32(0)}); err != nil {
		t.Fatalf("Event: %v", err)
	}
	select {
	case got := <-clicks:
		if got != "Русский" {
			t.Errorf("сработал пункт %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("клик по пункту меню не доехал до OnClick")
	}
	select {
	case got := <-selects:
		if got != "Русский" {
			t.Errorf("OnSelect дал %q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("OnSelect не сработал")
	}

	// ── Клик по самой иконке ─────────────────────────────────────────────────
	if _, err := de.call(itemName, sniItemPath, sniItemIface, "Activate", "ii",
		[]any{int32(10), int32(20)}); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	select {
	case b := <-trayClicks:
		if b != 0 {
			t.Errorf("Activate дал кнопку %d, want 0 (левая)", b)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Activate не доехал до колбэка клика")
	}
	if _, err := de.call(itemName, sniItemPath, sniItemIface, "SecondaryActivate", "ii",
		[]any{int32(0), int32(0)}); err != nil {
		t.Fatalf("SecondaryActivate: %v", err)
	}
	select {
	case b := <-trayClicks:
		if b != 2 {
			t.Errorf("SecondaryActivate дал кнопку %d, want 2 (средняя)", b)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SecondaryActivate не доехал")
	}
	// ContextMenu и Scroll обязаны отвечать, а не молчать в таймаут.
	if _, err := de.call(itemName, sniItemPath, sniItemIface, "ContextMenu", "ii",
		[]any{int32(0), int32(0)}); err != nil {
		t.Errorf("ContextMenu: %v", err)
	}
	if _, err := de.call(itemName, sniItemPath, sniItemIface, "Scroll", "is",
		[]any{int32(120), "vertical"}); err != nil {
		t.Errorf("Scroll: %v", err)
	}

	// ── Смена меню шлёт LayoutUpdated ────────────────────────────────────────
	updates := make(chan uint32, 4)
	de.onSignal(func(msg *dbusMessage) {
		if msg.Interface == dbusmenuIface && msg.Member == "LayoutUpdated" && len(msg.Body) > 0 {
			rev, _ := msg.Body[0].(uint32)
			select {
			case updates <- rev:
			default:
			}
		}
	})
	if err := de.addMatch("type='signal',interface='" + dbusmenuIface + "'"); err != nil {
		t.Fatalf("AddMatch: %v", err)
	}
	menu.SetItems([]widget.MenuItem{{Text: "Только выход", OnClick: func() { clicks <- "Только выход" }}})
	tray.setTrayMenuNative(menu)
	select {
	case <-updates:
	case <-time.After(3 * time.Second):
		t.Fatal("LayoutUpdated после смены меню не пришёл")
	}
	rep, err = de.call(itemName, dbusmenuPath, dbusmenuIface, "GetLayout", "iias",
		[]any{int32(0), int32(-1), []string{}})
	if err != nil {
		t.Fatalf("GetLayout после смены: %v", err)
	}
	if k := layoutKids(t, rep.Body[1].(dbusStruct)); len(k) != 1 {
		t.Errorf("после смены меню пунктов %d, want 1", len(k))
	}

	// ── Снятие иконки: имя освобождается ─────────────────────────────────────
	tray.removeTrayIcon()
	if de.nameHasOwner(itemName) {
		t.Errorf("после removeTrayIcon имя %s всё ещё занято", itemName)
	}
}

// TestTrayWithoutWatcher — без службы трея setTrayIcon отдаёт понятную ошибку,
// а не молчит и не паникует. Если трей в системе есть — проверять нечего.
func TestTrayWithoutWatcher(t *testing.T) {
	c := dialTestBus(t)
	if c.nameHasOwner(sniWatcherName) {
		t.Skip("в системе работает настоящий StatusNotifierWatcher")
	}
	tray := &linuxTray{}
	err := tray.setTrayIcon(image.NewRGBA(image.Rect(0, 0, 4, 4)), "нет трея")
	if err == nil {
		t.Fatal("без Watcher'а setTrayIcon должен вернуть ошибку")
	}
	if !errors.Is(err, errNoTrayWatcher) {
		t.Errorf("ошибка %v, want errNoTrayWatcher", err)
	}
}
