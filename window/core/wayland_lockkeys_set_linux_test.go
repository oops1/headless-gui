//go:build linux && !android

package core

import (
	"encoding/binary"
	"net"
	"os"
	"reflect"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/internal/goid"
	"github.com/oops1/headless-gui/v3/platform/keyboard"
)

// SetLockKeys на Wayland через zwp_virtual_keyboard_v1. Композитор подменён
// тестом: события подаются прямо в handleEvent, запросы читаются с другого
// конца сокета. Опкоды и числа записаны буквально из virtual-keyboard-unstable-v1,
// независимо от констант в коде.

const (
	lkvkRegID   = 2   // wl_registry в тестовом окне
	lkvkMgrName = 77  // имя глобала zwp_virtual_keyboard_manager_v1
	lkvkMgrID   = 100 // id менеджера: первый newID окна (bind)
	lkvkKbdID   = 101 // id виртуальной клавиатуры: следующий
	lkvkSeatID  = 7   // wl_seat тестового окна

	lkvkKeymap = "xkb_keymap {\n\txkb_symbols \"x\" {};\n};\n"
)

// lkvkTimeout на время теста задаёт срок ожидания подтверждения.
func lkvkTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := lkVirtTimeout
	lkVirtTimeout = d
	t.Cleanup(func() { lkVirtTimeout = old })
}

// lkvkWindow — окно в фокусе клавиатуры с объявленным менеджером виртуальной
// клавиатуры, раскладкой и состоянием: нажат Shift (depressed=1), защёлкнут
// Control (latched=4), группа 1, зафиксировано locked.
func lkvkWindow(t *testing.T, locked uint32) (*Window, *wlTestConn, *rkEngine) {
	t.Helper()
	c := lkNewWlWindow(t)
	c.w.registryID = lkvkRegID
	c.w.nextID = lkvkMgrID
	c.w.gVirtualKeyboard.Store(lkvkMgrName)
	c.sendKeymap(t, lkvkKeymap)
	c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(1, 4, locked, 1))
	eng := &rkEngine{}
	return lkTestWindow(c.w, eng), c, eng
}

// lkvkCall зовёт SetLockKeys на отдельной горутине (она ждёт подтверждения).
func lkvkCall(win *Window, want keyboard.LockKeys) <-chan bool {
	ch := make(chan bool, 1)
	go func() { ch <- win.SetLockKeys(want) }()
	return ch
}

func lkvkResult(t *testing.T, ch <-chan bool) bool {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("SetLockKeys не вернулся")
		return false
	}
}

// lkvkRead читает всё, что окно отправило композитору.
//
// Запрос состоит из нескольких сообщений, и под -race они доходят с заметными
// паузами, поэтому читаем не «до тишины», а до modifiers — им запрос кончается.
func lkvkRead(t *testing.T, c *wlTestConn) ([]wlWireMsg, []int) {
	t.Helper()
	var raw []byte
	var fds []int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c.peer.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		buf, oob := make([]byte, 8192), make([]byte, 256)
		n, oobn, _, _, err := c.peer.ReadMsgUnix(buf, oob)
		raw = append(raw, buf[:n]...)
		if oobn > 0 {
			if msgs, e := syscall.ParseSocketControlMessage(oob[:oobn]); e == nil {
				for i := range msgs {
					if f, e := syscall.ParseUnixRights(&msgs[i]); e == nil {
						fds = append(fds, f...)
					}
				}
			}
		}
		// Только целые сообщения: хвост может быть ещё в пути.
		whole := 0
		for whole+8 <= len(raw) {
			size := int(binary.LittleEndian.Uint16(raw[whole+6 : whole+8]))
			if size < 8 || whole+size > len(raw) {
				break
			}
			whole += size
		}
		msgs := splitWlMsgs(t, raw[:whole])
		if lkvkIndex(msgs, lkvkKbdID, 2) >= 0 || (err != nil && !lkvkPending(err)) {
			return msgs, fds
		}
	}
	return splitWlMsgs(t, raw), fds
}

