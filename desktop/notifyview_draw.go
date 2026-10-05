// notifyview_draw.go — отрисовка центра уведомлений Windows 10.
//
// Здесь нет ни одного цвета и ни одного размера: цвета и подложки приходят из
// частей стиля "notificationcenter" (panel, card, action, field, glyph, dim,
// link, group, quick.tile…), размеры — из метрик и из раскладки (notifyview.go).
package desktop

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ncTileLineRatio — межстрочный интервал подписи плитки быстрого действия: она
// в две строки и должна уместиться под значком, поэтому теснее обычного текста.
const ncTileLineRatio = 1.25

// ncGlyphDiv — доля стороны слота (32 точки) под фигуру крестика и шеврона.
const ncGlyphDiv = 3

// ncUI — снимок состояния ввода на время одной отрисовки.
type ncUI struct {
	hover, pressed, focus zoneKey
	ring                  bool
	hoverNote             NotificationID
	hoverApp              AppID
	thumb                 bool
	dropHot               int
	dropSel               int
}

// uiSnapshot копирует состояние ввода под замком.
func (v *richView) uiSnapshot(ring bool) ncUI {
	v.mu.Lock()
	defer v.mu.Unlock()
	u := ncUI{hover: v.hover, pressed: v.pressed, focus: v.focus, ring: ring, dropHot: v.dropHot}
	switch v.hover.kind {
	case zoneCard, zoneCardClose, zoneCardToggle, zoneAction, zoneReply, zoneReplySend, zoneSelect, zoneOption:
		u.hoverNote = v.hover.note
	case zoneGroup, zoneGroupClose, zoneGroupToggle:
		u.hoverApp = v.hover.app
	}
	u.thumb = time.Now().Before(v.thumbUntil)
	return u
}

// state возвращает состояние зоны по снимку ввода.
func (u ncUI) state(k zoneKey, disabled bool) theme.State {
	hover := u.hover == k
	return StateOf(hover, u.pressed == k && hover, false, disabled, u.ring && u.focus == k)
}

// style читает стиль части зоны с плавным переходом цвета.
func (v *richView) style(k zoneKey, r image.Rectangle, part string, st theme.State) *theme.Style {
	return v.mo.Style(v.tm, k, r, st, func(s theme.State) *theme.Style { return v.part(part, s) })
}

// draw рисует панель panel (в её текущем положении, то есть вместе со сдвигом
// анимации появления). ring — показывать ли рамку клавиатурного фокуса.
func (v *richView) draw(ctx widget.DrawContext, panel image.Rectangle, ring bool) {
	if panel.Empty() {
		return
	}
	l := v.layout(panel)
	u := v.uiSnapshot(ring)
	prev := ctx.Clip()
	defer ctx.SetClip(prev)

	if v.mode == ncModeToast {
		for _, g := range l.groups {
			for _, c := range g.cards {
				v.drawCard(ctx, l, u, c, prev)
			}
		}
		v.drawFocusRing(ctx, l, u, prev)
		return
	}

	v.drawLink(ctx, l, u, zoneKey{kind: zoneManage}, l.manage, tr(StrNotifManage))

	// Список: всё, что не помещается, обрезается областью просмотра.
	ctx.SetClip(l.viewport.Intersect(prev))
	if l.empty {
		st := v.part(ncPartDim, theme.StateNormal)
		DrawTextCentered(ctx, l.viewport, v.emptyLabel(), st)
	}
	for _, g := range l.groups {
		v.drawGroup(ctx, l, u, g)
		for _, c := range g.cards {
			v.drawCard(ctx, l, u, c, l.viewport.Intersect(prev))
		}
	}
	v.drawScrollbar(ctx, l, u)
	ctx.SetClip(prev)

	// Нижняя часть.
	if !l.clear.Empty() {
		v.drawLink(ctx, l, u, zoneKey{kind: zoneClear}, l.clear, tr(StrNotifClear))
	}
	if !l.expand.Empty() {
		label := StrNotifExpand
		v.mu.Lock()
		if v.quickOpen {
			label = StrNotifCollapse
		}
		v.mu.Unlock()
		v.drawLink(ctx, l, u, zoneKey{kind: zoneExpand}, l.expand, tr(label))
	}
	for _, t := range l.tiles {
		v.drawTile(ctx, l, u, t)
	}
	v.drawDropdown(ctx, l, u)
	v.drawFocusRing(ctx, l, u, prev)
}

