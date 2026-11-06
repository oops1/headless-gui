package engine

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math/rand"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// Объявленный перенос не перерисовывает окно программы (movekeep.go; пилот
// сцены WinLine, ENGINE_ISSUES «A declared move is applied to front only: the
// moved window is redrawn whole every step»).
//
// Главная проверка — тождество: два движка делают одно и то же, у одного
// пропуск включён, у другого выключен; после каждого кадра их кадры равны
// побайтно. Плюс счётчик: пропуск действительно случился там, где должен, и
// не случился там, где доказать нечего.

// mkScene — стол с окном программы: полосатый фон, поверхность и то, что тест
// положит поверх.
type mkScene struct {
	e    *Engine
	root *widget.Panel
	surf *widget.ExternalSurface
	buf  *image.RGBA
	at   image.Rectangle
}

// mkPicture заполняет буфер картинкой, в которой каждый пиксель свой:
// сдвинутая на пиксель картинка не совпадает с исходной нигде.
func mkPicture(buf *image.RGBA, seed int) {
	for y := buf.Rect.Min.Y; y < buf.Rect.Max.Y; y++ {
		for x := buf.Rect.Min.X; x < buf.Rect.Max.X; x++ {
			o := buf.PixOffset(x, y)
			buf.Pix[o] = byte(x*7 + y*3 + seed)
			buf.Pix[o+1] = byte(x*5 ^ y*11 + seed)
			buf.Pix[o+2] = byte(x + y*13)
			buf.Pix[o+3] = 0xff
		}
	}
}

func newMKScene(t *testing.T, scale float64, reuse bool) *mkScene {
	t.Helper()
	e := New(900, 640, 30)
	t.Cleanup(e.Stop)
	e.SetTooltipsEnabled(false)
	e.SetRenderOnDemand(true)
	if scale != 1 {
		e.SetScale(scale)
	}
	e.SetMoveReuse(reuse)
	root := widget.NewPanel(color.RGBA{R: 20, G: 60, B: 110, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 900, 640))
	for i := 0; i < 15; i++ {
		p := widget.NewPanel(color.RGBA{R: uint8(17 * i), G: uint8(240 - 15*i), B: 90, A: 255})
		p.ShowHeader = false
		p.SetBounds(image.Rect(i*60, 0, i*60+30, 640))
		root.AddChild(p)
	}
	s := &mkScene{e: e, root: root, surf: widget.NewExternalSurface()}
	s.at = image.Rect(60, 50, 60+520, 50+400)
	pw, ph := int(float64(s.at.Dx())*scale+0.5), int(float64(s.at.Dy())*scale+0.5)
	s.buf = image.NewRGBA(image.Rect(0, 0, pw, ph))
	mkPicture(s.buf, 1)
	s.surf.SetBuffer(s.buf, true)
	s.surf.SetBounds(s.at)
	root.AddChild(s.surf)
	e.SetRoot(root)
	return s
}

// slide двигает окно на d и объявляет перенос, как это делает оболочка.
func (s *mkScene) slide(d image.Point) {
	old := s.at
	s.at = s.at.Add(d)
	widget.NotifyWidgetMove(s.surf, old, s.at)
	s.surf.SetBounds(s.at)
}

func (s *mkScene) frame() []byte {
	fr := s.e.renderFrame()
	fr.Release()
	return s.e.canvas.Front().Pix
}

func (s *mkScene) moved() uint64 { return s.e.Stats().TilesMoved }

// pair — две одинаковые сцены: с пропуском и без.
type pair struct {
	on, off *mkScene
}

func newPair(t *testing.T, scale float64, setup func(s *mkScene)) *pair {
	t.Helper()
	p := &pair{on: newMKScene(t, scale, true), off: newMKScene(t, scale, false)}
	p.each(func(s *mkScene) {
		if setup != nil {
			setup(s)
		}
		s.frame()
	})
	return p
}

func (p *pair) each(fn func(s *mkScene)) { fn(p.on); fn(p.off) }

// step выполняет действие в обеих сценах, рисует кадр и сверяет их; возвращает,
// сколько тайлов кадр с пропуском оставил переносу.
func (p *pair) step(t *testing.T, what string, fn func(s *mkScene)) uint64 {
	t.Helper()
	before := p.on.moved()
	p.each(fn)
	a, b := p.on.frame(), p.off.frame()
	if !bytes.Equal(a, b) {
		t.Fatalf("%s: кадр с пропуском после переноса отличается от кадра без пропуска", what)
	}
	if n := p.off.moved(); n != 0 {
		t.Fatalf("%s: выключенный пропуск оставил переносу %d тайлов", what, n)
	}
	return p.on.moved() - before
}

