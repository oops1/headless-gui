package theme

import "image/color"

// «Пуск» Windows 11: сетка закреплённых, «Рекомендуем», «Все приложения» и
// нижняя полоса «пользователь — питание».
//
// Это третий вид меню desktop.StartMenu (рядом с плоским списком и плитками
// Windows 10); профиль просит его презентером PresenterStartGrid, а компонент
// спрашивает у темы, есть ли такой, и имени темы не знает.
//
// Все цвета частей — либо прозрачные плёнки нейтрального серого (одинаково
// ложатся на светлую и тёмную поверхность), либо ссылки на токены "text",
// "border", "accent": тёмная разновидность подменяет токены и не повторяет
// стили (предел её собственных токенов — 15, см. TestBuiltinProfiles_DarkVariantsAreThin).
// Единственный новый токен — KeyField (заливка поля поиска): светлому полю на
// светлой панели нужна белая заливка, тёмному — тёмно-серая, нейтральной плёнкой
// это не выразить.

// PresenterStartGrid — имя презентера меню «Пуск» Windows 11: поиск сверху,
// сетка закреплённых приложений, «Рекомендуем», отдельный вид «Все приложения»,
// нижняя полоса. Профиль называет его в Profile.Presenters["startmenu"].
const PresenterStartGrid = "grid"

// KeyField — заливка поля ввода внутри панелей оболочки (строка поиска «Пуска»).
const KeyField Key = "field"

// win11Veil — нейтральная серая плёнка с прозрачностью a: наведение, нажатие,
// карточка. Серый 128 даёт заметную плёнку и на светлой (243), и на тёмной (32)
// поверхности.
func win11Veil(a uint8) color.RGBA { return RGBA(128, 128, 128, a) }

