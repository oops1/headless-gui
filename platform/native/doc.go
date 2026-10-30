// Package native — контракты и общие типы нативных бэкендов окна (ADR-0013):
// слой ниже бэкендов Win32, X11, Wayland и Cocoa и ядра window/core.
//
// Здесь:
//   - контракт окна NativeWindow, коды клавиш VK_*, необязательные
//     возможности бэкенда, на которые ссылаются по имени (IconSetter,
//     ActivateBackend, PointerLocker);
//   - мониторы (Monitor, MatchMonitor, ключ монитора бэкенда), зоны окна
//     HitArea, начальное положение (InitialPlacement и подсказки под ОС),
//     планы полноэкранного режима, значки, скругление углов, преобразования
//     пикселей, масштаб из Xft.dpi и GDK_SCALE;
//   - общая часть буфера обмена и перетаскивания X11 и Wayland: MIME-типы,
//     списки URI и файлов GNOME, HTML, лениво кодируемая картинка,
//     виртуальные файлы, DragEffect.
//
// Программы этот пакет не импортируют: открытые имена (NativeWindow, VK_*,
// Monitor, HitArea, DragEffect, ErrIconUnsupported) приходят через фасад
// window. Служебные имена, экспортированные для бэкендов и ядра, помечены
// //facadegen:skip и в фасад не попадают.
package native
