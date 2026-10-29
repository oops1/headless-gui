//go:build linux && !android

package core

// X11: «поверх всех» — _NET_WM_STATE_ABOVE (EWMH), тем же ClientMessage
// _NET_WM_STATE, что и полный экран. Итог читается из свойства, которое пишет
// оконный менеджер (x11SyncWMState): менеджер вправе отказать.

// SetAlwaysOnTop реализует alwaysOnTopBackend.
func (w *X11Window) SetAlwaysOnTop(on bool) bool {
	if w.wid == 0 || w.atomNetWMState == 0 || w.atomWMStateAbove == 0 {
		return false
	}
	action := uint32(netWMStateRemove)
	if on {
		action = netWMStateAdd
	}
	w.x11Send(x11NetWMStateMessage(w.rootWin, w.wid, w.atomNetWMState, action, w.atomWMStateAbove))
	return true
}

// IsAlwaysOnTop реализует alwaysOnTopBackend.
func (w *X11Window) IsAlwaysOnTop() bool { return w.above.Load() }
