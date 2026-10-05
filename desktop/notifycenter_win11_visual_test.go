package desktop_test

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Снимки центра уведомлений и календаря Windows 11: светлый и тёмный, центр с
// группами и действиями, календарь развёрнутый и свёрнутый, «Фокусировка» с
// идущим отсчётом. Кадры сохраняются при NC_OUT (каталог) — смотреть глазами.

// nc11Opts — что менять в снимке.
type nc11Opts struct {
	dark      bool
	scale     float64
	w, h      int
	collapsed bool // календарь свёрнут
	noFocus   bool // без модуля «Фокусировка»
	running   bool // сеанс «Фокусировки» идёт
	dnd       bool // «Не беспокоить» включено
	empty     bool // без уведомлений
	extra     func(ns *desktop.FakeNotifications)
	after     func(s *nc11Scene)
}

// nc11Scene — рабочий стол с открытой парой «центр + календарь».
type nc11Scene struct {
	eng   *engine.Engine
	nc    *desktop.NotificationCenter
	cal   *desktop.CalendarFlyout
	group *desktop.FlyoutGroup
	ns    *desktop.FakeNotifications
	fs    *desktop.FakeFocusSession
	dnd   *desktop.DoNotDisturbState
	clock *desktop.FakeClock
	bar   *desktop.Taskbar
	tm    *theme.Manager
	w, h  int
}

func nc11Time() time.Time { return time.Date(2026, 10, 5, 7, 37, 0, 0, time.Local) }

// nc11Notes — пример: одиночные уведомления, группа из двух, действия.
func nc11Notes(clock time.Time) *desktop.FakeNotifications {
	ns := ncSampleNotes(clock)
	return ns
}

// newNC11Scene собирает стол Windows 11 с панелью задач и открывает группу.
func newNC11Scene(t *testing.T, o nc11Opts) *nc11Scene {
	t.Helper()
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	profile := theme.ProfileWindows11
	if o.dark {
		profile = theme.ProfileWindows11Dark
	}
	if err := m.SetTheme(profile); err != nil {
		t.Fatal(err)
	}
	m.SetIconResolver(widget.BuiltinIcons())

	w, h := o.w, o.h
	if w == 0 {
		w, h = 1280, 800
	}
	scale := o.scale
	if scale <= 0 {
		scale = 1
	}
	eng := engine.New(w, h, 30)
	registerFonts(t, eng)
	eng.SetScale(scale)
	if err := eng.SetBackground(matWallpaper(w, h)); err != nil {
		t.Fatal(err)
	}
	if err := eng.ApplyThemeProfile(m); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		m2 := theme.NewManager()
		_ = theme.RegisterBuiltinProfiles(m2)
		_ = m2.SetTheme(theme.ProfileWindows10)
		_ = eng.ApplyThemeProfile(m2)
		widget.StopAllAnimations()
	})

	root := widget.NewCanvas()
	root.SetBounds(image.Rect(0, 0, w, h))

	clock := nc11Time()
	fclk := desktop.NewFakeClock(clock)
	status := desktop.NewFakeSystemStatus()
	bar := desktop.NewTaskbar(m)
	bar.AddItem(desktop.SlotStart, desktop.NewStartButton(m))
	bar.AddItem(desktop.SlotTray, desktop.NewNetworkStatus(m, status))
	bar.AddItem(desktop.SlotTray, desktop.NewVolumeStatus(m, status))
	bar.AddItem(desktop.SlotTray, desktop.NewPowerStatus(m, status))
	ck := desktop.NewClock(m, fclk)
	bar.AddItem(desktop.SlotTray, ck)
	barH := bar.Height()
	bar.SetBounds(image.Rect(0, h-barH, w, h))
	root.AddChild(bar)
	t.Cleanup(bar.Close)

	ns := nc11Notes(clock)
	if o.empty {
		ns = desktop.NewFakeNotifications()
	}
	if o.extra != nil {
		o.extra(ns)
	}
	dnd := desktop.NewDoNotDisturb(o.dnd)
	nc := desktop.NewNotificationCenter(m, ns)
	nc.Clock = fclk
	nc.Culture = desktop.LocaleCulture{}
	nc.Screen = image.Rect(0, 0, w, h)
	nc.SetDoNotDisturb(dnd)
	cal := desktop.NewCalendarFlyout(m, fclk)
	cal.Culture = desktop.LocaleCulture{}
	cal.Screen = image.Rect(0, 0, w, h)
	fs := desktop.NewFakeFocusSession(fclk)
	if !o.noFocus {
		cal.SetFocusSession(fs)
	}
	if o.running {
		fs.Start()
	}
	cal.Ticker = func(time.Duration, func()) func() { return func() {} }
	cal.SetCollapsed(o.collapsed)
	group := desktop.LinkNotificationCenter(nc, cal)
	fm := desktop.NewFlyoutManager()
	fm.Register("calendar", cal)
	fm.Register("notifications", nc)
	root.AddChild(fm)
	t.Cleanup(nc.Close)
	t.Cleanup(cal.Close)

	eng.SetRoot(root)
	anchor := ck.Bounds()
	group.OpenAll(image.Rect(anchor.Min.X, bar.Bounds().Min.Y, anchor.Max.X, bar.Bounds().Max.Y))
	nc.Settle()
	cal.Settle()
	s := &nc11Scene{eng: eng, nc: nc, cal: cal, group: group, ns: ns, fs: fs, dnd: dnd, clock: fclk, bar: bar, tm: m, w: w, h: h}
	if o.after != nil {
		o.after(s)
	}
	return s
}

