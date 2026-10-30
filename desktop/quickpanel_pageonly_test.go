package desktop_test

import (
	"github.com/oops1/headless-gui/v4/internal/testreset"
	"image"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/desktop"
	"github.com/oops1/headless-gui/v4/widget"
)

// Плитки «только страница», обход Tab по столбцу страниц и вертикальный сдвиг
// страниц плиток (v3.34).

// pageOnlyModel делает плитку «Специальные возможности» плиткой «только
// страница». В образце у неё HasDetails (разделена на половинки): PageOnly
// главнее, так что потребитель, поднявший оба признака, получает целую плитку.
func pageOnlyModel(m *desktop.QuickActionList) {
	for _, a := range m.List() {
		if a.ID == "access" {
			a.PageOnly = true
			m.Set(a)
		}
	}
}

// pageOnlyAlone — то же, но HasDetails снят: страницу плитке даёт один PageOnly.
func pageOnlyAlone(m *desktop.QuickActionList) {
	for _, a := range m.List() {
		if a.ID == "access" {
			a.HasDetails, a.PageOnly = false, true
			m.Set(a)
		}
	}
}

// qsTabs нажимает Tab n раз и возвращает зоны, на которых останавливался фокус.
func qsTabs(q *desktop.QuickSettings, n int) [][2]int {
	var out [][2]int
	for i := 0; i < n; i++ {
		qsKey(q, widget.KeyTab, 0)
		k, idx := desktop.QSFocus(q)
		out = append(out, [2]int{k, idx})
	}
	return out
}

// Щелчок по любому месту плитки «только страница» — левее и правее прежней
// границы половинок — открывает страницу; Toggle не зовётся.
func TestPageOnly_WholeTileOpensPage(t *testing.T) {
	testreset.Globals(t)
	for name, model := range map[string]func(*desktop.QuickActionList){"с HasDetails": pageOnlyModel, "без HasDetails": pageOnlyAlone} {
		t.Run(name, func(t *testing.T) { pageOnlyWholeTile(t, model) })
	}
}

func pageOnlyWholeTile(t *testing.T, model func(*desktop.QuickActionList)) {
	s := qsBuild(t, qsOpts{model: model})
	q := s.q
	tile := desktop.QSTileRect(q, "access")
	chev := desktop.QSChevronRect(q, "access")
	for name, pt := range map[string]image.Point{
		"левая часть":  image.Pt(tile.Min.X+8, tile.Min.Y+8),
		"правая часть": centreOf(chev),
		"«›» у края":   image.Pt(tile.Max.X-4, tile.Min.Y+tile.Dy()/2),
	} {
		qsClick(q, pt)
		qsFinish()
		if id, open := q.DetailsOpen(); !open || id != "access" {
			t.Errorf("%s: страница не открылась (%q, %v)", name, id, open)
		}
		q.CloseDetails()
		qsFinish()
	}
	if len(s.model.Toggled) != 0 {
		t.Errorf("плитка «только страница» переключилась: %v", s.model.Toggled)
	}
}

// Плитка целая: линии между половинками нет, наведение накрывает всю плитку, а у
// обычной плитки с «›» линия и раздельная подсветка остаются.
func TestPageOnly_NotSplit(t *testing.T) {
	testreset.Globals(t)
	for name, model := range map[string]func(*desktop.QuickActionList){"с HasDetails": pageOnlyModel, "без HasDetails": pageOnlyAlone} {
		t.Run(name, func(t *testing.T) { pageOnlyNotSplit(t, model) })
	}
}

func pageOnlyNotSplit(t *testing.T, model func(*desktop.QuickActionList)) {
	s := qsBuild(t, qsOpts{model: model})
	q := s.q
	img := s.settled()
	row := func(id desktop.QuickActionID) (line, near image.Point) {
		tile := desktop.QSTileRect(q, id)
		chev := desktop.QSChevronRect(q, id)
		y := tile.Min.Y + 4
		return image.Pt(chev.Min.X, y), image.Pt(chev.Min.X-3, y)
	}
	line, near := row("access")
	if img.RGBAAt(line.X, line.Y) != img.RGBAAt(near.X, near.Y) {
		t.Error("у плитки «только страница» осталась линия между половинками")
	}
	line, near = row("bt")
	if img.RGBAAt(line.X, line.Y) == img.RGBAAt(near.X, near.Y) {
		t.Error("у плитки с «›» пропала линия между половинками")
	}

	// «›» — часть плитки, а не отдельная кнопка: подсказки «Подробнее» над ней нет
	// (у разделённой плитки есть).
	tile := desktop.QSTileRect(q, "access")
	chev := desktop.QSChevronRect(q, "access")
	if tip := q.ToolTipAt(chev.Min.X+8, chev.Min.Y+8); tip != "" {
		t.Errorf("над «›» плитки «только страница» подсказка %q", tip)
	}
	if bt := desktop.QSChevronRect(q, "bt"); q.ToolTipAt(bt.Min.X+8, bt.Min.Y+8) == "" {
		t.Error("у разделённой плитки пропала подсказка над «›»")
	}

	// Курсор на левой части: подсвечена и правая (одна кнопка).
	q.OnMouseMove(tile.Min.X+6, tile.Min.Y+4)
	hot := s.settled()
	left := hot.RGBAAt(tile.Min.X+6, tile.Min.Y+4)
	right := hot.RGBAAt(chev.Min.X+6, tile.Min.Y+4)
	if left != right {
		t.Errorf("наведение на левую часть не накрыло правую: %v и %v", left, right)
	}
	if left == img.RGBAAt(tile.Min.X+6, tile.Min.Y+4) {
		t.Error("наведение на плитку «только страница» не видно")
	}
}

