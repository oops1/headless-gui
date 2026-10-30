//go:build windows

package shell

import (
	"reflect"
	"testing"

	"golang.org/x/sys/windows"
)

// Список предпочтений Windows приходит многострочным буфером UTF-16 с двойным
// нулём в конце.
func TestSplitMultiString(t *testing.T) {
	enc := func(parts ...string) []uint16 {
		var b []uint16
		for _, p := range parts {
			u, _ := windows.UTF16FromString(p) // с завершающим нулём
			b = append(b, u...)
		}
		return append(b, 0) // двойной ноль
	}
	cases := []struct {
		name string
		buf  []uint16
		want []string
	}{
		{"два языка", enc("ru-RU", "en-US"), []string{"ru-RU", "en-US"}},
		{"один", enc("en-US"), []string{"en-US"}},
		{"пусто", []uint16{0, 0}, nil},
		{"нет буфера", nil, nil},
		{"без двойного нуля", []uint16{'r', 'u', 0, 'e', 'n'}, []string{"ru"}},
	}
	for _, c := range cases {
		if got := splitMultiString(c.buf); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q, ждал %q", c.name, got, c.want)
		}
	}
}

// Ответ системы — непустой нормализованный список; первый язык совпадает с
// языком интерфейса пользователя (GetUserDefaultUILanguage), по основному
// подтегу.
func TestSystemLanguages_Windows(t *testing.T) {
	langs := systemLanguages()
	if len(langs) == 0 {
		t.Skip("система не сообщила ни одного языка")
	}
	for _, l := range langs {
		if normalizeLanguageTag(l) != l {
			t.Errorf("%q не в нормализованной записи", l)
		}
	}
	if len(userPreferredUILanguages()) == 0 {
		t.Skip("GetUserPreferredUILanguages недоступна — проверен запасной путь")
	}
	// Независимая проверка: первый язык списка — язык интерфейса пользователя.
	id, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
	primary := map[uint32]string{0x19: "ru", 0x09: "en", 0x07: "de", 0x0C: "fr", 0x0A: "es"}[uint32(id)&0x3FF]
	if primary != "" && langs[0][:len(primary)] != primary {
		t.Errorf("первый язык %q, а язык интерфейса пользователя — %q", langs[0], primary)
	}
}
