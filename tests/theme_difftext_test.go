package tests

import (
	"image/color"
	"math"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// GG-64: у темы есть цвета ТЕКСТА для добавленного и удалённого. DiffAddStrong
// и DiffDelStrong — это полосы, смешанные с фоном поля ввода, и «+42» ими в
// списке изменений выходит бледным.

// wcagLum — относительная яркость по WCAG 2.
func wcagLum(c color.RGBA) float64 {
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// wcagContrast — контраст двух цветов, от 1 до 21.
func wcagContrast(a, b color.RGBA) float64 {
	la, lb := wcagLum(a), wcagLum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Во всех пресетах цвета текста заданы, читаются на фоне поля ввода (контраст
// не ниже 4,5:1 — порог WCAG AA для обычного текста) и отличаются от полос:
// ради этого пункт и заводился.
func TestTheme_DiffTextColorsReadable(t *testing.T) {
	for _, name := range widget.ThemeNames() {
		th := widget.ThemeByName(name)
		if th == nil {
			t.Fatalf("пресет %q не найден", name)
		}
		bg := th.InputBG
		if bg.A == 0 {
			bg = th.PanelBG
		}
		for _, c := range []struct {
			field      string
			text, band color.RGBA
		}{
			{"DiffAddText", th.DiffAddText, th.DiffAddStrong},
			{"DiffDelText", th.DiffDelText, th.DiffDelStrong},
		} {
			if c.text.A == 0 {
				t.Errorf("%s: %s не задан", name, c.field)
				continue
			}
			if got := wcagContrast(c.text, bg); got < 4.5 {
				t.Errorf("%s: %s %v на фоне %v — контраст %.2f, нужно не ниже 4.5", name, c.field, c.text, bg, got)
			}
			if c.text == c.band {
				t.Errorf("%s: %s совпадает с цветом полосы — текстом им писать нельзя", name, c.field)
			}
		}
	}
}

// Поля проходят мост тем туда и обратно: тема, превращённая в профиль и
// обратно, сохраняет цвета текста.
func TestTheme_DiffTextColorsRoundTrip(t *testing.T) {
	th := widget.ThemeByName("Win11 Light")
	want := color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 255}
	th.DiffAddText = want
	th.DiffDelText = color.RGBA{R: 0x65, G: 0x43, B: 0x21, A: 255}

	back := roundTrip(t, th)
	if back.DiffAddText != th.DiffAddText || back.DiffDelText != th.DiffDelText {
		t.Fatalf("мост тем потерял цвета текста: %v %v → %v %v",
			th.DiffAddText, th.DiffDelText, back.DiffAddText, back.DiffDelText)
	}
}