// lkvkPending — ошибка чтения означает лишь «пока ничего нет» (срок).
func lkvkPending(err error) bool {
	ne, ok := err.(net.Error)
	return ok && ne.Timeout()
}

// lkvkSilent проверяет, что окно ничего не отправило.
func lkvkSilent(t *testing.T, c *wlTestConn) {
	t.Helper()
	c.peer.SetReadDeadline(time.Now().Add(60 * time.Millisecond))
	buf := make([]byte, 256)
	if n, err := c.peer.Read(buf); err == nil {
		t.Errorf("окно отправило %d байт, ждал тишины: %+v", n, splitWlMsgs(t, buf[:n]))
	}
}

// lkvkFDText — содержимое принятого fd (позиция файла общая с отправителем,
// поэтому читаем с нуля).
func lkvkFDText(t *testing.T, fd int) []byte {
	t.Helper()
	f := os.NewFile(uintptr(fd), "vk-keymap")
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatalf("stat fd: %v", err)
	}
	b := make([]byte, st.Size())
	if _, err := f.ReadAt(b, 0); err != nil && len(b) > 0 {
		t.Fatalf("чтение fd: %v", err)
	}
	return b
}

// lkvkIndex — позиция первого сообщения obj/opcode в потоке (-1 — нет).
func lkvkIndex(msgs []wlWireMsg, obj uint32, opcode uint16) int {
	for i, m := range msgs {
		if m.obj == obj && m.opcode == opcode {
			return i
		}
	}
	return -1
}

func lkvkModifiers(t *testing.T, msgs []wlWireMsg) wlWireMsg {
	t.Helper()
	m, ok := findWlMsg(msgs, lkvkKbdID, 2)
	if !ok {
		t.Fatalf("modifiers не отправлен: %+v", msgs)
	}
	return m
}

// Главный сценарий: запрос уходит в нужном порядке с нужными числами, после
// modifiers композитора — true, а приложение не видит ни одной клавиши.
func TestLkvkSetCapsLock(t *testing.T) {
	lkvkTimeout(t, 5*time.Second)
	win, c, eng := lkvkWindow(t, 0x40) // зафиксирован Mod4: чужой бит должен уцелеть
	ch := lkvkCall(win, keyboard.CapsLock)

	msgs, fds := lkvkRead(t, c)
	// Привязка менеджера: wl_registry.bind(name, "zwp_virtual_keyboard_manager_v1", 1, id).
	bind, ok := findWlMsg(msgs, lkvkRegID, 0)
	if !ok || bind.args[0] != lkvkMgrName || bind.args[len(bind.args)-1] != lkvkMgrID {
		t.Fatalf("bind менеджера: %+v", msgs)
	}
	if bind.args[len(bind.args)-2] != 1 {
		t.Errorf("версия менеджера %d, ждал 1", bind.args[len(bind.args)-2])
	}
	// create_virtual_keyboard(seat, id) на seat окна.
	create, ok := findWlMsg(msgs, lkvkMgrID, 0)
	if !ok || !reflect.DeepEqual(create.args, []uint32{lkvkSeatID, lkvkKbdID}) {
		t.Fatalf("create_virtual_keyboard: %+v", msgs)
	}
	// keymap(format=xkb_v1, fd, size): содержимое — копия раскладки композитора
	// с завершающим нулём, размер включает его.
	km, ok := findWlMsg(msgs, lkvkKbdID, 0)
	want := lkvkKeymap + "\x00"
	if !ok || !reflect.DeepEqual(km.args, []uint32{1, uint32(len(want))}) {
		t.Fatalf("keymap: %+v", msgs)
	}
	if len(fds) != 1 {
		t.Fatalf("fd в запросах: %d, ждал 1", len(fds))
	}
	if got := lkvkFDText(t, fds[0]); string(got) != want {
		t.Errorf("содержимое keymap %q, ждал %q", got, want)
	}
	// modifiers: depressed/latched/group прежние, в locked добавлен Lock, а чужой
	// зафиксированный бит остался.
	mod := lkvkModifiers(t, msgs)
	if !reflect.DeepEqual(mod.args, []uint32{1, 4, 0x40 | 2, 1}) {
		t.Errorf("modifiers %v, ждал [1 4 66 1]", mod.args)
	}
	// Порядок: создание, раскладка, modifiers; клавиш нет.
	ic, ik, im := lkvkIndex(msgs, lkvkMgrID, 0), lkvkIndex(msgs, lkvkKbdID, 0), lkvkIndex(msgs, lkvkKbdID, 2)
	if !(ic < ik && ik < im) {
		t.Errorf("порядок запросов: create=%d keymap=%d modifiers=%d", ic, ik, im)
	}
	if lkvkIndex(msgs, lkvkKbdID, 1) >= 0 {
		t.Error("виртуальной клавиатуре отправлено key")
	}

	// Композитор применил: modifiers с теми же depressed/latched/group.
	select {
	case r := <-ch:
		t.Fatalf("SetLockKeys вернулся (%v) до modifiers композитора", r)
	case <-time.After(30 * time.Millisecond):
	}
	replied := time.Now()
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(1, 4, 0x40|2, 1))
	if !lkvkResult(t, ch) {
		t.Fatal("SetLockKeys ответил false после modifiers с нужным mods_locked")
	}
	// Ожидающий разбужен самим modifiers, а не сроком.
	if d := time.Since(replied); d > time.Second {
		t.Errorf("ответ через %v после modifiers: ожидание не разбужено", d)
	}
	if k, ok := win.LockKeys(); !ok || k != keyboard.CapsLock {
		t.Errorf("LockKeys = %v, %v; ждал caps", k, ok)
	}
	eng.run()
	if len(eng.keys) != 0 {
		t.Errorf("приложение получило клавиши: %v", eng.keys)
	}
}

