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

// stemRuns разбирает строку маски на стволы — подряд идущие закрашенные
// пиксели — и возвращает для каждого число полных (≥ 95 %) и серых (5…95 %)
// пикселей. Остаток у основания кромки (< 5 %) — не серый столбец.
func stemRuns(row []uint8) (runs [][2]int) {
	full, grey, in := 0, 0, false
	flush := func() {
		if in {
			runs = append(runs, [2]int{full, grey})
		}
		full, grey, in = 0, 0, false
	}
	for _, a := range row {
		switch {
		case a <= 12:
			flush()
		case a >= 243:
			full, in = full+1, true
		default:
			grey, in = grey+1, true
		}
	}
	flush()
	return runs
}

// stemRow — строка маски глифа на 3 px выше базовой линии: у прямых букв там
// только вертикальные стволы.
func stemRow(g hgtext.CachedGlyph) []uint8 {
	if g.Mask == nil {
		return nil
	}
	y := -3 - g.OffY
	if y < 0 || y >= g.Mask.Rect.Dy() {
		return nil
	}
	return g.Mask.Pix[y*g.Mask.Stride : y*g.Mask.Stride+g.Mask.Rect.Dx()]
}

// У ствола прямой буквы на пиксельной сетке стоит хотя бы одна кромка: в нём
// есть полностью закрашенный столбец и не больше одного серого (у ствола
// целой толщины серых нет вовсе). Без привязки ствол ложится на границу
// пикселей — два серых столбца и ни одного полного; тест это различает.
// Проверяются все четыре подпиксельные фазы пера: привязка считает и сдвиг
// пера.
func TestTextHintX_StemsOnPixelGrid(t *testing.T) {
	testreset.Globals(t)
	const letters = "lIHnmhНПпшШ" // без «и», «н»: на этой высоте у них диагональ и перекладина
	for _, file := range hxFonts {
		for _, q := range []hgtext.TextQuality{{HintingX: true}, hgtext.DefaultQuality()} {
			fc := hxFont(t, file, q)
			for _, px := range []float64{11, 12, 13, 14, 16} {
				for _, r := range letters {
					for ph := 0; ph < hgtext.SubpixelPhases; ph++ {
						row := stemRow(fc.GlyphAt(pxToPt(px), r, ph))
						for i, run := range stemRuns(row) {
							if run[0] == 0 || run[1] > 1 {
								t.Errorf("%s %+v %.0fpx %q фаза %d: ствол %d — полных столбцов %d, серых %d: кромка не на сетке (%v)",
									file, q, px, r, ph, i, run[0], run[1], row)
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
	soft := 0
	for _, r := range letters {
		for _, run := range stemRuns(stemRow(fc.Glyph(pxToPt(11), r))) {
			if run[0] == 0 || run[1] > 1 {
				soft++
			}
		}
	}
	if soft == 0 {
		t.Error("без HintingX стволы Open Sans 11 px уже на сетке — тест ничего не проверяет")
	}
}

// Толщина ствола, далёкая от целого числа пикселей, сохраняется: буква не
// худеет и не сужается (отчёт Go.Git GG-98 на v4.4.0 — Go Regular 10 pt, ствол
// 1,28 px округлялся до 1 px, текст выходил тонким и разреженным). Количество
// чернил строки с привязкой почти то же, что без неё, на ходовых кеглях
// интерфейса у всех шрифтов набора.
func TestTextHintX_KeepsWeight(t *testing.T) {
	testreset.Globals(t)
	const text = "Репозиторий Правка ENGINE awesome-design Hill minimum"
	ink := func(fc *hgtext.FontCache, pt float64) int {
		sum := 0
		for _, r := range text {
			if g := fc.Glyph(pt, r); g.Mask != nil {
				for _, a := range g.Mask.Pix {
					sum += int(a)
				}
			}
		}
		return sum
	}
	for _, file := range hxFonts {
		plain := hxFont(t, file, hgtext.TextQuality{Hinting: true})
		snapped := hxFont(t, file, hgtext.TextQuality{Hinting: true, HintingX: true})
		for _, pt := range []float64{8, 9, 10, 11, 12, 14} {
			a, b := ink(plain, pt), ink(snapped, pt)
			if ratio := float64(b) / float64(a); ratio < 0.93 || ratio > 1.07 {
				t.Errorf("%s %g pt: чернил с привязкой %.3f от прежнего — вес текста изменился", file, pt, ratio)
			}
		}
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
