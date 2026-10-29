package base

// textfield.go — поле ввода текста как возможность (ADR-0012, 8.3): ядро,
// привязка данных XAML и загрузчики работают с полем через TextEditor, не
// зная пакета, в котором оно живёт (TextInput и TextBox — widget/text).

import (
	"image/color"
	"sync/atomic"
)

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

var textInputFactory atomic.Pointer[func(placeholder string) TextEditor]

// SetTextInputFactory задаёт конструктор однострочного поля, которым
// диалоги ядра (ввод строки, имя файла) строят своё поле. Задаёт
// widget/text при загрузке; до переезда диалогов в свой пакет (ADR-0012,
// 8.6) это единственная связь ядра с полем ввода.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func SetTextInputFactory(f func(placeholder string) TextEditor) {
	textInputFactory.Store(&f)
}

// newTextInputField — однострочное поле для диалогов ядра.
func newTextInputField(placeholder string) TextEditor {
	f := textInputFactory.Load()
	if f == nil || *f == nil {
		panic("widget/base: нет поля ввода — импортируйте widget/text (или widget)")
	}
	return (*f)(placeholder)
}
