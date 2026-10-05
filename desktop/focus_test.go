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

// Клавиатура на панели задач: в desktop ничего не было widget.Focusable, и
// панель нельзя было обойти без мыши. Тесты идут через настоящий движок
// (Tab доставляет он), а не вызовом методов элементов, — иначе проверялось бы
// не то, что увидит пользователь.

type focusScene struct {
	eng   *engine.Engine
	bar   *desktop.Taskbar
	start *desktop.StartButton
	apps  *desktop.ApplicationArea
	net   *desktop.NetworkItem
	vol   *desktop.VolumeItem
	pow   *desktop.PowerItem
	clock *desktop.ClockItem
	wm    *desktop.FakeWindowModel
	cat   *desktop.StaticAppCatalog

	startClicks, clockClicks, netClicks int
}

// newFocusScene собирает панель на теме name. trayFirst добавляет трей раньше
// приложений: порядок обхода обязан зависеть от областей, а не от порядка
// добавления.
func newFocusScene(t *testing.T, name string, trayFirst bool) *focusScene {
	t.Helper()
	const w, h = 640, 120

	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(name); err != nil {
		t.Fatal(err)
	}
	m.SetIconResolver(widget.BuiltinIcons())

	s := &focusScene{}
	s.wm = desktop.NewFakeWindowModel(desktop.WindowInfo{ID: 7, Title: "Терминал", AppID: "term", Active: true})
	s.cat = desktop.NewStaticAppCatalog(
		desktop.AppInfo{ID: "term", Title: "Терминал"},
		desktop.AppInfo{ID: "mail", Title: "Почта"},
		desktop.AppInfo{ID: "files", Title: "Файлы"},
	)
	s.cat.Pin("term")
	s.cat.Pin("mail")
	s.cat.Pin("files")

	status := desktop.NewFakeSystemStatus()
	clk := desktop.NewFakeClock(time.Date(2026, 3, 14, 15, 9, 26, 0, time.UTC))

	s.start = desktop.NewStartButton(m)
	s.start.OnClick = func() { s.startClicks++ }
	s.apps = desktop.NewApplicationArea(m, s.cat, s.wm)
	s.net = desktop.NewNetworkStatus(m, status)
	s.net.OnClick = func() { s.netClicks++ }
	s.vol = desktop.NewVolumeStatus(m, status)
	s.pow = desktop.NewPowerStatus(m, status)
	s.clock = desktop.NewClock(m, clk)
	s.clock.OnClick = func() { s.clockClicks++ }

	s.bar = desktop.NewTaskbar(m)
	t.Cleanup(s.bar.Close)
	t.Cleanup(s.apps.Close)
	t.Cleanup(s.clock.Close)

	addTray := func() {
		s.bar.AddItem(desktop.SlotTray, s.net)
		s.bar.AddItem(desktop.SlotTray, s.vol)
		s.bar.AddItem(desktop.SlotTray, s.pow)
		s.bar.AddItem(desktop.SlotTray, s.clock)
	}
	if trayFirst {
		addTray()
	}
	s.bar.AddItem(desktop.SlotStart, s.start)
	s.bar.AddItem(desktop.SlotApps, s.apps)
	if !trayFirst {
		addTray()
	}

	root := widget.NewPanel(color.RGBA{R: 40, G: 70, B: 120, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w, h))
	barH := s.bar.Height()
	if barH <= 0 {
		t.Fatalf("тема %s не задала высоту панели", name)
	}
	s.bar.SetBounds(image.Rect(0, h-barH, w, h))
	root.AddChild(s.bar)

	s.eng = engine.New(w, h, 30)
	s.eng.SetRoot(root)
	return s
}

func key(code widget.KeyCode) widget.KeyEvent {
	return widget.KeyEvent{Code: code, Pressed: true}
}

func (s *focusScene) press(code widget.KeyCode) { s.eng.SendKeyEvent(key(code)) }

func (s *focusScene) shiftTab() {
	s.eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true, Mod: widget.ModShift})
}

