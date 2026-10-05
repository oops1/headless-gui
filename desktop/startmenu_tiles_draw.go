// startmenu_tiles_draw.go — отрисовка меню «Пуск» с плитками.
//
// Порядок слоёв: плитки, список приложений, боковая панель (она лежит поверх
// списка, когда развёрнута), затем рамки клавиатурного фокуса и плитка, которую
// несут мышью. Каждая область клипуется своим прямоугольником, поэтому
// частичная перерисовка (шаг анимации ширины панели, наведение на плитку)
// рисует только то, что попало в заявленную область.
//
// Draw ничего не меняет: всё, что он читает, — снимки состояния и неизменяемые
// после публикации модели (listRows, layoutTiles).
package desktop

import (
	"image"
	"image/color"
	"strings"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// startSnap — снимок состояния ввода для одного кадра.
type startSnap struct {
	hover, press string
	sel          map[startArea]string
	kbd          bool
	area         startArea
	listScroll   int
	tileScroll   int
	sidebar      []StartSidebarItem
	groups       []TileGroup
	query        string
	drag         tileDrag
	hasBox       bool
}

func (m *StartMenu) snapshot() startSnap {
	v := m.v
	v.mu.Lock()
	defer v.mu.Unlock()
	sel := make(map[startArea]string, len(v.sel))
	for k, x := range v.sel {
		sel[k] = x
	}
	return startSnap{
		hover: v.hover, press: v.press, sel: sel, kbd: v.kbd, area: v.area,
		listScroll: v.listScroll, tileScroll: v.tileScroll,
		sidebar: v.sidebar, groups: v.groups, query: v.query, drag: v.drag,
		hasBox: v.box != nil,
	}
}

// drawTiled рисует меню с плитками в прямоугольнике r (внутренность панели).
func (m *StartMenu) drawTiled(ctx widget.DrawContext, r image.Rectangle) {
	g := m.startGeometry(r)
	if g.list.Empty() {
		return
	}
	sn := m.snapshot()
	prev := ctx.Clip()
	defer ctx.SetClip(prev)

	if g.cols > 0 {
		m.drawTilesArea(ctx, g, sn, prev)
	}
	m.drawList(ctx, g, sn, prev)
	m.drawSidebar(ctx, g, sn, prev)
	if sn.drag.active {
		m.drawDragGhost(ctx, g, sn, prev)
	}
}

// clipTo ограничивает рисование пересечением rect с внешним клипом prev.
func clipTo(ctx widget.DrawContext, rect, prev image.Rectangle) image.Rectangle {
	c := rect.Intersect(prev)
	ctx.SetClip(c)
	return c
}

// ─── Плитки ──────────────────────────────────────────────────────────────────

func (m *StartMenu) drawTilesArea(ctx widget.DrawContext, g startGeo, sn startSnap, prev image.Rectangle) {
	// Клип шире полей: рамка наведения у крайних плиток не должна обрезаться.
	clip := clipTo(ctx, g.tiles, prev)
	if clip.Empty() {
		return
	}
	groups := sn.groups
	var l *tileLayout
	if sn.drag.active {
		groups = dragPreview(groups, sn.drag)
		l = computeTileLayout(groups, m.tileKeyFor(g))
	} else {
		l = m.layoutTiles(g)
	}
	scroll := clampScroll(sn.tileScroll, l.height, tilesViewHeight(g))
	origin := image.Pt(g.tinner.Min.X, g.tinner.Min.Y-scroll)

	hs := m.withFont(m.tpart("tile.group", theme.StateNormal), "title")
	for _, gg := range l.groups {
		hr := gg.header.Add(origin)
		if !hr.Overlaps(clip) || gg.title == "" {
			continue
		}
		drawTextAt(ctx, hr, hr.Min.X, Elide(ctx, gg.title, hs, hr.Dx()), hs)
	}

	for _, tg := range l.tiles {
		rect := tg.rect.Add(origin)
		if !rect.Overlaps(clip) {
			continue
		}
		if tg.group >= len(groups) || tg.index >= len(groups[tg.group].Tiles) {
			continue
		}
		t := groups[tg.group].Tiles[tg.index]
		if t.ID != tg.id {
			continue
		}
		if sn.drag.active && t.ID == sn.drag.id {
			// Место, куда ляжет плитка: пунктирная рамка вместо неё самой.
			s := m.tpart("tile", theme.StateHover)
			strokeInset(ctx, rect, 1, s.Border)
			continue
		}
		key := prefTile + string(t.ID)
		st := theme.StateNormal
		switch {
		case sn.press == key:
			st = theme.StatePressed
		case sn.hover == key:
			st = theme.StateHover
		case sn.kbd && sn.area == areaTiles && sn.sel[areaTiles] == key:
			st = theme.StateFocused
		}
		m.drawTile(ctx, rect, t, st, key)
	}

	if bar := m.thinBarRect(g.tiles, l.height, tilesViewHeight(g), scroll, g.tinner.Min.Y); !bar.Empty() {
		m.drawThumb(ctx, bar, m.v.tileBar.Alpha())
	}
}

// tileKeyFor — ключ раскладки для текущих метрик (для раскладки без кэша).
func (m *StartMenu) tileKeyFor(g startGeo) tileKey {
	return tileKey{
		cols: g.cols, unit: m.metricInt(KeyTileUnit), gap: m.metricInt(KeyTileGap),
		header: m.metricInt(KeyTileGroupHeader), headerGap: m.metricInt(KeyTileGroupHeaderGap),
		groupGap: m.metricInt(KeyTileGroupGap),
	}
}

// drawTile рисует одну плитку: подложка по стилю, значок, название, подзаголовок
// и метка.
func (m *StartMenu) drawTile(ctx widget.DrawContext, rect image.Rectangle, t Tile, st theme.State, key string) {
	s := m.fade.Style(m.tm, key, rect, st, func(st theme.State) *theme.Style { return m.tpart("tile", st) })
	ink := s.Text
	if bg := t.Content.Background; bg.A > 0 {
		c := *s
		c.Fill = bg
		ink = contrastInk(bg)
		c.Text = ink
		s = &c
	}
	PaintStyle(ctx, rect, s)
	if bw := int(s.BorderWidth); bw > 1 && s.Border.A > 0 {
		strokeInset(ctx, rect, bw, s.Border)
	}

	unit := m.metricInt(KeyTileUnit)
	pad := 2 * m.metricInt(KeyTileGap)

	// Значок: четверть-треть плитки по размеру единицы сетки.
	iconSide := unit * 2 / 3
	switch t.Size {
	case TileSmall:
		iconSide = unit / 2
	case TileLarge:
		iconSide = unit * 4 / 3
	}
	hasText := t.Size != TileSmall && (t.Content.Title != "" || t.Content.Subtitle != "")
	textH := 0
	body := s
	capStyle := m.withFont(body, "caption")
	if hasText {
		textH = int(float64(fontPt(body)) * 1.4)
		if t.Content.Subtitle != "" {
			textH += int(float64(fontPt(capStyle)) * 1.4)
		}
		textH += pad
	}
	area := image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Max.Y-textH)
	if t.Content.Icon != nil || t.Content.IconAt != nil {
		ir := image.Rect(0, 0, iconSide, iconSide).Add(image.Pt(
			area.Min.X+(area.Dx()-iconSide)/2, area.Min.Y+(area.Dy()-iconSide)/2))
		drawAppIcon(ctx, t.Content.Icon, t.Content.IconAt, ir)
	}

	prev := ctx.Clip()
	ctx.SetClip(rect.Inset(1).Intersect(prev))
	defer ctx.SetClip(prev)
	tx := rect.Min.X + pad
	tw := rect.Dx() - 2*pad
	if hasText && tw > 0 {
		lineH := int(float64(fontPt(body)) * 1.4)
		ty := rect.Max.Y - pad - lineH
		title := Elide(ctx, t.Content.Title, body, tw)
		drawTextTop(ctx, tx, ty, title, body)
		if t.Content.Subtitle != "" {
			subStyle := *capStyle
			subStyle.Text = withAlpha(ink, 190)
			sh := int(float64(fontPt(capStyle)) * 1.4)
			sub := Elide(ctx, t.Content.Subtitle, &subStyle, tw)
			drawTextTop(ctx, tx, ty-sh, sub, &subStyle)
		}
	}
	if b := t.Content.Badge; b != "" && t.Size != TileSmall {
		bs := m.withFont(body, "caption")
		w := MeasureText(ctx, b, bs)
		drawTextTop(ctx, rect.Max.X-pad/2-w, rect.Min.Y+pad/2, b, bs)
	}
}

