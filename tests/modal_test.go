package tests

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/engine"
	"github.com/oops1/headless-gui/widget"
)

// ─── Dialog ──────────────────────────────────────────────────────────────────

func TestDialog_NewDialog_Defaults(t *testing.T) {
	dlg := widget.NewDialog("Заголовок", 400, 200)
	if dlg.Title != "Заголовок" {
		t.Fatalf("Title = %q", dlg.Title)
	}
	b := dlg.Bounds()
	if b.Dx() != 400 || b.Dy() != 200 {
		t.Fatalf("bounds = %v, want 400×200", b)
	}
	if !dlg.IsModal() {
		t.Fatal("new dialog should be modal by default")
	}
}

func TestDialog_DimColor(t *testing.T) {
	dlg := widget.NewDialog("T", 300, 150)
	dim := dlg.DimColor()
	if dim.A == 0 {
		t.Fatal("dim color should have non-zero alpha")
	}
}

func TestDialog_SetModal(t *testing.T) {
	dlg := widget.NewDialog("T", 300, 150)
	dlg.SetModal(false)
	if dlg.IsModal() {
		t.Fatal("SetModal(false) should disable modal")
	}
	dlg.SetModal(true)
	if !dlg.IsModal() {
		t.Fatal("SetModal(true) should enable modal")
	}
}

func TestDialog_ContentBounds(t *testing.T) {
	dlg := widget.NewDialog("T", 400, 200)
	cb := dlg.ContentBounds()
	// ContentBounds должен быть меньше общих bounds и ниже заголовка
	if cb.Min.Y <= dlg.Bounds().Min.Y {
		t.Fatal("content bounds should start below dialog top")
	}
	if cb.Min.X <= dlg.Bounds().Min.X {
		t.Fatal("content bounds should have padding from left edge")
	}
	if cb.Max.X >= dlg.Bounds().Max.X {
		t.Fatal("content bounds should have padding from right edge")
	}
}

func TestDialog_AddChildren(t *testing.T) {
	dlg := widget.NewDialog("T", 400, 200)
	btn := widget.NewButton("OK")
	dlg.AddChild(btn)
	if len(dlg.Children()) != 1 {
		t.Fatal("dialog should have 1 child")
	}
}

func TestDialog_ApplyTheme(t *testing.T) {
	dlg := widget.NewDialog("T", 400, 200)
	theme := widget.LightTheme()
	dlg.ApplyTheme(theme)
	if dlg.BorderColor != theme.Border {
		t.Fatal("ApplyTheme should update BorderColor")
	}
	if dlg.TitleColor != theme.TitleText {
		t.Fatal("ApplyTheme should update TitleColor")
	}
}

// ─── NewConfirmDialog ─────────────────────────────────────────────────────────

func TestNewConfirmDialog_Structure(t *testing.T) {
	var result *bool
	dlg := widget.NewConfirmDialog("Подтверждение", "Вы уверены?", func(ok bool) {
		b := ok
		result = &b
	})

	if dlg.Title != "Подтверждение" {
		t.Fatalf("Title = %q", dlg.Title)
	}
	// Должно быть: Label + OK кнопка + Cancel кнопка = 3 дочерних
	if len(dlg.Children()) != 3 {
		t.Fatalf("expected 3 children (label + 2 buttons), got %d", len(dlg.Children()))
	}
	_ = result // используется в callback
}

func TestNewConfirmDialog_OKButton(t *testing.T) {
	resultCh := make(chan bool, 1)
	dlg := widget.NewConfirmDialog("T", "M", func(ok bool) {
		resultCh <- ok
	})

	// Ищем кнопки среди детей (кнопки типа *widget.Button)
	// Первая кнопка — OK, вторая — Отмена
	buttons := make([]*widget.Button, 0)
	for _, ch := range dlg.Children() {
		if btn, ok := ch.(*widget.Button); ok {
			buttons = append(buttons, btn)
		}
	}
	if len(buttons) < 2 {
		t.Fatalf("expected at least 2 buttons, got %d", len(buttons))
	}

	// Нажимаем OK
	if buttons[0].OnClick != nil {
		buttons[0].OnClick()
	}
	select {
	case got := <-resultCh:
		if !got {
			t.Fatal("OK button should return true")
		}
	default:
		t.Fatal("OnClick did not call result callback")
	}
}

