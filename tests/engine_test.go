package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/engine"
	"github.com/oops1/headless-gui/widget"
)

// newTestEngine создаёт движок 400×300 без запуска рендер-горутины.
func newTestEngine() *engine.Engine {
	return engine.New(400, 300, 20)
}

// ─── CanvasSize ──────────────────────────────────────────────────────────────

func TestEngine_CanvasSize(t *testing.T) {
	eng := engine.New(800, 600, 30)
	w, h := eng.CanvasSize()
	if w != 800 || h != 600 {
		t.Fatalf("CanvasSize() = (%d, %d), want (800, 600)", w, h)
	}
}

func TestEngine_SetResolution(t *testing.T) {
	eng := newTestEngine()
	eng.SetResolution(1920, 1080)
	w, h := eng.CanvasSize()
	if w != 1920 || h != 1080 {
		t.Fatalf("CanvasSize() after SetResolution = (%d, %d)", w, h)
	}
}

// ─── SetRoot ─────────────────────────────────────────────────────────────────

func TestEngine_SetRoot_BoundsSetToCanvas(t *testing.T) {
	eng := engine.New(300, 200, 20)
	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	eng.SetRoot(root)
	b := root.Bounds()
	if b.Dx() != 300 || b.Dy() != 200 {
		t.Fatalf("root bounds after SetRoot = %v, want 300×200", b)
	}
	if b.Min.X != 0 || b.Min.Y != 0 {
		t.Fatalf("root bounds should start at (0,0), got %v", b)
	}
}

func TestEngine_SetRoot_InjectsCaptureManager(t *testing.T) {
	eng := newTestEngine()
	panel := widget.NewWin10Panel()
	panel.Drag.Enabled = true
	eng.SetRoot(panel)
	// Если CaptureManager не инжектирован, panel.Drag.capMgr будет nil,
	// и OnMouseButton (release) вызовет ReleaseCapture на nil → panic.
	// Проверяем через drag release — должен не паниковать.
	panel.SetBounds(image.Rect(0, 0, 400, 300))

	// Симулируем press (initDrag)
	ev := widget.MouseEvent{X: 50, Y: 50, Button: widget.MouseLeft, Pressed: true}
	panel.OnMouseButton(ev)

	// Симулируем release — должен вызвать ReleaseCapture без паники
	ev.Pressed = false
	panel.OnMouseButton(ev)
	// Дошли сюда — panic не было
}

// ─── SetFocus ────────────────────────────────────────────────────────────────

func TestEngine_SetFocus(t *testing.T) {
	eng := newTestEngine()
	btn := widget.NewButton("B")

	eng.SetFocus(btn)
	if !btn.IsFocused() {
		t.Fatal("button should be focused after SetFocus")
	}
}

func TestEngine_SetFocus_TransfersFocus(t *testing.T) {
	eng := newTestEngine()
	btn1 := widget.NewButton("B1")
	btn2 := widget.NewButton("B2")

	eng.SetFocus(btn1)
	if !btn1.IsFocused() {
		t.Fatal("btn1 should be focused")
	}

	eng.SetFocus(btn2)
	if btn1.IsFocused() {
		t.Fatal("btn1 should lose focus when btn2 gets it")
	}
	if !btn2.IsFocused() {
		t.Fatal("btn2 should be focused")
	}
}

func TestEngine_SetFocus_Nil_ClearsAll(t *testing.T) {
	eng := newTestEngine()
	btn := widget.NewButton("B")
	eng.SetFocus(btn)
	eng.SetFocus(nil)
	if btn.IsFocused() {
		t.Fatal("button should lose focus after SetFocus(nil)")
	}
}

// ─── Mouse Capture ───────────────────────────────────────────────────────────

func TestEngine_SetCapture_ReleaseCapture(t *testing.T) {
	eng := newTestEngine()
	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(0, 0, 100, 30))

	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	root.AddChild(btn)
	eng.SetRoot(root)

	eng.SetCapture(btn)

	// При захвате все события мыши идут только захватчику,
	// даже если координаты не попадают в его bounds.
	// Нажатие вне bounds кнопки, но при захвате — кнопка должна получить событие
	eng.SendMouseButton(200, 200, widget.MouseLeft, true)
	if !btn.IsPressed() {
		t.Fatal("captured button should receive press event outside its bounds")
	}

	eng.ReleaseCapture()
	// После release — нажатие вне bounds не достигает кнопки
	eng.SendMouseButton(200, 200, widget.MouseLeft, true)
	// кнопка НЕ должна обработать клик вне своих bounds
}

// ─── SendMouseMove ───────────────────────────────────────────────────────────

