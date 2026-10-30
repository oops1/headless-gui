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

// Быстрые настройки Windows 11, v3.34: разделённая плитка, страницы с точками,
// нижняя полоса, неактивные ползунки, подпись в две строки.

// ─── Разделённая плитка ──────────────────────────────────────────────────────

// Плитка с подробностями делится на две половинки с тонкой линией: левая
// переключает, правая открывает страницу. Плитка без подробностей не делится.
func TestQuickPanel_SplitTileZonesAndDivider(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	bt, chev := desktop.QSTileRect(q, "bt"), desktop.QSChevronRect(q, "bt")
	if chev.Min.X != bt.Min.X+bt.Dx()/2 || chev.Max.X != bt.Max.X {
		t.Fatalf("правая половинка %v должна занимать правую половину плитки %v", chev, bt)
	}
	img := s.render()
	// Линия между половинками — столбец другого цвета на самой кромке плитки,
	// вдали от значка и «›».
	y := bt.Min.Y + 3
	line, left := img.RGBAAt(chev.Min.X, y), img.RGBAAt(chev.Min.X-4, y)
	if line == left {
		t.Errorf("между половинками нет линии: %v совпадает с соседним пикселем", line)
	}
	plane := desktop.QSTileRect(q, "plane")
	mid := plane.Min.X + plane.Dx()/2
	if a, b := img.RGBAAt(mid, plane.Min.Y+3), img.RGBAAt(mid-4, plane.Min.Y+3); a != b {
		t.Errorf("плитка без подробностей разделена линией: %v и %v", a, b)
	}

	// Левая половинка переключает и не открывает страницу.
	qsClick(q, image.Pt(bt.Min.X+8, bt.Min.Y+24))
	if len(s.model.Toggled) != 1 || s.model.Toggled[0] != "bt" {
		t.Fatalf("левая половинка не переключила плитку: %v", s.model.Toggled)
	}
	if _, open := q.DetailsOpen(); open {
		t.Error("левая половинка открыла страницу")
	}
	// Правая открывает страницу и не переключает.
	qsClick(q, image.Pt(chev.Max.X-8, chev.Min.Y+24))
	if id, open := q.DetailsOpen(); !open || id != "bt" {
		t.Fatalf("правая половинка не открыла страницу: %q %v", id, open)
	}
	if len(s.model.Toggled) != 1 {
		t.Errorf("правая половинка переключила плитку: %v", s.model.Toggled)
	}
}

// Наведение подсвечивает только свою половинку: другая остаётся как была.
func TestQuickPanel_SplitTileHoverIsPerHalf(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	bt, chev := desktop.QSTileRect(q, "bt"), desktop.QSChevronRect(q, "bt")
	lp, rp := image.Pt(bt.Min.X+4, bt.Min.Y+4), image.Pt(chev.Max.X-5, chev.Min.Y+4)
	rest := s.settled()

	q.OnMouseMove(bt.Min.X+10, bt.Min.Y+24)
	left := s.settled()
	if left.RGBAAt(lp.X, lp.Y) == rest.RGBAAt(lp.X, lp.Y) {
		t.Error("наведение на левую половинку не подсветило её")
	}
	if left.RGBAAt(rp.X, rp.Y) != rest.RGBAAt(rp.X, rp.Y) {
		t.Error("наведение на левую половинку подсветило правую")
	}

	q.OnMouseMove(chev.Min.X+10, chev.Min.Y+24)
	right := s.settled()
	if right.RGBAAt(rp.X, rp.Y) == rest.RGBAAt(rp.X, rp.Y) {
		t.Error("наведение на правую половинку не подсветило её")
	}
	if right.RGBAAt(lp.X, lp.Y) != rest.RGBAAt(lp.X, lp.Y) {
		t.Error("наведение на правую половинку подсветило левую")
	}

	// Нажатие темнее наведения и тоже только на своей половинке.
	q.OnMouseMove(bt.Min.X+10, bt.Min.Y+24)
	left = s.settled()
	q.OnMouseButton(widget.MouseEvent{X: bt.Min.X + 10, Y: bt.Min.Y + 24, Button: widget.MouseLeft, Pressed: true})
	pressed := s.settled()
	if pressed.RGBAAt(lp.X, lp.Y) == left.RGBAAt(lp.X, lp.Y) {
		t.Error("нажатие на левую половинку не отличается от наведения")
	}
	if pressed.RGBAAt(rp.X, rp.Y) != rest.RGBAAt(rp.X, rp.Y) {
		t.Error("нажатие на левую половинку тронуло правую")
	}
	q.OnMouseButton(widget.MouseEvent{X: bt.Min.X + 10, Y: bt.Min.Y + 24, Button: widget.MouseLeft})
}

