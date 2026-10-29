package base

import (
	"image/color"
	"testing"
)

// Подпись крупного кегля центрируется по высоте своей строки, а не по
// зашитым 13 px (замечание проекта calc на v3.39.0). Для шрифта по умолчанию
// и мельче высота прежняя — 13 (GG-94: уменьшенная опускала мелкую подпись).
func TestButtonTextHeight_FollowsFontSize(t *testing.T) {
	def := DefaultFontSize()
	if got := buttonTextHeight(def); got != 13 {
		t.Errorf("шрифт по умолчанию: высота %d, ждали прежние 13", got)
	}
	if got := buttonTextHeight(def * 2); got != 26 {
		t.Errorf("вдвое крупнее: высота %d, ждали 26", got)
	}
	for _, pt := range []float64{def / 2, 8, def - 0.5} {
		if got := buttonTextHeight(pt); got != 13 {
			t.Errorf("%.1f pt (мельче шрифта по умолчанию): высота %d, ждали прежние 13", pt, got)
		}
	}
}

// Выключенная кнопка: непрозрачный фон по-прежнему уводится к своему фону;
// прозрачный остаётся прозрачным; с FlatWhenDisabled фон не меняется вовсе —
// у плоской кнопки калькулятора не появляется плашка.
func TestButtonDisabled_FlatAndTransparentKeepBackground(t *testing.T) {
	win := color.RGBA{R: 243, G: 243, B: 243, A: 255}
	b := NewButton("7")
	b.Background = win
	b.SetEnabled(false)
	if bg, _, _ := b.stateColors(false); bg == win {
		t.Error("обычная выключенная кнопка не приглушила фон — поведение изменилось")
	}
	b.FlatWhenDisabled = true
	bg, txt, _ := b.stateColors(false)
	if bg != win {
		t.Errorf("FlatWhenDisabled: фон %v, ждали прежний %v", bg, win)
	}
	if txt == b.TextColor {
		t.Error("FlatWhenDisabled: подпись не приглушена")
	}
	c := NewButton("8")
	c.Background = color.RGBA{}
	c.SetEnabled(false)
	if bg, _, _ := c.stateColors(false); bg.A != 0 {
		t.Errorf("прозрачная выключенная кнопка получила плашку %v", bg)
	}
}
