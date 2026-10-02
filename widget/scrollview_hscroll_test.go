package widget

import (
	"image"
	"image/color"
	"sync"
	"testing"
)

// Содержимое ScrollView шире области раньше просто обрезалось: правую часть
// нельзя было ни увидеть, ни прокрутить. Тесты держат новое — ContentWidth,
// смещение вбок, полосу внизу, колесо, Shift+колесо и перетаскивание ползунка —
// и главное обещание: при нулевом ContentWidth ничего не меняется.

// hsRec — контекст, записывающий примитивы вместе с координатами. Остальные
// методы DrawContext (измерение текста и пр.) ScrollView не зовёт; если вдруг
// позовёт, nil-интерфейс упадёт с паникой — это и нужно.
type hsRec struct {
	DrawContext
	ops     []hsOp
	clip    image.Rectangle
	hasClip bool
}

type hsOp struct {
	name string
	r    image.Rectangle // x, y, w, h примитива (для линий — x, y и длина)
}

func (c *hsRec) add(name string, x, y, w, h int) {
	c.ops = append(c.ops, hsOp{name, image.Rect(x, y, x+w, y+h)})
}

func (c *hsRec) FillRect(x, y, w, h int, col color.RGBA)      { c.add("FillRect", x, y, w, h) }
func (c *hsRec) FillRectAlpha(x, y, w, h int, col color.RGBA) { c.add("FillRectAlpha", x, y, w, h) }
func (c *hsRec) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	c.add("FillRoundRect", x, y, w, h)
}
func (c *hsRec) DrawBorder(x, y, w, h int, col color.RGBA) { c.add("DrawBorder", x, y, w, h) }
func (c *hsRec) DrawRoundBorder(x, y, w, h, r int, col color.RGBA) {
	c.add("DrawRoundBorder", x, y, w, h)
}
func (c *hsRec) SetPixel(x, y int, col color.RGBA)          { c.add("SetPixel", x, y, 0, 0) }
func (c *hsRec) DrawHLine(x, y, length int, col color.RGBA) { c.add("DrawHLine", x, y, length, 0) }
func (c *hsRec) DrawVLine(x, y, length int, col color.RGBA) { c.add("DrawVLine", x, y, 0, length) }
func (c *hsRec) DrawImage(src image.Image, x, y int)        { c.add("DrawImage", x, y, 0, 0) }
func (c *hsRec) DrawImageScaled(src image.Image, x, y, w, h int) {
	c.add("DrawImageScaled", x, y, w, h)
}
func (c *hsRec) DrawText(text string, x, y int, col color.RGBA) { c.add("DrawText", x, y, 0, 0) }
func (c *hsRec) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	c.add("DrawTextSize", x, y, 0, 0)
}
func (c *hsRec) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
	c.add("DrawTextFont", x, y, 0, 0)
}
func (c *hsRec) SetClip(r image.Rectangle) {
	c.clip, c.hasClip = r, true
	c.add("SetClip", r.Min.X, r.Min.Y, r.Dx(), r.Dy())
}
func (c *hsRec) ClearClip() {
	c.clip, c.hasClip = image.Rectangle{}, false
	c.add("ClearClip", 0, 0, 0, 0)
}

// Clip — как у настоящего холста: без отсечения это вся его площадь, а не
// пустой прямоугольник. Иначе вложенное отсечение (пересечение с текущим)
// сужалось бы в ничто.
func (c *hsRec) Clip() image.Rectangle {
	if !c.hasClip {
		return image.Rect(0, 0, 1<<15, 1<<15)
	}
	return c.clip
}

// Сглаженные примитивы — чтобы svOffsetCtxAA мог обернуть именно этот контекст.
func (c *hsRec) FillEllipseAA(cx, cy, rx, ry int, col color.RGBA) {
	c.add("FillEllipseAA", cx, cy, 0, 0)
}
func (c *hsRec) StrokeEllipseAA(cx, cy, rx, ry int, th float64, col color.RGBA) {
	c.add("StrokeEllipseAA", cx, cy, 0, 0)
}

