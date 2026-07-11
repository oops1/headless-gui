package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// fakeHost — тестовая реализация ModalHost: accept управляет тем, принимает
// ли хост модалку; shown/closed фиксируют маршрутизацию.
type fakeHost struct {
	accept bool
	shown  widget.ModalWidget
	closed widget.ModalWidget
}

func (f *fakeHost) ShowModal(m widget.ModalWidget) bool {
	if !f.accept {
		return false
	}
	f.shown = m
	return true
}

func (f *fakeHost) CloseModal(m widget.ModalWidget) bool {
	if !f.accept {
		return false
	}
	f.closed = m
	return true
}

func newHostEngine() *Engine {
	e := New(600, 400, 20)
	e.SetTooltipsEnabled(false)
	root := widget.NewPanel(color.RGBA{R: 30, G: 30, B: 30, A: 255})
	root.SetBounds(image.Rect(0, 0, 600, 400))
	e.SetRoot(root)
	return e
}

// Хост принял модалку → она НЕ попадает в стек движка, ввод родителя не
// блокируется, а CloseModal маршрутизируется хосту.
func TestModalHost_Accepts(t *testing.T) {
	e := newHostEngine()

	// Кнопка в корне: проверим, что клик по ней проходит (родитель не заблокирован).
	clicked := false
	btn := widget.NewButton("root")
	btn.SetBounds(image.Rect(10, 10, 110, 40))
	btn.OnClick = func() { clicked = true }
	e.Root().(*widget.Panel).AddChild(btn)

	host := &fakeHost{accept: true}
	e.SetModalHost(host)

	dlg := widget.NewDialog("hosted", 300, 200)
	e.ShowModal(dlg)

	if host.shown != dlg {
		t.Fatalf("ShowModal не маршрутизирован хосту: shown=%v", host.shown)
	}
	if e.topModal() != nil {
		t.Fatalf("хостируемая модалка не должна попадать в стек движка")
	}

	// Ввод родителя не блокируется — клик по кнопке в корне срабатывает.
	e.SendMouseButton(20, 20, widget.MouseLeft, true)
	e.SendMouseButton(20, 20, widget.MouseLeft, false)
	if !clicked {
		t.Fatal("клик по корневой кнопке не прошёл — ввод родителя заблокирован")
	}

	// CloseModal уходит хосту.
	e.CloseModal(dlg)
	if host.closed != dlg {
		t.Fatalf("CloseModal не маршрутизирован хосту: closed=%v", host.closed)
	}
}

// Хост отказал → прежнее in-canvas поведение: модалка в стеке, центрирована
// (клэмп для больших диалогов проверяется отдельно в tests/dialogs_test.go).
func TestModalHost_Declines(t *testing.T) {
	e := newHostEngine()
	host := &fakeHost{accept: false}
	e.SetModalHost(host)

	dlg := widget.NewDialog("in-canvas", 300, 200)
	e.ShowModal(dlg)

	if e.topModal() != dlg {
		t.Fatal("при отказе хоста модалка должна быть в стеке движка")
	}
	// Центрирование под холст 600×400.
	if b := dlg.Bounds(); b.Min.X != 150 || b.Min.Y != 100 {
		t.Fatalf("модалка не отцентрирована: %v", b)
	}
	e.CloseModal(dlg)
	if e.topModal() != nil {
		t.Fatal("CloseModal должен убрать модалку из стека")
	}
}

// SetOnModalClosed: колбэк вызывается в конце CloseModal с фактически закрытой
// модалкой (путь вторичного движка: диалог закрывается своим closer'ом).
func TestOnModalClosed_Fires(t *testing.T) {
	e := newHostEngine() // без хоста → in-canvas
	var got widget.ModalWidget
	e.SetOnModalClosed(func(m widget.ModalWidget) { got = m })

	dlg := widget.NewDialog("d", 200, 120)
	e.ShowModal(dlg)
	e.CloseModal(dlg)
	if got != dlg {
		t.Fatalf("OnModalClosed не вызван с закрытой модалкой: got=%v", got)
	}

	// Закрытие «верхней» (nil) тоже сообщает фактически закрытую модалку.
	got = nil
	dlg2 := widget.NewDialog("d2", 200, 120)
	e.ShowModal(dlg2)
	e.CloseModal(nil)
	if got != dlg2 {
		t.Fatalf("OnModalClosed(nil-close) не сообщил верхнюю модалку: got=%v", got)
	}
}