// Недоступная плитка не переключается, но её «›» живая.
func TestQuickPanel_SplitTileUnavailableKeepsChevron(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	saver, chev := desktop.QSTileRect(q, "saver"), desktop.QSChevronRect(q, "saver")
	// Третий ряд сетки на второй странице: листаем.
	qsClick(q, image.Pt(saver.Min.X+8, saver.Min.Y+24))
	if len(s.model.Toggled) != 0 {
		t.Errorf("недоступная плитка переключилась: %v", s.model.Toggled)
	}
	qsClick(q, centreOf(chev))
	if id, open := q.DetailsOpen(); !open || id != "saver" {
		t.Errorf("«›» недоступной плитки не открыла страницу: %q %v", id, open)
	}
}

// ─── Страницы ────────────────────────────────────────────────────────────────

func TestQuickPanel_PagesInsteadOfScroll(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	if got := q.Pages(); got != 2 {
		t.Fatalf("восемь плиток по два ряда на страницу: страниц %d, ждали 2", got)
	}
	if !desktop.QSZoneRect(q, "thumb").Empty() {
		t.Error("в постраничном режиме осталась полоса прокрутки")
	}
	pager, up, down := desktop.QSZoneRect(q, "pager"), desktop.QSZoneRect(q, "pageup"), desktop.QSZoneRect(q, "pagedown")
	grid := desktop.QSZoneRect(q, "grid")
	if pager.Empty() || up.Empty() || down.Empty() {
		t.Fatalf("справа от сетки нет столбца страниц: %v %v %v", pager, up, down)
	}
	if pager.Min.X < grid.Max.X || !pager.In(desktop.QSPanelRect(q)) {
		t.Errorf("столбец страниц %v должен стоять справа от сетки %v внутри панели", pager, grid)
	}
	if mid := pager.Min.Y + pager.Dy()/2; mid < grid.Min.Y || mid > grid.Max.Y {
		t.Errorf("столбец %v не на высоте сетки %v", pager, grid)
	}
	if !(up.Max.Y <= desktop.QSDotRect(q, 0).Min.Y && desktop.QSDotRect(q, 1).Max.Y <= down.Min.Y) {
		t.Error("порядок сверху вниз: ▲, точки, ▼")
	}
	// Колесо листает страницы целиком: третий ряд встаёт на место первого.
	vpn := desktop.QSTileRect(q, "vpn")
	if vpn.Min.Y < grid.Max.Y {
		t.Fatalf("плитка третьего ряда %v должна быть ниже окна сетки %v", vpn, grid)
	}
	if !q.OnMouseWheelPixels(grid.Min.X+5, grid.Min.Y+5, 0, 40) {
		t.Fatal("колесо над сеткой не принято")
	}
	if q.Page() != 1 {
		t.Fatalf("колесо вниз: страница %d, ждали 1", q.Page())
	}
	vpn = desktop.QSTileRect(q, "vpn")
	if vpn.Min.Y != grid.Min.Y {
		t.Errorf("на второй странице плитка третьего ряда %v должна стоять в первом ряду окна %v", vpn, grid)
	}
	if desktop.QSScroll(q) != 0 {
		t.Error("постраничный режим сдвинул пиксельную прокрутку")
	}
	// Дальше последней страницы — некуда.
	q.OnMouseWheelPixels(grid.Min.X+5, grid.Min.Y+5, 0, 400)
	if q.Page() != 1 {
		t.Errorf("колесо за последней страницей: %d", q.Page())
	}
	q.OnMouseWheelPixels(grid.Min.X+5, grid.Min.Y+5, 0, -40)
	if q.Page() != 0 {
		t.Errorf("колесо вверх: страница %d, ждали 0", q.Page())
	}
}

