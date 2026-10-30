package raster

import (
	"crypto/sha256"
	"encoding/hex"
	hgtext "github.com/oops1/headless-gui/v4/core/text"
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
)

// Слой качества текста (TextQuality): хинтинг по вертикали, контраст, LCD.
// Шрифт — Open Sans из assets/fonts (его и используют клиенты); нет файла —
// тест пропускается.

// tqFont читает Open Sans Regular и создаёт кэш шрифта с заданным слоем.
func tqFont(t *testing.T, q hgtext.TextQuality) *hgtext.FontCache {
	t.Helper()
	data, err := os.ReadFile("../../assets/fonts/OpenSans-Regular.ttf")
	if err != nil {
		t.Skipf("Open Sans недоступен: %v", err)
	}
	fc := hgtext.NewFontCacheFromData(data, hgtext.DefaultDPI)
	fc.SetQuality(q)
	return fc
}

const tqBG = 243

// tqRender рисует строки шрифтом fc серым цветом lum на фоне tqBG (кегль в
// пикселях при 96 dpi) и возвращает холст.
func tqRender(fc *hgtext.FontCache, sizePx int, lum uint8, texts ...string) *Canvas {
	w, h := 420, len(texts)*(sizePx+10)+8
	c := NewCanvasScaled(w, h, 1, fc)
	c.FillRect(0, 0, w, h, color.RGBA{tqBG, tqBG, tqBG, 255})
	for i, s := range texts {
		c.DrawTextSize(s, 4, 4+i*(sizePx+10), float64(sizePx)*72/96, color.RGBA{lum, lum, lum, 255})
	}
	return c
}

// tqFill — заливка пикселя по формуле клиента: (фон − яркость) / (фон − 20).
func tqFill(img *image.RGBA, x, y int) float64 {
	return (tqBG - float64(img.Pix[img.PixOffset(x, y)])) / (tqBG - 20)
}

// tqStats — доля «полных» пикселей текста (заливка ≥ 0,6) и средняя заливка
// пикселей текста (заливка > 0,02), как в замере клиента.
func tqStats(img *image.RGBA) (full, mean float64) {
	n, nf := 0, 0
	sum := 0.0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			f := tqFill(img, x, y)
			if f <= 0.02 {
				continue
			}
			n++
			sum += f
			if f >= 0.6 {
				nf++
			}
		}
	}
	if n == 0 {
		return 0, 0
	}
	return float64(nf) / float64(n), sum / float64(n)
}

var tqLines = []string{"oops-hp.local", "Подключение", "Tandem", "oops-hp · WinLine"}

// Приёмка клиента: на строках из замера (Open Sans 12 и 14 px, тёмный 20 и
// серый 99 текст на фоне 243) доля полных пикселей не ниже 31 %, средняя
// заливка не ниже 0,46 — по двум цветам вместе, как в таблице клиента.
func TestTextQuality_Sharpness(t *testing.T) {
	testreset.Globals(t)
	on := hgtext.TextQuality{Hinting: true, Contrast: hgtext.DefaultTextContrast}
	for _, sz := range []int{12, 14} {
		var fullOff, meanOff, fullOn, meanOn float64
		for _, lum := range []uint8{20, 99} {
			f, m := tqStats(tqRender(tqFont(t, hgtext.TextQuality{}), sz, lum, tqLines...).back)
			fullOff, meanOff = fullOff+f/2, meanOff+m/2
			f, m = tqStats(tqRender(tqFont(t, on), sz, lum, tqLines...).back)
			fullOn, meanOn = fullOn+f/2, meanOn+m/2
		}
		t.Logf("%d px: выкл — полных %.1f%%, средняя %.3f; вкл — полных %.1f%%, средняя %.3f",
			sz, fullOff*100, meanOff, fullOn*100, meanOn)
		if fullOn < 0.31 {
			t.Errorf("%d px: полных пикселей %.1f%% < 31%%", sz, fullOn*100)
		}
		if meanOn < 0.46 {
			t.Errorf("%d px: средняя заливка %.3f < 0,46", sz, meanOn)
		}
		if fullOn <= fullOff || meanOn <= meanOff {
			t.Errorf("%d px: режим не улучшил: полных %.3f→%.3f, средняя %.3f→%.3f", sz, fullOff, fullOn, meanOff, meanOn)
		}
	}
}

