package base

import "testing"

// keepStrings запоминает таблицы переводов и запасной язык и возвращает их
// после теста.
//
// ClearStrings стирает ВСЕ таблицы, в том числе встроенные подписи диалогов и
// системного меню окна, которые регистрируются один раз при загрузке пакета и
// заново не появятся. Тест, вызвавший её и не вернувший таблицы, ломает тесты,
// идущие после него (go test -shuffle=on): «нет перевода dlg.file.encoding для
// EN». Звать перед ClearStrings.
func keepStrings(t *testing.T) {
	t.Helper()
	stringsMu.Lock()
	saved := make(map[string]map[string]string, len(stringTables))
	for lang, tbl := range stringTables {
		cp := make(map[string]string, len(tbl))
		for k, v := range tbl {
			cp[k] = v
		}
		saved[lang] = cp
	}
	fb := fallbackLanguage
	stringsMu.Unlock()
	t.Cleanup(func() {
		stringsMu.Lock()
		stringTables = saved
		fallbackLanguage = fb
		stringsMu.Unlock()
	})
}
