package xaml

// fieldfake_test.go — поле ввода для тестов xaml. Настоящие поля живут в
// widget/text, а он импортирует xaml (регистрация тегов) — внутренние тесты
// xaml его импортировать не могут. Заглушка с контрактом base.TextEditor,
// зарегистрированная на теги ввода текста, проверяет то, что делает сам
// загрузчик: привязку, локализацию, свойства.

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget/base"
)

type fakeField struct {
	base.Base
	text        string
	placeholder string
	textColor   color.RGBA
	background  color.RGBA
	onChange    func(string)
}

func (f *fakeField) Draw(ctx base.DrawContext)        { f.DrawChildren(ctx) }
func (f *fakeField) ApplyTheme(*base.Theme)           {}
func (f *fakeField) GetText() string                  { return f.text }
func (f *fakeField) SetText(s string)                 { f.text = s }
func (f *fakeField) SetPlaceholder(s string)          { f.placeholder = s }
func (f *fakeField) SetTextColor(c color.RGBA)        { f.textColor = c }
func (f *fakeField) SetBackgroundColor(c color.RGBA)  { f.background = c }
func (f *fakeField) OnTextChanged() func(string)      { return f.onChange }
func (f *fakeField) SetOnTextChanged(fn func(string)) { f.onChange = fn }

var fieldTags = []string{"textbox", "textinput", "input", "richtextbox", "passwordbox"}

// useFakeField регистрирует заглушку на теги ввода текста на время теста.
func useFakeField(t *testing.T) {
	t.Helper()
	Default().Register(func(el *Element, _ *Loader) (base.Widget, error) {
		return &fakeField{
			text:        el.Attr("Text"),
			placeholder: el.Attr("Tag", "Placeholder", "PlaceholderText", "Hint"),
		}, nil
	}, fieldTags...)
	t.Cleanup(func() { Default().Unregister(fieldTags...) })
}
