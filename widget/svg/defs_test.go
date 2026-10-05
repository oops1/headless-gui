package svg

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"
)

// ── defs / symbol / clipPath / mask не рисуются напрямую ────────────────────

// Фикстура WinLine «hidden»: объявленное в defs, clipPath и display:none не
// рисуется, остаётся только красный квадрат.
func TestDefs_NotDrawnDirectly(t *testing.T) {
	const src = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16">
  <defs>
    <path id="declared" d="M0 0h16v16H0z" fill="#00ff00"/>
    <clipPath id="cut"><rect width="8" height="8"/></clipPath>
    <mask id="m"><rect width="16" height="16" fill="#fff"/></mask>
    <symbol id="s"><rect width="16" height="16" fill="#0000ff"/></symbol>
  </defs>
  <rect width="16" height="16" fill="#ff0000"/>
  <g display="none"><rect width="16" height="16" fill="#00ff00"/></g>
</svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 1 {
		t.Fatalf("shapes=%d, want 1 (только красный квадрат)", len(doc.Shapes))
	}
	img := doc.Rasterize(16, 16, blackOpaque, false)
	expectPx(t, img, 12, 12, 255, 0, 0, 255, 0)
	expectPx(t, img, 2, 2, 255, 0, 0, 255, 0)
}

// ── use ──────────────────────────────────────────────────────────────────────

// Фикстура WinLine «reused»: правая половина — <use> с x=8.
func TestUse_ElementWithXY(t *testing.T) {
	const src = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 16 16">
  <defs><rect id="half" width="8" height="16" fill="#0000ff"/></defs>
  <rect width="8" height="16" fill="#ff0000"/>
  <use xlink:href="#half" x="8"/>
</svg>`
	img := render(t, src, 16, 16)
	expectPx(t, img, 3, 8, 255, 0, 0, 255, 0)
	expectPx(t, img, 12, 8, 0, 0, 255, 255, 0)
}

func TestUse_TransformAndInheritedFill(t *testing.T) {
	// fill наследуется от <use>, если у цели его нет; transform use применяется
	// до x/y.
	const src = `<svg viewBox="0 0 20 10"><defs><rect id="r" width="4" height="4"/></defs>
	<use href="#r" fill="#00ff00" transform="scale(2)" x="2"/></svg>`
	img := render(t, src, 20, 10)
	// после scale(2): квадрат 8×8 в позиции x=4..12, y=0..8
	expectPx(t, img, 8, 4, 0, 255, 0, 255, 0)
	if c := img.RGBAAt(2, 4); c.A != 0 {
		t.Errorf("левее квадрата пусто, got %v", c)
	}
	if c := img.RGBAAt(14, 4); c.A != 0 {
		t.Errorf("правее квадрата пусто, got %v", c)
	}
}

func TestUse_SymbolViewBox(t *testing.T) {
	// symbol с viewBox 0 0 10 10 вписывается в use width/height 20×20.
	const src = `<svg viewBox="0 0 40 20"><defs>
	<symbol id="s" viewBox="0 0 10 10"><rect width="10" height="10" fill="#f00"/><rect x="5" width="5" height="5" fill="#00f"/></symbol></defs>
	<use href="#s" x="10" width="20" height="20"/></svg>`
	img := render(t, src, 40, 20)
	expectPx(t, img, 12, 15, 255, 0, 0, 255, 0) // низ-лево символа
	expectPx(t, img, 27, 5, 0, 0, 255, 255, 0)  // верх-право (синий)
	if c := img.RGBAAt(5, 10); c.A != 0 {
		t.Errorf("левее символа пусто, got %v", c)
	}
}

func TestUse_GroupWithGradientUserSpace(t *testing.T) {
	// градиент userSpaceOnUse берёт систему координат ссылающегося use.
	const src = `<svg viewBox="0 0 20 10"><defs>
	<linearGradient id="g" gradientUnits="userSpaceOnUse" x1="0" x2="10"><stop offset="0" stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient>
	<rect id="r" width="10" height="10" fill="url(#g)"/></defs>
	<use href="#r"/><use href="#r" x="10"/></svg>`
	img := render(t, src, 20, 10)
	// вторая копия сдвинута на 10, а градиент в её системе тоже сдвинут:
	// слева у обеих чёрный, справа белый.
	if a, b := img.RGBAAt(1, 5).R, img.RGBAAt(11, 5).R; a > 40 || b > 40 {
		t.Errorf("левый край обеих копий тёмный: %d %d", a, b)
	}
	if a, b := img.RGBAAt(8, 5).R, img.RGBAAt(18, 5).R; a < 200 || b < 200 {
		t.Errorf("правый край обеих копий светлый: %d %d", a, b)
	}
}

func TestUse_Cycles(t *testing.T) {
	const src = `<svg xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 8 8">
	<g id="a"><use xlink:href="#b"/></g><g id="b"><use xlink:href="#a"/></g>
	<use xlink:href="#a"/><use xlink:href="#self" id="self"/><rect width="4" height="4" fill="#f00"/></svg>`
	done := make(chan struct{})
	go func() {
		defer close(done)
		doc, err := Parse([]byte(src))
		if err != nil || len(doc.Shapes) != 1 {
			t.Errorf("err=%v shapes=%v", err, len(doc.Shapes))
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Parse завис на циклических use")
	}
}

// «Взрыв» вложенных use: файл мал, а раскрывается экспоненциально — разбор
// должен быть ограничен.
func TestUse_ExplosionIsBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<svg xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 8 8"><defs><rect id="l0" width="1" height="1"/>`)
	const levels = 40
	for i := 1; i <= levels; i++ {
		prev := "l" + itoa(i-1)
		b.WriteString(`<g id="l` + itoa(i) + `"><use xlink:href="#` + prev + `"/><use xlink:href="#` + prev + `"/></g>`)
	}
	b.WriteString(`</defs><use xlink:href="#l` + itoa(levels) + `"/></svg>`)
	start := time.Now()
	doc, err := Parse([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("разбор use-бомбы занял %v", d)
	}
	if len(doc.Shapes) > maxUseVisits {
		t.Errorf("shapes=%d превышает предел", len(doc.Shapes))
	}
}

