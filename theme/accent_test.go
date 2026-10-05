package theme_test

import (
	"image/color"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
)

// builtinManager — менеджер со всеми встроенными профилями и активной темой.
func builtinManager(t *testing.T, active string) *theme.Manager {
	t.Helper()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(active); err != nil {
		t.Fatal(err)
	}
	return m
}

func lum(c color.RGBA) int { return int(c.R) + int(c.G) + int(c.B) }

// ─── Производные ────────────────────────────────────────────────────────────

// TestDeriveAccent — наведение светлее, нажатие и тёмный оттенок темнее,
// светлый светлее; текст на акценте читается: белый на синем, чёрный на
// жёлтом.
func TestDeriveAccent(t *testing.T) {
	blue := theme.DeriveAccent(theme.RGB(0, 120, 215))
	if lum(blue.Hover) <= lum(blue.Base) {
		t.Errorf("наведение %v не светлее базы %v", blue.Hover, blue.Base)
	}
	if lum(blue.Pressed) >= lum(blue.Base) || lum(blue.Dark) >= lum(blue.Pressed) {
		t.Errorf("нажатие %v и тёмный %v должны быть темнее базы, тёмный — темнее нажатия", blue.Pressed, blue.Dark)
	}
	if lum(blue.Light) <= lum(blue.Hover) {
		t.Errorf("светлый оттенок %v не светлее наведения %v", blue.Light, blue.Hover)
	}
	if blue.Text != theme.RGB(255, 255, 255) {
		t.Errorf("на синем акценте Windows текст %v, ждали белый", blue.Text)
	}
	for _, c := range []color.RGBA{blue.Base, blue.Hover, blue.Pressed, blue.Dark, blue.Light, blue.Text} {
		if c.A != 255 {
			t.Errorf("производная непрозрачна не полностью: %v", c)
		}
	}

	if yellow := theme.DeriveAccent(theme.RGB(255, 185, 0)); yellow.Text != theme.RGB(0, 0, 0) {
		t.Errorf("на жёлтом акценте текст %v, ждали чёрный", yellow.Text)
	}
	// Все акценты, которые встроенные темы объявляют сами, остаются с белым
	// текстом: иначе смена токена переписала бы вид существующих тем.
	for _, a := range []color.RGBA{theme.RGB(0, 120, 215), theme.RGB(0, 103, 192), theme.RGB(0, 122, 255)} {
		if theme.DeriveAccent(a).Text != theme.RGB(255, 255, 255) {
			t.Errorf("встроенный акцент %v получил не белый текст", a)
		}
	}
	// Прозрачность базы отбрасывается.
	if got := theme.DeriveAccent(color.RGBA{R: 10, G: 10, B: 10, A: 20}).Base.A; got != 255 {
		t.Errorf("альфа базы = %d, ждали 255", got)
	}
}

// ─── SetAccent ──────────────────────────────────────────────────────────────