// Композитор молчит (применяет modifiers только к событиям виртуальной
// клавиатуры): честный ответ false после срока.
func TestLkvkNoConfirmationIsFalse(t *testing.T) {
	lkvkTimeout(t, 60*time.Millisecond)
	win, c, _ := lkvkWindow(t, 0)
	start := time.Now()
	ch := lkvkCall(win, keyboard.CapsLock)
	if lkvkResult(t, ch) {
		t.Error("SetLockKeys ответил true без modifiers")
	}
	if d := time.Since(start); d < 50*time.Millisecond {
		t.Errorf("вернулся через %v, не дождавшись срока", d)
	}
	msgs, _ := lkvkRead(t, c)
	lkvkModifiers(t, msgs) // запрос при этом ушёл
}

// Срок по умолчанию — 300 мс, как у SetOnActivateLocks.
func TestLkvkDefaultTimeout(t *testing.T) {
	if lkVirtTimeout != 300*time.Millisecond {
		t.Errorf("lkVirtTimeout = %v, ждал 300ms", lkVirtTimeout)
	}
}

// Чужое modifiers (нажали Shift), не меняющее переключателей, не подтверждение:
// ожидание продолжается и получает следующее.
func TestLkvkUnrelatedModifiersKeepWaiting(t *testing.T) {
	lkvkTimeout(t, 5*time.Second)
	win, c, _ := lkvkWindow(t, 0)
	ch := lkvkCall(win, keyboard.CapsLock)
	lkvkRead(t, c)
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(1|4, 4, 0, 1))
	select {
	case r := <-ch:
		t.Fatalf("SetLockKeys вернулся (%v) на чужом modifiers", r)
	case <-time.After(40 * time.Millisecond):
	}
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(1, 4, 2, 1))
	if !lkvkResult(t, ch) {
		t.Error("false после подходящего modifiers")
	}
}

