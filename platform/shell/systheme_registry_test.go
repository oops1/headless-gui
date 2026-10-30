package shell

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// DWM\AccentColor — DWORD в порядке ABGR, красный в младшем байте. Перепутать
// с ARGB — получить красный вместо синего.
func TestAccentFromDWMAccent(t *testing.T) {
	cases := []struct {
		v    uint32
		want color.RGBA
	}{
		{0xFFD47800, color.RGBA{R: 0x00, G: 0x78, B: 0xD4, A: 255}}, // акцент Windows по умолчанию
		{0xFF0000FF, color.RGBA{R: 0xFF, A: 255}},                   // красный — в младшем байте
		{0xFFFF0000, color.RGBA{B: 0xFF, A: 255}},
		{0x00112233, color.RGBA{R: 0x33, G: 0x22, B: 0x11, A: 255}}, // альфа игнорируется
	}
	for _, c := range cases {
		if got := accentFromDWMAccent(c.v); got != c.want {
			t.Errorf("AccentColor=%#x → %v, ждал %v", c.v, got, c.want)
		}
	}
}

// ColorizationColor, наоборот, ARGB.
func TestAccentFromColorization(t *testing.T) {
	if got, want := accentFromColorization(0xC40078D4), (color.RGBA{R: 0x00, G: 0x78, B: 0xD4, A: 255}); got != want {
		t.Errorf("ColorizationColor → %v, ждал %v", got, want)
	}
}

func TestSystemThemeFromAppsUseLight(t *testing.T) {
	cases := []struct {
		v    uint64
		want native.SystemTheme
	}{
		{0, native.SystemThemeDark},
		{1, native.SystemThemeLight},
		// Не 0 и не 1 — не «светлая по умолчанию»: что значит двойка, не
		// знает никто, и белое окно в тёмной теме хуже, чем свой выбор
		// приложения.
		{2, native.SystemThemeUnknown},
		{0xFFFFFFFF, native.SystemThemeUnknown},
	}
	for _, c := range cases {
		if got := systemThemeFromAppsUseLight(c.v); got != c.want {
			t.Errorf("AppsUseLightTheme=%d → %v, ждал %v", c.v, got, c.want)
		}
	}
}

func TestIsImmersiveColorSet(t *testing.T) {
	if !isImmersiveColorSet("ImmersiveColorSet") {
		t.Error("ImmersiveColorSet не опознан")
	}
	// Остальные области WM_SETTINGCHANGE тему не меняют — на них перечитывать
	// реестр незачем.
	for _, s := range []string{"", "intl", "Environment", "Policy", "immersivecolorset", "ImmersiveColorSet2"} {
		if isImmersiveColorSet(s) {
			t.Errorf("%q принято за смену цветовой схемы", s)
		}
	}
}
