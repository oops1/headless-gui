package widget

import (
	"image"
	"testing"
)

// GG-77: кнопки приложения в заголовке DockPane.

type paneFixture struct {
	p              *DockPane
	clicks, builds int
}

// Панель 300×200: штатные кнопки pin 239..257, приложения — «Вид» 215..233,
// «GitHub» 195..213; по вертикали 3..21.
func newPaneFixture(t *testing.T) *paneFixture {
	t.Helper()
	withScreen(t)
	f := &paneFixture{p: NewDockPane("branches", "Ветки", nil)}
	f.p.SetBounds(image.Rect(0, 0, 300, 200))
	f.p.SetTitleButtons([]DockPaneButton{
		{Tooltip: "GitHub", Icon: image.NewRGBA(image.Rect(0, 0, 16, 16)), OnClick: func() { f.clicks++ }},
		{Tooltip: "Вид", MenuFunc: func() []MenuItem {
			f.builds++
			return []MenuItem{{Text: "Сортировать"}}
		}},
	})
	return f
}

func center(r image.Rectangle) (int, int) {
	return r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2
}

// Кнопки стоят слева от штатных, последняя ближе всех к ним; скрытая места не
// занимает.
func TestDockPaneTitleButtons_Geometry(t *testing.T) {
	f := newPaneFixture(t)
	_, _, pinR := f.p.buttonRects()
	r := f.p.titleButtonRects()
	if r[1].Max.X+dockPaneAppGap != pinR.Min.X || r[0].Max.X >= r[1].Min.X {
		t.Fatalf("кнопки %v, штатная pin %v", r, pinR)
	}
	if r[0].Min.Y != pinR.Min.Y || r[0].Dy() != pinR.Dy() {
		t.Fatalf("кнопка %v не на одной линии со штатными %v", r[0], pinR)
	}
	if lim := f.p.titleTextLimit(); lim != r[0].Min.X {
		t.Fatalf("заголовок обрезается по %d, ждал %d", lim, r[0].Min.X)
	}

	f.p.SetTitleButtonHidden(1, true)
	h := f.p.titleButtonRects()
	if !h[1].Empty() || h[0] != r[1] {
		t.Fatalf("после скрытия второй кнопки: %v, ждал первую на месте второй %v", h, r[1])
	}
}

// Щелчок выполняет действие; нажатие с отпусканием мимо — нет; за кнопку
// панель не перетаскивается; подсказка своя у каждой кнопки.
func TestDockPaneTitleButtons_Click(t *testing.T) {
	f := newPaneFixture(t)
	r := f.p.titleButtonRects()
	x, y := center(r[0])

	if f.p.titleDragHit(x, y) {
		t.Fatal("нажатие на кнопку начало бы перетаскивание панели")
	}
	if !f.p.WantsCapture(MouseEvent{X: x, Y: y, Button: MouseLeft, Pressed: true}) {
		t.Fatal("кнопка не захватывает мышь на нажатии")
	}
	click(f.p, x, y)
	if f.clicks != 1 {
		t.Fatalf("щелчков %d, ждал 1", f.clicks)
	}
	press(f.p, x, y)
	release(f.p, 150, 100)
	if f.clicks != 1 {
		t.Fatal("отпускание мимо кнопки выполнило действие")
	}

	if got := f.p.ToolTipAt(x, y); got != "GitHub" {
		t.Fatalf("подсказка первой кнопки %q", got)
	}
	if got := f.p.ToolTipAt(center(r[1])); got != "Вид" {
		t.Fatalf("подсказка второй кнопки %q", got)
	}
	if got := f.p.ToolTipAt(20, 12); got != "" {
		t.Fatalf("подсказка над заголовком %q, ждал пустую", got)
	}

	f.p.SetTitleButtonDisabled(0, true)
	click(f.p, x, y)
	if f.clicks != 1 {
		t.Fatal("выключенная кнопка выполнила действие")
	}
}

// Кнопка с меню: меню собирается при каждом открытии и встаёт под кнопкой;
// повторный щелчок его закрывает; Dismiss — тоже.
func TestDockPaneTitleButtons_Menu(t *testing.T) {
	f := newPaneFixture(t)
	r := f.p.titleButtonRects()
	x, y := center(r[1])

	click(f.p, x, y)
	if !f.p.HasOverlay() || f.builds != 1 {
		t.Fatalf("меню: открыто %v, собрано %d раз", f.p.HasOverlay(), f.builds)
	}
	if got := f.p.OverlayBounds().Min; got != image.Pt(r[1].Min.X, r[1].Max.Y) {
		t.Fatalf("меню в %v, ждал под кнопкой %v", got, image.Pt(r[1].Min.X, r[1].Max.Y))
	}

	click(f.p, x, y)
	if f.p.HasOverlay() || f.builds != 1 {
		t.Fatalf("повторный щелчок: открыто %v, собрано %d раз", f.p.HasOverlay(), f.builds)
	}

	click(f.p, x, y)
	if f.builds != 2 {
		t.Fatalf("меню при повторном открытии собрано %d раз, ждал 2", f.builds)
	}
	f.p.Dismiss()
	if f.p.HasOverlay() {
		t.Fatal("Dismiss не закрыл меню")
	}
}
