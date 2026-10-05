package widget

import (
	"strings"
	"testing"
)

type fakeModals struct{ shown []ModalWidget }

func (f *fakeModals) ShowModal(m ModalWidget)  { f.shown = append(f.shown, m) }
func (f *fakeModals) CloseModal(m ModalWidget) {}

func inputWidth(t *testing.T, title, label, initial string, width int) int {
	t.Helper()
	fm := &fakeModals{}
	mb := NewMessageBox(fm)
	var id *InputDialog
	if width == 0 {
		id = mb.ShowInput(title, label, initial, nil, nil)
	} else {
		id = mb.ShowInputWidth(title, label, initial, nil, nil, width)
	}
	if len(fm.shown) != 1 {
		t.Fatal("диалог не показан")
	}
	return id.Dialog().Bounds().Dx()
}

// Ширина ShowInput следует за содержимым: короткое остаётся прежних 380,
// длинная подпись или начальный текст раздвигают диалог до предела.
func TestShowInput_WidthFollowsContent(t *testing.T) {
	if w := inputWidth(t, "Имя", "Имя:", "a.txt", 0); w != inputDlgMinW {
		t.Errorf("короткое содержимое: ширина %d, ждали прежние %d", w, inputDlgMinW)
	}
	long := strings.Repeat("Очень длинная подпись поля ввода. ", 3)
	if w := inputWidth(t, "Имя", long, "x", 0); w <= inputDlgMinW || w > inputDlgMaxW {
		t.Errorf("длинная подпись: ширина %d, ждали между %d и %d", w, inputDlgMinW, inputDlgMaxW)
	}
	path := `C:\Users\someone\Documents\Projects\Some Long Project Name\subfolder\file.txt`
	if w := inputWidth(t, "Имя", "Путь:", path, 0); w <= inputDlgMinW {
		t.Errorf("длинный начальный текст: ширина %d не выросла", w)
	}
	huge := strings.Repeat("W", 400)
	if w := inputWidth(t, "Имя", huge, huge, 0); w != inputDlgMaxW {
		t.Errorf("предел: ширина %d, ждали %d", w, inputDlgMaxW)
	}
}

func TestShowInputWidth_Explicit(t *testing.T) {
	if w := inputWidth(t, "Имя", "Имя:", "", 520); w != 520 {
		t.Errorf("явная ширина: %d, ждали 520", w)
	}
	if w := inputWidth(t, "Имя", "Имя:", "", 50); w != 200 {
		t.Errorf("слишком узкая ширина должна подняться до 200, а не %d", w)
	}
}
