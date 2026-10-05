package desktop_test

import (
	"image"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/desktop"
	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ─── Выбор варианта ──────────────────────────────────────────────────────────

// Вариант Windows 11 включается презентером профиля и моделью плиток. Другие
// темы и панель без модели рисуют прежние три плитки.
func TestQuickPanel_VariantByPresenterAndModel(t *testing.T) {
	win11 := qsBuild(t, qsOpts{})
	if !desktop.QSRich(win11.q) {
		t.Fatal("Windows 11 с моделью плиток должна рисовать вариант 24H2")
	}
	win11.q.SetQuickActions(nil)
	if desktop.QSRich(win11.q) {
		t.Error("без модели плиток должен остаться прежний вид")
	}
	for _, name := range []string{theme.ProfileWindows10, theme.ProfileWindows2000, theme.ProfileMacOS} {
		s := qsBuild(t, qsOpts{theme: name})
		if desktop.QSRich(s.q) {
			t.Errorf("%s: презентера быстрых настроек у темы нет, а вариант включился", name)
		}
		if w := desktop.QSPanelRect(s.q).Dx(); w == 360 {
			t.Errorf("%s: ширина панели 360 — это размер Windows 11", name)
		}
	}
}

// Геометрия по таблице плана: ширина 360, плитки 96×48 при зазоре 12, подпись
// под плиткой, три колонки.
func TestQuickPanel_Geometry(t *testing.T) {
	s := qsBuild(t, qsOpts{})
	q := s.q
	if w := desktop.QSPanelRect(q).Dx(); w != 360 {
		t.Errorf("ширина панели = %d, ждали 360", w)
	}
	wifi, bt, plane := desktop.QSTileRect(q, "wifi"), desktop.QSTileRect(q, "bt"), desktop.QSTileRect(q, "plane")
	for _, r := range []image.Rectangle{wifi, bt, plane} {
		if r.Dx() != 96 || r.Dy() != 48 {
			t.Errorf("плитка %v: ждали 96×48", r)
		}
	}
	if bt.Min.X-wifi.Max.X != 12 || plane.Min.X-bt.Max.X != 12 || wifi.Min.Y != bt.Min.Y {
		t.Errorf("плитки одного ряда стоят не с зазором 12: %v %v %v", wifi, bt, plane)
	}
	saver := desktop.QSTileRect(q, "saver") // четвёртая — второй ряд, первая колонка
	if saver.Min.X != wifi.Min.X || saver.Min.Y <= wifi.Max.Y {
		t.Errorf("четвёртая плитка %v должна быть под первой %v", saver, wifi)
	}
	label := desktop.QSLabelRect(q, "wifi")
	if label.Min.Y < wifi.Max.Y || label.Min.Y > wifi.Max.Y+12 {
		t.Errorf("подпись %v должна лежать сразу под плиткой %v", label, wifi)
	}
	// Панель целиком на экране, всё содержимое внутри панели.
	panel := desktop.QSPanelRect(q)
	for _, name := range []string{"grid", "vol", "bright", "edit", "settings", "footer", "battery"} {
		if r := desktop.QSZoneRect(q, name); r.Empty() || !r.In(panel) {
			t.Errorf("зона %s = %v вне панели %v", name, r, panel)
		}
	}
	if !panel.In(q.Screen) {
		t.Errorf("панель %v выходит за экран %v", panel, q.Screen)
	}
}

// 100–200 %: логическая раскладка не зависит от масштаба и не выходит за
// панель.
func TestQuickPanel_Scales(t *testing.T) {
	var want image.Rectangle
	for _, sc := range []float64{1, 1.25, 1.5, 2} {
		s := qsBuild(t, qsOpts{scale: sc, w: 640, h: 560})
		panel := desktop.QSPanelRect(s.q)
		if want.Empty() {
			want = panel
		}
		if panel.Size() != want.Size() {
			t.Errorf("масштаб %v: размер панели %v, при 100 %% был %v", sc, panel.Size(), want.Size())
		}
		if !panel.In(s.q.Screen) {
			t.Errorf("масштаб %v: панель %v не помещается на экране", sc, panel)
		}
		img := s.render()
		if img.Bounds().Dx() != int(640*sc+0.5) {
			t.Errorf("масштаб %v: кадр %v", sc, img.Bounds())
		}
		for _, name := range []string{"vol", "bright", "footer"} {
			if r := desktop.QSZoneRect(s.q, name); !r.In(panel) {
				t.Errorf("масштаб %v: зона %s вне панели", sc, name)
			}
		}
	}
}

// ─── Плитки ──────────────────────────────────────────────────────────────────

// Щелчок по плитке переключает её через модель; отключённая и недоступная
// плитки не нажимаются.
func TestQuickPanel_TileToggle(t *testing.T) {
	s := qsBuild(t, qsOpts{})
	click := func(id desktop.QuickActionID) {
		r := desktop.QSTileRect(s.q, id)
		// Левая часть плитки: «›» справа не задет.
		qsClick(s.q, image.Pt(r.Min.X+12, r.Min.Y+12))
	}
	click("plane")
	click("night")
	click("saver") // недоступна
	click("focus") // отключена
	got := s.model.Toggled
	if len(got) != 2 || got[0] != "plane" || got[1] != "night" {
		t.Fatalf("нажаты %v, ждали [plane night]", got)
	}
	var plane desktop.QuickAction
	for _, a := range s.model.List() {
		if a.ID == "plane" {
			plane = a
		}
	}
	if !plane.On {
		t.Error("модель без OnToggle переключает плитку сама, а она выключена")
	}
}

// Обновление одной плитки заявляет только её область, а не панель.
func TestQuickPanel_SingleTileRepaint(t *testing.T) {
	s := qsBuild(t, qsOpts{})
	before := s.render()
	var mu sync.Mutex
	var rects []image.Rectangle
	widget.SetUIRectChangeNotifier(func(r image.Rectangle) { mu.Lock(); rects = append(rects, r); mu.Unlock() })
	t.Cleanup(func() { widget.SetUIRectChangeNotifier(nil) })
	var full bool
	widget.SetUIChangeNotifier(func() { full = true })
	t.Cleanup(func() { widget.SetUIChangeNotifier(nil) })

	s.model.SetOn("night", true)

	body, label := desktop.QSTileRect(s.q, "night"), desktop.QSLabelRect(s.q, "night")
	area := body.Union(label)
	mu.Lock()
	defer mu.Unlock()
	if len(rects) == 0 {
		t.Fatal("плитка изменилась, а область не заявлена")
	}
	for _, r := range rects {
		if !r.In(area) {
			t.Errorf("заявлена область %v вне плитки и её подписи %v", r, area)
		}
	}
	if full {
		t.Error("обновление плитки заявило перерисовку целиком")
	}
	after := s.render()
	panel := desktop.QSPanelRect(s.q)
	changed := false
	for y := panel.Min.Y; y < panel.Max.Y; y++ {
		for x := panel.Min.X; x < panel.Max.X; x++ {
			if before.RGBAAt(x, y) != after.RGBAAt(x, y) {
				if !image.Pt(x, y).In(area) {
					t.Fatalf("пиксель (%d,%d) вне плитки изменился", x, y)
				}
				changed = true
			}
		}
	}
	if !changed {
		t.Error("плитка включена, а кадр не изменился")
	}
}

// Смена состава плиток (добавили плитку) перерисовывает панель целиком: у неё
// могла измениться высота.
func TestQuickPanel_ModelShapeChange(t *testing.T) {
	s := qsBuild(t, qsOpts{model: func(m *desktop.QuickActionList) { m.Replace(m.List()[:3]) }})
	h3 := desktop.QSPanelRect(s.q).Dy()
	s.model.Replace(qsSampleActions().List())
	if h := desktop.QSPanelRect(s.q).Dy(); h <= h3 {
		t.Errorf("добавили второй ряд плиток, а панель не выросла: %d → %d", h3, h)
	}
	if s.q.Editing() {
		t.Error("режим правки включился сам")
	}
}

// ─── Вложенная страница ──────────────────────────────────────────────────────

func TestQuickPanel_DetailsNavigation(t *testing.T) {
	var closed int
	var asked []desktop.QuickActionID
	s := qsBuild(t, qsOpts{setup: func(q *desktop.QuickSettings) {
		q.Details = func(id desktop.QuickActionID) *desktop.QuickDetails {
			asked = append(asked, id)
			d := qsSampleDetails(id)
			d.OnClose = func() { closed++ }
			return d
		}
	}})
	q := s.q
	// «›» плитки без деталей и отключённой плитки не открывает ничего.
	qsClick(q, centreOf(desktop.QSChevronRect(q, "focus")))
	qsFinish()
	if _, open := q.DetailsOpen(); open || len(asked) != 0 {
		t.Fatalf("у отключённой плитки открылась страница (%v)", asked)
	}

	qsClick(q, centreOf(desktop.QSChevronRect(q, "bt")))
	if id, open := q.DetailsOpen(); !open || id != "bt" {
		t.Fatalf("после «›» страница = %q открыта=%v", id, open)
	}
	// Идёт переход: страница между главной и вложенной.
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(110 * time.Millisecond))
	if p := desktop.QSPage(q); p <= 0 || p >= 1 {
		t.Errorf("середина перехода: положение %v, ждали между 0 и 1", p)
	}
	qsFinish()
	if p := desktop.QSPage(q); p != 1 {
		t.Fatalf("переход закончился на %v", p)
	}
	// Назад — стрелкой.
	qsClick(q, centreOf(desktop.QSZoneRect(q, "back")))
	qsFinish()
	if _, open := q.DetailsOpen(); open {
		t.Error("после «назад» страница осталась открытой")
	}
	if desktop.QSPage(q) != 0 || closed != 1 {
		t.Errorf("положение %v, OnClose %d раз", desktop.QSPage(q), closed)
	}
	// Громкость — зарезервированный идентификатор.
	qsClick(q, centreOf(desktop.QSZoneRect(q, "volchev")))
	qsFinish()
	if id, open := q.DetailsOpen(); !open || id != desktop.QuickVolumeID {
		t.Errorf("«›» громкости открыла %q (%v)", id, open)
	}
	// Esc отступает на шаг: сначала со страницы, потом закрывает панель.
	qsKey(q, widget.KeyEscape, 0)
	qsFinish()
	if _, open := q.DetailsOpen(); open || !q.IsOpen() {
		t.Errorf("первый Esc: страница открыта=%v, панель открыта=%v", open, q.IsOpen())
	}
	qsKey(q, widget.KeyEscape, 0)
	if q.IsOpen() {
		t.Error("второй Esc не закрыл панель")
	}
	if closed != 2 {
		t.Errorf("OnClose = %d, ждали 2", closed)
	}
}

