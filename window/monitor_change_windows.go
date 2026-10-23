//go:build windows

package window

// monitor_change_windows.go — смена конфигурации мониторов на Win32: сообщения
// окна (описание — в monitor_change.go).

// spiSetWorkArea — SPI_SETWORKAREA: рабочая область изменилась (панель задач
// сдвинули, скрыли, изменили её высоту). Приходит в WPARAM сообщения
// WM_SETTINGCHANGE.
const spiSetWorkArea = 0x002F

// watchMonitors подписывается на смену мониторов (monitorChangeSource).
func (w *Win32Window) watchMonitors(notify func()) (stop func()) {
	if notify == nil {
		return nil
	}
	w.onMonitors.Store(&notify)
	return func() { w.onMonitors.Store(nil) }
}

// noteMonitorsChanged будит подписчика. Зовётся из wndProc, поэтому быстро и
// без блокировок.
func (w *Win32Window) noteMonitorsChanged() {
	if fn := w.onMonitors.Load(); fn != nil {
		(*fn)()
	}
}

// monitorsChangedBySetting — WM_SETTINGCHANGE про рабочую область.
func monitorsChangedBySetting(wparam uintptr) bool { return wparam == spiSetWorkArea }
