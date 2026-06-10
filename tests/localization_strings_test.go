package tests

import (
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

func TestStrings_TrFallbackAndMissing(t *testing.T) {
	widget.ClearStrings()
	widget.SetFallbackLocale("EN")
	widget.RegisterStrings("EN", map[string]string{"Hi": "Hello", "Only": "EN-only"})
	widget.RegisterStrings("RU", map[string]string{"Hi": "Привет"})

	widget.SetLanguage("RU")
	if got := widget.Tr("Hi"); got != "Привет" {
		t.Fatalf("RU Hi = %q", got)
	}
	// Нет в RU → откат к EN.
	if got := widget.Tr("Only"); got != "EN-only" {
		t.Fatalf("fallback Only = %q", got)
	}
	// Нет нигде → сам ключ.
	if got := widget.Tr("Missing"); got != "Missing" {
		t.Fatalf("missing = %q", got)
	}
	widget.SetLanguage("EN")
}

// Язык интерфейса (Language) и раскладка клавиатуры (Locale) независимы.
func TestStrings_LanguageIndependentFromKeyboardLocale(t *testing.T) {
	widget.ClearStrings()
	widget.SetFallbackLanguage("EN")
	widget.RegisterStrings("EN", map[string]string{"Hi": "Hello"})
	widget.RegisterStrings("RU", map[string]string{"Hi": "Привет"})

	// Интерфейс на русском, клавиатура (ввод) на английском — разные значения.
	widget.SetLanguage("RU")
	widget.SetLocale("EN")
	if widget.Tr("Hi") != "Привет" {
		t.Fatalf("Tr должен зависеть от Language(RU), got %q", widget.Tr("Hi"))
	}
	if widget.Locale() != "EN" {
		t.Fatalf("Locale (клавиатура) = %q, want EN", widget.Locale())
	}

	// Смена раскладки клавиатуры НЕ меняет язык интерфейса.
	widget.SetLocale("DE")
	if widget.Tr("Hi") != "Привет" {
		t.Fatalf("смена Locale не должна менять перевод, got %q", widget.Tr("Hi"))
	}
	if widget.Language() != "RU" {
		t.Fatalf("Language изменился из-за SetLocale: %q", widget.Language())
	}

	widget.SetLanguage("EN")
	widget.SetLocale("EN")
}

func TestStrings_LoadJSON(t *testing.T) {
	widget.ClearStrings()
	if err := widget.LoadStringsJSON("DE", []byte(`{"Hi":"Hallo"}`)); err != nil {
		t.Fatal(err)
	}
	if got := widget.TrIn("DE", "Hi"); got != "Hallo" {
		t.Fatalf("DE Hi = %q", got)
	}
}

func TestStrings_RemoveLanguageListener(t *testing.T) {
	widget.SetLanguage("EN")
	called := false
	id := widget.AddLanguageListener(func(string) { called = true })
	widget.RemoveLanguageListener(id)
	widget.SetLanguage("RU")
	if called {
		t.Fatal("слушатель вызван после RemoveLanguageListener")
	}
	widget.SetLanguage("EN")
}

func TestBindingScope_DisposeStopsLocUpdates(t *testing.T) {
	widget.ClearStrings()
	widget.RegisterStrings("EN", map[string]string{"DisposeKey": "Hello"})
	widget.RegisterStrings("RU", map[string]string{"DisposeKey": "Привет"})
	widget.SetLanguage("EN")

	const xaml = `<Canvas xmlns="clr">
		<TextBlock Name="lbl" Text="{Loc DisposeKey}"/>
	</Canvas>`

	_, reg, scope, err := widget.LoadUIFromXAMLBindings([]byte(xaml), nil)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	lbl := reg["lbl"].(*widget.Label)

	if lbl.Text() != "Hello" {
		t.Fatalf("начальный текст = %q, want Hello", lbl.Text())
	}

	scope.Dispose()
	widget.SetLanguage("RU")
	if lbl.Text() != "Hello" {
		t.Fatalf("после Dispose текст изменился на %q, want Hello (не должно меняться)", lbl.Text())
	}
	widget.SetLanguage("EN")
}

func TestStrings_Trf(t *testing.T) {
	widget.ClearStrings()
	widget.RegisterString("EN", "Count", "Items: %d")
	widget.SetLanguage("EN")
	if got := widget.Trf("Count", 5); got != "Items: 5" {
		t.Fatalf("Trf = %q", got)
	}
}

func TestStrings_XAMLStaticAndDynamic(t *testing.T) {
	widget.ClearStrings()
	widget.SetFallbackLocale("EN")
	widget.RegisterStrings("EN", map[string]string{"Greeting": "Hello", "Btn": "Save"})
	widget.RegisterStrings("RU", map[string]string{"Greeting": "Привет", "Btn": "Сохранить"})
	widget.SetLanguage("EN")

	const xaml = `<Canvas xmlns="clr">
		<TextBlock Name="lbl" Text="{Loc Greeting}"/>
		<Button Name="btn" Content="{Loc Btn}"/>
		<TextBlock Name="plain" Text="Untranslated"/>
	</Canvas>`

	_, reg, err := widget.LoadUIFromXAML([]byte(xaml))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	lbl := reg["lbl"].(*widget.Label)
	plain := reg["plain"].(*widget.Label)

	// Начальное значение EN.
	if lbl.Text() != "Hello" {
		t.Fatalf("initial lbl = %q, want Hello", lbl.Text())
	}
	// Обычная строка не затронута (обратная совместимость).
	if plain.Text() != "Untranslated" {
		t.Fatalf("plain text changed: %q", plain.Text())
	}

	// Смена локали → динамический перевод.
	widget.SetLanguage("RU")
	if lbl.Text() != "Привет" {
		t.Fatalf("after RU lbl = %q, want Привет", lbl.Text())
	}
	if plain.Text() != "Untranslated" {
		t.Fatalf("plain changed after locale switch: %q", plain.Text())
	}
	widget.SetLanguage("EN")
}
