package engine

// lockorder_test.go — порядок замков движка (ADR-0011): frameMu — единственный
// замок, под которым берут другие; остальные листовые. Проверка —
// internal/lockcheck (статический разбор исходников пакета). Вызовы через
// e.in (core/input), e.lp (core/loop) и e.ov (core/overlay) считаются
// захватом замков этих пакетов: под frameMu их звать можно, под листовым
// замком движка — нет.

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
		Opaque: map[string]lockcheck.Ref{
			"in": {Name: "input", Dir: "../core/input", Receiver: "Dispatcher",
				Opaque: map[string]lockcheck.Ref{"host": {Name: "engine"}}},
			"lp": {Name: "loop", Dir: "../core/loop", Receiver: "Loop",
				Opaque: map[string]lockcheck.Ref{"host": {Name: "engine"}}},
			"ov": {Name: "overlay", Dir: "../core/overlay", Receiver: "Manager",
				Opaque: map[string]lockcheck.Ref{"host": {Name: "engine"}}},
		},
		MinFuncs: 100,
	})
}
