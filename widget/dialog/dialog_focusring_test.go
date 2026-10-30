package dialog

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget/base"
)

// Все кнопки диалогов движка рисуют фокус общей рамкой: MessageBox, диалог
// подтверждения, диалог ввода.
func TestDialogButtons_UseSharedFocusRing(t *testing.T) {
	sh := &fakeShower{}
	mb := NewMessageBox(sh)
	for name, dlg := range map[string]*Dialog{
		"MessageBox":    mb.ShowYesNoCancel("Вопрос", "Сохранить?", nil),
		"Подтверждение": NewConfirmDialog("Подтверждение", "Удалить?", nil),
	} {
		n := 0
		for _, c := range dlg.Children() {
			if b, ok := c.(*base.Button); ok {
				n++
				if !b.FocusRing {
					t.Errorf("%s: кнопка %q без общей рамки фокуса", name, b.Text)
				}
			}
		}
		if n == 0 {
			t.Errorf("%s: кнопок в диалоге не найдено", name)
		}
	}
}
