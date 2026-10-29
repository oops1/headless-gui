package engine

import (
	hgraster "github.com/oops1/headless-gui/v3/core/raster"
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// E7: widget.ExternalSurface — чужой буфер выводится в холст без копии в
// кэш, а заявленные приложением области доходят до диффа движка как есть.

// esRig — движок lw×lh с корнем-панелью и поверхностью в заданных границах.
type esRig struct {
	e    *Engine
	root *widget.Panel
	surf *widget.ExternalSurface
}

func newESRig(t *testing.T, lw, lh int, scale float64, bounds image.Rectangle) *esRig {
	t.Helper()
	e := New(lw, lh, 20)
	t.Cleanup(e.Stop)
	e.SetTooltipsEnabled(false)
	if scale != 1 {
		e.SetScale(scale)
	}
	root := widget.NewPanel(color.RGBA{R: 200, G: 30, B: 30, A: 255})
	root.SetBounds(image.Rect(0, 0, lw, lh))
	surf := widget.NewExternalSurface()
	surf.SetBounds(bounds)
	root.AddChild(surf)
	e.SetRoot(root)
	return &esRig{e: e, root: root, surf: surf}
}

// frame рисует кадр и возвращает номера затронутых тайлов (по их углам).
func (r *esRig) frame() map[image.Point]bool {
	fr := r.e.renderFrame()
	defer fr.Release()
	out := map[image.Point]bool{}
	for _, tl := range fr.Tiles {
		out[image.Pt(tl.X, tl.Y)] = true
	}
	return out
}

// tilesOf — углы всех тайлов 64×64, которые пересекает физический rect.
func tilesOf(r image.Rectangle) map[image.Point]bool {
	out := map[image.Point]bool{}
	ts := output.TileSize
	for ty := r.Min.Y / ts; ty <= (r.Max.Y-1)/ts; ty++ {
		for tx := r.Min.X / ts; tx <= (r.Max.X-1)/ts; tx++ {
			out[image.Pt(tx*ts, ty*ts)] = true
		}
	}
	return out
}

func sameSet(a, b map[image.Point]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// backPixel — пиксель кадра (физические координаты): front, при одном
// буфере кадра back холста движка — окно (ADR-0007).
func (r *esRig) backPixel(x, y int) color.RGBA { return r.e.canvas.Front().RGBAAt(x, y) }

// esNoise — непрозрачный буфер; seed меняет все пиксели.
func esNoise(w, h, seed int) *image.RGBA { return opaqueNoise(w, h, seed) }

// refill перекрашивает прямоугольник буфера новым содержимым (каждый пиксель
// отличается от прежнего).
func refill(buf *image.RGBA, r image.Rectangle, seed int) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			o := buf.PixOffset(x, y)
			buf.Pix[o+0] = uint8(x*7 + seed)
			buf.Pix[o+1] = uint8(y*11 + seed*3)
			buf.Pix[o+2] = uint8((x ^ y) + seed*5)
			buf.Pix[o+3] = 255
		}
	}
}

// checkMatches сверяет физическую область холста с буфером: пиксель буфера
// (bx, by) лежит в холсте в (org.X+bx, org.Y+by).
func (r *esRig) checkMatches(t *testing.T, buf *image.RGBA, org image.Point, area image.Rectangle) {
	t.Helper()
	bad := 0
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			got := r.backPixel(org.X+x, org.Y+y)
			want := buf.RGBAAt(x, y)
			if got != want {
				if bad++; bad <= 3 {
					t.Errorf("пиксель буфера (%d,%d): холст %v, буфер %v", x, y, got, want)
				}
			}
		}
	}
	if bad > 0 {
		t.Fatalf("расхождений: %d", bad)
	}
}