// Мелкие события колеса копятся до порога: сенсорная панель не пролистывает
// все страницы одним жестом.
func TestQuickPanel_WheelAccumulates(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	grid := desktop.QSZoneRect(q, "grid")
	for i := 0; i < 2; i++ {
		q.OnMouseWheelPixels(grid.Min.X+5, grid.Min.Y+5, 0, 10)
	}
	if q.Page() != 0 {
		t.Fatalf("20 из 30 пикселей уже листают: страница %d", q.Page())
	}
	q.OnMouseWheelPixels(grid.Min.X+5, grid.Min.Y+5, 0, 10)
	if q.Page() != 1 {
		t.Errorf("30 пикселей не перелистнули страницу: %d", q.Page())
	}
	// Колесо над столбцом страниц тоже листает.
	pager := desktop.QSZoneRect(q, "pager")
	q.OnMouseWheelPixels(pager.Min.X+3, pager.Min.Y+3, 0, -40)
	if q.Page() != 0 {
		t.Errorf("колесо над столбцом страниц: %d", q.Page())
	}
}

func TestQuickPanel_PagerArrowsAndDots(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	qsClick(q, centreOf(desktop.QSZoneRect(q, "pageup")))
	if q.Page() != 0 {
		t.Errorf("▲ на первой странице: %d", q.Page())
	}
	qsClick(q, centreOf(desktop.QSZoneRect(q, "pagedown")))
	if q.Page() != 1 {
		t.Fatalf("▼: страница %d, ждали 1", q.Page())
	}
	qsClick(q, centreOf(desktop.QSZoneRect(q, "pagedown")))
	if q.Page() != 1 {
		t.Errorf("▼ на последней странице: %d", q.Page())
	}
	qsClick(q, centreOf(desktop.QSZoneRect(q, "pageup")))
	if q.Page() != 0 {
		t.Fatalf("▲: страница %d, ждали 0", q.Page())
	}
	qsClick(q, centreOf(desktop.QSDotRect(q, 1)))
	if q.Page() != 1 {
		t.Errorf("точка второй страницы: %d", q.Page())
	}
	// Текущая точка отличается от остальных, и кадр на второй странице другой.
	img := s.settled()
	d0, d1 := centreOf(desktop.QSDotRect(q, 0)), centreOf(desktop.QSDotRect(q, 1))
	if img.RGBAAt(d0.X, d0.Y) == img.RGBAAt(d1.X, d1.Y) {
		t.Error("точка текущей страницы не отличается от прочих")
	}
	// Последняя страница: ▼ приглушена (отличается от ▲ на первой странице).
	down := centreOf(desktop.QSZoneRect(q, "pagedown"))
	q.SetPage(0)
	first := s.settled()
	if first.RGBAAt(down.X, down.Y) == img.RGBAAt(down.X, down.Y) {
		t.Error("▼ на последней странице не приглушена")
	}
	q.SetPage(-3)
	if q.Page() != 0 {
		t.Errorf("SetPage(-3) = %d", q.Page())
	}
	q.SetPage(99)
	if q.Page() != 1 {
		t.Errorf("SetPage(99) = %d, ждали последнюю", q.Page())
	}
}

// Страница сбрасывается при повторном открытии панели.
func TestQuickPanel_PageResetsOnReopen(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	q.SetPage(1)
	q.Close()
	q.Settle()
	bb := s.bar.Bounds()
	q.Open(image.Rect(q.Screen.Dx()-150, bb.Min.Y, q.Screen.Dx()-100, bb.Max.Y))
	q.Settle()
	if q.Page() != 0 {
		t.Errorf("после повторного открытия страница %d", q.Page())
	}
}

// Плитку в режиме правки тянут к нижнему краю сетки — страница листается один
// раз за заход в краевую зону; потом плитку кладут на другую страницу.
func TestQuickPanel_DragToEdgeTurnsPage(t *testing.T) {
	testreset.Globals(t)
	var order []desktop.QuickActionID
	s := qsBuild(t, qsOpts{setup: func(q *desktop.QuickSettings) {
		q.OnReorder = func(o []desktop.QuickActionID) { order = o }
	}})
	q := s.q
	q.SetEditing(true)
	grid := desktop.QSZoneRect(q, "grid")
	from := centreOf(desktop.QSTileRect(q, "wifi"))
	q.OnMouseButton(widget.MouseEvent{X: from.X, Y: from.Y, Button: widget.MouseLeft, Pressed: true})
	q.OnMouseMove(from.X+8, from.Y+20)
	edge := image.Pt(from.X, grid.Max.Y-4)
	q.OnMouseMove(edge.X, edge.Y)
	if q.Page() != 1 {
		t.Fatalf("плитка у нижнего края: страница %d, ждали 1", q.Page())
	}
	q.OnMouseMove(edge.X+1, edge.Y)
	q.OnMouseMove(edge.X+2, edge.Y)
	if q.Page() != 1 {
		t.Errorf("краевая зона листает снова и снова: %d", q.Page())
	}
	// Бросить на первое место второй страницы (плитка vpn).
	drop := image.Pt(grid.Min.X+48, grid.Min.Y+24)
	q.OnMouseMove(drop.X, drop.Y)
	q.OnMouseButton(widget.MouseEvent{X: drop.X, Y: drop.Y, Button: widget.MouseLeft})
	pos := -1
	for i, id := range order {
		if id == "wifi" {
			pos = i
		}
	}
	if pos < 6 {
		t.Errorf("плитку не положили на вторую страницу (место %d): %v", pos, order)
	}
}

