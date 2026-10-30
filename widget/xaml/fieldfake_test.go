package xaml

// fieldfake_test.go — поле ввода для тестов xaml. Настоящие поля живут в
// widget/text, а он импортирует xaml (регистрация тегов) — внутренние тесты
// xaml его импортировать не могут. Заглушка с контрактом base.TextEditor,
// зарегистрированная на теги ввода текста, проверяет то, что делает сам
// загрузчик: привязку, локализацию, свойства.

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/widget/base"
)

type FakeField struct {
	base.Base
	text        string
	Placeholder string
	textColor   color.RGBA
	background  color.RGBA
	onChange    func(string)
}

func (f *FakeField) Draw(ctx base.DrawContext)        { f.DrawChildren(ctx) }
func (f *FakeField) ApplyTheme(*base.Theme)           {}
func (f *FakeField) GetText() string                  { return f.text }
func (f *FakeField) SetText(s string)                 { f.text = s }
func (f *FakeField) SetPlaceholder(s string)          { f.Placeholder = s }
func (f *FakeField) SetTextColor(c color.RGBA)        { f.textColor = c }
func (f *FakeField) SetBackgroundColor(c color.RGBA)  { f.background = c }
func (f *FakeField) OnTextChanged() func(string)      { return f.onChange }
func (f *FakeField) SetOnTextChanged(fn func(string)) { f.onChange = fn }

var fieldTags = []string{"textbox", "textinput", "input", "richtextbox", "passwordbox"}

// UseFakeField регистрирует заглушку на теги ввода текста на время теста.
func UseFakeField(t *testing.T) {
	t.Helper()
	Default().Register(func(el *Element, _ *Loader) (base.Widget, error) {
		return &FakeField{
			text:        el.Attr("Text"),
			Placeholder: el.Attr("Tag", "Placeholder", "PlaceholderText", "Hint"),
		}, nil
	}, fieldTags...)
	t.Cleanup(func() { Default().Unregister(fieldTags...) })
}
