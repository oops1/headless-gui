package minimal

import (
	"testing"

	"github.com/oops1/headless-gui/v3/internal/facadegen"
)

// Проверки фасада — internal/facadegen (verify.go), как у window; тесты
// только читают исходники.

// Фасад свеж: файлы *_gen.go совпадают с выводом генератора.
func TestFacade_UpToDate(t *testing.T) {
	facadegen.VerifyUpToDate(t, facadegen.WindowMinimal("../.."))
}

// Фасад полон: каждое экспортированное имя источников объявлено тем же видом.
func TestFacade_Complete(t *testing.T) {
	facadegen.VerifyComplete(t, facadegen.WindowMinimal("../.."), ".", 150)
}

// Фасад тождествен исходным пакетам по типам (под текущей ОС).
func TestFacade_TypesIdentical(t *testing.T) {
	if testing.Short() {
		t.Skip("проверка типов из исходников")
	}
	facadegen.VerifyTypesIdentical(t, facadegen.WindowMinimal("../.."), "github.com/oops1/headless-gui/v3/window/minimal", 150)
}

// API тот же, что у window: снимок общий (обновляется только из window —
// go test ./window -run APISnapshot -update-api).
func TestFacade_SameAPIAsWindow(t *testing.T) {
	facadegen.VerifyAPISnapshot(t, ".", "../testdata/api.txt", false)
}
