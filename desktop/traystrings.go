// traystrings.go — строки трея (кнопка уведомлений, «Показать рабочий стол»).
//
// Компоненты берут текст через widget.Tr при каждом показе подсказки, поэтому
// следуют за сменой языка без пересоздания. Здесь только встроенные
// переводы-умолчания: приложение вправе перекрыть любой ключ своим каталогом
// (widget.RegisterStrings переопределяет существующие ключи).
package desktop

import "github.com/oops1/headless-gui/v3/widget"

// Ключи строк трея.
const (
	// StrShowDesktop — подсказка полоски «Показать рабочий стол».
	StrShowDesktop = "ShowDesktop"
	// StrNotificationCenter — подсказка кнопки центра уведомлений без новых.
	StrNotificationCenter = "NotificationCenter"
	// StrNoNewNotifications — подсказка кнопки, когда уведомлений нет.
	StrNoNewNotifications = "NoNewNotifications"
	// StrNewNotifications — подсказка кнопки со счётчиком; в строке один %d.
	StrNewNotifications = "NewNotificationsCount"
)

func init() {
	widget.RegisterStrings("EN", map[string]string{
		StrShowDesktop:        "Show desktop",
		StrNotificationCenter: "Notification center",
		StrNoNewNotifications: "No new notifications",
		StrNewNotifications:   "New notifications: %d",
	})
	widget.RegisterStrings("RU", map[string]string{
		StrShowDesktop:        "Показать рабочий стол",
		StrNotificationCenter: "Центр уведомлений",
		StrNoNewNotifications: "Нет новых уведомлений",
		StrNewNotifications:   "Новых уведомлений: %d",
	})
}