func (s *nc11Scene) render() *image.RGBA {
	s.eng.Invalidate()
	return s.eng.RenderOnce()
}

func (s *nc11Scene) click(x, y int) {
	// Событие идёт по движку, как от мыши: нажатие и отпускание.
	s.eng.SendMouseMove(x, y)
	s.eng.SendMouseButton(x, y, widget.MouseLeft, true)
	s.eng.SendMouseButton(x, y, widget.MouseLeft, false)
}

var _ = color.RGBA{}

func TestVisual_NotificationCenterWin11(t *testing.T) {
	cases := []struct {
		name string
		o    nc11Opts
	}{
		{"light", nc11Opts{}},
		{"dark", nc11Opts{dark: true}},
		{"light_collapsed", nc11Opts{collapsed: true}},
		{"dark_collapsed", nc11Opts{dark: true, collapsed: true}},
		{"light_running", nc11Opts{running: true}},
		{"dark_running", nc11Opts{dark: true, running: true}},
		{"light_dnd", nc11Opts{dnd: true}},
		{"dark_empty", nc11Opts{dark: true, empty: true}},
		{"light_200", nc11Opts{scale: 2, w: 960, h: 600}},
		{"dark_100_small", nc11Opts{dark: true, w: 800, h: 500}},
		{"dark_dnd", nc11Opts{dark: true, dnd: true}},
		{"light_running_half", nc11Opts{running: true, after: func(s *nc11Scene) {
			s.clock.Advance(15 * time.Minute)
		}}},
		{"dark_running_half", nc11Opts{dark: true, running: true, after: func(s *nc11Scene) {
			s.clock.Advance(20*time.Minute + 30*time.Second)
		}}},
		{"light_hover_card", nc11Opts{after: func(s *nc11Scene) {
			s.nc.OnMouseMove(1000, 130)
		}}},
		{"dark_hover_bell", nc11Opts{dark: true, after: func(s *nc11Scene) {
			s.nc.OnMouseMove(1137, 38)
		}}},
		{"light_focus_ring", nc11Opts{after: func(s *nc11Scene) {
			s.nc.SetFocused(true)
			s.nc.OnKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true})
			s.nc.OnKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true})
		}}},
		{"dark_focus_ring_calendar", nc11Opts{dark: true, after: func(s *nc11Scene) {
			s.cal.SetFocused(true)
			for i := 0; i < 6; i++ {
				s.cal.OnKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true})
			}
		}}},
		{"light_focus_ring_grid", nc11Opts{after: func(s *nc11Scene) {
			s.cal.SetFocused(true)
			for i := 0; i < 4; i++ {
				s.cal.OnKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true})
			}
			s.cal.OnKeyEvent(widget.KeyEvent{Code: widget.KeyRight, Pressed: true})
			s.cal.OnKeyEvent(widget.KeyEvent{Code: widget.KeyDown, Pressed: true})
		}}},
		{"dark_group_collapsed", nc11Opts{dark: true, after: func(s *nc11Scene) {
			s.nc.SetGroupCollapsed("cisco", true)
			finish := time.Now()
			widget.StepAnimations(finish)
			widget.StepAnimations(finish.Add(time.Hour))
		}}},
		{"light_severity", nc11Opts{extra: func(ns *desktop.FakeNotifications) {
			clock := nc11Time()
			ns.Add(desktop.Notification{AppID: "sys", AppName: "Система", Icon: ncGlyph(48, color.RGBA{R: 90, G: 90, B: 90, A: 255}, 2),
				Title: "Низкий заряд батареи", Body: "Осталось 9 %. Подключите питание.",
				Timestamp: clock.Add(-5 * time.Minute), Severity: desktop.SeverityWarning})
			ns.Add(desktop.Notification{AppID: "sys2", AppName: "Обновление", Icon: ncGlyph(48, color.RGBA{R: 90, G: 90, B: 90, A: 255}, 2),
				Title: "Ошибка обновления", Body: "Не удалось установить обновление 0x80070057.",
				Timestamp: clock.Add(-2 * time.Minute), Severity: desktop.SeverityError})
		}}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			s := newNC11Scene(t, c.o)
			img := s.render()
			if img == nil {
				t.Fatal("кадр не отрисован")
			}
			if s.nc.OverlayBounds().Empty() || s.cal.OverlayBounds().Empty() {
				t.Fatalf("панели не открылись: центр %v, календарь %v", s.nc.OverlayBounds(), s.cal.OverlayBounds())
			}
			ncSave(t, img, "nc_win11_"+c.name)
		})
	}
}