// Тёмный текст на светлом не светлеет: с контрастом средняя заливка не ниже,
// чем без него (и хинтинг один тоже не делает текст бледнее).
func TestTextQuality_DarkTextNotLighter(t *testing.T) {
	testreset.Globals(t)
	for _, sz := range []int{11, 12, 13, 14, 16} {
		for _, lum := range []uint8{0, 20, 99} {
			_, m0 := tqStats(tqRender(tqFont(t, hgtext.TextQuality{}), sz, lum, tqLines...).back)
			_, mh := tqStats(tqRender(tqFont(t, hgtext.TextQuality{Hinting: true}), sz, lum, tqLines...).back)
			_, mc := tqStats(tqRender(tqFont(t, hgtext.TextQuality{Contrast: hgtext.DefaultTextContrast}), sz, lum, tqLines...).back)
			if mh < m0-0.005 || mc <= m0 {
				t.Errorf("%d px, яркость %d: средняя заливка выкл %.3f, хинтинг %.3f, контраст %.3f", sz, lum, m0, mh, mc)
			}
		}
	}
}

// tqRows — первая и последняя строка маски глифа, где покрытие не ниже порога,
// в координатах от базовой линии.
func tqRows(g hgtext.CachedGlyph, thr uint8) (top, bot int, ok bool) {
	if g.Mask == nil {
		return 0, 0, false
	}
	w, h := g.Mask.Rect.Dx(), g.Mask.Rect.Dy()
	top, bot = 1<<30, -1<<30
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if g.Mask.Pix[y*g.Mask.Stride+x] >= thr {
				if g.OffY+y < top {
					top = g.OffY + y
				}
				if g.OffY+y > bot {
					bot = g.OffY + y
				}
			}
		}
	}
	return top, bot, bot >= top
}

// tqLowercaseMismatches считает размеры, где строчные буквы одного кегля
// (круглые «o a e c s» и плоские «x z v», латиница и кириллица) не делят
// верхнюю и нижнюю строку пикселей.
func tqLowercaseMismatches(t *testing.T, q hgtext.TextQuality, sub bool) (bad, total int) {
	t.Helper()
	for px := 9; px <= 24; px++ {
		fc := tqFont(t, q)
		pt := float64(px) * 72 / 96
		var tops, bots map[int][]rune = map[int][]rune{}, map[int][]rune{}
		phases := 1
		if sub {
			fc.SetSubpixel(true)
			fc.SetQuality(q)
			phases = hgtext.SubpixelPhases
		}
		for _, r := range "oaecsxzvоахе" {
			for ph := 0; ph < phases; ph++ {
				top, bot, ok := tqRows(fc.GlyphAt(pt, r, ph), 100)
				if !ok {
					t.Fatalf("глиф %q (%d px) пуст", r, px)
				}
				tops[top] = append(tops[top], r)
				bots[bot] = append(bots[bot], r)
			}
		}
		total++
		if len(tops) != 1 || len(bots) != 1 {
			bad++
			t.Logf("%d px: верх %v, низ %v", px, tops, bots)
		}
	}
	return
}

// Верх и низ строчных одного кегля лежат на одних и тех же строках пикселей.
// Контрольная проверка: без хинтинга на части кеглей это не так (тест умеет
// видеть разницу).
func TestTextQuality_LowercaseRowsAlign(t *testing.T) {
	testreset.Globals(t)
	for _, sub := range []bool{false, true} {
		bad, total := tqLowercaseMismatches(t, hgtext.TextQuality{Hinting: true}, sub)
		if bad != 0 {
			t.Errorf("subpixel=%v: хинтинг: %d из %d кеглей с разъехавшимися строчными", sub, bad, total)
		}
	}
	bad, total := tqLowercaseMismatches(t, hgtext.TextQuality{}, false)
	t.Logf("без хинтинга разъезжаются %d из %d кеглей", bad, total)
	if bad == 0 {
		t.Error("без хинтинга строчные ни на одном кегле не разъезжаются — проверка ничего не различает")
	}
}

