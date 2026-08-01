package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// xamlPath — разметка showcase относительно каталога пакета.
const xamlPath = "../../assets/ui/showcase.xaml"

// locRe вылавливает ссылки {Loc ключ} из разметки.
var locRe = regexp.MustCompile(`"\{Loc ([^"}]*)\}"`)

// attrRe — атрибуты вида Имя="значение".
var attrRe = regexp.MustCompile(`(\w+)\s*=\s*"([^"]*)"`)

// локализуемые атрибуты — те, чьё значение видит пользователь.
var localizable = map[string]bool{
	"Text": true, "Header": true, "Title": true,
	"ToolTip": true, "Content": true, "Placeholder": true,
}

func readXAML(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(xamlPath)
	if err != nil {
		t.Skipf("разметка недоступна: %v", err)
	}
	return string(data)
}

// TestEveryLocKeyHasRussian — у каждого ключа {Loc …} из разметки есть перевод.
// Пропущенный перевод не ломает сборку (Tr вернёт сам ключ), поэтому ловим
// его тестом: иначе в русском интерфейсе тихо появится английская надпись.
func TestEveryLocKeyHasRussian(t *testing.T) {
	xaml := readXAML(t)
	missing := map[string]bool{}
	for _, m := range locRe.FindAllStringSubmatch(xaml, -1) {
		key := strings.TrimSpace(m[1])
		if key == "" {
			t.Errorf("пустой ключ в %q", m[0])
			continue
		}
		// Перевод может жить в любой из двух таблиц: разметка и код
		// регистрируются в одну локаль (см. registerShowcaseStrings).
		_, inXAML := showcaseRU[key]
		_, inCode := showcaseCodeRU[key]
		if !inXAML && !inCode {
			missing[key] = true
		}
	}
	if len(missing) > 0 {
		t.Errorf("нет русского перевода для %d ключей разметки:", len(missing))
		for k := range missing {
			t.Errorf("  %q", k)
		}
	}
}

// TestNoUntranslatedRussianInXAML — в разметке не осталось русского текста
// «мимо» механизма локализации: иначе при переключении на английский часть
// подписей останется русской (ровно та беда, ради которой всё затевалось).
func TestNoUntranslatedRussianInXAML(t *testing.T) {
	xaml := readXAML(t)
	var bad []string
	for _, m := range attrRe.FindAllStringSubmatch(xaml, -1) {
		name, val := m[1], m[2]
		if !localizable[name] || strings.HasPrefix(strings.TrimSpace(val), "{") {
			continue
		}
		if strings.ContainsFunc(val, isCyrillic) {
			bad = append(bad, name+"="+strconv.Quote(val))
		}
	}
	if len(bad) > 0 {
		t.Errorf("русский текст мимо {Loc} (%d):", len(bad))
		for _, s := range bad {
			t.Errorf("  %s", s)
		}
	}
}

// TestCodeStringsHaveRussian — ключи, которыми пользуется код, тоже переведены.
// Проверяем выборочно самые заметные: строка состояния, трей-меню, диалоги.
func TestCodeStringsHaveRussian(t *testing.T) {
	for _, key := range []string{
		"Ready — Ctrl+S to save",
		"Saved at %s",
		"Language:",
		"Show",
		"Minimise",
		"Exit",
		"Layout restored",
		"Menu: %s (section %d, item %d)",
		"Document saved. A backup copy has been created.",
	} {
		if _, ok := showcaseCodeRU[key]; !ok {
			t.Errorf("нет русского перевода для ключа кода %q", key)
		}
	}
}

// TestTablesDoNotConflict — один и тот же ключ не должен получать разные
// переводы в двух таблицах: обе регистрируются в одну локаль, и вторая молча
// перекрыла бы первую.
func TestTablesDoNotConflict(t *testing.T) {
	for key, ru := range showcaseCodeRU {
		if other, ok := showcaseRU[key]; ok && other != ru {
			t.Errorf("ключ %q переведён по-разному:\n  разметка: %q\n  код:      %q", key, other, ru)
		}
	}
}

func isCyrillic(r rune) bool {
	return (r >= 'а' && r <= 'я') || (r >= 'А' && r <= 'Я') || r == 'ё' || r == 'Ё'
}
