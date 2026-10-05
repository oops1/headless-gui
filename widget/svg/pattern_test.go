package svg

import (
	"image/color"
	"testing"
)

// Шахматка 2×2 клетки по 4 px: красная и синяя по диагонали.
const checkerDefs = `<defs><pattern id="p" width="8" height="8" patternUnits="userSpaceOnUse">
<rect width="4" height="4" fill="#ff0000"/><rect x="4" y="4" width="4" height="4" fill="#0000ff"/></pattern></defs>`

func TestPattern_UserSpaceTiles(t *testing.T) {
	img := render(t, `<svg viewBox="0 0 24 24">`+checkerDefs+`<rect width="24" height="24" fill="url(#p)"/></svg>`, 24, 24)
	expectPx(t, img, 2, 2, 255, 0, 0, 255, 3)   // красная клетка
	expectPx(t, img, 6, 2, 0, 0, 0, 0, 3)       // пустая
	expectPx(t, img, 6, 6, 0, 0, 255, 255, 3)   // синяя
	expectPx(t, img, 10, 2, 255, 0, 0, 255, 3)  // повтор вправо
	expectPx(t, img, 18, 18, 255, 0, 0, 255, 3) // повтор по диагонали
	expectPx(t, img, 22, 22, 0, 0, 255, 255, 3)
}

func TestPattern_ObjectBoundingBoxUnits(t *testing.T) {
	// patternUnits по умолчанию — доли bbox: плитка 0,5×0,5 от 16×16 даёт 2×2.
	const src = `<svg viewBox="0 0 16 16"><defs><pattern id="p" width="0.5" height="0.5">
<rect width="4" height="4" fill="#ff0000"/></pattern></defs><rect width="16" height="16" fill="url(#p)"/></svg>`
	img := render(t, src, 16, 16)
	// Содержимое в единицах пользователя: квадрат 4×4 в начале каждой плитки 8×8.
	expectPx(t, img, 2, 2, 255, 0, 0, 255, 3)
	expectPx(t, img, 6, 6, 0, 0, 0, 0, 3)
	expectPx(t, img, 10, 2, 255, 0, 0, 255, 3)
	expectPx(t, img, 10, 10, 255, 0, 0, 255, 3)
	expectPx(t, img, 14, 10, 0, 0, 0, 0, 3)
}

func TestPattern_ContentUnitsObjectBoundingBox(t *testing.T) {
	// Содержимое в долях bbox: квадрат 0,25×0,25 в плитке 0,5 → 4 px в плитке 8 px.
	const src = `<svg viewBox="0 0 16 16"><defs><pattern id="p" width="0.5" height="0.5" patternContentUnits="objectBoundingBox">
<rect width="0.25" height="0.25" fill="#ff0000"/></pattern></defs><rect width="16" height="16" fill="url(#p)"/></svg>`
	img := render(t, src, 16, 16)
	expectPx(t, img, 2, 2, 255, 0, 0, 255, 3)
	expectPx(t, img, 6, 2, 0, 0, 0, 0, 3)
	expectPx(t, img, 10, 10, 255, 0, 0, 255, 3)
}

func TestPattern_ViewBoxScalesContent(t *testing.T) {
	// Плитка 8×8 с viewBox 0 0 2 2: клетка 1×1 растягивается до 4×4.
	const src = `<svg viewBox="0 0 16 16"><defs><pattern id="p" width="8" height="8" viewBox="0 0 2 2" patternUnits="userSpaceOnUse">
<rect width="1" height="1" fill="#ff0000"/></pattern></defs><rect width="16" height="16" fill="url(#p)"/></svg>`
	img := render(t, src, 16, 16)
	expectPx(t, img, 3, 3, 255, 0, 0, 255, 3)
	expectPx(t, img, 5, 5, 0, 0, 0, 0, 3)
	expectPx(t, img, 11, 11, 255, 0, 0, 255, 3)
}

func TestPattern_XYOffsetsTheGrid(t *testing.T) {
	const src = `<svg viewBox="0 0 16 16"><defs><pattern id="p" x="2" y="0" width="8" height="8" patternUnits="userSpaceOnUse">
<rect width="4" height="8" fill="#ff0000"/></pattern></defs><rect width="16" height="16" fill="url(#p)"/></svg>`
	img := render(t, src, 16, 16)
	expectPx(t, img, 1, 4, 0, 0, 0, 0, 3) // красная полоса идёт с x=2
	expectPx(t, img, 3, 4, 255, 0, 0, 255, 3)
	expectPx(t, img, 7, 4, 0, 0, 0, 0, 3)
	expectPx(t, img, 11, 4, 255, 0, 0, 255, 3)
}