// emptyLabel — подпись пустого центра.
func (v *richView) emptyLabel() string {
	if v.src.emptyMsg != nil {
		if s := v.src.emptyMsg(); s != "" {
			return s
		}
	}
	return tr(StrNotifEmpty)
}

// drawLink рисует ссылку: цвет акцента, наведение и нажатие — его оттенки.
func (v *richView) drawLink(ctx widget.DrawContext, l *richLayout, u ncUI, k zoneKey, r image.Rectangle, label string) {
	st := v.style(k, r, ncPartLink, u.state(k, false))
	label = l.fonts.link.elide(label, r.Dx())
	w := l.fonts.link.width(label)
	l.fonts.link.draw(ctx, label, r.Min.X+(r.Dx()-w)/2, r.Min.Y+(r.Dy()-l.fonts.link.lineH())/2, st.Text)
}

// draw выводит строку: y — верх строки, центр глифа ставится в середину её
// высоты.
func (t ncText) draw(ctx widget.DrawContext, s string, x, y int, col color.RGBA) {
	if s == "" {
		return
	}
	gy := y + (t.lineH()-t.glyphH())/2
	if t.face != "" {
		ctx.DrawTextFont(s, x, gy, t.size, t.face, col)
		return
	}
	ctx.DrawTextSize(s, x, gy, t.size, col)
}

// drawGroup рисует заголовок группы: значок и имя приложения по центру,
// справа — шеврон и крестик (под курсором или в фокусе).
func (v *richView) drawGroup(ctx widget.DrawContext, l *richLayout, u ncUI, g richGroup) {
	if g.rect.Empty() || g.rect.Max.Y < l.viewport.Min.Y || g.rect.Min.Y > l.viewport.Max.Y {
		return
	}
	k := zoneKey{kind: zoneGroup, app: g.app}
	st := v.style(k, g.rect, ncPartGroup, u.state(k, false))
	PaintStyle(ctx, g.rect, st)

	f := l.fonts
	name := g.name
	if g.collapsed {
		name += "  " + strconv.Itoa(g.count)
	}
	iconSide := l.m.groupIcon
	gap := l.m.cardGap * 2
	avail := g.rect.Dx() - 2*l.m.slot
	name = f.title.elide(name, avail-iconSide-gap)
	tw := f.title.width(name)
	total := tw
	hasIcon := hasNoteIcon(g.note)
	if hasIcon {
		total += iconSide + gap
	}
	x := g.rect.Min.X + (g.rect.Dx()-total)/2
	if hasIcon {
		ir := image.Rect(x, g.rect.Min.Y+(g.rect.Dy()-iconSide)/2, x+iconSide, g.rect.Min.Y+(g.rect.Dy()-iconSide)/2+iconSide)
		drawAppIcon(ctx, g.note.Icon, g.note.IconAt, ir)
		x += iconSide + gap
	}
	f.title.draw(ctx, name, x, g.rect.Min.Y+(g.rect.Dy()-f.title.lineH())/2, st.Text)

	if u.hoverApp == g.app || (u.focus.kind == zoneGroup && u.focus.app == g.app) {
		gk := zoneKey{kind: zoneGroupToggle, app: g.app}
		gs := v.style(gk, g.chevronRect, ncPartGlyph, u.state(gk, false))
		drawGlyphChevron(ctx, g.chevronRect, !g.collapsed, gs.Text, l.m.slot/ncGlyphDiv)
		ck := zoneKey{kind: zoneGroupClose, app: g.app}
		cs := v.style(ck, g.closeRect, ncPartGlyph, u.state(ck, false))
		drawGlyphCross(ctx, g.closeRect, cs.Text, l.m.slot/ncGlyphDiv)
	}
}