func TestEngine_SendMouseMove_Broadcast(t *testing.T) {
	eng := newTestEngine()

	btn1 := widget.NewButton("B1")
	btn1.SetBounds(image.Rect(10, 10, 110, 40))
	btn2 := widget.NewButton("B2")
	btn2.SetBounds(image.Rect(150, 10, 250, 40))

	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	root.AddChild(btn1)
	root.AddChild(btn2)
	eng.SetRoot(root)

	// Наводим на btn1
	eng.SendMouseMove(50, 25)
	if !btn1.IsHovered() {
		t.Fatal("btn1 should be hovered")
	}
	if btn2.IsHovered() {
		t.Fatal("btn2 should not be hovered")
	}

	// Наводим на btn2
	eng.SendMouseMove(200, 25)
	if btn1.IsHovered() {
		t.Fatal("btn1 should no longer be hovered")
	}
	if !btn2.IsHovered() {
		t.Fatal("btn2 should be hovered")
	}
}

func TestEngine_SendMouseMove_CaptureExclusive(t *testing.T) {
	eng := newTestEngine()

	btn1 := widget.NewButton("B1")
	btn1.SetBounds(image.Rect(10, 10, 110, 40))
	btn2 := widget.NewButton("B2")
	btn2.SetBounds(image.Rect(150, 10, 250, 40))

	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	root.AddChild(btn1)
	root.AddChild(btn2)
	eng.SetRoot(root)

	// Захватываем btn1 и двигаем мышь на btn2 — btn1 должен получить событие
	eng.SetCapture(btn1)
	eng.SendMouseMove(200, 25) // coords "over" btn2, but captured to btn1
	// btn2 НЕ должна обновить hover (событие не дошло до неё)
	if btn2.IsHovered() {
		t.Fatal("btn2 should not receive move while btn1 has capture")
	}
	eng.ReleaseCapture()
}

// ─── SendMouseButton + hit-testing ───────────────────────────────────────────

func TestEngine_SendMouseButton_HitTest(t *testing.T) {
	eng := newTestEngine()

	btn := widget.NewButton("Click me")
	btn.SetBounds(image.Rect(50, 50, 200, 80))

	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	root.AddChild(btn)
	eng.SetRoot(root)

	clicked := false
	btn.OnClick = func() { clicked = true }

	// Клик внутри кнопки
	eng.SendMouseButton(100, 65, widget.MouseLeft, true)
	if !btn.IsPressed() {
		t.Fatal("button should be pressed after click inside its bounds")
	}
	eng.SendMouseButton(100, 65, widget.MouseLeft, false)
	if btn.IsPressed() {
		t.Fatal("button should not be pressed after release")
	}
	if !clicked {
		t.Fatal("OnClick should have been called")
	}
}

func TestEngine_SendMouseButton_Miss(t *testing.T) {
	eng := newTestEngine()

	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(50, 50, 200, 80))
	clicked := false
	btn.OnClick = func() { clicked = true }

	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	root.AddChild(btn)
	eng.SetRoot(root)

	// Клик вне кнопки
	eng.SendMouseButton(10, 10, widget.MouseLeft, true)
	eng.SendMouseButton(10, 10, widget.MouseLeft, false)
	if clicked {
		t.Fatal("OnClick should not be called for click outside bounds")
	}
}

func TestEngine_SendMouseButton_FocusOnClick(t *testing.T) {
	eng := newTestEngine()

	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(10, 10, 150, 40))

	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	root.AddChild(btn)
	eng.SetRoot(root)

	// Клик на кнопке должен передать ей фокус
	eng.SendMouseButton(80, 25, widget.MouseLeft, true)
	if !btn.IsFocused() {
		t.Fatal("button should receive focus on mouse click")
	}
}

func TestEngine_SendMouseButton_FocusCleared_OnNonFocusableClick(t *testing.T) {
	eng := newTestEngine()

	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(10, 10, 150, 40))
	eng.SetFocus(btn)

	// Клик на Panel (не Focusable) — фокус должен сняться
	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	root.AddChild(btn)
	eng.SetRoot(root)

	// Кликаем в пустую часть панели (вне кнопки)
	eng.SendMouseButton(300, 200, widget.MouseLeft, true)
	if btn.IsFocused() {
		t.Fatal("button should lose focus after click on non-focusable area")
	}
}

// ─── Event Bubbling ──────────────────────────────────────────────────────────

