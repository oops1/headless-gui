package svg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image/color"
	"io"
	"os"
	"strings"
	"sync"
)

// Shape — одна залитая/обведённая фигура: плоские контуры (в координатах
// viewBox, все transform уже применены) плюс параметры цвета.
type Shape struct {
	Paths []Contour

	// Заливка.
	HasFill     bool
	FillCurrent bool       // fill=currentColor — подставить цвет виджета/темы
	Fill        color.RGBA // валиден при HasFill && !FillCurrent; для FillGradient — MeanColor
	FillOpacity float64    // 0..1 (уже с учётом group opacity)
	EvenOdd     bool       // fill-rule=evenodd
	// FillGradient — заливка fill="url(#градиент)". Если задан, цвет берётся из
	// градиента, а Fill хранит его средний цвет (для тех, кто градиенты не
	// рисует). Градиент из одного стопа сводится к обычному Fill.
	FillGradient *Gradient
	// FillPattern — заливка fill="url(#pattern)": плитка узора. Тогда Fill не
	// определён (нулевой цвет): узор рисует только растеризатор.
	FillPattern *Pattern

	// Обводка (базовая поддержка, см. doc.go).
	HasStroke      bool
	StrokeCurrent  bool
	Stroke         color.RGBA
	StrokeWidth    float64 // в координатах viewBox (масштаб предков учтён)
	StrokeOpacity  float64
	StrokeGradient *Gradient // как FillGradient, для stroke="url(#…)"
	StrokePattern  *Pattern  // как FillPattern, для stroke="url(#…)"
	// Параметры обводки, которые учитывает только режим Options.StrokeJoins
	// (stroke-linejoin/-linecap/-miterlimit/-dasharray/-dashoffset). Длины в
	// единицах viewBox, как StrokeWidth. MiterLimit 0 читается как 4.
	StrokeJoin LineJoin
	StrokeCap  LineCap
	MiterLimit float64
	Dash       []float64 // чётное число длин штрих/пробел; nil — сплошная
	DashOffset float64

	// Эффекты (всё в координатах viewBox, порядок применения: размытие, clip,
	// mask, затем цвет и непрозрачность).
	Clips       []*ClipPath // clip-path фигуры и всех её предков; результат — пересечение
	Masks       []*Mask     // mask фигуры и всех её предков; результат — произведение
	BlurX       float64     // feGaussianBlur stdDeviation, в единицах viewBox; 0 — без размытия
	BlurY       float64
	ColorMatrix *[20]float64 // feColorMatrix (строка за строкой, 4×5) над цветом заливки/обводки

	// Groups — группы предков (внешняя первой), чей эффект действует на
	// результат группы целиком: mask, filter и (с Options.GroupLayers)
	// opacity. Групповые mask/filter в Masks/BlurX/ColorMatrix фигуры не
	// попадают — они только здесь. Фигуры одной группы идут в Shapes подряд.
	Groups []*Group

	// Image — растровая картинка (<image>): тогда Paths — прямоугольник, в
	// который она рисуется, а заливка/обводка не используются.
	Image *Image
}

// Document — разобранный SVG: система координат (viewBox) и список фигур.
type Document struct {
	// ViewBox: [minX, minY, width, height] в пользовательских координатах.
	ViewBox [4]float64
	Shapes  []Shape

	mu    sync.Mutex
	cache map[rasterKey]*rasterEntry

	// Собственный режим растеризации (SetOptions); optSet=false — общий.
	optSet bool
	opt    Options
}