// ── clipPath ─────────────────────────────────────────────────────────────────

func TestClipPath_RectClipsGroup(t *testing.T) {
	const src = `<svg viewBox="0 0 20 10"><defs><clipPath id="c"><rect x="0" y="0" width="10" height="10"/></clipPath></defs>
	<g clip-path="url(#c)"><rect width="20" height="10" fill="#f00"/></g></svg>`
	img := render(t, src, 20, 10)
	expectPx(t, img, 5, 5, 255, 0, 0, 255, 0)
	if c := img.RGBAAt(15, 5); c.A != 0 {
		t.Errorf("за пределами clip пусто, got %v", c)
	}
}

func TestClipPath_ElementTransformAndAttrStyle(t *testing.T) {
	// clip-path в style; clipPath в системе координат элемента (его transform).
	const src = `<svg viewBox="0 0 20 10"><defs><clipPath id="c"><circle cx="5" cy="5" r="5"/></clipPath></defs>
	<rect width="10" height="10" fill="#00f" transform="translate(10 0)" style="clip-path:url(#c)"/></svg>`
	img := render(t, src, 20, 10)
	expectPx(t, img, 15, 5, 0, 0, 255, 255, 0)
	if c := img.RGBAAt(10, 0); c.A > 20 {
		t.Errorf("угол квадрата вырезан кругом, got %v", c)
	}
	if c := img.RGBAAt(5, 5); c.A != 0 {
		t.Errorf("левая половина пуста, got %v", c)
	}
}

