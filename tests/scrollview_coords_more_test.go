package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Продолжение scrollview_coords_test.go: приколотые меню, контейнеры, которые
// держат содержимое мимо Base.AddChild, и перерисовка соседей прокрутки.

// Готовое контекстное меню, приколотое к кнопке в прокрутке (как это делает
// XAML: меню — ребёнок кнопки), открывается у курсора: Show получает точку в
// кадре меню, а рисуется оно через тот же сдвиг, что и кнопка.
func TestScrollViewCoords_AttachedContextMenu(t *testing.T) {
	env := newVScrollScene(t)
	pm := widget.NewPopupMenu()
	pm.SetItems([]widget.MenuItem{{Text: "Первый"}, {Text: "Второй"}, {Text: "Третий"}})
	env.c.btn.SetContextMenu(pm)
	env.c.btn.AddChild(pm)
	env.sv.SetScrollY(200)
	before := env.eng.RenderOnce()

	env.eng.SendMouseButton(100, 15, widget.MouseRight, true)
	env.eng.SendMouseButton(100, 15, widget.MouseRight, false)
	if !pm.IsOpen() {
		t.Fatalf("приколотое меню не открылось")
	}
	// В кадре кнопки меню лежит у точки (100, 15+200).
	if got, want := pm.OverlayBounds().Min, image.Pt(100, 215); got != want {
		t.Fatalf("меню открыто в %v, ждали %v (кадр содержимого)", got, want)
	}
	img := env.eng.RenderOnce()
	changed := 0
	for y := 18; y < 80; y++ {
		for x := 104; x < 150; x++ {
			if img.RGBAAt(x, y) != before.RGBAAt(x, y) {
				changed++
			}
		}
	}
	if changed < 100 {
		t.Fatalf("у курсора меню не нарисовано: изменилось %d точек", changed)
	}
}

// Полоса прокрутки ВНУТРИ другой прокрутки: щелчок по треку и перетаскивание
// ползунка идут в кадре внутренней прокрутки, а внешняя при этом стоит на месте.
func TestScrollViewCoords_NestedScrollbar(t *testing.T) {
	env := newNestedScene(t)

	// Внутренняя на экране в 0..100, полоса у правого края (x=270..280).
	// Щелчок в верхней части трека уводит содержимое к началу.
	env.eng.SendMouseButton(275, 5, widget.MouseLeft, true)
	env.eng.SendMouseButton(275, 5, widget.MouseLeft, false)
	if got := env.inner.ScrollY(); got >= 120 {
		t.Fatalf("щелчок по треку внутренней прокрутки не сдвинул её: %d", got)
	}
	if got := env.outer.ScrollY(); got != 300 {
		t.Fatalf("щелчок по треку внутренней сдвинул внешнюю: %d", got)
	}

	// Ползунок: вернём внутреннюю в конец и потянем вверх с захватом мыши.
	env.inner.SetScrollY(120)
	env.eng.RenderOnce()
	env.eng.SendMouseButton(275, 80, widget.MouseLeft, true) // ползунок — внизу трека
	env.eng.SendMouseMove(275, 40)
	env.eng.SendMouseButton(275, 40, widget.MouseLeft, false)
	if got := env.inner.ScrollY(); got >= 120 || got <= 0 {
		t.Fatalf("перетаскивание ползунка внутренней прокрутки дало %d, ждали значение между 0 и 120", got)
	}
	if got := env.outer.ScrollY(); got != 300 {
		t.Fatalf("перетаскивание ползунка внутренней сдвинуло внешнюю: %d", got)
	}
}

// Содержимое вкладки внутри прокрутки: контейнер держит содержимое мимо
// Base.AddChild, поэтому обязан сам объявить его своим ребёнком — иначе область
// перерисовки кнопки на вкладке осталась бы в координатах содержимого.
func TestScrollViewCoords_InvalidateInsideTab(t *testing.T) {
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 300, 500))
	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 300, 100))
	sv.ContentHeight = 400
	root.AddChild(sv)

	tc := widget.NewTabControl()
	tc.SetBounds(image.Rect(0, 200, 300, 300))
	page := widget.NewPanel(color.RGBA{R: 40, G: 40, B: 40, A: 255})
	page.ShowHeader = false
	btn := widget.NewButton("на вкладке")
	tc.AddTab("Вкладка", page)
	tc.SetBounds(image.Rect(0, 200, 300, 300)) // раскладывает содержимое вкладки
	pb := page.Bounds()
	btn.SetBounds(image.Rect(pb.Min.X+5, pb.Min.Y+5, pb.Min.X+150, pb.Min.Y+35))
	page.AddChild(btn)
	sv.AddChild(tc)

	eng := engine.New(300, 500, 30)
	eng.SetRoot(root)
	eng.SetRenderOnDemand(true)
	eng.RenderOnce()
	sv.SetScrollY(200)
	eng.RenderOnce()

	btn.SetHovered(true)
	partial := eng.RenderOnce()
	eng.Invalidate()
	full := eng.RenderOnce()
	if p, bad := firstDiff(partial, full); bad {
		t.Fatalf("частичная перерисовка разошлась с полной в точке %v: %v против %v",
			p, partial.RGBAAt(p.X, p.Y), full.RGBAAt(p.X, p.Y))
	}
}

// Прокрутка не «съедает» перерисовку соседей: изменение вне прокрутки по-прежнему
// перерисовывается ровно там, где оно произошло.
func TestScrollViewCoords_InvalidateOutsideScrollUnchanged(t *testing.T) {
	env := newVScrollScene(t)
	other := newSvKid("вне", image.Rect(10, 150, 200, 180)) // ниже прокрутки (0..100)
	env.root.AddChild(other.btn)
	env.eng.SetRenderOnDemand(true)
	env.sv.SetScrollY(200)
	env.eng.RenderOnce()

	other.btn.SetHovered(true)
	partial := env.eng.RenderOnce()
	env.eng.Invalidate()
	full := env.eng.RenderOnce()
	if p, bad := firstDiff(partial, full); bad {
		t.Fatalf("частичная перерисовка разошлась с полной в точке %v: %v против %v",
			p, partial.RGBAAt(p.X, p.Y), full.RGBAAt(p.X, p.Y))
	}
}
