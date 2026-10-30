package engine

import (
	"image"
	"image/color"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/output"
	"github.com/oops1/headless-gui/v4/widget"
)

// videoScene — окно программы 256×192 ровно по тайлам, и окно над ним.
type videoScene struct {
	eng     *Engine
	windows *widget.Panel
	surf    *widget.ExternalSurface
	buf     *image.RGBA
	frame   int
}

func newVideoScene(t *testing.T) *videoScene {
	t.Helper()
	eng := New(512, 384, 30)
	t.Cleanup(eng.Stop)
	root := widget.NewPanel(color.RGBA{R: 30, G: 60, B: 90, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 512, 384))
	windows := widget.NewPanel(color.RGBA{})
	windows.ShowHeader = false
	windows.SetBounds(image.Rect(0, 0, 512, 384))
	root.AddChild(windows)
	surf := widget.NewExternalSurface()
	surf.SetBounds(image.Rect(64, 64, 320, 256))
	buf := image.NewRGBA(image.Rect(0, 0, 256, 192))
	surf.SetBuffer(buf, true)
	windows.AddChild(surf)
	eng.SetRoot(root)
	eng.RenderOnce()
	return &videoScene{eng: eng, windows: windows, surf: surf, buf: buf}
}

// play обновляет n раз подряд: n обновлений за доли секунды — частота n в
// секунду и выше.
func (vs *videoScene) play(n int) {
	r := image.Rect(0, 0, 256, 192)
	for i := 0; i < n; i++ {
		vs.surf.Update(func(b *image.RGBA) []image.Rectangle {
			// Весь кадр новый, как у фильма: меняются все тайлы окна.
			v := byte(vs.frame)
			for j := range b.Pix {
				b.Pix[j] = v
			}
			vs.frame++
			return []image.Rectangle{r}
		})
	}
}

func videoOf(fr output.Frame, id uint32) (output.VideoRegion, bool) {
	for _, v := range fr.Videos {
		if v.ID == id {
			return v, true
		}
	}
	return output.VideoRegion{}, false
}

// 30 обновлений в секунду — видео: окно в Frame.Videos, его тайлы —
// RegionVideo с его номером.
func TestSceneVideo_HighRateIsVideo(t *testing.T) {
	testreset.Globals(t)
	vs := newVideoScene(t)
	vs.play(30)
	fr := vs.eng.RenderFrameNow()
	v, ok := videoOf(fr, vs.surf.SurfaceID())
	if !ok {
		t.Fatalf("окна нет в Frame.Videos: %+v", fr.Videos)
	}
	if v.Rect != image.Rect(64, 64, 320, 256) || len(v.Cover) != 0 {
		t.Fatalf("видео: rect %v cover %v", v.Rect, v.Cover)
	}
	n := 0
	for _, r := range fr.Regions {
		if r.Rect.In(v.Rect) {
			if r.Kind != output.RegionVideo || r.VideoID != v.ID {
				t.Fatalf("тайл %v внутри видео: %v id %d", r.Rect, r.Kind, r.VideoID)
			}
			n++
		}
	}
	if n != 4*3 {
		t.Fatalf("тайлов видео %d, ждали 12", n)
	}
}

// Редкие обновления — не видео.
func TestSceneVideo_LowRateIsNotVideo(t *testing.T) {
	testreset.Globals(t)
	vs := newVideoScene(t)
	vs.play(1)
	fr := vs.eng.RenderFrameNow()
	if _, ok := videoOf(fr, vs.surf.SurfaceID()); ok {
		t.Fatalf("одно обновление признано видео: %+v", fr.Videos)
	}
}

// Подсказка программы сильнее частоты.
func TestSceneVideo_ContentHint(t *testing.T) {
	testreset.Globals(t)
	vs := newVideoScene(t)
	vs.surf.SetContentHint(widget.ContentStatic)
	vs.play(30)
	if _, ok := videoOf(vs.eng.RenderFrameNow(), vs.surf.SurfaceID()); ok {
		t.Fatal("ContentStatic с частыми обновлениями признан видео")
	}
	vs.surf.SetContentHint(widget.ContentVideo)
	vs.eng.Invalidate()
	if _, ok := videoOf(vs.eng.RenderFrameNow(), vs.surf.SurfaceID()); !ok {
		t.Fatal("ContentVideo не признан видео")
	}
}

// Окно над видео попадает в Cover, тайлы под ним — не RegionVideo.
func TestSceneVideo_WindowAboveIsCover(t *testing.T) {
	testreset.Globals(t)
	vs := newVideoScene(t)
	above := widget.NewPanel(color.RGBA{R: 200, A: 255})
	above.ShowHeader = false
	above.SetBounds(image.Rect(256, 192, 448, 320))
	vs.windows.AddChild(above)
	vs.play(30)
	fr := vs.eng.RenderFrameNow()
	v, ok := videoOf(fr, vs.surf.SurfaceID())
	if !ok {
		t.Fatal("видео не найдено")
	}
	want := image.Rect(256, 192, 320, 256)
	if len(v.Cover) != 1 || v.Cover[0] != want {
		t.Fatalf("cover %v, ждали [%v]", v.Cover, want)
	}
	for _, r := range fr.Regions {
		if r.Rect.Overlaps(want) && r.Kind == output.RegionVideo {
			t.Fatalf("тайл %v под окном помечен видео", r.Rect)
		}
	}
	// Окно опустили под видео — покрытия нет.
	vs.windows.LowerChild(above)
	vs.play(1)
	fr = vs.eng.RenderFrameNow()
	if v, _ = videoOf(fr, vs.surf.SurfaceID()); len(v.Cover) != 0 {
		t.Fatalf("после LowerChild cover %v", v.Cover)
	}
}

