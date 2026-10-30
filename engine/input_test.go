package engine

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/widget"
)

// input_test.go — стык движка и core/input (ADR-0011, 7.2).

// grabber — виджет, который берёт мышь на нажатии и запоминает выданный ему
// менеджер захвата.
type grabber struct {
	widget.Base
	cm widget.CaptureManager
}

func (g *grabber) Draw(widget.DrawContext)                    {}
func (g *grabber) WantsCapture(widget.MouseEvent) bool        { return true }
func (g *grabber) SetCaptureManager(cm widget.CaptureManager) { g.cm = cm }

// plainBox — контейнер без SetCaptureManager: менеджер детям не раздаёт (в
// отличие от Panel), как контейнер, который движок обходит при SetRoot.
type plainBox struct{ widget.Base }

func (b *plainBox) Draw(widget.DrawContext) {}

// Виджет, до которого injectCaptureManager при SetRoot не дошёл (добавлен
// позже), получает менеджер при первом нажатии — и это тот же движок, что
// раздаётся при SetRoot, а не Dispatcher ввода: виджеты достают из менеджера
// Post движка (DiffView), и без него обработчик чужой горутины трогал бы
// дерево мимо очереди. Регрессия ревью 7.2.
func TestInput_FallbackCaptureManagerIsEngine(t *testing.T) {
	testreset.Globals(t)
	e := New(200, 200, 20)
	defer e.Stop()
	e.SetTooltipsEnabled(false)
	root := &plainBox{}
	root.SetBounds(image.Rect(0, 0, 200, 200))
	e.SetRoot(root)

	g := &grabber{}
	g.SetBounds(image.Rect(20, 20, 80, 80))
	root.AddChild(g) // после SetRoot: инъекции при SetRoot не было
	if g.cm != nil {
		t.Fatal("менеджер выдан до нажатия — тест не проверяет запасной путь")
	}
	e.SendMouseButton(40, 40, widget.MouseLeft, true)
	e.SendMouseButton(40, 40, widget.MouseLeft, false)

	if g.cm != widget.CaptureManager(e) {
		t.Fatalf("запасная инъекция выдала %T, ждали сам движок (*Engine)", g.cm)
	}
	if _, ok := g.cm.(interface{ Post(func()) }); !ok {
		t.Error("у менеджера захвата нет Post движка")
	}
}
