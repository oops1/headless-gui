package theme

import "image/color"

// Трей Windows 10: размеры по замерам референса (100 %, панель 40 px) и
// подсветка значков на всю высоту полосы.

// KeyTrayFillStrip — флаг темы: подсветка наведения значков трея занимает всю
// высоту полосы панели, а не только квадрат значка. В Windows 10 наведённый
// значок трея, кнопка центра уведомлений и «Пуск» подсвечиваются на всю
// высоту 40; в остальных темах значок подсвечивается своим квадратом.
const KeyTrayFillStrip Key = "tray.fill.strip"

// declareWin10Tray доводит трей Windows 10 до замеров: шаг значков 24, кнопка
// центра уведомлений 40 со значком 16 (горит, пока центр открыт), полоска
// «Показать рабочий стол» 5 px с линией 1 px слева.
//
// Зовётся после inheritTrayStyles: берёт уже унаследованные от tray.volume
// стили и правит в них только отступы и состояния.
func declareWin10Tray(p *Profile) {
	p.SetFlag(KeyTrayFillStrip, true).
		SetMetric("tray.showdesktop.width", 5).
		SetMetric("tray.showdesktop.line", 1)

	// Значок 16 с отступом 4 с каждой стороны — шаг 24.
	setPad := func(comp string, padX float64) {
		k := StyleKey{Component: comp, State: StateNormal}
		d := p.Styles[k]
		d.PadX = N(padX)
		p.Styles[k] = d
	}
	for _, comp := range []string{"tray.network", "tray.volume", "tray.power", "tray.icon"} {
		setPad(comp, 4)
	}
	// Кнопка центра уведомлений: 16 + 2·12 = 40.
	setPad("tray.notifications", 12)

	// Пока центр открыт, кнопка горит — тем же оттенком, что кнопки приложений
	// («активно»).
	p.SetStyle("tray.notifications", "", StateActive, StyleDelta{Fill: C(RGBA(255, 255, 255, 38))})
	p.SetStyleWhen(KeyTaskbarLight, "tray.notifications", "", StateActive, StyleDelta{Fill: C(RGBA(0, 0, 0, 34))})

	// Линия слева от полоски «Показать рабочий стол».
	line := func(c color.RGBA) StyleDelta { return StyleDelta{Fill: C(c)} }
	p.SetStyle("tray.showdesktop", "line", StateNormal, line(RGBA(255, 255, 255, 64)))
	p.SetStyleWhen(KeyTaskbarLight, "tray.showdesktop", "line", StateNormal, line(RGBA(0, 0, 0, 64)))
}