// drawCard рисует карточку: подложка, значок, заголовок, текст, время,
// крестик и шеврон раскрытия, действия.
func (v *richView) drawCard(ctx widget.DrawContext, l *richLayout, u ncUI, c richCard, visible image.Rectangle) {
	if c.rect.Intersect(visible).Empty() {
		return
	}
	f := l.fonts
	id := c.n.ID
	k := zoneKey{kind: zoneCard, note: id}
	cardState := StateOf(u.hoverNote == id, u.pressed == k && u.hover == k, false, false, u.ring && u.focus == k)
	cs := v.mo.Style(v.tm, k, c.rect, cardState, func(s theme.State) *theme.Style { return v.part(ncPartCard, s) })
	PaintStyle(ctx, c.rect, cs)

	if c.hasIcon {
		drawAppIcon(ctx, c.n.Icon, c.n.IconAt, c.icon)
	}
	dim := v.part(ncPartDim, theme.StateNormal)
	f.title.draw(ctx, c.titleText, c.title.Min.X, c.title.Min.Y, cs.Text)
	for i, line := range c.bodyLines {
		f.body.draw(ctx, line, c.bodyX, c.bodyY+i*f.body.lineH(), dim.Text)
	}
	f.caption.draw(ctx, c.timeText, c.timeRect.Min.X, c.timeRect.Min.Y, dim.Text)

	gside := l.m.slot / ncGlyphDiv
	if u.hoverNote == id || (u.focus.note == id && u.ring) {
		ck := zoneKey{kind: zoneCardClose, note: id}
		gs := v.style(ck, c.closeRect, ncPartGlyph, u.state(ck, false))
		drawGlyphCross(ctx, c.closeRect, gs.Text, gside)
	}
	if c.canToggle {
		tk := zoneKey{kind: zoneCardToggle, note: id}
		gs := v.style(tk, c.toggleRect, ncPartGlyph, u.state(tk, false))
		drawGlyphChevron(ctx, c.toggleRect, c.open, gs.Text, gside)
	}
	for _, a := range c.acts {
		v.drawAction(ctx, l, u, c, a)
	}
}

// drawAction рисует одно действие карточки.
func (v *richView) drawAction(ctx widget.DrawContext, l *richLayout, u ncUI, c richCard, a richAct) {
	f := l.fonts
	id := c.n.ID
	switch a.a.Kind {
	case NotificationActionSelect:
		dim := v.part(ncPartCard, theme.StateNormal)
		f.body.draw(ctx, f.body.elide(a.a.Title, a.label.Dx()), a.label.Min.X, a.label.Min.Y, dim.Text)
		k := zoneKey{kind: zoneSelect, note: id, action: a.a.ID}
		st := v.style(k, a.rect, ncPartField, u.state(k, false))
		PaintStyle(ctx, a.rect, st)
		v.mu.Lock()
		cur := v.selected(c.n, a.a)
		open := v.drop == (ncInputKey{id, a.a.ID})
		v.mu.Unlock()
		text := ""
		if cur < len(a.a.Options) {
			text = a.a.Options[cur]
		}
		pad := l.m.cardPad - 4
		arrow := l.m.slot / 2
		f.body.draw(ctx, f.body.elide(text, a.rect.Dx()-pad-arrow-pad), a.rect.Min.X+pad,
			a.rect.Min.Y+(a.rect.Dy()-f.body.lineH())/2, st.Text)
		ar := image.Rect(a.rect.Max.X-arrow-pad/2, a.rect.Min.Y, a.rect.Max.X-pad/2, a.rect.Max.Y)
		drawGlyphChevron(ctx, ar, open, st.Text, l.m.slot/ncGlyphDiv)
	case NotificationActionReply:
		rk := zoneKey{kind: zoneReply, note: id, action: a.a.ID}
		st := v.style(rk, a.rect, ncPartField, u.state(rk, false))
		PaintStyle(ctx, a.rect, st)
		v.mu.Lock()
		rp := v.replyOf(c.n, a.a)
		text := string(rp.text)
		caret := rp.caret
		v.mu.Unlock()
		pad := l.m.cardPad - 4
		inner := image.Rect(a.rect.Min.X+pad, a.rect.Min.Y, a.rect.Max.X-pad, a.rect.Max.Y)
		ty := a.rect.Min.Y + (a.rect.Dy()-f.body.lineH())/2
		if text == "" {
			hint := a.a.Placeholder
			if hint == "" {
				hint = tr(StrNotifReplyHint)
			}
			dim := v.part(ncPartDim, theme.StateNormal)
			f.body.draw(ctx, f.body.elide(hint, inner.Dx()), inner.Min.X, ty, dim.Text)
		} else {
			// Длинный ответ прокручивается: виден хвост у каретки.
			rs := []rune(text)
			start := 0
			for start < caret && f.body.width(string(rs[start:caret])) > inner.Dx() {
				start++
			}
			prev := ctx.Clip()
			ctx.SetClip(inner.Intersect(prev))
			f.body.draw(ctx, string(rs[start:]), inner.Min.X, ty, st.Text)
			ctx.SetClip(prev)
			text = string(rs[start:caret])
		}
		if u.focus == rk {
			cx := inner.Min.X
			if text != "" {
				cx += f.body.width(text)
			}
			if cx > inner.Max.X {
				cx = inner.Max.X
			}
			ctx.FillRect(cx, ty, 1, f.body.lineH(), st.Text)
		}
		sk := zoneKey{kind: zoneReplySend, note: id, action: a.a.ID}
		ss := v.style(sk, a.send, ncPartAction, u.state(sk, false))
		PaintStyle(ctx, a.send, ss)
		f.body.draw(ctx, a.text, a.send.Min.X+(a.send.Dx()-f.body.width(a.text))/2,
			a.send.Min.Y+(a.send.Dy()-f.body.lineH())/2, ss.Text)
	case NotificationActionLink:
		k := zoneKey{kind: zoneAction, note: id, action: a.a.ID}
		st := v.style(k, a.rect, ncPartLink, u.state(k, false))
		f.link.draw(ctx, a.text, a.rect.Min.X+(a.rect.Dx()-f.link.width(a.text))/2,
			a.rect.Min.Y+(a.rect.Dy()-f.link.lineH())/2, st.Text)
	default:
		k := zoneKey{kind: zoneAction, note: id, action: a.a.ID}
		st := v.style(k, a.rect, ncPartAction, u.state(k, false))
		PaintStyle(ctx, a.rect, st)
		f.body.draw(ctx, a.text, a.rect.Min.X+(a.rect.Dx()-f.body.width(a.text))/2,
			a.rect.Min.Y+(a.rect.Dy()-f.body.lineH())/2, st.Text)
	}
}

