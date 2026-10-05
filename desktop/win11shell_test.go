package desktop

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Панель задач Windows 11: сцена со всеми новыми элементами. Общая для тестов
// поведения и для снимков (win11shell_visual_test.go).

const (
	w11W = 1000
	w11H = 110
)

// w11Scene — панель Windows 11: виджеты, «Пуск», поиск, Task View, кнопки окон с
// индикаторами, группа сети, колокольчик и часы.
type w11Scene struct {
	t        *testing.T
	tm       *theme.Manager
	root     *widget.Panel
	bar      *Taskbar
	widgets  *WidgetsButton
	start    *StartButton
	search   *SearchBox
	taskview *TaskViewButton
	area     *ApplicationArea
	wm       *FakeWindowModel
	tray     *SystemTray
	clock    *ClockItem
	group    *TrayGroup
	bell     *NotificationButton
	notes    *FakeNotifications
	status   *FakeSystemStatus
	clicks   map[string]int
}

// w11Windows — окна сцены: по одному на состояние индикатора.
func w11Windows() []WindowInfo {
	return []WindowInfo{
		{ID: 1, AppID: "web", Title: "Browser", Active: true},
		{ID: 2, AppID: "files", Title: "Files", ProgressState: ProgressNormal, Progress: 0.45},
		{ID: 3, AppID: "code", Title: "Editor", ProgressState: ProgressPaused, Progress: 0.7},
		{ID: 4, AppID: "mail", Title: "Mail", Badge: 3},
		{ID: 5, AppID: "term", Title: "bash", ProgressState: ProgressError, Progress: 0.3},
		{ID: 6, AppID: "calc", Title: "Calc", Attention: true},
		{ID: 7, AppID: "music", Title: "Music", Badge: 120, Minimized: true},
	}
}

func newW11Scene(t *testing.T, profile string) *w11Scene {
	t.Helper()
	tm := managerFor(t, profile)
	root := widget.NewPanel(color.RGBA{R: 30, G: 80, B: 120, A: 255})
	root.ShowHeader = false
	root.SetBounds(image.Rect(0, 0, w11W, w11H))
	const cell = 24
	for y := 0; y < w11H; y += cell {
		for x := 0; x < w11W; x += cell {
			shade := uint8(70)
			if (x/cell+y/cell)%2 == 0 {
				shade = 130
			}
			p := widget.NewPanel(color.RGBA{R: shade, G: uint8(100 + int(shade)/4), B: 160, A: 255})
			p.ShowHeader = false
			p.SetBounds(image.Rect(x, y, x+cell, y+cell))
			root.AddChild(p)
		}
	}

	icons := map[AppID]color.RGBA{
		"web": {R: 40, G: 140, B: 230, A: 255}, "files": {R: 240, G: 180, B: 40, A: 255},
		"code": {R: 60, G: 190, B: 120, A: 255}, "mail": {R: 220, G: 70, B: 80, A: 255},
		"term": {R: 150, G: 90, B: 210, A: 255}, "calc": {R: 90, G: 100, B: 120, A: 255},
		"music": {R: 230, G: 110, B: 160, A: 255}, "store": {R: 20, G: 160, B: 170, A: 255},
	}
	var apps []AppInfo
	order := []AppID{"web", "files", "code", "mail", "term", "calc", "music", "store"}
	for _, id := range order {
		apps = append(apps, AppInfo{ID: id, Title: string(id), Icon: solidIcon(icons[id])})
	}
	cat := NewStaticAppCatalog(apps...)
	for _, id := range order {
		cat.Pin(id)
	}

	s := &w11Scene{t: t, tm: tm, root: root, clicks: map[string]int{}}
	s.wm = NewFakeWindowModel(w11Windows()...)
	s.status = NewFakeSystemStatus()
	s.status.SetNetwork(NetState{Kind: NetWiFi, Quality: 0.8, Name: "home"})
	s.status.SetVolume(VolState{Level: 0.6})
	s.status.SetPower(PowerState{Charge: 0.75})
	s.notes = NewFakeNotifications()
	for i := 0; i < 4; i++ {
		s.notes.Add(Notification{Title: "n"})
	}

	s.bar = NewTaskbar(tm)
	s.widgets = NewWidgetsButton(tm)
	s.widgets.SetContent(WidgetsContent{
		Icon: solidIcon(color.RGBA{R: 250, G: 200, B: 40, A: 255}), Temperature: "21°C", Caption: "Sunny",
	})
	s.widgets.OnClick = func() { s.clicks["widgets"]++ }
	s.start = NewStartButton(tm)
	s.start.OnClick = func() { s.clicks["start"]++ }
	s.search = NewSearchBox(tm, NewFakeSearchProvider())
	s.search.SetMode(SearchModeIconOnly)
	s.search.OnActivate = func() { s.clicks["search"]++ }
	s.taskview = NewTaskViewButton(tm)
	s.taskview.OnClick = func() { s.clicks["taskview"]++ }
	s.area = NewApplicationArea(tm, cat, s.wm)
	s.group = NewTrayGroup(tm, NewNetworkStatus(tm, s.status), NewVolumeStatus(tm, s.status), NewPowerStatus(tm, s.status))
	s.group.OnClick = func() { s.clicks["group"]++ }
	s.bell = NewNotificationButton(tm, s.notes)
	s.bell.OnClick = func() { s.clicks["bell"]++ }
	s.tray = NewSystemTray(tm)
	s.tray.AddItem(s.group)
	s.tray.AddItem(s.bell)
	s.clock = NewClock(tm, NewFakeClock(time.Date(2026, 10, 5, 14, 30, 0, 0, time.UTC)))

	s.bar.AddItem(SlotWidgets, s.widgets)
	s.bar.AddItem(SlotStart, s.start)
	s.bar.AddItem(SlotStart, s.search)
	s.bar.AddItem(SlotStart, s.taskview)
	s.bar.AddItem(SlotApps, s.area)
	s.bar.AddItem(SlotTray, s.tray)
	s.bar.AddItem(SlotTray, s.clock)
	s.bar.SetBounds(image.Rect(0, w11H-s.bar.Height(), w11W, w11H))
	root.AddChild(s.bar)
	t.Cleanup(func() {
		s.tray.Close()
		s.bar.Close()
		s.area.Close()
		widget.StopAllAnimations()
	})
	return s
}