// focused называет элемент с фокусом ("" — фокуса нет на панели).
func (s *focusScene) focused() string {
	switch {
	case s.start.IsFocused():
		return "start"
	case s.apps.IsFocused():
		return "apps"
	case s.net.IsFocused():
		return "net"
	case s.vol.IsFocused():
		return "vol"
	case s.pow.IsFocused():
		return "pow"
	case s.clock.IsFocused():
		return "clock"
	}
	return ""
}

func TestFocus_TabWalksTaskbarInSlotOrder(t *testing.T) {
	for _, trayFirst := range []bool{false, true} {
		s := newFocusScene(t, theme.ProfileWindows10, trayFirst)
		want := []string{"start", "apps", "net", "vol", "pow", "clock", "start"}
		for i, w := range want {
			s.press(widget.KeyTab)
			if got := s.focused(); got != w {
				t.Fatalf("trayFirst=%v: шаг %d: фокус на %q, ждал %q", trayFirst, i, got, w)
			}
		}
		// Shift+Tab идёт обратно.
		s.shiftTab()
		if got := s.focused(); got != "clock" {
			t.Errorf("trayFirst=%v: Shift+Tab от «Пуска» дал %q, ждал clock", trayFirst, got)
		}
		s.shiftTab()
		if got := s.focused(); got != "pow" {
			t.Errorf("trayFirst=%v: Shift+Tab дал %q, ждал pow", trayFirst, got)
		}
	}
}

// Значок без места на панели (батареи нет) не остаётся невидимой остановкой.
func TestFocus_TabSkipsItemsWithoutRoom(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	st := desktop.NewFakeSystemStatus()
	st.SetPower(desktop.PowerState{NoBattery: true}) // батареи нет: значок места не получает
	start := desktop.NewStartButton(m)
	pow := desktop.NewPowerStatus(m, st)
	defer pow.Close()
	net := desktop.NewNetworkStatus(m, st)
	defer net.Close()

	bar := desktop.NewTaskbar(m)
	defer bar.Close()
	bar.AddItem(desktop.SlotStart, start)
	bar.AddItem(desktop.SlotTray, net)
	bar.AddItem(desktop.SlotTray, pow)
	bar.SetBounds(image.Rect(0, 60, 400, 100))

	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 400, 100))
	root.AddChild(bar)
	eng := engine.New(400, 100, 30)
	eng.SetRoot(root)

	if !pow.Bounds().Empty() {
		t.Fatalf("значок питания без батареи получил место: %v", pow.Bounds())
	}
	for i := 0; i < 6; i++ {
		eng.SendKeyEvent(key(widget.KeyTab))
		if pow.IsFocused() {
			t.Fatalf("Tab остановился на невидимом значке питания (шаг %d)", i)
		}
	}
}

func TestFocus_ClockWithoutClickHandlerIsNotAStop(t *testing.T) {
	s := newFocusScene(t, theme.ProfileWindows10, false)
	s.clock.OnClick = nil
	var order []string
	for i := 0; i < 6; i++ {
		s.press(widget.KeyTab)
		order = append(order, s.focused())
	}
	for _, n := range order {
		if n == "clock" {
			t.Fatalf("часы без календаря стали остановкой Tab: %v", order)
		}
	}
}

func TestFocus_EnterAndSpaceActivate(t *testing.T) {
	s := newFocusScene(t, theme.ProfileWindows10, false)

	s.press(widget.KeyTab) // «Пуск»
	s.press(widget.KeyEnter)
	if s.startClicks != 1 {
		t.Fatalf("Enter на «Пуске»: нажатий %d, ждал 1", s.startClicks)
	}
	s.press(widget.KeySpace)
	if s.startClicks != 2 {
		t.Errorf("Space на «Пуске»: нажатий %d, ждал 2", s.startClicks)
	}
	// Автоповтор удержанного Enter не открывает меню снова и снова.
	s.eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true, Repeat: true})
	if s.startClicks != 2 {
		t.Errorf("повтор Enter нажал кнопку: %d", s.startClicks)
	}
	// Ctrl+Enter — не нажатие.
	s.eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true, Mod: widget.ModCtrl})
	if s.startClicks != 2 {
		t.Errorf("Ctrl+Enter нажал кнопку: %d", s.startClicks)
	}
	// Отпускание клавиши — не нажатие.
	s.eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: false})
	if s.startClicks != 2 {
		t.Errorf("отпускание Enter нажало кнопку: %d", s.startClicks)
	}

	// Значок сети и часы.
	for s.focused() != "net" {
		s.press(widget.KeyTab)
	}
	s.press(widget.KeyEnter)
	if s.netClicks != 1 {
		t.Errorf("Enter на значке сети: %d", s.netClicks)
	}
	for s.focused() != "clock" {
		s.press(widget.KeyTab)
	}
	s.press(widget.KeySpace)
	if s.clockClicks != 1 {
		t.Errorf("Space на часах: %d", s.clockClicks)
	}
}