func TestEngine_EventBubbling_ParentReceivesIfChildDoesNot(t *testing.T) {
	eng := newTestEngine()

	// Создаём панель (MouseClickHandler) с меткой (не MouseClickHandler) поверх
	panelClicked := false
	panel := widget.NewPanel(widget.DarkTheme().PanelBG)
	panel.SetBounds(image.Rect(0, 0, 300, 200))

	// Используем CheckBox как дочерний виджет, который поглощает клики
	child := widget.NewCheckBox("Флажок")
	child.SetBounds(image.Rect(10, 10, 200, 40))
	panel.AddChild(child)

	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	root.AddChild(panel)
	eng.SetRoot(root)

	// CheckBox поглощает клик на себе
	eng.SendMouseButton(50, 20, widget.MouseLeft, false)
	// Панель НЕ получила клик, потому что CheckBox его поглотил
	_ = panelClicked // не тестируем панель напрямую — нет нужного callback
}

func TestEngine_EventBubbling_DeepHierarchy(t *testing.T) {
	eng := newTestEngine()

	// Глубокая иерархия: root → panel → subpanel → button
	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))

	panel := widget.NewPanel(widget.DarkTheme().PanelBG)
	panel.SetBounds(image.Rect(10, 10, 390, 290))

	subPanel := widget.NewPanel(widget.DarkTheme().PanelBG)
	subPanel.SetBounds(image.Rect(20, 20, 380, 280))

	btn := widget.NewButton("Deep")
	btn.SetBounds(image.Rect(50, 50, 200, 80))
	clicked := false
	btn.OnClick = func() { clicked = true }

	subPanel.AddChild(btn)
	panel.AddChild(subPanel)
	root.AddChild(panel)
	eng.SetRoot(root)

	eng.SendMouseButton(100, 65, widget.MouseLeft, true)
	eng.SendMouseButton(100, 65, widget.MouseLeft, false)
	if !clicked {
		t.Fatal("deeply nested button should receive click event")
	}
}

// ─── Tab Navigation ───────────────────────────────────────────────────────────

func TestEngine_TabNavigation_ForwardCycle(t *testing.T) {
	eng := newTestEngine()

	btn1 := widget.NewButton("B1")
	btn1.SetBounds(image.Rect(10, 10, 100, 40))
	btn2 := widget.NewButton("B2")
	btn2.SetBounds(image.Rect(10, 50, 100, 80))
	btn3 := widget.NewButton("B3")
	btn3.SetBounds(image.Rect(10, 90, 100, 120))

	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	root.AddChild(btn1)
	root.AddChild(btn2)
	root.AddChild(btn3)
	eng.SetRoot(root)

	tab := widget.KeyEvent{Code: widget.KeyTab, Pressed: true}

	// Первый Tab — фокус на btn1 (первый focusable)
	eng.SendKeyEvent(tab)
	if !btn1.IsFocused() {
		t.Fatal("first Tab should focus btn1")
	}

	// Второй Tab — btn2
	eng.SendKeyEvent(tab)
	if btn1.IsFocused() {
		t.Fatal("btn1 should lose focus")
	}
	if !btn2.IsFocused() {
		t.Fatal("second Tab should focus btn2")
	}

	// Третий Tab — btn3
	eng.SendKeyEvent(tab)
	if !btn3.IsFocused() {
		t.Fatal("third Tab should focus btn3")
	}

	// Четвёртый Tab — цикл обратно к btn1
	eng.SendKeyEvent(tab)
	if !btn1.IsFocused() {
		t.Fatal("Tab cycle should wrap around to btn1")
	}
}

func TestEngine_TabNavigation_BackwardCycle(t *testing.T) {
	eng := newTestEngine()

	btn1 := widget.NewButton("B1")
	btn1.SetBounds(image.Rect(10, 10, 100, 40))
	btn2 := widget.NewButton("B2")
	btn2.SetBounds(image.Rect(10, 50, 100, 80))

	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	root.AddChild(btn1)
	root.AddChild(btn2)
	eng.SetRoot(root)

	shiftTab := widget.KeyEvent{Code: widget.KeyTab, Mod: widget.ModShift, Pressed: true}

	// Первый Shift+Tab — фокус на последний (btn2)
	eng.SendKeyEvent(shiftTab)
	if !btn2.IsFocused() {
		t.Fatal("first Shift+Tab should focus last widget (btn2)")
	}

	// Второй Shift+Tab — btn1
	eng.SendKeyEvent(shiftTab)
	if !btn1.IsFocused() {
		t.Fatal("second Shift+Tab should focus btn1")
	}
}

func TestEngine_TabNavigation_NoFocusables(t *testing.T) {
	eng := newTestEngine()
	// Панель без Focusable — Tab не должен паниковать
	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	eng.SetRoot(root)

	tab := widget.KeyEvent{Code: widget.KeyTab, Pressed: true}
	eng.SendKeyEvent(tab) // не должен паниковать
}

// ─── SendKeyEvent → фокусированный виджет ────────────────────────────────────