// Повторный вызов: ни нового создания, ни раскладки; в modifiers — свежие
// depressed/latched/group и locked из последнего modifiers.
func TestLkvkSecondCallReusesKeyboard(t *testing.T) {
	lkvkTimeout(t, 5*time.Second)
	win, c, _ := lkvkWindow(t, 0)
	ch := lkvkCall(win, keyboard.CapsLock)
	lkvkRead(t, c)
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(1, 4, 2, 1))
	if !lkvkResult(t, ch) {
		t.Fatal("первый вызов: false")
	}

	// Пока окно было в работе, пользователь нажал Alt (depressed=8), сменил группу.
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(8, 0, 2, 0))
	ch = lkvkCall(win, keyboard.CapsLock|keyboard.NumLock)
	msgs, fds := lkvkRead(t, c)
	if len(fds) != 0 || lkvkIndex(msgs, lkvkKbdID, 0) >= 0 {
		t.Errorf("повторный вызов отдал раскладку заново: %+v, fd %d", msgs, len(fds))
	}
	if lkvkIndex(msgs, lkvkMgrID, 0) >= 0 || lkvkIndex(msgs, lkvkRegID, 0) >= 0 {
		t.Errorf("повторный вызов завёл вторую клавиатуру: %+v", msgs)
	}
	if mod := lkvkModifiers(t, msgs); !reflect.DeepEqual(mod.args, []uint32{8, 0, 2 | 0x10, 0}) {
		t.Errorf("modifiers %v, ждал [8 0 18 0]", mod.args)
	}
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(8, 0, 2|0x10, 0))
	if !lkvkResult(t, ch) {
		t.Error("второй вызов: false")
	}
}

// Приведение «ровно к набору»: лишнее гасится, остальные биты не трогаются.
func TestLkvkClearsOthers(t *testing.T) {
	lkvkTimeout(t, 5*time.Second)
	win, c, _ := lkvkWindow(t, 2|0x10|0x40) // Caps, Num, и чужой Mod4
	ch := lkvkCall(win, keyboard.NumLock)
	msgs, _ := lkvkRead(t, c)
	if mod := lkvkModifiers(t, msgs); mod.args[2] != 0x10|0x40 {
		t.Errorf("locked = %#x, ждал 0x50 (Caps погашен, Num и Mod4 на месте)", mod.args[2])
	}
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(1, 4, 0x10|0x40, 1))
	if !lkvkResult(t, ch) {
		t.Error("false")
	}
}

// Уже как просили: запросов нет, ответ true.
func TestLkvkAlreadyEqualSendsNothing(t *testing.T) {
	win, c, _ := lkvkWindow(t, 2)
	if !win.SetLockKeys(keyboard.CapsLock) {
		t.Error("false, хотя Caps Lock уже включён")
	}
	lkvkSilent(t, c)
}

// Нет менеджера, нет фокуса, нет состояния, нет раскладки — false без запросов.
func TestLkvkRefusals(t *testing.T) {
	lkvkTimeout(t, 40*time.Millisecond)
	t.Run("без менеджера", func(t *testing.T) {
		win, c, _ := lkvkWindow(t, 0)
		c.w.gVirtualKeyboard.Store(0)
		if win.SetLockKeys(keyboard.CapsLock) {
			t.Error("true без zwp_virtual_keyboard_manager_v1")
		}
		lkvkSilent(t, c)
	})
	t.Run("без менеджера и состояние уже нужное", func(t *testing.T) {
		win, c, _ := lkvkWindow(t, 2)
		c.w.gVirtualKeyboard.Store(0)
		if win.SetLockKeys(keyboard.CapsLock) {
			t.Error("true без менеджера")
		}
		lkvkSilent(t, c)
	})
	t.Run("после leave", func(t *testing.T) {
		win, c, _ := lkvkWindow(t, 0)
		c.kbd(wlKeyboardEvLeave, lkWlLeaveBody(11))
		if win.SetLockKeys(keyboard.CapsLock) {
			t.Error("true для окна вне фокуса")
		}
		lkvkSilent(t, c)
	})
	t.Run("до enter и modifiers", func(t *testing.T) {
		c := lkNewWlWindow(t)
		c.w.gVirtualKeyboard.Store(lkvkMgrName)
		c.sendKeymap(t, lkvkKeymap)
		win := lkTestWindow(c.w, &rkEngine{})
		if win.SetLockKeys(keyboard.CapsLock) {
			t.Error("true при неизвестном состоянии")
		}
		lkvkSilent(t, c)
	})
	t.Run("enter без modifiers", func(t *testing.T) {
		c := lkNewWlWindow(t)
		c.w.gVirtualKeyboard.Store(lkvkMgrName)
		c.sendKeymap(t, lkvkKeymap)
		c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
		win := lkTestWindow(c.w, &rkEngine{})
		if win.SetLockKeys(keyboard.CapsLock) {
			t.Error("true при неизвестном состоянии")
		}
		lkvkSilent(t, c)
	})
	t.Run("раскладка не приходила", func(t *testing.T) {
		c := lkNewWlWindow(t)
		c.w.gVirtualKeyboard.Store(lkvkMgrName)
		c.kbd(wlKeyboardEvEnter, lkWlEnterBody(11))
		c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, 0, 0))
		win := lkTestWindow(c.w, &rkEngine{})
		if win.SetLockKeys(keyboard.CapsLock) {
			t.Error("true без раскладки (no_keymap — фатальная ошибка)")
		}
		lkvkSilent(t, c)
	})
	t.Run("нет seat", func(t *testing.T) {
		win, c, _ := lkvkWindow(t, 0)
		c.w.seatID = 0
		if win.SetLockKeys(keyboard.CapsLock) {
			t.Error("true без seat")
		}
		lkvkSilent(t, c)
	})
}

