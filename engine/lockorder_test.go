package engine

// lockorder_test.go — порядок замков движка (ADR-0011): frameMu — единственный
// замок, под которым берут другие; остальные листовые. Проверка —
// internal/lockcheck (статический разбор исходников пакета). Вызовы через
// e.in (core/input) считаются захватом замков ввода: под frameMu их звать
// можно, под листовым замком движка — нет.

import (
	"testing"

	"github.com/oops1/headless-gui/v3/internal/lockcheck"
	"github.com/oops1/headless-gui/v3/internal/testreset"
)

func TestLockOrder_FrameMuIsTheOnlyNestingLock(t *testing.T) {
	testreset.Globals(t)
	lockcheck.Run(t, lockcheck.Config{
		Dir:      ".",
		Root:     "frameMu",
		Receiver: "Engine",
		Opaque:   map[string]string{"in": "input"},
		MinFuncs: 100,
	})
}
