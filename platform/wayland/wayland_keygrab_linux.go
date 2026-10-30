//go:build linux && !android

package wayland

// wayland_keygrab_linux.go — захват клавиатуры на Wayland
// (zwp_keyboard_shortcuts_inhibit_manager_v1; описание поведения — в window/core/keygrab.go).
//
// Клиент Wayland клавиатуру «захватить» не может: ввод получает тот, кому его
// даёт компоновщик, а свои сочетания (Super, Alt+Tab, переключение
// рабочих столов) он обрабатывает раньше клиентов. Для окон, которым нужны ВСЕ
// клавиши (удалённый рабочий стол, виртуальная машина), протокол
// keyboard-shortcuts-inhibit-unstable-v1 позволяет попросить компоновщик
// не трогать свои сочетания, пока поверхность в фокусе:
//
//	manager = bind(zwp_keyboard_shortcuts_inhibit_manager_v1)
//	inhibitor = manager.inhibit_shortcuts(surface, seat)
//	  → событие active   — компоновщик согласился, сочетания идут окну;
//	  → событие inactive — отказал или фокус ушёл; возобновится само при
//	                       следующем получении фокуса;
//	inhibitor.destroy()  — вернуть сочетания компоновщику.
//
// Решение всегда за компоновщиком: он вправе спросить подтверждение у
// пользователя (GNOME) и оставить аварийное сочетание, которое не отдаётся
// никому (в GNOME это долгое удержание Super+Esc; пользователь может
// переключить режим и сам). Поэтому SetKeyboardGrab говорит «запрос
// отправлен», а действует ли захват сейчас, видно из KeyboardGrabbed.
//
// Глобал необязателен: его нет у части компоновщиков (WinLine, Weston до
// определённой версии) — тогда SetKeyboardGrab(true) отвечает false.
// Защищённые от захвата сочетания (Ctrl+Alt+Del у Windows-хоста, переключение
// терминалов ядром) от этого протокола не зависят.

import "sync"

// Запросы и события zwp_keyboard_shortcuts_inhibit_*_v1.
const (
	wlShortcutsMgrDestroy   = 0 // zwp_keyboard_shortcuts_inhibit_manager_v1.destroy
	wlShortcutsMgrInhibit   = 1 // inhibit_shortcuts(id, surface, seat)
	wlShortcutsInhibDestroy = 0 // zwp_keyboard_shortcuts_inhibitor_v1.destroy

	wlShortcutsEvActive   = 0 // active
	wlShortcutsEvInactive = 1 // inactive
)

// wlKeyGrab — привязка менеджера, живой запрос и его состояние.
type wlKeyGrab struct {
	mu      sync.Mutex
	mgrID   uint32 // привязанный менеджер; 0 — ещё не привязан
	inhibID uint32 // живой zwp_keyboard_shortcuts_inhibitor_v1; 0 — нет
	active  bool   // компоновщик подтвердил (событие active)
}

// SetKeyboardGrab просит компоновщик не обрабатывать свои сочетания, пока окно
// в фокусе (keyboardGrabBackend). false — глобала нет или окно ещё не создано.
func (w *WaylandWindow) SetKeyboardGrab(on bool) bool {
	if !on {
		w.kgrab.mu.Lock()
		id := w.kgrab.inhibID
		w.kgrab.inhibID, w.kgrab.active = 0, false
		w.kgrab.mu.Unlock()
		if id != 0 {
			wlLog("shortcuts_inhibitor: destroy")
			w.send(newWlMsg(id, wlShortcutsInhibDestroy), -1)
		}
		return true
	}
	if w.gShortcutsInhibit == 0 || w.surfaceID == 0 || w.seatID == 0 {
		return false
	}
	w.kgrab.mu.Lock()
	if w.kgrab.inhibID != 0 {
		w.kgrab.mu.Unlock()
		return true // протокол запрещает второй запрос на тот же seat и поверхность
	}
	if w.kgrab.mgrID == 0 {
		w.kgrab.mgrID = w.bind(w.gShortcutsInhibit, "zwp_keyboard_shortcuts_inhibit_manager_v1", 1)
	}
	mgr := w.kgrab.mgrID
	id := w.newID()
	w.kgrab.inhibID = id
	w.kgrab.mu.Unlock()
	wlLog("shortcuts_inhibitor: inhibit_shortcuts")
	w.send(newWlMsg(mgr, wlShortcutsMgrInhibit).putUint(id).putUint(w.surfaceID).putUint(w.seatID), -1)
	return true
}

// KeyboardGrabActive — компоновщик подтвердил захват и окно в фокусе
// (keyboardGrabQuerier).
func (w *WaylandWindow) KeyboardGrabActive() bool {
	w.kgrab.mu.Lock()
	defer w.kgrab.mu.Unlock()
	return w.kgrab.active
}

// keyGrabEvent разбирает события нашего inhibitor'а. true — событие
// принадлежало ему.
func (w *WaylandWindow) keyGrabEvent(obj uint32, opcode uint16) bool {
	w.kgrab.mu.Lock()
	defer w.kgrab.mu.Unlock()
	if obj == 0 || obj != w.kgrab.inhibID {
		return false
	}
	switch opcode {
	case wlShortcutsEvActive:
		w.kgrab.active = true
	case wlShortcutsEvInactive:
		w.kgrab.active = false
	}
	return true
}
