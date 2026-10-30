package raster

import (
	"bytes"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"image"
	"image/color"
	"testing"
)

// DrawShadow с параметрами DrawSoftShadow побайтно совпадает с ним: прежние
// тени не изменились.
func TestDrawShadow_LegacyIdentical(t *testing.T) {
	testreset.Globals(t)
	draw := func(f func(c *Canvas)) []byte {
		c := NewCanvasScaled(160, 120, 1, nil)
		c.FillRect(0, 0, 160, 120, color.RGBA{230, 230, 230, 255})
		f(c)
		return bytes.Clone(c.back.Pix)
	}
	r := image.Rect(40, 30, 120, 80)
	col := color.RGBA{0, 0, 0, 70}
	a := draw(func(c *Canvas) { c.DrawSoftShadow(r, 8, 12, col) })
	b := draw(func(c *Canvas) { c.DrawShadow(r, 8, 12, 0, 6, col) })
	if !bytes.Equal(a, b) {
		t.Fatal("DrawSoftShadow и DrawShadow(blur=h, dy=h/2) расходятся")
	}
}

// Смещение по X двигает тень вбок, непрозрачность (альфа цвета) ослабляет.
func TestDrawShadow_OffsetAndOpacity(t *testing.T) {
	testreset.Globals(t)
	shadowAt := func(ox, oy float64, a uint8, x, y int) uint8 {
		c := NewCanvasScaled(200, 160, 1, nil)
		c.FillRect(0, 0, 200, 160, color.RGBA{255, 255, 255, 255})
		c.DrawShadow(image.Rect(60, 50, 140, 100), 6, 10, ox, oy, color.RGBA{0, 0, 0, a})
		return c.back.RGBAAt(x, y).R
	}
	// Точка справа от окна: со сдвигом вправо темнее, чем со сдвигом влево.
	right := shadowAt(12, 0, 255, 148, 75)
	left := shadowAt(-12, 0, 255, 148, 75)
	if right >= left {
		t.Errorf("сдвиг вправо не затемнил точку справа: %d против %d", right, left)
	}
	// Точка над окном: сдвиг вниз её светлит.
	down := shadowAt(0, 14, 255, 100, 42)
	none := shadowAt(0, 0, 255, 100, 42)
	if down <= none {
		t.Errorf("сдвиг вниз не высветлил точку над окном: %d против %d", down, none)
	}
	// Меньшая непрозрачность — светлее.
	strong := shadowAt(0, 6, 255, 100, 106)
	weak := shadowAt(0, 6, 90, 100, 106)
	if weak <= strong {
		t.Errorf("слабая тень %d не светлее сильной %d", weak, strong)
	}
	// Тени нет за пределами запаса.
	if far := shadowAt(0, 6, 255, 5, 5); far != 255 {
		t.Errorf("тень достала до далёкой точки: %d", far)
	}
}