// Окно тащат: тайлы внутри него не рисуются, а кадр тот же.
func TestMoveKeep_DragSkipsWindowTiles(t *testing.T) {
	testreset.Globals(t)
	for _, scale := range []float64{1, 1.5, 2} {
		for _, tiled := range []bool{true, false} {
			p := newPair(t, scale, func(s *mkScene) { s.e.SetTiledRaster(tiled) })
			total := uint64(0)
			for i, d := range []image.Point{{7, 3}, {13, -5}, {-9, 11}, {64, 0}, {1, 1}, {0, 37}} {
				d := d
				total += p.step(t, fmt.Sprintf("scale=%v tiled=%v шаг %d", scale, tiled, i),
					func(s *mkScene) { s.slide(d) })
			}
			if total == 0 {
				t.Errorf("scale=%v tiled=%v: за шесть шагов перетаскивания ни один тайл не оставлен переносу", scale, tiled)
			}
		}
	}
}

// Большая часть окна на каждом шаге остаётся переносу: рисуется кайма.
func TestMoveKeep_MostOfWindowIsKept(t *testing.T) {
	testreset.Globals(t)
	p := newPair(t, 1, nil)
	p.step(t, "первый шаг", func(s *mkScene) { s.slide(image.Pt(5, 4)) })
	kept := p.step(t, "второй шаг", func(s *mkScene) { s.slide(image.Pt(6, 2)) })
	// Окно 520×400: целиком внутри него и назначения — не меньше 6×4 тайлов.
	if kept < 24 {
		t.Errorf("перенос оставил %d тайлов, ждали не меньше 24", kept)
	}
}

// Содержимое окна изменилось вместе с переносом: доказывать нечего, окно
// рисуется, кадр верен.
func TestMoveKeep_ChangedContentIsRedrawn(t *testing.T) {
	testreset.Globals(t)
	p := newPair(t, 1, nil)
	p.step(t, "разгон", func(s *mkScene) { s.slide(image.Pt(5, 5)) })
	kept := p.step(t, "перенос с изменением", func(s *mkScene) {
		s.surf.Update(func(b *image.RGBA) []image.Rectangle {
			mkPicture(b, 99)
			return []image.Rectangle{b.Rect}
		})
		s.slide(image.Pt(8, 3))
	})
	if kept != 0 {
		t.Errorf("содержимое менялось, а перенос оставил %d тайлов", kept)
	}
	// Следующий чистый шаг — снова с пропуском.
	if p.step(t, "чистый шаг после изменения", func(s *mkScene) { s.slide(image.Pt(4, 4)) }) == 0 {
		t.Error("после перерисовки пропуск не вернулся")
	}
}

// Изменился кусок окна, заявленный до переноса и уже нарисованный: перенос
// несёт новое содержимое.
func TestMoveKeep_ContentChangedBetweenMovesIsCarried(t *testing.T) {
	testreset.Globals(t)
	p := newPair(t, 1, nil)
	p.step(t, "разгон", func(s *mkScene) { s.slide(image.Pt(5, 5)) })
	p.step(t, "изменение на месте", func(s *mkScene) {
		s.surf.Update(func(b *image.RGBA) []image.Rectangle {
			r := image.Rect(100, 80, 260, 200)
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					o := b.PixOffset(x, y)
					b.Pix[o], b.Pix[o+1], b.Pix[o+2] = 0xff, 0x00, 0x80
				}
			}
			return []image.Rectangle{r}
		})
	})
	if p.step(t, "перенос после изменения", func(s *mkScene) { s.slide(image.Pt(9, -2)) }) == 0 {
		t.Error("нарисованное изменение не дало переносу оставить тайлы")
	}
}

