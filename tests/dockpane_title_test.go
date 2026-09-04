package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Цвет заголовка активной панели дока — запрос GG-30.
//
// Фонов у титлбара два (TitleBG и TitleActiveBG), а цвет текста был один: у
// активной панели он оказывался подобран не под тот фон, поверх которого
// нарисован. Заметно там, где цвет текста вычисляется из фона.

func titleScene(t *testing.T) (*engine.Engine, *widget.DockManager, *widget.DockPane, *widget.DockPane) {
	t.Helper()
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 24, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 500, 300))

	m := widget.NewDockManager()
	m.SetBounds(image.Rect(0, 0, 500, 300))
	root.AddChild(m)

	a := widget.NewDockPane("a", "Репозитории", nil)
	b := widget.NewDockPane("b", "Ветки", nil)
	m.AddPane(a, widget.DockLeft)
	m.AddPane(b, widget.DockLeft)
	m.SetSideSize(widget.DockLeft, 200)
	m.SetBounds(image.Rect(0, 0, 500, 300))

	eng := engine.New(500, 300, 30)
	eng.SetRoot(root)
	eng.RenderOnce()
	return eng, m, a, b
}

// Активность панели видна снаружи.
func TestDockPane_IsActiveIsExported(t *testing.T) {
	_, _, a, b := titleScene(t)

	if !a.IsActive() && !b.IsActive() {
		t.Fatal("ни одна панель стопки не активна")
	}
	if a.IsActive() && b.IsActive() {
		t.Error("активны обе панели стопки сразу")
	}
}

// Заголовок активной панели можно покрасить отдельно.
func TestDockPane_TitleTextActive(t *testing.T) {
	eng, _, a, b := titleScene(t)

	active := a
	if !a.IsActive() {
		active = b
	}

	before := snapshotRGBA(eng.RenderOnce())
	active.TitleTextActive = color.RGBA{R: 255, G: 0, B: 0, A: 255}
	eng.Invalidate()
	after := snapshotRGBA(eng.RenderOnce())

	tb := active.Bounds()
	tb.Max.Y = tb.Min.Y + 24
	// Сравниваются ВСЕ три канала: у активной панели заголовок белый, и
	// красный от него отличается только зелёным и синим.
	changed := 0
	for y := tb.Min.Y; y < tb.Max.Y; y++ {
		for x := tb.Min.X; x < tb.Max.X; x++ {
			i := before.PixOffset(x, y)
			if before.Pix[i] != after.Pix[i] ||
				before.Pix[i+1] != after.Pix[i+1] ||
				before.Pix[i+2] != after.Pix[i+2] {
				changed++
			}
		}
	}
	if changed == 0 {
		t.Error("TitleTextActive не изменил заголовок активной панели")
	}

	// Нулевая альфа означает «как у неактивной»: прежнее поведение сохранено.
	active.TitleTextActive = color.RGBA{}
	eng.Invalidate()
	back := snapshotRGBA(eng.RenderOnce())
	for i := range before.Pix {
		if before.Pix[i] != back.Pix[i] {
			t.Error("сброс TitleTextActive не вернул прежний вид заголовка")
			break
		}
	}
}

// О смене активной панели сообщают: без этого цвет, вычисленный из фона,
// пересчитать не по чему.
func TestDockPane_ActiveChangeIsReported(t *testing.T) {
	_, m, a, b := titleScene(t)

	seen := map[string]int{}
	a.OnStateChanged = func(p *widget.DockPane) { seen["a"]++ }
	b.OnStateChanged = func(p *widget.DockPane) { seen["b"]++ }

	other := b
	if b.IsActive() {
		other = a
	}
	m.ActivatePane(other)

	if seen["a"] == 0 || seen["b"] == 0 {
		t.Errorf("о смене активной панели сообщили как %v — ждали обе панели", seen)
	}
	if !other.IsActive() {
		t.Error("панель не стала активной после ActivatePane")
	}
}
