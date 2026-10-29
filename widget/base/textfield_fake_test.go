package base

// textfield_fake_test.go — поле ввода для тестов ядра. Настоящие TextInput и
// TextBox живут в widget/text, а внутренние тесты base его импортировать не
// могут (цикл); диалогам ядра (ввод строки, имя файла) поле нужно — здесь
// простая замена с тем же контрактом TextEditor.

import (
	"image/color"
	"sync"
)

type fakeTextField struct {
	Base
	mu          sync.Mutex
	text        string
	placeholder string
	textColor   color.RGBA
	background  color.RGBA
	onChange    func(string)
	validation  string
}

func (f *fakeTextField) Draw(ctx DrawContext)      { f.DrawChildren(ctx) }
func (f *fakeTextField) ApplyTheme(t *Theme)       {}
func (f *fakeTextField) SetPlaceholder(s string)   { f.placeholder = s }
func (f *fakeTextField) SetTextColor(c color.RGBA) { f.textColor = c }

func (f *fakeTextField) SetBackgroundColor(c color.RGBA)  { f.background = c }
func (f *fakeTextField) OnTextChanged() func(string)      { return f.onChange }
func (f *fakeTextField) SetOnTextChanged(fn func(string)) { f.onChange = fn }
func (f *fakeTextField) SetValidationError(msg string)    { f.validation = msg }
func (f *fakeTextField) ValidationError() string          { return f.validation }

func (f *fakeTextField) GetText() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.text
}

func (f *fakeTextField) SetText(s string) {
	f.mu.Lock()
	f.text = s
	f.mu.Unlock()
}

func init() {
	SetTextInputFactory(func(placeholder string) TextEditor {
		return &fakeTextField{placeholder: placeholder}
	})
}
