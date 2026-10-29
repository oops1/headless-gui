package core

import (
	"testing"

	"github.com/oops1/headless-gui/v3/platform/dbus"
	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/widget"
)

// Тему ОС каждое приложение читало у себя, и каждое по-своему: кто-то брал
// SystemUsesLightTheme вместо AppsUseLightTheme, кто-то считал любое
// непонятное значение светлой темой. Ошибки здесь стоят белого окна на тёмной
// системе, поэтому разбор значений закреплён тестами на любой машине.

func TestSystemTheme_String(t *testing.T) {
	cases := map[native.SystemTheme]string{
		native.SystemThemeUnknown: "unknown",
		native.SystemThemeLight:   "light",
		native.SystemThemeDark:    "dark",
		native.SystemTheme(42):    "unknown",
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
	var th native.SystemTheme
	if th != native.SystemThemeUnknown {
		t.Errorf("нулевая тема %v, ждал unknown", th)
	}
}

func TestSystemThemeFromPortalValue(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want native.SystemTheme
	}{
		{"тёмная", uint32(1), native.SystemThemeDark},
		{"светлая", uint32(2), native.SystemThemeLight},
		{"нет предпочтения", uint32(0), native.SystemThemeUnknown},
		{"значение из будущего", uint32(3), native.SystemThemeUnknown},
		// ReadOne: один variant.
		{"в variant", dbus.Variant{Sig: "u", Val: uint32(1)}, native.SystemThemeDark},
		// Старый Read: variant в variant.
		{"в двух variant", dbus.Variant{Sig: "v", Val: dbus.Variant{Sig: "u", Val: uint32(2)}}, native.SystemThemeLight},
		// Реализации портала, отдающие другой целый тип, встречаются; значение
		// от этого не меняется.
		{"int32", int32(1), native.SystemThemeDark},
		{"байт", byte(2), native.SystemThemeLight},
		{"отрицательное", int32(-1), native.SystemThemeUnknown},
		{"строка", "dark", native.SystemThemeUnknown},
		{"nil", nil, native.SystemThemeUnknown},
		{"пустой variant", dbus.Variant{}, native.SystemThemeUnknown},
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
		want native.SystemTheme
	}{
		{"Adwaita:dark", native.SystemThemeDark},
		{"Adwaita:light", native.SystemThemeLight},
		{"  Adwaita:Dark  ", native.SystemThemeDark},
		{"Adwaita-dark", native.SystemThemeDark},
		{"Yaru-dark", native.SystemThemeDark},
		{"Materia-Dark-Compact", native.SystemThemeDark},
		// Вариант важнее имени: «dark» в названии набора не делает тёмным
		// вариант, явно названный светлым.
		{"Darkroom:light", native.SystemThemeLight},
		// Имя без подсказки ничего не говорит: Adwaita бывает любой, а
		// тёмный Nordic не содержит слова dark.
		{"Adwaita", native.SystemThemeUnknown},
		{"Nordic", native.SystemThemeUnknown},
		{"", native.SystemThemeUnknown},
		{"   ", native.SystemThemeUnknown},
		{"Adwaita:hc", native.SystemThemeUnknown},
	}
	for _, c := range cases {
		if got := systemThemeFromGTKTheme(c.env); got != c.want {
			t.Errorf("GTK_THEME=%q → %v, ждал %v", c.env, got, c.want)
		}
	}
}

func TestSystemThemeFromPortalSignal(t *testing.T) {
	sig := func(ns, key string, val any) *dbus.Message {
		return &dbus.Message{
			Type: dbus.TypeSignal, Path: portalObjPath,
			Interface: portalSettingsIface, Member: "SettingChanged",
			Sig:  "ssv",
			Body: []any{ns, key, val},
		}
	}
	dark := dbus.Variant{Sig: "u", Val: uint32(1)}

	if th, ok := systemThemeFromPortalSignal(sig(portalAppearanceNS, portalColorSchemeKey, dark)); !ok || th != native.SystemThemeDark {
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
	e.reset(native.SystemThemeLight)

	steps := []struct {
		in   native.SystemTheme
		want bool
	}{
		{native.SystemThemeLight, false},   // повтор прежней
		{native.SystemThemeDark, true},     // настоящая смена
		{native.SystemThemeDark, false},    // то же сообщение ещё раз
		{native.SystemThemeUnknown, false}, // портал не ответил — не смена
		{native.SystemThemeDark, false},    // и «неизвестно» не сбросило запомненную
		{native.SystemThemeLight, true},    // обратно
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
	if !e.update(native.SystemThemeDark) {
		t.Error("первая известная тема после unknown не стала сменой")
	}
}

// Колбэк приложения должен ставиться в очередь движка и звать только на
// настоящую смену; nil снимает подписку.
func TestWindowSystemThemeEvent(t *testing.T) {
	eng := &postEngine{root: widget.NewWindow("T", 100, 100)}
	win := New(eng, "T")

	var got []native.SystemTheme
	// Окна ОС ещё нет: подписка отложена до Run, а колбэк уже сохранён.
	win.SetOnSystemThemeChanged(func(t native.SystemTheme) { got = append(got, t) })
	win.sysTheme.edge.reset(native.SystemThemeLight)

	win.systemThemeEvent(native.SystemThemeLight)
	win.systemThemeEvent(native.SystemThemeDark)
	win.systemThemeEvent(native.SystemThemeDark)
	win.systemThemeEvent(native.SystemThemeUnknown)
	win.systemThemeEvent(native.SystemThemeLight)

	if len(got) != 2 || got[0] != native.SystemThemeDark || got[1] != native.SystemThemeLight {
		t.Errorf("колбэк получил %v, ждал [dark light]", got)
	}
	if eng.posts != 2 {
		t.Errorf("через Post ушло %d вызовов, ждал 2: колбэк обязан идти горутиной движка", eng.posts)
	}

	win.SetOnSystemThemeChanged(nil)
	win.systemThemeEvent(native.SystemThemeDark)
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
