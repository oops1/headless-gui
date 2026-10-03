package widget

// richtext_htmlparse.go — разбор HTML в абзацы и раны: вставка из буфера обмена.
//
// Word, браузеры и почтовики кладут в буфер HTML. Нужен не полноценный
// браузерный разбор, а терпимое чтение того, что понимает модель RichRun:
// жирный, курсив, подчёркивание, зачёркивание, цвет, фон, кегль, ссылка,
// выравнивание и отступы абзаца. Всё остальное выбрасывается, а текст
// сохраняется.
//
// Внешних зависимостей нет (golang.org/x/net/html в go.mod не входит, а ради
// вставки из буфера тянуть его не стоит): токенизатор простой, но не падает и
// не зависает на мусоре — HTML приходит из чужих приложений, и «битая»
// разметка здесь норма, а не исключение.
//
// Что НЕ переносится. Имена шрифтов (font-family) движку неизвестны —
// Calibri из Word не должен превратиться в запрос несуществующего шрифта, а
// молчаливо подменять его чем-то нельзя. Жирность и курсив выражаются
// встроенными шрифтами движка, как в разметке XAML (xrRun). Единственное
// исключение — моноширинное семейство (courier, consolas, monospace, теги
// code/pre): оно переводится во встроенный BuiltinFontMono, потому что
// выгрузка в HTML (richRunCSS) пишет его именно так, и круг «выгрузить —
// разобрать» должен вернуть тот же шрифт.

import (
	"html"
	"image/color"
	"math"
	"strconv"
	"strings"
)

// Пределы разбора: вложенность и размер определяет чужой документ, а стек
// нельзя растить до исчерпания памяти.
const (
	rhMaxDepth  = 256
	rhMaxIndent = 10000
)

// RichParagraphsFromHTML разбирает HTML (фрагмент или документ) в абзацы.
// Результат пуст (nil), если текста нет. Раны приведены к инвариантам
// документа: пустых нет, соседние с одинаковым оформлением слиты.
//
// Пробелы схлопываются по правилам HTML: подряд идущие — в один, пробелы в
// начале и в конце абзаца и вокруг <br> отбрасываются; неразрывный пробел
// (&nbsp;) не схлопывается и становится обычным пробелом. Если нужен
// пробел, который не должен пропасть, его надо писать как &nbsp;.
func RichParagraphsFromHTML(src string) []RichParagraph {
	p := &rhParser{lastSpace: true}
	p.run(src)
	p.flush()
	return p.paras
}

// rhStyle — наследуемое оформление. Жирность и курсив — флагами, а имя шрифта
// выбирается при выпуске текста (как xrStyle в разборе XAML).
type rhStyle struct {
	run                     RichRun
	bold, italic, mono, pre bool
}

// font возвращает ран с выбранным встроенным шрифтом.
func (s rhStyle) font() RichRun {
	r := s.run
	r.Text = ""
	switch {
	case s.mono:
		r.Font = BuiltinFontMono
	case s.bold && s.italic:
		r.Font = BuiltinFontBoldItalic
	case s.bold:
		r.Font = BuiltinFontBold
	case s.italic:
		r.Font = BuiltinFontItalic
	}
	return r
}

// rhFrame — открытый элемент. align и indent наследуются вложенными, поля
// before/after — нет: поля абзаца принадлежат своему блоку.
type rhFrame struct {
	name          string
	st            rhStyle
	block         bool
	align         TextAlign
	indent        int
	before, after int
}

// rhPiece — кусок текста абзаца с одним оформлением.
type rhPiece struct {
	style RichRun
	sb    strings.Builder
}

// rhPara — абзац в разработке.
type rhPara struct {
	fmt     RichParagraph
	pieces  []*rhPiece
	pre     bool // пробелы значимы — хвостовой пробел не отбрасывать
	touched bool // были текст или <br>: пустой <p></p> абзаца не даёт, как в браузере
}

