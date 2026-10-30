package engine

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/widget"
)

// keyRoot — корень приложения, ловящий клавиши без фокуса.
type keyRoot struct {
	*widget.Panel
	got []widget.KeyEvent
}

func (r *keyRoot) OnUnfocusedKey(ev widget.KeyEvent) bool {
	r.got = append(r.got, ev)
	return true
}

// После щелчка по пустому месту фокуса нет ни у кого, и клавиши раньше
// пропадали. Корень с UnfocusedKeyHandler получает их целиком — с руной и
// отпусканием (замечание проекта calc на v3.39.0). Когда фокус есть, корень
// клавиш не видит.
func TestUnfocusedKey_GoesToRoot(t *testing.T) {
	testreset.Globals(t)
	e := New(300, 200, 30)
	defer e.Stop()
	root := &keyRoot{Panel: widget.NewPanel(widget.CurrentTheme().PanelBG)}
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 300, 200))
	in := widget.NewTextInput("")
	in.SetBounds(image.Rect(10, 10, 200, 40))
	root.AddChild(in)
	e.SetRoot(root)

	e.SendMouseButton(250, 150, widget.MouseLeft, true) // пустое место — фокус снят
	e.SendMouseButton(250, 150, widget.MouseLeft, false)
	e.SendKeyEvent(widget.KeyEvent{Rune: '7', Pressed: true})
	e.SendKeyEvent(widget.KeyEvent{Rune: '7'})
	if len(root.got) != 2 || root.got[0].Rune != '7' || !root.got[0].Pressed || root.got[1].Pressed {
		t.Fatalf("корень получил %+v, ждали нажатие и отпускание '7'", root.got)
	}

	e.SetFocus(in)
	n := len(root.got)
	e.SendKeyEvent(widget.KeyEvent{Rune: '8', Pressed: true})
	if len(root.got) != n {
		t.Fatalf("клавиша при фокусе ушла корню: %+v", root.got[n:])
	}
	if in.GetText() != "8" {
		t.Fatalf("поле с фокусом не получило клавишу: %q", in.GetText())
	}
}
