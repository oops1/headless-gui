package desktop_test

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Панель задач у любого края экрана и на каждом мониторе.
//
// Проверяется то, что обещает раздел «Края и мониторы»: слоты лежат столбцом у
// бокового края, всплывающие панели раскрываются от своего края и не выходят за
// свой монитор, автоскрытие работает у боковых краёв, а по панели на экран
// собирает ScreenBars.

// edgeShell — панель задач со всем содержимым и всплывающими панелями.
type edgeShell struct {
	bar    *desktop.Taskbar
	start  *desktop.StartButton
	apps   *desktop.ApplicationArea
	tray   *desktop.SystemTray
	clock  *desktop.ClockItem
	menu   *desktop.StartMenu
	cal    *desktop.CalendarFlyout
	center *desktop.NotificationCenter
	quick  *desktop.QuickSettings
}

func (s *edgeShell) shell() *desktop.MonitorShell {
	return &desktop.MonitorShell{Bar: s.bar, Flyouts: map[string]desktop.FlyoutPanel{
		"start": s.menu, "calendar": s.cal, "notifications": s.center, "quick": s.quick,
		"overflow": s.tray.Overflow(),
	}}
}

func newEdgeShell(tm *theme.Manager, edge desktop.Edge) *edgeShell {
	icons := widget.BuiltinIcons()
	ico := func(name string) image.Image { return icons.ResolveIcon(theme.IconRef{Name: name}, 24) }
	apps := []desktop.AppInfo{
		{ID: "term", Title: "Терминал", Icon: ico("network.ethernet")},
		{ID: "files", Title: "Проводник", Icon: ico("start")},
		{ID: "mail", Title: "Почта", Icon: ico("battery")},
	}
	cat := desktop.NewStaticAppCatalog(apps...)
	cat.Pin("term")
	cat.Pin("files")
	wm := desktop.NewFakeWindowModel(
		desktop.WindowInfo{ID: 1, AppID: "files", Title: "Проводник", Active: true},
		desktop.WindowInfo{ID: 2, AppID: "mail", Title: "Почта", Icon: ico("battery")},
	)
	status := desktop.NewFakeSystemStatus()
	clk := desktop.NewFakeClock(time.Date(2026, 3, 14, 15, 9, 26, 0, time.UTC))
	notes := desktop.NewFakeNotifications()
	notes.Add(desktop.Notification{Title: "Обновление", Body: "Готово к установке", Time: clk.Now()})

	s := &edgeShell{
		bar:    desktop.NewTaskbar(tm),
		start:  desktop.NewStartButton(tm),
		apps:   desktop.NewApplicationArea(tm, cat, wm),
		tray:   desktop.NewSystemTray(tm),
		clock:  desktop.NewClock(tm, clk),
		menu:   desktop.NewStartMenu(tm, cat),
		cal:    desktop.NewCalendarFlyout(tm, clk),
		center: desktop.NewNotificationCenter(tm, notes),
		quick:  desktop.NewQuickSettings(tm, status),
	}
	s.tray.AddItem(desktop.NewNetworkStatus(tm, status))
	s.tray.AddItem(desktop.NewVolumeStatus(tm, status))
	s.tray.AddItem(desktop.NewPowerStatus(tm, status))
	s.bar.SetEdge(edge)
	s.bar.AddItem(desktop.SlotStart, s.start)
	s.bar.AddItem(desktop.SlotApps, s.apps)
	s.bar.AddItem(desktop.SlotTray, s.tray)
	s.bar.AddItem(desktop.SlotTray, s.clock)
	return s
}

