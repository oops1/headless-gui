package widget

// richtext_html.go — выделение как HTML для буфера обмена.
//
// Зачем HTML. Простой текст теряет всё, ради чего виджет существует: вставка
// в Word или почтовик даёт серую однородную массу. HTML рядом с простым
// текстом (SetClipboardHTML) вставляется с оформлением в приложения, которые
// его понимают, а Блокнот и терминал берут простую версию.
//
// Что НЕ выгружается: цвет текста по умолчанию, кегль и шрифт по умолчанию.
// Они — свойство виджета и темы (светлый текст на тёмном фоне), а не документа:
// выгрузи мы белый цвет темы, вставка на белую страницу Word была бы
// невидимой. Выгружается только то, что ран задал явно.

import (
	"fmt"
	"html"
	"image/color"
	"strconv"
	"strings"
)

// richSelectionHTML — HTML-фрагмент для диапазона [lo, hi) в рунах документа.
func richSelectionHTML(paras []RichParagraph, d *richDoc, lo, hi int) string {
	var sb strings.Builder
	for pi, p := range paras {
		ps, pe := d.paraStart[pi], d.paraEnd(pi)
		// Абзац входит, если выделение захватывает хоть символ или его конец
		// (пустой абзац — лишь переводом). Выделение, оканчивающееся ровно
		// на разделителе перед абзацем, этот абзац не включает.
		if hi <= ps || lo > pe {
			continue
		}
		sb.WriteString(richParagraphOpen(p))
		off := ps
		for _, r := range p.Runs {
			rs := []rune(r.Text)
			a, b := lo, hi
			if a < off {
				a = off
			}
			if b > off+len(rs) {
				b = off + len(rs)
			}
			if a < b {
				sb.WriteString(richRunHTML(r, string(rs[a-off:b-off])))
			}
			off += len(rs)
		}
		sb.WriteString("</p>")
	}
	return sb.String()
}

// richParagraphOpen — открывающий тег абзаца со стилем.
//
// Поля задаются всегда, даже нулевые: у <p> в HTML есть собственные поля по
// умолчанию, и без явных нулей вставленные абзацы разъехались бы пустыми
// строками, которых в виджете не было.
func richParagraphOpen(p RichParagraph) string {
	var st []string
	switch p.Align {
	case TextAlignCenter:
		st = append(st, "text-align:center")
	case TextAlignRight:
		st = append(st, "text-align:right")
	}
	st = append(st,
		"margin-top:"+strconv.Itoa(maxInt(p.SpaceBefore, 0))+"px",
		"margin-bottom:"+strconv.Itoa(maxInt(p.SpaceAfter, 0))+"px",
		"margin-left:"+strconv.Itoa(maxInt(p.Indent, 0))+"px",
		"margin-right:0")
	return `<p style="` + strings.Join(st, ";") + `">`
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// richRunHTML — кусок рана text как <span>, а у ссылки — внутри <a>.
func richRunHTML(r RichRun, text string) string {
	body := html.EscapeString(text)
	body = strings.ReplaceAll(body, "\n", "<br>")
	if st := richRunCSS(r); st != "" {
		body = `<span style="` + st + `">` + body + `</span>`
	}
	if r.Link != "" && richSafeLink(r.Link) {
		body = `<a href="` + html.EscapeString(r.Link) + `">` + body + `</a>`
	}
	return body
}

// richSafeLink — адрес ссылки допустим для выгрузки в буфер. Исполняемые
// схемы (javascript:, vbscript:, data:) в HTML для вставки не нужны, а
// редактор, который вставит такой фрагмент без очистки, получил бы живой
// скрипт: ссылку приходится отбрасывать, а не доверять получателю.
func richSafeLink(u string) bool {
	// Браузеры игнорируют пробелы и управляющие символы внутри схемы
	// ("java\tscript:"), поэтому убираем их до сравнения.
	var sb strings.Builder
	for _, r := range u {
		if r > ' ' && r != 0x7f {
			sb.WriteRune(r)
		}
	}
	low := strings.ToLower(sb.String())
	for _, bad := range []string{"javascript:", "vbscript:", "data:"} {
		if strings.HasPrefix(low, bad) {
			return false
		}
	}
	return true
}

// richRunCSS — свойства CSS, которые ран задал явно.
func richRunCSS(r RichRun) string {
	var st []string
	bold, italic, family := false, false, ""
	switch r.Font {
	case "":
	case BuiltinFontBold:
		bold = true
	case BuiltinFontItalic:
		italic = true
	case BuiltinFontBoldItalic:
		bold, italic = true, true
	case BuiltinFontMono:
		family = "'Courier New',monospace"
	default:
		// Имя шрифта — произвольная строка из приложения: в CSS попадает
		// только безопасный набор символов.
		if n := richSafeFontName(r.Font); n != "" {
			family = "'" + n + "'"
		}
	}
	if family != "" {
		st = append(st, "font-family:"+family)
	}
	if bold {
		st = append(st, "font-weight:bold")
	}
	if italic {
		st = append(st, "font-style:italic")
	}
	if r.Size > 0 {
		st = append(st, "font-size:"+strconv.FormatFloat(r.Size, 'f', -1, 64)+"pt")
	}
	if r.Color.A != 0 {
		st = append(st, "color:"+richCSSColor(r.Color))
	}
	if r.BG.A != 0 {
		st = append(st, "background-color:"+richCSSColor(r.BG))
	}
	switch {
	case r.Underline && r.Strike:
		st = append(st, "text-decoration:underline line-through")
	case r.Underline:
		st = append(st, "text-decoration:underline")
	case r.Strike:
		st = append(st, "text-decoration:line-through")
	}
	return strings.Join(st, ";")
}

// richSafeFontName оставляет в имени шрифта буквы, цифры и несколько
// знаков: кавычки и точка с запятой позволили бы вырваться из значения CSS.
func richSafeFontName(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r == ' ', r == '-', r == '_', r == '.':
			sb.WriteRune(r)
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r > 127:
			sb.WriteRune(r)
		}
	}
	return strings.TrimSpace(sb.String())
}

// richCSSColor записывает цвет RGBA (с предумноженной альфой) для CSS.
func richCSSColor(c color.RGBA) string {
	if c.A == 255 {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	// Движок хранит цвет с предумноженной альфой, CSS ждёт обычный.
	un := func(v uint8) int {
		if n := int(v) * 255 / int(c.A); n < 255 {
			return n
		}
		return 255
	}
	return fmt.Sprintf("rgba(%d,%d,%d,%.2f)", un(c.R), un(c.G), un(c.B), float64(c.A)/255)
}
