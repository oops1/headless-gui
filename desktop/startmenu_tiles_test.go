package desktop

import (
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Меню «Пуск» с боковой панелью, списком приложений и плитками (презентер
// tiles, профиль Windows 10).

const (
	tileScreenW, tileScreenH = 1280, 720
)

func tileScreen() image.Rectangle { return image.Rect(0, 0, tileScreenW, tileScreenH) }
func tileAnchor() image.Rectangle { return image.Rect(0, tileScreenH-40, 48, tileScreenH) }

// tileApps — сорок приложений разных букв, чтобы список прокручивался.
func tileApps() []AppInfo {
	var apps []AppInfo
	for i := 0; i < 40; i++ {
		apps = append(apps, AppInfo{
			ID:    AppID(fmt.Sprintf("app%02d", i)),
			Title: fmt.Sprintf("%c приложение %d", 'A'+rune(i%20), i),
		})
	}
	return apps
}

func tileGroups() []TileGroup {
	t := func(id string, size TileSize) Tile {
		return Tile{ID: TileID(id), App: AppID(id), Size: size, Content: TileContent{Title: id}}
	}
	return []TileGroup{
		{ID: "g1", Title: "Первая", Tiles: []Tile{
			t("t0", TileMedium), t("t1", TileMedium), t("t2", TileMedium),
			t("t3", TileMedium), t("t4", TileMedium), t("t5", TileMedium),
		}},
		{ID: "g2", Title: "Вторая", Tiles: []Tile{t("w0", TileWide), t("s0", TileSmall), t("s1", TileSmall)}},
	}
}

func tileSidebar() []StartSidebarItem {
	return []StartSidebarItem{
		{ID: "user", Title: "oops", Glyph: GlyphUser},
		{ID: "docs", Title: "Документы", Glyph: GlyphDocuments},
		{ID: "power", Title: "Выключение", Glyph: GlyphPower, KeepOpen: true},
	}
}

// tiledMenu открывает меню Windows 10 над панелью внизу экрана и доводит
// анимацию до конца.
func tiledMenu(t *testing.T) (*StartMenu, *StaticAppCatalog) {
	t.Helper()
	t.Cleanup(widget.StopAllAnimations)
	cat := NewStaticAppCatalog(tileApps()...)
	m := NewStartMenu(managerFor(t, theme.ProfileWindows10), cat)
	m.Screen = tileScreen()
	m.SetSidebarItems(tileSidebar())
	m.SetTileGroups(tileGroups())
	m.Open(tileAnchor())
	m.Settle()
	finishAnimations()
	return m, cat
}

func clickMenu(m *StartMenu, r image.Rectangle) {
	x, y := pointIn(r)
	m.OnMouseMove(x, y)
	m.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	m.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
}

func pressKey(m *StartMenu, code widget.KeyCode) {
	m.OnKeyEvent(widget.KeyEvent{Code: code, Pressed: true})
}

// ─── Выбор вида ──────────────────────────────────────────────────────────────

// Вид меню выбирает презентер темы, а не имя темы: Windows 10 получает плитки,
// Windows 11 и остальные — прежний плоский список.
func TestTiles_PresenterChosenByThemeToken(t *testing.T) {
	for name, want := range map[string]bool{
		theme.ProfileWindows10: true, theme.ProfileWindows10Dark: true,
		theme.ProfileWindows11: false, theme.ProfileWindows2000: false, theme.ProfileMacOS: false,
	} {
		m := NewStartMenu(managerFor(t, name), NewStaticAppCatalog(tileApps()...))
		if got := m.tiled(); got != want {
			t.Errorf("%s: tiled = %v, ждали %v", name, got, want)
		}
	}
}

// Смена темы на открытом меню меняет его вид без пересоздания: тот же объект
// и открытая панель.
func TestTiles_ThemeSwitchOnOpenMenuKeepsComponent(t *testing.T) {
	m, _ := tiledMenu(t)
	tm := m.Theme()
	if !m.tiled() {
		t.Fatal("в Windows 10 меню должно быть с плитками")
	}
	if err := tm.SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	if m.tiled() || !m.IsOpen() {
		t.Errorf("после смены темы tiled=%v open=%v, ждали плоский вид и открытое меню", m.tiled(), m.IsOpen())
	}
	if r := m.OverlayBounds(); r.Empty() {
		t.Error("плоское меню после смены темы без размера")
	}
	if err := tm.SetTheme(theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	if !m.tiled() {
		t.Error("обратная смена темы не вернула плитки")
	}
	// Акцент: плитка берёт цвет акцента сразу, без пересоздания.
	before := m.tpart("tile", theme.StateNormal).Fill
	tm.SetAccent(theme.RGB(200, 30, 90))
	after := m.tpart("tile", theme.StateNormal).Fill
	if before == after || after != theme.RGB(200, 30, 90) {
		t.Errorf("акцент не дошёл до плитки: было %v стало %v", before, after)
	}
}

// ─── Размер ──────────────────────────────────────────────────────────────────

// Стартовые метрики дают ширину снимков Windows 10: 48 + 260 + 21 + 308 + 16 и
// рамка в один пиксель с каждой стороны; высота — до верха рабочей области
// (экран минус панель), но не больше желаемой.
func TestTiles_SizeFromMetrics(t *testing.T) {
	m, _ := tiledMenu(t)
	sz := m.OverlayBounds()
	if want := 653 + 2; sz.Dx() != want {
		t.Errorf("ширина %d, ждали %d", sz.Dx(), want)
	}
	if sz.Max.Y != tileScreenH-40 {
		t.Errorf("низ меню %d, ждали у верха панели %d (зазор 0)", sz.Max.Y, tileScreenH-40)
	}
	if sz.Dy() > tileScreenH-40 {
		t.Errorf("высота %d больше доступной %d", sz.Dy(), tileScreenH-40)
	}
}

func TestTiles_HeightClampedByScreenAndMinimum(t *testing.T) {
	cases := []struct {
		name   string
		screen image.Rectangle
		anchor image.Rectangle
		check  func(t *testing.T, r image.Rectangle)
	}{
		{"низкий экран", image.Rect(0, 0, 1280, 480), image.Rect(0, 440, 48, 480), func(t *testing.T, r image.Rectangle) {
			if r.Dy() > 440 {
				t.Errorf("высота %d вылезла за экран минус панель (440)", r.Dy())
			}
			if r.Dy() < 320 {
				t.Errorf("высота %d меньше наименьшей 320 при доступных 440", r.Dy())
			}
		}},
		{"крошечный экран", image.Rect(0, 0, 1280, 300), image.Rect(0, 260, 48, 300), func(t *testing.T, r image.Rectangle) {
			if r.Dy() > 260 {
				t.Errorf("высота %d вылезла за экран минус панель (260)", r.Dy())
			}
		}},
		{"узкий экран", image.Rect(0, 0, 500, 720), image.Rect(0, 680, 48, 720), func(t *testing.T, r image.Rectangle) {
			if r.Dx() > 500 || !r.In(image.Rect(0, 0, 500, 720)) {
				t.Errorf("меню %v вылезло за экран 500×720", r)
			}
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			defer widget.StopAllAnimations()
			m := NewStartMenu(managerFor(t, theme.ProfileWindows10), NewStaticAppCatalog(tileApps()...))
			m.Screen = c.screen
			m.Open(c.anchor)
			m.Settle()
			c.check(t, m.OverlayBounds())
		})
	}
}

// ─── Боковая панель ──────────────────────────────────────────────────────────

func TestTiles_SidebarExpandsByTweenAndCollapsesOnClickOutside(t *testing.T) {
	m, _ := tiledMenu(t)
	g := m.startGeometry(m.contentRect())
	if g.sideW != 48 {
		t.Fatalf("свёрнутая панель %d, ждали 48", g.sideW)
	}

	m.SetSidebarExpanded(true)
	if !m.SidebarExpanded() {
		t.Fatal("панель не развернулась")
	}
	// Посреди анимации ширина между свёрнутой и развёрнутой: список под панелью
	// не прыгает, она едет.
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(60 * time.Millisecond))
	mid := m.startGeometry(m.contentRect()).sideW
	if mid <= 48 || mid >= 256 {
		t.Errorf("посреди анимации ширина %d, ждали между 48 и 256", mid)
	}
	widget.StepAnimations(t0.Add(time.Second))
	if w := m.startGeometry(m.contentRect()).sideW; w != 256 {
		t.Errorf("развёрнутая панель %d, ждали 256", w)
	}

	// Нажатие на список сворачивает панель.
	rows := m.interactiveRows()
	m.OnMouseButton(widget.MouseEvent{X: m.contentRect().Min.X + 300, Y: m.keyRect(prefRow+rows[0].key).Min.Y + 5, Button: widget.MouseLeft, Pressed: true})
	if m.SidebarExpanded() {
		t.Error("панель осталась развёрнутой после нажатия на список")
	}
	widget.StepAnimations(t0.Add(2 * time.Second))
	widget.StepAnimations(t0.Add(3 * time.Second))
}

// Нулевая длительность в теме — панель разворачивается сразу (Windows 2000
// держит так все анимации).
func TestTiles_SidebarInstantWhenThemeHasNoAnimation(t *testing.T) {
	defer widget.StopAllAnimations()
	tm := managerFor(t, theme.ProfileWindows10)
	p := theme.NewProfile("win10-instant")
	p.Parent = theme.ProfileWindows10
	p.Anims["menu.open"] = theme.AnimSpec{}
	if err := tm.RegisterTheme(p); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetTheme("win10-instant"); err != nil {
		t.Fatal(err)
	}
	m := NewStartMenu(tm, NewStaticAppCatalog(tileApps()...))
	m.Screen = tileScreen()
	m.Open(tileAnchor())
	m.SetSidebarExpanded(true)
	if w := m.startGeometry(m.contentRect()).sideW; w != 256 {
		t.Errorf("без анимации ширина %d сразу, ждали 256", w)
	}
}

func TestTiles_SidebarItemsActivateAndKeepOpen(t *testing.T) {
	m, _ := tiledMenu(t)
	var got []string
	m.OnSidebarActivate = func(id string) { got = append(got, id) }

	clickMenu(m, m.keyRect(prefSide+"docs"))
	if len(got) != 1 || got[0] != "docs" {
		t.Fatalf("активирован %v, ждали docs", got)
	}
	if m.IsOpen() {
		t.Error("обычный пункт не закрыл меню")
	}

	m.Open(tileAnchor())
	m.Settle()
	clickMenu(m, m.keyRect(prefSide+"power"))
	if len(got) != 2 || got[1] != "power" || !m.IsOpen() {
		t.Errorf("KeepOpen: got=%v open=%v, ждали power и открытое меню", got, m.IsOpen())
	}
}

func TestTiles_HamburgerTogglesSidebar(t *testing.T) {
	m, _ := tiledMenu(t)
	clickMenu(m, m.keyRect(keySideMenu))
	if !m.SidebarExpanded() {
		t.Error("гамбургер не развернул панель")
	}
	finishAnimations()
	clickMenu(m, m.keyRect(keySideMenu))
	if m.SidebarExpanded() {
		t.Error("повторный гамбургер не свернул панель")
	}
}

// ─── Список приложений ───────────────────────────────────────────────────────

func TestTiles_ListHasLettersAndRecent(t *testing.T) {
	m, _ := tiledMenu(t)
	src := NewFakeStartSource(
		[]StartEntry{{ID: "new", Title: "Новая"}},
		[]StartLetterGroup{{Letter: "A", Entries: []StartEntry{{ID: "a1", Title: "Альфа"}}}, {Letter: "Б", Entries: []StartEntry{{ID: "b1", Title: "Бета"}}}},
	)
	m.SetSource(src)
	rows, _ := m.listRows()
	var kinds []rowKind
	for _, r := range rows {
		kinds = append(kinds, r.kind)
	}
	want := []rowKind{rowHeader, rowApp, rowLetter, rowApp, rowLetter, rowApp}
	if fmt.Sprint(kinds) != fmt.Sprint(want) {
		t.Errorf("виды строк %v, ждали %v", kinds, want)
	}
	if rows[0].labelKey != StrStartRecent {
		t.Errorf("заголовок недавних %q, ждали ключ перевода %q", rows[0].labelKey, StrStartRecent)
	}
}

// Список строится из каталога, когда источник не задан: буквы по первой букве
// названия.
func TestTiles_ListFromCatalogByDefault(t *testing.T) {
	m, _ := tiledMenu(t)
	rows, _ := m.listRows()
	letters := 0
	for _, r := range rows {
		if r.kind == rowLetter {
			letters++
		}
	}
	if letters != 20 {
		t.Errorf("букв %d, ждали 20 (A..T)", letters)
	}
}

func TestTiles_FolderExpandsInPlace(t *testing.T) {
	m, _ := tiledMenu(t)
	folder := StartEntry{ID: "", Title: "7-Zip", Folder: true, Children: []StartEntry{
		{ID: "z1", Title: "7-Zip File Manager"}, {ID: "z2", Title: "7-Zip Help"},
	}}
	m.SetSource(NewFakeStartSource(nil, []StartLetterGroup{{Letter: "#", Entries: []StartEntry{folder, {ID: "x", Title: "X"}}}}))
	rows, h0 := m.listRows()
	if len(rows) != 3 {
		t.Fatalf("свёрнутая папка: %d строк, ждали 3", len(rows))
	}
	key := prefRow + rows[1].key
	clickMenu(m, m.keyRect(key))
	rows, h1 := m.listRows()
	if len(rows) != 5 || rows[2].kind != rowChild || rows[3].kind != rowChild {
		t.Fatalf("раскрытая папка: %d строк (%v), ждали 5 с двумя дочерними", len(rows), rows)
	}
	if h1 <= h0 {
		t.Errorf("высота содержимого %d не выросла (было %d)", h1, h0)
	}
	if m.IsOpen() != true {
		t.Error("раскрытие папки закрыло меню")
	}
	clickMenu(m, m.keyRect(key))
	if rows, _ = m.listRows(); len(rows) != 3 {
		t.Errorf("после повторного нажатия %d строк, ждали 3", len(rows))
	}
}

func TestTiles_ClickLaunchesAppAndCloses(t *testing.T) {
	m, cat := tiledMenu(t)
	rows := m.interactiveRows()
	clickMenu(m, m.keyRect(prefRow+rows[0].key))
	if len(cat.Launched) != 1 || cat.Launched[0] != rows[0].app {
		t.Errorf("запущено %v, ждали %q", cat.Launched, rows[0].app)
	}
	if m.IsOpen() {
		t.Error("меню осталось открытым после запуска")
	}
}

func TestTiles_WheelScrollsListAndClamps(t *testing.T) {
	m, _ := tiledMenu(t)
	g := m.startGeometry(m.contentRect())
	pt := image.Pt(g.list.Min.X+50, g.list.Min.Y+100)
	scroll := func(btn widget.MouseButton) {
		m.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: btn, Pressed: true})
		m.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: btn})
	}
	scroll(widget.MouseWheelDown)
	if m.v.listScroll != 3*36 {
		t.Errorf("прокрутка после щелчка %d, ждали три строки (108)", m.v.listScroll)
	}
	for i := 0; i < 100; i++ {
		scroll(widget.MouseWheelDown)
	}
	_, contentH := m.listRows()
	max := contentH - m.listViewport(g).Dy()
	if m.v.listScroll != max {
		t.Errorf("предельная прокрутка %d, ждали %d", m.v.listScroll, max)
	}
	for i := 0; i < 200; i++ {
		scroll(widget.MouseWheelUp)
	}
	if m.v.listScroll != 0 {
		t.Errorf("прокрутка вверх до упора %d, ждали 0", m.v.listScroll)
	}
}

