package engine

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// opaqueNoise — непрозрачное RGBA-изображение с неоднородным содержимым.
func opaqueNoise(w, h int, seed int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := img.PixOffset(x, y)
			img.Pix[o+0] = uint8(x*7 + seed)
			img.Pix[o+1] = uint8(y*11 + seed*3)
			img.Pix[o+2] = uint8((x ^ y) + seed*5)
			img.Pix[o+3] = 255
		}
	}
	return img
}

// Непрозрачный источник, положенный быстрым путём (Src, полосами по ядрам),
// даёт тот же кадр, что и композиция Over, — для каждого типа источника,
// порядка каналов, масштаба, клипа и скруглённого клипа, малых и больших
// (параллельных) областей.
func TestOpaqueBlit_EqualsOver(t *testing.T) {
	testreset.Globals(t)
	type srcMaker struct {
		name string
		make func(w, h int) image.Image
	}
	makers := []srcMaker{
		{"RGBA", func(w, h int) image.Image { return opaqueNoise(w, h, 1) }},
		{"NRGBA", func(w, h int) image.Image {
			n := image.NewNRGBA(image.Rect(0, 0, w, h))
			copy(n.Pix, opaqueNoise(w, h, 2).Pix)
			return n
		}},
		{"YCbCr", func(w, h int) image.Image {
			y := image.NewYCbCr(image.Rect(0, 0, w, h), image.YCbCrSubsampleRatio420)
			for i := range y.Y {
				y.Y[i] = uint8(i * 3)
			}
			for i := range y.Cb {
				y.Cb[i], y.Cr[i] = uint8(i*5), uint8(255-i)
			}
			return y
		}},
		{"Gray", func(w, h int) image.Image {
			g := image.NewGray(image.Rect(0, 0, w, h))
			for i := range g.Pix {
				g.Pix[i] = uint8(i * 13)
			}
			return g
		}},
		{"RGBA со смещением Min", func(w, h int) image.Image {
			return opaqueNoise(w+9, h+7, 3).SubImage(image.Rect(4, 3, 4+w, 3+h))
		}},
	}
	sizes := [][2]int{{64, 40}, {700, 500}} // второй — больше порога параллельности
	for _, mk := range makers {
		for _, f := range []PixelFormat{FormatRGBA, FormatBGRX} {
			for _, sz := range sizes {
				for _, mode := range []string{"plain", "clip", "round"} {
					w, h := sz[0], sz[1]
					cw, ch := w+60, h+50
					fast := imageRoundCanvas(cw, ch, 1, f)
					ref := imageRoundCanvas(cw, ch, 1, f)
					src := mk.make(w, h)
					prep := func(c *Canvas) {
						switch mode {
						case "clip":
							c.SetClip(image.Rect(20, 15, cw-25, ch-20))
						case "round":
							c.SetRoundClip(image.Rect(20, 15, cw-25, ch-20), 18)
						}
					}
					prep(fast)
					fast.DrawImage(widget.OpaqueImage(src), 17, 11)
					prep(ref)
					ref.DrawImage(src, 17, 11)
					for _, c := range []*Canvas{fast, ref} {
						c.ClearRoundClip()
						c.ClearClip()
					}
					if !bytes.Equal(fast.back.Pix, ref.back.Pix) {
						t.Errorf("%s fmt=%v %dx%d %s: быстрый путь расходится с Over (макс. разница %d)",
							mk.name, f, w, h, mode, maxPixDiff(fast.back, ref.back))
					}
				}
			}
		}
	}
}

