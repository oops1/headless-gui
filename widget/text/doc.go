// Package text — ввод текста (ADR-0012, этап 8): однострочное поле
// TextInput (и поле пароля), многострочный редактор TextBox, композиция IME
// и их XAML-теги (RegisterXAML).
//
// Пакет стоит над widget/base; XAML-теги регистрирует в реестре widget/xaml
// явно. Программы пользуются им через фасад widget (widget.NewTextInput и
// прочие — те же функции).
//
// До переезда диалогов в свой пакет (ADR-0012, 8.6) диалоги ядра строят
// своё поле ввода через фабрику, которую пакет задаёт при загрузке
// (base.SetTextInputFactory).
package text

import "github.com/oops1/headless-gui/v3/widget/base"

func init() {
	base.SetTextInputFactory(func(placeholder string) base.TextEditor {
		return NewTextInput(placeholder)
	})
}
