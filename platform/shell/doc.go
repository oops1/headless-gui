// Package shell — службы оболочки окна (ADR-0013 §3): открыть ссылку или
// файл назначенным в системе приложением и показать файл в файловом
// менеджере (ShellExecute, портал xdg-desktop-portal с запасным xdg-open,
// open), языки интерфейса ОС, системная тема и акцентный цвет и их
// наблюдение (реестр и WM_SETTINGCHANGE на Windows, портал настроек на
// Linux).
//
// Подключается пустым импортом (так делает фасад window):
//
//	import _ "github.com/oops1/headless-gui/v4/platform/shell"
//
// init регистрирует службы в platform/native; window.OpenURL, OpenFile,
// RevealFile, SystemLanguages, DetectSystemTheme, DetectSystemAccent и
// подписки окна на смену оформления зовут их. Проверка входа (политика схем
// ссылок и файлов-программ) остаётся в ядре окна и действует перед любой
// реализацией. Без пакета (window/minimal) ссылки не открываются
// (ErrOpenUnsupported), язык не задан, тема неизвестна.
//
// На Windows смену темы окно узнаёт сообщением WM_SETTINGCHANGE: обработчик
// пакет ставит через win32.AddMessageHook и всегда пропускает сообщение
// дальше. Пакет не импортирует ни движка, ни фасада widget, ни window:
// только platform/native, platform/win32 или platform/dbus.
package shell
