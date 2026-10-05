// Кегль по умолчанию следует теме: профиль Windows 10 просит 8,5 pt для всех
// виджетов (заголовки окон, вкладки, меню), остальные темы остаются на 10 pt.
package tests

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

func fontTestManager(t *testing.T) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	return m
}

// resetDefaultFont возвращает глобальные настройки, которые трогают тесты.
func resetDefaultFont(t *testing.T) {
	t.Helper()
	widget.SetDefaultFontSize(0)
	t.Cleanup(func() {
		widget.SetDefaultFontSize(0)
		widget.ApplyGlobalTheme(widget.DarkTheme())
	})
}

// labelWidth — ширина подписи без своего кегля: она следует кеглю по умолчанию.
func labelWidth(text string) int {
	w, _ := widget.NewLabel(text, color.RGBA{A: 255}).DesiredSize()
	return w
}

func TestDefaultFont_NoProfileStaysTenPoints(t *testing.T) {
	resetDefaultFont(t)
	if got := widget.DefaultFontSize(); got != widget.DefaultFontSizePt {
		t.Fatalf("без профиля кегль %v, ждали %v", got, widget.DefaultFontSizePt)
	}
}

func TestDefaultFont_Windows10ProfileFollowsFontsDefault(t *testing.T) {
	resetDefaultFont(t)
	eng := engine.New(200, 100, 30)
	m := fontTestManager(t)

	before := labelWidth("Заголовок окна приложения")
	if err := eng.SetThemeProfile(m, theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	if got := widget.DefaultFontSize(); got != 8.5 {
		t.Fatalf("кегль после профиля Windows 10 = %v, ждали 8.5 (Fonts[default])", got)
	}
	after := labelWidth("Заголовок окна приложения")
	if after >= before {
		t.Errorf("подпись без своего кегля не сузилась: было %d, стало %d", before, after)
	}

	// Другая тема возвращает прежние десять пунктов.
	if err := eng.SetThemeProfile(m, theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	if got := widget.DefaultFontSize(); got != widget.DefaultFontSizePt {
		t.Errorf("после Windows 11 кегль %v, ждали возврата к %v", got, widget.DefaultFontSizePt)
	}
	if again := labelWidth("Заголовок окна приложения"); again != before {
		t.Errorf("ширина после возврата %d, была %d", again, before)
	}
}

// Windows 11, Windows 2000 и macOS объявляют Fonts[default] = 9 pt, но флага
// не ставят: применение профиля не должно менять раскладку каждого окна.
func TestDefaultFont_OtherProfilesDoNotChange(t *testing.T) {
	resetDefaultFont(t)
	eng := engine.New(200, 100, 30)
	m := fontTestManager(t)
	for _, name := range []string{
		theme.ProfileWindows2000, theme.ProfileWindows11, theme.ProfileWindows11Dark,
		theme.ProfileMacOS, theme.ProfileMacOSDark,
	} {
		if err := eng.SetThemeProfile(m, name); err != nil {
			t.Fatal(err)
		}
		if got := widget.DefaultFontSize(); got != widget.DefaultFontSizePt {
			t.Errorf("%s: кегль %v, ждали %v", name, got, widget.DefaultFontSizePt)
		}
	}
}

// Флаг можно включить на лету любому профилю: Manager.SetFlag и
// ApplyThemeProfile.
func TestDefaultFont_FlagOnTheFly(t *testing.T) {
	resetDefaultFont(t)
	eng := engine.New(200, 100, 30)
	m := fontTestManager(t)
	if err := eng.SetThemeProfile(m, theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	m.SetFlag(theme.FlagFontDefaultGlobal, true)
	if err := eng.ApplyThemeProfile(m); err != nil {
		t.Fatal(err)
	}
	if got := widget.DefaultFontSize(); got != 9 {
		t.Errorf("кегль = %v, ждали 9 (Fonts[default] Windows 11)", got)
	}
	m.ResetFlag(theme.FlagFontDefaultGlobal)
	if err := eng.ApplyThemeProfile(m); err != nil {
		t.Fatal(err)
	}
	if got := widget.DefaultFontSize(); got != widget.DefaultFontSizePt {
		t.Errorf("после сброса флага кегль %v", got)
	}
}

// Размер, назначенный приложением явно, тема не трогает.
func TestDefaultFont_ExplicitWinsOverTheme(t *testing.T) {
	resetDefaultFont(t)
	eng := engine.New(200, 100, 30)
	m := fontTestManager(t)
	widget.SetDefaultFontSize(12)
	if err := eng.SetThemeProfile(m, theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	if got := widget.DefaultFontSize(); got != 12 {
		t.Errorf("явный кегль вытеснен темой: %v", got)
	}
	if err := eng.SetThemeProfile(m, theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	if got := widget.DefaultFontSize(); got != 12 {
		t.Errorf("явный кегль снят сменой темы: %v", got)
	}
}

// Мост туда и обратно сохраняет просьбу о кегле.
func TestDefaultFont_BridgeRoundTrip(t *testing.T) {
	src := widget.DarkTheme()
	src.Style.DefaultFontSize = 8.5
	got := roundTrip(t, src)
	if got.Style.DefaultFontSize != 8.5 {
		t.Errorf("кегль потерян в мосте: %v", got.Style.DefaultFontSize)
	}
	src.Style.DefaultFontSize = 0
	if got := roundTrip(t, src); got.Style.DefaultFontSize != 0 {
		t.Errorf("кегль появился из ничего: %v", got.Style.DefaultFontSize)
	}
}