func TestPattern_TransformRotateAndScale(t *testing.T) {
	// patternTransform масштабирует плитку вдвое: красный квадрат 4 → 8.
	const scaled = `<svg viewBox="0 0 32 32"><defs><pattern id="p" width="16" height="16" patternUnits="userSpaceOnUse" patternTransform="scale(2)">
<rect width="4" height="4" fill="#ff0000"/></pattern></defs><rect width="32" height="32" fill="url(#p)"/></svg>`
	img := render(t, scaled, 32, 32)
	expectPx(t, img, 6, 6, 255, 0, 0, 255, 3)
	expectPx(t, img, 10, 10, 0, 0, 0, 0, 3)
	// поворот на 90°: узор не пуст и не залит целиком
	const rot = `<svg viewBox="0 0 16 16"><defs><pattern id="p" width="8" height="8" patternUnits="userSpaceOnUse" patternTransform="rotate(90)">
<rect width="4" height="2" fill="#ff0000"/></pattern></defs><rect width="16" height="16" fill="url(#p)"/></svg>`
	img = render(t, rot, 16, 16)
	var on, off int
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			if img.RGBAAt(x, y).A > 200 {
				on++
			} else if img.RGBAAt(x, y).A == 0 {
				off++
			}
		}
	}
	if on < 20 || off < 100 {
		t.Errorf("повёрнутый узор: закрашено %d, пусто %d", on, off)
	}
}

func TestPattern_ElementTransformCarriesToTile(t *testing.T) {
	// transform фигуры сдвигает и узор (узор живёт в системе координат фигуры).
	const src = `<svg viewBox="0 0 32 16"><defs><pattern id="p" width="8" height="8" patternUnits="userSpaceOnUse">
<rect width="4" height="8" fill="#ff0000"/></pattern></defs><rect width="16" height="16" fill="url(#p)" transform="translate(16 0)"/></svg>`
	img := render(t, src, 32, 16)
	expectPx(t, img, 17, 4, 255, 0, 0, 255, 3)
	expectPx(t, img, 21, 4, 0, 0, 0, 0, 3)
	expectPx(t, img, 25, 4, 255, 0, 0, 255, 3)
	expectPx(t, img, 4, 4, 0, 0, 0, 0, 3)
}

func TestPattern_HrefInheritsAttributesAndChildren(t *testing.T) {
	const src = `<svg viewBox="0 0 16 16" xmlns:xlink="http://www.w3.org/1999/xlink"><defs>
<pattern id="base" width="8" height="8" patternUnits="userSpaceOnUse"><rect width="4" height="4" fill="#ff0000"/></pattern>
<pattern id="p" xlink:href="#base" x="4"/></defs><rect width="16" height="16" fill="url(#p)"/></svg>`
	img := render(t, src, 16, 16)
	expectPx(t, img, 6, 2, 255, 0, 0, 255, 3) // сдвинутая x=4 плитка из base
	expectPx(t, img, 2, 2, 0, 0, 0, 0, 3)
}

func TestPattern_StrokePattern(t *testing.T) {
	const src = `<svg viewBox="0 0 16 16"><defs><pattern id="p" width="4" height="4" patternUnits="userSpaceOnUse"><rect width="4" height="4" fill="#00ff00"/></pattern></defs>
<rect x="4" y="4" width="8" height="8" fill="none" stroke="url(#p)" stroke-width="4"/></svg>`
	img := render(t, src, 16, 16)
	expectPx(t, img, 4, 8, 0, 255, 0, 255, 3)
	expectPx(t, img, 8, 8, 0, 0, 0, 0, 3)
}

func TestPattern_DegenerateAndMissing(t *testing.T) {
	cases := map[string]string{
		"нулевая ширина": `<pattern id="p" width="0" height="4" patternUnits="userSpaceOnUse"><rect width="2" height="2"/></pattern>`,
		"без размеров":   `<pattern id="p" patternUnits="userSpaceOnUse"><rect width="2" height="2"/></pattern>`,
	}
	for name, def := range cases {
		img := render(t, `<svg viewBox="0 0 8 8"><defs>`+def+`</defs><rect width="8" height="8" fill="url(#p) #f00"/></svg>`, 8, 8)
		// Узор найден, но пуст: «ничего», а не запасной цвет.
		if img.RGBAAt(4, 4).A != 0 {
			t.Errorf("%s: ожидалось «ничего», got %v", name, img.RGBAAt(4, 4))
		}
	}
	img := render(t, `<svg viewBox="0 0 8 8"><rect width="8" height="8" fill="url(#нет) #f00"/></svg>`, 8, 8)
	expectPx(t, img, 4, 4, 255, 0, 0, 255, 1) // ссылка в пустоту — запасной цвет
}

