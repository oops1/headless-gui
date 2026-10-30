package engine

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/widget"
)

// E7: сессия 4K (33 МБ), в кадре меняется 10% площади. Что стоит кадр:
//
//   - ExternalSurface: приложение пишет кусок в свой буфер, виджет копирует в
//     холст только его (потом дифф только его тайлов);
//   - Copy_Damage10: прежний обходной путь — приложение копирует кусок в буфер
//     ImageWidget (одна лишняя копия), виджет кладёт его через
//     DrawImage(OpaqueImage), движку заявлена та же область;
//   - Full: DrawImage(OpaqueImage) полного буфера при полном damage — то, что
//     делал бы клиент без заявления областей (см. BenchmarkPan4K_OpaqueRecycle).
//
// Приложение «декодирует» кадр обновлением буфера (refill на 10% площади);
// в обходном пути это ещё и копирование в чужой буфер — оно входит в замер.

const (
	esBenchW, esBenchH = 3840, 2160
)

// esBenchRect — прямоугольник 10% площади, сдвигающийся по кадрам (чтобы
// содержимое каждый раз отличалось, а позиция — нет: тайлы одни и те же).
func esBenchRect() image.Rectangle {
	// 1214×683 ≈ 10% от 3840×2160, выровнено по тайлам не специально.
	return image.Rect(300, 200, 300+1214, 200+683)
}

// writeRect — «декодирование»: новое содержимое в прямоугольник буфера.
func writeRect(buf *image.RGBA, r image.Rectangle, seed int) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := buf.Pix[y*buf.Stride+r.Min.X*4 : y*buf.Stride+r.Max.X*4]
		v := uint8(seed + y)
		for i := 0; i < len(row); i += 4 {
			row[i+0], row[i+1], row[i+2], row[i+3] = v+uint8(i), v, uint8(seed), 255
		}
	}
}

func benchSurface(b *testing.B, rects []image.Rectangle) {
	eng := New(esBenchW, esBenchH, 20)
	b.Cleanup(eng.Stop)
	eng.SetFrameRecycling(true)
	buf := image.NewRGBA(image.Rect(0, 0, esBenchW, esBenchH))
	writeRect(buf, buf.Rect, 1)
	s := widget.NewExternalSurface()
	s.SetBounds(buf.Rect)
	s.SetBuffer(buf, true)
	eng.SetRoot(s)
	eng.renderFrame().Release()
	var area int
	for _, r := range rects {
		area += r.Dx() * r.Dy()
	}
	b.ReportAllocs()
	b.SetBytes(int64(area * 4))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Update(func(buf *image.RGBA) []image.Rectangle {
			for _, r := range rects {
				writeRect(buf, r, i+2)
			}
			return rects
		})
		fr := eng.renderFrame()
		if len(fr.Tiles) == 0 {
			b.Fatal("кадр без тайлов")
		}
		fr.Release()
	}
}

func BenchmarkExternalSurface4K_Damage10(b *testing.B) {
	benchSurface(b, []image.Rectangle{esBenchRect()})
}

// Те же 10%, но четырьмя далёкими областями: копируется охват (это цена
// объединения клипа), тайлы в дифф идут только свои.
func BenchmarkExternalSurface4K_Damage10Scattered(b *testing.B) {
	benchSurface(b, []image.Rectangle{
		image.Rect(0, 0, 607, 342),
		image.Rect(3233, 0, 3840, 342),
		image.Rect(0, 1818, 607, 2160),
		image.Rect(3233, 1818, 3840, 2160),
	})
}

// Полный буфер заявлен целиком (первый кадр, смена размера).
func BenchmarkExternalSurface4K_Full(b *testing.B) {
	benchSurface(b, []image.Rectangle{image.Rect(0, 0, esBenchW, esBenchH)})
}

// Прежний обходной путь с 10%: приложение копирует кусок в буфер ImageWidget,
// движку заявляется та же область.
func BenchmarkExternalSurface4K_CopyIntoImageWidget_Damage10(b *testing.B) {
	eng := New(esBenchW, esBenchH, 20)
	b.Cleanup(eng.Stop)
	eng.SetFrameRecycling(true)
	app := image.NewRGBA(image.Rect(0, 0, esBenchW, esBenchH)) // буфер приложения
	writeRect(app, app.Rect, 1)
	own := image.NewRGBA(app.Rect) // буфер виджета
	copy(own.Pix, app.Pix)
	iw := widget.NewImageWidget()
	iw.Stretch = widget.ImageStretchNone
	iw.SetBounds(app.Rect)
	iw.SetImage(widget.OpaqueImage(own))
	eng.SetRoot(iw)
	eng.renderFrame().Release()
	r := esBenchRect()
	b.ReportAllocs()
	b.SetBytes(int64(r.Dx() * r.Dy() * 4))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writeRect(app, r, i+2)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			o := y*app.Stride + r.Min.X*4
			copy(own.Pix[o:o+r.Dx()*4], app.Pix[o:o+r.Dx()*4])
		}
		widget.InvalidateRect(r)
		fr := eng.renderFrame()
		if len(fr.Tiles) == 0 {
			b.Fatal("кадр без тайлов")
		}
		fr.Release()
	}
}

// Полный буфер через DrawImage(OpaqueImage), без заявления областей: то, что
// пришлось бы платить, не умея сказать «изменилось вот это».
func BenchmarkExternalSurface4K_DrawImageOpaque_Full(b *testing.B) {
	benchPan(b, esBenchW, esBenchH, widget.ImageStretchNone, true, true)
}

// Весь буфер заменяется двойной буферизацией (Swap): сопоставимо с
// BenchmarkExternalSurface4K_DrawImageOpaque_Full — содержимое каждого кадра
// новое, без записи.
func BenchmarkExternalSurface4K_SwapFull(b *testing.B) {
	eng := New(esBenchW, esBenchH, 20)
	b.Cleanup(eng.Stop)
	eng.SetFrameRecycling(true)
	bufs := panBuffers(esBenchW, esBenchH)
	s := widget.NewExternalSurface()
	s.SetBounds(bufs[0].Rect)
	s.SetBuffer(bufs[0], true)
	eng.SetRoot(s)
	eng.renderFrame().Release()
	b.ReportAllocs()
	b.SetBytes(int64(esBenchW * esBenchH * 4))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Swap(bufs[(i+1)&1], true, nil)
		fr := eng.renderFrame()
		if len(fr.Tiles) == 0 {
			b.Fatal("кадр без тайлов")
		}
		fr.Release()
	}
}
