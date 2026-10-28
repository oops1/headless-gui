package engine

// tiled_bench_test.go — тайловый растр против воспроизведения одной горутиной
// (ADR-0005, приёмка 3). Пары «tiled/serial» в одном запуске; стадия растра
// отдельно: кадр целиком включает запись, фон, дифф и всё прочее.

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

func benchTiledPair(b *testing.B, run func(b *testing.B, tiled bool)) {
	b.Run("tiled", func(b *testing.B) { run(b, true) })
	b.Run("serial", func(b *testing.B) { run(b, false) })
}

// BenchmarkTiled_FullFrame — полный текстовый кадр (Invalidate перед каждым).
func BenchmarkTiled_FullFrame(b *testing.B) {
	benchTiledPair(b, func(b *testing.B, tiled bool) {
		eng, _ := benchTextScene(1280, 800)
		defer eng.Stop()
		eng.SetTiledRaster(tiled)
		eng.renderFrame()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eng.Invalidate()
			eng.renderFrame()
		}
	})
}

// BenchmarkTiled_RasterStage — только растр списка кадра (без записи, фона и
// диффа): то, что меняет 4а.2.
func BenchmarkTiled_RasterStage(b *testing.B) {
	benchTiledPair(b, func(b *testing.B, tiled bool) {
		eng, root := benchTextScene(1280, 800)
		defer eng.Stop()
		eng.SetTiledRaster(tiled)
		canvas := eng.canvas
		eng.rec.BeginFrame(canvas, false, nil)
		widget.DrawTraced(root, eng.rec)
		list, exit := eng.rec.EndFrame()
		canvas.BlitBackground()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eng.rasterTree(canvas, list, exit, false, image.Rectangle{})
		}
	})
}

// BenchmarkTiled_DesktopFull — полный кадр рабочего стола (с размытием панели).
func BenchmarkTiled_DesktopFull(b *testing.B) {
	benchTiledPair(b, func(b *testing.B, tiled bool) {
		sc := buildPipelineDesktop()
		defer sc.closeAll()
		defer sc.eng.Stop()
		eng := sc.eng
		eng.SetTiledRaster(tiled)
		eng.renderFrame()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eng.Invalidate()
			eng.renderFrame()
		}
	})
}

// BenchmarkTiled_DesktopClockTick — тик часов (InvalidateRect области часов).
func BenchmarkTiled_DesktopClockTick(b *testing.B) {
	benchTiledPair(b, func(b *testing.B, tiled bool) {
		sc := buildPipelineDesktop()
		defer sc.closeAll()
		defer sc.eng.Stop()
		eng := sc.eng
		eng.SetTiledRaster(tiled)
		eng.SetRenderOnDemand(true)
		eng.renderFrame()
		clockRect := image.Rect(pipeW-98, pipeH-44, pipeW-8, pipeH-4)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eng.InvalidateRect(clockRect)
			eng.renderFrame()
		}
	})
}

// BenchmarkTiled_DesktopUnderBlur — изменение полосы обоев под панелью задач:
// промах кэча размытия каждый кадр (ореол против последовательного растра).
func BenchmarkTiled_DesktopUnderBlur(b *testing.B) {
	benchTiledPair(b, func(b *testing.B, tiled bool) {
		sc := buildPipelineDesktop()
		defer sc.closeAll()
		defer sc.eng.Stop()
		eng := sc.eng
		eng.SetTiledRaster(tiled)
		eng.SetRenderOnDemand(true)
		eng.renderFrame()
		under := image.Rect(0, pipeH-60, pipeW, pipeH-40)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eng.InvalidateRect(under)
			eng.renderFrame()
		}
	})
}

// BenchmarkTiled_FullscreenBlurChange — стекло во весь экран на 1,25 и
// правка лейбла под ним каждый кадр: промах кэша, ореол на всех тайлах слоя.
func BenchmarkTiled_FullscreenBlurChange(b *testing.B) {
	benchTiledPair(b, func(b *testing.B, tiled bool) {
		eng := New(1280, 800, 20)
		defer eng.Stop()
		eng.SetScale(1.25)
		eng.SetTiledRaster(tiled)
		eng.SetRenderOnDemand(true)
		root, labels := busyScene(1280, 800)
		root.AddChild(newGlass(image.Rect(0, 0, 1280, 800), 22, color.RGBA{R: 200, G: 200, B: 220, A: 100}, "весь экран"))
		eng.SetRoot(root)
		eng.renderFrame()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			labels[5].SetText(fmt.Sprintf("значение %d", i&1))
			eng.renderFrame()
		}
	})
}

// BenchmarkTiled_LabelChange — SetText одного лейбла из 180 (мелкое
// повреждение: прямой путь внутри тайлового).
func BenchmarkTiled_LabelChange(b *testing.B) {
	benchTiledPair(b, func(b *testing.B, tiled bool) {
		eng, root := benchTextScene(1280, 800)
		defer eng.Stop()
		eng.SetTiledRaster(tiled)
		eng.SetRenderOnDemand(true)
		var ls []*widget.Label
		labelsOf(root, &ls)
		eng.renderFrame()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ls[77].SetText(fmt.Sprintf("значение %d", i&1))
			eng.renderFrame()
		}
	})
}

// BenchmarkTiled_BigFull и BigPartial — сцена на 21 тыс. команд.
func BenchmarkTiled_BigFull(b *testing.B) {
	benchTiledPair(b, func(b *testing.B, tiled bool) {
		eng, _ := benchBigScene()
		defer eng.Stop()
		eng.SetTiledRaster(tiled)
		eng.renderFrame()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eng.Invalidate()
			eng.renderFrame()
		}
	})
}

func BenchmarkTiled_BigPartial(b *testing.B) {
	benchTiledPair(b, func(b *testing.B, tiled bool) {
		eng, status := benchBigScene()
		defer eng.Stop()
		eng.SetTiledRaster(tiled)
		eng.SetRenderOnDemand(true)
		eng.renderFrame()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			status.SetText(fmt.Sprintf("%d", i&1))
			eng.renderFrame()
		}
	})
}
