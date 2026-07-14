package svg

import (
	"image/color"
	"testing"
)

// alphaAt возвращает альфу пикселя (0..255).
func alphaAt(img interface {
	At(x, y int) color.Color
}, x, y int) uint32 {
	_, _, _, a := img.At(x, y).RGBA()
	return a >> 8
}

// TestRasterize_CircleCenterFilledCornersEmpty: круг заполняет центр, но не углы.
func TestRasterize_Circle(t *testing.T) {
	const src = `<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="11" fill="#ffffff"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	img := doc.Rasterize(48, 48, color.RGBA{}, false)

	// Центр закрашен.
	if a := alphaAt(img, 24, 24); a < 250 {
		t.Errorf("центр круга альфа=%d, ожидалось ~255", a)
	}
	// Углы прозрачны.
	for _, pt := range [][2]int{{1, 1}, {46, 1}, {1, 46}, {46, 46}} {
		if a := alphaAt(img, pt[0], pt[1]); a > 5 {
			t.Errorf("угол (%d,%d) альфа=%d, ожидалось ~0", pt[0], pt[1], a)
		}
	}
	// Проверим цвет центра — белый.
	r, g, b, _ := img.At(24, 24).RGBA()
	if r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
		t.Errorf("центр не белый: rgb(%d,%d,%d)", r>>8, g>>8, b>>8)
	}
}

// TestRasterize_EvenOddRing: кольцо (внешний + внутренний круг, evenodd) —
// центр пуст (дырка), тело кольца закрашено.
func TestRasterize_EvenOddRing(t *testing.T) {
	const src = `<svg viewBox="0 0 24 24">
		<path fill-rule="evenodd" fill="#ffffff"
		      d="M12 1 A11 11 0 1 0 12 23 A11 11 0 1 0 12 1 Z
		         M12 7 A5 5 0 1 0 12 17 A5 5 0 1 0 12 7 Z"/>
	</svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Shapes[0].EvenOdd {
		t.Fatal("shape не evenodd")
	}
	img := doc.Rasterize(48, 48, color.RGBA{}, false)

	// Центр (дырка) прозрачен.
	if a := alphaAt(img, 24, 24); a > 20 {
		t.Errorf("центр кольца альфа=%d, ожидалось ~0 (дырка)", a)
	}
	// Тело кольца (между r=5 и r=11, напр. радиус ~8 → пиксель ~ x=24, y=8) закрашено.
	// В координатах 48px: центр (24,24), радиус 8 ед → 16px, точка (24, 24-16)=(24,8).
	if a := alphaAt(img, 24, 8); a < 200 {
		t.Errorf("тело кольца альфа=%d, ожидалось закрашено", a)
	}
	// Снаружи прозрачно.
	if a := alphaAt(img, 1, 1); a > 5 {
		t.Errorf("снаружи кольца альфа=%d", a)
	}
}

// TestRasterize_Recolor: currentColor подставляется; Tint перекрашивает всё.
func TestRasterize_Recolor(t *testing.T) {
	const src = `<svg viewBox="0 0 10 10">
		<rect x="0" y="0" width="10" height="10" fill="#ff0000"/>
		<rect x="2" y="2" width="6" height="6" fill="currentColor"/>
	</svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	green := color.RGBA{0, 255, 0, 255}

	// Без tint: currentColor → зелёный, красный остаётся красным.
	img := doc.Rasterize(20, 20, green, false)
	// Центр (10,10) — внутренний rect currentColor → зелёный.
	r, g, b, _ := img.At(10, 10).RGBA()
	if !(g>>8 > 200 && r>>8 < 60 && b>>8 < 60) {
		t.Errorf("currentColor центр rgb(%d,%d,%d) не зелёный", r>>8, g>>8, b>>8)
	}
	// Угол внешнего rect (1,1) — красный.
	r2, g2, b2, _ := img.At(1, 1).RGBA()
	if !(r2>>8 > 200 && g2>>8 < 60 && b2>>8 < 60) {
		t.Errorf("внешний rect rgb(%d,%d,%d) не красный", r2>>8, g2>>8, b2>>8)
	}

	// С tint: всё зелёное.
	imgT := doc.Rasterize(20, 20, green, true)
	r3, g3, b3, _ := imgT.At(1, 1).RGBA()
	if !(g3>>8 > 200 && r3>>8 < 60 && b3>>8 < 60) {
		t.Errorf("tint: внешний rect rgb(%d,%d,%d) не перекрашен в зелёный", r3>>8, g3>>8, b3>>8)
	}
}

// TestRasterize_Cache: повторный вызов RasterizeCached возвращает тот же объект.
func TestRasterizeCached_ReturnsSameInstance(t *testing.T) {
	const src = `<svg viewBox="0 0 10 10"><rect width="10" height="10" fill="#000"/></svg>`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	a := doc.RasterizeCached(16, 16, color.RGBA{}, false)
	b := doc.RasterizeCached(16, 16, color.RGBA{}, false)
	if a != b {
		t.Errorf("кэш не переиспользован: %p != %p", a, b)
	}
	// Другой размер — другой объект.
	c := doc.RasterizeCached(32, 32, color.RGBA{}, false)
	if c == a {
		t.Errorf("разные размеры дали один объект")
	}
	doc.InvalidateCache()
	d := doc.RasterizeCached(16, 16, color.RGBA{}, false)
	if d == a {
		t.Errorf("после InvalidateCache объект должен пересоздаться")
	}
}

// TestRasterize_AspectRatio: неквадратный bounds — иконка вписана и центрирована.
func TestRasterize_AspectRatioCentered(t *testing.T) {
	const src = `<svg viewBox="0 0 10 10"><rect width="10" height="10" fill="#fff"/></svg>`
	doc, _ := Parse([]byte(src))
	// 40 шириной, 20 высотой → масштаб по высоте (20), иконка 20×20 по центру:
	// занимает x∈[10,30). Пиксель (2,10) — вне иконки (прозрачен), (20,10) — внутри.
	img := doc.Rasterize(40, 20, color.RGBA{}, false)
	if a := alphaAt(img, 2, 10); a > 5 {
		t.Errorf("слева от центрированной иконки альфа=%d, ожидалось 0", a)
	}
	if a := alphaAt(img, 20, 10); a < 250 {
		t.Errorf("центр иконки альфа=%d, ожидалось 255", a)
	}
}
