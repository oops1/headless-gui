package theme_test

import (
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
)

// Компоненты, у которых профили Windows 11 объявляют материал и тень.
var materialComponents = []string{"startmenu", "quicksettings", "notifications", "notificationcenter", "calendar", "window", "menu", "dialog"}

var allProfiles = []string{
	theme.ProfileWindows2000, theme.ProfileWindows2000Blue, theme.ProfileWindows10, theme.ProfileWindows10Dark,
	theme.ProfileWindows11, theme.ProfileWindows11Dark, theme.ProfileMacOS, theme.ProfileMacOSDark,
}

// Без флагов ни один встроенный профиль не называет материал и не объявляет
// токены тени: вид прежний.
func TestMaterials_OffByDefault(t *testing.T) {
	for _, name := range allProfiles {
		m := builtinManager(t, name)
		for _, comp := range materialComponents {
			for _, st := range []theme.State{theme.StateNormal, theme.StateHover} {
				s := m.GetStyle(comp, "", st)
				if s.Backdrop.Material != theme.MaterialDefault {
					t.Errorf("%s/%s: материал %v объявлен без флага", name, comp, s.Backdrop.Material)
				}
				if s.ShadowBlur != 0 || s.ShadowOpacity != 0 || s.ShadowOffsetX != 0 || s.ShadowOffsetY != 0 {
					t.Errorf("%s/%s: токены тени объявлены без флага: %+v", name, comp, s)
				}
			}
		}
	}
}

// Mica включается флагом на панелях Windows 11 и берёт цвет из токенов темы:
// светлая светлее тёмной, MicaAlt темнее Mica.
func TestMaterials_MicaByFlag(t *testing.T) {
	light := builtinManager(t, theme.ProfileWindows11)
	dark := builtinManager(t, theme.ProfileWindows11Dark)
	for _, m := range []*theme.Manager{light, dark} {
		m.SetFlag(theme.FlagBackdropMica, true)
	}
	for _, comp := range []string{"startmenu", "quicksettings", "notificationcenter", "calendar", "window"} {
		l := light.GetStyle(comp, "", theme.StateNormal).Backdrop
		d := dark.GetStyle(comp, "", theme.StateNormal).Backdrop
		if l.Material != theme.MaterialMica || d.Material != theme.MaterialMica {
			t.Fatalf("%s: материал %v / %v, ждали Mica", comp, l.Material, d.Material)
		}
		if l.Noise != 0 {
			t.Errorf("%s: у Mica не должно быть шума, а он %v", comp, l.Noise)
		}
		if l.Fallback != theme.RGB(243, 243, 243) || d.Fallback != theme.RGB(32, 32, 32) {
			t.Errorf("%s: запасной цвет %v / %v — ждали surface темы", comp, l.Fallback, d.Fallback)
		}
		if lum(d.Tint) >= lum(l.Tint) {
			t.Errorf("%s: тёмная подкраска %v не темнее светлой %v", comp, d.Tint, l.Tint)
		}
	}
	// MicaAlt темнее Mica в обеих темах.
	for _, m := range []*theme.Manager{light, dark} {
		mica := m.GetStyle("startmenu", "", theme.StateNormal).Backdrop
		m.SetFlag(theme.FlagBackdropMicaAlt, true)
		alt := m.GetStyle("startmenu", "", theme.StateNormal).Backdrop
		if alt.Material != theme.MaterialMicaAlt {
			t.Fatalf("материал %v, ждали MicaAlt", alt.Material)
		}
		if lum(alt.Fallback) >= lum(mica.Fallback) {
			t.Errorf("MicaAlt %v не темнее Mica %v", alt.Fallback, mica.Fallback)
		}
		if lum(alt.Tint) >= lum(mica.Tint) {
			t.Errorf("MicaAlt: подкраска %v не темнее, чем у Mica %v", alt.Tint, mica.Tint)
		}
	}
	// Флаг можно снять на лету — панели возвращаются к сплошной заливке.
	light.SetFlag(theme.FlagBackdropMica, false)
	light.SetFlag(theme.FlagBackdropMicaAlt, false)
	if mat := light.GetStyle("startmenu", "", theme.StateNormal).Backdrop.Material; mat != theme.MaterialDefault {
		t.Errorf("после снятия флагов материал %v", mat)
	}
}

