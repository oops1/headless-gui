//go:build darwin

package window

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
func detectSystemTheme() SystemTheme {
	return SystemThemeUnknown
}

// watchSystemTheme — наблюдения за темой на macOS нет (см. выше); nil означает
// «платформа наблюдать не умеет».
func watchSystemTheme(native NativeWindow, emit func(SystemTheme)) (stop func()) {
	return nil
}
