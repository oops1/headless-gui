// Package tray — иконка в системном трее и уведомления на Linux по D-Bus
// (ADR-0013): StatusNotifierItem с меню dbusmenu (SNI) и
// org.freedesktop.Notifications (Notifier).
//
// Бэкенды X11 и Wayland встраивают SNI и Notifier в своё окно: их методы
// делают окно trayHost и balloonHost для ядра window/core. Пакет — слой ниже
// бэкендов: зависит только от platform/dbus и widget/base.
// На 9.8 трей подключится к бэкендам через точки расширения (ADR-0013 §3),
// и направление зависимости развернётся.
//
// Программы этот пакет не импортируют: трей — методы window.Window.
package tray
