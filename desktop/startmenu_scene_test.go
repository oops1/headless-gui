package desktop_test

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Сцена Windows 10 для проверки меню «Пуск» с плитками: панель задач со
// строкой поиска, обои, данные приложений, пункты боковой панели и плитки.
// Используется и тестами, и снимками (GOLDEN_OUT).

// roundIcon — значок-заглушка: цветной скруглённый квадрат со светлой серединой
// нужной стороны. Настоящие значки приходят от потребителя.
func roundIcon(c color.RGBA) func(size int) image.Image {
	return func(size int) image.Image {
		img := image.NewRGBA(image.Rect(0, 0, size, size))
		inset := size / 8
		core := image.Rect(size/3, size/3, size-size/3, size-size/3)
		light := color.RGBA{R: 200, G: 200, B: 200, A: 200} // предумноженная альфа
		for y := inset; y < size-inset; y++ {
			for x := inset; x < size-inset; x++ {
				p := image.Pt(x, y)
				if p.In(core) {
					img.SetRGBA(x, y, light)
				} else {
					img.SetRGBA(x, y, c)
				}
			}
		}
		return img
	}
}

var (
	cBlue   = color.RGBA{R: 0, G: 120, B: 215, A: 255}
	cGreen  = color.RGBA{R: 16, G: 150, B: 80, A: 255}
	cOrange = color.RGBA{R: 235, G: 120, B: 30, A: 255}
	cRed    = color.RGBA{R: 200, G: 50, B: 60, A: 255}
	cPurple = color.RGBA{R: 120, G: 70, B: 190, A: 255}
	cYellow = color.RGBA{R: 240, G: 190, B: 40, A: 255}
	cGray   = color.RGBA{R: 110, G: 120, B: 130, A: 255}
)

func entry(id, title, sub string, c color.RGBA) desktop.StartEntry {
	return desktop.StartEntry{ID: desktop.AppID(id), Title: title, Subtitle: sub, IconAt: roundIcon(c)}
}

// sampleStartSource — недавние и алфавитные группы в духе снимков Windows 10:
// «#», латиница, кириллица, папка и приложения со второй строкой.
func sampleStartSource() *desktop.FakeStartSource {
	recent := []desktop.StartEntry{entry("music", "Яндекс.Музыка", "", cRed)}
	zip := entry("7zip", "7-Zip", "", cYellow)
	zip.Folder = true
	zip.Children = []desktop.StartEntry{entry("7zfm", "7-Zip File Manager", "", cYellow), entry("7zhelp", "7-Zip Help", "", cGray)}
	cisco := entry("cisco", "Cisco", "", cYellow)
	cisco.Folder = true
	cisco.Children = []desktop.StartEntry{entry("anyconnect", "AnyConnect", "", cBlue)}
	groups := []desktop.StartLetterGroup{
		{Letter: "#", Entries: []desktop.StartEntry{zip}},
		{Letter: "C", Entries: []desktop.StartEntry{cisco, entry("cortana", "Cortana", "", cBlue)}},
		{Letter: "G", Entries: []desktop.StartEntry{entry("gamebar", "Game Bar", "Система", cGreen)}},
		{Letter: "M", Entries: []desktop.StartEntry{
			entry("m365", "Microsoft 365 Copilot", "", cPurple),
			entry("edge", "Microsoft Edge", "", cBlue),
			entry("store", "Microsoft Store", "Система", cBlue),
		}},
		{Letter: "O", Entries: []desktop.StartEntry{
			entry("onedrive", "OneDrive", "", cBlue),
			entry("onenote", "OneNote for Windows 10", "", cPurple),
		}},
		{Letter: "P", Entries: []desktop.StartEntry{entry("paint3d", "Paint 3D", "", cOrange)}},
		{Letter: "Я", Entries: []desktop.StartEntry{entry("yandex", "Яндекс Браузер", "", cRed)}},
	}
	return desktop.NewFakeStartSource(recent, groups)
}

