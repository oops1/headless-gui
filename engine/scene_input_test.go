package engine

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// inputLog записывает события окна программы.
type inputLog struct {
	events []string
	focus  []bool
}

func (l *inputLog) input() widget.SurfaceInput {
	return widget.SurfaceInput{
		Move:  func(x, y int) { l.events = append(l.events, fmt.Sprintf("move %d,%d", x, y)) },
		Leave: func() { l.events = append(l.events, "leave") },
		Button: func(e widget.MouseEvent) bool {
			l.events = append(l.events, fmt.Sprintf("button %d,%d %v", e.X, e.Y, e.Pressed))
			return true
		},
		Key:   func(e widget.KeyEvent) { l.events = append(l.events, fmt.Sprintf("key %d", e.Code)) },
		Focus: func(on bool) { l.focus = append(l.focus, on) },
		Cursor: func(x, y int) widget.Cursor {
			return widget.CursorIBeam
		},
	}
}

func (l *inputLog) count(prefix string) int {
	n := 0
	for _, e := range l.events {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			n++
		}
	}
	return n
}

// sceneInput — сцена сессии: окно программы и панель задач поверх его низа.
type sceneInput struct {
	eng     *Engine
	windows *widget.Panel
	surf    *widget.ExternalSurface
	log     *inputLog
	bar     *widget.Panel
}

func newSceneInput(t *testing.T) *sceneInput {
	t.Helper()
	eng := New(300, 200, 30)
	t.Cleanup(eng.Stop)
	root := widget.NewPanel(color.RGBA{R: 30, G: 60, B: 90, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 300, 200))

	windows := widget.NewPanel(color.RGBA{})
	windows.ShowHeader = false
	windows.SetBounds(image.Rect(0, 0, 300, 200))
	root.AddChild(windows)

	surf := widget.NewExternalSurface()
	surf.SetBounds(image.Rect(20, 20, 220, 180))
	surf.SetBuffer(image.NewRGBA(image.Rect(0, 0, 200, 160)), true)
	log := &inputLog{}
	surf.SetInput(log.input())
	t.Cleanup(func() { surf.SetInput(widget.SurfaceInput{}) })
	windows.AddChild(surf)

	bar := widget.NewPanel(color.RGBA{R: 200, G: 200, B: 200, A: 255})
	bar.ShowHeader = false
	bar.SetBounds(image.Rect(0, 150, 300, 200))
	root.AddChild(bar)

	eng.SetRoot(root)
	eng.RenderOnce()
	return &sceneInput{eng: eng, windows: windows, surf: surf, log: log, bar: bar}
}

// Щелчок по окну программы: фокус ему, событие в его координатах.
func TestSceneInput_ClickFocusesSurface(t *testing.T) {
	testreset.Globals(t)
	sc := newSceneInput(t)
	sc.eng.SendMouseButton(60, 50, widget.MouseLeft, true)
	sc.eng.SendMouseButton(60, 50, widget.MouseLeft, false)
	if got := sc.log.events; len(got) < 2 || got[0] != "button 40,30 true" || got[len(got)-1] != "button 40,30 false" {
		t.Fatalf("события: %q", got)
	}
	if !sc.surf.IsFocused() || len(sc.log.focus) != 1 || !sc.log.focus[0] {
		t.Fatalf("фокус не пришёл: focused=%v, колбэки %v", sc.surf.IsFocused(), sc.log.focus)
	}
	sc.eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyA, Pressed: true})
	sc.eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true})
	if sc.log.count("key") != 2 {
		t.Fatalf("клавиши не дошли (Tab тоже должен): %q", sc.log.events)
	}
}

// Щелчок по панели поверх окна программы до программы не доходит.
func TestSceneInput_ChromeAboveSurfaceTakesClick(t *testing.T) {
	testreset.Globals(t)
	sc := newSceneInput(t)
	sc.eng.SendMouseButton(100, 170, widget.MouseLeft, true)
	sc.eng.SendMouseButton(100, 170, widget.MouseLeft, false)
	if n := sc.log.count("button"); n != 0 {
		t.Fatalf("щелчок по панели дошёл до окна программы: %q", sc.log.events)
	}
}

// Движение: над окном — Move; над панелью поверх окна — Leave, без Move;
// уход указателя с окна ОС — Leave.
func TestSceneInput_PointerOwnershipFollowsZ(t *testing.T) {
	testreset.Globals(t)
	sc := newSceneInput(t)
	sc.eng.SendMouseMove(60, 50)
	sc.eng.SendMouseMove(70, 60)
	sc.eng.SendMouseMove(100, 170) // над панелью, но в границах окна
	sc.eng.SendMouseMove(110, 175)
	want := []string{"move 40,30", "move 50,40", "leave"}
	if fmt.Sprint(sc.log.events) != fmt.Sprint(want) {
		t.Fatalf("события: %q, ждали %q", sc.log.events, want)
	}
	sc.eng.SendMouseMove(60, 50)
	sc.eng.SendPointerLeave()
	if got := sc.log.events[len(sc.log.events)-1]; got != "leave" {
		t.Fatalf("уход с окна не сообщён: %q", sc.log.events)
	}
	if c := sc.eng.CursorAt(60, 50); c != widget.CursorIBeam {
		t.Fatalf("курсор над окном программы: %v", c)
	}
}

