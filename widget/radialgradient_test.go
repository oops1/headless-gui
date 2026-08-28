package widget

import (
	"image"
	"image/color"
	"testing"
)

// Радиальный градиент: цвет расходится кругом от центра.

// gradCtx — контекст, запоминающий последнее нарисованное изображение:
// радиальный градиент рисуется образом с растяжением, и проверять надо
// именно его содержимое.
type gradCtx struct {
	DrawContext
	img        image.Image
	x, y, w, h int
	fills      int
}

func (c *gradCtx) DrawImageScaled(img image.Image, x, y, w, h int) {
	c.img, c.x, c.y, c.w, c.h = img, x, y, w, h
}

func (c *gradCtx) FillRectAlpha(int, int, int, int, color.RGBA) { c.fills++ }

func TestRadialGradient_BrightInCenterFadesToEdge(t *testing.T) {
	ctx := &gradCtx{}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	g := NewRadialGradient(white)

	r := image.Rect(10, 20, 74, 84)
	DrawRadialGradient(ctx, r, g)

	if ctx.img == nil {
		t.Fatal("градиент ничего не нарисовал")
	}
	if ctx.x != r.Min.X || ctx.y != r.Min.Y || ctx.w != r.Dx() || ctx.h != r.Dy() {
		t.Errorf("образ лёг в (%d,%d) %dx%d, ждали %v", ctx.x, ctx.y, ctx.w, ctx.h, r)
	}

	b := ctx.img.Bounds()
	cx, cy := b.Dx()/2, b.Dy()/2
	center := colorAtPix(ctx.img, cx, cy)
	edge := colorAtPix(ctx.img, b.Dx()-1, cy)
	mid := colorAtPix(ctx.img, (cx+b.Dx()-1)/2, cy)

	// Не ровно 255: центр образа приходится между точками, и ближайшая к
	// нему лежит на пол-точки в стороне — это градиент, а не заливка.
	if center.A < 240 {
		t.Errorf("в центре альфа %d, ждали почти непрозрачность", center.A)
	}
	if edge.A != 0 {
		t.Errorf("на краю альфа %d, ждали прозрачность", edge.A)
	}
	if !(mid.A < center.A && mid.A > edge.A) {
		t.Errorf("на полпути альфа %d — она обязана лежать между %d и %d",
			mid.A, center.A, edge.A)
	}
}

// Градиент симметричен: точки на равном расстоянии от центра одинаковы.
func TestRadialGradient_IsSymmetric(t *testing.T) {
	ctx := &gradCtx{}
	DrawRadialGradient(ctx, image.Rect(0, 0, 64, 64), NewRadialGradient(color.RGBA{R: 200, A: 200}))
	if ctx.img == nil {
		t.Fatal("градиент ничего не нарисовал")
	}
	b := ctx.img.Bounds()
	cx, cy := b.Dx()/2, b.Dy()/2
	d := b.Dx() / 4

	left := colorAtPix(ctx.img, cx-d, cy)
	right := colorAtPix(ctx.img, cx+d, cy)
	up := colorAtPix(ctx.img, cx, cy-d)
	down := colorAtPix(ctx.img, cx, cy+d)

	for _, pair := range [][2]color.RGBA{{left, right}, {up, down}, {left, up}} {
		if diffA(pair[0], pair[1]) > 8 {
			t.Errorf("градиент несимметричен: %v против %v", pair[0], pair[1])
		}
	}
}

// Смещённый центр действительно смещает светлое пятно.
func TestRadialGradient_CenterMoves(t *testing.T) {
	ctx := &gradCtx{}
	g := NewRadialGradient(color.RGBA{R: 255, G: 255, B: 255, A: 255})
	g.CenterX, g.CenterY = 0.2, 0.5

	DrawRadialGradient(ctx, image.Rect(0, 0, 64, 64), g)
	b := ctx.img.Bounds()
	leftSide := colorAtPix(ctx.img, b.Dx()/5, b.Dy()/2)
	rightSide := colorAtPix(ctx.img, b.Dx()*4/5, b.Dy()/2)

	if leftSide.A <= rightSide.A {
		t.Errorf("пятно не сместилось влево: слева %d, справа %d", leftSide.A, rightSide.A)
	}
}

// Вырожденная область не роняет отрисовку и не рисует мусор.
func TestRadialGradient_DegenerateIsSafe(t *testing.T) {
	ctx := &gradCtx{}
	DrawRadialGradient(ctx, image.Rectangle{}, NewRadialGradient(color.RGBA{A: 255}))
	if ctx.img != nil || ctx.fills != 0 {
		t.Error("пустая область всё же что-то нарисовала")
	}

	// Полоса в один пиксель шириной: образ строить не из чего, но и падать
	// нельзя — заливается сплошным цветом центра.
	ctx = &gradCtx{}
	DrawRadialGradient(ctx, image.Rect(0, 0, 1, 40), NewRadialGradient(color.RGBA{A: 255}))
	if ctx.fills == 0 && ctx.img == nil {
		t.Error("узкая полоса осталась незакрашенной")
	}
}

func colorAtPix(img image.Image, x, y int) color.RGBA {
	r, g, b, a := img.At(x, y).RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func diffA(a, b color.RGBA) int {
	d := int(a.A) - int(b.A)
	if d < 0 {
		return -d
	}
	return d
}
