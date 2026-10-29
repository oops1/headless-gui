package engine

// popuparena_review_test.go — случаи из ревью 5.3 (арена попапов): тяжёлые
// попапы (тень, скругление, Mica, размытие, картинка, прозрачность) против
// свежего клона, режимы текста, смена формата пикселей, сжатие после
// закрытия большого меню, области размытия, переходы, смена разрешения,
// гонки с живым циклом.

import (
	"bytes"
	"image"
	"image/color"
	"sync"
	"testing"
	"time"

	hgdraw "github.com/oops1/headless-gui/v3/core/draw"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// revPopup — хостовый оверлей со всем «тяжёлым»: тень, скруглённый клип,
// Mica, размытие подложки, картинка с масштабом, текст, слой с прозрачностью.
type revPopup struct {
	*widget.Panel
	r     image.Rectangle
	open  bool
	img   *image.RGBA
	hov   int
	clean bool
}

func (p *revPopup) HasOverlay() bool               { return p.open }
func (p *revPopup) OverlayBounds() image.Rectangle { return p.r }
func (p *revPopup) DrawOverlay(ctx widget.DrawContext) {
	in := p.r.Inset(16)
	if sd, ok := ctx.(hgdraw.ShadowDrawer); ok {
		sd.DrawSoftShadow(in, 8, 12, color.RGBA{A: 120})
	}
	if rc, ok := ctx.(hgdraw.RoundClipper); ok {
		rc.SetRoundClip(in, 8)
	}
	if md, ok := ctx.(hgdraw.MicaDrawer); ok {
		md.MicaBehind(in, 0, color.RGBA{R: 30, G: 30, B: 40, A: 90})
	}
	ctx.FillRectAlpha(in.Min.X+4, in.Min.Y+4, in.Dx()/2, 20, color.RGBA{R: 200, G: 40, B: 40, A: 160})
	ctx.DrawText("Пункт меню", in.Min.X+8, in.Min.Y+30+p.hov, color.RGBA{R: 240, G: 240, B: 240, A: 255})
	if bd, ok := ctx.(hgdraw.BackdropDrawer); ok {
		bd.BlurBehind(image.Rect(in.Min.X, in.Min.Y, in.Max.X, in.Min.Y+40), 12, color.RGBA{R: 255, G: 255, B: 255, A: 40})
	}
	if p.img != nil {
		ctx.DrawImageScaled(p.img, in.Min.X+8, in.Min.Y+50, in.Dx()/3, in.Dy()/4)
	}
	if od, ok := ctx.(hgdraw.OpacityDrawer); ok {
		od.DrawWithOpacity(in, 0.6, func() {
			ctx.FillRoundRect(in.Max.X-40, in.Max.Y-30, 30, 20, 6, color.RGBA{G: 200, B: 90, A: 255})
		})
	}
	// Клипы намеренно не снимаются (кроме clean).
	if p.clean {
		if rc, ok := ctx.(hgdraw.RoundClipper); ok {
			rc.ClearRoundClip()
		}
		ctx.ClearClip()
	}
}

type revScene struct {
	e      *Engine
	ps     []*revPopup
	mu     sync.Mutex
	frames []PopupFrame
}

func revGradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Pix[img.PixOffset(x, y)+0] = uint8(x * 255 / w)
			img.Pix[img.PixOffset(x, y)+1] = uint8(y * 255 / h)
			img.Pix[img.PixOffset(x, y)+2] = uint8((x ^ y) & 0xff)
			img.Pix[img.PixOffset(x, y)+3] = 255
		}
	}
	return img
}

func newRevScene(t *testing.T, scale float64, rects ...image.Rectangle) *revScene {
	t.Helper()
	e := New(800, 600, 20)
	t.Cleanup(e.Stop)
	if scale != 1 {
		e.SetScale(scale)
	}
	e.SetTooltipsEnabled(false)
	if err := e.SetWallpaperSource(revGradient(800, 600)); err != nil {
		t.Fatal(err)
	}
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.SetBounds(image.Rect(0, 0, 800, 600))
	s := &revScene{e: e}
	pic := revGradient(64, 48)
	for _, r := range rects {
		p := &revPopup{Panel: widget.NewPanel(color.RGBA{}), r: r, img: pic}
		p.SetBounds(image.Rect(r.Min.X, r.Min.Y, r.Min.X+10, r.Min.Y+10))
		root.AddChild(p)
		s.ps = append(s.ps, p)
	}
	e.SetRoot(root)
	e.SetPopupSink(s.sink)
	return s
}

func (s *revScene) sink(fr []PopupFrame) {
	s.mu.Lock()
	s.frames = append(s.frames[:0], fr...)
	s.mu.Unlock()
}

func (s *revScene) frameOf(t *testing.T, p *revPopup) PopupFrame {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, fr := range s.frames {
		if fr.ID == widgetID(p) {
			return fr
		}
	}
	t.Fatalf("нет кадра попапа %v", p.r)
	return PopupFrame{}
}