// Главное свойство: в кадре меняются только тайлы заявленной области, и их
// пиксели равны буферу приложения.
func TestExternalSurface_OnlyDamagedTilesChange(t *testing.T) {
	testreset.Globals(t)
	r := newESRig(t, 512, 384, 1, image.Rect(0, 0, 512, 384))
	buf := esNoise(512, 384, 1)
	r.surf.SetBuffer(buf, true)
	if tiles := r.frame(); len(tiles) != 8*6 {
		t.Fatalf("первый кадр: %d тайлов, ждали все %d", len(tiles), 8*6)
	}
	r.checkMatches(t, buf, image.Point{}, buf.Rect)

	// Ничего не заявлено — кадр пуст.
	if tiles := r.frame(); len(tiles) != 0 {
		t.Fatalf("кадр без изменений дал %d тайлов", len(tiles))
	}

	dmg := image.Rect(130, 70, 200, 110)
	r.surf.Update(func(b *image.RGBA) []image.Rectangle {
		refill(b, dmg, 9)
		return []image.Rectangle{dmg}
	})
	tiles := r.frame()
	if want := tilesOf(dmg); !sameSet(tiles, want) {
		t.Fatalf("тайлы кадра %v, ждали только пересекающие %v: %v", tiles, dmg, want)
	}
	r.checkMatches(t, buf, image.Point{}, buf.Rect)

	// Две далёкие области: дифф идёт по каждой, а не по охвату.
	a, b := image.Rect(10, 10, 40, 30), image.Rect(450, 340, 500, 380)
	r.surf.Update(func(buf *image.RGBA) []image.Rectangle {
		refill(buf, a, 20)
		refill(buf, b, 21)
		return []image.Rectangle{a, b}
	})
	want := tilesOf(a)
	for k := range tilesOf(b) {
		want[k] = true
	}
	if tiles := r.frame(); !sameSet(tiles, want) {
		t.Fatalf("две области: тайлы %v, ждали %v", tiles, want)
	}
	r.checkMatches(t, buf, image.Point{}, buf.Rect)
}

// Поверхность не в углу холста и буфер с ненулевым Rect.Min: области
// переводятся в координаты кадра.
func TestExternalSurface_OffsetBoundsAndBufferOrigin(t *testing.T) {
	testreset.Globals(t)
	bounds := image.Rect(100, 50, 400, 250)
	r := newESRig(t, 512, 384, 1, bounds)
	// Буфер с началом в (10, 20): его левый верхний пиксель — в (100, 50).
	buf := image.NewRGBA(image.Rect(10, 20, 310, 220))
	refill(buf, buf.Rect, 3)
	r.surf.SetBuffer(buf, true)
	r.frame()
	r.checkMatches(t, buf, image.Pt(100-10, 50-20), buf.Rect)

	dmg := image.Rect(60, 80, 120, 120) // в координатах буфера
	r.surf.Update(func(b *image.RGBA) []image.Rectangle {
		refill(b, dmg, 8)
		return []image.Rectangle{dmg}
	})
	phys := dmg.Add(image.Pt(100-10, 50-20))
	if tiles := r.frame(); !sameSet(tiles, tilesOf(phys)) {
		t.Fatalf("тайлы %v, ждали %v", tiles, tilesOf(phys))
	}
	r.checkMatches(t, buf, image.Pt(100-10, 50-20), buf.Rect)
}

// Область вне буфера и вне виджета движку не заявляется; полупересекающаяся
// обрезается.
func TestExternalSurface_DamageOutsideIsIgnored(t *testing.T) {
	testreset.Globals(t)
	// Виджет больше буфера: отсекает область именно буфер, а не границы.
	r := newESRig(t, 400, 300, 1, image.Rect(50, 40, 350, 290))
	buf := esNoise(200, 150, 1)
	r.surf.SetBuffer(buf, true)
	r.frame()
	r.e.consumeDamage()

	r.surf.InvalidateRects([]image.Rectangle{
		image.Rect(500, 500, 600, 600), // совсем вне
		image.Rect(-50, -50, 0, 0),     // вне, с краю
		{},                             // пустая
		image.Rect(10, 10, 10, 40),     // вырожденная
	})
	if rects, all := r.e.consumeDamage(); len(rects) != 0 || all {
		t.Fatalf("область вне буфера оставила damage: %v (all=%v)", rects, all)
	}

	// Частично внутри: заявляется только пересечение с буфером (в кадре — со
	// смещением виджета).
	r.surf.InvalidateRects([]image.Rectangle{image.Rect(150, 100, 300, 300)})
	rects, all := r.e.consumeDamage()
	if all || len(rects) != 1 || rects[0] != image.Rect(200, 140, 250, 190) {
		t.Fatalf("damage = %v (all=%v), ждали [200,140 250,190]", rects, all)
	}

	// Буфера нет — заявлять нечего.
	r.surf.SetBuffer(nil, false)
	r.e.consumeDamage()
	r.surf.InvalidateRects([]image.Rectangle{image.Rect(0, 0, 10, 10)})
	if rects, all := r.e.consumeDamage(); len(rects) != 0 || all {
		t.Fatalf("без буфера damage = %v", rects)
	}
}