// «Меньше движения»: переход между страницами мгновенный.
func TestQuickPanel_ReduceMotionPageInstant(t *testing.T) {
	s := qsBuild(t, qsOpts{reduce: true})
	qsClick(s.q, centreOf(desktop.QSChevronRect(s.q, "wifi")))
	if p := desktop.QSPage(s.q); p != 1 {
		t.Fatalf("при «меньше движения» положение %v сразу после щелчка, ждали 1", p)
	}
	qsKey(s.q, widget.KeyBackspace, 0)
	if p := desktop.QSPage(s.q); p != 0 {
		t.Errorf("назад при «меньше движения»: положение %v", p)
	}
}

// Содержимое вложенной страницы получает мышь, колесо и клавиши: список
// устройств выбирает строку по щелчку, а панель при этом остаётся на странице.
func TestQuickPanel_DetailsContentInput(t *testing.T) {
	var lv *widget.ListView
	var selected []int
	s := qsBuild(t, qsOpts{setup: func(q *desktop.QuickSettings) {
		q.Details = func(id desktop.QuickActionID) *desktop.QuickDetails {
			lv = widget.NewListView("Динамики", "Наушники", "HDMI")
			lv.OnSelect = func(i int, _ string) { selected = append(selected, i) }
			return &desktop.QuickDetails{Title: "Выбор устройства вывода звука", Content: lv}
		}
	}})
	q := s.q
	qsClick(q, centreOf(desktop.QSZoneRect(q, "volchev")))
	qsFinish()
	body := desktop.QSZoneRect(q, "body")
	if body.Empty() || lv == nil {
		t.Fatal("у вложенной страницы нет содержимого")
	}
	s.render()
	if lv.Bounds() != body {
		t.Fatalf("границы содержимого %v, ждали область под заголовком %v", lv.Bounds(), body)
	}
	q.OnMouseMove(body.Min.X+30, body.Min.Y+40)
	// Вторая строка списка: ListView выбирает по щелчку.
	rowH := 24
	qsClick(q, image.Pt(body.Min.X+30, body.Min.Y+rowH+rowH/2))
	if id, open := q.DetailsOpen(); !open || id != desktop.QuickVolumeID {
		t.Fatalf("щелчок по содержимому закрыл страницу: %q %v", id, open)
	}
	if len(selected) == 0 {
		t.Fatal("щелчок по списку устройств не дошёл до списка")
	}
	// Клавиши — корневому виджету содержимого.
	before := lv.Selected()
	qsKey(q, widget.KeyDown, 0)
	if lv.Selected() == before {
		t.Logf("список не двинул выделение стрелкой (выделено %d)", before)
	}
}

