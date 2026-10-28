package engine

import (
	"bytes"
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

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
	k := 1 // картинки чередуются, чтобы каждый кадр отличался от прошлого
	for i := 0; i < 20 && !reused; i++ {
		iw.SetImage(bufs[k&1])
		k++
		fr := eng.renderFrame()
		first := &fr.Tiles[0].Data[0]
		eng.deliver(fr) // канал полон — кадр выброшен и освобождён
		if !eng.lostFrame.Load() {
			t.Fatal("потеря кадра не запомнена")
		}
		iw.SetImage(bufs[k&1])
		k++
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