// Пустой и отсутствующий буфер: виджет закрашивается фоном, паники нет.
func TestExternalSurface_EmptyBuffer(t *testing.T) {
	testreset.Globals(t)
	bounds := image.Rect(20, 20, 120, 100)
	r := newESRig(t, 200, 150, 1, bounds)
	r.surf.SetBackground(color.RGBA{R: 1, G: 2, B: 3, A: 255})
	check := func(what string) {
		t.Helper()
		r.frame()
		for _, p := range []image.Point{{20, 20}, {119, 99}, {60, 50}} {
			if got := r.backPixel(p.X, p.Y); got != (color.RGBA{R: 1, G: 2, B: 3, A: 255}) {
				t.Fatalf("%s: пиксель %v = %v, ждали фон", what, p, got)
			}
		}
		if got := r.backPixel(19, 20); got != (color.RGBA{R: 200, G: 30, B: 30, A: 255}) {
			t.Fatalf("%s: фон залил соседа слева: %v", what, got)
		}
		if got := r.backPixel(120, 99); got != (color.RGBA{R: 200, G: 30, B: 30, A: 255}) {
			t.Fatalf("%s: фон залил соседа справа: %v", what, got)
		}
	}
	check("буфера нет")
	r.surf.SetBuffer(image.NewRGBA(image.Rectangle{}), true)
	check("буфер нулевого размера")
	r.surf.SetBuffer(image.NewRGBA(image.Rect(0, 0, 0, 50)), true)
	check("буфер нулевой ширины")
	r.surf.InvalidateRects([]image.Rectangle{image.Rect(0, 0, 10, 10)})
	r.surf.SetBuffer(nil, true)
	r.surf.Update(func(*image.RGBA) []image.Rectangle { t.Fatal("Update вызвал fn без буфера"); return nil })
	check("снова без буфера")
}

// Смена размера буфера: меньше виджета (края фоном), больше (обрезка),
// возврат; прежний буфер отдаётся Swap.
func TestExternalSurface_BufferResize(t *testing.T) {
	testreset.Globals(t)
	bounds := image.Rect(30, 20, 230, 170) // 200×150
	r := newESRig(t, 300, 220, 1, bounds)
	bg := color.RGBA{R: 5, G: 6, B: 7, A: 255}
	r.surf.SetBackground(bg)

	small := esNoise(120, 90, 1)
	r.surf.SetBuffer(small, true)
	r.frame()
	r.checkMatches(t, small, image.Pt(30, 20), small.Rect)
	for _, p := range []image.Point{{30 + 120, 20}, {30 + 199, 20 + 149}, {30, 20 + 90}} {
		if got := r.backPixel(p.X, p.Y); got != bg {
			t.Fatalf("за краем буфера %v = %v, ждали фон", p, got)
		}
	}

	big := esNoise(500, 400, 2)
	if prev := r.surf.Swap(big, true, nil); prev != small {
		t.Fatalf("Swap вернул %p, ждали прежний буфер %p", prev, small)
	}
	// Полный кадр: клипа кадра нет, обрезать по границам должен сам виджет.
	r.e.Invalidate()
	r.frame()
	r.checkMatches(t, big, image.Pt(30, 20), image.Rect(0, 0, 200, 150))
	// Обрезка по границам виджета: соседи не тронуты.
	if got := r.backPixel(230, 20); got != (color.RGBA{R: 200, G: 30, B: 30, A: 255}) {
		t.Fatalf("буфер больше виджета залез за край: %v", got)
	}
	if got := r.backPixel(30, 170); got != (color.RGBA{R: 200, G: 30, B: 30, A: 255}) {
		t.Fatalf("буфер больше виджета залез под низ: %v", got)
	}

	// Swap с теми же размерами и областью: перерисовывается только она.
	next := esNoise(500, 400, 2)
	dmg := image.Rect(10, 10, 50, 40)
	refill(next, dmg, 77)
	r.surf.Swap(next, true, []image.Rectangle{dmg})
	if tiles := r.frame(); !sameSet(tiles, tilesOf(dmg.Add(image.Pt(30, 20)))) {
		t.Fatalf("Swap с областью: тайлы %v", tiles)
	}
	r.checkMatches(t, next, image.Pt(30, 20), image.Rect(0, 0, 200, 150))
}