// Над окном лежит панель: тайлы под ней рисуются, остальные остаются.
func TestMoveKeep_CoveredTilesAreRedrawn(t *testing.T) {
	testreset.Globals(t)
	var bars [2]*widget.Panel
	i := 0
	p := newPair(t, 1, func(s *mkScene) {
		bar := widget.NewPanel(color.RGBA{R: 240, G: 240, B: 240, A: 255})
		bar.ShowHeader = false
		bar.SetBounds(image.Rect(0, 300, 900, 360)) // полоса поперёк окна
		s.root.AddChild(bar)
		bars[i] = bar
		i++
	})
	p.step(t, "разгон", func(s *mkScene) { s.slide(image.Pt(6, 6)) })
	kept := p.step(t, "перенос под панелью", func(s *mkScene) { s.slide(image.Pt(10, 4)) })
	if kept == 0 {
		t.Error("панель над частью окна отменила пропуск целиком")
	}
	// Панель уходит — открывшееся рисуется.
	p.step(t, "панель убрана", func(s *mkScene) {
		bar := bars[0]
		if s == p.off {
			bar = bars[1]
		}
		s.root.RemoveChild(bar)
		s.slide(image.Pt(3, 3))
	})
}

// Перенос объявлен, а окно не двигалось (или двигалось иначе): пропуска нет,
// кадр верен — движок не верит объявлению. Повреждение здесь владельческое
// (InvalidateArea): ничейное рисует окно и без этой проверки.
func TestMoveKeep_LyingMoveIsNotTrusted(t *testing.T) {
	testreset.Globals(t)
	p := newPair(t, 1, nil)
	p.step(t, "разгон", func(s *mkScene) { s.slide(image.Pt(5, 5)) })
	kept := p.step(t, "ложный перенос", func(s *mkScene) {
		to := s.at.Add(image.Pt(20, 10))
		widget.NotifyMove(s.at, to)
		s.surf.InvalidateArea(s.at.Union(to))
	})
	if kept != 0 {
		t.Errorf("окно не двигалось, а перенос оставил %d тайлов", kept)
	}
	p.step(t, "выравнивание", func(s *mkScene) { s.e.Invalidate() })
	p.step(t, "разгон", func(s *mkScene) { s.slide(image.Pt(5, 5)) })
	kept = p.step(t, "перенос не на тот сдвиг", func(s *mkScene) {
		old := s.at
		s.at = s.at.Add(image.Pt(12, 0))
		lie := old.Add(image.Pt(30, 0))
		widget.NotifyWidgetMove(s.surf, old, lie)
		s.surf.SetBounds(s.at)
		s.surf.InvalidateArea(old.Union(lie))
	})
	if kept != 0 {
		t.Errorf("сдвиг переноса не тот, что у окна, а оставлено %d тайлов", kept)
	}
}

// Перенос шире окна — с ореолом тени, как у оболочки: тайлы на кромке окна
// лежат в назначении переноса, и последним в них рисует окно, но целиком окно
// их не закрывает — они рисуются.
func TestMoveKeep_HaloAroundWindowIsRedrawn(t *testing.T) {
	testreset.Globals(t)
	const halo = 40
	p := newPair(t, 1, nil)
	slide := func(s *mkScene, d image.Point) {
		old := s.at
		s.at = s.at.Add(d)
		widget.NotifyWidgetMove(s.surf, old.Inset(-halo), s.at.Inset(-halo))
		s.surf.SetBounds(s.at)
		s.surf.InvalidateArea(old.Union(s.at).Inset(-halo))
	}
	p.step(t, "разгон", func(s *mkScene) { slide(s, image.Pt(5, 5)) })
	total := uint64(0)
	for i, d := range []image.Point{{9, 4}, {-6, 12}, {17, -3}} {
		d := d
		total += p.step(t, fmt.Sprintf("шаг %d", i), func(s *mkScene) { slide(s, d) })
	}
	if total == 0 {
		t.Error("перенос с ореолом не оставил ни тайла")
	}
}

// roundFrame — рамка окна программы, как её строит оболочка: поверхность
// рисуется под скруглённым клипом.
type roundFrame struct {
	widget.Base
	corner int
}

func (f *roundFrame) Draw(ctx widget.DrawContext) {
	rc, ok := ctx.(widget.RoundClipper)
	prev := ctx.Clip()
	if ok {
		rc.SetRoundClip(f.Bounds(), f.corner)
	}
	f.DrawChildren(ctx)
	if ok {
		rc.ClearRoundClip()
		ctx.SetClip(prev)
	}
}