// inherited — наследуемое состояние при обходе дерева.
type inherited struct {
	transform     Matrix
	fill          Paint
	fillRule      bool // evenodd
	fillOpacity   float64
	stroke        Paint
	strokeWidth   float64 // в локальных координатах элемента (до его transform)
	strokeOpacity float64
	opacity       float64 // групповая непрозрачность (приближённо)
	hidden        bool    // visibility: hidden|collapse

	// own — opacity самого элемента (не накопленная): по ней строится Group.
	own float64

	// Параметры обводки (наследуются по CSS).
	lineJoin   LineJoin
	lineCap    LineCap
	miterLimit float64
	dash       []float64
	dashOffset float64

	// Шрифт (наследуется; разбирается только в документах с <text>).
	font fontProps

	// Эффекты предков: действуют на всё поддерево, а не наследуются по CSS,
	// но для плоского списка фигур накапливаются так же. Исключение — mask и
	// filter контейнеров: они в groups и рисуются слоем.
	clips        []*ClipPath
	masks        []*Mask
	blurX, blurY float64
	cmat         *[20]float64
	groups       []*Group
}

func defaultInherited() inherited {
	return inherited{
		transform:     Identity(),
		fill:          Paint{Kind: PaintColor, Color: color.RGBA{0, 0, 0, 255}}, // SVG default: black
		fillRule:      false,
		fillOpacity:   1,
		stroke:        Paint{Kind: PaintNone},
		strokeWidth:   1,
		strokeOpacity: 1,
		opacity:       1,
		own:           1,
		miterLimit:    defaultMiterLimit,
		font:          defaultFont(),
	}
}

// xnode — универсальный XML-узел (произвольная вложенность).
type xnode struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Nodes   []xnode    `xml:",any"`
	Text    string     // только для <style>: текст таблицы стилей
}

func (n *xnode) attr(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

// Parse разбирает SVG-данные в Document (лимиты MaxFileBytes и MaxDepth).
// Неизвестные элементы и атрибуты игнорируются.
func Parse(data []byte) (*Document, error) {
	return parseDoc(data, nil)
}

// parseDoc — разбор; tr — растеризатор текста (nil — действующий
// зарегистрированный).
func parseDoc(data []byte, tr TextRasterizer) (*Document, error) {
	if len(data) > MaxFileBytes {
		return nil, fmt.Errorf("svg: данные слишком велики (%d байт > %d)", len(data), MaxFileBytes)
	}
	root, err := decodeTree(data)
	if err != nil {
		return nil, err
	}

	doc := &Document{}
	st := defaultInherited()

	// viewBox / width / height берём с корневого <svg> (или с самого root,
	// если это уже svg).
	vbSet := false
	if strings.EqualFold(root.XMLName.Local, "svg") {
		vbSet = applyViewBox(doc, &root)
	}

	b := newBuilder(doc, &root)
	b.text = tr
	if b.text == nil && b.shared.hasText {
		b.text = currentTextRasterizer()
	}
	if vbSet {
		b.vpW, b.vpH = doc.ViewBox[2], doc.ViewBox[3]
	}
	b.walk(&root, st, 0)

	if !vbSet {
		// Нет viewBox/размеров — вычислим по границам содержимого.
		fitViewBox(doc)
	}
	return doc, nil
}

// MaxFileBytes — предельный размер SVG-данных (файла и Parse/SetSVG).
const MaxFileBytes = 16 << 20

// MaxDepth — предельная вложенность элементов SVG.
const MaxDepth = 256

// svgNS — пространство имён элементов SVG; чужие (метаданные редакторов)
// пропускаются вместе с содержимым.
const svgNS = "http://www.w3.org/2000/svg"

// maxUseVisits — предельное число узлов, обойденных внутри <use>: защита от
// «взрыва» вложенных ссылок, когда маленький файл раскрывается в гигантский.
const maxUseVisits = 400000

// decodeTree строит дерево потоковым декодером, без рекурсии.
func decodeTree(data []byte) (xnode, error) {
	var root xnode
	dec := xml.NewDecoder(bytes.NewReader(data))
	stack := make([]*xnode, 0, 32)
	rootDone := false
	textDepth := 0 // сколько <text> открыто: только внутри них копим буквы
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return root, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if len(stack) >= MaxDepth {
				return root, fmt.Errorf("svg: вложенность больше %d", MaxDepth)
			}
			if len(stack) == 0 {
				if rootDone {
					// Второй корневой элемент — как у xml.Unmarshal, игнорируем.
					if err := dec.Skip(); err != nil {
						return root, err
					}
					continue
				}
				root = xnode{XMLName: t.Name, Attrs: copyAttrs(t.Attr)}
				stack = append(stack, &root)
				continue
			}
			p := stack[len(stack)-1]
			p.Nodes = append(p.Nodes, xnode{XMLName: t.Name, Attrs: copyAttrs(t.Attr)})
			stack = append(stack, &p.Nodes[len(p.Nodes)-1])
			if t.Name.Local == "text" {
				textDepth++
			}
		case xml.CharData:
			// Текст нужен таблице стилей и <text>: у второго буквы лежат
			// узлами «#text» среди <tspan>, чтобы сохранить порядок.
			if len(stack) > 0 {
				top := stack[len(stack)-1]
				switch {
				case top.XMLName.Local == "style":
					top.Text += string(t)
				case textDepth > 0:
					top.Nodes = append(top.Nodes, xnode{XMLName: xml.Name{Local: "#text"}, Text: string(t)})
				}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				if stack[len(stack)-1].XMLName.Local == "text" {
					textDepth--
				}
				stack = stack[:len(stack)-1]
				if len(stack) == 0 {
					rootDone = true
				}
			}
		}
	}
	if !rootDone && len(stack) == 0 {
		return root, io.EOF // корневого элемента нет — как у xml.Unmarshal
	}
	return root, nil
}