func TestClipPath_ObjectBoundingBox(t *testing.T) {
	// 0..0.5 по bbox квадрата 10..20.
	const src = `<svg viewBox="0 0 20 10"><defs><clipPath id="c" clipPathUnits="objectBoundingBox"><rect width="0.5" height="1"/></clipPath></defs>
	<rect x="10" width="10" height="10" fill="#0f0" clip-path="url(#c)"/></svg>`
	img := render(t, src, 20, 10)
	expectPx(t, img, 12, 5, 0, 255, 0, 255, 0)
	if c := img.RGBAAt(17, 5); c.A != 0 {
		t.Errorf("правая половина вырезана, got %v", c)
	}
}

func TestClipPath_EvenOddRule(t *testing.T) {
	const src = `<svg viewBox="0 0 20 20"><defs><clipPath id="c"><path clip-rule="evenodd" d="M0 0h20v20h-20z M5 5h10v10h-10z"/></clipPath></defs>
	<rect width="20" height="20" fill="#f00" clip-path="url(#c)"/></svg>`
	img := render(t, src, 20, 20)
	expectPx(t, img, 2, 2, 255, 0, 0, 255, 0)
	if c := img.RGBAAt(10, 10); c.A != 0 {
		t.Errorf("дыра even-odd, got %v", c)
	}
}

func TestClipPath_NestedIntersect(t *testing.T) {
	// родитель вырезает левую половину, потомок — верхнюю: остаётся угол.
	const src = `<svg viewBox="0 0 20 20"><defs>
	<clipPath id="l"><rect width="10" height="20"/></clipPath>
	<clipPath id="t"><rect width="20" height="10"/></clipPath></defs>
	<g clip-path="url(#l)"><rect width="20" height="20" fill="#f00" clip-path="url(#t)"/></g></svg>`
	img := render(t, src, 20, 20)
	expectPx(t, img, 5, 5, 255, 0, 0, 255, 0)
	for _, p := range [][2]int{{15, 5}, {5, 15}, {15, 15}} {
		if c := img.RGBAAt(p[0], p[1]); c.A != 0 {
			t.Errorf("(%d,%d) должно быть пусто, got %v", p[0], p[1], c)
		}
	}
}

func TestClipPath_ChainOnClipPathItself(t *testing.T) {
	const src = `<svg viewBox="0 0 20 10"><defs>
	<clipPath id="inner"><rect width="10" height="10"/></clipPath>
	<clipPath id="outer" clip-path="url(#inner)"><rect width="20" height="5"/></clipPath></defs>
	<rect width="20" height="10" fill="#f00" clip-path="url(#outer)"/></svg>`
	img := render(t, src, 20, 10)
	expectPx(t, img, 5, 2, 255, 0, 0, 255, 0)
	if c := img.RGBAAt(15, 2); c.A != 0 {
		t.Errorf("вне inner пусто, got %v", c)
	}
	if c := img.RGBAAt(5, 8); c.A != 0 {
		t.Errorf("вне outer пусто, got %v", c)
	}
}

func TestClipPath_MissingReferenceDrawsUnclipped(t *testing.T) {
	img := render(t, `<svg viewBox="0 0 4 4"><rect width="4" height="4" fill="#f00" clip-path="url(#nope)"/></svg>`, 4, 4)
	expectPx(t, img, 2, 2, 255, 0, 0, 255, 0)
}

func TestClipPath_EmptyClipHidesAll(t *testing.T) {
	img := render(t, `<svg viewBox="0 0 4 4"><defs><clipPath id="c"/></defs><rect width="4" height="4" fill="#f00" clip-path="url(#c)"/></svg>`, 4, 4)
	if c := img.RGBAAt(2, 2); c.A != 0 {
		t.Errorf("пустой clipPath скрывает всё, got %v", c)
	}
}

func TestClipPath_SelfReferenceDoesNotHang(t *testing.T) {
	const src = `<svg viewBox="0 0 4 4"><defs><clipPath id="c" clip-path="url(#c)"><rect width="2" height="4"/></clipPath></defs>
	<rect width="4" height="4" fill="#f00" clip-path="url(#c)"/></svg>`
	img := render(t, src, 4, 4)
	expectPx(t, img, 0, 2, 255, 0, 0, 255, 0)
}

