package base

// textfield.go — поле ввода текста как возможность (ADR-0012, 8.3): ядро,
// привязка данных XAML и загрузчики работают с полем через TextEditor, не
// зная пакета, в котором оно живёт (TextInput и TextBox — widget/text).

import "image/color"

// TextEditor — поле ввода текста (TextInput, TextBox или поле стороннего
// пакета): то, что привязка данных, диалоги и загрузчики делают с полем.
type TextEditor interface {
	Widget
	GetText() string
	SetText(s string)
	// SetPlaceholder задаёт подсказку в пустом поле.
	SetPlaceholder(s string)
	// SetTextColor и SetBackgroundColor задают цвет текста и фона поля.
	SetTextColor(c color.RGBA)
	SetBackgroundColor(c color.RGBA)
	// OnTextChanged — текущий обработчик изменения текста (nil — нет);
	// SetOnTextChanged его заменяет.
	OnTextChanged() func(string)
	SetOnTextChanged(fn func(string))
}
