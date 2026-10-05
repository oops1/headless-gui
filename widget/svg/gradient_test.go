package svg

import (
	"image"
	"image/color"
	"testing"
)

// ── помощники тестов ─────────────────────────────────────────────────────────

// render разбирает src и растеризует w×h с документными цветами.
func render(t *testing.T, src string, w, h int) *image.RGBA {
	t.Helper()
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return doc.Rasterize(w, h, color.RGBA{A: 255}, false)
}

// near сравнивает цвет пикселя (premultiplied) с ожидаемым с допуском tol.
func near(got color.RGBA, r, g, b, a uint8, tol int) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(got.R, r) <= tol && d(got.G, g) <= tol && d(got.B, b) <= tol && d(got.A, a) <= tol
}

func expectPx(t *testing.T, img *image.RGBA, x, y int, r, g, b, a uint8, tol int) {
	t.Helper()
	got := img.RGBAAt(x, y)
	if !near(got, r, g, b, a, tol) {
		t.Errorf("пиксель (%d,%d) = %v, ожидался ~(%d,%d,%d,%d)±%d", x, y, got, r, g, b, a, tol)
	}
}

// ── linearGradient ───────────────────────────────────────────────────────────

// Фикстура WinLine: красный→синий сверху вниз, градиент-алиас через xlink:href.
// Раньше такая заливка становилась чёрной.
func TestGradient_LinearBBoxWithHrefAlias(t *testing.T) {
	const src = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 16 16">
  <defs>
    <linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#ff0000"/>
      <stop offset="1" style="stop-color:#0000ff;stop-opacity:1"/>
    </linearGradient>
    <linearGradient id="alias" xlink:href="#sky"/>
  </defs>
  <rect width="16" height="16" fill="url(#alias)"/>
</svg>`
	img := render(t, src, 16, 16)
	top := img.RGBAAt(8, 0)
	mid := img.RGBAAt(8, 8)
	bot := img.RGBAAt(8, 15)
	if top.R < 230 || top.B > 25 {
		t.Errorf("верх должен быть красным, got %v", top)
	}
	if bot.B < 230 || bot.R > 25 {
		t.Errorf("низ должен быть синим, got %v", bot)
	}
	if mid.R < 90 || mid.R > 160 || mid.B < 90 || mid.B > 160 || mid.G > 10 {
		t.Errorf("середина должна быть пурпурной, got %v", mid)
	}
	// по горизонтали цвет не меняется
	if l, r := img.RGBAAt(0, 8), img.RGBAAt(15, 8); l != r {
		t.Errorf("градиент вертикальный, а слева %v справа %v", l, r)
	}
}

func TestGradient_UserSpaceWithTransform(t *testing.T) {
	// userSpaceOnUse: от x=10 до x=30 в системе viewBox 0..40; gradientTransform
	// сдвигает вектор на +0 (проверим масштаб ×2 → 20..60 уйдёт за край).
	const src = `<svg viewBox="0 0 40 10"><defs>
	<linearGradient id="g" gradientUnits="userSpaceOnUse" x1="5" y1="0" x2="15" y2="0" gradientTransform="scale(2 1)">
	  <stop offset="0" stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient></defs>
	<rect width="40" height="10" fill="url(#g)"/></svg>`
	img := render(t, src, 40, 10)
	// после scale(2,1) вектор идёт от x=10 до x=30
	expectPx(t, img, 5, 5, 0, 0, 0, 255, 3)        // до начала — pad чёрный
	expectPx(t, img, 20, 5, 128, 128, 128, 255, 8) // середина вектора
	expectPx(t, img, 35, 5, 255, 255, 255, 255, 3) // за концом — pad белый
}

func TestGradient_SpreadMethods(t *testing.T) {
	mk := func(spread string) *image.RGBA {
		return render(t, `<svg viewBox="0 0 40 4"><defs>
		<linearGradient id="g" gradientUnits="userSpaceOnUse" x1="10" y1="0" x2="20" y2="0" spreadMethod="`+spread+`">
		  <stop offset="0" stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient></defs>
		<rect width="40" height="4" fill="url(#g)"/></svg>`, 40, 4)
	}
	// x=25 → t=1.5; x=5 → t=-0.5.
	pad, refl, rep := mk("pad"), mk("reflect"), mk("repeat")
	expectPx(t, pad, 25, 2, 255, 255, 255, 255, 3)
	expectPx(t, pad, 5, 2, 0, 0, 0, 255, 3)
	// reflect: t=1.5 → 0.5 (серый), t=-0.5 → 0.5
	expectPx(t, refl, 25, 2, 128, 128, 128, 255, 14)
	expectPx(t, refl, 5, 2, 128, 128, 128, 255, 14)
	// repeat: t=1.5 → 0.5, t=-0.5 → 0.5, но t=1.1 → 0.1 (тёмный), а не белый
	expectPx(t, rep, 25, 2, 128, 128, 128, 255, 14)
	if c := rep.RGBAAt(21, 2); c.R > 70 {
		t.Errorf("repeat: за концом вектора градиент должен начаться заново (тёмный), got %v", c)
	}
	if c := refl.RGBAAt(21, 2); c.R < 200 {
		t.Errorf("reflect: сразу за концом вектора ещё светло, got %v", c)
	}
}

func TestGradient_StopOpacityOffsetsAndStyle(t *testing.T) {
	// Стоп с процентным offset, stop-opacity через style и через атрибут;
	// offset, идущий назад, подтягивается к предыдущему.
	const src = `<svg viewBox="0 0 10 10"><defs>
	<linearGradient id="g" x1="0" y1="0" x2="1" y2="0">
	  <stop offset="0%" stop-color="#ff0000" stop-opacity="0"/>
	  <stop offset="50%" style="stop-color:#ff0000;stop-opacity:1"/>
	  <stop offset="20%" stop-color="#00ff00"/>
	</linearGradient></defs>
	<rect width="10" height="10" fill="url(#g)"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	g := doc.Shapes[0].FillGradient
	if g == nil || len(g.Stops) != 3 {
		t.Fatalf("ожидался градиент с 3 стопами, got %+v", g)
	}
	if g.Stops[0].Color.A != 0 || g.Stops[1].Color.A != 255 {
		t.Errorf("stop-opacity не разобран: %+v", g.Stops)
	}
	if g.Stops[2].Offset != 0.5 {
		t.Errorf("убывающий offset должен подтянуться к 0.5, got %v", g.Stops[2].Offset)
	}
	img := doc.Rasterize(10, 10, color.RGBA{A: 255}, false)
	if a := img.RGBAAt(0, 5).A; a > 70 {
		t.Errorf("слева стоп прозрачный, A=%d", a)
	}
	if a := img.RGBAAt(4, 5).A; a < 150 {
		t.Errorf("к середине непрозрачность растёт, A=%d", a)
	}
	// справа от 50% — зелёный
	if c := img.RGBAAt(8, 5); c.G < 200 || c.R > 30 {
		t.Errorf("справа должен быть зелёный, got %v", c)
	}
}

