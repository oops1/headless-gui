//go:build linux && !android

package core

// linuxhelpers_linux_test.go — помощники тестов ядра под Linux, общие для
// Wayland и X11 (копии — в platform/x11/helpers_linux_test.go).

import (
	"time"
)

// fsWaitFor ждёт условия до 3 с.
func fsWaitFor(cond func() bool) bool {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

// mouseCallLinux — вызов колбэка кнопки мыши.
type mouseCallLinux struct {
	x, y, b int
	p       bool
}
