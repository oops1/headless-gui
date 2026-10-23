package window

// syslang.go — языки интерфейса, выбранные в ОС, по убыванию предпочтения
// (задание RDP-клиента E11).
//
// Язык раскладки клавиатуры (widget.Locale) и язык интерфейса (widget.Language)
// — разные вещи, а языка ОС в движке до сих пор не было: приложение с
// переводами не знало, на каком языке открыть окно. SystemLanguages отвечает на
// этот вопрос так, как его понимает система: списком, а не одним кодом, потому
// что у человека бывает «русский, затем английский».
//
// Как и systheme.go, файл без платформенного суффикса: чтение реестра и
// переменных среды лежит в syslang_<ОС>.go, а нормализация и разбор
// переменных проверяются на любой машине.

import (
	"os"
	"strings"
)

// SystemLanguages возвращает языки интерфейса ОС в порядке предпочтения, в
// записи BCP-47 («ru-RU», «en-US», «sr-Latn-RS»): язык строчными, письменность
// с заглавной, регион прописными. Без повторов. Пустой список означает «язык
// не задан» (например, переменные среды не заданы или равны C/POSIX), и тогда
// приложение выбирает язык само.
//
// Windows: GetUserPreferredUILanguages — тот список, что человек собрал в
// параметрах «Язык и регион». Linux: переменные среды по правилам gettext —
// LANGUAGE (через двоеточие: «ru:en_US:en»), затем LC_ALL, LC_MESSAGES, LANG
// (первая непустая из трёх последних задаёт главный язык; LANGUAGE при
// локали C/POSIX игнорируется, как в gettext). Кодировка («.UTF-8») и
// модификаторы («@euro») отбрасываются, «@latin» и «@cyrillic» становятся
// письменностью. macOS: те же переменные среды (системные настройки через
// NSLocale не читаются, см. syslang_darwin.go).
//
// Безопасно звать из любой горутины и до создания окна.
func SystemLanguages() []string {
	return systemLanguages()
}

// normalizeLanguageTag приводит язык из реестра или переменной среды к записи
// BCP-47. Принимает «ru_RU.UTF-8», «ru_RU@euro», «sr_RS@latin», «en-us»,
// «zh-Hans-CN», «de-DE_phoneb». "" — не язык («C», «POSIX», пусто, мусор).
//
// Лишние подтеги (варианты, расширения) отбрасываются: приложению нужны язык,
// письменность и регион, а не вся грамматика BCP-47. Модификатор @latin и
// @cyrillic становится письменностью, если её нет в самой записи.
func normalizeLanguageTag(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var modifier string
	if i := strings.IndexByte(s, '@'); i >= 0 {
		modifier = strings.ToLower(s[i+1:])
		s = s[:i]
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i] // кодировка
	}
	s = strings.ReplaceAll(s, "_", "-")
	parts := strings.Split(s, "-")
	lang := strings.ToLower(parts[0])
	if lang == "c" || lang == "posix" || !isAlpha(lang) || len(lang) < 2 || len(lang) > 3 {
		return ""
	}
	var script, region string
	for _, p := range parts[1:] {
		switch {
		case script == "" && region == "" && len(p) == 4 && isAlpha(p):
			script = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		case region == "" && ((len(p) == 2 && isAlpha(p)) || (len(p) == 3 && isDigits(p))):
			region = strings.ToUpper(p)
		}
	}
	if script == "" {
		switch modifier {
		case "latin":
			script = "Latn"
		case "cyrillic":
			script = "Cyrl"
		}
	}
	out := lang
	if script != "" {
		out += "-" + script
	}
	if region != "" {
		out += "-" + region
	}
	return out
}

func isAlpha(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i] | 0x20
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// systemLanguagesFromEnv разбирает переменные среды по правилам gettext.
// getenv — подмена os.Getenv для тестов.
//
//   - Главная локаль — первая непустая из LC_ALL, LC_MESSAGES, LANG.
//   - Если она «C» или «POSIX», языка нет вовсе: gettext в этом случае не
//     переводит, и LANGUAGE тоже игнорируется. Без всех трёх переменных — то
//     же самое.
//   - Иначе список LANGUAGE (через двоеточие) идёт первым, главная локаль —
//     после него, если её там нет.
func systemLanguagesFromEnv(getenv func(string) string) []string {
	main := ""
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := strings.TrimSpace(getenv(k)); v != "" {
			main = v
			break
		}
	}
	if normalizeLanguageTag(main) == "" {
		return nil // нет языка или C/POSIX
	}
	var out []string
	seen := map[string]bool{}
	add := func(raw string) {
		if tag := normalizeLanguageTag(raw); tag != "" && !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	for _, e := range strings.Split(getenv("LANGUAGE"), ":") {
		add(e)
	}
	add(main)
	return out
}

// systemLanguagesEnv — SystemLanguages по переменным среды процесса.
func systemLanguagesEnv() []string { return systemLanguagesFromEnv(os.Getenv) }

// dedupLanguages нормализует список и убирает повторы, сохраняя порядок.
func dedupLanguages(raw []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range raw {
		if tag := normalizeLanguageTag(r); tag != "" && !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	return out
}
