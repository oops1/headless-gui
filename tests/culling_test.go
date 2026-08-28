package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Пропуск поддеревьев вне изменившейся области.
//
// Раньше кадр обходил дерево целиком: тикающие часы на панели раз в минуту
// стоили полного обхода ради двух тайлов. Теперь ветка, не задевающая ни
// одной изменившейся области, не рисуется вовсе.

// countingPanel — панель, считающая, сколько раз её нарисовали.
type countingPanel struct {
	*widget.Panel
	draws int
}

func newCountingPanel(bg color.RGBA, r image.Rectangle) *countingPanel {
	p := widget.NewPanel(bg)
	p.ShowHeader = false
	p.SetBounds(r)
	return &countingPanel{Panel: p}
}

func (c *countingPanel) Draw(ctx widget.DrawContext) {
	c.draws++
	c.Panel.Draw(ctx)
}

// cullScene — экран с двумя далёкими друг от друга панелями.
func cullScene(t *testing.T) (*engine.Engine, *countingPanel, *countingPanel) {
	t.Helper()

	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 800, 600))

	near := newCountingPanel(color.RGBA{R: 200, A: 255}, image.Rect(10, 10, 110, 110))
	far := newCountingPanel(color.RGBA{B: 200, A: 255}, image.Rect(600, 450, 780, 580))
	root.AddChild(near)
	root.AddChild(far)

	eng := engine.New(800, 600, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce() // первый кадр рисует всё
	return eng, near, far
}

func TestCulling_SkipsUntouchedSubtree(t *testing.T) {
	eng, near, far := cullScene(t)
	beforeNear, beforeFar := near.draws, far.draws

	// Изменилась только левая верхняя панель — заявляем её область.
	eng.InvalidateRect(image.Rect(10, 10, 110, 110))
	eng.RenderOnce()

	if near.draws == beforeNear {
		t.Error("панель внутри изменившейся области не нарисована")
	}
	if far.draws != beforeFar {
		t.Errorf("панель вдали от изменений нарисована %d раз — поддерево не пропущено",
			far.draws-beforeFar)
	}
}

// Выключатель возвращает прежнее поведение: рисуются все.
func TestCulling_SwitchRestoresFullWalk(t *testing.T) {
	eng, _, far := cullScene(t)
	eng.SetSubtreeCulling(false)
	t.Cleanup(func() { eng.SetSubtreeCulling(true) })

	before := far.draws
	eng.InvalidateRect(image.Rect(10, 10, 110, 110))
	eng.RenderOnce()

	if far.draws == before {
		t.Error("с выключенным пропуском дальняя панель всё равно не нарисована")
	}
}

// Полный кадр рисует всё: пропуск работает только там, где заявлены области.
func TestCulling_FullFrameDrawsEverything(t *testing.T) {
	eng, near, far := cullScene(t)
	bn, bf := near.draws, far.draws

	eng.Invalidate() // «изменилось всё»
	eng.RenderOnce()

	if near.draws == bn || far.draws == bf {
		t.Error("полный кадр нарисовал не всё дерево")
	}
}

// Виджет, рисующий за своими границами, не должен пропускаться, когда
// изменилась область его тени.
func TestCulling_RespectsDrawMargin(t *testing.T) {
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 300))

	shadowed := &marginPanel{countingPanel: newCountingPanel(color.RGBA{G: 200, A: 255},
		image.Rect(100, 100, 200, 200)), margin: 30}
	root.AddChild(shadowed)

	eng := engine.New(400, 300, 60)
	eng.SetRenderOnDemand(true)
	eng.SetRoot(root)
	eng.RenderOnce()

	before := shadowed.draws
	// Область ЗА границами виджета, но внутри его поля на тень.
	eng.InvalidateRect(image.Rect(80, 80, 95, 95))
	eng.RenderOnce()

	if shadowed.draws == before {
		t.Error("виджет с полем на тень пропущен — тень не перерисуется")
	}
}

// marginPanel — панель, объявляющая поле для отрисовки за своими границами.
type marginPanel struct {
	*countingPanel
	margin int
}

func (m *marginPanel) DrawMargin() int { return m.margin }

// Пропуск не меняет картинку — главное требование работы.
//
// Сравниваются кадры одной сцены при включённом и выключенном пропуске:
// движок волен не звать Draw, но результат обязан быть тем же до пикселя.
func TestCulling_FrameIsIdentical(t *testing.T) {
	render := func(culling bool) *image.RGBA {
		root := widget.NewPanel(color.RGBA{R: 30, G: 40, B: 60, A: 255})
		root.ShowHeader = false
		root.SetBounds(image.Rect(0, 0, 400, 300))
		for i := 0; i < 6; i++ {
			p := widget.NewPanel(color.RGBA{R: uint8(40 * i), G: 120, B: 200, A: 255})
			p.ShowHeader = false
			p.SetBounds(image.Rect(20+i*60, 40, 70+i*60, 160))
			root.AddChild(p)
		}
		btn := widget.NewButton("Кнопка")
		btn.SetBounds(image.Rect(40, 200, 200, 240))
		root.AddChild(btn)

		eng := engine.New(400, 300, 60)
		eng.SetRenderOnDemand(true)
		eng.SetSubtreeCulling(culling)
		eng.SetRoot(root)
		eng.RenderOnce()

		// Точечное изменение: именно здесь пути расходятся.
		eng.InvalidateRect(image.Rect(40, 200, 200, 240))
		return eng.RenderOnce()
	}

	// Возвращать выключатель в норму между вызовами не нужно: он живёт на
	// движке, а движок здесь каждый раз новый.
	withCulling := render(true)
	without := render(false)

	if withCulling == nil || without == nil {
		t.Fatal("кадр не отрисован")
	}
	if withCulling.Bounds() != without.Bounds() {
		t.Fatalf("размеры кадров разошлись: %v и %v", withCulling.Bounds(), without.Bounds())
	}
	for i := range withCulling.Pix {
		if withCulling.Pix[i] != without.Pix[i] {
			y := i / withCulling.Stride
			x := (i % withCulling.Stride) / 4
			t.Fatalf("кадры разошлись в точке (%d,%d): %d против %d",
				x, y, withCulling.Pix[i], without.Pix[i])
		}
	}
}
