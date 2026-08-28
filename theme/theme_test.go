package theme_test

import (
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
)

// ─── Состояния ──────────────────────────────────────────────────────────────

// Набор битов всегда сводится к одному состоянию, и порядок именно тот, что
// объявлен: отключённый компонент выглядит отключённым, даже если курсор над
// ним, а фокус — самое слабое.
func TestState_Dominant(t *testing.T) {
	cases := []struct {
		name string
		in   theme.State
		want theme.State
	}{
		{"покой", theme.StateNormal, theme.StateNormal},
		{"одно состояние", theme.StateHover, theme.StateHover},
		{"отключён важнее наведения", theme.StateHover | theme.StateDisabled, theme.StateDisabled},
		{"нажат важнее наведения", theme.StateHover | theme.StatePressed, theme.StatePressed},
		{"выбран важнее наведения", theme.StateHover | theme.StateActive, theme.StateActive},
		{"наведение важнее фокуса", theme.StateHover | theme.StateFocused, theme.StateHover},
		{"всё сразу — отключён", theme.StateHover | theme.StatePressed | theme.StateActive |
			theme.StateFocused | theme.StateDisabled, theme.StateDisabled},
	}
	for _, c := range cases {
		if got := c.in.Dominant(); got != c.want {
			t.Errorf("%s: Dominant(%d) = %v, ждали %v", c.name, c.in, got, c.want)
		}
	}
}

func TestState_ParseAndString(t *testing.T) {
	for _, name := range []string{"normal", "hover", "active", "pressed", "disabled", "focused"} {
		st, err := theme.ParseState(name)
		if err != nil {
			t.Fatalf("ParseState(%q): %v", name, err)
		}
		if got := st.String(); got != name {
			t.Errorf("ParseState(%q).String() = %q", name, got)
		}
	}
	if _, err := theme.ParseState("hovered"); err == nil {
		t.Error("неизвестное состояние принято без ошибки")
	}
}

// ─── Наследование и разрешение ──────────────────────────────────────────────

// baseProfile — родитель: палитра, метрики и стиль кнопки в двух состояниях.
func baseProfile() *theme.Profile {
	p := theme.NewProfile("Base")
	p.SetColor("accent", theme.RGB(0, 120, 215)).
		SetColor("surface", theme.RGB(45, 45, 48)).
		SetColor("text", theme.RGB(240, 240, 240)).
		SetMetric("control.corner", 4).
		SetMetric("taskbar.height", 48)
	p.SetStyle("button", "", theme.StateNormal, theme.StyleDelta{
		Fill:   theme.C(theme.RGB(51, 51, 55)),
		Text:   theme.C(theme.RGB(241, 241, 241)),
		Corner: theme.N(4),
		PadX:   theme.N(12),
	})
	p.SetStyle("button", "", theme.StateHover, theme.StyleDelta{
		Fill: theme.C(theme.RGB(62, 62, 66)),
	})
	return p
}

// TestInheritance_ChildOverridesParent — потомок объявляет несколько токенов,
// остальное берёт у родителя.
func TestInheritance_ChildOverridesParent(t *testing.T) {
	m := theme.NewManager()
	if err := m.RegisterTheme(baseProfile()); err != nil {
		t.Fatal(err)
	}

	dark := theme.NewProfile("Base Dark")
	dark.Parent = "Base"
	dark.SetColor("surface", theme.RGB(24, 24, 24))
	dark.SetStyle("button", "", theme.StateNormal, theme.StyleDelta{
		Fill: theme.C(theme.RGB(32, 32, 32)),
	})
	if err := m.RegisterTheme(dark); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("Base Dark"); err != nil {
		t.Fatal(err)
	}

	th := m.Active()
	if got := th.Chain(); len(got) != 2 || got[0] != "Base" || got[1] != "Base Dark" {
		t.Errorf("цепочка наследования = %v, ждали [Base, Base Dark]", got)
	}

	// Переопределённое взято у потомка.
	if c, _ := th.Color("surface"); c != theme.RGB(24, 24, 24) {
		t.Errorf("surface = %v — не взят у потомка", c)
	}
	// Незаявленное — у родителя.
	if c, _ := th.Color("accent"); c != theme.RGB(0, 120, 215) {
		t.Errorf("accent = %v — не унаследован", c)
	}
	if v, _ := th.Metric("taskbar.height"); v != 48 {
		t.Errorf("taskbar.height = %v — не унаследована", v)
	}

	// Стиль: заливка от потомка, отступ и скругление — от родителя.
	s := th.Style("button", "", theme.StateNormal)
	if s.Fill != theme.RGB(32, 32, 32) {
		t.Errorf("button.Fill = %v, ждали цвет потомка", s.Fill)
	}
	if s.PadX != 12 || s.Corner != 4 {
		t.Errorf("button: PadX=%v Corner=%v — не унаследованы от родителя", s.PadX, s.Corner)
	}
}