func sampleTiles() []desktop.TileGroup {
	t := func(id, title string, size desktop.TileSize, c color.RGBA) desktop.Tile {
		return desktop.Tile{
			ID: desktop.TileID(id), App: desktop.AppID(id), Size: size,
			Content: desktop.TileContent{Title: title, IconAt: roundIcon(c)},
		}
	}
	mail := t("mail", "Почта", desktop.TileMedium, cBlue)
	mail.Content.Subtitle = "Поддержка Yahoo"
	mail.Content.Badge = "3"
	return []desktop.TileGroup{
		{ID: "prod", Title: "Производительность", Tiles: []desktop.Tile{
			t("m365", "Microsoft 365", desktop.TileMedium, cPurple),
			t("office", "Office", desktop.TileMedium, cOrange),
			mail,
			t("edge", "Microsoft Edge", desktop.TileMedium, cBlue),
			t("photos", "Фотографии", desktop.TileMedium, cGray),
			t("calc", "Калькулятор", desktop.TileMedium, cGray),
		}},
		{ID: "view", Title: "Просмотр", Tiles: []desktop.Tile{
			t("store", "Microsoft Store", desktop.TileMedium, cBlue),
			t("todo", "Microsoft To Do", desktop.TileMedium, cBlue),
			t("movies", "Кино и ТВ", desktop.TileMedium, cBlue),
			t("solitaire", "Solitaire & Casual", desktop.TileMedium, cGreen),
			t("fun", "Развлечения", desktop.TileMedium, cPurple),
		}},
		{ID: "misc", Title: "Разное", Tiles: []desktop.Tile{
			t("big", "Большая плитка", desktop.TileLarge, cOrange),
			t("s1", "", desktop.TileSmall, cRed),
			t("s2", "", desktop.TileSmall, cGreen),
		}},
	}
}

func sampleSidebar(icons bool) []desktop.StartSidebarItem {
	return []desktop.StartSidebarItem{
		{ID: "user", Title: "oops", Glyph: desktop.GlyphUser},
		{ID: "docs", Title: "Документы", Glyph: desktop.GlyphDocuments},
		{ID: "pics", Title: "Изображения", Glyph: desktop.GlyphPictures},
		{ID: "settings", Title: "Параметры", Glyph: desktop.GlyphSettings},
		{ID: "power", Title: "Выключение", Glyph: desktop.GlyphPower, KeepOpen: true},
	}
}

// win10Scene — собранный рабочий стол Windows 10.
type win10Scene struct {
	eng    *engine.Engine
	tm     *theme.Manager
	root   *widget.Panel
	bar    *desktop.Taskbar
	start  *desktop.StartButton
	search *desktop.SearchBox
	menu   *desktop.StartMenu
	cat    *desktop.StaticAppCatalog
	prov   *desktop.FakeSearchProvider
	w, h   int
}

// fontsDir — каталог шрифтов репозитория относительно пакета.
func registerFonts(t *testing.T, eng *engine.Engine) {
	t.Helper()
	if err := eng.RegisterFontFS(os.DirFS(filepath.Join("..", "assets", "fonts")), "."); err != nil {
		t.Logf("шрифты не зарегистрированы: %v", err)
	}
	eng.SetTextSubpixel(true)
}