// HiDPI: буфер — физические пиксели, масштаб не применяется.
func TestExternalSurface_Scale2PhysicalOneToOne(t *testing.T) {
	testreset.Globals(t)
	// Логические границы 100×50 = физические 200×100 при масштабе 2.
	r := newESRig(t, 300, 200, 2, image.Rect(10, 10, 110, 60))
	buf := esNoise(200, 100, 4)
	r.surf.SetBuffer(buf, true)
	r.frame()
	r.checkMatches(t, buf, image.Pt(20, 20), buf.Rect)
	if got := r.backPixel(19, 20); got != (color.RGBA{R: 200, G: 30, B: 30, A: 255}) {
		t.Fatalf("за левым краем: %v", got)
	}
	if got := r.backPixel(220, 20); got != (color.RGBA{R: 200, G: 30, B: 30, A: 255}) {
		t.Fatalf("за правым краем: %v", got)
	}

	dmg := image.Rect(61, 21, 93, 45) // физические пиксели буфера
	r.surf.Update(func(b *image.RGBA) []image.Rectangle {
		refill(b, dmg, 12)
		return []image.Rectangle{dmg}
	})
	phys := dmg.Add(image.Pt(20, 20))
	if tiles := r.frame(); !sameSet(tiles, tilesOf(phys)) {
		t.Fatalf("scale 2: тайлы %v, ждали %v", tiles, tilesOf(phys))
	}
	r.checkMatches(t, buf, image.Pt(20, 20), buf.Rect)

	// Дробный масштаб: границы округляются по краям, буфер всё равно 1:1.
	r2 := newESRig(t, 200, 120, 1.25, image.Rect(10, 10, 110, 70))
	buf2 := esNoise(125, 75, 6)
	r2.surf.SetBuffer(buf2, true)
	r2.frame()
	ox, oy := 13, 13 // round(10·1.25) = 12.5 → 13 (math.Round от половины — вверх)
	r2.checkMatches(t, buf2, image.Pt(ox, oy), buf2.Rect)
	d2 := image.Rect(30, 20, 77, 41)
	r2.surf.Update(func(b *image.RGBA) []image.Rectangle {
		refill(b, d2, 31)
		return []image.Rectangle{d2}
	})
	r2.frame()
	r2.checkMatches(t, buf2, image.Pt(ox, oy), buf2.Rect)
}

// Чужое повреждение над поверхностью: движок стёр область и перерисовал фон,
// поверхность обязана вернуть там пиксели буфера, хотя приложение их не
// трогало и не заявляло.
func TestExternalSurface_ForeignDamageRestoresPixels(t *testing.T) {
	testreset.Globals(t)
	r := newESRig(t, 300, 200, 1, image.Rect(0, 0, 300, 200))
	buf := esNoise(300, 200, 1)
	r.surf.SetBuffer(buf, true)
	r.frame()

	foreign := image.Rect(100, 60, 180, 120)
	r.e.InvalidateRect(foreign)
	r.frame()
	r.checkMatches(t, buf, image.Point{}, buf.Rect)

	// Тот же случай при сдвоенном повреждении: свою область приложения и чужую
	// движок стирает по охвату.
	dmg := image.Rect(10, 10, 30, 30)
	r.surf.Update(func(b *image.RGBA) []image.Rectangle {
		refill(b, dmg, 5)
		return []image.Rectangle{dmg}
	})
	r.e.InvalidateRect(image.Rect(250, 150, 290, 190))
	r.frame()
	r.checkMatches(t, buf, image.Point{}, buf.Rect)
}

// Непрозрачный буфер объявляет поверхность закрывающей, непрозрачный фон и
// отсутствие буфера — нет.
func TestExternalSurface_OpaqueRegion(t *testing.T) {
	testreset.Globals(t)
	s := widget.NewExternalSurface()
	s.SetBounds(image.Rect(5, 6, 55, 66))
	if got := s.OpaqueRegion(); got != nil {
		t.Fatalf("без буфера: %v", got)
	}
	s.SetBuffer(image.NewRGBA(image.Rect(0, 0, 10, 10)), false)
	if got := s.OpaqueRegion(); got != nil {
		t.Fatalf("непрозрачность не обещана: %v", got)
	}
	s.SetBuffer(image.NewRGBA(image.Rect(0, 0, 10, 10)), true)
	if got := s.OpaqueRegion(); len(got) != 1 || got[0] != s.Bounds() {
		t.Fatalf("OpaqueRegion = %v, ждали границы %v", got, s.Bounds())
	}
	s.SetBackground(color.RGBA{A: 128})
	if got := s.OpaqueRegion(); got != nil {
		t.Fatalf("прозрачный фон: %v", got)
	}
}

