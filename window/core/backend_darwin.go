//go:build darwin

package core

// backend_darwin.go — бэкенд macOS для ядра: Cocoa (platform/cocoa).

import (
	"github.com/oops1/headless-gui/v3/platform/cocoa"
	"github.com/oops1/headless-gui/v3/platform/native"
)

// CocoaWindow — реализация NativeWindow через Cocoa/AppKit (purego), без CGO.
type CocoaWindow = cocoa.CocoaWindow

// NewNativeWindow создаёт окно ОС: на macOS — Cocoa.
func NewNativeWindow() native.NativeWindow {
	return &CocoaWindow{}
}