// Части панели (плитки, строки, карточки) не наследуют ни материал, ни токены
// тени панели — иначе каждая плитка получила бы свою Mica и свою тень.
func TestMaterials_PartsDoNotInherit(t *testing.T) {
	m := builtinManager(t, theme.ProfileWindows11)
	m.SetFlag(theme.FlagBackdropMica, true)
	m.SetFlag(theme.FlagShadowSoft, true)
	for _, c := range []struct{ comp, part string }{
		{"quicksettings", "tile.network"}, {"quicksettings", "slider"},
		{"notifications", "card.info"}, {"calendar", "day"}, {"startmenu", "section"},
	} {
		for _, st := range []theme.State{theme.StateNormal, theme.StateHover, theme.StateActive} {
			s := m.GetStyle(c.comp, c.part, st)
			if s.Backdrop.Material != theme.MaterialDefault || s.ShadowBlur != 0 {
				t.Errorf("%s.%s/%v унаследовала материал %v или тень %v", c.comp, c.part, st, s.Backdrop.Material, s.ShadowBlur)
			}
		}
	}
	// А сама панель получила и то и другое.
	if s := m.GetStyle("quicksettings", "", theme.StateNormal); s.Backdrop.Material != theme.MaterialMica || s.ShadowBlur == 0 {
		t.Errorf("панель: материал %v, тень %v", s.Backdrop.Material, s.ShadowBlur)
	}
}

// Мягкие тени: токены стиля по флагу; цвет и непрозрачность — токен темы.
func TestShadowTokens_ByFlag(t *testing.T) {
	m := builtinManager(t, theme.ProfileWindows11)
	legacy, ok := m.GetStyle("startmenu", "", theme.StateNormal).ResolveShadow()
	if !ok || legacy.Blur != 12 || legacy.OffsetY != 6 || legacy.OffsetX != 0 {
		t.Fatalf("прежняя тень панели = %+v, ждали размытие 12, смещение 6 (из Elevation)", legacy)
	}
	m.SetFlag(theme.FlagShadowSoft, true)
	for _, comp := range []string{"menu", "startmenu", "quicksettings", "notificationcenter", "calendar", "window", "dialog"} {
		sp, ok := m.GetStyle(comp, "", theme.StateNormal).ResolveShadow()
		if !ok || sp.Blur < 20 || sp.OffsetY < 8 || sp.Color.A == 0 {
			t.Errorf("%s: мягкая тень = %+v ok=%v", comp, sp, ok)
		}
		if comp == "window" || comp == "dialog" {
			if sp.Blur <= 20 {
				t.Errorf("%s: тень окна должна быть крупнее тени меню: %+v", comp, sp)
			}
		}
	}
	// Тёмная тема — плотнее.
	d := builtinManager(t, theme.ProfileWindows11Dark)
	d.SetFlag(theme.FlagShadowSoft, true)
	ls, _ := m.GetStyle("menu", "", theme.StateNormal).ResolveShadow()
	ds, _ := d.GetStyle("menu", "", theme.StateNormal).ResolveShadow()
	if ds.Color.A <= ls.Color.A {
		t.Errorf("тёмная тень %v не плотнее светлой %v", ds.Color, ls.Color)
	}
}

// ShadowOpacity множит альфу цвета, а цвет остаётся premultiplied; без токенов
// тень считается из Elevation, как раньше.
func TestStyle_ResolveShadow(t *testing.T) {
	s := &theme.Style{Elevation: 8, Shadow: theme.RGBA(0, 0, 0, 70)}
	sp, ok := s.ResolveShadow()
	if !ok || sp.Blur != 8 || sp.OffsetY != 4 || sp.Color != s.Shadow {
		t.Errorf("тень из Elevation = %+v", sp)
	}
	s = &theme.Style{ShadowBlur: 16, ShadowOffsetX: 2, ShadowOffsetY: 6, ShadowOpacity: 0.5}
	sp, ok = s.ResolveShadow()
	if !ok || sp.Blur != 16 || sp.OffsetX != 2 || sp.OffsetY != 6 {
		t.Fatalf("тень по токенам = %+v", sp)
	}
	if sp.Color.A < 126 || sp.Color.A > 129 || sp.Color.R != 0 {
		t.Errorf("цвет по умолчанию — чёрный с альфой по ShadowOpacity, а он %v", sp.Color)
	}
	// Цвет с альфой: каналы не превышают альфу.
	s = &theme.Style{ShadowBlur: 10, Shadow: theme.RGBA(40, 40, 40, 200), ShadowOpacity: 0.5}
	sp, _ = s.ResolveShadow()
	if sp.Color.R > sp.Color.A {
		t.Errorf("нарушена премультипликация: %v", sp.Color)
	}
	if _, ok := (&theme.Style{}).ResolveShadow(); ok {
		t.Error("пустой стиль не должен давать тень")
	}
	if got := (theme.ShadowSpec{Blur: 20, OffsetY: 8, Color: theme.RGB(0, 0, 0)}).Extent(); got < 48 {
		t.Errorf("запас под тень %d меньше размытия×2 + смещения", got)
	}
}

