package input

import (
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
)

// Тесты пакета не должны зависеть от порядка запуска (go test -shuffle=on):
// общее состояние процесса сбрасывается первой строкой каждого теста —
// testreset.Globals(t).
func TestHermetic_EveryTestResetsGlobals(t *testing.T) {
	testreset.Globals(t)
	testreset.RequireInEveryTest(t, 5)
}