// fontPt — кегль стиля в пунктах.
func fontPt(s *theme.Style) float64 {
	if s != nil && s.Font.Size > 0 {
		return s.Font.Size
	}
	return widget.DefaultFontSizePt
}

// drawTextTop рисует строку, верх которой на y.
func drawTextTop(ctx widget.DrawContext, x, y int, text string, s *theme.Style) {
	if text == "" || s == nil {
		return
	}
	drawText(ctx, text, x, y, fontPt(s), s)
}

// withAlpha возвращает цвет c с прозрачностью a (0..255). Цвета темы хранятся
// с предумноженной альфой, поэтому каналы масштабируются вместе с ней.
func withAlpha(c color.RGBA, a uint8) color.RGBA {
	k := func(v uint8) uint8 { return uint8(int(v) * int(a) / 255) }
	return color.RGBA{R: k(c.R), G: k(c.G), B: k(c.B), A: uint8(int(c.A) * int(a) / 255)}
}

// strokeInset рисует рамку толщиной w внутрь r.
func strokeInset(ctx widget.DrawContext, r image.Rectangle, w int, col color.RGBA) {
	if col.A == 0 {
		return
	}
	for i := 0; i < w; i++ {
		rr := r.Inset(i)
		if rr.Empty() {
			return
		}
		if col.A < 255 {
			ctx.FillRectAlpha(rr.Min.X, rr.Min.Y, rr.Dx(), 1, col)
			ctx.FillRectAlpha(rr.Min.X, rr.Max.Y-1, rr.Dx(), 1, col)
			ctx.FillRectAlpha(rr.Min.X, rr.Min.Y+1, 1, rr.Dy()-2, col)
			ctx.FillRectAlpha(rr.Max.X-1, rr.Min.Y+1, 1, rr.Dy()-2, col)
			continue
		}
		ctx.DrawBorder(rr.Min.X, rr.Min.Y, rr.Dx(), rr.Dy(), col)
	}
}

