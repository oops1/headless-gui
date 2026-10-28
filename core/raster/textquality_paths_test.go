package raster

import (
	hgtext "github.com/oops1/headless-gui/v3/core/text"
	"github.com/oops1/headless-gui/v3/internal/testreset"
	"image"
	"image/color"
	"testing"
)

// Шейпер (комбинируемые знаки, RTL): маски хинтуются, ширина прежняя, строчные
// на одних строках.
func TestTextQuality_ShapedPath(t *testing.T) {
	testreset.Globals(t)
	line := "o   a   c   é   o   a   c"
	if !hgtext.NeedsShaping(line) {
		t.Fatal("строка не идёт через шейпер")
	}
	for _, sz := range []int{10, 11, 12, 13, 14, 16, 21, 23} {
		off := tqRender(tqFont(t, hgtext.TextQuality{}), sz, 20, line)
		on := tqRender(tqFont(t, hgtext.TextQuality{Hinting: true, Contrast: hgtext.DefaultTextContrast}), sz, 20, line)
		if a, b := off.measureWithFallback(off.fontCache, line, float64(sz)*72/96), on.measureWithFallback(on.fontCache, line, float64(sz)*72/96); a != b {
			t.Errorf("%d px: ширина шейпера %d → %d", sz, a, b)
		}
		if string(off.back.Pix) == string(on.back.Pix) {
			t.Errorf("%d px: шейпер режим не заметил", sz)
		}
		spans := tqClusters(on.back, 0.5)
		if len(spans) < 7 {
			t.Fatalf("%d px: букв %d", sz, len(spans))
		}
		// «o a c» до и после акцента: одни и те же строки.
		for _, i := range []int{1, 2, 4, 5, 6} {
			if spans[i] != spans[0] {
				t.Errorf("%d px: буква %d на строках %v, первая — %v", sz, i, spans[i], spans[0])
			}
		}
		_, m0 := tqStats(off.back)
		if _, m1 := tqStats(on.back); m1 <= m0 {
			t.Errorf("%d px: средняя заливка шейпера %.3f → %.3f", sz, m0, m1)
		}
	}
}

// Повёрнутый текст: набор пикселей тот же при любом повороте, контраст
// применяется один раз (по настоящему цвету), и режим на него действует.
func TestTextQuality_RotatedText(t *testing.T) {
	testreset.Globals(t)
	ink := func(c *Canvas) (sum int) {
		for i := 0; i < len(c.back.Pix); i += 4 {
			sum += tqBG - int(c.back.Pix[i])
		}
		return sum
	}
	draw := func(q hgtext.TextQuality, angle int) int {
		fc := tqFont(t, q)
		c := NewCanvasScaled(120, 120, 1, fc)
		c.FillRect(0, 0, 120, 120, color.RGBA{tqBG, tqBG, tqBG, 255})
		if !c.DrawTextRotated("oops-hp.local", 60, 60, 9, "", angle, color.RGBA{20, 20, 20, 255}) {
			t.Fatalf("поворот %d не принят", angle)
		}
		return ink(c)
	}
	on := hgtext.TextQuality{Hinting: true, Contrast: hgtext.DefaultTextContrast}
	ref := draw(on, 0)
	for _, a := range []int{90, 180, 270} {
		if got := draw(on, a); got != ref {
			t.Errorf("поворот %d: сумма заливки %d, у горизонтального %d", a, got, ref)
		}
	}
	if off := draw(hgtext.TextQuality{}, 90); off >= ref {
		t.Errorf("режим на повёрнутом тексте не виден: выкл %d, вкл %d", off, ref)
	}
	// Контраст ровно один раз: сумма при 90° с хинтингом и контрастом равна
	// сумме у горизонтального, а не «контраст в квадрате».
	if a, b := draw(hgtext.TextQuality{Contrast: 1}, 90), draw(hgtext.TextQuality{Contrast: 1}, 0); a != b {
		t.Errorf("контраст на повёрнутом тексте: %d против %d", a, b)
	}
}

// HiDPI: зоны считаются на физических пикселях, строчные на одних строках при
// любом масштабе.
func TestTextQuality_HiDPI(t *testing.T) {
	testreset.Globals(t)
	for _, dpi := range []float64{96, 120, 144, 168, 192, 240} {
		for _, pt := range []float64{7.5, 9, 10, 10.5} {
			fc := tqFont(t, hgtext.TextQuality{Hinting: true})
			fc.SetDPI(dpi)
			tops, bots := map[int]bool{}, map[int]bool{}
			for _, r := range "oaecsxzv" {
				top, bot, ok := tqRows(fc.Glyph(pt, r), 100)
				if !ok {
					t.Fatalf("пустой глиф %q", r)
				}
				tops[top], bots[bot] = true, true
			}
			if len(tops) != 1 || len(bots) != 1 {
				t.Errorf("dpi=%v %vpt: верх %v низ %v", dpi, pt, tops, bots)
			}
		}
	}
}

