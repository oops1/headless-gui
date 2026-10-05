// win11strings.go — строки кнопок панели Windows 11: Task View, виджеты,
// «Не беспокоить» на колокольчике. Ключи в пространстве «desktop.*», как и у
// остальных компонентов рабочего стола; текст берётся при каждом показе
// подсказки, поэтому следует за языком без пересоздания.
package desktop

import "github.com/oops1/headless-gui/v3/widget"

// Ключи строк.
const (
	// StrTaskView — подсказка кнопки Task View.
	StrTaskView = "desktop.taskview"
	// StrWidgets — подсказка кнопки виджетов, если потребитель не дал своей.
	StrWidgets = "desktop.widgets"
	// StrTrayDND — подсказка колокольчика при включённом «Не беспокоить».
	StrTrayDND = "desktop.tray.dnd"
)

func init() {
	widget.RegisterStrings("EN", map[string]string{
		StrTaskView: "Task View",
		StrWidgets:  "Widgets",
		StrTrayDND:  "Do not disturb",
	})
	widget.RegisterStrings("RU", map[string]string{
		StrTaskView: "Представление задач",
		StrWidgets:  "Виджеты",
		StrTrayDND:  "Не беспокоить",
	})
}