// Обход Tab: у плитки «только страница» «›» не отдельная остановка; Enter на
// плитке открывает страницу.
func TestPageOnly_TabAndEnter(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{model: pageOnlyModel})
	q := s.q
	q.SetFocused(true)
	seenWifiChevron, accessTile := false, false
	for _, z := range qsTabs(q, 24) {
		if z[0] == desktop.QSKindChevron && z[1] == 5 {
			t.Fatal("Tab встал на «›» плитки «только страница»")
		}
		if z[0] == desktop.QSKindChevron && z[1] == 0 {
			seenWifiChevron = true
		}
		if z[0] == desktop.QSKindTile && z[1] == 5 {
			accessTile = true
		}
	}
	if !seenWifiChevron || !accessTile {
		t.Fatalf("обход потерял соседей: «›» Wi-Fi %v, плитка %v", seenWifiChevron, accessTile)
	}
	// Фокус на плитку и Enter.
	for i := 0; i < 40; i++ {
		if k, idx := desktop.QSFocus(q); k == desktop.QSKindTile && idx == 5 {
			break
		}
		qsKey(q, widget.KeyTab, 0)
	}
	qsKey(q, widget.KeyEnter, 0)
	qsFinish()
	if id, open := q.DetailsOpen(); !open || id != "access" {
		t.Errorf("Enter на плитке «только страница» открыл %q (%v)", id, open)
	}
}

// Отключённая плитка «только страница» не открывает страницу; в режиме правки её
// не открывает ни щелчок, ни Enter.
func TestPageOnly_DisabledAndEditing(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{model: func(m *desktop.QuickActionList) {
		pageOnlyModel(m)
		for _, a := range m.List() {
			if a.ID == "access" {
				a.Disabled = true
				m.Set(a)
			}
		}
	}})
	q := s.q
	qsClick(q, centreOf(desktop.QSTileRect(q, "access")))
	qsFinish()
	if _, open := q.DetailsOpen(); open {
		t.Error("отключённая плитка открыла страницу")
	}

	s2 := qsBuild(t, qsOpts{model: pageOnlyModel})
	q2 := s2.q
	q2.SetEditing(true)
	qsClick(q2, centreOf(desktop.QSTileRect(q2, "access")))
	qsFinish()
	if _, open := q2.DetailsOpen(); open {
		t.Error("в режиме правки плитка открыла страницу")
	}
}

// Недоступная (Unavailable) плитка «только страница» страницу всё равно открывает:
// там видно, почему плитка недоступна.
func TestPageOnly_UnavailableStillOpens(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{model: func(m *desktop.QuickActionList) {
		pageOnlyModel(m)
		for _, a := range m.List() {
			if a.ID == "access" {
				a.Unavailable = true
				m.Set(a)
			}
		}
	}})
	qsClick(s.q, centreOf(desktop.QSTileRect(s.q, "access")))
	qsFinish()
	if id, open := s.q.DetailsOpen(); !open || id != "access" {
		t.Errorf("недоступная плитка «только страница»: %q (%v)", id, open)
	}
	if len(s.model.Toggled) != 0 {
		t.Errorf("недоступная плитка переключилась: %v", s.model.Toggled)
	}
}

// ─── Обход Tab по столбцу страниц ────────────────────────────────────────────

