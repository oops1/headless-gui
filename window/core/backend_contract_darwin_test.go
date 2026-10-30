//go:build darwin

package core

import (
	"testing"

	"github.com/oops1/headless-gui/v3/platform/cocoa"
	"github.com/oops1/headless-gui/v3/platform/native"
)

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

// Стиля, владельца, Z-порядка и положения бэкенд Cocoa не умеет (шаг 9.7):
// окно ядра отвечает false, а не делает вид.
func TestBackendContract_CocoaNoStyle(t *testing.T) {
	var nw native.NativeWindow = &cocoa.CocoaWindow{}
	for name, ok := range map[string]bool{
		"Styler":      is[native.Styler](nw),
		"OwnerSetter": is[native.OwnerSetter](nw),
		"Stacker":     is[native.Stacker](nw),
		"Positioner":  is[native.Positioner](nw),
	} {
		if ok {
			t.Errorf("CocoaWindow объявил %s: добавьте его в контракт и в таблицу style.go", name)
		}
	}
	if styleBeforeRun() {
		t.Error("до Run стиль на macOS «запоминается», хотя бэкенд его не умеет")
	}
}

func is[T any](v any) bool { _, ok := v.(T); return ok }