// TestSetAccent_RecolorsReferencingStyles — акцент меняет и токены, и стили,
// которые на них ссылаются; не ссылающиеся не трогаются.
func TestSetAccent_RecolorsReferencingStyles(t *testing.T) {
	m := builtinManager(t, theme.ProfileWindows10)
	orange := theme.RGB(255, 140, 0)

	before := m.Active()
	m.SetAccent(orange)
	after := m.Active()
	if before == after {
		t.Fatal("SetAccent не пересобрал активную тему")
	}

	if got, _ := after.Color(theme.KeyAccent); got != orange {
		t.Errorf("токен accent = %v, ждали %v", got, orange)
	}
	if got, _ := after.Color(theme.KeySelection); got != orange {
		t.Errorf("выделение %v не последовало за акцентом", got)
	}
	d := theme.DeriveAccent(orange)
	for k, want := range map[theme.Key]color.RGBA{
		theme.KeyAccentHover: d.Hover, theme.KeyAccentPressed: d.Pressed,
		theme.KeyAccentDark: d.Dark, theme.KeyAccentLight: d.Light, theme.KeyAccentText: d.Text,
	} {
		if got, _ := after.Color(k); got != want {
			t.Errorf("токен %s = %v, ждали %v", k, got, want)
		}
	}

	// Ссылающиеся стили: заголовок активного окна, плитка «Пуска», флажок
	// дня в календаре, полоса под кнопкой окна.
	checks := []struct {
		name  string
		get   func(*theme.Theme) color.RGBA
		wantF color.RGBA
	}{
		{"заголовок окна", func(th *theme.Theme) color.RGBA {
			return th.Style("window", "titlebar", theme.StateFocused).Fill
		}, orange},
		{"плитка «Пуска»", func(th *theme.Theme) color.RGBA {
			return th.Style("startmenu", "tile", theme.StateNormal).Fill
		}, orange},
		{"плитка под курсором: другая заливка", func(th *theme.Theme) color.RGBA {
			return th.Style("notificationcenter", "quick.tile.on", theme.StateHover).Fill
		}, d.Hover},
		{"день календаря", func(th *theme.Theme) color.RGBA {
			return th.Style("calendar", "day", theme.StateActive).Fill
		}, orange},
		{"полоса под кнопкой окна", func(th *theme.Theme) color.RGBA {
			return th.Style("taskbutton", "", theme.StateActive).Border
		}, orange},
		{"текст на акценте", func(th *theme.Theme) color.RGBA {
			return th.Style("startmenu", "tile", theme.StateNormal).Text
		}, d.Text},
	}
	for _, c := range checks {
		if got := c.get(after); got != c.wantF {
			t.Errorf("%s: %v, ждали %v", c.name, got, c.wantF)
		}
		if got := c.get(before); got == c.wantF && c.wantF == orange {
			t.Errorf("%s: старая тема тоже оранжевая — снимок изменился задним числом", c.name)
		}
	}
	if acc, ok := m.Accent(); !ok || acc != orange {
		t.Errorf("Manager.Accent() = %v, %v", acc, ok)
	}
}

// TestSetAccent_NotifiesAndSurvivesThemeSwitch — наблюдатели получают новую
// тему; выбор переживает смену профиля; ResetAccent возвращает акцент профиля.
func TestSetAccent_NotifiesAndSurvivesThemeSwitch(t *testing.T) {
	m := builtinManager(t, theme.ProfileWindows10)
	var calls int32
	var last atomic.Pointer[theme.Theme]
	unsub := m.Subscribe(theme.ObserverFunc(func(th *theme.Theme) {
		atomic.AddInt32(&calls, 1)
		last.Store(th)
	}))
	defer unsub()

	red := theme.RGB(200, 30, 30)
	m.SetAccent(red)
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("наблюдатель вызван %d раз, ждали 1", calls)
	}
	if last.Load() != m.Active() {
		t.Error("наблюдателю отдали не активную тему")
	}
	m.SetAccent(red) // тот же цвет
	if atomic.LoadInt32(&calls) != 1 {
		t.Error("повторная установка того же акцента разослана заново")
	}

	if err := m.SetTheme(theme.ProfileWindows11Dark); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Active().Color(theme.KeyAccent); got != red {
		t.Errorf("после смены темы акцент %v, ждали выбранный %v", got, red)
	}

	m.ResetAccent()
	if got, _ := m.Active().Color(theme.KeyAccent); got != theme.RGB(0, 103, 192) {
		t.Errorf("после ResetAccent акцент %v, ждали акцент Windows 11", got)
	}
	m.ResetAccent() // уже сброшен
	// SetTheme + ResetAccent: три рассылки (SetAccent, SetTheme, ResetAccent).
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("всего рассылок %d, ждали 3", got)
	}
}

