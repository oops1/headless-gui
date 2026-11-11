package tests

import (
	"image"
	"image/color"
	"testing"
	"testing/fstest"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// GG-95: сторону DiffView можно свернуть в узкую полосу, вторая занимает всю
// ширину; граница между развёрнутыми сторонами тянется мышью.

const dcW, dcH = 900, 320

func diffCollapseScene(t *testing.T) (*engine.Engine, *widget.DiffView) {
	t.Helper()
	dv := widget.NewDiffView("", "")
	dv.SetText(widget.DiffLeft, "old.go", "HEAD", "a\nb\nc\nd\ne\n")
	dv.SetText(widget.DiffRight, "new.go", "рабочая копия", "a\nB\nc\nd\nE\n")
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, dcW, dcH))
	dv.SetBounds(image.Rect(0, 0, dcW, dcH))
	root.AddChild(dv)
	eng := engine.New(dcW, dcH, 20)
	eng.SetRoot(root)
	return eng, dv
}

// diffPanes — области сторон из дерева доступности: [левая, правая].
func diffPanes(t *testing.T, dv *widget.DiffView) [2]image.Rectangle {
	t.Helper()
	var out [2]image.Rectangle
	n := 0
	for _, c := range widget.BuildAccessTree(dv, nil).Children {
		if c.Role == widget.RoleTextInput && n < 2 {
			out[n] = c.Bounds
			n++
		}
	}
	if n != 2 {
		t.Fatalf("в дереве доступности %d панелей сторон, ждал 2", n)
	}
	return out
}

func diffClick(dv *widget.DiffView, x, y int) {
	dv.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	dv.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: false})
}

type collapseEv struct {
	side widget.DiffSide
	v    bool
}

func TestDiffView_CollapseSide(t *testing.T) {
	testreset.Globals(t)
	eng, dv := diffCollapseScene(t)
	if dv.Collapsed(widget.DiffLeft) || dv.Collapsed(widget.DiffRight) || dv.Split() != 0.5 || !dv.ShowCollapseButtons() {
		t.Fatalf("по умолчанию: свёрнуты %v/%v, доля %v, кнопки %v",
			dv.Collapsed(widget.DiffLeft), dv.Collapsed(widget.DiffRight), dv.Split(), dv.ShowCollapseButtons())
	}
	half := diffPanes(t, dv)
	before := eng.RenderOnce()

	var evs []collapseEv
	dv.OnCollapsedChanged = func(s widget.DiffSide, v bool) { evs = append(evs, collapseEv{s, v}) }
	var active []widget.DiffSide
	dv.OnActiveSideChanged = func(s widget.DiffSide) { active = append(active, s) }

	dv.SetActiveSide(widget.DiffLeft)
	dv.SetCollapsed(widget.DiffLeft, true)
	if !dv.Collapsed(widget.DiffLeft) || len(evs) != 1 || evs[0] != (collapseEv{widget.DiffLeft, true}) {
		t.Fatalf("SetCollapsed(Left): свёрнута %v, события %v", dv.Collapsed(widget.DiffLeft), evs)
	}
	if dv.ActiveSide() != widget.DiffRight || len(active) == 0 || active[len(active)-1] != widget.DiffRight {
		t.Fatalf("свернули активную сторону: активна %v, события %v", dv.ActiveSide(), active)
	}
	p := diffPanes(t, dv)
	if p[0].Dx() > 40 {
		t.Fatalf("свёрнутая сторона шириной %d, ждал полосу", p[0].Dx())
	}
	if p[1].Dx() < half[1].Dx()*2-40 {
		t.Fatalf("развёрнутая сторона %d, при двух сторонах было %d — ширину не заняла", p[1].Dx(), half[1].Dx())
	}
	after := eng.RenderOnce()
	if sameStrip(before, after, image.Rect(0, 0, dcW, dcH)) {
		t.Fatal("кадр со свёрнутой стороной не отличается")
	}

	// Свернуть вторую — первая разворачивается.
	evs = nil
	dv.SetCollapsed(widget.DiffRight, true)
	if dv.Collapsed(widget.DiffLeft) || !dv.Collapsed(widget.DiffRight) || len(evs) != 2 {
		t.Fatalf("свернули правую: левая %v, правая %v, события %v",
			dv.Collapsed(widget.DiffLeft), dv.Collapsed(widget.DiffRight), evs)
	}
	if dv.ActiveSide() != widget.DiffLeft {
		t.Fatalf("активна %v, ждал видимую левую", dv.ActiveSide())
	}

	// Активировать свёрнутую из кода — она разворачивается.
	dv.SetActiveSide(widget.DiffRight)
	if dv.Collapsed(widget.DiffRight) {
		t.Fatal("SetActiveSide на свёрнутую сторону её не развернул")
	}
	p = diffPanes(t, dv)
	if p != half {
		t.Fatalf("после разворачивания стороны %v, было %v", p, half)
	}
	if after := eng.RenderOnce(); !sameStrip(before, after, image.Rect(0, 0, dcW, dcH)) {
		t.Fatal("после разворачивания кадр не вернулся к прежнему")
	}
}

