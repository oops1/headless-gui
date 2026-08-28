// occlusion_bench_test.go — цена полного кадра со стопкой перекрытых окон.
//
// Движок рисовал снизу вверх, не спрашивая, видно ли то, что он рисует:
// нижнее окно отрисовывалось целиком, а затем закрашивалось верхним. На
// рабочем столе окна лежат стопкой почти всегда, и на каждый полный кадр это
// кратные затраты.
package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// stackedWindows — n перекрывающихся окон, каждое со своим содержимым.
// Верхнее закрывает все нижние целиком.
func stackedWindows(n int, w, h int) *widget.Panel {
	root := widget.NewPanel(color.RGBA{R: 24, G: 28, B: 36, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	for i := 0; i < n; i++ {
		// Каждое следующее окно шире предыдущего и накрывает его целиком.
		inset := (n - 1 - i) * 8
		win := widget.NewPanel(color.RGBA{R: uint8(40 + 20*i), G: 90, B: 160, A: 255})
		win.ShowHeader = false
		win.SetBounds(image.Rect(40+inset, 40+inset, w-40-inset, h-40-inset))
		root.AddChild(win)

		// Содержимое: полосы, которые надо честно растеризовать.
		for j := 0; j < 6; j++ {
			s := widget.NewPanel(color.RGBA{R: 220, G: uint8(30 + 30*j), B: 60, A: 255})
			s.ShowHeader = false
			s.SetBounds(image.Rect(60+inset+j*24, 70+inset, 76+inset+j*24, h-80-inset))
			root.AddChild(s)
		}
	}
	return root
}

func benchStack(b *testing.B, n int) {
	const w, h = 1280, 800
	root := stackedWindows(n, w, h)

	eng := New(w, h, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.Invalidate() // полный кадр: именно его и удешевляет вычитание
		eng.RenderFrameNow()
	}
}

func BenchmarkOcclusion_Stack1(b *testing.B)  { benchStack(b, 1) }
func BenchmarkOcclusion_Stack5(b *testing.B)  { benchStack(b, 5) }
func BenchmarkOcclusion_Stack10(b *testing.B) { benchStack(b, 10) }