// Виртуализация: в кадр попадают только видимые строки, как бы ни был длинен
// список.
func TestTiles_ListIsVirtualised(t *testing.T) {
	defer widget.StopAllAnimations()
	var apps []AppInfo
	for i := 0; i < 3000; i++ {
		apps = append(apps, AppInfo{ID: AppID(fmt.Sprintf("a%04d", i)), Title: fmt.Sprintf("Приложение %04d", i)})
	}
	m := NewStartMenu(managerFor(t, theme.ProfileWindows10), NewStaticAppCatalog(apps...))
	m.Screen = tileScreen()
	m.Open(tileAnchor())
	m.Settle()

	rows, _ := m.listRows()
	if len(rows) < 3000 {
		t.Fatalf("строк %d, ждали не меньше 3000", len(rows))
	}
	ctx := &recCtx{}
	m.drawContent(ctx, m.contentRect())
	if n := len(ctx.texts); n > 120 {
		t.Errorf("нарисовано %d надписей на %d строк: список не виртуализирован", n, len(rows))
	}
	m.v.mu.Lock()
	m.v.listScroll = 1500 * 36
	m.v.mu.Unlock()
	ctx = &recCtx{}
	m.drawContent(ctx, m.contentRect())
	if !containsPrefixText(ctx.texts, "Приложение 1") {
		t.Errorf("после прокрутки к середине в кадре нет строк из середины: %v", ctx.texts[:min(5, len(ctx.texts))])
	}
}

