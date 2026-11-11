package core

import (
	"image/color"
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

type framingNative struct {
	squareNative
	draws bool
	got   []color.RGBA
}

func (f *framingNative) DrawsFrame() bool { return f.draws }
func (f *framingNative) SetFrameColor(c color.RGBA, set bool) {
	if set {
		f.got = append(f.got, c)
	}
}

// Бэкенд, у которого рамку рисует ОС (Windows 11), ставит корневому окну
// SystemFrame и принимает цвет его рамки; бэкенд, переставший её рисовать,
// снимает флаг — контур снова за движком (GG-99).
func TestSyncSystemFrame(t *testing.T) {
	eng := engine.New(200, 100, 30)
	defer eng.Stop()
	ww := widget.NewWindow("x", 200, 100)
	ww.CornerRadius = 8
	eng.SetRoot(ww)
	fn := &framingNative{squareNative: squareNative{rounds: true}, draws: true}
	win := &Window{}
	win.eng = eng
	win.native = fn
	win.syncSquareCorners()
	if !ww.SystemFrame || ww.SquareCorners || ww.SystemFrameColor == nil {
		t.Fatalf("рамка ОС: SystemFrame=%v SquareCorners=%v колбэк=%v", ww.SystemFrame, ww.SquareCorners, ww.SystemFrameColor != nil)
	}
	c := color.RGBA{R: 0x3C, G: 0x3C, B: 0x3C, A: 255}
	ww.SetFrameColor(c)
	eng.RenderOnce()
	if len(fn.got) != 1 || fn.got[0] != c {
		t.Errorf("цвет рамки до бэкенда не дошёл: %v", fn.got)
	}

	fn.draws = false
	win.syncSquareCorners()
	if ww.SystemFrame || ww.SystemFrameColor != nil {
		t.Errorf("ОС рамку больше не рисует, а SystemFrame=%v", ww.SystemFrame)
	}

	// Бэкенд без интерфейса рамки — прежнее поведение.
	win.native = &squareNative{rounds: true}
	win.syncSquareCorners()
	if ww.SystemFrame {
		t.Error("бэкенд без DrawsFrame включил SystemFrame")
	}
}
