package svg

import (
	"image/color"
	"testing"
)

const overlapGroup = `<svg viewBox="0 0 32 32"><g opacity="0.5">
<rect x="2" y="2" width="20" height="20" fill="#ff0000"/>
<rect x="10" y="10" width="20" height="20" fill="#0000ff"/></g></svg>`

func TestGroupOpacity_LegacyShinesThrough(t *testing.T) {
	// По умолчанию — прежнее поведение: opacity на каждую фигуру, в
	// перекрытии покрытие 1-(1-.5)² = .75.
	img := strokeImg(t, overlapGroup, 32, 32, Options{})
	if v := a(img, 16, 16); v < 188 || v > 195 {
		t.Errorf("перекрытие без слоя: а=%d, ожидалось ~191", v)
	}
}

func TestGroupOpacity_LayerIsIsolated(t *testing.T) {
	img := strokeImg(t, overlapGroup, 32, 32, Options{GroupLayers: true})
	// В слое синий закрывает красный целиком, а группа в целом полупрозрачна.
	if c := img.RGBAAt(16, 16); !near(c, 0, 0, 128, 128, 2) {
		t.Errorf("перекрытие в слое: %v, ожидалось синий с а=128", c)
	}
	if c := img.RGBAAt(5, 5); !near(c, 128, 0, 0, 128, 2) {
		t.Errorf("красный вне перекрытия: %v", c)
	}
	if a(img, 28, 3) != 0 {
		t.Error("вне группы должно быть пусто")
	}
}

func TestGroupOpacity_NestedMultiplies(t *testing.T) {
	const src = `<svg viewBox="0 0 8 8"><g opacity="0.5"><g opacity="0.5"><rect width="8" height="8" fill="#000"/></g></g></svg>`
	for _, o := range []Options{{}, {GroupLayers: true}} {
		img := strokeImg(t, src, 8, 8, o)
		if v := a(img, 4, 4); v < 62 || v > 66 {
			t.Errorf("опции %+v: вложенные opacity .5×.5 дали а=%d, ожидалось 64", o, v)
		}
	}
}

func TestGroupOpacity_FillAndStrokeOfOneShape(t *testing.T) {
	// У фигуры opacity относится к склейке заливки и обводки: внутри обводки,
	// поверх заливки, покрытие остаётся .5 (а не .75).
	const src = `<svg viewBox="0 0 32 32"><rect x="6" y="6" width="20" height="20" fill="#f00" stroke="#00f" stroke-width="8" opacity="0.5"/></svg>`
	layered := strokeImg(t, src, 32, 32, Options{GroupLayers: true})
	legacy := strokeImg(t, src, 32, 32, Options{})
	// (8,16): внутри обводки (она шириной 8 вокруг границы x=6), поверх заливки.
	if v := a(layered, 8, 16); v < 126 || v > 130 {
		t.Errorf("слой: а=%d, ожидалось 128", v)
	}
	if v := a(legacy, 8, 16); v < 180 {
		t.Errorf("без слоя заливка просвечивает под обводкой: а=%d, ожидалось ~191", v)
	}
}

func TestGroupOpacity_OneAndZeroShapesUntouched(t *testing.T) {
	const src = `<svg viewBox="0 0 8 8"><g opacity="0"><rect width="8" height="8"/></g><g opacity="1"><rect x="0" width="4" height="8" fill="#0f0"/></g></svg>`
	for _, o := range []Options{{}, {GroupLayers: true}} {
		img := strokeImg(t, src, 8, 8, o)
		if a(img, 6, 4) != 0 || a(img, 2, 4) != 255 {
			t.Errorf("опции %+v: opacity 0/1: %d, %d", o, a(img, 6, 4), a(img, 2, 4))
		}
	}
}

