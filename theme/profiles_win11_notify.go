package theme

import (
	"image/color"
	"time"
)

// Центр уведомлений и календарь Windows 11: презентеры, токены, метрики и
// части стилей. Вынесено из profiles.go, чтобы описание этой пары панелей
// лежало в одном месте (как profiles_win10_notify.go у Windows 10).

// Презентеры Windows 11. Профиль называет их в Profile.Presenters
// ("notificationcenter", "calendar"); desktop.NotificationCenter и
// desktop.CalendarFlyout, найдя такое имя, рисуют центр с карточками и
// календарь отдельной скруглённой карточкой под ним. Регистрирует их пакет
// desktop; здесь — только имена, чтобы профиль и компонент называли их одинаково.
const (
	NotificationCenterWin11Presenter = "notificationcenter.win11"
	CalendarWin11Presenter           = "calendar.win11"
)

const (
	// KeySurfaceCard — заливка карточки на панели: уведомления, плитки
	// модулей. Светлая — белая плёнка поверх серой панели, тёмная — светлая
	// плёнка на тёмной. Тёмный профиль подменяет токен, а не стили.
	KeySurfaceCard Key = "surface.card"
	// KeyTextSecondary — приглушённый текст: время, приложение, подсказки,
	// недоступное. Тёмный профиль подменяет токен.
	KeyTextSecondary Key = "text.secondary"
)

// declareWin11NotificationTokens объявляет токены карточек (светлые значения).
func declareWin11NotificationTokens(p *Profile) {
	p.SetColor(KeySurfaceCard, RGBA(255, 255, 255, 178)).
		SetColor(KeyTextSecondary, RGB(96, 96, 96))
}

// declareWin11DarkNotificationTokens — тёмные значения тех же токенов.
func declareWin11DarkNotificationTokens(p *Profile) {
	p.SetColor(KeySurfaceCard, RGBA(255, 255, 255, 13)).
		SetColor(KeyTextSecondary, RGB(197, 197, 197))
}

