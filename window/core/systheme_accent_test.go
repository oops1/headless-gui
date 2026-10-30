package core

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget"
)

// Акцентный цвет ОС (E12): поток событий окна (тема и акцент одним значением,
// независимость колбэков) и сборка DetectSystemAppearance. Разбор значений
// портала и реестра и чтение реестра и шины — в platform/shell
// (systheme_accent_test.go, systheme_portal_test.go,
// systheme_registry_test.go, systheme_windows_test.go,
// systheme_linux_test.go).

// appearanceTestWin — окно без ОС-окна: подписка отложена, колбэки зовутся напрямую.
func appearanceTestWin() (*Window, *postEngine) {
	eng := &postEngine{root: widget.NewWindow("T", 100, 100)}
	return New(eng, "T"), eng
}

var (
	accentTestBlue = color.RGBA{R: 0, G: 120, B: 212, A: 255}
	accentTestRed  = color.RGBA{R: 200, G: 30, B: 30, A: 255}
)

// Тема и акцент приходят приложению одним значением; зовут его только при
// перемене любого из полей.
func TestWindowSystemAppearanceEvent(t *testing.T) {
	win, eng := appearanceTestWin()
	var got []SystemAppearance
	win.SetOnSystemAppearanceChanged(func(a SystemAppearance) { got = append(got, a) })
	base := SystemAppearance{Theme: native.SystemThemeLight, Accent: accentTestBlue, HasAccent: true}
	win.sysTheme.edge.reset(base.Theme)
	win.sysTheme.cur, win.sysTheme.last = base, base

	win.systemAccentEvent(accentTestBlue, true)     // тот же акцент
	win.systemThemeEvent(native.SystemThemeLight)   // та же тема
	win.systemAccentEvent(accentTestRed, true)      // смена акцента
	win.systemAccentEvent(accentTestRed, true)      // повтор
	win.systemThemeEvent(native.SystemThemeDark)    // смена темы
	win.systemThemeEvent(native.SystemThemeUnknown) // портал не ответил — не смена
	win.systemAccentEvent(color.RGBA{}, false)      // акцент неизвестен — не смена
	win.systemAccentEvent(accentTestBlue, true)     // обратно

	want := []SystemAppearance{
		{native.SystemThemeLight, accentTestRed, true},
		{native.SystemThemeDark, accentTestRed, true},
		{native.SystemThemeDark, accentTestBlue, true},
	}
	if len(got) != len(want) {
		t.Fatalf("колбэк получил %d событий, ждал %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("событие %d: %+v, ждал %+v", i, got[i], want[i])
		}
	}
	if eng.posts != 3 {
		t.Errorf("через Post ушло %d вызовов, ждал 3: колбэк обязан идти горутиной движка", eng.posts)
	}

	win.SetOnSystemAppearanceChanged(nil)
	win.systemAccentEvent(accentTestRed, true)
	if len(got) != 3 {
		t.Errorf("колбэк снят, но вызван: %d", len(got))
	}
}

// Первый известный акцент после «неизвестен» — перемена: приложение до этого
// рисовало цвет по умолчанию.
func TestWindowSystemAppearanceEvent_FirstAccent(t *testing.T) {
	win, _ := appearanceTestWin()
	var got []SystemAppearance
	win.SetOnSystemAppearanceChanged(func(a SystemAppearance) { got = append(got, a) })
	base := SystemAppearance{Theme: native.SystemThemeDark}
	win.sysTheme.cur, win.sysTheme.last = base, base

	win.systemAccentEvent(accentTestBlue, true)
	if len(got) != 1 || !got[0].HasAccent || got[0].Accent != accentTestBlue || got[0].Theme != native.SystemThemeDark {
		t.Errorf("получили %+v", got)
	}
}

// Колбэк темы и колбэк оформления независимы: смена только акцента тему не
// будит, а смена темы будит оба.
func TestWindowThemeAndAppearanceCallbacksIndependent(t *testing.T) {
	win, _ := appearanceTestWin()
	var themes []native.SystemTheme
	var apps []SystemAppearance
	win.SetOnSystemThemeChanged(func(t native.SystemTheme) { themes = append(themes, t) })
	win.SetOnSystemAppearanceChanged(func(a SystemAppearance) { apps = append(apps, a) })
	base := SystemAppearance{Theme: native.SystemThemeLight, Accent: accentTestBlue, HasAccent: true}
	win.sysTheme.edge.reset(base.Theme)
	win.sysTheme.cur, win.sysTheme.last = base, base

	win.systemAccentEvent(accentTestRed, true)
	if len(themes) != 0 || len(apps) != 1 {
		t.Fatalf("смена акцента: тем %v, оформлений %v", themes, apps)
	}
	win.systemThemeEvent(native.SystemThemeDark)
	if len(themes) != 1 || themes[0] != native.SystemThemeDark || len(apps) != 2 {
		t.Errorf("смена темы: тем %v, оформлений %v", themes, apps)
	}

	// Снять один колбэк — второй работает.
	win.SetOnSystemThemeChanged(nil)
	win.systemThemeEvent(native.SystemThemeLight)
	if len(themes) != 1 || len(apps) != 3 {
		t.Errorf("после снятия колбэка темы: тем %v, оформлений %v", themes, apps)
	}
}

// Оформление без колбэка темы идёт по своему счёту «смены»: SetOnSystemThemeChanged
// задавать не нужно.
func TestWindowAppearanceWithoutThemeCallback(t *testing.T) {
	win, _ := appearanceTestWin()
	var apps []SystemAppearance
	win.SetOnSystemAppearanceChanged(func(a SystemAppearance) { apps = append(apps, a) })
	base := SystemAppearance{Theme: native.SystemThemeLight}
	win.sysTheme.edge.reset(base.Theme)
	win.sysTheme.cur, win.sysTheme.last = base, base
	win.systemThemeEvent(native.SystemThemeDark)
	win.systemThemeEvent(native.SystemThemeDark)
	if len(apps) != 1 || apps[0].Theme != native.SystemThemeDark {
		t.Errorf("получили %+v", apps)
	}
}

// DetectSystemAppearance собирает то же, что DetectSystemTheme и
// DetectSystemAccent отдают по отдельности. Службы оболочки подменены:
// настоящие проверяет platform/shell.
func TestDetectSystemAppearance_Consistent(t *testing.T) {
	t.Cleanup(native.RegisterShell(native.Shell{
		Theme:  func() native.SystemTheme { return native.SystemThemeDark },
		Accent: func() (color.RGBA, bool) { return color.RGBA{R: 10, G: 20, B: 30, A: 255}, true },
	}))
	a := DetectSystemAppearance()
	if a.Theme != native.SystemThemeDark || !a.HasAccent {
		t.Fatalf("службы оболочки не опрошены: %+v", a)
	}
	if a.Theme != DetectSystemTheme() {
		t.Errorf("тема в оформлении %v, а DetectSystemTheme %v", a.Theme, DetectSystemTheme())
	}
	c, ok := DetectSystemAccent()
	if a.HasAccent != ok || (ok && a.Accent != c) {
		t.Errorf("акцент в оформлении %+v/%v, а DetectSystemAccent %v/%v", a.Accent, a.HasAccent, c, ok)
	}
	if ok && c.A != 255 {
		t.Errorf("акцент прозрачный: %v", c)
	}
}