// Групповая mask — слоем всегда: два перекрывающихся потомка маскируются как
// одна картинка, а не по отдельности.
func TestGroupMask_IsLayerAlways(t *testing.T) {
	const src = `<svg viewBox="0 0 32 32"><defs><mask id="m"><rect width="32" height="32" fill="#808080"/></mask></defs>
<g mask="url(#m)"><rect x="2" y="2" width="20" height="20" fill="#f00"/><rect x="10" y="10" width="20" height="20" fill="#00f"/></g></svg>`
	img := strokeImg(t, src, 32, 32, Options{})
	// Яркость 0,5 (~128): в перекрытии синий с а≈128, а не 191 от двух слоёв.
	if c := img.RGBAAt(16, 16); c.A < 124 || c.A > 132 || c.B < 120 || c.R > 4 {
		t.Errorf("перекрытие под маской группы: %v", c)
	}
	doc, _ := Parse([]byte(src))
	for _, sh := range doc.Shapes {
		if len(sh.Masks) != 0 || len(sh.Groups) != 1 || len(sh.Groups[0].Masks) != 1 {
			t.Fatalf("групповая маска должна жить в Group, а не в Shape.Masks: %+v", sh.Groups)
		}
	}
}

func TestGroupFilter_BlurIsOverUnion(t *testing.T) {
	// Две фигуры встык (шов в x=16), размытие на группе. Если размывать
	// фигуры по отдельности, на шве покрытие 1-(.5)² = .75; склеенная группа
	// размывается в сплошное.
	const src = `<svg viewBox="0 0 32 32"><defs><filter id="f"><feGaussianBlur stdDeviation="2"/></filter></defs>
<g filter="url(#f)"><rect x="4" y="4" width="12" height="24" fill="#000"/><rect x="16" y="4" width="12" height="24" fill="#000"/></g></svg>`
	img := strokeImg(t, src, 32, 32, Options{})
	if v := a(img, 15, 16); v < 245 {
		t.Errorf("шов размытой группы: а=%d, ожидалось ~255", v)
	}
	if v := a(img, 2, 16); v == 0 || v > 120 {
		t.Errorf("размытие должно выйти за край группы: а=%d", v)
	}
}

func TestGroupFilter_ColorMatrixOnLayer(t *testing.T) {
	const src = `<svg viewBox="0 0 8 8"><defs><filter id="f"><feColorMatrix type="saturate" values="0"/></filter></defs>
<g filter="url(#f)"><rect width="8" height="8" fill="#ff0000"/></g></svg>`
	img := strokeImg(t, src, 8, 8, Options{})
	c := img.RGBAAt(4, 4)
	if c.R != c.G || c.G != c.B || c.A != 255 || c.R == 0 {
		t.Errorf("saturate=0 на группе: %v, ожидался серый", c)
	}
}

func TestGroupLayers_WithClipAndNested(t *testing.T) {
	const src = `<svg viewBox="0 0 32 32"><defs><clipPath id="c"><rect width="16" height="32"/></clipPath></defs>
<g opacity="0.5" clip-path="url(#c)"><g opacity="0.5"><rect width="32" height="32" fill="#000"/><rect width="32" height="32" fill="#000"/></g></g></svg>`
	img := strokeImg(t, src, 32, 32, Options{GroupLayers: true})
	if v := a(img, 8, 16); v < 62 || v > 66 {
		t.Errorf("clip+вложенные слои: а=%d, ожидалось 64", v)
	}
	if a(img, 24, 16) != 0 {
		t.Error("вне clip должно быть пусто")
	}
}

func TestGroupLayers_UseAndDeepNestingDoNotBreak(t *testing.T) {
	src := `<svg viewBox="0 0 8 8" xmlns:xlink="http://www.w3.org/1999/xlink"><defs><g id="u" opacity="0.9"><rect width="8" height="8"/></g></defs>`
	for i := 0; i < 40; i++ {
		src += `<g opacity="0.99">`
	}
	src += `<use xlink:href="#u"/><use xlink:href="#u"/>`
	for i := 0; i < 40; i++ {
		src += `</g>`
	}
	src += `</svg>`
	img := strokeImg(t, src, 8, 8, PreciseOptions)
	if a(img, 4, 4) == 0 {
		t.Error("глубокая вложенность слоёв потеряла содержимое")
	}
}

func TestGroupLayers_TintAndCurrentColor(t *testing.T) {
	const src = `<svg viewBox="0 0 8 8"><g opacity="0.5"><rect width="8" height="8" fill="currentColor"/><rect width="8" height="8" fill="#f00"/></g></svg>`
	doc, _ := Parse([]byte(src))
	img := doc.RasterizeWith(8, 8, color.RGBA{0, 255, 0, 255}, true, Options{GroupLayers: true})
	if c := img.RGBAAt(4, 4); !near(c, 0, 128, 0, 128, 2) {
		t.Errorf("tint в слое: %v", c)
	}
}
