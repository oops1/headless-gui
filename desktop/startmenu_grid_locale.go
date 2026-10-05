// startmenu_grid_locale.go — строки меню «Пуск» Windows 11 и сброс его состояния.
//
// Строки — обычные строки движка (widget.Tr) с ключами «desktop.*», русским и
// английским переводом. Они лежат отдельным файлом и регистрируются сливанием
// таблиц (widget.RegisterStrings), поэтому приложение переопределяет любой ключ
// или связывает его со своей таблицей через widget.AliasStrings
// (StartGridAliases), как и остальные строки рабочего стола.
package desktop

import "github.com/oops1/headless-gui/v3/widget"

// Ключи строк меню «Пуск» Windows 11. «Закреплено» (StrStartPinned),
// «Все приложения» (StrStartAllApps), подсказка поиска (StrSearchPlaceholder) и
// «Ничего не найдено» (StrStartNoResults) общие с меню Windows 10.
const (
	StrStartRecommended = "desktop.start.recommended" // заголовок раздела «Рекомендуем»
	StrStartMore        = "desktop.start.more"        // кнопка «Дополнительно ›»
	StrStartBack        = "desktop.start.back"        // кнопка «‹ Назад»
	StrStartPower       = "desktop.start.power"       // подсказка кнопки питания
)

// StartGridAliases — как подключить ключи меню Windows 11 к таблице приложения,
// хранящего те же надписи под своими именами (WinLine: Start.Recommended,
// Start.More, Start.Back, Start.Power):
//
//	widget.AliasStrings(desktop.StartGridAliases("Start.Recommended",
//	    "Start.More", "Start.Back", "Start.Power"))
//
// Пустое имя пропускается, и ключ остаётся со встроенным переводом.
func StartGridAliases(recommended, more, back, power string) map[string]string {
	out := map[string]string{}
	for key, own := range map[string]string{
		StrStartRecommended: recommended,
		StrStartMore:        more,
		StrStartBack:        back,
		StrStartPower:       power,
	} {
		if own != "" {
			out[key] = own
		}
	}
	return out
}

func init() {
	widget.RegisterStrings("RU", map[string]string{
		StrStartRecommended: "Рекомендуем",
		StrStartMore:        "Дополнительно",
		StrStartBack:        "Назад",
		StrStartPower:       "Питание",
	})
	widget.RegisterStrings("EN", map[string]string{
		StrStartRecommended: "Recommended",
		StrStartMore:        "More",
		StrStartBack:        "Back",
		StrStartPower:       "Power",
	})
}

// ─── Сброс состояния ─────────────────────────────────────────────────────────

// resetGrid возвращает состояние вида Windows 11 в исходное при открытии: главный
// вид, первая страница, каретка в конце запроса, перетаскивания нет.
func (m *StartMenu) resetGrid() {
	n := len([]rune(m.Query()))
	g := m.g
	g.mu.Lock()
	g.view, g.page, g.caret = StartViewMain, 0, n
	g.drag = pinDrag{}
	g.kind = m.kind()
	g.mu.Unlock()
}

// syncKind зовётся при смене темы на открытом меню: если вид меню сменился
// (плоский, плитки, сетка), состояние прежнего вида новому не годится — область,
// выбор, наведение, страница, перетаскивание сбрасываются. Меню при этом не
// пересоздаётся и остаётся открытым.
func (m *StartMenu) syncKind() {
	k := m.kind()
	g := m.g
	g.mu.Lock()
	changed := g.kind != k
	g.kind = k
	if changed {
		g.view, g.page = StartViewMain, 0
		g.drag = pinDrag{}
	}
	g.mu.Unlock()
	if !changed {
		return
	}
	v := m.v
	v.mu.Lock()
	v.hover, v.press = "", ""
	v.sel = map[startArea]string{}
	v.kbd = false
	v.drag = tileDrag{}
	v.bar = barDrag{}
	v.grid, v.gridFrom = false, ""
	v.listScroll, v.tileScroll = 0, 0
	if k == startKindGrid {
		v.area = areaSearch
	} else {
		v.area = areaList
	}
	v.mu.Unlock()
	v.gridFade.Set(0)
}
