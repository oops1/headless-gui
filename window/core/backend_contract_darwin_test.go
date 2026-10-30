//go:build darwin

package core

import "github.com/oops1/headless-gui/v3/platform/cocoa"

// backend_contract_darwin_test.go — бэкенд Cocoa удовлетворяет
// интерфейсам-возможностям ядра.
//
// Ядро находит возможности бэкенда утверждением типа, и интерфейс с
// неэкспортированным методом тип из другого пакета молча перестаёт
// удовлетворять: ветка кода меняется без ошибки сборки (так на 9.2 выпал
// dragImageSupported). Список — снимок на шаге 9.3; бэкенд, потерявший
// возможность, здесь не соберётся. Новая возможность бэкенда — новая строка.

var (
	_ dirtyRectBlitter  = (*cocoa.CocoaWindow)(nil)
	_ fullscreenBackend = (*cocoa.CocoaWindow)(nil)
	_ rawKeySource      = (*cocoa.CocoaWindow)(nil)
)