func (s *revScene) check(t *testing.T, name string) {
	t.Helper()
	for i, p := range s.ps {
		if !p.open {
			continue
		}
		fr := s.frameOf(t, p)
		want := renderOverlay(s.e.canvas, p, fr.Rect)
		if fr.Img.Rect != want.Rect {
			t.Fatalf("%s: попап %d размером %v, свежий клон %v", name, i, fr.Img.Rect, want.Rect)
		}
		if !bytes.Equal(fr.Img.Pix, want.Pix) {
			n := 0
			for j := range want.Pix {
				if want.Pix[j] != fr.Img.Pix[j] {
					n++
				}
			}
			t.Errorf("%s: попап %d расходится со свежим клоном (%d байт)", name, i, n)
		}
	}
}

var (
	revBig   = image.Rect(40, 40, 560, 500)
	revSmall = image.Rect(600, 60, 760, 200)
	revMid   = image.Rect(580, 260, 780, 520)
)

// Линза 1: большой попап с тенью/Mica/размытием, затем маленький и средний —
// тождество со свежим клоном; повторный рендер; смена разрешения.
func TestPopupArenaReview_HeavyIdentity(t *testing.T) {
	testreset.Globals(t)
	for _, k := range []float64{1, 1.25} {
		t.Run(scaleName(k), func(t *testing.T) {
			s := newRevScene(t, k, revBig, revSmall, revMid)
			for _, p := range s.ps {
				p.open = true
			}
			s.e.renderFrame()
			s.check(t, "три попапа")
			s.ps[0].hov = 3
			s.e.Invalidate()
			s.e.renderFrame()
			s.check(t, "повтор")
			// Большой закрыт, маленький перерисован в заготовку от большого.
			s.ps[0].open = false
			s.ps[1].hov = 5
			s.e.Invalidate()
			s.e.renderFrame()
			s.check(t, "после большого")
		})
	}
}

// Линза 1: смена порядка каналов холста движка при открытом попапе — новый
// попап рендерится в заготовку прежнего формата.
func TestPopupArenaReview_PixelFormatChange(t *testing.T) {
	testreset.Globals(t)
	s := newRevScene(t, 1, revSmall, revMid)
	s.ps[0].open = true
	s.e.renderFrame()
	s.check(t, "до смены формата")
	if err := s.e.SetPixelFormat(FormatBGRX); err != nil {
		t.Fatal(err)
	}
	s.ps[1].open = true // открыт ПОСЛЕ смены формата
	s.e.renderFrame()
	if f := s.e.popupArena.c.Format(); f != s.e.canvas.Format() {
		t.Errorf("формат заготовки %v, холста движка %v", f, s.e.canvas.Format())
	}
	s.check(t, "после смены формата")
}

// Линза 4: память — заготовка держит размер самого большого попапа из
// когда-либо открытых, пока открыт хоть один.
func TestPopupArenaReview_ShrinkAfterBigCloses(t *testing.T) {
	testreset.Globals(t)
	s := newRevScene(t, 1, revBig, revSmall)
	for _, p := range s.ps {
		p.open = true
	}
	s.e.renderFrame()
	s.ps[0].open = false
	s.ps[1].hov = 2
	s.e.Invalidate()
	s.e.renderFrame()
	small := revSmall.Dx() * revSmall.Dy() * 4
	got := s.e.Stats().PopupBytes
	// Заготовка ужата: отпущена после закрытия большого (маленький со своей
	// готовой картинкой её не требует) или заведена под маленький — не больше
	// заготовки и картинки маленького.
	if want := uint64(2 * small); got > want {
		t.Errorf("открыт только маленький: PopupBytes = %d, ждали не больше заготовки+картинки %d (большой %d)",
			got, want, revBig.Dx()*revBig.Dy()*4)
	}
}

// Линза 4: исчезающий попап (переход) — заготовка не нужна, а держится.
func TestPopupArenaReview_SinkResetBeforeFrame(t *testing.T) {
	testreset.Globals(t)
	s := newRevScene(t, 1, revSmall)
	s.ps[0].open = true
	s.e.renderFrame()
	s.e.SetPopupSink(nil)
	s.e.SetPopupSink(s.sink)
	s.e.Invalidate()
	s.e.renderFrame()
	s.check(t, "после переподключения хоста")
	small := revSmall.Dx() * revSmall.Dy() * 4
	if got := s.e.Stats().PopupBytes; got != uint64(2*small) {
		t.Errorf("PopupBytes = %d, ждали %d", got, 2*small)
	}
}

