package theme_test

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
)

// Токен tile.cycle (переход живой плитки меню «Пуск», ADR-0010) и метрика вида
// перехода: объявлены по семействам, у классической темы — нуль, при «меньше
// движения» — мгновенно.

func TestTileCycleToken_DeclaredPerFamily(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	want := map[string]time.Duration{
		theme.ProfileWindows11:     ms(250),
		theme.ProfileWindows11Dark: ms(250),
		theme.ProfileWindows10:     ms(300),
		theme.ProfileWindows10Dark: ms(300),
		theme.ProfileMacOS:         ms(250),
		theme.ProfileMacOSDark:     ms(250),
	}
	for prof, d := range want {
		m := builtinManager(t, prof)
		a := m.GetAnimation(theme.AnimTileCycle)
		if a.Duration != d || a.Curve != "out-cubic" {
			t.Errorf("%s: tile.cycle %v %q, ждали %v out-cubic", prof, a.Duration, a.Curve, d)
		}
	}
}

// Классическая тема объявляет tile.cycle нулём явно: потомок с ненулевым
// запасным flyout.open всё равно получает мгновенный переход.
func TestTileCycleToken_ClassicDeclaredZero(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	child := theme.NewProfile("classic-child")
	child.Parent = theme.ProfileWindows2000
	child.Anims[theme.AnimFlyoutOpen] = theme.AnimSpec{Duration: 150 * time.Millisecond}
	if err := m.RegisterTheme(child); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("classic-child"); err != nil {
		t.Fatal(err)
	}
	if d := m.GetAnimationFirst(theme.AnimTileCycle, theme.AnimFlyoutOpen).Duration; d != 0 {
		t.Errorf("Windows 2000 и потомок: tile.cycle %v, ждали мгновенно (объявлен нулём)", d)
	}
}

// Метрика вида объявлена в профиле Windows 10 явно (0 — сдвиг).
func TestTileCycleToken_KindMetricDeclared(t *testing.T) {
	p := theme.Windows10Profile()
	v, ok := p.Metrics[theme.KeyTileCycleKind]
	if !ok {
		t.Fatal("Windows 10: метрика tile.cycle.kind не объявлена")
	}
	if v != 0 {
		t.Errorf("Windows 10: tile.cycle.kind %v, ждали 0 (сдвиг)", v)
	}
}

func TestTileCycleToken_ReducedMotionIsInstant(t *testing.T) {
	m := builtinManager(t, theme.ProfileWindows10)
	if a := m.GetAnimation(theme.AnimTileCycle); a.Duration == 0 {
		t.Fatal("Windows 10 без флага: переход должен длиться")
	}
	m.SetFlag(theme.FlagMotionReduce, true)
	if a := m.GetAnimation(theme.AnimTileCycle); a.Duration != 0 {
		t.Errorf("«меньше движения»: tile.cycle %v, ждали мгновенно (декоративная анимация)", a.Duration)
	}
}
