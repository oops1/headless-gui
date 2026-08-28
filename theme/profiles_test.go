package theme_test

import (
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
)

// TestBuiltinProfiles_AllResolve — каждый встроенный профиль собирается и
// отдаёт осмысленные значения. Битая цепочка наследования или опечатка в
// имени родителя всплывёт здесь, а не в приложении.
func TestBuiltinProfiles_AllResolve(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatalf("регистрация встроенных профилей: %v", err)
	}
	if len(m.ThemeNames()) != 8 {
		t.Errorf("зарегистрировано %d профилей, ждали 8", len(m.ThemeNames()))
	}

	for _, name := range m.ThemeNames() {
		name := name
		t.Run(name, func(t *testing.T) {
			if err := m.SetTheme(name); err != nil {
				t.Fatalf("не собирается: %v", err)
			}
			th := m.Active()
			if h, ok := th.Metric("taskbar.height"); !ok || h <= 0 {
				t.Errorf("высота панели задач не задана: %v", h)
			}
			// Панель задач должна быть чем-то видима: заливкой или подложкой.
			s := th.Style("taskbar", "", theme.StateNormal)
			if s.Fill.A == 0 && s.Backdrop.Mode == theme.BackdropNone {
				t.Error("панель задач невидима: ни заливки, ни подложки")
			}
			// Текст на кнопках должен быть непрозрачным, иначе подписи
			// пропадут.
			if c := th.Style("taskbutton", "", theme.StateNormal).Text; c.A == 0 {
				t.Error("цвет текста кнопок окон прозрачен")
			}
		})
	}
}

// TestBuiltinProfiles_DarkVariantsAreThin — тёмная разновидность объявляет
// не больше пятнадцати собственных токенов, остальное берёт у родителя
// (критерий приёмки: «Windows11Dark объявляет не более пятнадцати токенов»).
//
// Разросшийся дочерний профиль — признак того, что общее не вынесено в
// родителя, и тогда правка палитры потребует править обе темы.
func TestBuiltinProfiles_DarkVariantsAreThin(t *testing.T) {
	cases := []struct {
		name    string
		profile *theme.Profile
		limit   int
	}{
		{"Windows11 Dark", theme.Windows11DarkProfile(), 15},
		{"Windows10 Dark", theme.Windows10DarkProfile(), 15},
		{"macOS Dark", theme.MacOSDarkProfile(), 15},
		{"Windows2000 Blue", theme.Windows2000BlueProfile(), 15},
	}
	for _, c := range cases {
		if n := c.profile.TokenCount(); n > c.limit {
			t.Errorf("%s объявляет %d собственных токенов, предел %d — общее не вынесено в родителя",
				c.name, n, c.limit)
		}
		if c.profile.Parent == "" {
			t.Errorf("%s не наследует ни от кого", c.name)
		}
	}
}

// TestBuiltinProfiles_InheritanceActuallyWorks — тёмная тема действительно
// берёт у родителя незаявленное.
func TestBuiltinProfiles_InheritanceActuallyWorks(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows11Dark); err != nil {
		t.Fatal(err)
	}
	dark := m.Active()

	if err := m.SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	light := m.Active()

	// Унаследовано: геометрия и анимации у обеих одинаковы.
	for _, k := range []theme.Key{"taskbar.height", "control.corner", "window.corner", "taskbar.gap"} {
		l, _ := light.Metric(k)
		d, _ := dark.Metric(k)
		if l != d {
			t.Errorf("метрика %s не унаследована: светлая %v, тёмная %v", k, l, d)
		}
	}
	la, _ := light.Anim("menu.open")
	da, _ := dark.Anim("menu.open")
	if la != da {
		t.Error("анимация меню не унаследована")
	}
	// Переопределено: поверхность и текст различаются.
	lc, _ := light.Color("surface")
	dc, _ := dark.Color("surface")
	if lc == dc {
		t.Error("тёмная тема не переопределила поверхность")
	}
}

// TestBuiltinProfiles_ClassicHasNoMotionOrGlass — классика выражает свой
// характер данными: нулевые анимации, никакой прозрачности, фаска вместо
// плоской рамки. Компоненту для этого ничего знать не нужно.
func TestBuiltinProfiles_ClassicHasNoMotionOrGlass(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows2000); err != nil {
		t.Fatal(err)
	}
	th := m.Active()

	for _, k := range []theme.Key{"hover", "menu.open", "window.open"} {
		if a, _ := th.Anim(k); a.Duration != 0 {
			t.Errorf("анимация %s длится %v — классика не должна двигаться", k, a.Duration)
		}
	}
	if s := th.Style("taskbar", "", theme.StateNormal); s.Backdrop.Mode != theme.BackdropNone {
		t.Error("классическая панель просит подложку — прозрачности в этой теме нет")
	}
	if s := th.Style("startbutton", "", theme.StateNormal); s.Bevel == nil {
		t.Error("у классической кнопки «Пуск» нет фаски")
	}
	// Нажатая кнопка «вдавливается» — это и есть классический отклик.
	if s := th.Style("startbutton", "", theme.StatePressed); s.Bevel == nil || !s.Bevel.Sunken {
		t.Error("нажатая классическая кнопка не вдавлена")
	}
	if c, _ := th.Metric("control.corner"); c != 0 {
		t.Errorf("в классике скругление %v, ждали 0", c)
	}
}

// TestBuiltinProfiles_ModernAskForGlass — современные темы объявляют
// размытую подложку честно, а не подделывают её плоским цветом.
func TestBuiltinProfiles_ModernAskForGlass(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{theme.ProfileWindows11, theme.ProfileWindows11Dark, theme.ProfileMacOS} {
		if err := m.SetTheme(name); err != nil {
			t.Fatal(err)
		}
		s := m.Active().Style("taskbar", "", theme.StateNormal)
		if s.Backdrop.Mode != theme.BackdropBlur {
			t.Errorf("%s: панель задач без стекла", name)
		}
		if s.Backdrop.Radius <= 0 {
			t.Errorf("%s: нулевой радиус размытия", name)
		}
		if s.Backdrop.Tint.A == 0 {
			t.Errorf("%s: подкраска стекла прозрачна — сквозь него будет видно всё как есть", name)
		}
	}
}

// TestBuiltinProfiles_MacSuppliesDockPresenter — macOS меняет не палитру, а
// форму: область приложений рисуется доком. Тема приносит презентер, и это
// единственное место, где такое решение объявляется.
func TestBuiltinProfiles_MacSuppliesDockPresenter(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileMacOS); err != nil {
		t.Fatal(err)
	}
	if got := m.Active().PresenterName("runningapps"); got != "dock" {
		t.Errorf("презентер области приложений = %q, ждали dock", got)
	}
	// Тёмная разновидность наследует презентер, не объявляя его снова.
	if err := m.SetTheme(theme.ProfileMacOSDark); err != nil {
		t.Fatal(err)
	}
	if got := m.Active().PresenterName("runningapps"); got != "dock" {
		t.Errorf("тёмная macOS потеряла презентер: %q", got)
	}
	// Windows-темы дока не просят.
	if err := m.SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	if got := m.Active().PresenterName("runningapps"); got != "" {
		t.Errorf("Windows 11 просит презентер %q", got)
	}
}
