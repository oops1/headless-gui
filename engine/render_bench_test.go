package engine

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// benchTextScene строит текстоёмкую сцену: сетка лейблов на панели.
// Приближение реального UI (таблицы, формы) — основная нагрузка на текст.
func benchTextScene(w, h int) (*Engine, widget.Widget) {
	eng := New(w, h, 20)
	panel := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 34, A: 255})
	panel.SetBounds(image.Rect(0, 0, w, h))

	cols := 6
	rows := 30
	cellW := w / cols
	cellH := 24
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			lbl := widget.NewLabel(
				fmt.Sprintf("Ячейка row=%d col=%d value=%d", r, c, r*cols+c),
				color.RGBA{R: 220, G: 220, B: 220, A: 255},
			)
			lbl.SetBounds(image.Rect(c*cellW+4, r*cellH+4, (c+1)*cellW-4, (r+1)*cellH))
			panel.AddChild(lbl)
		}
	}
	eng.SetRoot(panel)
	return eng, panel
}

// BenchmarkRenderFrameFull — полный кадр 1280×800 с ~180 текстовыми лейблами
// (фон + дерево + полный diff). Главная метрика стоимости кадра.
func BenchmarkRenderFrameFull(b *testing.B) {
	eng, _ := benchTextScene(1280, 800)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.Invalidate() // полный редрав каждый кадр
		eng.renderFrame()
	}
}

// BenchmarkRenderFrameNoChange — кадр без изменений UI: фон + дерево + diff,
// но все тайлы совпадают (диагностирует стоимость перерисовки и сравнения).
func BenchmarkRenderFrameNoChange(b *testing.B) {
	eng, _ := benchTextScene(1280, 800)
	eng.renderFrame() // прогрев: первый кадр синхронизирует front
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.renderFrame()
	}
}

// BenchmarkRenderFramePartial — on-demand режим, инвалидирована одна ячейка
// 200×24: измеряет выигрыш частичной перерисовки/diff.
func BenchmarkRenderFramePartial(b *testing.B) {
	eng, _ := benchTextScene(1280, 800)
	eng.SetRenderOnDemand(true)
	eng.renderFrame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eng.InvalidateRect(image.Rect(400, 400, 600, 424))
		eng.renderFrame()
	}
}

// BenchmarkDrawText — чистая стоимость отрисовки строки (40 символов, кириллица+латиница).
func BenchmarkDrawText(b *testing.B) {
	eng := New(640, 480, 20)
	c := eng.canvas
	col := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.DrawTextSize("Пример строки Example string 0123456789", 10, 100, 12, col)
	}
}

// BenchmarkDiffFullNoChange — чистая стоимость полного diff 1920×1080 без изменений.
func BenchmarkDiffFullNoChange(b *testing.B) {
	eng := New(1920, 1080, 20)
	c := eng.canvas
	c.blitBackground()
	c.diffAndSync()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.diffAndSync()
	}
}
