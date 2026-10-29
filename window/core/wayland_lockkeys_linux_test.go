//go:build linux && !android

package core

import (
	"encoding/binary"
	"os"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/oops1/headless-gui/v3/platform/keyboard"
)

// Клавиши-переключатели на Wayland: состояние из wl_keyboard.modifiers, индексы
// модификаторов — из keymap композитора, активация вместе с состоянием.
// Композитор подменён тестом: события подаются прямо в handleEvent.

// lkWlModifiersBody — тело wl_keyboard.modifiers(serial, depressed, latched,
// locked, group).
func lkWlModifiersBody(depressed, latched, locked, group uint32) []byte {
	b := make([]byte, 20)
	binary.LittleEndian.PutUint32(b[0:4], 1)
	binary.LittleEndian.PutUint32(b[4:8], depressed)
	binary.LittleEndian.PutUint32(b[8:12], latched)
	binary.LittleEndian.PutUint32(b[12:16], locked)
	binary.LittleEndian.PutUint32(b[16:20], group)
	return b
}

// lkWlEnterBody — wl_keyboard.enter(serial, surface, keys[]); leave — без массива.
func lkWlEnterBody(surface uint32) []byte {
	b := make([]byte, 12)
	binary.LittleEndian.PutUint32(b[0:4], 1)
	binary.LittleEndian.PutUint32(b[4:8], surface)
	return b
}

func lkWlLeaveBody(surface uint32) []byte { return lkWlEnterBody(surface)[:8] }

// lkNewWlWindow — окно с клавиатурой, ещё не получавшее фокуса.
func lkNewWlWindow(t *testing.T) *wlTestConn {
	t.Helper()
	c := newWlTestWindow(t)
	c.w.keyboardID = 21
	return c
}

func (c *wlTestConn) kbd(opcode uint16, body []byte) {
	c.w.handleEvent(c.w.keyboardID, opcode, body)
}

// sendKeymap подаёт keymap так, как это делает композитор: fd с текстом и размер.
func (c *wlTestConn) sendKeymap(t *testing.T, text string) {
	t.Helper()
	fd, err := unix.MemfdCreate("lk-keymap", unix.MFD_CLOEXEC)
	if err != nil {
		t.Skipf("memfd_create: %v", err)
	}
	// Без os.NewFile: у него финализатор закрыл бы fd второй раз, уже после
	// того, как handleKeyboard его закрыл, и номер к тому времени мог достаться
	// чужому файлу.
	if _, err := unix.Write(fd, []byte(text)); err != nil {
		t.Fatalf("запись keymap: %v", err)
	}
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b[0:4], 1) // xkb_v1
	binary.LittleEndian.PutUint32(b[4:8], uint32(len(text)))
	c.w.rxFDs = append(c.w.rxFDs, fd)
	c.kbd(wlKeyboardEvKeymap, b) // закрывает fd сам
}

// До первого modifiers состояния нет: это «неизвестно», а не «ничего не включено».
func TestWaylandLockKeys_UnknownBeforeModifiers(t *testing.T) {
	c := lkNewWlWindow(t)
	if k, ok := c.w.LockKeys(); ok || k != 0 {
		t.Errorf("до modifiers LockKeys = %v, %v", k, ok)
	}
	if c.w.LockKeysSettled() {
		t.Error("LockKeysSettled до modifiers")
	}
	// Фокус получен, но modifiers ещё нет: по-прежнему неизвестно.
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	if _, ok := c.w.LockKeys(); ok {
		t.Error("enter без modifiers сделал состояние известным")
	}
}

// Штатные индексы без keymap: Caps — Lock, Num — Mod2; чужие биты игнорируются.
func TestWaylandLockKeys_DefaultMods(t *testing.T) {
	c := lkNewWlWindow(t)
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	for _, tc := range []struct {
		name                       string
		depressed, latched, locked uint32
		want                       keyboard.LockKeys
	}{
		{"ничего", 0, 0, 0, 0},
		{"Caps", 0, 0, xkbModLock, keyboard.CapsLock},
		{"Num", 0, 0, 1 << keyboard.XkbModIdxMod2, keyboard.NumLock},
		{"оба", 0, 0, xkbModLock | 1<<keyboard.XkbModIdxMod2, keyboard.CapsLock | keyboard.NumLock},
		{"зажатый Shift и Ctrl — не переключатели", 1 | 4, 0, 0, 0},
		{"зафиксирован только Mod4", 0, 0, 1 << keyboard.XkbModIdxMod4, 0},
		{"Caps вместе с нажатым Shift", 1, 0, xkbModLock, keyboard.CapsLock},
		{"Caps защёлкнут (latched), а не зафиксирован", 0, xkbModLock, 0, 0},
	} {
		c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(tc.depressed, tc.latched, tc.locked, 0))
		got, ok := c.w.LockKeys()
		if !ok || got != tc.want {
			t.Errorf("%s: LockKeys = %v, %v; ждал %v", tc.name, got, ok, tc.want)
		}
	}
}