func TestFocus_ArrowsStayInsideArea(t *testing.T) {
	s := newFocusScene(t, theme.ProfileWindows10, false)

	for s.focused() != "net" {
		s.press(widget.KeyTab)
	}
	s.press(widget.KeyRight)
	if got := s.focused(); got != "vol" {
		t.Fatalf("Right от сети: %q, ждал vol", got)
	}
	s.press(widget.KeyRight)
	s.press(widget.KeyRight)
	if got := s.focused(); got != "clock" {
		t.Fatalf("Right×3 от сети: %q, ждал clock", got)
	}
	// Край области: дальше стрелка не идёт и не прыгает в другую область.
	s.press(widget.KeyRight)
	if got := s.focused(); got != "clock" {
		t.Errorf("Right у края области: %q, ждал clock", got)
	}
	s.press(widget.KeyHome)
	if got := s.focused(); got != "net" {
		t.Errorf("Home: %q, ждал net", got)
	}
	s.press(widget.KeyLeft)
	if got := s.focused(); got != "net" {
		t.Errorf("Left у начала области: %q, ждал net (не должен выходить к приложениям)", got)
	}
	s.press(widget.KeyEnd)
	if got := s.focused(); got != "clock" {
		t.Errorf("End: %q, ждал clock", got)
	}
	// Ctrl+Right — сочетание приложения, а не навигация.
	s.eng.SendKeyEvent(widget.KeyEvent{Code: widget.KeyLeft, Pressed: true, Mod: widget.ModCtrl})
	if got := s.focused(); got != "clock" {
		t.Errorf("Ctrl+Left сдвинул фокус: %q", got)
	}
}

func TestFocus_ApplicationAreaCells(t *testing.T) {
	s := newFocusScene(t, theme.ProfileWindows11, false)
	s.press(widget.KeyTab)
	s.press(widget.KeyTab)
	if s.focused() != "apps" {
		t.Fatalf("фокус на %q, ждал apps", s.focused())
	}
	if got := s.apps.ButtonFocus(); got != 0 {
		t.Fatalf("первая ячейка: %d", got)
	}

	// Стрелки идут по ячейкам, фокус остаётся на области.
	s.press(widget.KeyRight)
	if s.focused() != "apps" || s.apps.ButtonFocus() != 1 {
		t.Fatalf("Right: фокус %q ячейка %d", s.focused(), s.apps.ButtonFocus())
	}
	s.press(widget.KeyEnd)
	if s.apps.ButtonFocus() != 2 {
		t.Errorf("End: ячейка %d, ждал 2", s.apps.ButtonFocus())
	}
	s.press(widget.KeyRight)
	if s.apps.ButtonFocus() != 2 {
		t.Errorf("Right у края: ячейка %d, ждал 2", s.apps.ButtonFocus())
	}
	s.press(widget.KeyHome)
	if s.apps.ButtonFocus() != 0 {
		t.Errorf("Home: ячейка %d", s.apps.ButtonFocus())
	}

	// Ячейка 0 — запущенный и активный терминал: Enter сворачивает его.
	s.press(widget.KeyEnter)
	if len(s.wm.Minimized) != 1 || s.wm.Minimized[0] != 7 {
		t.Errorf("Enter на активном окне: свёрнуто %v", s.wm.Minimized)
	}
	// Ячейка 1 — закреплённая «Почта» без окна: Space запускает.
	s.press(widget.KeyRight)
	s.press(widget.KeySpace)
	if len(s.cat.Launched) != 1 || s.cat.Launched[0] != "mail" {
		t.Errorf("Space на закреплённом: запущено %v", s.cat.Launched)
	}

	// Ячейка запоминается: выйти Tab'ом и вернуться Shift+Tab — та же.
	s.press(widget.KeyTab)
	if s.focused() != "net" {
		t.Fatalf("Tab из приложений: %q", s.focused())
	}
	s.shiftTab()
	if s.focused() != "apps" || s.apps.ButtonFocus() != 1 {
		t.Errorf("возврат: фокус %q ячейка %d, ждал apps/1", s.focused(), s.apps.ButtonFocus())
	}
}

