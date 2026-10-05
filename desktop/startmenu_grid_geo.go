// startmenu_grid_geo.go — выбор вида Windows 11, размер, положение и раскладка
// областей меню «Пуск» с сеткой закреплённых.
//
// Раскладка и попадание мыши считаются одной арифметикой (gridGeometry), как и у
// меню Windows 10: подсветка, клик и клавиатура не расходятся. Метрики —
// startmenu.w11.* профиля; значение, которого профиль не объявил, заменяется
// запасным (gm), чтобы меню не схлопывалось в ноль у потребителя со своим
// профилем.
package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
)

// Ключи метрик меню «Пуск» Windows 11 (логические пиксели при 100 %).
const (
	KeyStartW11Width        theme.Key = "startmenu.w11.width"
	KeyStartW11Height       theme.Key = "startmenu.w11.height"
	KeyStartW11HeightMin    theme.Key = "startmenu.w11.height.min"
	KeyStartW11Corner       theme.Key = "startmenu.w11.corner"
	KeyStartW11Pad          theme.Key = "startmenu.w11.pad"
	KeyStartW11Margin       theme.Key = "startmenu.w11.margin"
	KeyStartW11GridColumns  theme.Key = "startmenu.w11.grid.columns"
	KeyStartW11GridCellW    theme.Key = "startmenu.w11.grid.cell.w"
	KeyStartW11GridCellH    theme.Key = "startmenu.w11.grid.cell.h"
	KeyStartW11GridGapY     theme.Key = "startmenu.w11.grid.gap.y"
	KeyStartW11GridIcon     theme.Key = "startmenu.w11.grid.icon"
	KeyStartW11FooterHeight theme.Key = "startmenu.w11.footer.height"
	KeyStartW11SearchTop    theme.Key = "startmenu.w11.search.top"
	KeyStartW11SearchHeight theme.Key = "startmenu.w11.search.height"
	KeyStartW11SearchCorner theme.Key = "startmenu.w11.search.corner"
	KeyStartW11SearchPad    theme.Key = "startmenu.w11.search.pad"
	KeyStartW11SearchIcon   theme.Key = "startmenu.w11.search.icon"
	KeyStartW11SectionH     theme.Key = "startmenu.w11.section.height"
	KeyStartW11SectionGap   theme.Key = "startmenu.w11.section.gap"
	KeyStartW11RecColumns   theme.Key = "startmenu.w11.rec.columns"
	KeyStartW11RecRows      theme.Key = "startmenu.w11.rec.rows"
	KeyStartW11RecRowH      theme.Key = "startmenu.w11.rec.row.height"
	KeyStartW11RecIcon      theme.Key = "startmenu.w11.rec.icon"
	KeyStartW11DotSize      theme.Key = "startmenu.w11.dots.size"
	KeyStartW11DotGap       theme.Key = "startmenu.w11.dots.gap"
	KeyStartW11Avatar       theme.Key = "startmenu.w11.footer.avatar"
	KeyStartW11FooterButton theme.Key = "startmenu.w11.footer.button"
)

// grid сообщает, что активная тема просит меню Windows 11. Спрашивается у темы
// каждый раз: смена темы на открытом меню меняет вид без пересоздания.
func (m *StartMenu) grid() bool {
	if m.tm == nil {
		return false
	}
	t := m.tm.Active()
	return t != nil && t.PresenterName(ComponentStartMenu) == theme.PresenterStartGrid
}

// modern — меню рисует и разбирает ввод само (плитки Windows 10 или сетка
// Windows 11), а не плоским списком.
func (m *StartMenu) modern() bool { return m.tiled() || m.grid() }

// kind — какой вид просит тема сейчас.
func (m *StartMenu) kind() startKind {
	switch {
	case m.grid():
		return startKindGrid
	case m.tiled():
		return startKindTiles
	}
	return startKindFlat
}

// gm читает метрику вида Windows 11 с запасным значением def.
func (m *StartMenu) gm(k theme.Key, def int) int {
	if v := m.metricInt(k); v > 0 {
		return v
	}
	return def
}

// ─── Размер и положение ──────────────────────────────────────────────────────

