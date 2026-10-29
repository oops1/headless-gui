package input

// lockorder_test.go — порядок замков ввода (ADR-0011): все замки пакета
// листовые — под ними не берут другой замок и не зовут Host (вызов через
// поле host считается захватом замков движка). Проверка —
// internal/lockcheck.

import (
	"testing"

	"github.com/oops1/headless-gui/v3/internal/lockcheck"
	"github.com/oops1/headless-gui/v3/internal/testreset"
)

func TestLockOrder_InputLocksAreLeaves(t *testing.T) {
	testreset.Globals(t)
	lockcheck.Run(t, lockcheck.Config{
		Dir:      ".",
		Receiver: "Dispatcher",
		Opaque:   map[string]lockcheck.Ref{"host": {Name: "engine"}},
		MinFuncs: 60,
	})
}