// keymap композитора определяет, какой бит чей: Num Lock переставлен на Mod3,
// Scroll Lock — на Mod5; штатный Mod2 после этого — просто чужой бит.
func TestWaylandLockKeys_MasksFromKeymap(t *testing.T) {
	c := lkNewWlWindow(t)
	c.sendKeymap(t, keymapWith(
		"	key <CAPS> { [ Caps_Lock ] };\n	key <NMLK> { [ Num_Lock ] };\n	key <SCLK> { [ Scroll_Lock ] };\n",
		"modifier_map Lock { <CAPS> };\nmodifier_map Mod3 { <NMLK> };\nmodifier_map Mod5 { <SCLK> };"))
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))

	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, 1<<keyboard.XkbModIdxMod3, 0))
	if got, _ := c.w.LockKeys(); got != keyboard.NumLock {
		t.Errorf("Mod3: %v, ждал num", got)
	}
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, 1<<keyboard.XkbModIdxMod2, 0))
	if got, _ := c.w.LockKeys(); got != 0 {
		t.Errorf("Mod2 при Num на Mod3: %v, ждал пусто", got)
	}
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, 1<<keyboard.XkbModIdxMod5|xkbModLock, 0))
	if got, _ := c.w.LockKeys(); got != keyboard.ScrollLock|keyboard.CapsLock {
		t.Errorf("Mod5+Lock: %v, ждал caps|scroll", got)
	}
}

// Настоящий keymap Weston из WSLg проходит тот же путь.
func TestWaylandLockKeys_RealKeymap(t *testing.T) {
	data, err := os.ReadFile("testdata/wslg_keymap.txt")
	if err != nil {
		t.Skipf("нет testdata/wslg_keymap.txt: %v", err)
	}
	c := lkNewWlWindow(t)
	c.sendKeymap(t, string(data))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, 2|0x10, 0))
	if got, ok := c.w.LockKeys(); !ok || got != keyboard.CapsLock|keyboard.NumLock {
		t.Errorf("LockKeys = %v, %v", got, ok)
	}
}

// Фокус ушёл — состояние снова неизвестно: пока окна нет в фокусе, Caps Lock
// могут переключить где угодно. Вернулся — известно по новому modifiers.
func TestWaylandLockKeys_LeaveForgets(t *testing.T) {
	c := lkNewWlWindow(t)
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, xkbModLock, 0))
	if got, ok := c.w.LockKeys(); !ok || got != keyboard.CapsLock {
		t.Fatalf("LockKeys = %v, %v", got, ok)
	}
	c.kbd(wlKeyboardEvLeave, lkWlLeaveBody(11))
	if k, ok := c.w.LockKeys(); ok || k != 0 {
		t.Errorf("после leave LockKeys = %v, %v", k, ok)
	}
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, 1<<keyboard.XkbModIdxMod2, 0))
	if got, ok := c.w.LockKeys(); !ok || got != keyboard.NumLock {
		t.Errorf("после возврата LockKeys = %v, %v; ждал num", got, ok)
	}
}

// Без zwp_virtual_keyboard_manager_v1 менять состояние клавиатуры клиент
// Wayland не вправе (с ним — wayland_lockkeys_set_linux_test.go).
func TestWaylandLockKeys_SetNotSupported(t *testing.T) {
	win := lkTestWindow(lkNewWlWindow(t).w, &rkEngine{})
	if win.SetLockKeys(keyboard.CapsLock) {
		t.Error("SetLockKeys на Wayland ответил true")
	}
}

// Состояние первого modifiers нового фокуса уведомляет окно ровно один раз за
// фокус, а не на каждое изменение.
func TestWaylandLockKeys_SettledNotifiedOncePerFocus(t *testing.T) {
	c := lkNewWlWindow(t)
	n := 0
	c.w.SetOnLockKeysSettled(func() { n++ })
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, 0, 0))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, xkbModLock, 0))
	if n != 1 {
		t.Errorf("уведомлений %d, ждал 1", n)
	}
	c.kbd(wlKeyboardEvLeave, lkWlLeaveBody(11))
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, 0, 0))
	if n != 2 {
		t.Errorf("после нового фокуса уведомлений %d, ждал 2", n)
	}
}

