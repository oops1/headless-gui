//go:build linux && !android

package wayland

import (
	"encoding/binary"
	"os"
	"reflect"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/oops1/headless-gui/v4/platform/keyboard"
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
		t.Fatalf("нет testdata/wslg_keymap.txt (данные лежат в репозитории рядом с тестом): %v", err)
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
	if lkNewWlWindow(t).w.SetLockKeys(keyboard.CapsLock) {
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

// ─── Активация и состояние: что окно сообщает ядру ──────────────────────────
//
// Ядро (SetOnActivateLocks) откладывает колбэк активации до первого состояния
// нового фокуса — это проверяют его тесты с фейком бэкенда
// (TestLockKeys_ActivateLocks*, в том числе срок ожидания). Здесь — что
// настоящее окно Wayland сообщает активность и состояние при любом порядке
// configure(activated), enter и modifiers.

func lkWlActivated() []byte { return wlConfigure(0, 0, xdgStateActivated) }

// lkWlEvents — что окно сообщило: активность (SetOnActivate) и уведомления о
// первом состоянии фокуса (SetOnLockKeysSettled).
type lkWlEvents struct {
	acts    []bool
	settled int
}

func lkWlWatch(t *testing.T) (*wlTestConn, *lkWlEvents) {
	t.Helper()
	c := lkNewWlWindow(t)
	ev := &lkWlEvents{}
	c.w.SetOnActivate(func(active bool) { ev.acts = append(ev.acts, active) })
	c.w.SetOnLockKeysSettled(func() { ev.settled++ })
	return c, ev
}

// lkWlWant сверяет состояние переключателей окна.
func lkWlWant(t *testing.T, w *WaylandWindow, want keyboard.LockKeys, known bool) {
	t.Helper()
	if k, ok := w.LockKeys(); ok != known || (known && k != want) {
		t.Errorf("LockKeys = %v, %v; ждал %v, %v", k, ok, want, known)
	}
	if w.LockKeysSettled() != known {
		t.Errorf("LockKeysSettled = %v, ждал %v", !known, known)
	}
}

// Композитор присылает configure(activated) РАНЬШЕ enter и modifiers (Mutter,
// KWin так бывают): активность сообщена сразу, состояние пока неизвестно; с
// modifiers — уведомление и верное состояние.
func TestWaylandLockKeys_ActivateBeforeModifiers(t *testing.T) {
	c, ev := lkWlWatch(t)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, lkWlActivated())
	if !reflect.DeepEqual(ev.acts, []bool{true}) || ev.settled != 0 {
		t.Fatalf("после configure: активность %v, уведомлений %d", ev.acts, ev.settled)
	}
	lkWlWant(t, c.w, 0, false)
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, xkbModLock|1<<keyboard.XkbModIdxMod2, 0))
	if ev.settled != 1 {
		t.Errorf("уведомлений %d, ждал 1", ev.settled)
	}
	lkWlWant(t, c.w, keyboard.CapsLock|keyboard.NumLock, true)
}

// Обратный порядок (enter и modifiers раньше configure): к активации
// состояние уже известно.
func TestWaylandLockKeys_ActivateAfterModifiers(t *testing.T) {
	c, ev := lkWlWatch(t)
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, 1<<keyboard.XkbModIdxMod2, 0))
	if ev.settled != 1 {
		t.Errorf("уведомлений %d, ждал 1", ev.settled)
	}
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, lkWlActivated())
	if !reflect.DeepEqual(ev.acts, []bool{true}) {
		t.Errorf("активность %v, ждал [true]", ev.acts)
	}
	lkWlWant(t, c.w, keyboard.NumLock, true)
}

// Возврат фокуса после Caps Lock, нажатого в другом окне: leave забывает
// состояние, новый фокус приносит новое.
func TestWaylandLockKeys_RefocusCarriesNewState(t *testing.T) {
	c, ev := lkWlWatch(t)
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, 0, 0))
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, lkWlActivated())

	c.kbd(wlKeyboardEvLeave, lkWlLeaveBody(11))
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, wlConfigure(0, 0))
	lkWlWant(t, c.w, 0, false)
	// В другом окне включили Caps Lock; возвращаемся.
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, lkWlActivated())
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, xkbModLock, 0))

	if !reflect.DeepEqual(ev.acts, []bool{true, false, true}) {
		t.Errorf("активность %v, ждал [true false true]", ev.acts)
	}
	if ev.settled != 2 {
		t.Errorf("уведомлений %d, ждал 2 (по одному на фокус)", ev.settled)
	}
	lkWlWant(t, c.w, keyboard.CapsLock, true)
}

// Композитор так и не прислал modifiers: окно состояния не сообщает (срок
// ожидания — забота ядра, TestLockKeys_ActivateLocksTimeout).
func TestWaylandLockKeys_ActivateWithoutModifiers(t *testing.T) {
	c, ev := lkWlWatch(t)
	c.w.handleEvent(c.w.toplevelID, xdgToplevelEvConfigure, lkWlActivated())
	if !reflect.DeepEqual(ev.acts, []bool{true}) || ev.settled != 0 {
		t.Errorf("активность %v, уведомлений %d", ev.acts, ev.settled)
	}
	lkWlWant(t, c.w, 0, false)
}
