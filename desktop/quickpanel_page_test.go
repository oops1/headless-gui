package desktop_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/desktop"
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"github.com/oops1/headless-gui/v4/theme"
	"github.com/oops1/headless-gui/v4/widget"
)

// Страница-строитель вложенной страницы быстрых настроек: список, ползунок,
// переключатель, кнопка, подпись. Цвета — из профиля темы, без ApplyThemeTree.

// pageScene — панель с открытой страницей Wi-Fi, собранной строителем.
type pageScene struct {
	*qsScene
	page     *desktop.DetailPage
	list     *desktop.DetailList
	slider   *desktop.DetailSlider
	toggle   *desktop.DetailToggle
	button   *desktop.DetailButton
	selected []int
	levels   []float64
	toggles  []bool
	clicks   int
}

func newPageScene(t *testing.T, o qsOpts) *pageScene {
	t.Helper()
	ps := &pageScene{}
	prev := o.setup
	o.setup = func(q *desktop.QuickSettings) {
		if prev != nil {
			prev(q)
		}
		q.Details = func(id desktop.QuickActionID) *desktop.QuickDetails {
			p := q.NewDetailPage()
			ps.page = p
			ps.list = p.AddList([]desktop.DetailListItem{
				{Title: "Avanpost", Subtitle: "Защищённая сеть", State: "Подключено", Selected: true, Icon: ncGlyph(32, color.RGBA{A: 255}, 1)},
				{Title: "Guest", Subtitle: "Открытая сеть", Icon: ncGlyph(32, color.RGBA{A: 255}, 1)},
				{Title: "Недоступная", Subtitle: "Нет сигнала", Disabled: true},
			}, func(i int, _ desktop.DetailListItem) { ps.selected = append(ps.selected, i) })
			ps.slider = p.AddSlider(nil, 0.3, func(v float64) { ps.levels = append(ps.levels, v) })
			ps.toggle = p.AddToggle("Лимитное подключение", false, func(on bool) { ps.toggles = append(ps.toggles, on) })
			ps.button = p.AddButton("Другие сети", func() { ps.clicks++ })
			ps.button.SetAccent(true)
			return p.Details("Wi-Fi")
		}
	}
	ps.qsScene = qsBuild(t, o)
	qsClick(ps.q, centreOf(desktop.QSChevronRect(ps.q, "wifi")))
	qsFinish()
	ps.render()
	if ps.page == nil {
		t.Fatal("страница не запрошена")
	}
	return ps
}

func TestDetailPage_LayoutAndBounds(t *testing.T) {
	testreset.Globals(t)
	s := newPageScene(t, qsOpts{})
	body := desktop.QSZoneRect(s.q, "body")
	if s.page.Bounds() != body {
		t.Fatalf("границы страницы %v, ждали область под заголовком %v", s.page.Bounds(), body)
	}
	prevBottom := body.Min.Y
	for i, sub := range []int{0, 0, 0, 0} {
		_ = sub
		r := desktop.DPRect(s.page, i, 0)
		if r.Empty() || !r.In(body) {
			t.Errorf("элемент %d: %v вне области %v", i, r, body)
		}
		if r.Min.Y < prevBottom {
			t.Errorf("элемент %d наезжает на предыдущий", i)
		}
		prevBottom = r.Max.Y
	}
	qsSave(t, s.settled(), "qs_page_light")
	if got := desktop.DPRect(s.page, 0, 2); got.Dy() != 48 {
		t.Errorf("строка списка с подписью %v, ждали высоту 48", got)
	}
}

