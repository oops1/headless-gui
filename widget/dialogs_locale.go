package widget

// dialogs_locale.go — встроенная локализация стандартных диалогов.
//
// Ключи с префиксом «dlg.» регистрируются при старте для EN и RU через
// обычный механизм строковых таблиц (RegisterStrings) — приложение может
// переопределить любой ключ или добавить свой язык теми же вызовами.
// Живое переключение SetLanguage подхватывается открытыми диалогами
// (подписка на AddLanguageListener внутри диалогов).

func init() {
	// Гарантируем фолбэк, если приложение не настроило языки вообще.
	if FallbackLanguage() == "" {
		SetFallbackLanguage("EN")
	}

	RegisterStrings("EN", map[string]string{
		"dlg.ok":     "OK",
		"dlg.cancel": "Cancel",
		"dlg.yes":    "Yes",
		"dlg.no":     "No",
		"dlg.open":   "Open",
		"dlg.save":   "Save",
		"dlg.select": "Select",
		"dlg.retry":  "Retry",
		"dlg.close":  "Close",

		"dlg.title.info":     "Information",
		"dlg.title.question": "Question",
		"dlg.title.warning":  "Warning",
		"dlg.title.error":    "Error",
		"dlg.title.open":     "Open File",
		"dlg.title.save":     "Save As",
		"dlg.title.folder":   "Select Folder",
		"dlg.title.input":    "Input",
		"dlg.title.progress": "Operation in progress",

		"dlg.file.name":      "Name:",
		"dlg.file.filename":  "File name:",
		"dlg.file.filter":    "File type:",
		"dlg.file.overwrite": "A file with this name already exists — it will be overwritten.",
		"dlg.file.col.name":  "Name",
		"dlg.file.col.size":  "Size",
		"dlg.file.col.date":  "Modified",
		"dlg.file.up":        "Up",
		"dlg.file.refresh":   "Refresh",
		"dlg.place.home":     "Home",
		"dlg.place.root":     "This PC",
		"dlg.place.docs":      "Documents",
		"dlg.place.downloads": "Downloads",
	})

	RegisterStrings("RU", map[string]string{
		"dlg.ok":     "OK",
		"dlg.cancel": "Отмена",
		"dlg.yes":    "Да",
		"dlg.no":     "Нет",
		"dlg.open":   "Открыть",
		"dlg.save":   "Сохранить",
		"dlg.select": "Выбрать",
		"dlg.retry":  "Повторить",
		"dlg.close":  "Закрыть",

		"dlg.title.info":     "Информация",
		"dlg.title.question": "Вопрос",
		"dlg.title.warning":  "Предупреждение",
		"dlg.title.error":    "Ошибка",
		"dlg.title.open":     "Открыть файл",
		"dlg.title.save":     "Сохранить как",
		"dlg.title.folder":   "Выбор папки",
		"dlg.title.input":    "Ввод",
		"dlg.title.progress": "Выполнение операции",

		"dlg.file.name":      "Имя:",
		"dlg.file.filename":  "Имя файла:",
		"dlg.file.filter":    "Тип файла:",
		"dlg.file.overwrite": "Файл с таким именем уже существует — будет перезаписан.",
		"dlg.file.col.name":  "Имя",
		"dlg.file.col.size":  "Размер",
		"dlg.file.col.date":  "Изменён",
		"dlg.file.up":        "Вверх",
		"dlg.file.refresh":   "Обновить",
		"dlg.place.home":     "Домашняя",
		"dlg.place.root":     "Компьютер",
		"dlg.place.docs":      "Документы",
		"dlg.place.downloads": "Загрузки",
	})
}
