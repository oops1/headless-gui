package window

import "testing"

// После F10 или одиночного Alt окно уходило в модальный цикл системного меню:
// приложение получало KeyF10/KeyAlt и подсвечивало «Файл», а следующая
// стрелка вниз до него уже не доходила. Глотать надо ровно этот вход — и
// ничего больше.
func TestSwallowKeyboardMenu(t *testing.T) {
	cases := []struct {
		name           string
		wparam, lparam uintptr
		want           bool
	}{
		{"F10 или одиночный Alt", scKeymenu, 0, true},
		// Младшие биты wParam система занимает сама — сравнивать без них.
		{"то же с битами системы", scKeymenu | 0x3, 0, true},
		// Alt+Space — оконное меню: свернуть, переместить, закрыть.
		{"Alt+Space", scKeymenu, ' ', false},
		// Alt+буква — поиск мнемоники в меню окна.
		{"Alt+буква", scKeymenu, 'f', false},
		// Другие системные команды не трогаем вовсе.
		{"свернуть", 0xF020, 0, false},
		{"закрыть (Alt+F4)", 0xF060, 0, false},
	}
	for _, c := range cases {
		if got := swallowKeyboardMenu(c.wparam, c.lparam); got != c.want {
			t.Errorf("%s: %v, ждал %v", c.name, got, c.want)
		}
	}
}