// drawDragGhost рисует плитку под курсором во время перетаскивания.
func (m *StartMenu) drawDragGhost(ctx widget.DrawContext, g startGeo, sn startSnap, prev image.Rectangle) {
	clipTo(ctx, g.inner, prev)
	var t Tile
	found := false
	for _, grp := range sn.groups {
		for _, x := range grp.Tiles {
			if x.ID == sn.drag.id {
				t, found = x, true
			}
		}
	}
	if !found {
		return
	}
	unit, gap := m.metricInt(KeyTileUnit), m.metricInt(KeyTileGap)
	cols, rows := t.Size.Units()
	w, h := cols*unit+(cols-1)*gap, rows*unit+(rows-1)*gap
	c := sn.drag.pos
	rect := image.Rect(c.X-w/2, c.Y-h/2, c.X-w/2+w, c.Y-h/2+h)
	m.drawTile(ctx, rect, t, theme.StateHover, "drag")
}

// ─── Список ──────────────────────────────────────────────────────────────────

func (m *StartMenu) drawList(ctx widget.DrawContext, g startGeo, sn startSnap, prev image.Rectangle) {
	clip := clipTo(ctx, g.list, prev)
	if clip.Empty() {
		return
	}
	if sn.query != "" && !sn.hasBox {
		m.drawQueryBar(ctx, g, sn.query)
	}
	vp := m.listViewport(g)
	clip = clipTo(ctx, vp, prev)
	rows, contentH := m.listRows()
	scroll := clampScroll(sn.listScroll, contentH, vp.Dy())

	rowPad := m.metricInt(KeyStartRowPad)
	gap := m.metricInt(KeyStartRowIconGap)
	iconSide := m.metricInt(KeyStartMenuIconSize)
	letterS := m.tpart("letter", theme.StateNormal)

	for i := firstRowAt(rows, scroll); i < len(rows); i++ {
		row := rows[i]
		rect := image.Rect(vp.Min.X, vp.Min.Y+row.y-scroll, vp.Max.X, vp.Min.Y+row.y-scroll+row.h)
		if rect.Min.Y >= vp.Max.Y {
			break
		}
		if !rect.Overlaps(clip) {
			continue
		}
		iconX := rect.Min.X + rowPad
		switch row.kind {
		case rowLetter:
			w := MeasureText(ctx, row.label, letterS)
			drawTextAt(ctx, rect, iconX+iconSide/2-w/2, row.label, letterS)
		case rowHeader:
			label := row.label
			if row.labelKey != "" {
				label = tr(row.labelKey)
			}
			drawTextAt(ctx, rect, iconX, Elide(ctx, label, letterS, rect.Dx()-rowPad), letterS)
		case rowEmpty:
			label := tr(row.labelKey)
			w := MeasureText(ctx, label, letterS)
			drawTextAt(ctx, rect, rect.Min.X+(rect.Dx()-w)/2, label, letterS)
		default:
			m.drawRow(ctx, rect, row, sn, rowPad, gap, iconSide)
		}
	}

	if bar := m.thinBarRect(vp, contentH, vp.Dy(), scroll, vp.Min.Y); !bar.Empty() {
		m.drawThumb(ctx, bar, m.v.listBar.Alpha())
	}
	ctx.SetClip(prev)
}

