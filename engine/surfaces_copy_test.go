package engine

import (
	"bytes"
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
)

// Буфер перед кадром не догоняет прошлый кадр там, где этот кадр рисует
// заново, и всё равно после кадра совпадает с front целиком: виджет ездит
// туда-сюда, его старое и новое место перерисовываются, и так на каждом из
// двух буферов по очереди.
func TestSurfaces_RedrawnIsNotCopiedAndStillMatchesFront(t *testing.T) {
	testreset.Globals(t)
	for _, f := range []PixelFormat{FormatRGBA, FormatBGRX} {
		sc := newSurfacesScene(t, 2, f)
		eng := sc.eng
		at := image.Rect(20, 20, 120, 60)
		for i := 0; i < 12; i++ {
			fr := eng.RenderFrameNow()
			if fr.Surface == 0 {
				t.Fatalf("%v: кадр %d не нарисован в буфер", f, i)
			}
			if !bytes.Equal(sc.bufs[fr.Surface-1], sc.front()) {
				t.Fatalf("%v: кадр %d: буфер %d не равен front", f, i, fr.Surface)
			}
			eng.ReleaseSurface(fr.Surface)
			step := image.Pt(7, 3)
			if i%4 >= 2 {
				step = image.Pt(-5, 2)
			}
			at = at.Add(step)
			sc.btn.SetBounds(at)
		}
	}
}