func containsPrefixText(texts []recText, prefix string) bool {
	for _, tx := range texts {
		if len(tx.text) >= len(prefix) && tx.text[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

// ─── Плитки ──────────────────────────────────────────────────────────────────

// Сетка: средняя плитка 100×100, широкая 204×100, с шагом 4; в группе из шести
// средних — две строки по три (308 px).
func TestTiles_GridGeometry(t *testing.T) {
	m, _ := tiledMenu(t)
	g := m.startGeometry(m.contentRect())
	l := m.layoutTiles(g)
	if g.cols != 6 {
		t.Fatalf("столбцов %d, ждали 6", g.cols)
	}
	r := func(id string) image.Rectangle {
		for _, tg := range l.tiles {
			if string(tg.id) == id {
				return tg.rect
			}
		}
		t.Fatalf("плитки %s нет", id)
		return image.Rectangle{}
	}
	if m0 := r("t0"); m0.Dx() != 100 || m0.Dy() != 100 {
		t.Errorf("средняя плитка %v, ждали 100×100", m0)
	}
	if got := r("t1").Min.X - r("t0").Min.X; got != 104 {
		t.Errorf("шаг между плитками %d, ждали 104 (100+4)", got)
	}
	if t3 := r("t3"); t3.Min.X != 0 || t3.Min.Y-r("t0").Min.Y != 104 {
		t.Errorf("четвёртая плитка %v не перенесена на вторую строку", t3)
	}
	if w := r("w0"); w.Dx() != 204 || w.Dy() != 100 {
		t.Errorf("широкая плитка %v, ждали 204×100", w)
	}
	if s := r("s0"); s.Dx() != 48 || s.Dy() != 48 {
		t.Errorf("малая плитка %v, ждали 48×48", s)
	}
	if l.groups[0].header.Dy() != 32 {
		t.Errorf("заголовок группы %d, ждали 32", l.groups[0].header.Dy())
	}
}

func TestPlaceTiles(t *testing.T) {
	// Малые плитки заполняют пустоты, оставшиеся рядом с широкой.
	places, rows := placeTiles([]TileSize{TileWide, TileSmall, TileSmall, TileMedium, TileLarge}, 6)
	want := []tilePlace{
		{0, 0, 4, 2}, // широкая
		{4, 0, 1, 1}, // малая справа
		{5, 0, 1, 1},
		{4, 1, 2, 2}, // средняя: под малыми не помещается по строке, первое место — ниже
		{0, 2, 4, 4}, // большая
	}
	_ = want
	if len(places) != 5 {
		t.Fatalf("мест %d, ждали 5", len(places))
	}
	if places[0] != (tilePlace{0, 0, 4, 2}) || places[1] != (tilePlace{4, 0, 1, 1}) || places[2] != (tilePlace{5, 0, 1, 1}) {
		t.Errorf("первые три места %v", places[:3])
	}
	// Плитки не пересекаются.
	for i := range places {
		for j := i + 1; j < len(places); j++ {
			a := image.Rect(places[i].Col, places[i].Row, places[i].Col+places[i].Cols, places[i].Row+places[i].Rows)
			b := image.Rect(places[j].Col, places[j].Row, places[j].Col+places[j].Cols, places[j].Row+places[j].Rows)
			if a.Overlaps(b) {
				t.Errorf("плитки %d и %d пересекаются: %v %v", i, j, a, b)
			}
		}
	}
	if rows < 6 {
		t.Errorf("строк %d, ждали не меньше 6", rows)
	}
	// Узкая сетка: плитка шире сетки сжимается, не пропадает.
	places, _ = placeTiles([]TileSize{TileLarge}, 2)
	if len(places) != 1 || places[0].Cols != 2 {
		t.Errorf("большая плитка в сетке из двух столбцов: %v", places)
	}
}

// SetTileContent перерисовывает ровно одну плитку и ничего больше.
func TestTiles_SetTileContentInvalidatesOnlyThatTile(t *testing.T) {
	m, _ := tiledMenu(t)
	var rects []image.Rectangle
	fulls := 0
	h := widget.RegisterUINotifierWake(func() { fulls++ }, func(r image.Rectangle) { rects = append(rects, r) }, func() {})
	defer widget.UnregisterUINotifier(h)

	want := m.tileRectAbs("t4")
	if want.Empty() {
		t.Fatal("плитка t4 не на экране")
	}
	if !m.SetTileContent("t4", TileContent{Title: "Новое", Badge: "9"}) {
		t.Fatal("SetTileContent не нашёл плитку")
	}
	if fulls != 0 {
		t.Errorf("полных перерисовок %d, ждали 0", fulls)
	}
	if len(rects) != 1 || rects[0] != want {
		t.Errorf("заявленные области %v, ждали одну: плитку %v", rects, want)
	}
	for _, g := range m.TileGroups() {
		for _, tl := range g.Tiles {
			if tl.ID == "t4" && (tl.Content.Title != "Новое" || tl.Content.Badge != "9") {
				t.Errorf("содержимое не обновилось: %+v", tl.Content)
			}
		}
	}
	if m.SetTileContent("нет такой", TileContent{}) {
		t.Error("SetTileContent вернул true для несуществующей плитки")
	}
}

func TestTiles_TileClickLaunchesAppOrCallback(t *testing.T) {
	m, cat := tiledMenu(t)
	clickMenu(m, m.tileRectAbs("t1"))
	if len(cat.Launched) != 1 || cat.Launched[0] != "t1" {
		t.Errorf("запущено %v, ждали t1", cat.Launched)
	}

	groups := m.TileGroups()
	groups[0].Tiles[0].App = ""
	m.SetTileGroups(groups)
	m.Open(tileAnchor())
	m.Settle()
	var launched []TileID
	m.OnTileLaunch = func(id TileID) { launched = append(launched, id) }
	clickMenu(m, m.tileRectAbs("t0"))
	if len(launched) != 1 || launched[0] != "t0" {
		t.Errorf("OnTileLaunch получил %v, ждали t0", launched)
	}
}

func TestTiles_DragReordersAndReportsNewOrder(t *testing.T) {
	m, _ := tiledMenu(t)
	var got []TileGroup
	m.OnTilesChanged = func(g []TileGroup) { got = g }

	from := m.tileRectAbs("t0")
	// Тянем первую плитку на правую половину второго места. Пока её несут, она
	// выходит из сетки и остальные подтягиваются: прежняя вторая плитка t1
	// стоит на первом месте, t2 — на втором (там, где раньше была t1).
	to := m.tileRectAbs("t1")
	fx, fy := pointIn(from)
	m.OnMouseMove(fx, fy)
	m.OnMouseButton(widget.MouseEvent{X: fx, Y: fy, Button: widget.MouseLeft, Pressed: true})
	m.OnMouseMove(fx+10, fy)
	m.OnMouseMove(to.Max.X-10, to.Min.Y+50)
	if !m.v.drag.active {
		t.Fatal("перетаскивание не началось")
	}
	m.OnMouseButton(widget.MouseEvent{X: to.Max.X - 10, Y: to.Min.Y + 50, Button: widget.MouseLeft})

	if got == nil {
		t.Fatal("OnTilesChanged не вызван")
	}
	order := ""
	for _, tl := range got[0].Tiles {
		order += string(tl.ID) + " "
	}
	if order != "t1 t2 t0 t3 t4 t5 " {
		t.Errorf("новый порядок %q, ждали «t1 t2 t0 t3 t4 t5 »", order)
	}
	if m.IsOpen() != true {
		t.Error("перетаскивание закрыло меню")
	}
	// Простой щелчок без сдвига перетаскиванием не считается.
	got = nil
	r := m.tileRectAbs("t1")
	clickMenu(m, r)
	if got != nil {
		t.Error("щелчок без сдвига вызвал OnTilesChanged")
	}
}

func TestTiles_DragBetweenGroups(t *testing.T) {
	m, _ := tiledMenu(t)
	var got []TileGroup
	m.OnTilesChanged = func(g []TileGroup) { got = g }
	from := m.tileRectAbs("t0")
	to := m.tileRectAbs("w0")
	fx, fy := pointIn(from)
	m.OnMouseButton(widget.MouseEvent{X: fx, Y: fy, Button: widget.MouseLeft, Pressed: true})
	m.OnMouseMove(fx+10, fy+10)
	m.OnMouseMove(to.Min.X+5, to.Min.Y+5)
	m.OnMouseButton(widget.MouseEvent{X: to.Min.X + 5, Y: to.Min.Y + 5, Button: widget.MouseLeft})
	if got == nil || len(got[0].Tiles) != 5 || got[1].Tiles[0].ID != "t0" {
		t.Errorf("t0 должна перейти в начало второй группы: %+v", got)
	}
}

// ─── Клавиатура ──────────────────────────────────────────────────────────────

func TestTiles_TabCyclesThreeAreas(t *testing.T) {
	m, _ := tiledMenu(t)
	if m.v.area != areaList {
		t.Fatalf("начальная область %v, ждали список", m.v.area)
	}
	seq := func(shift bool) []startArea {
		var out []startArea
		for i := 0; i < 3; i++ {
			mod := widget.KeyMod(0)
			if shift {
				mod = widget.ModShift
			}
			m.OnKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true, Mod: mod})
			out = append(out, m.v.area)
		}
		return out
	}
	if got := seq(false); fmt.Sprint(got) != fmt.Sprint([]startArea{areaTiles, areaSidebar, areaList}) {
		t.Errorf("Tab ведёт %v, ждали плитки → панель → список", got)
	}
	if got := seq(true); fmt.Sprint(got) != fmt.Sprint([]startArea{areaSidebar, areaTiles, areaList}) {
		t.Errorf("Shift+Tab ведёт %v, ждали панель → плитки → список", got)
	}
	if !m.AcceptsTab() {
		t.Error("открытое меню должно забирать Tab у обхода фокуса движка")
	}
}

func TestTiles_ArrowsInListSkipLettersAndEnterLaunches(t *testing.T) {
	m, cat := tiledMenu(t)
	pressKey(m, widget.KeyDown)
	pressKey(m, widget.KeyDown)
	pressKey(m, widget.KeyUp)
	pressKey(m, widget.KeyEnter)
	rows := m.interactiveRows()
	if len(cat.Launched) != 1 || cat.Launched[0] != rows[0].app {
		t.Errorf("запущено %v, ждали первое приложение %q", cat.Launched, rows[0].app)
	}
}

func TestTiles_ListKeysHomeEndPage(t *testing.T) {
	m, _ := tiledMenu(t)
	rows := m.interactiveRows()
	pressKey(m, widget.KeyEnd)
	if got := m.v.sel[areaList]; got != prefRow+rows[len(rows)-1].key {
		t.Errorf("End выбрал %q, ждали последнюю строку", got)
	}
	if m.v.listScroll == 0 {
		t.Error("End не прокрутил список к выбранной строке")
	}
	pressKey(m, widget.KeyHome)
	if got := m.v.sel[areaList]; got != prefRow+rows[0].key || m.v.listScroll != 0 {
		t.Errorf("Home выбрал %q, прокрутка %d", got, m.v.listScroll)
	}
	pressKey(m, widget.KeyPageDown)
	if m.v.sel[areaList] == prefRow+rows[0].key {
		t.Error("PageDown не сдвинул выбор")
	}
}

// Стрелки в плитках идут по геометрии сетки, а не по порядку в списке.
func TestTiles_ArrowsFollowGridGeometry(t *testing.T) {
	m, cat := tiledMenu(t)
	m.OnKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true}) // → плитки
	sel := func() string { return m.v.sel[areaTiles] }
	if sel() != prefTile+"t0" {
		t.Fatalf("первый выбор %q, ждали t0", sel())
	}
	pressKey(m, widget.KeyRight)
	if sel() != prefTile+"t1" {
		t.Errorf("Вправо: %q, ждали t1", sel())
	}
	pressKey(m, widget.KeyDown)
	if sel() != prefTile+"t4" {
		t.Errorf("Вниз из t1: %q, ждали t4 (тот же столбец)", sel())
	}
	pressKey(m, widget.KeyLeft)
	if sel() != prefTile+"t3" {
		t.Errorf("Влево из t4: %q, ждали t3", sel())
	}
	pressKey(m, widget.KeyDown)
	if sel() != prefTile+"w0" {
		t.Errorf("Вниз из последней строки группы: %q, ждали w0 следующей группы", sel())
	}
	pressKey(m, widget.KeyUp)
	pressKey(m, widget.KeyUp)
	pressKey(m, widget.KeyUp)
	if sel() != prefTile+"t0" {
		t.Errorf("Вверх до упора: %q, ждали t0", sel())
	}
	pressKey(m, widget.KeyUp) // выше некуда — остаётся на месте
	if sel() != prefTile+"t0" {
		t.Errorf("Вверх с первой строки сдвинул выбор: %q", sel())
	}
	pressKey(m, widget.KeyEnd)
	if sel() != prefTile+"s1" {
		t.Errorf("End: %q, ждали последнюю плитку s1", sel())
	}
	pressKey(m, widget.KeyHome)
	m.OnKeyEvent(widget.KeyEvent{Code: widget.KeySpace, Pressed: true})
	if len(cat.Launched) != 1 || cat.Launched[0] != "t0" {
		t.Errorf("Space запустил %v, ждали t0", cat.Launched)
	}
}

