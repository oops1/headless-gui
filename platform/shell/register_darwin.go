//go:build darwin

package shell

import (
	"github.com/oops1/headless-gui/v4/platform/native"
)

// Тема и акцент на macOS пока не реализованы: служб Theme, Accent, WatchTheme
// и WatchAccent нет, ядро отвечает SystemThemeUnknown и ok=false.
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
// Подпроцесс `defaults read -g AppleInterfaceStyle` дела не меняет. Сам
// /usr/bin/defaults допустим так же, как /usr/bin/open (часть macOS, см.
// openurl_darwin.go), но он отдаёт лишь текущее значение, а не смену, и что
// он отвечает в каждом режиме оформления, включая «Авто», без Mac проверить
// так же негде, как и вызовы objc_msgSend.
func init() {
	native.RegisterShell(native.Shell{
		OpenURL:    platformOpenURL,
		OpenFile:   platformOpenFile,
		RevealFile: platformRevealFile,
		Languages:  systemLanguages,
	})
}