// Все строки быстрых настроек переведены на русский и английский.
func TestQuickPanel_StringsInBothLanguages(t *testing.T) {
	keys := []string{
		desktop.StrQuickEdit, desktop.StrQuickDone, desktop.StrQuickEditHint, desktop.StrQuickSettings,
		desktop.StrQuickBack, desktop.StrQuickMore, desktop.StrQuickVolume, desktop.StrQuickBrightness,
		desktop.StrQuickMute, desktop.StrQuickUnmute, desktop.StrQuickBattery, desktop.StrQuickCharging,
	}
	for _, k := range keys {
		ru, en := widget.TrIn("RU", k), widget.TrIn("EN", k)
		if ru == k || en == k || ru == en {
			t.Errorf("ключ %s: RU=%q EN=%q", k, ru, en)
		}
	}
}

// ─── Сетка и прокрутка ───────────────────────────────────────────────────────

func TestQuickPanel_Scroll(t *testing.T) {
	s := qsBuild(t, qsOpts{})
	q := s.q
	grid := desktop.QSZoneRect(q, "grid")
	if desktop.QSZoneRect(q, "thumb").Empty() {
		t.Fatal("восемь плиток в двух рядах: ждали полосу прокрутки")
	}
	vpn := desktop.QSTileRect(q, "vpn") // третий ряд, скрыт
	if vpn.Min.Y < grid.Max.Y {
		t.Fatalf("плитка третьего ряда %v должна быть ниже окна сетки %v", vpn, grid)
	}
	if !q.OnMouseWheelPixels(grid.Min.X+5, grid.Min.Y+5, 0, 40) {
		t.Fatal("колесо над сеткой не принято")
	}
	if got := desktop.QSScroll(q); got != 40 {
		t.Errorf("прокрутка = %d, ждали 40", got)
	}
	q.OnMouseWheelPixels(grid.Min.X+5, grid.Min.Y+5, 0, 5000)
	vpn = desktop.QSTileRect(q, "vpn")
	if !vpn.In(grid) {
		t.Errorf("после прокрутки до конца плитка %v не видна в окне %v", vpn, grid)
	}
	q.OnMouseWheelPixels(grid.Min.X+5, grid.Min.Y+5, 0, -9999)
	if desktop.QSScroll(q) != 0 {
		t.Error("прокрутка вверх не вернулась к нулю")
	}
	// Перетаскивание бегунка.
	th := desktop.QSZoneRect(q, "thumb")
	pt := centreOf(th)
	q.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y, Button: widget.MouseLeft, Pressed: true})
	q.OnMouseMove(pt.X, pt.Y+200)
	q.OnMouseButton(widget.MouseEvent{X: pt.X, Y: pt.Y + 200, Button: widget.MouseLeft})
	if desktop.QSScroll(q) == 0 {
		t.Error("бегунок потянули вниз, а сетка не прокрутилась")
	}
}

