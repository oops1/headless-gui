//go:build linux && !android

package core

// wayland_lockkeys_set_linux.go — SetLockKeys на Wayland через
// zwp_virtual_keyboard_v1 (virtual-keyboard-unstable-v1; описание поведения —
// в lockkeys.go).
//
// В основном протоколе клиент состояние клавиатуры менять не вправе. Но
// компоновщик может предложить расширение, которым клиент заводит ВИРТУАЛЬНУЮ
// клавиатуру на сиденье и подаёт ей раскладку и модификаторы:
//
//	manager = bind(zwp_virtual_keyboard_manager_v1)
//	kbd     = manager.create_virtual_keyboard(seat)
//	kbd.keymap(xkb_v1, fd, size)          — раскладка: ОБЯЗАТЕЛЬНО до остального
//	kbd.modifiers(depressed, latched, locked, group)
//	kbd.destroy()
//
// Клавиш виртуальной клавиатуре мы НЕ посылаем (key): Caps Lock «нажимается» не
// клавишей, а выставлением бита в mods_locked. Отсюда два следствия. Во-первых,
// композитор не рассылает окнам wl_keyboard.key, и приложение не видит
// KeyDown/KeyUp — а значит, нет петли «сервер попросил Caps → клиент нажал Caps
// → ушло серверу». Во-вторых, подтверждение приходит не как ответ на запрос, а
// как обычное wl_keyboard.modifiers, которое композитор рассылает окну в фокусе,
// если применил наши биты к сиденью.
//
// Раскладка — та же, что композитор прислал нам в wl_keyboard.keymap (копия
// лежит в w.lk.keymapRaw): индексы модификаторов виртуальной клавиатуры должны
// совпадать с теми, по которым мы читаем mods_locked. Виртуальная клавиатура
// одна на окно: создаётся при первом SetLockKeys и живёт до Close; раскладка
// отдаётся один раз (и заново — только если композитор сменил свою).
//
// Ошибки протокола. Любая ошибка на объекте виртуальной клавиатуры
// (unauthorized у менеджера, no_keymap у клавиатуры) композитор присылает как
// wl_display.error и рвёт соединение; движок на такое событие помечает окно
// закрытым (handleEvent). Поэтому запросы шлются только в порядке, исключающем
// no_keymap, и только если композитор сам объявил менеджера. Композитор вправе
// отказать конкретному клиенту (unauthorized), и тогда окно закроется: это риск
// протокола, на стороне клиента не снимаемый — «пробного» вызова, не ломающего
// соединение, в нём нет.

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"

	"github.com/oops1/headless-gui/v3/internal/goid"
	"github.com/oops1/headless-gui/v3/platform/keyboard"
)

// Запросы virtual-keyboard-unstable-v1 и формат keymap.
const (
	wlVkbdMgrCreate     = 0 // zwp_virtual_keyboard_manager_v1.create_virtual_keyboard(seat, id)
	wlVkbdKeymap        = 0 // zwp_virtual_keyboard_v1.keymap(format, fd, size)
	wlVkbdModifiers     = 2 // zwp_virtual_keyboard_v1.modifiers(depressed, latched, locked, group)
	wlVkbdDestroy       = 3 // zwp_virtual_keyboard_v1.destroy
	wlKeymapFormatXkbV1 = 1 // WL_KEYBOARD_KEYMAP_FORMAT_XKB_V1
)

// lkVirtTimeout — сколько SetLockKeys ждёт wl_keyboard.modifiers с новым
// состоянием (как lkSettleTimeout у SetOnActivateLocks).
var lkVirtTimeout = 300 * time.Millisecond

// wlVirtKbd — виртуальная клавиатура окна.
type wlVirtKbd struct {
	// callMu сериализует SetLockKeys целиком, вместе с ожиданием: два вызова
	// подряд не должны путать «следующий modifiers» друг друга.
	callMu sync.Mutex

	// mu охраняет объекты и порядок запросов к ним; Close берёт только его, а не
	// callMu, чтобы не ждать чужого SetLockKeys.
	mu    sync.Mutex
	mgrID uint32 // привязанный менеджер; 0 — ещё не привязан
	id    uint32 // живая zwp_virtual_keyboard_v1; 0 — нет
	sent  []byte // раскладка, уже отданная этой клавиатуре

	// dead — окно закрыто: новых запросов нет, ожидающие просыпаются.
	dead atomic.Bool
}

