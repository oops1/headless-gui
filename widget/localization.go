// localization.go — глобальное состояние текущей локали интерфейса.
//
// Движок headless-gui не навязывает систему перевода строк, но предоставляет
// единый «текущий язык» (locale), который окна и диалоги показывают в виде
// небольшого индикатора в заголовке (напр. «EN», «RU»). Индикатор можно
// отключить для каждого окна/диалога через свойство ShowLocaleIndicator.
//
// Локаль — это произвольная короткая метка (ISO 639-1 код или любая строка).
// Приложение само решает, что она означает, и меняет её через SetLocale.
package widget

import (
	"strings"
	"sync"
)

var (
	localeMu      sync.RWMutex
	currentLocale = "EN" // текущая локаль интерфейса (по умолчанию английская)
)

// SetLocale задаёт текущую локаль интерфейса (например "EN", "RU", "DE").
// Значение нормализуется: обрезаются пробелы, приводится к верхнему регистру.
// Потокобезопасно. Окна/диалоги с включённым индикатором покажут новую метку
// при следующей отрисовке.
func SetLocale(code string) {
	code = strings.ToUpper(strings.TrimSpace(code))
	localeMu.Lock()
	currentLocale = code
	localeMu.Unlock()
}

// Locale возвращает текущую локаль интерфейса. Потокобезопасно.
func Locale() string {
	localeMu.RLock()
	defer localeMu.RUnlock()
	return currentLocale
}
