package window

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Тему ОС каждое приложение читало у себя, и каждое по-своему: кто-то брал
// SystemUsesLightTheme вместо AppsUseLightTheme, кто-то считал любое
// непонятное значение светлой темой. Ошибки здесь стоят белого окна на тёмной
// системе, поэтому разбор значений закреплён тестами на любой машине.

func TestSystemTheme_String(t *testing.T) {
	cases := map[SystemTheme]string{
		SystemThemeUnknown: "unknown",
		SystemThemeLight:   "light",
		SystemThemeDark:    "dark",
		SystemTheme(42):    "unknown",
	}
	for th, want := range cases {
		if got := th.String(); got != want {
			t.Errorf("SystemTheme(%d).String() = %q, ждал %q", int(th), got, want)
		}
	}
}

// Нулевое значение типа обязано быть «неизвестно»: переменная, до которой
// ответ ещё не дошёл, не должна выдавать себя за светлую или тёмную тему.
func TestSystemTheme_ZeroValueIsUnknown(t *testing.T) {
	var th SystemTheme
	if th != SystemThemeUnknown {
		t.Errorf("нулевая тема %v, ждал unknown", th)
	}
}

func TestSystemThemeFromAppsUseLight(t *testing.T) {
	cases := []struct {
		v    uint64
		want SystemTheme
	}{
		{0, SystemThemeDark},
		{1, SystemThemeLight},
		// Не 0 и не 1 — не «светлая по умолчанию»: что значит двойка, не
		// знает никто, и белое окно в тёмной теме хуже, чем свой выбор
		// приложения.
		{2, SystemThemeUnknown},
		{0xFFFFFFFF, SystemThemeUnknown},
	}
	for _, c := range cases {
		if got := systemThemeFromAppsUseLight(c.v); got != c.want {
			t.Errorf("AppsUseLightTheme=%d → %v, ждал %v", c.v, got, c.want)
		}
	}
}

func TestSystemThemeFromPortalValue(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want SystemTheme
	}{
		{"тёмная", uint32(1), SystemThemeDark},
		{"светлая", uint32(2), SystemThemeLight},
		{"нет предпочтения", uint32(0), SystemThemeUnknown},
		{"значение из будущего", uint32(3), SystemThemeUnknown},
		// ReadOne: один variant.
		{"в variant", dbusVariant{Sig: "u", Val: uint32(1)}, SystemThemeDark},
		// Старый Read: variant в variant.
		{"в двух variant", dbusVariant{Sig: "v", Val: dbusVariant{Sig: "u", Val: uint32(2)}}, SystemThemeLight},
		// Реализации портала, отдающие другой целый тип, встречаются; значение
		// от этого не меняется.
		{"int32", int32(1), SystemThemeDark},
		{"байт", byte(2), SystemThemeLight},
		{"отрицательное", int32(-1), SystemThemeUnknown},
		{"строка", "dark", SystemThemeUnknown},
		{"nil", nil, SystemThemeUnknown},
		{"пустой variant", dbusVariant{}, SystemThemeUnknown},
	}
	for _, c := range cases {
		if got := systemThemeFromPortalValue(c.v); got != c.want {
			t.Errorf("%s: %#v → %v, ждал %v", c.name, c.v, got, c.want)
		}
	}
}

