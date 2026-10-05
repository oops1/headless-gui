// Подпиксельный текст следует профилю: Windows 10 просит его флагом
// theme.FlagTextSubpixel, остальные темы остаются на целочисленном шаге глифов,
// а явный Engine.SetTextSubpixel приложения главнее темы.
package tests

import (
	"strings"
	"testing"

	hgfonts "github.com/oops1/headless-gui/v3/assets/fonts"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

func subpixelEngine(t *testing.T) *engine.Engine {
	t.Helper()
	resetDefaultFont(t)
	eng := engine.New(200, 100, 30)
	if err := hgfonts.Register(eng); err != nil {
		t.Fatal(err)
	}
	eng.SetDefaultFont("OpenSans")
	return eng
}

// Профиль Windows 10 включает режим, профиль без флага выключает то, что
// включила тема; ширина строк при этом меняется и возвращается.
func TestTextSubpixel_FollowsProfile(t *testing.T) {
	eng := subpixelEngine(t)
	m := fontTestManager(t)
	str := strings.Repeat("i", 40)

	if err := eng.SetThemeProfile(m, theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	if eng.TextSubpixel() {
		t.Fatal("Windows 11 включила подпиксельный текст")
	}
	hinted := widget.MeasureUIText(str, 8.5)

	if err := eng.SetThemeProfile(m, theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	if !eng.TextSubpixel() {
		t.Fatal("профиль Windows 10 с флагом text.subpixel не включил подпиксельный текст")
	}
	if sub := widget.MeasureUIText(str, 8.5); sub == hinted {
		t.Errorf("ширина строки не изменилась после включения режима (%d): замеры не сброшены", sub)
	}

	if err := eng.SetThemeProfile(m, theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	if eng.TextSubpixel() {
		t.Error("после смены на профиль без флага режим остался включённым")
	}
	if back := widget.MeasureUIText(str, 8.5); back != hinted {
		t.Errorf("ширина после возврата %d, была %d", back, hinted)
	}
}

// Остальные темы режима не включают.
func TestTextSubpixel_OtherProfilesStayOff(t *testing.T) {
	eng := subpixelEngine(t)
	m := fontTestManager(t)
	for _, name := range []string{
		theme.ProfileWindows2000, theme.ProfileWindows11, theme.ProfileWindows11Dark,
		theme.ProfileMacOS, theme.ProfileMacOSDark,
	} {
		if err := eng.SetThemeProfile(m, name); err != nil {
			t.Fatal(err)
		}
		if eng.TextSubpixel() {
			t.Errorf("%s: подпиксельный текст включён", name)
		}
	}
}

// Флаг включается на лету любому профилю: Manager.SetFlag и ApplyThemeProfile.
func TestTextSubpixel_FlagOnTheFly(t *testing.T) {
	eng := subpixelEngine(t)
	m := fontTestManager(t)
	if err := eng.SetThemeProfile(m, theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	m.SetFlag(theme.FlagTextSubpixel, true)
	if err := eng.ApplyThemeProfile(m); err != nil {
		t.Fatal(err)
	}
	if !eng.TextSubpixel() {
		t.Fatal("флаг, включённый на лету, не дошёл до движка")
	}
	m.ResetFlag(theme.FlagTextSubpixel)
	if err := eng.ApplyThemeProfile(m); err != nil {
		t.Fatal(err)
	}
	if eng.TextSubpixel() {
		t.Error("после сброса флага режим остался включённым")
	}
}

// Явный вызов приложения главнее темы — в обе стороны, пока выбор не вернули.
func TestTextSubpixel_ExplicitWinsOverTheme(t *testing.T) {
	eng := subpixelEngine(t)
	m := fontTestManager(t)

	eng.SetTextSubpixel(false)
	if err := eng.SetThemeProfile(m, theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	if eng.TextSubpixel() {
		t.Error("тема включила режим, который приложение явно выключило")
	}

	eng.SetTextSubpixel(true)
	if err := eng.SetThemeProfile(m, theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	if !eng.TextSubpixel() {
		t.Error("тема выключила режим, который приложение явно включило")
	}

	// Выбор возвращён теме: режим становится таким, какого просит профиль.
	eng.UseThemeTextSubpixel()
	if eng.TextSubpixel() {
		t.Error("после UseThemeTextSubpixel режим не последовал Windows 11")
	}
	if err := eng.SetThemeProfile(m, theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	if !eng.TextSubpixel() {
		t.Error("после возврата выбора теме Windows 10 режим не включила")
	}
}

// Мост туда и обратно сохраняет просьбу.
func TestTextSubpixel_BridgeRoundTrip(t *testing.T) {
	m := fontTestManager(t)
	rt, ok := m.GetTheme(theme.ProfileWindows10)
	if !ok {
		t.Fatal("нет профиля Windows 10")
	}
	flat := widget.Materialize(rt)
	if !flat.Style.TextSubpixel {
		t.Fatal("Materialize потерял флаг text.subpixel")
	}
	back := widget.ProfileFromTheme(flat)
	if !back.Flags[theme.FlagTextSubpixel] {
		t.Error("ProfileFromTheme потерял просьбу о подпикселе")
	}
}
