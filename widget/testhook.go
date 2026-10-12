package widget

import "github.com/oops1/headless-gui/v3/internal/testhook"

// Сброс языка для тестовых помощников соседних пакетов (internal/testreset).
// Публичного способа снять «язык выбран явно» нет и быть не должно: для
// приложения выбор необратим. Тестам же нужно начинать каждый тест с
// состояния «приложение язык не выбирало», иначе результат зависит от того,
// какой тест успел вызвать SetLanguage раньше.
func init() {
	testhook.ResetLanguage = func() {
		langMu.Lock()
		currentLanguage = "EN"
		langExplicit = false
		langMu.Unlock()
	}
}