// TestSetAccent_BeforeAnyTheme — акцент можно выбрать до первой темы: он
// ляжет на неё при SetTheme.
func TestSetAccent_BeforeAnyTheme(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	m.SetAccent(theme.RGB(10, 200, 100))
	if m.Active() != nil {
		t.Fatal("SetAccent сам выбрал тему")
	}
	if err := m.SetTheme(theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Active().Color(theme.KeyAccent); got != theme.RGB(10, 200, 100) {
		t.Errorf("акцент = %v", got)
	}
}

// TestSetAccent_ClassicFollowsIt — Windows 2000 следует акценту: заголовок
// активного окна и выделение идут за ним (раньше были литералами и не менялись).
// Серая палитра и фаски остаются классическими. Синяя разновидность объявляет
// выделение сама, поэтому за акцентом идёт только её заголовок.
func TestSetAccent_ClassicFollowsIt(t *testing.T) {
	red := theme.RGB(255, 0, 0)
	m := builtinManager(t, theme.ProfileWindows2000)
	if got := m.Active().Style("window", "titlebar", theme.StateFocused).Fill; got != theme.RGB(10, 36, 106) {
		t.Fatalf("без SetAccent заголовок классики %v, ждали прежний тёмно-синий", got)
	}
	m.SetAccent(red)
	if got := m.Active().Style("window", "titlebar", theme.StateFocused).Fill; got != red {
		t.Errorf("заголовок классики %v, ждали акцент %v", got, red)
	}
	if got, _ := m.Active().Color(theme.KeySelection); got != red {
		t.Errorf("выделение классики %v, ждали акцент %v", got, red)
	}
	if got := m.Active().Style("menu", "item", theme.StateHover).Fill; got != red {
		t.Errorf("плашка меню классики %v, ждали акцент %v", got, red)
	}
	m.ResetAccent()
	if got := m.Active().Style("window", "titlebar", theme.StateFocused).Fill; got != theme.RGB(10, 36, 106) {
		t.Errorf("ResetAccent не вернул заголовок: %v", got)
	}

	blue := builtinManager(t, theme.ProfileWindows2000Blue)
	blue.SetAccent(red)
	if got := blue.Active().Style("window", "titlebar", theme.StateFocused).Fill; got != red {
		t.Errorf("заголовок синей классики %v, ждали акцент %v", got, red)
	}
	if got, _ := blue.Active().Color(theme.KeySelection); got != theme.RGB(10, 36, 106) {
		t.Errorf("выделение синей классики %v: профиль объявил его сам", got)
	}
}

// TestSetAccent_OverridesDeclaredDerived — производная, объявленная профилем,
// действует, пока акцент профиля; после SetAccent считается заново.
func TestSetAccent_OverridesDeclaredDerived(t *testing.T) {
	m := theme.NewManager()
	p := theme.NewProfile("custom")
	p.SetColor(theme.KeyAccent, theme.RGB(0, 0, 200)).
		SetColor(theme.KeyAccentHover, theme.RGB(1, 2, 3))
	if err := m.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("custom"); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Active().Color(theme.KeyAccentHover); got != theme.RGB(1, 2, 3) {
		t.Errorf("объявленное профилем наведение потеряно: %v", got)
	}
	if _, ok := m.Active().Color(theme.KeyAccentPressed); !ok {
		t.Error("недостающая производная не посчитана")
	}
	m.SetAccent(theme.RGB(200, 0, 0))
	if got, _ := m.Active().Color(theme.KeyAccentHover); got == theme.RGB(1, 2, 3) {
		t.Error("после SetAccent осталось наведение от старого акцента")
	}
}

// TestSetAccent_RegisterThemeKeepsIt — перерегистрация профиля не сбрасывает
// выбранный акцент.
func TestSetAccent_RegisterThemeKeepsIt(t *testing.T) {
	m := builtinManager(t, theme.ProfileWindows10)
	m.SetAccent(theme.RGB(1, 150, 80))
	if err := m.RegisterTheme(theme.Windows10Profile()); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Active().Color(theme.KeyAccent); got != theme.RGB(1, 150, 80) {
		t.Errorf("акцент после RegisterTheme = %v", got)
	}
}

// TestSetAccent_Concurrent — смена акцента из одной горутины при чтении из
// других (go test -race).
func TestSetAccent_Concurrent(t *testing.T) {
	m := builtinManager(t, theme.ProfileWindows10)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = m.GetStyle("startmenu", "tile", theme.StateNormal)
				_ = m.GetFlag(theme.KeyTaskbarLight, false)
			}
		}()
	}
	for i := 0; i < 50; i++ {
		m.SetAccent(theme.RGB(uint8(i*5), 100, 200))
		m.SetFlag(theme.KeyTaskbarLight, i%2 == 0)
	}
	wg.Wait()
}

// ─── Ссылки на токены ───────────────────────────────────────────────────────