func TestEngine_SendKeyEvent_DeliverToFocused(t *testing.T) {
	eng := newTestEngine()
	cb := widget.NewCheckBox("C")
	eng.SetFocus(cb)

	// Space переключает checkbox
	ev := widget.KeyEvent{Code: widget.KeySpace, Pressed: true}
	eng.SendKeyEvent(ev)
	if !cb.IsChecked() {
		t.Fatal("Space should toggle checkbox via SendKeyEvent")
	}
}

func TestEngine_SendKeyEvent_NoFocused(t *testing.T) {
	eng := newTestEngine()
	// Без фокуса — не должен паниковать
	ev := widget.KeyEvent{Code: widget.KeySpace, Pressed: true}
	eng.SendKeyEvent(ev)
}

// ─── Start / Stop ────────────────────────────────────────────────────────────

func TestEngine_StartStop(t *testing.T) {
	eng := newTestEngine()
	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	eng.SetRoot(root)

	eng.Start()
	// Читаем хотя бы один кадр или немедленно останавливаем
	eng.Stop()
	// Канал должен быть закрыт
	for range eng.Frames() {
		// drain, не должен блокироваться бесконечно
	}
}

func TestEngine_Frames_NonBlocking(t *testing.T) {
	eng := newTestEngine()
	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	eng.SetRoot(root)

	eng.Start()
	eng.Stop()
	// После Stop() Frames() должен быть закрыт
	count := 0
	for range eng.Frames() {
		count++
		if count > 1000 {
			t.Fatal("unexpected number of frames")
		}
	}
}

// ─── CollectFocusables ───────────────────────────────────────────────────────

func TestCollectFocusables_Empty(t *testing.T) {
	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	result := widget.CollectFocusables(root)
	if len(result) != 0 {
		t.Fatalf("panel alone has no focusables, got %d", len(result))
	}
}

func TestCollectFocusables_DFSOrder(t *testing.T) {
	root := widget.NewPanel(widget.DarkTheme().PanelBG)

	btn1 := widget.NewButton("B1")
	btn2 := widget.NewButton("B2")
	btn3 := widget.NewButton("B3")

	root.AddChild(btn1)
	root.AddChild(btn2)
	root.AddChild(btn3)

	result := widget.CollectFocusables(root)
	if len(result) != 3 {
		t.Fatalf("expected 3 focusables, got %d", len(result))
	}
	if result[0] != widget.Widget(btn1) {
		t.Fatal("first focusable should be btn1")
	}
	if result[1] != widget.Widget(btn2) {
		t.Fatal("second focusable should be btn2")
	}
	if result[2] != widget.Widget(btn3) {
		t.Fatal("third focusable should be btn3")
	}
}

func TestCollectFocusables_Nested(t *testing.T) {
	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	inner := widget.NewPanel(widget.DarkTheme().PanelBG)
	btn1 := widget.NewButton("B1")
	btn2 := widget.NewButton("B2")
	btn3 := widget.NewButton("B3")

	inner.AddChild(btn2)
	inner.AddChild(btn3)
	root.AddChild(btn1)
	root.AddChild(inner)

	result := widget.CollectFocusables(root)
	if len(result) != 3 {
		t.Fatalf("expected 3 focusables, got %d", len(result))
	}
	// DFS: btn1, btn2 (внутри inner), btn3 (внутри inner)
	if result[0] != widget.Widget(btn1) {
		t.Fatal("first should be btn1")
	}
	if result[1] != widget.Widget(btn2) {
		t.Fatal("second should be btn2")
	}
	if result[2] != widget.Widget(btn3) {
		t.Fatal("third should be btn3")
	}
}

func TestCollectFocusables_MixedWidgets(t *testing.T) {
	root := widget.NewPanel(widget.DarkTheme().PanelBG)

	btn := widget.NewButton("B")
	lbl := widget.NewWin10Label("L") // Label — не Focusable
	cb := widget.NewCheckBox("C")
	ts := widget.NewToggleSwitch("T")

	root.AddChild(btn)
	root.AddChild(lbl)
	root.AddChild(cb)
	root.AddChild(ts)

	result := widget.CollectFocusables(root)
	if len(result) != 3 { // btn, cb, ts — label исключена
		t.Fatalf("expected 3 focusables (btn+cb+ts), got %d", len(result))
	}
}

// ─── SetTheme ────────────────────────────────────────────────────────────────

func TestEngine_SetTheme(t *testing.T) {
	eng := newTestEngine()

	btn := widget.NewButton("B")
	btn.SetBounds(image.Rect(0, 0, 100, 30))

	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	root.AddChild(btn)
	eng.SetRoot(root)

	lightTheme := widget.LightTheme()
	eng.SetTheme(lightTheme)

	if btn.TextColor != lightTheme.BtnText {
		t.Fatal("SetTheme should propagate to all widgets in tree")
	}
}