// Многоугольник и ломаная записываются вектором от первой точки ко второй: если
// сдвиг применён не ко всем точкам, вектор изменится.
func (c *hsRec) FillPolygonAA(pts []image.Point, col color.RGBA) {
	c.add("FillPolygonAA", pts[0].X, pts[0].Y, pts[1].X-pts[0].X, pts[1].Y-pts[0].Y)
}
func (c *hsRec) StrokePolylineAA(pts []image.Point, th float64, closed bool, col color.RGBA) {
	c.add("StrokePolylineAA", pts[0].X, pts[0].Y, pts[1].X-pts[0].X, pts[1].Y-pts[0].Y)
}
func (c *hsRec) DrawLineAA(x1, y1, x2, y2 int, th float64, col color.RGBA) {
	c.add("DrawLineAA", x1, y1, x2-x1, y2-y1)
}

func (c *hsRec) find(name string) []image.Rectangle {
	var out []image.Rectangle
	for _, o := range c.ops {
		if o.name == name {
			out = append(out, o.r)
		}
	}
	return out
}

// hsBox — ребёнок, рисующий одну заливку по своим bounds.
type hsBox struct{ Base }

func (b *hsBox) Draw(ctx DrawContext) {
	r := b.Bounds()
	ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{R: 1, A: 255})
}

// hsView собирает ScrollView 200×100 с одним ребёнком, который стоит на
// (50,20) размером 30×10. ContentHeight по умолчанию с запасом (есть
// вертикальная полоса), ContentWidth задаётся тестом.
func hsView(contentW, contentH int) (*ScrollView, *hsBox) {
	sv := NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 200, 100))
	sv.ContentWidth = contentW
	sv.ContentHeight = contentH
	kid := &hsBox{}
	kid.SetBounds(image.Rect(50, 20, 80, 30))
	sv.AddChild(kid)
	return sv, kid
}

// ─── Совместимость: ContentWidth == 0 ────────────────────────────────────────

func TestScrollViewH_ZeroContentWidthChangesNothing(t *testing.T) {
	sv, _ := hsView(0, 500)

	if sv.needsHScrollbar() {
		t.Error("ContentWidth=0: горизонтальной полосы быть не должно")
	}
	if got := sv.contentHeight(); got != 100 {
		t.Errorf("ContentWidth=0: высота области %d, ждал полную 100", got)
	}
	if got := sv.maxScroll(); got != 400 {
		t.Errorf("ContentWidth=0: maxScroll=%d, ждал 400", got)
	}
	if got := sv.maxScrollX(); got != 0 {
		t.Errorf("ContentWidth=0: maxScrollX=%d, ждал 0", got)
	}

	sv.SetScrollX(75)
	if got := sv.ScrollX(); got != 0 {
		t.Errorf("ContentWidth=0: SetScrollX(75) дал %d, ждал 0", got)
	}

	// Колесо: dx игнорируется, Shift ничего не значит — как раньше.
	if !sv.OnMouseWheelPixelsMod(10, 10, 30, 0, 0) {
		t.Error("ContentWidth=0: горизонтальная дельта не должна менять поглощение вертикальной прокрутки")
	}
	sv.stopInertia()
	if sv.ScrollX() != 0 {
		t.Error("ContentWidth=0: колесо сдвинуло содержимое вбок")
	}

	// Shift+колесо в ScrollView без горизонтальной полосы по-прежнему листает вниз.
	sv.SetScrollY(0)
	sv.OnMouseWheelPixelsMod(10, 10, 0, 40, ModShift)
	sv.mu.Lock()
	vel := sv.vel
	sv.mu.Unlock()
	sv.stopInertia()
	if vel <= 0 {
		t.Errorf("ContentWidth=0: Shift+колесо должно запустить вертикальную прокрутку, vel=%v", vel)
	}

	// Поток рисования: клип по прежней области, ребёнок сдвинут только по Y.
	sv.SetScrollY(10)
	ctx := &hsRec{}
	sv.Draw(ctx)
	if got := ctx.find("SetClip"); len(got) == 0 || got[0] != image.Rect(0, 0, 190, 100) {
		t.Errorf("клип %v, ждал первым (0,0)-(190,100)", got)
	}
	// Первая заливка — ребёнок: x не тронут, y сдвинут на scrollY.
	if got := ctx.find("FillRect"); len(got) == 0 || got[0] != image.Rect(50, 10, 80, 20) {
		t.Errorf("ребёнок нарисован как %v, ждал (50,10)-(80,20)", got)
	}
}