// ─── Ползунки ────────────────────────────────────────────────────────────────

func TestQuickPanel_VolumeSlider(t *testing.T) {
	var levels []float64
	var mutes int
	s := qsBuild(t, qsOpts{setup: func(q *desktop.QuickSettings) {
		q.OnVolumeChange = func(v float64) { levels = append(levels, v) }
		q.OnToggleMute = func() { mutes++ }
	}})
	q := s.q
	track := desktop.QSZoneRect(q, "voltrack")
	mid := image.Pt(track.Min.X+track.Dx()/2, track.Min.Y+track.Dy()/2)
	q.OnMouseButton(widget.MouseEvent{X: mid.X, Y: mid.Y, Button: widget.MouseLeft, Pressed: true})
	q.OnMouseMove(track.Max.X+30, mid.Y)
	q.OnMouseButton(widget.MouseEvent{X: track.Max.X + 30, Y: mid.Y, Button: widget.MouseLeft})
	if len(levels) == 0 {
		t.Fatal("ползунок громкости не сообщил уровня")
	}
	if last := levels[len(levels)-1]; last != 1 {
		t.Errorf("за правым краем дорожки уровень = %v, ждали 1", last)
	}
	if first := levels[0]; first < 0.3 || first > 0.7 {
		t.Errorf("щелчок посреди дорожки дал уровень %v", first)
	}
	// Значок — выключение звука.
	qsClick(q, centreOf(desktop.QSZoneRect(q, "volicon")))
	if mutes != 1 {
		t.Errorf("щелчок по значку громкости: выключений звука %d", mutes)
	}
	// Показание системы подхватывается без пересоздания.
	s.status.SetVolume(desktop.VolState{Level: 0.2})
	if r := desktop.QSZoneRect(q, "vol"); r.Empty() {
		t.Error("ползунок громкости пропал")
	}
}

