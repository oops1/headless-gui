package window

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Акцентный цвет ОС (E12): разбор значений реестра и портала на любой машине и
// поток событий окна. Чтение реестра и шины — в systheme_windows_test.go и
// systheme_linux_test.go.

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

func accentTestVal(r, g, b float64) dbusVariant {
	return dbusVariant{Sig: "(ddd)", Val: dbusStruct{Fields: []any{r, g, b}}}
}

func TestAccentFromPortalValue(t *testing.T) {
	cases := []struct {
		name   string
		v      any
		want   color.RGBA
		wantOK bool
	}{
		{"синий GNOME", accentTestVal(0.2078, 0.5176, 0.8941), color.RGBA{R: 53, G: 132, B: 228, A: 255}, true},
		{"чёрный — это цвет", accentTestVal(0, 0, 0), color.RGBA{A: 255}, true},
		{"белый", accentTestVal(1, 1, 1), color.RGBA{R: 255, G: 255, B: 255, A: 255}, true},
		// Старый Read: variant в variant.
		{"в двух variant", dbusVariant{Sig: "v", Val: accentTestVal(1, 0, 0)}, color.RGBA{R: 255, A: 255}, true},
		{"структура без обёртки", dbusStruct{Fields: []any{0.0, 1.0, 0.0}}, color.RGBA{G: 255, A: 255}, true},
		{"срез", []any{0.0, 0.0, 1.0}, color.RGBA{B: 255, A: 255}, true},
		// Портал сообщает «акцента нет» цветом вне [0, 1].
		{"нет акцента (-1,-1,-1)", accentTestVal(-1, -1, -1), color.RGBA{}, false},
		{"больше единицы", accentTestVal(0.5, 2, 0.5), color.RGBA{}, false},
		{"два поля", dbusVariant{Val: dbusStruct{Fields: []any{0.1, 0.2}}}, color.RGBA{}, false},
		{"целые вместо double", dbusStruct{Fields: []any{1, 0, 0}}, color.RGBA{}, false},
		{"строка", "blue", color.RGBA{}, false},
		{"nil", nil, color.RGBA{}, false},
	}
	for _, c := range cases {
		got, ok := accentFromPortalValue(c.v)
		if ok != c.wantOK || got != c.want {
			t.Errorf("%s: %v, %v; ждал %v, %v", c.name, got, ok, c.want, c.wantOK)
		}
	}
}

