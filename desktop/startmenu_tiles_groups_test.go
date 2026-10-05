package desktop

import (
	"fmt"
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/widget"
)

// Новая группа плиток: плитку отпускают в пустое место ниже последней группы,
// между группами или над первой.

// groupsOrder — «g1:t0 t1|g2:w0» для сравнения групп глазами.
func groupsOrder(gs []TileGroup) string {
	s := ""
	for i, g := range gs {
		if i > 0 {
			s += "|"
		}
		s += g.ID + ":"
		for j, t := range g.Tiles {
			if j > 0 {
				s += " "
			}
			s += string(t.ID)
		}
	}
	return s
}

// dragTile несёт плитку id в точку to и, если drop, отпускает её там. Возвращает
// группы, сообщённые OnTilesChanged (nil — не вызывался).
func dragTile(t *testing.T, m *StartMenu, id TileID, to image.Point, drop bool) []TileGroup {
	t.Helper()
	var got []TileGroup
	m.OnTilesChanged = func(g []TileGroup) { got = g }
	from := m.tileRectAbs(id)
	if from.Empty() {
		t.Fatalf("плитки %q нет на экране", id)
	}
	fx, fy := pointIn(from)
	m.OnMouseMove(fx, fy)
	pressMenu(m, fx, fy)
	m.OnMouseMove(fx+10, fy+10)
	m.OnMouseMove(to.X, to.Y)
	if !m.v.drag.active {
		t.Fatal("перетаскивание не началось")
	}
	if drop {
		release(m, to.X, to.Y)
	}
	return got
}

// tilesPt — точка в области плиток: x от левого края сетки, y от верха первой
// группы при нулевой прокрутке.
func tilesPt(m *StartMenu, x, y int) image.Point {
	g := m.startGeometry(m.contentRect())
	return image.Pt(g.tinner.Min.X+x, g.tinner.Min.Y+y)
}

// Под последней группой плитка создаёт новую группу.
func TestGroups_DropBelowLastCreatesGroup(t *testing.T) {
	m, _ := tiledMenu(t)
	got := dragTile(t, m, "t0", tilesPt(m, 100, 560), true)
	if got == nil {
		t.Fatal("OnTilesChanged не вызван")
	}
	if len(got) != 3 {
		t.Fatalf("групп %d, ждали 3: %s", len(got), groupsOrder(got))
	}
	if n := len(got[0].Tiles); n != 5 {
		t.Errorf("в первой группе %d плиток, ждали 5 (t0 унесли)", n)
	}
	last := got[2]
	if len(last.Tiles) != 1 || last.Tiles[0].ID != "t0" {
		t.Errorf("новая группа: %+v, ждали одну плитку t0", last)
	}
	if last.Title != "" {
		t.Errorf("название новой группы %q, ждали пустое", last.Title)
	}
	if last.ID == "" || last.ID == got[0].ID || last.ID == got[1].ID {
		t.Errorf("идентификатор новой группы %q не уникален", last.ID)
	}
	// Модель меню обновилась, а перетаскивание закончилось.
	if s := groupsOrder(m.TileGroups()); s != groupsOrder(got) {
		t.Errorf("TileGroups %q расходится с событием %q", s, groupsOrder(got))
	}
	if m.v.drag.active || m.v.drag.pending {
		t.Error("перетаскивание не закончилось")
	}
	// Раскладка получила третий заголовок и плитку в нём.
	l := m.layoutTiles(m.startGeometry(m.contentRect()))
	if len(l.groups) != 3 || m.tileRectAbs("t0").Empty() {
		t.Errorf("раскладка после создания группы: %d групп, t0 на экране: %v", len(l.groups), !m.tileRectAbs("t0").Empty())
	}
}

// Между двумя группами новая группа встаёт между ними.
func TestGroups_DropBetweenGroupsCreatesGroup(t *testing.T) {
	m, _ := tiledMenu(t)
	g := m.startGeometry(m.contentRect())
	// Раскладка без переносимой s1: первая группа та же, граница под ней.
	rest, _, _ := withoutTile(m.TileGroups(), "s1")
	l := computeTileLayout(rest, m.tileKeyFor(g))
	y := l.groups[0].bottom + 2 // зазор между группами

	got := dragTile(t, m, "s1", tilesPt(m, 100, y), true)
	if groupsOrder(got) != "g1:t0 t1 t2 t3 t4 t5|group-3:s1|g2:w0 s0" {
		t.Errorf("порядок %q", groupsOrder(got))
	}
}

// Над первой группой — новая первая.
func TestGroups_DropAboveFirstCreatesFirstGroup(t *testing.T) {
	m, _ := tiledMenu(t)
	got := dragTile(t, m, "w0", tilesPt(m, 100, 3), true)
	if groupsOrder(got) != "group-3:w0|g1:t0 t1 t2 t3 t4 t5|g2:s0 s1" {
		t.Errorf("порядок %q", groupsOrder(got))
	}
}