// render рисует кадр сцены на холсте масштаба scale и доводит анимации до конца.
func (s *w11Scene) render(scale float64) *image.RGBA {
	eng := engine.New(w11W, w11H, 30)
	if scale != 1 {
		eng.SetScale(scale)
	}
	if err := eng.SetThemeProfile(s.tm, s.tm.Active().Name()); err != nil {
		s.t.Fatal(err)
	}
	eng.ApplyThemeProfile(s.tm)
	eng.SetRoot(s.root)
	eng.RenderOnce()
	finishAnimations()
	return eng.RenderOnce()
}

// ─── Геометрия по плану §2 ───────────────────────────────────────────────────

// Кнопки 40×40 с зазором 4, значок 24, панель 48 (100 %).
func TestWin11Bar_GeometryByPlan(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	if h := s.bar.Height(); h != 48 {
		t.Fatalf("высота панели %d, ждали 48", h)
	}
	s.area.layout()
	var prev image.Rectangle
	for i := 0; i < 7; i++ {
		r := s.area.ButtonRect(i)
		if r.Dx() != 40 || r.Dy() != 40 {
			t.Fatalf("кнопка %d: %v, ждали 40×40", i, r)
		}
		if want := (s.bar.Bounds().Dy() - 40) / 2; r.Min.Y-s.bar.Bounds().Min.Y != want {
			t.Errorf("кнопка %d не по центру высоты: Min.Y %d", i, r.Min.Y)
		}
		if i > 0 && r.Min.X-prev.Max.X != 4 {
			t.Errorf("зазор между кнопками %d и %d: %d, ждали 4", i-1, i, r.Min.X-prev.Max.X)
		}
		prev = r
	}
	for name, it := range map[string]Item{"пуск": s.start, "task view": s.taskview} {
		if b := it.Bounds(); b.Dx() != 40 || b.Dy() != 40 {
			t.Errorf("%s: %v, ждали 40×40", name, b)
		}
	}
	if b := s.search.Bounds(); b.Dx() != 40 || b.Dy() != 40 {
		t.Errorf("поиск-значок: %v, ждали 40×40", b)
	}
	if b := s.group.Bounds(); b.Dy() != 40 {
		t.Errorf("группа трея: высота %d, ждали 40", b.Dy())
	}
	// Три значка по 16 с шагом 24 и полем 4: 3·24 + 2·4 = 80.
	if b := s.group.Bounds(); b.Dx() != 80 {
		t.Errorf("группа трея: ширина %d, ждали 80", b.Dx())
	}
}

// Поле поиска: высота 32 по центру панели, скругление 16.
func TestWin11Search_BoxIs32High(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	s.search.SetMode(SearchModeBox)
	b := s.search.Bounds()
	if b.Dy() != 32 || b.Dx() != 200 {
		t.Fatalf("поле поиска %v, ждали 200×32", b)
	}
	if mid := b.Min.Y + b.Dy()/2; mid != s.bar.Bounds().Min.Y+24 {
		t.Errorf("поле не по центру панели: середина %d", mid)
	}
	if c := s.tm.GetStyle("searchbox", "", theme.StateNormal).Corner; c != 16 {
		t.Errorf("скругление поля %v, ждали 16", c)
	}
	s.search.SetMode(SearchModeIconAndLabel)
	if b := s.search.Bounds(); b.Dy() != 32 || b.Dx() != 112 {
		t.Errorf("кнопка «значок и подпись» %v, ждали 112×32", b)
	}
	s.search.SetMode(SearchModeHidden)
	if !s.search.Bounds().Empty() {
		t.Error("скрытый поиск занимает место")
	}
}