// После плиток идут ▲ (если есть куда листать назад), точки страниц и ▼ (если
// есть куда вперёд), затем яркость. Обход плиток второй ряд листает сам (фокус на
// скрытой плитке переворачивает страницу), так что к столбцу страниц Tab приходит
// со второй страницы: ▲ в обходе, ▼ приглушена и нет. Shift+Tab ходит назад.
func TestPager_TabOrder(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	q.SetFocused(true)
	if q.Pages() != 2 {
		t.Fatalf("страниц %d, ждали 2", q.Pages())
	}
	var order [][2]int
	for _, z := range qsTabs(q, 30) {
		if z[0] == desktop.QSKindEdit {
			break
		}
		order = append(order, z)
	}
	at := -1
	for i, z := range order {
		if z[0] == desktop.QSKindPageUp {
			at = i
		}
		if z[0] == desktop.QSKindPageDn {
			t.Error("в обходе приглушённая ▼ последней страницы")
		}
	}
	if at < 1 {
		t.Fatalf("в обходе нет ▲ второй страницы: %v", order)
	}
	want := [][2]int{{desktop.QSKindPageUp, 0}, {desktop.QSKindPageDot, 0}, {desktop.QSKindPageDot, 1}, {desktop.QSKindBright, 0}}
	for i, w := range want {
		if at+i >= len(order) || order[at+i] != w {
			t.Fatalf("после плиток ждали %v, обход %v", want, order[at:])
		}
	}
	if prev := order[at-1]; prev[0] != desktop.QSKindTile && prev[0] != desktop.QSKindChevron {
		t.Errorf("перед столбцом страниц не плитка: %v", prev)
	}
	// Назад: с яркости (она стоит сразу после столбца) на последнюю точку.
	for i := 0; i < 40; i++ {
		if k, _ := desktop.QSFocus(q); k == desktop.QSKindBright {
			break
		}
		qsKey(q, widget.KeyTab, 0)
	}
	qsKey(q, widget.KeyTab, widget.ModShift)
	if k, n := desktop.QSFocus(q); k != desktop.QSKindPageDot || n != 1 {
		t.Errorf("Shift+Tab с яркости: зона %d/%d, ждали точку 1", k, n)
	}
	// С первой страницы в обход входит ▼, а не ▲.
	q.SetPage(0)
	qsKey(q, widget.KeyLeft, 0) // с точки к предыдущей зоне обхода
	if k, n := desktop.QSFocus(q); k != desktop.QSKindPageDot || n != 0 {
		t.Errorf("← с точки 1: зона %d/%d, ждали точку 0", k, n)
	}
	qsKey(q, widget.KeyDown, 0)
	qsKey(q, widget.KeyDown, 0)
	if k, _ := desktop.QSFocus(q); k != desktop.QSKindPageDn {
		t.Errorf("↓ ↓ с точки 0 на первой странице: зона %d, ждали ▼", k)
	}
}

// Enter и Space на точках и стрелках листают; крайняя стрелка, нажатая с клавиатуры,
// отдаёт фокус точке страницы; рамка фокуса видна.
func TestPager_KeyboardActivates(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	q.SetFocused(true)
	focusOn := func(kind, idx int) {
		t.Helper()
		for i := 0; i < 40; i++ {
			qsKey(q, widget.KeyTab, 0)
			if k, n := desktop.QSFocus(q); k == kind && n == idx {
				return
			}
		}
		t.Fatalf("Tab не дошёл до зоны %d/%d", kind, idx)
	}
	plain := s.settled()

	focusOn(desktop.QSKindPageDot, 1)
	qsKey(q, widget.KeySpace, 0)
	if q.Page() != 1 {
		t.Fatalf("Space на второй точке: страница %d", q.Page())
	}
	qsFinish()
	ringed := s.settled()
	dot := desktop.QSDotRect(q, 1).Inset(-3)
	diff := 0
	for y := dot.Min.Y; y < dot.Max.Y; y++ {
		for x := dot.Min.X; x < dot.Max.X; x++ {
			if plain.RGBAAt(x, y) != ringed.RGBAAt(x, y) {
				diff++
			}
		}
	}
	if diff == 0 {
		t.Error("рамка фокуса вокруг точки страницы не видна")
	}

	// На второй странице ▲ в обходе, ▼ приглушена. Enter на ▲ возвращает страницу и,
	// раз ▲ на первой странице приглушилась, фокус уходит на точку.
	focusOn(desktop.QSKindPageUp, 0)
	qsKey(q, widget.KeyEnter, 0)
	if q.Page() != 0 {
		t.Fatalf("Enter на ▲: страница %d", q.Page())
	}
	if k, n := desktop.QSFocus(q); k != desktop.QSKindPageDot || n != 0 {
		t.Errorf("после ▲ на первой странице фокус на %d/%d, ждали точку 0", k, n)
	}
	// Enter на ▼ листает вперёд.
	focusOn(desktop.QSKindPageDn, 0)
	qsKey(q, widget.KeyEnter, 0)
	if q.Page() != 1 {
		t.Errorf("Enter на ▼: страница %d", q.Page())
	}
	if k, n := desktop.QSFocus(q); k != desktop.QSKindPageDot || n != 1 {
		t.Errorf("после ▼ на последней странице фокус на %d/%d, ждали точку 1", k, n)
	}
}

// ─── Сдвиг страниц плиток ────────────────────────────────────────────────────