// Непрозрачность обещана — Src: альфа и цвет буфера кладутся как есть;
// не обещана — Over: полупрозрачное смешивается с тем, что под ним.
func TestExternalSurface_OpaqueFlagSelectsBlend(t *testing.T) {
	testreset.Globals(t)
	r := newESRig(t, 64, 64, 1, image.Rect(0, 0, 32, 32))
	buf := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for i := 0; i < len(buf.Pix); i += 4 {
		buf.Pix[i], buf.Pix[i+1], buf.Pix[i+2], buf.Pix[i+3] = 10, 20, 30, 128 // premultiplied
	}
	r.surf.SetBackground(color.RGBA{A: 255})
	r.surf.SetBuffer(buf, true)
	r.frame()
	if got := r.backPixel(5, 5); got != (color.RGBA{R: 10, G: 20, B: 30, A: 128}) {
		t.Fatalf("Src: %v", got)
	}
	// Over по фону панели (200,30,30): 10 + 200·(1−128/255).
	r.surf.SetBuffer(buf, false)
	r.frame()
	got := r.backPixel(5, 5)
	if got.A != 255 || got.R <= 100 {
		t.Fatalf("Over: %v, ждали смешивание с фоном под поверхностью", got)
	}
}

// Писатель в отдельной горутине и кадры движка; под -race ловит чтение
// буфера вне замка. В конце холст совпадает с буфером.
func TestExternalSurface_WriterAndFramesRace(t *testing.T) {
	testreset.Globals(t)
	r := newESRig(t, 400, 300, 1, image.Rect(0, 0, 400, 300))
	buf := esNoise(400, 300, 1)
	r.surf.SetBuffer(buf, true)
	r.frame()

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			rc := image.Rect((i*13)%300, (i*7)%200, (i*13)%300+60, (i*7)%200+40)
			r.surf.Update(func(b *image.RGBA) []image.Rectangle {
				refill(b, rc, i)
				return []image.Rectangle{rc}
			})
		}
	}()
	// Дополнительно подменяет буфер на другой того же размера (Swap) и пишет
	// в прежний без замка — безопасно по контракту Swap.
	wg.Add(1)
	go func() {
		defer wg.Done()
		spare := esNoise(400, 300, 2)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			prev := r.surf.Swap(spare, true, nil)
			if prev != nil {
				refill(prev, image.Rect(0, 0, 50, 50), i) // без замка
				spare = prev
			}
		}
	}()
	for i := 0; i < 150; i++ {
		r.frame()
	}
	close(stop)
	wg.Wait()
	r.frame()
	cur := r.surf.Buffer()
	r.surf.Invalidate()
	r.frame()
	r.checkMatches(t, cur, image.Point{}, cur.Rect)
}

// Внутри окна-попапа (translatingContext) координаты сдвигаются.
func TestTranslatingContext_BlitPixels(t *testing.T) {
	testreset.Globals(t)
	src := esNoise(10, 10, 3)
	oc := hgraster.NewCanvas(40, 40, nil)
	tc := &translatingContext{inner: oc, dx: 100, dy: 200}
	tc.BlitPixels(src, src.Rect, 105, 203, true)
	if got := oc.Back().RGBAAt(5, 3); got != src.RGBAAt(0, 0) {
		t.Fatalf("(5,3) = %v", got)
	}
	var _ widget.PixelBlitter = tc
}

// Внутри прокрутки поверхность рисуется со сдвигом.
func TestExternalSurface_InsideScrollView(t *testing.T) {
	testreset.Globals(t)
	c := hgraster.NewCanvas(200, 150, nil)
	oc := widget.OffsetContext(c, 30, 20)
	s := widget.NewExternalSurface()
	s.SetBounds(image.Rect(50, 40, 90, 70))
	buf := esNoise(40, 30, 5)
	s.SetBuffer(buf, true)
	s.Draw(oc)
	// Экранная позиция = координаты содержимого − смещение.
	if got := c.Back().RGBAAt(20, 20); got != buf.RGBAAt(0, 0) {
		t.Fatalf("(20,20) = %v", got)
	}
	if got := c.Back().RGBAAt(59, 49); got != buf.RGBAAt(39, 29) {
		t.Fatalf("(59,49) = %v", got)
	}
	if got := c.Back().RGBAAt(60, 49); got != (color.RGBA{}) {
		t.Fatalf("за правым краем: %v", got)
	}
}