// declareWin11Notifications объявляет центр уведомлений и календарь Windows 11:
// презентеры, метрики (логические пиксели при 100 %), шрифты, анимацию и
// части стилей.
//
// Вызывается ДО declareWin11Materials: тот проходит по уже объявленным частям и
// отключает у них материал и мягкую тень панели, чтобы карточка не получила
// собственную Mica. Подложки самих панелей (компонент целиком) в материал
// включены там же; здесь нужно только, чтобы части объявились раньше.
//
// Цвета — ссылки на токены (surface, text, border, accent, surface.card,
// text.secondary) или нейтральная серая плёнка, одинаково читаемая в обоих
// режимах: тёмный профиль меняет токены и не повторяет стили.
func declareWin11Notifications(p *Profile) {
	p.Presenters["notificationcenter"] = NotificationCenterWin11Presenter
	p.Presenters["calendar"] = CalendarWin11Presenter

	// ── Центр уведомлений ───────────────────────────────────────────────
	// Ширина 364, скругление 8 (из стиля панели), карточки со скруглением 4.
	// Центр стоит у правого края рабочей области, над календарём; зазор между
	// двумя карточками — stack.gap, до края экрана и панели задач — edge.
	p.SetMetric("notificationcenter.width", 364).
		SetMetric("notificationcenter.margin", 0).
		SetMetric("notificationcenter.edge", 12).
		SetMetric("notificationcenter.stack.gap", 8).
		SetMetric("notificationcenter.pad", 12).
		SetMetric("notificationcenter.header.height", 52).
		SetMetric("notificationcenter.header.button", 32).
		SetMetric("notificationcenter.group.height", 36).
		SetMetric("notificationcenter.group.icon", 16).
		SetMetric("notificationcenter.card.gap", 8).
		SetMetric("notificationcenter.card.pad", 12).
		SetMetric("notificationcenter.card.icon", 16).
		SetMetric("notificationcenter.card.icon.gap", 8).
		SetMetric("notificationcenter.card.slot", 20).
		SetMetric("notificationcenter.card.lines", 2).
		SetMetric("notificationcenter.card.lines.max", 8).
		SetMetric("notificationcenter.action.height", 32).
		SetMetric("notificationcenter.action.gap", 8).
		SetMetric("notificationcenter.action.row.gap", 12).
		SetMetric("notificationcenter.list.min", 96).
		SetMetric("notificationcenter.scrollbar.width", 4).
		SetMetric("notificationcenter.severity.width", 3).
		SetMetric("notificationcenter.toast.width", 364).
		SetMetric("notificationcenter.toast.margin", 12).
		SetMetric("notificationcenter.toast.timeout", 5000)

	p.Fonts["caption"] = FontSpec{Size: 8}
	p.Fonts["title"] = FontSpec{Size: 9.5, Weight: WeightSemiBold}
	p.Fonts["heading"] = FontSpec{Size: 13.5, Weight: WeightSemiBold}
	p.Anims["notification.expand"] = AnimSpec{Duration: 150 * time.Millisecond, Curve: "out-cubic"}

	// ── Календарь ───────────────────────────────────────────────────────
	// Отдельная карточка шириной 364: строка «день недели, дата» со стрелкой
	// свернуть/развернуть, строка месяца с листанием, неделя и сетка дней,
	// затем модуль «Фокусировка». Сегодняшний день — круг акцента диаметром day.
	p.SetMetric("calendar.w11.width", 364).
		SetMetric("calendar.w11.pad", 16).
		SetMetric("calendar.w11.head", 44).
		SetMetric("calendar.w11.month", 36).
		SetMetric("calendar.w11.weekday", 28).
		SetMetric("calendar.w11.cell", 40).
		SetMetric("calendar.w11.day", 36).
		SetMetric("calendar.w11.nav", 32).
		SetMetric("calendar.w11.focus", 100).
		SetMetric("calendar.w11.button", 32).
		SetMetric("calendar.w11.edge", 12)

	// ── Стили ───────────────────────────────────────────────────────────
	clear := C(RGBA(0, 0, 0, 0))
	flat := func(d StyleDelta) StyleDelta {
		if d.Fill == nil && d.FillFrom == "" {
			d.Fill = clear
		}
		if d.Border == nil && d.BorderFrom == "" {
			d.Border = clear
		}
		if d.BorderWidth == nil {
			d.BorderWidth = N(0)
		}
		d.Elevation, d.Shadow = N(0), clear
		return d
	}
	// Нейтральная серая плёнка: светлеет тёмное и темнеет светлое, поэтому один
	// набор годится для обоих режимов.
	veil := func(a uint8) *color.RGBA { return C(RGBA(128, 128, 128, a)) }
	const comp = "notificationcenter"
	text, dim := Key("text"), KeyTextSecondary

	// Подложка центра: рамка в один пиксель; заливка, скругление и тень — из
	// стиля панели (notifications) и материала.
	p.SetStyle(comp, "", StateNormal, StyleDelta{BorderFrom: "border", BorderWidth: N(1)})

	set := func(part string, st State, d StyleDelta) { p.SetStyle(comp, part, st, flat(d)) }
	set("toast", StateNormal, StyleDelta{
		FillFrom: "surface", TextFrom: text, Corner: N(8), BorderFrom: "border", BorderWidth: N(1),
	})
	// Нижний слой тоста — тень по подъёму, как у панели.
	amendStyle(p, comp, "toast", StateNormal, func(d *StyleDelta) { d.Elevation, d.Shadow = N(12), C(RGBA(0, 0, 0, 70)) })

	set("header", StateNormal, StyleDelta{TextFrom: text})
	// Кнопки заголовка: колокольчик и «Очистить все». Активное состояние —
	// «Не беспокоить» включено.
	set("headbtn", StateNormal, StyleDelta{TextFrom: text, Corner: N(4)})
	set("headbtn", StateHover, StyleDelta{Fill: veil(40), TextFrom: text, Corner: N(4)})
	set("headbtn", StatePressed, StyleDelta{Fill: veil(64), TextFrom: text, Corner: N(4)})
	set("headbtn", StateFocused, StyleDelta{TextFrom: text, Corner: N(4), BorderFrom: text, BorderWidth: N(1)})
	set("headbtn", StateDisabled, StyleDelta{TextFrom: dim, Corner: N(4)})
	// «Не беспокоить» включено: колокольчик цветом акцента.
	set("headbtn.on", StateNormal, StyleDelta{TextFrom: KeyAccent, Corner: N(4)})
	set("headbtn.on", StateHover, StyleDelta{Fill: veil(40), TextFrom: KeyAccent, Corner: N(4)})
	set("headbtn.on", StatePressed, StyleDelta{Fill: veil(64), TextFrom: KeyAccent, Corner: N(4)})
	set("headbtn.on", StateFocused, StyleDelta{TextFrom: KeyAccent, Corner: N(4), BorderFrom: text, BorderWidth: N(1)})

	set("link", StateNormal, StyleDelta{TextFrom: KeyAccent})
	set("link", StateHover, StyleDelta{TextFrom: KeyAccentHover})
	set("link", StatePressed, StyleDelta{TextFrom: KeyAccentPressed})
	set("link", StateFocused, StyleDelta{TextFrom: KeyAccent, BorderFrom: text, BorderWidth: N(1), Corner: N(4)})

	set("group", StateNormal, StyleDelta{TextFrom: text, Corner: N(4)})
	set("group", StateHover, StyleDelta{Fill: veil(32), TextFrom: text, Corner: N(4)})
	set("group", StatePressed, StyleDelta{Fill: veil(56), TextFrom: text, Corner: N(4)})
	set("group", StateActive, StyleDelta{TextFrom: text, Corner: N(4)})
	set("group", StateFocused, StyleDelta{TextFrom: text, Corner: N(4), BorderFrom: text, BorderWidth: N(1)})
	set("pill", StateNormal, StyleDelta{Fill: veil(56), TextFrom: text, Corner: N(9)})

	// Карточка: заливка-плёнка, тонкая рамка, скругление 4.
	card := func(st State, b Key) {
		set("card", st, StyleDelta{FillFrom: KeySurfaceCard, TextFrom: text, Corner: N(4), BorderFrom: b, BorderWidth: N(1)})
	}
	card(StateNormal, "border")
	card(StateHover, KeyTextSecondary)
	card(StatePressed, KeyTextSecondary)
	card(StateActive, KeyTextSecondary)
	card(StateFocused, KeyTextSecondary)

	// Кнопки действий внутри карточки.
	set("action", StateNormal, StyleDelta{Fill: veil(40), TextFrom: text, Corner: N(4)})
	set("action", StateHover, StyleDelta{Fill: veil(64), TextFrom: text, Corner: N(4)})
	set("action", StatePressed, StyleDelta{Fill: veil(88), TextFrom: text, Corner: N(4)})
	set("action", StateActive, StyleDelta{Fill: veil(64), TextFrom: text, Corner: N(4)})
	set("action", StateFocused, StyleDelta{Fill: veil(40), TextFrom: text, Corner: N(4), BorderFrom: text, BorderWidth: N(1)})
	set("action", StateDisabled, StyleDelta{Fill: veil(40), TextFrom: dim, Corner: N(4)})

	field := func(st State, a, border uint8, w float64) {
		set("field", st, StyleDelta{Fill: veil(a), TextFrom: text, Corner: N(4), Border: veil(border), BorderWidth: N(w)})
	}
	field(StateNormal, 24, 110, 1)
	field(StateHover, 40, 160, 1)
	field(StatePressed, 40, 160, 1)
	set("field", StateActive, StyleDelta{Fill: veil(24), TextFrom: text, Corner: N(4), BorderFrom: KeyAccent, BorderWidth: N(1)})
	set("field", StateFocused, StyleDelta{Fill: veil(24), TextFrom: text, Corner: N(4), BorderFrom: KeyAccent, BorderWidth: N(2)})
	set("field", StateDisabled, StyleDelta{Fill: veil(24), TextFrom: dim, Corner: N(4), Border: veil(110), BorderWidth: N(1)})

	set("dim", StateNormal, StyleDelta{TextFrom: dim})
	set("glyph", StateNormal, StyleDelta{TextFrom: dim, Corner: N(4)})
	set("glyph", StateHover, StyleDelta{Fill: veil(48), TextFrom: text, Corner: N(4)})
	set("glyph", StatePressed, StyleDelta{Fill: veil(72), TextFrom: text, Corner: N(4)})
	set("glyph", StateFocused, StyleDelta{TextFrom: text, Corner: N(4), BorderFrom: text, BorderWidth: N(1)})

	// Важность: полоса у левого края карточки, нейтральные средние тона —
	// читаются и на светлой, и на тёмной панели.
	set("severity.warning", StateNormal, StyleDelta{Fill: C(RGB(224, 158, 0))})
	set("severity.error", StateNormal, StyleDelta{Fill: C(RGB(209, 52, 56))})

	set("scrollbar", StateNormal, StyleDelta{Fill: veil(140)})
	set("scrollbar", StateHover, StyleDelta{Fill: veil(190)})
	set("scrollbar", StatePressed, StyleDelta{Fill: veil(190)})

	// ── Календарь: части Windows 11 ─────────────────────────────────────
	// Подложка — компонент целиком (рамка и поля): панель без внутреннего
	// отступа Flyout, раскладку отступов ведёт метрика calendar.w11.pad.
	amendStyle(p, "calendar", "", StateNormal, func(d *StyleDelta) {
		d.PadX, d.PadY = N(0), N(0)
		d.BorderFrom, d.BorderWidth = "border", N(1)
	})
	const cal = "calendar"
	cset := func(part string, st State, d StyleDelta) { p.SetStyle(cal, part, st, flat(d)) }
	cset("date", StateNormal, StyleDelta{TextFrom: text})
	cset("month", StateNormal, StyleDelta{TextFrom: text})
	cset("nav", StateNormal, StyleDelta{TextFrom: text, Corner: N(4)})
	cset("nav", StateHover, StyleDelta{Fill: veil(40), TextFrom: text, Corner: N(4)})
	cset("nav", StatePressed, StyleDelta{Fill: veil(64), TextFrom: text, Corner: N(4)})
	cset("nav", StateFocused, StyleDelta{TextFrom: text, Corner: N(4), BorderFrom: text, BorderWidth: N(1)})
	cset("divider", StateNormal, StyleDelta{FillFrom: "border"})
	cset("dim", StateNormal, StyleDelta{TextFrom: dim})
	cset("focus.title", StateNormal, StyleDelta{TextFrom: text})
	cset("focus.value", StateNormal, StyleDelta{TextFrom: text})
	cset("focus.button", StateNormal, StyleDelta{Fill: veil(40), TextFrom: text, Corner: N(4)})
	cset("focus.button", StateHover, StyleDelta{Fill: veil(64), TextFrom: text, Corner: N(4)})
	cset("focus.button", StatePressed, StyleDelta{Fill: veil(88), TextFrom: text, Corner: N(4)})
	cset("focus.button", StateFocused, StyleDelta{Fill: veil(40), TextFrom: text, Corner: N(4), BorderFrom: text, BorderWidth: N(1)})
	cset("focus.button", StateDisabled, StyleDelta{Fill: veil(24), TextFrom: dim, Corner: N(4)})
	cset("focus.accent", StateNormal, StyleDelta{FillFrom: KeyAccent, TextFrom: KeyAccentText, Corner: N(4)})
	cset("focus.accent", StateHover, StyleDelta{FillFrom: KeyAccentHover, TextFrom: KeyAccentText, Corner: N(4)})
	cset("focus.accent", StatePressed, StyleDelta{FillFrom: KeyAccentPressed, TextFrom: KeyAccentText, Corner: N(4)})
	cset("focus.accent", StateFocused, StyleDelta{FillFrom: KeyAccent, TextFrom: KeyAccentText, Corner: N(4), BorderFrom: text, BorderWidth: N(1)})
	cset("progress", StateNormal, StyleDelta{Fill: veil(64), Corner: N(2)})
	cset("progress.fill", StateNormal, StyleDelta{FillFrom: KeyAccent, Corner: N(2)})

	// День: круг. Сегодня (активный) — акцент, наведение — серая плёнка. Часть
	// "day" уже объявлена для плоского календаря; здесь только скругление.
	for _, st := range []State{StateNormal, StateHover, StateActive, StateFocused, StateDisabled} {
		amendStyle(p, cal, "day", st, func(d *StyleDelta) {
			d.Corner = N(18)
			// Число — не панель: подъём и тень панели не наследует.
			d.Elevation, d.Shadow = N(0), clear
			// И рамки панели: её у числа нет, кроме рамки выбранного дня.
			if st != StateFocused {
				d.Border, d.BorderWidth = clear, N(0)
			}
		})
	}
	// Выбранный день, не сегодняшний: рамка акцента (состояние Focused уже
	// объявлено рамкой для плоского календаря и подходит как есть).
}

// declareWin11NotificationMaterials возвращает подложкам центра, тоста и
// календаря материал Mica и MicaAlt: declareWin11Materials отключает материал у
// всех объявленных частей, а подложка центра и календаря — части компонента
// целиком, её материал задан там же. Здесь ничего не требуется, кроме тени
// тоста: мягкая тень включается и на нём.
func declareWin11NotificationMaterials(p *Profile, shadow Key) {
	p.SetStyleWhen(FlagShadowSoft, "notificationcenter", "toast", StateNormal, StyleDelta{
		ShadowFrom: shadow, ShadowBlur: N(20), ShadowOffsetY: N(8),
	})
}