func TestGradient_HrefInheritsStopsAndAttributes(t *testing.T) {
	// Потомок задаёт геометрию, предок — стопы, а gradientUnits/transform
	// наследуются от предка.
	const src = `<svg xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 20 4"><defs>
	<linearGradient id="base" gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="20" y2="0">
	  <stop offset="0" stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient>
	<linearGradient id="child" xlink:href="#base" x2="10"/></defs>
	<rect width="20" height="4" fill="url(#child)"/></svg>`
	img := render(t, src, 20, 4)
	// вектор 0..10 в userSpace (унаследовано), x=10 уже белый
	expectPx(t, img, 10, 2, 255, 255, 255, 255, 14)
	expectPx(t, img, 5, 2, 128, 128, 128, 255, 14)
}

func TestGradient_HrefCycleDoesNotHang(t *testing.T) {
	const src = `<svg xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 4 4"><defs>
	<linearGradient id="a" xlink:href="#b"/><linearGradient id="b" xlink:href="#a"/></defs>
	<rect width="4" height="4" fill="url(#a)"/><rect width="4" height="4" x="0" fill="#00f"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	// градиент без стопов — «ничего», остаётся только синий квадрат
	if len(doc.Shapes) != 1 {
		t.Errorf("shapes=%d, want 1 (градиент без стопов не рисуется)", len(doc.Shapes))
	}
}

func TestGradient_Radial(t *testing.T) {
	const src = `<svg viewBox="0 0 20 20"><defs>
	<radialGradient id="g"><stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#000"/></radialGradient></defs>
	<rect width="20" height="20" fill="url(#g)"/></svg>`
	img := render(t, src, 20, 20)
	expectPx(t, img, 10, 10, 255, 255, 255, 255, 20) // центр белый
	if c := img.RGBAAt(0, 0); c.R > 20 {
		t.Errorf("угол (за радиусом) чёрный, got %v", c)
	}
	// монотонно темнеет от центра к краю
	prev := 256
	for x := 10; x < 20; x++ {
		r := int(img.RGBAAt(x, 10).R)
		if r > prev {
			t.Errorf("яркость растёт от центра: x=%d r=%d prev=%d", x, r, prev)
		}
		prev = r
	}
}

func TestGradient_RadialFocalPoint(t *testing.T) {
	// Фокус смещён влево: самая светлая точка — левее центра круга.
	const src = `<svg viewBox="0 0 40 20"><defs>
	<radialGradient id="g" gradientUnits="userSpaceOnUse" cx="20" cy="10" r="10" fx="14" fy="10">
	<stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#000"/></radialGradient></defs>
	<rect width="40" height="20" fill="url(#g)"/></svg>`
	img := render(t, src, 40, 20)
	best, bx := -1, 0
	for x := 0; x < 40; x++ {
		if r := int(img.RGBAAt(x, 10).R); r > best {
			best, bx = r, x
		}
	}
	if bx < 12 || bx > 16 {
		t.Errorf("самая светлая точка x=%d, ожидалась возле фокуса 14", bx)
	}
	// левее фокуса темнеет быстрее, чем правее
	if l, r := img.RGBAAt(11, 10).R, img.RGBAAt(17, 10).R; l >= r {
		t.Errorf("слева от фокуса (%d) должно быть темнее, чем справа (%d)", l, r)
	}
}

func TestGradient_Stroke(t *testing.T) {
	const src = `<svg viewBox="0 0 20 20"><defs>
	<linearGradient id="g" gradientUnits="userSpaceOnUse" x1="0" y1="0" x2="20" y2="0">
	<stop offset="0" stop-color="#f00"/><stop offset="1" stop-color="#00f"/></linearGradient></defs>
	<path d="M0 10 H20" fill="none" stroke="url(#g)" stroke-width="4"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Shapes[0].StrokeGradient == nil || doc.Shapes[0].FillGradient != nil {
		t.Fatalf("ожидался StrokeGradient без FillGradient")
	}
	img := doc.Rasterize(20, 20, color.RGBA{A: 255}, false)
	if l := img.RGBAAt(2, 10); l.R < 200 || l.B > 50 {
		t.Errorf("слева обводка красная, got %v", l)
	}
	if r := img.RGBAAt(17, 10); r.B < 200 || r.R > 50 {
		t.Errorf("справа обводка синяя, got %v", r)
	}
}