// ── mask ─────────────────────────────────────────────────────────────────────

func TestMask_Luminance(t *testing.T) {
	// белая левая треть, серая 50% средняя, чёрная правая — на квадрате 30 px.
	const src = `<svg viewBox="0 0 30 10"><defs><mask id="m" maskUnits="userSpaceOnUse" x="0" y="0" width="30" height="10">
	<rect width="10" height="10" fill="#fff"/><rect x="10" width="10" height="10" fill="#808080"/><rect x="20" width="10" height="10" fill="#000"/></mask></defs>
	<rect width="30" height="10" fill="#f00" mask="url(#m)"/></svg>`
	img := render(t, src, 30, 10)
	expectPx(t, img, 5, 5, 255, 0, 0, 255, 0)
	if c := img.RGBAAt(15, 5); c.A < 120 || c.A > 136 {
		t.Errorf("серая маска → ~50%% непрозрачности, got %v", c)
	}
	if c := img.RGBAAt(25, 5); c.A != 0 {
		t.Errorf("чёрная маска скрывает, got %v", c)
	}
}

func TestMask_GradientFadeOut(t *testing.T) {
	const src = `<svg viewBox="0 0 20 4"><defs>
	<linearGradient id="g" gradientUnits="userSpaceOnUse" x1="0" x2="20"><stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#000"/></linearGradient>
	<mask id="m"><rect width="20" height="4" fill="url(#g)"/></mask></defs>
	<rect width="20" height="4" fill="#00f" mask="url(#m)"/></svg>`
	img := render(t, src, 20, 4)
	l, r := img.RGBAAt(1, 2).A, img.RGBAAt(18, 2).A
	if l < 220 || r > 40 {
		t.Errorf("затухание слева направо: left A=%d right A=%d", l, r)
	}
}

func TestMask_AlphaType(t *testing.T) {
	const src = `<svg viewBox="0 0 10 10"><defs><mask id="m" mask-type="alpha" maskUnits="userSpaceOnUse" x="0" y="0" width="10" height="10">
	<rect width="5" height="10" fill="#000"/></mask></defs>
	<rect width="10" height="10" fill="#f00" mask="url(#m)"/></svg>`
	img := render(t, src, 10, 10)
	expectPx(t, img, 2, 5, 255, 0, 0, 255, 0) // чёрное, но непрозрачное — виден
	if c := img.RGBAAt(7, 5); c.A != 0 {
		t.Errorf("вне содержимого маски пусто, got %v", c)
	}
}

func TestMask_OnGroupAndRegion(t *testing.T) {
	// maskUnits по умолчанию objectBoundingBox с областью -10%..120% — содержимое
	// маски (userSpaceOnUse) вне области не действует.
	const src = `<svg viewBox="0 0 20 10"><defs><mask id="m" x="0" y="0" width="0.5" height="1">
	<rect width="20" height="10" fill="#fff"/></mask></defs>
	<g mask="url(#m)"><rect width="20" height="10" fill="#f00"/></g></svg>`
	img := render(t, src, 20, 10)
	expectPx(t, img, 5, 5, 255, 0, 0, 255, 0)
	if c := img.RGBAAt(15, 5); c.A != 0 {
		t.Errorf("вне области маски пусто, got %v", c)
	}
}

// Адвайта-идиом: <g filter> с feColorMatrix превращает любой цвет в белый с
// сохранением альфы — полупрозрачная чёрная заливка маски даёт частичную
// видимость, а не нулевую.
func TestMask_ColorMatrixWhiteIdiom(t *testing.T) {
	const src = `<svg viewBox="0 0 8 8"><defs>
	<filter id="h"><feColorMatrix in="SourceGraphic" type="matrix" values="0 0 0 0 1 0 0 0 0 1 0 0 0 0 1 0 0 0 1 0"/></filter>
	<mask id="m"><g filter="url(#h)"><rect fill-opacity="0.4" width="8" height="8"/></g></mask></defs>
	<rect width="8" height="8" fill="#f00" mask="url(#m)"/></svg>`
	img := render(t, src, 8, 8)
	if c := img.RGBAAt(4, 4); c.A < 90 || c.A > 115 {
		t.Errorf("ожидалась ~40%% (102) непрозрачности, got %v", c)
	}
}