// Путь действительно быстрый: полупрозрачные пиксели, объявленные
// непрозрачными, ПЕРЕЗАПИСЫВАЮТ фон (Src), а без объявления смешиваются (Over).
// Различие и доказывает, что выбрана композиция Src — для DrawImage,
// DrawImageScaled 1:1 и DrawImageRect 1:1, малой и большой (параллельной)
// области, обоих форматов.
func TestOpaqueBlit_UsesSrcComposition(t *testing.T) {
	testreset.Globals(t)
	half := color.RGBA{R: 100, G: 50, B: 20, A: 128}
	bg := color.RGBA{R: 30, G: 60, B: 90, A: 255}
	draws := map[string]func(c *Canvas, img image.Image, w, h int){
		"DrawImage": func(c *Canvas, img image.Image, w, h int) { c.DrawImage(img, 0, 0) },
		"DrawImageScaled": func(c *Canvas, img image.Image, w, h int) {
			c.DrawImageScaled(img, 0, 0, w, h)
		},
		"DrawImageRect": func(c *Canvas, img image.Image, w, h int) {
			c.DrawImageRect(img, image.Rect(0, 0, w, h), image.Rect(0, 0, w, h), widget.FilterBilinear)
		},
	}
	for name, draw := range draws {
		for _, f := range []PixelFormat{FormatRGBA, FormatBGRX} {
			for _, sz := range [][2]int{{40, 30}, {800, 600}} {
				w, h := sz[0], sz[1]
				src := flatImage(w, h, half)

				c := imageRoundCanvas(w, h, 1, f)
				draw(c, widget.OpaqueImage(src), w, h)
				want := half
				if f == FormatBGRX {
					want.R, want.B = want.B, want.R
				}
				for _, p := range [][2]int{{0, 0}, {w - 1, h - 1}, {w / 2, h / 2}, {3, h - 2}} {
					o := c.back.PixOffset(p[0], p[1])
					got := color.RGBA{c.back.Pix[o], c.back.Pix[o+1], c.back.Pix[o+2], c.back.Pix[o+3]}
					if got != want {
						t.Fatalf("%s fmt=%v %dx%d: пиксель %v = %v, ожидался Src %v", name, f, w, h, p, got, want)
					}
				}

				// Без объявления — прежний Over: фон просвечивает.
				c2 := imageRoundCanvas(w, h, 1, f)
				draw(c2, src, w, h)
				o := c2.back.PixOffset(w/2, h/2)
				got := color.RGBA{c2.back.Pix[o], c2.back.Pix[o+1], c2.back.Pix[o+2], c2.back.Pix[o+3]}
				if got == want {
					t.Fatalf("%s fmt=%v %dx%d: без объявления тоже получен Src %v — путь Over потерян", name, f, w, h, got)
				}
				_ = bg
			}
		}
	}
}

// DrawImageRect при масштабе 1:1 даёт то же, что полный пересчёт ядром
// (тождество), для *image.RGBA — в том числе с альфой.
func TestDrawImageRect_OneToOneMatchesResample(t *testing.T) {
	testreset.Globals(t)
	src := opaqueNoise(90, 70, 4)
	for i := 3; i < len(src.Pix); i += 4 * 5 { // немного полупрозрачных
		src.Pix[i-3], src.Pix[i-2], src.Pix[i-1], src.Pix[i] = 20, 30, 40, 100
	}
	for _, f := range []widget.ImageFilter{widget.FilterNearest, widget.FilterBilinear, widget.FilterCatmullRom} {
		c := imageRoundCanvas(150, 120, 1, FormatRGBA)
		ref := imageRoundCanvas(150, 120, 1, FormatRGBA)
		sr := image.Rect(5, 3, 85, 63)
		dr := image.Rect(30, 20, 110, 80)
		c.SetClip(image.Rect(40, 25, 100, 70))
		c.DrawImageRect(src, sr, dr, f)
		c.ClearClip()

		ref.SetClip(image.Rect(40, 25, 100, 70))
		vis := ref.clampRect(ref.sRect(dr)).Intersect(ref.back.Bounds())
		sc := &rectScratch{}
		tmp := sc.resampleRect(src, sr, ref.sRect(dr), vis, f)
		ref.blitOver(vis, tmp, tmp.Rect.Min)
		ref.ClearClip()
		if !bytes.Equal(c.back.Pix, ref.back.Pix) {
			t.Errorf("фильтр %v: 1:1 расходится с пересчётом (на %d)", f, maxPixDiff(c.back, ref.back))
		}
	}
}

