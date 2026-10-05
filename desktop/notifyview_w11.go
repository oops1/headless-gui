// notifyview_w11.go — вид центра уведомлений Windows 11: раскладка и рисование.
//
// Один вид на три раскладки: плоский центр рисует сам NotificationCenter,
// Windows 10 и Windows 11 — richView. Какую раскладку брать, решает презентер,
// который назначил профиль (theme.NotificationCenterWin11Presenter); имени темы
// здесь нет. Всё, что общее — зоны, ввод, прокрутка, анимация раскрытия, тост,
// действия карточки, — живёт в notifyview*.go и работает без изменений; здесь
// только то, что у Windows 11 устроено иначе:
//
//   - заголовок «Уведомления» с колокольчиком «Не беспокоить» и кнопкой «Очистить
//     все» (неактивной, пока уведомлений нет), вместо ссылок и плиток;
//   - карточка со скруглением из стиля: строка приложения (значок, название,
//     время), заголовок, текст, раскрытие, крестик, действия;
//   - группа — только когда в ней несколько уведомлений: заголовок со
//     значком, названием и счётчиком, карточки внутри без строки приложения.
//
// Ни цветов, ни размеров здесь нет: цвета приходят из частей стиля "notificationcenter"
// (card, group, pill, headbtn, action, field, glyph, dim…), размеры — из метрик
// профиля (theme/profiles_win11_notify.go) и раскладки.
package desktop

