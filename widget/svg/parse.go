package svg

import (
	"encoding/xml"
	"image/color"
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
	Fill        color.RGBA // валиден при HasFill && !FillCurrent
	FillOpacity float64    // 0..1 (уже с учётом group opacity)
	EvenOdd     bool       // fill-rule=evenodd

	// Обводка (базовая поддержка, см. doc.go).
	HasStroke     bool
	StrokeCurrent bool
	Stroke        color.RGBA
	StrokeWidth   float64 // в координатах viewBox (масштаб предков учтён)
	StrokeOpacity float64
}

// Document — разобранный SVG: система координат (viewBox) и список фигур.
type Document struct {
	// ViewBox: [minX, minY, width, height] в пользовательских координатах.
	ViewBox [4]float64
	Shapes  []Shape

	mu    sync.Mutex
	cache map[rasterKey]*rasterEntry
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
	}
}

// xnode — универсальный XML-узел (произвольная вложенность).
type xnode struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Nodes   []xnode    `xml:",any"`
}

func (n xnode) attr(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

// Parse разбирает SVG-данные в Document. Возвращает ошибку только при
// некорректном XML; неизвестные элементы/атрибуты молча игнорируются.
func Parse(data []byte) (*Document, error) {
	var root xnode
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	doc := &Document{}
	st := defaultInherited()

	// viewBox / width / height берём с корневого <svg> (или с самого root,
	// если это уже svg).
	vbSet := false
	if strings.EqualFold(root.XMLName.Local, "svg") {
		vbSet = applyViewBox(doc, root)
	}

	walk(root, st, doc)

	if !vbSet {
		// Нет viewBox/размеров — вычислим по границам содержимого.
		fitViewBox(doc)
	}
	return doc, nil
}

// ParseFile читает и разбирает SVG-файл.
func ParseFile(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// applyViewBox заполняет doc.ViewBox из атрибутов элемента svg.
// Возвращает true, если удалось определить рамку.
func applyViewBox(doc *Document, el xnode) bool {
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

// walk рекурсивно обходит дерево, накапливая состояние и собирая фигуры.
func walk(n xnode, parent inherited, doc *Document) {
	st := resolveState(n, parent)
	tag := strings.ToLower(n.XMLName.Local)

	switch tag {
	case "svg", "g", "a", "switch":
		// контейнеры — только рекурсия
	case "path":
		if d, ok := n.attr("d"); ok {
			addShape(doc, st, ParsePathData(d))
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
		addShape(doc, st, rectContours(x, y, w, h, rx, ry))
	case "circle":
		cx := lenAttr(n, "cx")
		cy := lenAttr(n, "cy")
		r := lenAttr(n, "r")
		addShape(doc, st, circleContours(cx, cy, r))
	case "ellipse":
		cx := lenAttr(n, "cx")
		cy := lenAttr(n, "cy")
		rx := lenAttr(n, "rx")
		ry := lenAttr(n, "ry")
		addShape(doc, st, ellipseContours(cx, cy, rx, ry))
	case "line":
		x1 := lenAttr(n, "x1")
		y1 := lenAttr(n, "y1")
		x2 := lenAttr(n, "x2")
		y2 := lenAttr(n, "y2")
		addShape(doc, st, lineContour(x1, y1, x2, y2))
	case "polyline":
		if s, ok := n.attr("points"); ok {
			addShape(doc, st, polyContours(parsePointList(s), false))
		}
	case "polygon":
		if s, ok := n.attr("points"); ok {
			addShape(doc, st, polyContours(parsePointList(s), true))
		}
	default:
		// неизвестный элемент — всё равно обходим детей (мог быть контейнер)
	}

	for _, c := range n.Nodes {
		walk(c, st, doc)
	}
}

// resolveState вычисляет наследуемое состояние для элемента n.
func resolveState(n xnode, parent inherited) inherited {
	st := parent

	// transform
	if s, ok := n.attr("transform"); ok {
		st.transform = parent.transform.Mul(ParseTransform(s))
	}

	// Презентационные свойства: сначала атрибуты, затем style="" (важнее).
	get := presentationGetter(n)

	if v, ok := get("fill"); ok {
		p := ParsePaint(v)
		if p.Kind != PaintInherit {
			st.fill = p
		}
	}
	if v, ok := get("fill-rule"); ok {
		st.fillRule = strings.EqualFold(strings.TrimSpace(v), "evenodd")
	}
	if v, ok := get("fill-opacity"); ok {
		st.fillOpacity = clampUnit(parseOpacity(v))
	}
	if v, ok := get("stroke"); ok {
		p := ParsePaint(v)
		if p.Kind != PaintInherit {
			st.stroke = p
		}
	}
	if v, ok := get("stroke-width"); ok {
		st.strokeWidth = parseLength(v)
	}
	if v, ok := get("stroke-opacity"); ok {
		st.strokeOpacity = clampUnit(parseOpacity(v))
	}
	if v, ok := get("opacity"); ok {
		st.opacity = parent.opacity * clampUnit(parseOpacity(v))
	}
	return st
}

// presentationGetter возвращает функцию доступа к свойству с учётом style="".
func presentationGetter(n xnode) func(string) (string, bool) {
	styleMap := map[string]string{}
	if s, ok := n.attr("style"); ok {
		for _, decl := range strings.Split(s, ";") {
			kv := strings.SplitN(decl, ":", 2)
			if len(kv) == 2 {
				styleMap[strings.ToLower(strings.TrimSpace(kv[0]))] = strings.TrimSpace(kv[1])
			}
		}
	}
	return func(name string) (string, bool) {
		if v, ok := styleMap[name]; ok {
			return v, true
		}
		return n.attr(name)
	}
}

// addShape формирует Shape из контуров (в локальных координатах) и состояния,
// применяя transform к точкам.
func addShape(doc *Document, st inherited, contours []Contour) {
	if len(contours) == 0 {
		return
	}
	hasFill := st.fill.Kind == PaintColor || st.fill.Kind == PaintCurrent
	hasStroke := (st.stroke.Kind == PaintColor || st.stroke.Kind == PaintCurrent) && st.strokeWidth > 0
	if !hasFill && !hasStroke {
		return
	}

	// Применяем transform к точкам.
	tc := make([]Contour, len(contours))
	for i, c := range contours {
		pts := make([]Point, len(c.Points))
		for j, p := range c.Points {
			pts[j] = st.transform.Apply(p)
		}
		tc[i] = Contour{Points: pts, Closed: c.Closed}
	}

	sh := Shape{
		Paths:         tc,
		HasFill:       hasFill,
		FillCurrent:   st.fill.Kind == PaintCurrent,
		Fill:          st.fill.Color,
		FillOpacity:   st.fillOpacity * st.opacity,
		EvenOdd:       st.fillRule,
		HasStroke:     hasStroke,
		StrokeCurrent: st.stroke.Kind == PaintCurrent,
		Stroke:        st.stroke.Color,
		StrokeWidth:   st.strokeWidth * st.transform.AvgScale(),
		StrokeOpacity: st.strokeOpacity * st.opacity,
	}
	doc.Shapes = append(doc.Shapes, sh)
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

func lenAttr(n xnode, name string) float64 {
	if s, ok := n.attr(name); ok {
		return parseLength(s)
	}
	return 0
}

func numAttr(n xnode, name string) (float64, bool) {
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
