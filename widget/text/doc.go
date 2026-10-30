// Package text — ввод текста (ADR-0012, этап 8): однострочное поле
// TextInput (и поле пароля), многострочный редактор TextBox, композиция IME
// и их XAML-теги (RegisterXAML).
//
// Пакет стоит над widget/base; XAML-теги регистрирует в реестре widget/xaml
// явно. Программы пользуются им через фасад widget (widget.NewTextInput и
// прочие — те же функции).
package text