// drawTile рисует плитку быстрого действия: значок сверху слева, подпись
// снизу слева в одну-две строки. Включённая заливается акцентом (часть
// quick.tile.on), недоступная приглушена.
func (v *richView) drawTile(ctx widget.DrawContext, l *richLayout, u ncUI, t richTile) {
	k := zoneKey{kind: zoneTile, action: string(t.a.ID)}
	part := ncPartTile
	if t.a.On && !t.a.Disabled {
		part = ncPartTileOn
	}
	st := v.style(k, t.rect, part, u.state(k, t.a.Disabled))
	PaintStyle(ctx, t.rect, st)

	m := l.m
	ir := image.Rect(t.rect.Min.X+m.qPad, t.rect.Min.Y+m.qPad, t.rect.Min.X+m.qPad+m.qIcon, t.rect.Min.Y+m.qPad+m.qIcon)
	drawAppIcon(ctx, t.a.Icon, t.a.IconAt, ir)

	f := l.fonts.body
	avail := t.rect.Dx() - 2*m.qPad
	// Слово, не влезающее в плитку целиком, не режется по буквам: подпись
	// переходит на мелкий шрифт.
	for _, w := range strings.Fields(t.a.Title) {
		if f.width(w) > avail {
			f = l.fonts.caption
			break
		}
	}
	lh := int(f.size*ncPxPerPt*ncTileLineRatio + 0.5)
	lines, _ := f.wrap(t.a.Title, avail, 2)
	y := t.rect.Max.Y - m.qPad + (lh-f.glyphH())/2 - len(lines)*lh + 2
	for i, line := range lines {
		gy := y + i*lh
		if f.face != "" {
			ctx.DrawTextFont(line, t.rect.Min.X+m.qPad, gy, f.size, f.face, st.Text)
		} else {
			ctx.DrawTextSize(line, t.rect.Min.X+m.qPad, gy, f.size, st.Text)
		}
	}
}