func TestTiles_SidebarKeys(t *testing.T) {
	m, _ := tiledMenu(t)
	var got []string
	m.OnSidebarActivate = func(id string) { got = append(got, id) }
	m.OnKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true, Mod: widget.ModShift}) // → панель
	if m.v.area != areaSidebar || m.v.sel[areaSidebar] != keySideMenu {
		t.Fatalf("область %v выбор %q, ждали панель и гамбургер", m.v.area, m.v.sel[areaSidebar])
	}
	pressKey(m, widget.KeyEnter)
	if !m.SidebarExpanded() {
		t.Error("Enter на гамбургере не развернул панель")
	}
	pressKey(m, widget.KeyDown)
	pressKey(m, widget.KeyDown)
	m.OnKeyEvent(widget.KeyEvent{Code: widget.KeySpace, Pressed: true})
	if len(got) != 1 || got[0] != "docs" {
		t.Errorf("активирован %v, ждали docs", got)
	}
}

func TestTiles_EscapeCloses(t *testing.T) {
	m, _ := tiledMenu(t)
	pressKey(m, widget.KeyEscape)
	if m.IsOpen() {
		t.Error("Esc не закрыл меню")
	}
}

// После повторного открытия состояние сброшено: ничего не выбрано, панель
// свёрнута, списки сверху.
func TestTiles_ReopenResetsState(t *testing.T) {
	m, _ := tiledMenu(t)
	m.SetSidebarExpanded(true)
	finishAnimations()
	pressKey(m, widget.KeyEnd)
	m.Close()
	m.Settle()
	m.Open(tileAnchor())
	m.Settle()
	if m.SidebarExpanded() || m.v.listScroll != 0 || len(m.v.sel) != 0 || m.v.hover != "" {
		t.Errorf("состояние не сброшено: expanded=%v scroll=%d sel=%v hover=%q",
			m.SidebarExpanded(), m.v.listScroll, m.v.sel, m.v.hover)
	}
	if w := m.startGeometry(m.contentRect()).sideW; w != 48 {
		t.Errorf("панель после переоткрытия %d, ждали 48", w)
	}
}

