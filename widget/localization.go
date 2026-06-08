// localization.go — глобальное состояние текущей локали интерфейса.
//
// Движок headless-gui не навязывает систему перевода строк, но предоставляет
// единый «текущий язык» (locale), который окна и диалоги показывают в виде
// небольшого индикатора в заголовке (напр. «EN», «RU»). Индикатор можно
// отключить для каждого окна/диалога через свойство ShowLocaleIndicator.
//
// Локаль — это произвольная короткая метка (ISO 639-1 код или раскладка ОС).
//
// Синхронизация с ОС:
//   - В headless-режиме источник истины — SetLocale (приложение управляет само).
//   - В оконном режиме пакет window подключает раскладку клавиатуры ОС:
//     при переключении системной комбинацией клавиш локаль обновляется
//     автоматически (см. SetLocale из поллера), а выбор из контекстного меню
//     переключает раскладку ОС через зарегистрированный applier.
package widget

import (
	"strings"
	"sync"
)

var (
	localeMu         sync.RWMutex
	currentLocale    = "EN"      // текущая РАСКЛАДКА КЛАВИАТУРЫ (язык ввода)
	availableLocales []string    // список доступных раскладок (для контекстного меню)
	localeListeners  []func(string) // подписчики на смену раскладки
	localeApplier    func(string) bool // применение раскладки к ОС (клавиатура)
)

// ─── Язык интерфейса (перевод строк) — НЕЗАВИСИМ от раскладки клавиатуры ─────
//
// Важно: язык интерфейса (на котором показаны надписи) и раскладка клавиатуры
// (на каком языке пользователь ВВОДИТ текст) — разные вещи. Приложение может
// быть на русском, а ввод вестись на английском или китайском. Поэтому перевод
// строк ({Loc}/Tr) управляется отдельным «языком интерфейса» (Language), а
// индикатор раскладки (Locale) отражает клавиатуру ОС.
var (
	langMu            sync.RWMutex
	currentLanguage   = "EN"
	languageListeners []func(string)
)

// SetLanguage задаёт ЯЗЫК ИНТЕРФЕЙСА (для перевода строк {Loc}/Tr) и уведомляет
// подписчиков. НЕ влияет на раскладку клавиатуры (см. SetLocale). Потокобезопасно.
func SetLanguage(code string) {
	code = normLocale(code)
	langMu.Lock()
	changed := code != currentLanguage
	currentLanguage = code
	var ls []func(string)
	if changed {
		ls = append(ls, languageListeners...)
	}
	langMu.Unlock()
	for _, l := range ls {
		l(code)
	}
}

// Language возвращает текущий язык интерфейса. Потокобезопасно.
func Language() string {
	langMu.RLock()
	defer langMu.RUnlock()
	return currentLanguage
}

// AddLanguageListener подписывает колбэк на смену языка интерфейса. Колбэк
// вызывается из горутины, изменившей язык. Потокобезопасно.
func AddLanguageListener(fn func(code string)) {
	if fn == nil {
		return
	}
	langMu.Lock()
	languageListeners = append(languageListeners, fn)
	langMu.Unlock()
}

// normLocale нормализует код локали: обрезает пробелы, приводит к верхнему регистру.
func normLocale(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// SetLocale задаёт текущую локаль интерфейса (например "EN", "RU", "DE") и
// уведомляет подписчиков. Используется как приложением (headless), так и
// поллером раскладки ОС (оконный режим) для отражения системного состояния.
// НЕ переключает раскладку ОС — для пользовательского выбора см. RequestLocale.
// Потокобезопасно.
func SetLocale(code string) {
	code = normLocale(code)
	localeMu.Lock()
	changed := code != currentLocale
	currentLocale = code
	var ls []func(string)
	if changed {
		ls = append(ls, localeListeners...)
	}
	localeMu.Unlock()
	for _, l := range ls {
		l(code)
	}
}

// Locale возвращает текущую локаль интерфейса. Потокобезопасно.
func Locale() string {
	localeMu.RLock()
	defer localeMu.RUnlock()
	return currentLocale
}

// RequestLocale — пользовательское намерение сменить локаль (напр. из
// контекстного меню индикатора). Если зарегистрирован applier — переключает
// раскладку ОС; в любом случае немедленно отражает выбор через SetLocale.
func RequestLocale(code string) {
	code = normLocale(code)
	localeMu.RLock()
	ap := localeApplier
	localeMu.RUnlock()
	if ap != nil {
		ap(code)
	}
	SetLocale(code)
}

// SetAvailableLocales задаёт список доступных локалей (для контекстного меню).
// Значения нормализуются и дедуплицируются с сохранением порядка.
// В оконном режиме пакет window заполняет его раскладками клавиатуры ОС.
func SetAvailableLocales(codes []string) {
	seen := make(map[string]bool, len(codes))
	var out []string
	for _, c := range codes {
		c = normLocale(c)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	localeMu.Lock()
	availableLocales = out
	localeMu.Unlock()
}

// AvailableLocales возвращает копию списка доступных локалей.
// Если список пуст, возвращает текущую локаль как единственный элемент.
func AvailableLocales() []string {
	localeMu.RLock()
	defer localeMu.RUnlock()
	if len(availableLocales) == 0 {
		if currentLocale != "" {
			return []string{currentLocale}
		}
		return nil
	}
	out := make([]string, len(availableLocales))
	copy(out, availableLocales)
	return out
}

// AddLocaleListener подписывает колбэк на смену локали (вызывается при каждом
// изменении). Удобно для приложений, переключающих переводы строк, и для
// перерисовки UI. Потокобезопасно. Колбэк вызывается из горутины, изменившей
// локаль (может быть поллер ОС) — оборачивайте доступ к UI-состоянию мьютексом.
func AddLocaleListener(fn func(code string)) {
	if fn == nil {
		return
	}
	localeMu.Lock()
	localeListeners = append(localeListeners, fn)
	localeMu.Unlock()
}

// SetLocaleApplier регистрирует функцию применения локали к ОС (переключение
// раскладки клавиатуры). Вызывается пакетом window. fn возвращает true, если
// раскладка успешно переключена. Передайте nil, чтобы снять applier.
func SetLocaleApplier(fn func(code string) bool) {
	localeMu.Lock()
	localeApplier = fn
	localeMu.Unlock()
}