// sizeGrid — Flyout.Size меню Windows 11: ширина по метрике, высота ужата под
// экран за вычетом панели задач и зазора, но не меньше наименьшей, пока это
// помещается.
func (m *StartMenu) sizeGrid() image.Point {
	w, h := m.gm(KeyStartW11Width, 642), m.gm(KeyStartW11Height, 726)
	if !m.Screen.Empty() && w > m.Screen.Dx() {
		w = m.Screen.Dx()
	}
	avail := m.availHeight()
	if avail > 0 && h > avail {
		h = avail
	}
	if min := m.gm(KeyStartW11HeightMin, 360); h < min {
		h = min
		if avail > 0 && h > avail {
			h = avail
		}
	}
	return image.Pt(w, h)
}

// placeGrid — Flyout.Place меню Windows 11: над кнопкой «Пуск» с зазором, по
// центру группы кнопок при центрированной панели (флаг taskbar.centered) и у
// левого края кнопки при левой. fitInto во Flyout не даёт меню выйти за экран.
// Боковые панели размещает обычный расчёт (false).
func (m *StartMenu) placeGrid(anchor, screen image.Rectangle, edge Edge, size image.Point) (image.Rectangle, bool) {
	if !m.grid() || edge.Vertical() {
		return image.Rectangle{}, false
	}
	margin := m.flyoutMargin()
	var y int
	switch {
	case anchor.Empty():
		y = screen.Max.Y - margin - size.Y
	case edge == EdgeTop:
		y = anchor.Max.Y + margin
	default:
		y = anchor.Min.Y - margin - size.Y
	}
	var x int
	if m.tm.GetFlag(KeyTaskbarCentered, false) {
		cx := screen.Min.X + screen.Dx()/2
		if m.GroupBounds != nil {
			if gb := m.GroupBounds(); !gb.Empty() {
				cx = (gb.Min.X + gb.Max.X) / 2
			}
		}
		x = cx - size.X/2
	} else if anchor.Empty() {
		x = screen.Min.X + margin
	} else {
		x = anchor.Min.X
	}
	return image.Rect(x, y, x+size.X, y+size.Y), true
}

// ─── Раскладка ───────────────────────────────────────────────────────────────

// gridMode — что занимает середину меню под строкой поиска.
type gridMode int

const (
	gridMain    gridMode = iota // закреплённые и «Рекомендуем»
	gridList                    // «Все приложения»
	gridMore                    // «Все рекомендации»
	gridResults                 // результаты поиска
)

// gridGeo — области меню Windows 11 в абсолютных координатах.
type gridGeo struct {
	panel image.Rectangle
	mode  gridMode
	pad   int

	search image.Rectangle

	// Главный вид.
	pinTitle, allBtn          image.Rectangle
	pinGrid                   image.Rectangle
	cols, rows, perPage       int
	cellW, cellH, gapY, cellX int // cellX — левый край первой колонки
	pages                     int
	dots                      image.Rectangle // колонка точек страниц (пусто — страница одна)
	recTitle, moreBtn         image.Rectangle
	recList                   image.Rectangle
	recCols, recRows, recRowH int

	// Вид-список («Все приложения», «Все рекомендации», результаты поиска).
	listTitle, backBtn image.Rectangle
	list               image.Rectangle

	// Нижняя полоса.
	footer, user, power image.Rectangle
}

// gridModeNow — какой вид показан сейчас: результаты поиска главнее остальных.
func (m *StartMenu) gridModeNow() gridMode {
	if m.Query() != "" {
		return gridResults
	}
	switch m.View() {
	case StartViewAllApps:
		return gridList
	case StartViewRecommended:
		return gridMore
	}
	return gridMain
}

// gridGeometry раскладывает панель по областям. Читает модель (число
// закреплённых и рекомендаций) и текущий вид; ничего не меняет.
func (m *StartMenu) gridGeometry(panel image.Rectangle) gridGeo {
	return m.gridGeometryFor(panel, m.gridModeNow(), len(m.pinnedList()), len(m.recommendedList()))
}

