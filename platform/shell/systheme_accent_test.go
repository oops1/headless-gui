package shell

import (
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v3/platform/dbus"
)

func accentTestVal(r, g, b float64) dbus.Variant {
	return dbus.Variant{Sig: "(ddd)", Val: dbus.Struct{Fields: []any{r, g, b}}}
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
		{"в двух variant", dbus.Variant{Sig: "v", Val: accentTestVal(1, 0, 0)}, color.RGBA{R: 255, A: 255}, true},
		{"структура без обёртки", dbus.Struct{Fields: []any{0.0, 1.0, 0.0}}, color.RGBA{G: 255, A: 255}, true},
		{"срез", []any{0.0, 0.0, 1.0}, color.RGBA{B: 255, A: 255}, true},
		// Портал сообщает «акцента нет» цветом вне [0, 1].
		{"нет акцента (-1,-1,-1)", accentTestVal(-1, -1, -1), color.RGBA{}, false},
		{"больше единицы", accentTestVal(0.5, 2, 0.5), color.RGBA{}, false},
		{"два поля", dbus.Variant{Val: dbus.Struct{Fields: []any{0.1, 0.2}}}, color.RGBA{}, false},
		{"целые вместо double", dbus.Struct{Fields: []any{1, 0, 0}}, color.RGBA{}, false},
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
	sig := func(ns, key string, val any) *dbus.Message {
		return &dbus.Message{
			Type: dbus.TypeSignal, Path: portalObjPath,
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