func TestGradient_SingleStopIsSolid(t *testing.T) {
	const src = `<svg viewBox="0 0 4 4"><defs><linearGradient id="g"><stop offset="0" stop-color="#0f0"/></linearGradient></defs>
	<rect width="4" height="4" fill="url(#g)"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if sh := doc.Shapes[0]; sh.FillGradient != nil || sh.Fill != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("один стоп должен свестись к сплошному цвету: %+v", sh)
	}
}

func TestGradient_CurrentColorStopAndTint(t *testing.T) {
	const src = `<svg viewBox="0 0 8 8"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0">
	<stop offset="0" stop-color="currentColor"/><stop offset="1" stop-color="#fff" stop-opacity="0"/></linearGradient></defs>
	<rect width="8" height="8" fill="url(#g)"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	cur := color.RGBA{200, 10, 20, 255}
	img := doc.Rasterize(8, 8, cur, false)
	if c := img.RGBAAt(0, 4); c.R < 150 || c.G > 40 {
		t.Errorf("стоп currentColor должен взять цвет виджета, got %v", c)
	}
	// tint: цвет везде current, форму задаёт альфа градиента (справа прозрачно)
	tinted := doc.Rasterize(8, 8, cur, true)
	if l, r := tinted.RGBAAt(0, 4), tinted.RGBAAt(7, 4); l.A <= r.A || r.A > 40 {
		t.Errorf("tint: альфа должна убывать слева направо: %v -> %v", l, r)
	}
}