// Нижняя половина заголовка и ряд плиток по-прежнему «в эту группу».
func TestGroups_DropOnHeaderLowerHalfOrTilesStaysInGroup(t *testing.T) {
	m, _ := tiledMenu(t)
	g := m.startGeometry(m.contentRect())
	rest, _, _ := withoutTile(m.TileGroups(), "s1")
	l := computeTileLayout(rest, m.tileKeyFor(g))
	y := l.groups[1].header.Min.Y + l.groups[1].header.Dy()*3/4 // нижняя половина заголовка g2

	got := dragTile(t, m, "t0", tilesPt(m, 100, y), true)
	if len(got) != 2 || got[1].Tiles[0].ID != "t0" {
		t.Errorf("плитка должна встать первой во вторую группу: %q", groupsOrder(got))
	}
}

// Группа, опустевшая после переноса, удаляется.
func TestGroups_EmptiedSourceGroupIsRemoved(t *testing.T) {
	m, _ := tiledMenu(t)
	one := func(id string) Tile { return Tile{ID: TileID(id), App: AppID(id), Size: TileMedium} }
	m.SetTileGroups([]TileGroup{
		{ID: "a", Title: "A", Tiles: []Tile{one("a1")}},
		{ID: "b", Title: "B", Tiles: []Tile{one("b1"), one("b2")}},
	})
	got := dragTile(t, m, "a1", tilesPt(m, 100, 560), true)
	if groupsOrder(got) != "b:b1 b2|group-3:a1" && groupsOrder(got) != "b:b1 b2|group-2:a1" {
		t.Errorf("порядок %q: группа a должна исчезнуть, a1 — в новой группе", groupsOrder(got))
	}
	if len(m.TileGroups()) != 2 {
		t.Errorf("в меню %d групп, ждали 2", len(m.TileGroups()))
	}
}

// Единственную плитку группы в «новую группу» рядом с её же пустой группой не
// переносят: ничего не меняется и потребителя не беспокоят.
func TestGroups_LoneTileDroppedBesideItsGroupChangesNothing(t *testing.T) {
	m, _ := tiledMenu(t)
	one := func(id string) Tile { return Tile{ID: TileID(id), App: AppID(id), Size: TileMedium} }
	m.SetTileGroups([]TileGroup{
		{ID: "a", Title: "A", Tiles: []Tile{one("a1")}},
		{ID: "b", Title: "B", Tiles: []Tile{one("b1")}},
	})
	g := m.startGeometry(m.contentRect())
	rest, _, _ := withoutTile(m.TileGroups(), "a1")
	l := computeTileLayout(rest, m.tileKeyFor(g))
	// Над группой a (новая первая) и под ней (новая вторая) — оба рядом с a.
	for _, y := range []int{2, l.groups[0].bottom + 2} {
		got := dragTile(t, m, "a1", tilesPt(m, 100, y), true)
		if got != nil {
			t.Errorf("y=%d: OnTilesChanged вызван с %q, ждали тишину", y, groupsOrder(got))
		}
		if s := groupsOrder(m.TileGroups()); s != "a:a1|b:b1" {
			t.Errorf("y=%d: порядок изменился: %q", y, s)
		}
	}
}

// Предпросмотр показывает будущую группу: подсказка вместо названия и плитка
// в ней; вне пустого места подсказки нет.
func TestGroups_PreviewShowsFutureGroup(t *testing.T) {
	m, _ := tiledMenu(t)
	hint := tr(StrStartNewGroup)

	dragTile(t, m, "t0", tilesPt(m, 100, 560), false)
	if !m.v.drag.toNew || m.v.drag.toGroup != 2 {
		t.Fatalf("цель переноса: %+v, ждали новую группу на месте 2", m.v.drag)
	}
	ctx := &recCtx{}
	m.drawContent(ctx, m.contentRect())
	if !containsText(ctx.texts, hint) {
		t.Errorf("в предпросмотре нет подсказки %q", hint)
	}
	groups, ni := dragApply(m.TileGroups(), m.v.drag, false)
	if ni != 2 || len(groups) != 3 || groups[2].Tiles[0].ID != "t0" {
		t.Errorf("предпросмотр: %q, новая группа %d", groupsOrder(groups), ni)
	}
	// Данные меню пока прежние: предпросмотр ничего не сохраняет.
	if len(m.TileGroups()) != 2 {
		t.Error("предпросмотр изменил модель до отпускания")
	}

	// Плитку несут над существующей группой — будущей группы нет.
	to := m.tileRectAbs("w0")
	mx, my := pointIn(to)
	m.OnMouseMove(mx, my)
	if m.v.drag.toNew {
		t.Error("над плиткой цель стала новой группой")
	}
	ctx = &recCtx{}
	m.drawContent(ctx, m.contentRect())
	if containsText(ctx.texts, hint) {
		t.Error("подсказка новой группы осталась, когда цель — старая группа")
	}
	release(m, mx, my)
}

