package tests

import (
	"bytes"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Байтовый доступ к паролю — запрос GG-36.
//
// GetText() отдавал секрет НЕИЗМЕНЯЕМОЙ Go-строкой: занулить её нельзя ничем,
// она живёт в куче до сборки мусора и попадает в дампы памяти. Приложению с
// правилом «секрет не превращается в string» приходилось нарушать это правило
// ровно на границе с движком.

func typeInto(ti *widget.TextInput, s string) {
	ti.SetFocused(true)
	for _, r := range s {
		ti.OnKeyEvent(widget.KeyEvent{Rune: r, Pressed: true})
	}
}

func TestTakeSecret_ReturnsBytesAndClearsField(t *testing.T) {
	ti := widget.NewPasswordInput("пароль")
	typeInto(ti, "тайна42")

	got := ti.TakeSecret()
	if !bytes.Equal(got, []byte("тайна42")) {
		t.Errorf("TakeSecret вернул %q", got)
	}
	if left := ti.GetText(); left != "" {
		t.Errorf("после TakeSecret в поле осталось %q", left)
	}
	// Повторный вызов даёт пустой, а не nil, срез: вызывающему не приходится
	// различать «ничего не ввели» и «что-то пошло не так».
	again := ti.TakeSecret()
	if again == nil {
		t.Error("повторный TakeSecret вернул nil вместо пустого среза")
	}
	if len(again) != 0 {
		t.Errorf("повторный TakeSecret вернул %q", again)
	}
}

// Срез принадлежит вызывающему: зануление его не ломает виджет и не трогает
// ничего чужого.
func TestTakeSecret_CallerOwnsTheBytes(t *testing.T) {
	ti := widget.NewPasswordInput("пароль")
	typeInto(ti, "abc")

	b := ti.TakeSecret()
	clearBytes(b)
	for i, v := range b {
		if v != 0 {
			t.Fatalf("байт %d не занулился: %d", i, v)
		}
	}
	typeInto(ti, "xyz")
	if got := ti.GetText(); got != "xyz" {
		t.Errorf("после зануления среза поле ведёт себя как %q", got)
	}
}

// Работает и на обычном поле: режим пароля прячет символы на экране, а к
// способу чтения отношения не имеет.
func TestTakeSecret_WorksOnPlainInput(t *testing.T) {
	ti := widget.NewTextInput("")
	typeInto(ti, "открытым текстом")

	if got := ti.TakeSecret(); !bytes.Equal(got, []byte("открытым текстом")) {
		t.Errorf("TakeSecret на обычном поле вернул %q", got)
	}
}

// WipeSecret стирает, ничего не возвращая, — на отмене диалога.
func TestWipeSecret_ClearsPlainInputToo(t *testing.T) {
	ti := widget.NewTextInput("")
	typeInto(ti, "секрет")

	ti.WipeSecret()
	if got := ti.GetText(); got != "" {
		t.Errorf("после WipeSecret в поле осталось %q", got)
	}
}

func clearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