// Kana Lock в modifiers отсутствует: просили её — итог false, но Caps всё равно
// приведён. Просили Kana, ничего больше и менять нечего — запросов нет.
func TestLkvkKanaIsPartial(t *testing.T) {
	lkvkTimeout(t, 5*time.Second)
	win, c, _ := lkvkWindow(t, 0)
	ch := lkvkCall(win, keyboard.CapsLock|keyboard.KanaLock)
	msgs, _ := lkvkRead(t, c)
	if mod := lkvkModifiers(t, msgs); mod.args[2] != 2 {
		t.Errorf("locked = %#x, ждал 2: остальное применяется", mod.args[2])
	}
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(1, 4, 2, 1))
	if lkvkResult(t, ch) {
		t.Error("true, хотя Kana Lock не выставить")
	}

	win2, c2, _ := lkvkWindow(t, 0)
	if win2.SetLockKeys(keyboard.KanaLock) {
		t.Error("только Kana: true")
	}
	lkvkSilent(t, c2)
}

// Scroll Lock — только если раскладка привязала его к модификатору.
func TestLkvkScrollLock(t *testing.T) {
	lkvkTimeout(t, 5*time.Second)
	t.Run("привязан", func(t *testing.T) {
		win, c, _ := lkvkWindow(t, 0)
		c.sendKeymap(t, keymapWith(
			"	key <SCLK> { [ Scroll_Lock ] };\n", "modifier_map Mod5 { <SCLK> };"))
		c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(1, 4, 0, 1))
		ch := lkvkCall(win, keyboard.ScrollLock)
		msgs, _ := lkvkRead(t, c)
		if mod := lkvkModifiers(t, msgs); mod.args[2] != 0x80 {
			t.Errorf("locked = %#x, ждал 0x80 (Mod5)", mod.args[2])
		}
		c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(1, 4, 0x80, 1))
		if !lkvkResult(t, ch) {
			t.Error("false")
		}
	})
	t.Run("не привязан", func(t *testing.T) {
		win, c, _ := lkvkWindow(t, 0)
		ch := lkvkCall(win, keyboard.ScrollLock|keyboard.CapsLock)
		msgs, _ := lkvkRead(t, c)
		if mod := lkvkModifiers(t, msgs); mod.args[2] != 2 {
			t.Errorf("locked = %#x, ждал 2: Caps применяется, Scroll нечем", mod.args[2])
		}
		c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(1, 4, 2, 1))
		if lkvkResult(t, ch) {
			t.Error("true, хотя Scroll Lock не привязан к модификатору")
		}
	})
}

