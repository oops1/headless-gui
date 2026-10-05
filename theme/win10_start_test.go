package theme

import "testing"

// Меню «Пуск» и строка поиска Windows 10: презентер, метрики и стили.

func win10Manager(t *testing.T, name string) *Manager {
	t.Helper()
	m := NewManager()
	if err := RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(name); err != nil {
		t.Fatal(err)
	}
	return m
}

// Меню с плитками просит только Windows 10 (и её тёмная разновидность по
// наследству): остальные темы остаются на плоском списке.
func TestWin10Start_PresenterOnlyForWindows10(t *testing.T) {
	for name, want := range map[string]string{
		ProfileWindows10: PresenterStartTiles, ProfileWindows10Dark: PresenterStartTiles,
		ProfileWindows11: "", ProfileWindows11Dark: "",
		ProfileWindows2000: "", ProfileWindows2000Blue: "",
		ProfileMacOS: "", ProfileMacOSDark: "",
	} {
		if got := win10Manager(t, name).Active().PresenterName("startmenu"); got != want {
			t.Errorf("%s: презентер меню %q, ждали %q", name, got, want)
		}
	}
}

// Стартовые значения метрик — из задания заказчика.
func TestWin10Start_StartingMetrics(t *testing.T) {
	m := win10Manager(t, ProfileWindows10)
	want := map[Key]float64{
		"startmenu.sidebar.collapsed": 48, "startmenu.sidebar.expanded": 256,
		"startmenu.list.width": 260, "startmenu.row.height": 36,
		"startmenu.letter.height": 36, "startmenu.icon.size": 24, "startmenu.corner": 0,
		"tile.unit": 48, "tile.gap": 4, "tile.group.header": 32,
		"search.width": 344, "search.icon.width": 48,
	}
	for k, v := range want {
		if got := m.GetMetric(k); got != v {
			t.Errorf("%s = %v, ждали %v", k, got, v)
		}
	}
	// Средняя плитка 2×2 единицы с зазором — 100, широкая и большая — 204.
	unit, gap := m.GetMetric("tile.unit"), m.GetMetric("tile.gap")
	if 2*unit+gap != 100 || 4*unit+3*gap != 204 {
		t.Errorf("сетка даёт %v и %v, ждали 100 и 204", 2*unit+gap, 4*unit+3*gap)
	}
	// Ни одной из них нет у других тем: там плоское меню со своими метриками.
	for _, name := range []string{ProfileWindows11, ProfileWindows2000, ProfileMacOS} {
		if got := win10Manager(t, name).GetMetric("startmenu.sidebar.collapsed"); got != 0 {
			t.Errorf("%s получила метрику боковой панели %v", name, got)
		}
	}
}

// Части стилей меню и строки поиска объявлены во всех состояниях и следуют за
// светлым флагом и акцентом.
func TestWin10Start_StylesAndLightFlag(t *testing.T) {
	m := win10Manager(t, ProfileWindows10)
	for _, part := range []string{"panel", "sidebar", "sidebar.item", "row", "letter", "tile", "tile.group", "row.sub", "scrollbar"} {
		if s := m.GetStyle("startmenu", part, StateNormal); s == nil {
			t.Errorf("нет части %s", part)
		}
	}
	// Поле поиска светлое и в тёмной панели; фокус — рамка акцента.
	box := m.GetStyle("searchbox", "", StateNormal)
	if box.Fill.R < 240 || box.BorderWidth != 1 {
		t.Errorf("поле поиска: заливка %v рамка %v", box.Fill, box.BorderWidth)
	}
	acc, _ := m.Accent()
	if got := m.GetStyle("searchbox", "", StateFocused).Border; got != acc {
		t.Errorf("рамка поля в фокусе %v, ждали акцент %v", got, acc)
	}
	if hover := m.GetStyle("searchbox", "", StateHover).Fill; hover.R < box.Fill.R {
		t.Errorf("при наведении поле темнее покоя: %v < %v", hover, box.Fill)
	}
	// Кнопка-значок прозрачна в покое, а текст следует за панелью.
	icon := m.GetStyle("searchbox", "icon", StateNormal)
	if icon.Fill.A != 0 || icon.Text.R < 200 {
		t.Errorf("значок поиска: заливка %v текст %v (тёмная панель — белый значок)", icon.Fill, icon.Text)
	}

	m.SetFlag(KeyTaskbarLight, true)
	if icon := m.GetStyle("searchbox", "icon", StateNormal); icon.Text.R > 50 {
		t.Errorf("светлая панель: значок поиска %v должен быть тёмным", icon.Text)
	}
	if panel := m.GetStyle("startmenu", "panel", StateNormal); panel.Backdrop.Tint.R < 200 {
		t.Errorf("светлая панель «Пуска»: подкраска %v", panel.Backdrop.Tint)
	}

	// Акцент плитки — по ссылке: смена акцента перекрашивает плитку.
	m.SetAccent(RGB(200, 20, 20))
	if fill := m.GetStyle("startmenu", "tile", StateNormal).Fill; fill != RGB(200, 20, 20) {
		t.Errorf("плитка после смены акцента: %v", fill)
	}
}

// Рамка панели «Пуска» занимает один пиксель; поля вокруг содержимого — тоже,
// чтобы боковая панель не затирала кромку.
func TestWin10Start_PanelPadForBorder(t *testing.T) {
	s := win10Manager(t, ProfileWindows10).GetStyle("startmenu", "panel", StateNormal)
	if s.PadX != 1 || s.BorderWidth != 1 || s.Elevation != 0 {
		t.Errorf("panel: PadX=%v BorderWidth=%v Elevation=%v, ждали 1, 1, 0", s.PadX, s.BorderWidth, s.Elevation)
	}
	// Плоский «Пуск» других тем остаётся с прежними полями.
	if f := win10Manager(t, ProfileWindows11).GetStyle("startmenu", "", StateNormal); f.PadX == 0 {
		t.Error("плоское меню Windows 11 потеряло поля")
	}
}
