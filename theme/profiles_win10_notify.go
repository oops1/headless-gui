package theme

import "image/color"

// Центр уведомлений Windows 10: метрики, презентер и части стилей, которых нет
// у плоского центра. Вынесено из profiles.go и profiles_win10_shell.go, чтобы
// описание одной панели лежало в одном месте.

// NotificationCenterPresenter — имя презентера, который профиль назначает
// компоненту "notificationcenter": desktop.NotificationCenter, найдя его,
// рисует центр Windows 10 (группы, действия, быстрые действия) вместо плоского
// списка. Регистрируется пакетом desktop; здесь — только имя, чтобы профиль и
// компонент называли его одинаково.
const NotificationCenterPresenter = "notificationcenter.win10"

// declareWin10NotificationMetrics объявляет размеры центра уведомлений —
// логические пиксели при 100 %, замеры по снимку Windows 10 (план WinLine,
// §1а): панель 396 с полями 16, карточка 364, плитка быстрого действия 88×64
// при зазоре 4.
//
// Зазор до панели задач `margin` равен нулю — только у этого центра: у прочих
// всплывающих панелей остаётся зазор Flyout.
func declareWin10NotificationMetrics(p *Profile) {
	p.Presenters["notificationcenter"] = NotificationCenterPresenter

	p.SetMetric("notificationcenter.width", 396).
		SetMetric("notificationcenter.margin", 0).
		SetMetric("notificationcenter.pad", 16).
		SetMetric("notificationcenter.header.height", 48).
		SetMetric("notificationcenter.group.height", 40).
		SetMetric("notificationcenter.group.icon", 16).
		SetMetric("notificationcenter.card.gap", 4).
		SetMetric("notificationcenter.card.pad", 16).
		SetMetric("notificationcenter.card.icon", 48).
		SetMetric("notificationcenter.card.icon.gap", 16).
		SetMetric("notificationcenter.card.slot", 32).
		SetMetric("notificationcenter.card.lines", 2).
		SetMetric("notificationcenter.card.lines.max", 8).
		SetMetric("notificationcenter.action.height", 32).
		SetMetric("notificationcenter.action.gap", 4).
		SetMetric("notificationcenter.action.row.gap", 12).
		SetMetric("notificationcenter.footer.height", 32).
		SetMetric("notificationcenter.list.min", 96).
		SetMetric("notificationcenter.quick.columns", 4).
		SetMetric("notificationcenter.quick.height", 64).
		SetMetric("notificationcenter.quick.gap", 4).
		SetMetric("notificationcenter.quick.pad", 8).
		SetMetric("notificationcenter.quick.icon", 16).
		SetMetric("notificationcenter.scrollbar.width", 4).
		SetMetric("notificationcenter.toast.width", 364).
		SetMetric("notificationcenter.toast.margin", 8).
		SetMetric("notificationcenter.toast.timeout", 5000)
}

// declareWin10NotificationParts объявляет части стилей центра уведомлений,
// которые добавляют к уже объявленным в declareWin10Panels (panel, header, link,
// group, card, action, quick.tile, quick.tile.on):
//
//	toast      — подложка всплывающего тоста над треем (акрил, как у панели);
//	field      — поле ответа и выпадающий список (покой, наведение, фокус);
//	dim        — приглушённый текст: тело уведомления, время, подсказки;
//	glyph      — крестик и шевроны карточки: приглушённые, под курсором яркие;
//	scrollbar  — бегунок тонкой полосы прокрутки списка.
//
// Режим (светлый или тёмный) определяется палитрой: тёмный текст — светлая
// панель.
func declareWin10NotificationParts(set func(comp, part string, st State, d StyleDelta), pal win10Shell) {
	light := int(pal.text.R)+int(pal.text.G)+int(pal.text.B) < 3*128
	clear := C(RGBA(0, 0, 0, 0))
	flat := func(d StyleDelta) StyleDelta {
		if d.Border == nil && d.BorderFrom == "" {
			d.Border = clear
		}
		if d.BorderWidth == nil {
			d.BorderWidth = N(0)
		}
		d.Elevation, d.Shadow = N(0), clear
		return d
	}
	const comp = "notificationcenter"

	set(comp, "toast", StateNormal, flat(StyleDelta{
		Backdrop: win10Acrylic(pal.tint, pal.fallback), Text: C(pal.text),
		Border: C(pal.border), BorderWidth: N(1),
	}))

	var fill, border, borderHover color.RGBA
	var thumb, thumbHover color.RGBA
	if light {
		fill, border, borderHover = RGBA(255, 255, 255, 215), RGBA(0, 0, 0, 110), RGBA(0, 0, 0, 170)
		thumb, thumbHover = RGBA(0, 0, 0, 110), RGBA(0, 0, 0, 170)
	} else {
		fill, border, borderHover = RGBA(0, 0, 0, 120), RGBA(255, 255, 255, 120), RGBA(255, 255, 255, 190)
		thumb, thumbHover = RGBA(255, 255, 255, 110), RGBA(255, 255, 255, 170)
	}
	field := func(st State, b color.RGBA, w float64) {
		set(comp, "field", st, flat(StyleDelta{
			Fill: C(fill), Text: C(pal.text), Border: C(b), BorderWidth: N(w),
		}))
	}
	field(StateNormal, border, 1)
	field(StateHover, borderHover, 1)
	field(StatePressed, borderHover, 1)
	field(StateActive, pal.focus, 1)
	field(StateFocused, pal.focus, 2)
	set(comp, "field", StateDisabled, flat(StyleDelta{Fill: C(fill), Text: C(pal.dim), Border: C(border), BorderWidth: N(1)}))

	// Приглушённый текст карточки: тело уведомления, время, подсказки.
	set(comp, "dim", StateNormal, flat(StyleDelta{Text: C(pal.dim)}))

	set(comp, "glyph", StateNormal, flat(StyleDelta{Text: C(pal.dim)}))
	set(comp, "glyph", StateHover, flat(StyleDelta{Fill: C(pal.hover), Text: C(pal.text)}))
	set(comp, "glyph", StatePressed, flat(StyleDelta{Fill: C(pal.pressed), Text: C(pal.text)}))
	set(comp, "glyph", StateFocused, flat(StyleDelta{
		Text: C(pal.text), Border: C(pal.focus), BorderWidth: N(1),
	}))

	set(comp, "scrollbar", StateNormal, flat(StyleDelta{Fill: C(thumb)}))
	set(comp, "scrollbar", StateHover, flat(StyleDelta{Fill: C(thumbHover)}))
	set(comp, "scrollbar", StatePressed, flat(StyleDelta{Fill: C(thumbHover)}))
}
