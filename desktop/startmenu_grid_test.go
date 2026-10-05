package desktop_test

import (
	"image"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/widget"
)

// Поведение меню «Пуск» Windows 11: запуск, перетаскивание, страницы, виды,
// поиск, клавиатура, контекстное и нижнее меню.

func launched(s *win11Scene) []string {
	var out []string
	for _, id := range s.cat.Launched {
		out = append(out, string(id))
	}
	return out
}

func TestStartMenu11_LaunchPinnedByClick(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.open()
	s.frame()
	s.click(s.cell(1, 0)) // «Word» = app1
	if got := launched(s); len(got) != 1 || got[0] != "app1" {
		t.Fatalf("запущено %v, ждали [app1]", got)
	}
	if s.menu.IsOpen() {
		t.Error("меню осталось открытым после запуска")
	}
}

func TestStartMenu11_PinnedWithoutAppUsesCallback(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.menu.SetPinned([]desktop.StartPinned{{ID: "doc", Title: "Заметки"}})
	var got string
	s.menu.OnPinnedLaunch = func(id string) { got = id }
	s.open()
	s.click(s.cell(0, 0))
	if got != "doc" || s.menu.IsOpen() {
		t.Errorf("OnPinnedLaunch(%q), открыто=%v", got, s.menu.IsOpen())
	}
}

func TestStartMenu11_RecommendedActivate(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	var got string
	s.menu.OnRecommendedActivate = func(id string) { got = id }
	s.open()
	s.click(s.recAt(1, 1)) // второй ряд, правая колонка = r4
	if got != "r4" {
		t.Errorf("активирована строка %q, ждали r4", got)
	}
	if s.menu.IsOpen() {
		t.Error("меню осталось открытым")
	}
}

// Закреплённые по умолчанию берутся из каталога.
func TestStartMenu11_PinnedFromCatalog(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.menu.SetPinned(nil)
	s.cat.Pin("edge")
	s.cat.Pin("store")
	p := s.menu.Pinned()
	if len(p) != 2 || p[0].App != "edge" || p[1].Title != "Microsoft Store" {
		t.Fatalf("закреплённые каталога: %+v", p)
	}
	s.open()
	s.click(s.cell(1, 0))
	if got := launched(s); len(got) != 1 || got[0] != "store" {
		t.Errorf("запущено %v, ждали [store]", got)
	}
}

// Перетаскивание ячейки: порог, новый порядок событием, клик без переноса не
// меняет порядок.
func TestStartMenu11_DragReorder(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	var orders [][]string
	s.menu.OnPinnedChanged = func(p []desktop.StartPinned) {
		var ids []string
		for _, x := range p {
			ids = append(ids, x.ID)
		}
		orders = append(orders, ids)
	}
	s.open()
	s.frame()

	// Нажатие без сдвига — запуск, не перенос.
	from, to := s.cell(0, 0), s.cell(2, 0)
	s.menu.OnMouseMove(from.X, from.Y)
	s.menu.OnMouseButton(widget.MouseEvent{X: from.X, Y: from.Y, Button: widget.MouseLeft, Pressed: true})
	s.menu.OnMouseMove(from.X+1, from.Y) // меньше порога
	s.menu.OnMouseButton(widget.MouseEvent{X: from.X + 1, Y: from.Y, Button: widget.MouseLeft})
	if len(orders) != 0 {
		t.Fatalf("клик без переноса прислал порядок %v", orders)
	}
	if len(launched(s)) != 1 {
		t.Fatalf("клик не запустил приложение: %v", launched(s))
	}

	// Перенос app0 на место app2.
	s.open()
	s.menu.OnMouseMove(from.X, from.Y)
	s.menu.OnMouseButton(widget.MouseEvent{X: from.X, Y: from.Y, Button: widget.MouseLeft, Pressed: true})
	for _, x := range []int{from.X + 10, from.X + 60, to.X} {
		s.menu.OnMouseMove(x, to.Y)
	}
	s.eng.Invalidate()
	savePNG(t, s.frame(), "start11_light_drag")
	s.menu.OnMouseButton(widget.MouseEvent{X: to.X, Y: to.Y, Button: widget.MouseLeft})
	if len(orders) != 1 {
		t.Fatalf("событий порядка %d, ждали 1", len(orders))
	}
	got := strings.Join(orders[0][:4], ",")
	if want := "app1,app2,app0,app3"; got != want {
		t.Errorf("порядок %s, ждали %s", got, want)
	}
	if len(orders[0]) != 18 {
		t.Errorf("в порядке %d закреплённых, ждали 18", len(orders[0]))
	}
	// Меню запомнило порядок, и он же виден через Pinned.
	if p := s.menu.Pinned(); p[0].ID != "app1" || p[2].ID != "app0" {
		t.Errorf("меню не запомнило порядок: %v %v", p[0].ID, p[2].ID)
	}
	if !s.menu.IsOpen() {
		t.Error("перенос закрыл меню")
	}
}