// Запас снизу: при заполненной области плитку тоже можно отпустить под
// последней группой — пока несут, прокрутка уходит дальше конца содержимого.
func TestGroups_DropBelowLastWhenContentFillsArea(t *testing.T) {
	m, _ := scrollMenu(t)
	g := m.startGeometry(m.contentRect())
	var got []TileGroup
	m.OnTilesChanged = func(gs []TileGroup) { got = gs }

	from := m.tileRectAbs("g0t0")
	fx, fy := pointIn(from)
	m.OnMouseMove(fx, fy)
	pressMenu(m, fx, fy)
	m.OnMouseMove(fx+10, fy+10)
	// Тянем к нижней кромке: каждое движение у неё прокручивает область.
	bottom := image.Pt(fx, g.tinner.Max.Y-2)
	for i := 0; i < 300; i++ {
		m.OnMouseMove(bottom.X, bottom.Y)
	}
	l := m.layoutTiles(g)
	if m.v.tileScroll <= l.height-tilesViewHeight(g) {
		t.Fatalf("при переносе прокрутка %d не ушла за конец содержимого (%d): нет места под последней группой",
			m.v.tileScroll, l.height-tilesViewHeight(g))
	}
	if !m.v.drag.toNew || m.v.drag.toGroup != 8 {
		t.Fatalf("цель переноса %+v, ждали новую группу в конце (8)", m.v.drag)
	}
	release(m, bottom.X, bottom.Y)
	if len(got) != 9 || got[8].Tiles[0].ID != "g0t0" {
		t.Errorf("групп %d: %s", len(got), fmt.Sprint(groupsOrder(got)))
	}
	// После переноса прокрутка возвращается в допустимые рамки.
	l = m.layoutTiles(g)
	if max := l.height - tilesViewHeight(g); m.v.tileScroll > max {
		// clampScroll при рисовании приведёт значение в рамки; само поле может
		// хранить прежнее, но видимая прокрутка не выходит за конец.
		if c := clampScroll(m.v.tileScroll, l.height, tilesViewHeight(g)); c > max {
			t.Errorf("видимая прокрутка %d за пределом %d", c, max)
		}
	}
}

// Отпускание за пределами меню завершает перенос, а не оставляет плитку
// «зажатой».
func TestGroups_ReleaseOutsideMenuCompletesDrop(t *testing.T) {
	m, _ := tiledMenu(t)
	var got []TileGroup
	m.OnTilesChanged = func(gs []TileGroup) { got = gs }
	from := m.tileRectAbs("t0")
	fx, fy := pointIn(from)
	pressMenu(m, fx, fy)
	m.OnMouseMove(fx+10, fy+10)
	to := tilesPt(m, 100, 560)
	m.OnMouseMove(to.X, to.Y)
	release(m, m.OverlayBounds().Max.X+300, to.Y)
	if m.v.drag.active || m.v.drag.pending {
		t.Error("перенос остался активным после отпускания вне меню")
	}
	if got == nil {
		t.Error("отпускание вне меню не сообщило новый порядок")
	}
	if !m.IsOpen() {
		t.Error("меню закрылось")
	}
}

// Закрытие меню во время переноса не оставляет его активным.
func TestGroups_CloseDuringDragCancels(t *testing.T) {
	m, _ := tiledMenu(t)
	dragTile(t, m, "t0", tilesPt(m, 100, 560), false)
	m.Close()
	if m.v.drag.active || m.v.drag.pending {
		t.Error("перенос пережил закрытие меню")
	}
	if groupsOrder(m.TileGroups()) != groupsOrder(tileGroups()) {
		t.Error("закрытие во время переноса изменило группы")
	}
}

// Идентификатор новой группы не совпадает с занятыми.
func TestGroups_NewGroupIDIsUnique(t *testing.T) {
	gs := []TileGroup{{ID: "group-2"}, {ID: "group-3"}}
	if id := newGroupID(gs); id == "group-2" || id == "group-3" {
		t.Errorf("идентификатор %q уже занят", id)
	}
}

// Перетаскивание плитки в существующую группу работает как прежде: порядок
// плиток внутри группы и между группами, без лишних групп.
func TestGroups_ExistingDropTargetsUnchanged(t *testing.T) {
	m, _ := tiledMenu(t)
	to := m.tileRectAbs("t1")
	got := dragTile(t, m, "t0", image.Pt(to.Max.X-10, to.Min.Y+50), true)
	if groupsOrder(got) != "g1:t1 t2 t0 t3 t4 t5|g2:w0 s0 s1" {
		t.Errorf("порядок %q", groupsOrder(got))
	}
	_ = widget.MouseLeft
}

// Раскладка плиток следует за моделью: после SetTileGroups кэш не отдаёт
// прежнюю раскладку (раньше он сбрасывался только сменой метрик).
func TestGroups_LayoutFollowsModelChange(t *testing.T) {
	m, _ := tiledMenu(t)
	g := m.startGeometry(m.contentRect())
	if n := len(m.layoutTiles(g).groups); n != 2 {
		t.Fatalf("исходных групп %d, ждали 2", n)
	}
	m.SetTileGroups(manyTileGroups())
	if n := len(m.layoutTiles(g).groups); n != 8 {
		t.Errorf("после SetTileGroups в раскладке %d групп, ждали 8", n)
	}
}
