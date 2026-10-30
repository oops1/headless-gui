package shell

// systheme_portal.go — разбор того, что отдаёт портал настроек рабочего стола
// (org.freedesktop.portal.Settings) и переменная GTK_THEME. Файл без суффикса
// ОС: разбор проверяется на любой машине, сами вызовы — в systheme_linux.go.

import (
	"image/color"
	"math"
	"strings"

	"github.com/oops1/headless-gui/v3/platform/dbus"
	"github.com/oops1/headless-gui/v3/platform/native"
)

// Портал настроек рабочего стола (XDG Desktop Portal) и его настройка темы.
// Имена лежат здесь, а не в systheme_linux.go: по ним же разбирается
// пришедший сигнал, а разбор проверяется на любой машине.
const (
	portalBusName        = "org.freedesktop.portal.Desktop"
	portalObjPath        = "/org/freedesktop/portal/desktop"
	portalSettingsIface  = "org.freedesktop.portal.Settings"
	portalAppearanceNS   = "org.freedesktop.appearance"
	portalColorSchemeKey = "color-scheme"
	portalAccentKey      = "accent-color"
)

// Значения ключа color-scheme.
const (
	portalSchemeNoPreference = 0 // человек ничего не выбирал
	portalSchemeDark         = 1
	portalSchemeLight        = 2
)

// systemThemeFromPortalSignal разбирает сигнал SettingChanged(ssv) портала.
//
// Сигнал приходит на любую настройку любого пространства — акцентный цвет,
// контраст, свои ключи GNOME, — и шина доставляет их все, раз правило
// подписки одно на пространство. ok=false — сигнал не про цветовую схему, и
// будить приложение он не должен.
func systemThemeFromPortalSignal(msg *dbus.Message) (t native.SystemTheme, ok bool) {
	if msg == nil || msg.Interface != portalSettingsIface || msg.Member != "SettingChanged" || len(msg.Body) < 3 {
		return native.SystemThemeUnknown, false
	}
	ns, _ := msg.Body[0].(string)
	key, _ := msg.Body[1].(string)
	if ns != portalAppearanceNS || key != portalColorSchemeKey {
		return native.SystemThemeUnknown, false
	}
	return systemThemeFromPortalValue(msg.Body[2]), true
}

// systemThemeFromPortalValue разбирает значение настройки, как оно пришло по
// D-Bus.
//
// Читать значение можно двумя методами портала, и обёрнуто оно по-разному:
// ReadOne (версия 2) отдаёт один variant, а старый Read — variant внутри
// variant, о чём в документации сказано одной строкой, а на деле ломает
// разбор вида «uint32 или ошибка». Поэтому variant снимается циклом, сколько
// бы их ни было. То же значение приходит и в сигнале SettingChanged.
//
// 0 («нет предпочтения») — Unknown: человек ничего не выбирал, и тему решает
// приложение.
func systemThemeFromPortalValue(v any) native.SystemTheme {
	for {
		vr, ok := v.(dbus.Variant)
		if !ok {
			break
		}
		v = vr.Val
	}
	if v == nil {
		return native.SystemThemeUnknown
	}
	n, err := dbus.ToUint(v)
	if err != nil {
		return native.SystemThemeUnknown
	}
	switch n {
	case portalSchemeDark:
		return native.SystemThemeDark
	case portalSchemeLight:
		return native.SystemThemeLight
	}
	return native.SystemThemeUnknown
}

// systemThemeFromGTKTheme разбирает переменную GTK_THEME.
//
// Её задают, когда нужно запустить программу в чужой теме поверх настроек
// рабочего стола, — а значит, она важнее портала: GTK-приложение в такой
// сессии поступает так же. Формат «Имя» или «Имя:вариант», где вариант —
// dark или light (Adwaita:dark).
//
// Имя без слова dark и без варианта — Unknown, а не «светлая»: у стороннего
// набора (Nordic, Dracula) тёмная тема и без этой подсказки в названии, и по
// одному имени нельзя сказать наверняка. Лучше отдать решение порталу, чем
// угадать неверно.
func systemThemeFromGTKTheme(s string) native.SystemTheme {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return native.SystemThemeUnknown
	}
	name, variant, _ := strings.Cut(s, ":")
	switch strings.TrimSpace(variant) {
	case "dark":
		return native.SystemThemeDark
	case "light":
		return native.SystemThemeLight
	}
	if strings.Contains(name, "dark") {
		return native.SystemThemeDark
	}
	return native.SystemThemeUnknown
}

// accentFromPortalValue разбирает значение accent-color портала: структура из
// трёх double (красный, зелёный, синий в [0, 1]). Снимает обёртки variant так
// же, как systemThemeFromPortalValue.
//
// Портал сообщает «акцента нет» цветом вне диапазона [0, 1] (GNOME до выбора
// акцента отдаёт (-1, -1, -1)): это ok=false, а не чёрный цвет.
func accentFromPortalValue(v any) (color.RGBA, bool) {
	for {
		vr, ok := v.(dbus.Variant)
		if !ok {
			break
		}
		v = vr.Val
	}
	var f []any
	switch t := v.(type) {
	case dbus.Struct:
		f = t.Fields
	case *dbus.Struct:
		if t != nil {
			f = t.Fields
		}
	case []any:
		f = t
	}
	if len(f) != 3 {
		return color.RGBA{}, false
	}
	var c [3]uint8
	for i, e := range f {
		x, ok := e.(float64)
		if !ok || math.IsNaN(x) || x < 0 || x > 1 {
			return color.RGBA{}, false
		}
		c[i] = uint8(math.Round(x * 255))
	}
	return color.RGBA{R: c[0], G: c[1], B: c[2], A: 255}, true
}

// accentFromPortalSignal разбирает сигнал SettingChanged(ssv) портала.
// matched=false — сигнал не про accent-color, и будить приложение он не должен.
// matched=true, ok=false — пользователь сбросил акцент или значение
// непонятно: последнее известное значение при этом остаётся.
func accentFromPortalSignal(msg *dbus.Message) (c color.RGBA, ok, matched bool) {
	if msg == nil || msg.Interface != portalSettingsIface || msg.Member != "SettingChanged" || len(msg.Body) < 3 {
		return color.RGBA{}, false, false
	}
	ns, _ := msg.Body[0].(string)
	key, _ := msg.Body[1].(string)
	if ns != portalAppearanceNS || key != portalAccentKey {
		return color.RGBA{}, false, false
	}
	c, ok = accentFromPortalValue(msg.Body[2])
	return c, ok, true
}