func TestMask_SelfReferenceDoesNotHang(t *testing.T) {
	const src = `<svg viewBox="0 0 4 4"><defs><mask id="m"><rect width="4" height="4" fill="#fff" mask="url(#m)"/></mask></defs>
	<rect width="4" height="4" fill="#f00" mask="url(#m)"/></svg>`
	if _, err := Parse([]byte(src)); err != nil {
		t.Fatal(err)
	}
	render(t, src, 4, 4)
}

// ── display / visibility ─────────────────────────────────────────────────────

func TestDisplayNone(t *testing.T) {
	cases := map[string]string{
		"атрибут":  `<rect width="4" height="4" display="none"/>`,
		"style":    `<rect width="4" height="4" style="display:none"/>`,
		"группа":   `<g style="display: none"><rect width="4" height="4"/></g>`,
		"класс":    `<style>.h{display:none}</style><rect class="h" width="4" height="4"/>`,
		"симв-use": `<defs><rect id="r" width="4" height="4" display="none"/></defs><use href="#r"/>`,
	}
	for name, body := range cases {
		doc, err := Parse([]byte(`<svg viewBox="0 0 4 4">` + body + `</svg>`))
		if err != nil {
			t.Fatal(err)
		}
		if len(doc.Shapes) != 0 {
			t.Errorf("%s: display:none нарисован (%d фигур)", name, len(doc.Shapes))
		}
	}
}