func copyAttrs(a []xml.Attr) []xml.Attr {
	if len(a) == 0 {
		return nil
	}
	out := make([]xml.Attr, len(a))
	copy(out, a)
	return out
}

// ParseFile читает и разбирает SVG-файл. Файл больше MaxFileBytes — ошибка.
func ParseFile(path string) (*Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > MaxFileBytes {
		return nil, fmt.Errorf("svg: %s: файл слишком большой (%d байт > %d)", path, st.Size(), MaxFileBytes)
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxFileBytes {
		return nil, fmt.Errorf("svg: %s: файл слишком большой (> %d байт)", path, MaxFileBytes)
	}
	return Parse(data)
}

// applyViewBox заполняет doc.ViewBox из атрибутов элемента svg.
// Возвращает true, если удалось определить рамку.
func applyViewBox(doc *Document, el *xnode) bool {
	if vb, ok := el.attr("viewBox"); ok {
		f := parseFloats(vb)
		if len(f) == 4 && f[2] > 0 && f[3] > 0 {
			doc.ViewBox = [4]float64{f[0], f[1], f[2], f[3]}
			return true
		}
	}
	w := 0.0
	h := 0.0
	if s, ok := el.attr("width"); ok {
		w = parseLength(s)
	}
	if s, ok := el.attr("height"); ok {
		h = parseLength(s)
	}
	if w > 0 && h > 0 {
		doc.ViewBox = [4]float64{0, 0, w, h}
		return true
	}
	return false
}

// builder — состояние разбора: куда складывать фигуры и что известно о
// документе (id-индекс, таблица стилей). Вложенные построения (содержимое
// clipPath и mask, bbox группы) делают свой builder с общим shared.
type builder struct {
	doc  *Document
	ids  map[string]*xnode
	css  *styleSheet
	vpW  float64 // размер viewport для процентов в userSpaceOnUse
	vpH  float64
	root *xnode

	// clipMode — строим тело clipPath: годится только геометрия, clip-rule
	// вместо fill-rule, краски и эффекты не нужны.
	clipMode bool
	// noEffects — считаем bbox: нужна геометрия всего видимого, без краски и
	// эффектов.
	noEffects bool
	// noTransformFor — элемент, чей собственный transform не применяется (bbox
	// считается в его локальной системе координат).
	noTransformFor *xnode
	// text — растеризатор для <text>; nil — текст не рисуется.
	text TextRasterizer

	shared *buildShared
}

// buildShared — то, что делят все вложенные builder'ы одного разбора.
type buildShared struct {
	useDepth  int
	useVisits int
	usePoints int             // точек контуров, созданных внутри <use>
	activeUse map[*xnode]bool // цели <use> на текущем пути (защита от циклов)
	activeRef map[*xnode]bool // clipPath/mask, которые строятся сейчас
	grads     map[*xnode]*gradDef
	clips     map[clipKey]*ClipPath
	masks     map[maskKey]*Mask
	pixels    int // сумма пикселей декодированных <image>
	pats      map[patKey]*Pattern
	hasText   bool // в документе есть <text>: нужен разбор свойств шрифта
	hasStroke bool // есть stroke-linejoin/-linecap/-miterlimit/-dasharray/-dashoffset
	textRuns  int  // сколько кусков текста уже разложено
}

func newBuilder(doc *Document, root *xnode) *builder {
	b := &builder{
		doc:  doc,
		ids:  map[string]*xnode{},
		vpW:  100,
		vpH:  100,
		root: root,
		shared: &buildShared{
			activeUse: map[*xnode]bool{},
			activeRef: map[*xnode]bool{},
			grads:     map[*xnode]*gradDef{},
			clips:     map[clipKey]*ClipPath{},
			masks:     map[maskKey]*Mask{},
		},
	}
	var css strings.Builder
	b.index(root, 0, &css)
	if css.Len() > 0 {
		b.css = parseStyleSheet(css.String())
	}
	return b
}

// sub делает builder для вложенного построения в новый Document.
func (b *builder) sub() *builder {
	return &builder{
		doc:    &Document{},
		ids:    b.ids,
		css:    b.css,
		vpW:    b.vpW,
		vpH:    b.vpH,
		root:   b.root,
		shared: b.shared,
		text:   b.text,
	}
}

// index собирает id-индекс и текст <style>.
func (b *builder) index(n *xnode, depth int, css *strings.Builder) {
	if depth > MaxDepth {
		return
	}
	if v, ok := n.attr("id"); ok && v != "" {
		if _, dup := b.ids[v]; !dup {
			b.ids[v] = n
		}
	}
	if n.XMLName.Local == "text" {
		b.shared.hasText = true
	}
	if !b.shared.hasStroke {
		for _, at := range n.Attrs {
			if strings.HasPrefix(at.Name.Local, "stroke-") || at.Name.Local == "style" {
				if mentionsStrokeExt(at.Name.Local, at.Value) {
					b.shared.hasStroke = true
					break
				}
			}
		}
	}
	if n.Text != "" && n.XMLName.Local == "style" && mentionsStrokeExt("style", n.Text) {
		b.shared.hasStroke = true
	}
	if n.Text != "" && n.XMLName.Local == "style" {
		if t, ok := n.attr("type"); !ok || t == "" || strings.EqualFold(t, "text/css") {
			css.WriteString(n.Text)
			css.WriteByte('\n')
		}
	}
	for i := range n.Nodes {
		b.index(&n.Nodes[i], depth+1, css)
	}
}

// nonRenderingTags — элементы, которые только объявляют что-то (или рисуются
// иначе, чем поддерживается): их содержимое напрямую не рисуется. Ссылки на
// них (url(#…), <use>) разбираются отдельно.
var nonRenderingTags = map[string]bool{
	"defs": true, "clippath": true, "mask": true, "symbol": true,
	"lineargradient": true, "radialgradient": true, "pattern": true,
	"marker": true, "filter": true, "style": true, "title": true, "desc": true,
	"metadata": true, "script": true, "foreignobject": true,
	"font": true, "font-face": true, "glyph": true, "cursor": true, "view": true,
	"animate": true, "animatetransform": true, "animatemotion": true, "set": true,
	"namedview": true,
}

// propGetter — доступ к свойствам элемента с учётом таблицы стилей и style="".
type propGetter struct {
	n    *xnode
	decl map[string]string // CSS-правила + style=""; nil — только атрибуты
}

func (p propGetter) get(name string) (string, bool) {
	if p.decl != nil {
		if v, ok := p.decl[name]; ok {
			return v, true
		}
	}
	return p.n.attr(name)
}

// props собирает свойства n: атрибуты представления слабее таблицы стилей,
// она слабее style="".
func (b *builder) props(n *xnode) propGetter {
	pg := propGetter{n: n}
	pg.decl = b.css.declarationsFor(n)
	if s, ok := n.attr("style"); ok && s != "" {
		own := parseDeclarations(s)
		if len(own) > 0 {
			if pg.decl == nil {
				pg.decl = own
			} else {
				for k, v := range own {
					pg.decl[k] = v
				}
			}
		}
	}
	return pg
}

// walk рекурсивно обходит дерево, накапливая состояние и собирая фигуры.
// depth ограничена MaxDepth — страховка от глубокого дерева.
func (b *builder) walk(n *xnode, parent inherited, depth int) {
	if depth >= MaxDepth {
		return
	}
	if ns := n.XMLName.Space; ns != "" && ns != svgNS && ns != "svg" {
		return // чужое пространство имён (метаданные редакторов и т.п.)
	}
	if b.shared.useDepth > 0 {
		b.shared.useVisits++
		if b.shared.useVisits > maxUseVisits {
			return
		}
	}
	tag := strings.ToLower(n.XMLName.Local)
	if nonRenderingTags[tag] {
		return
	}

	pg := b.props(n)
	if v, ok := pg.get("display"); ok && strings.TrimSpace(v) == "none" {
		return
	}
	st := b.resolveState(n, parent, pg)
	st = b.applyEffects(n, pg, st)

	switch tag {
	case "text":
		b.walkText(n, st, depth)
		return
	case "svg", "g", "a", "switch":
		// контейнеры — только рекурсия; вложенный svg ещё и задаёт свой viewport
		if tag == "svg" && depth > 0 {
			st = b.nestedViewport(n, st)
		}
	case "use":
		b.walkUse(n, st, depth)
		return
	case "image":
		b.addImage(n, st)
		return
	case "path":
		if d, ok := n.attr("d"); ok {
			b.addShape(st, ParsePathData(d))
		}
	case "rect":
		x := lenAttr(n, "x")
		y := lenAttr(n, "y")
		w := lenAttr(n, "width")
		h := lenAttr(n, "height")
		rx, rxOK := numAttr(n, "rx")
		ry, ryOK := numAttr(n, "ry")
		if !rxOK {
			rx = ry
		}
		if !ryOK {
			ry = rx
		}
		b.addShape(st, rectContours(x, y, w, h, rx, ry))
	case "circle":
		cx := lenAttr(n, "cx")
		cy := lenAttr(n, "cy")
		r := lenAttr(n, "r")
		if len(st.dash) > 0 {
			b.addShape(st, ellipseContoursDash(cx, cy, r, r))
		} else {
			b.addShape(st, circleContours(cx, cy, r))
		}
	case "ellipse":
		cx := lenAttr(n, "cx")
		cy := lenAttr(n, "cy")
		rx := lenAttr(n, "rx")
		ry := lenAttr(n, "ry")
		if len(st.dash) > 0 {
			b.addShape(st, ellipseContoursDash(cx, cy, rx, ry))
		} else {
			b.addShape(st, ellipseContours(cx, cy, rx, ry))
		}
	case "line":
		x1 := lenAttr(n, "x1")
		y1 := lenAttr(n, "y1")
		x2 := lenAttr(n, "x2")
		y2 := lenAttr(n, "y2")
		b.addShape(st, lineContour(x1, y1, x2, y2))
	case "polyline":
		if s, ok := n.attr("points"); ok {
			b.addShape(st, polyContours(parsePointList(s), false))
		}
	case "polygon":
		if s, ok := n.attr("points"); ok {
			b.addShape(st, polyContours(parsePointList(s), true))
		}
	default:
		// неизвестный элемент — всё равно обходим детей (мог быть контейнер)
	}

	for i := range n.Nodes {
		b.walk(&n.Nodes[i], st, depth+1)
	}
}

// resolveState вычисляет наследуемое состояние для элемента n.
func (b *builder) resolveState(n *xnode, parent inherited, get propGetter) inherited {
	st := parent

	st.own = 1

	// transform
	if s, ok := n.attr("transform"); ok && n != b.noTransformFor {
		st.transform = parent.transform.Mul(ParseTransform(s))
	}

	// Презентационные свойства: сначала атрибуты, затем таблица стилей и
	// style="" (важнее).
	if v, ok := get.get("fill"); ok {
		p := ParsePaint(v)
		if p.Kind != PaintInherit {
			st.fill = p
		}
	}
	ruleProp := "fill-rule"
	if b.clipMode {
		ruleProp = "clip-rule"
	}
	if v, ok := get.get(ruleProp); ok {
		st.fillRule = strings.EqualFold(strings.TrimSpace(v), "evenodd")
	}
	if v, ok := get.get("fill-opacity"); ok {
		st.fillOpacity = clampUnit(parseOpacity(v))
	}
	if v, ok := get.get("stroke"); ok {
		p := ParsePaint(v)
		if p.Kind != PaintInherit {
			st.stroke = p
		}
	}
	if v, ok := get.get("stroke-width"); ok {
		st.strokeWidth = parseLength(v)
	}
	if v, ok := get.get("stroke-opacity"); ok {
		st.strokeOpacity = clampUnit(parseOpacity(v))
	}
	if v, ok := get.get("opacity"); ok {
		st.own = clampUnit(parseOpacity(v))
		st.opacity = parent.opacity * st.own
	}
	if b.shared.hasStroke {
		if v, ok := get.get("stroke-linejoin"); ok {
			if j, ok := parseLineJoin(v); ok {
				st.lineJoin = j
			}
		}
		if v, ok := get.get("stroke-linecap"); ok {
			if c, ok := parseLineCap(v); ok {
				st.lineCap = c
			}
		}
		if v, ok := get.get("stroke-miterlimit"); ok {
			if f := parseLength(v); f >= 1 {
				st.miterLimit = f
			}
		}
		if v, ok := get.get("stroke-dasharray"); ok {
			st.dash = parseDashArray(v)
		}
		if v, ok := get.get("stroke-dashoffset"); ok {
			st.dashOffset = parseLength(v)
		}
	}
	if b.shared.hasText {
		st.font = resolveFont(get, parent.font)
	}
	if v, ok := get.get("visibility"); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "hidden", "collapse":
			st.hidden = true
		case "visible":
			st.hidden = false
		}
	}
	return st
}

