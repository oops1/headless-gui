package engine

import (
	"bytes"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
)

// Без байтов тайлов кадр говорит те же места изменений, а front идёт за
// back: тот же экран ещё раз — без тайлов.
func TestFrameTileDataOff(t *testing.T) {
	testreset.Globals(t)
	sc := newSurfacesScene(t, 2, FormatBGRX)
	eng := sc.eng
	eng.SetFrameTileData(false)
	if eng.FrameTileData() {
		t.Fatal("FrameTileData() = true после SetFrameTileData(false)")
	}
	fr := eng.RenderFrameNow()
	if len(fr.Tiles) == 0 {
		t.Fatal("первый кадр без тайлов")
	}
	for _, tl := range fr.Tiles {
		if tl.Data != nil || tl.W <= 0 || tl.H <= 0 {
			t.Fatalf("тайл %+v: Data %d байт, ждали nil и место", tl, len(tl.Data))
		}
	}
	if !bytes.Equal(sc.bufs[fr.Surface-1], sc.front()) {
		t.Fatal("буфер после кадра без данных не равен front")
	}
	eng.ReleaseSurface(fr.Surface)
	sc.btn.SetText("bb")
	fr = eng.RenderFrameNow()
	if len(fr.Tiles) == 0 {
		t.Fatal("изменение кнопки не дало тайлов")
	}
	for _, tl := range fr.Tiles {
		if tl.Data != nil {
			t.Fatalf("тайл %+v с данными", tl)
		}
	}
}
