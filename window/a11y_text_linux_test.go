//go:build linux && !android

package window

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/widget"
)

// Содержимое поля ввода уезжало к скринридеру одной строкой — «значением»
// элемента, как у ползунка. Для кнопки довольно, для текста — нет: Orca
// читает документ кусками, идёт за кареткой, озвучивает выделение. Без
// интерфейса Text она объявляла поле пустым.

// textNode собирает узел снимка для виджета (как это делает мост).
func textNode(t *testing.T, w widget.Widget) *a11yNode {
	t.Helper()
	root := widget.NewPanel(widget.Theme{}.WindowBG)
	root.SetBounds(image.Rect(0, 0, 300, 200))
	root.AddChild(w)
	eng := engine.New(300, 200, 30)
	eng.SetRoot(root)
	eng.SetFocus(w)

	snap := a11yFlatten(widget.BuildAccessTree(root, w))
	for i := range snap.Nodes {
		if snap.Nodes[i].Widget == w {
			return &snap.Nodes[i]
		}
	}
	t.Fatal("узел виджета не найден в снимке")
	return nil
}

func textBoxNode(t *testing.T, text string) (*a11yNode, *widget.TextBox) {
	t.Helper()
	tb := widget.NewTextBox("")
	tb.SetBounds(image.Rect(10, 10, 280, 150))
	tb.SetText(text)
	return textNode(t, tb), tb
}

// callText — вызов метода org.a11y.atspi.Text у узла.
func callText(b *atspiBridge, node *a11yNode, member string, args ...any) *dbusReply {
	return b.handleText(&dbusMessage{Interface: ifaceText, Member: member, Body: args}, node)
}

func TestATSPIText_ReadsContent(t *testing.T) {
	b := &atspiBridge{}
	node, _ := textBoxNode(t, "первая строка\nвторая строка")

	rep := callText(b, node, "GetText", int32(0), int32(-1))
	if rep == nil || rep.Body[0].(string) != "первая строка\nвторая строка" {
		t.Fatalf("GetText вернул %#v", rep)
	}
	// -1 означает «до конца»: так его шлёт libatspi.
	if got := callText(b, node, "GetText", int32(0), int32(6)).Body[0].(string); got != "первая" {
		t.Errorf("отрезок %q", got)
	}
	if n := callText(b, node, "GetCharacterCount").Body[0].(int32); n != 27 {
		t.Errorf("символов %d, ждал 27 (счёт в рунах, не в байтах)", n)
	}
}

// Текст спрашивается у ЖИВОГО виджета, а не из снимка семантики: снимок
// пересобирается раз в сто пятьдесят миллисекунд и отставал бы от набора.
func TestATSPIText_FollowsWidget(t *testing.T) {
	b := &atspiBridge{}
	node, tb := textBoxNode(t, "старое")

	tb.SetText("новое содержимое")
	if got := callText(b, node, "GetText", int32(0), int32(-1)).Body[0].(string); got != "новое содержимое" {
		t.Errorf("прочитано %q — мост отдал устаревший снимок", got)
	}
}

func TestATSPIText_CaretAndSelection(t *testing.T) {
	b := &atspiBridge{}
	node, tb := textBoxNode(t, "первая строка\nвторая строка")

	tb.SetCaretPosition(5)
	if off := callText(b, node, "GetCaretOffset").Body[0].(int32); off != 5 {
		t.Errorf("каретка %d, ждал 5", off)
	}
	if n := callText(b, node, "GetNSelections").Body[0].(int32); n != 0 {
		t.Errorf("выделений %d без выделения", n)
	}

	// Выделение «всё» — тем же путём, каким его делает человек.
	tb.SetFocused(true)
	tb.OnKeyEvent(widget.KeyEvent{Code: widget.KeyA, Mod: widget.ModCtrl, Pressed: true})
	if n := callText(b, node, "GetNSelections").Body[0].(int32); n != 1 {
		t.Fatalf("после выделения всего выделений %d", n)
	}
	sel := callText(b, node, "GetSelection", int32(0))
	if sel.Body[0].(int32) != 0 || sel.Body[1].(int32) != 27 {
		t.Errorf("выделение %v..%v, ждал 0..27", sel.Body[0], sel.Body[1])
	}
}

// Скринридер идёт по тексту кусками: строка вокруг каретки, слово, символ.
func TestATSPIText_Slices(t *testing.T) {
	b := &atspiBridge{}
	node, _ := textBoxNode(t, "первая строка\nвторая строка")

	rep := callText(b, node, "GetStringAtOffset", int32(16), uint32(a11yGranLine))
	if rep.Body[0].(string) != "вторая строка" {
		t.Errorf("строка %q", rep.Body[0])
	}
	if rep.Body[1].(int32) != 14 || rep.Body[2].(int32) != 27 {
		t.Errorf("границы строки %v..%v, ждал 14..27", rep.Body[1], rep.Body[2])
	}

	// Старый вид запроса (вид границы вместо зернистости) — им до сих пор
	// пользуется libatspi.
	rep = callText(b, node, "GetTextAtOffset", int32(2), uint32(1))
	if rep.Body[0].(string) != "первая" {
		t.Errorf("слово %q", rep.Body[0])
	}

	if ch := callText(b, node, "GetCharacterAtOffset", int32(0)).Body[0].(int32); ch != 'п' {
		t.Errorf("символ %q", rune(ch))
	}
}

// Поле пароля читать вслух нельзя: роль уже сказала, что это пароль.
func TestATSPIText_PasswordNotRead(t *testing.T) {
	b := &atspiBridge{}
	ti := widget.NewTextInput("")
	ti.SetBounds(image.Rect(10, 10, 200, 40))
	ti.SetText("секрет")
	ti.SetPasswordMode(true)
	node := textNode(t, ti)

	if got := callText(b, node, "GetText", int32(0), int32(-1)).Body[0].(string); got != "" {
		t.Errorf("пароль отдан скринридеру: %q", got)
	}
}

// Интерфейс объявляется только там, где он есть: кнопке Text ни к чему, а без
// него клиент и не спросит.
func TestATSPIText_InterfaceAdvertised(t *testing.T) {
	b := &atspiBridge{}

	node, _ := textBoxNode(t, "текст")
	if !hasIface(b.interfacesOf(1, node), ifaceText) {
		t.Error("у многострочного текста нет интерфейса Text")
	}

	btn := widget.NewButton("Кнопка")
	btn.SetBounds(image.Rect(10, 10, 100, 40))
	if hasIface(b.interfacesOf(2, textNode(t, btn)), ifaceText) {
		t.Error("у кнопки появился интерфейс Text")
	}
}

func hasIface(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// Многострочный текст в AT-SPI — «text», однострочное поле — «entry»: по
// этому различию скринридер решает, читать содержимое построчно или одной
// подписью.
func TestATSPIText_DocumentRole(t *testing.T) {
	if got := atspiRoleOf(widget.RoleDocument); got != atspiRoleText {
		t.Errorf("роль документа %d, ждал %d", got, atspiRoleText)
	}
	if got := atspiRoleOf(widget.RoleTextInput); got != atspiRoleEntry {
		t.Errorf("роль поля ввода %d", got)
	}
}
