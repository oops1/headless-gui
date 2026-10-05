package theme

import (
	"image/color"
	"time"
)

// Панель задач Windows 11: размеры и вид кнопок, Task View, виджеты, поиск,
// группа значков трея и индикаторы кнопок окон (пилюля, прогресс, счётчик,
// «внимание»).
//
// Вынесено из profiles.go по тому же правилу, что и profiles_win10_*.go: набор
// большой, а общая часть профиля (скругления, акцент, панели) остаётся там.
//
// Цвета плашек — токены, а не литералы стилей: тёмный профиль меняет только
// их (film.hover, film.pressed, field.fill) и остаётся коротким. Прочее
// объявлено ссылками на токены "text", "border", "accent" и производные, поэтому
// следует и за темой, и за сменой акцента.

const (
	// KeyFilmHover — плёнка подсветки кнопки под курсором: чёрная на светлой
	// панели, белая на тёмной.
	KeyFilmHover Key = "film.hover"
	// KeyFilmPressed — плёнка нажатой кнопки и кнопки, чья панель открыта.
	KeyFilmPressed Key = "film.pressed"
	// KeyFieldFill — заливка поля поиска на панели.
	KeyFieldFill Key = "field.fill"
)

// win11TaskbarPlates — компоненты панели, у которых подсветка — плашка со
// скруглением 4 и плёнкой из токенов (кнопки панели и значки трея).
var win11TaskbarPlates = []string{
	"startbutton", "taskbutton", "clock",
	"tray.network", "tray.volume", "tray.power", "tray.icon", "tray.notifications", "tray.showdesktop",
	"tray.chevron",
}

