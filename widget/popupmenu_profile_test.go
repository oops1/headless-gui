package widget

import (
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
)

// Меню читает вид из профиля темы само: метрики theme.KeyMenu*, стили
// «menu» / «menu.item» / «menu.separator» / «menu.shortcut». Тесты здесь —
// про данные и раскладку; про пиксели — tests/popupmenu_profile_test.go.

// menuProfile — профиль, объявляющий вид меню, и разрешённая тема.
func menuProfile(t *testing.T, build func(p *theme.Profile)) *theme.Theme {
	t.Helper()
	p := theme.NewProfile("menu-test")
	p.SetColor("surface", theme.RGB(240, 240, 240)).SetColor("text", theme.RGB(10, 10, 10))
	build(p)
	m := theme.NewManager()
	if err := m.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme("menu-test"); err != nil {
		t.Fatal(err)
	}
	return m.Active()
}

func TestMenuStyle_ReadsEveryDeclaredToken(t *testing.T) {
	rt := menuProfile(t, func(p *theme.Profile) {
		p.SetMetric(theme.KeyMenuItemHeight, 24).
			SetMetric(theme.KeyMenuSeparatorHeight, 8).
			SetMetric(theme.KeyMenuPadX, 12).
			SetMetric(theme.KeyMenuPadLeft, 10).
			SetMetric(theme.KeyMenuPadRight, 14).
			SetMetric(theme.KeyMenuPadY, 4).
			SetMetric(theme.KeyMenuIconSize, 16).
			SetMetric(theme.KeyMenuIconGap, 9).
			SetMetric(theme.KeyMenuWidthMin, 290).
			SetMetric(theme.KeyMenuItemInset, 4).
			SetMetric(theme.KeyMenuSeparatorInset, 6).
			SetMetric(theme.KeyMenuChevronRight, 18).
			SetMetric(theme.KeyMenuSubmenuDelay, 400).
			SetFlag(theme.FlagMenuIconTint, true)
		p.SetStyle("menu", "", theme.StateNormal, theme.StyleDelta{
			Fill: theme.C(theme.RGB(249, 249, 249)), Corner: theme.N(8),
			Elevation: theme.N(8), Shadow: theme.C(theme.RGBA(0, 0, 0, 70)),
		})
		p.SetStyle("menu", "item", theme.StateHover, theme.StyleDelta{
			Fill: theme.C(theme.RGBA(0, 0, 0, 9)), Corner: theme.N(4),
		})
		p.SetStyle("menu", "separator", theme.StateNormal, theme.StyleDelta{Fill: theme.C(theme.RGB(229, 229, 229))})
		p.SetStyle("menu", "shortcut", theme.StateNormal, theme.StyleDelta{Text: theme.C(theme.RGB(10, 10, 10))})
	})
	ms := MenuStyleFromTheme(rt)
	want := MenuStyle{
		Background: theme.RGB(249, 249, 249), Text: theme.RGB(10, 10, 10), Border: ms.Border,
		Shadow: theme.RGBA(0, 0, 0, 70), HoverBG: theme.RGBA(0, 0, 0, 9), HoverText: theme.RGB(10, 10, 10),
		Shortcut: theme.RGB(10, 10, 10), Separator: theme.RGB(229, 229, 229),
		Corner: 8, ItemCorner: 4, Elevation: 8,
		ItemHeight: 24, SeparatorHeight: 8, PadX: 12, PadLeft: 10, PadRight: 14, PadY: 4,
		IconSize: 16, IconGap: 9, WidthMin: 290, ItemInset: 4, SeparatorInset: 6,
		ChevronRight: 18, ChevronSize: 0, SubMenuDelay: 400, TintIcons: true,
	}
	if ms != want {
		t.Errorf("MenuStyleFromTheme:\n получили %+v\n ждали    %+v", ms, want)
	}

	m := NewPopupMenu()
	m.ApplyMenuStyle(ms)
	if m.ItemHeight != 24 || m.SeparatorH != 8 || m.PaddingX != 12 || m.PadLeft != 10 ||
		m.PadRight != 14 || m.PaddingY != 4 || m.MinWidth != 290 || m.ItemInset != 4 ||
		m.CornerRadius != 8 || m.ItemCorner != 4 || m.ChevronRight != 18 || m.SubMenuDelay != 400 {
		t.Errorf("поля меню не взяли профиль: %+v", m)
	}
	// Подменю освобождено от минимума: объявлен только минимум корня.
	if m.SubMenuMinWidth != -1 {
		t.Errorf("SubMenuMinWidth = %d, ждали -1", m.SubMenuMinWidth)
	}
}

