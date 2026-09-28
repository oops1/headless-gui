package treeview

import (
	"image"
	"image/color"
	"testing"
)

// Цвет у узла задавался строке целиком, и приписка — счётчик «↑3 ↓1» у ветки,
// размер у вложения — спорила с подписью. Второй фрагмент с собственным
// цветом закрывает это, не подменяя отрисовку всего дерева, как пришлось бы
// с CustomRenderer.

// recCtx — записывающий контекст: запоминает, что и каким цветом нарисовали.
type recCtx struct {
	texts []recText
}

type recText struct {
	s    string
	x, y int
	col  color.RGBA
	bold bool
}

func (c *recCtx) FillRect(x, y, w, h int, col color.RGBA)      {}
func (c *recCtx) FillRectAlpha(x, y, w, h int, col color.RGBA) {}
func (c *recCtx) DrawRect(x, y, w, h int, col color.RGBA)      {}
func (c *recCtx) DrawBorder(x, y, w, h int, col color.RGBA)    {}
func (c *recCtx) DrawHLine(x, y, length int, col color.RGBA)   {}
func (c *recCtx) DrawVLine(x, y, length int, col color.RGBA)   {}
func (c *recCtx) DrawText(text string, x, y int, col color.RGBA) {
	c.texts = append(c.texts, recText{s: text, x: x, y: y, col: col})
}
func (c *recCtx) SetPixel(x, y int, col color.RGBA)               {}
func (c *recCtx) DrawImageScaled(img image.Image, x, y, w, h int) {}
func (c *recCtx) SetClip(r image.Rectangle)                       {}
func (c *recCtx) ClearClip()                                      {}
func (c *recCtx) Clip() image.Rectangle                           { return image.Rect(0, 0, 1000, 1000) }
func (c *recCtx) MeasureText(text string, sizePt float64) int     { return len([]rune(text)) * 7 }
func (c *recCtx) MeasureTextBold(text string, sizePt float64) int { return len([]rune(text)) * 8 }
func (c *recCtx) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	c.texts = append(c.texts, recText{s: text, x: x, y: y, col: col})
}
func (c *recCtx) DrawTextBold(text string, x, y int, sizePt float64, col color.RGBA) {
	c.texts = append(c.texts, recText{s: text, x: x, y: y, col: col, bold: true})
}

// find возвращает запись отрисованного текста.
func (c *recCtx) find(s string) (recText, bool) {
	for _, t := range c.texts {
		if t.s == s {
			return t, true
		}
	}
	return recText{}, false
}

func drawTree(t *testing.T, item *TreeViewItem) *recCtx {
	t.Helper()
	tv := New()
	tv.SetBounds(image.Rect(0, 0, 300, 200))
	tv.AddRoot(item)
	ctx := &recCtx{}
	tv.Draw(ctx)
	return ctx
}

func TestTreeViewItem_DetailDrawnAfterText(t *testing.T) {
	item := NewItem("feature/login")
	item.Detail = "↑3 ↓1"
	ctx := drawTree(t, item)

	main, ok := ctx.find("feature/login")
	if !ok {
		t.Fatal("подпись узла не нарисована")
	}
	det, ok := ctx.find("↑3 ↓1")
	if !ok {
		t.Fatal("приписка не нарисована")
	}
	if det.x <= main.x {
		t.Errorf("приписка на x=%d, подпись на x=%d — ждал приписку правее", det.x, main.x)
	}
	if det.y != main.y {
		t.Errorf("приписка на y=%d, подпись на y=%d — ждал одну строку", det.y, main.y)
	}
	// Приписка тише подписи, но это по-прежнему видимый текст.
	if det.col == main.col {
		t.Error("приписка нарисована цветом подписи — ради этого всё и затевалось")
	}
	if det.col.A == 0 {
		t.Error("приписка прозрачна")
	}
}

func TestTreeViewItem_DetailColorHonoured(t *testing.T) {
	want := color.RGBA{R: 200, G: 120, B: 0, A: 255}
	item := NewItem("ветка")
	item.Detail = "+12"
	item.DetailColor = want

	det, ok := drawTree(t, item).find("+12")
	if !ok {
		t.Fatal("приписка не нарисована")
	}
	if det.col != want {
		t.Errorf("цвет приписки %v, ждал заданный %v", det.col, want)
	}
}

// Узел без приписки рисуется ровно как раньше — одним текстом.
func TestTreeViewItem_NoDetailDrawsNothingExtra(t *testing.T) {
	ctx := drawTree(t, NewItem("просто узел"))
	if len(ctx.texts) != 1 {
		t.Errorf("нарисовано текстов: %d, ждал один", len(ctx.texts))
	}
}

// Приписка не съезжает с жирной подписи: её сдвиг считается по тому же
// шрифту, каким нарисован текст.
func TestTreeViewItem_DetailAfterBoldText(t *testing.T) {
	item := NewItem("main")
	item.Bold = true
	item.Detail = "по умолчанию"

	ctx := drawTree(t, item)
	main, _ := ctx.find("main")
	det, ok := ctx.find("по умолчанию")
	if !ok {
		t.Fatal("приписка не нарисована")
	}
	if !main.bold {
		t.Fatal("подпись не жирная — тест бесполезен")
	}
	wantX := main.x + ctx.MeasureTextBold("main", 0) + detailGap
	if det.x != wantX {
		t.Errorf("приписка на x=%d, ждал %d (по ширине ЖИРНОЙ подписи)", det.x, wantX)
	}
}

func TestDetailColor_Muted(t *testing.T) {
	fg := color.RGBA{R: 240, G: 240, B: 240, A: 255}
	bg := color.RGBA{R: 40, G: 40, B: 40, A: 255}

	got := detailColor(color.RGBA{}, fg, bg)
	if got.A != 255 {
		t.Errorf("приглушённый цвет полупрозрачен (A=%d) — поверх выделения это грязь", got.A)
	}
	// Тише основного, но не сливается с фоном.
	if got.R >= fg.R || got.R <= bg.R {
		t.Errorf("приглушённый R=%d, ждал между фоном %d и текстом %d", got.R, bg.R, fg.R)
	}

	want := color.RGBA{R: 1, G: 2, B: 3, A: 255}
	if got := detailColor(want, fg, bg); got != want {
		t.Errorf("заданный цвет подменён: %v вместо %v", got, want)
	}
}
