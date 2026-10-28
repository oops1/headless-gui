package theme

// profiles_motion.go — токены движения и мягких теней четырёх семейств
// профилей (задача 3.5 плана 4.0).
//
// Источник — задания WinLine (ENGINE_WIN11_SHELL_TASK §7 «Тени», §8 «Меньше
// движения») и исходные ТЗ оболочек: чисел там нет, есть диапазоны — Windows
// 11 «короткие системные анимации 100–250 мс», «Пуск» — «fade + slight
// vertical movement», Windows 10 — 100–200 мс, «Пуск» снизу, центр
// уведомлений справа; Windows 2000 — без движения. Значения ниже выбраны в
// этих диапазонах и согласованы с тем, что профили уже объявляли
// (menu.open: 200 мс out-back, 150 и 180 мс out-cubic). Закрытие короче
// открытия: исчезающее не должно держать взгляд. Таблица:
//
//	ключ          Windows 11       Windows 10       macOS            Windows 2000
//	menu.open     200 out-back     150 out-cubic    180 out-cubic    0
//	menu.close    120 out-cubic    100 out-cubic    150 out-cubic    0
//	submenu.open  150 out-cubic    120 out-cubic    120 out-cubic    0
//	start.open    250 out-cubic    200 out-cubic    200 out-cubic    0
//	flyout.open   200 out-cubic    150 out-cubic    180 out-cubic    0
//
//	тень (shadow.soft)  всплывающие: blur/offY   окно и диалог: blur/offY   цвет
//	Windows 11          20 / 8                   32 / 12                    0,0,0,77 (тёмная 115)
//	Windows 10          8 / 4 — жёстче           16 / 6                     0,0,0,100
//	macOS               16 / 6                   28 / 10                    0,0,0,64 (тёмная 110)
//	Windows 2000        — (плоская тень меню со сдвигом 2, без токенов)
//
// Тени — только под флагом shadow.soft, как у Windows 11: без флага профили
// рисуют прежние тени из Elevation.

import (
	"image/color"
	"time"
)

// motionTokens — длительности и кривые семейства профилей.
type motionTokens struct {
	menuClose, submenuOpen, startOpen, flyoutOpen AnimSpec
}

func anim(ms int, curve string) AnimSpec {
	return AnimSpec{Duration: time.Duration(ms) * time.Millisecond, Curve: curve}
}

var (
	motionWin11 = motionTokens{
		menuClose: anim(120, "out-cubic"), submenuOpen: anim(150, "out-cubic"),
		startOpen: anim(250, "out-cubic"), flyoutOpen: anim(200, "out-cubic"),
	}
	motionWin10 = motionTokens{
		menuClose: anim(100, "out-cubic"), submenuOpen: anim(120, "out-cubic"),
		startOpen: anim(200, "out-cubic"), flyoutOpen: anim(150, "out-cubic"),
	}
	motionMacOS = motionTokens{
		menuClose: anim(150, "out-cubic"), submenuOpen: anim(120, "out-cubic"),
		startOpen: anim(200, "out-cubic"), flyoutOpen: anim(180, "out-cubic"),
	}
)

// declareMotion объявляет токены движения семейства.
func declareMotion(p *Profile, m motionTokens) {
	p.Anims[AnimMenuClose] = m.menuClose
	p.Anims[AnimSubmenuOpen] = m.submenuOpen
	p.Anims[AnimStartOpen] = m.startOpen
	p.Anims[AnimFlyoutOpen] = m.flyoutOpen
}

// classicMotionKeys — анимации, которые классический профиль объявляет
// нулевыми: отказ от движения, а не «не объявлено» (необъявленное ключи с
// запасом взяли бы menu.open).
var classicMotionKeys = []Key{
	"window.open", AnimMenuOpen, AnimMenuClose, AnimSubmenuOpen, AnimStartOpen, AnimFlyoutOpen,
	"hover", "taskbar.item",
}

// softShadow — мягкая тень семейства: всплывающие поверхности и окна.
type softShadow struct {
	popups                 []string
	popupBlur, popupOffY   float64
	windowBlur, windowOffY float64
	dialogOpacity          float64
	color                  color.RGBA
}

// declareSoftShadows объявляет мягкие тени под флагом shadow.soft: цвет —
// токен shadow.color, всплывающим поверхностям и окну с диалогом — свои
// размытие и сдвиг. Части компонентов (строки, плитки) тень сбрасывают:
// иначе плитка, нарисованная через PaintStyle, получила бы крупную тень
// панели. Поэтому вызов стоит в конце построения профиля — после всех
// частей.
func declareSoftShadows(p *Profile, s softShadow) {
	shadow := KeyShadowColor
	p.SetColor(shadow, s.color)
	isIn := func(comp string) bool {
		for _, c := range s.popups {
			if c == comp {
				return true
			}
		}
		return comp == "window" || comp == "dialog"
	}
	parts := map[StyleKey]bool{}
	for k := range p.Styles {
		if k.Part != "" && isIn(k.Component) {
			parts[StyleKey{Component: k.Component, Part: k.Part}] = true
		}
	}
	for _, comp := range s.popups {
		p.SetStyleWhen(FlagShadowSoft, comp, "", StateNormal, StyleDelta{
			ShadowFrom: shadow, ShadowBlur: N(s.popupBlur), ShadowOffsetY: N(s.popupOffY),
		})
	}
	noShadow := StyleDelta{ShadowBlur: N(0)}
	for k := range parts {
		p.SetStyleWhen(FlagShadowSoft, k.Component, k.Part, StateNormal, noShadow)
	}
	p.SetStyleWhen(FlagShadowSoft, "window", "", StateNormal, StyleDelta{
		ShadowFrom: shadow, ShadowBlur: N(s.windowBlur), ShadowOffsetY: N(s.windowOffY),
	})
	p.SetStyleWhen(FlagShadowSoft, "dialog", "", StateNormal, StyleDelta{
		ShadowBlur: N(s.windowBlur), ShadowOffsetY: N(s.windowOffY), ShadowOpacity: N(s.dialogOpacity),
	})
}

var (
	shadowWin10 = softShadow{
		popups:    []string{"menu", "startmenu", "quicksettings", "notifications", "notificationcenter", "calendar"},
		popupBlur: 8, popupOffY: 4, windowBlur: 16, windowOffY: 6, dialogOpacity: 0.35,
		color: RGBA(0, 0, 0, 100),
	}
	shadowMacOS = softShadow{
		popups:    []string{"menu", "startmenu", "quicksettings", "notifications", "calendar"},
		popupBlur: 16, popupOffY: 6, windowBlur: 28, windowOffY: 10, dialogOpacity: 0.30,
		color: RGBA(0, 0, 0, 64),
	}
)