// Профиль без токенов меню оставляет его прежним: то же самое, что ждали до
// появления токенов (30/9 для обычных, 22/7 для классики).
func TestMenuStyle_UndeclaredKeepsDefaults(t *testing.T) {
	rt := menuProfile(t, func(p *theme.Profile) {})
	ms := MenuStyleFromTheme(rt)
	m := NewPopupMenu()
	m.ApplyMenuStyle(ms)
	ref := NewPopupMenu()
	if m.ItemHeight != ref.ItemHeight || m.SeparatorH != ref.SeparatorH ||
		m.PaddingX != ref.PaddingX || m.MinWidth != ref.MinWidth ||
		m.ItemInset != 0 || m.CornerRadius != 0 || m.ChevronRight != 0 || m.SubMenuDelay != 0 {
		t.Errorf("профиль без токенов изменил меню: %+v", m)
	}
}

// Пункт 10: ApplyTheme при смене темы сбрасывал ItemHeight к 30/22 — теперь
// меню помнит то, что объявил профиль.
func TestPopupMenu_ApplyThemeKeepsProfileHeight(t *testing.T) {
	th := Win10DarkTheme()
	th.Style.Menu = MenuStyle{ItemHeight: 24, SeparatorHeight: 8, PadX: 12}
	m := NewPopupMenu()
	m.ApplyTheme(th)
	if m.ItemHeight != 24 || m.SeparatorH != 8 || m.PaddingX != 12 {
		t.Errorf("ApplyTheme сбросил профиль: высота %d, разделитель %d, поле %d",
			m.ItemHeight, m.SeparatorH, m.PaddingX)
	}
	// Тема без объявлений возвращает прежнее — и поле, которое ставил профиль.
	m.ApplyTheme(Win10DarkTheme())
	if m.ItemHeight != 30 || m.SeparatorH != 9 || m.PaddingX != 16 {
		t.Errorf("прежняя тема не вернула умолчание: %d/%d/%d", m.ItemHeight, m.SeparatorH, m.PaddingX)
	}
}

// Меню, созданные на лету (контекстные меню полей), следуют профилю, который
// движок применил как тему: Materialize несёт его в ThemeStyle.Menu.
func TestPopupMenu_NewFollowsMaterializedProfile(t *testing.T) {
	prev := win10
	defer ApplyGlobalTheme(&prev)
	prevStyle := win10.Style
	defer func() { win10.Style = prevStyle }()

	rt := menuProfile(t, func(p *theme.Profile) {
		p.SetMetric(theme.KeyMenuItemHeight, 26).SetMetric(theme.KeyMenuItemInset, 3)
	})
	ApplyGlobalTheme(Materialize(rt))
	m := NewPopupMenu()
	if m.ItemHeight != 26 || m.ItemInset != 3 {
		t.Errorf("новое меню не прочитало профиль: высота %d, отступ %d", m.ItemHeight, m.ItemInset)
	}
}

// Плоская тема (пресет) через мост «туда и обратно» не объявляет ничего для
// меню: скругление контролов не превращается в скругление меню.
func TestMenuStyle_FlatPresetDeclaresNothing(t *testing.T) {
	for _, name := range ThemeNames() {
		src := ThemeByName(name)
		rt := themeResolved(t, ProfileFromTheme(src))
		if ms := MenuStyleFromTheme(rt); ms.Corner != 0 || ms.ItemCorner != 0 || ms.ItemHeight != 0 {
			t.Errorf("%s: меню получило из пресета %+v", name, ms)
		}
		if got := Materialize(rt).Style.Menu; !got.isZero() {
			t.Errorf("%s: ThemeStyle.Menu не пуст: %+v", name, got)
		}
	}
}

func themeResolved(t *testing.T, p *theme.Profile) *theme.Theme {
	t.Helper()
	m := theme.NewManager()
	if err := m.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(p.Name); err != nil {
		t.Fatal(err)
	}
	return m.Active()
}

