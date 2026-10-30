package text

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget/base"
)

// TestAccessTreeHidesPassword — пароль не попадает в семантику.
func TestAccessTreeHidesPassword(t *testing.T) {
	ti := NewTextInput("")
	ti.SetBounds(image.Rect(0, 0, 100, 30))
	ti.SetPasswordMode(true)
	ti.SetText("hunter2")

	n := base.BuildAccessTree(ti, nil)
	if n == nil {
		t.Fatal("узел не построен")
	}
	if n.Value != "" {
		t.Errorf("Value=%q, ожидалось пусто", n.Value)
	}
	found := false
	for _, s := range n.States {
		if s == base.StatePassword {
			found = true
		}
	}
	if !found {
		t.Error("нет состояния password")
	}

	ti.SetPasswordMode(false)
	n = base.BuildAccessTree(ti, nil)
	if n.Value != "hunter2" {
		t.Errorf("обычный режим: Value=%q", n.Value)
	}
}