// Окно под скруглённым клипом рамки: угловые тайлы рисуются (под срезанным
// углом виден стол), остальные остаются переносу.
func TestMoveKeep_RoundClippedWindow(t *testing.T) {
	testreset.Globals(t)
	for _, scale := range []float64{1, 1.5} {
		var frames [2]*roundFrame
		i := 0
		p := newPair(t, scale, func(s *mkScene) {
			f := &roundFrame{corner: 24}
			f.SetBounds(s.at)
			s.root.RemoveChild(s.surf)
			f.AddChild(s.surf)
			s.root.AddChild(f)
			frames[i] = f
			i++
		})
		slide := func(s *mkScene, d image.Point) {
			f := frames[0]
			if s == p.off {
				f = frames[1]
			}
			old := s.at
			s.at = s.at.Add(d)
			widget.NotifyWidgetMove(f, old, s.at)
			f.SetBounds(s.at)
			s.surf.SetBounds(s.at)
		}
		p.step(t, "разгон", func(s *mkScene) { slide(s, image.Pt(5, 5)) })
		total := uint64(0)
		for k, d := range []image.Point{{9, 4}, {-6, 12}, {17, -3}, {64, 64}} {
			d := d
			total += p.step(t, fmt.Sprintf("scale=%v шаг %d", scale, k), func(s *mkScene) { slide(s, d) })
		}
		if total == 0 {
			t.Errorf("scale=%v: окно под скруглённым клипом не оставило переносу ни тайла", scale)
		}
	}
}

// Ничейное повреждение над окном (widget.InvalidateRect) рисует его: под
// областью могло измениться то, о чём команды не знают.
func TestMoveKeep_OrphanDamageRedraws(t *testing.T) {
	testreset.Globals(t)
	p := newPair(t, 1, nil)
	p.step(t, "разгон", func(s *mkScene) { s.slide(image.Pt(5, 5)) })
	kept := p.step(t, "перенос с ничейным повреждением", func(s *mkScene) {
		s.slide(image.Pt(7, 7))
		widget.InvalidateRect(s.at)
	})
	if kept != 0 {
		t.Errorf("ничейное повреждение над окном, а перенос оставил %d тайлов", kept)
	}
}

// Буфер с альфой и объявленной непрозрачной областью: тело остаётся
// переносу, кайма с углами рисуется.
func TestMoveKeep_OpaqueRegionOfAlphaBuffer(t *testing.T) {
	testreset.Globals(t)
	p := newPair(t, 1, func(s *mkScene) {
		for y := 0; y < s.buf.Rect.Dy(); y++ { // прозрачная кайма в 12 пикселей
			for x := 0; x < s.buf.Rect.Dx(); x++ {
				if x < 12 || y < 12 || x >= s.buf.Rect.Dx()-12 || y >= s.buf.Rect.Dy()-12 {
					o := s.buf.PixOffset(x, y)
					copy(s.buf.Pix[o:o+4], []byte{0, 0, 0, 0})
				}
			}
		}
		s.surf.SetBuffer(s.buf, false)
		s.surf.SetOpaqueRegion([]image.Rectangle{s.buf.Rect.Inset(12)})
	})
	p.step(t, "разгон", func(s *mkScene) { s.slide(image.Pt(5, 5)) })
	if p.step(t, "перенос", func(s *mkScene) { s.slide(image.Pt(9, 6)) }) == 0 {
		t.Error("тело окна с объявленной непрозрачной областью не оставлено переносу")
	}
	// Без области буфер с альфой смешивается с тем, что под ним, — оставлять
	// нечего.
	p.step(t, "область снята", func(s *mkScene) { s.surf.SetOpaqueRegion(nil) })
	if kept := p.step(t, "перенос без области", func(s *mkScene) { s.slide(image.Pt(9, 6)) }); kept != 0 {
		t.Errorf("буфер с альфой без непрозрачной области, а оставлено %d тайлов", kept)
	}
}

// Окно частью за краем экрана: перенос обрезан, кадр верен.
func TestMoveKeep_WindowLeavingScreen(t *testing.T) {
	testreset.Globals(t)
	p := newPair(t, 1, nil)
	for i := 0; i < 8; i++ {
		p.step(t, fmt.Sprintf("к краю, шаг %d", i), func(s *mkScene) { s.slide(image.Pt(70, 45)) })
	}
	for i := 0; i < 8; i++ {
		p.step(t, fmt.Sprintf("обратно, шаг %d", i), func(s *mkScene) { s.slide(image.Pt(-70, -45)) })
	}
}

