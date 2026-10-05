package svg

import (
	"math"
	"strings"
	"sync"
)

// FontSpec — какой шрифт нужен тексту: то, что SVG говорит словами
// (font-family, font-weight, font-style), без привязки к каким-либо файлам.
type FontSpec struct {
	// Families — font-family по порядку предпочтения (имена без кавычек,
	// включая общие serif, sans-serif, monospace). Пусто — шрифт по умолчанию.
	Families []string
	// Weight — font-weight числом (400 — обычный, 700 — жирный).
	Weight int
	// Italic — font-style italic или oblique.
	Italic bool
}

// TextRasterizer — мост между <text> и шрифтами. Пакет svg намеренно не знает
// ни о файлах шрифтов, ни о растеризаторе движка: движок (или приложение)
// регистрирует реализацию, а пакет просит у неё контуры букв и сам рисует их
// как обычные фигуры — поэтому на текст действуют fill, stroke, градиенты,
// clip-path и mask, как на любую фигуру.
type TextRasterizer interface {
	// Outline раскладывает строку text (одна строка, без переносов) шрифтом
	// font размера size (font-size, в единицах пользователя SVG) и возвращает
	// контуры букв, а также ширину строки advance (с кернингом).
	//
	// Начало координат — начало базовой линии, ось Y направлена вниз (буквы
	// над линией имеют отрицательный Y). Контуры замкнуты (Closed) и заливаются
	// правилом ненулевого числа оборотов, как в TrueType/OpenType. ok=false —
	// шрифта нет или он не умеет отдавать контуры: текст не рисуется.
	Outline(font FontSpec, size float64, text string) (contours []Contour, advance float64, ok bool)
}

type textReg struct {
	handle uint64
	tr     TextRasterizer
}

var (
	textMu   sync.Mutex
	textRegs []textReg
	textSeq  uint64
)

// RegisterTextRasterizer регистрирует растеризатор текста и делает его
// действующим для последующих Parse: отвечает последний зарегистрированный.
// Возвращает дескриптор для UnregisterTextRasterizer. Без зарегистрированного
// растеризатора <text> не рисуется.
//
// Раскладка текста происходит при РАЗБОРЕ документа, поэтому регистрировать
// надо до Parse: документ, разобранный раньше, останется без текста. Движок
// регистрирует свой растеризатор сам, в engine.New.
func RegisterTextRasterizer(tr TextRasterizer) uint64 {
	if tr == nil {
		return 0
	}
	textMu.Lock()
	defer textMu.Unlock()
	textSeq++
	textRegs = append(textRegs, textReg{handle: textSeq, tr: tr})
	return textSeq
}

// UnregisterTextRasterizer снимает регистрацию: действующим снова становится
// предыдущий. Неизвестный дескриптор игнорируется.
func UnregisterTextRasterizer(handle uint64) {
	textMu.Lock()
	defer textMu.Unlock()
	for i, r := range textRegs {
		if r.handle == handle {
			textRegs = append(textRegs[:i:i], textRegs[i+1:]...)
			return
		}
	}
}

func currentTextRasterizer() TextRasterizer {
	textMu.Lock()
	defer textMu.Unlock()
	if n := len(textRegs); n > 0 {
		return textRegs[n-1].tr
	}
	return nil
}

// ParseOptions — параметры разбора для ParseWith.
type ParseOptions struct {
	// Text — растеризатор для <text> именно этого разбора; nil — действующий
	// зарегистрированный (RegisterTextRasterizer).
	Text TextRasterizer
}

// ParseWith — Parse с параметрами.
func ParseWith(data []byte, o ParseOptions) (*Document, error) {
	return parseDoc(data, o.Text)
}

// maxTextRuns — предел кусков текста на документ: текст не должен раздувать
// файл в миллионы контуров.
const maxTextRuns = 20000

// ── шрифтовые свойства ───────────────────────────────────────────────────────

const (
	anchorStart uint8 = iota
	anchorMiddle
	anchorEnd
)

// fontProps — наследуемые свойства шрифта.
type fontProps struct {
	families []string
	size     float64
	weight   int
	italic   bool
	anchor   uint8
}

func defaultFont() fontProps { return fontProps{size: 12, weight: 400} }

func (f fontProps) spec() FontSpec {
	return FontSpec{Families: f.families, Weight: f.weight, Italic: f.italic}
}

