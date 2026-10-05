package desktop

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// После анимации закрытия на месте панели не должно остаться следа: последний
// шаг обязан заявить область, где панель стояла в покое, а не пустой
// прямоугольник. Кадр собирается по повреждениям (RenderOnDemand), как у
// потребителя, который отдаёт только изменившееся.
func TestFlyout_CloseLeavesNoTrace(t *testing.T) {
	testCloseLeavesNoTrace(t, false)
}

// То же для панели, у которой размер зависит от содержимого: когда она ушла
// совсем, содержимое освобождено и Size() отдаёт ноль. На последнем шаге
// restRect() пуст, и без запомненного при Close места область не заявлялась.
func TestFlyout_CloseLeavesNoTraceWhenSizeCollapses(t *testing.T) {
	testCloseLeavesNoTrace(t, true)
}

func testCloseLeavesNoTrace(t *testing.T, collapse bool) {
	defer widget.StopAllAnimations()
	const w, h = 600, 400

	tm := managerFor(t, theme.ProfileWindows10)
	root := widget.NewPanel(theme.RGB(200, 30, 30))
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))

	f := NewFlyout(tm, ComponentStartMenu)
	f.Size = func() image.Point {
		if collapse && !f.IsOpen() && f.Presence() == 0 {
			return image.Point{}
		}
		return image.Pt(300, 200)
	}
	f.Screen = image.Rect(0, 0, w, h)
	f.SetBounds(image.Rect(0, h-40, 48, h))
	root.AddChild(f)

	eng := engine.New(w, h, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	bg := eng.RenderOnce().RGBAAt(150, 100)
	eng.RenderOnce()

	f.Open(image.Rect(0, h-40, 48, h))
	t0 := time.Now()
	widget.StepAnimations(t0)
	eng.RenderOnce()
	widget.StepAnimations(t0.Add(time.Second))
	rest := f.restRect()
	probe := image.Pt(rest.Min.X+150, rest.Min.Y+100)
	if eng.RenderOnce().RGBAAt(probe.X, probe.Y) == bg {
		t.Fatal("панель не нарисована — тест не различает состояния")
	}

	// Заявленные области записываются: последний шаг обязан накрыть rest.
	var rects []image.Rectangle
	widget.SetUIRectChangeNotifier(func(r image.Rectangle) { rects = append(rects, r) })
	defer widget.SetUIRectChangeNotifier(nil)

	f.Close()
	t1 := t0.Add(2 * time.Second)
	widget.StepAnimations(t1)
	eng.RenderOnce()
	widget.StepAnimations(t1.Add(60 * time.Millisecond))
	eng.RenderOnce()
	// Последний шаг: присутствие 0. Заявленное на нём отделяется от прежнего —
	// иначе область с предыдущего шага скрыла бы пустую заявку последнего.
	before := len(rects)
	widget.StepAnimations(t1.Add(time.Second))
	img := eng.RenderOnce()
	if f.Presence() != 0 {
		t.Fatalf("присутствие %v, ждали 0", f.Presence())
	}
	covered := false
	for _, r := range rects[before:] {
		if rest.In(r) {
			covered = true
		}
	}
	if !covered {
		t.Errorf("последний шаг заявил %v, ждали область, накрывающую место в покое %v", rects[before:], rest)
	}
	for _, p := range []image.Point{probe, rest.Min, rest.Max.Sub(image.Pt(1, 1)),
		{X: rest.Min.X + 5, Y: rest.Max.Y - 5}} {
		if got := img.RGBAAt(p.X, p.Y); got != bg {
			t.Errorf("после закрытия в %v остался след панели: %v, фон %v", p, got, bg)
		}
	}
}
