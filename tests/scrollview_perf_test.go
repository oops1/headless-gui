package tests

// Регрессионные тесты оптимизации PERF-12: ScrollView рисует содержимое со
// смещением через транслирующий DrawContext и БОЛЬШЕ НЕ подменяет bounds
// дочерних виджетов на время отрисовки.

import (
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// opRec — одна записанная операция рисования (имя + геометрия/текст).
type opRec struct {
	op   string
	x    int
	y    int
	w    int
	h    int
	text string
}

// opCtx — DrawContext, записывающий все операции с координатами.
// Нужен, чтобы сравнить ПОТОК ОТРИСОВКИ старого и нового путей ScrollView.
type opCtx struct {
	ops  []opRec
	clip image.Rectangle
}

func (c *opCtx) add(op string, x, y, w, h int, text string) {
	c.ops = append(c.ops, opRec{op, x, y, w, h, text})
}

func (c *opCtx) FillRect(x, y, w, h int, col color.RGBA)      { c.add("FillRect", x, y, w, h, "") }
func (c *opCtx) FillRectAlpha(x, y, w, h int, col color.RGBA) { c.add("FillRectAlpha", x, y, w, h, "") }
func (c *opCtx) FillRoundRect(x, y, w, h, r int, col color.RGBA) {
	c.add("FillRoundRect", x, y, w, h, "")
}
func (c *opCtx) DrawBorder(x, y, w, h int, col color.RGBA) { c.add("DrawBorder", x, y, w, h, "") }
func (c *opCtx) DrawRoundBorder(x, y, w, h, r int, col color.RGBA) {
	c.add("DrawRoundBorder", x, y, w, h, "")
}
func (c *opCtx) SetPixel(x, y int, col color.RGBA)          { c.add("SetPixel", x, y, 0, 0, "") }
func (c *opCtx) DrawHLine(x, y, length int, col color.RGBA) { c.add("DrawHLine", x, y, length, 0, "") }
func (c *opCtx) DrawVLine(x, y, length int, col color.RGBA) { c.add("DrawVLine", x, y, 0, length, "") }
func (c *opCtx) DrawImage(src image.Image, x, y int)        { c.add("DrawImage", x, y, 0, 0, "") }
func (c *opCtx) DrawImageScaled(src image.Image, x, y, w, h int) {
	c.add("DrawImageScaled", x, y, w, h, "")
}
func (c *opCtx) DrawText(text string, x, y int, col color.RGBA) {
	c.add("DrawText", x, y, 0, 0, text)
}
func (c *opCtx) DrawTextSize(text string, x, y int, sizePt float64, col color.RGBA) {
	c.add("DrawTextSize", x, y, 0, 0, text)
}
func (c *opCtx) DrawTextFont(text string, x, y int, sizePt float64, fontName string, col color.RGBA) {
	c.add("DrawTextFont", x, y, 0, 0, text)
}
func (c *opCtx) MeasureText(text string, sizePt float64) int { return len([]rune(text)) * 7 }
func (c *opCtx) MeasureTextFont(text string, sizePt float64, fontName string) int {
	return len([]rune(text)) * 7
}
func (c *opCtx) MeasureRunePositions(text string, sizePt float64) []int {
	pos := make([]int, len([]rune(text))+1)
	for i := range pos {
		pos[i] = i * 7
	}
	return pos
}
func (c *opCtx) SetClip(r image.Rectangle) {
	c.clip = r
	c.add("SetClip", r.Min.X, r.Min.Y, r.Dx(), r.Dy(), "")
}
func (c *opCtx) ClearClip() {
	c.clip = image.Rect(0, 0, 1<<15, 1<<15)
	c.add("ClearClip", 0, 0, 0, 0, "")
}
func (c *opCtx) Clip() image.Rectangle {
	if c.clip.Empty() {
		return image.Rect(0, 0, 1<<15, 1<<15)
	}
	return c.clip
}

// buildScrollView собирает ScrollView 200×120 с тремя лейблами.
// ContentHeight намеренно оставляем МЕНЬШЕ высоты: тогда скроллбар не рисуется
// и поток операций содержит только детей — сравнивать можно один в один.
// Смещение задаётся напрямую (см. forceScroll).
func buildScrollView() (*widget.ScrollView, []widget.Widget) {
	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 200, 120))
	var kids []widget.Widget
	for i := 0; i < 3; i++ {
		l := widget.NewLabel("строка", color.RGBA{R: 200, G: 200, B: 200, A: 255})
		l.SetBounds(image.Rect(4, 10+i*30, 180, 34+i*30))
		sv.AddChild(l)
		kids = append(kids, l)
	}
	return sv, kids
}

