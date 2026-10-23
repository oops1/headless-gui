//go:build linux && !android

package window

import (
	"testing"
	"time"
)

// Захват клавиатуры на Wayland, на проводе: bind менеджера, inhibit_shortcuts
// на поверхность и seat окна, события active/inactive и destroy.

const wlShortcutsMgrName = "zwp_keyboard_shortcuts_inhibit_manager_v1"

// wlKeyGrabAnnounce объявляет глобал так, как это делает композитор.
func wlKeyGrabAnnounce(c *wlTestConn, name uint32) {
	c.w.registryID = 2
	body := append(wlU32(name), wlStr(wlShortcutsMgrName)...)
	c.w.handleEvent(2, wlRegistryEvGlobal, append(body, wlU32(1)...))
}

func TestWaylandKeyGrab_NoGlobal(t *testing.T) {
	c := newWlTestWindow(t)
	if c.w.SetKeyboardGrab(true) {
		t.Error("композитор без zwp_keyboard_shortcuts_inhibit_manager_v1, а SetKeyboardGrab вернул true")
	}
	if !c.w.SetKeyboardGrab(false) {
		t.Error("выключение захвата должно быть успешным")
	}
	if c.w.KeyboardGrabActive() {
		t.Error("KeyboardGrabActive true без глобала")
	}
}

func TestWaylandKeyGrab_RegistryRecordsGlobal(t *testing.T) {
	c := newWlTestWindow(t)
	wlKeyGrabAnnounce(c, 40)
	if c.w.gShortcutsInhibit != 40 {
		t.Errorf("gShortcutsInhibit = %d, ждал 40", c.w.gShortcutsInhibit)
	}
}

// inhibit_shortcuts(id, surface, seat) уходит менеджеру, привязанному по
// имени глобала; об активности говорит только событие компоновщика.
func TestWaylandKeyGrab_InhibitSent(t *testing.T) {
	c := newWlTestWindow(t)
	wlKeyGrabAnnounce(c, 40)
	if !c.w.SetKeyboardGrab(true) {
		t.Fatal("SetKeyboardGrab(true) вернул false при глобале")
	}
	msgs := splitWlMsgs(t, c.read(t))

	bind, ok := findWlMsg(msgs, 2, wlRegistryBind)
	if !ok || bind.args[0] != 40 {
		t.Fatalf("bind менеджера не отправлен или не по имени глобала: %+v", msgs)
	}
	mgr := bind.args[len(bind.args)-1] // new_id менеджера — последний аргумент
	inh, ok := findWlMsg(msgs, mgr, wlShortcutsMgrInhibit)
	if !ok {
		t.Fatalf("inhibit_shortcuts не отправлен: %+v", msgs)
	}
	if len(inh.args) != 3 || inh.args[1] != c.w.surfaceID || inh.args[2] != c.w.seatID {
		t.Errorf("inhibit_shortcuts %v, ждал (id, surface=%d, seat=%d)", inh.args, c.w.surfaceID, c.w.seatID)
	}
	if wlShortcutsMgrInhibit != 1 || wlShortcutsInhibDestroy != 0 {
		t.Error("опкоды не по протоколу keyboard-shortcuts-inhibit: inhibit_shortcuts=1, destroy=0")
	}
	if c.w.KeyboardGrabActive() {
		t.Error("KeyboardGrabActive true до события active от компоновщика")
	}

	inhibitor := inh.args[0]
	c.w.handleEvent(inhibitor, wlShortcutsEvActive, nil)
	if !c.w.KeyboardGrabActive() {
		t.Error("после active KeyboardGrabActive false")
	}
	// Фокус ушёл или компоновщик отказал.
	c.w.handleEvent(inhibitor, wlShortcutsEvInactive, nil)
	if c.w.KeyboardGrabActive() {
		t.Error("после inactive KeyboardGrabActive true")
	}
	c.w.handleEvent(inhibitor, wlShortcutsEvActive, nil)
	if !c.w.KeyboardGrabActive() {
		t.Error("возобновление active не замечено")
	}
}

// Второй запрос на тот же seat и поверхность — ошибка протокола
// (already_inhibited): окно его не повторяет.
func TestWaylandKeyGrab_NoDoubleInhibit(t *testing.T) {
	c := newWlTestWindow(t)
	wlKeyGrabAnnounce(c, 40)
	c.w.SetKeyboardGrab(true)
	first := splitWlMsgs(t, c.read(t))
	if !c.w.SetKeyboardGrab(true) {
		t.Error("повторное включение должно быть успешным")
	}
	c.peer.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	buf := make([]byte, 64)
	if n, _ := c.peer.Read(buf); n > 0 {
		t.Errorf("повторный запрос ушёл композитору (первый был %+v)", first)
	}
}

// Выключение уничтожает inhibitor, и события старого объекта больше не
// трогают состояние; новое включение создаёт новый.
func TestWaylandKeyGrab_OffDestroysAndReenables(t *testing.T) {
	c := newWlTestWindow(t)
	wlKeyGrabAnnounce(c, 40)
	c.w.SetKeyboardGrab(true)
	msgs := splitWlMsgs(t, c.read(t))
	bind, _ := findWlMsg(msgs, 2, wlRegistryBind)
	mgr := bind.args[len(bind.args)-1]
	inh, _ := findWlMsg(msgs, mgr, wlShortcutsMgrInhibit)
	old := inh.args[0]
	c.w.handleEvent(old, wlShortcutsEvActive, nil)

	if !c.w.SetKeyboardGrab(false) {
		t.Error("выключение вернуло false")
	}
	if c.w.KeyboardGrabActive() {
		t.Error("после выключения KeyboardGrabActive true")
	}
	if _, ok := findWlMsg(splitWlMsgs(t, c.read(t)), old, wlShortcutsInhibDestroy); !ok {
		t.Error("destroy inhibitor'а не отправлен")
	}
	// Запоздавшее событие старого объекта состояние не меняет.
	c.w.handleEvent(old, wlShortcutsEvActive, nil)
	if c.w.KeyboardGrabActive() {
		t.Error("событие уничтоженного inhibitor'а включило захват")
	}

	if !c.w.SetKeyboardGrab(true) {
		t.Fatal("повторное включение вернуло false")
	}
	msgs = splitWlMsgs(t, c.read(t))
	inh2, ok := findWlMsg(msgs, mgr, wlShortcutsMgrInhibit)
	if !ok || inh2.args[0] == old {
		t.Errorf("новый inhibitor не создан или переиспользован id: %+v", msgs)
	}
	// Менеджер второй раз не привязывается.
	if _, ok := findWlMsg(msgs, 2, wlRegistryBind); ok {
		t.Error("менеджер привязан повторно")
	}
}
