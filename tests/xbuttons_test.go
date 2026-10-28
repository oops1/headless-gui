package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// Боковые кнопки мыши X1/X2 («назад/вперёд») доходят до корня дерева, а
// стандартные виджеты на них не реагируют: ни клика, ни переключения, ни
// фокуса — ровно как на правую и среднюю кнопки.

// rootSpy — корень, записывающий события мыши, которые до него всплыли.
type rootSpy struct {
	*widget.Panel
	got []widget.MouseEvent
}

func (r *rootSpy) OnMouseButton(e widget.MouseEvent) bool {
	r.got = append(r.got, e)
	return false
}

func TestMouseButtonX1X2_Values(t *testing.T) {
	testreset.Globals(t)
	// Старые значения не сдвинулись, новые — в конце перечисления.
	if widget.MouseLeft != 0 || widget.MouseRight != 1 || widget.MouseMiddle != 2 ||
		widget.MouseWheelUp != 3 || widget.MouseWheelDown != 4 {
		t.Fatal("значения прежних кнопок мыши изменились")
	}
	if widget.MouseButtonX1 != 5 || widget.MouseButtonX2 != 6 {
		t.Fatalf("X1=%d X2=%d, ждал 5 и 6", widget.MouseButtonX1, widget.MouseButtonX2)
	}
}

func TestMouseButtonX1X2_ReachRoot(t *testing.T) {
	testreset.Globals(t)
	root := &rootSpy{Panel: widget.NewPanel(color.RGBA{})}
	root.SetBounds(image.Rect(0, 0, 300, 400))
	eng := engine.New(300, 400, 30)
	eng.SetRoot(root)
	eng.RenderOnce()

	// По пустому месту корня.
	for _, b := range []widget.MouseButton{widget.MouseButtonX1, widget.MouseButtonX2} {
		root.got = nil
		eng.SendMouseButton(250, 350, b, true)
		eng.SendMouseButton(250, 350, b, false)
		if len(root.got) != 2 {
			t.Fatalf("кнопка %d: корень получил %d событий, ждал 2", b, len(root.got))
		}
		if root.got[0].Button != b || !root.got[0].Pressed || root.got[1].Button != b || root.got[1].Pressed {
			t.Errorf("кнопка %d: события %+v", b, root.got)
		}
		if root.got[0].X != 250 || root.got[0].Y != 350 {
			t.Errorf("кнопка %d: координаты (%d,%d)", b, root.got[0].X, root.got[0].Y)
		}
	}
}

func TestMouseButtonX1X2_WidgetsIgnore(t *testing.T) {
	testreset.Globals(t)
	root := &rootSpy{Panel: widget.NewPanel(color.RGBA{})}
	root.SetBounds(image.Rect(0, 0, 300, 400))

	clicks := 0
	btn := widget.NewButton("ok")
	btn.OnClick = func() { clicks++ }
	btn.SetBounds(image.Rect(0, 0, 100, 30))

	chk := widget.NewCheckBox("c")
	chk.SetBounds(image.Rect(0, 40, 100, 70))

	tog := widget.NewToggleSwitch("t")
	tog.SetBounds(image.Rect(0, 80, 100, 110))

	rad := widget.NewRadioButton("r", "g")
	rad.SetBounds(image.Rect(0, 120, 100, 150))

	ti := widget.NewTextInput("")
	ti.SetBounds(image.Rect(0, 160, 200, 190))

	dd := widget.NewDropdown("a", "b", "c")
	dd.SetBounds(image.Rect(0, 200, 100, 230))

	for _, w := range []widget.Widget{btn, chk, tog, rad, ti, dd} {
		root.AddChild(w)
	}

	eng := engine.New(300, 400, 30)
	eng.SetRoot(root)
	eng.RenderOnce()

	for _, b := range []widget.MouseButton{widget.MouseButtonX1, widget.MouseButtonX2} {
		for _, w := range []widget.Widget{btn, chk, tog, rad, ti, dd} {
			r := w.Bounds()
			x, y := r.Min.X+10, r.Min.Y+r.Dy()/2
			root.got = nil
			eng.SendMouseButton(x, y, b, true)
			eng.SendMouseButton(x, y, b, false)
			// Не поглощено — значит всплыло до корня.
			if len(root.got) != 2 {
				t.Errorf("%T, кнопка %d: до корня дошло %d событий, ждал 2 (виджет проглотил боковую кнопку)",
					w, b, len(root.got))
			}
		}
	}

	if clicks != 0 {
		t.Errorf("Button принял боковую кнопку за клик: %d", clicks)
	}
	if chk.IsChecked() {
		t.Error("CheckBox переключился боковой кнопкой")
	}
	if tog.IsOn() {
		t.Error("ToggleSwitch переключился боковой кнопкой")
	}
	if ti.IsFocused() {
		t.Error("TextInput получил фокус от боковой кнопки")
	}
	if dd.IsOpen() {
		t.Error("Dropdown раскрылся от боковой кнопки")
	}

	// Контроль: левая кнопка по той же кнопке работает — тест не пустой.
	eng.SendMouseButton(10, 15, widget.MouseLeft, true)
	eng.SendMouseButton(10, 15, widget.MouseLeft, false)
	if clicks != 1 {
		t.Errorf("контроль: левый клик по Button дал %d срабатываний, ждал 1", clicks)
	}
}