// rpaint — краска, разрешённая для конкретной фигуры.
type rpaint struct {
	kind uint8 // 0 — нет, 1 — цвет, 2 — currentColor, 3 — градиент, 4 — узор
	col  color.RGBA
	grad *Gradient
	pat  *Pattern
}

const (
	rpNone uint8 = iota
	rpColor
	rpCurrent
	rpGradient
	rpPattern
)

// resolvePaint превращает Paint в краску фигуры. Для fill="url(#…)" нужны
// контуры: у градиента objectBoundingBox берёт их габариты.
func (b *builder) resolvePaint(p Paint, st inherited, contours []Contour) rpaint {
	switch p.Kind {
	case PaintColor:
		return rpaint{kind: rpColor, col: p.Color}
	case PaintCurrent:
		return rpaint{kind: rpCurrent}
	case PaintURL:
		return b.resolveServer(p, st, contours)
	}
	return rpaint{}
}

// resolveServer находит paint server по ссылке. Ссылка на отсутствующий или
// неподдержанный объект даёт запасную краску, а без неё — «ничего» (как в
// браузерах), а не тихо чёрный.
func (b *builder) resolveServer(p Paint, st inherited, contours []Contour) rpaint {
	fallback := func() rpaint {
		if p.Fallback != nil {
			return b.resolvePaint(*p.Fallback, st, contours)
		}
		return rpaint{}
	}
	n := b.ids[p.Ref]
	if p.Ref != "" && n != nil && strings.EqualFold(n.XMLName.Local, "pattern") {
		if pat := b.patternFor(n, st, contours); pat != nil {
			return rpaint{kind: rpPattern, pat: pat}
		}
		return rpaint{} // узор найден, но рисовать нечем: «ничего»
	}
	if p.Ref == "" || n == nil || !isGradientTag(n) {
		return fallback()
	}
	def := b.gradientDef(n)
	if def == nil || len(def.g.Stops) == 0 {
		return rpaint{} // градиент без стопов: «ничего» (запасной цвет не нужен — объект найден)
	}
	if len(def.g.Stops) == 1 {
		s := def.g.Stops[0]
		if s.Current {
			c := s.Color
			return rpaint{kind: rpCurrent, col: c}
		}
		return rpaint{kind: rpColor, col: s.Color}
	}
	m := st.transform
	if def.obb {
		x0, y0, x1, y1, ok := contoursBBox(contours)
		if !ok || x1-x0 <= 0 || y1-y0 <= 0 {
			return rpaint{} // bbox нулевой ширины/высоты: градиент неприменим
		}
		m = m.Mul(Matrix{A: x1 - x0, D: y1 - y0, E: x0, F: y0})
	}
	g := def.g
	g.Transform = m.Mul(def.tr)
	return rpaint{kind: rpGradient, grad: &g, col: g.MeanColor()}
}

