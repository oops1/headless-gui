package theme_test

import (
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
)

// Градиент и метрики заголовка окна Windows 2000.

func manager2000(t *testing.T, name string) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(name); err != nil {
		t.Fatal(err)
	}
	return m
}

// Классика объявляет градиент заголовка: у активного окна тёмно-синий → голубой
// #A6CAF0, у неактивного серый → светло-серый. Первые точки — заливки стиля
// window/titlebar, вторые — плоские токены.
func TestWindows2000_TitleGradientDeclared(t *testing.T) {
	for _, name := range []string{theme.ProfileWindows2000, theme.ProfileWindows2000Blue} {
		th := manager2000(t, name).Active()
		if got, ok := th.Color(theme.KeyWindowTitleGradient2); !ok || got != theme.RGB(166, 202, 240) {
			t.Errorf("%s: вторая точка градиента активного окна %v (задан %v), ждали #A6CAF0", name, got, ok)
		}
		if got, ok := th.Color(theme.KeyWindowTitleGradient2Inactive); !ok || got != theme.RGB(192, 192, 192) {
			t.Errorf("%s: вторая точка градиента неактивного окна %v (задан %v), ждали #C0C0C0", name, got, ok)
		}
	}
	// Первая точка неактивного — серый #808080, активного — акцент.
	th := manager2000(t, theme.ProfileWindows2000).Active()
	if got := th.Style("window", "titlebar", theme.StateNormal).Fill; got != theme.RGB(128, 128, 128) {
		t.Errorf("неактивный заголовок начинается с %v, ждали #808080", got)
	}
	if got := th.Style("window", "titlebar", theme.StateFocused).Fill; got != theme.RGB(10, 36, 106) {
		t.Errorf("активный заголовок начинается с %v, ждали #0A246A", got)
	}
}

// После SetAccent вторая точка градиента идёт за светлым оттенком акцента, а
// ResetAccent возвращает классический голубой.
func TestWindows2000_TitleGradientFollowsAccent(t *testing.T) {
	green := theme.RGB(16, 124, 16)
	for _, name := range []string{theme.ProfileWindows2000, theme.ProfileWindows2000Blue} {
		m := manager2000(t, name)
		m.SetAccent(green)
		got, _ := m.Active().Color(theme.KeyWindowTitleGradient2)
		if want := theme.DeriveAccent(green).Light; got != want {
			t.Errorf("%s: градиент после SetAccent %v, ждали светлый акцент %v", name, got, want)
		}
		// Неактивное окно акцентом не красится: серый остаётся серым.
		if got, _ := m.Active().Color(theme.KeyWindowTitleGradient2Inactive); got != theme.RGB(192, 192, 192) {
			t.Errorf("%s: серый градиент неактивного окна после SetAccent стал %v", name, got)
		}
		m.ResetAccent()
		if got, _ := m.Active().Color(theme.KeyWindowTitleGradient2); got != theme.RGB(166, 202, 240) {
			t.Errorf("%s: ResetAccent не вернул голубой: %v", name, got)
		}
	}
}

// Остальные темы градиента заголовка не объявляют: заголовок плоский, как был.
func TestOtherProfiles_NoTitleGradientNoCaptionMetrics(t *testing.T) {
	for _, name := range []string{theme.ProfileWindows10, theme.ProfileWindows10Dark,
		theme.ProfileWindows11, theme.ProfileWindows11Dark, theme.ProfileMacOS, theme.ProfileMacOSDark} {
		th := manager2000(t, name).Active()
		for _, k := range []theme.Key{theme.KeyWindowTitleGradient2, theme.KeyWindowTitleGradient2Inactive} {
			if c, ok := th.Color(k); ok {
				t.Errorf("%s объявляет градиент заголовка %s=%v", name, k, c)
			}
		}
		for _, k := range []theme.Key{theme.KeyWindowTitleBarHeight, theme.KeyWindowCaptionButtonW,
			theme.KeyWindowCaptionButtonH, theme.KeyWindowCaptionIconSize} {
			if v, ok := th.Metric(k); ok {
				t.Errorf("%s объявляет метрику заголовка %s=%v", name, k, v)
			}
		}
	}
}

// Метрики заголовка Windows 2000: 18 px, кнопки 16×14, значок 16 — наследует и
// синяя разновидность.
func TestWindows2000_CaptionMetrics(t *testing.T) {
	for _, name := range []string{theme.ProfileWindows2000, theme.ProfileWindows2000Blue} {
		th := manager2000(t, name).Active()
		want := map[theme.Key]float64{
			theme.KeyWindowTitleBarHeight:  18,
			theme.KeyWindowCaptionButtonW:  16,
			theme.KeyWindowCaptionButtonH:  14,
			theme.KeyWindowCaptionIconSize: 16,
		}
		for k, v := range want {
			if got, ok := th.Metric(k); !ok || got != v {
				t.Errorf("%s: %s = %v (задан %v), ждали %v", name, k, got, ok, v)
			}
		}
	}
}

// SetColorFrom: без SetAccent объявленное значение сильнее ссылки, а
// необъявленный токен берётся по ссылке; с SetAccent ссылка сильнее всего;
// ссылка потомка перекрывает ссылку предка; источник, которого в теме нет,
// ссылку отменяет.
func TestProfile_SetColorFrom(t *testing.T) {
	const k theme.Key = "x.test"
	blue := theme.RGB(0, 0, 200)
	declared := theme.RGB(1, 2, 3)

	build := func(declare bool, from theme.Key) *theme.Manager {
		m := theme.NewManager()
		p := theme.NewProfile("p").SetColor(theme.KeyAccent, blue).SetColorFrom(k, from)
		if declare {
			p.SetColor(k, declared)
		}
		if err := m.RegisterTheme(p); err != nil {
			t.Fatal(err)
		}
		if err := m.SetTheme("p"); err != nil {
			t.Fatal(err)
		}
		return m
	}
	light := theme.DeriveAccent(blue).Light

	if got, _ := build(true, theme.KeyAccentLight).Active().Color(k); got != declared {
		t.Errorf("объявленное значение без SetAccent: %v, ждали %v", got, declared)
	}
	if got, _ := build(false, theme.KeyAccentLight).Active().Color(k); got != light {
		t.Errorf("необъявленный токен по ссылке: %v, ждали %v", got, light)
	}
	m := build(true, theme.KeyAccentLight)
	red := theme.RGB(200, 0, 0)
	m.SetAccent(red)
	if got, _ := m.Active().Color(k); got != theme.DeriveAccent(red).Light {
		t.Errorf("после SetAccent: %v, ждали %v", got, theme.DeriveAccent(red).Light)
	}
	if got, ok := build(true, "no.such.token").Active().Color(k); !ok || got != declared {
		t.Errorf("ссылка на несуществующий токен должна отменяться: %v (%v)", got, ok)
	}

	// Потомок перекрывает ссылку предка.
	m = theme.NewManager()
	parent := theme.NewProfile("parent").SetColor(theme.KeyAccent, blue).
		SetColor(k, declared).SetColorFrom(k, theme.KeyAccentLight)
	child := theme.NewProfile("child")
	child.Parent = "parent"
	child.SetColorFrom(k, theme.KeyAccentDark)
	for _, p := range []*theme.Profile{parent, child} {
		if err := m.RegisterTheme(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.SetTheme("child"); err != nil {
		t.Fatal(err)
	}
	m.SetAccent(red)
	if got, _ := m.Active().Color(k); got != theme.DeriveAccent(red).Dark {
		t.Errorf("ссылка потомка: %v, ждали %v", got, theme.DeriveAccent(red).Dark)
	}
}
