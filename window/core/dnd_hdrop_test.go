package core

import (
	"encoding/binary"
	"reflect"
	"testing"
)

// Блок CF_HDROP: заголовок DROPFILES и пути UTF-16. Проверяется побайтно —
// это формат, который читает чужой код (Проводник), а не наш.
func TestBuildHDrop_Layout(t *testing.T) {
	b := buildHDrop([]string{`C:\a.txt`, `D:\Фото\б.png`})

	if got := binary.LittleEndian.Uint32(b[0:4]); got != 20 {
		t.Errorf("pFiles = %d, ждал 20", got)
	}
	if got := binary.LittleEndian.Uint32(b[16:20]); got != 1 {
		t.Errorf("fWide = %d, ждал 1", got)
	}
	// Список заканчивается двумя нулями подряд.
	n := len(b)
	if b[n-1] != 0 || b[n-2] != 0 || b[n-3] != 0 || b[n-4] != 0 {
		t.Errorf("список не завершён двойным нулём: % x", b[n-6:])
	}
	// Размер: заголовок + (8+1 + 13+1 + 1) знаков по 2 байта.
	if want := 20 + (9+14+1)*2; n != want {
		t.Errorf("длина блока %d, ждал %d", n, want)
	}
	// Первый путь сразу за заголовком.
	if got := binary.LittleEndian.Uint16(b[20:22]); got != 'C' {
		t.Errorf("первый знак пути %q", rune(got))
	}
}

func TestHDrop_RoundTrip(t *testing.T) {
	cases := [][]string{
		{`C:\one.txt`},
		{`C:\a b\c.txt`, `\\srv\share\файл.png`, `/home/u/😀.jpg`},
		nil,
	}
	for _, in := range cases {
		got, ok := parseHDrop(buildHDrop(in))
		if !ok {
			t.Errorf("parseHDrop(%q) не разобрал собственный блок", in)
			continue
		}
		if len(in) == 0 && len(got) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, in) {
			t.Errorf("круг: %q -> %q", in, got)
		}
	}
}

func TestParseHDrop_Hostile(t *testing.T) {
	good := buildHDrop([]string{`C:\a`})
	// Смещение списка за границей блока.
	bad := append([]byte(nil), good...)
	binary.LittleEndian.PutUint32(bad[0:4], 9999)
	if _, ok := parseHDrop(bad); ok {
		t.Error("смещение за границей принято")
	}
	// Смещение внутри заголовка.
	binary.LittleEndian.PutUint32(bad[0:4], 4)
	if _, ok := parseHDrop(bad); ok {
		t.Error("смещение внутри заголовка принято")
	}
	// Однобайтовые пути не поддерживаются.
	bad = append([]byte(nil), good...)
	binary.LittleEndian.PutUint32(bad[16:20], 0)
	if _, ok := parseHDrop(bad); ok {
		t.Error("ANSI-блок принят")
	}
	// Короткий блок и блок без завершающих нулей не паникуют.
	if _, ok := parseHDrop(good[:10]); ok {
		t.Error("обрезанный заголовок принят")
	}
	if got, _ := parseHDrop(good[:len(good)-4]); len(got) > 1 {
		t.Errorf("обрезанный список дал %q", got)
	}
}

func TestDragContinueResult(t *testing.T) {
	const left = mkLButton
	cases := []struct {
		name     string
		esc      bool
		keys, mk uint32
		want     uint32
	}{
		{"держим кнопку", false, left, left, 0},
		{"кнопку отпустили — сброс", false, 0, left, dragdropSDrop},
		{"Esc — отмена", true, left, left, dragdropSCancel},
		{"Esc важнее отпускания", true, 0, left, dragdropSCancel},
		{"держим другую кнопку — это не наша", false, mkRButton, left, dragdropSDrop},
		{"Ctrl не мешает", false, left | mkControl, left, 0},
		{"правой начали, правая держится", false, mkRButton, mkRButton, 0},
	}
	for _, tc := range cases {
		if got := dragContinueResult(tc.esc, tc.keys, tc.mk); got != tc.want {
			t.Errorf("%s: %#x, ждал %#x", tc.name, got, tc.want)
		}
	}
}
