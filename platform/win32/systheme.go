package win32

import (
	"image/color"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// systemThemeFromAppsUseLight переводит значение AppsUseLightTheme из реестра
// Windows в тему: 0 — тёмная, 1 — светлая.
//
// Любое другое число — Unknown, а не «светлая»: значение не документировано
// шире нуля и единицы, и угадывать за систему, что значит двойка, значит
// рисковать белым окном в тёмной теме.
func systemThemeFromAppsUseLight(v uint64) native.SystemTheme {
	switch v {
	case 0:
		return native.SystemThemeDark
	case 1:
		return native.SystemThemeLight
	}
	return native.SystemThemeUnknown
}

// isImmersiveColorSet — это сообщение WM_SETTINGCHANGE про смену цветовой
// схемы?
//
// Сообщение приходит по любому поводу (смена раскладки, переменных среды,
// рабочей области), и различает их только строка в lParam. Нужная нам строка —
// ImmersiveColorSet: так Windows называет группу параметров, куда входят и
// тема приложений, и акцентный цвет.
func isImmersiveColorSet(s string) bool {
	return s == "ImmersiveColorSet"
}

// accentFromDWMAccent переводит значение DWM\AccentColor из реестра Windows в
// цвет. Значение — DWORD в порядке байт ABGR (0xAABBGGRR), в отличие от
// привычного ARGB: красный лежит в младшем байте. Альфа в нём почти всегда
// 0xFF и ничего не значит: акцент непрозрачен.
func accentFromDWMAccent(v uint32) color.RGBA {
	return color.RGBA{R: uint8(v), G: uint8(v >> 8), B: uint8(v >> 16), A: 255}
}

// accentFromColorization переводит DWM\ColorizationColor (запасной источник
// акцента на системах без AccentColor): DWORD в порядке 0xAARRGGBB.
func accentFromColorization(v uint32) color.RGBA {
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}
