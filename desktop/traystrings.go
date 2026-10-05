// traystrings.go — строки трея (кнопка уведомлений, «Показать рабочий стол»).
//
// Компоненты берут текст при каждом показе подсказки, поэтому следуют за
// сменой языка без пересоздания, и берут его тем же tr() с DefaultLanguage, что
// и остальные компоненты рабочего стола: до явного widget.SetLanguage все они
// говорят на одном языке.
//
// Ключи лежат в пространстве «desktop.tray.*» — рядом с остальными ключами
// рабочего стола, а не в общем пространстве строк приложения. Прежние ключи без
// префикса (StrShowDesktop и другие) остаются алиасами: если приложение
// переопределило их у себя (widget.RegisterStrings, widget.AliasStrings), его
// текст используется, как и раньше. Встроенные переводы прежних ключей
// по-прежнему зарегистрированы, чтобы widget.Tr(StrShowDesktop) работал.
package desktop

import (
	"fmt"
	"strings"

	"github.com/oops1/headless-gui/v3/widget"
)

// Ключи строк трея.
const (
	// StrTrayShowDesktop — подсказка полоски «Показать рабочий стол».
	StrTrayShowDesktop = "desktop.tray.showDesktop"
	// StrTrayNotificationCenter — название центра уведомлений.
	StrTrayNotificationCenter = "desktop.tray.notificationCenter"
	// StrTrayNoNewNotifications — подсказка кнопки, когда уведомлений нет.
	StrTrayNoNewNotifications = "desktop.tray.noNotifications"
	// StrTrayNewNotifications — подсказка кнопки со счётчиком. Это основа
	// ключей по числу: «.one», «.few», «.many», «.other» (см. PluralForm); в
	// каждой строке один %d.
	StrTrayNewNotifications = "desktop.tray.notifications"
)

// Прежние ключи строк трея — алиасы новых, оставлены для совместимости.
const (
	// StrShowDesktop — прежний ключ StrTrayShowDesktop.
	StrShowDesktop = "ShowDesktop"
	// StrNotificationCenter — прежний ключ StrTrayNotificationCenter.
	StrNotificationCenter = "NotificationCenter"
	// StrNoNewNotifications — прежний ключ StrTrayNoNewNotifications.
	StrNoNewNotifications = "NoNewNotifications"
	// StrNewNotifications — прежний ключ счётчика; в строке один %d и одна
	// форма на любое число. Если приложение переопределило его, используется
	// его строка, иначе — формы по числу из StrTrayNewNotifications.
	StrNewNotifications = "NewNotificationsCount"
)

// trayBuiltin — встроенные переводы строк трея (новых и прежних ключей).
var trayBuiltin = map[string]map[string]string{
	"EN": {
		StrTrayShowDesktop:                 "Show desktop",
		StrTrayNotificationCenter:          "Notification center",
		StrTrayNoNewNotifications:          "No new notifications",
		StrTrayNewNotifications + ".one":   "%d new notification",
		StrTrayNewNotifications + ".other": "%d new notifications",

		StrShowDesktop:        "Show desktop",
		StrNotificationCenter: "Notification center",
		StrNoNewNotifications: "No new notifications",
		StrNewNotifications:   "New notifications: %d",
	},
	"RU": {
		StrTrayShowDesktop:                 "Показать рабочий стол",
		StrTrayNotificationCenter:          "Центр уведомлений",
		StrTrayNoNewNotifications:          "Нет новых уведомлений",
		StrTrayNewNotifications + ".one":   "%d новое уведомление",
		StrTrayNewNotifications + ".few":   "%d новых уведомления",
		StrTrayNewNotifications + ".many":  "%d новых уведомлений",
		StrTrayNewNotifications + ".other": "%d новых уведомлений",

		StrShowDesktop:        "Показать рабочий стол",
		StrNotificationCenter: "Центр уведомлений",
		StrNoNewNotifications: "Нет новых уведомлений",
		StrNewNotifications:   "Новых уведомлений: %d",
	},
}

func init() {
	for lang, table := range trayBuiltin {
		widget.RegisterStrings(lang, table)
	}
}

// builtinTray — встроенный перевод ключа для языка: язык без своей таблицы
// получает запасной (так же, как widget.TrIn).
func builtinTray(lang, key string) string {
	lang = strings.ToUpper(strings.TrimSpace(lang))
	if v, ok := trayBuiltin[lang][key]; ok {
		return v
	}
	return trayBuiltin[strings.ToUpper(widget.FallbackLanguage())][key]
}

// translated — есть ли перевод ключа (TrIn без перевода отдаёт сам ключ).
func widgetTranslated(lang, key string) bool { return widget.TrIn(lang, key) != key }

// trayLegacy возвращает строку прежнего ключа, если приложение её
// переопределило (отличается от встроенной), иначе "".
func trayLegacy(lang, legacy string) string {
	if v := widget.TrIn(lang, legacy); v != legacy && v != builtinTray(lang, legacy) {
		return v
	}
	return ""
}

// trayText возвращает строку трея: перевод нового ключа, а если приложение
// переопределило прежний ключ (и не трогало новый), — его строку.
func trayText(key, legacy string) string {
	lang := uiLanguage()
	v := tr(key)
	if v != builtinTray(lang, key) { // приложение переопределило новый ключ
		return v
	}
	if lv := trayLegacy(lang, legacy); lv != "" {
		return lv
	}
	return v
}

// trayCount возвращает подсказку со счётчиком n: форма по числу и языку; прежний
// ключ с одним шаблоном на все числа уважается, если приложение его задало.
func trayCount(n int) string {
	lang := uiLanguage()
	key := pluralKey(lang, StrTrayNewNotifications, n, func(k string) bool { return widgetTranslated(lang, k) })
	if key == "" {
		return fmt.Sprintf(widget.TrIn(lang, StrNewNotifications), n)
	}
	v := tr(key)
	if v != builtinTray(lang, key) { // приложение переопределило форму
		return fmt.Sprintf(v, n)
	}
	if lv := trayLegacy(lang, StrNewNotifications); lv != "" {
		return fmt.Sprintf(lv, n)
	}
	return fmt.Sprintf(v, n)
}