// wlVkPlan — снимок состояния, по которому строится запрос.
type wlVkPlan struct {
	keymap             []byte // раскладка композитора
	depressed, latched uint32 // из последнего wl_keyboard.modifiers
	locked, group      uint32
	affect, set        uint32 // биты mods_locked под управлением SetLockKeys и из них включаемые
	partial            bool   // часть запрошенного сделать нельзя (Kana, клавиша без модификатора)
}

// lkVirtMasks переводит набор want в биты mods_locked: affect — биты
// переключателей, которыми SetLockKeys распоряжается (их он и включает, и
// гасит), set — из них те, что должны гореть. partial — просят то, чего через
// модификаторы не сделать: Kana Lock (его в mods_locked нет вовсе) и клавишу,
// не привязанную к модификатору раскладки (Scroll Lock почти везде). Клавиша
// без модификатора, которую просят выключить, не ошибка: она и так «выключена».
func lkVirtMasks(m keyboard.WlLockMods, want keyboard.LockKeys) (affect, set uint32, partial bool) {
	for _, k := range [...]struct {
		flag keyboard.LockKeys
		mask uint32
	}{{keyboard.CapsLock, m.Caps}, {keyboard.NumLock, m.Num}, {keyboard.ScrollLock, m.Scroll}} {
		if k.mask == 0 {
			if want&k.flag != 0 {
				partial = true
			}
			continue
		}
		affect |= k.mask
		if want&k.flag != 0 {
			set |= k.mask
		}
	}
	if want&keyboard.KanaLock != 0 {
		partial = true
	}
	return affect, set, partial
}

// lkVirtPlan снимает состояние для запроса. false — менять нечем: окно не в
// фокусе клавиатуры или состояние ещё неизвестно (нет modifiers для текущего
// фокуса), либо композитор не присылал keymap, которую нужно отдать.
func (w *WaylandWindow) lkVirtPlan(want keyboard.LockKeys) (wlVkPlan, bool) {
	w.lk.mu.Lock()
	defer w.lk.mu.Unlock()
	if !w.lk.known || w.lk.keymapRaw == nil {
		return wlVkPlan{}, false
	}
	m := w.lk.mods
	if !w.lk.modsSet {
		m = keyboard.WlDefaultLockMods
	}
	p := wlVkPlan{
		keymap:    w.lk.keymapRaw,
		depressed: w.lk.depressed, latched: w.lk.latched,
		locked: w.lk.locked, group: w.lk.group,
	}
	p.affect, p.set, p.partial = lkVirtMasks(m, want)
	return p, true
}

// SetLockKeys приводит переключатели к набору want через виртуальную
// клавиатуру (lockKeysSetter). true — композитор прислал wl_keyboard.modifiers,
// в котором затронутые биты mods_locked стали такими, как просили (до
// lkVirtTimeout, 300 мс). false — не получилось или не удалось подтвердить:
//
//   - композитор не объявил zwp_virtual_keyboard_manager_v1, окно не в фокусе
//     клавиатуры, состояние ещё неизвестно: ни одного запроса;
//   - просили Kana Lock либо клавишу без модификатора в раскладке (Scroll Lock):
//     остальное всё равно применяется, итог false;
//   - композитор применяет modifiers виртуальной клавиатуры только к её
//     собственным событиям и состояние сиденья не изменил: честный ответ false.
//
// Ждёт подтверждения до 300 мс, поэтому звать надо из горутины приложения, не
// из обработчиков событий окна: подтверждение приходит в цикл событий, и ждать
// его оттуда нельзя. Позванный из цикла событий, вызов запрос отправляет, но
// не ждёт и отвечает false.
func (w *WaylandWindow) SetLockKeys(want keyboard.LockKeys) bool {
	name := w.gVirtualKeyboard.Load()
	if name == 0 || w.seatID == 0 || w.vk.dead.Load() || w.closed.Load() {
		return false
	}
	w.vk.callMu.Lock()
	defer w.vk.callMu.Unlock()

	p, ok := w.lkVirtPlan(want)
	if !ok {
		return false
	}
	if p.locked&p.affect == p.set {
		// Все управляемые биты уже такие, как просили: сообщать композитору
		// нечего (и виртуальную клавиатуру заводить незачем).
		return !p.partial
	}
	if !w.vkApply(name, p) {
		return false
	}
	if goid.Current() == w.loopGID.Load() {
		return false
	}
	return w.lkVirtWait(p.affect, p.set) && !p.partial
}

