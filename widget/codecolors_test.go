package widget

import (
	"image/color"
	"testing"
)

// Цвета сравнения и редактора кода в теме — пункт 5 замечаний difftool.
//
// DiffView выводил их сам, смешивая красный и зелёный с фоном «на глаз»: в
// Win2000 и Mac это давало оттенки, которых автор темы не задумывал, и
// поправить их было негде.

func codeColors(t *Theme) map[string]color.RGBA {
	return map[string]color.RGBA{
		"DiffAddBG": t.DiffAddBG, "DiffAddStrong": t.DiffAddStrong,
		"DiffDelBG": t.DiffDelBG, "DiffDelStrong": t.DiffDelStrong,
		"TextSelectionBG": t.TextSelectionBG,
		"SyntaxKeyword":   t.SyntaxKeyword, "SyntaxString": t.SyntaxString,
		"SyntaxComment": t.SyntaxComment, "SyntaxNumber": t.SyntaxNumber,
		"SyntaxFunc": t.SyntaxFunc,
	}
}

// Все поля заполнены во всех пресетах: пустое поле дало бы прозрачную полосу
// строки или невидимое ключевое слово.
func TestCodeColors_FilledInEveryPreset(t *testing.T) {
	for _, name := range ThemeNames() {
		for field, c := range codeColors(ThemeByName(name)) {
			if c.A != 255 {
				t.Errorf("%s: %s не заполнено (%v)", name, field, c)
			}
		}
	}
}

// Полосы строк — оттенки фона поля ввода, а не чистые красный и зелёный:
// тёмная тема даёт тёмные полосы, светлая — светлые. Иначе текст строки на
// полосе не читался бы.
func TestCodeColors_BandsFollowInputBrightness(t *testing.T) {
	for _, name := range ThemeNames() {
		th := ThemeByName(name)
		darkCard := luminance(th.InputBG) < 128
		for _, band := range []color.RGBA{th.DiffAddBG, th.DiffDelBG} {
			if darkBand := luminance(band) < 128; darkBand != darkCard {
				t.Errorf("%s: полоса %v по яркости не совпадает с полем ввода %v",
					name, band, th.InputBG)
			}
		}
		// «Сильный» вариант плотнее полосы: внутристрочная разница должна
		// выделяться на своей же строке.
		if colorDist(th.DiffAddStrong, th.InputBG) <= colorDist(th.DiffAddBG, th.InputBG) {
			t.Errorf("%s: сильный зелёный не плотнее полосы", name)
		}
		if colorDist(th.DiffDelStrong, th.InputBG) <= colorDist(th.DiffDelBG, th.InputBG) {
			t.Errorf("%s: сильный красный не плотнее полосы", name)
		}
	}
}

// Пресет, сменивший поле ввода, считает цвета от СВОЕГО поля, а не от базовой
// темы, с которой начинал.
func TestCodeColors_RecomputedForOwnInput(t *testing.T) {
	w10, w11 := Win10DarkTheme(), Win11DarkTheme()
	if w10.InputBG == w11.InputBG {
		t.Skip("у пресетов одинаковое поле ввода — проверять нечего")
	}
	if w10.DiffAddBG == w11.DiffAddBG {
		t.Error("Win11 Dark унаследовал полосы Win10 Dark вместо своих")
	}
}

// Подсветка видна на фоне редактора.
func TestCodeColors_SyntaxReadable(t *testing.T) {
	for _, name := range ThemeNames() {
		th := ThemeByName(name)
		for _, c := range []color.RGBA{th.SyntaxKeyword, th.SyntaxString, th.SyntaxFunc} {
			if colorDist(c, th.InputBG) < 120 {
				t.Errorf("%s: цвет подсветки %v почти сливается с полем %v", name, c, th.InputBG)
			}
		}
	}
}

func colorDist(a, b color.RGBA) int {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) + d(a.G, b.G) + d(a.B, b.B)
}
