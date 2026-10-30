// Package cocoa — бэкенд нативного окна macOS (ADR-0013): CocoaWindow на
// Cocoa/AppKit через github.com/ebitengine/purego (objc_msgSend без CGO).
//
// Бэкенд минимальный: окно, ввод мыши и клавиатуры, вывод кадра через
// CALayer. Мониторов, темы ОС и полного экрана нет — их нельзя проверить без
// Mac (window/core/monitor_darwin.go и window/core/systheme_darwin.go честно
// отвечают «не умею», SetFullscreen — false). Окно собирается только под
// darwin и проверяется кросс-сборкой; на других ОС пакет пуст (только эта
// документация).
//
// Программы этот пакет не импортируют: CocoaWindow и NewNativeWindow
// приходят через фасад window.
package cocoa
