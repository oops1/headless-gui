package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// GG-82: о закрытии модалки узнаёт не только хост нативных окон.
//
// SetOnModalClosed хранит ОДИН колбэк, и он занят хостом: приложение, которое
// подписывалось им, вытесняло хост или вытеснялось само. А диалог в собственном
// окне ОС закрывается мимо CloseModal главного движка — его закрывает
// вторичный движок окна, — и приложение о закрытии не узнавало вовсе.

func modalTestEngine(t *testing.T) (*Engine, *widget.Dialog) {
	t.Helper()
	e := New(400, 300, 20)
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 300))
	e.SetRoot(root)
	dlg := widget.NewDialog("Вопрос", 200, 120)
	return e, dlg
}

// Подписок может быть сколько угодно, и слот хоста их не вытесняет.
func TestModalClosed_MultipleSubscribers(t *testing.T) {
	e, dlg := modalTestEngine(t)

	var order []string
	e.SetOnModalClosed(func(widget.ModalWidget) { order = append(order, "хост") })
	first := e.AddOnModalClosed(func(widget.ModalWidget) { order = append(order, "первый") })
	e.AddOnModalClosed(func(widget.ModalWidget) { order = append(order, "второй") })
	dlg.OnClosed = func() { order = append(order, "диалог") }

	e.ShowModal(dlg)
	e.CloseModal(dlg)

	want := []string{"хост", "первый", "второй", "диалог"}
	if len(order) != len(want) {
		t.Fatalf("колбэки: %v, ждал %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("колбэки: %v, ждал %v", order, want)
		}
	}

	// Отписка снимает своего и не трогает остальных.
	order = nil
	e.RemoveOnModalClosed(first)
	e.ShowModal(dlg)
	e.CloseModal(dlg)
	for _, s := range order {
		if s == "первый" {
			t.Fatalf("отписанный колбэк всё ещё зовётся: %v", order)
		}
	}
	if len(order) != 3 {
		t.Fatalf("после отписки колбэков %d: %v", len(order), order)
	}
}

// NotifyModalClosed — путь хоста нативных окон: диалог закрыт вместе со своим
// окном ОС, CloseModal этого движка не звали.
func TestModalClosed_NotifyFromNativeHost(t *testing.T) {
	e, dlg := modalTestEngine(t)

	closed := 0
	e.AddOnModalClosed(func(m widget.ModalWidget) {
		if m == widget.ModalWidget(dlg) {
			closed++
		}
	})
	dialogClosed := 0
	dlg.OnClosed = func() { dialogClosed++ }

	e.NotifyModalClosed(dlg)

	if closed != 1 || dialogClosed != 1 {
		t.Fatalf("подписчик позван %d раз, OnClosed — %d, ждал по разу", closed, dialogClosed)
	}
}