// resolveFont читает свойства шрифта элемента поверх унаследованных.
func resolveFont(get propGetter, parent fontProps) fontProps {
	f := parent
	if v, ok := get.get("font-family"); ok {
		f.families = parseFontFamilies(v)
	}
	if v, ok := get.get("font-size"); ok {
		if sz, ok := parseFontSize(v, parent.size); ok {
			f.size = sz
		}
	}
	if v, ok := get.get("font-weight"); ok {
		if w, ok := parseFontWeight(v, parent.weight); ok {
			f.weight = w
		}
	}
	if v, ok := get.get("font-style"); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "italic", "oblique":
			f.italic = true
		case "normal":
			f.italic = false
		}
	}
	if v, ok := get.get("text-anchor"); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "start":
			f.anchor = anchorStart
		case "middle":
			f.anchor = anchorMiddle
		case "end":
			f.anchor = anchorEnd
		}
	}
	return f
}

func parseFontFamilies(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseFontSize разбирает font-size: число (px), единицы, em/ex/% от размера
// родителя, ключевые слова.
func parseFontSize(v string, parent float64) (float64, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return 0, false
	}
	switch v {
	case "xx-small":
		return 9, true
	case "x-small":
		return 10, true
	case "small":
		return 13, true
	case "medium":
		return 16, true
	case "large":
		return 18, true
	case "x-large":
		return 24, true
	case "xx-large":
		return 32, true
	case "smaller":
		return parent / 1.2, true
	case "larger":
		return parent * 1.2, true
	}
	units := []struct {
		suffix string
		k      float64
		rel    bool // от размера родителя
	}{
		{"px", 1, false}, {"pt", 4.0 / 3, false}, {"pc", 16, false},
		{"mm", 96 / 25.4, false}, {"cm", 96 / 2.54, false}, {"in", 96, false},
		{"em", 1, true}, {"ex", 0.5, true}, {"%", 0.01, true},
	}
	k, rel, num := 1.0, false, v
	for _, u := range units {
		if strings.HasSuffix(v, u.suffix) {
			k, rel, num = u.k, u.rel, strings.TrimSuffix(v, u.suffix)
			break
		}
	}
	f := parseLength(num)
	if f <= 0 || math.IsNaN(f) {
		return 0, false
	}
	f *= k
	if rel {
		f *= parent
	}
	if f > 10000 {
		f = 10000
	}
	return f, true
}

func parseFontWeight(v string, parent int) (int, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "normal":
		return 400, true
	case "bold":
		return 700, true
	case "bolder":
		return minInt(parent+300, 900), true
	case "lighter":
		return maxInt(parent-300, 100), true
	}
	f := parseLength(v)
	if f < 1 || f > 1000 {
		return 0, false
	}
	return int(f + 0.5), true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ── раскладка ────────────────────────────────────────────────────────────────

// textRun — кусок текста одним шрифтом и стилем, уже разложенный.
type textRun struct {
	text     string
	contours []Contour
	x, y     float64 // начало базовой линии
	adv      float64
	st       inherited
}

// textLayout — состояние раскладки одного <text>.
type textLayout struct {
	b  *builder
	tr TextRasterizer

	x, y  float64   // перо
	chunk []textRun // текущий «кусок»: общий text-anchor
	// Очереди значений x, y, dx, dy по буквам (списки в атрибутах).
	xq, yq, dxq, dyq []float64

	preserve  bool // xml:space="preserve"
	started   bool // уже выведена непустая буква (для обрезки пробелов в начале)
	lastSpace bool // предыдущая буква — пробел (для схлопывания)
}

// walkText рисует <text>: раскладывает буквы через TextRasterizer и добавляет
// контуры как обычные фигуры. Без растеризатора текст не рисуется.
func (b *builder) walkText(n *xnode, st inherited, depth int) {
	if b.text == nil || st.hidden && !b.noEffects && !b.clipMode {
		return
	}
	st.own = 1 // opacity самого <text> уже в его Group; у букв она не повторяется
	tl := &textLayout{b: b, tr: b.text}
	if v, ok := n.attr("space"); ok && strings.TrimSpace(v) == "preserve" {
		tl.preserve = true
	}
	tl.node(n, st, depth)
	tl.flush(true)
}

// node обходит <text>/<tspan>: позиции из атрибутов, затем содержимое.
func (tl *textLayout) node(n *xnode, st inherited, depth int) {
	if depth >= MaxDepth {
		return
	}
	// Списки x/y/dx/dy идут по буквам, начиная с первой буквы элемента, —
	// поэтому они только встают в очередь, а берутся в text().
	if v, ok := n.attr("x"); ok {
		if xs := parseLengthList(v); len(xs) > 0 {
			tl.xq = xs
		}
	}
	if v, ok := n.attr("y"); ok {
		if ys := parseLengthList(v); len(ys) > 0 {
			tl.yq = ys
		}
	}
	if v, ok := n.attr("dx"); ok {
		tl.dxq = parseLengthList(v)
	}
	if v, ok := n.attr("dy"); ok {
		tl.dyq = parseLengthList(v)
	}
	for i := range n.Nodes {
		c := &n.Nodes[i]
		switch strings.ToLower(c.XMLName.Local) {
		case "#text":
			tl.text(c.Text, st)
		case "tspan", "a":
			pg := tl.b.props(c)
			if v, ok := pg.get("display"); ok && strings.TrimSpace(v) == "none" {
				continue
			}
			tl.node(c, tl.b.resolveState(c, st, pg), depth+1)
		}
	}
}