// TestStateFallback_UnstatedStateFallsBackToNormal — состояние, для которого
// профиль ничего не объявил, наследует покой.
func TestStateFallback_UnstatedStateFallsBackToNormal(t *testing.T) {
	m := theme.NewManager()
	if err := m.RegisterTheme(baseProfile()); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("Base"); err != nil {
		t.Fatal(err)
	}
	th := m.Active()

	normal := th.Style("button", "", theme.StateNormal)
	hover := th.Style("button", "", theme.StateHover)
	pressed := th.Style("button", "", theme.StatePressed)

	if hover.Fill == normal.Fill {
		t.Error("наведение не переопределило заливку")
	}
	if hover.PadX != normal.PadX {
		t.Errorf("наведение потеряло PadX покоя: %v против %v", hover.PadX, normal.PadX)
	}
	// Нажатие профиль не объявлял — берётся покой целиком.
	if pressed.Fill != normal.Fill || pressed.PadX != normal.PadX {
		t.Errorf("нажатие не откатилось к покою: %+v", pressed)
	}
	// Сочетание битов сводится к доминирующему.
	if th.Style("button", "", theme.StateHover|theme.StateFocused) != hover {
		t.Error("hover|focused разрешилось не в hover")
	}
}

// TestPartFallback_UnknownPartFallsBackToComponent — часть, о которой тема
// не знает, получает стиль компонента целиком, а не пустой.
func TestPartFallback_UnknownPartFallsBackToComponent(t *testing.T) {
	m := theme.NewManager()
	p := baseProfile()
	p.SetStyle("menu", "", theme.StateNormal, theme.StyleDelta{Fill: theme.C(theme.RGB(30, 30, 30))})
	p.SetStyle("menu", "item", theme.StateHover, theme.StyleDelta{Fill: theme.C(theme.RGB(0, 90, 158))})
	if err := m.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("Base"); err != nil {
		t.Fatal(err)
	}
	th := m.Active()

	// Часть «item» в покое не объявлена — берёт фон меню.
	if got := th.Style("menu", "item", theme.StateNormal).Fill; got != theme.RGB(30, 30, 30) {
		t.Errorf("menu.item в покое = %v, ждали фон меню", got)
	}
	// Объявленное состояние части — своё.
	if got := th.Style("menu", "item", theme.StateHover).Fill; got != theme.RGB(0, 90, 158) {
		t.Errorf("menu.item при наведении = %v", got)
	}
	// Неизвестная часть тоже откатывается к компоненту.
	if got := th.Style("menu", "separator", theme.StateNormal).Fill; got != theme.RGB(30, 30, 30) {
		t.Errorf("menu.separator = %v, ждали фон меню", got)
	}
	// Неизвестный компонент не роняет отрисовку.
	if s := th.Style("нет-такого", "", theme.StateNormal); s == nil {
		t.Error("неизвестный компонент вернул nil вместо стиля по умолчанию")
	}
}

