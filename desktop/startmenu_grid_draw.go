// startmenu_grid_draw.go — отрисовка меню «Пуск» Windows 11.
//
// Порядок слоёв: середина (закреплённые и «Рекомендуем» либо вид-список), строка
// поиска, нижняя полоса, перетаскиваемая ячейка. Каждая область клипуется своим
// прямоугольником, поэтому частичная перерисовка (наведение, шаг перехода цвета)
// рисует только то, что попало в заявленную область. Список «Все приложения» и
// результаты поиска рисует общий drawList меню Windows 10 — тот же код, те же
// буквы, папки, полоса прокрутки.
//
// Draw ничего не меняет: всё, что он читает, — снимки состояния и неизменяемые
// после публикации модели.
package desktop

import (
	"image"
	"strconv"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Ключи наведения и выбора вида Windows 11; префиксы не пересекаются с
// s: r: t: g: b: других видов.
const (
	prefPin   = "p:" // ячейка закреплённого
	prefRec   = "x:" // строка «Рекомендуем»
	prefDot   = "d:" // точка страницы
	keySearch = "w:search"
	keyAll    = "w:all"  // «Все приложения ›»
	keyMore   = "w:more" // «Дополнительно ›»
	keyBack   = "w:back" // «‹ Назад»
	keyUser   = "f:user"
	keyPower  = "f:power"
)

// Значки-шевроны вида Windows 11 (контуры в сетке 24×24, как у glyphPaths).
const (
	glyphChevronRight StartGlyph = iota + 120
	glyphChevronLeft
)

func init() {
	glyphPaths[glyphChevronRight] = "M8.4 19.4 L7.3 18.3 L13.6 12 L7.3 5.7 L8.4 4.6 L15.8 12Z"
	glyphPaths[glyphChevronLeft] = "M15.6 19.4 L16.7 18.3 L10.4 12 L16.7 5.7 L15.6 4.6 L8.2 12Z"
}

// gridSnap — снимок состояния вида Windows 11 для одного кадра.
type gridSnap struct {
	page, caret int
	drag        pinDrag
}

func (m *StartMenu) gridSnapshot() gridSnap {
	g := m.g
	g.mu.Lock()
	defer g.mu.Unlock()
	return gridSnap{page: g.page, caret: g.caret, drag: g.drag}
}

// measureUI — ширина строки обычным шрифтом интерфейса размером pt.
func measureUI(text string, pt float64) int { return widget.MeasureUIText(text, pt) }

// drawGrid рисует меню Windows 11. Прямоугольник панели берётся у Flyout: он же
// сдвинут шагом появления, и содержимое едет вместе с подложкой.
func (m *StartMenu) drawGrid(ctx widget.DrawContext, _ image.Rectangle) {
	panel := m.rect()
	if panel.Empty() {
		return
	}
	g := m.gridGeometry(panel)
	sn := m.snapshot()
	gs := m.gridSnapshot()
	prev := ctx.Clip()
	defer ctx.SetClip(prev)

	switch g.mode {
	case gridMain:
		m.drawGridMain(ctx, g, sn, gs, prev)
	default:
		m.drawGridListView(ctx, g, sn, prev)
	}
	m.drawSearchField(ctx, g, sn, gs, prev)
	m.drawFooter(ctx, g, sn, prev)
	if gs.drag.active {
		m.drawPinGhost(ctx, g, gs, prev)
	}
}

// stateFor — состояние объекта с ключом key: нажат, под курсором, выбран
// клавишами в своей области.
func stateFor(sn startSnap, key string, area startArea) theme.State {
	switch {
	case sn.press == key:
		return theme.StatePressed
	case sn.hover == key:
		return theme.StateHover
	case sn.kbd && sn.area == area && sn.sel[area] == key:
		return theme.StateFocused
	}
	return theme.StateNormal
}

// ─── Главный вид ─────────────────────────────────────────────────────────────

func (m *StartMenu) drawGridMain(ctx widget.DrawContext, g gridGeo, sn startSnap, gs gridSnap, prev image.Rectangle) {
	clip := clipTo(ctx, g.panel.Inset(1), prev)
	if clip.Empty() {
		return
	}
	heading := m.tpart("heading", theme.StateNormal)
	if g.pinTitle.Overlaps(clip) {
		drawTextAt(ctx, g.pinTitle, g.pinTitle.Min.X, tr(StrStartPinned), heading)
		m.drawLink(ctx, g.allBtn, keyAll, tr(StrStartAllApps), true, sn, areaPinned)
	}

	// Закреплённые: ячейки текущей страницы.
	pinned := m.pinnedList()
	if gs.drag.active {
		pinned = pinDragOrder(pinned, gs.drag)
	}
	page := clampInt(gs.page, 0, g.pages-1)
	first := page * g.perPage
	for i := first; i < len(pinned) && i < first+g.perPage; i++ {
		rect := g.cellRect(i - first)
		if !rect.Overlaps(clip) {
			continue
		}
		p := pinned[i]
		if gs.drag.active && p.ID == gs.drag.id {
			// Место, куда ляжет ячейка: рамка вместо неё самой.
			s := m.tpart("pin", theme.StateHover)
			strokeInset(ctx, rect.Inset(1), 1, s.Text)
			continue
		}
		m.drawPinCell(ctx, rect, p, stateFor(sn, prefPin+p.ID, areaPinned), prefPin+p.ID, sn.kbd && sn.area == areaPinned && sn.sel[areaPinned] == prefPin+p.ID)
	}
	if g.pages > 1 {
		m.drawDots(ctx, g, sn, page)
	}

	// «Рекомендуем».
	if g.recRows > 0 {
		drawTextAt(ctx, g.recTitle, g.recTitle.Min.X, tr(StrStartRecommended), heading)
		m.drawLink(ctx, g.moreBtn, keyMore, tr(StrStartMore), true, sn, areaRec)
		items := m.recommendedList()
		colW := g.recList.Dx() / g.recCols
		for i := 0; i < g.recCols*g.recRows && i < len(items); i++ {
			rect := image.Rect(g.recList.Min.X+(i%g.recCols)*colW, g.recList.Min.Y+(i/g.recCols)*g.recRowH,
				g.recList.Min.X+(i%g.recCols+1)*colW, g.recList.Min.Y+(i/g.recCols+1)*g.recRowH)
			if !rect.Overlaps(clip) {
				continue
			}
			key := prefRec + items[i].ID
			m.drawRecItem(ctx, rect, items[i], stateFor(sn, key, areaRec), key, sn.kbd && sn.area == areaRec && sn.sel[areaRec] == key)
		}
	}
	ctx.SetClip(prev)
}

// cellRect — прямоугольник ячейки с номером i на странице.
func (g gridGeo) cellRect(i int) image.Rectangle {
	col, row := i%g.cols, i/g.cols
	x := g.cellX + col*g.cellW
	y := g.pinGrid.Min.Y + row*(g.cellH+g.gapY)
	return image.Rect(x, y, x+g.cellW, y+g.cellH)
}

// pinCellIcon — квадрат значка в ячейке.
func (m *StartMenu) pinCellIcon(rect image.Rectangle, lineH int) image.Rectangle {
	side := m.gm(KeyStartW11GridIcon, 32)
	content := side + 8 + lineH
	top := rect.Min.Y + (rect.Dy()-content)/2
	if top < rect.Min.Y {
		top = rect.Min.Y
	}
	cx := rect.Min.X + rect.Dx()/2
	return image.Rect(cx-side/2, top, cx-side/2+side, top+side)
}

// drawPinCell рисует ячейку закреплённого: плашка по состоянию, значок и
// подпись в одну строку с многоточием.
func (m *StartMenu) drawPinCell(ctx widget.DrawContext, rect image.Rectangle, p StartPinned, st theme.State, key string, focus bool) {
	s := m.fade.Style(m.tm, key, rect, st, func(st theme.State) *theme.Style { return m.tpart("pin", st) })
	PaintStyle(ctx, rect, s)
	lineH := int(fontPt(s) * 1.4)
	ir := m.pinCellIcon(rect, lineH)
	if p.Icon != nil || p.IconAt != nil {
		drawAppIcon(ctx, p.Icon, p.IconAt, ir)
	}
	label := Elide(ctx, p.Title, s, rect.Dx()-8)
	w := MeasureText(ctx, label, s)
	drawTextTop(ctx, rect.Min.X+(rect.Dx()-w)/2, ir.Max.Y+8, label, s)
	if focus {
		PaintFocusRing(ctx, rect, m.tm, s)
	}
}

// drawRecItem рисует строку «Рекомендуем»: значок, заголовок и подзаголовок.
func (m *StartMenu) drawRecItem(ctx widget.DrawContext, rect image.Rectangle, it StartRecommendedItem, st theme.State, key string, focus bool) {
	s := m.fade.Style(m.tm, key, rect, st, func(st theme.State) *theme.Style { return m.tpart("rec", st) })
	PaintStyle(ctx, rect, s)
	side := m.gm(KeyStartW11RecIcon, 32)
	ix := rect.Min.X + 12
	iy := rect.Min.Y + (rect.Dy()-side)/2
	if it.Icon != nil || it.IconAt != nil {
		drawAppIcon(ctx, it.Icon, it.IconAt, image.Rect(ix, iy, ix+side, iy+side))
	}
	tx := ix + side + 12
	avail := rect.Max.X - tx - 8
	if avail <= 0 {
		return
	}
	sub := m.tpart("rec.sub", theme.StateNormal)
	subS := *sub
	subS.Font = s.Font
	subS.Font.Size = fontPt(s) * 0.92
	line, subLine := int(fontPt(s)*1.4), int(fontPt(&subS)*1.4)
	if it.Subtitle == "" {
		drawTextTop(ctx, tx, rect.Min.Y+(rect.Dy()-line)/2, Elide(ctx, it.Title, s, avail), s)
	} else {
		top := rect.Min.Y + (rect.Dy()-line-subLine)/2
		drawTextTop(ctx, tx, top, Elide(ctx, it.Title, s, avail), s)
		drawTextTop(ctx, tx, top+line, Elide(ctx, it.Subtitle, &subS, avail), &subS)
	}
	if focus {
		PaintFocusRing(ctx, rect, m.tm, s)
	}
}

// drawLink рисует кнопку у заголовка раздела: плашка, подпись и шеврон справа
// (ShowChevron) либо слева («Назад»).
func (m *StartMenu) drawLink(ctx widget.DrawContext, rect image.Rectangle, key, label string, right bool, sn startSnap, area startArea) {
	if rect.Empty() {
		return
	}
	st := stateFor(sn, key, area)
	s := m.fade.Style(m.tm, key, rect, st, func(st theme.State) *theme.Style { return m.tpart("link", st) })
	PaintStyle(ctx, rect, s)
	// Плашка ниже строки заголовка: у неё свои поля.
	pad := m.linkPad()
	pt := fontPt(s)
	cw := m.linkChevronW(pt)
	side := int(pt * 1.1)
	if right {
		drawTextAt(ctx, rect, rect.Min.X+pad, label, s)
		cr := image.Rect(0, 0, side, side).Add(image.Pt(rect.Max.X-pad-side+2, rect.Min.Y+(rect.Dy()-side)/2))
		drawGlyph(ctx, glyphChevronRight, cr, s.Text)
	} else {
		cr := image.Rect(0, 0, side, side).Add(image.Pt(rect.Min.X+pad-2, rect.Min.Y+(rect.Dy()-side)/2))
		drawGlyph(ctx, glyphChevronLeft, cr, s.Text)
		drawTextAt(ctx, rect, rect.Min.X+pad+cw, label, s)
	}
	if st == theme.StateFocused {
		PaintFocusRing(ctx, rect, m.tm, s)
	}
}

// drawDots рисует точки страниц закреплённых справа от сетки.
func (m *StartMenu) drawDots(ctx widget.DrawContext, g gridGeo, sn startSnap, page int) {
	d := m.gm(KeyStartW11DotSize, 6)
	gap := m.gm(KeyStartW11DotGap, 8)
	h := g.pages*d + (g.pages-1)*gap
	cx := (g.dots.Min.X + g.dots.Max.X) / 2
	y := (g.dots.Min.Y+g.dots.Max.Y)/2 - h/2
	for i := 0; i < g.pages; i++ {
		st := theme.StateNormal
		switch {
		case i == page:
			st = theme.StateActive
		case sn.hover == prefDot+itoa(i):
			st = theme.StateHover
		}
		s := m.tpart("dot", st)
		ctx.FillRoundRect(cx-d/2, y+i*(d+gap), d, d, d/2, s.Fill)
	}
}

// drawPinGhost рисует ячейку под курсором во время перетаскивания.
func (m *StartMenu) drawPinGhost(ctx widget.DrawContext, g gridGeo, gs gridSnap, prev image.Rectangle) {
	clipTo(ctx, g.panel.Inset(1), prev)
	p, ok := m.pinnedByID(gs.drag.id)
	if !ok {
		return
	}
	c := gs.drag.pos
	rect := image.Rect(c.X-g.cellW/2, c.Y-g.cellH/2, c.X-g.cellW/2+g.cellW, c.Y-g.cellH/2+g.cellH)
	m.drawPinCell(ctx, rect, p, theme.StateHover, "drag", false)
	ctx.SetClip(prev)
}

// ─── Вид-список ──────────────────────────────────────────────────────────────

// drawGridListView рисует «Все приложения», «Все рекомендации» или результаты
// поиска: заголовок с кнопкой «Назад» и общий список меню.
func (m *StartMenu) drawGridListView(ctx widget.DrawContext, g gridGeo, sn startSnap, prev image.Rectangle) {
	if g.mode == gridList || g.mode == gridMore {
		clip := clipTo(ctx, g.panel.Inset(1), prev)
		if g.listTitle.Overlaps(clip) {
			title := tr(StrStartAllApps)
			if g.mode == gridMore {
				title = tr(StrStartRecommended)
			}
			drawTextAt(ctx, g.listTitle, g.listTitle.Min.X, title, m.tpart("heading", theme.StateNormal))
			m.drawLink(ctx, g.backBtn, keyBack, tr(StrStartBack), false, sn, areaList)
		}
		ctx.SetClip(prev)
	}
	m.drawList(ctx, startGeo{inner: g.panel, list: g.list}, sn, prev)
}

// ─── Строка поиска ───────────────────────────────────────────────────────────

// drawSearchField рисует строку поиска сверху: лупа, запрос или подсказка и
// каретка. В фокусе (набор с клавиатуры идёт сюда) рамка акцентная.
func (m *StartMenu) drawSearchField(ctx widget.DrawContext, g gridGeo, sn startSnap, gs gridSnap, prev image.Rectangle) {
	clip := clipTo(ctx, g.search.Inset(-2), prev)
	defer ctx.SetClip(prev)
	if clip.Empty() {
		return
	}
	focused := sn.area == areaSearch
	st := theme.StateNormal
	switch {
	case focused:
		st = theme.StateActive
	case sn.hover == keySearch:
		st = theme.StateHover
	}
	s := m.fade.Style(m.tm, keySearch, g.search, st, func(st theme.State) *theme.Style { return m.tpart("search", st) })
	c := *s
	c.Corner = float64(m.gm(KeyStartW11SearchCorner, 16))
	PaintStyle(ctx, g.search, &c)
	if focused && c.Border.A > 0 {
		// Рамка в фокусе в два пикселя: второй контур внутри первого.
		ctx.DrawRoundBorder(g.search.Min.X+1, g.search.Min.Y+1, g.search.Dx()-2, g.search.Dy()-2, int(c.Corner)-1, c.Border)
	}

	pad := m.gm(KeyStartW11SearchPad, 12)
	icon := m.gm(KeyStartW11SearchIcon, 16)
	ir := image.Rect(0, 0, icon, icon).Add(image.Pt(g.search.Min.X+pad, g.search.Min.Y+(g.search.Dy()-icon)/2))
	drawGlyph(ctx, glyphSearch, ir, c.Text)
	field := image.Rect(ir.Max.X+8, g.search.Min.Y, g.search.Max.X-pad, g.search.Max.Y)
	ctx.SetClip(field.Intersect(prev))

	query := []rune(sn.query)
	if len(query) == 0 {
		hint := *m.tpart("search.hint", theme.StateNormal)
		hint.Font = c.Font
		drawTextAt(ctx, field, field.Min.X+2, tr(StrSearchPlaceholder), &hint) // каретка стоит левее подсказки
		if focused {
			drawCaret(ctx, field, field.Min.X, fontPt(&c), &c)
		}
		return
	}
	caret := clampInt(gs.caret, 0, len(query))
	caretX := measureAt(ctx, string(query[:caret]), &c)
	shift := 0
	if over := caretX - (field.Dx() - 2); over > 0 {
		shift = over
	}
	drawTextAt(ctx, field, field.Min.X-shift, string(query), &c)
	if focused {
		drawCaret(ctx, field, field.Min.X-shift+caretX, fontPt(&c), &c)
	}
}

// ─── Нижняя полоса ───────────────────────────────────────────────────────────

func (m *StartMenu) drawFooter(ctx widget.DrawContext, g gridGeo, sn startSnap, prev image.Rectangle) {
	clip := clipTo(ctx, g.footer, prev)
	defer ctx.SetClip(prev)
	if clip.Empty() {
		return
	}
	fs := m.tpart("footer", theme.StateNormal)
	// Подложка со скруглением нижних углов: плоская полоса торчала бы за
	// скруглённую панель. Верхние углы скруглённого прямоугольника выше клипа.
	corner := m.gm(KeyStartW11Corner, 8) - 1
	if fs.Fill.A > 0 {
		ext := image.Rect(g.footer.Min.X, g.footer.Min.Y-corner, g.footer.Max.X, g.footer.Max.Y)
		ctx.FillRoundRect(ext.Min.X, ext.Min.Y, ext.Dx(), ext.Dy(), corner, fs.Fill)
	}
	if fs.Border.A > 0 {
		ctx.FillRectAlpha(g.footer.Min.X, g.footer.Min.Y, g.footer.Dx(), 1, fs.Border)
	}

	// Пользователь.
	user := m.User()
	ukey := keyUser
	us := m.fade.Style(m.tm, ukey, g.user, stateFor(sn, ukey, areaFooter), func(st theme.State) *theme.Style { return m.tpart("footer.item", st) })
	PaintStyle(ctx, g.user, us)
	av := m.gm(KeyStartW11Avatar, 32)
	ar := image.Rect(g.user.Min.X+8, g.user.Min.Y+(g.user.Dy()-av)/2, g.user.Min.X+8+av, g.user.Min.Y+(g.user.Dy()-av)/2+av)
	m.drawAvatar(ctx, ar, user)
	if user.Name != "" {
		drawTextAt(ctx, g.user, ar.Max.X+12, user.Name, us)
	}
	if sn.kbd && sn.area == areaFooter && sn.sel[areaFooter] == ukey {
		PaintFocusRing(ctx, g.user, m.tm, us)
	}

	// Питание.
	pkey := keyPower
	ps := m.fade.Style(m.tm, pkey, g.power, stateFor(sn, pkey, areaFooter), func(st theme.State) *theme.Style { return m.tpart("footer.item", st) })
	PaintStyle(ctx, g.power, ps)
	pi := m.gm(KeyStartW11SearchIcon, 16)
	pr := image.Rect(0, 0, pi, pi).Add(image.Pt(g.power.Min.X+(g.power.Dx()-pi)/2, g.power.Min.Y+(g.power.Dy()-pi)/2))
	drawGlyph(ctx, GlyphPower, pr, ps.Text)
	if sn.kbd && sn.area == areaFooter && sn.sel[areaFooter] == pkey {
		PaintFocusRing(ctx, g.power, m.tm, ps)
	}
}

// drawAvatar рисует аватар кругом: картинку потребителя либо серый круг со
// значком пользователя.
func (m *StartMenu) drawAvatar(ctx widget.DrawContext, r image.Rectangle, u StartUser) {
	if u.Avatar != nil || u.AvatarAt != nil {
		prev := ctx.Clip()
		if rc, ok := ctx.(widget.RoundClipper); ok {
			rc.SetRoundClip(r, r.Dx()/2)
			drawAppIcon(ctx, u.Avatar, u.AvatarAt, r)
			rc.ClearRoundClip()
			ctx.SetClip(prev)
			return
		}
		drawAppIcon(ctx, u.Avatar, u.AvatarAt, r)
		return
	}
	s := m.tpart("avatar", theme.StateNormal)
	ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), r.Dx()/2, s.Fill)
	gl := r.Inset(r.Dx() / 5)
	drawGlyph(ctx, GlyphUser, gl, s.Text)
}

// ─── Мелочи ──────────────────────────────────────────────────────────────────

func clampInt(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func itoa(n int) string { return strconv.Itoa(n) }