func TestScrollViewH_StaleScrollXDoesNotStick(t *testing.T) {
	sv, _ := hsView(500, 0)
	sv.SetScrollX(100)
	if sv.ScrollX() != 100 {
		t.Fatalf("подготовка: ScrollX=%d", sv.ScrollX())
	}

	// Полосу выключили, а смещение осталось: без зажима содержимое застряло бы
	// сдвинутым, а вернуть его нечем.
	sv.ContentWidth = 0
	if got := sv.ScrollX(); got != 0 {
		t.Errorf("после ContentWidth=0 ScrollX=%d, ждал 0", got)
	}
	ctx := &hsRec{}
	sv.Draw(ctx)
	if got := ctx.find("FillRect"); len(got) == 0 || got[0].Min.X != 50 {
		t.Errorf("ребёнок нарисован как %v, ждал x=50 (без смещения)", got)
	}
}

// ─── Появление полосы ────────────────────────────────────────────────────────

func TestScrollViewH_BarOnlyWhenContentWiderThanView(t *testing.T) {
	for _, tc := range []struct {
		name           string
		cw, ch         int
		wantH, wantV   bool
		wantViewH      int
		wantMaxX, maxY int
	}{
		{"равно ширине", 200, 0, false, false, 100, 0, 0},
		{"на пиксель шире", 201, 0, true, false, 90, 1, 0},
		{"уже", 150, 0, false, false, 100, 0, 0},
		// Вертикальная отнимает 10 px ширины: 195 при полных 200 влезало бы, при
		// 190 — нет.
		{"влезает без вертикальной, нет — с ней", 195, 500, true, true, 90, 5, 410},
		{"влезает и с вертикальной", 190, 500, false, true, 100, 0, 400},
		// Не влезает по ширине → полоса съедает высоту → вертикальная становится
		// нужной, хотя 95 < 100.
		{"горизонтальная делает нужной вертикальную", 300, 95, true, true, 90, 110, 5},
	} {
		sv, _ := hsView(tc.cw, tc.ch)
		if got := sv.needsHScrollbar(); got != tc.wantH {
			t.Errorf("%s: needsHScrollbar=%v, ждал %v", tc.name, got, tc.wantH)
		}
		if got := sv.needsScrollbar(); got != tc.wantV {
			t.Errorf("%s: needsScrollbar=%v, ждал %v", tc.name, got, tc.wantV)
		}
		if got := sv.contentHeight(); got != tc.wantViewH {
			t.Errorf("%s: contentHeight=%d, ждал %d", tc.name, got, tc.wantViewH)
		}
		if got := sv.maxScrollX(); got != tc.wantMaxX {
			t.Errorf("%s: maxScrollX=%d, ждал %d", tc.name, got, tc.wantMaxX)
		}
		if got := sv.maxScroll(); got != tc.maxY {
			t.Errorf("%s: maxScroll=%d, ждал %d", tc.name, got, tc.maxY)
		}
	}
}

func TestScrollViewH_DrawShowsBarOnlyWhenNeeded(t *testing.T) {
	// Горизонтальная полоса — это FillRoundRect у нижнего края.
	hasBottomBar := func(ctx *hsRec) bool {
		for _, r := range ctx.find("FillRoundRect") {
			if r.Min.Y >= 90 {
				return true
			}
		}
		return false
	}

	sv, _ := hsView(150, 0)
	ctx := &hsRec{}
	sv.Draw(ctx)
	if hasBottomBar(ctx) {
		t.Error("содержимое влезает, а полоса нарисована")
	}

	sv, _ = hsView(600, 0)
	ctx = &hsRec{}
	sv.Draw(ctx)
	if !hasBottomBar(ctx) {
		t.Error("содержимое шире области, а полосы нет")
	}
	// Область содержимого уменьшилась по высоте на высоту полосы.
	if got := ctx.find("SetClip"); len(got) == 0 || got[0] != image.Rect(0, 0, 200, 90) {
		t.Errorf("клип %v, ждал первым (0,0)-(200,90)", got)
	}
}

// ─── Пределы и смещение детей ────────────────────────────────────────────────

func TestScrollViewH_ScrollXLimits(t *testing.T) {
	sv, _ := hsView(500, 0) // без вертикальной: видимая ширина 200 → предел 300
	sv.SetScrollX(1000)
	if got := sv.ScrollX(); got != 300 {
		t.Errorf("SetScrollX(1000)=%d, ждал предел 300", got)
	}
	sv.SetScrollX(-5)
	if got := sv.ScrollX(); got != 0 {
		t.Errorf("SetScrollX(-5)=%d, ждал 0", got)
	}
	sv.SetScrollX(120)
	sv.ScrollXBy(50)
	if got := sv.ScrollX(); got != 170 {
		t.Errorf("120 + 50 = %d, ждал 170", got)
	}
	sv.ScrollXBy(-1000)
	if got := sv.ScrollX(); got != 0 {
		t.Errorf("ScrollXBy(-1000)=%d, ждал 0", got)
	}

	// С вертикальной полосой видимая ширина 190, предел 310.
	sv, _ = hsView(500, 800)
	sv.SetScrollX(1000)
	if got := sv.ScrollX(); got != 310 {
		t.Errorf("с вертикальной полосой предел %d, ждал 310", got)
	}
}