// drawQueryBar рисует строку с запросом над результатами, когда строки поиска
// на панели, куда его вводят, нет: набор букв в открытом меню — единственный
// источник запроса, и его нужно видеть.
func (m *StartMenu) drawQueryBar(ctx widget.DrawContext, g startGeo, query string) {
	h := m.metricInt(KeyStartLetterHeight)
	rect := image.Rect(g.list.Min.X, g.list.Min.Y, g.list.Max.X, g.list.Min.Y+h)
	pad := m.metricInt(KeyStartRowPad)
	s := m.tpart("letter", theme.StateNormal)
	side := m.metricInt(KeyStartMenuIconSize) * 2 / 3
	drawGlyph(ctx, glyphSearch, image.Rect(0, 0, side, side).Add(image.Pt(rect.Min.X+pad, rect.Min.Y+(h-side)/2)), s.Text)
	x := rect.Min.X + pad + side + m.metricInt(KeyStartRowIconGap)
	drawTextAt(ctx, rect, x, Elide(ctx, query, s, rect.Max.X-x-pad), s)
}

func (m *StartMenu) drawRow(ctx widget.DrawContext, rect image.Rectangle, row listRow, sn startSnap, pad, gap, iconSide int) {
	key := prefRow + row.key
	st := theme.StateNormal
	selected := sn.kbd && sn.area == areaList && sn.sel[areaList] == key
	switch {
	case sn.press == key:
		st = theme.StatePressed
	case sn.hover == key:
		st = theme.StateHover
	case selected:
		st = theme.StateActive
	}
	s := m.fade.Style(m.tm, key, rect, st, func(st theme.State) *theme.Style { return m.tpart("row", st) })
	PaintStyle(ctx, rect, s)

	iconX := rect.Min.X + pad
	if row.kind == rowChild {
		iconX += iconSide / 2
	}
	iconY := rect.Min.Y + (rect.Dy()-iconSide)/2
	if row.icon != nil || row.iconAt != nil {
		drawAppIcon(ctx, row.icon, row.iconAt, image.Rect(iconX, iconY, iconX+iconSide, iconY+iconSide))
	}
	textX := iconX + iconSide + gap
	right := rect.Max.X - pad/2
	if row.kind == rowFolder {
		// Стрелка раскрытия справа; текст до неё.
		cs := iconSide * 2 / 3
		g := glyphChevronDown
		if row.expanded {
			g = glyphChevronUp
		}
		cr := image.Rect(0, 0, cs, cs).Add(image.Pt(rect.Max.X-pad/2-cs, rect.Min.Y+(rect.Dy()-cs)/2))
		drawGlyph(ctx, g, cr, s.Text)
		right = cr.Min.X - gap
	}
	avail := right - textX
	if avail <= 0 {
		return
	}
	if row.sub == "" {
		drawTextAt(ctx, rect, textX, Elide(ctx, row.label, s, avail), s)
	} else {
		// Две строки: название сверху, вторая приглушённо и мельче под ним.
		subS := m.withFont(m.tpart("row.sub", theme.StateNormal), "caption")
		upper := image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+rect.Dy()/2+int(fontPt(s)*0.6))
		lower := image.Rect(rect.Min.X, rect.Min.Y+rect.Dy()/2-int(fontPt(subS)*0.2), rect.Max.X, rect.Max.Y)
		drawTextAt(ctx, upper, textX, Elide(ctx, row.label, s, avail), s)
		drawTextAt(ctx, lower, textX, Elide(ctx, row.sub, subS, avail), subS)
	}
	if selected {
		PaintFocusRing(ctx, rect, m.tm, s)
	}
}

