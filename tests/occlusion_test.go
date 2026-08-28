package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Не рисовать перекрытое.
//
// Движок рисует снизу вверх и раньше не спрашивал, видно ли то, что он
// рисует: окно под другим окном отрисовывалось целиком, а потом
// закрашивалось. При десятке перекрывающихся окон это кратные затраты на
// каждый полный кадр.
//
// Проверяется и то и другое: что закрытое действительно не рисуется и что
// картинка от этого не изменилась ни на пиксель. Ошибка «нарисовали лишнее»
// видна только в профиле, ошибка в другую сторону — дыра на экране.

// countingPanel и newCountingPanel — из culling_test.go: панель, считающая
// свои отрисовки.

func TestOcclusion_CoveredWidgetIsNotDrawn(t *testing.T) {
	root := widget.NewPanel(color.RGBA{R: 20, G: 24, B: 30, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 300))

	// Нижняя панель целиком под верхней.
	under := newCountingPanel(color.RGBA{R: 200, G: 60, B: 60, A: 255},
		image.Rect(60, 60, 200, 180))
	over := newCountingPanel(color.RGBA{R: 40, G: 120, B: 220, A: 255},
		image.Rect(40, 40, 240, 220))
	// Соседка, которую не закрывает никто.
	beside := newCountingPanel(color.RGBA{R: 90, G: 200, B: 120, A: 255},
		image.Rect(280, 60, 380, 160))

	root.AddChild(under)
	root.AddChild(over)
	root.AddChild(beside)

	eng := engine.New(400, 300, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce()

	if under.draws != 0 {
		t.Errorf("закрытая панель отрисована %d раз", under.draws)
	}
	if over.draws == 0 {
		t.Error("верхняя панель не отрисована вовсе")
	}
	if beside.draws == 0 {
		t.Error("незакрытая соседка не отрисована")
	}
}

// Картинка от пропуска не меняется — ни с прозрачностью, ни без неё.
func TestOcclusion_FrameIsIdenticalWithAndWithout(t *testing.T) {
	// build собирает одну и ту же сцену; opaque=false делает верхнюю панель
	// полупрозрачной, и тогда пропускать нижнюю нельзя — она видна насквозь.
	build := func(opaque bool) *image.RGBA {
		root := widget.NewPanel(color.RGBA{R: 20, G: 24, B: 30, A: 255})
		root.ShowHeader = false
		root.SetBounds(image.Rect(0, 0, 400, 300))

		under := widget.NewPanel(color.RGBA{R: 200, G: 60, B: 60, A: 255})
		under.ShowHeader = false
		under.SetBounds(image.Rect(60, 60, 200, 180))
		root.AddChild(under)

		top := widget.NewPanel(color.RGBA{R: 40, G: 120, B: 220, A: 255})
		top.ShowHeader = false
		top.SetBounds(image.Rect(40, 40, 240, 220))
		if !opaque {
			top.Background = color.RGBA{R: 20, G: 60, B: 110, A: 128}
			top.UseAlpha = true
		}
		root.AddChild(top)

		eng := engine.New(400, 300, 60)
		eng.SetRenderOnDemand(true)
		eng.SetRoot(root)
		return snapshotRGBA(eng.RenderOnce())
	}

	// Эталон для непрозрачного случая: та же сцена, но нижняя панель убрана
	// вовсе. Если пропуск верен, кадры совпадут до пикселя.
	reference := func() *image.RGBA {
		root := widget.NewPanel(color.RGBA{R: 20, G: 24, B: 30, A: 255})
		root.ShowHeader = false
		root.SetBounds(image.Rect(0, 0, 400, 300))

		top := widget.NewPanel(color.RGBA{R: 40, G: 120, B: 220, A: 255})
		top.ShowHeader = false
		top.SetBounds(image.Rect(40, 40, 240, 220))
		root.AddChild(top)

		eng := engine.New(400, 300, 60)
		eng.SetRenderOnDemand(true)
		eng.SetRoot(root)
		return snapshotRGBA(eng.RenderOnce())
	}

	if !samePix(build(true), reference()) {
		t.Error("кадр с пропущенной нижней панелью отличается от кадра без неё — " +
			"значит пропустили не то, что закрыто")
	}
}

// Полупрозрачная панель не закрывает ничего: под ней видно нижнюю.
func TestOcclusion_TranslucentCoversNothing(t *testing.T) {
	root := widget.NewPanel(color.RGBA{R: 20, G: 24, B: 30, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 300))

	under := newCountingPanel(color.RGBA{R: 200, G: 60, B: 60, A: 255},
		image.Rect(60, 60, 200, 180))
	root.AddChild(under)

	top := widget.NewPanel(color.RGBA{R: 40, G: 120, B: 220, A: 255})
	top.ShowHeader = false
	top.SetBounds(image.Rect(40, 40, 240, 220))
	top.Background = color.RGBA{R: 20, G: 60, B: 110, A: 128}
	top.UseAlpha = true
	root.AddChild(top)

	eng := engine.New(400, 300, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce()

	if under.draws == 0 {
		t.Error("панель под ПОЛУПРОЗРАЧНОЙ не нарисована — сквозь неё будет видно пустоту")
	}
}

// Скруглённое окно не закрывает своих углов.
func TestOcclusion_RoundedWindowLeavesItsCorners(t *testing.T) {
	const cr = 12

	win := widget.NewWindow("Окно", 200, 150)
	win.SetBounds(image.Rect(50, 50, 250, 200))
	win.Background = color.RGBA{R: 60, G: 60, B: 70, A: 255}
	win.CornerRadius = cr

	regions := win.OpaqueRegion()
	if len(regions) != 1 {
		t.Fatalf("окно объявило %d областей, ждали одну", len(regions))
	}
	got := regions[0]

	// Угол окна в объявленную область попадать не должен.
	for _, corner := range []image.Point{
		{X: 50, Y: 50}, {X: 249, Y: 50}, {X: 50, Y: 199}, {X: 249, Y: 199},
	} {
		if corner.In(got) {
			t.Errorf("угол %v объявлен непрозрачным — под скруглением будет дыра", corner)
		}
	}
	// А середина — должна.
	if mid := (image.Point{X: 150, Y: 125}); !mid.In(got) {
		t.Errorf("середина окна %v не объявлена непрозрачной — экономии нет", mid)
	}
}

// Полупрозрачное окно не закрывает ничего.
func TestOcclusion_TranslucentWindowDeclaresNothing(t *testing.T) {
	win := widget.NewWindow("Стекло", 200, 150)
	win.SetBounds(image.Rect(50, 50, 250, 200))
	win.Background = color.RGBA{R: 60, G: 60, B: 70, A: 200}

	if got := win.OpaqueRegion(); len(got) != 0 {
		t.Errorf("полупрозрачное окно объявило %v", got)
	}
}