// LCD: каналы получают разное покрытие, порядок RGB/BGR зеркален, на
// прозрачном фоне и без просьбы текст серый, ширина прежняя.
func TestTextQuality_LCD(t *testing.T) {
	testreset.Globals(t)
	// Центр заливки по X для канала ch (0 — R, 2 — B).
	centroid := func(img *image.RGBA, ch int) float64 {
		var sx, sw float64
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				w := float64(tqBG - int(img.Pix[img.PixOffset(x, y)+ch]))
				sx += w * float64(x)
				sw += w
			}
		}
		return sx / sw
	}
	gray := func(img *image.RGBA) bool {
		for i := 0; i < len(img.Pix); i += 4 {
			if img.Pix[i] != img.Pix[i+1] || img.Pix[i] != img.Pix[i+2] {
				return false
			}
		}
		return true
	}
	rgb := tqRender(tqFont(t, hgtext.TextQuality{Hinting: true, LCD: true}), 14, 20, tqLines...).back
	bgr := tqRender(tqFont(t, hgtext.TextQuality{Hinting: true, LCD: true, LCDOrder: hgtext.LCDOrderBGR}), 14, 20, tqLines...).back
	plain := tqRender(tqFont(t, hgtext.TextQuality{Hinting: true}), 14, 20, tqLines...).back
	if gray(rgb) || gray(bgr) {
		t.Fatal("LCD-текст на непрозрачном фоне остался серым")
	}
	if !gray(plain) {
		t.Fatal("без LCD текст получил цветную окантовку")
	}
	// Красный подпиксель слева: он выборкой снимает штрих левее синего, а в
	// индексах пикселей заливка канала лежит ПРАВЕЕ (пиксель с красным
	// подпикселем на штрихе — тот, чей правый край уже на штрихе): R−B ≈ +2/3.
	dr := centroid(rgb, 0) - centroid(rgb, 2)
	db := centroid(bgr, 0) - centroid(bgr, 2)
	if dr <= 0.1 || db >= -0.1 {
		t.Errorf("порядок подпикселей: RGB даёт сдвиг R−B %.3f, BGR %.3f", dr, db)
	}
	// Заливка в среднем по каналам — как у серого покрытия того же глифа.
	var lcdSum, grSum int
	for i := 0; i < len(rgb.Pix); i += 4 {
		lcdSum += 3*tqBG - int(rgb.Pix[i]) - int(rgb.Pix[i+1]) - int(rgb.Pix[i+2])
		grSum += 3 * (tqBG - int(plain.Pix[i]))
	}
	if r := float64(lcdSum) / float64(grSum); r < 0.9 || r > 1.1 {
		t.Errorf("суммарная заливка LCD/серый = %.3f", r)
	}

	// Прозрачный фон: покрытие серое по среднему, пиксели без цветной каймы.
	fc := tqFont(t, hgtext.TextQuality{LCD: true})
	c := NewCanvasScaled(80, 30, 1, fc)
	c.DrawTextSize("oops", 2, 4, 9, color.RGBA{99, 99, 99, 255})
	nz := 0
	for i := 0; i < len(c.back.Pix); i += 4 {
		p := c.back.Pix[i : i+4]
		if p[3] == 0 {
			continue
		}
		nz++
		if p[0] != p[1] || p[0] != p[2] {
			t.Fatalf("на прозрачном фоне пиксель цветной: %v", p)
		}
	}
	if nz == 0 {
		t.Fatal("на прозрачном фоне ничего не нарисовано")
	}

	// Формат BGRX: тот же кадр с переставленными R и B.
	rgbaC := tqRender(tqFont(t, hgtext.TextQuality{LCD: true}), 12, 20, tqLines...)
	fcB := tqFont(t, hgtext.TextQuality{LCD: true})
	w, h := 420, len(tqLines)*(12+10)+8
	bc := NewCanvasScaled(w, h, 1, fcB)
	bc.format = FormatBGRX
	bc.FillRect(0, 0, w, h, color.RGBA{tqBG, tqBG, tqBG, 255})
	for i, s := range tqLines {
		bc.DrawTextSize(s, 4, 4+i*(12+10), 9, color.RGBA{20, 20, 20, 255})
	}
	for i := 0; i < len(bc.back.Pix); i += 4 {
		a, b := rgbaC.back.Pix[i:i+3], bc.back.Pix[i:i+3]
		if a[0] != b[2] || a[1] != b[1] || a[2] != b[0] {
			t.Fatalf("BGRX: пиксель %d: RGBA %v, BGRX %v", i/4, a, b)
		}
	}

	// Ширина от LCD не зависит.
	base := tqFont(t, hgtext.TextQuality{})
	lcd := tqFont(t, hgtext.TextQuality{LCD: true})
	for _, s := range tqLines {
		if base.Measure(s, 9) != lcd.Measure(s, 9) {
			t.Errorf("ширина %q изменилась от LCD", s)
		}
	}
}

// Качество и клип: отсечение и скруглённое отсечение работают на LCD и
// хинтованных масках как на обычных (ничего не рисуется вне клипа).
func TestTextQuality_Clip(t *testing.T) {
	testreset.Globals(t)
	for _, q := range []hgtext.TextQuality{
		{Hinting: true, Contrast: 1}, {LCD: true}, {Hinting: true, LCD: true, Contrast: 0.5},
	} {
		fc := tqFont(t, q)
		c := NewCanvasScaled(100, 40, 1, fc)
		c.FillRect(0, 0, 100, 40, color.RGBA{tqBG, tqBG, tqBG, 255})
		c.SetClip(image.Rect(20, 0, 40, 40))
		c.DrawTextSize("oops-hp.local Tandem", 2, 10, 9, color.RGBA{0, 0, 0, 255})
		c.ClearClip()
		ink := 0
		for y := 0; y < 40; y++ {
			for x := 0; x < 100; x++ {
				if c.back.Pix[c.back.PixOffset(x, y)] != tqBG {
					ink++
					if x < 20 || x >= 40 {
						t.Fatalf("%+v: пиксель (%d,%d) вне клипа", q, x, y)
					}
				}
			}
		}
		if ink == 0 {
			t.Errorf("%+v: в клипе ничего нет", q)
		}
	}
}