// Постраничная прокрутка: колесо листает, точки справа переключают, клавиши
// PageUp/PageDown листают.
func TestStartMenu11_PagesByWheelDotsAndKeys(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.menu.SetPinned(win11Pinned(40))
	s.open()
	s.frame()
	pc := s.menu.PageCount()
	if pc < 2 {
		t.Fatalf("страниц %d для 40 закреплённых", pc)
	}
	if s.menu.Page() != 0 {
		t.Fatal("меню открылось не на первой странице")
	}
	at := s.cell(2, 1)
	s.menu.OnMouseButton(widget.MouseEvent{X: at.X, Y: at.Y, Button: widget.MouseWheelDown, Pressed: true})
	if s.menu.Page() != 1 {
		t.Fatalf("колесо вниз: страница %d, ждали 1", s.menu.Page())
	}
	s.menu.OnMouseButton(widget.MouseEvent{X: at.X, Y: at.Y, Button: widget.MouseWheelUp, Pressed: true})
	// Повторный щелчок сразу же игнорируется: трекпад шлёт десятки событий на жест.
	if s.menu.Page() != 1 {
		t.Errorf("второй щелчок без паузы сменил страницу: %d", s.menu.Page())
	}
	s.eng.Invalidate()
	savePNG(t, s.frame(), "start11_light_page2")

	// Первая ячейка второй страницы запускает приложение с номером «ячеек на страницу».
	perPage := 18
	if pc*perPage < 40 {
		perPage = 40 / pc
	}
	s.click(s.cell(0, 0))
	if got := launched(s); len(got) != 1 || !strings.HasPrefix(got[0], "app") {
		t.Fatalf("запущено %v", got)
	}
	if got := launched(s)[0]; got == "app0" {
		t.Errorf("запущена ячейка первой страницы %s вместо второй", got)
	}

	// Клавиши.
	s.open()
	s.key(widget.KeyTab)
	s.key(widget.KeyPageDown)
	if s.menu.Page() != 1 {
		t.Errorf("PageDown: страница %d, ждали 1", s.menu.Page())
	}
	s.key(widget.KeyPageUp)
	if s.menu.Page() != 0 {
		t.Errorf("PageUp: страница %d, ждали 0", s.menu.Page())
	}
}