// Шеврон раскрытия скрытых значков доступен с клавиатуры, когда значки не
// поместились.
func TestFocus_TrayChevronReachableAndToggles(t *testing.T) {
	m := theme.NewManager()
	if err := theme.RegisterBuiltinProfiles(m); err != nil {
		t.Fatal(err)
	}
	if err := m.SetTheme(theme.ProfileWindows11); err != nil {
		t.Fatal(err)
	}
	st := desktop.NewFakeSystemStatus()
	tray := desktop.NewSystemTray(m)
	tray.AddItem(desktop.NewNetworkStatus(m, st))
	tray.AddItem(desktop.NewVolumeStatus(m, st))
	tray.AddItem(desktop.NewPowerStatus(m, st))
	defer tray.Close()

	bar := desktop.NewTaskbar(m)
	defer bar.Close()
	bar.AddItem(desktop.SlotTray, tray)
	bar.SetBounds(image.Rect(0, 0, 70, 44)) // три значка не влезут

	root := widget.NewPanel(color.RGBA{A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, 200, 100))
	root.AddChild(bar)
	eng := engine.New(200, 100, 30)
	eng.SetRoot(root)

	if len(tray.Hidden()) == 0 {
		t.Fatal("значки поместились — тест не проверяет шеврон")
	}
	// Идём Tab'ом до остановки без OnClick — шеврон должен встретиться.
	toggled := false
	for i := 0; i < 8 && !toggled; i++ {
		eng.SendKeyEvent(key(widget.KeyTab))
		eng.SendKeyEvent(key(widget.KeyEnter))
		toggled = tray.Overflow().IsOpen()
	}
	if !toggled {
		t.Fatal("ни одна остановка Tab не раскрыла область скрытых значков")
	}
}

// ─── Рамка фокуса ────────────────────────────────────────────────────────────

func pixelAt(img *image.RGBA, x, y int) color.RGBA { return img.RGBAAt(x, y) }

// Рамка видна в светлой и тёмной темах каждого профиля: наружный и внутренний
// контуры отличаются от кадра без фокуса и друг от друга. Кадры при
// FOCUS_OUT=каталог сохраняются в PNG для просмотра.
func TestFocus_RingIsVisibleInEveryTheme(t *testing.T) {
	names := []string{
		theme.ProfileWindows2000,
		theme.ProfileWindows10,
		theme.ProfileWindows10Dark,
		theme.ProfileWindows11,
		theme.ProfileWindows11Dark,
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			s := newFocusScene(t, name, false)
			before := snapshot(s.eng.RenderOnce())

			s.press(widget.KeyTab) // «Пуск»
			if !s.start.FocusVisible() {
				t.Fatal("фокус пришёл по Tab, а рамка не видна")
			}
			after := snapshot(s.eng.RenderOnce())
			saveRing(t, after, name+"_start")

			b := s.start.Bounds()
			midY := (b.Min.Y + b.Max.Y) / 2
			outer0, outer1 := pixelAt(before, b.Min.X, midY), pixelAt(after, b.Min.X, midY)
			inner0, inner1 := pixelAt(before, b.Min.X+1, midY), pixelAt(after, b.Min.X+1, midY)
			if outer0 == outer1 {
				t.Errorf("наружный контур не нарисован: пиксель (%d,%d) остался %v", b.Min.X, midY, outer1)
			}
			if inner0 == inner1 {
				t.Errorf("внутренний контур не нарисован: пиксель (%d,%d) остался %v", b.Min.X+1, midY, inner1)
			}
			if outer1 == inner1 {
				t.Errorf("контуры одного цвета %v — на совпавшем фоне рамка пропала бы", outer1)
			}
			// И верхняя кромка, и нижняя — рамка замкнута.
			midX := (b.Min.X + b.Max.X) / 2
			for _, y := range []int{b.Min.Y, b.Max.Y - 1} {
				if pixelAt(before, midX, y) == pixelAt(after, midX, y) {
					t.Errorf("рамка не замкнута: пиксель (%d,%d) не изменился", midX, y)
				}
			}
			// Рамка обводит кнопку, а не заливает её: центр не тронут.
			cx, cy := midX, midY
			if pixelAt(before, cx, cy) != pixelAt(after, cx, cy) {
				t.Errorf("рамка закрасила содержимое кнопки: (%d,%d) %v → %v",
					cx, cy, pixelAt(before, cx, cy), pixelAt(after, cx, cy))
			}
		})
	}
}