// Переход по изменениям и перенос блока из кода работают при свёрнутой стороне.
func TestDiffView_CollapsedStillWorks(t *testing.T) {
	testreset.Globals(t)
	eng, dv := diffCollapseScene(t)
	dv.SetCollapsed(widget.DiffLeft, true)
	eng.RenderOnce()
	dv.NextChange()
	if dv.CurrentChange() != 0 {
		t.Fatalf("NextChange: текущее %d", dv.CurrentChange())
	}
	dv.CopyBlock(0, true)
	if got := dv.Lines(widget.DiffRight)[1]; got != "b" {
		t.Fatalf("CopyBlock при свёрнутой стороне: справа %q", got)
	}
	// Мышь над бывшим промежутком не находит кнопок переноса и ничего не ломает.
	for x := 0; x < dcW; x += 7 {
		dv.OnMouseMove(x, 120)
	}
	eng.RenderOnce()
}

// Кнопка «свернуть» в шапке и щелчок по полосе.
func TestDiffView_CollapseByMouse(t *testing.T) {
	testreset.Globals(t)
	eng, dv := diffCollapseScene(t)
	eng.RenderOnce()
	half := diffPanes(t, dv)
	// Кнопка — у правого края шапки левой стороны.
	diffClick(dv, half[0].Max.X-20, 25)
	if !dv.Collapsed(widget.DiffLeft) {
		t.Fatal("щелчок по кнопке «свернуть» не свернул левую сторону")
	}
	p := diffPanes(t, dv)
	diffClick(dv, (p[0].Min.X+p[0].Max.X)/2, 150)
	if dv.Collapsed(widget.DiffLeft) {
		t.Fatal("щелчок по полосе не развернул сторону")
	}

	dv.SetShowCollapseButtons(false)
	diffClick(dv, half[0].Max.X-20, 25)
	if dv.Collapsed(widget.DiffLeft) {
		t.Fatal("кнопки скрыты, а щелчок по их месту свернул сторону")
	}
}

func TestDiffView_Split(t *testing.T) {
	testreset.Globals(t)
	eng, dv := diffCollapseScene(t)
	eng.RenderOnce()
	half := diffPanes(t, dv)
	var got []float64
	dv.OnSplitChanged = func(v float64) { got = append(got, v) }

	dv.SetSplit(0.7)
	p := diffPanes(t, dv)
	if dv.Split() != 0.7 || p[0].Dx() <= p[1].Dx() || len(got) != 1 || got[0] != 0.7 {
		t.Fatalf("SetSplit(0.7): доля %v, стороны %d/%d, события %v", dv.Split(), p[0].Dx(), p[1].Dx(), got)
	}
	dv.SetSplit(5)
	if dv.Split() != 0.85 {
		t.Fatalf("SetSplit(5) не зажался: %v", dv.Split())
	}
	dv.SetSplit(0.5)
	if p := diffPanes(t, dv); p != half {
		t.Fatalf("доля 0.5 даёт стороны %v, по умолчанию было %v", p, half)
	}

	// Перетаскивание границы за промежуток: одно событие на отпускание.
	got = nil
	gx := (half[0].Max.X + half[1].Min.X) / 2
	dv.OnMouseButton(widget.MouseEvent{X: gx, Y: 150, Button: widget.MouseLeft, Pressed: true})
	if c := dv.Cursor(gx, 150); c != widget.CursorSizeWE {
		t.Fatalf("курсор над границей %v, ждал ↔", c)
	}
	for x := gx; x <= gx+120; x += 20 {
		dv.OnMouseMove(x, 150)
	}
	if len(got) != 0 {
		t.Fatalf("OnSplitChanged пришло во время перетаскивания: %v", got)
	}
	dv.OnMouseButton(widget.MouseEvent{X: gx + 120, Y: 150, Button: widget.MouseLeft, Pressed: false})
	if len(got) != 1 || dv.Split() <= 0.55 {
		t.Fatalf("после перетаскивания на 120 px доля %v, события %v", dv.Split(), got)
	}
	p = diffPanes(t, dv)
	if d := p[0].Max.X - half[0].Max.X; d < 110 || d > 130 {
		t.Fatalf("граница сдвинулась на %d px, ждал около 120", d)
	}

	// Двойной щелчок по границе возвращает 50/50.
	gx = (p[0].Max.X + p[1].Min.X) / 2
	diffClick(dv, gx, 150)
	diffClick(dv, gx, 150)
	if dv.Split() != 0.5 {
		t.Fatalf("двойной щелчок по границе: доля %v", dv.Split())
	}
}

func TestDiffView_CollapseXAML(t *testing.T) {
	testreset.Globals(t)
	_, reg, err := widget.LoadUIFromXAMLFS([]byte(`<Canvas Width="400" Height="300">
  <DiffView x:Name="d" CollapsedSide="Right" ShowCollapseButtons="False" Split="0.6"/>
</Canvas>`), fstest.MapFS{})
	if err != nil {
		t.Fatalf("разметка: %v", err)
	}
	dv := reg["d"].(*widget.DiffView)
	if !dv.Collapsed(widget.DiffRight) || dv.ShowCollapseButtons() || dv.Split() != 0.6 {
		t.Fatalf("атрибуты не применились: правая свёрнута %v, кнопки %v, доля %v",
			dv.Collapsed(widget.DiffRight), dv.ShowCollapseButtons(), dv.Split())
	}
}