// Флаг профиля погашен — прежняя прокрутка, столбца страниц нет.
func TestQuickPanel_PagedFlagOffKeepsScroll(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{scroll: true})
	q := s.q
	if q.Pages() != 1 || !desktop.QSZoneRect(q, "pager").Empty() {
		t.Errorf("при прокрутке страниц %d, столбец %v", q.Pages(), desktop.QSZoneRect(q, "pager"))
	}
	if desktop.QSZoneRect(q, "thumb").Empty() {
		t.Error("при прокрутке нет полосы")
	}
	grid := desktop.QSZoneRect(q, "grid")
	q.OnMouseWheelPixels(grid.Min.X+5, grid.Min.Y+5, 0, 30)
	if desktop.QSScroll(q) != 30 || q.Page() != 0 {
		t.Errorf("прокрутка %d, страница %d", desktop.QSScroll(q), q.Page())
	}
	q.SetPage(1) // не листает
	if q.Page() != 0 {
		t.Error("SetPage листает прокручиваемую панель")
	}
}

// Одна страница плиток — столбца страниц нет.
func TestQuickPanel_SinglePageHasNoPager(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{model: func(m *desktop.QuickActionList) { m.Replace(m.List()[:6]) }})
	if s.q.Pages() != 1 || !desktop.QSZoneRect(s.q, "pager").Empty() {
		t.Errorf("шесть плиток на одной странице: страниц %d, столбец %v", s.q.Pages(), desktop.QSZoneRect(s.q, "pager"))
	}
}

// ─── Нижняя полоса ───────────────────────────────────────────────────────────

func TestQuickPanel_FooterEditLeft(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	edit, bat, gear := desktop.QSZoneRect(q, "edit"), desktop.QSZoneRect(q, "battery"), desktop.QSZoneRect(q, "settings")
	footer := desktop.QSZoneRect(q, "footer")
	if !(edit.Min.X < bat.Min.X && bat.Max.X <= gear.Min.X) {
		t.Errorf("порядок слева направо: «Изменить» %v, батарея %v, шестерёнка %v", edit, bat, gear)
	}
	if edit.Min.X-footer.Min.X != 16 || footer.Max.X-gear.Max.X != 16 {
		t.Errorf("поля нижней полосы: слева %d, справа %d", edit.Min.X-footer.Min.X, footer.Max.X-gear.Max.X)
	}
	if edit.Dx() <= 32 {
		t.Errorf("у «Изменить» нет подписи: ширина %d", edit.Dx())
	}
	// Нажатие работает на всей кнопке, включая подпись.
	qsClick(q, image.Pt(edit.Max.X-4, centreOf(edit).Y))
	if !q.Editing() {
		t.Error("щелчок по подписи «Изменить» не включил правку")
	}
	// Без батареи кнопка стоит на месте, шестерёнка справа.
	q.SetEditing(false)
	s.status.SetPower(desktop.PowerState{NoBattery: true})
	if e2 := desktop.QSZoneRect(q, "edit"); e2 != edit {
		t.Errorf("без батареи кнопка сдвинулась: %v → %v", edit, e2)
	}
	if !desktop.QSZoneRect(q, "battery").Empty() {
		t.Error("батареи нет, а значение показано")
	}
	// Подпись переведена и меняется с языком.
	ru, en := widget.TrIn("RU", desktop.StrQuickEditShort), widget.TrIn("EN", desktop.StrQuickEditShort)
	if ru != "Изменить" || en != "Edit" {
		t.Errorf("подпись кнопки: RU=%q EN=%q", ru, en)
	}
}