// Рамка есть и у ячейки области приложений: обводится выбранная ячейка, а не
// вся область.
func TestFocus_RingOutlinesSelectedCell(t *testing.T) {
	s := newFocusScene(t, theme.ProfileWindows10Dark, false)
	before := snapshot(s.eng.RenderOnce())
	s.press(widget.KeyTab)
	s.press(widget.KeyTab)
	s.press(widget.KeyRight) // вторая ячейка
	after := snapshot(s.eng.RenderOnce())

	r := s.apps.ButtonRect(1)
	if r.Empty() {
		t.Fatal("у второй ячейки нет прямоугольника")
	}
	if pixelAt(before, r.Min.X, r.Min.Y+r.Dy()/2) == pixelAt(after, r.Min.X, r.Min.Y+r.Dy()/2) {
		t.Error("левая кромка выбранной ячейки не обведена")
	}
	first := s.apps.ButtonRect(0)
	if pixelAt(before, first.Min.X, first.Min.Y+first.Dy()/2) != pixelAt(after, first.Min.X, first.Min.Y+first.Dy()/2) {
		t.Error("обведена не выбранная ячейка")
	}
	saveRing(t, after, "cell")
}

// Щелчок мышью по «Пуску» отдаёт ему фокус, но рамки после щелчка нет: так
// ведёт себя Windows, и контур вокруг только что нажатой кнопки был бы шумом.
func TestFocus_MouseClickShowsNoRing(t *testing.T) {
	s := newFocusScene(t, theme.ProfileWindows10, false)
	b := s.start.Bounds()
	cx, cy := (b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2

	s.eng.SendMouseMove(cx, cy)
	s.eng.SendMouseButton(cx, cy, widget.MouseLeft, true)
	s.eng.SendMouseButton(cx, cy, widget.MouseLeft, false)
	if s.startClicks != 1 {
		t.Fatalf("щелчок не дошёл до кнопки: %d", s.startClicks)
	}
	if !s.start.IsFocused() {
		t.Fatal("щелчок не отдал кнопке фокус")
	}
	if s.start.FocusVisible() {
		t.Error("после щелчка мышью показывается рамка фокуса")
	}

	// Клавиатура возвращает рамку.
	s.press(widget.KeyEnter)
	if !s.start.FocusVisible() {
		t.Error("после нажатия клавиши рамка не вернулась")
	}
	// А Tab к другому и обратно — тоже с рамкой.
	s.press(widget.KeyTab)
	s.shiftTab()
	if !s.start.FocusVisible() {
		t.Error("фокус пришёл по Tab, а рамки нет")
	}
}

func saveRing(t *testing.T, img *image.RGBA, name string) {
	t.Helper()
	dir := os.Getenv("FOCUS_OUT")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, "focus_"+name+".png"))
	if err != nil {
		t.Logf("не создать PNG: %v", err)
		return
	}
	defer f.Close()
	_ = png.Encode(f, img)
}
