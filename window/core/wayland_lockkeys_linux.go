//go:build linux && !android

package core

// wayland_lockkeys_linux.go — клавиши-переключатели на Wayland (описание
// поведения — в lockkeys.go).
//
// Состояние переключателей знает только композитор, а клиенту оно приходит в
// wl_keyboard.modifiers (mods_locked) — и только пока у окна есть фокус
// клавиатуры. Поэтому:
//
//   - LockKeys() до первого modifiers возвращает «неизвестно»;
//   - после wl_keyboard.leave состояние снова «неизвестно»: пока окна нет в
//     фокусе, Caps Lock могут переключить где угодно, а нам не скажут;
//   - порядок wl_keyboard.enter + modifiers и xdg_toplevel.configure с
//     состоянием activated выбирает композитор: modifiers может прийти и
//     позже сообщения об активации. Активацию «вместе с состоянием»
//     (SetOnActivateLocks) окно поэтому откладывает до прихода состояния
//     (lockKeysSettler); первое modifiers нового фокуса и есть этот момент.
//
// Биты mods_locked — индексы модификаторов keymap'а; какой из них Caps, Num и
// Scroll Lock, берётся из keymap композитора (platform/keyboard/lockkeys_xkb.go). Scroll Lock на
// Wayland почти всегда не привязан к модификатору и остаётся невидимым.
//
// Менять состояние клавиатуры клиент Wayland в основном протоколе не вправе;
// SetLockKeys (wayland_lockkeys_set_linux.go) обходится виртуальной клавиатурой
// zwp_virtual_keyboard_v1, если композитор её предлагает, и подтверждает итог
// тем же modifiers, который приходит обычным путём.

import (
	"bytes"
	"sync"

	"github.com/oops1/headless-gui/v3/platform/keyboard"
)

// wlLockState — последнее состояние переключателей, присланное композитором.
type wlLockState struct {
	mu      sync.Mutex
	mods    keyboard.WlLockMods // где в mods_locked Caps, Num, Scroll
	modsSet bool                // keymap композитора уже разобран; нет — штатные Lock и Mod2
	locked  uint32              // последнее mods_locked
	known   bool                // modifiers пришло для текущего фокуса

	// Остальное содержимое последнего modifiers: SetLockKeys отдаёт его
	// виртуальной клавиатуре нетронутым, меняя только биты переключателей.
	depressed, latched, group uint32
	// changed закрывается при ближайшем modifiers, leave или закрытии окна —
	// будит ожидающего SetLockKeys. nil, пока никто не ждёт.
	changed chan struct{}
	// keymapRaw — копия keymap композитора: текст, обрезанный по первый NUL и
	// им же завершённый. Её же получает виртуальная клавиатура. nil — keymap
	// не приходил или формат не xkb_v1.
	keymapRaw []byte

	onSettled func()
}

// lkNoteKeymap принимает keymap композитора (содержимое файла wl_keyboard.keymap
// как есть): из него берутся модификаторы переключателей. Разобрать не удалось
// (nil-раскладка) — штатные Lock и Mod2. Копия сохраняется для виртуальной
// клавиатуры (SetLockKeys).
func (w *WaylandWindow) lkNoteKeymap(raw []byte) {
	m := keyboard.ParseKeymapLockMods(string(raw))
	// Композитор присылает размер вместе с завершающим нулём; на всякий случай
	// текст обрезается по первый NUL и завершается своим: композиторы разбирают
	// файл как C-строку и без нуля читали бы за его концом.
	if i := bytes.IndexByte(raw, 0); i >= 0 {
		raw = raw[:i]
	}
	cp := make([]byte, len(raw)+1)
	copy(cp, raw)
	w.lk.mu.Lock()
	w.lk.mods, w.lk.modsSet = m, true
	w.lk.keymapRaw = cp
	w.lk.mu.Unlock()
}

// lkNoteModifiers принимает wl_keyboard.modifiers. Первое состояние после
// получения фокуса сообщает окну, что активацию можно сообщать приложению.
func (w *WaylandWindow) lkNoteModifiers(depressed, latched, locked, group uint32) {
	w.lk.mu.Lock()
	first := !w.lk.known
	w.lk.locked, w.lk.known = locked, true
	w.lk.depressed, w.lk.latched, w.lk.group = depressed, latched, group
	w.lk.wakeLocked()
	fn := w.lk.onSettled
	w.lk.mu.Unlock()
	if first && fn != nil {
		fn()
	}
}

// lkNoteLeave — фокус клавиатуры ушёл: состояние больше не отслеживается.
func (w *WaylandWindow) lkNoteLeave() {
	w.lk.mu.Lock()
	w.lk.known = false
	w.lk.wakeLocked()
	w.lk.mu.Unlock()
}

// wakeLocked будит ожидающих состояния (под mu).
func (s *wlLockState) wakeLocked() {
	if s.changed != nil {
		close(s.changed)
		s.changed = nil
	}
}

// LockKeys — состояние переключателей по последнему wl_keyboard.modifiers
// (lockKeysBackend).
func (w *WaylandWindow) LockKeys() (keyboard.LockKeys, bool) {
	w.lk.mu.Lock()
	defer w.lk.mu.Unlock()
	if !w.lk.known {
		return 0, false
	}
	mods := w.lk.mods
	if !w.lk.modsSet {
		mods = keyboard.WlDefaultLockMods
	}
	return keyboard.LocksFromWlLocked(w.lk.locked, mods), true
}

// LockKeysSettled — состояние для текущего фокуса получено (lockKeysSettler).
func (w *WaylandWindow) LockKeysSettled() bool {
	w.lk.mu.Lock()
	defer w.lk.mu.Unlock()
	return w.lk.known
}

// SetOnLockKeysSettled регистрирует уведомление о первом состоянии нового
// фокуса (lockKeysSettler). Зовётся потоком событий Wayland.
func (w *WaylandWindow) SetOnLockKeysSettled(fn func()) {
	w.lk.mu.Lock()
	w.lk.onSettled = fn
	w.lk.mu.Unlock()
}