// Флаг погашен — прежняя полоса: батарея слева, карандаш и шестерёнка справа.
func TestQuickPanel_FooterOldLayoutByFlag(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{editOld: true})
	q := s.q
	edit, bat, gear := desktop.QSZoneRect(q, "edit"), desktop.QSZoneRect(q, "battery"), desktop.QSZoneRect(q, "settings")
	if !(bat.Min.X < edit.Min.X && edit.Max.X <= gear.Min.X) {
		t.Errorf("прежний порядок: батарея %v, карандаш %v, шестерёнка %v", bat, edit, gear)
	}
	if edit.Dx() != 32 {
		t.Errorf("прежний карандаш без подписи 32 шириной, а он %d", edit.Dx())
	}
}

// Режим правки: «Готово» встаёт на то же место, что «Изменить».
func TestQuickPanel_FooterDoneKeepsPlace(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	before := desktop.QSZoneRect(q, "edit")
	q.SetEditing(true)
	after := desktop.QSZoneRect(q, "edit")
	if after.Min.X != before.Min.X {
		t.Errorf("«Готово» стоит не там, где «Изменить»: %v и %v", after, before)
	}
}

// ─── Неактивные ползунки ─────────────────────────────────────────────────────

// px — цвет пикселя левой трети дорожки (заливка до бегунка).
func trackPix(s *qsScene, zone string) color.RGBA {
	r := desktop.QSZoneRect(s.q, zone)
	img := s.render()
	return img.RGBAAt(r.Min.X+10, r.Min.Y+r.Dy()/2)
}

func TestQuickPanel_VolumeDisabled(t *testing.T) {
	testreset.Globals(t)
	var levels []float64
	var mutes int
	s := qsBuild(t, qsOpts{setup: func(q *desktop.QuickSettings) {
		q.OnVolumeChange = func(v float64) { levels = append(levels, v) }
		q.OnToggleMute = func() { mutes++ }
	}})
	q := s.q
	if !q.VolumeEnabled() || !q.BrightnessEnabled() {
		t.Fatal("ползунки по умолчанию должны быть активны")
	}
	on := trackPix(s, "voltrack")
	q.SetVolumeEnabled(false)
	if q.VolumeEnabled() {
		t.Fatal("SetVolumeEnabled(false) не сработал")
	}
	off := trackPix(s, "voltrack")
	if off == on {
		t.Errorf("неактивный ползунок нарисован как активный: %v", off)
	}
	if off.R != off.G || off.G != off.B {
		t.Errorf("неактивный ползунок не серый: %v", off)
	}
	// Не реагирует: дорожка, значок, «›», колесо, клавиши.
	track := desktop.QSZoneRect(q, "voltrack")
	qsClick(q, image.Pt(track.Min.X+track.Dx()/2, track.Min.Y+track.Dy()/2))
	qsClick(q, centreOf(desktop.QSZoneRect(q, "volicon")))
	qsClick(q, centreOf(desktop.QSZoneRect(q, "volchev")))
	vol := desktop.QSZoneRect(q, "vol")
	q.OnMouseWheelPixels(vol.Min.X+50, vol.Min.Y+5, 0, -40)
	if len(levels) != 0 || mutes != 0 {
		t.Errorf("неактивная громкость отозвалась: уровни %v, выключений звука %d", levels, mutes)
	}
	if _, open := q.DetailsOpen(); open {
		t.Error("«›» неактивной громкости открыла страницу")
	}
	if tip := q.ToolTipAt(track.Min.X+5, track.Min.Y+3); tip != "" {
		t.Errorf("у неактивного ползунка подсказка %q", tip)
	}
	// Выпадает из обхода Tab, остальной обход цел.
	q.SetFocused(true)
	seen := map[int]bool{}
	for i := 0; i < 40; i++ {
		qsKey(q, widget.KeyTab, 0)
		k, _ := desktop.QSFocus(q)
		seen[k] = true
	}
	for _, k := range []int{desktop.QSKindVol, desktop.QSKindVolIcon, desktop.QSKindVolChev} {
		if seen[k] {
			t.Errorf("Tab встал на неактивную громкость (вид %d)", k)
		}
	}
	if !seen[desktop.QSKindBright] || !seen[desktop.QSKindEdit] {
		t.Errorf("Tab потерял соседние зоны: %v", seen)
	}
	// Яркость при этом живая.
	// Включённый обратно ползунок работает как раньше.
	q.SetVolumeEnabled(true)
	if got := trackPix(s, "voltrack"); got != on {
		t.Errorf("включённый обратно ползунок %v, был %v", got, on)
	}
	qsClick(q, image.Pt(track.Min.X+track.Dx()/2, track.Min.Y+track.Dy()/2))
	if len(levels) == 0 {
		t.Error("включённый обратно ползунок не отзывается")
	}
}