// Объявленное в ThemeStyle.Menu переживает путь «плоская тема → профиль →
// плоская тема».
func TestMenuStyle_RoundTripThroughProfile(t *testing.T) {
	src := Win10DarkTheme()
	src.Style.Menu = MenuStyle{
		ItemHeight: 24, SeparatorHeight: 8, PadX: 12, ItemInset: 4, Corner: 8, ItemCorner: 4,
		Elevation: 8, Text: color.RGBA{R: 1, G: 2, B: 3, A: 255}, ChevronRight: 18,
		Shortcut: color.RGBA{R: 9, G: 9, B: 9, A: 255}, TintIcons: true,
	}
	got := Materialize(themeResolved(t, ProfileFromTheme(src))).Style.Menu
	if got != src.Style.Menu {
		t.Errorf("вид меню потерян в мосте:\n было  %+v\n стало %+v", src.Style.Menu, got)
	}
}

// Профили движка: значения по эталону Windows 11 и Windows 10.
func TestBuiltinProfiles_DeclareMenuLook(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	get := func(name string, light bool) MenuStyle {
		t.Helper()
		if err := m.SetTheme(name); err != nil {
			t.Fatal(err)
		}
		m.SetFlag(theme.KeyTaskbarLight, light)
		return MenuStyleFromTheme(m.Active())
	}

	w11 := get(theme.ProfileWindows11, false)
	if w11.Corner != 8 || w11.ItemCorner != 4 || w11.ItemInset != 4 || w11.ItemHeight != 24 ||
		w11.PadX != 12 || w11.IconSize != 16 || w11.ChevronRight != 18 || w11.Shortcut.A == 0 {
		t.Errorf("Windows 11: %+v", w11)
	}
	if w11.HoverBG.A == 0 || w11.HoverBG.A == 255 {
		t.Errorf("плашка Windows 11 должна быть полупрозрачной: %v", w11.HoverBG)
	}
	// Тёмная разновидность наследует размеры, меняя цвета.
	w11d := get(theme.ProfileWindows11Dark, false)
	if w11d.ItemHeight != 24 || w11d.Corner != 8 || w11d.Background == w11.Background ||
		w11d.HoverBG == w11.HoverBG {
		t.Errorf("Windows 11 Dark: %+v", w11d)
	}

	w10 := get(theme.ProfileWindows10, false)
	if w10.Corner != 0 || w10.ItemHeight != 24 || w10.ItemInset != 1 || w10.ChevronRight != 0 ||
		w10.Elevation == 0 {
		t.Errorf("Windows 10: %+v", w10)
	}
	// Светлый режим оболочки красит разделитель по-своему.
	if get(theme.ProfileWindows10, true).Separator == w10.Separator {
		t.Error("светлый Windows 10: разделитель не сменился")
	}

	w2k := get(theme.ProfileWindows2000, false)
	if w2k.ItemHeight != 20 || w2k.SubMenuDelay != 400 || w2k.Corner != 0 {
		t.Errorf("Windows 2000: %+v", w2k)
	}
}

// Раздельные поля: левое — до значка, правое — до сочетания; от них зависит
// ширина меню.
func TestPopupMenu_SeparatePads(t *testing.T) {
	base := NewPopupMenu()
	base.SetItems([]MenuItem{{Text: "Копировать", Shortcut: "Ctrl+C"}})
	base.Show(0, 0)
	w0 := base.Bounds().Dx()
	base.Close()

	m := NewPopupMenu()
	m.SetItems([]MenuItem{{Text: "Копировать", Shortcut: "Ctrl+C"}})
	m.PadLeft, m.PadRight = 40, 40
	m.Show(0, 0)
	defer m.Close()
	// Поля по 16 → по 40: меню шире на 48.
	if got := m.Bounds().Dx(); got != w0+48 {
		t.Errorf("ширина с полями 40/40 — %d, ждали %d", got, w0+48)
	}
}