// Кэш масштабирования ключуется по обёрнутому изображению: сброс по любому из
// двух указателей — по обёртке или по самой картинке — убирает запись.
func TestOpaqueImage_ScaledCacheKeyedByInner(t *testing.T) {
	testreset.Globals(t)
	c := imageRoundCanvas(100, 100, 1, FormatRGBA)
	src := opaqueNoise(32, 32, 1)
	wrapped := widget.OpaqueImage(src)
	c.DrawImageScaled(wrapped, 0, 0, 50, 50)
	c.DrawImageScaled(src, 0, 0, 50, 50)
	if len(c.scaledCache) != 1 {
		t.Fatalf("обёртка и сама картинка должны делить запись кэша, записей: %d", len(c.scaledCache))
	}
	c.InvalidateImageCache(wrapped)
	if len(c.scaledCache) != 0 {
		t.Fatalf("сброс по обёртке не убрал запись, осталось %d", len(c.scaledCache))
	}
}

// ─── Переработка буферов кадров ─────────────────────────────────────────────

func recycleEngine(t *testing.T, w, h int) (*Engine, *widget.ImageWidget, [2]*image.RGBA) {
	t.Helper()
	eng := New(w, h, 20)
	t.Cleanup(eng.Stop)
	// Кадры готовит только тест: фоновый тикер не должен вклиниваться со своими
	// кадрами в сток, канал и пул буферов.
	eng.SetPacing(PacingExternal)
	bufs := [2]*image.RGBA{opaqueNoise(w, h, 1), opaqueNoise(w, h, 9)}
	iw := widget.NewImageWidget()
	iw.Stretch = widget.ImageStretchNone
	iw.SetBounds(image.Rect(0, 0, w, h))
	iw.SetImage(bufs[0])
	eng.SetRoot(iw)
	return eng, iw, bufs
}

func cloneTiles(tiles []output.DirtyTile) [][]byte {
	out := make([][]byte, len(tiles))
	for i, t := range tiles {
		out[i] = append([]byte(nil), t.Data...)
	}
	return out
}

// По умолчанию переработки нет: Data кадра принадлежит потребителю навсегда,
// Release — пустой вызов, а кадры не перезаписывают друг друга.
func TestFrameRecycling_OffByDefaultKeepsData(t *testing.T) {
	testreset.Globals(t)
	eng, iw, bufs := recycleEngine(t, 256, 192)
	if eng.FrameRecycling() {
		t.Fatal("переработка включена по умолчанию")
	}
	f1 := eng.renderFrame()
	want := cloneTiles(f1.Tiles)
	f1.Release() // без переработки — ничего не делает
	for i := 0; i < 21; i++ {
		iw.SetImage(bufs[(i+1)&1])
		eng.renderFrame().Release()
	}
	for i, tl := range f1.Tiles {
		if !bytes.Equal(tl.Data, want[i]) {
			t.Fatalf("тайл %d кадра изменился после Release без переработки", i)
		}
	}
}

// С переработкой: буфер после Release достаётся следующему кадру (память
// действительно переиспользуется), а кадр, который потребитель не освободил,
// остаётся нетронутым сколько бы кадров ни прошло.
func TestFrameRecycling_ReleaseReusesAndHeldFrameIsIntact(t *testing.T) {
	testreset.Globals(t)
	eng, iw, bufs := recycleEngine(t, 256, 192)
	eng.SetFrameRecycling(true)

	held := eng.renderFrame() // потребитель держит и не освобождает
	heldCopy := cloneTiles(held.Tiles)

	reused := false
	var prev *byte
	for i := 0; i < 30; i++ {
		iw.SetImage(bufs[(i+1)&1])
		fr := eng.renderFrame()
		if len(fr.Tiles) == 0 {
			t.Fatal("кадр без тайлов")
		}
		p := &fr.Tiles[0].Data[0]
		if prev != nil && p == prev {
			reused = true
		}
		prev = p
		fr.Release()
	}
	if !reused {
		t.Error("после Release буфер ни разу не достался следующему кадру")
	}
	for i, tl := range held.Tiles {
		if !bytes.Equal(tl.Data, heldCopy[i]) {
			t.Fatalf("неосвобождённый кадр изменился (тайл %d)", i)
		}
	}
}