// tqClusters режет строку-картинку на буквы по пустым столбцам и для каждой
// возвращает первую и последнюю строку, где заливка не ниже thr.
func tqClusters(img *image.RGBA, thr float64) (spans [][2]int) {
	b := img.Bounds()
	col := func(x int) bool {
		for y := b.Min.Y; y < b.Max.Y; y++ {
			if tqFill(img, x, y) > 0.1 {
				return true
			}
		}
		return false
	}
	for x := b.Min.X; x < b.Max.X; {
		if !col(x) {
			x++
			continue
		}
		top, bot := 1<<30, -1
		for ; x < b.Max.X && col(x); x++ {
			for y := b.Min.Y; y < b.Max.Y; y++ {
				if tqFill(img, x, y) >= thr {
					if y < top {
						top = y
					}
					if y > bot {
						bot = y
					}
				}
			}
		}
		spans = append(spans, [2]int{top, bot})
	}
	return spans
}

// «o», «a», «е» и другие строчные во всех вхождениях строки лежат на одних и
// тех же пиксельных строках — в обычном и в подпиксельном режиме (там маска
// каждой буквы снимается для своей четверти пикселя).
func TestTextQuality_SameRowsAcrossLine(t *testing.T) {
	testreset.Globals(t)
	line := "o   a   е   o   a   е   c   e   o   a   е   c   e   s   x   z"
	for _, sub := range []bool{false, true} {
		for _, sz := range []int{11, 12, 13, 14, 16} {
			fc := tqFont(t, hgtext.TextQuality{Hinting: true, Contrast: hgtext.DefaultTextContrast})
			fc.SetSubpixel(sub)
			spans := tqClusters(tqRender(fc, sz, 20, line).back, 0.5)
			if len(spans) != 16 {
				t.Fatalf("sub=%v %d px: найдено %d букв, ожидалось 16", sub, sz, len(spans))
			}
			for i, sp := range spans {
				if sp != spans[0] {
					t.Errorf("sub=%v %d px: буква %d занимает строки %v, первая — %v", sub, sz, i, sp, spans[0])
				}
			}
		}
	}
}

// Ширина строки, шаги глифов и кернинг от режима не зависят: ни в обычном
// режиме, ни в подпиксельном.
func TestTextQuality_MeasureUnchanged(t *testing.T) {
	testreset.Globals(t)
	modes := []hgtext.TextQuality{
		{}, {Hinting: true}, {Contrast: 1}, {Hinting: true, Contrast: hgtext.DefaultTextContrast},
		{Hinting: true, LCD: true}, {Hinting: true, Contrast: 0.5, LCD: true, LCDOrder: hgtext.LCDOrderBGR},
	}
	for _, sub := range []bool{false, true} {
		base := tqFont(t, hgtext.TextQuality{})
		base.SetSubpixel(sub)
		for _, q := range modes {
			fc := tqFont(t, q)
			fc.SetSubpixel(sub)
			for _, pt := range []float64{7.5, 9, 10.5, 12} {
				for _, s := range append(tqLines, "AVATAR To Wa", "Tandem") {
					if a, b := base.Measure(s, pt), fc.Measure(s, pt); a != b {
						t.Errorf("sub=%v %+v %.1fpt %q: ширина %d, было %d", sub, q, pt, s, b, a)
					}
					am, bm := base.MeasureRunes(s, pt), fc.MeasureRunes(s, pt)
					for i := range am {
						if am[i] != bm[i] {
							t.Errorf("sub=%v %+v %.1fpt %q: позиция %d: %d, было %d", sub, q, pt, s, i, bm[i], am[i])
							break
						}
					}
				}
				for _, r := range "oaHl.-Пд" {
					if a, b := base.Glyph(pt, r).Advance, fc.Glyph(pt, r).Advance; a != b {
						t.Errorf("sub=%v %+v %.1fpt %q: шаг %v, было %v", sub, q, pt, r, b, a)
					}
				}
				if a, b := base.Kern(pt, 'A', 'V'), fc.Kern(pt, 'A', 'V'); a != b {
					t.Errorf("кернинг %v, было %v", b, a)
				}
			}
		}
	}
}