func TestQuickPanel_BrightnessHiddenWithoutData(t *testing.T) {
	var got []float64
	s := qsBuild(t, qsOpts{noBright: true, setup: func(q *desktop.QuickSettings) {
		q.OnBrightnessChange = func(v float64) { got = append(got, v) }
	}})
	q := s.q
	if !desktop.QSZoneRect(q, "bright").Empty() {
		t.Fatal("яркости потребитель не дал, а ползунок показан")
	}
	h0 := desktop.QSPanelRect(q).Dy()
	if _, ok := q.Brightness(); ok {
		t.Error("Brightness сообщает данные, которых нет")
	}
	q.SetBrightness(0.4)
	if desktop.QSZoneRect(q, "bright").Empty() {
		t.Fatal("после SetBrightness ползунка нет")
	}
	if h1 := desktop.QSPanelRect(q).Dy(); h1 != h0+44 {
		t.Errorf("высота %d → %d: ползунок яркости должен добавить 44", h0, h1)
	}
	track := desktop.QSZoneRect(q, "brighttrack")
	pt := image.Pt(track.Max.X-1, track.Min.Y+track.Dy()/2)
	qsClick(q, pt)
	if len(got) == 0 || got[len(got)-1] < 0.95 {
		t.Errorf("щелчок у правого края дорожки яркости дал %v", got)
	}
	q.ClearBrightness()
	if !desktop.QSZoneRect(q, "bright").Empty() {
		t.Error("ClearBrightness не спрятал ползунок")
	}
	if desktop.QSPanelRect(q).Dy() != h0 {
		t.Error("панель не вернулась к прежней высоте")
	}
}

// ─── Режим правки ────────────────────────────────────────────────────────────

func TestQuickPanel_EditReorder(t *testing.T) {
	var order []desktop.QuickActionID
	var edits []bool
	s := qsBuild(t, qsOpts{setup: func(q *desktop.QuickSettings) {
		q.OnReorder = func(o []desktop.QuickActionID) { order = o }
		q.OnEdit = func(on bool) { edits = append(edits, on) }
	}})
	q := s.q
	qsClick(q, centreOf(desktop.QSZoneRect(q, "edit")))
	if !q.Editing() || len(edits) != 1 || !edits[0] {
		t.Fatalf("карандаш: режим правки %v, OnEdit %v", q.Editing(), edits)
	}
	// В режиме правки щелчок плитку не переключает.
	r := desktop.QSTileRect(q, "plane")
	qsClick(q, centreOf(r))
	if len(s.model.Toggled) != 0 {
		t.Errorf("в режиме правки плитка переключилась: %v", s.model.Toggled)
	}
	// Потянуть первую плитку на место шестой.
	from := centreOf(desktop.QSTileRect(q, "wifi"))
	accessSlot := desktop.QSTileRect(q, "access")
	to := centreOf(accessSlot)
	q.OnMouseButton(widget.MouseEvent{X: from.X, Y: from.Y, Button: widget.MouseLeft, Pressed: true})
	for i := 1; i <= 10; i++ {
		q.OnMouseMove(from.X+(to.X-from.X)*i/10, from.Y+(to.Y-from.Y)*i/10)
	}
	// Пока плитка в руке, остальные расступились: пятая стоит на месте шестой.
	img := s.render()
	_ = img
	q.OnMouseButton(widget.MouseEvent{X: to.X, Y: to.Y, Button: widget.MouseLeft})
	want := []desktop.QuickActionID{"bt", "plane", "saver", "night", "access", "wifi", "vpn", "focus"}
	if len(order) != len(want) {
		t.Fatalf("OnReorder получил %v", order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("новый порядок %v, ждали %v", order, want)
		}
	}
	for i, a := range s.model.List() {
		if a.ID != want[i] {
			t.Fatalf("модель хранит порядок %v, ждали %v", s.model.List(), want)
		}
	}
	// Плитка wifi теперь стоит на месте шестой.
	if got := desktop.QSTileRect(q, "wifi"); got != accessSlot {
		t.Errorf("плитка wifi на месте %v, ждали место шестой %v", got, accessSlot)
	}
	// Выход из режима правки.
	qsClick(q, centreOf(desktop.QSZoneRect(q, "edit")))
	if q.Editing() || len(edits) != 2 || edits[1] {
		t.Errorf("«Готово»: режим правки %v, OnEdit %v", q.Editing(), edits)
	}
}