// TestStyleDelta_TokenReference — ссылка раскрывается по итоговым токенам
// цепочки: потомок подменяет токен, не переписывая стили; свой литерал потомка
// перекрывает ссылку предка; ссылка на несуществующий токен ничего не ломает.
func TestStyleDelta_TokenReference(t *testing.T) {
	m := theme.NewManager()
	parent := theme.NewProfile("parent")
	parent.SetColor("brand", theme.RGB(10, 20, 30))
	parent.SetStyle("box", "", theme.StateNormal, theme.StyleDelta{
		FillFrom: "brand", Fill: theme.C(theme.RGB(1, 1, 1)), TextFrom: "no.such.token",
		Text: theme.C(theme.RGB(9, 9, 9)),
	})
	parent.SetStyle("lit", "", theme.StateNormal, theme.StyleDelta{FillFrom: "brand"})
	child := theme.NewProfile("child")
	child.Parent = "parent"
	child.SetColor("brand", theme.RGB(40, 50, 60))
	child.SetStyle("lit", "", theme.StateNormal, theme.StyleDelta{Fill: theme.C(theme.RGB(7, 7, 7))})
	for _, p := range []*theme.Profile{parent, child} {
		if err := m.RegisterTheme(p); err != nil {
			t.Fatal(err)
		}
	}

	th, _ := m.GetTheme("parent")
	s := th.Style("box", "", theme.StateNormal)
	if s.Fill != theme.RGB(10, 20, 30) {
		t.Errorf("ссылка победила литерал той же дельты: %v", s.Fill)
	}
	if s.Text != theme.RGB(9, 9, 9) {
		t.Errorf("ссылка на отсутствующий токен затёрла цвет: %v", s.Text)
	}

	th, _ = m.GetTheme("child")
	if got := th.Style("box", "", theme.StateNormal).Fill; got != theme.RGB(40, 50, 60) {
		t.Errorf("потомок подменил токен, а стиль не последовал: %v", got)
	}
	if got := th.Style("lit", "", theme.StateNormal).Fill; got != theme.RGB(7, 7, 7) {
		t.Errorf("литерал потомка не перекрыл ссылку предка: %v", got)
	}
}

// TestLoadTheme_TokenReference — «@accent» в JSON-профиле — ссылка.
func TestLoadTheme_TokenReference(t *testing.T) {
	src := `{"name":"j","colors":{"accent":"#0078D7"},
		"styles":{"btn":{"fill":"@accent","text":"@accent.text","border":"#112233"},
		          "bad":{"fill":"@"}}}`
	res, err := theme.LoadTheme(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "пустое имя токена") {
		t.Errorf("предупреждения: %v", res.Warnings)
	}
	m := theme.NewManager()
	if err := m.RegisterTheme(res.Profile); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("j"); err != nil {
		t.Fatal(err)
	}
	st := m.Active().Style("btn", "", theme.StateNormal)
	if st.Fill != theme.RGB(0, 120, 215) || st.Text != theme.RGB(255, 255, 255) {
		t.Errorf("ссылки из JSON не раскрылись: fill %v text %v", st.Fill, st.Text)
	}
	m.SetAccent(theme.RGB(200, 0, 0))
	if got := m.Active().Style("btn", "", theme.StateNormal).Fill; got != theme.RGB(200, 0, 0) {
		t.Errorf("стиль из JSON не последовал за акцентом: %v", got)
	}
}

// ─── Условные стили и флаги ─────────────────────────────────────────────────

// TestTaskbarLightFlag — по умолчанию панель Windows 10 тёмная (как была);
// флаг, выставленный менеджером на лету, делает её светлой, и наоборот.
func TestTaskbarLightFlag(t *testing.T) {
	m := builtinManager(t, theme.ProfileWindows10)
	dark := m.Active()
	if on, _ := dark.Flag(theme.KeyTaskbarLight); on {
		t.Fatal("светлая панель включена по умолчанию")
	}
	if got := dark.Style("taskbutton", "", theme.StateNormal).Text; got != theme.RGB(230, 230, 230) {
		t.Errorf("текст кнопок тёмной панели %v изменился", got)
	}

	var calls int32
	defer m.Subscribe(theme.ObserverFunc(func(*theme.Theme) { atomic.AddInt32(&calls, 1) }))()

	m.SetFlag(theme.KeyTaskbarLight, true)
	light := m.Active()
	if on, _ := light.Flag(theme.KeyTaskbarLight); !on {
		t.Fatal("SetFlag не поднял флаг")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("рассылок %d, ждали 1", calls)
	}
	bar := light.Style("taskbar", "", theme.StateNormal)
	if lum(bar.Backdrop.Tint) <= lum(dark.Style("taskbar", "", theme.StateNormal).Backdrop.Tint) ||
		lum(bar.Backdrop.Fallback) < 3*200 {
		t.Errorf("панель не посветлела: подкраска %v, запасной %v", bar.Backdrop.Tint, bar.Backdrop.Fallback)
	}
	if bar.Backdrop.Mode != theme.BackdropBlur || bar.Backdrop.Noise <= 0 {
		t.Errorf("светлая панель потеряла акрил: %+v", bar.Backdrop)
	}
	for _, comp := range []string{"startbutton", "taskbutton", "clock", "tray.volume"} {
		if lum(light.Style(comp, "", theme.StateNormal).Text) > 3*60 {
			t.Errorf("%s: текст на светлой панели не тёмный", comp)
		}
	}
	// Геометрия не меняется: флаг правит цвет, а не раскладку.
	if light.Style("taskbutton", "", theme.StateNormal).PadX != dark.Style("taskbutton", "", theme.StateNormal).PadX {
		t.Error("флаг изменил отступы кнопки")
	}
	// Плёнка наведения на светлом — тёмная, на тёмном — светлая.
	if h := light.Style("taskbutton", "", theme.StateHover).Fill; h.R > h.A/2 {
		t.Errorf("наведение на светлой панели светлое: %v", h)
	}

	m.ResetFlag(theme.KeyTaskbarLight)
	if got := m.Active().Style("taskbutton", "", theme.StateNormal).Text; got != theme.RGB(230, 230, 230) {
		t.Errorf("после ResetFlag панель не вернулась: %v", got)
	}
	m.SetFlag(theme.KeyTaskbarLight, false)
	if got := m.Active().Style("taskbar", "", theme.StateNormal).Backdrop.Tint; got != dark.Style("taskbar", "", theme.StateNormal).Backdrop.Tint {
		t.Errorf("флаг, выставленный в false, не совпал с умолчанием: %v", got)
	}
}

