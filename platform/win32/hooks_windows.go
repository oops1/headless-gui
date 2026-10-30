//go:build windows

package win32

// hooks_windows.go — точки расширения бэкенда Win32 для возможностей,
// живущих вне его пакета (ADR-0013 §3): обработчики оконных сообщений и
// дескриптор окна.

import "sync/atomic"

// MessageHook — обработчик оконного сообщения, поставленный возможностью
// (доступность — WM_GETOBJECT). handled=false — сообщение идёт дальше
// обычным путём оконной процедуры.
type MessageHook func(hwnd, wparam, lparam uintptr) (result uintptr, handled bool)

// messageHooks — обработчики по коду сообщения. Карта не меняется после
// публикации (копия при каждом AddMessageHook): оконная процедура читает её
// на каждом сообщении без блокировки.
var messageHooks atomic.Pointer[map[uint32][]MessageHook]

// AddMessageHook ставит обработчик h сообщения msg для всех окон бэкенда.
// Оконная процедура спрашивает обработчики раньше своих: первый, ответивший
// handled=true, задаёт результат сообщения. Звать при инициализации пакета
// возможности (init).
func AddMessageHook(msg uint32, h MessageHook) {
	for {
		old := messageHooks.Load()
		next := map[uint32][]MessageHook{}
		if old != nil {
			for k, v := range *old {
				next[k] = v
			}
		}
		next[msg] = append(append([]MessageHook(nil), next[msg]...), h)
		if messageHooks.CompareAndSwap(old, &next) {
			return
		}
	}
}

// runMessageHooks — обработчики сообщения msg по порядку постановки.
func runMessageHooks(hwnd uintptr, msg uint32, wparam, lparam uintptr) (uintptr, bool) {
	m := messageHooks.Load()
	if m == nil {
		return 0, false
	}
	for _, h := range (*m)[msg] {
		if r, ok := h(hwnd, wparam, lparam); ok {
			return r, true
		}
	}
	return 0, false
}

// HWND — дескриптор окна ОС; 0 — окно ещё не создано или уже закрыто.
func (w *Win32Window) HWND() uintptr { return uintptr(w.hwnd) }

// WindowOf — окно бэкенда с дескриптором hwnd (и создаваемое окно, пока
// CreateWindowExW ещё не вернул дескриптор): обработчик AddMessageHook
// находит по нему своё состояние, заведённое на окно (подписки ключом по
// *Win32Window — они работают и до Create). nil — окно не наше.
func WindowOf(hwnd uintptr) *Win32Window {
	win32Mu.Lock()
	defer win32Mu.Unlock()
	if w := win32Windows[hwnd]; w != nil {
		return w
	}
	return win32Creating
}
