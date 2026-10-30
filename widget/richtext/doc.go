// Package richtext — форматированный текст RichText (ADR-0012, этап 8):
// модель документа из абзацев и ранов, раскладка, каретка и правка, HTML
// (разбор и вывод), композиция IME и XAML-тег <RichText> (RegisterXAML).
//
// Пакет стоит над widget/base; XAML-тег регистрирует в реестре widget/xaml
// явно. Программы пользуются им через фасад widget (widget.NewRichText и
// прочие — те же функции).
package richtext
