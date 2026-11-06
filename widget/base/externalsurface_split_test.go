package base

import (
	"image"
	"math/rand"
	"testing"
)

// Разложение видимой части буфера по непрозрачной области (splitFor): остаток
// и непрозрачные части вместе накрывают всё ровно один раз — каждый пиксель
// либо копируется, либо смешивается, и смешивается не дважды.
func TestSurfaceSplit_CoversEveryPixelOnce(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for round := 0; round < 300; round++ {
		sr := image.Rect(0, 0, 8+rng.Intn(40), 8+rng.Intn(40)).Add(image.Pt(rng.Intn(5), rng.Intn(5)))
		n := 1 + rng.Intn(6)
		src := make([]image.Rectangle, n)
		for i := range src {
			x, y := rng.Intn(50)-5, rng.Intn(50)-5
			src[i] = image.Rect(x, y, x+rng.Intn(30), y+rng.Intn(30))
		}
		var c surfaceContent
		solid, rest, ok := c.splitFor(&src, sr)

		inSolid := func(p image.Point) bool {
			for _, r := range src {
				if p.In(r) {
					return true
				}
			}
			return false
		}
		any := false
		for y := sr.Min.Y; y < sr.Max.Y; y++ {
			for x := sr.Min.X; x < sr.Max.X; x++ {
				p := image.Pt(x, y)
				want := inSolid(p)
				any = any || want
				gotSolid, gotRest := 0, 0
				for _, r := range solid {
					if p.In(r) {
						gotSolid++
					}
				}
				for _, r := range rest {
					if p.In(r) {
						gotRest++
					}
				}
				if !ok {
					continue
				}
				if want && (gotSolid == 0 || gotRest != 0) {
					t.Fatalf("раунд %d: точка %v непрозрачной области: в частях %d, в остатке %d (sr=%v src=%v)",
						round, p, gotSolid, gotRest, sr, src)
				}
				if !want && (gotSolid != 0 || gotRest != 1) {
					t.Fatalf("раунд %d: точка %v вне области: в частях %d, в остатке %d (sr=%v src=%v)",
						round, p, gotSolid, gotRest, sr, src)
				}
			}
		}
		if ok != any {
			t.Fatalf("раунд %d: ok=%v, а область задевает буфер: %v (sr=%v src=%v)", round, ok, any, sr, src)
		}
		for _, r := range append(append([]image.Rectangle(nil), solid...), rest...) {
			if ok && !r.In(sr) {
				t.Fatalf("раунд %d: часть %v вышла за буфер %v", round, r, sr)
			}
		}
	}
}

// Разложение считается один раз на сочетание области и буфера.
func TestSurfaceSplit_Cached(t *testing.T) {
	src := []image.Rectangle{image.Rect(2, 2, 10, 10)}
	sr := image.Rect(0, 0, 16, 16)
	var c surfaceContent
	_, rest1, _ := c.splitFor(&src, sr)
	_, rest2, _ := c.splitFor(&src, sr)
	if len(rest1) == 0 || &rest1[0] != &rest2[0] {
		t.Error("повторный вызов с теми же областью и буфером пересчитал разложение")
	}
	if allocs := testing.AllocsPerRun(50, func() { c.splitFor(&src, sr) }); allocs != 0 {
		t.Errorf("повторное разложение выделяет память: %v", allocs)
	}
	// Другой буфер — другое разложение.
	if _, rest3, _ := c.splitFor(&src, image.Rect(0, 0, 8, 8)); len(rest3) == len(rest1) && rest3[0] == rest1[0] {
		t.Error("смена буфера не пересчитала разложение")
	}
}