func TestNewConfirmDialog_CancelButton(t *testing.T) {
	resultCh := make(chan bool, 1)
	dlg := widget.NewConfirmDialog("T", "M", func(ok bool) {
		resultCh <- ok
	})

	buttons := make([]*widget.Button, 0)
	for _, ch := range dlg.Children() {
		if btn, ok := ch.(*widget.Button); ok {
			buttons = append(buttons, btn)
		}
	}
	if len(buttons) < 2 {
		t.Fatalf("expected at least 2 buttons, got %d", len(buttons))
	}

	// Нажимаем Отмена
	if buttons[1].OnClick != nil {
		buttons[1].OnClick()
	}
	select {
	case got := <-resultCh:
		if got {
			t.Fatal("Cancel button should return false")
		}
	default:
		t.Fatal("OnClick did not call result callback")
	}
}

// ─── ModalShower (mock для тестирования MessageBox без Engine) ───────────────

// mockModalShower — фейковый ModalShower для юнит-тестов.
type mockModalShower struct {
	shown  []widget.ModalWidget
	closed []widget.ModalWidget
}

func (m *mockModalShower) ShowModal(mw widget.ModalWidget) {
	m.shown = append(m.shown, mw)
}

func (m *mockModalShower) CloseModal(mw widget.ModalWidget) {
	m.closed = append(m.closed, mw)
}

// ─── MessageBox ──────────────────────────────────────────────────────────────

func TestMessageBox_Show_CallsShowModal(t *testing.T) {
	ms := &mockModalShower{}
	mb := widget.NewMessageBox(ms)
	dlg := mb.Show("Ошибка", "Файл не найден")

	if dlg == nil {
		t.Fatal("Show should return a dialog")
	}
	if len(ms.shown) != 1 {
		t.Fatalf("ShowModal should be called once, got %d", len(ms.shown))
	}
	if ms.shown[0] != dlg {
		t.Fatal("ShowModal should be called with the returned dialog")
	}
}

func TestMessageBox_Show_SingleOKButton(t *testing.T) {
	ms := &mockModalShower{}
	mb := widget.NewMessageBox(ms)
	dlg := mb.Show("T", "M")

	buttons := collectButtons(dlg)
	if len(buttons) != 1 {
		t.Fatalf("MBOk should have 1 button, got %d", len(buttons))
	}
}

func TestMessageBox_ShowOKCancel_TwoButtons(t *testing.T) {
	ms := &mockModalShower{}
	mb := widget.NewMessageBox(ms)
	dlg := mb.ShowOKCancel("T", "M", nil)

	buttons := collectButtons(dlg)
	if len(buttons) != 2 {
		t.Fatalf("MBOkCancel should have 2 buttons, got %d", len(buttons))
	}
}

func TestMessageBox_ShowYesNo_TwoButtons(t *testing.T) {
	ms := &mockModalShower{}
	mb := widget.NewMessageBox(ms)
	dlg := mb.ShowYesNo("T", "M", nil)

	buttons := collectButtons(dlg)
	if len(buttons) != 2 {
		t.Fatalf("MBYesNo should have 2 buttons, got %d", len(buttons))
	}
}

func TestMessageBox_ShowYesNoCancel_ThreeButtons(t *testing.T) {
	ms := &mockModalShower{}
	mb := widget.NewMessageBox(ms)
	dlg := mb.ShowYesNoCancel("T", "M", nil)

	buttons := collectButtons(dlg)
	if len(buttons) != 3 {
		t.Fatalf("MBYesNoCancel should have 3 buttons, got %d", len(buttons))
	}
}

func TestMessageBox_ShowDialog_CallbackOnButtonClick(t *testing.T) {
	ms := &mockModalShower{}
	mb := widget.NewMessageBox(ms)
	resultCh := make(chan widget.MessageBoxResult, 1)

	dlg := mb.ShowDialog("T", "M", widget.MBYesNo, func(r widget.MessageBoxResult) {
		resultCh <- r
	})

	buttons := collectButtons(dlg)
	if len(buttons) < 1 {
		t.Fatal("expected at least 1 button")
	}

	// Нажимаем первую кнопку (Да/Yes)
	if buttons[0].OnClick != nil {
		buttons[0].OnClick()
	}

	select {
	case got := <-resultCh:
		if got != widget.MBResultYes {
			t.Fatalf("first button of MBYesNo should return MBResultYes, got %d", got)
		}
	default:
		t.Fatal("callback was not called")
	}

	// CloseModal должен быть вызван при нажатии кнопки
	if len(ms.closed) != 1 {
		t.Fatalf("CloseModal should be called on button click, got %d calls", len(ms.closed))
	}
}

