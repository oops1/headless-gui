package text

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget/xaml"
)

// Теги ввода текста строятся строителями пакета после RegisterXAML: поле,
// пароль и многострочный редактор со свойствами разметки. Без регистрации
// реестр тегов не знает — загрузка падает понятной ошибкой.
func TestRegisterXAML_TextTags(t *testing.T) {
	r := xaml.Default()
	tags := []string{"textbox", "textinput", "input", "richtextbox", "passwordbox"}
	prev := map[string]xaml.Builder{}
	for _, tag := range tags {
		if b, ok := r.Lookup(tag); ok {
			prev[tag] = b
		}
	}
	t.Cleanup(func() {
		r.Unregister(tags...)
		for tag, b := range prev {
			r.Register(b, tag)
		}
	})

	r.Unregister(tags...)
	if _, _, err := xaml.LoadUIFromXAML([]byte(`<TextBox/>`)); err == nil {
		t.Fatal("без RegisterXAML тег <TextBox> построился")
	}

	RegisterXAML(r)
	_, reg, err := xaml.LoadUIFromXAML([]byte(`<StackPanel>
  <TextBox Name="one" Text="abc" Placeholder="Имя" Foreground="#102030" MaxLength="5"/>
  <TextBox Name="multi" AcceptsReturn="True" IsReadOnly="True" Text="x"/>
  <PasswordBox Name="pw"/>
</StackPanel>`))
	if err != nil {
		t.Fatal(err)
	}
	one, ok := reg["one"].(*TextInput)
	if !ok {
		t.Fatalf("<TextBox> без AcceptsReturn — %T, ждали *TextInput", reg["one"])
	}
	if one.GetText() != "abc" || one.Placeholder != "Имя" || one.MaxLength != 5 ||
		one.TextColor != (color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 255}) {
		t.Errorf("свойства поля: текст %q, подсказка %q, MaxLength %d, цвет %v",
			one.GetText(), one.Placeholder, one.MaxLength, one.TextColor)
	}
	multi, ok := reg["multi"].(*TextBox)
	if !ok || !multi.ReadOnly || multi.GetText() != "x" {
		t.Errorf("<TextBox AcceptsReturn>: %T, только чтение %v", reg["multi"], ok && multi.ReadOnly)
	}
	pw, ok := reg["pw"].(*TextInput)
	if !ok || !pw.IsPasswordMode() || pw.Placeholder != "Пароль" {
		t.Errorf("<PasswordBox>: %T, пароль %v", reg["pw"], ok && pw.IsPasswordMode())
	}
}
