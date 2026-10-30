package base

// titlestyle.go — стиль заголовка окна (Windows/macOS/по ОС) и цвет значков
// кнопок заголовка: общие для Window (widget/chrome) и Panel.

import "image/color"

// WindowTitleStyle определяет визуальный стиль кнопок и текста заголовка.
type WindowTitleStyle int

const (
	// WindowTitleAuto — автоматический выбор стиля по текущей ОС:
	// macOS → Mac-стиль, Windows/Linux → Windows-стиль.
	// Это значение по умолчанию (zero value).
	WindowTitleAuto WindowTitleStyle = iota
	// WindowTitleWin — Windows: текст слева, кнопки ─ □ × справа.
	WindowTitleWin
	// WindowTitleMac — macOS: traffic lights ● ● ● слева, текст по центру.
	WindowTitleMac
)

// CaptionGlyphColor — цвет значков кнопок заголовка с фоном bg и текстом text
// (для неактивного окна оба уже приглушены titleColors).
//
// Прежний серый, если он читается на фоне (контраст не меньше 3:1); иначе цвет
// текста заголовка — на светлом акценте значки тёмные, как и надпись; и если
// текст почему-то не читается на собственном фоне — чёрный или белый, что
// контрастнее.
//
// Служебная: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
func CaptionGlyphColor(text, bg color.RGBA) color.RGBA {
	bg.A = 255
	if wcagContrast(CaptionGlyphLegacy, bg) >= captionGlyphMinContrast {
		return CaptionGlyphLegacy
	}
	text.A = 255
	if wcagContrast(text, bg) >= captionGlyphMinContrast {
		return text
	}
	black, white := color.RGBA{A: 255}, color.RGBA{R: 255, G: 255, B: 255, A: 255}
	if wcagContrast(black, bg) >= wcagContrast(white, bg) {
		return black
	}
	return white
}

// CaptionGlyphLegacy — прежний цвет значков кнопок заголовка. Остаётся там, где
// он по-прежнему читается: кадры окон с тёмным заголовком не меняются.
//
// Служебное: для подпакетов widget, в фасад не входит.
//
//facadegen:skip
var CaptionGlyphLegacy = color.RGBA{R: 180, G: 180, B: 180, A: 255}

// captionGlyphMinContrast — наименьший контраст значка с фоном заголовка:
// порог WCAG 2 для несущих смысл элементов интерфейса (не текста).
const captionGlyphMinContrast = 3.0

// DetectedOS возвращает строковое имя текущей ОС ("windows", "darwin", "linux").
func DetectedOS() string {
	return detectedOS()
}

// DetectedTitleStyle возвращает стиль заголовка, соответствующий текущей ОС.
func DetectedTitleStyle() WindowTitleStyle {
	return detectedTitleStyle()
}