func TestMessageBox_ShowDialog_OKResult(t *testing.T) {
	ms := &mockModalShower{}
	mb := widget.NewMessageBox(ms)
	resultCh := make(chan widget.MessageBoxResult, 1)

	dlg := mb.ShowDialog("T", "M", widget.MBOk, func(r widget.MessageBoxResult) {
		resultCh <- r
	})

	buttons := collectButtons(dlg)
	if len(buttons) < 1 {
		t.Fatal("expected 1 button")
	}
	if buttons[0].OnClick != nil {
		buttons[0].OnClick()
	}
	select {
	case got := <-resultCh:
		if got != widget.MBResultOK {
			t.Fatalf("MBOk button should return MBResultOK, got %d", got)
		}
	default:
		t.Fatal("callback not called")
	}
}

func TestMessageBox_ShowDialog_CancelResult(t *testing.T) {
	ms := &mockModalShower{}
	mb := widget.NewMessageBox(ms)
	resultCh := make(chan widget.MessageBoxResult, 1)

	dlg := mb.ShowDialog("T", "M", widget.MBOkCancel, func(r widget.MessageBoxResult) {
		resultCh <- r
	})

	buttons := collectButtons(dlg)
	if len(buttons) < 2 {
		t.Fatal("expected 2 buttons")
	}
	// Вторая кнопка — Отмена
	if buttons[1].OnClick != nil {
		buttons[1].OnClick()
	}
	select {
	case got := <-resultCh:
		if got != widget.MBResultCancel {
			t.Fatalf("Cancel button should return MBResultCancel, got %d", got)
		}
	default:
		t.Fatal("callback not called")
	}
}

func TestMessageBox_ShowDialog_NoCallback(t *testing.T) {
	ms := &mockModalShower{}
	mb := widget.NewMessageBox(ms)

	// Без callback — не должно паниковать при нажатии кнопки
	dlg := mb.ShowDialog("T", "M", widget.MBOk, nil)
	buttons := collectButtons(dlg)
	if len(buttons) > 0 && buttons[0].OnClick != nil {
		buttons[0].OnClick() // не должен паниковать
	}
}

func TestMessageBox_ShowDialog_MultilineMessage(t *testing.T) {
	ms := &mockModalShower{}
	mb := widget.NewMessageBox(ms)

	msg := "Первая строка\nВторая строка\nТретья строка"
	dlg := mb.ShowDialog("T", msg, widget.MBOk, nil)

	// Должно быть несколько Label для каждой строки
	labels := collectLabels(dlg)
	if len(labels) < 3 {
		t.Fatalf("multiline message should create multiple labels, got %d", len(labels))
	}
}

func TestMessageBox_ShowDialog_LongMessage_Wraps(t *testing.T) {
	ms := &mockModalShower{}
	mb := widget.NewMessageBox(ms)

	// Очень длинная строка — должна быть перенесена
	msg := "Это очень длинное сообщение которое точно должно переноситься потому что оно содержит много слов"
	dlg := mb.ShowDialog("T", msg, widget.MBOk, nil)

	labels := collectLabels(dlg)
	// Ожидаем перенос — минимум 2 label
	if len(labels) < 2 {
		t.Fatal("long message should wrap into multiple lines")
	}
}

func TestMessageBox_ShowDialog_DialogSize_Reasonable(t *testing.T) {
	ms := &mockModalShower{}
	mb := widget.NewMessageBox(ms)

	dlg := mb.ShowDialog("Заголовок диалога", "Короткое сообщение", widget.MBOk, nil)
	b := dlg.Bounds()

	if b.Dx() < 100 || b.Dx() > 600 {
		t.Fatalf("dialog width = %d, should be between 100 and 600", b.Dx())
	}
	if b.Dy() < 50 || b.Dy() > 400 {
		t.Fatalf("dialog height = %d, should be between 50 and 400", b.Dy())
	}
}

// ─── Engine ShowModal / CloseModal ───────────────────────────────────────────

