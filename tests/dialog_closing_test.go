package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Право остановить закрытие модалки — запрос GG-50 (первый пункт с этим
// номером).
//
// Закрытие по Escape и ✕ шло через замыкание движка: оно звало OnCancel и
// СРАЗУ ЖЕ CloseModal, независимо от того, что обработчик успел сделать.
// CancelAction был уведомлением постфактум, а не правом вето, и диалог,
// показавший из него вопрос «Сохранить изменения?», всё равно выталкивался из
// стека, не дождавшись ответа.

func closingScene(t *testing.T) (*engine.Engine, *widget.Dialog) {
	t.Helper()
	eng := newDialogEngine()
	dlg := widget.NewDialog("Настройки", 300, 200)
	eng.ShowModal(dlg)
	return eng, dlg
}

// Возврат false останавливает закрытие: диалог остаётся в стеке.
func TestDialogClosing_VetoKeepsDialog(t *testing.T) {
	_, dlg := closingScene(t)

	asked := 0
	dlg.OnClosing = func() bool { asked++; return false }

	dlg.RequestClose() // как ✕

	if asked != 1 {
		t.Fatalf("хук спрошен %d раз", asked)
	}
	if !dlg.IsModal() {
		t.Error("диалог вытолкнут из стека, хотя закрытие остановлено")
	}
}

// Возврат true закрывает, как и раньше.
func TestDialogClosing_AllowCloses(t *testing.T) {
	_, dlg := closingScene(t)
	dlg.OnClosing = func() bool { return true }

	dlg.RequestClose()

	if dlg.IsModal() {
		t.Error("диалог остался в стеке, хотя закрытие разрешено")
	}
}

// Без хука поведение прежнее.
func TestDialogClosing_NoHookClosesAsBefore(t *testing.T) {
	_, dlg := closingScene(t)

	dlg.RequestClose()

	if dlg.IsModal() {
		t.Error("диалог без хука не закрылся")
	}
}

// CancelAction не зовётся при остановленном закрытии: «отмена» — состоявшееся
// действие, и сообщать о ней, когда её не было, нельзя.
func TestDialogClosing_VetoSkipsCancelAction(t *testing.T) {
	_, dlg := closingScene(t)

	canceled := 0
	dlg.CancelAction = func() { canceled++ }
	dlg.OnClosing = func() bool { return false }

	dlg.RequestClose()
	if canceled != 0 {
		t.Errorf("CancelAction вызван %d раз при остановленном закрытии", canceled)
	}

	// Разрешили — теперь и отмена состоялась.
	dlg.OnClosing = func() bool { return true }
	dlg.RequestClose()
	if canceled != 1 {
		t.Errorf("CancelAction вызван %d раз при разрешённом закрытии", canceled)
	}
}

// Escape идёт тем же путём, что и ✕.
func TestDialogClosing_VetoHoldsAgainstEscape(t *testing.T) {
	eng, dlg := closingScene(t)
	dlg.OnClosing = func() bool { return false }

	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})

	if !dlg.IsModal() {
		t.Error("Escape закрыл диалог вопреки вето")
	}
}

// Ради чего всё затевалось: из хука можно показать второй диалог, а первый
// дождётся ответа, оставаясь в стеке.
func TestDialogClosing_SecondDialogWaitsForAnswer(t *testing.T) {
	eng, dlg := closingScene(t)

	var question *widget.Dialog
	dlg.OnClosing = func() bool {
		question = widget.NewDialog("Сохранить изменения?", 260, 140)
		eng.ShowModal(question)
		return false
	}

	dlg.RequestClose()

	if question == nil {
		t.Fatal("вопрос не показан")
	}
	if !dlg.IsModal() {
		t.Error("диалог настроек вытолкнут из стека, пока вопрос без ответа")
	}
	if !question.IsModal() {
		t.Error("вопрос не в стеке модалок")
	}

	// Пользователь ответил «не закрывать» — просто закрываем вопрос.
	eng.CloseModal(question)
	if !dlg.IsModal() {
		t.Error("после закрытия вопроса диалог настроек исчез из стека")
	}

	// Пользователь ответил «закрыть» — приложение закрывает сам.
	dlg.OnClosing = nil
	eng.CloseModal(dlg)
	if dlg.IsModal() {
		t.Error("диалог настроек не закрылся по команде приложения")
	}
}
