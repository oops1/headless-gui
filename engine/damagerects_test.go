package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Список damage-областей вместо одного объединения.
//
// Что именно это даёт. Тайлы, которые не изменились, не уходили потребителю и
// раньше — diff их отбрасывал. Цена объединения была в другом: СРАВНИВАЛИСЬ
// все тайлы прямоугольника, накрывающего обе области. Два изменения в
// противоположных углах экрана 640×480 — это 80 тайлов на сравнение вместо
// двух, то есть примерно мегабайт memcmp на каждый кадр перетаскивания.
//
// Поэтому проверяется именно граница сравнения: тайл, изменившийся вне
// заявленных областей, наружу не уходит.

func TestDamage_KeepsRectsApart(t *testing.T) {
	e := New(640, 480, 60)
	e.SetRenderOnDemand(true)
	e.consumeDamage()

	e.InvalidateRect(image.Rect(0, 0, 20, 20))
	e.InvalidateRect(image.Rect(600, 440, 620, 460))

	rects, all := e.consumeDamage()
	if all {
		t.Fatal("InvalidateRect не должен требовать полного кадра")
	}
	if len(rects) != 2 {
		t.Fatalf("областей %d, ждали 2: %v", len(rects), rects)
	}
}

func TestDamage_AbsorbsNested(t *testing.T) {
	e := New(640, 480, 60)
	e.consumeDamage()

	e.InvalidateRect(image.Rect(0, 0, 100, 100))
	e.InvalidateRect(image.Rect(10, 10, 20, 20)) // внутри первой
	rects, _ := e.consumeDamage()
	if len(rects) != 1 {
		t.Errorf("вложенная область не поглощена: %v", rects)
	}

	e.InvalidateRect(image.Rect(10, 10, 20, 20))
	e.InvalidateRect(image.Rect(0, 0, 100, 100)) // поглощает предыдущую
	rects, _ = e.consumeDamage()
	if len(rects) != 1 {
		t.Errorf("поглощающая область не вытеснила вложенную: %v", rects)
	}
}

func TestDamage_CollapsesWhenTooMany(t *testing.T) {
	e := New(640, 480, 60)
	e.consumeDamage()

	// Заведомо больше порога — и все непересекающиеся, чтобы ни одна не
	// поглотилась.
	for i := 0; i < maxDamageRects+5; i++ {
		x := i * 8
		e.InvalidateRect(image.Rect(x, 0, x+4, 4))
	}
	rects, _ := e.consumeDamage()
	if len(rects) > maxDamageRects {
		t.Errorf("список не схлопнулся: %d областей", len(rects))
	}
}

// Тайлы вне заявленных областей не сравниваются — даже если они изменились.
//
// Это и есть разница между списком и объединением: середина экрана лежит
// внутри объединения двух углов, но ни в одной из заявленных областей.
// Раньше её тайлы сравнивались и уходили потребителю; теперь — нет.
//
// Тест намеренно нарушает контракт InvalidateRect (заявлены не все
// изменившиеся области) — иначе разницу не увидеть.
func TestDamage_UnclaimedChangeStaysHome(t *testing.T) {
	const w, h = 640, 480
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	e := New(w, h, 60)
	e.SetRenderOnDemand(true)

	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	corner := widget.NewPanel(white)
	corner.ShowHeader = false
	corner.SetBounds(image.Rect(0, 0, 32, 32))
	root.AddChild(corner)

	middle := widget.NewPanel(white)
	middle.ShowHeader = false
	middleRect := image.Rect(300, 220, 340, 260)
	middle.SetBounds(middleRect)
	root.AddChild(middle)

	e.SetRoot(root)
	e.renderFrame() // первый кадр рисует всё
	e.consumeDamage()

	// Меняются оба, но заявлены только углы. Прямая запись в экспортированное
	// поле движку не видна — ровно тот случай, ради которого существует
	// InvalidateRect.
	corner.Background = color.RGBA{R: 255, A: 255}
	middle.Background = color.RGBA{B: 255, A: 255}
	e.InvalidateRect(image.Rect(0, 0, 32, 32))
	e.InvalidateRect(image.Rect(600, 440, 640, 480))

	frame := e.renderFrame()
	for _, tile := range frame.Tiles {
		r := image.Rect(tile.X, tile.Y, tile.X+tile.W, tile.Y+tile.H)
		if r.Overlaps(middleRect) {
			t.Errorf("тайл %v из незаявленной середины ушёл наружу — сравнение "+
				"опять идёт по объединению областей", r)
		}
	}
}
