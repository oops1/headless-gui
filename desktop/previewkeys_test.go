package desktop

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Список окон стопки, открытый с клавиатуры, получает клавиши: стрелки выбирают
// окно (рамка фокуса), Enter поднимает, Delete закрывает, Esc закрывает список
// и возвращает фокус на кнопку. Раньше фокус оставался на панели, и до списка
// клавиши не доходили.

// keyScene — сцена goldenScene в живом движке: клавиши идут через
// Engine.SendKeyEvent, как от окна ОС. Фокус стоит на ячейке «mail» (три окна:
// стопка), клавиатурный.
func keyScene(t *testing.T, light bool) (*goldenParts, *engine.Engine, int) {
	t.Helper()
	g := goldenScene(t, light)
	eng := engine.New(640, 240, 30)
	eng.SetRoot(g.root)
	eng.RenderOnce()
	finishAnimations()

	mail := -1
	for i, c := range g.area.Cells() {
		if c.Title == "Mail" {
			mail = i
		}
	}
	if mail < 0 {
		t.Fatal("в сцене нет кнопки «Mail»")
	}
	eng.SetFocus(g.area)
	g.area.FocusState.SetCell(mail)
	t.Cleanup(eng.Stop)
	return g, eng, mail
}

func press(eng *engine.Engine, code widget.KeyCode) {
	eng.SendKeyEvent(widget.KeyEvent{Code: code, Pressed: true})
	eng.SendKeyEvent(widget.KeyEvent{Code: code})
}

func TestPreviewList_EnterOpensListWithFocus(t *testing.T) {
	g, eng, _ := keyScene(t, false)
	press(eng, widget.KeyEnter)

	if !g.prev.IsOpen() {
		t.Fatal("Enter на стопке не открыл список окон")
	}
	if got := len(g.prev.ListedWindows()); got != 3 {
		t.Fatalf("в списке %d окон, ждали 3 (mail)", got)
	}
	if len(g.wm.Activated) != 0 {
		t.Errorf("Enter переключил окно %v вместо открытия списка", g.wm.Activated)
	}
	if !g.prev.IsFocused() {
		t.Error("список не получил фокус клавиатуры")
	}
	if g.area.IsFocused() {
		t.Error("фокус остался и на панели")
	}
	if idx, kbd := g.prev.ListSelection(); idx != 0 || !kbd {
		t.Errorf("выбор после открытия (%d, клавиатурный=%v), ждали (0, true)", idx, kbd)
	}
}

func TestPreviewList_ArrowsSelectAndRingIsDrawn(t *testing.T) {
	g, eng, _ := keyScene(t, false)
	press(eng, widget.KeyEnter)

	step := func(code widget.KeyCode, want int) {
		t.Helper()
		press(eng, code)
		if idx, kbd := g.prev.ListSelection(); idx != want || !kbd {
			t.Fatalf("после клавиши %v выбор (%d, %v), ждали (%d, true)", code, idx, kbd, want)
		}
	}
	step(widget.KeyRight, 1)
	step(widget.KeyDown, 2)
	step(widget.KeyRight, 0) // по кругу
	step(widget.KeyLeft, 2)
	step(widget.KeyHome, 0)
	step(widget.KeyEnd, 2)
	step(widget.KeyUp, 1)

	// Рамка фокуса видна: контур выбранного окна нарисован цветом текста, а у
	// невыбранного соседа в том же месте его нет.
	eng.RenderOnce()
	finishAnimations()
	img := eng.RenderOnce()
	items, _ := g.prev.listItems(g.prev.contentRect(), 3)
	ring := func(i int) bool {
		it := items[i]
		want := focusRingColor(g.tm, styleOf(g.tm, ComponentPreview, previewPartHeader, 0))
		return img.RGBAAt(it.Min.X-2, it.Min.Y+it.Dy()/2) == want
	}
	if !ring(1) {
		t.Error("у выбранного окна нет рамки фокуса")
	}
	if ring(0) || ring(2) {
		t.Error("рамка нарисована и у невыбранных окон")
	}
}

func TestPreviewList_EnterActivatesSelectedAndReturnsFocus(t *testing.T) {
	g, eng, _ := keyScene(t, false)
	press(eng, widget.KeyEnter)
	press(eng, widget.KeyRight) // второе окно (id 4)
	press(eng, widget.KeyEnter)

	if got := g.wm.Activated; len(got) != 1 || got[0] != 4 {
		t.Fatalf("активировано %v, ждали [4]", got)
	}
	if g.prev.IsOpen() {
		t.Error("после выбора окна список остался открыт")
	}
	if !g.area.IsFocused() || g.prev.IsFocused() {
		t.Errorf("фокус не вернулся на кнопку: панель=%v список=%v", g.area.IsFocused(), g.prev.IsFocused())
	}
}

