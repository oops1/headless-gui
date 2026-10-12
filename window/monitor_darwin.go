//go:build darwin

package window

// platformMonitors на macOS пока не реализована (NSScreen потребует проверки на
// живой системе): список мониторов пуст, MonitorOf отвечает ok=false.
func platformMonitors() []Monitor { return nil }
