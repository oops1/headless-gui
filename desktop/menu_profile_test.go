package desktop

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// themeMenu читает вид меню из профиля: цвета стилей «menu» и размеры
// метриками theme.KeyMenu*. Профиль без этих токенов оставляет меню прежним.

func menuTheme(t *testing.T, name string) *theme.Manager {
	t.Helper()
	tm := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(tm); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetTheme(name); err != nil {
		t.Fatal(err)
	}
	return tm
}

func TestThemeMenu_Windows11LookFromProfile(t *testing.T) {
	m := widget.NewPopupMenu()
	themeMenu(m, menuTheme(t, theme.ProfileWindows11))

	if m.ItemHeight != 24 || m.SeparatorH != 8 || m.PaddingX != 12 || m.ItemInset != 4 ||
		m.CornerRadius != 8 || m.ItemCorner != 4 || m.ChevronRight != 18 || m.Elevation != 8 {
		t.Errorf("меню Windows 11 не прочитало профиль: %+v", m)
	}
	if m.Background != theme.RGB(249, 249, 249) {
		t.Errorf("заливка %v", m.Background)
	}
	// Плашка — полупрозрачная плёнка, а не готовый непрозрачный цвет.
	if m.HoverBG.A == 0 || m.HoverBG.A == 255 {
		t.Errorf("плашка наведения %v", m.HoverBG)
	}
	if m.ShortcutColor.A == 0 {
		t.Error("цвет сочетаний профилем не задан")
	}
}

// Смена темы на лету: меню, показанное под одной темой, под другой получает
// её размеры, а значения прежней не остаются.
func TestThemeMenu_SwitchingProfilesLeavesNothingBehind(t *testing.T) {
	m := widget.NewPopupMenu()
	themeMenu(m, menuTheme(t, theme.ProfileWindows11))
	themeMenu(m, menuTheme(t, theme.ProfileWindows2000))
	if m.ItemHeight != 20 || m.CornerRadius != 0 || m.ItemCorner != 0 || m.ChevronRight != 0 ||
		m.Elevation != 0 || m.PaddingY != 0 || m.SubMenuDelay != 400 {
		t.Errorf("следы Windows 11 в меню Windows 2000: %+v", m)
	}
}

// Профиль без токенов меню: то, что было, остаётся — ни размеров, ни скруглений.
func TestThemeMenu_ProfileWithoutTokensKeepsEngineMenu(t *testing.T) {
	p := theme.NewProfile("bare")
	p.SetStyle("menu", "", theme.StateNormal, theme.StyleDelta{
		Fill: theme.C(color.RGBA{R: 9, G: 9, B: 9, A: 255}),
	})
	tm := theme.NewManager()
	if err := tm.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetTheme("bare"); err != nil {
		t.Fatal(err)
	}
	ref := widget.NewPopupMenu()
	m := widget.NewPopupMenu()
	themeMenu(m, tm)
	if m.Background != (color.RGBA{R: 9, G: 9, B: 9, A: 255}) {
		t.Errorf("заливка профиля не применена: %v", m.Background)
	}
	if m.ItemHeight != ref.ItemHeight || m.PaddingX != ref.PaddingX || m.MinWidth != ref.MinWidth ||
		m.ItemInset != 0 || m.CornerRadius != 0 || m.ItemCorner != 0 || m.PaddingY != 0 {
		t.Errorf("профиль без токенов изменил размеры: %+v", m)
	}
}
