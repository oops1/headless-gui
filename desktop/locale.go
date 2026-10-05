// locale.go — строки интерфейса рабочего стола и региональные правила даты.
//
// До этого файла все надписи пакета были русскими литералами в коде: «Пуск»,
// «Закреплено», «Очистить все», названия месяцев в календаре. Теперь это
// обычные строки движка (widget.Tr) с ключами «desktop.*», русским и
// английским переводом. Приложение переопределяет любой ключ тем же
// widget.RegisterStrings, добавляет свой язык или связывает ключи со своей
// таблицей через widget.AliasStrings — так же, как для стандартных диалогов.
//
// Компоненты не хранят переведённых строк, а берут их при отрисовке: смена
// языка (widget.SetLanguage) перерисовывает кадр целиком, и надписи меняются
// без пересоздания компонентов. Там, где от строки зависит раскладка (ширина
// часов), панель задач слушает смену языка и перекладывает элементы.
package desktop

import (
	"strconv"
	"strings"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// DefaultLanguage — язык надписей рабочего стола, пока приложение не выбрало
// язык интерфейса явно (widget.SetLanguage). Рабочий стол был русским до
// появления переводов, и приложение, не знающее о локализации, видит его
// прежним. Выбрав язык, приложение получает его, в том числе «EN».
var DefaultLanguage = "RU"

// Ключи строк рабочего стола. Значения — для widget.Tr и widget.AliasStrings.
const (
	// StrStart — подпись кнопки «Пуск».
	StrStart = "desktop.start"
	// StrStartPinned и StrStartAllApps — заголовки разделов меню «Пуск».
	StrStartPinned  = "desktop.startmenu.pinned"
	StrStartAllApps = "desktop.startmenu.allApps"
	// Меню «Пуск» с боковой панелью и плитками (презентер tiles) и строка
	// поиска на панели. Соответствие ключам WinLine для widget.AliasStrings —
	// StartMenuAliases.
	StrStartRecent       = "desktop.start.recent"       // раздел «Недавно добавленные»
	StrStartExpand       = "desktop.start.expand"       // подсказка гамбургера в свёрнутой боковой панели
	StrStartCollapse     = "desktop.start.collapse"     // …и в развёрнутой
	StrStartNoResults    = "desktop.start.noResults"    // поиск ничего не нашёл
	StrStartResults      = "desktop.start.results"      // заголовок списка результатов поиска
	StrSearchPlaceholder = "desktop.search.placeholder" // подсказка в пустой строке поиска
	StrSearchLabel       = "desktop.search.label"       // подпись и подсказка значка поиска

	// StrNotifEmpty и StrNotifClearAll — центр уведомлений.
	StrNotifEmpty    = "desktop.notif.empty"
	StrNotifClearAll = "desktop.notif.clearAll"

	// Подсказки значков трея; шаблоны — для fmt.Sprintf.
	StrNetNone      = "desktop.net.none"
	StrNetConnected = "desktop.net.connected"
	StrNetNamed     = "desktop.net.named" // %s — имя сети
	StrSoundMuted   = "desktop.sound.muted"
	StrSoundLevel   = "desktop.sound.level" // %d — процент
	StrPowerAC      = "desktop.power.ac"
	StrPowerBattery = "desktop.power.battery" // %d — процент

	// Кнопки приложений: команды меню по умолчанию (DefaultAppCommands) и
	// подсказка кнопки со многими окнами.
	StrAppPin         = "desktop.app.pin"
	StrAppUnpin       = "desktop.app.unpin"
	StrAppCloseWindow = "desktop.app.closeWindow"
	StrAppCloseAll    = "desktop.app.closeAll"
	StrAppWindows     = "desktop.app.windows" // %s — название, %d — число окон
	StrAppCloseTip    = "desktop.app.closeTip"

	// Культура часов и календаря (см. DateCulture). Названия месяцев в
	// именительном падеже и сокращения дней недели — общие с виджетом выбора
	// даты (ключи date.month.N, date.wd.N, date.firstDay), чтобы язык,
	// добавленный один раз, заработал и там и здесь.
	StrClockTimeFormat = "desktop.clock.timeFormat"
	StrClockDateFormat = "desktop.clock.dateFormat"
	StrCalLongDate     = "desktop.cal.longDate"
	strCalMonthGenPref = "desktop.cal.monthGen." // + номер месяца 1..12
)

// StartMenuAliases — как подключить ключи меню «Пуск» и строки поиска к таблице
// приложения, которое хранит те же надписи под своими именами (WinLine:
// Start, RecentlyAdded, SearchPlaceholder, Expand, Collapse):
//
//	widget.AliasStrings(desktop.StartMenuAliases("Start", "RecentlyAdded",
//	    "SearchPlaceholder", "Expand", "Collapse"))
//
// Пустое имя пропускается, и ключ остаётся со встроенным переводом.
func StartMenuAliases(start, recent, placeholder, expand, collapse string) map[string]string {
	out := map[string]string{}
	for key, own := range map[string]string{
		StrStart:             start,
		StrStartRecent:       recent,
		StrSearchPlaceholder: placeholder,
		StrStartExpand:       expand,
		StrStartCollapse:     collapse,
	} {
		if own != "" {
			out[key] = own
		}
	}
	return out
}

func init() {
	// Запасной язык движка нужен всегда: язык без своих переводов получает
	// английские строки, а не голые ключи.
	if widget.FallbackLanguage() == "" {
		widget.SetFallbackLanguage("EN")
	}

	ru := map[string]string{
		StrStart:         "Пуск",
		StrStartPinned:   "Закреплено",
		StrStartAllApps:  "Все приложения",
		StrNotifEmpty:    "Новых уведомлений нет",
		StrNotifClearAll: "Очистить все",

		StrStartRecent:       "Недавно добавленные",
		StrStartExpand:       "Развернуть",
		StrStartCollapse:     "Свернуть",
		StrStartNoResults:    "Ничего не найдено",
		StrStartResults:      "Результаты поиска",
		StrSearchPlaceholder: "Чтобы начать поиск, введите здесь запрос",
		StrSearchLabel:       "Поиск",

		StrNetNone:      "Сеть: нет подключения",
		StrNetConnected: "Сеть: подключено",
		StrNetNamed:     "Сеть: %s, подключено",
		StrSoundMuted:   "Звук: выключен",
		StrSoundLevel:   "Звук: %d%%",
		StrPowerAC:      "Питание от сети",
		StrPowerBattery: "Батарея: %d%%",

		StrAppPin:         "Закрепить на панели задач",
		StrAppUnpin:       "Открепить от панели задач",
		StrAppCloseWindow: "Закрыть окно",
		StrAppCloseAll:    "Закрыть все окна",
		StrAppWindows:     "%s — окон: %d",
		StrAppCloseTip:    "Закрыть окно",

		StrClockTimeFormat: "15:04",
		StrClockDateFormat: "02.01.2006",
		StrCalLongDate:     "{d} {M} {y}",
	}
	en := map[string]string{
		StrStart:         "Start",
		StrStartPinned:   "Pinned",
		StrStartAllApps:  "All apps",
		StrNotifEmpty:    "No new notifications",
		StrNotifClearAll: "Clear all",

		StrStartRecent:       "Recently added",
		StrStartExpand:       "Expand",
		StrStartCollapse:     "Collapse",
		StrStartNoResults:    "No results found",
		StrStartResults:      "Search results",
		StrSearchPlaceholder: "Type here to search",
		StrSearchLabel:       "Search",

		StrNetNone:      "Network: not connected",
		StrNetConnected: "Network: connected",
		StrNetNamed:     "Network: %s, connected",
		StrSoundMuted:   "Sound: muted",
		StrSoundLevel:   "Sound: %d%%",
		StrPowerAC:      "Plugged in",
		StrPowerBattery: "Battery: %d%%",

		StrAppPin:         "Pin to taskbar",
		StrAppUnpin:       "Unpin from taskbar",
		StrAppCloseWindow: "Close window",
		StrAppCloseAll:    "Close all windows",
		StrAppWindows:     "%s — %d windows",
		StrAppCloseTip:    "Close window",

		StrClockTimeFormat: "3:04 PM",
		StrClockDateFormat: "1/2/2006",
		StrCalLongDate:     "{M} {d}, {y}",
	}
	ruGen := []string{"января", "февраля", "марта", "апреля", "мая", "июня",
		"июля", "августа", "сентября", "октября", "ноября", "декабря"}
	enGen := []string{"January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December"}
	for i := range ruGen {
		key := strCalMonthGenPref + strconv.Itoa(i+1)
		ru[key] = ruGen[i]
		en[key] = enGen[i]
	}
	widget.RegisterStrings("RU", ru)
	widget.RegisterStrings("EN", en)
}

// uiLanguage — язык, на котором сейчас говорит рабочий стол.
func uiLanguage() string {
	if !widget.LanguageExplicit() {
		return DefaultLanguage
	}
	return widget.Language()
}

// tr возвращает перевод ключа на текущем языке рабочего стола.
func tr(key string) string { return widget.TrIn(uiLanguage(), key) }

// trOr — перевод ключа или def, если перевода нет вовсе (ни на языке, ни на
// запасном): Tr в этом случае возвращает сам ключ.
func trOr(key, def string) string {
	if v := tr(key); v != key {
		return v
	}
	return def
}

// ─── Культура даты и времени ─────────────────────────────────────────────────

// DateCulture — региональные правила часов и календаря: как называются месяцы
// и дни, с какого дня начинается неделя, как записываются время и дата.
//
// Реализует потребитель, если ему нужны правила, которых нет в таблицах строк
// (культура региона «ru-KZ», пользовательский формат из настроек системы).
// Остальным достаточно LocaleCulture — она читает всё из строк движка и
// поэтому следует за widget.SetLanguage.
type DateCulture interface {
	// MonthName — название месяца для заголовка календаря («Август»).
	MonthName(m time.Month) string
	// MonthGenitive — месяц в форме, подходящей к числу («августа»: «27
	// августа»). Языки без падежей отдают то же, что MonthName.
	MonthGenitive(m time.Month) string
	// WeekdayShort — сокращение дня недели для шапки сетки («Пн»).
	WeekdayShort(d time.Weekday) string
	// FirstWeekday — первый день недели: первый столбец сетки календаря.
	FirstWeekday() time.Weekday
	// TimeFormat и DateFormat — раскладки time.Format для часов.
	TimeFormat() string
	DateFormat() string
	// LongDate — дата словами для свёрнутого календаря («27 августа 2026»).
	LongDate(t time.Time) string
}

// LocaleCulture — культура по умолчанию: названия, формат и первый день
// недели берутся из таблиц строк движка для текущего языка. Нулевое значение
// готово к работе; встраивается в свою реализацию, когда нужно переопределить
// один-два метода.
type LocaleCulture struct{}

var _ DateCulture = LocaleCulture{}

// MonthName — ключ date.month.N (общий с виджетом выбора даты).
func (LocaleCulture) MonthName(m time.Month) string {
	return tr("date.month." + strconv.Itoa(int(m)))
}

// MonthGenitive — ключ desktop.cal.monthGen.N; если перевода нет, название
// месяца в именительном падеже, приведённое к нижнему регистру.
func (LocaleCulture) MonthGenitive(m time.Month) string {
	key := strCalMonthGenPref + strconv.Itoa(int(m))
	if v := tr(key); v != key {
		return v
	}
	return strings.ToLower(LocaleCulture{}.MonthName(m))
}

// WeekdayShort — ключ date.wd.N, где N — номер time.Weekday (0 — воскресенье).
func (LocaleCulture) WeekdayShort(d time.Weekday) string {
	return tr("date.wd." + strconv.Itoa(int(d)))
}

// FirstWeekday — ключ date.firstDay (0 — воскресенье, 1 — понедельник); язык
// без культуры начинает неделю с понедельника.
func (LocaleCulture) FirstWeekday() time.Weekday {
	if v := tr("date.firstDay"); len(v) == 1 && v[0] >= '0' && v[0] <= '6' {
		return time.Weekday(v[0] - '0')
	}
	return time.Monday
}

// TimeFormat — ключ desktop.clock.timeFormat.
func (LocaleCulture) TimeFormat() string { return trOr(StrClockTimeFormat, "15:04") }

// DateFormat — ключ desktop.clock.dateFormat.
func (LocaleCulture) DateFormat() string { return trOr(StrClockDateFormat, "02.01.2006") }

// LongDate подставляет число, месяц и год в шаблон desktop.cal.longDate:
// {d} — число, {M} — месяц в родительном падеже, {y} — год.
func (c LocaleCulture) LongDate(t time.Time) string {
	return expandLongDate(trOr(StrCalLongDate, "{d} {M} {y}"), t, c.MonthGenitive(t.Month()))
}

// expandLongDate раскрывает шаблон длинной даты. Вынесена, чтобы свои
// реализации DateCulture могли пользоваться тем же шаблоном.
func expandLongDate(tmpl string, t time.Time, monthWord string) string {
	return strings.NewReplacer(
		"{d}", strconv.Itoa(t.Day()),
		"{M}", monthWord,
		"{y}", strconv.Itoa(t.Year()),
	).Replace(tmpl)
}

// cultureOrDefault возвращает c или культуру по умолчанию, если c не задана.
func cultureOrDefault(c DateCulture) DateCulture {
	if c == nil {
		return LocaleCulture{}
	}
	return c
}
