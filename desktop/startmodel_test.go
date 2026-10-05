package desktop

import (
	"fmt"
	"testing"
)

// Модель меню «Пуск»: буквы, группировка, источник по умолчанию.

func TestStartLetter(t *testing.T) {
	cases := map[string]string{
		"7-Zip":         "#",
		"  Cisco":       "C",
		"cortana":       "C",
		"Яндекс.Музыка": "Я",
		"ёлка":          "Ё",
		"":              "#",
		"…":             "#",
		"Éclair":        "É",
	}
	for title, want := range cases {
		if got := StartLetter(title); got != want {
			t.Errorf("StartLetter(%q) = %q, ждали %q", title, got, want)
		}
	}
}

func TestGroupByLetter_OrderAndSorting(t *testing.T) {
	titles := []string{"Яндекс", "paint", "Блокнот", "7-Zip", "Calc", "calendar", "Alpha", "Zeta", "1C"}
	var in []StartEntry
	for i, tl := range titles {
		in = append(in, StartEntry{ID: AppID(fmt.Sprint(i)), Title: tl})
	}
	groups := GroupByLetter(in)
	var order string
	for _, g := range groups {
		order += g.Letter
	}
	// «#», затем латиница по алфавиту, затем кириллица.
	if order != "#ACPZБЯ" {
		t.Errorf("порядок групп %q, ждали «#ACPZБЯ»", order)
	}
	for _, g := range groups {
		if g.Letter == "C" {
			if len(g.Entries) != 2 || g.Entries[0].Title != "Calc" || g.Entries[1].Title != "calendar" {
				t.Errorf("группа C %v: записи не по алфавиту без учёта регистра", g.Entries)
			}
		}
		if g.Letter == "#" && len(g.Entries) != 2 {
			t.Errorf("группа # %v, ждали «1C» и «7-Zip»", g.Entries)
		}
	}
	if in[0].Title != "Яндекс" {
		t.Error("исходный срез изменён")
	}
}

type recentCatalog struct {
	*StaticAppCatalog
	recent []AppID
	subs   int
}

func (c *recentCatalog) Recent() []AppID { return c.recent }
func (c *recentCatalog) Subscribe(fn func()) func() {
	c.subs++
	return func() { c.subs-- }
}

func TestCatalogSource_GroupsAndRecent(t *testing.T) {
	cat := &recentCatalog{
		StaticAppCatalog: NewStaticAppCatalog(
			AppInfo{ID: "b", Title: "Beta"}, AppInfo{ID: "a", Title: "Alpha"}, AppInfo{ID: "c", Title: "Яблоко"},
		),
		recent: []AppID{"c", "missing", "a"},
	}
	src := NewCatalogSource(cat)
	g := src.Groups()
	if len(g) != 3 || g[0].Letter != "A" || g[1].Letter != "B" || g[2].Letter != "Я" {
		t.Errorf("группы %v", g)
	}
	rec := src.Recent()
	if len(rec) != 2 || rec[0].ID != "c" || rec[1].ID != "a" {
		t.Errorf("недавние %v: исчезнувшее приложение должно пропускаться", rec)
	}
	un := src.Subscribe(func() {})
	if cat.subs != 1 {
		t.Error("источник не подписался на каталог")
	}
	un()
	if cat.subs != 0 {
		t.Error("отписка не дошла до каталога")
	}
	// Каталог без Recent и Subscribe тоже годится.
	plain := NewCatalogSource(NewStaticAppCatalog(AppInfo{ID: "x", Title: "X"}))
	if len(plain.Recent()) != 0 || len(plain.Groups()) != 1 {
		t.Error("простой каталог дал неверные группы")
	}
	plain.Subscribe(func() {})() // не падает
}

func TestFakeStartSource_NotifiesAndUnsubscribes(t *testing.T) {
	s := NewFakeStartSource(nil, nil)
	n := 0
	un := s.Subscribe(func() { n++ })
	s.Set([]StartEntry{{ID: "r", Title: "R"}}, nil)
	if n != 1 || len(s.Recent()) != 1 {
		t.Errorf("уведомлений %d, недавних %d", n, len(s.Recent()))
	}
	un()
	s.Set(nil, nil)
	if n != 1 {
		t.Error("после отписки уведомление всё ещё приходит")
	}
}

func TestTileSizeUnits(t *testing.T) {
	cases := []struct {
		s      TileSize
		w, h   int
		string string
	}{{TileSmall, 1, 1, "small"}, {TileMedium, 2, 2, "medium"}, {TileWide, 4, 2, "wide"}, {TileLarge, 4, 4, "large"}, {TileSize(99), 2, 2, "medium"}}
	for _, c := range cases {
		w, h := c.s.Units()
		if w != c.w || h != c.h || c.s.String() != c.string {
			t.Errorf("%v: %d×%d %q, ждали %d×%d %q", int(c.s), w, h, c.s.String(), c.w, c.h, c.string)
		}
	}
}

func TestComputeTileLayout_GroupsStackWithGaps(t *testing.T) {
	groups := []TileGroup{
		{Title: "A", Tiles: []Tile{{ID: "1", Size: TileMedium}, {ID: "2", Size: TileMedium}}},
		{Title: "B", Tiles: []Tile{{ID: "3", Size: TileLarge}}},
		{Title: "Пустая"},
	}
	l := computeTileLayout(groups, tileKey{cols: 6, unit: 48, gap: 4, header: 32, headerGap: 5, groupGap: 4})
	if len(l.groups) != 3 || len(l.tiles) != 3 {
		t.Fatalf("групп %d, плиток %d", len(l.groups), len(l.tiles))
	}
	// Группа A: заголовок 32 + зазор 5 + строка плиток 100 → 137; +4 до следующей.
	if l.groups[1].header.Min.Y != 32+5+100+4 {
		t.Errorf("заголовок второй группы на %d, ждали %d", l.groups[1].header.Min.Y, 32+5+100+4)
	}
	if l.tiles[2].rect.Dx() != 204 || l.tiles[2].rect.Dy() != 204 {
		t.Errorf("большая плитка %v, ждали 204×204", l.tiles[2].rect)
	}
	if l.height <= l.groups[2].header.Min.Y {
		t.Errorf("высота %d не включает последнюю группу", l.height)
	}
	if empty := computeTileLayout(nil, tileKey{cols: 6, unit: 48}); empty.height != 0 {
		t.Errorf("пустая раскладка имеет высоту %d", empty.height)
	}
}