// declareWin11Taskbar объявляет панель Windows 11 по замерам плана (100 %):
// кнопки 40×40 с зазором 4 и значком 24 в панели 48, подсветка со скруглением 4,
// пилюля 3 px (запущено — 6 серым, активно — 16 акцентом), поле поиска 32 со
// скруглением 16, группа «сеть + звук + питание» одной плашкой.
//
// Зовётся ПОСЛЕ inheritTrayStyles: правит уже унаследованные от tray.volume
// стили кнопки уведомлений и обобщённого значка.
func declareWin11Taskbar(p *Profile) {
	p.SetColor(KeyFilmHover, RGBA(0, 0, 0, 20)).
		SetColor(KeyFilmPressed, RGBA(0, 0, 0, 32)).
		SetColor(KeyFieldFill, RGBA(255, 255, 255, 170))

	p.SetMetric("taskbar.item.height", 40).
		SetMetric("taskbutton.height", 40).
		SetMetric("taskbutton.width", 40).
		SetMetric("taskbutton.icon.size", 24).
		// Пилюля вместо метки Windows 10: 3 px высотой под кнопкой, у самого края
		// панели; запущено — 6 px серым, активно — 16 px акцентом.
		SetMetric("taskbutton.pill.height", 3).
		SetMetric("taskbutton.pill.idle", 6).
		SetMetric("taskbutton.pill.active", 16).
		SetMetric("taskbutton.pill.offset", -2).
		SetMetric("taskbutton.pill.idle.opacity", 0.6).
		SetMetric("taskbutton.progress.height", 4).
		SetMetric("taskbutton.badge.size", 14).
		SetMetric("taskbutton.attention.blinks", 3).
		// Поиск: поле 200×32 со скруглением 16, кнопка «значок и подпись» и
		// кнопка из одного значка.
		SetMetric("search.width", 200).
		SetMetric("search.label.width", 112).
		SetMetric("search.height", 32).
		SetMetric("search.icon.width", 40).
		SetMetric("search.icon.size", 16).
		SetMetric("search.pad", 12).
		SetMetric("search.icon.gap", 8).
		SetMetric("taskview.icon.size", 20).
		SetMetric("widgets.icon.size", 24).
		SetMetric("widgets.text.gap", 8).
		SetMetric("widgets.width.max", 170).
		// Трей: плашка значков 40 высотой; три значка 16 с шагом 24 в плашке с
		// полем 4; колокольчик 40 шириной со счётчиком-кружком 12.
		SetMetric("tray.item.height", 40).
		SetMetric("tray.group.pad", 4).
		SetMetric("tray.group.gap", 0).
		SetMetric("tray.group.height", 40).
		SetMetric("tray.badge.size", 12)

	p.SetFlag("taskbutton.attention", true).
		SetFlag("tray.bell", true).
		SetFlag("search.hint.short", true)

	p.Anims["taskbutton.pill"] = AnimSpec{Duration: 150 * time.Millisecond, Curve: "out-cubic"}
	// Одно мигание «внимания»; линейная кривая — мигание равномерное.
	p.Anims["taskbutton.attention"] = AnimSpec{Duration: 450 * time.Millisecond, Curve: "linear"}

	// ── Плашки уже объявленных компонентов: скругление 4 и плёнки из токенов ──
	isPlate := map[string]bool{}
	for _, c := range win11TaskbarPlates {
		isPlate[c] = true
	}
	for k, d := range p.Styles {
		if k.Part != "" || !isPlate[k.Component] {
			continue
		}
		if d.Corner != nil {
			d.Corner = N(4)
		}
		// Цвет значков трея — ссылкой на токен "text": литерал светлой темы
		// (чёрный) оставил бы значки чёрными на тёмной панели.
		if d.Text != nil {
			d.Text = nil
			d.TextFrom = "text"
		}
		switch k.State {
		case StateHover:
			d.FillFrom = KeyFilmHover
		case StatePressed, StateActive:
			d.FillFrom = KeyFilmPressed
		}
		p.Styles[k] = d
	}
	// Состояний «нажата» и «панель открыта» у значков трея и часов не было:
	// нажатая плашка оставалась такой же, как в покое.
	for _, comp := range []string{"clock", "tray.network", "tray.volume", "tray.power", "tray.icon", "tray.notifications"} {
		for _, st := range []State{StatePressed, StateActive} {
			k := StyleKey{Component: comp, State: st}
			if _, ok := p.Styles[k]; !ok {
				p.Styles[k] = StyleDelta{FillFrom: KeyFilmPressed, Corner: N(4)}
			}
		}
	}
	// Значок трея с полем 4: шаг 24 в группе; колокольчик — плашка 40.
	for _, comp := range []string{"tray.network", "tray.volume", "tray.power", "tray.icon"} {
		k := StyleKey{Component: comp, State: StateNormal}
		d := p.Styles[k]
		d.PadX = N(4)
		p.Styles[k] = d
	}
	{
		k := StyleKey{Component: "tray.notifications", State: StateNormal}
		d := p.Styles[k]
		d.PadX = N(12)
		p.Styles[k] = d
	}
	// Кружок счётчика на колокольчике: цвет акцента, цифры контрастные.
	p.SetStyle("tray.notifications", "badge", StateNormal, StyleDelta{
		FillFrom: KeyAccent, TextFrom: KeyAccentText, PadX: N(3), Font: &FontSpec{Size: 7, Weight: WeightSemiBold},
	})

	// ── Кнопки окон: индикаторы ──────────────────────────────────────────────
	clear := C(RGBA(0, 0, 0, 0))
	p.SetStyle("taskbutton", "pill", StateNormal, StyleDelta{FillFrom: "text", Fill: clear, BorderWidth: N(0)})
	p.SetStyle("taskbutton", "pill", StateActive, StyleDelta{FillFrom: KeyAccent})
	p.SetStyle("taskbutton", "progress", StateNormal, StyleDelta{Fill: C(RGBA(0, 0, 0, 120)), Corner: N(2)})
	p.SetStyle("taskbutton", "progress.fill", StateNormal, StyleDelta{FillFrom: KeyAccent, Corner: N(2)})
	p.SetStyle("taskbutton", "progress.paused", StateNormal, StyleDelta{Fill: C(RGB(255, 185, 0)), Corner: N(2)})
	p.SetStyle("taskbutton", "progress.error", StateNormal, StyleDelta{Fill: C(RGB(196, 43, 28)), Corner: N(2)})
	p.SetStyle("taskbutton", "badge", StateNormal, StyleDelta{
		FillFrom: KeyAccent, TextFrom: KeyAccentText, PadX: N(4), Font: &FontSpec{Size: 7, Weight: WeightSemiBold},
	})
	p.SetStyle("taskbutton", "attention", StateNormal, StyleDelta{Fill: C(RGBA(255, 140, 0, 235)), Corner: N(4)})

	// ── Новые кнопки: плашка 4, плёнки из токенов ────────────────────────────
	plate := func(comp string, padX float64) {
		p.SetStyle(comp, "", StateNormal, StyleDelta{TextFrom: "text", Fill: clear, Corner: N(4), PadX: N(padX)})
		p.SetStyle(comp, "", StateHover, StyleDelta{FillFrom: KeyFilmHover})
		p.SetStyle(comp, "", StatePressed, StyleDelta{FillFrom: KeyFilmPressed})
		p.SetStyle(comp, "", StateActive, StyleDelta{FillFrom: KeyFilmPressed})
	}
	plate("taskview", 10)
	plate("widgets", 8)
	plate("tray.group", 0)
	p.SetStyle("widgets", "temperature", StateNormal, StyleDelta{Font: &FontSpec{Size: 9.5, Weight: WeightSemiBold}})
	p.SetStyle("widgets", "caption", StateNormal, StyleDelta{Font: &FontSpec{Size: 8.5}})

	// ── Поиск ────────────────────────────────────────────────────────────────
	// Поле: заливка из токена, тонкая рамка; наведение подсвечивает рамку, фокус
	// и «открыто» — акцентом.
	field := func(st State, border StyleDelta) {
		d := StyleDelta{TextFrom: "text", BorderWidth: N(1), Corner: N(16), PadX: N(0)}
		d.FillFrom = KeyFieldFill
		d.BorderFrom = border.BorderFrom
		p.SetStyle("searchbox", "", st, d)
	}
	field(StateNormal, StyleDelta{BorderFrom: "border"})
	field(StateHover, StyleDelta{BorderFrom: KeyAccentLight})
	field(StatePressed, StyleDelta{BorderFrom: KeyAccentLight})
	field(StateActive, StyleDelta{BorderFrom: KeyAccent})
	field(StateFocused, StyleDelta{BorderFrom: KeyAccent})
	p.SetStyle("searchbox", "hint", StateNormal, StyleDelta{Text: C(RGBA(128, 128, 128, 220))})
	// Кнопка из одного значка — плашка, как у остальных кнопок панели.
	p.SetStyle("searchbox", "icon", StateNormal, StyleDelta{
		TextFrom: "text", Fill: clear, BorderWidth: N(0), Corner: N(4),
	})
	p.SetStyle("searchbox", "icon", StateHover, StyleDelta{FillFrom: KeyFilmHover})
	p.SetStyle("searchbox", "icon", StatePressed, StyleDelta{FillFrom: KeyFilmPressed})
	p.SetStyle("searchbox", "icon", StateActive, StyleDelta{FillFrom: KeyFilmPressed})
}

// win11DarkFilms — плёнки и поле тёмной панели: те же токены, другие значения.
// Единственное, что тёмный профиль знает о панели.
func win11DarkFilms() (hover, pressed, field color.RGBA) {
	return RGBA(255, 255, 255, 24), RGBA(255, 255, 255, 14), RGBA(255, 255, 255, 20)
}