func TestQuickPanel_BrightnessDisabled(t *testing.T) {
	testreset.Globals(t)
	var got []float64
	s := qsBuild(t, qsOpts{setup: func(q *desktop.QuickSettings) {
		q.OnBrightnessChange = func(v float64) { got = append(got, v) }
	}})
	q := s.q
	on := trackPix(s, "brighttrack")
	q.SetBrightnessEnabled(false)
	if trackPix(s, "brighttrack") == on {
		t.Error("неактивная яркость нарисована как активная")
	}
	track := desktop.QSZoneRect(q, "brighttrack")
	qsClick(q, image.Pt(track.Max.X-2, track.Min.Y+track.Dy()/2))
	qsKey(q, widget.KeyEnd, 0)
	if len(got) != 0 {
		t.Errorf("неактивная яркость отозвалась: %v", got)
	}
	if !q.VolumeEnabled() {
		t.Error("яркость выключила и громкость")
	}
	q.SetBrightnessEnabled(true)
	qsClick(q, image.Pt(track.Max.X-2, track.Min.Y+track.Dy()/2))
	if len(got) == 0 {
		t.Error("включённая обратно яркость не отзывается")
	}
}

// Ползунок выключили посреди перетаскивания — оно обрывается.
func TestQuickPanel_DisableWhileDragging(t *testing.T) {
	testreset.Globals(t)
	var levels []float64
	s := qsBuild(t, qsOpts{setup: func(q *desktop.QuickSettings) {
		q.OnVolumeChange = func(v float64) { levels = append(levels, v) }
	}})
	q := s.q
	track := desktop.QSZoneRect(q, "voltrack")
	mid := image.Pt(track.Min.X+track.Dx()/2, track.Min.Y+track.Dy()/2)
	q.OnMouseButton(widget.MouseEvent{X: mid.X, Y: mid.Y, Button: widget.MouseLeft, Pressed: true})
	n := len(levels)
	q.SetVolumeEnabled(false)
	q.OnMouseMove(track.Max.X-3, mid.Y)
	if len(levels) != n {
		t.Errorf("перетаскивание продолжилось после выключения: %v", levels)
	}
	q.OnMouseButton(widget.MouseEvent{X: track.Max.X - 3, Y: mid.Y, Button: widget.MouseLeft})
}

// ─── Подпись в две строки ────────────────────────────────────────────────────

// На экране: подпись из двух слов легла на две строки, детали (имя сети) —
// только когда название заняло одну.
func TestQuickPanel_LabelTwoLinesOnScreen(t *testing.T) {
	testreset.Globals(t)
	s := qsBuild(t, qsOpts{})
	q := s.q
	img := s.render()
	ink := func(r image.Rectangle) int {
		bg := img.RGBAAt(r.Min.X+1, r.Min.Y+1)
		n := 0
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X + 2; x < r.Max.X-2; x++ {
				if img.RGBAAt(x, y) != bg {
					n++
				}
			}
		}
		return n
	}
	rows := func(id desktop.QuickActionID) (first, second int) {
		l := desktop.QSLabelRect(q, id)
		h := l.Dy() / 2
		return ink(image.Rect(l.Min.X, l.Min.Y, l.Max.X, l.Min.Y+h)), ink(image.Rect(l.Min.X, l.Min.Y+h, l.Max.X, l.Max.Y))
	}
	if a, b := rows("plane"); a == 0 || b == 0 {
		t.Errorf("«Режим «в самолёте»» не в две строки: %d и %d пикселей", a, b)
	}
	if a, b := rows("access"); a == 0 || b == 0 {
		t.Errorf("«Специальные возможности» не в две строки: %d и %d", a, b)
	}
	if a, b := rows("night"); a == 0 || b != 0 {
		t.Errorf("«Ночной свет» должен занять одну строку: %d и %d", a, b)
	}
	// У Wi-Fi вторая строка — подробность («Avanpost»).
	if a, b := rows("wifi"); a == 0 || b == 0 {
		t.Errorf("Wi-Fi: название %d, подробность %d", a, b)
	}
}

var _ = theme.StateNormal

// settled снимает кадр после того, как плавные переходы цвета дошли до конца:
// первый кадр запускает переход, второй рисует его итог.
func (s *qsScene) settled() *image.RGBA {
	s.render()
	qsFinish()
	return s.render()
}
