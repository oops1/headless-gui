package theme

// Токены контекстного меню (widget.PopupMenu).
//
// Меню читает вид из профиля само: стили компонента "menu" (части "",
// "item", "separator", "shortcut") и метрики ниже. Метрика, которой в
// профиле нет, оставляет меню прежним — то есть профиль, не знающий этих
// токенов, выглядит ровно так же, как до их появления.
//
// Все метрики — в логических пикселях; ноль и «не объявлено» не различаются
// (кроме KeyMenuItemInset, где объявленный ноль означает «плашка вплотную к
// рамке»).
//
// Стили:
//
//	menu                     Fill, Text, Border (+BorderWidth), Corner, Shadow, Elevation
//	menu.item   (Hover)      Fill — плашка наведения, Text — цвет текста под ней, Corner — её скругление
//	menu.item   (Disabled)   Text — недоступный пункт
//	menu.separator           Fill — цвет линии разделителя (иначе цвет рамки)
//	menu.shortcut            Text — цвет сочетания клавиш (иначе приглушённый цвет текста)
const (
	// KeyMenuCorner и KeyMenuItemCorner — скругление меню и плашки наведения
	// метрикой. Нужны там, где стиль не годится: у стиля без Corner радиус
	// наследуется от контролов (control.corner), и профиль, собранный из
	// плоской темы, не отличить от объявившего его. Метрика, если есть,
	// сильнее Corner стиля.
	KeyMenuCorner     Key = "menu.corner"
	KeyMenuItemCorner Key = "menu.item.corner"
	// KeyMenuItemHeight — высота обычного пункта.
	KeyMenuItemHeight Key = "menu.item.height"
	// KeyMenuSeparatorHeight — место под разделитель вместе с линией.
	KeyMenuSeparatorHeight Key = "menu.separator.height"
	// KeyMenuPadX — отступ от края меню до значка (или подписи) и от
	// сочетания клавиш до противоположного края. Для раздельной настройки
	// сторон — KeyMenuPadLeft и KeyMenuPadRight.
	KeyMenuPadX Key = "menu.pad.x"
	// KeyMenuPadLeft — левое поле: от края меню до отметки/значка. Не
	// объявлено — берётся KeyMenuPadX.
	KeyMenuPadLeft Key = "menu.pad.left"
	// KeyMenuPadRight — правое поле: от сочетания клавиш до края меню. Не
	// объявлено — берётся KeyMenuPadX.
	KeyMenuPadRight Key = "menu.pad.right"
	// KeyMenuPadY — отступ первого пункта от верхней рамки (и последнего от
	// нижней).
	KeyMenuPadY Key = "menu.pad.y"
	// KeyMenuIconSize — сторона значка пункта по умолчанию.
	KeyMenuIconSize Key = "menu.icon.size"
	// KeyMenuIconGap — зазор между значком и подписью.
	KeyMenuIconGap Key = "menu.icon.gap"
	// KeyMenuWidthMin — наименьшая ширина корневого меню.
	KeyMenuWidthMin Key = "menu.width.min"
	// KeyMenuSubWidthMin — наименьшая ширина подменю. Не объявлено, но
	// объявлен KeyMenuWidthMin — у подменю наименьшей ширины нет: минимум
	// нужен корневому меню, а подменю по ширине содержимого.
	KeyMenuSubWidthMin Key = "menu.submenu.width.min"
	// KeyMenuItemInset — отступ плашки наведения от краёв меню. Объявленный
	// ноль — плашка вплотную к рамке (но не поверх неё).
	KeyMenuItemInset Key = "menu.item.inset"
	// KeyMenuSeparatorInset — отступ линии разделителя от краёв меню.
	KeyMenuSeparatorInset Key = "menu.separator.inset"
	// KeyMenuChevronRight — включает тонкий шеврон подменю «›» вместо
	// глифа и задаёт расстояние от ПРАВОГО КРАЯ меню до его середины.
	KeyMenuChevronRight Key = "menu.chevron.right"
	// KeyMenuChevronSize — высота шеврона (по умолчанию 8).
	KeyMenuChevronSize Key = "menu.chevron.size"
	// KeyMenuSubmenuDelay — задержка раскрытия и закрытия подменю при
	// наведении, мс. Нет — подменю раскрывается сразу. Нажатие и клавиши
	// задержкой не затрагиваются.
	KeyMenuSubmenuDelay Key = "menu.submenu.delay"
)

// FlagMenuIconTint — значки пунктов одноцветные и рисуются цветом текста
// пункта в его текущем состоянии (обычный, под плашкой, недоступный):
// чёрный значок на синей плашке Windows 2000 иначе не виден.
const FlagMenuIconTint Key = "menu.icon.tint"