// wallpaper — крупная клетка: стеклянные панели должны что-то размывать, а
// кадр не должен тянуть тысячи виджетов.
func wallpaper(w, h int) *widget.Panel {
	root := widget.NewPanel(color.RGBA{R: 30, G: 60, B: 110, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	const cell = 60
	for y := 0; y < h; y += cell {
		for x := 0; x < w; x += cell {
			shade := uint8(70)
			if (x/cell+y/cell)%2 == 0 {
				shade = 170
			}
			p := widget.NewPanel(color.RGBA{R: shade, G: uint8(90 + int(shade)/4), B: 180, A: 255})
			p.ShowHeader = false
			p.SetBounds(image.Rect(x, y, x+cell, y+cell))
			root.AddChild(p)
		}
	}
	return root
}

func edgeManager(t *testing.T, profile string) (*theme.Manager, *engine.Engine) {
	t.Helper()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	eng := engine.New(1, 1, 30)
	if err := eng.SetThemeProfile(m, profile); err != nil {
		t.Fatal(err)
	}
	m.SetIconResolver(widget.BuiltinIcons())
	t.Cleanup(func() { widget.SetDefaultFontSize(0) })
	return m, eng
}

func savePNG(t *testing.T, img *image.RGBA, name string) {
	t.Helper()
	dir := os.Getenv("GOLDEN_OUT")
	if dir == "" || img == nil {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// oneScreen собирает сцену с единственным монитором и панелью у края edge.
func oneScreen(t *testing.T, profile string, edge desktop.Edge, w, h int) (*engine.Engine, *desktop.ScreenBars, *edgeShell) {
	t.Helper()
	tm, _ := edgeManager(t, profile)
	eng := engine.New(w, h, 30)
	if err := eng.SetThemeProfile(tm, profile); err != nil {
		t.Fatal(err)
	}
	mon := desktop.Monitor{ID: "m0", Bounds: image.Rect(0, 0, w, h), Primary: true}
	var shell *edgeShell
	bars := desktop.NewScreenBars(desktop.NewFakeScreens(mon), func(m desktop.Monitor) *desktop.MonitorShell {
		shell = newEdgeShell(tm, edge)
		return shell.shell()
	})
	t.Cleanup(bars.Close)
	root := wallpaper(w, h)
	root.AddChild(bars)
	eng.SetRoot(root)
	return eng, bars, shell
}

func TestEdges_VerticalLayout(t *testing.T) {
	for _, edge := range []desktop.Edge{desktop.EdgeLeft, desktop.EdgeRight} {
		edge := edge
		t.Run(map[desktop.Edge]string{desktop.EdgeLeft: "left", desktop.EdgeRight: "right"}[edge], func(t *testing.T) {
			const w, h = 900, 600
			_, bars, s := oneScreen(t, theme.ProfileWindows10, edge, w, h)
			b := s.bar.Bounds()
			if th := s.bar.Thickness(); th <= 0 || b.Dx() != th || b.Dy() != h {
				t.Fatalf("панель %v: ждали столбец толщиной %d на всю высоту %d", b, th, h)
			}
			if edge == desktop.EdgeLeft && b.Min.X != 0 || edge == desktop.EdgeRight && b.Max.X != w {
				t.Errorf("панель %v не у своего края", b)
			}
			if !s.bar.Vertical() || s.bar.Edge() != edge {
				t.Errorf("Edge/Vertical не те: %v %v", s.bar.Edge(), s.bar.Vertical())
			}

			// Порядок сверху вниз: «Пуск», приложения, трей, часы; все внутри
			// панели и не налезают друг на друга.
			parts := []image.Rectangle{s.start.Bounds(), s.apps.Bounds(), s.tray.Bounds(), s.clock.Bounds()}
			names := []string{"Пуск", "приложения", "трей", "часы"}
			for i, r := range parts {
				if r.Empty() {
					t.Errorf("%s: нет места", names[i])
					continue
				}
				if !r.In(b) {
					t.Errorf("%s %v вышло за панель %v", names[i], r, b)
				}
				if i > 0 && !parts[i-1].Empty() && r.Min.Y < parts[i-1].Max.Y {
					t.Errorf("%s %v налезает на предыдущее %v", names[i], r, parts[i-1])
				}
			}
			// Приложения — по одной ячейке в ряд, друг под другом.
			if got := len(s.apps.Cells()); got < 2 {
				t.Fatalf("ячеек %d", got)
			}
			r0, r1 := s.apps.ButtonRect(0), s.apps.ButtonRect(1)
			if r0.Empty() || r1.Empty() || r1.Min.Y < r0.Max.Y || r1.Min.X != r0.Min.X {
				t.Errorf("ячейки не в столбец: %v %v", r0, r1)
			}
			// Рабочая область без панели.
			wa := s.bar.WorkArea()
			if wa.Dx() != w-b.Dx() || wa.Dy() != h {
				t.Errorf("рабочая область %v", wa)
			}
			_ = bars
		})
	}
}

// Окно раскрывается рядом с панелью, в сторону от неё, и остаётся на экране.
func TestEdges_FlyoutOpensAwayFromSideBar(t *testing.T) {
	const w, h = 900, 600
	for _, edge := range []desktop.Edge{desktop.EdgeLeft, desktop.EdgeRight, desktop.EdgeTop, desktop.EdgeBottom} {
		eng, bars, s := oneScreen(t, theme.ProfileWindows10, edge, w, h)
		_ = eng
		bar := s.bar.Bounds()
		for name, anchor := range map[string]image.Rectangle{
			"start": s.start.Bounds(), "calendar": s.clock.Bounds(),
		} {
			if !bars.Toggle("m0", name, anchor) {
				t.Fatalf("%v/%s: не открылась", edge, name)
			}
			p := bars.Flyout("m0", name).AsFlyout()
			p.Settle()
			r := p.OverlayBounds()
			if r.Empty() || !r.In(image.Rect(0, 0, w, h)) {
				t.Errorf("%v/%s: окно %v вне экрана", edge, name, r)
				continue
			}
			if r.Overlaps(bar) {
				t.Errorf("%v/%s: окно %v налезает на панель %v", edge, name, r, bar)
			}
			switch edge {
			case desktop.EdgeLeft:
				if r.Min.X < bar.Max.X {
					t.Errorf("left/%s: окно %v левее края панели %v", name, r, bar)
				}
			case desktop.EdgeRight:
				if r.Max.X > bar.Min.X {
					t.Errorf("right/%s: окно %v правее края панели %v", name, r, bar)
				}
			case desktop.EdgeTop:
				if r.Min.Y < bar.Max.Y {
					t.Errorf("top/%s: окно %v выше нижнего края панели %v", name, r, bar)
				}
			}
			bars.Flyouts().CloseAll()
		}
	}
}

func TestEdges_AutoHideSideBar(t *testing.T) {
	const w, h = 900, 600
	for _, edge := range []desktop.Edge{desktop.EdgeLeft, desktop.EdgeRight} {
		_, _, s := oneScreen(t, theme.ProfileWindows10, edge, w, h)
		full := s.bar.Bounds()
		s.bar.SetAutoHide(true)
		hidden := s.bar.Bounds()
		if !s.bar.ReservedArea().Empty() {
			t.Errorf("%v: автоскрытая панель занимает место", edge)
		}
		switch edge {
		case desktop.EdgeLeft:
			if hidden.Max.X != 0 {
				t.Errorf("left: скрытая панель %v не ушла за левый край", hidden)
			}
		default:
			if hidden.Min.X != w {
				t.Errorf("right: скрытая панель %v не ушла за правый край", hidden)
			}
		}
		// Курсор у самого края выдвигает панель.
		x := 0
		if edge == desktop.EdgeRight {
			x = w - 1
		}
		s.bar.OnMouseMove(x, h/2)
		for i := 0; i < 80 && widget.AnimationsActive(); i++ {
			widget.StepAnimations(time.Now().Add(time.Duration(i+1) * 20 * time.Millisecond))
		}
		if !s.bar.IsRevealed() || s.bar.Bounds() != full {
			t.Errorf("%v: панель не выехала: %v, ждали %v", edge, s.bar.Bounds(), full)
		}
		// Курсор ушёл — панель убирается.
		s.bar.OnMouseMove(w/2, h/2)
		if s.bar.IsRevealed() {
			t.Errorf("%v: панель не убралась", edge)
		}
		s.bar.SetAutoHide(false)
		if s.bar.Bounds() != full {
			t.Errorf("%v: после отключения автоскрытия панель %v, ждали %v", edge, s.bar.Bounds(), full)
		}
	}
}

// Смена края на лету переставляет панель и привязанные окна без пересоздания.
func TestEdges_SetEdgeMovesBarAndFlyouts(t *testing.T) {
	const w, h = 900, 600
	_, _, s := oneScreen(t, theme.ProfileWindows10, desktop.EdgeBottom, w, h)
	start := s.start
	if s.bar.Bounds().Dy() != s.bar.Thickness() {
		t.Fatalf("исходная панель %v", s.bar.Bounds())
	}
	s.bar.SetEdge(desktop.EdgeLeft)
	if s.bar.Bounds().Dx() != s.bar.Thickness() || s.bar.Bounds().Dy() != h {
		t.Errorf("после SetEdge(Left) панель %v", s.bar.Bounds())
	}
	if s.menu.AsFlyout().Edge != desktop.EdgeLeft {
		t.Errorf("окно «Пуск» не получило край: %v", s.menu.AsFlyout().Edge)
	}
	if got := s.menu.AsFlyout().Screen; got != image.Rect(0, 0, w, h) {
		t.Errorf("экран окна %v", got)
	}
	s.bar.ResetEdge()
	if s.bar.Edge() != desktop.EdgeBottom || s.bar.Bounds().Dy() != s.bar.Thickness() || s.bar.Bounds().Dx() != w {
		t.Errorf("после ResetEdge панель %v край %v", s.bar.Bounds(), s.bar.Edge())
	}
	if s.start != start {
		t.Error("компоненты пересозданы")
	}
}

// Горизонтальная раскладка прежняя: слоты в ряд на всю ширину.
func TestEdges_HorizontalLayoutKept(t *testing.T) {
	const w, h = 900, 600
	_, _, s := oneScreen(t, theme.ProfileWindows10, desktop.EdgeBottom, w, h)
	b := s.bar.Bounds()
	if b != image.Rect(0, h-s.bar.Height(), w, h) {
		t.Fatalf("панель %v", b)
	}
	if !(s.start.Bounds().Max.X <= s.apps.Bounds().Min.X && s.apps.Bounds().Max.X <= s.tray.Bounds().Min.X &&
		s.tray.Bounds().Max.X <= s.clock.Bounds().Min.X) {
		t.Errorf("слоты не слева направо: %v %v %v %v", s.start.Bounds(), s.apps.Bounds(), s.tray.Bounds(), s.clock.Bounds())
	}
}

// ─── Мониторы ───────────────────────────────────────────────────────────────

func twoMonitors() (desktop.Monitor, desktop.Monitor) {
	return desktop.Monitor{ID: "left", Name: "Левый", Bounds: image.Rect(0, 0, 800, 600), Primary: true},
		desktop.Monitor{ID: "right", Name: "Правый", Bounds: image.Rect(800, 0, 1600, 500)}
}

func TestScreens_BarPerMonitorWithOwnFlyouts(t *testing.T) {
	m1, m2 := twoMonitors()
	tm, _ := edgeManager(t, theme.ProfileWindows10)
	eng := engine.New(1600, 600, 30)
	if err := eng.SetThemeProfile(tm, theme.ProfileWindows10); err != nil {
		t.Fatal(err)
	}
	screens := desktop.NewFakeScreens(m1, m2)
	shells := map[string]*edgeShell{}
	bars := desktop.NewScreenBars(screens, func(m desktop.Monitor) *desktop.MonitorShell {
		s := newEdgeShell(tm, desktop.EdgeBottom)
		shells[m.ID] = s
		return s.shell()
	})
	defer bars.Close()
	root := wallpaper(1600, 600)
	root.AddChild(bars)
	eng.SetRoot(root)

	if got := len(bars.Bars()); got != 2 {
		t.Fatalf("панелей %d, ждали 2", got)
	}
	for _, m := range []desktop.Monitor{m1, m2} {
		b := shells[m.ID].bar.Bounds()
		want := image.Rect(m.Bounds.Min.X, m.Bounds.Max.Y-shells[m.ID].bar.Height(), m.Bounds.Max.X, m.Bounds.Max.Y)
		if b != want {
			t.Errorf("%s: панель %v, ждали %v", m.ID, b, want)
		}
		if got := bars.BarAt(m.Bounds.Min.Add(image.Pt(5, 5))); got != shells[m.ID].bar {
			t.Errorf("%s: BarAt вернул не ту панель", m.ID)
		}
	}

	// «Пуск» второго монитора открывается на втором мониторе и не выходит из него.
	s2 := shells["right"]
	bars.Toggle("right", "start", s2.start.Bounds())
	s2.menu.Settle()
	r := s2.menu.OverlayBounds()
	if r.Empty() || !r.In(m2.Bounds) {
		t.Errorf("меню второго монитора %v вне %v", r, m2.Bounds)
	}
	// Открытие на втором мониторе закрывает календарь первого: слой общий.
	s1 := shells["left"]
	bars.Toggle("left", "calendar", s1.clock.Bounds())
	if s2.menu.IsOpen() {
		t.Error("панель второго монитора осталась открытой")
	}
	s1.cal.Settle()
	if r := s1.cal.OverlayBounds(); r.Empty() || !r.In(m1.Bounds) {
		t.Errorf("календарь первого монитора %v вне %v", r, m1.Bounds)
	}
	bars.Flyouts().CloseAll()

	// Центр уведомлений второго монитора — у правого края этого монитора.
	s2.center.PinToEdge(desktop.EdgeRight)
	s2.center.Margin = 0
	bars.Toggle("right", "notifications", s2.clock.Bounds())
	s2.center.Settle()
	cr := s2.center.OverlayBounds()
	if cr.Empty() || cr.Max.X != m2.Bounds.Max.X || !cr.In(m2.Bounds) {
		t.Errorf("центр уведомлений %v не у правого края монитора %v", cr, m2.Bounds)
	}
	if edge, ok := s2.center.AsFlyout().Pinned(); !ok || edge != desktop.EdgeRight {
		t.Errorf("Pinned = %v %v", edge, ok)
	}
	// Для снимка: календарь первого монитора и центр второго открыты вместе
	// (в слое они закрывают друг друга, поэтому открываем напрямую).
	bars.Flyouts().Group("left/calendar", "right/notifications")
	bars.Flyouts().CloseAll()
	s2.center.Open(s2.clock.Bounds())
	s1.cal.Open(s1.clock.Bounds())
	for _, p := range []*desktop.Flyout{s1.cal.AsFlyout(), s2.center.AsFlyout(), s2.menu.AsFlyout()} {
		p.Settle()
	}
	savePNG(t, eng.RenderOnce(), "edges_two_monitors")
	bars.Flyouts().CloseAll()
}

// Изменение набора мониторов: панель переезжает, новая появляется, пропавшая
// закрывается; компоненты остающихся не пересоздаются.
func TestScreens_SyncAddsMovesRemoves(t *testing.T) {
	m1, m2 := twoMonitors()
	tm, _ := edgeManager(t, theme.ProfileWindows11)
	screens := desktop.NewFakeScreens(m1)
	created := 0
	removed := ""
	bars := desktop.NewScreenBars(screens, func(m desktop.Monitor) *desktop.MonitorShell {
		created++
		return newEdgeShell(tm, desktop.EdgeBottom).shell()
	})
	defer bars.Close()
	bars.OnShellRemoved = func(id string, _ *desktop.MonitorShell) { removed = id }
	first := bars.Shell("left")
	if created != 1 || first == nil {
		t.Fatalf("создано %d", created)
	}

	screens.SetMonitors(m1, m2)
	if created != 2 || len(bars.Bars()) != 2 {
		t.Fatalf("после подключения: создано %d, панелей %d", created, len(bars.Bars()))
	}
	if bars.Shell("left") != first {
		t.Error("оболочка существующего монитора пересоздана")
	}

	// Сменилось разрешение первого.
	m1b := m1
	m1b.Bounds = image.Rect(0, 0, 1024, 768)
	screens.SetMonitors(m1b, m2)
	if got := first.Bar.Bounds(); got.Dx() != 1024 || got.Max.Y != 768 {
		t.Errorf("панель не переехала: %v", got)
	}

	screens.SetMonitors(m1b)
	if removed != "right" || len(bars.Bars()) != 1 || bars.Shell("right") != nil {
		t.Errorf("пропавший монитор: removed=%q панелей %d", removed, len(bars.Bars()))
	}
}

// Пока панель стоит на втором мониторе, окно прижимается к краю ЕГО области, а
// не общего экрана; панель справа вычитается из области.
func TestScreens_PinnedFlyoutUsesWorkAreaWithoutSideBar(t *testing.T) {
	m1, m2 := twoMonitors()
	tm, _ := edgeManager(t, theme.ProfileWindows10)
	var s2 *edgeShell
	bars := desktop.NewScreenBars(desktop.NewFakeScreens(m1, m2), func(m desktop.Monitor) *desktop.MonitorShell {
		s := newEdgeShell(tm, desktop.EdgeRight)
		if m.ID == "right" {
			s2 = s
		}
		return s.shell()
	})
	defer bars.Close()
	root := wallpaper(1600, 600)
	root.AddChild(bars)
	engine.New(1600, 600, 30).SetRoot(root)

	s2.center.PinToEdge(desktop.EdgeRight)
	s2.center.Margin = 0
	bars.Toggle("right", "notifications", s2.clock.Bounds())
	s2.center.Settle()
	cr := s2.center.OverlayBounds()
	bar := s2.bar.Bounds()
	if cr.Empty() || cr.Max.X != bar.Min.X {
		t.Errorf("центр %v должен упираться в панель %v с левой стороны", cr, bar)
	}
}

// Полный цикл: снимки панели у каждого края с открытым окном.
func TestEdges_Shots(t *testing.T) {
	if os.Getenv("GOLDEN_OUT") == "" {
		t.Skip("снимки делаются только при заданном GOLDEN_OUT")
	}
	const w, h = 900, 600
	for _, profile := range []string{theme.ProfileWindows10, theme.ProfileWindows11Dark, theme.ProfileWindows2000} {
		for _, c := range []struct {
			name string
			edge desktop.Edge
		}{{"left", desktop.EdgeLeft}, {"right", desktop.EdgeRight}, {"top", desktop.EdgeTop}, {"bottom", desktop.EdgeBottom}} {
			eng, bars, s := oneScreen(t, profile, c.edge, w, h)
			bars.Toggle("m0", "start", s.start.Bounds())
			s.menu.Settle()
			savePNG(t, eng.RenderOnce(), "edge_"+profile+"_"+c.name+"_start")
			bars.Flyouts().CloseAll()
			s.menu.Settle()
			bars.Toggle("m0", "calendar", s.clock.Bounds())
			s.cal.Settle()
			savePNG(t, eng.RenderOnce(), "edge_"+profile+"_"+c.name+"_calendar")
			bars.Flyouts().CloseAll()
			s.cal.Settle()
			// Центр уведомлений прижат к правому краю монитора, где бы ни стояла панель.
			s.center.PinToEdge(desktop.EdgeRight)
			s.center.Margin = 0
			s.center.Align = desktop.AlignEnd
			bars.Toggle("m0", "notifications", s.clock.Bounds())
			s.center.Settle()
			savePNG(t, eng.RenderOnce(), "edge_"+profile+"_"+c.name+"_notify")
		}
	}
}