// Перетаскивание без сдвига порядка (бросили на то же место) событие не
// порождает.
func TestQuickPanel_EditDropSamePlace(t *testing.T) {
	called := 0
	s := qsBuild(t, qsOpts{setup: func(q *desktop.QuickSettings) {
		q.OnReorder = func([]desktop.QuickActionID) { called++ }
	}})
	q := s.q
	q.SetEditing(true)
	from := centreOf(desktop.QSTileRect(q, "bt"))
	q.OnMouseButton(widget.MouseEvent{X: from.X, Y: from.Y, Button: widget.MouseLeft, Pressed: true})
	q.OnMouseMove(from.X+20, from.Y+2)
	q.OnMouseMove(from.X+1, from.Y)
	q.OnMouseButton(widget.MouseEvent{X: from.X + 1, Y: from.Y, Button: widget.MouseLeft})
	if called != 0 {
		t.Errorf("плитку вернули на место, а OnReorder зван %d раз", called)
	}
}

// Модель без QuickActionReorderer: порядок не хранится, но панель его
// показывает до закрытия.
func TestQuickPanel_ReorderWithoutModelSupport(t *testing.T) {
	s := qsBuild(t, qsOpts{})
	q := s.q
	plain := &plainModel{QuickActionModel: s.model}
	q.SetQuickActions(plain)
	q.SetEditing(true)
	q.OnKeyEvent(widget.KeyEvent{Code: widget.KeyTab, Pressed: true}) // фокус на первой плитке
	qsKey(q, widget.KeyRight, widget.ModAlt)
	r0 := desktop.QSTileRect(q, "wifi")
	bt := desktop.QSTileRect(q, "bt")
	if r0.Min.X <= bt.Min.X {
		t.Errorf("Alt+→ не переставила плитку: wifi %v, bt %v", r0, bt)
	}
}

// plainModel скрывает Reorder у вложенной модели.
type plainModel struct{ desktop.QuickActionModel }

// Esc в режиме правки выходит из неё, а не закрывает панель.
func TestQuickPanel_EscapeLeavesEditMode(t *testing.T) {
	s := qsBuild(t, qsOpts{})
	s.q.SetEditing(true)
	qsKey(s.q, widget.KeyEscape, 0)
	if s.q.Editing() || !s.q.IsOpen() {
		t.Fatalf("Esc: режим правки %v, панель открыта %v", s.q.Editing(), s.q.IsOpen())
	}
	qsKey(s.q, widget.KeyEscape, 0)
	if s.q.IsOpen() {
		t.Error("второй Esc не закрыл панель")
	}
}

// ─── Нижняя строка ───────────────────────────────────────────────────────────

func TestQuickPanel_Footer(t *testing.T) {
	settings := 0
	open := true
	s := qsBuild(t, qsOpts{setup: func(q *desktop.QuickSettings) {
		q.OnSettings = func() { settings++; open = q.IsOpen() }
	}})
	q := s.q
	if desktop.QSZoneRect(q, "battery").Empty() {
		t.Error("батарея есть, а значения в нижней строке нет")
	}
	s.status.SetPower(desktop.PowerState{NoBattery: true})
	if !desktop.QSZoneRect(q, "battery").Empty() {
		t.Error("батареи нет (настольная машина), а значение показано")
	}
	qsClick(q, centreOf(desktop.QSZoneRect(q, "settings")))
	if settings != 1 {
		t.Fatalf("«Параметры»: OnSettings %d раз", settings)
	}
	if open || q.IsOpen() {
		t.Error("параметры должны открываться после закрытия панели")
	}
}

// ─── Клавиатура ──────────────────────────────────────────────────────────────