// TestStyle_NoAllocationsOnHotPath — горячий путь не аллоцирует: стиль
// отдаётся указателем в уже разрешённую таблицу.
func TestStyle_NoAllocationsOnHotPath(t *testing.T) {
	m := theme.NewManager()
	if err := m.RegisterTheme(baseProfile()); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("Base"); err != nil {
		t.Fatal(err)
	}

	allocs := testing.AllocsPerRun(200, func() {
		_ = m.GetStyle("button", "", theme.StateHover)
		_ = m.GetMetric("taskbar.height")
	})
	if allocs != 0 {
		t.Errorf("GetStyle/GetMetric аллоцируют %v раз за вызов, ждали 0", allocs)
	}
}

// TestStyle_SharedPointerIsStable — один и тот же стиль отдаётся тем же
// указателем: это и есть отсутствие аллокаций, и это же причина запрета
// его менять.
func TestStyle_SharedPointerIsStable(t *testing.T) {
	m := theme.NewManager()
	if err := m.RegisterTheme(baseProfile()); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("Base"); err != nil {
		t.Fatal(err)
	}
	a := m.GetStyle("button", "", theme.StateHover)
	b := m.GetStyle("button", "", theme.StateHover)
	if a != b {
		t.Error("один и тот же стиль отдан разными указателями")
	}
	if c := a.Clone(); c == a {
		t.Error("Clone вернул тот же указатель")
	}
}

// ─── Менеджер ───────────────────────────────────────────────────────────────

// TestManager_UnloadRefusesActiveAndParent — выгрузка отказывает там, где
// оставила бы висящую ссылку.
func TestManager_UnloadRefusesActiveAndParent(t *testing.T) {
	m := theme.NewManager()
	if err := m.RegisterTheme(baseProfile()); err != nil {
		t.Fatal(err)
	}
	child := theme.NewProfile("Child")
	child.Parent = "Base"
	if err := m.RegisterTheme(child); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("Child"); err != nil {
		t.Fatal(err)
	}

	if err := m.UnloadTheme("Child"); err == nil {
		t.Error("выгрузили активную тему")
	}
	if err := m.UnloadTheme("Base"); err == nil {
		t.Error("выгрузили родителя зарегистрированной темы")
	}
	if err := m.UnloadTheme("нет-такой"); err == nil {
		t.Error("выгрузили незарегистрированный профиль")
	}

	// Убрали потомка — родитель освободился.
	if err := m.SetTheme("Base"); err != nil {
		t.Fatal(err)
	}
	if err := m.UnloadTheme("Child"); err != nil {
		t.Errorf("не выгрузился неактивный профиль без потомков: %v", err)
	}
}

// TestManager_MissingParentAndCycle — битая цепочка наследования сообщает о
// себе ошибкой, а не паникой и не молчанием.
func TestManager_MissingParentAndCycle(t *testing.T) {
	m := theme.NewManager()
	orphan := theme.NewProfile("Orphan")
	orphan.Parent = "Нет-такого-родителя"
	if err := m.RegisterTheme(orphan); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("Orphan"); err == nil {
		t.Error("тема с несуществующим родителем собралась")
	}

	a := theme.NewProfile("A")
	a.Parent = "B"
	b := theme.NewProfile("B")
	b.Parent = "A"
	if err := m.RegisterTheme(a); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterTheme(b); err != nil {
		t.Fatal(err)
	}
	err := m.SetTheme("A")
	if err == nil {
		t.Fatal("зацикленное наследование собралось")
	}
	if !strings.Contains(err.Error(), "цикл") {
		t.Errorf("ошибка не про цикл: %v", err)
	}
}

