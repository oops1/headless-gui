//go:build darwin

package core

// backend_contract_darwin_test.go — бэкенд Cocoa удовлетворяет
// интерфейсам-возможностям ядра.
//
// Ядро находит возможности бэкенда утверждением типа, и интерфейс с
// неэкспортированным методом тип из другого пакета молча перестаёт
// удовлетворять: ветка кода меняется без ошибки сборки (так на 9.2 выпал
// dragImageSupported). Список — снимок на шаге 9.3; бэкенд, потерявший
// возможность, здесь не соберётся. Новая возможность бэкенда — новая строка.

var (
	_ dirtyRectBlitter  = (*CocoaWindow)(nil)
	_ fullscreenBackend = (*CocoaWindow)(nil)
	_ rawKeySource      = (*CocoaWindow)(nil)
)
