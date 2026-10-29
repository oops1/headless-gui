package overlay

// lockorder_test.go — порядок замков оверлеев (ADR-0011): все замки пакета
// листовые — под ними не берут другой замок. Host под ними можно звать только
// без замков движка: вызов через поле host сверяется с моделью адаптера
// движка (engine.overlayHost) — BumpGen, ThemeManager и TransitionsOn замков
// не берут. Проверка — internal/lockcheck.

import (
	"testing"

	"github.com/oops1/headless-gui/v3/internal/lockcheck"
	"github.com/oops1/headless-gui/v3/internal/testreset"
)

func TestLockOrder_OverlayLocksAreLeaves(t *testing.T) {
	testreset.Globals(t)
	back := map[string]lockcheck.Ref{"host": {Name: "engine"}}
	lockcheck.Run(t, lockcheck.Config{
		Dir:      ".",
		Receiver: "Manager",
		Opaque: map[string]lockcheck.Ref{
			"host": {Name: "engine", Dir: "../../engine", Receiver: "overlayHost",
				Opaque: map[string]lockcheck.Ref{
					"lp": {Name: "loop", Dir: "../loop", Receiver: "Loop", Opaque: back},
					"in": {Name: "input", Dir: "../input", Receiver: "Dispatcher", Opaque: back},
					"ov": {Name: "overlay"},
				}},
		},
		MinFuncs: 40,
	})
}
