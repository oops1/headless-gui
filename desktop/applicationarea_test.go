package desktop

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// areaFixture — область приложений: два закреплённых («term» запущен,
// «notes» нет) и одно окно незакреплённого приложения.
func areaFixture(t *testing.T) (*ApplicationArea, *StaticAppCatalog, *FakeWindowModel) {
	t.Helper()
	tm := testThemeManager(t)
	cat := NewStaticAppCatalog(
		AppInfo{ID: "term", Title: "Терминал"},
		AppInfo{ID: "notes", Title: "Заметки"},
	)
	cat.Pin("term")
	cat.Pin("notes")
	wm := NewFakeWindowModel(
		WindowInfo{ID: 10, AppID: "term", Title: "Терминал", Active: true},
		WindowInfo{ID: 11, AppID: "mail", Title: "Почта"},
	)
	a := NewApplicationArea(tm, cat, wm)
	a.SetBounds(image.Rect(0, 0, 600, 40))
	return a, cat, wm
}

// Закреплённое приложение с открытым окном показывается ОДНОЙ ячейкой.
func TestApplicationArea_PinnedAndRunningMerge(t *testing.T) {
	a, _, _ := areaFixture(t)
	defer a.Close()

	cells := a.Cells()
	if len(cells) != 3 {
		t.Fatalf("ячеек %d, ждали 3 (term, notes, mail): %+v", len(cells), cells)
	}
	if cells[0].Title != "Терминал" || !cells[0].Active {
		t.Errorf("первая ячейка должна быть активным «Терминалом»: %+v", cells[0])
	}
	if !cells[1].Muted {
		t.Error("закреплённое незапущенное приложение должно быть приглушено")
	}
	if cells[2].Title != "Почта" {
		t.Errorf("третьей ячейкой ждали окно «Почта», получили %q", cells[2].Title)
	}
}

// Щелчок по закреплённому незапущенному запускает приложение.
func TestApplicationArea_ClickLaunchesPinned(t *testing.T) {
	a, cat, wm := areaFixture(t)
	defer a.Close()

	r := cellRect(t, a, 1) // «Заметки» — закреплены, не запущены
	clickAt(a, r)

	if got := cat.Launched; len(got) != 1 || got[0] != "notes" {
		t.Errorf("запущено %v, ждали [notes]", got)
	}
	if len(wm.Activated) != 0 {
		t.Error("незапущенное приложение не должно активировать окно")
	}
}

// Щелчок по активному окну сворачивает его, по неактивному — активирует.
func TestApplicationArea_ClickTogglesWindow(t *testing.T) {
	a, _, wm := areaFixture(t)
	defer a.Close()

	clickAt(a, cellRect(t, a, 0)) // «Терминал» активен
	if got := wm.Minimized; len(got) != 1 || got[0] != 10 {
		t.Errorf("свёрнуто %v, ждали [10]", got)
	}

	clickAt(a, cellRect(t, a, 2)) // «Почта» неактивна
	if got := wm.Activated; len(got) != 1 || got[0] != 11 {
		t.Errorf("активировано %v, ждали [11]", got)
	}
}

// Отпускание в стороне отменяет нажатие — как у всех кнопок панели.
func TestApplicationArea_ReleaseElsewhereCancels(t *testing.T) {
	a, cat, wm := areaFixture(t)
	defer a.Close()

	r := cellRect(t, a, 1)
	a.OnMouseButton(widget.MouseEvent{X: r.Min.X + 2, Y: r.Min.Y + 2, Button: widget.MouseLeft, Pressed: true})
	a.OnMouseButton(widget.MouseEvent{X: r.Max.X + 100, Y: r.Min.Y + 2, Button: widget.MouseLeft})

	if len(cat.Launched) != 0 || len(wm.Activated) != 0 {
		t.Error("отпускание в стороне всё же сработало")
	}
}

// Появление окна перестраивает содержимое: подписка на модель работает.
func TestApplicationArea_FollowsWindowModel(t *testing.T) {
	a, _, wm := areaFixture(t)
	defer a.Close()

	before := len(a.Cells())
	wm.SetWindows([]WindowInfo{
		{ID: 10, AppID: "term", Title: "Терминал", Active: true},
		{ID: 11, AppID: "mail", Title: "Почта"},
		{ID: 12, AppID: "calc", Title: "Калькулятор"},
	})
	if got := len(a.Cells()); got != before+1 {
		t.Errorf("ячеек %d, ждали %d — область не следит за моделью окон", got, before+1)
	}
}

// Под macOS отрисовку забирает док, и поведение при этом не меняется.
func TestApplicationArea_UsesThemePresenter(t *testing.T) {
	tm := managerFor(t, theme.ProfileMacOS)
	cat := NewStaticAppCatalog(AppInfo{ID: "term", Title: "Терминал"})
	cat.Pin("term")
	wm := NewFakeWindowModel(WindowInfo{ID: 10, AppID: "mail", Title: "Почта"})

	a := NewApplicationArea(tm, cat, wm)
	defer a.Close()
	a.SetBounds(image.Rect(0, 0, 600, 64))

	if PresenterFor(tm, PresenterKeyRunningApps) == nil {
		t.Fatal("тема macOS не назначила презентер области приложений")
	}
	// Ячейки дока — квадраты в размер значка, а не кнопки во всю высоту.
	r := cellRect(t, a, 0)
	if r.Dx() != r.Dy() {
		t.Errorf("ячейка дока %v не квадратная — раскладка не от презентера", r)
	}
	// И щелчок по ней по-прежнему запускает приложение.
	clickAt(a, r)
	if got := cat.Launched; len(got) != 1 {
		t.Errorf("под доком щелчок не запустил приложение: %v", got)
	}
}

// ─── Помощники ──────────────────────────────────────────────────────────────

func cellRect(t *testing.T, a *ApplicationArea, i int) image.Rectangle {
	t.Helper()
	a.mu.RLock()
	defer a.mu.RUnlock()
	if i >= len(a.rects) {
		t.Fatalf("ячейки %d нет: всего %d", i, len(a.rects))
	}
	return a.rects[i]
}

func clickAt(a *ApplicationArea, r image.Rectangle) {
	x, y := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	a.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft, Pressed: true})
	a.OnMouseButton(widget.MouseEvent{X: x, Y: y, Button: widget.MouseLeft})
}