// Пункт 8: минимальная ширина нужна корневому меню, подменю — по содержимому.
func TestPopupMenu_SubMenuMinWidth(t *testing.T) {
	build := func(subMin int) (root, child int) {
		m := NewPopupMenu()
		m.MinWidth = 300
		m.SubMenuMinWidth = subMin
		m.SetItems([]MenuItem{{Text: "Создать", SubItems: []MenuItem{{Text: "Папку"}}}})
		m.Show(0, 0)
		defer m.Close()
		m.openChild(0)
		c, _ := m.openChildOf()
		return m.popupRect().Dx(), c.popupRect().Dx()
	}
	if r, c := build(0); r != 320 || c != 300 {
		t.Errorf("прежнее поведение: корень %d, подменю %d, ждали 320 (300 и стрелка) и 300", r, c)
	}
	if r, c := build(-1); r != 320 || c >= 300 {
		t.Errorf("подменю без минимума: корень %d, подменю %d", r, c)
	}
	if _, c := build(120); c != 120 {
		t.Errorf("свой минимум подменю: %d, ждали 120", c)
	}
}

// Задержка раскрытия подменю: наведение раскрывает его не сразу, уход на
// другой пункт закрывает не сразу; щелчок и клавиши задержку не ждут.
func TestPopupMenu_SubMenuDelay(t *testing.T) {
	newMenu := func() *PopupMenu {
		m := NewPopupMenu()
		m.SubMenuDelay = 200
		m.SetItems([]MenuItem{
			{Text: "Создать", SubItems: []MenuItem{{Text: "Папку"}}},
			{Text: "Свойства"},
		})
		m.Show(10, 10)
		return m
	}
	rowY := func(m *PopupMenu, i int) int { return m.layout().itemsTop + i*m.ItemHeight + 5 }

	m := newMenu()
	defer m.Close()
	t0 := time.Now()
	m.OnMouseMove(30, rowY(m, 0))
	if c, _ := m.openChildOf(); c != nil {
		t.Fatal("подменю раскрылось мгновенно, хотя задана задержка")
	}
	StepAnimations(t0)
	StepAnimations(t0.Add(100 * time.Millisecond))
	if c, _ := m.openChildOf(); c != nil {
		t.Fatal("подменю раскрылось раньше срока")
	}
	StepAnimations(t0.Add(300 * time.Millisecond))
	if c, _ := m.openChildOf(); c == nil {
		t.Fatal("подменю не раскрылось по истечении задержки")
	}

	// Курсор ушёл на «Свойства»: плашка перешла сразу, подменю ещё живо.
	t1 := time.Now()
	m.OnMouseMove(30, rowY(m, 1))
	if c, _ := m.openChildOf(); c == nil {
		t.Fatal("подменю закрылось мгновенно, хотя задана задержка")
	}
	StepAnimations(t1)
	StepAnimations(t1.Add(300 * time.Millisecond))
	if c, _ := m.openChildOf(); c != nil {
		t.Error("подменю не закрылось после задержки")
	}

	// Вернулись и не дождались: переключение отменено.
	m.OnMouseMove(30, rowY(m, 0))
	m.OnMouseMove(30, rowY(m, 1))
	m.OnMouseMove(30, rowY(m, 0))
	t2 := time.Now()
	StepAnimations(t2)
	StepAnimations(t2.Add(500 * time.Millisecond))
	if c, i := m.openChildOf(); c == nil || i != 0 {
		t.Error("после возврата на «Создать» подменю должно раскрыться")
	}

	// Клавиша «вправо» и щелчок задержки не ждут.
	k := newMenu()
	defer k.Close()
	k.setHoverIdx(0)
	k.OnKeyEvent(KeyEvent{Code: KeyRight, Pressed: true})
	if c, _ := k.openChildOf(); c == nil {
		t.Error("клавиша «вправо» ждёт задержку")
	}
}

// Без профиля подменю раскрывается сразу, как раньше.
func TestPopupMenu_SubMenuImmediateByDefault(t *testing.T) {
	m := NewPopupMenu()
	m.SetItems([]MenuItem{{Text: "Создать", SubItems: []MenuItem{{Text: "Папку"}}}})
	m.Show(10, 10)
	defer m.Close()
	m.OnMouseMove(30, m.layout().itemsTop+5)
	if c, _ := m.openChildOf(); c == nil {
		t.Error("подменю без задержки не раскрылось сразу")
	}
}

func manyItems(n int) []MenuItem {
	items := make([]MenuItem, n)
	for i := range items {
		items[i] = MenuItem{Text: fmt.Sprintf("Пункт %d", i)}
	}
	return items
}

