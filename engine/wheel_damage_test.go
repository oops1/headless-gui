package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// wheelRoot — движок 300×300 с панелью на весь холст.
func wheelRoot(t *testing.T) (*Engine, *widget.Panel) {
	t.Helper()
	e := New(300, 300, 20)
	e.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 10, G: 10, B: 10, A: 255})
	root.SetBounds(image.Rect(0, 0, 300, 300))
	e.SetRoot(root)
	return e, root
}

// Пиксельное колесо над списком повреждает только область виджета.
func TestWheelPixels_DamageIsWidgetRect(t *testing.T) {
	e, root := wheelRoot(t)
	defer e.Stop()

	items := make([]string, 40)
	for i := range items {
		items[i] = "item"
	}
	lv := widget.NewListView(items...)
	lv.SetBounds(image.Rect(20, 20, 220, 120))
	root.AddChild(lv)
	e.consumeDamage()

	e.SendMouseWheelPixels(100, 60, 0, 120)
	if lv.ScrollY() <= 0 {
		t.Fatalf("прокрутка не сдвинулась: scrollY = %d", lv.ScrollY())
	}

	damage, all := e.consumeDamage()
	if all {
		t.Fatal("колесо не должно инвалидировать весь холст")
	}
	if damage.Empty() {
		t.Fatal("колесо не оставило damage-области")
	}
	if !damage.In(lv.Bounds()) {
		t.Fatalf("damage %v выходит за границы списка %v", damage, lv.Bounds())
	}
}

// Тиковый фолбэк: N тиков дают одну инвалидацию области виджета.
func TestWheelTicks_FallbackSingleDamage(t *testing.T) {
	e, root := wheelRoot(t)
	defer e.Stop()

	// NumericUpDown знает только тиковое колесо — попадаем в фолбэк.
	nud := widget.NewNumericUpDown()
	nud.SetBounds(image.Rect(20, 20, 140, 50))
	root.AddChild(nud)
	e.consumeDamage()

	e.SendMouseWheelPixels(80, 35, 0, -240) // 6 тиков вверх
	if nud.Value() == 0 {
		t.Fatal("тиковое колесо не изменило значение")
	}

	damage, all := e.consumeDamage()
	if all {
		t.Fatal("тиковый фолбэк не должен инвалидировать весь холст")
	}
	if damage.Empty() {
		t.Fatal("тиковый фолбэк не оставил damage-области")
	}
	if !damage.In(nud.Bounds()) {
		t.Fatalf("damage %v выходит за границы виджета %v", damage, nud.Bounds())
	}
}

// Колесо над пустым местом никого не будит.
func TestWheel_NoTargetNoDamage(t *testing.T) {
	e, _ := wheelRoot(t)
	defer e.Stop()
	e.consumeDamage()

	e.SendMouseWheelPixels(280, 280, 0, 120)

	damage, all := e.consumeDamage()
	if all || !damage.Empty() {
		t.Fatalf("колесо в пустоту дало damage %v (all=%v)", damage, all)
	}
}
