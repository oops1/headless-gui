package theme

import "image/color"

// Панели оболочки Windows 10: акрил и стили «Пуска» и центра уведомлений.
//
// Вынесено из profiles.go, потому что этот набор объявляется дважды — для
// тёмного режима и для светлого (флаг taskbar.light), а описывать одно и то
// же вручную значило бы разводить два списка, расходящихся при первой правке.

// win10Acrylic — подложка акрила Windows 10: размытие под слоем, затемняющая
// подкраска tint и слабое зерно поверх. fallback — непрозрачный цвет, который
// рисуется, когда размывать нечем.
func win10Acrylic(tint, fallback color.RGBA) *BackdropSpec {
	return &BackdropSpec{
		Mode: BackdropBlur, Radius: 20, Tint: tint,
		Noise: 0.02, Fallback: fallback,
	}
}

// win10Shell — палитра панелей оболочки Windows 10 в одном режиме. Режимов два
// (тёмный и светлый), набор стилей один: declareWin10Panels объявляет его
// дважды — обычными правилами и правилами под флагом taskbar.light.
type win10Shell struct {
	tint, fallback               color.RGBA // акрил панели
	sidebarTint, sidebarFallback color.RGBA // акрил боковой панели «Пуска»
	text, dim                    color.RGBA // основной и приглушённый текст
	hover, pressed, selected     color.RGBA // плёнки строк и кнопок
	card, cardHover              color.RGBA // карточка уведомления
	action, actionHover          color.RGBA // кнопка действия и плитка «выкл.»
	focus, tileHover             color.RGBA // рамка фокуса и рамка плитки под курсором
	border                       color.RGBA // кромка панели
	accentLink                   Key        // токен цвета ссылок
	searchFill, searchHover      color.RGBA // строка поиска на панели: покой и наведение
	searchBorder, searchText     color.RGBA // её рамка и вводимый текст
	searchHint                   color.RGBA // подсказка-заполнитель
	thumb, thumbHover            color.RGBA // ползунок тонкой полосы прокрутки списков
}

var (
	win10ShellDark = win10Shell{
		tint: RGBA(24, 24, 24, 225), fallback: RGB(30, 30, 30),
		sidebarTint: RGBA(10, 10, 10, 205), sidebarFallback: RGB(22, 22, 22),
		text: RGB(255, 255, 255), dim: RGBA(255, 255, 255, 170),
		hover: RGBA(255, 255, 255, 26), pressed: RGBA(255, 255, 255, 40), selected: RGBA(255, 255, 255, 38),
		card: RGBA(255, 255, 255, 22), cardHover: RGBA(255, 255, 255, 34),
		action: RGBA(255, 255, 255, 30), actionHover: RGBA(255, 255, 255, 44),
		focus: RGB(255, 255, 255), tileHover: RGBA(255, 255, 255, 150),
		border:     RGBA(255, 255, 255, 24),
		accentLink: KeyAccentLight,
		// Строка поиска светлая и в тёмной панели: так её рисует Windows 10.
		searchFill: RGB(242, 242, 242), searchHover: RGB(255, 255, 255),
		searchBorder: RGB(194, 194, 194), searchText: RGB(24, 24, 24),
		searchHint: RGB(96, 96, 96),
		thumb:      RGBA(255, 255, 255, 110), thumbHover: RGBA(255, 255, 255, 170),
	}
	win10ShellLight = win10Shell{
		tint: RGBA(243, 243, 243, 225), fallback: RGB(243, 243, 243),
		sidebarTint: RGBA(232, 232, 232, 232), sidebarFallback: RGB(232, 232, 232),
		text: RGB(0, 0, 0), dim: RGBA(0, 0, 0, 150),
		hover: RGBA(0, 0, 0, 20), pressed: RGBA(0, 0, 0, 34), selected: RGBA(0, 0, 0, 28),
		card: RGBA(255, 255, 255, 170), cardHover: RGBA(255, 255, 255, 225),
		action: RGBA(0, 0, 0, 18), actionHover: RGBA(0, 0, 0, 30),
		focus: RGB(0, 0, 0), tileHover: RGBA(0, 0, 0, 110),
		border:     RGBA(0, 0, 0, 30),
		accentLink: KeyAccentDark,
		searchFill: RGB(255, 255, 255), searchHover: RGB(255, 255, 255),
		searchBorder: RGB(170, 170, 170), searchText: RGB(24, 24, 24),
		searchHint: RGB(96, 96, 96),
		thumb:      RGBA(0, 0, 0, 110), thumbHover: RGBA(0, 0, 0, 170),
	}
)

