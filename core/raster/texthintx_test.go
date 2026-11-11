package raster

import (
	"os"
	"testing"

	hgtext "github.com/oops1/headless-gui/v4/core/text"
	"github.com/oops1/headless-gui/v4/internal/testreset"
)

// Привязка вертикальных штрихов к пиксельной сетке (TextQuality.HintingX).

// hxFonts — шрифты из assets/fonts, на которых проверяется привязка: у них
// разные пропорции и разная толщина ствола на мелком кегле.
var hxFonts = []string{"OpenSans-Regular.ttf", "Roboto-Regular.ttf", "Inter-Regular.ttf", "Go-Regular.ttf"}

func hxFont(t *testing.T, file string, q hgtext.TextQuality) *hgtext.FontCache {
	t.Helper()
	data, err := os.ReadFile("../../assets/fonts/" + file)
	if err != nil {
		t.Skipf("%s недоступен: %v", file, err)
	}
	fc := hgtext.NewFontCacheFromData(data, hgtext.DefaultDPI)
	fc.SetQuality(q)
	return fc
}

// pxToPt — кегль в пунктах для размера em в пикселях при 96 dpi.
func pxToPt(px float64) float64 { return px * 72 / 96 }

// Стволы прямых букв встают ровно на пиксели: в строке маски на три пикселя
// выше базовой линии (там у этих букв только вертикальные стволы) каждый
// пиксель либо пуст, либо закрашен целиком. Без привязки стволы ложатся на
// границу пикселей и дают серые столбцы — тест это различает. Проверяются все
// четыре подпиксельные фазы пера: привязка считает и сдвиг пера.
func TestTextHintX_StemsOnPixelGrid(t *testing.T) {
	testreset.Globals(t)
	const letters = "lIHnmhНПпшШ" // без «и», «н»: на этой высоте у них диагональ и перекладина
	for _, file := range hxFonts {
		for _, q := range []hgtext.TextQuality{{HintingX: true}, hgtext.DefaultQuality()} {
			fc := hxFont(t, file, q)
			for _, px := range []float64{11, 12, 13, 14, 16} {
				for _, r := range letters {
					for ph := 0; ph < hgtext.SubpixelPhases; ph++ {
						g := fc.GlyphAt(pxToPt(px), r, ph)
						if g.Mask == nil {
							continue
						}
						y := -3 - g.OffY // строка маски на 3 px выше базовой линии
						if y < 0 || y >= g.Mask.Rect.Dy() {
							continue
						}
						row := g.Mask.Pix[y*g.Mask.Stride : y*g.Mask.Stride+g.Mask.Rect.Dx()]
						for x, a := range row {
							if a > 12 && a < 243 { // серый: 5…95 % (остаток у основания кромки — не серый столбец)
								t.Errorf("%s %+v %.0fpx %q фаза %d: пиксель %d строки ствола %d — ствол не на сетке (%v)",
									file, q, px, r, ph, x, a, row)
								break
							}
						}
					}
				}
			}
		}
	}

	// Без привязки та же проверка падает хотя бы на одной букве: тест
	// различает режимы.
	fc := hxFont(t, "OpenSans-Regular.ttf", hgtext.TextQuality{Hinting: true})
	grey := 0
	for _, r := range letters {
		g := fc.Glyph(pxToPt(11), r)
		y := -2 - g.OffY
		for _, a := range g.Mask.Pix[y*g.Mask.Stride : y*g.Mask.Stride+g.Mask.Rect.Dx()] {
			if a != 0 && a < 250 {
				grey++
			}
		}
	}
	if grey == 0 {
		t.Error("без HintingX стволы Open Sans 11 px уже на сетке — тест ничего не проверяет")
	}
}

// inkSpan — суммарное покрытие маски и её закрашенные столбцы (от начала
// пера: OffX учтён).
func inkSpan(g hgtext.CachedGlyph) (sum, left, right int) {
	if g.Mask == nil {
		return 0, 0, 0
	}
	w, h := g.Mask.Rect.Dx(), g.Mask.Rect.Dy()
	left, right = 1<<30, -1<<30
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := int(g.Mask.Pix[y*g.Mask.Stride+x])
			if a < 32 {
				continue
			}
			sum += a
			left = min(left, g.OffX+x)
			right = max(right, g.OffX+x)
		}
	}
	return sum, left, right
}

// Привязка не меняет вид буквы сильнее, чем округление штрихов: количество
// чернил от 0,6 до 1,4 прежнего — толщина около 1,5 px округляется до целой
// (короткий стык контуров, ошибочно принятый за ствол, раздувал «а», «р», «ф»,
// «L» у Roboto сильнее), а края глифа сдвигаются не
// больше чем на пиксель (штрих сдвигается не больше чем на полпикселя).
func TestTextHintX_ShapeKept(t *testing.T) {
	testreset.Globals(t)
	const set = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" +
		"абвгдеёжзийклмнопрстуфхцчшщъыьэюяАБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ"
	for _, file := range hxFonts {
		plain := hxFont(t, file, hgtext.TextQuality{Hinting: true})
		snapped := hxFont(t, file, hgtext.TextQuality{Hinting: true, HintingX: true})
		for _, px := range []float64{11, 12, 14, 18} {
			for _, r := range set {
				a, al, ar := inkSpan(plain.Glyph(pxToPt(px), r))
				b, bl, br := inkSpan(snapped.Glyph(pxToPt(px), r))
				if a == 0 {
					continue
				}
				if ratio := float64(b) / float64(a); ratio < 0.6 || ratio > 1.4 {
					t.Errorf("%s %.0fpx %q: чернил %.2f от прежнего", file, px, r, ratio)
				}
				if d := bl - al; d < -1 || d > 1 {
					t.Errorf("%s %.0fpx %q: левый край сдвинулся на %d px", file, px, r, d)
				}
				if d := br - ar; d < -1 || d > 1 {
					t.Errorf("%s %.0fpx %q: правый край сдвинулся на %d px", file, px, r, d)
				}
			}
		}
	}
}

// Одинаковые буквы в строке рисуются одинаково, а стволы «Ш» на мелком
// кегле стоят через равные промежутки — сдвиги округления не копятся слева
// направо.
func TestTextHintX_EvenStems(t *testing.T) {
	testreset.Globals(t)
	for _, file := range hxFonts {
		fc := hxFont(t, file, hgtext.DefaultQuality())
		for _, px := range []float64{11, 12, 13, 14} {
			for _, r := range "шШmщ" {
				g := fc.Glyph(pxToPt(px), r)
				y := -2 - g.OffY
				if g.Mask == nil || y < 0 || y >= g.Mask.Rect.Dy() {
					continue
				}
				row := g.Mask.Pix[y*g.Mask.Stride : y*g.Mask.Stride+g.Mask.Rect.Dx()]
				var starts []int
				for x, a := range row {
					if a >= 250 && (x == 0 || row[x-1] < 250) {
						starts = append(starts, x)
					}
				}
				if len(starts) != 3 {
					continue // у буквы в этой строке не три ствола (хвост «щ» и т. п.)
				}
				d1, d2 := starts[1]-starts[0], starts[2]-starts[1]
				if d := d1 - d2; d < -1 || d > 1 {
					t.Errorf("%s %.0fpx %q: стволы через %d и %d px (%v)", file, px, r, d1, d2, row)
				}
			}
		}
	}
}