// Num Lock переставлен раскладкой на Mod3: биты берутся оттуда.
func TestLkvkMasksFromKeymap(t *testing.T) {
	lkvkTimeout(t, 5*time.Second)
	win, c, _ := lkvkWindow(t, 0)
	c.sendKeymap(t, keymapWith(
		"	key <CAPS> { [ Caps_Lock ] };\n	key <NMLK> { [ Num_Lock ] };\n",
		"modifier_map Lock { <CAPS> };\nmodifier_map Mod3 { <NMLK> };"))
	ch := lkvkCall(win, keyboard.NumLock)
	msgs, _ := lkvkRead(t, c)
	if mod := lkvkModifiers(t, msgs); mod.args[2] != 1<<5 {
		t.Errorf("locked = %#x, ждал Mod3 (0x20)", mod.args[2])
	}
	c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(1, 4, 1<<5, 1))
	if !lkvkResult(t, ch) {
		t.Error("false")
	}
}

// Раскладка отдаётся один раз; сменил её композитор — отдаётся заново, а та же
// самая, присланная повторно, не в счёт.
func TestLkvkKeymapResentOnlyOnChange(t *testing.T) {
	lkvkTimeout(t, 40*time.Millisecond)
	win, c, _ := lkvkWindow(t, 0)
	lkvkCall(win, keyboard.CapsLock)
	lkvkRead(t, c)
	time.Sleep(60 * time.Millisecond)

	c.sendKeymap(t, lkvkKeymap) // та же
	ch := lkvkCall(win, keyboard.CapsLock)
	msgs, fds := lkvkRead(t, c)
	lkvkResult(t, ch)
	if len(fds) != 0 || lkvkIndex(msgs, lkvkKbdID, 0) >= 0 {
		t.Errorf("та же раскладка отдана заново: %+v", msgs)
	}

	other := keymapWith("	key <CAPS> { [ Caps_Lock ] };\n", "modifier_map Lock { <CAPS> };")
	c.sendKeymap(t, other)
	ch = lkvkCall(win, keyboard.CapsLock)
	msgs, fds = lkvkRead(t, c)
	lkvkResult(t, ch)
	if len(fds) != 1 || lkvkIndex(msgs, lkvkKbdID, 0) < 0 {
		t.Fatalf("новая раскладка не отдана: %+v", msgs)
	}
	if got := lkvkFDText(t, fds[0]); string(got) != other+"\x00" {
		t.Errorf("отдана раскладка %q", got)
	}
	if lkvkIndex(msgs, lkvkMgrID, 0) >= 0 {
		t.Error("смена раскладки завела новую клавиатуру")
	}
	if ik, im := lkvkIndex(msgs, lkvkKbdID, 0), lkvkIndex(msgs, lkvkKbdID, 2); ik > im {
		t.Error("modifiers раньше раскладки")
	}
}

// Композитор мог прислать текст без завершающего нуля или с мусором после него:
// виртуальная клавиатура получает строку, завершённую нулём ровно один раз.
func TestLkvkKeymapIsNulTerminated(t *testing.T) {
	for _, tc := range []struct{ name, sent, want string }{
		{"без нуля", lkvkKeymap, lkvkKeymap + "\x00"},
		{"с нулём", lkvkKeymap + "\x00", lkvkKeymap + "\x00"},
		{"мусор после нуля", lkvkKeymap + "\x00junk", lkvkKeymap + "\x00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lkvkTimeout(t, 30*time.Millisecond)
			win, c, _ := lkvkWindow(t, 0)
			c.sendKeymap(t, tc.sent)
			ch := lkvkCall(win, keyboard.CapsLock)
			msgs, fds := lkvkRead(t, c)
			lkvkResult(t, ch)
			km, ok := findWlMsg(msgs, lkvkKbdID, 0)
			if !ok || len(fds) != 1 {
				t.Fatalf("keymap не отправлен: %+v", msgs)
			}
			if km.args[1] != uint32(len(tc.want)) {
				t.Errorf("размер %d, ждал %d", km.args[1], len(tc.want))
			}
			if got := lkvkFDText(t, fds[0]); string(got) != tc.want {
				t.Errorf("содержимое %q, ждал %q", got, tc.want)
			}
		})
	}
}

