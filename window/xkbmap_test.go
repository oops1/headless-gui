package window

import (
	"os"
	"strings"
	"testing"
)

// Реальный keymap WSLg (US-раскладка, снят с живого композитора).
func TestXkb_ParseRealWSLgKeymap(t *testing.T) {
	data, err := os.ReadFile("testdata/wslg_keymap.txt")
	if err != nil {
		t.Skipf("нет testdata/wslg_keymap.txt: %v", err)
	}
	km := parseXkbKeymap(strings.TrimRight(string(data), "\x00"))
	if km == nil {
		t.Fatal("парсер вернул nil на реальном keymap")
	}
	if len(km.keys) < 100 {
		t.Fatalf("распарсено %d клавиш, ожидалось >100", len(km.keys))
	}

	// <AD01> = 24 → q/Q; <AE01> = 10 → 1/!.
	cases := []struct {
		code        uint32
		shift, caps bool
		want        rune
	}{
		{24, false, false, 'q'},
		{24, true, false, 'Q'},
		{24, false, true, 'Q'},  // CapsLock
		{24, true, true, 'q'},   // CapsLock + Shift
		{10, false, false, '1'},
		{10, true, false, '!'},
		{10, false, true, '1'}, // CapsLock не влияет на цифры
	}
	for _, c := range cases {
		if got := km.runeFor(c.code, 0, c.shift, c.caps); got != c.want {
			t.Errorf("code=%d shift=%v caps=%v: %q, ожидалось %q",
				c.code, c.shift, c.caps, got, c.want)
		}
	}
}

// Синтетический keymap с двумя группами (EN+RU) — переключение раскладки.
const ruKeymap = `xkb_keymap {
xkb_keycodes "test" {
	minimum = 8;
	maximum = 255;
	<AD01> = 24;
	<AC01> = 38;
	<AE03> = 12;
	alias <LatQ> = <AD01>;
};
xkb_types "test" { };
xkb_compat "test" { };
xkb_symbols "test" {
	name[Group1]= "English (US)";
	name[Group2]= "Russian";
	key <AD01> { [ q, Q ], [ Cyrillic_shorti, Cyrillic_SHORTI ] };
	key <AC01> {
		type= "ALPHABETIC",
		symbols[Group1]= [ a, A ],
		symbols[Group2]= [ Cyrillic_ef, Cyrillic_EF ]
	};
	key <AE03> { [ 3, numbersign ], [ 3, numerosign ] };
};
};`

func TestXkb_Groups(t *testing.T) {
	km := parseXkbKeymap(ruKeymap)
	if km == nil {
		t.Fatal("nil")
	}
	// Группа 0 — латиница.
	if got := km.runeFor(24, 0, false, false); got != 'q' {
		t.Errorf("EN q: %q", got)
	}
	// Группа 1 — кириллица.
	if got := km.runeFor(24, 1, false, false); got != 'й' {
		t.Errorf("RU й: %q", got)
	}
	if got := km.runeFor(24, 1, true, false); got != 'Й' {
		t.Errorf("RU Й (shift): %q", got)
	}
	if got := km.runeFor(24, 1, false, true); got != 'Й' {
		t.Errorf("RU Й (caps): %q", got)
	}
	if got := km.runeFor(38, 1, false, false); got != 'ф' {
		t.Errorf("RU ф: %q", got)
	}
	// numerosign во второй группе с Shift.
	if got := km.runeFor(12, 1, true, false); got != '№' {
		t.Errorf("RU №: %q", got)
	}
	// Несуществующая группа — фолбэк на первую.
	if got := km.runeFor(24, 5, false, false); got != 'q' {
		t.Errorf("фолбэк группы: %q", got)
	}
}

func TestKeysymNames(t *testing.T) {
	cases := map[string]rune{
		"a": 'a', "Z": 'Z', "1": '1',
		"space": ' ', "exclam": '!', "asciitilde": '~',
		"Cyrillic_a": 'а', "Cyrillic_A": 'А', "Cyrillic_shcha": 'щ',
		"Cyrillic_SHCHA": 'Щ', "Cyrillic_io": 'ё', "Cyrillic_IO": 'Ё',
		"U0416": 'Ж', "U+0436": 'ж',
		"adiaeresis": 'ä', "ssharp": 'ß',
		"numerosign": '№',
		"dead_grave": 0, "NoSymbol": 0, "Shift_L": 0,
	}
	for name, want := range cases {
		if got := keysymNameToRune(name); got != want {
			t.Errorf("%q: %q (%d), ожидалось %q", name, got, got, want)
		}
	}
}

func TestKeysymNumeric(t *testing.T) {
	cases := map[uint32]rune{
		0x41: 'A', 0x7A: 'z', 0xE9: 'é',
		0x01000416: 'Ж',
		0x6C1: 'а', 0x6E1: 'А', 0x6C6: 'ф', 0x6E6: 'Ф',
		0x6A3: 'ё', 0x6B3: 'Ё', 0x6B0: '№',
		0xFF0D: 0, // Enter — непечатаемый
	}
	for sym, want := range cases {
		if got := keysymToRune(sym); got != want {
			t.Errorf("0x%X: %q, ожидалось %q", sym, got, want)
		}
	}
}
