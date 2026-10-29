//go:build darwin

package core

import "github.com/oops1/headless-gui/v3/platform/native"

// platformMonitors на macOS пока не реализована (NSScreen потребует проверки на
// живой системе): список мониторов пуст, MonitorOf отвечает ok=false.
func platformMonitors() []native.Monitor { return nil }
