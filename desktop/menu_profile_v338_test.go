package desktop

import (
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Меню Windows 10 — тонкий шеврон подменю и мягкая тень, меню Windows 2000 —
// значки, перекрашиваемые по состоянию пункта: всё из профиля, потребителю не
// нужно ставить IconTint на пункты (замечание WinLine после v3.32.0, п.3).
func TestThemeMenu_Windows10ChevronAndWindows2000IconTint(t *testing.T) {
	testreset.Globals(t)
	look := func(name string) *widget.PopupMenu {
		m := widget.NewPopupMenu()
		themeMenu(m, menuTheme(t, name))
		return m
	}

	w10 := look(theme.ProfileWindows10)
	if w10.ChevronRight != 18 {
		t.Errorf("Windows 10: шеврон подменю %d, ждали 18 (как у Windows 11)", w10.ChevronRight)
	}
	if w10.Elevation <= 0 || w10.ShadowColor.A == 0 {
		t.Errorf("Windows 10: мягкой тени меню нет: высота %v, цвет %v", w10.Elevation, w10.ShadowColor)
	}
	if w10.TintIcons {
		t.Error("Windows 10: значки меню цветные, перекрашивать их нельзя")
	}
	// Тёмная и светлая разновидность Windows 10 наследуют шеврон.
	if m := look(theme.ProfileWindows10Dark); m.ChevronRight != 18 {
		t.Errorf("Windows 10 Dark: шеврон %d", m.ChevronRight)
	}

	for _, name := range []string{theme.ProfileWindows2000, theme.ProfileWindows2000Blue} {
		if m := look(name); !m.TintIcons {
			t.Errorf("%s: значки меню не перекрашиваются по состоянию", name)
		}
	}
	for _, name := range []string{theme.ProfileWindows11, theme.ProfileWindows11Dark, theme.ProfileMacOS} {
		if m := look(name); m.TintIcons {
			t.Errorf("%s: значки меню перекрашиваются, а должны остаться цветными", name)
		}
	}

	// Смена темы на лету: перекраска Windows 2000 не остаётся в меню Windows 11.
	m := widget.NewPopupMenu()
	themeMenu(m, menuTheme(t, theme.ProfileWindows2000))
	themeMenu(m, menuTheme(t, theme.ProfileWindows11))
	if m.TintIcons {
		t.Error("перекраска значков осталась после смены темы на Windows 11")
	}
}