func TestDetailPage_ListSelectToggleButton(t *testing.T) {
	testreset.Globals(t)
	s := newPageScene(t, qsOpts{})
	q := s.q
	// Строка списка: выбор и вызов onSelect; недоступная не нажимается.
	qsClick(q, centreOf(desktop.DPRect(s.page, 0, 1)))
	if len(s.selected) != 1 || s.selected[0] != 1 || s.list.Selected() != 1 {
		t.Fatalf("выбор строки: вызовы %v, выбрана %d", s.selected, s.list.Selected())
	}
	qsClick(q, centreOf(desktop.DPRect(s.page, 0, 2)))
	if len(s.selected) != 1 || s.list.Selected() != 1 {
		t.Errorf("недоступная строка выбрана: %v %d", s.selected, s.list.Selected())
	}
	if id, open := q.DetailsOpen(); !open || id != "wifi" {
		t.Fatalf("щелчок по содержимому закрыл страницу: %q %v", id, open)
	}
	// Переключатель.
	qsClick(q, centreOf(desktop.DPRect(s.page, 2, 0)))
	if len(s.toggles) != 1 || !s.toggles[0] || !s.toggle.On() {
		t.Errorf("переключатель: %v, On=%v", s.toggles, s.toggle.On())
	}
	qsClick(q, centreOf(desktop.DPRect(s.page, 2, 0)))
	if len(s.toggles) != 2 || s.toggles[1] || s.toggle.On() {
		t.Errorf("повторное нажатие: %v", s.toggles)
	}
	// SetOn не зовёт onChange.
	s.toggle.SetOn(true)
	if len(s.toggles) != 2 {
		t.Error("SetOn позвал onChange")
	}
	// Кнопка.
	qsClick(q, centreOf(desktop.DPRect(s.page, 3, 0)))
	if s.clicks != 1 {
		t.Errorf("кнопка нажата %d раз", s.clicks)
	}
	s.button.SetEnabled(false)
	qsClick(q, centreOf(desktop.DPRect(s.page, 3, 0)))
	if s.clicks != 1 {
		t.Error("отключённая кнопка нажалась")
	}
}

// Ползунок страницы тянется мышью; отпускание за краем страницы и панели его
// не «залипает».
func TestDetailPage_SliderDragAndRelease(t *testing.T) {
	testreset.Globals(t)
	s := newPageScene(t, qsOpts{})
	q := s.q
	row := desktop.DPRect(s.page, 1, 0)
	mid := image.Pt(row.Min.X+row.Dx()/2, row.Min.Y+row.Dy()/2)
	q.OnMouseButton(widget.MouseEvent{X: mid.X, Y: mid.Y, Button: widget.MouseLeft, Pressed: true})
	if len(s.levels) == 0 || s.levels[0] < 0.3 || s.levels[0] > 0.7 {
		t.Fatalf("нажатие на середину дорожки: %v", s.levels)
	}
	// Курсор ушёл далеко за страницу — ползунок упирается в край.
	q.OnMouseMove(row.Max.X+200, mid.Y)
	if last := s.levels[len(s.levels)-1]; last != 1 || s.slider.Level() != 1 {
		t.Errorf("перетаскивание за край: %v, уровень %v", s.levels, s.slider.Level())
	}
	q.OnMouseButton(widget.MouseEvent{X: row.Max.X + 200, Y: mid.Y, Button: widget.MouseLeft})
	n := len(s.levels)
	q.OnMouseMove(mid.X, mid.Y)
	if len(s.levels) != n {
		t.Error("после отпускания ползунок продолжил следовать за мышью")
	}
	// SetLevel не зовёт onChange.
	s.slider.SetLevel(0.1)
	if len(s.levels) != n || s.slider.Level() != 0.1 {
		t.Errorf("SetLevel: вызовы %v, уровень %v", s.levels, s.slider.Level())
	}
	// Неактивный ползунок не двигается.
	s.slider.SetEnabled(false)
	q.OnMouseButton(widget.MouseEvent{X: mid.X, Y: mid.Y, Button: widget.MouseLeft, Pressed: true})
	q.OnMouseButton(widget.MouseEvent{X: mid.X, Y: mid.Y, Button: widget.MouseLeft})
	if len(s.levels) != n {
		t.Error("неактивный ползунок страницы отозвался")
	}
}

// Смена профиля и акцента перекрашивает страницу сама — без ApplyThemeTree.
func TestDetailPage_RecolorsWithThemeAndAccent(t *testing.T) {
	testreset.Globals(t)
	s := newPageScene(t, qsOpts{})
	list := desktop.DPRect(s.page, 0, 0)
	btn := desktop.DPRect(s.page, 3, 0)
	listPt := image.Pt(list.Max.X-60, list.Min.Y+4)
	btnPt := image.Pt(btn.Min.X+6, btn.Min.Y+4)
	img := s.settled()
	lightList, lightBtn := img.RGBAAt(listPt.X, listPt.Y), img.RGBAAt(btnPt.X, btnPt.Y)

	if err := s.tm.SetTheme(theme.ProfileWindows11Dark); err != nil {
		t.Fatal(err)
	}
	img = s.settled()
	if img.RGBAAt(listPt.X, listPt.Y) == lightList {
		t.Error("выбранная строка не перекрасилась в тёмной теме")
	}
	darkBtn := img.RGBAAt(btnPt.X, btnPt.Y)
	s.tm.SetAccent(theme.RGB(200, 40, 40))
	img = s.settled()
	red := img.RGBAAt(btnPt.X, btnPt.Y)
	if red == darkBtn || red.R < 150 || red.G > 100 {
		t.Errorf("акцентная кнопка после смены акцента: %v (было %v, %v)", red, lightBtn, darkBtn)
	}
}