// thinBarRect возвращает прямоугольник ползунка тонкой полосы у правого края
// области view: content — высота содержимого, viewH — высота окна, scroll —
// смещение, top — верх окна. Пустой, если прокручивать нечего.
func (m *StartMenu) thinBarRect(view image.Rectangle, content, viewH, scroll, top int) image.Rectangle {
	if content <= viewH || viewH <= 0 {
		return image.Rectangle{}
	}
	w := m.metricInt(KeyScrollThinWidth)
	if w <= 0 {
		w = 4
	}
	thumb := viewH * viewH / content
	if min := w * 4; thumb < min {
		thumb = min
	}
	if thumb > viewH {
		thumb = viewH
	}
	span := content - viewH
	y := top
	if span > 0 {
		y += (viewH - thumb) * scroll / span
	}
	return image.Rect(view.Max.X-w-1, y, view.Max.X-1, y+thumb)
}

// drawThumb рисует ползунок с прозрачностью alpha (0..1).
func (m *StartMenu) drawThumb(ctx widget.DrawContext, r image.Rectangle, alpha float64) {
	if alpha <= 0.01 || r.Empty() {
		return
	}
	s := m.tpart("scrollbar", theme.StateNormal)
	a := uint8(alpha * 255)
	ctx.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), withAlpha(s.Fill, a))
}

// ─── Боковая панель ──────────────────────────────────────────────────────────

// sidebarRects возвращает прямоугольники гамбургера и n пунктов: гамбургер
// вверху, пункты стопкой у нижнего края (последний — самый нижний).
func (m *StartMenu) sidebarRects(g startGeo, n int) (burger image.Rectangle, items []image.Rectangle) {
	h := g.collapsed
	sb := g.sidebar
	burger = image.Rect(sb.Min.X, sb.Min.Y, sb.Max.X, sb.Min.Y+h)
	items = make([]image.Rectangle, n)
	for i := 0; i < n; i++ {
		y := sb.Max.Y - (n-i)*h
		items[i] = image.Rect(sb.Min.X, y, sb.Max.X, y+h)
	}
	return burger, items
}

func (m *StartMenu) drawSidebar(ctx widget.DrawContext, g startGeo, sn startSnap, prev image.Rectangle) {
	clip := clipTo(ctx, g.sidebar, prev)
	if clip.Empty() {
		return
	}
	PaintStyle(ctx, g.sidebar, m.tpart("sidebar", theme.StateNormal))

	expanded := g.sideW > g.collapsed
	iconSide := m.metricInt(KeyStartSidebarIconSize)
	burger, rects := m.sidebarRects(g, len(sn.sidebar))

	one := func(rect image.Rectangle, key, label string, glyph StartGlyph, icon image.Image, iconAt func(int) image.Image, titleFont bool) {
		if !rect.Overlaps(clip) {
			return
		}
		st := theme.StateNormal
		selected := sn.kbd && sn.area == areaSidebar && sn.sel[areaSidebar] == key
		switch {
		case sn.press == key:
			st = theme.StatePressed
		case sn.hover == key:
			st = theme.StateHover
		case selected:
			st = theme.StateActive
		}
		s := m.fade.Style(m.tm, key, rect, st, func(st theme.State) *theme.Style { return m.tpart("sidebar.item", st) })
		PaintStyle(ctx, rect, s)
		ir := image.Rect(0, 0, iconSide, iconSide).Add(image.Pt(
			rect.Min.X+(g.collapsed-iconSide)/2, rect.Min.Y+(rect.Dy()-iconSide)/2))
		switch {
		case icon != nil || iconAt != nil:
			drawAppIcon(ctx, icon, iconAt, ir)
		case glyph != GlyphNone:
			drawGlyph(ctx, glyph, ir, s.Text)
		}
		if expanded && label != "" {
			ts := s
			if titleFont {
				ts = m.withFont(s, "title")
			}
			x := rect.Min.X + g.collapsed
			drawTextAt(ctx, rect, x, Elide(ctx, label, ts, rect.Max.X-x-iconSide/2), ts)
		}
		if selected {
			PaintFocusRing(ctx, rect, m.tm, s)
		}
	}

	// Заголовок развёрнутой панели — «ПУСК» полужирным рядом с гамбургером.
	title := strings.ToUpper(tr(StrStart))
	one(burger, keySideMenu, title, glyphMenu, nil, nil, true)
	for i, it := range sn.sidebar {
		one(rects[i], prefSide+it.ID, it.Title, it.Glyph, it.Icon, it.IconAt, false)
	}
	ctx.SetClip(prev)
}