// contoursBBox — габариты точек контуров.
func contoursBBox(cs []Contour) (x0, y0, x1, y1 float64, ok bool) {
	for _, c := range cs {
		for _, p := range c.Points {
			if !ok {
				x0, y0, x1, y1, ok = p.X, p.Y, p.X, p.Y, true
				continue
			}
			x0 = minf(x0, p.X)
			y0 = minf(y0, p.Y)
			x1 = maxf(x1, p.X)
			y1 = maxf(y1, p.Y)
		}
	}
	return
}

// addShape формирует Shape из контуров (в локальных координатах) и состояния,
// применяя transform к точкам.
func (b *builder) addShape(st inherited, contours []Contour) {
	if len(contours) == 0 || b.overUseBudget(contours) {
		return
	}
	if b.clipMode || b.noEffects {
		b.addGeometry(st, contours)
		return
	}
	if st.hidden {
		return
	}
	fill := b.resolvePaint(st.fill, st, contours)
	stroke := b.resolvePaint(st.stroke, st, contours)
	hasFill := fill.kind != rpNone
	hasStroke := stroke.kind != rpNone && st.strokeWidth > 0
	if !hasFill && !hasStroke {
		return
	}

	scale := st.transform.AvgScale()
	groups := st.groups
	if hasFill && hasStroke && st.own < 1 {
		// Заливка и обводка одной фигуры склеиваются, и только потом
		// накладывается opacity (в режиме GroupLayers).
		groups = append(groups[:len(groups):len(groups)], &Group{Opacity: st.own})
	}
	sh := Shape{
		Paths:         applyTransform(contours, st.transform),
		HasFill:       hasFill,
		FillCurrent:   fill.kind == rpCurrent,
		Fill:          fill.col,
		FillOpacity:   st.fillOpacity * st.opacity,
		EvenOdd:       st.fillRule,
		HasStroke:     hasStroke,
		StrokeCurrent: stroke.kind == rpCurrent,
		Stroke:        stroke.col,
		StrokeWidth:   st.strokeWidth * scale,
		StrokeOpacity: st.strokeOpacity * st.opacity,
		StrokeJoin:    st.lineJoin,
		StrokeCap:     st.lineCap,
		MiterLimit:    st.miterLimit,
		Groups:        groups,
		Clips:         st.clips,
		Masks:         st.masks,
		BlurX:         st.blurX,
		BlurY:         st.blurY,
		ColorMatrix:   st.cmat,
	}
	if fill.kind == rpGradient {
		sh.FillGradient = fill.grad
	}
	if stroke.kind == rpGradient {
		sh.StrokeGradient = stroke.grad
	}
	if fill.kind == rpPattern {
		sh.FillPattern = fill.pat
	}
	if stroke.kind == rpPattern {
		sh.StrokePattern = stroke.pat
	}
	if hasStroke && len(st.dash) > 0 {
		sh.Dash = make([]float64, len(st.dash))
		for i, d := range st.dash {
			sh.Dash[i] = d * scale
		}
		sh.DashOffset = st.dashOffset * scale
	}
	b.doc.Shapes = append(b.doc.Shapes, sh)
}

