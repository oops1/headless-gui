package engine

import (
	"bytes"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/output"
	"github.com/oops1/headless-gui/v4/widget"
)

// surfacesScene — движок 256×128 с кнопкой и два чужих буфера.
type surfacesScene struct {
	eng  *Engine
	btn  *widget.Button
	bufs [][]byte
}

func newSurfacesScene(t *testing.T, n int, f PixelFormat) *surfacesScene {
	t.Helper()
	eng := New(256, 128, 30)
	t.Cleanup(eng.Stop)
	eng.SetRenderOnDemand(true)
	root := widget.NewPanel(color.RGBA{R: 30, G: 60, B: 90, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 256, 128))
	btn := widget.NewButton("a")
	btn.SetBounds(image.Rect(20, 20, 120, 60))
	root.AddChild(btn)
	eng.SetRoot(root)
	bufs := make([][]byte, n)
	for i := range bufs {
		bufs[i] = make([]byte, 256*128*4)
	}
	if err := eng.SetSurfaces(bufs, 256*4, f); err != nil {
		t.Fatal(err)
	}
	return &surfacesScene{eng: eng, btn: btn, bufs: bufs}
}

func (sc *surfacesScene) front() []byte { return sc.eng.canvas.Front().Pix }

// Кадры чередуют буферы; перед рисованием буфер приводится к прошлому кадру,
// так что после кадра он совпадает с front целиком, а не только в damage.
func TestSurfaces_AlternateAndMatchFront(t *testing.T) {
	testreset.Globals(t)
	for _, f := range []PixelFormat{FormatRGBA, FormatBGRX} {
		sc := newSurfacesScene(t, 2, f)
		eng := sc.eng
		f1 := eng.RenderFrameNow()
		if f1.Surface != 1 || len(f1.Tiles) == 0 {
			t.Fatalf("%v: первый кадр: Surface %d, тайлов %d", f, f1.Surface, len(f1.Tiles))
		}
		if !bytes.Equal(sc.bufs[0], sc.front()) {
			t.Fatalf("%v: буфер 1 после кадра не равен front", f)
		}
		sc.btn.SetText("bb")
		f2 := eng.RenderFrameNow()
		if f2.Surface != 2 || len(f2.Tiles) == 0 {
			t.Fatalf("%v: второй кадр: Surface %d, тайлов %d", f, f2.Surface, len(f2.Tiles))
		}
		if !bytes.Equal(sc.bufs[1], sc.front()) {
			t.Fatalf("%v: буфер 2 после кадра не равен front (копия прошлого кадра вне damage)", f)
		}
		if bytes.Equal(sc.bufs[0], sc.bufs[1]) {
			t.Fatalf("%v: кадры не отличаются — тест ничего не проверил", f)
		}
		// Оба у потребителя: кадр пропускается, damage не теряется.
		sc.btn.SetText("ccc")
		skipped := eng.Stats().FramesSkipped
		f3 := eng.RenderFrameNow()
		if f3.Surface != 0 || len(f3.Tiles) != 0 {
			t.Fatalf("%v: при занятых буферах кадр нарисован: Surface %d, тайлов %d", f, f3.Surface, len(f3.Tiles))
		}
		if eng.Stats().FramesSkipped != skipped+1 {
			t.Fatalf("%v: пропуск не посчитан", f)
		}
		eng.ReleaseSurface(1)
		f4 := eng.RenderFrameNow()
		if f4.Surface != 1 || len(f4.Tiles) == 0 {
			t.Fatalf("%v: после ReleaseSurface(1): Surface %d, тайлов %d", f, f4.Surface, len(f4.Tiles))
		}
		if !bytes.Equal(sc.bufs[0], sc.front()) {
			t.Fatalf("%v: буфер 1 после двух пропущенных им кадров не равен front", f)
		}
	}
}

type surfaceSink struct{ ch chan output.Frame }

func (s surfaceSink) Present(fr output.Frame) { s.ch <- fr }

// Внешний темп: ReleaseSurface после пропущенного кадра готовит кадр сам.
func TestSurfaces_ReleaseWakesExternalPacing(t *testing.T) {
	testreset.Globals(t)
	sc := newSurfacesScene(t, 2, FormatRGBA)
	eng := sc.eng
	sink := surfaceSink{ch: make(chan output.Frame, 8)}
	eng.SetFrameSink(sink)
	eng.SetPacing(PacingExternal)
	eng.Start()
	next := func(what string) output.Frame {
		t.Helper()
		select {
		case fr := <-sink.ch:
			return fr
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: кадр не пришёл", what)
		}
		return output.Frame{}
	}
	eng.RequestFrame()
	if fr := next("первый"); fr.Surface != 1 {
		t.Fatalf("первый кадр в буфере %d", fr.Surface)
	}
	eng.Post(func() { sc.btn.SetText("bb") })
	eng.RequestFrame()
	if fr := next("второй"); fr.Surface != 2 {
		t.Fatalf("второй кадр в буфере %d", fr.Surface)
	}
	eng.Post(func() { sc.btn.SetText("ccc") })
	eng.RequestFrame()
	select {
	case fr := <-sink.ch:
		t.Fatalf("кадр при занятых буферах: Surface %d", fr.Surface)
	case <-time.After(150 * time.Millisecond):
	}
	eng.ReleaseSurface(1)
	if fr := next("после ReleaseSurface"); fr.Surface != 1 || len(fr.Tiles) == 0 {
		t.Fatalf("после ReleaseSurface: Surface %d, тайлов %d", fr.Surface, len(fr.Tiles))
	}
}

// Смена разрешения сбрасывает набор; один буфер — прежний SetSurface;
// негодные наборы отвергаются.
func TestSurfaces_DropAndErrors(t *testing.T) {
	testreset.Globals(t)
	sc := newSurfacesScene(t, 2, FormatRGBA)
	eng := sc.eng
	eng.RenderFrameNow()
	eng.SetResolution(300, 100)
	if n := eng.Surfaces(); n != 0 {
		t.Fatalf("после SetResolution набор не сброшен: %d", n)
	}
	if fr := eng.RenderFrameNow(); fr.Surface != 0 {
		t.Fatalf("после сброса кадр в буфере %d", fr.Surface)
	}

	one := make([]byte, 300*100*4)
	if err := eng.SetSurfaces([][]byte{one}, 300*4, FormatRGBA); err != nil || eng.Surfaces() != 0 {
		t.Fatalf("один буфер: err %v, набор %d", err, eng.Surfaces())
	}
	if fr := eng.RenderFrameNow(); fr.Surface != 0 || eng.canvas.Back().Pix == nil || &eng.canvas.Back().Pix[0] != &one[0] {
		t.Fatalf("один буфер: кадр Surface %d, задний буфер не тот", fr.Surface)
	}

	five := make([][]byte, 5)
	for i := range five {
		five[i] = make([]byte, 300*100*4)
	}
	if err := eng.SetSurfaces(five, 300*4, FormatRGBA); err == nil {
		t.Fatal("пять буферов приняты")
	}
	small := [][]byte{make([]byte, 300*100*4), make([]byte, 10)}
	if err := eng.SetSurfaces(small, 300*4, FormatRGBA); err == nil {
		t.Fatal("малый буфер принят")
	}
	if err := eng.SetSurfaces([][]byte{one, make([]byte, 300*100*4)}, 300*4, PixelFormat(7)); err == nil {
		t.Fatal("неизвестный формат принят")
	}
	// Набор после ошибки не задан: прежний одиночный буфер остаётся.
	if eng.Surfaces() != 0 {
		t.Fatal("после отвергнутого набора Surfaces != 0")
	}
}
