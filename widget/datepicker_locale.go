package widget

// datepicker_locale.go — культура и строки поля даты.
//
// Формат даты (раскладка time.Format) и первый день недели (0 — воскресенье,
// 1 — понедельник) лежат в тех же таблицах строк, что и надписи: приложение,
// добавившее язык, добавляет и его культуру вызовом RegisterStrings, а язык без
// своей культуры получает ISO 8601 и понедельник. Дни недели — по номеру
// time.Weekday.

func init() {
	RegisterStrings("RU", map[string]string{
		"date.format":      "02.01.2006",
		"date.firstDay":    "1",
		"date.placeholder": "дд.мм.гггг",
		"date.a11y":        "Выбор даты",
		"date.month.1":     "Январь",
		"date.month.2":     "Февраль",
		"date.month.3":     "Март",
		"date.month.4":     "Апрель",
		"date.month.5":     "Май",
		"date.month.6":     "Июнь",
		"date.month.7":     "Июль",
		"date.month.8":     "Август",
		"date.month.9":     "Сентябрь",
		"date.month.10":    "Октябрь",
		"date.month.11":    "Ноябрь",
		"date.month.12":    "Декабрь",
		"date.wd.0":        "Вс",
		"date.wd.1":        "Пн",
		"date.wd.2":        "Вт",
		"date.wd.3":        "Ср",
		"date.wd.4":        "Чт",
		"date.wd.5":        "Пт",
		"date.wd.6":        "Сб",
	})
	RegisterStrings("EN", map[string]string{
		"date.format":      "01/02/2006",
		"date.firstDay":    "0",
		"date.placeholder": "mm/dd/yyyy",
		"date.a11y":        "Date picker",
		"date.month.1":     "January",
		"date.month.2":     "February",
		"date.month.3":     "March",
		"date.month.4":     "April",
		"date.month.5":     "May",
		"date.month.6":     "June",
		"date.month.7":     "July",
		"date.month.8":     "August",
		"date.month.9":     "September",
		"date.month.10":    "October",
		"date.month.11":    "November",
		"date.month.12":    "December",
		"date.wd.0":        "Su",
		"date.wd.1":        "Mo",
		"date.wd.2":        "Tu",
		"date.wd.3":        "We",
		"date.wd.4":        "Th",
		"date.wd.5":        "Fr",
		"date.wd.6":        "Sa",
	})
}
