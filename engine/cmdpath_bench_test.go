package engine

// cmdpath_bench_test.go — путь команд против прямого на одних сценах
// (ADR-0004, приёмка 3). Пары «cmd/direct» в одном запуске: сравнивать надо
// отношения внутри запуска, а не абсолютные числа между запусками.

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

// benchCmdPair гонит run с путём команд и без него.
func benchCmdPair(b *testing.B, run func(b *testing.B, on bool)) {
	b.Run("cmd", func(b *testing.B) { run(b, true) })
	b.Run("direct", func(b *testing.B) { run(b, false) })
}

// BenchmarkCmdPath_FullFrame — полный кадр текстовой сцены (Invalidate перед
// каждым): запись всего дерева и воспроизведение против прямого Draw.
func BenchmarkCmdPath_FullFrame(b *testing.B) {
	benchCmdPair(b, func(b *testing.B, on bool) {
		eng, _ := benchTextScene(1280, 800)
		defer eng.Stop()
		eng.SetCommandPath(on)
		eng.renderFrame()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eng.Invalidate()
			eng.renderFrame()
		}
	})
}

// BenchmarkCmdPath_LabelChange — SetText одного лейбла из 180 (владельческая
// инвалидация): на пути команд перезаписывается один лейбл, остальное —
// воспроизведение по повреждению.
func BenchmarkCmdPath_LabelChange(b *testing.B) {
	benchCmdPair(b, func(b *testing.B, on bool) {
		eng, root := benchTextScene(1280, 800)
		defer eng.Stop()
		eng.SetCommandPath(on)
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

// BenchmarkCmdPath_ForceRect — InvalidateRect одной ячейки (ничейное
// повреждение): перезаписываются корень и лейблы под областью.
func BenchmarkCmdPath_ForceRect(b *testing.B) {
	benchCmdPair(b, func(b *testing.B, on bool) {
		eng, _ := benchTextScene(1280, 800)
		defer eng.Stop()
		eng.SetCommandPath(on)
		eng.SetRenderOnDemand(true)
		eng.renderFrame()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eng.InvalidateRect(image.Rect(400, 400, 600, 424))
			eng.renderFrame()
		}
	})
}

// BenchmarkCmdPath_NoDamage — кадр без заявленного повреждения (RenderOnce):
// на обоих путях — всё заново.
func BenchmarkCmdPath_NoDamage(b *testing.B) {
	benchCmdPair(b, func(b *testing.B, on bool) {
		eng, _ := benchTextScene(1280, 800)
		defer eng.Stop()
		eng.SetCommandPath(on)
		eng.renderFrame()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eng.renderFrame()
		}
	})
}

// BenchmarkCmdPath_DesktopClockTick — тик часов рабочего стола: область
// часов заявлена движку (как в Pipeline_ClockTick).
func BenchmarkCmdPath_DesktopClockTick(b *testing.B) {
	benchCmdPair(b, func(b *testing.B, on bool) {
		sc := buildPipelineDesktop()
		defer sc.closeAll()
		defer sc.eng.Stop()
		eng := sc.eng
		eng.SetCommandPath(on)
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

// BenchmarkCmdPath_DesktopHover — наведение на «Пуск» и уход с него
// (как Pipeline_HoverButton): сеттер кнопки, владельческая инвалидация.
func BenchmarkCmdPath_DesktopHover(b *testing.B) {
	benchCmdPair(b, func(b *testing.B, on bool) {
		sc := buildPipelineDesktop()
		defer sc.closeAll()
		defer sc.eng.Stop()
		eng := sc.eng
		eng.SetCommandPath(on)
		eng.SetRenderOnDemand(true)
		eng.renderFrame()
		btn := sc.startBtn.Bounds()
		onX, onY := (btn.Min.X+btn.Max.X)/2, (btn.Min.Y+btn.Max.Y)/2
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if i%2 == 0 {
				eng.SendMouseMove(onX, onY)
			} else {
				eng.SendMouseMove(onX, pipeH/2)
			}
			eng.renderFrame()
		}
	})
}

// bigRow — строка таблицы на десять столбцов.
type bigRow struct{ C0, C1, C2, C3, C4, C5, C6, C7, C8, C9 string }

// benchBigScene — сцена на ≥10 тыс. команд: матрица из 12 тыс. коротких
// лейблов во весь экран, поверх неё таблица 20×8 и два окна с текстом.
// Число команд полного кадра печатает BenchmarkCmdPath_BigPartial.
func benchBigScene() (*Engine, *widget.Label) {
	const w, h = 1920, 1200
	eng := New(w, h, 20)
	root := widget.NewPanel(color.RGBA{R: 28, G: 30, B: 36, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	var status *widget.Label
	for r := 0; r < 75; r++ {
		for c := 0; c < 160; c++ {
			l := widget.NewLabel(fmt.Sprintf("%d", (r+c)%10), color.RGBA{R: 200, G: 200, B: 200, A: 255})
			l.SetBounds(image.Rect(c*12, r*16, c*12+12, r*16+16))
			root.AddChild(l)
			if r == 74 && c == 0 {
				status = l
			}
		}
	}

	dg := widget.NewDataGridWidget()
	oc := datagrid.NewObservableCollection()
	for i := 0; i < 20; i++ {
		oc.Add(bigRow{
			fmt.Sprintf("%03d", i), "Иванов", "Пётр", "Сергеевич", "Отдел продаж",
			fmt.Sprintf("%d", 1000+i*7), "активен", "2026-10-28", "Москва", "примечание",
		})
	}
	dg.Grid.SetItemsSource(oc)
	for c := 0; c < 8; c++ {
		name := fmt.Sprintf("C%d", c)
		dg.Grid.AddColumn(datagrid.NewTextColumn(name, name))
	}
	dg.SetBounds(image.Rect(100, 100, 740, 100+dg.Grid.HeaderHeight+dg.Grid.RowHeight*20))
	root.AddChild(dg)

	for k := 0; k < 2; k++ {
		win := widget.NewWindow(fmt.Sprintf("Окно %d", k+1), 500, 300)
		win.SetBounds(image.Rect(1000, 100+k*400, 1500, 400+k*400))
		for r := 0; r < 8; r++ {
			for c := 0; c < 3; c++ {
				l := widget.NewLabel(fmt.Sprintf("поле %d.%d = %d", r, c, r*3+c), color.RGBA{R: 220, G: 220, B: 220, A: 255})
				l.SetBounds(image.Rect(1010+c*160, 140+k*400+r*26, 1160+c*160, 162+k*400+r*26))
				win.AddChild(l)
			}
		}
		root.AddChild(win)
	}
	eng.SetRoot(root)
	return eng, status
}

// BenchmarkCmdPath_BigForce — та же сцена, ничейное повреждение одной
// ячейки (InvalidateRect): корень перезаписывается целиком.
func BenchmarkCmdPath_BigForce(b *testing.B) {
	benchCmdPair(b, func(b *testing.B, on bool) {
		eng, status := benchBigScene()
		defer eng.Stop()
		eng.SetCommandPath(on)
		eng.SetRenderOnDemand(true)
		eng.renderFrame()
		rc := status.Bounds()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eng.InvalidateRect(rc)
			eng.renderFrame()
		}
	})
}

// BenchmarkCmdPath_BigPartial — частичный кадр на сцене ≥10 тыс. команд:
// меняется один лейбл строки состояния.
func BenchmarkCmdPath_BigPartial(b *testing.B) {
	benchCmdPair(b, func(b *testing.B, on bool) {
		eng, status := benchBigScene()
		defer eng.Stop()
		eng.SetCommandPath(on)
		eng.SetRenderOnDemand(true)
		eng.renderFrame()
		if on {
			b.Logf("команд в полном кадре: %d", eng.Stats().Recorded)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			status.SetText(fmt.Sprintf("%d", i&1))
			eng.renderFrame()
		}
	})
}

// BenchmarkCmdPath_BigFull — полный кадр той же сцены.
func BenchmarkCmdPath_BigFull(b *testing.B) {
	benchCmdPair(b, func(b *testing.B, on bool) {
		eng, _ := benchBigScene()
		defer eng.Stop()
		eng.SetCommandPath(on)
		eng.renderFrame()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eng.Invalidate()
			eng.renderFrame()
		}
	})
}
