package native

import "testing"

func TestSystemTheme_String(t *testing.T) {
	cases := map[SystemTheme]string{
		SystemThemeUnknown: "unknown",
		SystemThemeLight:   "light",
		SystemThemeDark:    "dark",
		SystemTheme(42):    "unknown",
	}
	for th, want := range cases {
		if got := th.String(); got != want {
			t.Errorf("SystemTheme(%d).String() = %q, ждал %q", int(th), got, want)
		}
	}
}

// Нулевое значение типа обязано быть «неизвестно»: переменная, до которой
// ответ ещё не дошёл, не должна выдавать себя за светлую или тёмную тему.
func TestSystemTheme_ZeroValueIsUnknown(t *testing.T) {
	var th SystemTheme
	if th != SystemThemeUnknown {
		t.Errorf("нулевая тема %v, ждал unknown", th)
	}
}
