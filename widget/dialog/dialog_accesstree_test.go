package dialog

import (
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
)

// fakeShower — движок, которому MessageBox показывает диалог.
type fakeShower struct{ shown []ModalWidget }

func (f *fakeShower) ShowModal(m ModalWidget)  { f.shown = append(f.shown, m) }
func (f *fakeShower) CloseModal(m ModalWidget) {}

// У контейнера диалога есть роль, имя — заголовок и описание — текст сообщения.
func TestMessageBox_DialogInTree(t *testing.T) {
	sh := &fakeShower{}
	dlg := NewMessageBox(sh).Show("Ошибка", "Файл не найден\nПроверьте путь")

	tree := base.BuildAccessTree(dlg, nil)
	if tree.Role != base.RoleDialog {
		t.Fatalf("роль диалога %q, ожидалась dialog", tree.Role)
	}
	if tree.Name != "Ошибка" {
		t.Errorf("имя диалога %q", tree.Name)
	}
	if !strings.Contains(tree.Description, "Файл не найден") || !strings.Contains(tree.Description, "Проверьте путь") {
		t.Errorf("описание диалога %q не содержит текст сообщения", tree.Description)
	}
	if tree.Bounds.Empty() {
		t.Error("у диалога нет границ")
	}
	// Кнопка ✕ в заголовке — кнопка с именем, а не безымянный элемент.
	if cl := accNodeNamed(tree, base.Tr("dlg.close")); cl == nil || cl.Role != base.RoleButton {
		t.Errorf("кнопка закрытия не названа: %+v", cl)
	}
	// Заголовок меняется вместе с полем — имя живёт по заголовку, не по копии.
	dlg.SetTitle("Другое")
	if base.BuildAccessTree(dlg, nil).Name != "Другое" {
		t.Error("имя диалога не следует за заголовком")
	}
}

// Описание можно задать вручную, иначе берётся подсказка; выключенный диалог
// помечается.
func TestDialog_AccessInfoFallbacks(t *testing.T) {
	d := NewDialog("Заголовок", 300, 200)
	if got := d.AccessInfo(); got.Description != "" {
		t.Errorf("описание без источника: %q", got.Description)
	}
	d.ToolTip = "подсказка"
	if got := d.AccessInfo().Description; got != "подсказка" {
		t.Errorf("описание из подсказки: %q", got)
	}
	d.Description = "своё"
	if got := d.AccessInfo().Description; got != "своё" {
		t.Errorf("явное описание: %q", got)
	}
	d.SetEnabled(false)
	if info := d.AccessInfo(); !accessHasState(info.States, base.StateDisabled) {
		t.Errorf("выключенный диалог без состояния disabled: %v", info.States)
	}
}

// NewConfirmDialog и диалог ввода тоже называют себя и описывают.
func TestOtherDialogs_Described(t *testing.T) {
	c := NewConfirmDialog("Подтверждение", "Удалить файл?", nil)
	if i := c.AccessInfo(); i.Name != "Подтверждение" || i.Description != "Удалить файл?" {
		t.Errorf("диалог подтверждения: %+v", i)
	}
}