// Close уничтожает виртуальную клавиатуру; без неё — ничего лишнего.
func TestLkvkCloseDestroys(t *testing.T) {
	lkvkTimeout(t, 30*time.Millisecond)
	win, c, _ := lkvkWindow(t, 0)
	lkvkResult(t, lkvkCall(win, keyboard.CapsLock))
	lkvkRead(t, c)
	c.w.Close()
	msgs, _ := lkvkRead(t, c)
	if m, ok := findWlMsg(msgs, lkvkKbdID, 3); !ok || len(m.args) != 0 {
		t.Errorf("destroy не отправлен: %+v", msgs)
	}
	if win.SetLockKeys(keyboard.NumLock) {
		t.Error("SetLockKeys после Close ответил true")
	}
}

func TestLkvkCloseWithoutKeyboardSendsNoDestroy(t *testing.T) {
	_, c, _ := lkvkWindow(t, 0)
	c.w.Close()
	c.peer.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1024)
	n, _ := c.peer.Read(buf)
	for _, m := range splitWlMsgs(t, buf[:n]) {
		if m.obj == lkvkKbdID || m.obj == lkvkMgrID {
			t.Errorf("Close отправил запрос виртуальной клавиатуре: %+v", m)
		}
	}
}

// Close будит ожидающего вызов: не нужно ждать срок.
func TestLkvkCloseWakesWaiter(t *testing.T) {
	lkvkTimeout(t, 10*time.Second)
	win, c, _ := lkvkWindow(t, 0)
	ch := lkvkCall(win, keyboard.CapsLock)
	lkvkRead(t, c)
	start := time.Now()
	c.w.Close()
	if lkvkResult(t, ch) {
		t.Error("true для закрытого окна")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("вызов проснулся через %v", d)
	}
}

// Потеря фокуса во время ожидания: подтверждения уже не будет.
func TestLkvkLeaveAbortsWait(t *testing.T) {
	lkvkTimeout(t, 10*time.Second)
	win, c, _ := lkvkWindow(t, 0)
	ch := lkvkCall(win, keyboard.CapsLock)
	lkvkRead(t, c)
	start := time.Now()
	c.kbd(wlKeyboardEvLeave, lkWlLeaveBody(11))
	if lkvkResult(t, ch) {
		t.Error("true после потери фокуса")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("вызов проснулся через %v", d)
	}
}

// Позванный из цикла событий вызов отправляет запрос, но не ждёт (подтверждение
// пришло бы в этот же цикл) и отвечает false.
func TestLkvkFromEventLoopDoesNotWait(t *testing.T) {
	lkvkTimeout(t, 10*time.Second)
	win, c, _ := lkvkWindow(t, 0)
	c.w.loopGID.Store(goid.Current())
	start := time.Now()
	if win.SetLockKeys(keyboard.CapsLock) {
		t.Error("true без ожидания подтверждения")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("вызов из цикла событий ждал %v", d)
	}
	msgs, _ := lkvkRead(t, c)
	lkvkModifiers(t, msgs)
}

// Параллельные вызовы, поток modifiers и Close не гоняют гонок (-race) и не
// зависают.
func TestLkvkConcurrent(t *testing.T) {
	lkvkTimeout(t, 20*time.Millisecond)
	win, c, _ := lkvkWindow(t, 0)
	go func() { // читатель: сокет не должен переполниться
		buf := make([]byte, 65536)
		for {
			if _, _, _, _, err := c.peer.ReadMsgUnix(buf, make([]byte, 256)); err != nil {
				return
			}
		}
	}()
	stop := make(chan struct{})
	go func() {
		for i := uint32(0); ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			c.kbd(wlKeyboardEvModifiers, lkWlModifiersBody(0, 0, (i&1)<<1, 0))
			time.Sleep(time.Millisecond)
		}
	}()
	done := make(chan struct{}, 4)
	for g := 0; g < 4; g++ {
		go func(g int) {
			for i := 0; i < 5; i++ {
				win.SetLockKeys(keyboard.LockKeys(1 << (uint(g+i) % 2)))
			}
			done <- struct{}{}
		}(g)
	}
	for g := 0; g < 4; g++ {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("SetLockKeys завис")
		}
	}
	close(stop)
	c.w.Close()
}

