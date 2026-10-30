package raster

import (
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
)

// Тесты пакета не должны зависеть от порядка запуска (go test -shuffle=on):
// общее состояние процесса (кегль по умолчанию, тема виджетов и прочее)
// сбрасывается первой строкой каждого теста — testreset.Globals(t).
func TestHermetic_EveryTestResetsGlobals(t *testing.T) {
	testreset.Globals(t)
	testreset.RequireInEveryTest(t, 100)
}
