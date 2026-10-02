package widget

import "testing"

// Мнемоника — подчёркнутая буква в подписи пункта, второй способ дойти до
// пункта меню кроме мыши. Меню движка этого не умело вовсе: клавиатурой до
// пункта можно было добраться только стрелками.

func TestSplitMnemonic(t *testing.T) {
	cases := []struct {
		src   string
		label string
		key   rune
		pos   int
	}{
		{"_Файл", "Файл", 'ф', 0},
		{"Сохранить _как", "Сохранить как", 'к', len("Сохранить ")},
		{"_Open", "Open", 'o', 0},
		// Двойное подчёркивание — сам знак: имя файла в контекстном меню.
		{"my__file.txt", "my_file.txt", 0, -1},
		{"Без мнемоники", "Без мнемоники", 0, -1},
		// Подчёркивание перед пробелом ничего не помечает — помечать нечего.
		{"Файл _ копия", "Файл  копия", 0, -1},
		// Подчёркивание в конце подписи остаётся как есть.
		{"конец_", "конец_", 0, -1},
		// Вторая мнемоника в той же подписи — опечатка: берём первую.
		{"_Файл _копия", "Файл копия", 'ф', 0},
	}
	for _, c := range cases {
		label, key, pos := splitMnemonic(c.src)
		if label != c.label || key != c.key || pos != c.pos {
			t.Errorf("splitMnemonic(%q) = %q, %q, %d; ждал %q, %q, %d",
				c.src, label, key, pos, c.label, c.key, c.pos)
		}
	}
}

func TestMatchesMnemonic_Latin(t *testing.T) {
	if !matchesMnemonic('o', KeyEvent{Code: KeyO, Pressed: true}) {
		t.Error("латинская мнемоника не поймана по коду клавиши")
	}
	if matchesMnemonic('o', KeyEvent{Code: KeyP, Pressed: true}) {
		t.Error("поймана чужая клавиша")
	}
}

// Кириллическую мнемонику иначе не поймать: до приложения доходит код
// латинской буквы (так сделано, чтобы Ctrl+S работал и в русской раскладке),
// а какая раскладка включена, движок не знает.
func TestMatchesMnemonic_Cyrillic(t *testing.T) {
	if !matchesMnemonic('ф', KeyEvent{Code: KeyA, Pressed: true}) {
		t.Error("Ф не поймана по клавише A — на ней стоит Ф в ЙЦУКЕН")
	}
	if !matchesMnemonic('п', KeyEvent{Code: KeyG, Pressed: true}) {
		t.Error("П не поймана по клавише G")
	}
	if matchesMnemonic('ф', KeyEvent{Code: KeyF, Pressed: true}) {
		t.Error("Ф поймана по клавише F — на ней стоит А")
	}
}

// Если бэкенд прислал символ, он главнее догадок по физической клавише.
func TestMatchesMnemonic_RuneWins(t *testing.T) {
	if !matchesMnemonic('ю', KeyEvent{Code: KeyUnknown, Rune: 'Ю', Pressed: true}) {
		t.Error("символ события не сопоставлен (и регистр должен игнорироваться)")
	}
}

func TestMatchesMnemonic_Digits(t *testing.T) {
	if !matchesMnemonic('1', KeyEvent{Code: Key1, Pressed: true}) {
		t.Error("цифровая мнемоника не поймана")
	}
}

// ─── Меню ──────────────────────────────────────────────────────────────────

func TestPopupMenu_MnemonicSelectsItem(t *testing.T) {
	var clicked string
	m := NewPopupMenu()
	m.UseMnemonics = true
	m.SetItems([]MenuItem{
		{Text: "_Открыть", OnClick: func() { clicked = "открыть" }},
		{Text: "_Сохранить", OnClick: func() { clicked = "сохранить" }},
	})
	m.Show(10, 10)

	// В ЙЦУКЕН С стоит на клавише C — её и нажимаем.
	m.OnKeyEvent(KeyEvent{Code: KeyC, Pressed: true})
	if clicked != "сохранить" {
		t.Errorf("выбрано %q, ждал «сохранить»", clicked)
	}
	if m.IsOpen() {
		t.Error("меню осталось открытым после выбора пункта")
	}
}

// Без флага подчёркивания — обычный текст, и буквы ничего не выбирают: иначе
// у приложений, которые пишут в подписях настоящие подчёркивания, пункты
// начали бы срабатывать от набора текста.
func TestPopupMenu_MnemonicsOffByDefault(t *testing.T) {
	var clicked bool
	m := NewPopupMenu()
	m.SetItems([]MenuItem{{Text: "_Открыть", OnClick: func() { clicked = true }}})
	m.Show(10, 10)

	m.OnKeyEvent(KeyEvent{Code: KeyO, Pressed: true})
	if clicked {
		t.Error("пункт сработал при выключенных мнемониках")
	}
	if !m.IsOpen() {
		t.Error("меню закрылось")
	}
	if got := m.Items()[0].Text; got != "_Открыть" {
		t.Errorf("текст пункта %q — подчёркивание не должно пропадать", got)
	}
}

