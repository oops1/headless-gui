package core

import (
	"testing"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget"
)

type squareNative struct {
	native.NativeWindow
	rounds bool
}

func (s *squareNative) RoundsCorners() bool { return s.rounds }

// Бэкенд, который углы не скругляет, переводит корневое widget.Window на
// прямые углы; скругляющий — оставляет как есть.
func TestSyncSquareCorners(t *testing.T) {
	for _, rounds := range []bool{false, true} {
		eng := engine.New(200, 100, 30)
		ww := widget.NewWindow("x", 200, 100)
		ww.CornerRadius = 8
		eng.SetRoot(ww)
		win := &Window{}
		win.eng = eng
		win.native = &squareNative{rounds: rounds}
		win.syncSquareCorners()
		if ww.SquareCorners == rounds {
			t.Errorf("ОС скругляет=%v: SquareCorners=%v", rounds, ww.SquareCorners)
		}
		eng.Stop()
	}
}
