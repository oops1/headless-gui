package svg

import (
	"math"
	"strings"
	"testing"
)

// boxText — растеризатор-заглушка: каждая буква — квадрат size×size над
// базовой линией, шаг буквы равен size (пробел — пустой). Запоминает вызовы.
type boxText struct {
	calls []boxCall
}

type boxCall struct {
	font FontSpec
	size float64
	text string
}

func (b *boxText) Outline(f FontSpec, size float64, text string) ([]Contour, float64, bool) {
	b.calls = append(b.calls, boxCall{f, size, text})
	var cs []Contour
	x := 0.0
	for _, r := range text {
		if r != ' ' {
			cs = append(cs, Contour{Closed: true, Points: []Point{{x, -size}, {x + size, -size}, {x + size, 0}, {x, 0}}})
		}
		x += size
	}
	return cs, x, true
}

func textDoc(t *testing.T, tr TextRasterizer, body string) *Document {
	t.Helper()
	doc, err := ParseWith([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 100">`+body+`</svg>`), ParseOptions{Text: tr})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

// bboxOf — габариты фигуры i в координатах viewBox.
func bboxOf(doc *Document, i int) (x0, y0, x1, y1 float64) {
	x0, y0, x1, y1, _ = contoursBBox(doc.Shapes[i].Paths)
	return
}

func near2(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestText_NoBridgeDrawsNothing(t *testing.T) {
	doc, err := Parse([]byte(`<svg viewBox="0 0 8 8"><text x="0" y="6" font-size="6">A</text><rect width="2" height="2"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 1 {
		t.Errorf("без растеризатора текста быть не должно: shapes=%d", len(doc.Shapes))
	}
}

func TestText_PositionAndFont(t *testing.T) {
	tr := &boxText{}
	doc := textDoc(t, tr, `<text x="10" y="50" font-size="20" font-family="'Open Sans', Arial, sans-serif" font-weight="bold" font-style="italic" fill="#f00">AB</text>`)
	if len(doc.Shapes) != 1 {
		t.Fatalf("shapes=%d", len(doc.Shapes))
	}
	x0, y0, x1, y1 := bboxOf(doc, 0)
	if !near2(x0, 10) || !near2(x1, 50) || !near2(y0, 30) || !near2(y1, 50) {
		t.Errorf("габариты (%v,%v)-(%v,%v), ожидались (10,30)-(50,50)", x0, y0, x1, y1)
	}
	c := tr.calls[0]
	if c.size != 20 || c.font.Weight != 700 || !c.font.Italic || len(c.font.Families) != 3 || c.font.Families[0] != "Open Sans" || c.font.Families[2] != "sans-serif" {
		t.Errorf("шрифт передан неверно: %+v", c)
	}
	if doc.Shapes[0].Fill.R != 255 || doc.Shapes[0].Fill.G != 0 {
		t.Errorf("fill текста: %v", doc.Shapes[0].Fill)
	}
}

func TestText_AnchorMiddleAndEnd(t *testing.T) {
	tr := &boxText{}
	doc := textDoc(t, tr, `<text x="100" y="20" font-size="10" text-anchor="middle">ABCD</text>
<text x="100" y="40" font-size="10" text-anchor="end">ABCD</text>
<text x="100" y="60" font-size="10" style="text-anchor:middle">AB</text>`)
	// ширина ABCD = 40
	x0, _, x1, _ := bboxOf(doc, 0)
	if !near2(x0, 80) || !near2(x1, 120) {
		t.Errorf("middle: %v..%v, ожидалось 80..120", x0, x1)
	}
	x0, _, x1, _ = bboxOf(doc, 1)
	if !near2(x0, 60) || !near2(x1, 100) {
		t.Errorf("end: %v..%v, ожидалось 60..100", x0, x1)
	}
	x0, _, x1, _ = bboxOf(doc, 2)
	if !near2(x0, 90) || !near2(x1, 110) {
		t.Errorf("middle из style: %v..%v, ожидалось 90..110", x0, x1)
	}
}

func TestText_TspanAbsoluteAndRelative(t *testing.T) {
	tr := &boxText{}
	doc := textDoc(t, tr, `<text x="10" y="30" font-size="10" fill="#000">AB<tspan fill="#00f" dx="5" dy="10">C</tspan><tspan x="10" y="80" font-size="20">D</tspan></text>`)
	if len(doc.Shapes) != 3 {
		t.Fatalf("shapes=%d, ожидалось 3 (AB, C, D)", len(doc.Shapes))
	}
	// C: после AB перо в 30, +dx 5 → 35; y 30+10=40 → верх 30, низ 40
	x0, y0, x1, y1 := bboxOf(doc, 1)
	if !near2(x0, 35) || !near2(x1, 45) || !near2(y0, 30) || !near2(y1, 40) {
		t.Errorf("tspan dx/dy: (%v,%v)-(%v,%v)", x0, y0, x1, y1)
	}
	if doc.Shapes[1].Fill.B != 255 || doc.Shapes[1].Fill.R != 0 {
		t.Errorf("fill tspan: %v", doc.Shapes[1].Fill)
	}
	// D: абсолютная позиция (10,80), размер 20
	x0, y0, x1, y1 = bboxOf(doc, 2)
	if !near2(x0, 10) || !near2(x1, 30) || !near2(y0, 60) || !near2(y1, 80) {
		t.Errorf("tspan x/y/size: (%v,%v)-(%v,%v)", x0, y0, x1, y1)
	}
}

func TestText_PerCharacterLists(t *testing.T) {
	tr := &boxText{}
	doc := textDoc(t, tr, `<text x="10 40 70" y="50" font-size="10">ABC</text>`)
	// Три буквы — три отдельных куска, каждая в своей x.
	if len(doc.Shapes) != 3 {
		t.Fatalf("shapes=%d", len(doc.Shapes))
	}
	for i, want := range []float64{10, 40, 70} {
		if x0, _, _, _ := bboxOf(doc, i); !near2(x0, want) {
			t.Errorf("буква %d: x=%v, ожидалось %v", i, x0, want)
		}
	}
}

func TestText_WhitespaceCollapsed(t *testing.T) {
	tr := &boxText{}
	textDoc(t, tr, "<text x=\"0\" y=\"20\" font-size=\"10\">\n   A \n\t B  </text>")
	if len(tr.calls) == 0 {
		t.Fatal("нет вызовов")
	}
	// Первый вызов — со всеми схлопнутыми пробелами, последний — после обрезки
	// хвостового (она нужна, чтобы text-anchor не сдвигался на пробел).
	if first := tr.calls[0].text; first != "A B " {
		t.Errorf("схлопнутый текст %q, ожидалось %q", first, "A B ")
	}
	if last := tr.calls[len(tr.calls)-1].text; last != "A B" {
		t.Errorf("текст после обрезки хвоста %q, ожидалось %q", last, "A B")
	}
}

func TestText_PreserveKeepsSpaces(t *testing.T) {
	tr := &boxText{}
	textDoc(t, tr, `<text x="0" y="20" font-size="10" xml:space="preserve">A  B</text>`)
	if tr.calls[0].text != "A  B" {
		t.Errorf("xml:space=preserve: %q", tr.calls[0].text)
	}
}

func TestText_FontSizeUnitsAndInheritance(t *testing.T) {
	tr := &boxText{}
	textDoc(t, tr, `<g font-size="10" font-family="Foo"><text x="0" y="20">A</text><text x="0" y="40" font-size="2em">B</text><text x="0" y="60" font-size="150%">C</text><text x="0" y="80" font-size="12pt">D</text></g>`)
	want := []float64{10, 20, 15, 16}
	for i, c := range tr.calls {
		if !near2(c.size, want[i]) || c.font.Families[0] != "Foo" {
			t.Errorf("текст %d: size=%v families=%v, ожидалось %v Foo", i, c.size, c.font.Families, want[i])
		}
	}
}

func TestText_FillStrokeGradientAndOpacity(t *testing.T) {
	tr := &boxText{}
	doc := textDoc(t, tr, `<defs><linearGradient id="g"><stop offset="0" stop-color="#f00"/><stop offset="1" stop-color="#00f"/></linearGradient></defs>
<text x="10" y="50" font-size="40" fill="url(#g)" stroke="#000" stroke-width="2" opacity="0.5">AB</text>`)
	sh := doc.Shapes[0]
	if sh.FillGradient == nil || !sh.HasStroke || sh.StrokeWidth != 2 {
		t.Errorf("градиент/обводка текста: %+v", sh)
	}
	if sh.FillOpacity != 0.5 || len(sh.Groups) != 1 || sh.Groups[0].Opacity != 0.5 {
		t.Errorf("opacity текста должна быть в Group ровно один раз: fo=%v groups=%v", sh.FillOpacity, sh.Groups)
	}
}

func TestText_TransformAndDisplayNone(t *testing.T) {
	tr := &boxText{}
	doc := textDoc(t, tr, `<text x="0" y="10" font-size="10" transform="translate(50 20) scale(2)">A</text><text x="0" y="10" font-size="10" display="none">B</text><text x="0" y="10" font-size="10" visibility="hidden">C</text>`)
	if len(doc.Shapes) != 1 {
		t.Fatalf("shapes=%d (скрытые не рисуются)", len(doc.Shapes))
	}
	x0, y0, x1, y1 := bboxOf(doc, 0)
	if !near2(x0, 50) || !near2(x1, 70) || !near2(y0, 20) || !near2(y1, 40) {
		t.Errorf("transform: (%v,%v)-(%v,%v)", x0, y0, x1, y1)
	}
}

func TestText_ClipPathFromText(t *testing.T) {
	tr := &boxText{}
	doc := textDoc(t, tr, `<defs><clipPath id="c"><text x="0" y="50" font-size="50">A</text></clipPath></defs>
<rect width="200" height="100" fill="#000" clip-path="url(#c)"/>`)
	img := doc.Rasterize(200, 100, color4(0, 0, 0), false)
	if img.RGBAAt(25, 25).A != 255 || img.RGBAAt(100, 25).A != 0 {
		t.Errorf("clip по тексту: внутри %v, снаружи %v", img.RGBAAt(25, 25), img.RGBAAt(100, 25))
	}
}

func TestText_RunLimit(t *testing.T) {
	tr := &boxText{}
	body := `<text x="0" y="10" font-size="1">` + strings.Repeat("A", 1) + `</text>`
	var sb strings.Builder
	for i := 0; i < maxTextRuns+50; i++ {
		sb.WriteString(body)
	}
	doc := textDoc(t, tr, sb.String())
	if len(doc.Shapes) > maxTextRuns {
		t.Errorf("кусков текста %d больше предела %d", len(doc.Shapes), maxTextRuns)
	}
}

func TestText_RegisterUnregisterAndParseUsesLatest(t *testing.T) {
	a, b := &boxText{}, &boxText{}
	ha := RegisterTextRasterizer(a)
	hb := RegisterTextRasterizer(b)
	defer UnregisterTextRasterizer(ha)
	if _, err := Parse([]byte(`<svg viewBox="0 0 8 8"><text y="6" font-size="6">A</text></svg>`)); err != nil {
		t.Fatal(err)
	}
	if len(b.calls) != 1 || len(a.calls) != 0 {
		t.Errorf("отвечает последний: a=%d b=%d", len(a.calls), len(b.calls))
	}
	UnregisterTextRasterizer(hb)
	if _, err := Parse([]byte(`<svg viewBox="0 0 8 8"><text y="6" font-size="6">A</text></svg>`)); err != nil {
		t.Fatal(err)
	}
	if len(a.calls) != 1 {
		t.Errorf("после снятия b отвечает a: a=%d", len(a.calls))
	}
	UnregisterTextRasterizer(ha)
	doc, _ := Parse([]byte(`<svg viewBox="0 0 8 8"><text y="6" font-size="6">A</text></svg>`))
	if len(doc.Shapes) != 0 {
		t.Error("все сняты: текста быть не должно")
	}
	UnregisterTextRasterizer(12345) // неизвестный дескриптор — не паника
	if RegisterTextRasterizer(nil) != 0 {
		t.Error("nil не регистрируется")
	}
}

func TestText_FailingRasterizerDrawsNothing(t *testing.T) {
	doc := textDoc(t, failText{}, `<text x="0" y="10" font-size="10">A</text><rect width="2" height="2"/>`)
	if len(doc.Shapes) != 1 {
		t.Errorf("ok=false: текст не рисуется, прямоугольник — да; shapes=%d", len(doc.Shapes))
	}
}

type failText struct{}

func (failText) Outline(FontSpec, float64, string) ([]Contour, float64, bool) {
	return nil, 0, false
}

func TestParseFontSizeAndWeight(t *testing.T) {
	cases := []struct {
		in     string
		parent float64
		want   float64
		ok     bool
	}{
		{"12", 10, 12, true}, {"12px", 10, 12, true}, {"1.5em", 10, 15, true}, {"200%", 10, 20, true},
		{"larger", 10, 12, true}, {"medium", 10, 16, true}, {"0", 10, 0, false}, {"-3", 10, 0, false}, {"abc", 10, 0, false},
	}
	for _, c := range cases {
		got, ok := parseFontSize(c.in, c.parent)
		if ok != c.ok || (ok && !near2(got, c.want)) {
			t.Errorf("parseFontSize(%q)=%v,%v, ожидалось %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
	if w, _ := parseFontWeight("bold", 400); w != 700 {
		t.Error("bold")
	}
	if w, _ := parseFontWeight("bolder", 400); w != 700 {
		t.Error("bolder")
	}
	if w, _ := parseFontWeight("600", 400); w != 600 {
		t.Error("600")
	}
}
