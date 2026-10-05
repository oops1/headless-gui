package desktop_test

import (
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Сцена Windows 11 для проверки меню «Пуск» с сеткой закреплённых: панель задач,
// обои, закреплённые, «Рекомендуем», пользователь, меню питания. Используется и
// тестами, и снимками (GOLDEN_OUT).

// win11Pinned — закреплённые в духе снимка Windows 11: n приложений.
func win11Pinned(n int) []desktop.StartPinned {
	names := []string{"Edge", "Word", "PowerPoint", "OneNote", "Mail", "To Do", "Store", "Photos", "Phone",
		"Snipping Tool", "Spotify", "Settings", "Xbox", "Calculator", "Paint", "Teams", "Explorer", "Clock",
		"Excel", "Outlook", "Terminal", "Notepad", "Maps", "Weather", "Camera", "Clipchamp", "Solitaire", "Movies",
		"Calendar", "Alarms", "Journal", "Whiteboard", "Sticky Notes", "Voice"}
	cols := []color.RGBA{cBlue, cPurple, cOrange, cPurple, cBlue, cBlue, cBlue, cGray, cBlue, cGray, cGreen, cGray,
		cGreen, cGray, cOrange, cPurple, cYellow, cGray}
	var out []desktop.StartPinned
	for i := 0; i < n; i++ {
		name := names[i%len(names)]
		if i >= len(names) {
			name = fmt.Sprintf("%s %d", name, i/len(names)+1)
		}
		out = append(out, desktop.StartPinned{
			ID: fmt.Sprintf("app%d", i), App: desktop.AppID(fmt.Sprintf("app%d", i)),
			Title: name, IconAt: roundIcon(cols[i%len(cols)]),
		})
	}
	return out
}

func win11Recommended() []desktop.StartRecommendedItem {
	r := func(id, title, sub string, c color.RGBA) desktop.StartRecommendedItem {
		return desktop.StartRecommendedItem{ID: id, Title: title, Subtitle: sub, IconAt: roundIcon(c)}
	}
	return []desktop.StartRecommendedItem{
		r("r1", "Adobe Photoshop", "Недавно добавлено", cGray),
		r("r2", "Ежемесячные расходы 2021", "17 мин назад", cGreen),
		r("r3", "Ремонт и дизайн дома", "2 ч назад", cGray),
		r("r4", "Инструкции по пожертвованиям", "12 ч назад", cGray),
		r("r5", "Zero-Waste Chef", "Вчера", cGray),
		r("r6", "Договор аренды", "Вчера", cPurple),
		r("r7", "Отчёт за квартал", "Пн", cPurple),
		r("r8", "План проекта", "Вт", cBlue),
	}
}

// win11Scene — собранный рабочий стол Windows 11.
type win11Scene struct {
	eng   *engine.Engine
	tm    *theme.Manager
	root  *widget.Panel
	bar   *desktop.Taskbar
	start *desktop.StartButton
	menu  *desktop.StartMenu
	cat   *desktop.StaticAppCatalog
	prov  *desktop.FakeSearchProvider
	rec   *desktop.FakeStartRecommended
	w, h  int
}

func newWin11Scene(t *testing.T, w, h int, dark bool, scale float64) *win11Scene {
	t.Helper()
	tm := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(tm); err != nil {
		t.Fatal(err)
	}
	profile := theme.ProfileWindows11
	if dark {
		profile = theme.ProfileWindows11Dark
	}
	if err := tm.SetTheme(profile); err != nil {
		t.Fatal(err)
	}
	tm.SetIconResolver(widget.BuiltinIcons())

	eng := engine.New(w, h, 30)
	if scale != 1 {
		eng.SetScale(scale)
	}
	registerFonts(t, eng)
	_ = eng.ApplyThemeProfile(tm)

	bg := color.RGBA{R: 205, G: 224, B: 245, A: 255}
	blob := color.RGBA{R: 40, G: 120, B: 240, A: 255}
	if dark {
		bg = color.RGBA{R: 26, G: 28, B: 38, A: 255}
	}
	root := widget.NewPanel(bg)
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	for i, r := range []image.Rectangle{
		image.Rect(w/8, h/12, w*5/8, h*2/3),
		image.Rect(w*3/8, h/4, w*7/8, h*5/6),
	} {
		p := widget.NewPanel(color.RGBA{R: blob.R, G: blob.G - uint8(i*20), B: blob.B, A: 255})
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
	)

	bar := desktop.NewTaskbar(tm)
	start := desktop.NewStartButton(tm)
	prov := desktop.NewFakeSearchProvider(
		desktop.SearchResult{ID: "edge", Title: "Microsoft Edge", Subtitle: "Приложение", Category: "Приложения", IconAt: roundIcon(cBlue)},
		desktop.SearchResult{ID: "calc", Title: "Калькулятор", Subtitle: "Приложение", Category: "Приложения", IconAt: roundIcon(cGray)},
		desktop.SearchResult{ID: "report", Title: "Отчёт за квартал.docx", Subtitle: "Документы", Category: "Документы", IconAt: roundIcon(cPurple)},
	)
	bar.AddItem(desktop.SlotStart, start)
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
	menu.SetSearchProvider(prov)
	menu.SetPinned(win11Pinned(18))
	rec := desktop.NewFakeStartRecommended(win11Recommended()...)
	menu.SetRecommended(rec)
	menu.SetUser(desktop.StartUser{Name: "oops"})
	menu.PowerMenu = func() []widget.MenuItem {
		return []widget.MenuItem{{Text: "Заблокировать"}, {Text: "Отключиться"}, {Text: "Выйти"}}
	}
	start.OnClick = func() { menu.Toggle(start.Bounds()) }
	root.AddChild(menu)
	eng.SetRoot(root)

	sc := &win11Scene{eng: eng, tm: tm, root: root, bar: bar, start: start, menu: menu, cat: cat,
		prov: prov, rec: rec, w: w, h: h}
	t.Cleanup(func() { bar.Close(); widget.StopAllAnimations() })
	return sc
}

// open открывает меню от кнопки «Пуск» и доводит анимацию до конца.
func (s *win11Scene) open() {
	if s.menu.IsOpen() { // открытое меню закрывается: каждое открытие начинается заново
		s.menu.Close()
		s.menu.Settle()
	}
	s.menu.Open(s.start.Bounds())
	s.menu.Settle()
}

func (s *win11Scene) frame() *image.RGBA { return s.eng.RenderOnce() }

// key отправляет нажатие клавиши меню.
func (s *win11Scene) key(code widget.KeyCode, mods ...widget.KeyMod) {
	var mod widget.KeyMod
	for _, m := range mods {
		mod |= m
	}
	s.menu.OnKeyEvent(widget.KeyEvent{Code: code, Pressed: true, Mod: mod})
}

// typ набирает текст в меню (каждая руна — отдельным нажатием).
func (s *win11Scene) typ(text string) {
	for _, r := range text {
		s.menu.OnKeyEvent(widget.KeyEvent{Rune: r, Pressed: true})
	}
}

// click — нажатие и отпускание левой кнопки в точке.
func (s *win11Scene) click(p image.Point) {
	s.menu.OnMouseMove(p.X, p.Y)
	s.menu.OnMouseButton(widget.MouseEvent{X: p.X, Y: p.Y, Button: widget.MouseLeft, Pressed: true})
	s.menu.OnMouseButton(widget.MouseEvent{X: p.X, Y: p.Y, Button: widget.MouseLeft})
}

// rect — прямоугольник открытой панели.
func (s *win11Scene) rect() image.Rectangle { return s.menu.OverlayBounds() }