// Перетаскивание из окна программы на панель держит захват: движение и
// отпускание приходят окну.
func TestSceneInput_DragKeepsCapture(t *testing.T) {
	testreset.Globals(t)
	sc := newSceneInput(t)
	sc.eng.SendMouseMove(60, 50)
	sc.eng.SendMouseButton(60, 50, widget.MouseLeft, true)
	sc.eng.SendMouseMove(100, 170)
	sc.eng.SendMouseMove(250, 190) // за границей окна
	sc.eng.SendMouseButton(250, 190, widget.MouseLeft, false)
	want := []string{"move 40,30", "button 40,30 true", "move 80,150", "move 230,170", "button 230,170 false"}
	if fmt.Sprint(sc.log.events) != fmt.Sprint(want) {
		t.Fatalf("события: %q, ждали %q", sc.log.events, want)
	}
	if sc.eng.getCaptured() != nil {
		t.Fatal("захват не снят после отпускания")
	}
}

// Поверхность без ввода ведёт себя как прежде: щелчок снимает фокус, Tab её
// обходит, владельцем указателя она не становится.
func TestSceneInput_NoInputKeepsOldBehaviour(t *testing.T) {
	testreset.Globals(t)
	sc := newSceneInput(t)
	sc.surf.SetInput(widget.SurfaceInput{})
	btn := widget.NewButton("ok")
	btn.SetBounds(image.Rect(240, 20, 290, 50))
	sc.windows.AddChild(btn)
	sc.eng.SetFocus(btn)
	sc.eng.SendMouseButton(60, 50, widget.MouseLeft, true)
	sc.eng.SendMouseButton(60, 50, widget.MouseLeft, false)
	if len(sc.log.events) != 0 {
		t.Fatalf("поверхность без ввода получила события: %q", sc.log.events)
	}
	if sc.surf.AcceptsTab() || sc.surf.TabIndex() >= 0 {
		t.Fatal("поверхность без ввода забирает Tab или стоит в обходе Tab")
	}
	for _, w := range widget.CollectFocusables(sc.eng.Root()) {
		if w == widget.Widget(sc.surf) {
			t.Fatal("поверхность попала в обход Tab")
		}
	}
}

// RaiseChild меняет, кто получает щелчок в перекрытии, и заявляет область
// поднятого окна.
func TestSceneInput_RaiseChildChangesHitAndDamage(t *testing.T) {
	testreset.Globals(t)
	sc := newSceneInput(t)
	top := widget.NewExternalSurface()
	top.SetBounds(image.Rect(150, 30, 280, 120))
	top.SetBuffer(image.NewRGBA(image.Rect(0, 0, 130, 90)), true)
	topLog := &inputLog{}
	top.SetInput(topLog.input())
	t.Cleanup(func() { top.SetInput(widget.SurfaceInput{}) })
	sc.windows.AddChild(top)
	sc.eng.RenderOnce()

	click := func() {
		sc.eng.SendMouseButton(170, 50, widget.MouseLeft, true)
		sc.eng.SendMouseButton(170, 50, widget.MouseLeft, false)
	}
	click()
	if topLog.count("button") != 2 || sc.log.count("button") != 0 {
		t.Fatalf("до подъёма щелчок не у верхнего: top %q, bottom %q", topLog.events, sc.log.events)
	}

	sc.eng.consumeDamage()
	if !sc.windows.RaiseChild(sc.surf) {
		t.Fatal("RaiseChild: окно не найдено")
	}
	rects, all := sc.eng.consumeDamage()
	covered := false
	for _, r := range rects {
		if r.Intersect(sc.surf.Bounds()) == sc.surf.Bounds() {
			covered = true
		}
	}
	if !all && !covered {
		t.Fatalf("подъём не заявил область окна: %v", rects)
	}
	click()
	if sc.log.count("button") != 2 {
		t.Fatalf("после подъёма щелчок не у поднятого: %q", sc.log.events)
	}
	if idx := sc.windows.ChildIndex(sc.surf); idx != len(sc.windows.Children())-1 {
		t.Fatalf("поднятое окно на позиции %d", idx)
	}
	if !sc.windows.LowerChild(sc.surf) || sc.windows.ChildIndex(sc.surf) != 0 {
		t.Fatal("LowerChild не опустил окно")
	}
}

// Canvas переставляет attached-свойства вместе с детьми.
func TestSceneInput_CanvasReorderKeepsProps(t *testing.T) {
	testreset.Globals(t)
	c := widget.NewCanvas()
	c.SetBounds(image.Rect(0, 0, 300, 200))
	a, b, d := widget.NewPanel(color.RGBA{}), widget.NewPanel(color.RGBA{}), widget.NewPanel(color.RGBA{})
	c.AddChildAt(a, widget.CanvasAttached{Left: 10, Top: 10, Right: -1, Bottom: -1}, 50, 50)
	c.AddChildAt(b, widget.CanvasAttached{Left: 100, Top: 10, Right: -1, Bottom: -1}, 50, 50)
	c.RaiseChild(a)
	if got := c.GetChildCanvasProps(c.ChildIndex(a)).Left; got != 10 {
		t.Fatalf("после RaiseChild у a Left=%d", got)
	}
	if got := c.GetChildCanvasProps(c.ChildIndex(b)).Left; got != 100 {
		t.Fatalf("после RaiseChild у b Left=%d", got)
	}
	c.InsertChildAt(0, d)
	if c.ChildIndex(d) != 0 || c.GetChildCanvasProps(c.ChildIndex(a)).Left != 10 {
		t.Fatal("InsertChildAt сдвинул свойства")
	}
	c.LowerChild(a)
	if c.ChildIndex(a) != 0 || c.GetChildCanvasProps(0).Left != 10 || c.GetChildCanvasProps(c.ChildIndex(b)).Left != 100 {
		t.Fatal("LowerChild сдвинул свойства")
	}
}