// JSON: материал и токены тени читаются из файла.
func TestLoadTheme_MaterialAndShadow(t *testing.T) {
	src := `{"name":"X","styles":{"panel":{"shadow_blur":24,"shadow_offset_y":8,"shadow_opacity":0.3,
	 "backdrop":{"mode":"blur","material":"mica-alt","radius":64,"tint":"#202020C8","fallback":"#202020"}}}}`
	res, err := theme.LoadTheme(strings.NewReader(src))
	if err != nil || len(res.Warnings) != 0 {
		t.Fatalf("err=%v warnings=%v", err, res.Warnings)
	}
	d := res.Profile.Styles[theme.StyleKey{Component: "panel"}]
	if d.ShadowBlur == nil || *d.ShadowBlur != 24 || *d.ShadowOffsetY != 8 || *d.ShadowOpacity != 0.3 {
		t.Errorf("токены тени не прочитаны: %+v", d)
	}
	if d.Backdrop == nil || d.Backdrop.Material != theme.MaterialMicaAlt || d.Backdrop.Radius != 64 {
		t.Errorf("материал не прочитан: %+v", d.Backdrop)
	}
	res, _ = theme.LoadTheme(strings.NewReader(`{"name":"Y","styles":{"p":{"backdrop":{"material":"glass"}}}}`))
	if len(res.Warnings) == 0 {
		t.Error("неизвестный материал должен дать предупреждение")
	}
}

// ─── Меньше движения ────────────────────────────────────────────────────────

func TestMotionReduce_DecorativeAndFunctional(t *testing.T) {
	m := builtinManager(t, theme.ProfileWindows10)
	hover := m.GetAnimation("hover")
	expand := m.GetAnimation("notification.expand")
	if hover.Duration <= 0 || expand.Duration <= 0 {
		t.Fatalf("у Windows 10 без флага анимации объявлены: hover=%v expand=%v", hover, expand)
	}
	if m.MotionReduced() {
		t.Fatal("флаг motion.reduce выключен по умолчанию")
	}

	m.SetFlag(theme.FlagMotionReduce, true)
	if !m.MotionReduced() {
		t.Fatal("флаг не принят")
	}
	for _, k := range []theme.Key{"hover", "menu.open", "window.open", "taskbar.item", "taskbar.slide", "dock.magnify", "notification.expand"} {
		if a := m.GetAnimation(k); a.Duration != 0 {
			t.Errorf("%s при «меньше движения»: %v", k, a.Duration)
		}
	}
	// Кривая не теряется, сырое значение доступно.
	if raw := m.GetAnimationRaw("hover"); raw.Duration != hover.Duration || raw.Curve != hover.Curve {
		t.Errorf("GetAnimationRaw = %+v, ждали %+v", raw, hover)
	}

	m.SetFlag(theme.FlagMotionReduce, false)
	if a := m.GetAnimation("hover"); a != hover {
		t.Errorf("после выключения hover = %+v, было %+v", a, hover)
	}
}

// Функциональная анимация укорачивается до метрики профиля, а не обнуляется,
// когда профиль об этом просит; декоративная — всегда нуль.
func TestMotionReduce_FunctionalCap(t *testing.T) {
	m := theme.NewManager()
	p := theme.NewProfile("M")
	p.Anims["a.expand"] = theme.AnimSpec{Duration: 300 * time.Millisecond, Curve: "out-cubic"}
	p.Anims["a.fade"] = theme.AnimSpec{Duration: 300 * time.Millisecond}
	p.SetMetric(theme.KeyMotionReduceFunctionalMS, 60)
	p.SetFlag(theme.FlagMotionReduce, true)
	if err := m.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("M"); err != nil {
		t.Fatal(err)
	}
	theme.RegisterAnimationKind("a.expand", theme.AnimFunctional)
	if k := theme.AnimationKindOf("a.expand"); k != theme.AnimFunctional {
		t.Fatalf("род = %v", k)
	}
	if k := theme.AnimationKindOf("a.fade"); k != theme.AnimDecorative {
		t.Fatalf("незарегистрированный токен должен быть декоративным, а он %v", k)
	}
	if a := m.GetAnimation("a.expand"); a.Duration != 60*time.Millisecond || a.Curve != "out-cubic" {
		t.Errorf("функциональная = %+v, ждали 60 мс", a)
	}
	if a := m.GetAnimation("a.fade"); a.Duration != 0 {
		t.Errorf("декоративная = %v, ждали 0", a.Duration)
	}
	// Короче метрики — не удлиняется.
	p2 := theme.NewProfile("N")
	p2.Anims["a.expand"] = theme.AnimSpec{Duration: 20 * time.Millisecond}
	p2.SetMetric(theme.KeyMotionReduceFunctionalMS, 60)
	p2.SetFlag(theme.FlagMotionReduce, true)
	_ = m.RegisterTheme(p2)
	_ = m.SetTheme("N")
	if a := m.GetAnimation("a.expand"); a.Duration != 20*time.Millisecond {
		t.Errorf("короткая анимация изменилась: %v", a.Duration)
	}
}

// Windows 2000 и без флага не анимирует, с флагом тоже: ничего не ломается.
func TestMotionReduce_Windows2000(t *testing.T) {
	m := builtinManager(t, theme.ProfileWindows2000)
	m.SetFlag(theme.FlagMotionReduce, true)
	if a := m.GetAnimation("hover"); a.Duration != 0 {
		t.Errorf("Windows 2000: %v", a.Duration)
	}
}
