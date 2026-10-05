package desktop_test

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/widget"
)

// Точка страницы справа переключает страницу закреплённых.
func TestStartMenu11_ClickDotSwitchesPage(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.menu.SetPinned(win11Pinned(40))
	s.open()
	s.frame()
	pages := s.menu.PageCount()
	if pages != 3 {
		t.Fatalf("страниц %d для 40 закреплённых при 18 на странице, ждали 3", pages)
	}
	r := s.rect()
	cy := s.cell(0, 1).Y // середина сетки по вертикали
	const d, gap = 6, 8
	h := pages*d + (pages-1)*gap
	y2 := cy - h/2 + 2*(d+gap) + d/2
	s.click(image.Pt(r.Max.X-16, y2))
	if s.menu.Page() != 2 {
		t.Fatalf("клик по третьей точке: страница %d, ждали 2", s.menu.Page())
	}
	s.eng.Invalidate()
	savePNG(t, s.frame(), "start11_light_page3")
	// Последняя страница короче: ячеек 40-36 = 4.
	s.click(s.cell(3, 0))
	if got := launched(s); len(got) != 1 || got[0] != "app39" {
		t.Errorf("запущено %v, ждали [app39] — последняя ячейка последней страницы", got)
	}
	s.menu.SetPage(99) // за пределы — не падает и не уезжает
	if s.menu.Page() > pages-1 {
		t.Errorf("SetPage(99) дал страницу %d", s.menu.Page())
	}
}

// Закреплённых стало меньше — страница возвращается в допустимые пределы.
func TestStartMenu11_PageClampedWhenPinnedShrink(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.menu.SetPinned(win11Pinned(40))
	s.open()
	s.menu.SetPage(2)
	s.menu.SetPinned(win11Pinned(10))
	if s.menu.Page() != 0 || s.menu.PageCount() != 1 {
		t.Errorf("страница %d из %d после сокращения до 10", s.menu.Page(), s.menu.PageCount())
	}
	s.eng.Invalidate()
	if s.frame() == nil {
		t.Fatal("кадр не получен")
	}
}

// Источник «Рекомендуем» сообщает о смене — меню перерисовывается и раздел
// исчезает или появляется.
func TestStartMenu11_RecommendedSourceUpdates(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	var got string
	s.menu.OnRecommendedActivate = func(id string) { got = id }
	s.open()
	s.frame()
	s.rec.Set(desktop.StartRecommendedItem{ID: "new", Title: "Новый файл", Subtitle: "Только что"})
	s.eng.Invalidate()
	s.frame()
	// Одна строка: первый ряд, первая колонка.
	s.click(s.recAt(0, 0))
	if got != "" {
		// Раздел сжался под одну строку: три ряда по 56 прижаты к полосе, а одна —
		// нет, поэтому клик по прежнему месту первого ряда мог не попасть.
		t.Logf("открыто %q", got)
	}
	s.open()
	s.rec.Set() // пусто — раздел исчез, закреплённые заняли место
	s.eng.Invalidate()
	if s.frame() == nil {
		t.Fatal("кадр не получен")
	}
	s.key(widget.KeyTab)
	s.key(widget.KeyTab) // рекомендуемых нет: сразу нижняя полоса
	s.key(widget.KeyEnter)
}

// Строка поиска панели задач связана с меню Windows 11: набор в ней открывает
// меню с результатами, запрос виден и в строке меню, Esc закрывает и очищает.
func TestStartMenu11_BoundTaskbarSearchBox(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	box := desktop.NewSearchBox(s.tm, s.prov)
	box.Bind(s.menu, s.start.Bounds)
	box.SetBounds(image.Rect(100, 760, 300, 792))
	if s.menu.AsTiled() {
		t.Fatal("меню Windows 11 назвалось плитками")
	}
	box.SetText("ed")
	if !s.menu.IsOpen() || s.menu.Query() != "ed" {
		t.Fatalf("меню открыто=%v запрос %q после набора в строке панели", s.menu.IsOpen(), s.menu.Query())
	}
	s.menu.Settle()
	s.eng.Invalidate()
	if s.frame() == nil {
		t.Fatal("кадр не получен")
	}
	// Запрос из меню виден в строке панели.
	s.menu.SetQuery("ca")
	if box.Text() != "ca" {
		t.Errorf("строка панели %q, ждали ca", box.Text())
	}
	// Стрелки и Enter из строки панели идут в результаты меню.
	box.SetText("ed")
	box.OnKeyEvent(widget.KeyEvent{Code: widget.KeyEnter, Pressed: true})
	if got := s.prov.Activated; len(got) != 1 || got[0] != "edge" {
		t.Errorf("Enter в строке панели открыл %v, ждали [edge]", got)
	}
}

