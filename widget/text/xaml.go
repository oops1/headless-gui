package text

// xaml.go — теги ввода текста в разметке (ADR-0012 §4): <TextBox>,
// <TextInput>, <Input>, <RichTextBox> (однострочное поле или многострочный
// редактор — по AcceptsReturn/TextWrapping) и <PasswordBox>. Реестр XAML о
// пакете text не знает: теги регистрирует RegisterXAML.

import (
	"strconv"
	"strings"

	"github.com/oops1/headless-gui/v4/widget/base"
	"github.com/oops1/headless-gui/v4/widget/xaml"
)

// RegisterXAML регистрирует теги ввода текста в реестре XAML r. Фасад
// widget зовёт её при первой загрузке разметки; программа на widget/base и
// widget/xaml зовёт сама: text.RegisterXAML(xaml.Default()). В фасад не
// входит: фасад регистрирует теги сам.
//
//facadegen:skip
func RegisterXAML(r *xaml.Registry) {
	r.Register(buildField, "textbox", "textinput", "input", "richtextbox")
	r.Register(buildPassword, "passwordbox")
}

// buildField — многострочный (AcceptsReturn / TextWrapping="Wrap") —
// TextBox-редактор, иначе однострочное поле.
func buildField(el *xaml.Element, l *xaml.Loader) (base.Widget, error) {
	if wantsMultiline(el) {
		return buildTextBox(el), nil
	}
	return buildTextInput(el, false, l), nil
}

func buildPassword(el *xaml.Element, l *xaml.Loader) (base.Widget, error) {
	return buildTextInput(el, true, l), nil
}

func buildTextInput(el *xaml.Element, isPassword bool, l *xaml.Loader) base.Widget {
	placeholder := el.Attr("Tag", "Placeholder", "PlaceholderText", "Hint")
	if isPassword && placeholder == "" {
		placeholder = "Пароль"
	}
	var ti *TextInput
	if isPassword {
		ti = NewPasswordInput(placeholder)
	} else {
		ti = NewTextInput(placeholder)
	}

	if text := el.Attr("Text"); text != "" {
		ti.SetText(text)
	}
	if strings.EqualFold(el.Attr("Focused", "IsFocused", "Focus"), "true") {
		ti.SetFocused(true)
	}
	xaml.ApplyColor(&ti.TextColor, *el, "Foreground")
	xaml.ApplyColor(&ti.Background, *el, "Background")

	if ff := el.Attr("FontFamily"); ff != "" {
		ti.FontName = ff
	}
	if fs := el.Attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			ti.FontSize = v
		}
	}
	if strings.EqualFold(el.Attr("AcceptsReturn"), "true") {
		ti.AcceptsReturn = true
	}
	if wrap := strings.ToLower(el.Attr("TextWrapping")); wrap == "wrap" || wrap == "wrapwithoverflow" {
		ti.AcceptsReturn = true
	}
	if ml := el.Attr("MaxLength"); ml != "" {
		if v, err := strconv.Atoi(strings.TrimSpace(ml)); err == nil && v > 0 {
			ti.MaxLength = v
		}
	}

	// Значок в начале поля (лупа у строки поиска). Путь ограничен каталогом
	// разметки или fs.FS — тем же OpenResource, что и у прочих ресурсов.
	if src := el.Attr("Icon", "IconSource", "LeadingIcon"); src != "" {
		if img, err := l.Image(src); err == nil {
			ti.LeadingIcon = img
		}
	}
	if sz := xaml.ParseInt(el.Attr("IconSize")); sz > 0 {
		ti.LeadingIconSize = sz
	}

	return ti
}

// wantsMultiline — тег текстового поля описывает многострочный редактор.
func wantsMultiline(el *xaml.Element) bool {
	if strings.EqualFold(el.Attr("AcceptsReturn"), "true") {
		return true
	}
	wrap := strings.ToLower(el.Attr("TextWrapping"))
	return wrap == "wrap" || wrap == "wrapwithoverflow"
}

// buildTextBox создаёт многострочный TextBox из XAML-элемента.
func buildTextBox(el *xaml.Element) base.Widget {
	tb := NewTextBox(el.Attr("Tag", "Placeholder", "PlaceholderText", "Hint"))

	wrap := strings.ToLower(el.Attr("TextWrapping"))
	tb.Wrap = wrap == "" || wrap == "wrap" || wrap == "wrapwithoverflow"
	if strings.EqualFold(el.Attr("IsReadOnly", "ReadOnly"), "true") {
		tb.ReadOnly = true
	}
	if text := el.Attr("Text"); text != "" {
		tb.SetText(text)
	}
	if strings.EqualFold(el.Attr("Focused", "IsFocused", "Focus"), "true") {
		tb.SetFocused(true)
	}
	xaml.ApplyColor(&tb.TextColor, *el, "Foreground")
	xaml.ApplyColor(&tb.Background, *el, "Background")
	if fs := el.Attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			tb.FontSize = v
		}
	}
	// FontFamily — именованный шрифт (RegisterFont); для кода — моноширинный.
	if ff := el.Attr("FontFamily"); ff != "" {
		tb.FontName = ff
	}
	// AcceptsTab — Tab вставляет табуляцию, а не переводит фокус.
	if strings.EqualFold(el.Attr("AcceptsTab"), "true") {
		tb.AcceptTab = true
	}
	return tb
}