func TestAccentFromPortalSignal(t *testing.T) {
	sig := func(ns, key string, val any) *dbusMessage {
		return &dbusMessage{
			Type: dbusTypeSignal, Path: portalObjPath,
			Interface: portalSettingsIface, Member: "SettingChanged",
			Sig:  "ssv",
			Body: []any{ns, key, val},
		}
	}
	c, ok, matched := accentFromPortalSignal(sig(portalAppearanceNS, portalAccentKey, accentTestVal(1, 0, 0)))
	if !matched || !ok || c != (color.RGBA{R: 255, A: 255}) {
		t.Errorf("сигнал accent-color → %v, %v, %v", c, ok, matched)
	}
	// Акцент сбросили: сигнал наш, цвета нет — последнее значение у окна остаётся.
	if _, ok, matched := accentFromPortalSignal(sig(portalAppearanceNS, portalAccentKey, accentTestVal(-1, -1, -1))); !matched || ok {
		t.Errorf("сброс акцента → ok=%v matched=%v, ждал false, true", ok, matched)
	}
	// Схема цветов — не акцент (и наоборот: тема акцентный сигнал не принимает).
	if _, _, matched := accentFromPortalSignal(sig(portalAppearanceNS, portalColorSchemeKey, uint32(1))); matched {
		t.Error("color-scheme принят за accent-color")
	}
	if _, matched := systemThemeFromPortalSignal(sig(portalAppearanceNS, portalAccentKey, accentTestVal(1, 0, 0))); matched {
		t.Error("accent-color принят за смену темы")
	}
	// Чужое пространство, чужой интерфейс, обрезанное тело, nil.
	if _, _, matched := accentFromPortalSignal(sig("org.gnome.desktop.interface", portalAccentKey, accentTestVal(1, 0, 0))); matched {
		t.Error("accent-color чужого пространства принят")
	}
	other := sig(portalAppearanceNS, portalAccentKey, accentTestVal(1, 0, 0))
	other.Interface = "org.freedesktop.Notifications"
	if _, _, matched := accentFromPortalSignal(other); matched {
		t.Error("сигнал чужого интерфейса принят")
	}
	short := sig(portalAppearanceNS, portalAccentKey, accentTestVal(1, 0, 0))
	short.Body = short.Body[:2]
	if _, _, matched := accentFromPortalSignal(short); matched {
		t.Error("сигнал без значения принят")
	}
	if _, _, matched := accentFromPortalSignal(nil); matched {
		t.Error("nil принят")
	}
}

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
	base := SystemAppearance{Theme: SystemThemeLight, Accent: accentTestBlue, HasAccent: true}
	win.sysTheme.edge.reset(base.Theme)
	win.sysTheme.cur, win.sysTheme.last = base, base

	win.systemAccentEvent(accentTestBlue, true)         // тот же акцент
	win.systemThemeEvent(SystemThemeLight)       // та же тема
	win.systemAccentEvent(accentTestRed, true)          // смена акцента
	win.systemAccentEvent(accentTestRed, true)          // повтор
	win.systemThemeEvent(SystemThemeDark)        // смена темы
	win.systemThemeEvent(SystemThemeUnknown)     // портал не ответил — не смена
	win.systemAccentEvent(color.RGBA{}, false)   // акцент неизвестен — не смена
	win.systemAccentEvent(accentTestBlue, true)         // обратно

	want := []SystemAppearance{
		{SystemThemeLight, accentTestRed, true},
		{SystemThemeDark, accentTestRed, true},
		{SystemThemeDark, accentTestBlue, true},
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
	base := SystemAppearance{Theme: SystemThemeDark}
	win.sysTheme.cur, win.sysTheme.last = base, base

	win.systemAccentEvent(accentTestBlue, true)
	if len(got) != 1 || !got[0].HasAccent || got[0].Accent != accentTestBlue || got[0].Theme != SystemThemeDark {
		t.Errorf("получили %+v", got)
	}
}

// Колбэк темы и колбэк оформления независимы: смена только акцента тему не
// будит, а смена темы будит оба.
func TestWindowThemeAndAppearanceCallbacksIndependent(t *testing.T) {
	win, _ := appearanceTestWin()
	var themes []SystemTheme
	var apps []SystemAppearance
	win.SetOnSystemThemeChanged(func(t SystemTheme) { themes = append(themes, t) })
	win.SetOnSystemAppearanceChanged(func(a SystemAppearance) { apps = append(apps, a) })
	base := SystemAppearance{Theme: SystemThemeLight, Accent: accentTestBlue, HasAccent: true}
	win.sysTheme.edge.reset(base.Theme)
	win.sysTheme.cur, win.sysTheme.last = base, base

	win.systemAccentEvent(accentTestRed, true)
	if len(themes) != 0 || len(apps) != 1 {
		t.Fatalf("смена акцента: тем %v, оформлений %v", themes, apps)
	}
	win.systemThemeEvent(SystemThemeDark)
	if len(themes) != 1 || themes[0] != SystemThemeDark || len(apps) != 2 {
		t.Errorf("смена темы: тем %v, оформлений %v", themes, apps)
	}

	// Снять один колбэк — второй работает.
	win.SetOnSystemThemeChanged(nil)
	win.systemThemeEvent(SystemThemeLight)
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
	base := SystemAppearance{Theme: SystemThemeLight}
	win.sysTheme.edge.reset(base.Theme)
	win.sysTheme.cur, win.sysTheme.last = base, base
	win.systemThemeEvent(SystemThemeDark)
	win.systemThemeEvent(SystemThemeDark)
	if len(apps) != 1 || apps[0].Theme != SystemThemeDark {
		t.Errorf("получили %+v", apps)
	}
}

func TestDetectSystemAppearance_Consistent(t *testing.T) {
	a := DetectSystemAppearance()
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