// Тост Windows 11: карточка со строкой приложения в правом нижнем углу.
func TestVisual_NotificationToastWin11(t *testing.T) {
	for _, dark := range []bool{false, true} {
		m := theme.NewManager()
		if err := theme.RegisterBuiltinProfiles(m); err != nil {
			t.Fatal(err)
		}
		profile, tag := theme.ProfileWindows11, "light"
		if dark {
			profile, tag = theme.ProfileWindows11Dark, "dark"
		}
		if err := m.SetTheme(profile); err != nil {
			t.Fatal(err)
		}
		m.SetIconResolver(widget.BuiltinIcons())
		const w, h = 1280, 800
		eng := engine.New(w, h, 30)
		registerFonts(t, eng)
		if err := eng.SetBackground(matWallpaper(w, h)); err != nil {
			t.Fatal(err)
		}
		if err := eng.ApplyThemeProfile(m); err != nil {
			t.Fatal(err)
		}
		root := widget.NewCanvas()
		root.SetBounds(image.Rect(0, 0, w, h))
		bar := desktop.NewTaskbar(m)
		bar.AddItem(desktop.SlotStart, desktop.NewStartButton(m))
		bar.SetBounds(image.Rect(0, h-bar.Height(), w, h))
		root.AddChild(bar)

		ns := desktop.NewFakeNotifications()
		ts := desktop.NewNotificationToast(m, ns)
		ts.Screen = image.Rect(0, 0, w, h)
		ts.Anchor = image.Rect(w-100, h-bar.Height(), w-10, h)
		ts.Clock = desktop.NewFakeClock(nc11Time())
		root.AddChild(ts)
		eng.SetRoot(root)
		ns.Add(desktop.Notification{
			AppID: "mail", AppName: "Почта", Icon: ncGlyph(48, color.RGBA{R: 60, G: 170, B: 90, A: 255}, 2),
			Title: "Анна Иванова", Body: "Привет! Посмотри, пожалуйста, договор до конца дня.",
			Timestamp: nc11Time().Add(-time.Minute),
			Actions: []desktop.NotificationAction{
				{ID: "reply", Kind: desktop.NotificationActionReply},
				{ID: "open", Kind: desktop.NotificationActionLink, Title: "Открыть письмо"},
			},
		})
		ts.Settle()
		eng.Invalidate()
		img := eng.RenderOnce()
		if ts.OverlayBounds().Empty() {
			t.Fatal("тост не показан")
		}
		ncSave(t, img, "nc_win11_toast_"+tag)
		ts.Close()
		bar.Close()
		widget.StopAllAnimations()
	}
}