// maxUsePoints — предел точек контуров, порождённых раскрытием <use>: одним
// тяжёлым путём, на который ссылаются тысячи раз, память не раздуть.
const maxUsePoints = 4 << 20

// overUseBudget учитывает точки фигуры, созданной внутри <use>, и сообщает,
// не исчерпан ли бюджет.
func (b *builder) overUseBudget(contours []Contour) bool {
	if b.shared.useDepth == 0 {
		return false
	}
	for _, c := range contours {
		b.shared.usePoints += len(c.Points)
	}
	return b.shared.usePoints > maxUsePoints
}

// addGeometry добавляет фигуру, у которой важна только геометрия (тело
// clipPath, габариты группы): заливка условно чёрная, обводки нет.
func (b *builder) addGeometry(st inherited, contours []Contour) {
	if st.hidden && b.clipMode {
		return
	}
	b.doc.Shapes = append(b.doc.Shapes, Shape{
		Paths:       applyTransform(contours, st.transform),
		HasFill:     true,
		Fill:        color.RGBA{0, 0, 0, 255},
		FillOpacity: 1,
		EvenOdd:     st.fillRule,
		Clips:       st.clips,
	})
}

func applyTransform(contours []Contour, m Matrix) []Contour {
	tc := make([]Contour, len(contours))
	for i, c := range contours {
		pts := make([]Point, len(c.Points))
		for j, p := range c.Points {
			pts[j] = m.Apply(p)
		}
		tc[i] = Contour{Points: pts, Closed: c.Closed}
	}
	return tc
}