func TestScrollViewH_ScrollMovesChildren(t *testing.T) {
	sv, kid := hsView(500, 0)
	sv.SetScrollX(30)

	ctx := &hsRec{}
	sv.Draw(ctx)

	fills := ctx.find("FillRect")
	if len(fills) == 0 || fills[0] != image.Rect(20, 20, 50, 30) {
		t.Errorf("ребёнок при scrollX=30 нарисован как %v, ждал (20,20)-(50,30)", fills)
	}
	// Bounds ребёнка не тронуты: сдвигает контекст, а не данные (PERF-12).
	if kid.Bounds() != image.Rect(50, 20, 80, 30) {
		t.Errorf("bounds ребёнка изменились: %v", kid.Bounds())
	}

	// Обе оси сразу.
	sv, _ = hsView(500, 400)
	sv.SetScrollX(30)
	sv.SetScrollY(5)
	ctx = &hsRec{}
	sv.Draw(ctx)
	if fills := ctx.find("FillRect"); len(fills) == 0 || fills[0] != image.Rect(20, 15, 50, 25) {
		t.Errorf("при (30,5) ребёнок нарисован как %v, ждал (20,15)-(50,25)", fills)
	}
}

func TestScrollViewH_OffscreenChildrenSkipped(t *testing.T) {
	sv, kid := hsView(2000, 0)
	far := &hsBox{}
	far.SetBounds(image.Rect(1500, 20, 1530, 30))
	sv.AddChild(far)
	_ = kid

	ctx := &hsRec{}
	sv.Draw(ctx)
	for _, r := range ctx.find("FillRect") {
		if r.Min.X >= 1000 {
			t.Errorf("ребёнок далеко за правым краем нарисован: %v", r)
		}
	}

	// Прокрутили к нему — теперь он на виду, а первый ушёл влево.
	sv.SetScrollX(1400)
	ctx = &hsRec{}
	sv.Draw(ctx)
	var seen []image.Rectangle
	for _, r := range ctx.find("FillRect") {
		if r.Dx() == 30 { // дети; полоса и фон иной ширины
			seen = append(seen, r)
		}
	}
	if len(seen) != 1 || seen[0] != image.Rect(100, 20, 130, 30) {
		t.Errorf("после прокрутки к дальнему ребёнку нарисовано %v, ждал только (100,20)-(130,30)", seen)
	}
}

// Угол, где сходятся полосы, закрашивается один раз, и ни одна полоса в него не заходит.
func TestScrollViewH_CornerDrawnOnce(t *testing.T) {
	sv, _ := hsView(500, 400)
	ctx := &hsRec{}
	sv.Draw(ctx)

	corner := image.Rect(190, 90, 200, 100)
	n := 0
	for _, o := range ctx.ops {
		switch o.name {
		case "FillRect", "FillRoundRect", "FillRectAlpha":
			if !o.r.Intersect(corner).Empty() {
				n++
			}
		}
	}
	if n != 1 {
		t.Errorf("угол %v задет %d примитивами, ждал ровно 1 (операции: %v)", corner, n, ctx.ops)
	}

	// Геометрия полос не пересекается.
	if v := sv.vbarRect(); v.Max.Y != 90 {
		t.Errorf("вертикальная полоса кончается на %d, ждал 90 (над горизонтальной)", v.Max.Y)
	}
	if h := sv.hbarStrip(); h.Max.X != 190 {
		t.Errorf("горизонтальная полоса кончается на %d, ждал 190 (левее вертикальной)", h.Max.X)
	}
}

// ─── Обёртка DrawContext ─────────────────────────────────────────────────────

