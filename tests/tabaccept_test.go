package tests

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Tab, забираемый виджетом, — пункт 2 замечаний difftool.
//
// Движок перехватывал Tab до доставки фокусному виджету и отдавал обходу
// фокуса: редактор кода табуляцию вставить не мог, его OnKeyEvent Tab не
// видел вовсе.

// codeEditor — фокусируемый виджет, считающий доставленные Tab.
type codeEditor struct {
	widget.Base
	focused bool
	accept  bool
	tabs    int
}

func (c *codeEditor) Draw(widget.DrawContext) {}
func (c *codeEditor) SetFocused(v bool)       { c.focused = v }
func (c *codeEditor) IsFocused() bool         { return c.focused }
func (c *codeEditor) AcceptsTab() bool        { return c.accept }
func (c *codeEditor) OnKeyEvent(e widget.KeyEvent) {
	if e.Code == widget.KeyTab && e.Pressed {
		c.tabs++
	}
}

func tabScene(t *testing.T, accept bool) (*engine.Engine, *codeEditor, *widget.TextInput) {
	t.Helper()
	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 300, 200))

	ed := &codeEditor{accept: accept}
	ed.SetBounds(image.Rect(10, 10, 290, 120))
	next := widget.NewTextInput("дальше")
	next.SetBounds(image.Rect(10, 140, 290, 170))
	root.AddChild(ed)
	root.AddChild(next)

	eng := engine.New(300, 200, 30)
	eng.SetRoot(root)
	eng.SetFocus(ed)
	return eng, ed, next
}

func pressTab(eng *engine.Engine, mod widget.KeyMod) {
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Mod: mod, Pressed: true})
	eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Mod: mod, Pressed: false})
}

// Виджет, принимающий Tab, получает его сам, и фокус остаётся на нём.
func TestTabAcceptor_TabGoesToWidget(t *testing.T) {
	eng, ed, next := tabScene(t, true)

	pressTab(eng, 0)

	if ed.tabs != 1 {
		t.Errorf("редактор получил %d нажатий Tab", ed.tabs)
	}
	if !ed.IsFocused() || next.IsFocused() {
		t.Error("фокус ушёл из редактора, забравшего Tab")
	}
}

// Ctrl+Tab остаётся навигацией: иначе из редактора не выйти с клавиатуры.
func TestTabAcceptor_CtrlTabNavigates(t *testing.T) {
	eng, ed, next := tabScene(t, true)

	pressTab(eng, widget.ModCtrl)

	if ed.tabs != 0 {
		t.Error("Ctrl+Tab доставлен редактору")
	}
	if !next.IsFocused() {
		t.Error("Ctrl+Tab не перевёл фокус")
	}
}

// Виджет вправе не принимать Tab (метод, а не флаг): тогда — прежняя
// навигация. Так же ведут себя все виджеты без интерфейса.
func TestTabAcceptor_DeclinedTabNavigates(t *testing.T) {
	eng, ed, next := tabScene(t, false)

	pressTab(eng, 0)

	if ed.tabs != 0 {
		t.Error("виджет, отказавшийся от Tab, получил его")
	}
	if !next.IsFocused() {
		t.Error("Tab не перевёл фокус")
	}
}
