package widget

// richtext_xaml.go — тег <RichText> в разметке, по образцу FlowDocument из WPF:
//
//	<RichText x:Name="help" Foreground="#DDD" FontSize="10" Padding="8,6">
//	    <Paragraph FontSize="20" FontWeight="Bold" Margin="0,0,0,6">Справка</Paragraph>
//	    <Paragraph TextAlignment="Center">
//	        <Run Text="Обычный текст, " />
//	        <Run Text="жирный" FontWeight="Bold" Foreground="#E06C75" />
//	        <Run Text=", " /><Bold><Run Text="вложенный" /></Bold>
//	        <Hyperlink NavigateUri="https://example.com">ссылка</Hyperlink>
//	        <LineBreak />
//	        <Run Text="зачёркнутый" TextDecorations="Strikethrough" />
//	    </Paragraph>
//	</RichText>
//
// Правка. По умолчанию <RichText> — просмотр. IsReadOnly="False" (или
// Editable="True") делает его редактором, AcceptsTab="True" — Tab вставляет
// отступ вместо перехода фокуса:
//
//	<RichText x:Name="note" IsReadOnly="False" AcceptsTab="True" />
//
// Ограничение разбора. Разборщик XAML сохраняет у элемента ОДНУ строку текста
// между тегами и теряет порядок «текст — тег — текст»: смешанное содержимое
// <Paragraph>до <Bold>середина</Bold> после</Paragraph> восстановить нельзя.
// Поэтому текст, как в примере, кладут в <Run>. Пробелы по краям текста между
// тегами разборщик обрезает, так что пробел на стыке ранов пишут в атрибуте
// Text ("Обычный текст, "), а не между тегами.

import (
	"strconv"
	"strings"
)

// xrStyle — оформление, наследуемое вложенными элементами (как в WPF).
// Жирность и курсив хранятся флагами, а не именем шрифта: имя выбирается в
// конце, когда известно, не задал ли элемент свой FontFamily.
type xrStyle struct {
	run          RichRun
	bold, italic bool
}

// buildXAMLRichText строит RichText из элемента разметки.
func buildXAMLRichText(el xElement) Widget {
	t := NewRichText()
	applyColor(&t.Background, el, "Background")
	applyColor(&t.TextColor, el, "Foreground")
	applyColor(&t.LinkColor, el, "LinkForeground")
	if fs := el.attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			t.FontSize = v
		}
	}
	if ff := el.attr("FontFamily"); ff != "" {
		t.FontName = ff
	}
	if v := el.attr("LineSpacing"); v != "" {
		t.LineSpacing = xatoi(v)
	}
	if v := el.attr("Padding"); v != "" {
		m := parseMargin(v)
		t.PaddingX, t.PaddingY = m.Left, m.Top
	}
	// Правка включается IsReadOnly="False" или Editable="True". По умолчанию
	// виджет — просмотр, поэтому одного IsReadOnly="True" достаточно лишь для
	// ясности; если заданы оба атрибута, решает Editable: он прямее.
	if v := el.attr("IsReadOnly", "ReadOnly"); v != "" {
		t.Editable = strings.EqualFold(v, "false")
	}
	if v := el.attr("Editable"); v != "" {
		t.Editable = strings.EqualFold(v, "true")
	}
	if strings.EqualFold(el.attr("AcceptsTab"), "true") {
		t.AcceptTab = true
	}

	var paras []RichParagraph
	for _, ch := range el.Children {
		if !strings.EqualFold(ch.Tag, "Paragraph") {
			continue
		}
		paras = append(paras, xrParagraph(ch))
	}
	if len(paras) > 0 {
		t.SetParagraphs(paras)
	} else if text := el.attr("Text"); text != "" {
		t.SetText(text)
	} else if el.Text != "" {
		t.SetText(el.Text)
	}
	return t
}

