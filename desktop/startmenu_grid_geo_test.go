package desktop

import (
	"fmt"
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Геометрия меню «Пуск» Windows 11: размер, положение, раскладка областей.

func gridPinned(n int) []StartPinned {
	out := make([]StartPinned, n)
	for i := range out {
		out[i] = StartPinned{ID: fmt.Sprintf("p%d", i), App: AppID(fmt.Sprintf("p%d", i)), Title: fmt.Sprintf("Приложение %d", i)}
	}
	return out
}

func gridRec(n int) *FakeStartRecommended {
	items := make([]StartRecommendedItem, n)
	for i := range items {
		items[i] = StartRecommendedItem{ID: fmt.Sprintf("r%d", i), Title: fmt.Sprintf("Файл %d", i), Subtitle: "Вчера"}
	}
	return NewFakeStartRecommended(items...)
}

// gridMenu — открытое меню Windows 11 на экране w×h с кнопкой «Пуск» у нижнего
// края (центр группы — центр экрана).
func gridMenu(t *testing.T, profile string, w, h, pinned, rec int) (*StartMenu, image.Rectangle) {
	t.Helper()
	tm := managerFor(t, profile)
	m := NewStartMenu(tm, NewStaticAppCatalog())
	m.Screen = image.Rect(0, 0, w, h)
	m.SetPinned(gridPinned(pinned))
	m.SetRecommended(gridRec(rec))
	anchor := image.Rect(w/2-110, h-44, w/2-70, h-4)
	m.Open(anchor)
	m.Settle()
	t.Cleanup(func() { m.Close(); widget.StopAllAnimations() })
	return m, anchor
}

func TestStartGrid_PresenterByTheme(t *testing.T) {
	for name, want := range map[string]startKind{
		theme.ProfileWindows11: startKindGrid, theme.ProfileWindows11Dark: startKindGrid,
		theme.ProfileWindows10: startKindTiles, theme.ProfileWindows10Dark: startKindTiles,
		theme.ProfileWindows2000: startKindFlat, theme.ProfileMacOS: startKindFlat,
	} {
		m := NewStartMenu(managerFor(t, name), NewStaticAppCatalog())
		if got := m.kind(); got != want {
			t.Errorf("%s: вид %d, ждали %d", name, got, want)
		}
		if m.AsGrid() != (want == startKindGrid) || m.AsTiled() != (want == startKindTiles) {
			t.Errorf("%s: AsGrid/AsTiled расходятся с видом", name)
		}
	}
}

// Метрики задания: 642×726, скругление 8, поля 32, сетка 6 колонок, ячейка 96×84,
// значок 32, нижняя полоса 64, поиск 32 высотой со скруглением 16.
func TestStartGrid_Metrics100(t *testing.T) {
	m, _ := gridMenu(t, theme.ProfileWindows11, 1920, 1080, 18, 6)
	want := map[theme.Key]float64{
		KeyStartW11Width: 642, KeyStartW11Height: 726, KeyStartW11Corner: 8, KeyStartW11Pad: 32,
		KeyStartW11GridColumns: 6, KeyStartW11GridCellW: 96, KeyStartW11GridCellH: 84, KeyStartW11GridIcon: 32,
		KeyStartW11FooterHeight: 64, KeyStartW11SearchHeight: 32, KeyStartW11SearchCorner: 16,
	}
	for k, v := range want {
		if got := m.tm.GetMetric(k); got != v {
			t.Errorf("%s = %v, ждали %v", k, got, v)
		}
	}
	r := m.OverlayBounds()
	if r.Dx() != 642 || r.Dy() != 726 {
		t.Fatalf("панель %v: ждали 642×726", r)
	}
	g := m.gridGeometry(r)
	if g.mode != gridMain || g.cols != 6 || g.cellW != 96 || g.cellH != 84 {
		t.Errorf("сетка: режим %v, %d колонок, ячейка %d×%d", g.mode, g.cols, g.cellW, g.cellH)
	}
	if g.search.Dy() != 32 || g.search.Min.X-r.Min.X != 32 || r.Max.X-g.search.Max.X != 32 {
		t.Errorf("поиск %v в панели %v: ждали высоту 32 и поля 32", g.search, r)
	}
	if g.footer.Dy() != 64 {
		t.Errorf("нижняя полоса %v: ждали высоту 64", g.footer)
	}
	if g.pinGrid.Dx() != 6*96 {
		t.Errorf("ширина сетки %d, ждали 576", g.pinGrid.Dx())
	}
	if g.recCols != 2 || g.recRows != 3 {
		t.Errorf("«Рекомендуем»: %d колонок, %d рядов, ждали 2 и 3", g.recCols, g.recRows)
	}
	if g.pages != 1 {
		t.Errorf("страниц %d для 18 закреплённых, ждали 1", g.pages)
	}
	checkGridSane(t, g)
}

// checkGridSane — области внутри панели, не налезают друг на друга и идут сверху
// вниз: поиск, заголовок, сетка, «Рекомендуем», нижняя полоса.
func checkGridSane(t *testing.T, g gridGeo) {
	t.Helper()
	in := func(name string, r image.Rectangle) {
		if !r.Empty() && !r.In(g.panel) {
			t.Errorf("%s %v вне панели %v", name, r, g.panel)
		}
	}
	in("search", g.search)
	in("footer", g.footer)
	in("user", g.user)
	in("power", g.power)
	in("list", g.list)
	if !g.user.Empty() && g.user.Overlaps(g.power) {
		t.Errorf("пользователь %v и питание %v пересекаются", g.user, g.power)
	}
	order := []struct {
		name string
		r    image.Rectangle
	}{{"search", g.search}}
	if g.mode == gridMain {
		in("pinGrid", g.pinGrid)
		in("recList", g.recList)
		order = append(order, struct {
			name string
			r    image.Rectangle
		}{"pinTitle", g.pinTitle}, struct {
			name string
			r    image.Rectangle
		}{"pinGrid", g.pinGrid})
		if g.recRows > 0 {
			order = append(order, struct {
				name string
				r    image.Rectangle
			}{"recTitle", g.recTitle}, struct {
				name string
				r    image.Rectangle
			}{"recList", g.recList})
		}
	} else {
		order = append(order, struct {
			name string
			r    image.Rectangle
		}{"list", g.list})
	}
	order = append(order, struct {
		name string
		r    image.Rectangle
	}{"footer", g.footer})
	for i := 1; i < len(order); i++ {
		if order[i].r.Min.Y < order[i-1].r.Max.Y && !order[i].r.Empty() {
			t.Errorf("%s %v начинается выше конца %s %v", order[i].name, order[i].r, order[i-1].name, order[i-1].r)
		}
	}
}

// Меню над кнопкой «Пуск»: при центрированной панели — по центру группы, при
// левой — у левого края кнопки; зазор до кнопки из метрики.
func TestStartGrid_PlacementCenteredAndLeft(t *testing.T) {
	m, anchor := gridMenu(t, theme.ProfileWindows11, 1920, 1080, 18, 6)
	r := m.OverlayBounds()
	if cx := (r.Min.X + r.Max.X) / 2; cx != 960 {
		t.Errorf("центрированная панель: меню по центру %d, ждали 960 (центр экрана)", cx)
	}
	if want := anchor.Min.Y - 12; r.Max.Y != want {
		t.Errorf("низ меню %d, ждали %d (кнопка минус зазор 12)", r.Max.Y, want)
	}

	// Центр группы задаёт потребитель.
	m.GroupBounds = func() image.Rectangle { return image.Rect(500, 1032, 700, 1072) }
	if cx := (m.OverlayBounds().Min.X + m.OverlayBounds().Max.X) / 2; cx != 600 {
		t.Errorf("по группе: центр %d, ждали 600", cx)
	}
	m.GroupBounds = nil

	// Левое выравнивание панели: на лету, без пересоздания меню.
	m.tm.SetFlag(KeyTaskbarCentered, false)
	left := image.Rect(8, 1032, 48, 1072)
	m.Close()
	m.Open(left)
	m.Settle()
	if r := m.OverlayBounds(); r.Min.X != left.Min.X {
		t.Errorf("левая панель: левый край меню %d, ждали %d (край кнопки)", r.Min.X, left.Min.X)
	}
}

// Меню не уезжает за край монитора: ни справа, ни слева, ни вверх; на низкой
// рабочей области высота ужимается.
func TestStartGrid_StaysInsideMonitor(t *testing.T) {
	// Кнопка у правого края при левой панели.
	tm := managerFor(t, theme.ProfileWindows11)
	tm.SetFlag(KeyTaskbarCentered, false)
	m := NewStartMenu(tm, NewStaticAppCatalog())
	m.Screen = image.Rect(0, 0, 1000, 600)
	m.SetPinned(gridPinned(12))
	defer widget.StopAllAnimations()
	m.Open(image.Rect(960, 556, 1000, 596))
	m.Settle()
	r := m.OverlayBounds()
	if !r.In(m.Screen) {
		t.Errorf("меню %v вышло за экран %v", r, m.Screen)
	}
	if r.Dy() > 600-44-12 {
		t.Errorf("высота %d не ужата под экран 600 и панель", r.Dy())
	}
	m.Close()

	// Узкий экран: меню сужается, а не уходит за край.
	m.Screen = image.Rect(0, 0, 500, 800)
	m.Open(image.Rect(0, 756, 40, 796))
	m.Settle()
	r = m.OverlayBounds()
	if !r.In(m.Screen) {
		t.Errorf("узкий экран: меню %v вышло за %v", r, m.Screen)
	}
	checkGridSane(t, m.gridGeometry(r))
}

// Масштабы 100–200 %: логический экран меньше физического, раскладка не
// обрезается и не выходит за панель.
func TestStartGrid_ScalesWithoutClipping(t *testing.T) {
	for _, phys := range []image.Point{{1920, 1080}, {1366, 768}, {1280, 720}} {
		for _, scale := range []float64{1, 1.25, 1.5, 1.75, 2} {
			w, h := int(float64(phys.X)/scale), int(float64(phys.Y)/scale)
			t.Run(fmt.Sprintf("%dx%d@%.0f%%", phys.X, phys.Y, scale*100), func(t *testing.T) {
				m, _ := gridMenu(t, theme.ProfileWindows11, w, h, 24, 6)
				r := m.OverlayBounds()
				if !r.In(m.Screen) {
					t.Fatalf("меню %v вышло за экран %v", r, m.Screen)
				}
				g := m.gridGeometry(r)
				checkGridSane(t, g)
				if g.rows < 1 || g.perPage < 1 {
					t.Errorf("пустая сетка: %d рядов", g.rows)
				}
				if g.pinGrid.Max.Y > g.footer.Min.Y {
					t.Errorf("сетка %v заходит под нижнюю полосу %v", g.pinGrid, g.footer)
				}
				if g.recRows > 0 && g.recList.Max.Y > g.footer.Min.Y {
					t.Errorf("«Рекомендуем» %v заходит под нижнюю полосу %v", g.recList, g.footer)
				}
			})
		}
	}
}

// Выключенный источник: раздел «Рекомендуем» исчезает, закреплённые занимают
// его место.
func TestStartGrid_RecommendedOffGivesRoomToPinned(t *testing.T) {
	m, _ := gridMenu(t, theme.ProfileWindows11, 1920, 1080, 40, 6)
	with := m.gridGeometry(m.OverlayBounds())
	if with.recRows == 0 {
		t.Fatal("раздел «Рекомендуем» не показан при включённом источнике")
	}
	m.SetRecommendedEnabled(false)
	without := m.gridGeometry(m.OverlayBounds())
	if without.recRows != 0 || !without.recList.Empty() || !without.moreBtn.Empty() {
		t.Errorf("раздел остался при выключенном источнике: %d рядов", without.recRows)
	}
	if without.perPage <= with.perPage {
		t.Errorf("закреплённым не прибавилось места: %d → %d на страницу", with.perPage, without.perPage)
	}
	// Пустой источник — то же.
	m.SetRecommendedEnabled(true)
	m.SetRecommended(NewFakeStartRecommended())
	if g := m.gridGeometry(m.OverlayBounds()); g.recRows != 0 {
		t.Errorf("пустой источник показал %d рядов", g.recRows)
	}
}

// Закреплённых больше страницы: появляются точки страниц справа.
func TestStartGrid_PagesAndDots(t *testing.T) {
	m, _ := gridMenu(t, theme.ProfileWindows11, 1920, 1080, 40, 6)
	g := m.gridGeometry(m.OverlayBounds())
	if g.perPage != g.cols*g.rows {
		t.Fatalf("на страницу %d, ждали %d", g.perPage, g.cols*g.rows)
	}
	if want := (40 + g.perPage - 1) / g.perPage; g.pages != want || g.pages < 2 {
		t.Fatalf("страниц %d, ждали %d", g.pages, want)
	}
	if g.dots.Empty() || g.dots.Min.X < g.pinGrid.Max.X || !g.dots.In(g.panel) {
		t.Errorf("точки страниц %v не справа от сетки %v в панели %v", g.dots, g.pinGrid, g.panel)
	}
	few, _ := gridMenu(t, theme.ProfileWindows11, 1920, 1080, 6, 6)
	if g := few.gridGeometry(few.OverlayBounds()); g.pages != 1 || !g.dots.Empty() {
		t.Errorf("6 закреплённых: страниц %d, точки %v", g.pages, g.dots)
	}
}

// Тёмный Windows 11 даёт ту же раскладку: меняются только токены.
func TestStartGrid_DarkSameGeometry(t *testing.T) {
	l, _ := gridMenu(t, theme.ProfileWindows11, 1920, 1080, 18, 6)
	d, _ := gridMenu(t, theme.ProfileWindows11Dark, 1920, 1080, 18, 6)
	if l.OverlayBounds() != d.OverlayBounds() {
		t.Errorf("панель светлой %v и тёмной %v различается", l.OverlayBounds(), d.OverlayBounds())
	}
	if l.gridGeometry(l.OverlayBounds()) != d.gridGeometry(d.OverlayBounds()) {
		t.Error("раскладка светлой и тёмной тем различается")
	}
}