// Каждый метод, принимающий x, обязан сдвинуть его на dx. Забытый метод рисовал
// бы мимо после прокрутки вбок.
func TestScrollViewH_OffsetCtxShiftsEveryX(t *testing.T) {
	const dx, dy = 7, 3
	inner := &hsRec{}
	sv := NewScrollView()
	ctx := sv.offsetContext(inner, dx, dy)
	aa, ok := ctx.(AAShapes)
	if !ok {
		t.Fatal("обёртка над контекстом с AAShapes обязана их пробрасывать")
	}
	col := color.RGBA{A: 255}
	pts := []image.Point{{X: 100, Y: 200}, {X: 110, Y: 210}}

	calls := []struct {
		name string
		do   func()
	}{
		{"FillRect", func() { ctx.FillRect(100, 200, 5, 6, col) }},
		{"FillRectAlpha", func() { ctx.FillRectAlpha(100, 200, 5, 6, col) }},
		{"FillRoundRect", func() { ctx.FillRoundRect(100, 200, 5, 6, 2, col) }},
		{"DrawBorder", func() { ctx.DrawBorder(100, 200, 5, 6, col) }},
		{"DrawRoundBorder", func() { ctx.DrawRoundBorder(100, 200, 5, 6, 2, col) }},
		{"SetPixel", func() { ctx.SetPixel(100, 200, col) }},
		{"DrawHLine", func() { ctx.DrawHLine(100, 200, 5, col) }},
		{"DrawVLine", func() { ctx.DrawVLine(100, 200, 5, col) }},
		{"DrawImage", func() { ctx.DrawImage(image.NewRGBA(image.Rect(0, 0, 1, 1)), 100, 200) }},
		{"DrawImageScaled", func() { ctx.DrawImageScaled(image.NewRGBA(image.Rect(0, 0, 1, 1)), 100, 200, 5, 6) }},
		{"DrawText", func() { ctx.DrawText("t", 100, 200, col) }},
		{"DrawTextSize", func() { ctx.DrawTextSize("t", 100, 200, 9, col) }},
		{"DrawTextFont", func() { ctx.DrawTextFont("t", 100, 200, 9, "f", col) }},
		{"FillEllipseAA", func() { aa.FillEllipseAA(100, 200, 4, 4, col) }},
		{"StrokeEllipseAA", func() { aa.StrokeEllipseAA(100, 200, 4, 4, 1, col) }},
		{"FillPolygonAA", func() { aa.FillPolygonAA(pts, col) }},
		{"StrokePolylineAA", func() { aa.StrokePolylineAA(pts, 1, false, col) }},
		{"DrawLineAA", func() { aa.DrawLineAA(100, 200, 110, 210, 1, col) }},
	}
	// Размер (или вектор между точками) сдвигом измениться не должен — это
	// ловит метод, сдвинувший только часть координат (вторую точку отрезка).
	wantSize := map[string]image.Point{
		"FillRect": {X: 5, Y: 6}, "FillRectAlpha": {X: 5, Y: 6}, "FillRoundRect": {X: 5, Y: 6},
		"DrawBorder": {X: 5, Y: 6}, "DrawRoundBorder": {X: 5, Y: 6}, "DrawImageScaled": {X: 5, Y: 6},
		"DrawHLine": {X: 5}, "DrawVLine": {Y: 5},
		"FillPolygonAA": {X: 10, Y: 10}, "StrokePolylineAA": {X: 10, Y: 10}, "DrawLineAA": {X: 10, Y: 10},
	}
	for _, c := range calls {
		inner.ops = nil
		c.do()
		if len(inner.ops) != 1 {
			t.Errorf("%s: внутренний контекст получил %d вызовов, ждал 1", c.name, len(inner.ops))
			continue
		}
		if got := inner.ops[0].r.Min; got != image.Pt(100-dx, 200-dy) {
			t.Errorf("%s: координата (%d,%d), ждал (%d,%d)", c.name, got.X, got.Y, 100-dx, 200-dy)
		}
		if got := inner.ops[0].r.Size(); got != wantSize[c.name] {
			t.Errorf("%s: размер/вектор %v, ждал %v", c.name, got, wantSize[c.name])
		}
	}
	// Исходный срез точек не испорчен сдвигом.
	if pts[0] != image.Pt(100, 200) {
		t.Errorf("shift изменил исходные точки: %v", pts)
	}

	// Клип симметричен: SetClip вычитает, Clip прибавляет — иначе дети,
	// сужающие клип по Clip(), получили бы чужую систему координат.
	inner.ops = nil
	ctx.SetClip(image.Rect(100, 200, 150, 260))
	if got := inner.find("SetClip"); len(got) != 1 || got[0] != image.Rect(100-dx, 200-dy, 150-dx, 260-dy) {
		t.Errorf("SetClip: во внутренний контекст ушло %v", got)
	}
	if got := ctx.Clip(); got != image.Rect(100, 200, 150, 260) {
		t.Errorf("Clip() после SetClip(r) вернул %v, ждал r", got)
	}
}