// ─── Поиск ───────────────────────────────────────────────────────────────────

func searchMenu(t *testing.T) (*StartMenu, *FakeSearchProvider, *StaticAppCatalog) {
	t.Helper()
	m, cat := tiledMenu(t)
	p := NewFakeSearchProvider(
		SearchResult{ID: "edge", Title: "Microsoft Edge", Category: "Приложения"},
		SearchResult{ID: "paint", Title: "Paint 3D", Category: "Приложения"},
		SearchResult{ID: "pic", Title: "Edge.png", Category: "Файлы"},
	)
	m.SetSearchProvider(p)
	return m, p, cat
}

// Набор буквы на открытом меню — переход в поиск: список заменяется
// результатами, запрос уходит поставщику.
func TestTiles_TypingStartsSearchAndShowsResultsInsteadOfList(t *testing.T) {
	m, p, _ := searchMenu(t)
	m.OnKeyEvent(widget.KeyEvent{Code: widget.KeyE, Rune: 'e', Pressed: true})
	m.OnKeyEvent(widget.KeyEvent{Code: widget.KeyD, Rune: 'd', Pressed: true})
	if m.Query() != "ed" {
		t.Fatalf("запрос %q, ждали «ed»", m.Query())
	}
	if n := len(p.Queries); n == 0 || p.Queries[n-1] != "ed" {
		t.Errorf("поставщик получил запросы %v, ждали последний «ed»", p.Queries)
	}
	rows, _ := m.listRows()
	var results, apps int
	for _, r := range rows {
		switch r.kind {
		case rowResult:
			results++
		case rowApp, rowLetter:
			apps++
		}
	}
	if results != 2 || apps != 0 {
		t.Errorf("в списке %d результатов и %d строк каталога, ждали 2 и 0", results, apps)
	}
	ctx := &recCtx{}
	m.drawContent(ctx, m.contentRect())
	if !containsText(ctx.texts, "Microsoft Edge") || containsPrefixText(ctx.texts, "A приложение") {
		t.Errorf("кадр показывает не результаты: %v", ctx.texts)
	}

	m.OnKeyEvent(widget.KeyEvent{Code: widget.KeyBackspace, Pressed: true})
	m.OnKeyEvent(widget.KeyEvent{Code: widget.KeyBackspace, Pressed: true})
	if m.Query() != "" {
		t.Errorf("после стирания запрос %q", m.Query())
	}
	if rows, _ = m.listRows(); rows[0].kind == rowResult || rows[0].kind == rowEmpty {
		t.Error("пустой запрос не вернул список приложений")
	}
}

