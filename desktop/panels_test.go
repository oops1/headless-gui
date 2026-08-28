package desktop

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Тесты меню «Пуск», быстрых настроек и центра уведомлений.
//
// Они лежат в одном файле, потому что проверяют одно и то же обещание с трёх
// сторон: панель делает своё дело мышью и клавиатурой, отпускает подписки и
// закрывается, когда должна.

// panelTheme — тема с метриками панелей: без них панель просит нулевой
// размер и не открывается.
func panelTheme(t *testing.T) *theme.Manager {
	t.Helper()
	return managerFor(t, theme.ProfileWindows10)
}

const panelScreenW, panelScreenH = 800, 600

func panelScreen() image.Rectangle         { return image.Rect(0, 0, panelScreenW, panelScreenH) }
func panelAnchor() image.Rectangle         { return image.Rect(0, panelScreenH-40, 48, panelScreenH) }
func pointIn(r image.Rectangle) (int, int) { return r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2 }

// ─── Меню «Пуск» ────────────────────────────────────────────────────────────

func startMenuFixture(t *testing.T) (*StartMenu, *StaticAppCatalog) {
	t.Helper()
	cat := NewStaticAppCatalog(
		AppInfo{ID: "term", Title: "Терминал"},
		AppInfo{ID: "files", Title: "Проводник"},
		AppInfo{ID: "mail", Title: "Почта"},
	)
	m := NewStartMenu(panelTheme(t), cat)
	m.Screen = panelScreen()
	m.Open(panelAnchor())
	return m, cat
}

func TestStartMenu_ClickLaunchesAndCloses(t *testing.T) {
	m, cat := startMenuFixture(t)

	rows := m.layoutRows()
	var target image.Rectangle
	for _, lr := range rows {
		if lr.row.kind == startMenuRowApp {
			target = lr.rect
			break
		}
	}
	if target.Empty() {
		t.Fatal("в меню не нашлось ни одной строки приложения")
	}

	x, y := pointIn(target)
	m.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	m.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})

	if len(cat.Launched) != 1 {
		t.Errorf("запущено %v, ждали одно приложение", cat.Launched)
	}
	if m.IsOpen() {
		t.Error("меню осталось открытым после запуска приложения")
	}
}

func TestStartMenu_KeyboardSelectsAndLaunches(t *testing.T) {
	m, cat := startMenuFixture(t)

	// Первое нажатие вниз уже что-то выделяет — без лишнего повтора.
	m.OnKeyEvent(widget.KeyEvent{Code: widget.KeyDown, Pressed: true})
	m.OnKeyEvent(widget.KeyEvent{Code: widget.KeyDown, Pressed: true})
	m.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})

	if len(cat.Launched) != 1 {
		t.Fatalf("клавишами запущено %v, ждали одно приложение", cat.Launched)
	}
	if cat.Launched[0] != "files" {
		t.Errorf("запущено %q — выделение сдвинулось не на ту строку", cat.Launched[0])
	}
}

func TestStartMenu_EscapeAndClickAwayClose(t *testing.T) {
	m, _ := startMenuFixture(t)

	m.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEscape, Pressed: true})
	if m.IsOpen() {
		t.Error("Esc не закрыл меню")
	}

	m.Open(panelAnchor())
	m.OnMouseButton(widget.MouseEvent{
		X: panelScreenW - 1, Y: 0, Button: widget.MouseLeft, Pressed: true,
	})
	if m.IsOpen() {
		t.Error("щелчок мимо не закрыл меню")
	}
}

// ─── Быстрые настройки ──────────────────────────────────────────────────────

func quickFixture(t *testing.T) (*QuickSettings, *FakeSystemStatus) {
	t.Helper()
	st := NewFakeSystemStatus()
	q := NewQuickSettings(panelTheme(t), st)
	q.Screen = panelScreen()
	q.Open(panelAnchor())
	return q, st
}

func TestQuickSettings_TilesFireCallbacks(t *testing.T) {
	q, _ := quickFixture(t)
	defer q.Close()

	wifi, mute := 0, 0
	q.OnToggleWiFi = func() { wifi++ }
	q.OnToggleMute = func() { mute++ }

	for i, hits := range []*int{&wifi, &mute} {
		r := q.tileRectAt(i)
		if r.Empty() {
			t.Fatalf("плитка %d не разложена", i)
		}
		x, y := pointIn(r)
		q.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
		q.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
		if *hits != 1 {
			t.Errorf("плитка %d: колбэк вызван %d раз", i, *hits)
		}
	}
}

