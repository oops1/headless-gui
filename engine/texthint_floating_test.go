package engine

import (
	"bytes"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
)

// Знаки вне зон («•» в поле пароля, «·» между именем и адресом, «°») с
// хинтингом не вытягиваются: маска побайтно та же, что без хинтинга, и того же
// размера — меняется разве что строка, на которой она стоит (сдвиг на целые
// пиксели). Отчёт RDP-клиента на v3.43.0: «•» 14 px был 4×5 вместо 4×4.
func TestTextHint_FloatingSignsKeepShape(t *testing.T) {
	testreset.Globals(t)
	plain := tqFont(t, TextQuality{})
	hinted := tqFont(t, TextQuality{Hinting: true, Contrast: DefaultTextContrast})
	for px := 9; px <= 24; px++ {
		pt := float64(px) * 72 / 96
		for _, r := range "•·°" {
			a, b := plain.GlyphAt(pt, r, 0), hinted.GlyphAt(pt, r, 0)
			if a.mask == nil || b.mask == nil {
				t.Fatalf("%d px %q: пустая маска", px, r)
			}
			if a.mask.Rect.Size() != b.mask.Rect.Size() || a.offX != b.offX {
				t.Errorf("%d px %q: рамка %v (x %d) → %v (x %d)", px, r,
					a.mask.Rect.Size(), a.offX, b.mask.Rect.Size(), b.offX)
				continue
			}
			if !bytes.Equal(a.mask.Pix, b.mask.Pix) {
				t.Errorf("%d px %q: форма знака изменилась", px, r)
			}
			if d := b.offY - a.offY; d < -2 || d > 2 {
				t.Errorf("%d px %q: сдвиг на %d строк — больше, чем растяжение зон", px, r, d)
			}
		}
	}
}

// Знаки со штрихами вне зон (дефис) по-прежнему выравниваются: толщина
// штриха — целое число пикселей, маска отличается от маски без хинтинга.
func TestTextHint_StemSignsStillSnap(t *testing.T) {
	testreset.Globals(t)
	plain := tqFont(t, TextQuality{})
	hinted := tqFont(t, TextQuality{Hinting: true})
	changed := 0
	for px := 9; px <= 24; px++ {
		pt := float64(px) * 72 / 96
		a, b := plain.GlyphAt(pt, '-', 0), hinted.GlyphAt(pt, '-', 0)
		if a.mask == nil || b.mask == nil {
			t.Fatalf("%d px: пустая маска дефиса", px)
		}
		if !bytes.Equal(a.mask.Pix, b.mask.Pix) || a.mask.Rect.Size() != b.mask.Rect.Size() {
			changed++
		}
	}
	if changed == 0 {
		t.Error("дефис с хинтингом ни на одном кегле не выровнялся — штрихи потеряли выравнивание")
	}
}

// То же в шейпере (строки с комбинируемыми знаками и RTL растеризуются
// отдельно, rasterizeGlyph): высота «•» с хинтингом та же, что без него.
func TestTextHint_FloatingSignsShaped(t *testing.T) {
	testreset.Globals(t)
	line := "é  •  é" // комбинируемый акцент — строка идёт через шейпер
	if !needsShaping(line) {
		t.Fatal("строка не идёт через шейпер")
	}
	for _, sz := range []int{12, 14, 16, 21} {
		off := tqClusters(tqRender(tqFont(t, TextQuality{}), sz, 20, line).back, 0.1)
		on := tqClusters(tqRender(tqFont(t, TextQuality{Hinting: true}), sz, 20, line).back, 0.1)
		if len(off) != 3 || len(on) != 3 {
			t.Fatalf("%d px: знаков %d и %d, ждали 3", sz, len(off), len(on))
		}
		if ho, hn := off[1][1]-off[1][0], on[1][1]-on[1][0]; ho != hn {
			t.Errorf("%d px: высота «•» в шейпере %d → %d строк", sz, ho+1, hn+1)
		}
	}
}