func TestGradient_UnresolvedPaint(t *testing.T) {
	// Ссылка в пустоту: запасной цвет, а без него — не рисовать (не чёрный!).
	const src = `<svg viewBox="0 0 8 8">
	<rect width="4" height="8" fill="url(#nope)"/>
	<rect x="4" width="4" height="8" fill="url(#nope) #00ff00"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 1 || doc.Shapes[0].Fill != (color.RGBA{0, 255, 0, 255}) {
		t.Fatalf("ожидалась одна зелёная фигура (запасной цвет), got %+v", doc.Shapes)
	}
	img := doc.Rasterize(8, 8, color.RGBA{A: 255}, false)
	if c := img.RGBAAt(1, 4); c.A != 0 {
		t.Errorf("слева (ссылка в пустоту без запаса) должно быть пусто, got %v", c)
	}
}

// Ссылка на элемент, который не paint server (здесь — rect), даёт запасной
// цвет. Узоры pattern теперь поддержаны (pattern_test.go).
func TestGradient_NonServerRefFallsBack(t *testing.T) {
	const src = `<svg viewBox="0 0 8 8"><defs><rect id="p" width="2" height="2"/></defs>
	<rect width="8" height="8" fill="url(#p) #f00"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 1 || doc.Shapes[0].Fill != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("ссылка не на paint server: ожидался запасной красный, got %+v", doc.Shapes)
	}
}

func TestGradient_ZeroSizeBBoxIsSkipped(t *testing.T) {
	const src = `<svg viewBox="0 0 8 8"><defs><linearGradient id="g"><stop offset="0" stop-color="#f00"/><stop offset="1" stop-color="#00f"/></linearGradient></defs>
	<path d="M0 4 H8" fill="none" stroke="url(#g)" stroke-width="2"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 0 {
		t.Errorf("bbox нулевой высоты: градиент objectBoundingBox неприменим, got %d фигур", len(doc.Shapes))
	}
}

func TestParsePaint_URL(t *testing.T) {
	p := ParsePaint("url(#a)")
	if p.Kind != PaintURL || p.Ref != "a" || p.Fallback != nil {
		t.Errorf("url(#a) → %+v", p)
	}
	p = ParsePaint(` url( '#b' )  red `)
	if p.Kind != PaintURL || p.Ref != "b" || p.Fallback == nil || p.Fallback.Color != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("url('#b') red → %+v", p)
	}
	p = ParsePaint("url(icons.svg#a) none")
	if p.Kind != PaintURL || p.Ref != "" || p.Fallback == nil || p.Fallback.Kind != PaintNone {
		t.Errorf("внешняя ссылка → %+v", p)
	}
}

func TestGradient_MeanColor(t *testing.T) {
	g := &Gradient{Stops: []GradientStop{
		{Color: color.RGBA{255, 0, 0, 255}},
		{Color: color.RGBA{0, 0, 255, 255}},
		{Color: color.RGBA{0, 255, 0, 0}}, // невидимый стоп не должен тянуть цвет
	}}
	m := g.MeanColor()
	if m.R < 125 || m.R > 130 || m.B < 125 || m.B > 130 || m.G != 0 || m.A < 165 || m.A > 175 {
		t.Errorf("MeanColor = %v", m)
	}
	if (*Gradient)(nil).MeanColor() != (color.RGBA{}) {
		t.Error("MeanColor(nil) должен быть нулевым")
	}
}

func TestGradient_ShapeKeepsMeanFill(t *testing.T) {
	const src = `<svg viewBox="0 0 4 4"><defs><linearGradient id="g"><stop offset="0" stop-color="#f00"/><stop offset="1" stop-color="#00f"/></linearGradient></defs>
	<rect width="4" height="4" fill="url(#g)"/></svg>`
	doc, _ := Parse([]byte(src))
	sh := doc.Shapes[0]
	if !sh.HasFill || sh.FillGradient == nil || sh.Fill.A == 0 {
		t.Errorf("Shape.Fill для градиента должен хранить средний цвет: %+v", sh)
	}
}