func TestVisibilityHidden(t *testing.T) {
	// hidden наследуется, но потомок может вернуть visible.
	const src = `<svg viewBox="0 0 8 4"><g visibility="hidden"><rect width="4" height="4"/><rect x="4" width="4" height="4" visibility="visible" fill="#f00"/></g></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 1 || doc.Shapes[0].Fill.R != 255 {
		t.Errorf("ожидалась одна видимая фигура, got %+v", doc.Shapes)
	}
}

// ── вложенный svg, чужие пространства имён, text ─────────────────────────────

func TestNestedSVGViewport(t *testing.T) {
	const src = `<svg viewBox="0 0 20 10"><svg x="10" y="0" width="10" height="10" viewBox="0 0 5 5"><rect width="5" height="5" fill="#f00"/></svg></svg>`
	img := render(t, src, 20, 10)
	expectPx(t, img, 15, 5, 255, 0, 0, 255, 0)
	if c := img.RGBAAt(5, 5); c.A != 0 {
		t.Errorf("слева пусто, got %v", c)
	}
}

func TestForeignNamespaceAndTextSkipped(t *testing.T) {
	const src = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:x="urn:x" viewBox="0 0 4 4">
	<x:rect width="4" height="4"/><text x="0" y="3"><tspan>A</tspan></text><rect width="2" height="2" fill="#f00"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 1 {
		t.Errorf("shapes=%d, want 1", len(doc.Shapes))
	}
}

// ── filter ───────────────────────────────────────────────────────────────────

func TestFilter_GaussianBlurSpreadsCoverage(t *testing.T) {
	const src = `<svg viewBox="0 0 40 40"><defs><filter id="b"><feGaussianBlur stdDeviation="3"/></filter></defs>
	<rect x="15" y="15" width="10" height="10" fill="#000" filter="url(#b)"/></svg>`
	img := render(t, src, 40, 40)
	// за пределами квадрата появилась полупрозрачная кайма, центр чуть слабее 100%
	if c := img.RGBAAt(12, 20); c.A < 15 || c.A > 140 {
		t.Errorf("размытие должно дать кайму вокруг фигуры, got %v", c)
	}
	if c := img.RGBAAt(20, 20); c.A < 200 {
		t.Errorf("центр остаётся плотным, got %v", c)
	}
	// без фильтра каймы нет
	plain := render(t, `<svg viewBox="0 0 40 40"><rect x="15" y="15" width="10" height="10"/></svg>`, 40, 40)
	if c := plain.RGBAAt(12, 20); c.A != 0 {
		t.Errorf("контроль: без фильтра пусто, got %v", c)
	}
}

// ── стили ────────────────────────────────────────────────────────────────────

func TestStyleSheet_ClassTagIdAndPriority(t *testing.T) {
	const src = `<svg viewBox="0 0 40 4"><style type="text/css"><![CDATA[
	/* комментарий */
	rect { fill: #111111 }
	.st0 { fill:#ff0000; }
	.st1, .st2 { fill: #00ff00 !important }
	#only { fill: #0000ff }
	@media print { .st0 { fill: #ffffff } }
	g rect { fill: #abcdef }
	]]></style>
	<rect width="4" height="4"/>
	<rect x="4" width="4" height="4" class="st0"/>
	<rect x="8" width="4" height="4" class="st2"/>
	<rect x="12" width="4" height="4" class="st0" fill="#123456"/>
	<rect x="16" width="4" height="4" class="st0" style="fill:#fedcba"/>
	<rect id="only" x="20" width="4" height="4" class="st0"/>
	</svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"#111111", "#ff0000", "#00ff00", "#ff0000", "#fedcba", "#0000ff"}
	if len(doc.Shapes) != len(want) {
		t.Fatalf("shapes=%d, want %d", len(doc.Shapes), len(want))
	}
	for i, w := range want {
		c, _ := ParseColor(w)
		if doc.Shapes[i].Fill != c {
			t.Errorf("фигура %d: fill=%v, want %s", i, doc.Shapes[i].Fill, w)
		}
	}
}

func TestStyleSheet_GradientViaClass(t *testing.T) {
	const src = `<svg viewBox="0 0 4 4"><style>.g{fill:url(#a)}</style><defs><linearGradient id="a"><stop offset="0" stop-color="#f00"/><stop offset="1" stop-color="#00f"/></linearGradient></defs><rect class="g" width="4" height="4"/></svg>`
	doc, _ := Parse([]byte(src))
	if len(doc.Shapes) != 1 || doc.Shapes[0].FillGradient == nil {
		t.Errorf("fill:url() из класса: %+v", doc.Shapes)
	}
}

func TestStyleSheet_StopColorViaClass(t *testing.T) {
	const src = `<svg viewBox="0 0 4 4"><style>.s1{stop-color:#ff0000}.s2{stop-color:#0000ff}</style><defs><linearGradient id="a" x1="0" x2="1" y1="0" y2="0"><stop offset="0" class="s1"/><stop offset="1" class="s2"/></linearGradient></defs><rect width="4" height="4" fill="url(#a)"/></svg>`
	img := render(t, src, 4, 4)
	if l, r := img.RGBAAt(0, 2), img.RGBAAt(3, 2); l.R < 150 || r.B < 150 {
		t.Errorf("stop-color из классов: left=%v right=%v", l, r)
	}
}

func TestStyleSheet_Parser(t *testing.T) {
	ss := parseStyleSheet(`a.b#c, .x{fill:red;stroke:blue} svg|rect{fill:green} p > q{fill:aqua} .y:hover{fill:lime}`)
	if len(ss.rules) != 2 {
		t.Fatalf("правил %d, want 2 (комбинаторы и псевдоклассы пропускаются)", len(ss.rules))
	}
}

// ── image ────────────────────────────────────────────────────────────────────

func TestImage_DataURI(t *testing.T) {
	href := testPNGDataURI(t)
	src := `<svg xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 20 20"><image x="0" y="0" width="20" height="20" xlink:href="` + href + `"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 1 || doc.Shapes[0].Image == nil {
		t.Fatalf("ожидалась одна фигура-картинка: %+v", doc.Shapes)
	}
	img := doc.Rasterize(20, 20, blackOpaque, false)
	// PNG 2×2: красный, зелёный / синий, белый
	expectPx(t, img, 3, 3, 255, 0, 0, 255, 40)
	expectPx(t, img, 16, 3, 0, 255, 0, 255, 40)
	expectPx(t, img, 3, 16, 0, 0, 255, 255, 40)
	expectPx(t, img, 16, 16, 255, 255, 255, 255, 40)
}

func TestImage_AspectAndTransformAndOpacity(t *testing.T) {
	href := testPNGDataURI(t)
	// картинка 2×2 в прямоугольник 20×10 (meet, по центру → 10×10 в x=5..15)
	src := `<svg viewBox="0 0 20 10"><image width="20" height="10" opacity="0.5" href="` + href + `"/></svg>`
	img := render(t, src, 20, 10)
	if c := img.RGBAAt(2, 5); c.A != 0 {
		t.Errorf("поля слева пусты (meet), got %v", c)
	}
	if c := img.RGBAAt(7, 2); c.A < 120 || c.A > 136 || c.R < 100 {
		t.Errorf("красная четверть при opacity .5, got %v", c)
	}
}

func TestImage_BadDataIsIgnored(t *testing.T) {
	for _, h := range []string{"data:image/png;base64,AAAA", "file:///etc/passwd", "http://example.com/a.png", "data:image/png;base64,###"} {
		doc, err := Parse([]byte(`<svg viewBox="0 0 4 4"><image width="4" height="4" href="` + h + `"/></svg>`))
		if err != nil || len(doc.Shapes) != 0 {
			t.Errorf("%q: err=%v shapes=%d", h, err, len(doc.Shapes))
		}
	}
}

func TestImage_TintUsesAlphaOnly(t *testing.T) {
	href := testPNGDataURI(t)
	doc, _ := Parse([]byte(`<svg viewBox="0 0 4 4"><image width="4" height="4" href="` + href + `"/></svg>`))
	img := doc.Rasterize(4, 4, color4(10, 200, 30), true)
	expectPx(t, img, 1, 1, 10, 200, 30, 255, 6)
	expectPx(t, img, 2, 2, 10, 200, 30, 255, 6)
}

// Фикстуры WinLine: «только текст» не даёт фигур, а плоский значок из
// Inkscape с метаданными рисуется как раньше.
func TestWinLineFixtures(t *testing.T) {
	text, err := Parse([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><text x="2" y="12">A</text></svg>`))
	if err != nil || len(text.Shapes) != 0 {
		t.Errorf("textonly: err=%v shapes=%d", err, len(text.Shapes))
	}
	const flat = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd" viewBox="0 0 16 16" width="16" height="16">
  <sodipodi:namedview id="namedview1" inkscape:zoom="8"/>
  <metadata><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"/></metadata>
  <defs id="defs1"/>
  <rect x="0" y="0" width="16" height="16" fill="#ff0000"/>
  <circle cx="8" cy="8" r="4" style="fill:#0000ff;stroke:none"/>
</svg>`
	img := render(t, flat, 16, 16)
	expectPx(t, img, 1, 1, 255, 0, 0, 255, 0)
	expectPx(t, img, 8, 8, 0, 0, 255, 255, 0)
}

// ── помощники ────────────────────────────────────────────────────────────────

var blackOpaque = color.RGBA{A: 255}

func color4(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }

// testPNGDataURI — PNG 2×2 (красный, зелёный / синий, белый) как data:-ссылка.
func testPNGDataURI(t *testing.T) string {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	im.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	im.SetNRGBA(1, 0, color.NRGBA{0, 255, 0, 255})
	im.SetNRGBA(0, 1, color.NRGBA{0, 0, 255, 255})
	im.SetNRGBA(1, 1, color.NRGBA{255, 255, 255, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}