func TestTiles_SearchResultsNavigateAndActivate(t *testing.T) {
	m, p, _ := searchMenu(t)
	m.SetQuery("edge")
	m.SearchKey(widget.KeyEvent{Code: widget.KeyDown, Pressed: true})
	m.SearchKey(widget.KeyEvent{Code: widget.KeyDown, Pressed: true})
	if !m.SearchKey(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true}) {
		t.Fatal("Enter не использован")
	}
	if len(p.Activated) != 1 || p.Activated[0] != "pic" {
		t.Errorf("открыто %v, ждали второй результат pic", p.Activated)
	}
	if m.IsOpen() {
		t.Error("меню осталось открытым после открытия результата")
	}
	if m.Query() != "" {
		t.Errorf("запрос %q пережил закрытие меню", m.Query())
	}
}

func TestTiles_EnterWithoutSelectionOpensFirstResult(t *testing.T) {
	m, p, _ := searchMenu(t)
	m.SetQuery("paint")
	m.SearchKey(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if len(p.Activated) != 1 || p.Activated[0] != "paint" {
		t.Errorf("открыто %v, ждали paint", p.Activated)
	}
}

func TestTiles_NoResultsShowsMessage(t *testing.T) {
	m, _, _ := searchMenu(t)
	m.SetQuery("яяяя")
	rows, _ := m.listRows()
	if len(rows) != 1 || rows[0].kind != rowEmpty {
		t.Fatalf("строки %v, ждали одну «ничего не найдено»", rows)
	}
	ctx := &recCtx{}
	m.drawContent(ctx, m.contentRect())
	if !containsText(ctx.texts, tr(StrStartNoResults)) {
		t.Errorf("сообщения нет в кадре: %v", ctx.texts)
	}
}

// Результаты, пришедшие от поставщика позже запроса, подхватываются по
// подписке.
func TestTiles_AsyncResultsArriveBySubscription(t *testing.T) {
	m, p, _ := searchMenu(t)
	m.SetQuery("zz")
	if rows, _ := m.listRows(); rows[0].kind != rowEmpty {
		t.Fatalf("до прихода результатов ждали «ничего не найдено»")
	}
	p.mu.Lock()
	p.items = append(p.items, SearchResult{ID: "zzz", Title: "Zzz Tool"})
	p.mu.Unlock()
	p.SetQuery("zz") // поставщик оповестил подписчиков
	rows, _ := m.listRows()
	found := false
	for _, r := range rows {
		found = found || (r.kind == rowResult && r.result.ID == "zzz")
	}
	if !found {
		t.Errorf("поздний результат не показан: %v", rows)
	}
}

// ─── Подписки и язык ─────────────────────────────────────────────────────────

// Закрытое меню не держит подписок: смена данных его не будит.
func TestTiles_ClosedMenuDoesNotWakeFrame(t *testing.T) {
	m, _ := tiledMenu(t)
	src := NewFakeStartSource(nil, nil)
	m.SetSource(src)
	m.Close()
	m.Settle()
	var calls int
	h := widget.RegisterUINotifier(func() { calls++ }, func(image.Rectangle) { calls++ })
	defer widget.UnregisterUINotifier(h)
	src.Set(nil, []StartLetterGroup{{Letter: "A", Entries: []StartEntry{{ID: "x", Title: "X"}}}})
	m.Theme().SetAccent(theme.RGB(10, 200, 10))
	if calls != 0 {
		t.Errorf("закрытое меню вызвало %d перерисовок", calls)
	}
}

func TestTiles_LanguageSwitchOnOpenMenu(t *testing.T) {
	m, _ := tiledMenu(t)
	m.SetSource(NewFakeStartSource([]StartEntry{{ID: "n", Title: "N"}}, nil))
	m.SetSidebarExpanded(true)
	finishAnimations()
	useLanguage(t, "RU")
	ctx := &recCtx{}
	m.drawContent(ctx, m.contentRect())
	if !containsText(ctx.texts, "Недавно добавленные") {
		t.Fatalf("русский заголовок не нарисован: %v", ctx.texts)
	}
	widget.SetLanguage("EN")
	ctx = &recCtx{}
	m.drawContent(ctx, m.contentRect())
	if !containsText(ctx.texts, "Recently added") || !containsText(ctx.texts, "START") {
		t.Errorf("английские заголовки не нарисованы: %v", ctx.texts)
	}
}

// Ключи меню и поиска переведены на оба языка и подключаются к таблице
// потребителя через StartMenuAliases.
func TestLocale_StartKeysAndAliases(t *testing.T) {
	keys := []string{StrStartRecent, StrStartExpand, StrStartCollapse, StrStartNoResults, StrStartResults, StrSearchPlaceholder, StrSearchLabel}
	for _, lang := range []string{"RU", "EN"} {
		for _, k := range keys {
			if v, ok := widget.Translation(lang, k); !ok || v == "" {
				t.Errorf("нет перевода %s для %s", lang, k)
			}
		}
	}
	a := StartMenuAliases("Start", "RecentlyAdded", "SearchPlaceholder", "Expand", "")
	if a[StrStartRecent] != "RecentlyAdded" || a[StrStart] != "Start" || a[StrSearchPlaceholder] != "SearchPlaceholder" {
		t.Errorf("соответствие %v", a)
	}
	if _, ok := a[StrStartCollapse]; ok {
		t.Error("пустое имя должно пропускаться")
	}
}

// ─── Контекстное меню ────────────────────────────────────────────────────────

func TestTiles_RightClickBuildsContextMenu(t *testing.T) {
	m, _ := tiledMenu(t)
	var targets []StartTarget
	m.ContextMenu = func(tg StartTarget) []widget.MenuItem {
		targets = append(targets, tg)
		return []widget.MenuItem{{Text: "Открепить"}}
	}
	rows := m.interactiveRows()
	x, y := pointIn(m.keyRect(prefRow + rows[1].key))
	m.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseRight, Pressed: true})
	m.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseRight})
	if len(targets) != 1 || targets[0].Kind != StartTargetApp || targets[0].App != rows[1].app {
		t.Fatalf("цель %v, ждали приложение %q", targets, rows[1].app)
	}
	if p := m.v.popup; p == nil || !p.IsOpen() {
		t.Error("всплывающее меню не открыто")
	}
	if !m.IsOpen() {
		t.Error("правая кнопка закрыла «Пуск»")
	}
	// Esc сначала закрывает контекстное меню, а «Пуск» остаётся.
	pressKey(m, widget.KeyEscape)
	if m.v.popup.IsOpen() || !m.IsOpen() {
		t.Errorf("Esc: popup=%v open=%v, ждали закрытое контекстное меню и открытый «Пуск»", m.v.popup.IsOpen(), m.IsOpen())
	}

	// Плитка и пункт боковой панели.
	targets = nil
	x, y = pointIn(m.tileRectAbs("t2"))
	m.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseRight})
	x, y = pointIn(m.keyRect(prefSide + "docs"))
	m.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseRight})
	if len(targets) != 2 || targets[0].Kind != StartTargetTile || targets[0].Tile != "t2" ||
		targets[1].Kind != StartTargetSidebar || targets[1].Sidebar != "docs" {
		t.Errorf("цели %+v", targets)
	}
}