// ─── Значения встроенных профилей ───────────────────────────────────────────
//
// Цифры сняты с эталонов: меню рабочего стола Windows 11 при 100 %
// (строка 24, значок 16 в 12 от края, плашка наведения со скруглением 4 и
// отступом 4, рамка 1, скругление меню 8). Windows 10 — то же, но с прямыми
// углами и плашкой на всю ширину; Windows 2000 — классика.

// amendStyle меняет один стиль профиля, сохранив заявленное им ранее:
// SetStyle заменяет запись целиком, а у меню в этих профилях уже есть
// размытие подложки, рамка и тень, которые не должны пропасть.
func amendStyle(p *Profile, component, part string, st State, change func(*StyleDelta)) {
	d := p.Styles[StyleKey{Component: component, Part: part, State: st.Dominant()}]
	change(&d)
	p.SetStyle(component, part, st, d)
}

// declareMenuGeometry — метрики, общие для всех семейств.
func declareMenuGeometry(p *Profile, height, separator, padX, icon, inset float64) {
	p.SetMetric(KeyMenuItemHeight, height).
		SetMetric(KeyMenuSeparatorHeight, separator).
		SetMetric(KeyMenuPadX, padX).
		SetMetric(KeyMenuIconSize, icon).
		SetMetric(KeyMenuItemInset, inset)
}

// declareClassicMenu — меню Windows 2000: строка 20, плашка на всю ширину,
// значок почти у края, подменю раскрывается через 400 мс (SPI_GETMENUSHOWDELAY).
func declareClassicMenu(p *Profile) {
	declareMenuGeometry(p, 20, 7, 10, 16, 0)
	p.SetMetric(KeyMenuPadLeft, 4).
		SetMetric(KeyMenuIconGap, 6).
		SetMetric(KeyMenuSubmenuDelay, 400)
}

// declareWin10Menu — меню Windows 10: прямые углы, плашка на всю ширину.
func declareWin10Menu(p *Profile) {
	declareMenuGeometry(p, 24, 8, 12, 16, 0)
	p.SetMetric(KeyMenuWidthMin, 290)
	amendStyle(p, "menu", "", StateNormal, func(d *StyleDelta) { d.Corner = N(0) })
	amendStyle(p, "menu", "item", StateHover, func(d *StyleDelta) { d.Corner = N(0) })
	amendStyle(p, "menu", "separator", StateNormal, func(d *StyleDelta) {
		d.Fill = C(RGB(70, 70, 70))
	})
	p.SetStyleWhen(KeyTaskbarLight, "menu", "separator", StateNormal,
		StyleDelta{Fill: C(RGB(225, 225, 225))})
}

// declareWin11Menu — меню Windows 11, светлое: скругление 8, плашка со
// скруглением 4 и отступом 4, тонкий шеврон, сочетания цветом текста.
//
// Заливка непрозрачная: размытие, которое просит Backdrop, меню в окне-попапе
// нарисовать нечем, и стиль без заливки оставил бы меню в цвете виджетов.
func declareWin11Menu(p *Profile) {
	declareMenuGeometry(p, 24, 8, 12, 16, 4)
	p.SetMetric(KeyMenuWidthMin, 290).
		SetMetric(KeyMenuPadY, 4).
		SetMetric(KeyMenuIconGap, 9).
		SetMetric(KeyMenuChevronRight, 18)
	amendStyle(p, "menu", "", StateNormal, func(d *StyleDelta) {
		d.Fill = C(RGB(249, 249, 249))
	})
	// Плашка — тонкая чёрная плёнка: 249 → 240, как на эталоне.
	p.SetStyle("menu", "item", StateHover, StyleDelta{Fill: C(RGBA(0, 0, 0, 9)), Corner: N(4)})
	p.SetStyle("menu", "item", StateDisabled, StyleDelta{Text: C(RGB(159, 159, 159))})
	p.SetStyle("menu", "separator", StateNormal, StyleDelta{Fill: C(RGB(229, 229, 229))})
	p.SetStyle("menu", "shortcut", StateNormal, StyleDelta{TextFrom: "text"})
}

// declareWin11DarkMenu — тёмная разновидность: только цвета, размеры
// наследуются.
func declareWin11DarkMenu(p *Profile) {
	amendStyle(p, "menu", "", StateNormal, func(d *StyleDelta) {
		d.Fill = C(RGB(44, 44, 44))
		d.Border = C(RGB(67, 67, 67))
	})
	p.SetStyle("menu", "item", StateHover, StyleDelta{Fill: C(RGBA(255, 255, 255, 22)), Corner: N(4)})
	p.SetStyle("menu", "item", StateDisabled, StyleDelta{Text: C(RGB(128, 128, 128))})
	p.SetStyle("menu", "separator", StateNormal, StyleDelta{Fill: C(RGB(69, 69, 69))})
}
