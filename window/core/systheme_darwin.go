//go:build darwin

package core

import (
	"image/color"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// systheme_darwin.go — тема ОС на macOS: пока не реализована.
//
// Это заглушка, а не «на macOS темы нет»: тема есть, и узнать её можно —
// NSApplication.effectiveAppearance (имя NSAppearanceNameDarkAqua), а смену
// ловить через KVO по этому свойству или через распределённое уведомление
// AppleInterfaceThemeChangedNotification. Но бэкенд macOS написан на purego,
// и каждый такой вызов — ручная сигнатура objc_msgSend; а проверить её негде:
// собрать и запустить Cocoa-бэкенд можно только на Mac, и такой машины у
// проекта нет. Код, которого никто не запускал, обязывал бы приложения ему
// верить; честный Unknown даёт им взять свою тему по умолчанию.
//
// Читать defaults через `defaults read -g AppleInterfaceStyle` подпроцессом
// не годится по той же причине, что gsettings на Linux: внешних утилит в
// движке нет.

// detectSystemTheme — на macOS всегда SystemThemeUnknown.
func detectSystemTheme() native.SystemTheme {
	return native.SystemThemeUnknown
}

// watchSystemTheme — наблюдения за темой на macOS нет (см. выше); nil означает
// «платформа наблюдать не умеет».
func watchSystemTheme(nw native.NativeWindow, emit func(native.SystemTheme)) (stop func()) {
	return nil
}

// detectSystemAccent — на macOS акцентный цвет не читается (см. выше): ok=false.
func detectSystemAccent() (color.RGBA, bool) { return color.RGBA{}, false }

// watchSystemAccent — наблюдения нет; nil означает «платформа наблюдать не умеет».
func watchSystemAccent(nw native.NativeWindow, emit func(color.RGBA, bool)) (stop func()) {
	return nil
}