// Клавиатура: Tab переводит на содержимое, стрелки ходят по строкам, Enter
// выбирает, стрелки двигают ползунок.
func TestDetailPage_Keyboard(t *testing.T) {
	testreset.Globals(t)
	s := newPageScene(t, qsOpts{})
	q := s.q
	q.SetFocused(true)
	qsKey(q, widget.KeyTab, 0) // с «назад» на содержимое
	if !s.page.IsFocused() {
		t.Fatal("страница не получила клавиатуру")
	}
	qsKey(q, widget.KeyDown, 0) // первая строка
	qsKey(q, widget.KeyDown, 0) // вторая
	qsKey(q, widget.KeyEnter, 0)
	if len(s.selected) != 1 || s.selected[0] != 1 {
		t.Fatalf("Enter на второй строке: %v", s.selected)
	}
	// Недоступная строка в обход не входит: следующая — ползунок.
	qsKey(q, widget.KeyDown, 0)
	qsKey(q, widget.KeyRight, 0)
	if len(s.levels) != 1 || s.levels[0] < 0.34 || s.levels[0] > 0.36 {
		t.Errorf("→ на ползунке (0,30): %v", s.levels)
	}
	qsKey(q, widget.KeyDown, 0)
	qsKey(q, widget.KeyEnter, 0)
	if len(s.toggles) != 1 {
		t.Errorf("Enter на переключателе: %v", s.toggles)
	}
	qsKey(q, widget.KeyDown, 0)
	qsKey(q, widget.KeySpace, 0)
	if s.clicks != 1 {
		t.Errorf("пробел на кнопке: %d", s.clicks)
	}
	// Tab возвращает на «назад», страница клавиатуру отдаёт.
	qsKey(q, widget.KeyTab, 0)
	if s.page.IsFocused() {
		t.Error("страница держит клавиатуру после Tab")
	}
}

// Страница выше области прокручивается колесом; рамка и содержимое не
// выходят за границы.
func TestDetailPage_ScrollsWhenTooTall(t *testing.T) {
	testreset.Globals(t)
	var p *desktop.DetailPage
	s := qsBuild(t, qsOpts{setup: func(q *desktop.QuickSettings) {
		q.Details = func(desktop.QuickActionID) *desktop.QuickDetails {
			p = q.NewDetailPage()
			var items []desktop.DetailListItem
			for i := 0; i < 12; i++ {
				items = append(items, desktop.DetailListItem{Title: "Сеть", Subtitle: "Защищённая"})
			}
			p.AddList(items, nil)
			return p.Details("Wi-Fi")
		}
	}})
	qsClick(s.q, centreOf(desktop.QSChevronRect(s.q, "wifi")))
	qsFinish()
	s.render()
	body := desktop.QSZoneRect(s.q, "body")
	if !s.q.OnMouseWheelPixels(body.Min.X+20, body.Min.Y+20, 0, 100) {
		t.Fatal("колесо над страницей не принято")
	}
	if desktop.DPScroll(p) == 0 {
		t.Error("колесо не прокрутило высокую страницу")
	}
	s.q.OnMouseWheelPixels(body.Min.X+20, body.Min.Y+20, 0, 100000)
	last := desktop.DPRect(p, 0, 11)
	if last.Max.Y > body.Max.Y || last.Max.Y < body.Max.Y-1 {
		t.Errorf("прокрутка до конца: последняя строка %v, область %v", last, body)
	}
	s.render()
}

// SetItems, Clear и пустая страница не падают.
func TestDetailPage_SetItemsAndClear(t *testing.T) {
	testreset.Globals(t)
	s := newPageScene(t, qsOpts{})
	s.list.SetItems([]desktop.DetailListItem{{Title: "A"}, {Title: "B", Selected: true}})
	if s.list.Selected() != 1 || len(s.list.Items()) != 2 {
		t.Errorf("SetItems: выбрана %d, строк %d", s.list.Selected(), len(s.list.Items()))
	}
	s.list.SetSelected(0)
	if s.list.Selected() != 0 {
		t.Error("SetSelected")
	}
	s.render()
	s.page.Clear()
	s.render()
	qsClick(s.q, centreOf(desktop.QSZoneRect(s.q, "body")))
	if _, open := s.q.DetailsOpen(); !open {
		t.Error("щелчок по пустой странице закрыл её")
	}
}