func isXMLSpace(r rune) bool { return r == ' ' || r == '\n' || r == '\r' || r == '\t' }

// text выводит строку s: нормализует пробелы и кладёт буквы по очередям
// позиций (по одной), а остаток — одним куском с кернингом.
func (tl *textLayout) text(s string, st inherited) {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case isXMLSpace(r) && tl.preserve:
			sb.WriteByte(' ')
		case isXMLSpace(r):
			if tl.started && !tl.lastSpace {
				sb.WriteByte(' ')
			}
			tl.lastSpace = true
		default:
			sb.WriteRune(r)
			tl.lastSpace = false
			tl.started = true
		}
	}
	runes := []rune(sb.String())
	for i := 0; i < len(runes); {
		if len(tl.xq) > 0 || len(tl.yq) > 0 {
			tl.flush(false) // абсолютная позиция начинает новый кусок
		}
		if len(tl.xq) > 0 {
			tl.x, tl.xq = tl.xq[0], tl.xq[1:]
		}
		if len(tl.yq) > 0 {
			tl.y, tl.yq = tl.yq[0], tl.yq[1:]
		}
		if len(tl.dxq) > 0 {
			tl.x, tl.dxq = tl.x+tl.dxq[0], tl.dxq[1:]
		}
		if len(tl.dyq) > 0 {
			tl.y, tl.dyq = tl.y+tl.dyq[0], tl.dyq[1:]
		}
		// Очереди опустели — остаток строки идёт одним куском (с кернингом);
		// иначе следующей букве нужна своя позиция, и эта идёт отдельно.
		if len(tl.xq) == 0 && len(tl.yq) == 0 && len(tl.dxq) == 0 && len(tl.dyq) == 0 {
			tl.addRun(string(runes[i:]), st)
			return
		}
		tl.addRun(string(runes[i]), st)
		i++
	}
}

// addRun раскладывает кусок text шрифтом st.font и сдвигает перо.
func (tl *textLayout) addRun(text string, st inherited) {
	sh := tl.b.shared
	if text == "" || sh.textRuns >= maxTextRuns || st.font.size <= 0 {
		return
	}
	sh.textRuns++
	cs, adv, ok := tl.tr.Outline(st.font.spec(), st.font.size, text)
	if !ok || math.IsNaN(adv) || math.IsInf(adv, 0) {
		return
	}
	tl.chunk = append(tl.chunk, textRun{text: text, contours: cs, x: tl.x, y: tl.y, adv: adv, st: st})
	tl.x += adv
}

// flush завершает кусок: сдвигает его по text-anchor первой буквы и добавляет
// фигуры. final — конец <text>: хвостовые пробелы отбрасываются (xml:space по
// умолчанию), иначе они сдвигали бы выравнивание.
func (tl *textLayout) flush(final bool) {
	if len(tl.chunk) == 0 {
		return
	}
	chunk := tl.chunk
	tl.chunk = nil
	end := tl.x
	if final && !tl.preserve {
		last := &chunk[len(chunk)-1]
		if trimmed := strings.TrimRight(last.text, " "); trimmed != last.text {
			if trimmed == "" {
				end -= last.adv
				chunk = chunk[:len(chunk)-1]
			} else if cs, adv, ok := tl.tr.Outline(last.st.font.spec(), last.st.font.size, trimmed); ok {
				end -= last.adv - adv
				last.contours, last.adv, last.text = cs, adv, trimmed
			}
		}
	}
	if len(chunk) == 0 {
		return
	}
	width := end - chunk[0].x
	shift := 0.0
	switch chunk[0].st.font.anchor {
	case anchorMiddle:
		shift = -width / 2
	case anchorEnd:
		shift = -width
	}
	for i := range chunk {
		r := &chunk[i]
		st := r.st
		st.fillRule = false // контуры шрифта — по ненулевому правилу
		st.transform = st.transform.Mul(Translate(r.x+shift, r.y))
		tl.b.addShape(st, r.contours)
	}
}
