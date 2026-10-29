package window

import (
	"flag"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/facadegen"
)

// Проверки фасада — internal/facadegen (verify.go); глобального состояния
// тесты не трогают — только читают исходники.

// Фасад свеж: файлы *_gen.go (и файлы условий сборки) совпадают с выводом
// генератора.
func TestFacade_UpToDate(t *testing.T) {
	facadegen.VerifyUpToDate(t, facadegen.Window(".."))
}

// Фасад полон: каждое экспортированное имя window/core, platform/native и
// platform/keyboard без //facadegen:skip (по всем ОС)
// объявлено в window тем же видом, лишних имён нет.
func TestFacade_Complete(t *testing.T) {
	facadegen.VerifyComplete(t, facadegen.Window(".."), ".", 150)
}

// Фасад тождествен исходным пакетам по типам (под текущей ОС).
func TestFacade_TypesIdentical(t *testing.T) {
	if testing.Short() {
		t.Skip("проверка типов из исходников")
	}
	facadegen.VerifyTypesIdentical(t, facadegen.Window(".."), "github.com/oops1/headless-gui/v3/window", 150)
}

var updateAPI = flag.Bool("update-api", false, "переписать testdata/api.txt по фасаду")

// Снимок открытого API фасада (имена и виды по всем ОС): любое изменение —
// осознанное (go test ./window -run APISnapshot -update-api).
func TestFacade_APISnapshot(t *testing.T) {
	facadegen.VerifyAPISnapshot(t, ".", "testdata/api.txt", *updateAPI)
}
