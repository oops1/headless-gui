package richtext

// xaml.go — тег <RichText> в разметке, по образцу FlowDocument из WPF:
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

	"github.com/oops1/headless-gui/v3/widget/base"
	"github.com/oops1/headless-gui/v3/widget/xaml"
)

// xrStyle — оформление, наследуемое вложенными элементами (как в WPF).
// Жирность и курсив хранятся флагами, а не именем шрифта: имя выбирается в
// конце, когда известно, не задал ли элемент свой FontFamily.
type xrStyle struct {
	run          RichRun
	bold, italic bool
}

// RegisterXAML регистрирует тег <RichText> в реестре XAML r. Фасад widget
// зовёт её при первой загрузке разметки; программа на widget/base и
// widget/xaml зовёт сама: richtext.RegisterXAML(xaml.Default()). В фасад не
// входит: фасад регистрирует теги сам.
//
//facadegen:skip
func RegisterXAML(r *xaml.Registry) {
	r.Register(func(el *xaml.Element, _ *xaml.Loader) (base.Widget, error) {
		return buildXAMLRichText(*el), nil
	}, "richtext")
}

// OwnsMarkupChildren сообщает загрузчику XAML, что дети элемента <RichText> —
// абзацы и раны (их разбирает строитель), а не виджеты: загрузчик строит из
// них только теги-свойства (<RichText.ContextMenu>). Так и для RichText,
// построенного пользовательским тегом (RegisterXAMLWidget).
func (t *RichText) OwnsMarkupChildren() bool { return true }

// buildXAMLRichText строит RichText из элемента разметки.
func buildXAMLRichText(el xaml.Element) base.Widget {
	t := NewRichText()
	xaml.ApplyColor(&t.Background, el, "Background")
	xaml.ApplyColor(&t.TextColor, el, "Foreground")
	xaml.ApplyColor(&t.LinkColor, el, "LinkForeground")
	if fs := el.Attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			t.FontSize = v
		}
	}
	if ff := el.Attr("FontFamily"); ff != "" {
		t.FontName = ff
	}
	if v := el.Attr("LineSpacing"); v != "" {
		t.LineSpacing = xaml.ParseInt(v)
	}
	if v := el.Attr("Padding"); v != "" {
		m := xaml.ParseMargin(v)
		t.PaddingX, t.PaddingY = m.Left, m.Top
	}
	// Правка включается IsReadOnly="False" или Editable="True". По умолчанию
	// виджет — просмотр, поэтому одного IsReadOnly="True" достаточно лишь для
	// ясности; если заданы оба атрибута, решает Editable: он прямее.
	if v := el.Attr("IsReadOnly", "ReadOnly"); v != "" {
		t.Editable = strings.EqualFold(v, "false")
	}
	if v := el.Attr("Editable"); v != "" {
		t.Editable = strings.EqualFold(v, "true")
	}
	if strings.EqualFold(el.Attr("AcceptsTab"), "true") {
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
	} else if text := el.Attr("Text"); text != "" {
		t.SetText(text)
	} else if el.Text != "" {
		t.SetText(el.Text)
	}
	return t
}

// xrParagraph разбирает <Paragraph>.
func xrParagraph(el xaml.Element) RichParagraph {
	var p RichParagraph
	switch strings.ToLower(el.Attr("TextAlignment", "HorizontalContentAlignment")) {
	case "center":
		p.Align = base.TextAlignCenter
	case "right":
		p.Align = base.TextAlignRight
	}
	// Margin абзаца WPF → отступ слева и места до и после.
	if v := el.Attr("Margin"); v != "" {
		m := xaml.ParseMargin(v)
		p.Indent, p.SpaceBefore, p.SpaceAfter = m.Left, m.Top, m.Bottom
	}
	if v := el.Attr("Indent"); v != "" {
		p.Indent = xaml.ParseInt(v)
	}
	if v := el.Attr("SpaceBefore"); v != "" {
		p.SpaceBefore = xaml.ParseInt(v)
	}
	if v := el.Attr("SpaceAfter"); v != "" {
		p.SpaceAfter = xaml.ParseInt(v)
	}

	baseStyle := xrStyle{}
	xrApply(&baseStyle, el)
	// Текст самого <Paragraph> (без вложенных тегов) — один ран.
	if el.Text != "" {
		p.Runs = append(p.Runs, xrRun(baseStyle, el.Text))
	}
	for _, ch := range el.Children {
		p.Runs = xrInline(ch, baseStyle, p.Runs, 0)
	}
	return p
}

// xrApply накладывает атрибуты оформления элемента на st.
func xrApply(st *xrStyle, el xaml.Element) {
	if fs := el.Attr("FontSize"); fs != "" {
		if v, err := strconv.ParseFloat(fs, 64); err == nil && v > 0 {
			st.run.Size = v
		}
	}
	if ff := el.Attr("FontFamily"); ff != "" {
		st.run.Font = ff
	}
	switch strings.ToLower(el.Attr("FontWeight")) {
	case "bold", "semibold", "black", "heavy", "extrabold", "ultrabold":
		st.bold = true
	case "normal", "regular", "light", "thin":
		st.bold = false
	}
	switch strings.ToLower(el.Attr("FontStyle")) {
	case "italic", "oblique":
		st.italic = true
	case "normal":
		st.italic = false
	}
	xaml.ApplyColor(&st.run.Color, el, "Foreground")
	xaml.ApplyColor(&st.run.BG, el, "Background")
	td := strings.ToLower(el.Attr("TextDecorations"))
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
			r.Font = base.BuiltinFontBoldItalic
		case st.bold:
			r.Font = base.BuiltinFontBold
		case st.italic:
			r.Font = base.BuiltinFontItalic
		}
	}
	return r
}

// maxXRichDepth — предел вложенности <Span>/<Bold>: разметка из чужих рук не
// должна исчерпать стек рекурсией (то же соображение, что maxXAMLDepth).
const maxXRichDepth = 32

// xrInline разбирает вложенный элемент абзаца и дописывает раны в out.
func xrInline(el xaml.Element, baseStyle xrStyle, out []RichRun, depth int) []RichRun {
	if depth > maxXRichDepth {
		return out
	}
	tag := strings.ToLower(el.Tag)
	if tag == "linebreak" {
		return append(out, xrRun(baseStyle, "\n"))
	}
	st := baseStyle
	switch tag {
	case "run", "span":
	case "bold":
		st.bold = true
	case "italic":
		st.italic = true
	case "underline":
		st.run.Underline = true
	case "hyperlink":
		st.run.Link = el.Attr("NavigateUri", "Uri", "Link")
	default:
		return out // неизвестный вложенный тег игнорируется
	}
	xrApply(&st, el)

	text := el.Attr("Text")
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