// pageNow — текущая страница закреплённых.
func (m *StartMenu) pageNow() int {
	m.g.mu.Lock()
	defer m.g.mu.Unlock()
	return m.g.page
}

// gridGeometryFor — раскладка для заданных режима, числа закреплённых pinned и
// числа рекомендаций rec.
func (m *StartMenu) gridGeometryFor(panel image.Rectangle, mode gridMode, pinned, rec int) gridGeo {
	g := gridGeo{panel: panel, mode: mode}
	g.pad = m.gm(KeyStartW11Pad, 32)
	if max := panel.Dx() / 4; g.pad > max {
		g.pad = max // на узком экране поля не съедают меню
	}
	// Рамка панели — один пиксель внутрь: нижняя полоса и списки её не затирают.
	inner := panel.Inset(1)
	x0, x1 := panel.Min.X+g.pad, panel.Max.X-g.pad

	top := m.gm(KeyStartW11SearchTop, 28)
	sh := m.gm(KeyStartW11SearchHeight, 32)
	g.search = image.Rect(x0, panel.Min.Y+top, x1, panel.Min.Y+top+sh)

	fh := m.gm(KeyStartW11FooterHeight, 64)
	g.footer = image.Rect(inner.Min.X, inner.Max.Y-fh, inner.Max.X, inner.Max.Y)
	m.layoutFooter(&g)

	secH := m.gm(KeyStartW11SectionH, 28)
	secGap := m.gm(KeyStartW11SectionGap, 16)
	bottom := g.footer.Min.Y - 8

	switch mode {
	case gridResults:
		g.list = image.Rect(panel.Min.X+g.pad/2, g.search.Max.Y+12, panel.Max.X-g.pad/2, g.footer.Min.Y)
		return g
	case gridList, gridMore:
		hy := g.search.Max.Y + secGap + 4
		g.listTitle = image.Rect(x0, hy, x1, hy+secH)
		g.backBtn = m.linkRect(g.listTitle, tr(StrStartBack))
		g.list = image.Rect(panel.Min.X+g.pad/2, g.listTitle.Max.Y+8, panel.Max.X-g.pad/2, g.footer.Min.Y)
		return g
	}

	// Главный вид.
	g.cellW, g.cellH = m.gm(KeyStartW11GridCellW, 96), m.gm(KeyStartW11GridCellH, 84)
	g.gapY = m.metricInt(KeyStartW11GridGapY)
	g.cols = m.gm(KeyStartW11GridColumns, 6)
	if max := (panel.Dx() - g.pad) / g.cellW; g.cols > max {
		g.cols = max
	}
	if g.cols < 1 {
		g.cols = 1
	}
	g.cellX = panel.Min.X + (panel.Dx()-g.cols*g.cellW)/2

	g.pinTitle = image.Rect(x0, g.search.Max.Y+secGap+4, x1, g.search.Max.Y+secGap+4+secH)
	g.allBtn = m.linkRect(g.pinTitle, tr(StrStartAllApps))
	gridTop := g.pinTitle.Max.Y + 8

	// Сколько рядов рекомендаций оставить: три, если закреплённым остаётся хотя
	// бы два ряда; иначе меньше — вплоть до исчезновения раздела.
	cols := m.gm(KeyStartW11RecColumns, 2)
	rowH := m.gm(KeyStartW11RecRowH, 56)
	want := m.gm(KeyStartW11RecRows, 3)
	if need := (rec + cols - 1) / cols; need < want {
		want = need
	}
	pitch := g.cellH + g.gapY
	recBlock := func(rows int) int {
		if rows <= 0 {
			return 0
		}
		return secGap + secH + 8 + rows*rowH
	}
	rows, recRows := 0, want
	for ; ; recRows-- {
		avail := bottom - gridTop - recBlock(recRows)
		rows = (avail + g.gapY) / pitch
		minRows := 2
		if pinned <= g.cols {
			minRows = 1
		}
		if rows >= minRows || recRows <= 0 {
			break
		}
	}
	if rows < 1 {
		rows = 1
	}
	g.rows, g.recRows, g.recCols, g.recRowH = rows, recRows, cols, rowH
	g.perPage = g.cols * rows
	g.pinGrid = image.Rect(g.cellX, gridTop, g.cellX+g.cols*g.cellW, gridTop+rows*g.cellH+(rows-1)*g.gapY)
	g.pages = 1
	if pinned > g.perPage {
		g.pages = (pinned + g.perPage - 1) / g.perPage
	}
	if g.pages > 1 {
		d := m.gm(KeyStartW11DotSize, 6)
		gap := m.gm(KeyStartW11DotGap, 8)
		h := g.pages*d + (g.pages-1)*gap
		cx := panel.Max.X - g.pad/2
		cy := (g.pinGrid.Min.Y + g.pinGrid.Max.Y) / 2
		g.dots = image.Rect(cx-g.pad/4, cy-h/2-gap, cx+g.pad/4+1, cy+h/2+gap)
	}

	if recRows > 0 {
		recBottom := bottom
		g.recList = image.Rect(x0, recBottom-recRows*rowH, x1, recBottom)
		ty := g.recList.Min.Y - 8 - secH
		g.recTitle = image.Rect(x0, ty, x1, ty+secH)
		g.moreBtn = m.linkRect(g.recTitle, tr(StrStartMore))
	}
	return g
}

