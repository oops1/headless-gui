package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// Непрозрачные части меню для WinLine: оболочка кладёт картинку меню поверх
// окон чужих программ ровно в эти прямоугольники. Охватывающий OverlayBounds
// у меню с подменю брал пустое место между уровнями, у скруглённого меню —
// углы, и поверх окна Блокнота проступали обои.
//
// Проверка без допущений о том, как меню нарисовано: кадр снимается дважды,
// на двух разных фонах. Пиксель, который хоть как-то зависит от того, что под
// меню, — фон в щели между уровнями, сглаженный край угла, тень, — в двух
// кадрах разный. Внутри OverlayRects таких быть не должно ни одного.

func cascadeMenuScene(t *testing.T, scale float64, radius int) (*Engine, *widget.Panel, *widget.PopupMenu) {
	t.Helper()
	e := New(640, 420, 60)
	t.Cleanup(e.Stop)
	if scale != 1 {
		e.SetScale(scale)
	}
	root := widget.NewPanel(color.RGBA{R: 255, G: 0, B: 255, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 640, 420))

	m := widget.NewPopupMenu()
	m.CornerRadius = radius
	m.SubMenuDelay = 0
	// Непрозрачный фон: у темы по умолчанию фон меню может быть
	// полупрозрачным (материал), и тогда тело честно зависит от того, что под
	// ним, — это не та зависимость, которую ищет тест.
	m.Background = color.RGBA{R: 40, G: 44, B: 52, A: 255}
	m.BorderColor = color.RGBA{R: 90, G: 90, B: 100, A: 255}
	m.SetItems([]widget.MenuItem{
		{Text: "Вид"},
		{Text: "Персонализация", SubItems: []widget.MenuItem{
			{Text: "Фон"}, {Text: "Цвета"}, {Text: "Темы"},
			{Text: "Экран блокировки"}, {Text: "Пуск"}, {Text: "Панель задач"},
		}},
		{Text: "Обновить"},
	})
	root.AddChild(m)
	e.SetRoot(root)
	m.Show(60, 60)

	// Наведение на «Персонализацию» раскрывает подменю вправо.
	body := m.OverlayBounds()
	m.OnMouseMove(body.Min.X+20, body.Min.Y+m.ItemHeight+m.ItemHeight/2+4)
	if got := m.OverlayRects(); len(got) == 0 {
		t.Fatal("у открытого меню нет непрозрачных частей")
	}
	if m.OverlayBounds() == body {
		t.Fatal("подменю не раскрылось — сцена не воспроизвела случай")
	}
	return e, root, m
}

func frameOn(e *Engine, root *widget.Panel, bg color.RGBA) *image.RGBA {
	root.Background = bg
	e.Invalidate()
	img := e.RenderOnce()
	out := image.NewRGBA(img.Rect)
	copy(out.Pix, img.Pix)
	return out
}

func TestPopupMenu_OverlayRectsAreOpaque(t *testing.T) {
	testreset.Globals(t)
	for _, tc := range []struct {
		scale  float64
		radius int
	}{{1, 8}, {2, 8}, {1, 4}, {1, 0}, {1.5, 8}} {
		e, root, m := cascadeMenuScene(t, tc.scale, tc.radius)
		a := frameOn(e, root, color.RGBA{R: 255, G: 0, B: 255, A: 255})
		b := frameOn(e, root, color.RGBA{R: 0, G: 200, B: 0, A: 255})

		phys := func(lr image.Rectangle) image.Rectangle {
			k := tc.scale
			return image.Rect(int(float64(lr.Min.X)*k), int(float64(lr.Min.Y)*k),
				int(float64(lr.Max.X)*k+0.999), int(float64(lr.Max.Y)*k+0.999)).Intersect(a.Rect)
		}
		same := func(x, y int) bool {
			o := a.PixOffset(x, y)
			return a.Pix[o] == b.Pix[o] && a.Pix[o+1] == b.Pix[o+1] && a.Pix[o+2] == b.Pix[o+2]
		}
		count := func(pr image.Rectangle, want bool) (n int) {
			for y := pr.Min.Y; y < pr.Max.Y; y++ {
				for x := pr.Min.X; x < pr.Max.X; x++ {
					if same(x, y) == want {
						n++
					}
				}
			}
			return n
		}

		rects := m.OverlayRects()
		area := 0
		for _, r := range rects {
			if n := count(phys(r), false); n > 0 {
				t.Errorf("масштаб %v, радиус %d: в %v %d точек зависят от фона под меню", tc.scale, tc.radius, r, n)
			}
			area += phys(r).Dx() * phys(r).Dy()
		}
		all := phys(m.OverlayBounds())
		// Сцена значима: охватывающий прямоугольник фон захватывает.
		if count(all, false) == 0 {
			t.Errorf("масштаб %v: в OverlayBounds нет точек фона — сцена ничего не проверяет", tc.scale)
		}
		// И прямоугольники покрывают почти всё тело меню (точки охвата, не
		// зависящие от фона), а не отделываются малым.
		if body := count(all, true); area*100 < body*97 {
			t.Errorf("масштаб %v, радиус %d: покрыто %d из %d точек тела", tc.scale, tc.radius, area, body)
		}
		if len(rects) < 2 {
			t.Errorf("прямоугольников %d — по меньшей мере по одному на уровень", len(rects))
		}
	}
}

func TestPopupMenu_OverlayRectsClosedAndLevels(t *testing.T) {
	testreset.Globals(t)
	_, _, m := cascadeMenuScene(t, 1, 8)
	// Каждый прямоугольник лежит внутри охвата и не выходит за него.
	all := m.OverlayBounds()
	for _, r := range m.OverlayRects() {
		if !r.In(all) {
			t.Errorf("%v вне OverlayBounds %v", r, all)
		}
	}
	m.Close()
	if got := m.OverlayRects(); got != nil {
		t.Errorf("у закрытого меню части %v", got)
	}
	var _ widget.OverlayRectsProvider = m
}
