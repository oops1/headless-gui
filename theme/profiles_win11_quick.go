package theme

import (
	"image/color"
	"time"
)

// Быстрые настройки Windows 11 24H2: презентер, метрики и части стилей.
// Вынесено из profiles.go, чтобы описание одной панели лежало в одном месте.
//
// Вид выбирает презентер, который профиль назначает компоненту
// "quicksettings": desktop.QuickSettings, найдя его (и получив модель плиток
// SetQuickActions), рисует панель 24H2 — плитки с подписью под ними, ползунки
// громкости и яркости, нижнюю строку — вместо трёх плиток прежней панели.
// Остальные профили презентера не называют и остаются с прежним видом.

// QuickSettingsPresenter — имя презентера быстрых настроек Windows 11.
// Регистрируется пакетом desktop; здесь — только имя, чтобы профиль и
// компонент называли его одинаково.
const QuickSettingsPresenter = "quicksettings.win11"

// KeyQuickPage — токен анимации перехода между главной страницей панели и
// вложенной (страница выезжает в сторону). Не объявлен — переход мгновенный.
const KeyQuickPage Key = "quicksettings.page"

// KeyQuickIconTint — флаг профиля: значки плиток Windows 11 перекрашиваются
// цветом текста плитки (по альфа-каналу значка). Windows 11 рисует значки
// плиток одним цветом — тёмным на серой плитке и светлым на акцентной; значок
// потребителя, нарисованный одним цветом, иначе пропал бы на одном из фонов.
// Потребитель с цветными значками гасит флаг: m.SetFlag(theme.KeyQuickIconTint, false).
const KeyQuickIconTint Key = "quicksettings.icon.tint"