// linkRect — прямоугольник кнопки у заголовка (в строке row, у правого края):
// шрифт подписи и поля плашки считаются по стилю части link. Ширина берётся из
// MeasureUIText: кнопка раскладывается вне Draw. Шеврон («›» справа, «‹» слева
// у «Назад») занимает место рядом с подписью.
func (m *StartMenu) linkRect(row image.Rectangle, label string) image.Rectangle {
	if label == "" {
		return image.Rectangle{}
	}
	s := m.tpart("link", theme.StateNormal)
	w := measureUI(label, fontPt(s)) + m.linkChevronW(fontPt(s)) + 2*m.linkPad()
	h := row.Dy()
	return image.Rect(row.Max.X-w, row.Min.Y, row.Max.X, row.Min.Y+h)
}

// linkPad — поле плашки кнопки у заголовка.
func (m *StartMenu) linkPad() int { return 10 }

// linkChevronW — место под шеврон рядом с подписью кнопки.
func (m *StartMenu) linkChevronW(pt float64) int { return int(pt*1.4) + 4 }

// layoutFooter раскладывает кнопки нижней полосы: пользователь слева, питание
// справа.
func (m *StartMenu) layoutFooter(g *gridGeo) {
	f := g.footer
	bh := m.gm(KeyStartW11FooterButton, 40) + 8
	if bh > f.Dy() {
		bh = f.Dy()
	}
	av := m.gm(KeyStartW11Avatar, 32)
	name := m.User().Name
	s := m.tpart("footer.item", theme.StateNormal)
	nameW := 0
	if name != "" {
		nameW = measureUI(name, fontPt(s))
	}
	w := 8 + av + 12 + nameW + 16
	if name == "" {
		w = 8 + av + 8
	}
	y := f.Min.Y + (f.Dy()-bh)/2
	left := g.panel.Min.X + g.pad - 8
	g.user = image.Rect(left, y, left+w, y+bh)
	pw := m.gm(KeyStartW11FooterButton, 40) + 8
	right := g.panel.Max.X - g.pad + 8
	g.power = image.Rect(right-pw, y, right, y+bh)
	if g.user.Max.X > g.power.Min.X {
		g.user.Max.X = g.power.Min.X - 4
	}
}

// pageCount возвращает число страниц закреплённых в главном виде.
func (m *StartMenu) pageCount() int {
	panel := m.rect()
	if panel.Empty() {
		return 1
	}
	g := m.gridGeometryFor(panel, gridMain, len(m.pinnedList()), len(m.recommendedList()))
	return g.pages
}

// ─── Геометрия для общих помощников списка ───────────────────────────────────

// gridStartGeo — startGeo для видов, использующих общий список (строки,
// буквы, полоса прокрутки): окно списка — g.list, боковой панели и плиток нет.
func (m *StartMenu) gridStartGeo(inner image.Rectangle) startGeo {
	g := m.gridGeometry(inner)
	return startGeo{inner: inner, list: g.list}
}
