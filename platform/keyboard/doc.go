// Package keyboard — переносимая часть клавиатуры нативных бэкендов окна
// (ADR-0013): слой ниже бэкендов Win32, X11, Wayland и Cocoa, над
// platform/native (коды VK_*).
//
// Здесь:
//   - таблицы keysym и keycode X11/evdev → VK_* и символы, разбор keymap XKB
//     (XkbKeymap), сырая клавиша со скан-кодом RawKey;
//   - клавиши-переключатели LockKeys (Caps, Num, Scroll, Kana) и их связь с
//     модификаторами XKB Wayland;
//   - фильтр системных сочетаний при захвате клавиатуры (GrabFilter) и
//     подавление входа в меню окна по Alt/F10 (SwallowKeyboardMenu).
//
// Программы этот пакет не импортируют: LockKeys и его флаги приходят через
// фасад window. Служебные имена, экспортированные для бэкендов и ядра,
// помечены //facadegen:skip и в фасад не попадают.
package keyboard
