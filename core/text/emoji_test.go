package text

import (
	"github.com/go-text/typesetting/font/opentype/tables"
	"image"
	"image/color"
	"testing"
)

// TestEmoji_NeedsShaping — эмодзи и селекторы вариаций уходят в шейпер
// (кластеры ZWJ/тона), а не в быстрый per-rune путь.
func TestEmoji_NeedsShaping(t *testing.T) {
	shaped := []string{
		"🙂",     // U+1F642 (эмотикон)
		"👍🏽",    // палец + модификатор тона кожи
		"👨‍👩‍👧", // ZWJ-семья
		"🇺🇸",    // региональные индикаторы (флаг)
		"☺️",    // база <0x1F000 + VS16 (эмодзи-презентация)
		"a🙂b",
	}
	for _, s := range shaped {
		if !NeedsShaping(s) {
			t.Errorf("needsShaping(%q) = false, want true", s)
		}
	}
	// Обычный текст (в т.ч. em-dash из golden-сцен) НЕ должен триггерить шейпинг.
	for _, s := range []string{"Hello", "Метка — Label", "Пункт 1"} {
		if NeedsShaping(s) {
			t.Errorf("needsShaping(%q) = true, want false", s)
		}
	}
}

// TestEmoji_PremulColor — перевод straight→premultiplied альфа.
func TestEmoji_PremulColor(t *testing.T) {
	cases := []struct{ in, want color.RGBA }{
		{color.RGBA{255, 0, 0, 255}, color.RGBA{255, 0, 0, 255}}, // непрозрачный — без изменений
		{color.RGBA{255, 0, 0, 128}, color.RGBA{128, 0, 0, 128}}, // половинная альфа
		{color.RGBA{200, 100, 50, 0}, color.RGBA{0, 0, 0, 0}},    // полностью прозрачный
		{color.RGBA{255, 255, 255, 64}, color.RGBA{64, 64, 64, 64}},
	}
	for _, c := range cases {
		if got := premulColor(c.in); got != c.want {
			t.Errorf("premulColor(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestEmoji_CompositeMaskColor — композиция слоя (альфа-маска × цвет) на
// синтетических данных: проверка Over-смешивания premultiplied.
func TestEmoji_CompositeMaskColor(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 2, 1))
	mask := image.NewAlpha(image.Rect(0, 0, 2, 1))
	mask.Pix[0] = 255                 // полностью покрыт
	mask.Pix[1] = 128                 // полупокрыт
	red := color.RGBA{255, 0, 0, 255} // premultiplied непрозрачный красный
	compositeMaskColor(dst, mask, 0, 0, red)

	p0 := dst.RGBAAt(0, 0)
	if p0 != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("пиксель[0] = %v, want {255,0,0,255}", p0)
	}
	p1 := dst.RGBAAt(1, 0)
	if p1 != (color.RGBA{128, 0, 0, 128}) {
		t.Errorf("пиксель[1] = %v, want {128,0,0,128}", p1)
	}

	// Второй слой поверх: синий по всей маске (255) — перекрывает красный там,
	// где непрозрачен, и Over-смешивается на полупрозрачном пикселе.
	mask2 := image.NewAlpha(image.Rect(0, 0, 2, 1))
	mask2.Pix[0], mask2.Pix[1] = 255, 255
	blue := color.RGBA{0, 0, 255, 255}
	compositeMaskColor(dst, mask2, 0, 0, blue)
	if got := dst.RGBAAt(0, 0); got != (color.RGBA{0, 0, 255, 255}) {
		t.Errorf("после синего слоя пиксель[0] = %v, want {0,0,255,255}", got)
	}
}

// TestEmoji_CPALPremul — выбор цвета из палитры CPAL с домножением на alpha и
// премультипликацией; foreground (0xFFFF) и индекс за пределами палитры.
func TestEmoji_CPALPremul(t *testing.T) {
	// CPAL хранит записи как B,G,R,A.
	pal := []tables.ColorRecord{
		{Blue: 0, Green: 0, Red: 255, Alpha: 255}, // 0: красный
		{Blue: 255, Green: 0, Red: 0, Alpha: 255}, // 1: синий
	}
	textCol := color.RGBA{R: 10, G: 20, B: 30, A: 255}

	// Непрозрачный красный, alpha=1.0.
	dyn := false
	c, ok := cpalPremul(pal, 0, 1.0, textCol, &dyn)
	if !ok || c != (color.RGBA{255, 0, 0, 255}) || dyn {
		t.Errorf("pal[0]@1.0 = (%v,%v,dyn=%v), want ({255,0,0,255},true,false)", c, ok, dyn)
	}
	// Красный с alpha=0.5 → премультиплицированный.
	dyn = false
	c, ok = cpalPremul(pal, 0, 0.5, textCol, &dyn)
	if !ok || c.A < 126 || c.A > 129 || c.R != c.A {
		t.Errorf("pal[0]@0.5 = %v (ok=%v), ожидался премультиплицированный красный ~A=128", c, ok)
	}
	// foreground (0xFFFF) → цвет текста, помечает dynamic.
	dyn = false
	c, ok = cpalPremul(pal, 0xFFFF, 1.0, textCol, &dyn)
	if !ok || c != textCol || !dyn {
		t.Errorf("pal[0xFFFF] = (%v,ok=%v,dyn=%v), want (%v,true,true)", c, ok, dyn, textCol)
	}
	// Индекс за пределами палитры → не найден.
	dyn = false
	if _, ok = cpalPremul(pal, 99, 1.0, textCol, &dyn); ok {
		t.Error("индекс за пределами палитры не должен возвращать ok=true")
	}
}

// TestEmoji_RepresentativeColor — цвет заливки PaintGlyph: сплошной сквозь
// преобразование, усреднённый градиент.
func TestEmoji_RepresentativeColor(t *testing.T) {
	pal := []tables.ColorRecord{
		{Blue: 0, Green: 0, Red: 255, Alpha: 255}, // 0: красный
		{Blue: 255, Green: 0, Red: 0, Alpha: 255}, // 1: синий
	}
	textCol := color.RGBA{A: 255}
	dyn := false

	// PaintSolid, обёрнутый в PaintTransform — цвет достаётся сквозь трансформ.
	wrapped := tables.PaintTransform{Paint: tables.PaintSolid{PaletteIndex: 0, Alpha: 16384}}
	c, ok := representativeColor(wrapped, pal, textCol, &dyn, 0)
	if !ok || c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("solid сквозь transform = (%v,%v), want ({255,0,0,255},true)", c, ok)
	}

	// Линейный градиент red→blue: усреднение даёт ~{128,0,128,255}.
	grad := tables.PaintLinearGradient{ColorLine: tables.ColorLine{ColorStops: []tables.ColorStop{
		{PaletteIndex: 0, Alpha: 16384},
		{PaletteIndex: 1, Alpha: 16384},
	}}}
	c, ok = representativeColor(grad, pal, textCol, &dyn, 0)
	if !ok || c.R < 120 || c.R > 135 || c.B < 120 || c.B > 135 || c.A != 255 {
		t.Errorf("градиент red→blue усреднён = %v, ожидался ~{128,0,128,255}", c)
	}
}

// TestEmoji_Affine — композиция аффинных преобразований (перенос + масштаб).
func TestEmoji_Affine(t *testing.T) {
	// Масштаб ×2, затем (внешний) перенос на (10, 0): точка (1,1) → (12, 2).
	m := compose(affTranslate(10, 0), affScale(2, 2))
	x := m.xx*1 + m.xy*1 + m.dx
	y := m.yx*1 + m.yy*1 + m.dy
	if x != 12 || y != 2 {
		t.Errorf("compose(translate, scale) точка (1,1) → (%v,%v), want (12,2)", x, y)
	}
}