func TestStartMenu11_AllAppsViewAndBack(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.open()
	s.frame()
	if s.menu.View() != desktop.StartViewMain {
		t.Fatal("меню открылось не в главном виде")
	}
	s.click(s.allBtn())
	if s.menu.View() != desktop.StartViewAllApps {
		t.Fatalf("после «Все приложения ›» вид %v", s.menu.View())
	}
	s.eng.Invalidate()
	if s.frame() == nil {
		t.Fatal("кадр «Все приложения» не получен")
	}

	// Тот же список с буквами, что у Windows 10: заголовок буквы открывает сетку букв.
	if !s.menu.OpenLetterGrid() {
		t.Fatal("сетка букв не открылась")
	}
	if !s.menu.LetterGridOpen() {
		t.Error("сетка букв не открыта")
	}
	if !s.menu.JumpToLetter("M") {
		t.Error("нет группы M")
	}
	if s.menu.LetterGridOpen() {
		t.Error("сетка букв осталась после перехода")
	}

	// Папка раскрывается на месте.
	s.menu.SetFolderExpanded("7-Zip", true)
	s.eng.Invalidate()
	savePNG(t, s.frame(), "start11_light_all_folder")

	// «‹ Назад» возвращает главный вид; запуск из списка закрывает меню.
	r := s.rect()
	s.click(image.Pt(r.Max.X-32-20, r.Min.Y+28+32+20+14))
	if s.menu.View() != desktop.StartViewMain {
		t.Errorf("после «Назад» вид %v", s.menu.View())
	}

	// Повторное открытие всегда с главного вида.
	s.menu.SetView(desktop.StartViewAllApps)
	s.menu.Close()
	s.open()
	if s.menu.View() != desktop.StartViewMain {
		t.Errorf("меню открылось с вида %v", s.menu.View())
	}
}

func TestStartMenu11_AllAppsLaunchesFromList(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.open()
	s.menu.SetView(desktop.StartViewAllApps)
	s.eng.Invalidate()
	s.frame()
	// Клавиатура: вниз — первая строка списка, Enter.
	s.key(widget.KeyDown)
	s.key(widget.KeyEnter)
	if got := launched(s); len(got) != 1 {
		t.Fatalf("Enter на строке списка запустил %v", got)
	}
}

func TestStartMenu11_RecommendedMoreView(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	var got string
	s.menu.OnRecommendedActivate = func(id string) { got = id }
	s.open()
	s.frame()
	s.click(s.moreBtn())
	if s.menu.View() != desktop.StartViewRecommended {
		t.Fatalf("после «Дополнительно ›» вид %v", s.menu.View())
	}
	s.eng.Invalidate()
	savePNG(t, s.frame(), "start11_light_more")
	// В «Все рекомендации» видны все строки, в том числе за пределами шести.
	s.key(widget.KeyDown)
	s.key(widget.KeyEnd)
	s.key(widget.KeyEnter)
	if got != "r8" {
		t.Errorf("End+Enter открыл %q, ждали последнюю r8", got)
	}
}

