// Package a11y — возможность доступности окна (ADR-0013 §3): мост к службе
// доступности ОС — UI Automation на Windows, AT-SPI на Linux; на прочих ОС
// пакет ничего не подключает.
//
// Подключается пустым импортом (так делает фасад window):
//
//	import _ "github.com/oops1/headless-gui/v3/platform/a11y"
//
// init регистрирует фабрику мостов в platform/native, и окно ядра
// (window/core) поднимает мост на каждое окно после создания окна ОС, а
// снимает после цикла событий. Окно мост видит через native.AccessHost:
// дерево семантики и действия движка, окно ОС, масштаб, заголовок, решение
// SetAccessibilityEnabled. На Windows запросы клиента приходят окну
// сообщением WM_GETOBJECT — пакет отвечает на них через win32.AddMessageHook.
//
// Пакет не импортирует ни движка, ни фасада widget, ни window: только
// platform/native, platform/win32 или platform/dbus и widget/base.
package a11y
