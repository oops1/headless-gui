package theme_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
)

// Windows 2000 следует SetAccent: акцентные цвета профиля — ссылки на токены, а
// не литералы. Без вызова SetAccent вид остаётся ПОБИТНО прежним — это держит
// снимок всех стилей обеих классических тем, снятый до замены литералов.

// classicDump — все стили профиля и его цепочки, во всех состояниях, одной
// строкой на стиль. Указатель на фаску раскрывается в значение (иначе в строку
// попал бы адрес).
func classicDump(t *testing.T, name string) string {
	t.Helper()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(name); err != nil {
		t.Fatal(err)
	}
	type cp struct{ c, p string }
	seen := map[cp]bool{}
	add := func(p *theme.Profile) {
		for k := range p.Styles {
			seen[cp{k.Component, k.Part}] = true
		}
		for _, c := range p.Conditional {
			seen[cp{c.Key.Component, c.Key.Part}] = true
		}
	}
	for _, n := range []string{theme.ProfileWindows2000, theme.ProfileWindows2000Blue} {
		switch n {
		case theme.ProfileWindows2000:
			add(theme.Windows2000Profile())
		default:
			add(theme.Windows2000BlueProfile())
		}
	}
	for _, c := range []string{"tray.icon", "tray.notifications", "tray.showdesktop", "searchbox", "menu"} {
		seen[cp{c, ""}] = true
	}
	var keys []cp
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].c != keys[j].c {
			return keys[i].c < keys[j].c
		}
		return keys[i].p < keys[j].p
	})
	states := []theme.State{
		theme.StateNormal, theme.StateHover, theme.StatePressed, theme.StateActive,
		theme.StateDisabled, theme.StateFocused,
	}
	var b strings.Builder
	for _, k := range keys {
		for _, st := range states {
			s := *m.GetStyle(k.c, k.p, st)
			bevel := s.Bevel
			s.Bevel = nil
			fmt.Fprintf(&b, "%s/%s/%d: %+v", k.c, k.p, st, s)
			if bevel != nil {
				fmt.Fprintf(&b, " bevel=%+v", *bevel)
			}
			b.WriteByte('\n')
		}
	}
	th := m.Active()
	for _, k := range []theme.Key{"accent", "accent.hover", "accent.pressed", "accent.dark",
		"accent.light", "accent.text", "selection", "surface", "text"} {
		cc, _ := th.Color(k)
		fmt.Fprintf(&b, "color %s=%v;", k, cc)
	}
	return b.String()
}

func classicHash(t *testing.T, name string) string {
	sum := sha256.Sum256([]byte(classicDump(t, name)))
	return hex.EncodeToString(sum[:8])
}

// Снимки сняты на профилях ДО замены литералов акцента ссылками.
func TestClassic_LookIsBitIdenticalWithoutSetAccent(t *testing.T) {
	want := map[string]string{
		// Хэши включают стили заголовка и затемнения диалога
		// (theme/dialogstyles.go): они добавлены намеренно, без них хэши
		// были 6e38def94d5286e3 и d4da5b1d98443b59.
		theme.ProfileWindows2000:     "55f17c214cf64e74",
		theme.ProfileWindows2000Blue: "19fa4cc9bf21998b",
	}
	for name, hash := range want {
		if got := classicHash(t, name); got != hash {
			t.Errorf("%s: снимок стилей %s, ждали %s — вид без SetAccent изменился", name, got, hash)
		}
	}
}

// SetAccent перекрашивает классические темы: заголовок активного окна, выделение
// меню и «Пуска», заливка ползунка и выбранного дня. Фаски и серая палитра
// остаются на месте.
func TestClassic_FollowsSetAccent(t *testing.T) {
	green := theme.RGB(16, 124, 16)
	for _, name := range []string{theme.ProfileWindows2000, theme.ProfileWindows2000Blue} {
		m := theme.NewManager()
		if err := theme.RegisterBuiltinProfiles(m); err != nil {
			t.Fatal(err)
		}
		if err := m.SetTheme(name); err != nil {
			t.Fatal(err)
		}
		before := *m.GetStyle("window", "titlebar", theme.StateFocused)
		face := m.GetStyle("window", "", theme.StateNormal).Fill
		m.SetAccent(green)

		if got := m.GetStyle("window", "titlebar", theme.StateFocused).Fill; got != green {
			t.Errorf("%s: заголовок активного окна %v, ждали акцент %v (было %v)", name, got, green, before.Fill)
		}
		if got := m.GetStyle("window", "", theme.StateNormal).Fill; got != face {
			t.Errorf("%s: серая поверхность окна изменилась: %v → %v", name, face, got)
		}
		// Текст на акценте — по токену, не литерал.
		if got := m.GetStyle("window", "titlebar", theme.StateFocused).Text; got != theme.DeriveAccent(green).Text {
			t.Errorf("%s: текст заголовка %v, ждали %v", name, got, theme.DeriveAccent(green).Text)
		}
		m.ResetAccent()
		if got := m.GetStyle("window", "titlebar", theme.StateFocused).Fill; got != before.Fill {
			t.Errorf("%s: ResetAccent не вернул заголовок: %v, было %v", name, got, before.Fill)
		}
	}

	// Классическая тема без Blue: выделение меню идёт за акцентом целиком.
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows2000); err != nil {
		t.Fatal(err)
	}
	m.SetAccent(green)
	checks := []struct {
		comp, part string
		st         theme.State
	}{
		{"menu", "item", theme.StateHover},
		{"startmenu", "", theme.StateHover},
		{"quicksettings", "slider.fill", theme.StateNormal},
		{"quicksettings", "tile.network", theme.StateActive},
		{"calendar", "day", theme.StateActive},
	}
	for _, c := range checks {
		if got := m.GetStyle(c.comp, c.part, c.st).Fill; got != green {
			t.Errorf("%s/%s: заливка %v, ждали акцент %v", c.comp, c.part, got, green)
		}
	}
	if got := m.GetStyle("calendar", "day", theme.StateFocused).Border; got != green {
		t.Errorf("calendar/day focused: рамка %v, ждали акцент", got)
	}
}
