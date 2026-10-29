// Package window предоставляет нативное OS-окно для GUI-движка headless-gui.
//
// Использует собственные нативные бэкенды:
//   - Windows: Win32 API (user32/gdi32), чистый Go без CGO
//   - Linux:   X11 и Wayland протоколы напрямую через Unix socket, без CGO
//   - macOS:   Cocoa через purego (Objective-C runtime), без CGO
//
// Использование:
//
//	eng := engine.New(1920, 1080, 30)
//	eng.SetRoot(buildUI())
//	eng.Start()
//
//	win := window.New(eng, "My App")
//	win.Run() // блокирует до закрытия окна
//
//	eng.Stop()
//
// С версии 4.0 пакет — фасад (ADR-0013): код живёт в window/core (и, по
// шагам этапа 9, в platform/*), а здесь для каждого экспортированного имени
// — алиас типа, обёртка функции с той же документацией, константа и
// переменная. Алиас — тот же тип: методы Window и утверждения типов
// прежние. Имена, объявленные только под своей ОС (Win32Window, X11Window,
// NewNativeWindow…), — в файлах фасада с тем же условием сборки.
//
// Файлы *_gen.go генерирует internal/facadegen:
//
//	go run ./internal/facadegen/cmd/facadegen
package window