func newWin10Scene(t *testing.T, w, h int, light bool, scale float64) *win10Scene {
	t.Helper()
	tm := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(tm); err != nil {
		t.Fatal(err)
	}
	if err := tm.SetTheme(theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	if light {
		tm.SetFlag(theme.KeyTaskbarLight, true)
	}
	tm.SetIconResolver(widget.BuiltinIcons())

	eng := engine.New(w, h, 30)
	if scale != 1 {
		eng.SetScale(scale)
	}
	registerFonts(t, eng)
	_ = eng.ApplyThemeProfile(tm)

	root := widget.NewPanel(color.RGBA{R: 0, G: 96, B: 180, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	// Обои: светлые окна логотипа на синем — на них видно размытие.
	for i, r := range []image.Rectangle{
		image.Rect(w*5/8, h/5, w*5/8+w/6, h/5+h/4),
		image.Rect(w*5/8+w/6+8, h/5-12, w*5/8+w/3+8, h/5+h/4-12),
		image.Rect(w*5/8, h/5+h/4+8, w*5/8+w/6, h/5+h/2+8),
		image.Rect(w*5/8+w/6+8, h/5+h/4-4, w*5/8+w/3+8, h/5+h/2-4),
	} {
		p := widget.NewPanel(color.RGBA{R: 40 + uint8(i*10), G: 160, B: 235, A: 255})
		p.ShowHeader = false
		p.SetBounds(r)
		root.AddChild(p)
	}

	icons := widget.BuiltinIcons()
	wm := desktop.NewFakeWindowModel(
		desktop.WindowInfo{ID: 1, AppID: "files", Title: "Проводник", Icon: icons.ResolveIcon(theme.IconRef{Name: "start"}, 24)},
		desktop.WindowInfo{ID: 2, AppID: "term", Title: "Терминал", Active: true, Icon: icons.ResolveIcon(theme.IconRef{Name: "network.ethernet"}, 24)},
	)
	status := desktop.NewFakeSystemStatus()
	clk := desktop.NewFakeClock(time.Date(2026, 10, 5, 7, 35, 0, 0, time.UTC))
	cat := desktop.NewStaticAppCatalog(
		desktop.AppInfo{ID: "edge", Title: "Microsoft Edge"},
		desktop.AppInfo{ID: "store", Title: "Microsoft Store"},
		desktop.AppInfo{ID: "calc", Title: "Калькулятор"},
	)

	bar := desktop.NewTaskbar(tm)
	start := desktop.NewStartButton(tm)
	prov := desktop.NewFakeSearchProvider(
		desktop.SearchResult{ID: "edge", Title: "Microsoft Edge", Subtitle: "Приложение", Category: "Приложения", IconAt: roundIcon(cBlue)},
		desktop.SearchResult{ID: "calc", Title: "Калькулятор", Subtitle: "Приложение", Category: "Приложения", IconAt: roundIcon(cGray)},
		desktop.SearchResult{ID: "report", Title: "Отчёт за квартал.docx", Subtitle: "Документы", Category: "Документы", IconAt: roundIcon(cPurple)},
	)
	search := desktop.NewSearchBox(tm, prov)
	bar.AddItem(desktop.SlotStart, start)
	bar.AddItem(desktop.SlotStart, search)
	bar.AddItem(desktop.SlotApps, desktop.NewRunningApplications(tm, wm))
	bar.AddItem(desktop.SlotTray, desktop.NewNetworkStatus(tm, status))
	bar.AddItem(desktop.SlotTray, desktop.NewVolumeStatus(tm, status))
	bar.AddItem(desktop.SlotTray, desktop.NewPowerStatus(tm, status))
	bar.AddItem(desktop.SlotTray, desktop.NewClock(tm, clk))
	barH := bar.Height()
	bar.SetBounds(image.Rect(0, h-barH, w, h))
	root.AddChild(bar)

	menu := desktop.NewStartMenu(tm, cat)
	menu.Screen = image.Rect(0, 0, w, h)
	menu.SetSource(sampleStartSource())
	menu.SetSidebarItems(sampleSidebar(false))
	menu.SetTileGroups(sampleTiles())
	search.Bind(menu, start.Bounds)
	start.OnClick = func() { menu.Toggle(start.Bounds()) }
	root.AddChild(menu)
	eng.SetRoot(root)

	sc := &win10Scene{eng: eng, tm: tm, root: root, bar: bar, start: start, search: search,
		menu: menu, cat: cat, prov: prov, w: w, h: h}
	t.Cleanup(func() { bar.Close(); widget.StopAllAnimations() })
	return sc
}

// open открывает меню от кнопки «Пуск» и доводит анимацию до конца.
func (s *win10Scene) open() {
	s.menu.Open(s.start.Bounds())
	s.menu.Settle()
}

func (s *win10Scene) frame() *image.RGBA { return s.eng.RenderOnce() }

// savePNG — общий для тестов пакета, см. edges_test.go.