// Линза 4: смена разрешения при открытом попапе — заготовка пересоздаётся
// под новый холст и не держит прежний.
func TestPopupArenaReview_ResolutionChange(t *testing.T) {
	testreset.Globals(t)
	s := newRevScene(t, 1, revSmall)
	s.ps[0].open = true
	s.e.renderFrame()
	old := s.e.canvas
	s.e.SetResolution(900, 700)
	s.e.renderFrame()
	if s.e.popupArena.src == old {
		t.Error("заготовка держит прежний холст движка")
	}
	s.check(t, "после SetResolution")
}

// Линза 5: аллокации перерисовки попапа в заготовку.
func TestPopupArenaReview_AllocsPerRerender(t *testing.T) {
	testreset.Globals(t)
	s := newRevScene(t, 1, revSmall, revMid)
	for _, p := range s.ps {
		p.open = true
	}
	s.e.renderFrame()
	src := s.e.canvas
	a := &s.e.popupArena
	n := testing.AllocsPerRun(50, func() {
		a.canvasFor(src, revSmall)
		a.canvasFor(src, revMid)
	})
	t.Logf("аллокаций на два перевода заготовки: %.1f", n)
}

// Линза 4: исчезающий попап — PopupBytes во время перехода и после конца.
func TestPopupArenaReview_FadeBytes(t *testing.T) {
	testreset.Globals(t)
	defer widget.StopAllAnimations()
	s, rec := newPopupTransScene(t, true, 10)
	s.setOpen(true)
	gen := s.eng.lp.Gen()
	s.eng.RenderFrameNow()
	t0 := time.Now()
	for ms := 0; ms <= 120; ms += 20 {
		widget.StepAnimationsFor(s.eng, t0.Add(time.Duration(ms)*time.Millisecond))
		renderLikeLoop(s.eng, &gen)
	}
	open := s.eng.Stats().PopupBytes
	s.setOpen(false)
	s.eng.RenderFrameNow()
	gen = s.eng.lp.Gen()
	t.Logf("открыт %d, в начале исчезновения %d, заготовка %v", open, s.eng.Stats().PopupBytes, s.eng.popupArena.c != nil)
	t1 := time.Now()
	for ms := 0; ms <= 120; ms += 20 {
		widget.StepAnimationsFor(s.eng, t1.Add(time.Duration(ms)*time.Millisecond))
		renderLikeLoop(s.eng, &gen)
	}
	if f := rec.last(); len(f) != 0 {
		t.Fatalf("после исчезновения в кадрах %+v", f)
	}
	if got := s.eng.Stats().PopupBytes; got != 0 || s.eng.popupArena.c != nil {
		t.Errorf("после исчезновения PopupBytes = %d, заготовка %v", got, s.eng.popupArena.c != nil)
	}
}

// Линза 3: живой цикл кадра, сток снимают и ставят из другой горутины.
func TestPopupArenaReview_LiveRace(t *testing.T) {
	testreset.Globals(t)
	s := newRevScene(t, 1, revSmall, revMid)
	for _, p := range s.ps {
		p.open = true
	}
	s.e.Start()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			if i%2 == 0 {
				s.e.SetPopupSink(nil)
			} else {
				s.e.SetPopupSink(s.sink)
			}
			_ = s.e.Stats().PopupBytes
			s.e.Invalidate()
			time.Sleep(time.Millisecond)
		}
	}()
	<-done
}

// Линза 5: перерисовка трёх открытых попапов каждый кадр (полный кадр).
func BenchmarkPopupArenaReview_RerenderAll(b *testing.B) {
	e := New(800, 600, 20)
	defer e.Stop()
	e.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.SetBounds(image.Rect(0, 0, 800, 600))
	for i, r := range []image.Rectangle{revSmall, revMid, image.Rect(40, 40, 300, 300)} {
		p := &revPopup{Panel: widget.NewPanel(color.RGBA{}), r: r, open: true, clean: true}
		p.SetBounds(image.Rect(r.Min.X, r.Min.Y, r.Min.X+10+i, r.Min.Y+10))
		root.AddChild(p)
	}
	e.SetRoot(root)
	e.SetPopupSink(func([]PopupFrame) {})
	e.renderFrame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.frameMu.Lock()
		e.renderPopups(e.canvas, root, nil, image.Rectangle{}, true)
		e.frameMu.Unlock()
	}
}

// Линза 1: смена качества текста (LCD, контраст, хинтинг) и подпикселя при
// открытом попапе — ревизия шрифтов пересоздаёт заготовку.
func TestPopupArenaReview_TextModes(t *testing.T) {
	testreset.Globals(t)
	s := newRevScene(t, 1, revSmall, revMid)
	s.ps[0].open = true
	s.e.renderFrame()
	q := s.e.TextQuality()
	q.Hinting, q.Contrast, q.LCD = true, 0.5, true
	s.e.SetTextQuality(q)
	s.e.SetTextSubpixel(true)
	s.ps[1].open = true
	s.e.renderFrame()
	s.check(t, "LCD+подпиксель")
}
