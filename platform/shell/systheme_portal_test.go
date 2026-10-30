package shell

import (
	"testing"

	"github.com/oops1/headless-gui/v3/platform/dbus"
	"github.com/oops1/headless-gui/v3/platform/native"
)

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