// forceScroll выставляет смещение, временно подняв ContentHeight (SetScrollY
// зажимает значение по maxScroll), и возвращает ContentHeight обратно —
// скроллбар в Draw рисоваться не будет, а scrollY останется заданным.
func forceScroll(sv *widget.ScrollView, y int) {
	sv.ContentHeight = 10000
	sv.SetScrollY(y)
	sv.ContentHeight = 50
}

// TestScrollViewDrawKeepsChildBounds — после Draw bounds детей не изменились,
// и ни один Draw не оставил их сдвинутыми.
func TestScrollViewDrawKeepsChildBounds(t *testing.T) {
	sv, kids := buildScrollView()
	before := make([]image.Rectangle, len(kids))
	for i, k := range kids {
		before[i] = k.Bounds()
	}
	forceScroll(sv, 40)

	sv.Draw(&opCtx{})

	for i, k := range kids {
		if k.Bounds() != before[i] {
			t.Fatalf("ребёнок %d: bounds изменились после Draw: %v → %v", i, before[i], k.Bounds())
		}
	}
}

// TestScrollViewDrawOffsetMatchesShiftedBounds — новый путь (смещение через
// контекст) рисует РОВНО то же, что прежний (временно сдвинутые bounds).
func TestScrollViewDrawOffsetMatchesShiftedBounds(t *testing.T) {
	const dy = 40

	// Новый путь: дети на своих местах, scrollY = dy.
	svNew, _ := buildScrollView()
	forceScroll(svNew, dy)
	gotCtx := &opCtx{}
	svNew.Draw(gotCtx)

	// Эталон: дети заранее сдвинуты на -dy, scrollY = 0 (обёртка не включается,
	// путь идентичен прежнему SetBounds(shifted) → Draw).
	svRef, refKids := buildScrollView()
	svRef.ContentHeight = 50
	for _, k := range refKids {
		k.SetBounds(k.Bounds().Add(image.Pt(0, -dy)))
	}
	wantCtx := &opCtx{}
	svRef.Draw(wantCtx)

	if len(gotCtx.ops) != len(wantCtx.ops) {
		t.Fatalf("разное число операций: %d против %d\n%v\n%v",
			len(gotCtx.ops), len(wantCtx.ops), gotCtx.ops, wantCtx.ops)
	}
	for i := range gotCtx.ops {
		if gotCtx.ops[i] != wantCtx.ops[i] {
			t.Fatalf("операция %d отличается: %+v против %+v", i, gotCtx.ops[i], wantCtx.ops[i])
		}
	}
}

// TestScrollViewIdleDoesNotRender — прокрученный ScrollView в покое НЕ должен
// заставлять движок рендерить кадры.
//
// Прежний Draw звал child.SetBounds дважды на ребёнка, а Base.SetBounds шлёт
// notifyRectChanged → движок инвалидировался прямо во время отрисовки, и
// on-demand рендер с живым ScrollView крутился на полном FPS вечно.
func TestScrollViewIdleDoesNotRender(t *testing.T) {
	eng, _ := newOnDemandEngine() // 50 fps, tooltips выключены
	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(10, 10, 300, 150))
	for i := 0; i < 5; i++ {
		l := widget.NewLabel("строка", color.RGBA{R: 200, G: 200, B: 200, A: 255})
		l.SetBounds(image.Rect(14, 14+i*30, 280, 38+i*30))
		sv.AddChild(l)
	}
	sv.ContentHeight = 400
	sv.SetScrollY(40)
	eng.Root().AddChild(sv)
	eng.SetRenderOnDemand(true)
	eng.Start()
	defer eng.Stop()

	waitCount(eng, 1)
	time.Sleep(150 * time.Millisecond)
	base := eng.RenderCount()
	time.Sleep(400 * time.Millisecond) // ~20 тиков
	if got := eng.RenderCount(); got > base+1 {
		t.Fatalf("прокрученный ScrollView в покое родил %d кадров (%d → %d)", got-base, base, got)
	}
}

// TestScrollViewDrawRaceWithHitTest — hit-test (чтение bounds ребёнка) идёт
// параллельно с Draw и НЕ должен ловить гонку данных.
// Прежний код мутировал bounds на время отрисовки — под -race тест падал.
func TestScrollViewDrawRaceWithHitTest(t *testing.T) {
	sv, kids := buildScrollView()
	forceScroll(sv, 40)

	const iters = 300
	var wg sync.WaitGroup
	wg.Add(2)

	go func() { // рендер-горутина
		defer wg.Done()
		ctx := &opCtx{}
		for i := 0; i < iters; i++ {
			ctx.ops = ctx.ops[:0]
			sv.Draw(ctx)
		}
	}()

	go func() { // «горутина событий»: hit-test по дереву
		defer wg.Done()
		for i := 0; i < iters; i++ {
			for _, k := range kids {
				_ = k.Bounds().Dy()
			}
		}
	}()

	wg.Wait()
}
