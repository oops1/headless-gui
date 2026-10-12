package widget

import (
	"image"
	"testing"
)

// irRec — контекст с поддержкой ImageRectDrawer и ImageCacheInvalidator:
// записывает, что до него дошло.
type irRec struct {
	hsRec
	rects       []image.Rectangle
	srs         []image.Rectangle
	filters     []ImageFilter
	invalidated []image.Image
}

func (c *irRec) DrawImageRect(src image.Image, sr, dr image.Rectangle, f ImageFilter) {
	c.srs, c.rects, c.filters = append(c.srs, sr), append(c.rects, dr), append(c.filters, f)
}

func (c *irRec) InvalidateImageCache(src image.Image) {
	c.invalidated = append(c.invalidated, src)
}

// Контекст с ImageRectDrawer получает вызов как есть.
func TestDrawImageRect_Helper_UsesDrawer(t *testing.T) {
	ctx := &irRec{}
	src := image.NewRGBA(image.Rect(0, 0, 40, 30))
	DrawImageRect(ctx, src, image.Rect(5, 5, 25, 20), image.Rect(100, 100, 300, 250), FilterCatmullRom)
	if len(ctx.rects) != 1 || ctx.rects[0] != image.Rect(100, 100, 300, 250) || ctx.filters[0] != FilterCatmullRom {
		t.Fatalf("вызов не дошёл до ImageRectDrawer как есть: %v %v", ctx.rects, ctx.filters)
	}
}

// Контекст без поддержки: вырезанная часть уходит в DrawImageScaled, sr
// обрезается по источнику вместе с пропорциональной частью dr.
func TestDrawImageRect_Helper_Fallback(t *testing.T) {
	ctx := &hsRec{}
	src := image.NewRGBA(image.Rect(0, 0, 40, 30))
	DrawImageRect(ctx, src, image.Rect(0, 0, 80, 30), image.Rect(10, 20, 170, 80), FilterNearest)
	if len(ctx.ops) != 1 || ctx.ops[0].name != "DrawImageScaled" {
		t.Fatalf("ожидался один DrawImageScaled, получено %v", ctx.ops)
	}
	// Источник вдвое уже sr — dr вдвое уже: 160 → 80.
	if got, want := ctx.ops[0].r, image.Rect(10, 20, 10+80, 20+60); got != want {
		t.Fatalf("запасной путь: прямоугольник %v, ожидался %v", got, want)
	}

	ctx = &hsRec{}
	DrawImageRect(ctx, src, image.Rect(100, 100, 200, 200), image.Rect(0, 0, 10, 10), FilterNearest)
	DrawImageRect(ctx, src, image.Rect(0, 0, 0, 0), image.Rect(0, 0, 10, 10), FilterNearest)
	DrawImageRect(ctx, nil, image.Rect(0, 0, 4, 4), image.Rect(0, 0, 10, 10), FilterNearest)
	if len(ctx.ops) != 0 {
		t.Fatalf("пустое пересечение с источником не должно ничего рисовать: %v", ctx.ops)
	}
}

func TestClampImageRect(t *testing.T) {
	b := image.Rect(0, 0, 100, 50)
	cases := []struct {
		name         string
		sr, dr       image.Rectangle
		wantS, wantD image.Rectangle
	}{
		{"внутри", image.Rect(10, 10, 60, 40), image.Rect(0, 0, 500, 300), image.Rect(10, 10, 60, 40), image.Rect(0, 0, 500, 300)},
		{"справа вдвое", image.Rect(0, 0, 200, 50), image.Rect(0, 0, 400, 100), image.Rect(0, 0, 100, 50), image.Rect(0, 0, 200, 100)},
		{"слева и сверху", image.Rect(-50, -25, 50, 25), image.Rect(100, 100, 300, 200), image.Rect(0, 0, 50, 25), image.Rect(200, 150, 300, 200)},
		{"целиком вне", image.Rect(200, 200, 300, 300), image.Rect(0, 0, 10, 10), image.Rectangle{}, image.Rectangle{}},
		{"пустой dr", image.Rect(0, 0, 10, 10), image.Rectangle{}, image.Rectangle{}, image.Rectangle{}},
	}
	for _, c := range cases {
		s, d := ClampImageRect(b, c.sr, c.dr)
		if s != c.wantS || d != c.wantD {
			t.Errorf("%s: получено %v → %v, ожидалось %v → %v", c.name, s, d, c.wantS, c.wantD)
		}
	}
}

// Обёртка ScrollView пробрасывает DrawImageRect со сдвигом и сброс кэша.
func TestScrollViewOffsetCtx_ImageRectAndInvalidate(t *testing.T) {
	inner := &irRec{}
	sv := NewScrollView()
	ctx := sv.offsetContext(inner, 7, 3)

	if _, ok := ctx.(ImageRectDrawer); !ok {
		t.Fatal("обёртка ScrollView не реализует ImageRectDrawer")
	}
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	DrawImageRect(ctx, src, src.Bounds(), image.Rect(100, 200, 140, 230), FilterBilinear)
	if len(inner.rects) != 1 || inner.rects[0] != image.Rect(93, 197, 133, 227) {
		t.Fatalf("прямоугольник не сдвинут на (-7,-3): %v", inner.rects)
	}
	if inner.filters[0] != FilterBilinear {
		t.Errorf("фильтр потерян: %v", inner.filters[0])
	}

	InvalidateImageCache(ctx, src)
	if len(inner.invalidated) != 1 || inner.invalidated[0] != image.Image(src) {
		t.Fatalf("сброс кэша не дошёл до внутреннего контекста: %v", inner.invalidated)
	}

	// Внутренний контекст без поддержки: сброс — тихо ничего, рисование —
	// запасным путём с тем же сдвигом.
	plain := &hsRec{}
	ctx = sv.offsetContext(plain, 7, 3)
	InvalidateImageCache(ctx, src)
	DrawImageRect(ctx, src, src.Bounds(), image.Rect(100, 200, 140, 230), FilterBilinear)
	if len(plain.ops) != 1 || plain.ops[0].r != image.Rect(93, 197, 133, 227) {
		t.Fatalf("запасной путь через обёртку: %v", plain.ops)
	}
}