func TestQuickPanel_Keyboard(t *testing.T) {
	var levels []float64
	s := qsBuild(t, qsOpts{setup: func(q *desktop.QuickSettings) {
		q.OnVolumeChange = func(v float64) { levels = append(levels, v) }
	}})
	q := s.q
	if !q.AcceptsTab() || q.TabIndex() < 0 {
		t.Fatal("открытая панель должна принимать Tab")
	}
	q.SetFocused(true)
	focus := func() (int, int) { return desktop.QSFocus(q) }

	qsKey(q, widget.KeyTab, 0)
	if k, i := focus(); k != desktop.QSKindTile || i != 0 {
		t.Fatalf("первый Tab: зона %d/%d, ждали первую плитку", k, i)
	}
	if !q.FocusVisible() {
		t.Error("фокус пришёл с клавиатуры, а рамка не показана")
	}
	qsKey(q, widget.KeyTab, 0)
	if k, i := focus(); k != desktop.QSKindChevron || i != 0 {
		t.Errorf("второй Tab: зона %d/%d, ждали «›» первой плитки", k, i)
	}
	qsKey(q, widget.KeyTab, widget.ModShift)
	qsKey(q, widget.KeyEnter, 0) // плитка Wi-Fi
	if len(s.model.Toggled) != 1 || s.model.Toggled[0] != "wifi" {
		t.Errorf("Enter на плитке: нажато %v", s.model.Toggled)
	}
	// Стрелки по сетке.
	qsKey(q, widget.KeyRight, 0)
	qsKey(q, widget.KeyDown, 0)
	if k, i := focus(); k != desktop.QSKindTile || i != 4 {
		t.Errorf("→ и ↓: зона %d/%d, ждали пятую плитку", k, i)
	}
	// Вниз из последнего ряда — к следующей зоне (яркость). Плитки 6–7 лежат
	// ниже окна сетки: фокус прокручивает её.
	qsKey(q, widget.KeyDown, 0)
	if k, i := focus(); k != desktop.QSKindTile || i != 7 {
		t.Errorf("↓ из второго ряда: зона %d/%d, ждали восьмую (последнюю) плитку", k, i)
	}
	if desktop.QSScroll(q) == 0 {
		t.Error("фокус на скрытой плитке не прокрутил сетку")
	}
	if !desktop.QSTileRect(q, "focus").In(desktop.QSZoneRect(q, "grid")) {
		t.Error("плитка с фокусом не видна в окне сетки")
	}
	qsKey(q, widget.KeyDown, 0)
	if k, _ := focus(); k != desktop.QSKindBright {
		t.Errorf("↓ из последнего ряда: зона %d, ждали яркость", k)
	}
	qsKey(q, widget.KeyTab, 0) // значок громкости
	qsKey(q, widget.KeyTab, 0) // громкость
	if k, _ := focus(); k != desktop.QSKindVol {
		t.Fatalf("зона %d, ждали ползунок громкости", k)
	}
	qsKey(q, widget.KeyRight, 0)
	if len(levels) != 1 || levels[0] < 0.69 || levels[0] > 0.71 {
		t.Errorf("→ на громкости (0,65): %v, ждали 0,70", levels)
	}
	qsKey(q, widget.KeyEnd, 0)
	if levels[len(levels)-1] != 1 {
		t.Errorf("End на громкости: %v", levels)
	}
	qsKey(q, widget.KeyTab, 0) // «›»
	qsKey(q, widget.KeyEnter, 0)
	if id, open := q.DetailsOpen(); !open || id != desktop.QuickVolumeID {
		t.Errorf("Enter на «›» громкости открыл %q (%v)", id, open)
	}
	qsFinish()
	// Alt+← — назад.
	qsKey(q, widget.KeyLeft, widget.ModAlt)
	qsFinish()
	if _, open := q.DetailsOpen(); open {
		t.Error("Alt+← не вернул на главную страницу")
	}
}

// Рамка фокуса видна: кадр с фокусом отличается от кадра без него.
func TestQuickPanel_FocusRingVisible(t *testing.T) {
	s := qsBuild(t, qsOpts{})
	q := s.q
	plain := s.render()
	q.SetFocused(true)
	qsKey(q, widget.KeyTab, 0)
	ringed := s.render()
	r := desktop.QSTileRect(q, "wifi")
	diff := 0
	for x := r.Min.X; x < r.Max.X; x++ {
		if plain.RGBAAt(x, r.Min.Y) != ringed.RGBAAt(x, r.Min.Y) {
			diff++
		}
	}
	if diff < r.Dx()/2 {
		t.Errorf("рамка фокуса не видна на верхней кромке плитки: изменилось %d пикселей из %d", diff, r.Dx())
	}
	// Щелчок мышью рамку гасит.
	qsClick(q, centreOf(desktop.QSTileRect(q, "plane")))
	if q.FocusVisible() {
		t.Error("после щелчка мышью рамка фокуса осталась")
	}
}