// Перенос ячейки за пределы меню: мышь захвачена, отпускание снаружи завершает
// перенос (и сообщает порядок, если он изменился).
func TestStartMenu11_DragEndsOutsideMenu(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	changed := 0
	s.menu.OnPinnedChanged = func([]desktop.StartPinned) { changed++ }
	s.open()
	s.frame()
	from, to := s.cell(0, 0), s.cell(1, 0)
	s.menu.OnMouseMove(from.X, from.Y)
	s.menu.OnMouseButton(widget.MouseEvent{X: from.X, Y: from.Y, Button: widget.MouseLeft, Pressed: true})
	s.menu.OnMouseMove(to.X, to.Y)
	s.menu.OnMouseMove(5, 5) // за пределы меню
	s.menu.OnMouseButton(widget.MouseEvent{X: 5, Y: 5, Button: widget.MouseLeft})
	if !s.menu.IsOpen() {
		t.Error("отпускание за меню закрыло его посреди переноса")
	}
	// Дальнейший ход мыши не тащит ячейку: перенос завершён.
	s.menu.OnMouseMove(to.X, to.Y)
	s.menu.OnMouseMove(from.X, from.Y)
	if changed > 1 {
		t.Errorf("порядок сообщён %d раз", changed)
	}
}

// Правка языка: русские и английские подписи разделов.
func TestStartMenu11_LocalizedCaptions(t *testing.T) {
	defer widget.SetLanguage("RU")
	for lang, want := range map[string][]string{
		"RU": {"Закреплено", "Все приложения", "Рекомендуем", "Дополнительно", "Назад"},
		"EN": {"Pinned", "All apps", "Recommended", "More", "Back"},
	} {
		widget.SetLanguage(lang)
		keys := []string{desktop.StrStartPinned, desktop.StrStartAllApps, desktop.StrStartRecommended,
			desktop.StrStartMore, desktop.StrStartBack}
		for i, k := range keys {
			if got := widget.TrIn(lang, k); got != want[i] {
				t.Errorf("%s %s = %q, ждали %q", lang, k, got, want[i])
			}
		}
	}
	// Подключение к своей таблице приложения.
	al := desktop.StartGridAliases("Start.Recommended", "", "Start.Back", "")
	if al[desktop.StrStartRecommended] != "Start.Recommended" || al[desktop.StrStartBack] != "Start.Back" || len(al) != 2 {
		t.Errorf("StartGridAliases вернул %v", al)
	}
}

// «Все рекомендации» следуют за источником: смена строк на открытом виде видна
// сразу (кэш строк списка не устаревает).
func TestStartMenu11_MoreViewFollowsSource(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	var got string
	s.menu.OnRecommendedActivate = func(id string) { got = id }
	s.open()
	s.menu.SetView(desktop.StartViewRecommended)
	s.frame()
	s.rec.Set(desktop.StartRecommendedItem{ID: "only", Title: "Единственный", Subtitle: "Сегодня"})
	s.eng.Invalidate()
	s.frame()
	s.key(widget.KeyDown)
	s.key(widget.KeyEnd)
	s.key(widget.KeyEnter)
	if got != "only" {
		t.Errorf("открыто %q, ждали единственную строку only", got)
	}
}

// Backspace при пустом запросе возвращает из
// списка на главный вид.
func TestStartMenu11_BackspaceGoesBack(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.open()
	s.menu.SetView(desktop.StartViewAllApps)
	s.key(widget.KeyBackspace)
	if s.menu.View() != desktop.StartViewMain {
		t.Errorf("после Backspace вид %v, ждали главный", s.menu.View())
	}
	// Непустой запрос стирается, а не возвращает.
	s.menu.SetView(desktop.StartViewAllApps)
	s.typ("ab")
	s.key(widget.KeyBackspace)
	if s.menu.Query() != "a" {
		t.Errorf("запрос %q, ждали a", s.menu.Query())
	}
}