// xrParagraph разбирает <Paragraph>.
func xrParagraph(el xElement) RichParagraph {
	var p RichParagraph
	switch strings.ToLower(el.attr("TextAlignment", "HorizontalContentAlignment")) {
	case "center":
		p.Align = TextAlignCenter
	case "right":
		p.Align = TextAlignRight
	}
	// Margin абзаца WPF → отступ слева и места до и после.
	if v := el.attr("Margin"); v != "" {
		m := parseMargin(v)
		p.Indent, p.SpaceBefore, p.SpaceAfter = m.Left, m.Top, m.Bottom
	}
	if v := el.attr("Indent"); v != "" {
		p.Indent = xatoi(v)
	}
	if v := el.attr("SpaceBefore"); v != "" {
		p.SpaceBefore = xatoi(v)
	}
	if v := el.attr("SpaceAfter"); v != "" {
		p.SpaceAfter = xatoi(v)
	}

	base := xrStyle{}
	xrApply(&base, el)
	// Текст самого <Paragraph> (без вложенных тегов) — один ран.
	if el.Text != "" {
		p.Runs = append(p.Runs, xrRun(base, el.Text))
	}
	for _, ch := range el.Children {
		p.Runs = xrInline(ch, base, p.Runs, 0)
	}
	return p
}

// xrApply накладывает атрибуты оформления элемента на st.
func xrApply(st *xrStyle, el xElement) {
	if fs := el.attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			st.run.Size = v
		}
	}
	if ff := el.attr("FontFamily"); ff != "" {
		st.run.Font = ff
	}
	switch strings.ToLower(el.attr("FontWeight")) {
	case "bold", "semibold", "black", "heavy", "extrabold", "ultrabold":
		st.bold = true
	case "normal", "regular", "light", "thin":
		st.bold = false
	}
	switch strings.ToLower(el.attr("FontStyle")) {
	case "italic", "oblique":
		st.italic = true
	case "normal":
		st.italic = false
	}
	applyColor(&st.run.Color, el, "Foreground")
	applyColor(&st.run.BG, el, "Background")
	td := strings.ToLower(el.attr("TextDecorations"))
	if strings.Contains(td, "underline") {
		st.run.Underline = true
	}
	if strings.Contains(td, "strikethrough") {
		st.run.Strike = true
	}
}

// xrRun собирает RichRun: шрифт по флагам жирности и курсива, если элемент не
// назвал свой — синтетического «утолщения» в движке нет, а встроенные
// начертания есть.
func xrRun(st xrStyle, text string) RichRun {
	r := st.run
	r.Text = text
	if r.Font == "" {
		switch {
		case st.bold && st.italic:
			r.Font = BuiltinFontBoldItalic
		case st.bold:
			r.Font = BuiltinFontBold
		case st.italic:
			r.Font = BuiltinFontItalic
		}
	}
	return r
}

// maxXRichDepth — предел вложенности <Span>/<Bold>: разметка из чужих рук не
// должна исчерпать стек рекурсией (то же соображение, что maxXAMLDepth).
const maxXRichDepth = 32

// xrInline разбирает вложенный элемент абзаца и дописывает раны в out.
func xrInline(el xElement, base xrStyle, out []RichRun, depth int) []RichRun {
	if depth > maxXRichDepth {
		return out
	}
	tag := strings.ToLower(el.Tag)
	if tag == "linebreak" {
		return append(out, xrRun(base, "\n"))
	}
	st := base
	switch tag {
	case "run", "span":
	case "bold":
		st.bold = true
	case "italic":
		st.italic = true
	case "underline":
		st.run.Underline = true
	case "hyperlink":
		st.run.Link = el.attr("NavigateUri", "Uri", "Link")
	default:
		return out // неизвестный вложенный тег игнорируется
	}
	xrApply(&st, el)

	text := el.attr("Text")
	if text == "" {
		text = el.Text
	}
	if text != "" {
		out = append(out, xrRun(st, text))
	}
	for _, ch := range el.Children {
		out = xrInline(ch, st, out, depth+1)
	}
	return out
}