// vkApply заводит виртуальную клавиатуру (если её ещё нет), отдаёт ей раскладку
// (если не отдавали или она сменилась) и шлёт modifiers. false — запрос не ушёл.
func (w *WaylandWindow) vkApply(mgrName uint32, p wlVkPlan) bool {
	w.vk.mu.Lock()
	defer w.vk.mu.Unlock()
	if w.vk.dead.Load() {
		return false
	}
	if w.vk.id == 0 {
		if w.vk.mgrID == 0 {
			w.vk.mgrID = w.bind(mgrName, "zwp_virtual_keyboard_manager_v1", 1)
		}
		id := w.newID()
		if err := w.send(newWlMsg(w.vk.mgrID, wlVkbdMgrCreate).putUint(w.seatID).putUint(id), -1); err != nil {
			wlLog("virtual_keyboard: create: %v", err)
			return false
		}
		wlLog("virtual_keyboard: create_virtual_keyboard")
		w.vk.id, w.vk.sent = id, nil
	}
	if !bytes.Equal(w.vk.sent, p.keymap) {
		// Раскладка обязана предшествовать modifiers (иначе no_keymap — а это
		// фатальная ошибка соединения): не ушла — modifiers не шлём.
		if err := w.vkSendKeymap(w.vk.id, p.keymap); err != nil {
			wlLog("virtual_keyboard: keymap: %v", err)
			return false
		}
		w.vk.sent = p.keymap
	}
	locked := p.locked&^p.affect | p.set
	wlLog("virtual_keyboard: modifiers locked=%#x", locked)
	err := w.send(newWlMsg(w.vk.id, wlVkbdModifiers).
		putUint(p.depressed).putUint(p.latched).putUint(locked).putUint(p.group), -1)
	return err == nil
}

// vkSendKeymap отдаёт раскладку виртуальной клавиатуре через новый memfd:
// формат xkb_v1, размер — длина текста вместе с завершающим нулём (keymapRaw
// им уже завершён), как у композитора.
func (w *WaylandWindow) vkSendKeymap(id uint32, keymap []byte) error {
	fd, err := unix.MemfdCreate("headless-gui-vkbd-keymap", unix.MFD_CLOEXEC)
	if err != nil {
		return fmt.Errorf("memfd_create: %w", err)
	}
	defer unix.Close(fd) // sendmsg уже передал композитору свою копию
	for off := 0; off < len(keymap); {
		n, err := unix.Write(fd, keymap[off:])
		if err != nil {
			return fmt.Errorf("запись keymap: %w", err)
		}
		off += n
	}
	return w.send(newWlMsg(id, wlVkbdKeymap).putUint(wlKeymapFormatXkbV1).putUint(uint32(len(keymap))), fd)
}

// lkVirtWait ждёт, пока в последнем wl_keyboard.modifiers биты affect не станут
// равны set. К началу ожидания они заведомо другие (иначе запрос не уходит), так
// что совпадение — это modifiers, пришедшее уже после него; чужие modifiers
// (нажали Shift) на состоянии переключателей не сказываются, и ожидание
// продолжается. Не пришло за lkVirtTimeout, пропал фокус или закрыли окно —
// false. Не вызывать из цикла событий.
func (w *WaylandWindow) lkVirtWait(affect, set uint32) bool {
	timer := time.NewTimer(lkVirtTimeout)
	defer timer.Stop()
	for {
		w.lk.mu.Lock()
		if !w.lk.known || w.vk.dead.Load() {
			w.lk.mu.Unlock()
			return false
		}
		if w.lk.locked&affect == set {
			w.lk.mu.Unlock()
			return true
		}
		if w.lk.changed == nil {
			w.lk.changed = make(chan struct{})
		}
		ch := w.lk.changed
		w.lk.mu.Unlock()

		select {
		case <-ch:
		case <-timer.C:
			// Последний взгляд: событие могло прийти одновременно со сроком.
			w.lk.mu.Lock()
			ok := w.lk.known && w.lk.locked&affect == set
			w.lk.mu.Unlock()
			return ok
		}
	}
}

// closeVirtualKeyboard уничтожает виртуальную клавиатуру и будит ожидающего
// SetLockKeys. Зовётся из Close.
func (w *WaylandWindow) closeVirtualKeyboard() {
	w.vk.mu.Lock()
	w.vk.dead.Store(true)
	id := w.vk.id
	w.vk.id = 0
	if id != 0 {
		wlLog("virtual_keyboard: destroy")
		w.send(newWlMsg(id, wlVkbdDestroy), -1)
	}
	w.vk.mu.Unlock()
	// Ожидающий проверяет dead под lk.mu, закрывающий будит под ним же:
	// пробуждение не проскочит мимо.
	w.lk.mu.Lock()
	w.lk.wakeLocked()
	w.lk.mu.Unlock()
}