// Набор с клавиатуры уходит в строку поиска; результаты — вместо содержимого.
func TestStartMenu11_SearchTypingAndResults(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.open()
	s.frame()
	main := append([]uint8(nil), s.frame().Pix...)
	s.typ("ed")
	if q := s.menu.Query(); q != "ed" {
		t.Fatalf("запрос %q, ждали ed", q)
	}
	if got := s.prov.Queries; len(got) == 0 || got[len(got)-1] != "ed" {
		t.Errorf("поставщик получил запросы %v", got)
	}
	s.eng.Invalidate()
	res := s.frame()
	same := true
	for i := range main {
		if main[i] != res.Pix[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("результаты не заменили содержимое меню")
	}
	// Enter открывает первый результат.
	s.key(widget.KeyEnter)
	if got := s.prov.Activated; len(got) != 1 || got[0] != "edge" {
		t.Errorf("открыто %v, ждали [edge]", got)
	}
	if s.menu.IsOpen() {
		t.Error("меню осталось открытым после результата")
	}
}

// Набор в других областях (закреплённые, нижняя полоса) тоже уходит в поиск.
func TestStartMenu11_TypingFromAnyArea(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.open()
	s.key(widget.KeyTab)
	s.key(widget.KeyTab)
	s.key(widget.KeyTab) // нижняя полоса
	s.typ("c")
	if s.menu.Query() != "c" {
		t.Errorf("запрос %q, ждали c", s.menu.Query())
	}
	// Пробел — часть запроса только в поле; в сетке он остаётся активацией.
}

func TestStartMenu11_SearchFieldEditing(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.open()
	s.typ("abcd")
	s.key(widget.KeyLeft)
	s.key(widget.KeyBackspace)
	if q := s.menu.Query(); q != "abd" {
		t.Errorf("после Left+Backspace запрос %q, ждали abd", q)
	}
	s.key(widget.KeyHome)
	s.key(widget.KeyDelete)
	if q := s.menu.Query(); q != "bd" {
		t.Errorf("после Home+Delete запрос %q, ждали bd", q)
	}
	s.key(widget.KeyEnd)
	s.typ("Я ")
	if q := s.menu.Query(); q != "bdЯ " {
		t.Errorf("запрос %q, ждали %q", q, "bdЯ ")
	}
	// Щелчок в поле ставит каретку.
	s.click(s.searchPt())
	s.typ("!")
	if !strings.Contains(s.menu.Query(), "!") {
		t.Errorf("запрос %q потерял символ", s.menu.Query())
	}
	// Программный запрос ставит каретку в конец.
	s.menu.SetQuery("xyz")
	s.typ("1")
	if s.menu.Query() != "xyz1" {
		t.Errorf("запрос %q, ждали xyz1", s.menu.Query())
	}
}

func TestStartMenu11_EscClosesAndClearsQuery(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	s.open()
	s.typ("ed")
	s.key(widget.KeyEscape)
	if s.menu.IsOpen() {
		t.Error("Esc не закрыл меню")
	}
	if s.menu.Query() != "" {
		t.Errorf("запрос %q остался после закрытия", s.menu.Query())
	}
	s.open()
	if s.menu.Query() != "" {
		t.Error("меню открылось с прошлым запросом")
	}
}

// Tab: поиск → закреплённые → рекомендуемые → нижняя полоса → поиск.
func TestStartMenu11_TabOrder(t *testing.T) {
	s := newWin11Scene(t, 1280, 800, false, 1)
	var rec string
	var side []string
	s.menu.OnRecommendedActivate = func(id string) { rec = id }
	s.menu.OnSidebarActivate = func(id string) { side = append(side, id) }
	s.menu.PowerMenu = nil // без меню питание сообщает событием боковой панели
	s.open()

	s.key(widget.KeyTab) // закреплённые: выбрана первая ячейка
	s.key(widget.KeyRight)
	s.key(widget.KeyEnter)
	if got := launched(s); len(got) != 1 || got[0] != "app1" {
		t.Fatalf("закреплённые: запущено %v, ждали [app1]", got)
	}

	s.open()
	s.key(widget.KeyTab)
	s.key(widget.KeyTab) // рекомендуемые
	s.key(widget.KeyEnter)
	if rec != "r1" {
		t.Errorf("рекомендуемые: открыто %q, ждали r1", rec)
	}

	s.open()
	s.key(widget.KeyTab)
	s.key(widget.KeyTab)
	s.key(widget.KeyTab) // нижняя полоса: пользователь
	s.key(widget.KeyEnter)
	if len(side) != 1 || side[0] != "user" {
		t.Errorf("нижняя полоса: события %v, ждали [user]", side)
	}

	// Полный круг: четвёртый Tab возвращает в поиск, набор идёт в строку.
	s.open()
	for i := 0; i < 4; i++ {
		s.key(widget.KeyTab)
	}
	s.typ("q")
	if s.menu.Query() != "q" {
		t.Errorf("после круга Tab запрос %q, ждали q", s.menu.Query())
	}
	// Shift+Tab идёт в обратную сторону.
	s.menu.Close()
	s.open()
	s.key(widget.KeyTab, widget.ModShift) // из поиска — на нижнюю полосу
	s.key(widget.KeyRight)
	s.key(widget.KeyEnter)
	if len(side) != 2 || side[1] != "power" {
		t.Errorf("Shift+Tab, Вправо, Enter: события %v, ждали power вторым", side)
	}
}