// TestTaskbarLightFlag_FromProfile — профиль-потомок поднимает флаг данными.
func TestTaskbarLightFlag_FromProfile(t *testing.T) {
	m := builtinManager(t, theme.ProfileWindows10)
	p := theme.NewProfile("W10 Light")
	p.Parent = theme.ProfileWindows10
	p.SetFlag(theme.KeyTaskbarLight, true)
	if err := m.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("W10 Light"); err != nil {
		t.Fatal(err)
	}
	if got := m.Active().Style("clock", "", theme.StateNormal).Text; lum(got) > 3*60 {
		t.Errorf("текст часов %v: светлая панель не включилась", got)
	}
}

// TestSetStyleWhen_LayersOverUnconditional — условное правило ложится поверх
// обычного правила предка и перекрывается обычным правилом потомка.
func TestSetStyleWhen_LayersOverUnconditional(t *testing.T) {
	m := theme.NewManager()
	base := theme.NewProfile("base")
	base.SetFlag("on", true)
	base.SetStyle("c", "", theme.StateNormal, theme.StyleDelta{Fill: theme.C(theme.RGB(1, 1, 1)), PadX: theme.N(4)})
	base.SetStyleWhen("on", "c", "", theme.StateNormal, theme.StyleDelta{Fill: theme.C(theme.RGB(2, 2, 2))})
	base.SetStyleWhen("off", "c", "", theme.StateNormal, theme.StyleDelta{PadX: theme.N(99)})
	child := theme.NewProfile("child")
	child.Parent = "base"
	child.SetStyle("c", "", theme.StateHover, theme.StyleDelta{Text: theme.C(theme.RGB(3, 3, 3))})
	for _, p := range []*theme.Profile{base, child} {
		if err := m.RegisterTheme(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.SetTheme("child"); err != nil {
		t.Fatal(err)
	}
	s := m.Active().Style("c", "", theme.StateNormal)
	if s.Fill != theme.RGB(2, 2, 2) || s.PadX != 4 {
		t.Errorf("условное правило легло неверно: fill %v padx %v", s.Fill, s.PadX)
	}
	m.SetFlag("off", true)
	if got := m.Active().Style("c", "", theme.StateNormal).PadX; got != 99 {
		t.Errorf("флаг off не включил правило: %v", got)
	}
	m.SetFlag("on", false)
	if got := m.Active().Style("c", "", theme.StateNormal).Fill; got != theme.RGB(1, 1, 1) {
		t.Errorf("флаг on=false не выключил правило: %v", got)
	}
}

// ─── Наследование компонента ────────────────────────────────────────────────

// TestNotificationCenter_InheritsNotifications — новое имя компонента для
// каждой встроенной темы выглядит в точности как notifications: так ничего не
// меняется для существующих тем.
func TestNotificationCenter_InheritsNotifications(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	parts := []string{"", "card.info", "card.warning", "card.error", "empty", "clear"}
	states := []theme.State{theme.StateNormal, theme.StateHover, theme.StatePressed, theme.StateActive}
	for _, name := range m.ThemeNames() {
		th, _ := m.GetTheme(name)
		for _, part := range parts {
			for _, st := range states {
				want := th.Style("notifications", part, st)
				got := th.Style("notificationcenter", part, st)
				if got.Fill != want.Fill || got.Text != want.Text || got.Border != want.Border ||
					got.Corner != want.Corner || got.PadX != want.PadX || got.PadY != want.PadY ||
					got.BorderWidth != want.BorderWidth || got.Elevation != want.Elevation {
					t.Errorf("%s: notificationcenter.%s[%v] расходится с notifications", name, part, st)
				}
			}
		}
	}
}

// TestStyleBase_OwnRulesWin — собственное правило производного компонента
// перекрывает унаследованное, остальное достаётся от базы.
func TestStyleBase_OwnRulesWin(t *testing.T) {
	m := theme.NewManager()
	p := theme.NewProfile("p")
	p.SetStyle("old", "", theme.StateNormal, theme.StyleDelta{Fill: theme.C(theme.RGB(1, 1, 1)), PadX: theme.N(5)})
	p.SetStyle("old", "part", theme.StateHover, theme.StyleDelta{Text: theme.C(theme.RGB(8, 8, 8))})
	p.SetStyle("new", "", theme.StateNormal, theme.StyleDelta{Fill: theme.C(theme.RGB(2, 2, 2))})
	p.SetStyleBase("new", "old")
	if err := m.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("p"); err != nil {
		t.Fatal(err)
	}
	s := m.Active().Style("new", "", theme.StateNormal)
	if s.Fill != theme.RGB(2, 2, 2) || s.PadX != 5 {
		t.Errorf("fill %v padx %v: свой цвет и унаследованный отступ", s.Fill, s.PadX)
	}
	if got := m.Active().Style("new", "part", theme.StateHover).Text; got != theme.RGB(8, 8, 8) {
		t.Errorf("часть базы не унаследована: %v", got)
	}
}

// ─── Windows 10: акрил и панели ─────────────────────────────────────────────

// TestWindows10_AcrylicTaskbar — панель Windows 10: размытие, подкраска,
// шум и непрозрачный запасной цвет, равный прежнему цвету панели.
func TestWindows10_AcrylicTaskbar(t *testing.T) {
	for _, name := range []string{theme.ProfileWindows10, theme.ProfileWindows10Dark} {
		m := builtinManager(t, name)
		b := m.Active().Style("taskbar", "", theme.StateNormal).Backdrop
		if b.Mode != theme.BackdropBlur || b.Radius <= 0 {
			t.Errorf("%s: панель без размытия: %+v", name, b)
		}
		if b.Tint.A == 0 || b.Tint.A == 255 {
			t.Errorf("%s: подкраска должна быть полупрозрачной: %v", name, b.Tint)
		}
		if b.Noise <= 0 || b.Noise > 0.1 {
			t.Errorf("%s: шум %v вне разумного диапазона", name, b.Noise)
		}
		if b.Fallback != theme.RGB(31, 31, 31) {
			t.Errorf("%s: запасной цвет %v, ждали прежний RGB(31,31,31)", name, b.Fallback)
		}
	}
}

// TestWindows10_PanelParts — части «Пуска» и центра уведомлений объявлены,
// интерактивные доступны во всех состояниях, плитки ссылаются на акцент.
func TestWindows10_PanelParts(t *testing.T) {
	m := builtinManager(t, theme.ProfileWindows10)
	th := m.Active()
	for _, c := range []struct{ comp, part string }{
		{"startmenu", "panel"}, {"startmenu", "sidebar"},
		{"notificationcenter", "panel"},
	} {
		s := th.Style(c.comp, c.part, theme.StateNormal)
		if s.Backdrop.Mode != theme.BackdropBlur || s.Backdrop.Fallback.A != 255 {
			t.Errorf("%s.%s: нет акрила с запасным цветом: %+v", c.comp, c.part, s.Backdrop)
		}
		if s.Elevation != 0 || s.Shadow.A != 0 {
			t.Errorf("%s.%s унаследовал тень плоской панели", c.comp, c.part)
		}
	}
	for _, c := range []struct{ comp, part string }{
		{"startmenu", "row"}, {"startmenu", "sidebar.item"}, {"startmenu", "letter"},
		{"notificationcenter", "card"}, {"notificationcenter", "action"},
		{"notificationcenter", "group"}, {"notificationcenter", "quick.tile"},
	} {
		rest := th.Style(c.comp, c.part, theme.StateNormal)
		if rest.Border.A != 0 || rest.BorderWidth != 0 {
			t.Errorf("%s.%s в покое с рамкой", c.comp, c.part)
		}
		if th.Style(c.comp, c.part, theme.StateHover).Fill == rest.Fill {
			t.Errorf("%s.%s: наведение не отличается от покоя", c.comp, c.part)
		}
		if th.Style(c.comp, c.part, theme.StatePressed).Fill == th.Style(c.comp, c.part, theme.StateHover).Fill {
			t.Errorf("%s.%s: нажатие не отличается от наведения", c.comp, c.part)
		}
		if f := th.Style(c.comp, c.part, theme.StateFocused); f.BorderWidth <= 0 || f.Border.A == 0 {
			t.Errorf("%s.%s: у фокуса нет рамки", c.comp, c.part)
		}
		if th.Style(c.comp, c.part, theme.StateDisabled).Text.A == 0 {
			t.Errorf("%s.%s: у отключённого нет текста", c.comp, c.part)
		}
	}

	acc, _ := th.Color(theme.KeyAccent)
	if got := th.Style("startmenu", "tile", theme.StateNormal).Fill; got != acc {
		t.Errorf("плитка %v, акцент %v", got, acc)
	}
	if th.Style("startmenu", "tile", theme.StateHover).BorderWidth <= 0 {
		t.Error("у плитки под курсором нет рамки")
	}

	// Светлый режим переписывает палитру панелей, но не части.
	m.SetFlag(theme.KeyTaskbarLight, true)
	lt := m.Active()
	if lum(lt.Style("startmenu", "panel", theme.StateNormal).Backdrop.Fallback) < 3*200 {
		t.Error("светлый «Пуск» остался тёмным")
	}
	if lum(lt.Style("startmenu", "row", theme.StateNormal).Text) > 3*60 {
		t.Error("текст строки светлого «Пуска» не тёмный")
	}
	if got := lt.Style("startmenu", "tile", theme.StateNormal).Fill; got != acc {
		t.Errorf("плитка в светлом режиме потеряла акцент: %v", got)
	}
}

// TestBuiltinProfiles_NoNewEffectsForOtherThemes — Windows 2000, 11 и macOS
// не получили ни шума, ни запасных цветов, ни частей Windows 10: их вид не
// менялся.
func TestBuiltinProfiles_NoNewEffectsForOtherThemes(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		theme.ProfileWindows2000, theme.ProfileWindows2000Blue,
		theme.ProfileWindows11, theme.ProfileWindows11Dark, theme.ProfileMacOS, theme.ProfileMacOSDark,
	} {
		th, _ := m.GetTheme(name)
		for _, comp := range []string{"taskbar", "menu", "startmenu"} {
			b := th.Style(comp, "", theme.StateNormal).Backdrop
			if b.Noise != 0 || b.Fallback != (color.RGBA{}) {
				t.Errorf("%s.%s получил эффекты Windows 10: %+v", name, comp, b)
			}
		}
		if th.Style("startmenu", "panel", theme.StateNormal).Backdrop.Mode != theme.BackdropNone {
			t.Errorf("%s: появилась панель «Пуска» с акрилом", name)
		}
		if on, ok := th.Flag(theme.KeyTaskbarLight); ok && on {
			t.Errorf("%s: поднят флаг светлой панели", name)
		}
	}
}

// TestJSON_BackdropNoiseAndFallback — шум и запасной цвет читаются из файла.
func TestJSON_BackdropNoiseAndFallback(t *testing.T) {
	src := `{"name":"j","styles":{"taskbar":{"backdrop":
		{"mode":"blur","radius":20,"tint":"#1F1F1FD2","noise":0.02,"fallback":"#1F1F1F"}}}}`
	res, err := theme.LoadTheme(strings.NewReader(src))
	if err != nil || len(res.Warnings) != 0 {
		t.Fatalf("загрузка: %v %v", err, res.Warnings)
	}
	b := *res.Profile.Styles[theme.StyleKey{Component: "taskbar"}].Backdrop
	if b.Noise != 0.02 || b.Fallback != theme.RGB(31, 31, 31) {
		t.Errorf("не прочитано: %+v", b)
	}
}