// TestManager_SubscribeAndUnsubscribe — подписка получает смену темы,
// отписка перестаёт получать и не течёт.
func TestManager_SubscribeAndUnsubscribe(t *testing.T) {
	m := theme.NewManager()
	if err := m.RegisterTheme(baseProfile()); err != nil {
		t.Fatal(err)
	}
	other := theme.NewProfile("Other")
	other.Parent = "Base"
	if err := m.RegisterTheme(other); err != nil {
		t.Fatal(err)
	}

	got := 0
	var lastName string
	unsub := m.Subscribe(theme.ObserverFunc(func(th *theme.Theme) {
		got++
		lastName = th.Name()
	}))
	if m.ObserverCount() != 1 {
		t.Fatalf("подписчиков %d, ждали 1", m.ObserverCount())
	}

	if err := m.SetTheme("Base"); err != nil {
		t.Fatal(err)
	}
	if got != 1 || lastName != "Base" {
		t.Errorf("уведомлений %d, последняя тема %q", got, lastName)
	}

	unsub()
	if m.ObserverCount() != 0 {
		t.Errorf("после отписки осталось %d подписчиков", m.ObserverCount())
	}
	if err := m.SetTheme("Other"); err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Errorf("отписавшийся получил уведомление (%d)", got)
	}
}

// TestManager_NoActiveThemeIsSurvivable — до установки темы менеджер
// отвечает значениями по умолчанию, а не паникой: отрисовка не должна
// зависеть от порядка запуска.
func TestManager_NoActiveThemeIsSurvivable(t *testing.T) {
	m := theme.NewManager()
	if s := m.GetStyle("button", "", theme.StateHover); s == nil {
		t.Error("GetStyle без активной темы вернул nil")
	}
	if v := m.GetMetric("taskbar.height"); v != 0 {
		t.Errorf("GetMetric без темы = %v, ждали 0", v)
	}
	if a := m.GetAnimation("window.open"); !a.IsZero() {
		t.Errorf("GetAnimation без темы = %+v, ждали пустую", a)
	}
	if img := m.GetIcon("start", 16); img != nil {
		t.Error("GetIcon без темы вернул изображение")
	}
}

// TestManager_ThirdPartyThemeWorks — тема, объявленная ВНЕ пакетов движка,
// подключается и работает без правок ядра (критерий приёмки №5).
func TestManager_ThirdPartyThemeWorks(t *testing.T) {
	m := theme.NewManager()
	if err := m.RegisterTheme(baseProfile()); err != nil {
		t.Fatal(err)
	}

	// Профиль «стороннего разработчика»: свой компонент, о котором движок
	// ничего не знает, и переопределение известного.
	custom := theme.NewProfile("Кислотная")
	custom.Parent = "Base"
	custom.SetColor("accent", theme.RGB(255, 0, 128)).
		SetMetric("taskbar.height", 64)
	custom.SetStyle("мой-виджет", "", theme.StateNormal, theme.StyleDelta{
		Fill: theme.C(theme.RGB(10, 200, 90)),
	})
	if err := m.RegisterTheme(custom); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("Кислотная"); err != nil {
		t.Fatal(err)
	}

	if got := m.GetStyle("мой-виджет", "", theme.StateNormal).Fill; got != theme.RGB(10, 200, 90) {
		t.Errorf("стиль стороннего компонента = %v", got)
	}
	if got := m.GetMetric("taskbar.height"); got != 64 {
		t.Errorf("метрика сторонней темы = %v, ждали 64", got)
	}
	// Унаследованное от базовой темы на месте.
	if got := m.GetStyle("button", "", theme.StateNormal).PadX; got != 12 {
		t.Errorf("унаследованный отступ кнопки = %v", got)
	}
}

// TestManager_ReregisterParentRebuildsActive — правка родителя доходит до
// активной темы-потомка без переустановки.
func TestManager_ReregisterParentRebuildsActive(t *testing.T) {
	m := theme.NewManager()
	if err := m.RegisterTheme(baseProfile()); err != nil {
		t.Fatal(err)
	}
	child := theme.NewProfile("Child")
	child.Parent = "Base"
	if err := m.RegisterTheme(child); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("Child"); err != nil {
		t.Fatal(err)
	}
	if got := m.GetMetric("taskbar.height"); got != 48 {
		t.Fatalf("исходная метрика = %v", got)
	}

	updated := baseProfile()
	updated.SetMetric("taskbar.height", 40)
	if err := m.RegisterTheme(updated); err != nil {
		t.Fatal(err)
	}
	if got := m.GetMetric("taskbar.height"); got != 40 {
		t.Errorf("после правки родителя метрика = %v, ждали 40", got)
	}
}

