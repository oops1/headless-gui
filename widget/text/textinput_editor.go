package text

// textinput_editor.go — TextInput и TextBox как TextEditor (textfield.go):
// методы-обёртки над полями, через которые привязка данных и диалоги
// работают с полем, не зная его типа; размер по умолчанию (DefaultSizer) и
// семантика доступности (AccessDescriber) — вместо веток по типу в ядре.

import (
	"image/color"

	"github.com/oops1/headless-gui/v3/widget/base"
)

// SetPlaceholder задаёт подсказку в пустом поле (поле Placeholder).
func (t *TextInput) SetPlaceholder(s string) { t.Placeholder = s }

// SetTextColor задаёт цвет текста (поле TextColor).
func (t *TextInput) SetTextColor(c color.RGBA) { t.TextColor = c }

// SetBackgroundColor задаёт цвет фона (поле Background).
func (t *TextInput) SetBackgroundColor(c color.RGBA) { t.Background = c }

// OnTextChanged возвращает обработчик изменения текста (поле OnChange).
func (t *TextInput) OnTextChanged() func(string) { return t.OnChange }

// SetOnTextChanged заменяет обработчик изменения текста (поле OnChange).
func (t *TextInput) SetOnTextChanged(fn func(string)) { t.OnChange = fn }

// SetPlaceholder задаёт подсказку в пустом поле (поле Placeholder).
func (t *TextBox) SetPlaceholder(s string) { t.Placeholder = s }

// SetTextColor задаёт цвет текста (поле TextColor).
func (t *TextBox) SetTextColor(c color.RGBA) { t.TextColor = c }

// SetBackgroundColor задаёт цвет фона (поле Background).
func (t *TextBox) SetBackgroundColor(c color.RGBA) { t.Background = c }

// OnTextChanged возвращает обработчик изменения текста (поле OnChange).
func (t *TextBox) OnTextChanged() func(string) { return t.OnChange }

// SetOnTextChanged заменяет обработчик изменения текста (поле OnChange).
func (t *TextBox) SetOnTextChanged(fn func(string)) { t.OnChange = fn }

var (
	_ base.TextEditor = (*TextInput)(nil)
	_ base.TextEditor = (*TextBox)(nil)
)

// DefaultSize — размер однострочного поля по умолчанию для авторазмера
// (DefaultSizer): 120×26.
func (t *TextInput) DefaultSize() (int, int) { return 120, 26 }

// DescribeAccess — семантика поля для доступности (AccessDescriber): поле
// ввода; у пароля — состояние «пароль» без значения.
func (t *TextInput) DescribeAccess(info *base.AccessInfo) {
	info.Role = base.RoleTextInput
	if t.IsPasswordMode() {
		info.States = append(info.States, base.StatePassword)
	} else {
		info.Value = t.GetText()
	}
}

// DescribeAccess — семантика редактора для доступности (AccessDescriber):
// документ со значением; только чтение — состояние.
func (t *TextBox) DescribeAccess(info *base.AccessInfo) {
	info.Role = base.RoleDocument
	info.Value = t.GetText()
	if t.ReadOnly {
		info.States = append(info.States, base.StateReadOnly)
	}
}

var (
	_ base.DefaultSizer    = (*TextInput)(nil)
	_ base.AccessDescriber = (*TextInput)(nil)
	_ base.AccessDescriber = (*TextBox)(nil)
)
