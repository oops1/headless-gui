package tests

import (
	"image"
	"sync/atomic"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Engine.SetCursorOverride: принудительная форма курсора поверх курсора
// виджетов, без ожидания движения мыши.

func TestCursorValues_AppendedAtEnd(t *testing.T) {
	// Старые значения не изменились, новые — в конце перечисления.
	old := []widget.Cursor{
		widget.CursorArrow, widget.CursorIBeam, widget.CursorHand, widget.CursorSizeWE,
		widget.CursorSizeNS, widget.CursorSizeNWSE, widget.CursorSizeNESW,
	}
	for i, c := range old {
		if int(c) != i {
			t.Errorf("старое значение #%d сдвинулось: %d", i, c)
		}
	}
	added := []widget.Cursor{
		widget.CursorNone, widget.CursorGrab, widget.CursorGrabbing,
		widget.CursorSizeAll, widget.CursorCrosshair, widget.CursorWait,
	}
	for i, c := range added {
		if int(c) != len(old)+i {
			t.Errorf("новое значение #%d = %d, ждал %d", i, c, len(old)+i)
		}
	}
}

// XAML: Cursor="..." знает новые формы.
func TestCursor_XAMLNewShapes(t *testing.T) {
	cases := map[string]widget.Cursor{
		"None": widget.CursorNone, "Grab": widget.CursorGrab, "Grabbing": widget.CursorGrabbing,
		"SizeAll": widget.CursorSizeAll, "Cross": widget.CursorCrosshair,
		"Crosshair": widget.CursorCrosshair, "Wait": widget.CursorWait,
	}
	for name, want := range cases {
		xaml := `<Canvas xmlns="clr"><Button Name="b" Cursor="` + name + `"/></Canvas>`
		_, reg, err := widget.LoadUIFromXAML([]byte(xaml))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got, has := reg["b"].(*widget.Button).CursorOverride()
		if !has || got != want {
			t.Errorf("Cursor=%q: %v,%v; ждал %v", name, got, has, want)
		}
	}
}

func overrideEngineWithHandButton(t *testing.T) *engine.Engine {
	t.Helper()
	btn := widget.NewButton("x")
	btn.SetCursor(widget.CursorHand)
	btn.SetBounds(image.Rect(0, 0, 200, 100))
	eng := engine.New(200, 100, 30)
	eng.SetRoot(btn)
	eng.RenderOnce()
	return eng
}

func TestCursorOverride_BeatsWidgetWithoutMouseMove(t *testing.T) {
	eng := overrideEngineWithHandButton(t)

	if got := eng.CursorAt(50, 50); got != widget.CursorHand {
		t.Fatalf("без override: %v, ждал Hand", got)
	}
	if _, ok := eng.CursorOverride(); ok {
		t.Fatal("override задан без вызова")
	}

	// Ни одного SendMouseMove: форма меняется сразу.
	eng.SetCursorOverride(widget.CursorNone)
	if got := eng.CursorAt(50, 50); got != widget.CursorNone {
		t.Fatalf("с override: %v, ждал None", got)
	}
	if c, ok := eng.CursorOverride(); !ok || c != widget.CursorNone {
		t.Fatalf("CursorOverride() = %v,%v", c, ok)
	}
	// Он сильнее виджета в любой точке, даже там, где виджета нет.
	if got := eng.CursorAt(-1, -1); got != widget.CursorNone {
		t.Errorf("вне виджетов: %v, ждал None", got)
	}

	// Смена формы override.
	eng.SetCursorOverride(widget.CursorGrabbing)
	if got := eng.CursorAt(50, 50); got != widget.CursorGrabbing {
		t.Errorf("после смены: %v, ждал Grabbing", got)
	}

	eng.ClearCursorOverride()
	if got := eng.CursorAt(50, 50); got != widget.CursorHand {
		t.Fatalf("после снятия: %v, ждал Hand", got)
	}
	if _, ok := eng.CursorOverride(); ok {
		t.Fatal("override остался после ClearCursorOverride")
	}
}

// Окно подписывается на смену override: уведомление приходит при установке,
// смене и снятии, но не на повторную ту же форму и не на лишний Clear.
func TestCursorOverride_Notifies(t *testing.T) {
	eng := overrideEngineWithHandButton(t)
	var n atomic.Int32
	eng.SetOnCursorOverrideChanged(func() { n.Add(1) })

	step := func(want int32, what string) {
		t.Helper()
		if got := n.Load(); got != want {
			t.Fatalf("%s: уведомлений %d, ждал %d", what, got, want)
		}
	}
	eng.ClearCursorOverride() // снимать нечего
	step(0, "Clear без override")
	eng.SetCursorOverride(widget.CursorNone)
	step(1, "первая установка")
	eng.SetCursorOverride(widget.CursorNone)
	step(1, "та же форма")
	eng.SetCursorOverride(widget.CursorGrab)
	step(2, "другая форма")
	eng.ClearCursorOverride()
	step(3, "снятие")
	eng.ClearCursorOverride()
	step(3, "повторное снятие")

	// Форма по умолчанию (стрелка) — тоже override: она перекрывает виджеты.
	eng.SetCursorOverride(widget.CursorArrow)
	step(4, "установка Arrow")
	if got := eng.CursorAt(50, 50); got != widget.CursorArrow {
		t.Errorf("override Arrow: %v", got)
	}

	eng.SetOnCursorOverrideChanged(nil)
	eng.SetCursorOverride(widget.CursorWait)
	step(4, "после снятия подписки")
}

// Override ставят с любой горутины (таймер бездействия), читает горутина движка.
func TestCursorOverride_ConcurrentUse(t *testing.T) {
	eng := overrideEngineWithHandButton(t)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			eng.SetCursorOverride(widget.CursorNone)
			eng.ClearCursorOverride()
		}
		close(done)
	}()
	for i := 0; i < 1000; i++ {
		_ = eng.CursorAt(10, 10)
	}
	<-done
}