// ─── Колесо ──────────────────────────────────────────────────────────────────

func TestScrollViewH_HorizontalWheelDelta(t *testing.T) {
	sv, _ := hsView(500, 0)

	if !sv.OnMouseWheelPixelsMod(10, 10, 60, 0, 0) {
		t.Fatal("горизонтальная дельта должна быть поглощена")
	}
	if got := sv.ScrollX(); got != 60 {
		t.Errorf("dx=60 дал ScrollX=%d, ждал 60", got)
	}
	if got := sv.ScrollY(); got != 0 {
		t.Errorf("горизонтальная дельта сдвинула ScrollY=%d", got)
	}

	// Влево.
	sv.OnMouseWheelPixelsMod(10, 10, -20, 0, 0)
	if got := sv.ScrollX(); got != 40 {
		t.Errorf("dx=-20: ScrollX=%d, ждал 40", got)
	}

	// Упёрлись в край — дельта всплывает к родителю.
	sv.SetScrollX(sv.maxScrollX())
	if sv.OnMouseWheelPixelsMod(10, 10, 30, 0, 0) {
		t.Error("у правого края дельта вправо должна всплыть (false)")
	}
	sv.SetScrollX(0)
	if sv.OnMouseWheelPixelsMod(10, 10, -30, 0, 0) {
		t.Error("у левого края дельта влево должна всплыть (false)")
	}

	// Дробные дельты тачпада накапливаются, а не теряются.
	sv.SetScrollX(0)
	for i := 0; i < 4; i++ {
		sv.OnMouseWheelPixelsMod(10, 10, 0.5, 0, 0)
	}
	if got := sv.ScrollX(); got != 2 {
		t.Errorf("4×0.5 px дали ScrollX=%d, ждал 2", got)
	}
}

func TestScrollViewH_ShiftWheelScrollsSideways(t *testing.T) {
	sv, _ := hsView(500, 800) // есть обе полосы

	if !sv.OnMouseWheelPixelsMod(10, 10, 0, 40, ModShift) {
		t.Fatal("Shift+колесо должно быть поглощено")
	}
	if got := sv.ScrollX(); got != 40 {
		t.Errorf("Shift+dy=40 дал ScrollX=%d, ждал 40", got)
	}
	sv.mu.Lock()
	vel := sv.vel
	sv.mu.Unlock()
	if vel != 0 || sv.ScrollY() != 0 {
		t.Errorf("Shift+колесо тронуло вертикаль: vel=%v y=%d", vel, sv.ScrollY())
	}

	// Без Shift то же колесо — вертикально, вбок не едет.
	sv.OnMouseWheelPixelsMod(10, 10, 0, 40, 0)
	sv.mu.Lock()
	vel = sv.vel
	sv.mu.Unlock()
	sv.stopInertia()
	if vel <= 0 {
		t.Errorf("колесо без Shift не запустило вертикальную прокрутку, vel=%v", vel)
	}
	if got := sv.ScrollX(); got != 40 {
		t.Errorf("колесо без Shift сдвинуло ScrollX: %d", got)
	}
}

func TestScrollViewH_TickWheel(t *testing.T) {
	sv, _ := hsView(500, 800)

	// Shift+тик — вбок на шаг колеса.
	sv.OnMouseButton(MouseEvent{Button: MouseWheelDown, Pressed: true, Mod: ModShift})
	if got := sv.ScrollX(); got != 40 {
		t.Errorf("Shift+тик вниз: ScrollX=%d, ждал 40", got)
	}
	sv.OnMouseButton(MouseEvent{Button: MouseWheelUp, Pressed: true, Mod: ModShift})
	if got := sv.ScrollX(); got != 0 {
		t.Errorf("Shift+тик вверх: ScrollX=%d, ждал 0", got)
	}
	// Без Shift — вертикально.
	sv.OnMouseButton(MouseEvent{Button: MouseWheelDown, Pressed: true})
	if got := sv.ScrollY(); got != 40 {
		t.Errorf("тик без Shift: ScrollY=%d, ждал 40", got)
	}
	if got := sv.ScrollX(); got != 0 {
		t.Errorf("тик без Shift сдвинул ScrollX=%d", got)
	}
}

