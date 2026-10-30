//go:build windows

package core

// backend_windows.go — бэкенд Windows для ядра: Win32 (platform/win32). Ядро
// зовёт возможности ОС одними именами на всех платформах; здесь они
// переданы бэкенду.

import (
	"time"

	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/platform/win32"
)

// Win32Window — реализация NativeWindow через Win32 API.
// Чистый Go, без CGO. Работает через golang.org/x/sys/windows.
type Win32Window = win32.Win32Window

// AllowForeground разрешает процессу pid выводить свои окна на передний план
// (AllowSetForegroundWindow). Зовёт тот, у кого сейчас право на передний план
// — обычно процесс, запущенный пользователем из проводника, — перед тем, как
// передать работу уже работающему процессу: тогда Activate у того получит
// право. Право одноразовое и пропадает после следующего ввода пользователя
// в другое окно.
//
// pid < 0 — любой процесс (ASFW_ANY). true — право выдано; false — у вызвавшего
// процесса самого нет права на передний план, или pid неверный.
//
// На других платформах такого механизма нет (там доказательство пользовательского
// действия передаётся токеном, см. TakeActivationToken): всегда false.
func AllowForeground(pid int) bool { return win32.AllowForeground(pid) }

// NewNativeWindow создаёт окно ОС: на Windows — Win32.
func NewNativeWindow() native.NativeWindow { return win32.NewWindow() }

func nativeDoubleClickTime() time.Duration { return win32.DoubleClickTime() }

func platformMonitors() []native.Monitor { return win32.EnumMonitors() }
