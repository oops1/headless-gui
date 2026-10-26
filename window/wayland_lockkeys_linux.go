//go:build linux && !android

package window

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
// Scroll Lock, берётся из keymap композитора (lockkeys_xkb.go). Scroll Lock на
// Wayland почти всегда не привязан к модификатору и остаётся невидимым.
//
// Менять состояние клавиатуры клиент Wayland не вправе: SetLockKeys на этом
// бэкенде не реализован (окно отвечает false).

import "sync"

// wlLockState — последнее состояние переключателей, присланное композитором.
type wlLockState struct {
	mu     sync.Mutex
	mods    lkWlMods // где в mods_locked Caps, Num, Scroll
	modsSet bool     // keymap композитора уже разобран; нет — штатные Lock и Mod2
	locked  uint32   // последнее mods_locked
	known   bool     // modifiers пришло для текущего фокуса

	onSettled func()
}

// lkNoteKeymap принимает keymap композитора: из него берутся модификаторы
// переключателей. Разобрать не удалось (nil-раскладка) — штатные Lock и Mod2.
func (w *WaylandWindow) lkNoteKeymap(keymap string) {
	m := lkParseKeymapMods(keymap)
	w.lk.mu.Lock()
	w.lk.mods, w.lk.modsSet = m, true
	w.lk.mu.Unlock()
}

// lkNoteModifiers принимает mods_locked из wl_keyboard.modifiers. Первое
// состояние после получения фокуса сообщает окну, что активацию можно
// сообщать приложению.
func (w *WaylandWindow) lkNoteModifiers(locked uint32) {
	w.lk.mu.Lock()
	first := !w.lk.known
	w.lk.locked, w.lk.known = locked, true
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
	w.lk.mu.Unlock()
}

// LockKeys — состояние переключателей по последнему wl_keyboard.modifiers
// (lockKeysBackend).
func (w *WaylandWindow) LockKeys() (LockKeys, bool) {
	w.lk.mu.Lock()
	defer w.lk.mu.Unlock()
	if !w.lk.known {
		return 0, false
	}
	mods := w.lk.mods
	if !w.lk.modsSet {
		mods = lkWlDefaultMods
	}
	return lkFromWlLocked(w.lk.locked, mods), true
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