func TestSystemThemeFromGTKTheme(t *testing.T) {
	cases := []struct {
		env  string
		want SystemTheme
	}{
		{"Adwaita:dark", SystemThemeDark},
		{"Adwaita:light", SystemThemeLight},
		{"  Adwaita:Dark  ", SystemThemeDark},
		{"Adwaita-dark", SystemThemeDark},
		{"Yaru-dark", SystemThemeDark},
		{"Materia-Dark-Compact", SystemThemeDark},
		// Вариант важнее имени: «dark» в названии набора не делает тёмным
		// вариант, явно названный светлым.
		{"Darkroom:light", SystemThemeLight},
		// Имя без подсказки ничего не говорит: Adwaita бывает любой, а
		// тёмный Nordic не содержит слова dark.
		{"Adwaita", SystemThemeUnknown},
		{"Nordic", SystemThemeUnknown},
		{"", SystemThemeUnknown},
		{"   ", SystemThemeUnknown},
		{"Adwaita:hc", SystemThemeUnknown},
	}
	for _, c := range cases {
		if got := systemThemeFromGTKTheme(c.env); got != c.want {
			t.Errorf("GTK_THEME=%q → %v, ждал %v", c.env, got, c.want)
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

func TestSystemThemeFromPortalSignal(t *testing.T) {
	sig := func(ns, key string, val any) *dbusMessage {
		return &dbusMessage{
			Type: dbusTypeSignal, Path: portalObjPath,
			Interface: portalSettingsIface, Member: "SettingChanged",
			Sig:  "ssv",
			Body: []any{ns, key, val},
		}
	}
	dark := dbusVariant{Sig: "u", Val: uint32(1)}

	if th, ok := systemThemeFromPortalSignal(sig(portalAppearanceNS, portalColorSchemeKey, dark)); !ok || th != SystemThemeDark {
		t.Errorf("сигнал color-scheme → %v, %v; ждал dark, true", th, ok)
	}

	// Акцентный цвет приходит тем же сигналом и тему не меняет.
	if _, ok := systemThemeFromPortalSignal(sig(portalAppearanceNS, "accent-color", dark)); ok {
		t.Error("смена акцентного цвета принята за смену темы")
	}
	// Тот же ключ в чужом пространстве — не наш.
	if _, ok := systemThemeFromPortalSignal(sig("org.gnome.desktop.interface", portalColorSchemeKey, dark)); ok {
		t.Error("color-scheme чужого пространства принят за наш")
	}

	// Чужой интерфейс и обрезанное тело — не повод падать.
	other := sig(portalAppearanceNS, portalColorSchemeKey, dark)
	other.Interface = "org.freedesktop.Notifications"
	if _, ok := systemThemeFromPortalSignal(other); ok {
		t.Error("сигнал чужого интерфейса принят")
	}
	short := sig(portalAppearanceNS, portalColorSchemeKey, dark)
	short.Body = short.Body[:2]
	if _, ok := systemThemeFromPortalSignal(short); ok {
		t.Error("сигнал без значения принят")
	}
	if _, ok := systemThemeFromPortalSignal(nil); ok {
		t.Error("nil принят")
	}
}

// Windows на одно переключение шлёт сообщение несколько раз, портал — сигнал
// и на акцентный цвет: без отсева приложение перекрашивало бы всё дерево
// каждый раз.
func TestThemeEdge(t *testing.T) {
	var e themeEdge
	e.reset(SystemThemeLight)

	steps := []struct {
		in   SystemTheme
		want bool
	}{
		{SystemThemeLight, false},   // повтор прежней
		{SystemThemeDark, true},     // настоящая смена
		{SystemThemeDark, false},    // то же сообщение ещё раз
		{SystemThemeUnknown, false}, // портал не ответил — не смена
		{SystemThemeDark, false},    // и «неизвестно» не сбросило запомненную
		{SystemThemeLight, true},    // обратно
	}
	for i, s := range steps {
		if got := e.update(s.in); got != s.want {
			t.Errorf("шаг %d: update(%v) = %v, ждал %v", i, s.in, got, s.want)
		}
	}
}

// Если исходная тема была неизвестна, первое же определённое значение — смена:
// приложение до этого рисовало свою тему по умолчанию.
func TestThemeEdge_FromUnknown(t *testing.T) {
	var e themeEdge
	if !e.update(SystemThemeDark) {
		t.Error("первая известная тема после unknown не стала сменой")
	}
}

// Колбэк приложения должен ставиться в очередь движка и звать только на
// настоящую смену; nil снимает подписку.
func TestWindowSystemThemeEvent(t *testing.T) {
	eng := &postEngine{root: widget.NewWindow("T", 100, 100)}
	win := New(eng, "T")

	var got []SystemTheme
	// Окна ОС ещё нет: подписка отложена до Run, а колбэк уже сохранён.
	win.SetOnSystemThemeChanged(func(t SystemTheme) { got = append(got, t) })
	win.sysTheme.edge.reset(SystemThemeLight)

	win.systemThemeEvent(SystemThemeLight)
	win.systemThemeEvent(SystemThemeDark)
	win.systemThemeEvent(SystemThemeDark)
	win.systemThemeEvent(SystemThemeUnknown)
	win.systemThemeEvent(SystemThemeLight)

	if len(got) != 2 || got[0] != SystemThemeDark || got[1] != SystemThemeLight {
		t.Errorf("колбэк получил %v, ждал [dark light]", got)
	}
	if eng.posts != 2 {
		t.Errorf("через Post ушло %d вызовов, ждал 2: колбэк обязан идти горутиной движка", eng.posts)
	}

	win.SetOnSystemThemeChanged(nil)
	win.systemThemeEvent(SystemThemeDark)
	if len(got) != 2 {
		t.Errorf("колбэк снят, но вызван: %v", got)
	}
}

// Закрытие окна без подписки и повторное снятие подписки не должны падать.
func TestWindowStopSystemThemeWatch_Idempotent(t *testing.T) {
	win := New(&postEngine{root: widget.NewWindow("T", 100, 100)}, "T")
	win.stopSystemThemeWatch()
	win.stopSystemThemeWatch()
}