// declareWin10Panels объявляет стили панелей оболочки Windows 10 через set
// (SetStyle либо условное правило).
//
// Это ЧАСТИ компонентов, которых у плоских панелей нет, поэтому существующий
// вид не меняется. «Пуск» (startmenu): panel, sidebar, sidebar.item, row,
// letter, tile, tile.group. Центр уведомлений (notificationcenter): panel,
// header, link, group, card, action, quick.tile, quick.tile.on.
// Интерактивные части объявлены во всех состояниях (покой, наведение,
// нажатие, активно, фокус, отключено), плитки — с рамкой наведения.
//
// Часть наследует от компонента целиком заливку, рамку и тень плоской
// панели; строкам и кнопкам они не нужны, поэтому flat их обнуляет.
func declareWin10Panels(set func(comp, part string, st State, d StyleDelta), pal win10Shell) {
	clear := C(RGBA(0, 0, 0, 0))
	// flat — часть без унаследованных рамки, тени и подъёма.
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
	// button объявляет состояния плоской интерактивной части.
	button := func(comp, part string, rest, hover, pressed, selected color.RGBA) {
		text := C(pal.text)
		set(comp, part, StateNormal, flat(StyleDelta{Fill: C(rest), Text: text}))
		set(comp, part, StateHover, flat(StyleDelta{Fill: C(hover), Text: text}))
		set(comp, part, StatePressed, flat(StyleDelta{Fill: C(pressed), Text: text}))
		set(comp, part, StateActive, flat(StyleDelta{Fill: C(selected), Text: text}))
		set(comp, part, StateFocused, flat(StyleDelta{
			Fill: C(rest), Text: text, Border: C(pal.focus), BorderWidth: N(1),
		}))
		set(comp, part, StateDisabled, flat(StyleDelta{Fill: C(rest), Text: C(pal.dim)}))
	}
	// tile объявляет плитку, залитую акцентом: наведение — светлая рамка,
	// нажатие — тёмный оттенок.
	tile := func(comp, part string, fill, hoverFill, pressedFill Key) {
		set(comp, part, StateNormal, flat(StyleDelta{FillFrom: fill, TextFrom: KeyAccentText}))
		set(comp, part, StateHover, flat(StyleDelta{
			FillFrom: hoverFill, TextFrom: KeyAccentText, Border: C(pal.tileHover), BorderWidth: N(2),
		}))
		set(comp, part, StatePressed, flat(StyleDelta{FillFrom: pressedFill, TextFrom: KeyAccentText}))
		set(comp, part, StateFocused, flat(StyleDelta{
			FillFrom: fill, TextFrom: KeyAccentText, Border: C(pal.focus), BorderWidth: N(2),
		}))
	}
	none := RGBA(0, 0, 0, 0)

	// ── «Пуск» ──────────────────────────────────────────────────────────
	// Поля в один пиксель — под рамку панели: боковая панель и списки
	// рисуются внутри, не затирая кромку.
	set("startmenu", "panel", StateNormal, flat(StyleDelta{
		Backdrop: win10Acrylic(pal.tint, pal.fallback), Text: C(pal.text),
		Border: C(pal.border), BorderWidth: N(1), PadX: N(1), PadY: N(1),
	}))
	set("startmenu", "sidebar", StateNormal, flat(StyleDelta{
		Backdrop: win10Acrylic(pal.sidebarTint, pal.sidebarFallback), Text: C(pal.text),
	}))
	button("startmenu", "sidebar.item", none, pal.hover, pal.pressed, pal.selected)
	button("startmenu", "row", none, pal.hover, pal.pressed, pal.selected)
	button("startmenu", "letter", none, pal.hover, pal.pressed, pal.selected)
	tile("startmenu", "tile", KeyAccent, KeyAccent, KeyAccentPressed)
	set("startmenu", "tile.group", StateNormal, flat(StyleDelta{Text: C(pal.dim)}))
	// Вторая строка приложения («Система») и подпись плитки — приглушённым.
	set("startmenu", "row.sub", StateNormal, flat(StyleDelta{Text: C(pal.dim)}))
	// Тонкая полоса прокрутки списков «Пуска»: цвет ползунка в покое и под
	// курсором; ширина — метрики scrollbar.thin.width / .hover.width.
	set("startmenu", "scrollbar", StateNormal, flat(StyleDelta{Fill: C(pal.thumb)}))
	set("startmenu", "scrollbar", StateHover, flat(StyleDelta{Fill: C(pal.thumbHover)}))

	// ── Строка поиска на панели задач ───────────────────────────────────
	// Часть "" — рамка поля; "hint" — подсказка-заполнитель; "icon" — режим
	// «только значок» (прозрачная кнопка панели, как «Пуск»).
	box := func(st State, fill color.RGBA, border StyleDelta) {
		d := StyleDelta{Fill: C(fill), Text: C(pal.searchText), BorderWidth: N(1)}
		d.Border, d.BorderFrom = border.Border, border.BorderFrom
		set("searchbox", "", st, flat(d))
	}
	box(StateNormal, pal.searchFill, StyleDelta{Border: C(pal.searchBorder)})
	box(StateHover, pal.searchHover, StyleDelta{Border: C(pal.searchBorder)})
	box(StatePressed, pal.searchHover, StyleDelta{Border: C(pal.searchBorder)})
	box(StateActive, pal.searchHover, StyleDelta{BorderFrom: KeyAccent})
	box(StateFocused, pal.searchHover, StyleDelta{BorderFrom: KeyAccent})
	set("searchbox", "hint", StateNormal, flat(StyleDelta{Text: C(pal.searchHint)}))
	button("searchbox", "icon", none, pal.hover, pal.pressed, pal.selected)

	// ── Центр уведомлений ───────────────────────────────────────────────
	set("notificationcenter", "panel", StateNormal, flat(StyleDelta{
		Backdrop: win10Acrylic(pal.tint, pal.fallback), Text: C(pal.text),
		Border: C(pal.border), BorderWidth: N(1),
	}))
	set("notificationcenter", "header", StateNormal, flat(StyleDelta{Text: C(pal.text)}))
	// Ссылки («Управление уведомлениями», «Очистить уведомления»): цвет
	// акцента, при наведении и нажатии — его варианты. Часть называется link,
	// а не clear: clear — кнопка плоского центра, и её стиль не трогаем.
	set("notificationcenter", "link", StateNormal, flat(StyleDelta{TextFrom: pal.accentLink}))
	set("notificationcenter", "link", StateHover, flat(StyleDelta{TextFrom: KeyAccentHover}))
	set("notificationcenter", "link", StatePressed, flat(StyleDelta{TextFrom: KeyAccentPressed}))
	set("notificationcenter", "link", StateFocused, flat(StyleDelta{
		TextFrom: pal.accentLink, Border: C(pal.focus), BorderWidth: N(1),
	}))
	button("notificationcenter", "group", none, pal.hover, pal.pressed, pal.selected)
	button("notificationcenter", "card", pal.card, pal.cardHover, pal.card, pal.cardHover)
	button("notificationcenter", "action", pal.action, pal.actionHover, pal.pressed, pal.actionHover)
	button("notificationcenter", "quick.tile", pal.action, pal.actionHover, pal.pressed, pal.actionHover)
	tile("notificationcenter", "quick.tile.on", KeyAccent, KeyAccentHover, KeyAccentPressed)
	// toast, field, glyph, scrollbar — см. profiles_win10_notify.go.
	declareWin10NotificationParts(set, pal)
}