// ─── Мышь: ползунок и трек ───────────────────────────────────────────────────

func TestScrollViewH_DragThumb(t *testing.T) {
	sv, _ := hsView(600, 0) // видимая ширина 200, предел 400

	sv.mu.Lock()
	strip := sv.hbarStrip()
	th := sv.hbarThumbLocked()
	track := sv.hbarTrack()
	sv.mu.Unlock()
	if strip.Empty() || th.Empty() {
		t.Fatalf("полосы нет: strip=%v thumb=%v", strip, th)
	}
	y := strip.Min.Y + strip.Dy()/2

	// Берём ползунок за точку на 3 px правее его левого края.
	grab := th.Min.X + 3
	press := MouseEvent{X: grab, Y: y, Button: MouseLeft, Pressed: true}
	if !sv.WantsCapture(press) {
		t.Error("нажатие на ползунок обязано захватывать мышь")
	}
	if !sv.OnMouseButton(press) {
		t.Fatal("нажатие на ползунок не поглощено")
	}
	if got := sv.ScrollX(); got != 0 {
		t.Errorf("нажатие на ползунок сдвинуло содержимое (скачок под курсор): %d", got)
	}

	// Тянем на 20 px вправо — ровно так же должен уехать ползунок.
	sv.OnMouseMove(grab+20, y+40) // курсор может уйти вниз за границу: drag идёт по x
	want := int(hbarScrollForThumbX(track, th.Min.X+20, 400, 200, 600) + 0.5)
	if got := sv.ScrollX(); got != want || got == 0 {
		t.Errorf("после сдвига ползунка на 20 px ScrollX=%d, ждал %d", got, want)
	}
	sv.mu.Lock()
	moved := sv.hbarThumbLocked()
	sv.mu.Unlock()
	if moved.Min.X != th.Min.X+20 {
		t.Errorf("ползунок встал на x=%d, ждал %d", moved.Min.X, th.Min.X+20)
	}

	// Далеко за правым краем — упирается в предел, не дальше.
	sv.OnMouseMove(5000, y)
	if got := sv.ScrollX(); got != 400 {
		t.Errorf("перетаскивание за край дало %d, ждал предел 400", got)
	}
	sv.OnMouseMove(-5000, y)
	if got := sv.ScrollX(); got != 0 {
		t.Errorf("перетаскивание за левый край дало %d, ждал 0", got)
	}

	// Отпустили — дальнейшее движение содержимое не трогает.
	if !sv.OnMouseButton(MouseEvent{X: 0, Y: y, Button: MouseLeft, Pressed: false}) {
		t.Error("отпускание после перетаскивания должно быть поглощено")
	}
	sv.OnMouseMove(strip.Max.X-5, y)
	if got := sv.ScrollX(); got != 0 {
		t.Errorf("после отпускания движение мыши сдвинуло содержимое: %d", got)
	}
	if sv.ScrollY() != 0 {
		t.Errorf("перетаскивание горизонтальной полосы сдвинуло ScrollY=%d", sv.ScrollY())
	}
}

func TestScrollViewH_ClickTrackJumps(t *testing.T) {
	sv, _ := hsView(600, 0)
	sv.mu.Lock()
	strip := sv.hbarStrip()
	th := sv.hbarThumbLocked()
	track := sv.hbarTrack()
	sv.mu.Unlock()
	y := strip.Min.Y + strip.Dy()/2

	// Щелчок мимо ползунка: содержимое прыгает так, что ползунок встаёт серединой под курсор.
	x := 150
	if x >= th.Min.X && x < th.Max.X {
		t.Fatalf("подготовка: точка %d попала на ползунок %v", x, th)
	}
	sv.OnMouseButton(MouseEvent{X: x, Y: y, Button: MouseLeft, Pressed: true})
	want := int(hbarScrollAt(track, x, 400, 200, 600) + 0.5)
	if got := sv.ScrollX(); got != want || got == 0 {
		t.Errorf("прыжок по треку: ScrollX=%d, ждал %d", got, want)
	}
	sv.mu.Lock()
	jumped := sv.hbarThumbLocked()
	sv.mu.Unlock()
	if mid := (jumped.Min.X + jumped.Max.X) / 2; mid < x-2 || mid > x+2 {
		t.Errorf("после прыжка середина ползунка %d далеко от курсора %d", mid, x)
	}

	// Не отпуская, ведём: ползунок идёт за курсором без дёрганья левым краем под него.
	sv.OnMouseMove(x-30, y)
	sv.mu.Lock()
	after := sv.hbarThumbLocked()
	sv.mu.Unlock()
	if d := after.Min.X - (jumped.Min.X - 30); d < -2 || d > 2 {
		t.Errorf("ползунок после сдвига на 30 px влево стоит на %d, ждал около %d", after.Min.X, jumped.Min.X-30)
	}
	sv.OnMouseButton(MouseEvent{X: x - 30, Y: y, Button: MouseLeft, Pressed: false})
}

