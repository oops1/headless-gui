// plural.go — множественное число в строках рабочего стола.
//
// Счётчик «новых уведомлений» в русском имеет три формы (1 уведомление,
// 2–4 уведомления, 5 и больше уведомлений), в английском две. Строка с одним
// %d для этого не годится, поэтому у строки со счётчиком несколько ключей —
// по форме: «ключ.one», «ключ.few», «ключ.many», «ключ.other». Нужная форма
// выбирается по языку и числу (PluralForm); нет перевода формы — берётся
// «ключ.other», затем «ключ.many», затем «ключ.one».
package desktop

import "strings"

// Имена форм множественного числа (категории CLDR).
const (
	PluralOne   = "one"   // 1, 21, 31… (в русском) и ровно 1 (в английском)
	PluralFew   = "few"   // 2–4, 22–24… (в русском)
	PluralMany  = "many"  // 0, 5–20, 25–30… (в русском)
	PluralOther = "other" // всё остальное (в английском — любое число, кроме 1)
)

// PluralForm возвращает форму слова при числе n для языка интерфейса language
// («RU», «EN»…). Правила заведены для русского и родственных ему восточно-
// славянских (UK, BE), польского и чешского со словацким; остальные языки —
// «один / другое». Язык, которому нужны иные правила, ставит их тем же
// ключом: переводит форму one/few/many/other в нужные ему ключи.
func PluralForm(language string, n int) string {
	if n < 0 {
		n = -n
	}
	n10, n100 := n%10, n%100
	switch strings.ToUpper(strings.TrimSpace(language)) {
	case "RU", "UK", "BE":
		switch {
		case n10 == 1 && n100 != 11:
			return PluralOne
		case n10 >= 2 && n10 <= 4 && (n100 < 12 || n100 > 14):
			return PluralFew
		}
		return PluralMany
	case "PL":
		switch {
		case n == 1:
			return PluralOne
		case n10 >= 2 && n10 <= 4 && (n100 < 12 || n100 > 14):
			return PluralFew
		}
		return PluralMany
	case "CS", "SK":
		switch {
		case n == 1:
			return PluralOne
		case n >= 2 && n <= 4:
			return PluralFew
		}
		return PluralOther
	}
	if n == 1 {
		return PluralOne
	}
	return PluralOther
}

// pluralKey выбирает ключ формы для числа n: ключ.форма, а если для неё нет
// записи (exists), то ключ.other, ключ.many, ключ.one. Пусто — ни одной формы.
func pluralKey(language, base string, n int, exists func(key string) bool) string {
	for _, form := range []string{PluralForm(language, n), PluralOther, PluralMany, PluralOne} {
		if k := base + "." + form; exists(k) {
			return k
		}
	}
	return ""
}