// drawDropdown рисует раскрытый выпадающий список поверх панели.
func (v *richView) drawDropdown(ctx widget.DrawContext, l *richLayout, u ncUI) {
	if l.dropRect.Empty() {
		return
	}
	prev := ctx.Clip()
	ctx.SetClip(l.panel.Intersect(prev))
	defer ctx.SetClip(prev)
	PaintStyle(ctx, l.dropRect, v.part(ncPartPanel, theme.StateNormal))
	ctx.DrawBorder(l.dropRect.Min.X, l.dropRect.Min.Y, l.dropRect.Dx(), l.dropRect.Dy(), v.part(ncPartField, theme.StateNormal).Border)
	v.mu.Lock()
	cur := -1
	for _, g := range l.groups {
		for _, c := range g.cards {
			if c.n.ID == l.dropNote {
				for _, a := range c.acts {
					if a.a.ID == l.dropAct {
						cur = v.selected(c.n, a.a)
					}
				}
			}
		}
	}
	v.mu.Unlock()
	f := l.fonts
	for i, item := range l.dropItems {
		ir := image.Rect(l.dropRect.Min.X, l.dropRect.Min.Y+i*l.m.actH, l.dropRect.Max.X, l.dropRect.Min.Y+(i+1)*l.m.actH)
		k := zoneKey{kind: zoneOption, note: l.dropNote, action: l.dropAct, index: i}
		st := theme.StateNormal
		switch {
		case u.dropHot == i:
			st = theme.StateHover
		case cur == i:
			st = theme.StateActive
		}
		s := v.part(ncPartAction, st)
		PaintStyle(ctx, ir.Inset(1), s)
		_ = k
		f.body.draw(ctx, f.body.elide(item, ir.Dx()-2*l.m.cardPad), ir.Min.X+l.m.cardPad-4, ir.Min.Y+(ir.Dy()-f.body.lineH())/2, s.Text)
	}
}

// drawScrollbar рисует тонкий бегунок на правом краю списка: он виден, пока
// мышь над списком или вскоре после прокрутки, и не занимает места.
func (v *richView) drawScrollbar(ctx widget.DrawContext, l *richLayout, u ncUI) {
	if l.maxScrl <= 0 || !u.thumb {
		return
	}
	track := l.viewport
	th := track.Dy() * track.Dy() / l.contentH
	if th < ncThumbMin {
		th = ncThumbMin
	}
	if th > track.Dy() {
		th = track.Dy()
	}
	ty := track.Min.Y + (track.Dy()-th)*l.scroll/l.maxScrl
	x := track.Max.X - l.m.sbW - 2
	r := image.Rect(x, ty, x+l.m.sbW, ty+th)
	st := v.part(ncPartScrollbar, theme.StateNormal)
	if st.Fill.A > 0 {
		ctx.FillRectAlpha(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), st.Fill)
	}
}

// drawFocusRing обводит зону клавиатурного фокуса.
func (v *richView) drawFocusRing(ctx widget.DrawContext, l *richLayout, u ncUI, prev image.Rectangle) {
	if !u.ring || u.focus.kind == zoneNone {
		return
	}
	z, ok := l.find(u.focus)
	if !ok {
		return
	}
	r := z.hit
	if r.Empty() {
		return
	}
	ctx.SetClip(l.panel.Intersect(prev))
	PaintFocusRing(ctx, r, v.tm, v.part(ncPartAction, theme.StateNormal))
}

// ─── Фигуры ──────────────────────────────────────────────────────────────────

// plotPx закрашивает одну логическую точку с наложением: цвета темы у
// приглушённых значков полупрозрачны, и SetPixel записал бы их мимо смешивания.
func plotPx(ctx widget.DrawContext, x, y int, col color.RGBA) {
	ctx.FillRectAlpha(x, y, 1, 1, col)
}

// drawGlyphCross рисует крестик закрытия стороной side в центре r.
func drawGlyphCross(ctx widget.DrawContext, r image.Rectangle, col color.RGBA, side int) {
	if side < 3 || col.A == 0 {
		return
	}
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	x0, y0 := cx-side/2, cy-side/2
	for i := 0; i < side; i++ {
		plotPx(ctx, x0+i, y0+i, col)
		plotPx(ctx, x0+side-1-i, y0+i, col)
	}
}

// drawGlyphChevron рисует шеврон шириной side в центре r: вверх (up) либо вниз.
func drawGlyphChevron(ctx widget.DrawContext, r image.Rectangle, up bool, col color.RGBA, side int) {
	if side < 3 || col.A == 0 {
		return
	}
	half := side / 2
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
	for i := 0; i <= half; i++ {
		// Вершина (i=0): у «вверх» она наверху, у «вниз» — внизу.
		y := cy + half/2 - i
		if up {
			y = cy - half/2 + i
		}
		plotPx(ctx, cx-i, y, col)
		plotPx(ctx, cx+i, y, col)
	}
}