// Буква у двух пунктов — нажатие не выбирает ни одного, а переставляет
// подсветку: выбрать за человека наугад нельзя.
func TestPopupMenu_MnemonicAmbiguousMovesHover(t *testing.T) {
	var clicks int
	m := NewPopupMenu()
	m.UseMnemonics = true
	m.SetItems([]MenuItem{
		{Text: "_Открыть", OnClick: func() { clicks++ }},
		{Text: "_Обновить", OnClick: func() { clicks++ }},
	})
	m.Show(10, 10)

	m.OnKeyEvent(KeyEvent{Code: KeyJ, Pressed: true}) // на J стоит О — обе мнемоники
	if clicks != 0 {
		t.Errorf("пункт выполнен %d раз при неоднозначной мнемонике", clicks)
	}
	if !m.IsOpen() {
		t.Error("меню закрылось")
	}
}

// Мнемоника пункта с подменю открывает подменю, а не выполняет пункт.
func TestPopupMenu_MnemonicOpensSubmenu(t *testing.T) {
	m := NewPopupMenu()
	m.UseMnemonics = true
	m.SetItems([]MenuItem{
		{Text: "_Экспорт", SubItems: []MenuItem{{Text: "PDF"}}},
	})
	m.Show(10, 10)

	// Э сидит на клавише апострофа, кода буквы у неё нет — ловим по символу.
	m.OnKeyEvent(KeyEvent{Code: KeyOemQuote, Rune: 'э', Pressed: true})
	c, _ := m.openChildOf()
	if c == nil {
		t.Fatal("подменю не открылось")
	}
	if !c.UseMnemonics {
		t.Error("подменю не унаследовало флаг мнемоник")
	}
}

// Ширина меню считается по подписи без служебного подчёркивания: иначе пункт
// «_Файл» занимал бы место шире нарисованного.
func TestPopupMenu_MnemonicWidthIgnoresUnderscore(t *testing.T) {
	plain := shownMenuWidth(t, MenuItem{Text: "Файл"}, false)
	mn := shownMenuWidth(t, MenuItem{Text: "_Файл"}, true)
	if mn != plain {
		t.Errorf("ширина с мнемоникой %d, без неё %d — должны совпадать", mn, plain)
	}
}

func TestMenuBar_ActivateMnemonic(t *testing.T) {
	mb := NewMenuBar()
	mb.UseMnemonics = true
	mb.AddMenu("_Файл", MenuItem{Text: "Новый"})
	mb.AddMenu("_Правка", MenuItem{Text: "Копировать"})

	// Alt+A — физическая клавиша буквы Ф в ЙЦУКЕН.
	if !mb.ActivateMnemonic(KeyEvent{Code: KeyA, Mod: ModAlt, Pressed: true}) {
		t.Fatal("меню «Файл» не открылось по Alt+Ф")
	}
	if got := openMenuFirstItem(t, mb); got != "Новый" {
		t.Errorf("открыто меню с первым пунктом %q, ждал «Новый»", got)
	}
	if !mb.ActivateMnemonic(KeyEvent{Code: KeyG, Mod: ModAlt, Pressed: true}) {
		t.Fatal("меню «Правка» не открылось по Alt+П")
	}
	if got := openMenuFirstItem(t, mb); got != "Копировать" {
		t.Errorf("открыто меню с первым пунктом %q, ждал «Копировать»", got)
	}
	if mb.ActivateMnemonic(KeyEvent{Code: KeyZ, Mod: ModAlt, Pressed: true}) {
		t.Error("нашлась мнемоника там, где её нет")
	}
}

func TestMenuBar_MnemonicsOffByDefault(t *testing.T) {
	mb := NewMenuBar()
	mb.AddMenu("_Файл", MenuItem{Text: "Новый"})
	if mb.ActivateMnemonic(KeyEvent{Code: KeyA, Mod: ModAlt, Pressed: true}) {
		t.Error("мнемоника сработала при выключенном флаге")
	}
}

func TestMenuBar_UseMnemonicsFromXAML(t *testing.T) {
	const src = `<Canvas Width="400" Height="200">
  <Menu Name="main" Left="0" Top="0" Width="400" Height="28" UseMnemonics="True">
    <MenuItem Header="_Файл">
      <MenuItem Text="_Новый"/>
    </MenuItem>
  </Menu>
</Canvas>`
	root, reg, err := LoadUIFromXAML([]byte(src))
	if err != nil {
		t.Fatalf("разбор разметки: %v", err)
	}
	defer ReleaseXAML(root)

	mb, ok := reg["main"].(*MenuBar)
	if !ok {
		t.Fatalf("main — %T", reg["main"])
	}
	if !mb.UseMnemonics {
		t.Error("атрибут UseMnemonics не прочитан")
	}
}

// openMenuFirstItem — подпись первого пункта раскрытого подменю: по ней видно,
// какое из меню полосы открыто.
func openMenuFirstItem(t *testing.T, mb *MenuBar) string {
	t.Helper()
	if !mb.popup.IsOpen() {
		t.Fatal("подменю не открыто")
	}
	items := mb.popup.Items()
	if len(items) == 0 {
		t.Fatal("подменю пустое")
	}
	return items[0].Text
}
