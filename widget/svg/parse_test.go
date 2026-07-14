package svg

import (
	"image/color"
	"math"
	"testing"
)

func approx(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

// ─── Цвета ───────────────────────────────────────────────────────────────────

func TestParseColor(t *testing.T) {
	cases := []struct {
		in   string
		want color.RGBA
		ok   bool
	}{
		{"#000", color.RGBA{0, 0, 0, 255}, true},
		{"#fff", color.RGBA{255, 255, 255, 255}, true},
		{"#f00", color.RGBA{255, 0, 0, 255}, true},
		{"#ff8800", color.RGBA{255, 136, 0, 255}, true},
		{"#12345678", color.RGBA{0x12, 0x34, 0x56, 0x78}, true},
		{"rgb(255,0,0)", color.RGBA{255, 0, 0, 255}, true},
		{"rgb(100%, 0%, 0%)", color.RGBA{255, 0, 0, 255}, true},
		{"rgba(0,128,255,0.5)", color.RGBA{0, 128, 255, 128}, true},
		{"red", color.RGBA{255, 0, 0, 255}, true},
		{"white", color.RGBA{255, 255, 255, 255}, true},
		{"nonsense", color.RGBA{}, false},
	}
	for _, c := range cases {
		got, ok := ParseColor(c.in)
		if ok != c.ok {
			t.Errorf("ParseColor(%q) ok=%v want %v", c.in, ok, c.ok)
			continue
		}
		if ok && got != c.want {
			t.Errorf("ParseColor(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestParsePaintKinds(t *testing.T) {
	if p := ParsePaint("none"); p.Kind != PaintNone {
		t.Errorf("none → %v", p.Kind)
	}
	if p := ParsePaint("currentColor"); p.Kind != PaintCurrent {
		t.Errorf("currentColor → %v", p.Kind)
	}
	if p := ParsePaint(""); p.Kind != PaintInherit {
		t.Errorf("empty → %v", p.Kind)
	}
	if p := ParsePaint("#123456"); p.Kind != PaintColor {
		t.Errorf("#hex → %v", p.Kind)
	}
}

// ─── Path d ──────────────────────────────────────────────────────────────────

func TestParsePathData_Triangle(t *testing.T) {
	// M/L/Z: замкнутый треугольник.
	cs := ParsePathData("M0 0 L10 0 L10 10 Z")
	if len(cs) != 1 {
		t.Fatalf("contours=%d want 1", len(cs))
	}
	if !cs[0].Closed {
		t.Errorf("контур должен быть замкнут")
	}
	if len(cs[0].Points) < 3 {
		t.Errorf("точек=%d want >=3", len(cs[0].Points))
	}
	first := cs[0].Points[0]
	if !approx(first.X, 0, 1e-9) || !approx(first.Y, 0, 1e-9) {
		t.Errorf("первая точка %v", first)
	}
}

func TestParsePathData_RelativeAndH_V(t *testing.T) {
	// m относительный, h/v — рисуем квадрат 10×10 от (5,5).
	cs := ParsePathData("m5 5 h10 v10 h-10 z")
	if len(cs) != 1 {
		t.Fatalf("contours=%d", len(cs))
	}
	pts := cs[0].Points
	// Крайние координаты должны укладываться в [5,15].
	var minX, minY, maxX, maxY = pts[0].X, pts[0].Y, pts[0].X, pts[0].Y
	for _, p := range pts {
		minX, minY = math.Min(minX, p.X), math.Min(minY, p.Y)
		maxX, maxY = math.Max(maxX, p.X), math.Max(maxY, p.Y)
	}
	if !approx(minX, 5, 1e-6) || !approx(minY, 5, 1e-6) || !approx(maxX, 15, 1e-6) || !approx(maxY, 15, 1e-6) {
		t.Errorf("bounds min(%.2f,%.2f) max(%.2f,%.2f) want (5,5)-(15,15)", minX, minY, maxX, maxY)
	}
}

func TestParsePathData_CubicFlattening(t *testing.T) {
	// Кубическая кривая должна дать много промежуточных точек.
	cs := ParsePathData("M0 0 C0 10 10 10 10 0")
	if len(cs) != 1 {
		t.Fatalf("contours=%d", len(cs))
	}
	if len(cs[0].Points) < 5 {
		t.Errorf("кривая сплющена в %d точек, ожидалось больше", len(cs[0].Points))
	}
	last := cs[0].Points[len(cs[0].Points)-1]
	if !approx(last.X, 10, 1e-6) || !approx(last.Y, 0, 1e-6) {
		t.Errorf("конец кривой %v want (10,0)", last)
	}
}

func TestParsePathData_Arc(t *testing.T) {
	// Дуга: полуокружность радиуса 5 от (0,0) до (10,0).
	cs := ParsePathData("M0 0 A5 5 0 0 1 10 0")
	if len(cs) != 1 {
		t.Fatalf("contours=%d", len(cs))
	}
	pts := cs[0].Points
	last := pts[len(pts)-1]
	if !approx(last.X, 10, 1e-3) || !approx(last.Y, 0, 1e-3) {
		t.Errorf("конец дуги %v want (10,0)", last)
	}
	// Вершина дуги должна отклониться от прямой (не вырожденная).
	var maxY float64
	for _, p := range pts {
		maxY = math.Max(maxY, math.Abs(p.Y))
	}
	if maxY < 3 {
		t.Errorf("дуга слишком плоская, maxY=%.2f", maxY)
	}
}

// ─── Transform ───────────────────────────────────────────────────────────────

func TestParseTransform_Translate(t *testing.T) {
	m := ParseTransform("translate(10,20)")
	p := m.Apply(Point{1, 2})
	if !approx(p.X, 11, 1e-9) || !approx(p.Y, 22, 1e-9) {
		t.Errorf("translate → %v", p)
	}
}

func TestParseTransform_ScaleRotateMatrix(t *testing.T) {
	// scale
	if p := ParseTransform("scale(2,3)").Apply(Point{2, 2}); !approx(p.X, 4, 1e-9) || !approx(p.Y, 6, 1e-9) {
		t.Errorf("scale → %v", p)
	}
	// rotate 90°: (1,0) → (0,1)
	if p := ParseTransform("rotate(90)").Apply(Point{1, 0}); !approx(p.X, 0, 1e-6) || !approx(p.Y, 1, 1e-6) {
		t.Errorf("rotate → %v", p)
	}
	// matrix(a b c d e f) = matrix(1 0 0 1 5 5) → translate
	if p := ParseTransform("matrix(1 0 0 1 5 5)").Apply(Point{0, 0}); !approx(p.X, 5, 1e-9) || !approx(p.Y, 5, 1e-9) {
		t.Errorf("matrix → %v", p)
	}
}

func TestParseTransform_Compose(t *testing.T) {
	// "translate(10,0) scale(2)" применяет translate к системе, затем scale:
	// точка (1,0) → scale → (2,0) → translate → (12,0).
	m := ParseTransform("translate(10,0) scale(2)")
	p := m.Apply(Point{1, 0})
	if !approx(p.X, 12, 1e-9) || !approx(p.Y, 0, 1e-9) {
		t.Errorf("compose → %v want (12,0)", p)
	}
}

// ─── Полный документ ─────────────────────────────────────────────────────────

func TestParse_Document(t *testing.T) {
	const src = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
		<rect x="2" y="2" width="20" height="20" fill="#ff0000"/>
		<circle cx="12" cy="12" r="6" fill="currentColor"/>
		<path d="M0 0 L24 0 L24 24 Z" fill="none" stroke="#00ff00" stroke-width="2"/>
	</svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if doc.ViewBox != [4]float64{0, 0, 24, 24} {
		t.Errorf("viewBox=%v", doc.ViewBox)
	}
	if len(doc.Shapes) != 3 {
		t.Fatalf("shapes=%d want 3", len(doc.Shapes))
	}
	// rect: обычная заливка красным.
	if !doc.Shapes[0].HasFill || doc.Shapes[0].FillCurrent {
		t.Errorf("rect fill: %+v", doc.Shapes[0])
	}
	if doc.Shapes[0].Fill != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("rect fill color %v", doc.Shapes[0].Fill)
	}
	// circle: currentColor.
	if !doc.Shapes[1].FillCurrent {
		t.Errorf("circle должен быть currentColor")
	}
	// path: без заливки, с обводкой.
	if doc.Shapes[2].HasFill {
		t.Errorf("path не должен иметь fill")
	}
	if !doc.Shapes[2].HasStroke {
		t.Errorf("path должен иметь stroke")
	}
	if !approx(doc.Shapes[2].StrokeWidth, 2, 1e-9) {
		t.Errorf("stroke-width=%.3f", doc.Shapes[2].StrokeWidth)
	}
}

func TestParse_GroupTransformInheritance(t *testing.T) {
	// Группа со сдвигом — точки фигуры должны быть смещены.
	const src = `<svg viewBox="0 0 100 100">
		<g transform="translate(50,0)">
			<rect x="0" y="0" width="10" height="10" fill="#000"/>
		</g>
	</svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 1 {
		t.Fatalf("shapes=%d", len(doc.Shapes))
	}
	var minX = math.Inf(1)
	for _, c := range doc.Shapes[0].Paths {
		for _, p := range c.Points {
			minX = math.Min(minX, p.X)
		}
	}
	if !approx(minX, 50, 1e-6) {
		t.Errorf("group translate не применён: minX=%.2f want 50", minX)
	}
}

func TestParse_StyleAttributeOverride(t *testing.T) {
	const src = `<svg viewBox="0 0 10 10">
		<rect x="0" y="0" width="10" height="10" fill="#ff0000" style="fill:#0000ff"/>
	</svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 1 || doc.Shapes[0].Fill != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("style должен переопределять атрибут fill: %+v", doc.Shapes)
	}
}

func TestParse_FillRuleEvenOdd(t *testing.T) {
	const src = `<svg viewBox="0 0 10 10"><path d="M0 0 L10 0 L10 10 Z" fill-rule="evenodd" fill="#000"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 1 || !doc.Shapes[0].EvenOdd {
		t.Errorf("evenodd не распознан: %+v", doc.Shapes)
	}
}