// ─── Выравнивание на лету ────────────────────────────────────────────────────

func TestWin11Bar_AlignmentLive(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	bar := s.bar.Bounds()
	groupLeft := func() int { return s.start.Bounds().Min.X }
	groupRight := func() int {
		r := s.area.Bounds()
		last := s.area.ButtonRect(len(s.area.Cells()) - 1)
		if last.Empty() {
			return r.Max.X
		}
		return last.Max.X
	}
	// По центру: середина группы — середина панели (±2).
	mid := (groupLeft() + groupRight()) / 2
	if d := mid - (bar.Min.X+bar.Max.X)/2; d < -2 || d > 2 {
		t.Fatalf("группа не по центру: середина %d, панели %d", mid, (bar.Min.X+bar.Max.X)/2)
	}
	if s.bar.Alignment() != BarAlignCenter {
		t.Error("Alignment() не Center у Windows 11")
	}
	widgetsRight := s.widgets.Bounds().Max.X

	// Влево флагом темы — на лету, без пересоздания: те же объекты.
	items := s.bar.Items(SlotStart)
	s.tm.SetFlag(KeyTaskbarCentered, false)
	if s.bar.Alignment() != BarAlignLeft {
		t.Error("флаг taskbar.centered=false не прижал группу")
	}
	if got := groupLeft(); got < widgetsRight || got > widgetsRight+8 {
		t.Errorf("слева «Пуск» стоит на %d, слот виджетов кончается на %d", got, widgetsRight)
	}
	for i, it := range s.bar.Items(SlotStart) {
		if it != items[i] {
			t.Error("элементы пересозданы при смене выравнивания")
		}
	}

	// Явное назначение побеждает флаг; Reset возвращает его.
	s.bar.SetAlignment(BarAlignCenter)
	if s.bar.Alignment() != BarAlignCenter || groupLeft() < widgetsRight+20 {
		t.Errorf("SetAlignment(Center) не поставил группу по центру: %d", groupLeft())
	}
	s.bar.ResetAlignment()
	if s.bar.Alignment() != BarAlignLeft {
		t.Error("ResetAlignment не вернул решение флагу (false → влево)")
	}
	s.tm.SetFlag(KeyTaskbarCentered, true)
	if s.bar.Alignment() != BarAlignCenter {
		t.Error("возврат флага не вернул группу в центр")
	}
}

// Смена выравнивания и темы перерисовывает только полосу панели.
func TestWin11Bar_AlignmentDamagesOnlyBar(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	s.render(1)
	var rects []image.Rectangle
	fulls := 0
	h := widget.RegisterUINotifierWake(func() { fulls++ }, func(r image.Rectangle) { rects = append(rects, r) }, func() {})
	defer widget.UnregisterUINotifier(h)

	s.bar.SetAlignment(BarAlignLeft)
	s.bar.SetAlignment(BarAlignCenter)
	if fulls != 0 {
		t.Errorf("смена выравнивания вызвала %d полных перерисовок", fulls)
	}
	for _, r := range rects {
		if !r.Empty() && !rectIn(r, s.bar.Bounds()) {
			t.Errorf("заявлена область %v вне панели %v", r, s.bar.Bounds())
		}
	}
	if len(rects) == 0 {
		t.Error("смена выравнивания ничего не перерисовала")
	}
}

// Смена темы (светлая ↔ тёмная) и акцента на живой панели: те же объекты, кадр
// другой.
func TestWin11Bar_ThemeAndAccentLive(t *testing.T) {
	s := newW11Scene(t, theme.ProfileWindows11)
	light := s.render(1)
	startBefore := s.start
	if err := s.tm.SetTheme(theme.ProfileWindows11Dark); err != nil {
		t.Fatal(err)
	}
	dark := s.render(1)
	if imagesEqualRGBA(light, dark) {
		t.Error("смена темы не изменила кадр")
	}
	if s.bar.Items(SlotStart)[0] != Item(startBefore) {
		t.Error("«Пуск» пересоздан при смене темы")
	}
	s.tm.SetAccent(theme.RGB(190, 30, 60))
	red := s.render(1)
	if imagesEqualRGBA(dark, red) {
		t.Error("смена акцента не изменила кадр (пилюля активного окна)")
	}
}
