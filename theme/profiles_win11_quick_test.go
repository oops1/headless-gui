package theme_test

import (
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
)

// Windows 11 называет презентер быстрых настроек; остальные профили — нет, и
// их панель остаётся прежней. Тёмная разновидность наследует презентер.
func TestQuickSettingsWin11_PresenterOnlyForWindows11(t *testing.T) {
	for _, name := range allProfiles {
		m := builtinManager(t, name)
		got := m.Active().PresenterName("quicksettings")
		want := ""
		if name == theme.ProfileWindows11 || name == theme.ProfileWindows11Dark {
			want = theme.QuickSettingsPresenter
		}
		if got != want {
			t.Errorf("%s: презентер быстрых настроек %q, ждали %q", name, got, want)
		}
	}
}

// Размеры по таблице плана: ширина 360, плитки 96×48, скругление плитки 4,
// панели 8, всё через метрики.
func TestQuickSettingsWin11_Metrics(t *testing.T) {
	for _, name := range []string{theme.ProfileWindows11, theme.ProfileWindows11Dark} {
		m := builtinManager(t, name)
		for k, want := range map[theme.Key]float64{
			"quicksettings.w11.width":   360,
			"quicksettings.w11.tile.w":  96,
			"quicksettings.w11.tile.h":  48,
			"quicksettings.w11.columns": 3,
		} {
			if got := m.GetMetric(k); got != want {
				t.Errorf("%s: %s = %v, ждали %v", name, k, got, want)
			}
		}
		if c := m.GetStyle("quicksettings", "w11.tile", theme.StateNormal).Corner; c != 4 {
			t.Errorf("%s: скругление плитки %v, ждали 4", name, c)
		}
		if c := m.GetStyle("quicksettings", "", theme.StateNormal).Corner; c != 8 {
			t.Errorf("%s: скругление панели %v, ждали 8", name, c)
		}
		if !m.GetFlag(theme.KeyQuickIconTint, false) {
			t.Errorf("%s: значки плиток по умолчанию должны перекрашиваться", name)
		}
	}
	// Анимация перехода объявлена, и «меньше движения» делает её мгновенной.
	m := builtinManager(t, theme.ProfileWindows11)
	if a := m.GetAnimation(theme.KeyQuickPage); a.Duration <= 0 {
		t.Fatalf("переход страниц не анимирован: %+v", a)
	}
	m.SetFlag(theme.FlagMotionReduce, true)
	if a := m.GetAnimation(theme.KeyQuickPage); a.Duration != 0 {
		t.Errorf("при «меньше движения» переход идёт %v", a.Duration)
	}
}

// Плитки светлой и тёмной тем различаются без собственных токенов тёмной:
// акцентная плитка заливается акцентом, серая — накладкой, видной на обеих.
func TestQuickSettingsWin11_TileStates(t *testing.T) {
	for _, name := range []string{theme.ProfileWindows11, theme.ProfileWindows11Dark} {
		m := builtinManager(t, name)
		accent, _ := m.Active().Color(theme.KeyAccent)
		on := m.GetStyle("quicksettings", "w11.tile.on", theme.StateNormal)
		if on.Fill != accent {
			t.Errorf("%s: включённая плитка %v, ждали акцент %v", name, on.Fill, accent)
		}
		hover := m.GetStyle("quicksettings", "w11.tile.on", theme.StateHover)
		if hover.Fill == on.Fill {
			t.Errorf("%s: наведение на включённую плитку не меняет заливку", name)
		}
		off := m.GetStyle("quicksettings", "w11.tile", theme.StateNormal)
		offHover := m.GetStyle("quicksettings", "w11.tile", theme.StateHover)
		if off.Fill.A == 0 || offHover.Fill.A <= off.Fill.A {
			t.Errorf("%s: выключенная плитка %v, под курсором %v — ждали накладку, густеющую при наведении", name, off.Fill, offHover.Fill)
		}
		dis := m.GetStyle("quicksettings", "w11.tile", theme.StateDisabled)
		if dis.Text.A == 0 || dis.Text == off.Text {
			t.Errorf("%s: у отключённой плитки текст %v не приглушён", name, dis.Text)
		}
		// Ни одна часть панели не несёт тени и материала панели.
		for _, part := range []string{"w11.tile", "w11.tile.on", "w11.button", "w11.slider.thumb", "w11.footer", "w11.label"} {
			s := m.GetStyle("quicksettings", part, theme.StateNormal)
			if s.Elevation != 0 || s.ShadowBlur != 0 || s.Backdrop.Material != theme.MaterialDefault {
				t.Errorf("%s: часть %s унаследовала тень или материал панели", name, part)
			}
		}
	}
	// Заливка бегунка идёт от токена поверхности: тёмная тема меняет токен.
	l := builtinManager(t, theme.ProfileWindows11).GetStyle("quicksettings", "w11.slider.thumb", theme.StateNormal).Fill
	d := builtinManager(t, theme.ProfileWindows11Dark).GetStyle("quicksettings", "w11.slider.thumb", theme.StateNormal).Fill
	if l == d {
		t.Errorf("бегунок светлой и тёмной тем одного цвета %v", l)
	}
}

// Части быстрых настроек Windows 11 не получают ни материал Mica, ни мягкую
// тень панели, когда флаги подняты.
func TestQuickSettingsWin11_PartsKeepOutOfMaterial(t *testing.T) {
	m := builtinManager(t, theme.ProfileWindows11)
	m.SetFlag(theme.FlagBackdropMica, true)
	m.SetFlag(theme.FlagShadowSoft, true)
	for _, part := range []string{"w11.tile", "w11.tile.on", "w11.tile.chevron", "w11.button", "w11.done",
		"w11.slider.track", "w11.slider.thumb", "w11.footer", "w11.scrollbar", "w11.title"} {
		for _, st := range []theme.State{theme.StateNormal, theme.StateHover, theme.StatePressed} {
			s := m.GetStyle("quicksettings", part, st)
			if s.Backdrop.Material != theme.MaterialDefault || s.ShadowBlur != 0 {
				t.Errorf("%s/%v унаследовала материал %v или тень %v", part, st, s.Backdrop.Material, s.ShadowBlur)
			}
		}
	}
	if s := m.GetStyle("quicksettings", "", theme.StateNormal); s.Backdrop.Material != theme.MaterialMica {
		t.Errorf("сама панель без Mica: %v", s.Backdrop.Material)
	}
}
