// Package tray — возможность окна (ADR-0013 §3): иконка в системном трее, её
// меню и клики, системные уведомления. Windows — Shell_NotifyIcon на окне
// бэкенда Win32 (события иконки и WM_DESTROY — через win32.AddMessageHook);
// Linux, X11 и Wayland одинаково — StatusNotifierItem с меню dbusmenu (SNI) и
// org.freedesktop.Notifications (Notifier) по D-Bus, без окна бэкенда.
//
// Подключается пустым импортом: init регистрирует native.RegisterTray, и ядро
// окна создаёт трей каждому окну ОС, а при закрытии окна снимает. Фасад window
// импортирует пакет, window/minimal — нет: там иконки и уведомлений нет, а
// HideToTray и RestoreFromTray — операции окна — работают.
//
// Программы этот пакет не импортируют: трей — методы window.Window.
package tray