// Чужие буферы вывода (SetSurfaces): буфер потребителя после кадра с
// пропуском — тот же кадр целиком.
func TestMoveKeep_ConsumerBuffersAreWhole(t *testing.T) {
	testreset.Globals(t)
	const w, h = 900, 640
	bufs := [2][][]byte{}
	i := 0
	p := newPair(t, 1, func(s *mkScene) {
		bufs[i] = [][]byte{make([]byte, w*h*4), make([]byte, w*h*4)}
		if err := s.e.SetSurfaces(bufs[i], w*4, FormatRGBA); err != nil {
			t.Fatal(err)
		}
		i++
	})
	for step := 0; step < 6; step++ {
		var got [2][]byte
		k := 0
		before := p.on.moved()
		p.each(func(s *mkScene) {
			s.slide(image.Pt(9, 5))
			fr := s.e.renderFrame()
			if fr.Surface == 0 {
				t.Fatalf("шаг %d: кадр не в буфере потребителя", step)
			}
			got[k] = append([]byte(nil), bufs[k][fr.Surface-1]...)
			if !bytes.Equal(got[k], s.e.canvas.Front().Pix) {
				t.Fatalf("шаг %d: буфер потребителя не равен кадру движка", step)
			}
			s.e.ReleaseSurface(fr.Surface)
			fr.Release()
			k++
		})
		if !bytes.Equal(got[0], got[1]) {
			t.Fatalf("шаг %d: буфер потребителя с пропуском отличается от буфера без пропуска", step)
		}
		if step > 0 && p.on.moved() == before {
			t.Errorf("шаг %d: при чужих буферах перенос не оставил ни тайла", step)
		}
	}
}

// Два буфера кадра (SetSingleBuffer(false)): то же тождество.
func TestMoveKeep_TwoBuffers(t *testing.T) {
	testreset.Globals(t)
	p := newPair(t, 1, func(s *mkScene) { s.e.SetSingleBuffer(false) })
	total := uint64(0)
	for i := 0; i < 5; i++ {
		total += p.step(t, fmt.Sprintf("шаг %d", i), func(s *mkScene) { s.slide(image.Pt(11, 6)) })
	}
	if total == 0 {
		t.Error("при двух буферах перенос не оставил ни тайла")
	}
}

// Случайная работа со сценой: переносы вперемежку с изменениями содержимого,
// панелью над окном, ложными объявлениями и полными кадрами. Кадры двух
// движков равны всегда.
func TestMoveKeep_RandomSessionMatches(t *testing.T) {
	testreset.Globals(t)
	for _, scale := range []float64{1, 1.25} {
		rng := rand.New(rand.NewSource(int64(scale * 1000)))
		var bars [2]*widget.Panel
		i := 0
		p := newPair(t, scale, func(s *mkScene) {
			bar := widget.NewPanel(color.RGBA{R: 250, G: 250, B: 200, A: 255})
			bar.ShowHeader = false
			bar.SetBounds(image.Rect(200, 500, 700, 560))
			s.root.AddChild(bar)
			bars[i] = bar
			i++
		})
		barOf := func(s *mkScene) *widget.Panel {
			if s == p.on {
				return bars[0]
			}
			return bars[1]
		}
		kept := uint64(0)
		for step := 0; step < 120; step++ {
			op := rng.Intn(10)
			d := image.Pt(rng.Intn(41)-20, rng.Intn(41)-20)
			seed := rng.Intn(200)
			patch := image.Rect(rng.Intn(200), rng.Intn(200), 200+rng.Intn(200), 200+rng.Intn(150))
			kept += p.step(t, fmt.Sprintf("scale=%v шаг %d (действие %d)", scale, step, op), func(s *mkScene) {
				switch op {
				case 0, 1, 2, 3, 4: // чистый перенос
					s.slide(d)
				case 5: // перенос с изменением куска
					s.surf.Update(func(b *image.RGBA) []image.Rectangle {
						r := patch.Intersect(b.Rect)
						for y := r.Min.Y; y < r.Max.Y; y++ {
							for x := r.Min.X; x < r.Max.X; x++ {
								o := b.PixOffset(x, y)
								b.Pix[o] = byte(seed + x)
							}
						}
						return []image.Rectangle{r}
					})
					s.slide(d)
				case 6: // панель над окном переезжает
					b := barOf(s)
					b.SetBounds(b.Bounds().Add(d))
				case 7: // ложный перенос
					widget.NotifyMove(s.at, s.at.Add(d))
					widget.InvalidateRect(s.at.Union(s.at.Add(d)))
				case 8: // всё заново
					s.e.Invalidate()
				case 9: // перенос и ничейное повреждение рядом
					s.slide(d)
					widget.InvalidateRect(patch)
				}
			})
		}
		if kept == 0 {
			t.Errorf("scale=%v: за сессию перенос не оставил ни тайла — проверка пуста", scale)
		}
	}
}