// Пауза дольше VideoHold — не видео; простой дольше VideoHold+секунды —
// поверхность уходит из реестра.
func TestSceneVideo_HysteresisAndIdle(t *testing.T) {
	testreset.Globals(t)
	oldHold := VideoHold
	VideoHold = 50 * time.Millisecond
	t.Cleanup(func() { VideoHold = oldHold })
	vs := newVideoScene(t)
	vs.play(30)
	if _, ok := videoOf(vs.eng.RenderFrameNow(), vs.surf.SurfaceID()); !ok {
		t.Fatal("видео не признано")
	}
	time.Sleep(1200 * time.Millisecond)
	before := widget.VideoCandidates()
	vs.eng.Invalidate()
	if _, ok := videoOf(vs.eng.RenderFrameNow(), vs.surf.SurfaceID()); ok {
		t.Fatal("после паузы окно всё ещё видео")
	}
	if after := widget.VideoCandidates(); after >= before {
		t.Fatalf("простаивающее окно не ушло из реестра: %d → %d", before, after)
	}
	// Новые обновления возвращают его в реестр: снова видео. Общее число
	// сравнивать нельзя — в том же кадре уходят простаивающие окна соседних
	// тестов.
	vs.play(30)
	if _, ok := videoOf(vs.eng.RenderFrameNow(), vs.surf.SurfaceID()); !ok {
		t.Fatal("после новых обновлений окно не вернулось в видео")
	}
}

// drawCounter считает свои Draw.
type drawCounter struct {
	widget.Base
	n atomic.Int32
}

func (d *drawCounter) Draw(ctx widget.DrawContext) {
	d.n.Add(1)
	r := d.Bounds()
	ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), color.RGBA{G: 255, A: 255})
}

// Непрозрачная область от клиента закрывает то, что под окном, даже у
// буфера с альфой.
func TestSceneVideo_OpaqueRegionOccludes(t *testing.T) {
	testreset.Globals(t)
	vs := newVideoScene(t)
	under := &drawCounter{}
	under.SetBounds(image.Rect(100, 100, 200, 200))
	vs.windows.InsertChildAt(0, under)
	vs.surf.SetBuffer(vs.buf, false) // буфер с альфой
	vs.eng.Invalidate()
	vs.eng.RenderOnce()
	if under.n.Load() == 0 {
		t.Fatal("под прозрачным окном виджет не рисовался — тест не о том")
	}
	vs.surf.SetOpaqueRegion([]image.Rectangle{vs.buf.Rect})
	before := under.n.Load()
	vs.eng.Invalidate()
	vs.eng.RenderOnce()
	if under.n.Load() != before {
		t.Fatal("под непрозрачной областью виджет рисуется")
	}
	if got := vs.surf.OpaqueRegion(); len(got) != 1 || got[0] != vs.surf.Bounds() {
		t.Fatalf("OpaqueRegion %v", got)
	}
	vs.surf.SetOpaqueRegion(nil)
	if got := vs.surf.OpaqueRegion(); got != nil {
		t.Fatalf("после снятия у буфера с альфой OpaqueRegion %v", got)
	}
}

// Видео чужого окна (поверхность в реестре процесса, но в другом дереве) не
// стоит аллокаций при сборе видео для этого корня: прежде screenCover копил
// покрытие соседа в срез до проверки дерева, и движок без видео аллоцировал
// каждый кадр, пока видео соседа не остынет (TestPopupCache_NoAllocPerFrame
// после TestSceneModal_CoversSurface под -shuffle=on).
func TestSceneVideo_ForeignVideoNoAlloc(t *testing.T) {
	testreset.Globals(t)
	vs := newVideoScene(t)
	// Окно над видео — у чужой поверхности непустое покрытие.
	over := widget.NewPanel(color.RGBA{R: 200, A: 255})
	over.ShowHeader = false
	over.SetBounds(image.Rect(200, 200, 400, 300))
	vs.windows.AddChild(over)
	vs.play(30)

	now := time.Now()
	own := widget.CollectVideoSurfaces(vs.eng.Root(), now, VideoRate, VideoHold, nil)
	if len(own) != 1 || len(own[0].Cover) == 0 {
		t.Fatalf("своё видео с покрытием не найдено — тест не о том: %+v", own)
	}

	other := widget.NewPanel(color.RGBA{A: 255})
	other.SetBounds(image.Rect(0, 0, 300, 300))
	var buf []widget.VideoSurface
	allocs := testing.AllocsPerRun(50, func() {
		buf = widget.CollectVideoSurfaces(other, now, VideoRate, VideoHold, buf[:0])
	})
	if len(buf) != 0 {
		t.Fatalf("чужое видео попало в чужое дерево: %+v", buf)
	}
	if allocs != 0 {
		t.Fatalf("сбор видео для дерева без видео аллоцирует: %v на вызов", allocs)
	}
}