// ─── Наведение и перерисовка ─────────────────────────────────────────────────

func TestTiles_HoverInvalidatesOnlyItsRow(t *testing.T) {
	m, _ := tiledMenu(t)
	var rects []image.Rectangle
	fulls := 0
	h := widget.RegisterUINotifierWake(func() { fulls++ }, func(r image.Rectangle) { rects = append(rects, r) }, func() {})
	defer widget.UnregisterUINotifier(h)

	rows := m.interactiveRows()
	r0, r1 := m.keyRect(prefRow+rows[0].key), m.keyRect(prefRow+rows[1].key)
	x, y := pointIn(r0)
	m.OnMouseMove(x, y)
	x, y = pointIn(r1)
	m.OnMouseMove(x, y)
	if fulls != 0 {
		t.Errorf("наведение вызвало %d полных перерисовок", fulls)
	}
	menu := m.OverlayBounds()
	for _, r := range rects {
		if r.Dx()*r.Dy()*4 > menu.Dx()*menu.Dy() {
			t.Errorf("перерисована область %v — слишком большая для одной строки", r)
		}
	}
	if len(rects) == 0 {
		t.Error("наведение ничего не заявило")
	}
}

func TestTiles_OpenInvalidatesOnlyItsArea(t *testing.T) {
	defer widget.StopAllAnimations()
	m := NewStartMenu(managerFor(t, theme.ProfileWindows10), NewStaticAppCatalog(tileApps()...))
	m.Screen = tileScreen()
	m.SetTileGroups(tileGroups())
	var rects []image.Rectangle
	fulls := 0
	h := widget.RegisterUINotifierWake(func() { fulls++ }, func(r image.Rectangle) { rects = append(rects, r) }, func() {})
	defer widget.UnregisterUINotifier(h)

	m.Open(tileAnchor())
	m.Settle()
	if fulls != 0 {
		t.Errorf("открытие вызвало %d полных перерисовок", fulls)
	}
	area := m.OverlayBounds()
	for _, r := range rects {
		if !r.In(area.Inset(-60)) {
			t.Errorf("при открытии заявлена область %v вне меню %v", r, area)
		}
	}
	if len(rects) == 0 {
		t.Error("открытие ничего не заявило")
	}
}