func TestQuickSettings_DragChangesVolume(t *testing.T) {
	q, _ := quickFixture(t)
	defer q.Close()

	var last float64 = -1
	q.OnVolumeChange = func(v float64) { last = v }

	track := q.trackRect()
	if track.Empty() {
		t.Fatal("дорожка ползунка не разложена")
	}
	_, y := pointIn(track)
	// Тянем к правому краю — громкость должна уйти к максимуму.
	q.OnMouseButton(widget.MouseEvent{X: track.Min.X + 2, Y: y, Button: widget.MouseLeft, Pressed: true})
	q.OnMouseMove(track.Max.X-1, y)
	q.OnMouseButton(widget.MouseEvent{X: track.Max.X - 1, Y: y, Button: widget.MouseLeft})

	if last < 0.9 {
		t.Errorf("после перетаскивания вправо громкость = %v, ждали около единицы", last)
	}
}

func TestQuickSettings_CloseUnsubscribes(t *testing.T) {
	q, st := quickFixture(t)

	q.Close()
	if q.IsOpen() {
		t.Error("Close не закрыл панель")
	}

	// После Close изменение состояния системы не должно будить отрисовку —
	// иначе закрытая панель вечно просыпается на каждый чих сети и звука.
	var calls int
	handle := widget.RegisterUINotifier(nil, func(image.Rectangle) { calls++ })
	defer widget.UnregisterUINotifier(handle)
	st.SetVolume(VolState{Level: 0.9})
	if calls != 0 {
		t.Errorf("после Close изменение статуса вызвало %d перерисовок", calls)
	}
}

// ─── Центр уведомлений ──────────────────────────────────────────────────────

func notifFixture(t *testing.T) (*NotificationCenter, *FakeNotifications) {
	t.Helper()
	ns := notesFake()
	nc := NewNotificationCenter(panelTheme(t), ns)
	nc.Screen = panelScreen()
	nc.Open(panelAnchor())
	return nc, ns
}

func TestNotificationCenter_CrossDismissesOnRelease(t *testing.T) {
	nc, ns := notifFixture(t)
	defer nc.Close()

	layout := nc.computeLayout(nc.contentRect(), nc.list())
	if len(layout.cards) == 0 {
		t.Fatal("карточки не разложены")
	}
	cross := layout.cards[0].closeRect
	x, y := pointIn(cross)

	nc.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	if len(ns.List()) != 2 {
		t.Error("уведомление закрылось на нажатии, а не на отпускании")
	}
	nc.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
	if got := len(ns.List()); got != 1 {
		t.Errorf("осталось %d уведомлений, ждали 1", got)
	}
}

func TestNotificationCenter_ClearAll(t *testing.T) {
	nc, ns := notifFixture(t)
	defer nc.Close()

	layout := nc.computeLayout(nc.contentRect(), nc.list())
	if layout.clearAll.Empty() {
		t.Fatal("кнопка «очистить все» не разложена при двух уведомлениях")
	}
	x, y := pointIn(layout.clearAll)
	nc.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	nc.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})

	if got := len(ns.List()); got != 0 {
		t.Errorf("осталось %d уведомлений — очистка не сработала", got)
	}
}

func TestNotificationCenter_EmptyStateAndClose(t *testing.T) {
	tm := panelTheme(t)
	ns := NewFakeNotifications()
	nc := NewNotificationCenter(tm, ns)
	nc.Screen = panelScreen()
	nc.Open(panelAnchor())

	if nc.EmptyText == "" {
		t.Error("не задан текст пустого состояния")
	}
	if nc.OverlayBounds().Empty() {
		t.Error("панель без уведомлений не открылась — пустое состояние показать негде")
	}

	nc.Close()

	var calls int
	handle := widget.RegisterUINotifier(nil, func(image.Rectangle) { calls++ })
	defer widget.UnregisterUINotifier(handle)
	ns.Add(Notification{Title: "Ещё одно", Time: sampleTime()})
	if calls != 0 {
		t.Errorf("после Close новое уведомление вызвало %d перерисовок", calls)
	}
}