// Повторный Release (в том числе по копии кадра) не отдаёт один буфер двум
// следующим кадрам.
func TestFrameRecycling_DoubleReleaseIsSafe(t *testing.T) {
	testreset.Globals(t)
	eng, iw, bufs := recycleEngine(t, 256, 192)
	eng.SetFrameRecycling(true)
	fr := eng.renderFrame()
	cp := fr
	fr.Release()
	cp.Release()
	fr.Release()

	iw.SetImage(bufs[1])
	a := eng.renderFrame()
	iw.SetImage(bufs[0])
	b := eng.renderFrame()
	if len(a.Tiles) == 0 || len(b.Tiles) == 0 {
		t.Fatal("пустые кадры")
	}
	if &a.Tiles[0].Data[0] == &b.Tiles[0].Data[0] {
		t.Fatal("один буфер отдан двум живым кадрам")
	}
	// Содержимое обоих — свои.
	if bytes.Equal(a.Tiles[0].Data, b.Tiles[0].Data) {
		t.Fatal("содержимое двух кадров совпало — буфер общий")
	}
}

type retainSink struct{ got []output.Frame }

func (s *retainSink) Present(f output.Frame) { s.got = append(s.got, f) }

// Кадры, показанные синхронному стоку, в пул не возвращаются: что сток сделал
// с Data, движок не знает.
func TestFrameRecycling_SinkFramesArePinned(t *testing.T) {
	testreset.Globals(t)
	eng, iw, bufs := recycleEngine(t, 256, 192)
	eng.SetFrameRecycling(true)
	sink := &retainSink{}
	eng.SetFrameSink(sink)

	fr := eng.renderFrame()
	eng.deliver(fr)
	want := cloneTiles(fr.Tiles)
	fr.Release() // потребитель канала дошёл до конца — но кадр закреплён стоком
	for i := 0; i < 11; i++ {
		iw.SetImage(bufs[(i+1)&1])
		f := eng.renderFrame()
		f.Release()
	}
	for i, tl := range sink.got[0].Tiles {
		if !bytes.Equal(tl.Data, want[i]) {
			t.Fatalf("кадр стока перезаписан (тайл %d)", i)
		}
	}
}

// Кадр, выброшенный из переполненного канала, возвращается в пул самим
// движком: его буфер достаётся следующему кадру. (sync.Pool под -race вправе
// отбросить часть возвратов — поэтому несколько попыток.)
func TestFrameRecycling_DroppedFrameGoesBack(t *testing.T) {
	testreset.Globals(t)
	eng, iw, bufs := recycleEngine(t, 256, 192)
	eng.SetFrameRecycling(true)
	for len(eng.frames) < cap(eng.frames) { // заполняем канал
		eng.frames <- output.Frame{}
	}
	reused := false
	for i := 0; i < 20 && !reused; i++ {
		iw.SetImage(bufs[(i+1)&1])
		fr := eng.renderFrame()
		first := &fr.Tiles[0].Data[0]
		eng.deliver(fr) // канал полон — кадр выброшен и освобождён
		if !eng.lostFrame.Load() {
			t.Fatal("потеря кадра не запомнена")
		}
		iw.SetImage(bufs[i&1])
		next := eng.renderFrame()
		reused = &next.Tiles[0].Data[0] == first
		next.Release()
	}
	if !reused {
		t.Fatal("буфер выброшенного кадра ни разу не вернулся в пул")
	}
}

// Содержимое кадров с переработкой совпадает с кадрами без неё.
func TestFrameRecycling_SameFrames(t *testing.T) {
	testreset.Globals(t)
	run := func(recycle bool) [][][]byte {
		eng, iw, bufs := recycleEngine(t, 300, 200)
		eng.SetFrameRecycling(recycle)
		var out [][][]byte
		for i := 0; i < 6; i++ {
			iw.SetImage(bufs[i&1])
			fr := eng.renderFrame()
			out = append(out, cloneTiles(fr.Tiles))
			fr.Release()
		}
		return out
	}
	a, b := run(false), run(true)
	for i := range a {
		if len(a[i]) != len(b[i]) {
			t.Fatalf("кадр %d: число тайлов %d и %d", i, len(a[i]), len(b[i]))
		}
		for j := range a[i] {
			if !bytes.Equal(a[i][j], b[i][j]) {
				t.Fatalf("кадр %d тайл %d: данные расходятся", i, j)
			}
		}
	}
}