func TestPreviewList_DeleteClosesSelectedWindow(t *testing.T) {
	g, eng, _ := keyScene(t, false)
	press(eng, widget.KeyEnter)
	press(eng, widget.KeyRight) // окно 4
	press(eng, widget.KeyDelete)

	if got := g.wm.Closed; len(got) != 1 || got[0] != 4 {
		t.Fatalf("закрыто %v, ждали [4]", got)
	}
	if !g.prev.IsOpen() || len(g.prev.ListedWindows()) != 2 {
		t.Fatalf("список после закрытия окна: открыт=%v окон=%d", g.prev.IsOpen(), len(g.prev.ListedWindows()))
	}
	if idx, kbd := g.prev.ListSelection(); idx != 1 || !kbd {
		t.Errorf("выбор после Delete (%d, %v), ждали (1, true): на месте закрытого окна", idx, kbd)
	}
	// Ещё Delete: остаётся одно окно — список не нужен, фокус на кнопке.
	press(eng, widget.KeyDelete)
	if g.prev.IsOpen() {
		t.Error("осталось одно окно, а список открыт")
	}
	if !g.area.IsFocused() {
		t.Error("фокус не вернулся на кнопку после закрытия списка")
	}
}

func TestPreviewList_EscapeClosesAndReturnsFocus(t *testing.T) {
	g, eng, mail := keyScene(t, false)
	press(eng, widget.KeyEnter)
	press(eng, widget.KeyEscape)

	if g.prev.IsOpen() {
		t.Fatal("Esc не закрыл список")
	}
	if !g.area.IsFocused() {
		t.Fatal("после Esc фокус не на панели")
	}
	if got := g.area.FocusState.Cell(g.area.cellCount()); got != mail {
		t.Errorf("фокус вернулся на ячейку %d, ждали кнопку стопки %d", got, mail)
	}
	if len(g.wm.Activated) != 0 || len(g.wm.Closed) != 0 {
		t.Error("Esc тронул окна")
	}
	// И снова открывается.
	press(eng, widget.KeyEnter)
	if !g.prev.IsOpen() {
		t.Error("после Esc список не открылся повторно")
	}
}

// Мышь, ушедшая мимо, выбор клавишами не сбрасывает и список не закрывает;
// мышь над списком выбор забирает себе.
func TestPreviewList_MouseDoesNotStealKeyboardSelection(t *testing.T) {
	g, eng, _ := keyScene(t, false)
	press(eng, widget.KeyEnter)
	press(eng, widget.KeyRight)

	eng.SendMouseMove(10, 10)
	finishAnimations()
	if !g.prev.IsOpen() {
		t.Fatal("мышь, ушедшая мимо, закрыла список, открытый с клавиатуры")
	}
	if idx, kbd := g.prev.ListSelection(); idx != 1 || !kbd {
		t.Errorf("выбор после ухода мыши (%d, %v), ждали (1, true)", idx, kbd)
	}

	items, _ := g.prev.listItems(g.prev.contentRect(), 3)
	pt := items[2].Min.Add(image.Pt(5, 5))
	eng.SendMouseMove(pt.X, pt.Y)
	if idx, kbd := g.prev.ListSelection(); idx != 2 || kbd {
		t.Errorf("выбор после наведения мыши (%d, клавиатурный=%v), ждали (2, false)", idx, kbd)
	}
}

// Список, открытый мышью, фокус не берёт: клавиши остаются у панели.
func TestPreviewList_MouseOpenedListKeepsPanelFocus(t *testing.T) {
	g, eng, mail := keyScene(t, false)
	clickAt(g.area, g.area.ButtonRect(mail))
	if !g.prev.IsOpen() {
		t.Fatal("щелчок не открыл список")
	}
	if g.prev.IsFocused() || !g.area.IsFocused() {
		t.Errorf("список, открытый мышью, забрал фокус: список=%v панель=%v", g.prev.IsFocused(), g.area.IsFocused())
	}
	if _, kbd := g.prev.ListSelection(); kbd {
		t.Error("список, открытый мышью, показывает рамку фокуса")
	}
	_ = eng
}

// Потеря фокуса (Tab) закрывает список, открытый с клавиатуры.
func TestPreviewList_TabAwayClosesList(t *testing.T) {
	g, eng, _ := keyScene(t, false)
	press(eng, widget.KeyEnter)
	press(eng, widget.KeyTab)
	if g.prev.IsFocused() {
		t.Fatal("Tab не увёл фокус со списка")
	}
	waitFor(t, "закрытие списка после потери фокуса", func() bool { return !g.prev.IsOpen() })
}

// Без предпросмотра за областью Enter на стопке по-прежнему листает окна.
func TestPreviewList_NoPreviewKeepsCycling(t *testing.T) {
	a, _, wm := groupScene(t, win10Fast(t))
	a.SetFocused(true)
	a.FocusState.SetCell(1)
	a.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if got := wm.Activated; len(got) != 1 || got[0] != 2 {
		t.Errorf("Enter активировал %v, ждали [2]", got)
	}
}

// Снимки: список окон стопки с рамкой фокуса на выбранном окне (при GOLDEN_OUT).
func TestPreviewList_Shots(t *testing.T) {
	for _, light := range []bool{false, true} {
		name := "dark"
		if light {
			name = "light"
		}
		g, eng, _ := keyScene(t, light)
		press(eng, widget.KeyEnter)
		press(eng, widget.KeyRight)
		eng.RenderOnce()
		finishAnimations()
		savePNG(t, "win10_list_focus_"+name+".png", eng.RenderOnce())
		if _, kbd := g.prev.ListSelection(); !kbd {
			t.Error("рамка фокуса не включена")
		}
	}
}