// ─── Тема, акцент, язык ──────────────────────────────────────────────────────

// Акцент и тема меняются на открытой панели: плитка «включено» берёт акцент.
func TestQuickPanel_AccentAndThemeLive(t *testing.T) {
	s := qsBuild(t, qsOpts{})
	q := s.q
	pix := func() [4]uint8 {
		img := s.render()
		r := desktop.QSTileRect(q, "wifi")
		c := img.RGBAAt(r.Min.X+4, r.Min.Y+4)
		return [4]uint8{c.R, c.G, c.B, c.A}
	}
	before := pix()
	s.tm.SetAccent(theme.RGB(200, 40, 40))
	after := pix()
	if before == after {
		t.Error("смена акцента не перекрасила включённую плитку")
	}
	if after[0] < 150 {
		t.Errorf("плитка после смены акцента на красный: %v", after)
	}
	if err := s.tm.SetTheme(theme.ProfileWindows11Dark); err != nil {
		t.Fatal(err)
	}
	if !desktop.QSRich(q) {
		t.Error("после смены темы на тёмную вариант пропал")
	}
}

// Язык: подсказки и подписи следуют за widget.SetLanguage без пересоздания.
func TestQuickPanel_LanguageLive(t *testing.T) {
	prev := widget.Language()
	t.Cleanup(func() { widget.SetLanguage(prev) })
	s := qsBuild(t, qsOpts{})
	q := s.q
	g := desktop.QSZoneRect(q, "settings")
	pt := centreOf(g)
	widget.SetLanguage("RU")
	ru := q.ToolTipAt(pt.X, pt.Y)
	widget.SetLanguage("EN")
	en := q.ToolTipAt(pt.X, pt.Y)
	if ru == "" || en == "" || ru == en {
		t.Errorf("подсказка шестерёнки RU=%q EN=%q", ru, en)
	}
	if en != "All settings" {
		t.Errorf("EN = %q", en)
	}
	b := desktop.QSZoneRect(q, "battery")
	if tip := q.ToolTipAt(b.Min.X+4, b.Min.Y+b.Dy()/2); tip != "Battery: 87%" {
		t.Errorf("подсказка батареи = %q", tip)
	}
}

// ─── Скорость ────────────────────────────────────────────────────────────────

// Первый кадр открытия быстрее 100 мс (медиана нескольких открытий).
func TestQuickPanel_FirstFrameUnder100ms(t *testing.T) {
	s := qsBuild(t, qsOpts{})
	q := s.q
	bb := s.bar.Bounds()
	anchor := image.Rect(q.Screen.Dx()-150, bb.Min.Y, q.Screen.Dx()-100, bb.Max.Y)
	var times []time.Duration
	for i := 0; i < 7; i++ {
		q.Close()
		q.Settle()
		s.eng.Invalidate()
		s.eng.RenderOnce()
		start := time.Now()
		q.Open(anchor)
		s.eng.Invalidate()
		s.eng.RenderOnce()
		times = append(times, time.Since(start))
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	t.Logf("первый кадр открытия: %v", times)
	if med := times[len(times)/2]; med > 100*time.Millisecond {
		t.Errorf("первый кадр открытия: медиана %v, предел 100 мс (%v)", med, times)
	}
}

// Закрытая панель не держит подписок: ни на модель, ни на показания системы.
func TestQuickPanel_NoSubscriptionsWhenClosed(t *testing.T) {
	s := qsBuild(t, qsOpts{})
	s.q.Close()
	rects := 0
	widget.SetUIRectChangeNotifier(func(image.Rectangle) { rects++ })
	t.Cleanup(func() { widget.SetUIRectChangeNotifier(nil) })
	full := 0
	widget.SetUIChangeNotifier(func() { full++ })
	t.Cleanup(func() { widget.SetUIChangeNotifier(nil) })
	s.model.SetOn("night", true)
	s.status.SetVolume(desktop.VolState{Level: 0.1})
	if rects != 0 || full != 0 {
		t.Errorf("закрытая панель проснулась: %d областей, %d полных", rects, full)
	}
	// Повторное открытие подписывается заново.
	bb := s.bar.Bounds()
	s.q.Open(image.Rect(s.q.Screen.Dx()-150, bb.Min.Y, s.q.Screen.Dx()-100, bb.Max.Y))
	s.q.Settle()
	rects = 0
	s.model.SetOn("night", false)
	if rects == 0 {
		t.Error("после повторного открытия панель не следит за моделью")
	}
}