import (
	"image"
	"image/color"
	"strconv"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Пропорции раскладки Windows 11, которые не размеры темы, а устройство фигур.
const (
	// ncW11BtnPad — поле по бокам надписи кнопки «Очистить все».
	ncW11BtnPad = 12
	// ncW11HeadGap — зазор между колокольчиком и «Очистить все».
	ncW11HeadGap = 4
	// ncW11HeadIndent — отступ заголовка «Уведомления» от левого края поля.
	ncW11HeadIndent = 4
	// ncW11GroupPad — поле слева в заголовке группы.
	ncW11GroupPad = 8
	// ncW11PillH и ncW11PillPad — высота счётчика группы и поле по бокам числа.
	ncW11PillH   = 18
	ncW11PillPad = 12
	// ncW11TextGap — зазор между названием приложения, временем и столбцом
	// кнопок карточки.
	ncW11TextGap = 8
)

// w11 сообщает, что активная тема назначила центру презентер Windows 11.
// Вид спрашивает не имя темы, а имя презентера своего компонента.
func (v *richView) w11() bool {
	if v.tm == nil {
		return false
	}
	t := v.tm.Active()
	return t != nil && t.PresenterName(ComponentNotificationCenter) == theme.NotificationCenterWin11Presenter
}

// ─── Раскладка ───────────────────────────────────────────────────────────────

// layoutW11 раскладывает панель Windows 11: заголовок, список групп с прокруткой.
func (v *richView) layoutW11(l *richLayout) {
	m, f, panel := l.m, l.fonts, l.panel
	l.w11 = true

	var notes []Notification
	if v.src.notes != nil {
		notes = v.src.notes()
	}
	// Анимации раскрытия запускаются уже без замка (см. slideStep).
	defer v.flushSlides()
	v.mu.Lock()
	defer v.mu.Unlock()

	pad := m.pad
	x0 := panel.Min.X + pad
	w := panel.Dx() - 2*pad
	if w < 1 {
		w = 1
	}
	l.content = image.Rect(x0, panel.Min.Y, x0+w, panel.Max.Y)

	// Заголовок: слева название, справа колокольчик и «Очистить все».
	l.header = image.Rect(panel.Min.X, panel.Min.Y, panel.Max.X, panel.Min.Y+m.headerH)
	hb := m.headBtn
	by := l.header.Min.Y + (m.headerH-hb)/2
	clearLabel := f.body.elide(tr(StrNotifClearAll), w/2)
	cw := f.body.width(clearLabel) + 2*ncW11BtnPad
	l.clear = image.Rect(x0+w-cw, by, x0+w, by+hb)
	l.dnd = image.Rect(l.clear.Min.X-ncW11HeadGap-hb, by, l.clear.Min.X-ncW11HeadGap, by+hb)
	l.heading = image.Rect(x0+ncW11HeadIndent, l.header.Min.Y, l.dnd.Min.X-ncW11TextGap, l.header.Max.Y)
	l.clearOff = len(notes) == 0

	// Окно списка — под заголовком до нижнего поля.
	top := l.header.Max.Y
	if top > panel.Max.Y {
		top = panel.Max.Y
	}
	bottom := panel.Max.Y - pad
	if bottom < top {
		bottom = top
	}
	l.viewport = image.Rectangle{Min: image.Pt(panel.Min.X, top), Max: image.Pt(panel.Max.X, bottom)}

	groups, y := v.flowGroups(m, f, notes, x0, w)
	l.groups = groups
	l.empty = len(groups) == 0
	l.contentH = y - m.cardGap // без нижнего зазора: поле даёт панель
	if l.empty || l.contentH < 0 {
		l.contentH = 0
	}
	l.maxScrl = l.contentH - l.viewport.Dy()
	if l.maxScrl < 0 {
		l.maxScrl = 0
	}
	if v.scroll > l.maxScrl {
		v.scroll = l.maxScrl
	}
	if v.scroll < 0 {
		v.scroll = 0
	}
	l.scroll = v.scroll

	v.shiftGroups(l)
	v.buildZones(l)
	l.lastKeep(v)
}

// sizeW11 возвращает размер панели Windows 11 по содержимому: заголовок, список
// без прокрутки и нижнее поле, но не выше maxH и не ниже заголовка с пустым
// списком. Пустая панель занимает listMin под надпись «Новых уведомлений нет».
func (v *richView) sizeW11(width, maxH int) image.Point {
	m := ncReadMetrics(v.tm)
	f := v.fonts()
	var notes []Notification
	if v.src.notes != nil {
		notes = v.src.notes()
	}
	defer v.flushSlides()
	v.mu.Lock()
	_, y := v.flowGroups(m, f, notes, 0, width-2*m.pad)
	v.mu.Unlock()
	list := y - m.cardGap
	if len(notes) == 0 || list < m.listMin {
		list = m.listMin
	}
	h := m.headerH + list + m.pad
	if maxH > 0 && h > maxH {
		h = maxH
	}
	if h < 1 {
		h = 1
	}
	return image.Pt(width, h)
}

// layoutCardAsW11 раскладывает карточку Windows 11 в прямоугольнике с левым
// верхним углом (x, y) и шириной w. showApp — карточка одиночная и несёт строку
// приложения; в группе её роль играет заголовок группы, а время и крестик
// переезжают в строку заголовка карточки. open — раскрыта ли карточка.
//
// Справа у карточки столбец из двух гнёзд стороной slot: крестик закрытия (в
// первой строке, на месте времени, пока над карточкой мышь) и шеврон раскрытия
// (во второй). Текст заголовка и тела обходит столбец, поэтому раскрытие не
// перекладывает строки.
func (v *richView) layoutCardAsW11(m ncMetrics, f ncFonts, n Notification, x, y, w int, showApp, open bool) richCard {
	c := richCard{n: n, open: open}
	p := m.cardPad
	x0, x1 := x+p, x+w-p
	slot := m.slot

	appName := n.AppName
	if appName == "" {
		appName = string(n.AppID)
	}
	c.showApp = showApp && appName != ""
	c.hasIcon = c.showApp && hasNoteIcon(n)

	c.timeText = v.timeLabel(n.At())
	timeW := f.caption.width(c.timeText)
	capH := f.caption.lineH()
	textRight := x1 - slot - 4
	if textRight < x0+8 {
		textRight = x0 + 8
	}

	cy := y + p
	var rowTop, rowH int
	toggleY := 0
	if c.showApp {
		// Строка приложения: значок, название, время; под ней заголовок.
		rowH = maxInt(slot, maxInt(capH, m.cardIcon))
		rowTop = cy
		ix := x0
		if c.hasIcon {
			c.icon = image.Rect(x0, rowTop+(rowH-m.cardIcon)/2, x0+m.cardIcon, rowTop+(rowH-m.cardIcon)/2+m.cardIcon)
			ix = x0 + m.cardIcon + m.cardIconGp
		}
		nameRight := x1 - maxInt(timeW, slot) - ncW11TextGap
		c.appText = f.caption.elide(appName, nameRight-ix)
		c.appRect = image.Rect(ix, rowTop, nameRight, rowTop+rowH)
		cy += rowH + 2
		c.titleText = f.title.elide(n.Title, textRight-x0)
		c.title = image.Rect(x0, cy, textRight, cy+f.title.lineH())
		toggleY = cy + (f.title.lineH()-slot)/2
		cy += f.title.lineH()
	} else {
		// Заголовок и время делят одну строку.
		rowH = maxInt(slot, f.title.lineH())
		rowTop = cy
		titleRight := x1 - maxInt(timeW, slot) - ncW11TextGap
		c.titleText = f.title.elide(n.Title, titleRight-x0)
		c.title = image.Rect(x0, rowTop+(rowH-f.title.lineH())/2, titleRight, rowTop+(rowH-f.title.lineH())/2+f.title.lineH())
		cy += rowH
		toggleY = cy
	}
	c.timeRect = image.Rect(x1-timeW, rowTop+(rowH-capH)/2, x1, rowTop+(rowH-capH)/2+capH)
	c.closeRect = image.Rect(x1-slot, rowTop+(rowH-slot)/2, x1, rowTop+(rowH-slot)/2+slot)

	// Текст.
	if n.Body != "" {
		cy += 2
		bodyW := textRight - x0
		short, shortCut := f.body.wrap(n.Body, bodyW, m.lines)
		full, _ := f.body.wrap(n.Body, bodyW, m.linesMax)
		c.bodyLines = short
		if c.open {
			c.bodyLines = full
		}
		c.bodyX, c.bodyY = x0, cy
		if !c.showApp {
			toggleY = cy + (f.body.lineH()-slot)/2
		}
		cy += len(c.bodyLines) * f.body.lineH()
		c.canToggle = shortCut || len(full) > len(short)
	}
	if len(n.Actions) > 0 {
		c.canToggle = true
	}
	if v.mode == ncModeToast {
		c.canToggle = false // тост показывает всё сразу и не сворачивается
	}
	c.toggleRect = image.Rect(x1-slot, toggleY, x1, toggleY+slot)

	// Действия.
	if c.open && len(n.Actions) > 0 {
		cy += m.actRow
		c.acts, cy = v.layoutActions(m, f, n, x0, cy, w-2*p)
	}
	cy += p
	c.rect = image.Rect(x, y, x+w, cy)
	return c
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ─── Рисование ───────────────────────────────────────────────────────────────

// drawW11 рисует центр Windows 11: заголовок с кнопками и список групп.
func (v *richView) drawW11(ctx widget.DrawContext, l *richLayout, u ncUI, prev image.Rectangle) {
	f := l.fonts
	hs := v.part(ncPartHeader, theme.StateNormal)
	if !l.heading.Empty() {
		head := f.heading.elide(tr(StrNotifTitle), l.heading.Dx())
		f.heading.draw(ctx, head, l.heading.Min.X, l.heading.Min.Y+(l.heading.Dy()-f.heading.lineH())/2, hs.Text)
	}
	v.drawBell(ctx, l, u)
	v.drawClearAll(ctx, l, u)

	// Список: всё, что не помещается, обрезается окном просмотра.
	ctx.SetClip(l.viewport.Intersect(prev))
	if l.empty {
		st := v.part(ncPartDim, theme.StateNormal)
		DrawTextCentered(ctx, l.viewport, v.emptyLabel(), st)
	}
	for _, g := range l.groups {
		v.drawGroupW11(ctx, l, u, g)
		vis := l.viewport.Intersect(prev)
		if g.anim {
			vis = vis.Intersect(g.clip)
			ctx.SetClip(vis)
		}
		for _, c := range g.cards {
			v.drawCard(ctx, l, u, c, vis)
		}
		if g.anim {
			ctx.SetClip(l.viewport.Intersect(prev))
		}
	}
	v.drawScrollbar(ctx, l, u)
	ctx.SetClip(prev)

	v.drawDropdown(ctx, l, u)
	v.drawFocusRing(ctx, l, u, prev)
}

// drawBell рисует колокольчик «Не беспокоить»: контур, а при включённом режиме —
// перечёркнутый и цветом акцента (часть headbtn.on).
func (v *richView) drawBell(ctx widget.DrawContext, l *richLayout, u ncUI) {
	if l.dnd.Empty() {
		return
	}
	on := v.src.dnd != nil && v.src.dnd()
	part := ncPartHeadBtn
	if on {
		part = ncPartHeadBtn + ".on"
	}
	k := zoneKey{kind: zoneDND}
	st := v.style(k, l.dnd, part, u.state(k, false))
	PaintStyle(ctx, l.dnd, st)
	side := l.dnd.Dy() * 5 / 8
	drawGlyphBell(ctx, l.dnd, st.Text, side, on)
}

// drawClearAll рисует «Очистить все»; без уведомлений она приглушена.
func (v *richView) drawClearAll(ctx widget.DrawContext, l *richLayout, u ncUI) {
	if l.clear.Empty() {
		return
	}
	k := zoneKey{kind: zoneClear}
	st := v.style(k, l.clear, ncPartHeadBtn, u.state(k, l.clearOff))
	PaintStyle(ctx, l.clear, st)
	label := l.fonts.body.elide(tr(StrNotifClearAll), l.clear.Dx())
	tw := l.fonts.body.width(label)
	l.fonts.body.draw(ctx, label, l.clear.Min.X+(l.clear.Dx()-tw)/2,
		l.clear.Min.Y+(l.clear.Dy()-l.fonts.body.lineH())/2, st.Text)
}

// drawGroupW11 рисует заголовок группы: значок, название, счётчик, а справа —
// шеврон и крестик (крестик — под курсором или в фокусе).
func (v *richView) drawGroupW11(ctx widget.DrawContext, l *richLayout, u ncUI, g richGroup) {
	if g.rect.Empty() || g.rect.Max.Y < l.viewport.Min.Y || g.rect.Min.Y > l.viewport.Max.Y {
		return
	}
	k := zoneKey{kind: zoneGroup, app: g.app}
	st := v.style(k, g.rect, ncPartGroup, u.state(k, false))
	PaintStyle(ctx, g.rect, st)

	f, m := l.fonts, l.m
	x := g.rect.Min.X + ncW11GroupPad
	if hasNoteIcon(g.note) {
		side := m.groupIcon
		ir := image.Rect(x, g.rect.Min.Y+(g.rect.Dy()-side)/2, x+side, g.rect.Min.Y+(g.rect.Dy()-side)/2+side)
		drawAppIcon(ctx, g.note.Icon, g.note.IconAt, ir)
		x += side + ncW11TextGap
	}
	count := strconv.Itoa(g.count)
	pillW := f.caption.width(count) + ncW11PillPad
	name := f.title.elide(g.name, g.rect.Max.X-2*m.slot-pillW-ncW11TextGap-x)
	f.title.draw(ctx, name, x, g.rect.Min.Y+(g.rect.Dy()-f.title.lineH())/2, st.Text)
	x += f.title.width(name) + ncW11TextGap

	pr := image.Rect(x, g.rect.Min.Y+(g.rect.Dy()-ncW11PillH)/2, x+pillW, g.rect.Min.Y+(g.rect.Dy()-ncW11PillH)/2+ncW11PillH)
	ps := v.part(ncPartPill, theme.StateNormal)
	PaintStyle(ctx, pr, ps)
	f.caption.draw(ctx, count, pr.Min.X+(pr.Dx()-f.caption.width(count))/2, pr.Min.Y+(pr.Dy()-f.caption.lineH())/2, ps.Text)

	gside := m.slot / ncGlyphDiv
	gk := zoneKey{kind: zoneGroupToggle, app: g.app}
	gs := v.style(gk, g.chevronRect, ncPartGlyph, u.state(gk, false))
	drawGlyphChevron(ctx, g.chevronRect, !g.collapsed, gs.Text, gside)
	if u.hoverApp == g.app || (u.focus.kind == zoneGroup && u.focus.app == g.app) {
		ck := zoneKey{kind: zoneGroupClose, app: g.app}
		cs := v.style(ck, g.closeRect, ncPartGlyph, u.state(ck, false))
		PaintStyle(ctx, g.closeRect, cs)
		drawGlyphCross(ctx, g.closeRect, cs.Text, gside)
	}
}

// drawCardW11 рисует карточку Windows 11: подложка, строка приложения (у
// одиночной), заголовок, текст, время либо крестик, шеврон раскрытия, действия.
func (v *richView) drawCardW11(ctx widget.DrawContext, l *richLayout, u ncUI, c richCard, visible image.Rectangle) {
	if c.rect.Intersect(visible).Empty() {
		return
	}
	f := l.fonts
	id := c.n.ID
	k := zoneKey{kind: zoneCard, note: id}
	cardState := StateOf(u.hoverNote == id, u.pressed == k && u.hover == k, false, false, u.ring && u.focus == k)
	cs := v.mo.Style(v.tm, k, c.rect, cardState, func(s theme.State) *theme.Style { return v.part(ncPartCard, s) })
	PaintStyle(ctx, c.rect, cs)
	if c.anim {
		prev := ctx.Clip()
		ctx.SetClip(c.rect.Intersect(prev))
		defer ctx.SetClip(prev)
	}
	v.drawSeverityW11(ctx, l, c, cs)

	dim := v.part(ncPartDim, theme.StateNormal)
	if c.showApp {
		if c.hasIcon {
			drawAppIcon(ctx, c.n.Icon, c.n.IconAt, c.icon)
		}
		f.caption.draw(ctx, c.appText, c.appRect.Min.X, c.appRect.Min.Y+(c.appRect.Dy()-f.caption.lineH())/2, dim.Text)
	}
	gside := l.m.slot / ncGlyphDiv
	if u.hoverNote == id || (u.focus.note == id && u.ring) {
		// Под курсором время уступает место крестику.
		ck := zoneKey{kind: zoneCardClose, note: id}
		gs := v.style(ck, c.closeRect, ncPartGlyph, u.state(ck, false))
		PaintStyle(ctx, c.closeRect, gs)
		drawGlyphCross(ctx, c.closeRect, gs.Text, gside)
	} else {
		f.caption.draw(ctx, c.timeText, c.timeRect.Min.X, c.timeRect.Min.Y, dim.Text)
	}
	f.title.draw(ctx, c.titleText, c.title.Min.X, c.title.Min.Y, cs.Text)
	for i, line := range c.bodyLines {
		f.body.draw(ctx, line, c.bodyX, c.bodyY+i*f.body.lineH(), dim.Text)
	}
	if c.canToggle {
		tk := zoneKey{kind: zoneCardToggle, note: id}
		gs := v.style(tk, c.toggleRect, ncPartGlyph, u.state(tk, false))
		PaintStyle(ctx, c.toggleRect, gs)
		drawGlyphChevron(ctx, c.toggleRect, c.open, gs.Text, gside)
	}
	for _, a := range c.acts {
		v.drawAction(ctx, l, u, c, a)
	}
}

// drawSeverityW11 метит предупреждение и ошибку полосой у левого края карточки,
// не доходящей до скруглённых углов.
func (v *richView) drawSeverityW11(ctx widget.DrawContext, l *richLayout, c richCard, cs *theme.Style) {
	part := ""
	switch c.n.Severity {
	case SeverityWarning:
		part = ncPartSevWarn
	case SeverityError:
		part = ncPartSevError
	default:
		return
	}
	st := v.part(part, theme.StateNormal)
	if st.Fill.A == 0 {
		return
	}
	w := l.m.sevW
	if w < ncSevMin {
		w = ncSevMin
	}
	inset := int(cs.Corner)
	h := c.rect.Dy() - 2*inset
	if h <= 0 {
		return
	}
	ctx.FillRectAlpha(c.rect.Min.X, c.rect.Min.Y+inset, w, h, st.Fill)
}

// drawGlyphBell рисует колокольчик контуром в квадрате со стороной side по
// центру r; slashed — перечёркнутый («Не беспокоить» включено). Контекст без
// сглаженных фигур получает ступенчатый колокол из прямоугольников.
func drawGlyphBell(ctx widget.DrawContext, r image.Rectangle, col color.RGBA, side int, slashed bool) {
	if side < 6 || col.A == 0 {
		return
	}
	ox := r.Min.X + (r.Dx()-side)/2
	oy := r.Min.Y + (r.Dy()-side)/2
	pt := func(fx, fy float64) image.Point {
		return image.Pt(ox+int(fx*float64(side)+0.5), oy+int(fy*float64(side)+0.5))
	}
	aa, ok := ctx.(widget.AAShapes)
	if !ok {
		// Без сглаживания: купол, юбка и язычок прямоугольниками.
		ctx.FillRectAlpha(pt(0.3, 0.2).X, pt(0.3, 0.2).Y, side*2/5, side*3/5, col)
		ctx.FillRectAlpha(pt(0.12, 0.72).X, pt(0.12, 0.72).Y, side*3/4, side/8, col)
		return
	}
	th := float64(side) / 12
	if th < 1 {
		th = 1
	}
	// Контур колокола: купол, скат к юбке, юбка.
	body := []image.Point{
		pt(0.5, 0.14), pt(0.36, 0.2), pt(0.29, 0.34), pt(0.28, 0.52), pt(0.22, 0.66),
		pt(0.12, 0.76), pt(0.88, 0.76), pt(0.78, 0.66), pt(0.72, 0.52), pt(0.71, 0.34),
		pt(0.64, 0.2),
	}
	aa.StrokePolylineAA(body, th, true, col)
	// Язычок под юбкой.
	aa.StrokePolylineAA([]image.Point{pt(0.4, 0.86), pt(0.5, 0.92), pt(0.6, 0.86)}, th, false, col)
	// Ушко сверху.
	aa.DrawLineAA(pt(0.5, 0.06).X, pt(0.5, 0.06).Y, pt(0.5, 0.14).X, pt(0.5, 0.14).Y, th, col)
	if slashed {
		a, b := pt(0.1, 0.1), pt(0.9, 0.92)
		aa.DrawLineAA(a.X, a.Y, b.X, b.Y, th*1.2, col)
	}
}