// clipFrame — рамка, обрезающая ребёнка своими границами (и, если задано,
// скруглением): окно программы, прокручиваемое внутри неё.
type clipFrame struct {
	widget.Base
	corner int
}

func (f *clipFrame) Draw(ctx widget.DrawContext) {
	prev := ctx.Clip()
	rc, round := ctx.(widget.RoundClipper)
	round = round && f.corner > 0
	if round {
		rc.SetRoundClip(f.Bounds(), f.corner)
	} else {
		ctx.SetClip(f.Bounds().Intersect(prev))
	}
	f.DrawChildren(ctx)
	if round {
		rc.ClearRoundClip()
	}
	ctx.SetClip(prev)
}

// inFrame сажает окно сцены в неподвижную рамку at и растягивает его буфер на
// extra точек вниз: содержимое, которое едет внутри рамки.
func inFrame(s *mkScene, at image.Rectangle, corner, extra int) *clipFrame {
	f := &clipFrame{corner: corner}
	f.SetBounds(at)
	s.root.RemoveChild(s.surf)
	s.at = image.Rect(at.Min.X, at.Min.Y, at.Max.X, at.Max.Y+extra)
	s.buf = image.NewRGBA(image.Rect(0, 0, s.at.Dx(), s.at.Dy()))
	mkPicture(s.buf, 3)
	s.surf.SetBuffer(s.buf, true)
	s.surf.SetBounds(s.at)
	f.AddChild(s.surf)
	s.root.AddChild(f)
	return f
}

// Скруглённый клип выровнен по сетке тайлов: угловые тайлы целиком лежат в
// окне и в назначении переноса, но под срезанным углом виден стол — они
// рисуются.
func TestMoveKeep_RoundCornersOnTileGrid(t *testing.T) {
	testreset.Globals(t)
	var frames [2]*roundFrame
	i := 0
	p := newPair(t, 1, func(s *mkScene) {
		s.at = image.Rect(64, 64, 64+512, 64+384)
		s.buf = image.NewRGBA(image.Rect(0, 0, 512, 384))
		mkPicture(s.buf, 5)
		s.surf.SetBuffer(s.buf, true)
		s.surf.SetBounds(s.at)
		f := &roundFrame{corner: 24}
		f.SetBounds(s.at)
		s.root.RemoveChild(s.surf)
		f.AddChild(s.surf)
		s.root.AddChild(f)
		frames[i] = f
		i++
	})
	slide := func(s *mkScene, d image.Point) {
		f := frames[0]
		if s == p.off {
			f = frames[1]
		}
		old := s.at
		s.at = s.at.Add(d)
		widget.NotifyWidgetMove(f, old, s.at)
		f.SetBounds(s.at)
		s.surf.SetBounds(s.at)
	}
	total := uint64(0)
	for k, d := range []image.Point{{64, 0}, {0, 64}, {64, 64}, {-64, 0}} {
		d := d
		total += p.step(t, fmt.Sprintf("шаг %d", k), func(s *mkScene) { slide(s, d) })
	}
	if total == 0 {
		t.Error("окно на сетке тайлов не оставило переносу ни тайла")
	}
}

// Содержимое едет под неподвижным скруглённым клипом (прокрутка внутри рамки):
// тайл, приехавший из срезанного угла, несёт стол, а не окно, — рисуется;
// тайл, приехавший из середины в середину, остаётся переносу. Рамка стоит на
// сетке тайлов, сдвиги — на целый тайл и нет: углы задевают и источник, и
// назначение.
func TestMoveKeep_ContentSlidesUnderFixedRoundClip(t *testing.T) {
	testreset.Globals(t)
	for _, scale := range []float64{1, 1.5} {
		frame := image.Rect(64, 64, 64+512, 64+384)
		p := newPair(t, scale, func(s *mkScene) {
			inFrame(s, frame, 24, 150)
			if scale != 1 { // буфер inFrame — в логических точках; здесь нужен физический
				s.buf = image.NewRGBA(image.Rect(0, 0, int(float64(s.at.Dx())*scale+0.5), int(float64(s.at.Dy())*scale+0.5)))
				mkPicture(s.buf, 3)
				s.surf.SetBuffer(s.buf, true)
			}
		})
		total := uint64(0)
		for k, d := range []image.Point{{0, -64}, {0, 64}, {0, -64}, {0, -17}, {0, 30}, {0, 51}} {
			d := d
			total += p.step(t, fmt.Sprintf("scale=%v шаг %d", scale, k), func(s *mkScene) { s.slide(d) })
		}
		if total == 0 {
			t.Errorf("scale=%v: под неподвижным скруглённым клипом переносу не осталось ни тайла", scale)
		}
	}
}