// Длинный список не вылезает за экран: меню получает высоту области и
// прокручивается колесом, стрелками на концах и клавишами.
func TestPopupMenu_LongListScrolls(t *testing.T) {
	SetScreenBounds(400, 300)
	defer SetScreenBounds(0, 0)

	m := NewPopupMenu()
	m.SetItems(manyItems(40)) // 40×30 = 1200 точек при экране в 300
	m.Show(50, 100)
	defer m.Close()

	r := m.popupRect()
	if r.Min.Y < 0 || r.Max.Y > 300 || r.Dy() != 300 {
		t.Fatalf("меню %v не вписано в экран 300 по высоте", r)
	}
	l := m.layout()
	if !l.scrollable || l.maxScroll <= 0 {
		t.Fatalf("прокрутки нет: %+v", l)
	}

	// Колесо вниз листает; вверх — возвращает.
	mid := image.Pt(r.Min.X+20, r.Min.Y+100)
	m.OnMouseButton(MouseEvent{X: mid.X, Y: mid.Y, Button: MouseWheelDown, Pressed: true})
	if m.layout().scroll == 0 {
		t.Error("колесо вниз не прокрутило меню")
	}
	m.OnMouseButton(MouseEvent{X: mid.X, Y: mid.Y, Button: MouseWheelUp, Pressed: true})
	if m.layout().scroll != 0 {
		t.Error("колесо вверх не вернуло меню на место")
	}

	// Клавиатура: последний пункт достижим, и меню показывает его целиком.
	m.setHoverIdx(-1)
	for i := 0; i < 40; i++ {
		m.OnKeyEvent(KeyEvent{Code: KeyDown, Pressed: true})
	}
	last := m.layout()
	if int(m.hoverIdx) != 0 && last.scroll == 0 {
		t.Error("после прохода вниз клавишей меню не прокручено")
	}
	m.OnKeyEvent(KeyEvent{Code: KeyUp, Pressed: true})
	m.OnKeyEvent(KeyEvent{Code: KeyUp, Pressed: true})
	// Пункт под курсором клавиатуры лежит в окне просмотра.
	hi := int(m.hoverIdx)
	top := m.layout().itemsTop + hi*m.ItemHeight
	if l2 := m.layout(); top < l2.viewTop || top+m.ItemHeight > l2.viewBottom {
		t.Errorf("пункт %d вне окна просмотра: %d..%d из %d..%d", hi, top, top+m.ItemHeight, l2.viewTop, l2.viewBottom)
	}

	// Щелчок по стрелке в нижней полосе листает вниз.
	m.scrollBy(-100000)
	before := m.layout().scroll
	m.OnMouseButton(MouseEvent{X: r.Min.X + 20, Y: r.Max.Y - 4, Button: MouseLeft})
	if m.layout().scroll <= before {
		t.Error("щелчок по нижней стрелке не прокрутил меню")
	}
	// В полосе нет пунктов: наведение не подсвечивает ничего.
	if idx := m.itemAtY(r.Max.Y - 4); idx != -1 {
		t.Errorf("в полосе стрелки нашёлся пункт %d", idx)
	}
}

// Курсор на стрелке листает меню сам, пока стоит на ней.
func TestPopupMenu_ArrowHoverAutoScroll(t *testing.T) {
	SetScreenBounds(400, 300)
	defer SetScreenBounds(0, 0)
	m := NewPopupMenu()
	m.SetItems(manyItems(40))
	m.Show(50, 0)
	defer m.Close()
	r := m.popupRect()
	m.OnMouseMove(r.Min.X+20, r.Max.Y-4)
	s0 := m.layout().scroll
	t0 := time.Now()
	for i := 0; i < 5; i++ {
		StepAnimations(t0.Add(time.Duration(i) * 60 * time.Millisecond))
	}
	if m.layout().scroll <= s0 {
		t.Error("курсор на стрелке не прокручивает меню")
	}
	m.OnMouseMove(r.Min.X+20, r.Min.Y+100) // ушёл со стрелки
	s1 := m.layout().scroll
	StepAnimations(t0.Add(2 * time.Second))
	if m.layout().scroll != s1 {
		t.Error("прокрутка продолжается после ухода со стрелки")
	}
}

