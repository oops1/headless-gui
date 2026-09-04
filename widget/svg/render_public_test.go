package svg

import (
	"bytes"
	"image/color"
	"testing"
)

// TestRender_Basic: простой прямоугольник растеризуется в непустую картинку
// заданного размера — базовое доказательство того, что Render вообще рисует.
func TestRender_Basic(t *testing.T) {
	const src = `<svg viewBox="0 0 10 10"><rect width="10" height="10" fill="#3366ff"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	img := Render(doc, 20, 20, color.RGBA{})
	if img == nil {
		t.Fatal("Render вернул nil для валидного документа")
	}
	b := img.Bounds()
	if b.Dx() != 20 || b.Dy() != 20 {
		t.Errorf("размер картинки %dx%d, ожидалось 20x20", b.Dx(), b.Dy())
	}
	if a := alphaAt(img, 10, 10); a < 250 {
		t.Errorf("центр альфа=%d, ожидалось ~255 (непустая картинка)", a)
	}
}

// TestRender_TintOpaqueRecolors: tint с ненулевой альфой перекрашивает весь
// контент документа в tint — цвет пикселя внутри фигуры совпадает с tint.
func TestRender_TintOpaqueRecolors(t *testing.T) {
	const src = `<svg viewBox="0 0 10 10"><rect width="10" height="10" fill="#ff0000"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	tint := color.RGBA{0, 255, 0, 255} // зелёный, документ — красный
	img := Render(doc, 20, 20, tint)
	if img == nil {
		t.Fatal("Render вернул nil")
	}
	// Сравниваем оттенок (не точное значение) — сглаживание по краю не трогаем,
	// берём точку заведомо внутри фигуры.
	r, g, b, _ := img.At(10, 10).RGBA()
	if !(g>>8 > 200 && r>>8 < 60 && b>>8 < 60) {
		t.Errorf("tint не перекрасил фигуру: rgb(%d,%d,%d), ожидался зелёный", r>>8, g>>8, b>>8)
	}
}

// TestRender_TintTransparentKeepsDocumentColors: tint с нулевой альфой — это
// НЕ «не рисовать», а «оставить цвета документа». Проверяем, что фигура с
// явным fill остаётся своего цвета, а не становится прозрачной/перекрашенной.
func TestRender_TintTransparentKeepsDocumentColors(t *testing.T) {
	const src = `<svg viewBox="0 0 10 10"><rect width="10" height="10" fill="#ff0000"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	img := Render(doc, 20, 20, color.RGBA{}) // A=0 — прозрачный "tint"
	if img == nil {
		t.Fatal("Render вернул nil")
	}
	r, g, b, a := img.At(10, 10).RGBA()
	if a>>8 < 250 {
		t.Fatalf("фигура не должна стать прозрачной при tint.A==0, альфа=%d", a>>8)
	}
	if !(r>>8 > 200 && g>>8 < 60 && b>>8 < 60) {
		t.Errorf("цвет документа (красный) не сохранён: rgb(%d,%d,%d)", r>>8, g>>8, b>>8)
	}
}

// TestRender_DegenerateInputsReturnNil: nil-документ, нулевой и отрицательный
// размер — не паника, результат nil.
func TestRender_DegenerateInputsReturnNil(t *testing.T) {
	const src = `<svg viewBox="0 0 10 10"><rect width="10" height="10" fill="#fff"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	tint := color.RGBA{255, 0, 0, 255}

	cases := []struct {
		name string
		doc  *Document
		w, h int
	}{
		{"nil-doc", nil, 10, 10},
		{"zero-w", doc, 0, 10},
		{"zero-h", doc, 10, 0},
		{"negative-w", doc, -5, 10},
		{"negative-h", doc, 10, -5},
		{"zero-both", doc, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Render(%s) запаниковал: %v", c.name, r)
				}
			}()
			if img := Render(c.doc, c.w, c.h, tint); img != nil {
				t.Errorf("Render(%s) = %v, ожидался nil", c.name, img)
			}
		})
	}
}

// TestRender_MatchesSVGIconRasterization: Render и растеризация, которой
// пользуется сам SVGIcon (doc.RasterizeCached), должны давать одинаковый
// результат для одного документа/размера/цвета — Render не должен быть
// отдельным путём растеризации, ломающим кэш или дающим другую картинку.
//
// Проверяем оба режима tint:
//   - непрозрачный tint — то же самое, что SVGIcon.SetColor(tint) +
//     SetTint(true): совпадает однозначно, оба пути форсируют весь контент
//     в tint;
//   - прозрачный tint (цвета документа) — сравниваем с прямым вызовом
//     doc.Rasterize(w, h, ..., false) на фигуре без currentColor, где
//     подставляемый в currentColor цвет не влияет на пиксели, так что
//     результат идентичен независимо от выбранного Render'ом дефолта.
func TestRender_MatchesSVGIconRasterization(t *testing.T) {
	const src = `<svg viewBox="0 0 10 10"><rect width="10" height="10" fill="#123456"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("tint", func(t *testing.T) {
		tint := color.RGBA{10, 20, 30, 255}
		want := doc.RasterizeCached(16, 16, tint, true) // путь SVGIcon: Tint=true, Color=tint
		got := Render(doc, 16, 16, tint)
		if !bytes.Equal(want.Pix, got.Pix) || want.Rect != got.Rect {
			t.Errorf("Render(tint) разошёлся с RasterizeCached(..., true)")
		}
	})

	t.Run("no-tint", func(t *testing.T) {
		// Фигура без currentColor — значение "current" не влияет на пиксели,
		// поэтому какой бы дефолт Render ни подставлял при tint.A==0,
		// результат обязан совпасть с обычной растеризацией.
		want := doc.RasterizeCached(16, 16, color.RGBA{99, 99, 99, 255}, false)
		got := Render(doc, 16, 16, color.RGBA{})
		if !bytes.Equal(want.Pix, got.Pix) || want.Rect != got.Rect {
			t.Errorf("Render(без tint) разошёлся с RasterizeCached(..., false)")
		}
	})
}

// TestRender_UsesDocumentCache: Render не заводит отдельный кэш — повторный
// вызов с теми же параметрами отдаёт тот же объект, что и RasterizeCached
// (единый кэш документа, который используют и SVGIcon, и Render).
func TestRender_UsesDocumentCache(t *testing.T) {
	const src = `<svg viewBox="0 0 10 10"><rect width="10" height="10" fill="#fff"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	tint := color.RGBA{1, 2, 3, 255}
	a := Render(doc, 12, 12, tint)
	b := doc.RasterizeCached(12, 12, tint, true)
	if a != b {
		t.Errorf("Render не переиспользует кэш документа: %p != %p", a, b)
	}
}
