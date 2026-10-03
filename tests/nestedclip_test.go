package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Отсечение не вкладывалось: виджет, ограничивший область рисования своей
// частью, в конце СНИМАЛ ограничение целиком — вместе с тем, которое
// поставил родитель. Дальше рисовали уже без него, и содержимое прокрутки
// вылезало за её края.
//
// Правило простое: ребёнок не рисует там, где не рисует родитель. Проверяем
// его на настоящем кадре, а не на вызовах контекста: важно, какие пиксели
// легли в холст.

// clipSpy — виджет, который ведёт себя как встроенные до починки: сужает
// область рисования своей частью и в конце СНИМАЕТ сужение целиком, вместе
// с родительским. Контейнер не должен от этого страдать: доверять детям он
// не может.
type clipSpy struct {
	widget.Base
	fill color.RGBA
}

func (c *clipSpy) Draw(ctx widget.DrawContext) {
	b := c.Bounds()
	ctx.SetClip(b)
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), c.fill)
	ctx.ClearClip()
}

// overflowing — виджет, который рисует заведомо шире своих границ: так ведут
// себя тени, подсветка фокуса и всё, что выходит за рамку.
type overflowing struct {
	widget.Base
	fill color.RGBA
}

func (o *overflowing) Draw(ctx widget.DrawContext) {
	b := o.Bounds()
	ctx.FillRect(b.Min.X, b.Min.Y-200, b.Dx(), b.Dy()+400, o.fill)
}

func TestNestedClip_ChildDoesNotEscapeParent(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	green := color.RGBA{G: 200, A: 255}

	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.SetBounds(image.Rect(0, 0, 200, 300))

	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 100, 200, 200)) // полоса высотой 100 посередине
	root.AddChild(sv)

	// Первый ребёнок ведёт себя как TextBox: сужает клип и снимает его.
	spy := &clipSpy{fill: green}
	spy.SetBounds(image.Rect(0, 100, 200, 140))
	sv.AddChild(spy)

	// Второй рисует далеко за своими границами. Клип прокрутки обязан его
	// удержать — даже после того, как первый снял своё сужение.
	over := &overflowing{fill: red}
	over.SetBounds(image.Rect(0, 150, 200, 190))
	sv.AddChild(over)

	eng := engine.New(200, 300, 30)
	eng.SetRoot(root)
	img := eng.RenderOnce()

	// Над прокруткой и под ней красного быть не должно.
	for _, y := range []int{10, 50, 95, 210, 260, 290} {
		c := img.RGBAAt(100, y)
		if c.R > 150 && c.G < 100 {
			t.Fatalf("на высоте %d красное (%v) — содержимое вылезло за прокрутку", y, c)
		}
	}
	// Внутри прокрутки оно обязано быть: иначе проверка ничего не стоит.
	if c := img.RGBAAt(100, 170); c.R < 150 {
		t.Errorf("внутри прокрутки нет красного (%v) — тест не о том", c)
	}
}

// PushClip сужает область ПЕРЕСЕЧЕНИЕМ с текущей и возвращает её обратно, а
// не снимает отсечение вовсе.
func TestPushClip_IntersectsAndRestores(t *testing.T) {
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 100, 100))

	var outer, inner, after image.Rectangle
	probe := &clipProbe{fn: func(ctx widget.DrawContext) {
		ctx.SetClip(image.Rect(10, 10, 60, 60))
		outer = ctx.Clip()
		restore := widget.PushClip(ctx, image.Rect(40, 0, 90, 90))
		inner = ctx.Clip()
		restore()
		after = ctx.Clip()
		ctx.ClearClip()
	}}
	probe.SetBounds(image.Rect(0, 0, 100, 100))
	root.AddChild(probe)

	eng := engine.New(100, 100, 30)
	eng.SetRoot(root)
	eng.RenderOnce()

	if want := image.Rect(40, 10, 60, 60); inner != want {
		t.Errorf("вложенный клип %v, ждал пересечение %v", inner, want)
	}
	if after != outer {
		t.Errorf("после восстановления клип %v, был %v", after, outer)
	}
}

// clipProbe — виджет, выполняющий произвольную проверку в момент отрисовки.
type clipProbe struct {
	widget.Base
	fn func(ctx widget.DrawContext)
}

func (p *clipProbe) Draw(ctx widget.DrawContext) { p.fn(ctx) }