// fitViewBox вычисляет ViewBox по границам всех точек (fallback).
func fitViewBox(doc *Document) {
	first := true
	var minX, minY, maxX, maxY float64
	for _, sh := range doc.Shapes {
		for _, c := range sh.Paths {
			for _, p := range c.Points {
				if first {
					minX, minY, maxX, maxY = p.X, p.Y, p.X, p.Y
					first = false
					continue
				}
				minX = minf(minX, p.X)
				minY = minf(minY, p.Y)
				maxX = maxf(maxX, p.X)
				maxY = maxf(maxY, p.Y)
			}
		}
	}
	if first || maxX <= minX || maxY <= minY {
		doc.ViewBox = [4]float64{0, 0, 1, 1}
		return
	}
	doc.ViewBox = [4]float64{minX, minY, maxX - minX, maxY - minY}
}

// ── мелкие помощники ─────────────────────────────────────────────────────────

func lenAttr(n *xnode, name string) float64 {
	if s, ok := n.attr(name); ok {
		return parseLength(s)
	}
	return 0
}

func numAttr(n *xnode, name string) (float64, bool) {
	if s, ok := n.attr(name); ok {
		return parseLength(s), true
	}
	return 0, false
}

func parseOpacity(s string) float64 {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		return parseLength(strings.TrimSuffix(s, "%")) / 100
	}
	return parseLength(s)
}

func clampUnit(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// mentionsStrokeExt — упоминает ли значение атрибута (name — его имя) или
// текст таблицы стилей свойства обводки, которые нужны только режиму
// StrokeJoins. Без них разбор их не ищет: плоским значкам это лишние обходы.
func mentionsStrokeExt(name, v string) bool {
	if strings.HasPrefix(name, "stroke-") {
		switch name {
		case "stroke-linejoin", "stroke-linecap", "stroke-miterlimit", "stroke-dasharray", "stroke-dashoffset":
			return true
		}
		return false
	}
	return strings.Contains(v, "stroke-line") || strings.Contains(v, "stroke-miter") || strings.Contains(v, "stroke-dash")
}
