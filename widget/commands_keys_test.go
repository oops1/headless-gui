package widget

import "testing"

// Разметка должна уметь назвать новые клавиши: без этого InputBindings
// покрывает лишь часть таблицы кодов.
func TestParseKeyName_NewKeys(t *testing.T) {
	cases := map[string]KeyCode{
		"S":           KeyS,
		"o":           KeyO,
		"Numpad5":     KeyNumpad5,
		"num0":        KeyNumpad0,
		"Add":         KeyAdd,
		"divide":      KeyDivide,
		"OemPlus":     KeyOemPlus,
		"=":           KeyOemPlus,
		"comma":       KeyOemComma,
		"CapsLock":    KeyCapsLock,
		"PrintScreen": KeyPrintScreen,
		"Apps":        KeyMenu,
		"Win":         KeyWin,
		"LWin":        KeyWin,
		"RWin":        KeyWin,
		"Super":       KeyWin,
		"F13":         KeyF13,
		"Escape":      KeyEscape,
	}
	for name, want := range cases {
		if got := parseKeyName(name); got != want {
			t.Errorf("имя %q разобрано как %d, ждал %d", name, got, want)
		}
	}
	for _, bad := range []string{"нетакой", "", "f25", "numpad"} {
		if got := parseKeyName(bad); got != KeyUnknown {
			t.Errorf("мусорное имя %q дало %d", bad, got)
		}
	}
}