// Шаги анимации ширины боковой панели перерисовывают только её область, а не
// меню целиком.
func TestTiles_SidebarAnimationInvalidatesOnlySidebarColumn(t *testing.T) {
	m, _ := tiledMenu(t)
	var rects []image.Rectangle
	h := widget.RegisterUINotifier(nil, func(r image.Rectangle) { rects = append(rects, r) })
	defer widget.UnregisterUINotifier(h)

	m.SetSidebarExpanded(true)
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(70 * time.Millisecond))
	if len(rects) == 0 {
		t.Fatal("анимация ничего не заявила")
	}
	c := m.contentRect()
	for _, r := range rects {
		if r.Dx() > 260 || r.Min.X < c.Min.X {
			t.Errorf("шаг анимации заявил %v — шире боковой панели (256)", r)
		}
	}
	widget.StepAnimations(t0.Add(time.Second))
}

// ─── Полоса прокрутки ────────────────────────────────────────────────────────

func TestTiles_ThinScrollbarAppearsOnWheelAndFades(t *testing.T) {
	old1, old2 := thinBarHold, thinBarFade
	thinBarHold, thinBarFade = 30*time.Millisecond, 20*time.Millisecond
	defer func() { thinBarHold, thinBarFade = old1, old2 }()

	m, _ := tiledMenu(t)
	if a := m.v.listBar.Alpha(); a != 0 {
		t.Fatalf("полоса в покое видна (alpha %v)", a)
	}
	g := m.startGeometry(m.contentRect())
	pt := image.Pt(g.list.Min.X+50, g.list.Min.Y+100)
	m.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseWheelDown, Pressed: true})
	if a := m.v.listBar.Alpha(); a != 1 {
		t.Errorf("после прокрутки alpha %v, ждали 1", a)
	}
	ctx := &recCtx{}
	m.drawContent(ctx, m.contentRect())
	thumb := false
	for _, f := range ctx.fills {
		if f.w == m.metricInt(KeyScrollThinWidth) && f.h > 20 {
			thumb = true
		}
	}
	if !thumb {
		t.Error("ползунок не нарисован")
	}

	time.Sleep(60 * time.Millisecond)
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(time.Second))
	if a := m.v.listBar.Alpha(); a != 0 {
		t.Errorf("полоса не погасла: alpha %v", a)
	}
}