// ─── Активация вместе с состоянием, сквозь настоящее окно ───────────────────

func lkWlActivated() []byte { return wlConfigure(0, 0, xdgStateActivated) }

type lkWlCollector struct {
	got []lkActCall
}

func lkWaylandWindow(t *testing.T) (*Window, *wlTestConn, *rkEngine, *lkWlCollector) {
	t.Helper()
	c := lkNewWlWindow(t)
	eng := &rkEngine{}
	win := lkTestWindow(c.w, eng)
	col := &lkWlCollector{}
	win.SetOnActivateLocks(func(active bool, locks keyboard.LockKeys, known bool) {
		col.got = append(col.got, lkActCall{active, locks, known})
	})
	return win, c, eng, col
}

// Композитор присылает configure(activated) РАНЬШЕ enter и modifiers (Mutter,
// KWin так бывают): колбэк ждёт состояние и получает уже верное.
func TestWaylandLockKeys_ActivateBeforeModifiers(t *testing.T) {
	_, c, eng, col := lkWaylandWindow(t)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, lkWlActivated())
	eng.run()
	if len(col.got) != 0 {
		t.Fatalf("колбэк вызван до modifiers: %v", col.got)
	}
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, xkbModLock|1<<keyboard.XkbModIdxMod2, 0))
	eng.run()
	if want := []lkActCall{{true, keyboard.CapsLock | keyboard.NumLock, true}}; !reflect.DeepEqual(col.got, want) {
		t.Errorf("%v, ждал %v", col.got, want)
	}
}

// Обратный порядок (enter и modifiers раньше configure): без ожидания.
func TestWaylandLockKeys_ActivateAfterModifiers(t *testing.T) {
	_, c, eng, col := lkWaylandWindow(t)
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, 1<<keyboard.XkbModIdxMod2, 0))
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, lkWlActivated())
	eng.run()
	if want := []lkActCall{{true, keyboard.NumLock, true}}; !reflect.DeepEqual(col.got, want) {
		t.Errorf("%v, ждал %v", col.got, want)
	}
	// Повторные configure того же состояния (при каждом ресайзе) не множат вызовы.
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, lkWlActivated())
	eng.run()
	if len(col.got) != 1 {
		t.Errorf("повторный configure: %v", col.got)
	}
}

// Возврат фокуса после Caps Lock, нажатого в другом окне: приложение получает
// новое состояние, а не то, что было до ухода.
func TestWaylandLockKeys_RefocusCarriesNewState(t *testing.T) {
	_, c, eng, col := lkWaylandWindow(t)
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, 0, 0))
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, lkWlActivated())
	eng.run()

	c.kbd(wlKeyboardEvLeave, lkWlLeaveBody(11))
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(0, 0))
	eng.run()
	// В другом окне включили Caps Lock; возвращаемся.
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, lkWlActivated())
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, xkbModLock, 0))
	eng.run()

	if len(col.got) != 3 {
		t.Fatalf("вызовов %d: %v", len(col.got), col.got)
	}
	if col.got[1].active || col.got[1].known {
		t.Errorf("при потере фокуса: %v (состояние должно быть неизвестно)", col.got[1])
	}
	if want := (lkActCall{true, keyboard.CapsLock, true}); col.got[2] != want {
		t.Errorf("при возврате: %v, ждал %v", col.got[2], want)
	}
}

// Композитор так и не прислал modifiers: колбэк приходит по сроку с known=false.
func TestWaylandLockKeys_ActivateWithoutModifiersTimesOut(t *testing.T) {
	old := lkSettleTimeout
	lkSettleTimeout = 20 * time.Millisecond
	t.Cleanup(func() { lkSettleTimeout = old })
	_, c, eng, col := lkWaylandWindow(t)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, lkWlActivated())
	deadline := time.Now().Add(3 * time.Second)
	for len(col.got) == 0 && time.Now().Before(deadline) {
		eng.run()
		time.Sleep(2 * time.Millisecond)
	}
	if want := []lkActCall{{true, 0, false}}; !reflect.DeepEqual(col.got, want) {
		t.Errorf("%v, ждал %v", col.got, want)
	}
}
