package theme

import "image/color"

// Материалы фона и мягкие тени Windows 11 — условные стили под флагами.
//
// По умолчанию профиль Windows 11 не меняется: панели оболочки сплошные
// (Solid), тени прежние. Mica по каналу удалённого рабочего стола дорога при
// движении окон (под окном большая гладкая область, которая меняется на каждый
// сдвиг), поэтому включает её потребитель из своих настроек:
//
//	m.SetFlag(theme.FlagBackdropMica, true)    // Mica на панелях
//	m.SetFlag(theme.FlagBackdropMicaAlt, true) // темнее
//	m.SetFlag(theme.FlagShadowSoft, true)      // мягкие тени
//	eng.ApplyThemeProfile(m)
//
// Mica без обоев (у движка не задан фон и Engine.SetWallpaperSource) рисуется
// сплошным Fallback — тем же цветом, что и у Solid.
//
// Цвета материалов — токены, а не литералы стилей: "surface" (Mica и
// Solid), "surface.alt" (MicaAlt, темнее) и "shadow.color" (цвет мягкой тени
// вместе с её непрозрачностью). Тёмная разновидность подменяет эти токены и
// остаётся короткой.

const (
	// KeySurfaceAlt — основа MicaAlt: темнее поверхности.
	KeySurfaceAlt Key = "surface.alt"
	// KeyShadowColor — цвет мягкой тени Windows 11 (альфа — её непрозрачность).
	KeyShadowColor Key = "shadow.color"
)

// win11MicaRadius — радиус размытия обоев в логических пикселях: Mica почти
// не читается как размытая картинка, а как мягкое пятно в тоне обоев.
const win11MicaRadius = 80

// win11MicaTintAlpha / win11MicaAltTintAlpha — доля подкраски поверх размытых
// обоев: чем больше, тем меньше обоев проступает.
const (
	win11MicaTintAlpha    = 205
	win11MicaAltTintAlpha = 205
)

// win11MaterialPanels — компоненты, которые рисуются на Mica.
var win11MaterialPanels = []string{"startmenu", "quicksettings", "notifications", "notificationcenter", "calendar", "window"}

// win11SoftShadowPopups — компоненты с тенью всплывающей поверхности.
var win11SoftShadowPopups = []string{"menu", "startmenu", "quicksettings", "notifications", "notificationcenter", "calendar"}

// declareWin11Materials объявляет токены и условные стили Mica, MicaAlt и
// мягких теней. Токены surface.alt и shadow.color — светлые значения; тёмный
// профиль переопределяет их.
func declareWin11Materials(p *Profile, altColor, shadowColor color.RGBA) {
	surfaceAlt, shadow := KeySurfaceAlt, KeyShadowColor
	p.SetColor(surfaceAlt, altColor).SetColor(shadow, shadowColor)

	mica := func(base Key, alpha uint8, mat BackdropMaterial) StyleDelta {
		return StyleDelta{
			Backdrop: &BackdropSpec{
				Mode: BackdropBlur, Material: mat, Radius: win11MicaRadius,
				Tint: RGBA(0, 0, 0, alpha), // цвет подставит BackdropFrom
			},
			BackdropFrom: base,
		}
	}
	// Части компонента (плитки, строки, карточки) наследуют стиль компонента
	// целиком, а с ним — материал и тень панели: плитка, нарисованная через
	// PaintStyle, получила бы свою Mica и свою крупную тень. Поэтому каждая
	// объявленная к этому часу часть панели сбрасывает токены обратно: у неё
	// нет ни материала, ни тени по токенам (прежняя тень от Elevation остаётся
	// как была). Части, объявленные ПОСЛЕ этого вызова, должны делать то же —
	// поэтому он стоит в конце построения профиля.
	parts := map[StyleKey]bool{}
	for k := range p.Styles {
		if k.Part != "" {
			parts[StyleKey{Component: k.Component, Part: k.Part}] = true
		}
	}
	isIn := func(list []string, comp string) bool {
		for _, c := range list {
			if c == comp {
				return true
			}
		}
		return false
	}
	noBackdrop := StyleDelta{Backdrop: &BackdropSpec{}}
	noShadow := StyleDelta{ShadowBlur: N(0)}

	for _, comp := range win11MaterialPanels {
		p.SetStyleWhen(FlagBackdropMica, comp, "", StateNormal, mica("surface", win11MicaTintAlpha, MaterialMica))
		p.SetStyleWhen(FlagBackdropMicaAlt, comp, "", StateNormal, mica(surfaceAlt, win11MicaAltTintAlpha, MaterialMicaAlt))
	}
	for _, comp := range win11SoftShadowPopups {
		p.SetStyleWhen(FlagShadowSoft, comp, "", StateNormal, StyleDelta{
			ShadowFrom: shadow, ShadowBlur: N(20), ShadowOffsetY: N(8),
		})
	}
	for k := range parts {
		if isIn(win11MaterialPanels, k.Component) {
			p.SetStyleWhen(FlagBackdropMica, k.Component, k.Part, StateNormal, noBackdrop)
			p.SetStyleWhen(FlagBackdropMicaAlt, k.Component, k.Part, StateNormal, noBackdrop)
		}
		if isIn(win11SoftShadowPopups, k.Component) || k.Component == "window" || k.Component == "dialog" {
			p.SetStyleWhen(FlagShadowSoft, k.Component, k.Part, StateNormal, noShadow)
		}
	}
	// Окно и диалог: тень крупнее. Цвет стиля "dialog" питает ещё и плоскую
	// ShadowColor старых виджетов, поэтому диалогу цвет не задаётся — токены
	// сами дают чёрную тень.
	p.SetStyleWhen(FlagShadowSoft, "window", "", StateNormal, StyleDelta{
		ShadowFrom: shadow, ShadowBlur: N(32), ShadowOffsetY: N(12),
	})
	p.SetStyleWhen(FlagShadowSoft, "dialog", "", StateNormal, StyleDelta{
		ShadowBlur: N(32), ShadowOffsetY: N(12), ShadowOpacity: N(0.30),
	})
}
