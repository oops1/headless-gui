package tests

// Горизонтальная прокрутка ScrollView через настоящий движок: hit-test,
// захват мыши, колесо с модификаторами. Модульные проверки геометрии и
// отрисовки лежат в widget/scrollview_hscroll_test.go.

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// hsEngineView — ScrollView 300×200 на весь движок, содержимое шире (900) и
// выше (500) области; ребёнок-кнопка залезает на нижнюю полосу, как это бывает
// с детьми, чьи bounds не сдвигаются прокруткой.
func hsEngineView() (*engine.Engine, *widget.ScrollView, *int) {
	clicks := new(int)
	btn := widget.NewButton("широкая кнопка")
	btn.SetBounds(image.Rect(0, 150, 300, 200))
	btn.OnClick = func() { *clicks++ }

	sv := widget.NewScrollView()
	sv.SetBounds(image.Rect(0, 0, 300, 200))
	sv.ContentWidth = 900
	sv.ContentHeight = 500
	sv.AddChild(btn)

	eng := engine.New(300, 200, 60)
	eng.SetRoot(sv)
	return eng, sv, clicks
}

// Щелчок по горизонтальной полосе не доходит до ребёнка под ней, а двигает
// содержимое; ведение с зажатой кнопкой работает и за границей виджета.
func TestScrollViewHScroll_BarClickAndDragViaEngine(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	eng, sv, clicks := hsEngineView()

	// Полоса — нижние 10 px левее вертикальной (x < 290). Щелчок мимо
	// ползунка (он у левого края) — прыжок.
	eng.SendMouseButton(200, 195, widget.MouseLeft, true)
	jumped := sv.ScrollX()
	if jumped == 0 {
		t.Fatal("щелчок по полосе не сдвинул содержимое")
	}
	// Ведём курсор вправо и даже ниже виджета — захват не должен оборваться.
	eng.SendMouseMove(280, 400)
	if got := sv.ScrollX(); got <= jumped {
		t.Errorf("перетаскивание не продолжилось за границей виджета: %d → %d", jumped, got)
	}
	eng.SendMouseButton(280, 400, widget.MouseLeft, false)

	if *clicks != 0 {
		t.Errorf("щелчок по полосе дошёл до кнопки под ней: %d кликов", *clicks)
	}
	if sv.ScrollY() != 0 {
		t.Errorf("горизонтальная полоса сдвинула ScrollY=%d", sv.ScrollY())
	}

	// Контроль: вне полосы кнопка по-прежнему кликается.
	eng.SendMouseButton(100, 170, widget.MouseLeft, true)
	eng.SendMouseButton(100, 170, widget.MouseLeft, false)
	if *clicks != 1 {
		t.Errorf("кнопка вне полосы не получила клик: %d", *clicks)
	}
}

// Движок доставляет Shift+колесо через wheelPixelModHandler.
func TestScrollViewHScroll_ShiftWheelViaEngine(t *testing.T) {
	widget.StopAllAnimations()
	defer widget.StopAllAnimations()

	eng, sv, _ := hsEngineView()

	eng.SetModifiers(widget.ModShift)
	eng.SendMouseWheelPixels(150, 100, 0, 50)
	eng.SetModifiers(0)
	if got := sv.ScrollX(); got != 50 {
		t.Errorf("Shift+колесо через движок: ScrollX=%d, ждал 50", got)
	}
	if got := sv.ScrollY(); got != 0 {
		t.Errorf("Shift+колесо сдвинуло ScrollY=%d", got)
	}

	// Горизонтальная составляющая без модификаторов.
	eng.SendMouseWheelPixels(150, 100, 30, 0)
	if got := sv.ScrollX(); got != 80 {
		t.Errorf("dx=30 через движок: ScrollX=%d, ждал 80", got)
	}
}