// Под неподвижным скруглённым клипом на сетке тайлов: угловые тайлы целиком
// лежат в рамке, и после сдвига на тайл в них приезжает середина окна — но под
// срезанным углом виден стол, они рисуются. И обратно: из углового тайла в
// середину приезжает срезанный угол.
func TestMoveKeep_FixedRoundClipCornersAreRedrawn(t *testing.T) {
	testreset.Globals(t)
	frame := image.Rect(64, 64, 64+512, 64+384)
	p := newPair(t, 1, func(s *mkScene) {
		f := inFrame(s, frame, 40, 0)
		// Содержимое шире и выше рамки и едет во все стороны.
		s.at = frame.Inset(-128)
		s.buf = image.NewRGBA(image.Rect(0, 0, s.at.Dx(), s.at.Dy()))
		mkPicture(s.buf, 9)
		s.surf.SetBuffer(s.buf, true)
		s.surf.SetBounds(s.at)
		_ = f
	})
	// Прокрутка: переезжает то, что видно в рамке и остаётся в ней.
	scroll := func(s *mkScene, d image.Point) {
		dst := frame.Intersect(frame.Add(d))
		s.at = s.at.Add(d)
		widget.NotifyMove(dst.Sub(d), dst)
		s.surf.SetBounds(s.at)
	}
	total := uint64(0)
	for k, d := range []image.Point{{64, 0}, {0, 64}, {-64, -64}, {64, 64}, {-64, 0}, {0, -64}} {
		d := d
		total += p.step(t, fmt.Sprintf("шаг %d", k), func(s *mkScene) { scroll(s, d) })
	}
	if total == 0 {
		t.Error("содержимое под скруглённой рамкой не оставило переносу ни тайла")
	}
}

// Содержимое едет под прямоугольным клипом, край которого режет тайл: источник
// переноса частью лежал за клипом, там был стол — такие тайлы рисуются.
func TestMoveKeep_SourceOutsideClipIsRedrawn(t *testing.T) {
	testreset.Globals(t)
	frame := image.Rect(64, 64, 64+512, 430) // нижний край — посреди ряда тайлов
	p := newPair(t, 1, func(s *mkScene) { inFrame(s, frame, 0, 150) })
	total := uint64(0)
	for k, d := range []image.Point{{0, -30}, {0, -20}, {0, 45}, {0, -50}, {0, 40}} {
		d := d
		total += p.step(t, fmt.Sprintf("шаг %d", k), func(s *mkScene) { s.slide(d) })
	}
	if total == 0 {
		t.Error("содержимое под клипом не оставило переносу ни тайла")
	}
}

// Перенос объявлен над областью, где команды не менялись: её пиксели сменил
// перенос, и хэш-дифф обязан нарисовать их, а не пропустить как прежние.
func TestMoveKeep_DestinationOfMoveIsRedrawn(t *testing.T) {
	testreset.Globals(t)
	for _, reuse := range []bool{true, false} {
		s := newMKScene(t, 1, reuse)
		s.frame()
		want := append([]byte(nil), s.frame()...)

		// Кусок стола «переехал» на другой кусок стола; повреждение заявлено,
		// а дерево под ним прежнее.
		src, dst := image.Rect(600, 500, 760, 620), image.Rect(650, 470, 810, 590)
		widget.NotifyMove(src, dst)
		s.root.InvalidateArea(src.Union(dst))
		if got := s.frame(); !bytes.Equal(got, want) {
			t.Fatalf("reuse=%v: после переноса над неизменившимся местом кадр не тот, что был", reuse)
		}
		// И кадр за ним: тайлы назначения не остались помечены прежним хэшем.
		s.root.InvalidateArea(src.Union(dst))
		if got := s.frame(); !bytes.Equal(got, want) {
			t.Fatalf("reuse=%v: кадр после переноса расходится", reuse)
		}
	}
}