// Биты переключателей по раскладке.
func TestLkvkMasks(t *testing.T) {
	std := keyboard.WlDefaultLockMods
	scr := keyboard.WlLockMods{Caps: 2, Num: 0x10, Scroll: 0x80}
	for _, tc := range []struct {
		name        string
		m           keyboard.WlLockMods
		want        keyboard.LockKeys
		affect, set uint32
		partial     bool
	}{
		{"ничего", std, 0, 0x12, 0, false},
		{"caps", std, keyboard.CapsLock, 0x12, 2, false},
		{"num", std, keyboard.NumLock, 0x12, 0x10, false},
		{"caps и num", std, keyboard.CapsLock | keyboard.NumLock, 0x12, 0x12, false},
		{"scroll без модификатора", std, keyboard.ScrollLock, 0x12, 0, true},
		{"scroll привязан", scr, keyboard.ScrollLock, 0x92, 0x80, false},
		{"kana", std, keyboard.KanaLock, 0x12, 0, true},
		{"caps и kana", std, keyboard.CapsLock | keyboard.KanaLock, 0x12, 2, true},
		{"caps без модификатора", keyboard.WlLockMods{Num: 0x10}, keyboard.CapsLock, 0x10, 0, true},
	} {
		affect, set, partial := lkVirtMasks(tc.m, tc.want)
		if affect != tc.affect || set != tc.set || partial != tc.partial {
			t.Errorf("%s: affect=%#x set=%#x partial=%v; ждал %#x %#x %v",
				tc.name, affect, set, partial, tc.affect, tc.set, tc.partial)
		}
	}
}

// Настоящий RunEventLoop: SetLockKeys из обработчика клавиши (то есть из цикла
// событий) не ждёт подтверждения, которое пришло бы в этот же цикл.
func TestLkvkRunEventLoopHandlerDoesNotWait(t *testing.T) {
	lkvkTimeout(t, 10*time.Second)
	win, c, _ := lkvkWindow(t, 0)
	res := make(chan bool, 1)
	var once sync.Once
	c.w.onRawKey = func(k keyboard.RawKey) {
		if k.Pressed {
			once.Do(func() { res <- win.SetLockKeys(keyboard.CapsLock) })
		}
	}
	go c.w.RunEventLoop()
	t.Cleanup(func() {
		c.w.repeat.cancel() // автоповтор нажатой клавиши не должен пережить тест
		c.w.closed.Store(true)
		c.peer.Close()
	})

	key := func(state uint32) []byte {
		b := make([]byte, 16)
		binary.LittleEndian.PutUint32(b[8:12], 30) // KEY_A
		binary.LittleEndian.PutUint32(b[12:16], state)
		return wlEvent(c.w.keyboardID, wlKeyboardEvKey, b)
	}
	c.peer.Write(key(1))
	select {
	case r := <-res:
		if r {
			t.Error("true из цикла событий")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SetLockKeys из обработчика событий ждёт подтверждения: цикл не узнан")
	}
}

// Глобал объявляется в реестре и запоминается.
func TestLkvkGlobalAnnounced(t *testing.T) {
	c := lkNewWlWindow(t)
	c.w.registryID = lkvkRegID
	b := newWlMsg(lkvkRegID, wlRegistryEvGlobal).
		putUint(lkvkMgrName).putString("zwp_virtual_keyboard_manager_v1").putUint(1).bytes()
	c.w.handleEvent(lkvkRegID, wlRegistryEvGlobal, b[8:])
	if got := c.w.gVirtualKeyboard.Load(); got != lkvkMgrName {
		t.Errorf("gVirtualKeyboard = %d, ждал %d", got, lkvkMgrName)
	}
}