// declareWin11QuickSettings объявляет презентер, метрики и части стилей
// панели. Зовётся ДО declareWin11Materials: части, объявленные к тому часу,
// получают от него сброс материала и тени панели (плитке не нужны ни Mica, ни
// своя крупная тень).
//
// Цвета — прозрачные серые накладки (128,128,128,α) и ссылки на токены
// (accent, surface, text): они читаются и на светлой, и на тёмной панели, так
// что Windows11Dark не переписывает ничего из этого.
func declareWin11QuickSettings(p *Profile) {
	p.Presenters["quicksettings"] = QuickSettingsPresenter

	// Размеры — логические пиксели при 100 %. Ширина панели 360, поля 24,
	// плитки 96×48 при зазоре 12: 24+3·96+2·12+24 = 360.
	p.SetFlag(KeyQuickIconTint, true)

	p.SetMetric("quicksettings.w11.width", 360).
		SetMetric("quicksettings.w11.margin", 12).
		SetMetric("quicksettings.w11.pad", 24).
		SetMetric("quicksettings.w11.pad.top", 24).
		SetMetric("quicksettings.w11.columns", 3).
		SetMetric("quicksettings.w11.rows", 2).
		SetMetric("quicksettings.w11.tile.w", 96).
		SetMetric("quicksettings.w11.tile.h", 48).
		SetMetric("quicksettings.w11.col.gap", 12).
		SetMetric("quicksettings.w11.row.gap", 12).
		SetMetric("quicksettings.w11.label.gap", 6).
		SetMetric("quicksettings.w11.label.h", 32).
		SetMetric("quicksettings.w11.chev.w", 28).
		SetMetric("quicksettings.w11.icon", 20).
		SetMetric("quicksettings.w11.grid.gap", 16).
		SetMetric("quicksettings.w11.slider.h", 40).
		SetMetric("quicksettings.w11.slider.gap", 4).
		SetMetric("quicksettings.w11.slider.zone", 32).
		SetMetric("quicksettings.w11.slider.icon", 20).
		SetMetric("quicksettings.w11.slider.track", 4).
		SetMetric("quicksettings.w11.slider.thumb", 20).
		SetMetric("quicksettings.w11.slider.value", 32).
		SetMetric("quicksettings.w11.slider.bottom", 16).
		SetMetric("quicksettings.w11.footer.h", 48).
		SetMetric("quicksettings.w11.footer.pad", 16).
		SetMetric("quicksettings.w11.footer.btn", 32).
		SetMetric("quicksettings.w11.header.h", 48).
		SetMetric("quicksettings.w11.scrollbar.w", 4)

	// Переход на вложенную страницу — ход вбок. Это украшение: при «меньше
	// движения» мгновенно (род по умолчанию — декоративный).
	p.Anims[KeyQuickPage] = AnimSpec{Duration: 220 * time.Millisecond, Curve: "out-cubic"}

	const comp = "quicksettings"
	clear := C(RGBA(0, 0, 0, 0))
	// flat — часть без унаследованных от панели заливки, рамки, тени и полей.
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
		if d.Corner == nil {
			d.Corner = N(0)
		}
		d.PadX, d.PadY = N(0), N(0)
		d.Elevation, d.Shadow = N(0), clear
		return d
	}
	set := func(part string, st State, d StyleDelta) { p.SetStyle(comp, part, st, flat(d)) }

	gray := func(a uint8) color.RGBA { return RGBA(128, 128, 128, a) }
	dim := C(gray(230))

	// Плитка в покое («выключено»): серая накладка, наведение и нажатие
	// густеют. Скругление 4 — как у плиток Windows 11.
	set("w11.tile", StateNormal, StyleDelta{Fill: C(gray(46)), TextFrom: "text", Corner: N(4)})
	set("w11.tile", StateHover, StyleDelta{Fill: C(gray(72)), TextFrom: "text", Corner: N(4)})
	set("w11.tile", StatePressed, StyleDelta{Fill: C(gray(96)), TextFrom: "text", Corner: N(4)})
	set("w11.tile", StateDisabled, StyleDelta{Fill: C(gray(24)), Text: C(gray(150)), Corner: N(4)})
	// Плитка «включено»: акцент. Состояние Active у плитки не используется:
	// «включено» — отдельная часть, чтобы наведение на включённую плитку
	// отличалось от покоя (приоритет состояний этого не позволил бы).
	set("w11.tile.on", StateNormal, StyleDelta{FillFrom: KeyAccent, TextFrom: KeyAccentText, Corner: N(4)})
	set("w11.tile.on", StateHover, StyleDelta{FillFrom: KeyAccentHover, TextFrom: KeyAccentText, Corner: N(4)})
	set("w11.tile.on", StatePressed, StyleDelta{FillFrom: KeyAccentPressed, TextFrom: KeyAccentText, Corner: N(4)})
	set("w11.tile.on", StateDisabled, StyleDelta{Fill: C(gray(24)), Text: C(gray(150)), Corner: N(4)})
	// Обводка плитки в режиме правки.
	set("w11.tile.edit", StateNormal, StyleDelta{Border: C(gray(170)), BorderWidth: N(1), Corner: N(4), Text: dim})
	// Зона «›» на плитке: тонкая затемняющая накладка под курсором. Рисуется
	// поверх плитки, поэтому в покое прозрачна.
	set("w11.tile.chevron", StateNormal, StyleDelta{TextFrom: "text"})
	set("w11.tile.chevron", StateHover, StyleDelta{Fill: C(RGBA(0, 0, 0, 28)), TextFrom: "text", Corner: N(4)})
	set("w11.tile.chevron", StatePressed, StyleDelta{Fill: C(RGBA(0, 0, 0, 52)), TextFrom: "text", Corner: N(4)})

	// Подпись под плиткой: название и вторая строка.
	set("w11.label", StateNormal, StyleDelta{TextFrom: "text"})
	set("w11.label", StateDisabled, StyleDelta{Text: dim})
	set("w11.detail", StateNormal, StyleDelta{Text: dim})
	// Заголовок вложенной страницы: крупнее и плотнее подписи плитки.
	set("w11.title", StateNormal, StyleDelta{TextFrom: "text", Font: &FontSpec{Size: 11, Weight: WeightSemiBold}})
	// Мелкий приглушённый текст: значение ползунка, заряд, подсказка правки.
	set("w11.dim", StateNormal, StyleDelta{Text: dim})

	// Кнопка-значок (звук, «›», карандаш, шестерёнка, «назад»): прозрачна, под
	// курсором — серая плашка со скруглением 4.
	set("w11.button", StateNormal, StyleDelta{TextFrom: "text", Corner: N(4)})
	set("w11.button", StateHover, StyleDelta{Fill: C(gray(60)), TextFrom: "text", Corner: N(4)})
	set("w11.button", StatePressed, StyleDelta{Fill: C(gray(90)), TextFrom: "text", Corner: N(4)})
	set("w11.button", StateDisabled, StyleDelta{Text: C(gray(150)), Corner: N(4)})
	// «Готово» режима правки: акцентная кнопка.
	set("w11.done", StateNormal, StyleDelta{FillFrom: KeyAccent, TextFrom: KeyAccentText, Corner: N(4)})
	set("w11.done", StateHover, StyleDelta{FillFrom: KeyAccentHover, TextFrom: KeyAccentText, Corner: N(4)})
	set("w11.done", StatePressed, StyleDelta{FillFrom: KeyAccentPressed, TextFrom: KeyAccentText, Corner: N(4)})

	// Ползунок: дорожка, заполненная часть, бегунок (кольцо цвета панели с
	// тонкой рамкой) и точка в нём (акцент; под курсором крупнее — размер
	// считает компонент, здесь только цвет).
	set("w11.slider.track", StateNormal, StyleDelta{Fill: C(gray(110)), Corner: N(2)})
	set("w11.slider.fill", StateNormal, StyleDelta{FillFrom: KeyAccent, Corner: N(2)})
	set("w11.slider.thumb", StateNormal, StyleDelta{FillFrom: "surface", Border: C(gray(120)), BorderWidth: N(1), Corner: N(10)})
	set("w11.slider.dot", StateNormal, StyleDelta{FillFrom: KeyAccent, Corner: N(6)})
	set("w11.slider.dot", StateHover, StyleDelta{FillFrom: KeyAccentHover, Corner: N(6)})
	set("w11.slider.dot", StatePressed, StyleDelta{FillFrom: KeyAccentPressed, Corner: N(6)})

	// Нижняя строка: полоса со своей заливкой и тонкой линией сверху.
	set("w11.footer", StateNormal, StyleDelta{Fill: C(gray(30)), Border: C(gray(52)), BorderWidth: N(1), TextFrom: "text"})

	// Тонкая полоса прокрутки сетки плиток.
	set("w11.scrollbar", StateNormal, StyleDelta{Fill: C(gray(140)), Corner: N(2)})
	set("w11.scrollbar", StateHover, StyleDelta{Fill: C(gray(200)), Corner: N(2)})
	set("w11.scrollbar", StatePressed, StyleDelta{Fill: C(gray(220)), Corner: N(2)})
}