// declareWin11StartMenu объявляет метрики и части меню «Пуск» Windows 11. Зовётся
// в конце Windows11Profile ПЕРЕД declareWin11Materials: тот сбрасывает материал и
// тень у каждой уже объявленной части, а части «Пуска» не должны получать своей
// Mica и крупной тени.
func declareWin11StartMenu(p *Profile) {
	p.Presenters["startmenu"] = PresenterStartGrid
	p.SetColor(KeyField, RGB(252, 252, 252))

	// ── Метрики (логические пиксели при 100 %) ──────────────────────────
	p.SetMetric("startmenu.w11.width", 642).
		SetMetric("startmenu.w11.height", 726).
		SetMetric("startmenu.w11.height.min", 360).
		SetMetric("startmenu.w11.corner", 8).
		SetMetric("startmenu.w11.pad", 32).
		SetMetric("startmenu.w11.margin", 12).
		SetMetric("startmenu.w11.grid.columns", 6).
		SetMetric("startmenu.w11.grid.cell.w", 96).
		SetMetric("startmenu.w11.grid.cell.h", 84).
		SetMetric("startmenu.w11.grid.gap.y", 16).
		SetMetric("startmenu.w11.grid.icon", 32).
		SetMetric("startmenu.w11.footer.height", 64).
		SetMetric("startmenu.w11.search.top", 28).
		SetMetric("startmenu.w11.search.height", 32).
		SetMetric("startmenu.w11.search.corner", 16).
		SetMetric("startmenu.w11.search.pad", 12).
		SetMetric("startmenu.w11.search.icon", 16).
		SetMetric("startmenu.w11.section.height", 28).
		SetMetric("startmenu.w11.section.gap", 16).
		SetMetric("startmenu.w11.rec.columns", 2).
		SetMetric("startmenu.w11.rec.rows", 3).
		SetMetric("startmenu.w11.rec.row.height", 56).
		SetMetric("startmenu.w11.rec.icon", 32).
		SetMetric("startmenu.w11.dots.size", 6).
		SetMetric("startmenu.w11.dots.gap", 8).
		SetMetric("startmenu.w11.footer.avatar", 32).
		SetMetric("startmenu.w11.footer.button", 40).
		// Список «Все приложения» и результаты поиска — те же метрики, что у
		// Windows 10 (строка, заголовок буквы, поля, полоса прокрутки), с
		// размерами Windows 11.
		SetMetric("startmenu.row.height", 40).
		SetMetric("startmenu.letter.height", 40).
		SetMetric("startmenu.icon.size", 24).
		SetMetric("startmenu.row.pad", 16).
		SetMetric("startmenu.row.icon.gap", 12).
		SetMetric("scrollbar.thin.width", 3).
		SetMetric("scrollbar.thin.hover.width", 6)

	// ── Панель: тонкая рамка и поля под раскладку по метрикам ──────────
	// Корень объявлен выше общим циклом четырёх панелей; здесь он только
	// получает рамку Windows 11 (токен "border") — поля в раскладке не
	// участвуют, она считает от прямоугольника панели.
	base := p.Styles[StyleKey{Component: "startmenu", Part: "", State: StateNormal}]
	base.BorderFrom, base.BorderWidth = "border", N(1)
	p.SetStyle("startmenu", "", StateNormal, base)

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
	veil := func(a uint8) *color.RGBA { return C(win11Veil(a)) }
	dim := RGB(118, 118, 118) // приглушённый текст: контрастен и на светлом, и на тёмном
	corner := N(4)

	// plate объявляет плоскую интерактивную часть: покой прозрачен, наведение,
	// нажатие и выбор клавишами — плёнка, фокус клавиатуры рисует рамку сам
	// компонент (PaintFocusRing). rest — заливка в покое.
	plate := func(part string, rest *color.RGBA, hover, pressed, selected uint8) {
		set := func(st State, fill *color.RGBA) {
			p.SetStyle("startmenu", part, st, flat(StyleDelta{
				Fill: fill, TextFrom: "text", Corner: corner,
			}))
		}
		set(StateNormal, rest)
		set(StateHover, veil(hover))
		set(StatePressed, veil(pressed))
		set(StateActive, veil(selected))
		set(StateFocused, veil(hover))
		p.SetStyle("startmenu", part, StateDisabled, flat(StyleDelta{
			Fill: rest, Text: C(dim), Corner: corner,
		}))
	}
	none := C(RGBA(0, 0, 0, 0))

	// Заголовки разделов («Закреплено», «Рекомендуем»), вид «Назад».
	p.SetStyle("startmenu", "heading", StateNormal, flat(StyleDelta{
		TextFrom: "text", Fill: none,
		Font: &FontSpec{Size: 10, Weight: WeightSemiBold},
	}))
	// Кнопки у заголовков («Все приложения ›», «Дополнительно ›», «‹ Назад»): едва
	// заметная плёнка уже в покое — как у Windows 11.
	plate("link", veil(26), 50, 74, 50)
	// Ячейка закреплённого и строка «Рекомендуем».
	plate("pin", none, 44, 66, 44)
	plate("rec", none, 44, 66, 44)
	p.SetStyle("startmenu", "rec.sub", StateNormal, flat(StyleDelta{Text: C(dim), Fill: none}))
	// Список «Все приложения» и результаты поиска.
	plate("row", none, 44, 66, 44)
	plate("letter", none, 44, 66, 44)
	p.SetStyle("startmenu", "row.sub", StateNormal, flat(StyleDelta{Text: C(dim), Fill: none}))
	p.SetStyle("startmenu", "scrollbar", StateNormal, flat(StyleDelta{Fill: veil(150)}))
	p.SetStyle("startmenu", "scrollbar", StateHover, flat(StyleDelta{Fill: veil(210)}))

	// Строка поиска: скруглённое поле с тонкой рамкой; в фокусе (Active) рамка
	// акцентная. Наведение чуть усиливает рамку.
	search := func(st State, border StyleDelta, width float64) {
		d := StyleDelta{FillFrom: KeyField, TextFrom: "text", Corner: N(16), BorderWidth: N(width)}
		d.Border, d.BorderFrom = border.Border, border.BorderFrom
		p.SetStyle("startmenu", "search", st, flat(d))
	}
	search(StateNormal, StyleDelta{Border: veil(70)}, 1)
	search(StateHover, StyleDelta{Border: veil(120)}, 1)
	search(StatePressed, StyleDelta{Border: veil(120)}, 1)
	search(StateActive, StyleDelta{BorderFrom: KeyAccent}, 2)
	search(StateFocused, StyleDelta{BorderFrom: KeyAccent}, 2)
	p.SetStyle("startmenu", "search.hint", StateNormal, flat(StyleDelta{Text: C(dim), Fill: none}))

	// Нижняя полоса: слегка затемнённая плёнка и тонкая линия сверху (рисует
	// компонент цветом Border); кнопки пользователя и питания — как пластины.
	p.SetStyle("startmenu", "footer", StateNormal, flat(StyleDelta{
		Fill: C(RGBA(0, 0, 0, 14)), TextFrom: "text", BorderFrom: "border", BorderWidth: N(1),
	}))
	plate("footer.item", none, 44, 66, 44)
	// Аватар без картинки: серый круг со значком.
	p.SetStyle("startmenu", "avatar", StateNormal, flat(StyleDelta{Fill: veil(90), TextFrom: "text"}))
	// Точки страниц закреплённых: неактивная — плёнка, активная — цвет текста.
	p.SetStyle("startmenu", "dot", StateNormal, flat(StyleDelta{Fill: veil(120)}))
	p.SetStyle("startmenu", "dot", StateActive, flat(StyleDelta{FillFrom: "text"}))
	p.SetStyle("startmenu", "dot", StateHover, flat(StyleDelta{Fill: veil(190)}))
}