// PresenterStartTiles — имя презентера меню «Пуск» с боковой панелью, списком
// приложений и плитками. Профиль называет его в Profile.Presenters["startmenu"];
// компонент меню спрашивает у темы, есть ли у неё такой презентер, и не знает,
// какая именно тема активна. Темы без него получают прежний плоский список.
const PresenterStartTiles = "tiles"

// declareWin10StartMenu объявляет метрики меню «Пуск» Windows 10 и строки
// поиска на панели задач и назначает меню презентер плиток.
//
// Все размеры логические, в пикселях при масштабе 100 %. Стартовые значения —
// из задания WinLine и замеров по снимкам Windows 10: ширина меню на них 653 =
// 48 (боковая панель) + 260 (список) + 21 (поле) + 308 (плитки) + 16 (поле).
// Метрики плоского меню (startmenu.width, startmenu.row.height, icon.size) этот
// набор переопределяет: плоское меню в Windows 10 больше не рисуется, а размеры
// строк и значков у обоих устройств разные.
func declareWin10StartMenu(p *Profile) {
	p.Presenters["startmenu"] = PresenterStartTiles

	p.SetMetric("startmenu.sidebar.collapsed", 48).
		SetMetric("startmenu.sidebar.expanded", 256).
		SetMetric("startmenu.sidebar.icon.size", 20).
		SetMetric("startmenu.list.width", 260).
		SetMetric("startmenu.row.height", 36).
		SetMetric("startmenu.letter.height", 36).
		SetMetric("startmenu.icon.size", 24).
		SetMetric("startmenu.corner", 0).
		// Строка списка: поле слева от значка и зазор между значком и названием.
		SetMetric("startmenu.row.pad", 20).
		SetMetric("startmenu.row.icon.gap", 8).
		// Поля области плиток; высота меню: желаемая и наименьшая. Желаемая
		// высота ужимается под экран за вычетом панели задач.
		SetMetric("startmenu.tiles.pad.left", 21).
		SetMetric("startmenu.tiles.pad.right", 16).
		SetMetric("startmenu.tiles.pad.top", 18).
		SetMetric("startmenu.tiles.pad.bottom", 12).
		SetMetric("startmenu.tiles.columns", 6).
		SetMetric("startmenu.height", 700).
		SetMetric("startmenu.height.min", 320).
		SetMetric("startmenu.margin", 0).
		// Плитки: единица сетки, зазор, заголовок группы.
		SetMetric("tile.unit", 48).
		SetMetric("tile.gap", 4).
		SetMetric("tile.group.header", 32).
		SetMetric("tile.group.header.gap", 5).
		SetMetric("tile.group.gap", 4).
		// Тонкая полоса прокрутки списков меню.
		SetMetric("scrollbar.thin.width", 4).
		SetMetric("scrollbar.thin.hover.width", 8).
		// Строка поиска на панели: ширина поля, ширина кнопки-значка, значок.
		SetMetric("search.width", 344).
		SetMetric("search.icon.width", 48).
		SetMetric("search.icon.size", 16).
		SetMetric("search.pad", 12).
		SetMetric("search.icon.gap", 8)
}
