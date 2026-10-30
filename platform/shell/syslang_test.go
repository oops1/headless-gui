package shell

import (
	"reflect"
	"testing"
)

// Системные языки (E11): нормализация в BCP-47 и разбор переменных среды —
// чистые функции, проверяются на любой машине.

func TestNormalizeLanguageTag(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ru_RU.UTF-8", "ru-RU"},
		{"ru_RU", "ru-RU"},
		{"ru-RU", "ru-RU"},
		{"en_US.utf8", "en-US"},
		{"EN-us", "en-US"},
		{"de", "de"},
		{"pt_BR@euro", "pt-BR"},
		{"zh_Hans_CN", "zh-Hans-CN"},
		{"zh-hant-tw", "zh-Hant-TW"},
		{"sr_RS@latin", "sr-Latn-RS"},
		{"sr@cyrillic", "sr-Cyrl"},
		{"sr-Cyrl-RS@latin", "sr-Cyrl-RS"}, // письменность в записи важнее модификатора
		{"es-419", "es-419"},               // регион из трёх цифр
		{"de-DE_phoneb", "de-DE"},          // вариант сортировки Windows
		{"ca-ES-valencia", "ca-ES"},
		{"fil", "fil"}, // трёхбуквенный язык
		{"  ru_RU  ", "ru-RU"},
		// Не языки.
		{"", ""},
		{"C", ""},
		{"POSIX", ""},
		{"C.UTF-8", ""},
		{"c", ""},
		{"x", ""},
		{"1234", ""},
		{"русский", ""},
		{"english", ""},
		{"-US", ""},
	}
	for _, c := range cases {
		if got := normalizeLanguageTag(c.in); got != c.want {
			t.Errorf("normalizeLanguageTag(%q) = %q, ждал %q", c.in, got, c.want)
		}
	}
}

// langTestEnv собирает getenv из карты.
func langTestEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestSystemLanguagesFromEnv(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"только LANG", map[string]string{"LANG": "ru_RU.UTF-8"}, []string{"ru-RU"}},
		{"LANGUAGE перед LANG", map[string]string{"LANG": "ru_RU.UTF-8", "LANGUAGE": "en_US:fr"},
			[]string{"en-US", "fr", "ru-RU"}},
		{"LANGUAGE уже содержит главный язык", map[string]string{"LANG": "ru_RU.UTF-8", "LANGUAGE": "ru_RU:en"},
			[]string{"ru-RU", "en"}},
		{"LC_ALL главнее LANG", map[string]string{"LC_ALL": "de_DE.UTF-8", "LANG": "ru_RU.UTF-8"},
			[]string{"de-DE"}},
		{"LC_MESSAGES главнее LANG", map[string]string{"LC_MESSAGES": "fr_FR.UTF-8", "LANG": "ru_RU.UTF-8"},
			[]string{"fr-FR"}},
		{"LC_ALL главнее LC_MESSAGES", map[string]string{"LC_ALL": "de_DE", "LC_MESSAGES": "fr_FR", "LANG": "ru_RU"},
			[]string{"de-DE"}},
		// Как в gettext: при локали C переводов нет, LANGUAGE тоже не действует.
		{"LANG=C игнорирует LANGUAGE", map[string]string{"LANG": "C", "LANGUAGE": "ru:en"}, nil},
		{"LC_ALL=C перекрывает LANG", map[string]string{"LC_ALL": "C", "LANG": "ru_RU.UTF-8", "LANGUAGE": "ru"}, nil},
		{"POSIX", map[string]string{"LANG": "POSIX"}, nil},
		{"C.UTF-8", map[string]string{"LANG": "C.UTF-8", "LANGUAGE": "ru"}, nil},
		{"ничего не задано", map[string]string{}, nil},
		{"LANGUAGE без локали", map[string]string{"LANGUAGE": "ru:en"}, nil},
		{"мусор и повторы в LANGUAGE", map[string]string{"LANG": "en_US.UTF-8", "LANGUAGE": ":ru:::ru_RU:C:ru:POSIX"},
			[]string{"ru", "ru-RU", "en-US"}},
		{"пробелы вокруг", map[string]string{"LANG": "  ru_RU.UTF-8  "}, []string{"ru-RU"}},
		{"пустой LC_ALL не считается", map[string]string{"LC_ALL": "", "LANG": "ru_RU"}, []string{"ru-RU"}},
	}
	for _, c := range cases {
		got := systemLanguagesFromEnv(langTestEnv(c.env))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q, ждал %q", c.name, got, c.want)
		}
	}
}

func TestDedupLanguages(t *testing.T) {
	got := dedupLanguages([]string{"ru-RU", "RU_ru", "en-US", "C", "", "en-us", "de-DE_phoneb"})
	want := []string{"ru-RU", "en-US", "de-DE"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dedupLanguages = %q, ждал %q", got, want)
	}
	if got := dedupLanguages(nil); got != nil {
		t.Errorf("dedupLanguages(nil) = %q", got)
	}
}

// Любой ответ SystemLanguages — нормализованные теги без повторов.
func TestSystemLanguages_WellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, l := range systemLanguages() {
		if normalizeLanguageTag(l) != l {
			t.Errorf("%q не в нормализованной записи", l)
		}
		if seen[l] {
			t.Errorf("повтор %q", l)
		}
		seen[l] = true
	}
}