func TestEngine_ShowModal_CentersDialog(t *testing.T) {
	eng := engine.New(400, 300, 20)

	dlg := widget.NewDialog("T", 200, 150)
	eng.ShowModal(dlg)

	b := dlg.Bounds()
	// Диалог 200×150 на экране 400×300 → центр должен быть в (200, 150)
	// Min.X = (400-200)/2 = 100, Min.Y = (300-150)/2 = 75
	expectedX := (400 - 200) / 2
	expectedY := (300 - 150) / 2
	if b.Min.X != expectedX {
		t.Fatalf("centered X = %d, want %d", b.Min.X, expectedX)
	}
	if b.Min.Y != expectedY {
		t.Fatalf("centered Y = %d, want %d", b.Min.Y, expectedY)
	}
}

func TestEngine_ShowModal_ChildrenShifted(t *testing.T) {
	eng := engine.New(400, 300, 20)

	dlg := widget.NewDialog("T", 200, 150)
	child := widget.NewButton("OK")
	// Кнопка в локальных координатах диалога
	child.SetBounds(image.Rect(10, 100, 90, 130))
	dlg.AddChild(child)

	eng.ShowModal(dlg)

	// Дочерний виджет должен быть сдвинут вместе с диалогом
	dlgB := dlg.Bounds()
	childB := child.Bounds()

	// child.Min.X должен быть смещён на dlgB.Min.X от 0
	if childB.Min.X != dlgB.Min.X+10 {
		t.Fatalf("child X after ShowModal = %d, want %d", childB.Min.X, dlgB.Min.X+10)
	}
}

func TestEngine_CloseModal_SpecificDialog(t *testing.T) {
	eng := engine.New(400, 300, 20)

	dlg1 := widget.NewDialog("D1", 200, 150)
	dlg2 := widget.NewDialog("D2", 200, 150)
	eng.ShowModal(dlg1)
	eng.ShowModal(dlg2)

	// Закрываем dlg1 явно
	eng.CloseModal(dlg1)

	// После закрытия dlg1 ввод должен идти к dlg2
	// Проверяем косвенно — создаём кнопку внутри dlg2 и кликаем
	btn := widget.NewButton("B")
	btn.SetBounds(dlg2.Bounds()) // занимаем весь диалог
	dlg2.AddChild(btn)

	clicked := false
	btn.OnClick = func() { clicked = true }

	b := dlg2.Bounds()
	eng.SendMouseButton(b.Min.X+10, b.Min.Y+10, widget.MouseLeft, true)
	eng.SendMouseButton(b.Min.X+10, b.Min.Y+10, widget.MouseLeft, false)
	if !clicked {
		t.Fatal("button in dlg2 should receive events after dlg1 is closed")
	}
}

func TestEngine_CloseModal_Nil_ClosesTop(t *testing.T) {
	eng := engine.New(400, 300, 20)

	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	rootBtn := widget.NewButton("Root")
	rootBtn.SetBounds(image.Rect(10, 10, 200, 50))
	root.AddChild(rootBtn)
	eng.SetRoot(root)

	dlg := widget.NewDialog("D", 300, 200)
	eng.ShowModal(dlg)

	// Закрываем верхний (nil = top)
	eng.CloseModal(nil)

	// Теперь модальных нет — клик на root кнопке должен работать
	rootClicked := false
	rootBtn.OnClick = func() { rootClicked = true }
	eng.SendMouseButton(100, 30, widget.MouseLeft, true)
	eng.SendMouseButton(100, 30, widget.MouseLeft, false)
	if !rootClicked {
		t.Fatal("root button should be clickable after modal is closed")
	}
}

func TestEngine_Modal_BlocksRootInput(t *testing.T) {
	eng := engine.New(400, 300, 20)

	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	rootBtn := widget.NewButton("Root")
	rootBtn.SetBounds(image.Rect(10, 10, 200, 50))
	root.AddChild(rootBtn)
	eng.SetRoot(root)

	// Открываем модальный диалог поверх корня
	dlg := widget.NewDialog("Modal", 300, 200)
	eng.ShowModal(dlg)

	rootClicked := false
	rootBtn.OnClick = func() { rootClicked = true }

	// Клик на root button должен быть заблокирован модальным диалогом
	eng.SendMouseButton(100, 30, widget.MouseLeft, true)
	eng.SendMouseButton(100, 30, widget.MouseLeft, false)
	if rootClicked {
		t.Fatal("root button should be blocked while modal is active")
	}
}

