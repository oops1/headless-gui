package widget

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
)

// Диалог встроенных профилей: затемнение полупрозрачное (стол остаётся виден),
// полоса заголовка совпадает с заголовком активного окна, а не с серым телом
// под белой подписью.
func TestBuiltinProfiles_DialogScrimAndTitle(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{
		theme.ProfileWindows2000, theme.ProfileWindows2000Blue,
		theme.ProfileWindows10, theme.ProfileWindows10Dark,
		theme.ProfileWindows11, theme.ProfileWindows11Dark,
		theme.ProfileMacOS, theme.ProfileMacOSDark,
	} {
		th := ThemeFromProfile(m, n)
		if th.DialogDim.A == 0 || th.DialogDim.A >= 200 {
			t.Errorf("%s: затемнение диалога %v должно быть полупрозрачным", n, th.DialogDim)
		}
		if th.DialogDim.R != 0 || th.DialogDim.G != 0 || th.DialogDim.B != 0 {
			t.Errorf("%s: затемнение диалога %v должно быть чёрным", n, th.DialogDim)
		}
		if th.DialogTitleBG != th.TitleBG {
			t.Errorf("%s: заголовок диалога %v должен совпадать с заголовком активного окна %v", n, th.DialogTitleBG, th.TitleBG)
		}
	}

	// Классика: тёмно-синий заголовок с белым текстом (читается), а не серый.
	w2k := ThemeFromProfile(m, theme.ProfileWindows2000)
	if w2k.DialogTitleBG != (color.RGBA{10, 36, 106, 255}) || w2k.TitleText != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("Windows 2000: заголовок диалога %v, текст %v", w2k.DialogTitleBG, w2k.TitleText)
	}
	if w2k.DialogTitleBG == w2k.DialogBG {
		t.Error("Windows 2000: заголовок диалога слился с его телом")
	}
	// Синяя разновидность получает свой заголовок, а не родительский.
	blue := ThemeFromProfile(m, theme.ProfileWindows2000Blue)
	if blue.DialogTitleBG != (color.RGBA{0, 84, 227, 255}) {
		t.Errorf("Windows 2000 Blue: заголовок диалога %v", blue.DialogTitleBG)
	}
}