// Выключенный режим — прежний вид побайтно: маски совпадают с растеризацией
// opentype, а вкл → выкл возвращает ровно те же байты.
func TestTextQuality_OffIsLegacy(t *testing.T) {
	testreset.Globals(t)
	legacy := tqFont(t, hgtext.TextQuality{})
	toggled := tqFont(t, hgtext.TextQuality{})
	toggled.SetQuality(hgtext.TextQuality{Hinting: true, Contrast: 1})
	for _, r := range "oaеHl,Пg" {
		toggled.Glyph(9, r) // прогреть кэш режимом «вкл»
	}
	toggled.SetQuality(hgtext.TextQuality{})
	for _, r := range "oaеHl,Пg" {
		a, b := legacy.Glyph(9, r), toggled.Glyph(9, r)
		if a.OffX != b.OffX || a.OffY != b.OffY || a.Advance != b.Advance ||
			string(a.Mask.Pix) != string(b.Mask.Pix) {
			t.Errorf("%q: после вкл→выкл маска отличается от прежней", r)
		}
	}
	// Хинтинг действительно меняет маску (тест различает режимы).
	h := tqFont(t, hgtext.TextQuality{Hinting: true})
	if string(h.Glyph(9, 'e').Mask.Pix) == string(legacy.Glyph(9, 'e').Mask.Pix) {
		t.Error("хинтинг не изменил маску «e»")
	}
}

// Детерминизм: хэш масок (все режимы, несколько кеглей, фазы) зафиксирован и
// одинаков на windows/amd64 и linux/amd64; прогон дважды даёт то же.
//
// v3.43.1: хэши режимов с хинтингом сдвинулись намеренно — знаки вне зон
// («·», «.», «@» из набора) больше не растягиваются, а сдвигаются целиком
// (TestTextHint_FloatingSignsKeepShape); маски букв набора не изменились.
func TestTextQuality_MaskHash(t *testing.T) {
	testreset.Globals(t)
	sum := func(q hgtext.TextQuality, sub bool) string {
		fc := tqFont(t, q)
		fc.SetSubpixel(sub)
		fc.SetQuality(q)
		h := sha256.New()
		for _, pt := range []float64{7.5, 9, 10.5, 13.5} {
			for _, r := range "oaеxHlgpgPЁ,.-·й@" {
				for ph := 0; ph < hgtext.SubpixelPhases; ph++ {
					g := fc.GlyphAt(pt, r, ph)
					h.Write([]byte{byte(g.OffX), byte(g.OffY), byte(g.Advance)})
					if g.Mask != nil {
						h.Write([]byte{byte(g.Mask.Rect.Dx()), byte(g.Mask.Rect.Dy())})
						h.Write(g.Mask.Pix)
					}
					if g.LCD != nil {
						h.Write([]byte{byte(g.LCD.W), byte(g.LCD.H)})
						h.Write(g.LCD.Pix)
					}
				}
			}
		}
		return hex.EncodeToString(h.Sum(nil))[:16]
	}
	cases := []struct {
		name string
		q    hgtext.TextQuality
		sub  bool
		want string
	}{
		{"выкл", hgtext.TextQuality{}, false, "df8183d9d3efa9e2"},
		{"хинтинг", hgtext.TextQuality{Hinting: true}, false, "bd66ca7c469c697f"},
		{"хинтинг+subpixel", hgtext.TextQuality{Hinting: true}, true, "4c2520ead3db99a9"},
		{"lcd rgb", hgtext.TextQuality{Hinting: true, LCD: true}, false, "a121546a0d5def02"},
		{"lcd bgr", hgtext.TextQuality{LCD: true, LCDOrder: hgtext.LCDOrderBGR}, false, "30d41a494b94623d"},
	}
	for _, c := range cases {
		got := sum(c.q, c.sub)
		if again := sum(c.q, c.sub); again != got {
			t.Errorf("%s: нестабилен: %s и %s", c.name, got, again)
		}
		t.Logf("%s: %s", c.name, got)
		if c.want != got {
			t.Errorf("%s: хэш масок %s, ожидался %s", c.name, got, c.want)
		}
	}
}
