package loop

// lockorder_test.go — порядок замков цикла (ADR-0011): все замки пакета
// листовые — под ними не берут другой замок и не зовут Host (вызов через
// поле host считается захватом замков движка). Проверка —
// internal/lockcheck.

import (
	"testing"

	"github.com/oops1/headless-gui/v4/internal/lockcheck"
	"github.com/oops1/headless-gui/v4/internal/testreset"
)

func TestLockOrder_LoopLocksAreLeaves(t *testing.T) {
	testreset.Globals(t)
	lockcheck.Run(t, lockcheck.Config{
		Dir:      ".",
		Receiver: "Loop",
		Opaque:   map[string]lockcheck.Ref{"host": {Name: "engine"}},
		MinFuncs: 30,
	})
}