type rhParser struct {
	frames []rhFrame
	paras  []RichParagraph
	cur    *rhPara

	// lastSpace — предыдущий символ в абзаце свёрнутый пробел либо начало
	// строки: следующий пробел пропускается.
	lastSpace bool
	// pendingBR — <br> ещё не превращён в '\n'. <br> в самом конце блока
	// строки не создаёт (браузер так же его игнорирует), поэтому решение
	// откладывается до следующего содержимого.
	pendingBR      bool
	pendingBRStyle RichRun
	pendingBullet  bool    // ближайший абзац начнётся с маркера списка
	bulletStyle    rhStyle // оформление маркера — пункта списка, а не его содержимого
	preSkipNL      bool    // первый перевод строки сразу после <pre> не считается

	skipName  string // элемент, содержимое которого выбрасывается
	skipDepth int
	raw       string // script/style: содержимое до закрывающего тега не разбирается
}

// ---------------------------------------------------------------------------
// Токенизатор

func rhIsAlpha(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func rhIsSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func (p *rhParser) run(s string) {
	i := 0
	for i < len(s) {
		lt := strings.IndexByte(s[i:], '<')
		if lt < 0 {
			p.text(s[i:])
			return
		}
		if lt > 0 {
			p.text(s[i : i+lt])
			i += lt
		}
		rest := s[i:]
		switch {
		case strings.HasPrefix(rest, "<!--"):
			// Комментарий: сюда попадают и маркеры <!--StartFragment-->, и
			// условные блоки Word <!--[if gte mso 9]>...<![endif]-->.
			end := strings.Index(rest[4:], "-->")
			if end < 0 {
				return // незакрытый комментарий съедает остаток, как в браузере
			}
			i += 4 + end + 3
		case strings.HasPrefix(rest, "<!") || strings.HasPrefix(rest, "<?"):
			// doctype, CDATA, «<![if !supportLists]>» и <?xml ...?>.
			end := strings.IndexByte(rest, '>')
			if end < 0 {
				return
			}
			i += end + 1
		case len(rest) > 1 && (rhIsAlpha(rest[1]) || rest[1] == '/' && len(rest) > 2 && rhIsAlpha(rest[2])):
			n := p.tag(rest)
			if n == 0 {
				return // тег не закрыт до конца данных
			}
			i += n
			if p.raw != "" {
				// Содержимое script/style — не разметка: ищем закрывающий
				// тег как подстроку, не разбирая «<» внутри.
				idx := rhIndexFold(s[i:], "</"+p.raw)
				p.raw = ""
				if idx < 0 {
					return
				}
				i += idx
			}
		default:
			p.text("<")
			i++
		}
	}
}

// rhIndexFold — индекс подстроки без учёта регистра ASCII.
func rhIndexFold(s, sub string) int {
	n := len(sub)
	for i := 0; i+n <= len(s); i++ {
		if strings.EqualFold(s[i:i+n], sub) {
			return i
		}
	}
	return -1
}

// tag разбирает тег в начале s и возвращает число съеденных байтов (0 — тег
// не закрыт).
func (p *rhParser) tag(s string) int {
	i := 1
	closing := false
	if s[i] == '/' {
		closing = true
		i++
	}
	j := i
	for j < len(s) && !rhIsSpaceByte(s[j]) && s[j] != '/' && s[j] != '>' {
		j++
	}
	name := strings.ToLower(s[i:j])
	attrs := map[string]string{}
	selfClose := false
	for {
		for j < len(s) && (rhIsSpaceByte(s[j]) || s[j] == '/') {
			if s[j] == '/' {
				selfClose = true
			} else {
				selfClose = false
			}
			j++
		}
		if j >= len(s) {
			return 0
		}
		if s[j] == '>' {
			j++
			break
		}
		selfClose = false
		k := j
		for k < len(s) && !rhIsSpaceByte(s[k]) && s[k] != '=' && s[k] != '>' && s[k] != '/' {
			k++
		}
		if k == j { // одиночный '=' или другой мусор: пропустить символ
			j++
			continue
		}
		an := strings.ToLower(s[j:k])
		j = k
		for j < len(s) && rhIsSpaceByte(s[j]) {
			j++
		}
		val := ""
		if j < len(s) && s[j] == '=' {
			j++
			for j < len(s) && rhIsSpaceByte(s[j]) {
				j++
			}
			if j >= len(s) {
				return 0
			}
			if q := s[j]; q == '"' || q == '\'' {
				end := strings.IndexByte(s[j+1:], q)
				if end < 0 {
					return 0
				}
				val = s[j+1 : j+1+end]
				j += end + 2
			} else {
				k = j
				for k < len(s) && !rhIsSpaceByte(s[k]) && s[k] != '>' {
					k++
				}
				val = s[j:k]
				j = k
			}
			if strings.Contains(val, "&") {
				val = html.UnescapeString(val)
			}
		}
		if _, dup := attrs[an]; !dup {
			attrs[an] = val
		}
	}
	if closing {
		p.endTag(name)
	} else {
		p.startTag(name, attrs, selfClose)
	}
	return j
}

// ---------------------------------------------------------------------------
// Теги

var rhVoid = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// rhSkip — элементы, содержимое которых не текст документа.
var rhSkip = map[string]bool{
	"head": true, "title": true, "script": true, "style": true,
	"template": true, "noscript": true, "iframe": true, "object": true,
	"applet": true, "svg": true, "xml": true, "select": true, "datalist": true,
}

var rhBlock = map[string]bool{
	"p": true, "div": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "ul": true, "ol": true, "li": true, "dl": true,
	"dt": true, "dd": true, "blockquote": true, "pre": true, "table": true,
	"thead": true, "tbody": true, "tfoot": true, "tr": true, "caption": true,
	"section": true, "article": true, "header": true, "footer": true,
	"nav": true, "aside": true, "main": true, "form": true, "fieldset": true,
	"address": true, "figure": true, "figcaption": true, "center": true,
	"details": true, "summary": true, "legend": true,
}

// rhHeadingPt — кегли заголовков h1–h6 в пунктах (как у браузера при 12 pt).
var rhHeadingPt = map[string]float64{
	"h1": 24, "h2": 18, "h3": 14, "h4": 12, "h5": 10, "h6": 8,
}

func (p *rhParser) top() rhFrame {
	if n := len(p.frames); n > 0 {
		return p.frames[n-1]
	}
	return rhFrame{}
}

// closeOpen закрывает открытый элемент target, если он есть над границей
// (stops) — так <p>a<p>b не вкладывает один абзац в другой, а оформление
// первого не утекает во второй.
func (p *rhParser) closeOpen(target string, stops ...string) {
	for i := len(p.frames) - 1; i >= 0; i-- {
		n := p.frames[i].name
		if n == target {
			p.frames = p.frames[:i]
			return
		}
		for _, s := range stops {
			if n == s {
				return
			}
		}
	}
}

var rhParaStops = []string{"div", "td", "th", "ul", "ol", "li", "blockquote", "table", "tr", "section", "article", "center", "dd"}

func rhStyleHidden(css string) bool {
	c := strings.ToLower(strings.ReplaceAll(css, " ", ""))
	return strings.Contains(c, "display:none") || strings.Contains(c, "mso-hide:all")
}

func (p *rhParser) startTag(name string, attrs map[string]string, selfClose bool) {
	if p.skipDepth > 0 {
		if p.skipName == "head" && name == "body" {
			p.skipDepth = 0 // <head> без закрывающего тега: тело начинается здесь
		} else {
			if name == p.skipName && !rhVoid[name] {
				p.skipDepth++
			}
			return
		}
	}
	css := attrs["style"]
	hidden := rhSkip[name] || rhStyleHidden(css)
	if _, ok := attrs["hidden"]; ok {
		hidden = true
	}
	if name == "span" {
		// Маркер списка Word: «·» и пробелы перед текстом пункта.
		c := strings.ToLower(strings.ReplaceAll(css, " ", ""))
		if strings.Contains(c, "mso-list:ignore") {
			hidden = true
		}
	}
	if hidden && !rhVoid[name] {
		p.skipName, p.skipDepth = name, 1
		if name == "script" || name == "style" {
			p.raw = name
		}
		return
	}

	switch name {
	case "br":
		p.br()
		return
	case "hr":
		p.flush()
		return
	}
	if rhVoid[name] {
		return
	}

	isBlock := rhBlock[name]
	if isBlock {
		if name == "li" {
			p.closeOpen("li", "ul", "ol", "table")
		}
		p.closeOpen("p", rhParaStops...)
		p.flush()
	}

	if name == "td" || name == "th" {
		// Ячейки таблицы идут в одну строку абзаца, разделённые пробелом.
		p.text(" ")
	}

	parent := p.top()
	f := rhFrame{name: name, st: parent.st, block: isBlock, align: parent.align}
	baseIndent := parent.indent
	switch name {
	case "b", "strong", "th":
		f.st.bold = true
	case "i", "em", "cite", "dfn", "var":
		f.st.italic = true
	case "u", "ins":
		f.st.run.Underline = true
	case "s", "strike", "del":
		f.st.run.Strike = true
	case "code", "tt", "kbd", "samp":
		f.st.mono = true
	case "pre":
		f.st.mono, f.st.pre = true, true
		p.preSkipNL = true
	case "mark":
		f.st.run.BG = color.RGBA{255, 255, 0, 255}
	case "a":
		if href := strings.TrimSpace(attrs["href"]); href != "" && richSafeLink(href) {
			f.st.run.Link = href
		}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		f.st.bold = true
		f.st.run.Size = rhHeadingPt[name]
	case "ul", "ol":
		baseIndent += 24
	case "blockquote", "dd":
		baseIndent += 32
	case "center":
		f.align = TextAlignCenter
	case "li":
		p.pendingBullet, p.bulletStyle = true, f.st
	case "font":
		if c, ok := rhParseColor(attrs["color"]); ok && c.A != 0 {
			f.st.run.Color = c
		}
		if sz, ok := rhFontTagSize(attrs["size"]); ok {
			f.st.run.Size = sz
		}
		if rhIsMonoFamily(attrs["face"]) {
			f.st.mono = true
		}
	}
	if isBlock {
		if a, ok := rhParseAlign(attrs["align"]); ok {
			f.align = a
		}
	}

	var bp rhBlockProps
	if css != "" {
		bp = p.applyCSS(&f.st, css)
	}
	if isBlock {
		if bp.alignSet {
			f.align = bp.align
		}
		f.before, f.after = bp.before, bp.after
		baseIndent += bp.left
		if bp.bullet {
			p.pendingBullet, p.bulletStyle = true, f.st
		}
	}
	f.indent = min(max(baseIndent, 0), rhMaxIndent)

	// «<span/>» браузеры читают как открывающий тег; здесь самозакрытие
	// учитывается только у строчных элементов (<o:p/> из Word), блоки не
	// теряют границы.
	if len(p.frames) < rhMaxDepth && (isBlock || !selfClose) {
		p.frames = append(p.frames, f)
	}
}

func (p *rhParser) endTag(name string) {
	if p.skipDepth > 0 {
		if name == p.skipName {
			p.skipDepth--
		}
		return
	}
	if name == "br" {
		p.br() // «</br>» браузеры читают как <br>
		return
	}
	for i := len(p.frames) - 1; i >= 0; i-- {
		if p.frames[i].name != name {
			continue
		}
		for _, f := range p.frames[i:] {
			if f.block {
				p.flush()
				break
			}
		}
		p.frames = p.frames[:i]
		if name == "li" || name == "ul" || name == "ol" {
			p.pendingBullet = false
		}
		return
	}
	// Закрывающий тег без открывающего игнорируется.
}

// ---------------------------------------------------------------------------
// Текст и абзацы

// ensurePara начинает абзац, если его ещё нет. Оформление абзаца берётся у
// ближайшего блока вокруг в момент начала абзаца.
func (p *rhParser) ensurePara() {
	if p.cur != nil {
		return
	}
	top := p.top()
	var before, after int
	for i := len(p.frames) - 1; i >= 0; i-- {
		if p.frames[i].block {
			before, after = p.frames[i].before, p.frames[i].after
			break
		}
	}
	p.cur = &rhPara{
		fmt: RichParagraph{
			Align: top.align, Indent: top.indent,
			SpaceBefore: before, SpaceAfter: after,
		},
		pre: top.st.pre,
	}
	if p.pendingBullet {
		p.pendingBullet = false
		// Маркер без ссылки, подчёркивания и фона: он не часть текста пункта.
		st := p.bulletStyle
		st.run.Link, st.run.Underline, st.run.Strike, st.run.BG = "", false, false, color.RGBA{}
		p.writeRaw("• ", st.font())
		p.cur.touched = true
		p.lastSpace = true
	}
}

// writeRaw дописывает текст в текущий абзац без обработки пробелов.
func (p *rhParser) writeRaw(text string, style RichRun) {
	c := p.cur
	if n := len(c.pieces); n > 0 && c.pieces[n-1].style == style {
		c.pieces[n-1].sb.WriteString(text)
		return
	}
	pc := &rhPiece{style: style}
	pc.sb.WriteString(text)
	c.pieces = append(c.pieces, pc)
}

// trimTrailingSpace убирает хвостовой обычный пробел абзаца: перед переводом
// строки и в конце абзаца он в HTML не виден.
func (p *rhParser) trimTrailingSpace() { rhTrimParaSpace(p.cur) }

func rhTrimParaSpace(c *rhPara) {
	if c == nil || c.pre {
		return
	}
	for i := len(c.pieces) - 1; i >= 0; i-- {
		s := c.pieces[i].sb.String()
		if s == "" {
			continue
		}
		if strings.HasSuffix(s, " ") {
			c.pieces[i].sb.Reset()
			c.pieces[i].sb.WriteString(s[:len(s)-1])
		}
		return
	}
}

// br — <br>: мягкий перевод строки внутри абзаца.
func (p *rhParser) br() {
	p.ensurePara()
	p.cur.touched = true
	style := p.top().st.font()
	if p.cur.pre {
		p.writeRaw("\n", style)
		return
	}
	if p.pendingBR {
		p.trimTrailingSpace()
		p.writeRaw("\n", p.pendingBRStyle)
	}
	p.pendingBR, p.pendingBRStyle = true, style
	p.lastSpace = true // после перевода строки начало строки: пробелы пропускаются
	p.writeRaw("", style)
}

// text обрабатывает текстовый узел: сущности, схлопывание пробелов, оформление
// из стека открытых элементов.
func (p *rhParser) text(raw string) {
	if p.skipDepth > 0 || raw == "" {
		return
	}
	if strings.IndexByte(raw, '&') >= 0 {
		raw = html.UnescapeString(raw)
	}
	// Абзац с маркером списка начинается ДО разбора пробелов: маркер заканчивается
	// пробелом, и пробел в начале текста пункта не должен прибавляться к нему.
	if p.cur == nil && p.pendingBullet && strings.Trim(raw, " \t\r\n\f") != "" {
		p.ensurePara()
	}
	top := p.top()
	pre := top.st.pre
	if pre && p.preSkipNL {
		p.preSkipNL = false
		raw = strings.TrimPrefix(strings.TrimPrefix(raw, "\r"), "\n")
	}
	var sb strings.Builder
	for _, r := range raw {
		switch {
		case pre:
			switch r {
			case '\n':
				sb.WriteByte('\n')
			case '\r':
				// "\r\n" и одиночный '\r' — один перевод строки.
			case '\t':
				sb.WriteString("    ")
			default:
				if r >= 0x20 && !(r >= 0x7f && r < 0xa0) {
					sb.WriteRune(r)
				}
			}
			p.lastSpace = false
		case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f':
			if !p.lastSpace {
				sb.WriteByte(' ')
				p.lastSpace = true
			}
		case r == 0xA0:
			// Неразрывный пробел не схлопывается; в обычный пробел он
			// превращается при выпуске абзаца.
			sb.WriteRune(r)
			p.lastSpace = false
		case r < 0x20 || (r >= 0x7f && r < 0xa0) || r == 0xFEFF:
			// Управляющие символы дали бы «коробки» вместо текста.
		default:
			sb.WriteRune(r)
			p.lastSpace = false
		}
	}
	if sb.Len() == 0 {
		return
	}
	p.ensurePara()
	p.cur.touched = true
	style := top.st.font()
	if p.pendingBR {
		p.pendingBR = false
		p.trimTrailingSpace()
		p.writeRaw("\n", p.pendingBRStyle)
	}
	p.writeRaw(sb.String(), style)
}

// flush завершает текущий абзац и добавляет его в результат.
func (p *rhParser) flush() {
	c := p.cur
	p.cur = nil
	p.lastSpace = true
	p.pendingBR = false // <br> в конце блока строки не создаёт
	if c == nil || !c.touched {
		return
	}
	rhTrimParaSpace(c)
	runs := make([]RichRun, 0, len(c.pieces))
	for _, pc := range c.pieces {
		r := pc.style
		r.Text = strings.ReplaceAll(pc.sb.String(), " ", " ")
		runs = append(runs, r)
	}
	c.fmt.Runs = richTidyRuns(runs)
	p.paras = append(p.paras, c.fmt)
}

// ---------------------------------------------------------------------------
// CSS

// rhBlockProps — свойства блока, найденные в style.
type rhBlockProps struct {
	align         TextAlign
	alignSet      bool
	left          int // margin-left + padding-left, пиксели
	before, after int
	bullet        bool // абзац — пункт списка Word (mso-list)
}

// rhSplitCSS делит содержимое style на объявления по ';', не разрывая
// кавычки и скобки (url(data:...;base64,...) и 'Font; Name').
func rhSplitCSS(s string) []string {
	var out []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			if depth > 0 {
				depth--
			}
		case c == ';' && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

// applyCSS накладывает объявления на оформление и возвращает свойства блока.
func (p *rhParser) applyCSS(st *rhStyle, css string) rhBlockProps {
	var bp rhBlockProps
	var ml, pl int
	for _, decl := range rhSplitCSS(css) {
		colon := strings.IndexByte(decl, ':')
		if colon < 0 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(decl[:colon]))
		v := strings.TrimSpace(decl[colon+1:])
		if i := strings.Index(strings.ToLower(v), "!important"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		lv := strings.ToLower(v)
		switch k {
		case "color":
			if c, ok := rhParseColor(v); ok && c.A != 0 {
				st.run.Color = c
			}
		case "background-color":
			if c, ok := rhParseColor(v); ok {
				st.run.BG = c
			}
		case "background":
			for _, tok := range strings.Fields(v) {
				if c, ok := rhParseColor(tok); ok {
					st.run.BG = c
					break
				}
			}
		case "font-size":
			if sz, ok := rhParseFontSize(v, st.run.Size); ok {
				st.run.Size = sz
			}
		case "font-weight":
			switch lv {
			case "bold", "bolder":
				st.bold = true
			case "normal", "lighter":
				st.bold = false
			default:
				if n, err := strconv.Atoi(lv); err == nil {
					st.bold = n >= 600
				}
			}
		case "font-style":
			switch lv {
			case "italic", "oblique":
				st.italic = true
			case "normal":
				st.italic = false
			}
		case "font-family":
			st.mono = rhIsMonoFamily(v)
		case "text-decoration", "text-decoration-line":
			// «none» не обрабатывается: в CSS оно не снимает подчёркивание,
			// заданное предком, а в плоской модели иначе не выразить.
			if strings.Contains(lv, "underline") {
				st.run.Underline = true
			}
			if strings.Contains(lv, "line-through") {
				st.run.Strike = true
			}
		case "white-space":
			if strings.HasPrefix(lv, "pre") {
				st.pre = true
			} else if lv == "normal" || lv == "nowrap" {
				st.pre = false
			}
		case "text-align":
			if a, ok := rhParseAlign(lv); ok {
				bp.align, bp.alignSet = a, true
			}
		case "margin":
			f := strings.Fields(v)
			var t, b, l string
			switch len(f) {
			case 1:
				t, b, l = f[0], f[0], f[0]
			case 2:
				t, b, l = f[0], f[0], f[1]
			case 3:
				t, b, l = f[0], f[2], f[1]
			case 4:
				t, b, l = f[0], f[2], f[3]
			}
			if px, ok := rhParseLength(t); ok {
				bp.before = rhPx(px)
			}
			if px, ok := rhParseLength(b); ok {
				bp.after = rhPx(px)
			}
			if px, ok := rhParseLength(l); ok {
				ml = rhPx(px)
			}
		case "margin-top":
			if px, ok := rhParseLength(v); ok {
				bp.before = rhPx(px)
			}
		case "margin-bottom":
			if px, ok := rhParseLength(v); ok {
				bp.after = rhPx(px)
			}
		case "margin-left":
			if px, ok := rhParseLength(v); ok {
				ml = rhPx(px)
			}
		case "padding-left":
			if px, ok := rhParseLength(v); ok {
				pl = rhPx(px)
			}
		case "mso-list":
			// «l0 level1 lfo1» — пункт списка; «none» и «Ignore» — нет.
			if len(lv) > 1 && lv[0] == 'l' && lv[1] >= '0' && lv[1] <= '9' {
				bp.bullet = true
			}
		}
	}
	bp.left = ml + pl
	return bp
}

// rhPx округляет пиксели до неотрицательного целого в разумных пределах.
func rhPx(px float64) int {
	n := int(math.Round(px))
	return min(max(n, 0), rhMaxIndent)
}

// rhParseAlign разбирает выравнивание. justify в модели нет — это левое.
func rhParseAlign(v string) (TextAlign, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "left", "start", "justify":
		return TextAlignLeft, true
	case "center":
		return TextAlignCenter, true
	case "right", "end":
		return TextAlignRight, true
	}
	return 0, false
}

// rhSplitNumber отделяет число в начале строки от единицы измерения.
func rhSplitNumber(v string) (float64, string, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	i := 0
	for i < len(v) {
		c := v[i]
		if c >= '0' && c <= '9' || c == '.' || (i == 0 && (c == '+' || c == '-')) {
			i++
			continue
		}
		break
	}
	if i == 0 {
		return 0, "", false
	}
	n, err := strconv.ParseFloat(v[:i], 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, "", false
	}
	return n, strings.TrimSpace(v[i:]), true
}

// rhParseLength переводит длину CSS в пиксели.
func rhParseLength(v string) (float64, bool) {
	n, unit, ok := rhSplitNumber(v)
	if !ok {
		return 0, false
	}
	switch unit {
	case "", "px":
		return n, true
	case "pt":
		return n * 96 / 72, true
	case "pc":
		return n * 16, true
	case "in":
		return n * 96, true
	case "cm":
		return n * 96 / 2.54, true
	case "mm":
		return n * 96 / 25.4, true
	case "em", "rem":
		return n * 16, true
	}
	return 0, false
}

var rhFontSizeWords = map[string]float64{
	"xx-small": 6.75, "x-small": 7.5, "small": 9.75, "medium": 12,
	"large": 13.5, "x-large": 18, "xx-large": 24, "xxx-large": 36,
}

// rhParseFontSize разбирает font-size в пункты. parent — кегль родителя (0 —
// кегль виджета, неизвестный: относительные размеры считаются от 12 pt, как у
// браузера).
func rhParseFontSize(v string, parent float64) (float64, bool) {
	lv := strings.ToLower(strings.TrimSpace(v))
	if pt, ok := rhFontSizeWords[lv]; ok {
		return pt, true
	}
	base := parent
	if base <= 0 {
		base = 12
	}
	switch lv {
	case "smaller":
		return base * 0.83, true
	case "larger":
		return base * 1.2, true
	}
	n, unit, ok := rhSplitNumber(lv)
	if !ok || n <= 0 {
		return 0, false
	}
	var pt float64
	switch unit {
	case "pt":
		pt = n
	case "px", "":
		pt = n * 0.75
	case "pc":
		pt = n * 12
	case "in":
		pt = n * 72
	case "cm":
		pt = n * 72 / 2.54
	case "mm":
		pt = n * 72 / 25.4
	case "em":
		pt = n * base
	case "rem":
		pt = n * 12
	case "%":
		pt = n / 100 * base
	default:
		return 0, false
	}
	if pt <= 0 || pt > 1000 {
		return 0, false
	}
	return pt, true
}

// rhFontTagSize — атрибут size тега <font>: 1–7 или относительный (+1, -1).
func rhFontTagSize(v string) (float64, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	if v[0] == '+' || v[0] == '-' {
		n += 3
	}
	pts := [...]float64{7.5, 10, 12, 13.5, 18, 24, 36}
	n = min(max(n, 1), 7)
	return pts[n-1], true
}

// rhIsMonoFamily — в списке семейств есть моноширинное.
func rhIsMonoFamily(v string) bool {
	lv := strings.ToLower(v)
	for _, w := range []string{"monospace", "courier", "consolas", "monaco", "menlo", "lucida console", "mono"} {
		if strings.Contains(lv, w) {
			return true
		}
	}
	return false
}

var rhNamedColors = map[string][3]uint8{
	"black": {0, 0, 0}, "white": {255, 255, 255}, "red": {255, 0, 0},
	"green": {0, 128, 0}, "blue": {0, 0, 255}, "yellow": {255, 255, 0},
	"gray": {128, 128, 128}, "grey": {128, 128, 128}, "silver": {192, 192, 192},
	"orange": {255, 165, 0}, "purple": {128, 0, 128}, "maroon": {128, 0, 0},
	"navy": {0, 0, 128}, "teal": {0, 128, 128}, "olive": {128, 128, 0},
	"lime": {0, 255, 0}, "aqua": {0, 255, 255}, "cyan": {0, 255, 255},
	"fuchsia": {255, 0, 255}, "magenta": {255, 0, 255}, "brown": {165, 42, 42},
	"pink": {255, 192, 203}, "gold": {255, 215, 0}, "darkgray": {169, 169, 169},
	"darkgrey": {169, 169, 169}, "lightgray": {211, 211, 211},
	"lightgrey": {211, 211, 211}, "darkred": {139, 0, 0},
	"darkgreen": {0, 100, 0}, "darkblue": {0, 0, 139},
	"crimson": {220, 20, 60}, "indigo": {75, 0, 130}, "violet": {238, 130, 238},
	"coral": {255, 127, 80}, "tomato": {255, 99, 71},
}

// rhHex разбирает hex-цифры.
func rhHex(s string) (uint8, bool) {
	n, err := strconv.ParseUint(s, 16, 8)
	return uint8(n), err == nil
}

// rhPremul собирает цвет движка: он хранит цвет с предумноженной альфой
// (см. richCSSColor, обратное преобразование).
func rhPremul(r, g, b uint8, a float64) color.RGBA {
	a = math.Min(math.Max(a, 0), 1)
	f := func(v uint8) uint8 { return uint8(math.Round(float64(v) * a)) }
	return color.RGBA{f(r), f(g), f(b), uint8(math.Round(a * 255))}
}

// rhParseColor разбирает цвет CSS: #rgb, #rgba, #rrggbb, #rrggbbaa,
// rgb()/rgba(), имена. transparent даёт цвет с нулевой альфой.
func rhParseColor(v string) (color.RGBA, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	switch {
	case v == "":
		return color.RGBA{}, false
	case v == "transparent":
		return color.RGBA{}, true
	case v[0] == '#':
		h := v[1:]
		switch len(h) {
		case 3, 4:
			var c [4]uint8
			c[3] = 255
			for i := 0; i < len(h); i++ {
				x, ok := rhHex(string([]byte{h[i], h[i]}))
				if !ok {
					return color.RGBA{}, false
				}
				c[i] = x
			}
			return rhPremul(c[0], c[1], c[2], float64(c[3])/255), true
		case 6, 8:
			var c [4]uint8
			c[3] = 255
			for i := 0; i < len(h)/2; i++ {
				x, ok := rhHex(h[2*i : 2*i+2])
				if !ok {
					return color.RGBA{}, false
				}
				c[i] = x
			}
			return rhPremul(c[0], c[1], c[2], float64(c[3])/255), true
		}
		return color.RGBA{}, false
	case strings.HasPrefix(v, "rgb"):
		open := strings.IndexByte(v, '(')
		closeIdx := strings.LastIndexByte(v, ')')
		if open < 0 || closeIdx < open {
			return color.RGBA{}, false
		}
		args := strings.FieldsFunc(v[open+1:closeIdx], func(r rune) bool {
			return r == ',' || r == ' ' || r == '/' || r == '\t'
		})
		if len(args) < 3 || len(args) > 4 {
			return color.RGBA{}, false
		}
		var ch [3]uint8
		for i := 0; i < 3; i++ {
			n, unit, ok := rhSplitNumber(args[i])
			if !ok {
				return color.RGBA{}, false
			}
			if unit == "%" {
				n = n * 255 / 100
			}
			ch[i] = uint8(math.Min(math.Max(math.Round(n), 0), 255))
		}
		a := 1.0
		if len(args) == 4 {
			n, unit, ok := rhSplitNumber(args[3])
			if !ok {
				return color.RGBA{}, false
			}
			if unit == "%" {
				n /= 100
			}
			a = n
		}
		return rhPremul(ch[0], ch[1], ch[2], a), true
	}
	if c, ok := rhNamedColors[v]; ok {
		return color.RGBA{c[0], c[1], c[2], 255}, true
	}
	return color.RGBA{}, false
}
