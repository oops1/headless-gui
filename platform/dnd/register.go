//go:build windows || (linux && !android)

package dnd

import "github.com/oops1/headless-gui/v3/platform/native"

// Перетаскивание умеют бэкенды Win32, X11 и Wayland; на прочих ОС флаг не
// ставится.
func init() { native.RegisterDragDrop() }