// slideRects собирает заявленные области перерисовки.
type slideRects struct{ rects []image.Rectangle }

func watchSlide(t *testing.T) *slideRects {
	t.Helper()
	r := &slideRects{}
	h := widget.RegisterUINotifier(nil, func(rc image.Rectangle) { r.rects = append(r.rects, rc) })
	t.Cleanup(func() { widget.UnregisterUINotifier(h) })
	return r
}

func gridPixels(img *image.RGBA, r image.Rectangle) []byte {
	var out []byte
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.RGBAAt(x, y)
			out = append(out, c.R, c.G, c.B, c.A)
		}
	}
	return out
}

func samePixels(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Смена страницы — вертикальный сдвиг по токену quicksettings.pages: сразу после
// SetPage сетка ещё на старом месте, посреди движения кадр отличается от обеих
// страниц, а в конце совпадает с мгновенным переключением при «меньше движения».
func TestPageSlide_MovesThenSettles(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	grid := desktop.QSZoneRect(q, "grid")
	first := gridPixels(s.settled(), grid)

	q.SetPage(1)
	if pos, anim := desktop.QSSlide(q); !anim || pos != 0 {
		t.Fatalf("сразу после SetPage: положение %v, движение %v", pos, anim)
	}
	if q.Page() != 1 {
		t.Fatalf("целевая страница %d", q.Page())
	}
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(70 * time.Millisecond))
	pos, anim := desktop.QSSlide(q)
	if !anim || pos <= 0 || pos >= 1 {
		t.Fatalf("посреди движения положение %v, движение %v", pos, anim)
	}
	mid := gridPixels(s.render(), grid)
	if samePixels(mid, first) {
		t.Error("посреди движения кадр сетки не отличается от первой страницы")
	}
	qsFinish()
	if pos, anim := desktop.QSSlide(q); anim || pos != 1 {
		t.Fatalf("после движения положение %v, движение %v", pos, anim)
	}
	last := gridPixels(s.render(), grid)
	if samePixels(mid, last) || samePixels(first, last) {
		t.Error("кадры первой страницы, середины и конца не различаются")
	}

	// «Меньше движения»: страница меняется мгновенно и кадр тот же, что в конце движения.
	r := qsBuild(t, qsOpts{reduce: true})
	r.q.SetPage(1)
	if pos, anim := desktop.QSSlide(r.q); anim || pos != 1 {
		t.Fatalf("motion.reduce: положение %v, движение %v (ждали мгновенно)", pos, anim)
	}
	if !samePixels(gridPixels(r.settled(), desktop.QSZoneRect(r.q, "grid")), last) {
		t.Error("кадр мгновенной смены страницы отличается от кадра по завершении движения")
	}
}

// Сдвиг заявляет перерисовку только окна сетки, а встречный SetPage подхватывает
// движение с текущего положения без скачка.
func TestPageSlide_InvalidatesGridAndRetargets(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	grid := desktop.QSZoneRect(q, "grid")
	panel := desktop.QSPanelRect(q)
	qsFinish() // выезд панели закончен: его шаги не мешают счёту
	w := watchSlide(t)

	q.SetPage(1)
	w.rects = nil
	t0 := time.Now()
	widget.StepAnimations(t0)
	widget.StepAnimations(t0.Add(60 * time.Millisecond))
	if len(w.rects) == 0 {
		t.Fatal("шаг сдвига ничего не заявил")
	}
	hitGrid := false
	for _, r := range w.rects {
		if !r.In(panel) {
			t.Errorf("шаг сдвига заявил область %v вне панели %v", r, panel)
		}
		if r == grid {
			hitGrid = true
		}
	}
	if !hitGrid {
		t.Errorf("шаг сдвига не заявил окно сетки %v: %v", grid, w.rects)
	}

	before, _ := desktop.QSSlide(q)
	q.SetPage(0)
	after, anim := desktop.QSSlide(q)
	if !anim || after < before-0.01 || after > before+0.01 {
		t.Errorf("встречный SetPage: было %.3f, стало %.3f (движение %v)", before, after, anim)
	}
	qsFinish()
	if pos, _ := desktop.QSSlide(q); pos != 0 {
		t.Errorf("после возврата положение %v", pos)
	}
}

// Новое открытие панели сбрасывает сдвиг: страница 0 без движения.
func TestPageSlide_ResetsOnReopen(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	q.SetPage(1)
	q.Close()
	q.Settle()
	bb := s.bar.Bounds()
	q.Open(image.Rect(q.Screen.Dx()-150, bb.Min.Y, q.Screen.Dx()-100, bb.Max.Y))
	q.Settle()
	if pos, anim := desktop.QSSlide(q); pos != 0 || anim {
		t.Errorf("после повторного открытия положение %v, движение %v", pos, anim)
	}
}
