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

// localeEntry — запись слушателя раскладки с уникальным id.
type localeEntry struct {
	id int
	fn func(string)
}

var (
	localeMu         sync.RWMutex
	currentLocale    = "EN"        // текущая РАСКЛАДКА КЛАВИАТУРЫ (язык ввода)
	availableLocales []string      // список доступных раскладок (для контекстного меню)
	localeListeners  []localeEntry // подписчики на смену раскладки
	localeNextID     int
	localeApplier    func(string) bool // применение раскладки к ОС (клавиатура)
)

// ─── Язык интерфейса (перевод строк) — НЕЗАВИСИМ от раскладки клавиатуры ─────
//
// Важно: язык интерфейса (на котором показаны надписи) и раскладка клавиатуры
// (на каком языке пользователь ВВОДИТ текст) — разные вещи. Приложение может
// быть на русском, а ввод вестись на английском или китайском. Поэтому перевод
// строк ({Loc}/Tr) управляется отдельным «языком интерфейса» (Language), а
// индикатор раскладки (Locale) отражает клавиатуру ОС.

// langEntry — запись слушателя языка с уникальным id.
type langEntry struct {
	id int
	fn func(string)
}

var (
	langMu            sync.RWMutex
	currentLanguage   = "EN"
	languageListeners []langEntry
	langNextID        int
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
		for _, e := range languageListeners {
			ls = append(ls, e.fn)
		}
	}
	langMu.Unlock()
	for _, l := range ls {
		l(code)
	}
	if changed {
		notifyUIChanged() // надписи могли смениться (on-demand рендер)
	}
}

// Language возвращает текущий язык интерфейса. Потокобезопасно.
func Language() string {
	langMu.RLock()
	defer langMu.RUnlock()
	return currentLanguage
}

// AddLanguageListener подписывает колбэк на смену языка интерфейса и возвращает
// id для отписки. Колбэк вызывается из горутины, изменившей язык. Потокобезопасно.
func AddLanguageListener(fn func(code string)) int {
	if fn == nil {
		return -1
	}
	langMu.Lock()
	id := langNextID
	langNextID++
	languageListeners = append(languageListeners, langEntry{id: id, fn: fn})
	langMu.Unlock()
	return id
}

// RemoveLanguageListener отписывает слушателя по id (no-op, если нет). Потокобезопасно.
func RemoveLanguageListener(id int) {
	langMu.Lock()
	for i, e := range languageListeners {
		if e.id == id {
			languageListeners = append(languageListeners[:i], languageListeners[i+1:]...)
			break
		}
	}
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
		for _, e := range localeListeners {
			ls = append(ls, e.fn)
		}
	}
	localeMu.Unlock()
	for _, l := range ls {
		l(code)
	}
	if changed {
		notifyUIChanged() // индикатор раскладки перерисуется (on-demand рендер)
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

// AddLocaleListener подписывает колбэк на смену локали и возвращает id для
// отписки. Потокобезопасно. Колбэк вызывается из горутины, изменившей
// локаль (может быть поллер ОС) — оборачивайте доступ к UI-состоянию мьютексом.
func AddLocaleListener(fn func(code string)) int {
	if fn == nil {
		return -1
	}
	localeMu.Lock()
	id := localeNextID
	localeNextID++
	localeListeners = append(localeListeners, localeEntry{id: id, fn: fn})
	localeMu.Unlock()
	return id
}

// RemoveLocaleListener отписывает слушателя по id (no-op, если нет). Потокобезопасно.
func RemoveLocaleListener(id int) {
	localeMu.Lock()
	for i, e := range localeListeners {
		if e.id == id {
			localeListeners = append(localeListeners[:i], localeListeners[i+1:]...)
			break
		}
	}
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
