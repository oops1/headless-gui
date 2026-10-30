package theme_test

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/theme"
)

// Токены движения и мягких теней четырёх семейств (profiles_motion.go).

func TestMotionTokens_Table(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	want := map[string]map[theme.Key]time.Duration{
		theme.ProfileWindows11: {theme.AnimMenuOpen: ms(200), theme.AnimMenuClose: ms(120), theme.AnimSubmenuOpen: ms(150), theme.AnimStartOpen: ms(250), theme.AnimFlyoutOpen: ms(200)},
		theme.ProfileWindows10: {theme.AnimMenuOpen: ms(150), theme.AnimMenuClose: ms(100), theme.AnimSubmenuOpen: ms(120), theme.AnimStartOpen: ms(200), theme.AnimFlyoutOpen: ms(150)},
		theme.ProfileMacOS:     {theme.AnimMenuOpen: ms(180), theme.AnimMenuClose: ms(150), theme.AnimSubmenuOpen: ms(120), theme.AnimStartOpen: ms(200), theme.AnimFlyoutOpen: ms(180)},
	}
	// Тёмные разновидности наследуют движение родителя.
	want[theme.ProfileWindows11Dark] = want[theme.ProfileWindows11]
	want[theme.ProfileWindows10Dark] = want[theme.ProfileWindows10]
	want[theme.ProfileMacOSDark] = want[theme.ProfileMacOS]
	for prof, keys := range want {
		m := builtinManager(t, prof)
		for k, d := range keys {
			if got := m.GetAnimation(k).Duration; got != d {
				t.Errorf("%s %s = %v, ждали %v", prof, k, got, d)
			}
			// Закрытие и панели — короче или равны открытию «Пуска»: ни одна
			// анимация не выходит за 250 мс (ТЗ Windows 11 — 100–250).
			if d > ms(250) {
				t.Errorf("%s %s = %v — дольше 250 мс", prof, k, d)
			}
		}
		if c, o := keys[theme.AnimMenuClose], keys[theme.AnimMenuOpen]; c >= o {
			t.Errorf("%s: закрытие меню %v не короче открытия %v", prof, c, o)
		}
	}
	// Классика: все новые ключи объявлены нулевыми, а не пропущены (иначе
	// запасной ключ взял бы menu.open у чужого профиля).
	for _, prof := range []string{theme.ProfileWindows2000, theme.ProfileWindows2000Blue} {
		m := builtinManager(t, prof)
		for _, k := range []theme.Key{theme.AnimMenuOpen, theme.AnimMenuClose, theme.AnimSubmenuOpen, theme.AnimStartOpen, theme.AnimFlyoutOpen} {
			a, ok := m.Active().Anim(k)
			if !ok || a.Duration != 0 {
				t.Errorf("%s %s: объявлен=%v, длительность %v — ждали объявленный ноль", prof, k, ok, a.Duration)
			}
		}
	}
}

// Новые ключи декоративные: «меньше движения» их обнуляет.
func TestMotionTokens_Reduced(t *testing.T) {
	m := builtinManager(t, theme.ProfileWindows11)
	m.SetFlag(theme.FlagMotionReduce, true)
	for _, k := range []theme.Key{theme.AnimMenuClose, theme.AnimSubmenuOpen, theme.AnimStartOpen, theme.AnimFlyoutOpen} {
		if d := m.GetAnimation(k).Duration; d != 0 {
			t.Errorf("%s при «меньше движения» = %v", k, d)
		}
	}
}

// GetAnimationFirst — первый объявленный ключ цепочки; без объявленных — ноль.
func TestMotionTokens_FirstDeclared(t *testing.T) {
	m := theme.NewManager()
	p := theme.NewProfile("chain")
	p.Anims[theme.AnimMenuOpen] = theme.AnimSpec{Duration: 90 * time.Millisecond}
	if err := m.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	_ = m.SetTheme("chain")
	if d := m.GetAnimationFirst(theme.AnimStartOpen, theme.AnimFlyoutOpen, theme.AnimMenuOpen).Duration; d != 90*time.Millisecond {
		t.Fatalf("цепочка без start.open и flyout.open: %v, ждали menu.open 90 мс", d)
	}
	if d := m.GetAnimationFirst(theme.AnimStartOpen).Duration; d != 0 {
		t.Fatalf("необъявленный ключ без запасного: %v", d)
	}
}

// Мягкие тени Windows 10 и macOS — только под флагом shadow.soft; у Windows
// 10 — жёстче (меньше размытие), чем у Windows 11; у классики — нет.
func TestShadowTokens_Families(t *testing.T) {
	w11 := builtinManager(t, theme.ProfileWindows11)
	w11.SetFlag(theme.FlagShadowSoft, true)
	s11, _ := w11.GetStyle("menu", "", theme.StateNormal).ResolveShadow()
	for _, prof := range []string{theme.ProfileWindows10, theme.ProfileMacOS} {
		m := builtinManager(t, prof)
		if sp, ok := m.GetStyle("menu", "", theme.StateNormal).ResolveShadow(); ok && sp.Blur > 10 {
			t.Errorf("%s без флага: тень меню %+v — токены просочились", prof, sp)
		}
		m.SetFlag(theme.FlagShadowSoft, true)
		for _, comp := range []string{"menu", "startmenu", "quicksettings", "calendar", "window", "dialog"} {
			sp, ok := m.GetStyle(comp, "", theme.StateNormal).ResolveShadow()
			if !ok || sp.Blur <= 0 || sp.OffsetY <= 0 {
				t.Errorf("%s %s: мягкая тень %+v ok=%v", prof, comp, sp, ok)
			}
		}
		menu, _ := m.GetStyle("menu", "", theme.StateNormal).ResolveShadow()
		win, _ := m.GetStyle("window", "", theme.StateNormal).ResolveShadow()
		if win.Blur <= menu.Blur {
			t.Errorf("%s: тень окна %+v не крупнее тени меню %+v", prof, win, menu)
		}
		if prof == theme.ProfileWindows10 && menu.Blur >= s11.Blur {
			t.Errorf("Windows 10: тень меню %v не жёстче Windows 11 %v", menu.Blur, s11.Blur)
		}
	}
	d := builtinManager(t, theme.ProfileMacOSDark)
	d.SetFlag(theme.FlagShadowSoft, true)
	l := builtinManager(t, theme.ProfileMacOS)
	l.SetFlag(theme.FlagShadowSoft, true)
	ds, _ := d.GetStyle("menu", "", theme.StateNormal).ResolveShadow()
	ls, _ := l.GetStyle("menu", "", theme.StateNormal).ResolveShadow()
	if ds.Color.A <= ls.Color.A {
		t.Errorf("macOS: тёмная тень %v не плотнее светлой %v", ds.Color, ls.Color)
	}
	c := builtinManager(t, theme.ProfileWindows2000)
	c.SetFlag(theme.FlagShadowSoft, true)
	if sp, ok := c.GetStyle("menu", "", theme.StateNormal).ResolveShadow(); ok && sp.Blur > 0 {
		t.Errorf("Windows 2000 получила мягкую тень: %+v", sp)
	}
}
