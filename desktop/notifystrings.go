// notifystrings.go — строки центра уведомлений в стиле Windows 10.
//
// Как и остальные строки пакета (locale.go), это ключи widget.Tr с русским и
// английским переводом, которые читаются при отрисовке: смена языка меняет
// надписи на открытом центре, ничего не пересоздавая.
//
// Потребитель, у которого эти слова уже есть в своём каталоге, не заводит их
// второй раз, а связывает ключи движка со своими (widget.AliasStrings):
//
//	widget.AliasStrings(map[string]string{
//	    desktop.StrNotifManage:   "ManageNotifications",
//	    desktop.StrNotifClear:    "ClearAllNotifications",
//	    desktop.StrNotifExpand:   "Expand",
//	    desktop.StrNotifCollapse: "Collapse",
//	})
//
// Отдельный файл, а не locale.go, чтобы набор строк панели не смешивался с
// общими.
package desktop

import "github.com/oops1/headless-gui/v3/widget"

// Ключи строк центра уведомлений.
const (
	// StrNotifManage — ссылка в заголовке центра («Управление уведомлениями»).
	StrNotifManage = "desktop.notif.manage"
	// StrNotifClear — ссылка над быстрыми действиями («Очистить уведомления»).
	// Плоский центр по-прежнему пишет «Очистить все» (StrNotifClearAll).
	StrNotifClear = "desktop.notif.clear"
	// StrNotifExpand и StrNotifCollapse — ссылка, раскрывающая и сворачивающая
	// сетку быстрых действий.
	StrNotifExpand   = "desktop.notif.expand"
	StrNotifCollapse = "desktop.notif.collapse"
	// StrNotifReply — кнопка отправки ответа, если действие не назвало свою.
	StrNotifReply = "desktop.notif.reply"
	// StrNotifReplyHint — подсказка пустого поля ответа.
	StrNotifReplyHint = "desktop.notif.replyHint"
	// StrNotifYesterday — время вчерашнего уведомления.
	StrNotifYesterday = "desktop.notif.yesterday"
)

func init() {
	widget.RegisterStrings("RU", map[string]string{
		StrNotifManage:    "Управление уведомлениями",
		StrNotifClear:     "Очистить уведомления",
		StrNotifExpand:    "Развернуть",
		StrNotifCollapse:  "Свернуть",
		StrNotifReply:     "Ответить",
		StrNotifReplyHint: "Введите ответ",
		StrNotifYesterday: "Вчера",
	})
	widget.RegisterStrings("EN", map[string]string{
		StrNotifManage:    "Manage notifications",
		StrNotifClear:     "Clear notifications",
		StrNotifExpand:    "Expand",
		StrNotifCollapse:  "Collapse",
		StrNotifReply:     "Reply",
		StrNotifReplyHint: "Type a reply",
		StrNotifYesterday: "Yesterday",
	})
}
