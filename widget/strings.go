// strings.go — ресурс локализованных строк (таблицы переводов) для UI.
//
// Механизм НЕ ломает обратную совместимость: обычные строки в XAML и в коде
// остаются как есть. Локализация подключается явно — через таблицы переводов и
// markup-расширение {Loc Key} в XAML (или функцию Tr в коде). Если перевод не
// зарегистрирован, Tr возвращает сам ключ, поэтому добавление {Loc ...} в новый
// XAML безопасно даже без таблиц.
//
// Цепочка поиска перевода Tr(key):
//  1. таблица текущей локали (widget.Locale())
//  2. таблица «запасной» локали (SetFallbackLocale, по умолчанию "EN")
//  3. сам ключ (key) — как маркер отсутствующего перевода
//
// Пример (код):
//
//	widget.RegisterStrings("EN", map[string]string{"Greeting": "Hello"})
//	widget.RegisterStrings("RU", map[string]string{"Greeting": "Привет"})
//	widget.SetLocale("RU")
//	label.SetText(widget.Tr("Greeting")) // "Привет"
//
// Пример (XAML, динамически при смене локали):
//
//	<TextBlock Text="{Loc Greeting}"/>
package widget

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	stringsMu        sync.RWMutex
	stringTables     = map[string]map[string]string{} // language → key → value
	fallbackLanguage = "EN"
)

// SetFallbackLanguage задаёт запасной язык интерфейса для поиска перевода
// (по умолчанию "EN"). Пустая строка отключает запасной поиск.
func SetFallbackLanguage(language string) {
	stringsMu.Lock()
	fallbackLanguage = normLocale(language)
	stringsMu.Unlock()
}

// FallbackLanguage возвращает текущий запасной язык интерфейса.
func FallbackLanguage() string {
	stringsMu.RLock()
	defer stringsMu.RUnlock()
	return fallbackLanguage
}

// SetFallbackLocale — устаревший псевдоним SetFallbackLanguage (язык интерфейса).
// Deprecated: используйте SetFallbackLanguage.
func SetFallbackLocale(language string) { SetFallbackLanguage(language) }

// FallbackLocale — устаревший псевдоним FallbackLanguage.
// Deprecated: используйте FallbackLanguage.
func FallbackLocale() string { return FallbackLanguage() }

// RegisterStrings добавляет (сливает) таблицу переводов для локали. Существующие
// ключи перезаписываются переданными. Потокобезопасно.
func RegisterStrings(locale string, table map[string]string) {
	locale = normLocale(locale)
	if locale == "" || len(table) == 0 {
		return
	}
	stringsMu.Lock()
	defer stringsMu.Unlock()
	t := stringTables[locale]
	if t == nil {
		t = make(map[string]string, len(table))
		stringTables[locale] = t
	}
	for k, v := range table {
		t[k] = v
	}
}

// RegisterString регистрирует один перевод. Потокобезопасно.
func RegisterString(locale, key, value string) {
	RegisterStrings(locale, map[string]string{key: value})
}

// ClearStrings удаляет все зарегистрированные таблицы (полезно в тестах).
func ClearStrings() {
	stringsMu.Lock()
	stringTables = map[string]map[string]string{}
	stringsMu.Unlock()
}

// Tr возвращает перевод key для текущего ЯЗЫКА ИНТЕРФЕЙСА (Language) с откатом
// к запасному языку и возвратом самого key, если перевод не найден.
// Не зависит от раскладки клавиатуры (Locale).
func Tr(key string) string {
	return TrIn(Language(), key)
}

// TrIn возвращает перевод key для заданного языка интерфейса.
func TrIn(language, key string) string {
	language = normLocale(language)
	stringsMu.RLock()
	defer stringsMu.RUnlock()
	if t := stringTables[language]; t != nil {
		if v, ok := t[key]; ok {
			return v
		}
	}
	if fallbackLanguage != "" && fallbackLanguage != language {
		if t := stringTables[fallbackLanguage]; t != nil {
			if v, ok := t[key]; ok {
				return v
			}
		}
	}
	return key
}

// Trf возвращает перевод key, отформатированный аргументами (fmt.Sprintf).
// Полезно для строк-шаблонов: RegisterString("RU","Items","Элементов: %d").
func Trf(key string, args ...interface{}) string {
	return fmt.Sprintf(Tr(key), args...)
}

// Translation возвращает перевод и признак его наличия (без отката к key).
func Translation(locale, key string) (string, bool) {
	locale = normLocale(locale)
	stringsMu.RLock()
	defer stringsMu.RUnlock()
	if t := stringTables[locale]; t != nil {
		if v, ok := t[key]; ok {
			return v, true
		}
	}
	return "", false
}

// LoadStringsJSON регистрирует переводы локали из JSON-объекта {"key":"value"}.
func LoadStringsJSON(locale string, jsonData []byte) error {
	var table map[string]string
	if err := json.Unmarshal(jsonData, &table); err != nil {
		return fmt.Errorf("strings: parse json for %q: %w", locale, err)
	}
	RegisterStrings(locale, table)
	return nil
}

// LoadStringsFile загружает переводы локали из JSON-файла.
func LoadStringsFile(locale, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("strings: read %q: %w", path, err)
	}
	return LoadStringsJSON(locale, data)
}

// LoadStringsDir загружает все *.json из директории; локаль = имя файла без
// расширения (например ru.json → "RU", en-US.json → "EN-US"). Пропускает
// нечитаемые/некорректные файлы, возвращая первую возникшую ошибку (если была).
func LoadStringsDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("strings: read dir %q: %w", dir, err)
	}
	var firstErr error
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.EqualFold(filepath.Ext(name), ".json") {
			continue
		}
		locale := strings.TrimSuffix(name, filepath.Ext(name))
		if err := LoadStringsFile(locale, filepath.Join(dir, name)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// parseLocKey извлекает ключ из markup-расширения {Loc Key}.
func parseLocKey(expr string) string {
	inner := strings.TrimSpace(expr)
	inner = strings.TrimPrefix(inner, "{")
	inner = strings.TrimSuffix(inner, "}")
	inner = strings.TrimSpace(strings.TrimPrefix(inner, "Loc"))
	// Возможный синтаксис {Loc Key=Greeting} или {Loc Greeting}.
	if i := strings.Index(inner, "="); i >= 0 {
		if strings.EqualFold(strings.TrimSpace(inner[:i]), "Key") {
			inner = inner[i+1:]
		}
	}
	return strings.Trim(strings.TrimSpace(inner), `'"`)
}
