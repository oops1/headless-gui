package base

import (
	"sync/atomic"

	"github.com/oops1/headless-gui/v3/internal/testhook"
)

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

// Состояние для тестов подпакетов (widget/xaml и др.): они не видят
// неэкспортированного, а вернуть измеритель, таблицы переводов и посчитать
// подписчиков языка после теста нужно.
func init() {
	testhook.SaveTextMeasurer = func() func() {
		prev := uiTextMeasurer.Load()
		measurersMu.Lock()
		prevBase := baseMeasurer
		measurersMu.Unlock()
		return func() {
			// Вернуть надо оба: baseMeasurer и опубликованный uiTextMeasurer.
			measurersMu.Lock()
			baseMeasurer = prevBase
			measurersMu.Unlock()
			if prev != nil {
				uiTextMeasurer.Store(prev)
			} else {
				// В atomic.Value nil не положить: до первой публикации он пуст.
				uiTextMeasurer = atomic.Value{}
			}
			BumpTextMetricsRev()
		}
	}
	testhook.SaveStrings = func() func() {
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
		return func() {
			stringsMu.Lock()
			stringTables = saved
			fallbackLanguage = fb
			stringsMu.Unlock()
		}
	}
	testhook.LanguageListeners = func() int {
		langMu.RLock()
		defer langMu.RUnlock()
		return len(languageListeners)
	}
}