// ─── JSON ───────────────────────────────────────────────────────────────────

const sampleJSON = `{
  "name": "Из файла",
  "parent": "Base",
  "colors": { "accent": "#0078D7", "glass": "#FFFFFF40" },
  "metrics": { "taskbar.height": 40 },
  "flags": { "taskbar.centered": true },
  "fonts": { "default": { "family": "Segoe UI", "size": 9 } },
  "icons": { "start": { "source": "icons/start.svg" } },
  "anims": { "window.open": { "duration_ms": 150, "curve": "out-cubic" } },
  "styles": {
    "button": { "fill": "#333337", "corner": 4, "pad_x": 12 },
    "button:hover": { "fill": "#3E3E42" },
    "menu.item:hover": { "fill": "#005A9E", "text": "#FFFFFF" },
    "taskbar": {
      "backdrop": { "mode": "blur", "radius": 30, "tint": "#20202080" },
      "elevation": 4
    },
    "panel": { "bevel": { "light": "#FFFFFF", "shadow": "#808080", "dark": "#000000", "width": 2 } }
  },
  "presenters": { "runningapps": "dock" }
}`

func TestLoadTheme_ReadsEverything(t *testing.T) {
	res, err := theme.LoadTheme(strings.NewReader(sampleJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("неожиданные предупреждения: %v", res.Warnings)
	}
	p := res.Profile
	if p.Name != "Из файла" || p.Parent != "Base" {
		t.Errorf("имя/родитель = %q/%q", p.Name, p.Parent)
	}
	if c := p.Colors["accent"]; c != theme.RGB(0, 120, 215) {
		t.Errorf("accent = %v", c)
	}
	// Полупрозрачный цвет пришёл премультиплицированным.
	glass := p.Colors["glass"]
	if glass.A != 0x40 || glass.R != 0x40 {
		t.Errorf("glass = %v — альфа не премультиплицирована", glass)
	}
	if p.Metrics["taskbar.height"] != 40 || !p.Flags["taskbar.centered"] {
		t.Error("метрики или флаги не разобраны")
	}
	if f := p.Fonts["default"]; f.Family != "Segoe UI" || f.Size != 9 {
		t.Errorf("шрифт = %+v", f)
	}
	if ic := p.Icons["start"]; ic.Source != "icons/start.svg" {
		t.Errorf("иконка = %+v", ic)
	}
	if a := p.Anims["window.open"]; a.Duration != 150*time.Millisecond || a.Curve != "out-cubic" {
		t.Errorf("анимация = %+v", a)
	}
	if p.Presenters["runningapps"] != "dock" {
		t.Errorf("презентер = %q", p.Presenters["runningapps"])
	}

	// Ключи стилей разобраны в тройки.
	if d, ok := p.Styles[theme.StyleKey{Component: "button", State: theme.StateHover}]; !ok {
		t.Error("button:hover не разобран")
	} else if d.Fill == nil || *d.Fill != theme.RGB(0x3E, 0x3E, 0x42) {
		t.Errorf("button:hover fill = %v", d.Fill)
	}
	if _, ok := p.Styles[theme.StyleKey{Component: "menu", Part: "item", State: theme.StateHover}]; !ok {
		t.Error("menu.item:hover не разобран")
	}
	tb := p.Styles[theme.StyleKey{Component: "taskbar"}]
	if tb.Backdrop == nil || tb.Backdrop.Mode != theme.BackdropBlur || tb.Backdrop.Radius != 30 {
		t.Errorf("подложка панели = %+v", tb.Backdrop)
	}
	if tb.Elevation == nil || *tb.Elevation != 4 {
		t.Error("elevation панели не разобран")
	}
	if pn := p.Styles[theme.StyleKey{Component: "panel"}]; pn.Bevel == nil || pn.Bevel.Width != 2 {
		t.Errorf("фаска панели = %+v", pn.Bevel)
	}
}

// TestLoadTheme_SurvivesBadValues — профиль с опечаткой в одной строке
// грузится целиком, а непонятое перечислено в предупреждениях.
func TestLoadTheme_SurvivesBadValues(t *testing.T) {
	const broken = `{
	  "name": "С опечатками",
	  "colors": { "good": "#112233", "bad": "не-цвет" },
	  "styles": {
	    "button": { "fill": "#334455" },
	    "button:hovered": { "fill": "#000000" },
	    "list.item": { "fill": "розовый" }
	  }
	}`
	res, err := theme.LoadTheme(strings.NewReader(broken))
	if err != nil {
		t.Fatalf("загрузка отказала целиком: %v", err)
	}
	if len(res.Warnings) != 3 {
		t.Errorf("предупреждений %d, ждали 3: %v", len(res.Warnings), res.Warnings)
	}
	if _, ok := res.Profile.Colors["good"]; !ok {
		t.Error("исправный цвет потерян из-за соседней опечатки")
	}
	if _, ok := res.Profile.Styles[theme.StyleKey{Component: "button"}]; !ok {
		t.Error("исправный стиль потерян")
	}
}

func TestLoadTheme_RejectsUnusable(t *testing.T) {
	if _, err := theme.LoadTheme(strings.NewReader("{это не json")); err == nil {
		t.Error("сломанный JSON принят")
	}
	if _, err := theme.LoadTheme(strings.NewReader(`{"colors":{}}`)); err == nil {
		t.Error("профиль без имени принят")
	}
}

func TestParseColor_Roundtrip(t *testing.T) {
	cases := []struct {
		in   string
		want color.RGBA
	}{
		{"#FFF", theme.RGB(255, 255, 255)},
		{"#0078D7", theme.RGB(0, 120, 215)},
		{"#00000000", color.RGBA{}},
		{"#FFFFFF80", theme.RGBA(255, 255, 255, 0x80)},
	}
	for _, c := range cases {
		got, err := theme.ParseColor(c.in)
		if err != nil {
			t.Errorf("ParseColor(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseColor(%q) = %v, ждали %v", c.in, got, c.want)
		}
		// Обратная запись читается в тот же цвет.
		back, err := theme.ParseColor(theme.FormatColor(got))
		if err != nil || back != got {
			t.Errorf("%q: обратный разбор дал %v (%v)", c.in, back, err)
		}
	}
	if _, err := theme.ParseColor("#12345"); err == nil {
		t.Error("цвет неверной длины принят")
	}
}

func TestStyleKey_ParseAndString(t *testing.T) {
	cases := []struct {
		raw  string
		want theme.StyleKey
	}{
		{"button", theme.StyleKey{Component: "button"}},
		{"menu.item", theme.StyleKey{Component: "menu", Part: "item"}},
		{"button:hover", theme.StyleKey{Component: "button", State: theme.StateHover}},
		{"menu.item:disabled", theme.StyleKey{Component: "menu", Part: "item", State: theme.StateDisabled}},
	}
	for _, c := range cases {
		got, err := theme.ParseStyleKey(c.raw)
		if err != nil {
			t.Errorf("ParseStyleKey(%q): %v", c.raw, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseStyleKey(%q) = %+v, ждали %+v", c.raw, got, c.want)
		}
		if s := got.String(); s != c.raw {
			t.Errorf("StyleKey(%+v).String() = %q, ждали %q", got, s, c.raw)
		}
	}
	if _, err := theme.ParseStyleKey(":hover"); err == nil {
		t.Error("ключ без компонента принят")
	}
}
