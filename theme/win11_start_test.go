package theme

import "testing"

// Меню «Пуск» Windows 11: презентер, метрики из задания и части стиля.

// Сетку закреплённых просит только Windows 11 (и её тёмная разновидность по
// наследству).
func TestWin11Start_PresenterOnlyForWindows11(t *testing.T) {
	for name, want := range map[string]string{
		ProfileWindows11: PresenterStartGrid, ProfileWindows11Dark: PresenterStartGrid,
		ProfileWindows10: PresenterStartTiles, ProfileWindows10Dark: PresenterStartTiles,
		ProfileWindows2000: "", ProfileWindows2000Blue: "", ProfileMacOS: "", ProfileMacOSDark: "",
	} {
		if got := win10Manager(t, name).Active().PresenterName("startmenu"); got != want {
			t.Errorf("%s: презентер меню %q, ждали %q", name, got, want)
		}
	}
}

// Метрики задания: 642×726, скругление 8, поля 32, сетка 6 колонок, ячейка
// 96×84, значок 32, нижняя полоса 64.
func TestWin11Start_MetricsFromTask(t *testing.T) {
	want := map[Key]float64{
		"startmenu.w11.width": 642, "startmenu.w11.height": 726, "startmenu.w11.corner": 8,
		"startmenu.w11.pad": 32, "startmenu.w11.grid.columns": 6, "startmenu.w11.grid.cell.w": 96,
		"startmenu.w11.grid.cell.h": 84, "startmenu.w11.grid.icon": 32, "startmenu.w11.footer.height": 64,
		"startmenu.w11.search.height": 32, "startmenu.w11.search.corner": 16,
	}
	for _, name := range []string{ProfileWindows11, ProfileWindows11Dark} {
		m := win10Manager(t, name)
		for k, v := range want {
			if got := m.GetMetric(k); got != v {
				t.Errorf("%s: %s = %v, ждали %v", name, k, got, v)
			}
		}
	}
	// Метрик Windows 11 нет у других тем: там своё меню.
	for _, name := range []string{ProfileWindows10, ProfileWindows2000, ProfileMacOS} {
		if got := win10Manager(t, name).GetMetric("startmenu.w11.width"); got != 0 {
			t.Errorf("%s получила метрику меню Windows 11: %v", name, got)
		}
	}
}

// Тёмная разновидность меняет цвета токенами: общие части стиля светлой темы
// (плёнки наведения, рамки, текст) одинаковы, а токены дают свои значения.
func TestWin11Start_DarkChangesTokensOnly(t *testing.T) {
	l, d := win10Manager(t, ProfileWindows11), win10Manager(t, ProfileWindows11Dark)
	if lc, _ := l.Active().Color(KeyField); lc.R < 240 {
		t.Errorf("светлое поле поиска %v: ждали почти белое", lc)
	}
	if dc, _ := d.Active().Color(KeyField); dc.R > 80 {
		t.Errorf("тёмное поле поиска %v: ждали тёмно-серое", dc)
	}
	for _, part := range []string{"pin", "rec", "row", "link", "footer.item"} {
		lf := l.GetStyle("startmenu", part, StateHover).Fill
		df := d.GetStyle("startmenu", part, StateHover).Fill
		if lf != df {
			t.Errorf("часть %s: плёнка наведения светлой %v и тёмной %v должны совпадать (нейтральный серый)", part, lf, df)
		}
		if lf.A == 0 {
			t.Errorf("часть %s: плёнка наведения невидима", part)
		}
	}
	// Текст берётся из токена: белый в тёмной, чёрный в светлой.
	if lt := l.GetStyle("startmenu", "pin", StateNormal).Text; lt.R > 30 {
		t.Errorf("текст ячейки светлой темы %v", lt)
	}
	if dt := d.GetStyle("startmenu", "pin", StateNormal).Text; dt.R < 225 {
		t.Errorf("текст ячейки тёмной темы %v", dt)
	}
	// Акцентная рамка поля в фокусе следует за акцентом.
	l.SetAccent(RGB(190, 30, 60))
	if b := l.GetStyle("startmenu", "search", StateActive).Border; b.R < 150 || b.G > 80 {
		t.Errorf("рамка поля в фокусе %v не стала красной с акцентом (190,30,60)", b)
	}
}

// Части «Пуска» не получают Mica и большой тени панели: материал и тень по
// токенам — только у корня.
func TestWin11Start_PartsGetNoMaterialOrShadow(t *testing.T) {
	m := win10Manager(t, ProfileWindows11)
	m.SetFlag(FlagBackdropMica, true)
	m.SetFlag(FlagShadowSoft, true)
	root := m.GetStyle("startmenu", "", StateNormal)
	if root.Backdrop.Material != MaterialMica {
		t.Fatalf("корень меню без Mica при флаге: %+v", root.Backdrop)
	}
	if root.ShadowBlur == 0 {
		t.Error("корень меню без мягкой тени при флаге")
	}
	for _, part := range []string{"pin", "rec", "link", "search", "footer", "row", "dot", "avatar"} {
		s := m.GetStyle("startmenu", part, StateNormal)
		if s.Backdrop.Material != MaterialDefault {
			t.Errorf("часть %s получила материал %v", part, s.Backdrop.Material)
		}
		if s.ShadowBlur != 0 {
			t.Errorf("часть %s получила тень %v", part, s.ShadowBlur)
		}
	}
}

// Без флагов — сплошная панель с прежними токенами (Solid по умолчанию).
func TestWin11Start_SolidByDefault(t *testing.T) {
	root := win10Manager(t, ProfileWindows11).GetStyle("startmenu", "", StateNormal)
	if root.Backdrop.Material != MaterialDefault || root.Backdrop.Mode != BackdropNone {
		t.Errorf("по умолчанию панель не сплошная: %+v", root.Backdrop)
	}
	if root.BorderWidth != 1 || root.Border.A == 0 {
		t.Errorf("рамка панели %v шириной %v", root.Border, root.BorderWidth)
	}
}
