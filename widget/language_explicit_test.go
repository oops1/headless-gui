package widget

import "testing"

// LanguageExplicit отличает выбор приложения от запасного «EN»: компоненты со
// своим языком по умолчанию (рабочий стол — русский) остаются на нём, пока
// приложение язык не выбрало.
func TestLanguageExplicit_FlipsOnSetLanguage(t *testing.T) {
	prev := Language()
	defer SetLanguage(prev)

	SetLanguage("DE")
	if !LanguageExplicit() {
		t.Fatal("после SetLanguage язык не считается выбранным")
	}
	// Выбор «EN» — тоже выбор, а не запасное значение.
	SetLanguage("EN")
	if !LanguageExplicit() {
		t.Error("явный SetLanguage(\"EN\") не считается выбором")
	}
	if got := Language(); got != "EN" {
		t.Errorf("Language() = %q", got)
	}
}
