package core

// alwaysontop.go — окно «поверх всех окон».
//
// Нужен калькулятору (режим «Поверх других окон» Windows 11) и позже
// Проводнику и WinLine. Протоколы:
//
//	Win32   SetWindowPos(HWND_TOPMOST / HWND_NOTOPMOST);
//	X11     _NET_WM_STATE_ABOVE через ClientMessage _NET_WM_STATE (как полный
//	        экран); менеджер вправе не послушаться, IsAlwaysOnTop отражает то,
//	        что он записал в _NET_WM_STATE;
//	Wayland в xdg-shell такого нет вовсе — false;
//	macOS   не реализовано — false.

import (
	"runtime"
	"sync"
)

// alwaysOnTopBackend — бэкенд, умеющий держать окно поверх остальных.
type alwaysOnTopBackend interface {
	// SetAlwaysOnTop включает или выключает режим. true — запрос принят
	// (на X11 решает оконный менеджер); false — бэкенд этого не умеет.
	SetAlwaysOnTop(on bool) bool
	// IsAlwaysOnTop — окно сейчас поверх всех.
	IsAlwaysOnTop() bool
}

// alwaysOnTopState — просьба, сделанная до Run: окна ещё нет.
type alwaysOnTopState struct {
	mu          sync.Mutex
	pre, preSet bool
}

// SetAlwaysOnTop держит окно поверх всех остальных (on) или возвращает его в
// обычный порядок. true — запрос принят; false — оконная система этого не
// умеет (Wayland, macOS) или окна ещё нет и не будет.
//
// До Run просьба запоминается и применяется при создании окна; тогда true
// значит «запомнено» (на Wayland она будет отклонена — смотрите
// IsAlwaysOnTop после запуска).
func (win *Window) SetAlwaysOnTop(on bool) bool {
	if win.native == nil {
		win.aot.mu.Lock()
		win.aot.pre, win.aot.preSet = on, true
		win.aot.mu.Unlock()
		return runtime.GOOS != "darwin"
	}
	ab, ok := win.native.(alwaysOnTopBackend)
	if !ok {
		return false
	}
	return ab.SetAlwaysOnTop(on)
}

// IsAlwaysOnTop — окно сейчас поверх всех остальных.
func (win *Window) IsAlwaysOnTop() bool {
	if win.native == nil {
		win.aot.mu.Lock()
		defer win.aot.mu.Unlock()
		return false
	}
	if ab, ok := win.native.(alwaysOnTopBackend); ok {
		return ab.IsAlwaysOnTop()
	}
	return false
}

// setupAlwaysOnTop применяет просьбу, сделанную до Run.
func (win *Window) setupAlwaysOnTop() {
	win.aot.mu.Lock()
	on, set := win.aot.pre, win.aot.preSet
	win.aot.preSet = false
	win.aot.mu.Unlock()
	if !set {
		return
	}
	if ab, ok := win.native.(alwaysOnTopBackend); ok {
		ab.SetAlwaysOnTop(on)
	}
}
