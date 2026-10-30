package raster

import (
	"bytes"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"image"
	"testing"
)

// fillNoise заполняет back холста детерминированным шумом с полосами, чтобы
// часть тайлов изменилась, а часть — нет.
func fillNoise(c *Canvas, seed byte) {
	pix := c.back.Pix
	for i := range pix {
		if (i/(c.back.Stride*64))%3 == 0 {
			pix[i] = byte(i) ^ seed
		}
	}
}

// Дифф через пул совпадает с последовательным по тайлам и данным.
func TestWorkPool_DiffMatchesSequential(t *testing.T) {
	testreset.Globals(t)
	c := engineCanvas(1920, 1080)
	c.BlitBackground()
	c.DiffAndSync()
	fillNoise(c, 0x5a)

	// Последовательный ответ — на копии front, чтобы второй дифф видел то же.
	front := append([]byte(nil), c.front.Pix...)
	want := c.diffTileRows(0, 0, c.tilesX-1, c.tilesY-1)
	wantData := make([][]byte, len(want))
	for i, tl := range want {
		wantData[i] = append([]byte(nil), tl.Data...)
	}
	copy(c.front.Pix, front)

	got := c.diffTiles(0, 0, c.tilesX-1, c.tilesY-1)
	if len(got) != len(want) {
		t.Fatalf("тайлов %d, последовательно %d", len(got), len(want))
	}
	for i := range got {
		g, w := got[i], want[i]
		if g.X != w.X || g.Y != w.Y || g.W != w.W || g.H != w.H || !bytes.Equal(g.Data, wantData[i]) {
			t.Fatalf("тайл %d расходится: (%d,%d) против (%d,%d)", i, g.X, g.Y, w.X, w.Y)
		}
	}
	if len(got) == 0 {
		t.Fatal("тест ничего не проверил: изменившихся тайлов нет")
	}
}

// Большой непрозрачный блит через пул кладёт те же пиксели, что и
// последовательный.
func TestWorkPool_OpaqueBlitMatchesSequential(t *testing.T) {
	testreset.Globals(t)
	c := engineCanvas(1920, 1080)
	src := image.NewRGBA(image.Rect(0, 0, 1600, 900))
	for i := range src.Pix {
		src.Pix[i] = byte(i * 7)
	}
	for i := 3; i < len(src.Pix); i += 4 {
		src.Pix[i] = 255
	}
	r := image.Rect(100, 80, 1700, 980)

	c.blitSrcBand(r, src, image.Point{})
	want := append([]byte(nil), c.back.Pix...)
	for i := range c.back.Pix {
		c.back.Pix[i] = 0
	}
	c.blitSrcRect(r, src, image.Point{})
	if !bytes.Equal(c.back.Pix, want) {
		t.Fatal("блит через пул расходится с последовательным")
	}
	if c.blitWork.src != nil {
		t.Fatal("задание блита держит картинку приложения после работы")
	}
}