func TestScrollViewH_BarHitDoesNotReachChildren(t *testing.T) {
	sv, _ := hsView(600, 0)
	sv.mu.Lock()
	strip := sv.hbarStrip()
	sv.mu.Unlock()
	y := strip.Min.Y + 2

	// Щелчок по любому месту полосы — не только по ползунку — захватывается
	// ScrollView, иначе он достался бы ребёнку, лежащему под полосой.
	for _, x := range []int{strip.Min.X + 1, strip.Max.X / 2, strip.Max.X - 1} {
		if !sv.WantsCapture(MouseEvent{X: x, Y: y, Button: MouseLeft, Pressed: true}) {
			t.Errorf("щелчок по полосе в x=%d не захвачен", x)
		}
	}
	// Выше полосы и в углу захватывать нечего.
	for _, pt := range []image.Point{{X: 100, Y: strip.Min.Y - 1}} {
		if sv.WantsCapture(MouseEvent{X: pt.X, Y: pt.Y, Button: MouseLeft, Pressed: true}) {
			t.Errorf("точка %v не на полосе, а захвачена", pt)
		}
	}
	// Правая кнопка — не наше.
	if sv.WantsCapture(MouseEvent{X: 100, Y: y, Button: MouseRight, Pressed: true}) {
		t.Error("правая кнопка не должна захватываться")
	}
}

func TestScrollViewH_CornerClickIsDead(t *testing.T) {
	sv, _ := hsView(500, 400)
	// Угол между полосами не принадлежит вертикальной: прыжок по ней отсюда
	// считал бы долю от высоты, которой тут нет.
	if sv.OnMouseButton(MouseEvent{X: 195, Y: 95, Button: MouseLeft, Pressed: true}) {
		t.Error("щелчок в углу между полосами не должен ничего делать")
	}
	if sv.ScrollY() != 0 || sv.ScrollX() != 0 {
		t.Errorf("щелчок в углу сдвинул содержимое: x=%d y=%d", sv.ScrollX(), sv.ScrollY())
	}
	if sv.WantsCapture(MouseEvent{X: 195, Y: 95, Button: MouseLeft, Pressed: true}) {
		t.Error("угол между полосами не должен захватывать мышь")
	}
}

// Вертикальная полоса при наличии горизонтальной по-прежнему тянется до конца
// содержимого: нижние строки не остаются под горизонтальной полосой.
func TestScrollViewH_VerticalReachesBottomAboveHBar(t *testing.T) {
	sv, _ := hsView(500, 400)
	sv.SetScrollY(10000)
	if got := sv.ScrollY(); got != 310 { // 400 - (100-10)
		t.Errorf("предел вертикали %d, ждал 310 (высота полосы вычтена)", got)
	}
	sv.mu.Lock()
	th := sv.thumbRect()
	vb := sv.vbarRect()
	sv.mu.Unlock()
	if th.Max.Y > vb.Max.Y {
		t.Errorf("вертикальный ползунок %v выходит за трек %v", th, vb)
	}
	if th.Max.Y < vb.Max.Y-1 {
		t.Errorf("в конце прокрутки ползунок %v не доходит до низа трека %v", th, vb)
	}
}

// Draw идёт в рендер-горутине, мышь и колесо — в другой: гонки не должно быть.
func TestScrollViewH_RaceDrawVsInput(t *testing.T) {
	sv, _ := hsView(900, 600)
	const iters = 300
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			sv.Draw(&hsRec{})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			sv.OnMouseMove(i%200, 95)
			sv.OnMouseButton(MouseEvent{X: 20 + i%100, Y: 95, Button: MouseLeft, Pressed: i%2 == 0})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			sv.OnMouseWheelPixelsMod(10, 10, float64(i%7-3), 0, 0)
			sv.SetScrollX(i)
			_ = sv.ScrollX()
		}
	}()
	wg.Wait()
}
