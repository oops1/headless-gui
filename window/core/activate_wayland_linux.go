//go:build linux && !android

package core

// activate_wayland_linux.go — вывод окна на передний план на Wayland
// (xdg-activation-v1, задание PixGo E14).
//
// Клиент не вправе сам поднять своё окно: это решает компоновщик, и доверяет
// он только токену, который сам же выдал. Протокол:
//
//	manager = bind(xdg_activation_v1)
//	manager.activate(token, surface)        // есть токен от запустившего
//
// Токен приходит от запустившего процесса в XDG_ACTIVATION_TOKEN
// (TakeActivationToken), второй процесс передаёт его основному, тот зовёт
// ActivateWithToken. Если токена нет, окно просит его у компоновщика само —
// под свой последний ввод (самоактивация):
//
//	t = manager.get_activation_token()
//	t.set_serial(serial, seat)              // serial последнего ввода окна
//	t.set_surface(surface); t.set_app_id(id)
//	t.commit()                              // → событие done(token)
//	manager.activate(token, surface); t.destroy()
//
// Компоновщик без глобала (WinLine) получает false: поднять окно нечем.
// Признака свёрнутости у xdg_toplevel нет, поэтому IsMinimized всегда false.

import (
	"sync"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// Запросы и события xdg_activation_v1 / xdg_activation_token_v1.
const (
	xdgActivationGetToken = 1 // xdg_activation_v1.get_activation_token(id)
	xdgActivationActivate = 2 // xdg_activation_v1.activate(token, surface)

	xdgActTokenSetSerial  = 0 // set_serial(serial, seat)
	xdgActTokenSetAppID   = 1 // set_app_id(app_id)
	xdgActTokenSetSurface = 2 // set_surface(surface)
	xdgActTokenCommit     = 3 // commit
	xdgActTokenDestroy    = 4 // destroy

	xdgActTokenEvDone = 0 // done(token)
)

var _ native.ActivateBackend = (*WaylandWindow)(nil)

// wlActivation — привязка менеджера и токены, которые ждут ответа.
type wlActivation struct {
	mu      sync.Mutex
	mgrID   uint32          // привязанный xdg_activation_v1; 0 — ещё не привязан
	pending map[uint32]bool // xdg_activation_token_v1, ждущие done
}

// activationManager привязывает xdg_activation_v1 при первом обращении.
func (w *WaylandWindow) activationManager() (uint32, bool) {
	if w.gActivation == 0 || w.surfaceID == 0 {
		return 0, false
	}
	w.activation.mu.Lock()
	defer w.activation.mu.Unlock()
	if w.activation.mgrID == 0 {
		w.activation.mgrID = w.bind(w.gActivation, "xdg_activation_v1", 1)
	}
	return w.activation.mgrID, true
}

// Activate просит у компоновщика токен под последний ввод окна и по его ответу
// активирует окно (ответ придёт в цикле событий). true — запрос отправлен;
// false — компоновщик не объявил xdg_activation_v1. Без свежего ввода токен
// выдаётся без serial'а, и компоновщик обычно лишь подсвечивает окно.
// Реализует native.ActivateBackend.
func (w *WaylandWindow) Activate() bool {
	mgr, ok := w.activationManager()
	if !ok {
		return false
	}
	tok := w.newID()
	w.activation.mu.Lock()
	if w.activation.pending == nil {
		w.activation.pending = map[uint32]bool{}
	}
	w.activation.pending[tok] = true
	w.activation.mu.Unlock()

	wlLog("xdg_activation: get_activation_token")
	w.send(newWlMsg(mgr, xdgActivationGetToken).putUint(tok), -1)
	if serial := w.inputSerial.Load(); serial != 0 && w.seatID != 0 {
		w.send(newWlMsg(tok, xdgActTokenSetSerial).putUint(serial).putUint(w.seatID), -1)
	}
	if w.appID != "" {
		w.send(newWlMsg(tok, xdgActTokenSetAppID).putString(w.appID), -1)
	}
	w.send(newWlMsg(tok, xdgActTokenSetSurface).putUint(w.surfaceID), -1)
	w.send(newWlMsg(tok, xdgActTokenCommit), -1)
	return true
}

// ActivateWithToken активирует окно токеном запустившего процесса
// (xdg_activation_v1.activate). Пустой токен — как Activate. true — запрос
// отправлен, решение за компоновщиком; false — нет глобала. Реализует
// native.ActivateBackend.
func (w *WaylandWindow) ActivateWithToken(token string) bool {
	if token == "" {
		return w.Activate()
	}
	mgr, ok := w.activationManager()
	if !ok {
		return false
	}
	wlLog("xdg_activation: activate(token)")
	w.send(newWlMsg(mgr, xdgActivationActivate).putString(token).putUint(w.surfaceID), -1)
	return true
}

// activationEvent разбирает события токенов, выданных по нашей просьбе:
// done(token) → activate(token, surface) и destroy токена. true — событие
// принадлежало токену.
func (w *WaylandWindow) activationEvent(obj uint32, opcode uint16, b []byte) bool {
	w.activation.mu.Lock()
	if !w.activation.pending[obj] {
		w.activation.mu.Unlock()
		return false
	}
	if opcode != xdgActTokenEvDone || len(b) < 4 {
		w.activation.mu.Unlock()
		return true
	}
	delete(w.activation.pending, obj)
	mgr := w.activation.mgrID
	w.activation.mu.Unlock()

	token, _ := wlString(b, 0)
	wlLog("xdg_activation: done → activate")
	if token != "" && mgr != 0 {
		w.send(newWlMsg(mgr, xdgActivationActivate).putString(token).putUint(w.surfaceID), -1)
	}
	w.send(newWlMsg(obj, xdgActTokenDestroy), -1)
	return true
}

// IsMinimized на Wayland всегда false: у xdg_toplevel нет признака
// свёрнутости (есть только просьба set_minimized), компоновщик его клиенту
// не сообщает. Реализует minimizedBackend.
func (w *WaylandWindow) IsMinimized() bool { return false }

// SetOnMinimizedChanged на Wayland — no-op: событий свёрнутости нет.
// Реализует minimizedBackend.
func (w *WaylandWindow) SetOnMinimizedChanged(func(bool)) {}