func TestPattern_ZeroBBoxIsNothing(t *testing.T) {
	img := render(t, `<svg viewBox="0 0 8 8"><defs><pattern id="p" width="0.5" height="0.5"><rect width="4" height="4"/></pattern></defs>
<path d="M0 4 H8" stroke="none" fill="url(#p)"/></svg>`, 8, 8)
	if img.RGBAAt(4, 4).A != 0 {
		t.Error("bbox нулевой высоты: узор неприменим")
	}
}

func TestPattern_SelfReferenceDoesNotHang(t *testing.T) {
	const src = `<svg viewBox="0 0 8 8"><defs><pattern id="p" width="4" height="4" patternUnits="userSpaceOnUse">
<rect width="4" height="4" fill="url(#p)"/><rect width="2" height="2" fill="#f00"/></pattern></defs><rect width="8" height="8" fill="url(#p)"/></svg>`
	img := render(t, src, 8, 8)
	expectPx(t, img, 1, 1, 255, 0, 0, 255, 3)
}

func TestPattern_NestedPatternAndGradientContent(t *testing.T) {
	const src = `<svg viewBox="0 0 16 16"><defs>
<linearGradient id="g" x1="0" x2="1"><stop offset="0" stop-color="#000"/><stop offset="1" stop-color="#fff"/></linearGradient>
<pattern id="in" width="2" height="2" patternUnits="userSpaceOnUse"><rect width="1" height="1" fill="#0000ff"/></pattern>
<pattern id="p" width="16" height="16" patternUnits="userSpaceOnUse"><rect width="8" height="16" fill="url(#g)"/><rect x="8" width="8" height="16" fill="url(#in)"/></pattern></defs>
<rect width="16" height="16" fill="url(#p)"/></svg>`
	img := render(t, src, 16, 16)
	l, r := img.RGBAAt(1, 8), img.RGBAAt(6, 8)
	if !(r.R > l.R+60) || l.A != 255 {
		t.Errorf("градиент в узоре: слева %v, справа %v", l, r)
	}
	expectPx(t, img, 8, 0, 0, 0, 255, 255, 3) // вложенный узор: синяя точка в начале плитки
	expectPx(t, img, 9, 0, 0, 0, 0, 0, 3)
}

func TestPattern_TintRecolorsTile(t *testing.T) {
	doc, _ := Parse([]byte(`<svg viewBox="0 0 8 8">` + checkerDefs + `<rect width="8" height="8" fill="url(#p)"/></svg>`))
	img := doc.Rasterize(8, 8, color.RGBA{0, 255, 0, 255}, true)
	expectPx(t, img, 2, 2, 0, 255, 0, 255, 3)
	expectPx(t, img, 6, 6, 0, 255, 0, 255, 3)
	expectPx(t, img, 6, 2, 0, 0, 0, 0, 3)
}

func TestPattern_ScaledRasterStaysSharp(t *testing.T) {
	// Плитка растрируется под масштаб вывода: при увеличении ×4 край клетки
	// остаётся резким (а не размытым растяжением 8-пиксельной плитки).
	doc, _ := Parse([]byte(`<svg viewBox="0 0 8 8">` + checkerDefs + `<rect width="8" height="8" fill="url(#p)"/></svg>`))
	img := doc.Rasterize(64, 64, color.RGBA{A: 255}, false)
	expectPx(t, img, 30, 30, 255, 0, 0, 255, 2)
	expectPx(t, img, 33, 30, 0, 0, 0, 0, 2)
}

func TestPattern_ShapeKeepsNoFillColor(t *testing.T) {
	doc, _ := Parse([]byte(`<svg viewBox="0 0 8 8">` + checkerDefs + `<rect width="8" height="8" fill="url(#p)"/></svg>`))
	if len(doc.Shapes) != 1 || doc.Shapes[0].FillPattern == nil || !doc.Shapes[0].HasFill {
		t.Fatalf("ожидалась фигура с FillPattern: %+v", doc.Shapes)
	}
	if len(doc.Shapes[0].FillPattern.Shapes) != 2 {
		t.Errorf("в плитке %d фигур, ожидалось 2", len(doc.Shapes[0].FillPattern.Shapes))
	}
}
