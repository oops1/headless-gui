package engine

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// actionOwner — виджет, рисующий два узла сам и принимающий действия над ними.
// Фокусируемый: действие, выбирающее узел, обязано перевести на него фокус.
type actionOwner struct {
	widget.Base
	focused bool
	reply   bool
	calls   []string
	// goid — горутина, на которой пришло последнее действие.
	onLoop bool
	eng    *Engine
}

func (o *actionOwner) Draw(widget.DrawContext) {}

func (o *actionOwner) AccessChildren() []widget.AccessInfo {
	b := o.Bounds()
	mid := (b.Min.X + b.Max.X) / 2
	return []widget.AccessInfo{
		{Role: widget.RoleTextInput, Name: "левая", ID: "L", Bounds: image.Rect(b.Min.X, b.Min.Y, mid, b.Max.Y)},
		{Role: widget.RoleTextInput, Name: "правая", ID: "R", Bounds: image.Rect(mid, b.Min.Y, b.Max.X, b.Max.Y)},
	}
}

func (o *actionOwner) AccessAction(id string, action widget.AccessActionKind) bool {
	o.calls = append(o.calls, id+":"+action.String())
	o.onLoop = o.eng.loopRunning() && o.eng.lp.GoroutineID() == curGoroutineID()
	return o.reply
}

func (o *actionOwner) SetFocused(v bool) { o.focused = v }
func (o *actionOwner) IsFocused() bool   { return o.focused }

// overlayOwner — то же, но владелец — оверлей (меню): фокус ему не передаётся.
type overlayOwner struct{ actionOwner }

func (o *overlayOwner) HasOverlay() bool               { return true }
func (o *overlayOwner) DrawOverlay(widget.DrawContext) {}

func actionScene(owner widget.Widget) (*Engine, *widget.Panel) {
	eng := New(400, 300, 20)
	root := widget.NewPanel(color.RGBA{A: 255})
	root.SetBounds(image.Rect(0, 0, 400, 300))
	owner.SetBounds(image.Rect(10, 10, 210, 110))
	root.AddChild(owner)
	eng.SetRoot(root)
	return eng, root
}

// Действие доходит до обработчика владельца с идентификатором узла, а не
// превращается в клик по центру владельца.
func TestAccessAction_ReachesNode(t *testing.T) {
	testreset.Globals(t)
	o := &actionOwner{reply: true}
	eng, _ := actionScene(o)
	o.eng = eng

	if !eng.AccessAction(o, "R", widget.AccessInvoke) {
		t.Fatal("AccessAction вернул false при согласии обработчика")
	}
	if len(o.calls) != 1 || o.calls[0] != "R:invoke" {
		t.Fatalf("обработчик получил %v, ожидалось [R:invoke]", o.calls)
	}
	// Узлы дерева несут то, чем адресовать действие.
	tree := eng.AccessibilityTree()
	r := accessFindByName(tree, "правая")
	if r == nil || r.ActionID != "R" || r.Widget != widget.Widget(o) {
		t.Fatalf("узел «правая»: %+v", r)
	}
	if !eng.AccessAction(r.Widget, r.ActionID, widget.AccessSelect) || o.calls[len(o.calls)-1] != "R:select" {
		t.Errorf("действие по узлу дерева: %v", o.calls)
	}
}

// Ответ обработчика окончательный, а действие идёт на горутине движка.
func TestAccessAction_AnswerAndGoroutine(t *testing.T) {
	testreset.Globals(t)
	o := &actionOwner{reply: false}
	eng, _ := actionScene(o)
	o.eng = eng
	eng.Start()
	defer eng.Stop()

	if eng.AccessAction(o, "L", widget.AccessInvoke) {
		t.Error("отказ обработчика превращён в успех")
	}
	if !o.onLoop {
		t.Error("обработчик вызван не на горутине движка")
	}
	o.reply = true
	if !eng.AccessAction(o, "L", widget.AccessExpand) {
		t.Error("согласие обработчика потеряно")
	}
}

// Выбирающее узел действие переводит фокус ввода на владельца; прочие — нет;
// владелец-оверлей фокуса не получает.
func TestAccessAction_FocusesOwner(t *testing.T) {
	testreset.Globals(t)
	o := &actionOwner{reply: true}
	eng, _ := actionScene(o)
	o.eng = eng

	if eng.AccessAction(o, "L", widget.AccessToggle); o.focused {
		t.Error("Toggle не должен двигать фокус")
	}
	for _, act := range []widget.AccessActionKind{widget.AccessInvoke, widget.AccessFocus, widget.AccessSelect} {
		o.focused = false
		eng.SetFocus(nil)
		if !eng.AccessAction(o, "L", act) {
			t.Fatalf("%v отвергнуто", act)
		}
		if !o.focused || eng.in.Focused() != widget.Widget(o) {
			t.Errorf("%v: фокус на владельце не получен", act)
		}
	}

	ov := &overlayOwner{actionOwner{reply: true}}
	eng2, _ := actionScene(ov)
	ov.eng = eng2
	if !eng2.AccessAction(ov, "L", widget.AccessFocus) || ov.focused {
		t.Errorf("владелец-оверлей получил фокус ввода: %v", ov.focused)
	}
	if len(ov.calls) != 1 {
		t.Errorf("обработчик оверлея не вызван: %v", ov.calls)
	}
}

// Скрытому, выключенному владельцу, не-владельцу и nil действия не достаются.
func TestAccessAction_Rejects(t *testing.T) {
	testreset.Globals(t)
	o := &actionOwner{reply: true}
	eng, root := actionScene(o)
	o.eng = eng

	if eng.AccessAction(nil, "L", widget.AccessInvoke) {
		t.Error("nil принят")
	}
	btn := widget.NewButton("Кнопка")
	btn.SetBounds(image.Rect(10, 150, 110, 180))
	root.AddChild(btn)
	if eng.AccessAction(btn, "0", widget.AccessInvoke) {
		t.Error("виджет без AccessActionHandler принял действие")
	}
	o.SetVisible(false)
	if eng.AccessAction(o, "L", widget.AccessInvoke) {
		t.Error("скрытый владелец принял действие")
	}
	o.SetVisible(true)
	o.SetEnabled(false)
	if eng.AccessAction(o, "L", widget.AccessInvoke) {
		t.Error("выключенный владелец принял действие")
	}
	if len(o.calls) != 0 {
		t.Errorf("отказные вызовы дошли до обработчика: %v", o.calls)
	}
}
