package desktop_test

import (
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Меню выбора длительности у «Фокусировки» через настоящий движок: правый щелчок
// доходит до календаря, меню рисуется поверх (оно дочернее календарю и выходит за
// его рамку вверх не обязано), щелчок по пункту не считается щелчком мимо
// календаря, Esc гасит сперва меню.
func TestFocusMenu_ThroughEngine(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(widget.StopAllAnimations)
	const w, h = 1280, 900
	root, bar := buildScene(t, m, w, h)
	t.Cleanup(bar.Close)

	clk := desktop.NewFakeClock(time.Date(2026, 10, 5, 7, 37, 0, 0, time.Local))
	fs := desktop.NewFakeFocusSession(clk)
	cal := desktop.NewCalendarFlyout(m, clk)
	cal.Screen = image.Rect(0, 0, w, h)
	cal.Ticker = func(time.Duration, func()) func() { return func() {} }
	cal.SetFocusSession(fs)
	root.AddChild(cal)
	eng := engine.New(w, h, 30)
	eng.SetRoot(root)
	bb := bar.Bounds()
	cal.Open(image.Rect(w-150, bb.Min.Y, w-100, bb.Max.Y))
	cal.Settle()
	qsFinish()
	eng.Invalidate()
	before := eng.RenderOnce()

	act := desktop.CalFocusAction(cal)
	x, y := act.Min.X+act.Dx()/2, act.Min.Y+act.Dy()/2
	eng.SendMouseButton(x, y, widget.MouseRight, true)
	eng.SendMouseButton(x, y, widget.MouseRight, false)
	if !cal.FocusMenuOpen() {
		t.Fatal("правый щелчок через движок не открыл меню")
	}
	eng.Invalidate()
	after := eng.RenderOnce()
	// Меню нарисовано над кнопкой, внутри календаря: кадр там изменился.
	diff := 0
	for yy := act.Min.Y - 160; yy < act.Min.Y; yy++ {
		for xx := act.Min.X; xx < act.Min.X+act.Dx(); xx++ {
			if before.RGBAAt(xx, yy) != after.RGBAAt(xx, yy) {
				diff++
			}
		}
	}
	if diff < 500 {
		t.Errorf("меню не нарисовано над кнопкой: изменилось %d пикселей", diff)
	}

	// Щелчок по пункту выбирает длительность и не закрывает календарь.
	menu := desktop.CalFocusMenuBounds(cal)
	if menu.Empty() {
		t.Fatal("у меню нет границ")
	}
	mx, my := menu.Min.X+menu.Dx()/2, menu.Min.Y+menu.Dy()/2
	eng.SendMouseMove(mx, my)
	eng.SendMouseButton(mx, my, widget.MouseLeft, true)
	eng.SendMouseButton(mx, my, widget.MouseLeft, false)
	if got := fs.State().Duration; got != 45*time.Minute {
		t.Errorf("выбор середины меню дал %v, ждали 45 мин", got)
	}
	if !cal.IsOpen() || cal.FocusMenuOpen() {
		t.Errorf("после выбора: календарь открыт=%v, меню открыто=%v", cal.IsOpen(), cal.FocusMenuOpen())
	}

	// Esc: сперва меню, потом календарь.
	eng.SendMouseButton(x, y, widget.MouseRight, true)
	eng.SendMouseButton(x, y, widget.MouseRight, false)
	if !cal.FocusMenuOpen() {
		t.Fatal("меню не открылось второй раз")
	}
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	if cal.FocusMenuOpen() || !cal.IsOpen() {
		t.Errorf("Esc: меню открыто=%v, календарь открыт=%v", cal.FocusMenuOpen(), cal.IsOpen())
	}
}