// Меню под присмотром переходов открыто над окном, пока окно тащат: слои
// после растра ложатся поверх тайлов, оставленных переносу. Сцены идут одна
// за другой, а не парой: открытое меню одно на процесс.
func TestMoveKeep_MenuWithTransitionOverWindow(t *testing.T) {
	testreset.Globals(t)
	// Переходы без длительности: два прогона рисуют кадры в разное время, и
	// шаг перехода по часам у них разошёлся бы. Присмотр за меню (слои после
	// растра) остаётся.
	run := func(reuse bool) (frames [][]byte, kept uint64) {
		testreset.Globals(t)
		widget.SetReduceMotion(true)
		defer widget.SetReduceMotion(false)
		s := newMKScene(t, 1, reuse)
		s.e.SetOverlayTransitions(true)
		m := widget.NewPopupMenu()
		m.AddItem("Открыть", func() {})
		m.AddItem("Сохранить", func() {})
		m.AddItem("Закрыть", func() {})
		s.root.AddChild(m)
		shot := func() { frames = append(frames, append([]byte(nil), s.frame()...)) }
		shot()
		s.slide(image.Pt(5, 5))
		shot()
		m.Show(260, 200)
		shot()
		for k := 0; k < 6; k++ {
			s.slide(image.Pt(14, 9))
			shot()
		}
		m.Close()
		s.slide(image.Pt(3, 3))
		shot()
		for k := 0; k < 4; k++ {
			s.slide(image.Pt(-11, 6))
			shot()
		}
		return frames, s.moved()
	}
	on, kept := run(true)
	off, none := run(false)
	if none != 0 {
		t.Fatalf("выключенный пропуск оставил переносу %d тайлов", none)
	}
	if kept == 0 {
		t.Error("с меню над окном перенос не оставил ни тайла — проверка пуста")
	}
	if len(on) != len(off) {
		t.Fatalf("кадров %d и %d", len(on), len(off))
	}
	for i := range on {
		if !bytes.Equal(on[i], off[i]) {
			t.Fatalf("кадр %d: с пропуском после переноса отличается от кадра без пропуска", i)
		}
	}
}

// Окно программы 1000×700 тащат по столу Full HD: кадр с пропуском после
// переноса и без него.
func benchSurfaceDrag(b *testing.B, reuse bool) {
	e := New(1920, 1080, 30)
	b.Cleanup(e.Stop)
	e.SetTooltipsEnabled(false)
	e.SetRenderOnDemand(true)
	e.SetMoveReuse(reuse)
	e.SetFrameTileData(false)
	root := widget.NewPanel(color.RGBA{R: 20, G: 60, B: 110, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 1920, 1080))
	surf := widget.NewExternalSurface()
	buf := image.NewRGBA(image.Rect(0, 0, 1000, 700))
	mkPicture(buf, 1)
	surf.SetBuffer(buf, true)
	at := image.Rect(100, 100, 1100, 800)
	surf.SetBounds(at)
	root.AddChild(surf)
	e.SetRoot(root)
	e.renderFrame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d := image.Pt(9, 5)
		if i/40%2 == 1 {
			d = image.Pt(-9, -5)
		}
		old := at
		at = at.Add(d)
		widget.NotifyWidgetMove(surf, old, at)
		surf.SetBounds(at)
		fr := e.renderFrame()
		fr.Release()
	}
}

func BenchmarkMoveKeep_SurfaceDrag(b *testing.B)        { benchSurfaceDrag(b, true) }
func BenchmarkMoveKeep_SurfaceDragRedrawn(b *testing.B) { benchSurfaceDrag(b, false) }

// Окно выросло и переехало разом, а буфер прежний: перенос объявлен на новый
// размер, но в источнике окно занимало только часть — рядом был стол. Тайл,
// приехавший с этой границы, несёт стол и рисуется.
func TestMoveKeep_SourceWiderThanWindowWasIsRedrawn(t *testing.T) {
	testreset.Globals(t)
	for _, d := range []image.Point{{64, 0}, {0, 64}, {37, 21}} {
		p := newPair(t, 1, func(s *mkScene) {
			s.surf.SetBounds(image.Rect(s.at.Min.X, s.at.Min.Y, s.at.Min.X+300, s.at.Min.Y+230))
		})
		p.step(t, fmt.Sprintf("сдвиг %v", d), func(s *mkScene) {
			old := s.at
			s.at = s.at.Add(d)
			widget.NotifyWidgetMove(s.surf, old, s.at)
			s.surf.SetBounds(s.at)
		})
	}
}