// Рабочая область: меню и подменю не ложатся на панель задач.
func TestPopupMenu_WorkAreaKeepsClearOfTaskbar(t *testing.T) {
	SetScreenBounds(400, 300)
	SetPopupWorkArea(image.Rect(0, 0, 400, 260)) // внизу панель в 40 точек
	defer func() { SetScreenBounds(0, 0); SetPopupWorkArea(image.Rectangle{}) }()

	m := NewPopupMenu()
	m.SetItems([]MenuItem{
		{Text: "Один"}, {Text: "Два"},
		{Text: "Создать", SubItems: []MenuItem{{Text: "а"}, {Text: "б"}, {Text: "в"}, {Text: "г"}}},
	})
	m.Show(20, 250) // у самого низа
	defer m.Close()
	if r := m.popupRect(); r.Max.Y > 260 {
		t.Errorf("меню легло на панель: %v", r)
	}
	m.openChild(2)
	c, _ := m.openChildOf()
	if r := c.popupRect(); r.Max.Y > 260 {
		t.Errorf("подменю легло на панель: %v", r)
	}
	// Пустая область — снова весь холст.
	SetPopupWorkArea(image.Rectangle{})
	m2 := NewPopupMenu()
	m2.SetItems([]MenuItem{{Text: "Один"}})
	m2.Show(20, 285)
	defer m2.Close()
	if r := m2.popupRect(); r.Max.Y > 300 || r.Max.Y <= 260 {
		t.Errorf("без рабочей области меню должно прижаться к краю холста: %v", r)
	}
}

// Одноцветный значок берёт цвет текста по альфа-каналу картинки.
func TestMenuTintImage_UsesInkColour(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	src.SetRGBA(0, 0, color.RGBA{A: 255})             // чёрная непрозрачная
	src.SetRGBA(1, 0, color.RGBA{A: 128})             // наполовину прозрачная
	ink := color.RGBA{R: 255, G: 255, B: 255, A: 255} // белые чернила
	out := menuTintImage(src, ink).(*image.RGBA)
	if got := out.RGBAAt(0, 0); got != ink {
		t.Errorf("непрозрачная точка: %v, ждали %v", got, ink)
	}
	if got := out.RGBAAt(1, 0); got.A < 126 || got.A > 130 || got.R != got.A {
		t.Errorf("полупрозрачная точка предумножена неверно: %v", got)
	}

	// Значок под плашкой: свой рисунок и цвет по состоянию.
	m := NewPopupMenu()
	hov := image.NewRGBA(image.Rect(0, 0, 1, 1))
	it := MenuItem{Icon: src, IconHover: hov}
	if m.iconFor(it, false, ink) != image.Image(src) || m.iconFor(it, true, ink) != image.Image(hov) {
		t.Error("IconHover должен подменять Icon только под плашкой")
	}
	m.TintIcons = true
	if _, same := m.iconFor(MenuItem{Icon: src}, false, ink).(*image.RGBA); !same ||
		m.iconFor(MenuItem{Icon: src}, false, ink) == image.Image(src) {
		t.Error("TintIcons должен перекрашивать значок")
	}
}

// Сочетание: цвет профиля применяется к обычному пункту, недоступный пункт
// следует своему приглушённому тексту.
func TestPopupMenu_ShortcutColourFromProfile(t *testing.T) {
	m := NewPopupMenu()
	text := color.RGBA{A: 255}
	if got := m.shortcutColorFor(text, false, false); got == text {
		t.Error("без цвета профиля сочетание должно быть приглушено")
	}
	m.ShortcutColor = color.RGBA{R: 1, A: 255}
	if got := m.shortcutColorFor(text, false, false); got != m.ShortcutColor {
		t.Errorf("цвет профиля не применён: %v", got)
	}
	if got := m.shortcutColorFor(text, false, true); got == m.ShortcutColor {
		t.Error("у недоступного пункта сочетание должно следовать приглушённому тексту")
	}
	// Под плашкой, меняющей цвет текста (классика), — цвет под плашкой.
	m.TextColor, m.HoverTextColor = color.RGBA{A: 255}, color.RGBA{R: 255, G: 255, B: 255, A: 255}
	if got := m.shortcutColorFor(m.HoverTextColor, true, false); got == m.ShortcutColor {
		t.Error("под тёмной плашкой сочетание не должно остаться чёрным")
	}
}