func TestEngine_Escape_ClosesTopModal(t *testing.T) {
	eng := engine.New(400, 300, 20)

	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	rootBtn := widget.NewButton("Root")
	rootBtn.SetBounds(image.Rect(10, 10, 200, 50))
	root.AddChild(rootBtn)
	eng.SetRoot(root)

	dlg := widget.NewDialog("D", 300, 200)
	eng.ShowModal(dlg)

	// Escape должен закрыть диалог
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})

	// Теперь root кнопка должна получать события
	rootClicked := false
	rootBtn.OnClick = func() { rootClicked = true }
	eng.SendMouseButton(100, 30, widget.MouseLeft, true)
	eng.SendMouseButton(100, 30, widget.MouseLeft, false)
	if !rootClicked {
		t.Fatal("root button should work after Escape closes modal")
	}
}

func TestEngine_TabCycle_WithinModal(t *testing.T) {
	eng := engine.New(400, 300, 20)

	root := widget.NewPanel(widget.DarkTheme().PanelBG)
	root.SetBounds(image.Rect(0, 0, 400, 300))
	rootBtn := widget.NewButton("Root")
	rootBtn.SetBounds(image.Rect(10, 10, 100, 40))
	root.AddChild(rootBtn)
	eng.SetRoot(root)

	dlg := widget.NewDialog("D", 300, 200)
	dlgBtn1 := widget.NewButton("D1")
	dlgBtn2 := widget.NewButton("D2")
	dlgBtn1.SetBounds(image.Rect(10, 50, 100, 80))
	dlgBtn2.SetBounds(image.Rect(10, 90, 100, 120))
	dlg.AddChild(dlgBtn1)
	dlg.AddChild(dlgBtn2)
	eng.ShowModal(dlg)

	tab := widget.KeyEvent{Code: widget.KeyTab, Pressed: true}

	// Tab должен циклиться только внутри диалога, не затрагивая rootBtn
	eng.SendKeyEvent(tab) // dlgBtn1
	if rootBtn.IsFocused() {
		t.Fatal("Tab should not focus widgets outside modal")
	}
	if !dlgBtn1.IsFocused() {
		t.Fatal("first Tab should focus dlgBtn1")
	}

	eng.SendKeyEvent(tab) // dlgBtn2
	if !dlgBtn2.IsFocused() {
		t.Fatal("second Tab should focus dlgBtn2")
	}
	if dlgBtn1.IsFocused() {
		t.Fatal("dlgBtn1 should lose focus")
	}
}

// ─── MessageBox с реальным Engine ───────────────────────────────────────────

func TestEngine_MessageBox_ShowAndClose(t *testing.T) {
	eng := engine.New(400, 300, 20)
	mb := widget.NewMessageBox(eng)

	resultCh := make(chan widget.MessageBoxResult, 1)
	dlg := mb.ShowYesNo("Вопрос", "Продолжить?", func(r widget.MessageBoxResult) {
		resultCh <- r
	})

	// Диалог должен быть показан и центрирован
	b := dlg.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		t.Fatal("dialog should have positive dimensions after ShowModal")
	}

	// Находим кнопки и нажимаем "Да" (первая)
	buttons := collectButtons(dlg)
	if len(buttons) < 2 {
		t.Fatalf("ShowYesNo should have 2 buttons, got %d", len(buttons))
	}
	// Нажимаем кнопку напрямую (без симуляции мыши)
	if buttons[0].OnClick != nil {
		buttons[0].OnClick()
	}

	select {
	case got := <-resultCh:
		if got != widget.MBResultYes {
			t.Fatalf("expected MBResultYes, got %d", got)
		}
	default:
		t.Fatal("callback not called")
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// collectButtons собирает все *widget.Button из прямых потомков w.
func collectButtons(w widget.Widget) []*widget.Button {
	var btns []*widget.Button
	for _, ch := range w.Children() {
		if btn, ok := ch.(*widget.Button); ok {
			btns = append(btns, btn)
		}
	}
	return btns
}

// collectLabels собирает все *widget.Label из прямых потомков w.
func collectLabels(w widget.Widget) []*widget.Label {
	var lbls []*widget.Label
	for _, ch := range w.Children() {
		if lbl, ok := ch.(*widget.Label); ok {
			lbls = append(lbls, lbl)
		}
	}
	return lbls
}

