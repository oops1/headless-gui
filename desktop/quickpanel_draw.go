// quickpanel_draw.go — отрисовка быстрых настроек Windows 11.
//
// Здесь нет ни одного цвета и ни одного размера: цвета и подложки приходят из
// частей стиля "quicksettings" (w11.tile, w11.tile.on, w11.label, w11.button,
// w11.slider.*, w11.footer…), размеры — из метрик quicksettings.w11.* и из
// раскладки (quickpanel_layout.go). Сама подложка панели — стиль компонента
// (материал и тень общие со всеми панелями, см. profiles_win11_material.go).
package desktop

import (
	"image"
	"image/color"
	"strconv"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// Размеры значков и бегунка, которые не меняют вид панели, а лишь подбирают
// фигуру под метрики: сторона шеврона и ручки в режиме правки; диаметр точки
// бегунка в покое, под курсором и при нажатии; размеры значка батареи.
const (
	qsChevronSide = 16
	qsDotRest     = 12
	qsDotHover    = 14
	qsDotPressed  = 10
	qsBatteryW    = 22
	qsBatteryH    = 11
)

// qsPresenter — презентер быстрых настроек Windows 11. Рисует и меряет
// компонент, который умеет это делать (*QuickSettings); любому другому
// отвечает пустотой, и тот рисует сам.
type qsPresenter struct{}

type qsRichComponent interface {
	richSize() image.Point
	richDraw(ctx widget.DrawContext)
}

// Measure возвращает размер панели: ширина из метрики, высота — по плиткам.
func (qsPresenter) Measure(c Component, _ image.Point) image.Point {
	if rc, ok := c.(qsRichComponent); ok {
		return rc.richSize()
	}
	return image.Point{}
}

// Layout не раскладывает ячейки: у панели свои зоны (quickpanel_layout.go).
func (qsPresenter) Layout(Component, image.Rectangle) []image.Rectangle { return nil }

// Draw рисует содержимое панели.
func (qsPresenter) Draw(ctx widget.DrawContext, c Component) {
	if rc, ok := c.(qsRichComponent); ok {
		rc.richDraw(ctx)
	}
}

// Cells — Component: у панели нет ячеек, презентер обращается к ней напрямую.
func (q *QuickSettings) Cells() []Cell { return nil }

// HoverIndex — Component: наведение хранит панель, не ячейки.
func (q *QuickSettings) HoverIndex() int { return -1 }

// qsUI — снимок состояния ввода на время одной отрисовки.
type qsUI struct {
	hover, press, focus qsZone
	ring                bool
	editing             bool
	drag                qsDrag
	vol                 VolState
	bright              float64
	hasBright           bool
	pow                 PowerState
	hasBat              bool
	details             *QuickDetails
	detailsID           QuickActionID
}

func (pn *qsPanel) snapshot(ring bool) qsUI {
	pn.mu.Lock()
	defer pn.mu.Unlock()
	u := qsUI{
		hover: pn.hover, press: pn.press, focus: pn.focus, ring: ring, editing: pn.editing,
		drag: pn.drag, vol: pn.vol, bright: pn.bright, hasBright: pn.hasBright,
		pow: pn.pow, hasBat: pn.q.st != nil && pn.seen && !pn.pow.NoBattery,
		details: pn.details, detailsID: pn.detailsID,
	}
	if pn.hasVolOver {
		u.vol.Level = pn.volOver
	}
	return u
}

// partStyle читает стиль части панели из темы (пустой стиль, если темы нет).
func (pn *qsPanel) partStyle(part string, st theme.State) *theme.Style {
	tm := pn.q.Theme()
	if tm == nil {
		return &theme.Style{}
	}
	return tm.GetStyle(ComponentQuickSettings, part, st)
}

// style читает стиль части для зоны z с плавным переходом цвета (наведение,
// нажатие).
func (pn *qsPanel) style(z qsZone, r image.Rectangle, part string, st theme.State) *theme.Style {
	return pn.mo.Style(pn.q.Theme(), z, r, st, func(s theme.State) *theme.Style { return pn.partStyle(part, s) })
}

// draw рисует панель panel (в её текущем положении, то есть вместе со сдвигом
// выезда). Пока идёт переход между страницами, рисуются обе, сдвинутые друг
// относительно друга.
func (pn *qsPanel) draw(ctx widget.DrawContext, panel image.Rectangle) {
	if panel.Empty() {
		return
	}
	t := pn.page.Value()
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	u := pn.snapshot(pn.q.fs.FocusVisible())
	pn.bound = panel
	prev := ctx.Clip()
	defer ctx.SetClip(prev)
	ctx.SetClip(panel.Intersect(prev))

	w := panel.Dx()
	if t < 1 {
		off := image.Pt(-int(float64(w)*t+0.5), 0)
		l, list := pn.layout(panel.Add(off))
		pn.drawMain(ctx, l, list, u, panel)
	}
	if t > 0 && u.details != nil {
		off := image.Pt(int(float64(w)*(1-t)+0.5), 0)
		l, _ := pn.layout(panel.Add(off))
		pn.drawDetails(ctx, l, u)
	}
}

// qsPaint рисует подложку по стилю, не выходя за окно clip. PaintStyle
// скруглённого слоя ставит скруглённое отсечение, которое подменяет
// прямоугольное, — плитка, наполовину заехавшая под край окна сетки, вылезла
// бы за него. Целиком видимая плитка рисуется как обычно; обрезанная — только
// заливкой и рамкой под прямоугольным отсечением окна.
func qsPaint(ctx widget.DrawContext, r image.Rectangle, s *theme.Style, clip image.Rectangle) {
	if clip.Empty() || r.In(clip) {
		PaintStyle(ctx, r, s)
		return
	}
	if s == nil || r.Empty() {
		return
	}
	corner := int(s.Corner)
	if s.Fill.A > 0 {
		fillSolid(ctx, r, corner, s.Fill)
	}
	if s.Border.A > 0 && s.BorderWidth > 0 {
		if corner > 0 {
			ctx.DrawRoundBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), corner, s.Border)
		} else {
			ctx.DrawBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), s.Border)
		}
	}
}

// paint рисует подложку части внутри панели: пока страницы едут, их элементы
// частично или целиком оказываются за краем панели, и скруглённый PaintStyle
// вылез бы за него (см. qsPaint).
func (pn *qsPanel) paint(ctx widget.DrawContext, r image.Rectangle, s *theme.Style) {
	qsPaint(ctx, r, s, pn.bound)
}

// drawMain рисует главную страницу: плитки, ползунки (или подсказку правки),
// нижнюю строку. truePanel — настоящая рамка панели без сдвига перехода: по ней
// нижняя полоса обрезается скруглением.
func (pn *qsPanel) drawMain(ctx widget.DrawContext, l *qsLayout, list []QuickAction, u qsUI, truePanel image.Rectangle) {
	clip := ctx.Clip()

	// Плитки обрезаются окном сетки: прокрученные ряды заезжают под край.
	if l.n > 0 {
		ctx.SetClip(l.grid.Intersect(clip))
		for p := 0; p < l.n; p++ {
			idx := l.at(p)
			if idx >= len(list) {
				continue
			}
			body, label := l.tile(p)
			if !body.Union(label).Overlaps(l.grid) {
				continue
			}
			if u.drag.kind == qdTile && u.drag.moved && u.drag.idx == idx {
				// На месте, куда ляжет плитка, остаётся пустая рамка.
				qsPaint(ctx, body, pn.partStyle("w11.tile.edit", theme.StateNormal), l.grid.Intersect(pn.bound))
				continue
			}
			pn.drawTile(ctx, l, list[idx], idx, body, label, u, l.grid.Intersect(pn.bound))
		}
		ctx.SetClip(clip)
	}
	pn.drawScrollbar(ctx, l, u)

	if u.editing {
		if !l.hint.Empty() {
			ds := pn.partStyle("w11.dim", theme.StateNormal)
			DrawTextCentered(ctx, l.hint, Elide(ctx, tr(StrQuickEditHint), ds, l.hint.Dx()), ds)
		}
	} else {
		if !l.bright.row.Empty() {
			pn.drawSlider(ctx, l, l.bright, u, qsZone{kind: qzBrightIcon}, qsZone{kind: qzBright},
				qsGlyphSun, u.bright, qsZone{})
		}
		if !l.vol.row.Empty() {
			pn.drawSlider(ctx, l, l.vol, u, qsZone{kind: qzVolIcon}, qsZone{kind: qzVol},
				qsVolumeGlyph(u.vol.Level, u.vol.Muted), u.vol.Level, qsZone{kind: qzVolChevron})
		}
	}
	pn.drawFooter(ctx, l, u, truePanel)

	// Перетаскиваемая плитка — над всем, под курсором.
	if u.drag.kind == qdTile && u.drag.moved && u.drag.idx < len(list) {
		fr := image.Rect(u.drag.cur.X-u.drag.grab.X, u.drag.cur.Y-u.drag.grab.Y,
			u.drag.cur.X-u.drag.grab.X+l.m.tileW, u.drag.cur.Y-u.drag.grab.Y+l.m.tileH)
		pn.drawTile(ctx, l, list[u.drag.idx], u.drag.idx, fr, image.Rectangle{}, u, image.Rectangle{})
	}

	pn.drawFocusRing(ctx, l, u, list)
}

// drawTile рисует плитку: подложку по состоянию, значок, «›» (или ручку в
// режиме правки) и подпись под ней. Окно clip — сетка плиток, из-под края
// которой плитка может выглядывать (см. qsPaint); пустое окно — плитка в руке
// при перетаскивании: без подписи и без наведения, поверх всего.
func (pn *qsPanel) drawTile(ctx widget.DrawContext, l *qsLayout, a QuickAction, idx int,
	body, label image.Rectangle, u qsUI, clip image.Rectangle) {

	floating := clip.Empty()
	zt, zc := qsZone{kind: qzTile, idx: idx}, qsZone{kind: qzChevron, idx: idx}
	disabled := a.Disabled || a.Unavailable
	hover := !floating && (u.hover == zt || u.hover == zc)
	pressed := !floating && u.press == zt && hover
	part := "w11.tile"
	if a.On && !disabled {
		part = "w11.tile.on"
	}
	s := pn.style(zt, body, part, StateOf(hover, pressed, false, disabled, false))
	qsPaint(ctx, body, s, clip)
	if u.editing {
		qsPaint(ctx, body, pn.partStyle("w11.tile.edit", theme.StateNormal), clip)
	}

	// «›» — отдельная зона справа; в режиме правки вместо неё ручка.
	chevron := l.chevron(body)
	iconArea := body
	switch {
	case u.editing:
		iconArea.Max.X = chevron.Min.X
		drawQSGlyph(ctx, qsGlyphGrip, chevron, qsChevronSide, s.Text)
	case a.HasDetails:
		iconArea.Max.X = chevron.Min.X
		if !a.Disabled {
			ch := pn.style(zc, chevron, "w11.tile.chevron",
				StateOf(!floating && u.hover == zc, !floating && u.press == zc && u.hover == zc, false, false, false))
			qsPaint(ctx, chevron, ch, clip)
		}
		drawQSGlyph(ctx, qsGlyphChevron, chevron, qsChevronSide, s.Text)
	}
	pn.drawTileIcon(ctx, a, iconArea, s.Text)

	if floating || label.Empty() {
		return
	}
	ls := pn.partStyle("w11.label", StateOf(false, false, false, disabled, false))
	row := l.m.labelH / 2
	titleR := image.Rect(label.Min.X, label.Min.Y, label.Max.X, label.Min.Y+row)
	if a.Title != "" {
		DrawTextCentered(ctx, titleR, Elide(ctx, a.Title, ls, label.Dx()-4), ls)
	}
	if a.Detail != "" {
		ds := pn.partStyle("w11.detail", theme.StateNormal)
		detailR := image.Rect(label.Min.X, titleR.Max.Y, label.Max.X, label.Max.Y)
		DrawTextCentered(ctx, detailR, Elide(ctx, a.Detail, ds, label.Dx()-4), ds)
	}
}

// drawTileIcon рисует значок плитки по центру области: потребитель отдаёт
// картинку нужного размера (IconAt получает физические пиксели). Если тема
// включила перекраску (quicksettings.icon.tint), значок берёт цвет текста
// плитки — Windows 11 рисует их одним цветом.
func (pn *qsPanel) drawTileIcon(ctx widget.DrawContext, a QuickAction, area image.Rectangle, col color.RGBA) {
	side := readQSMetrics(pn.q.Theme()).icon
	box := glyphSquare(image.Rect(area.Min.X+(area.Dx()-side)/2, area.Min.Y+(area.Dy()-side)/2,
		area.Min.X+(area.Dx()-side)/2+side, area.Min.Y+(area.Dy()-side)/2+side))
	if box.Empty() {
		return
	}
	tm := pn.q.Theme()
	if tm == nil || !tm.GetFlag(theme.KeyQuickIconTint, false) {
		drawAppIcon(ctx, a.Icon, a.IconAt, box)
		return
	}
	p := widget.PhysicalRect(ctx, box)
	phys := p.X
	if p.Y < phys {
		phys = p.Y
	}
	img := a.IconFor(phys)
	if img == nil {
		return
	}
	memo := pn.memos[a.ID]
	if memo == nil {
		memo = &glyphMemo{}
		pn.memos[a.ID] = memo
	}
	ctx.DrawImageScaled(memo.tinted(img, col), box.Min.X, box.Min.Y, box.Dx(), box.Dy())
}

// drawButton рисует кнопку-значок зоны z в прямоугольнике r.
func (pn *qsPanel) drawButton(ctx widget.DrawContext, u qsUI, z qsZone, r image.Rectangle, g qsGlyph, active bool) {
	hover := active && u.hover == z
	s := pn.style(z, r, "w11.button", StateOf(hover, active && u.press == z && hover, false, false, false))
	pn.paint(ctx, r, s)
	drawQSGlyph(ctx, g, r, qsChevronSide, s.Text)
}

// drawSlider рисует строку ползунка: значок, дорожку с заполнением, бегунок,
// значение и (у громкости) «›».
func (pn *qsPanel) drawSlider(ctx widget.DrawContext, l *qsLayout, row qsRow, u qsUI,
	zIcon, zSlider qsZone, g qsGlyph, level float64, zChev qsZone) {

	// Значок нажимается только у громкости (выключает звук); у яркости это
	// просто метка.
	pn.drawButton(ctx, u, zIcon, row.icon, g, zIcon.kind == qzVolIcon)

	cy := row.row.Min.Y + row.row.Dy()/2
	th := l.m.sliderTrack
	track := image.Rect(row.track.Min.X, cy-th/2, row.track.Max.X, cy-th/2+th)
	pn.paint(ctx, track, pn.partStyle("w11.slider.track", theme.StateNormal))
	cx := row.thumbCenter(level, l.m.sliderThumb)
	if fill := image.Rect(track.Min.X, track.Min.Y, cx, track.Max.Y); !fill.Empty() {
		pn.paint(ctx, fill, pn.partStyle("w11.slider.fill", theme.StateNormal))
	}
	thumb := image.Rect(cx-l.m.sliderThumb/2, cy-l.m.sliderThumb/2,
		cx-l.m.sliderThumb/2+l.m.sliderThumb, cy-l.m.sliderThumb/2+l.m.sliderThumb)
	pn.paint(ctx, thumb, pn.partStyle("w11.slider.thumb", theme.StateNormal))

	// Точка в бегунке: под курсором крупнее, под рукой мельче.
	dot, dst := qsDotRest, theme.StateNormal
	switch {
	case u.drag.kind == qdSlider && u.drag.zone == zSlider:
		dot, dst = qsDotPressed, theme.StatePressed
	case u.hover == zSlider:
		dot, dst = qsDotHover, theme.StateHover
	}
	ds := pn.partStyle("w11.slider.dot", dst)
	ctx.FillRoundRect(cx-dot/2, cy-dot/2, dot, dot, dot/2, ds.Fill)

	vs := pn.partStyle("w11.dim", theme.StateNormal)
	DrawTextCentered(ctx, row.value, strconv.Itoa(int(level*100+0.5)), vs)
	if !row.chev.Empty() {
		pn.drawButton(ctx, u, zChev, row.chev, qsGlyphChevron, true)
	}
}

// drawFooter рисует нижнюю строку: полосу со скруглением по рамке панели,
// батарею слева, «Изменить» (или «Готово») и «Параметры» справа.
func (pn *qsPanel) drawFooter(ctx widget.DrawContext, l *qsLayout, u qsUI, truePanel image.Rectangle) {
	fs := pn.partStyle("w11.footer", theme.StateNormal)
	prev := ctx.Clip()
	rc, round := ctx.(widget.RoundClipper)
	corner := int(pn.q.style(theme.StateNormal).Corner)
	if round && corner > 0 {
		rc.SetRoundClip(truePanel, corner)
	}
	if fs.Fill.A > 0 {
		ctx.FillRectAlpha(l.footer.Min.X, l.footer.Min.Y, l.footer.Dx(), l.footer.Dy(), fs.Fill)
	}
	if fs.Border.A > 0 && fs.BorderWidth > 0 {
		ctx.FillRectAlpha(l.footer.Min.X, l.footer.Min.Y, l.footer.Dx(), 1, fs.Border)
	}
	if round && corner > 0 {
		rc.ClearRoundClip()
		ctx.SetClip(prev)
	}

	if u.hasBat && !l.battery.Empty() {
		pn.drawBattery(ctx, l.battery, u.pow, fs.Text)
	}
	if u.editing {
		z := qsZone{kind: qzEdit}
		hover := u.hover == z
		ds := pn.style(z, l.edit, "w11.done", StateOf(hover, u.press == z && hover, false, false, false))
		pn.paint(ctx, l.edit, ds)
		DrawTextCentered(ctx, l.edit, tr(StrQuickDone), ds)
	} else {
		pn.drawButton(ctx, u, qsZone{kind: qzEdit}, l.edit, qsGlyphPencil, true)
	}
	pn.drawButton(ctx, u, qsZone{kind: qzSettings}, l.settings, qsGlyphGear, true)
}

// drawBattery рисует значок батареи по заряду и процент рядом. При питании от
// сети рядом с процентом молния.
func (pn *qsPanel) drawBattery(ctx widget.DrawContext, r image.Rectangle, p PowerState, col color.RGBA) {
	charge := p.Charge
	if charge < 0 {
		charge = 0
	}
	if charge > 1 {
		charge = 1
	}
	x := r.Min.X
	y := r.Min.Y + (r.Dy()-qsBatteryH)/2
	ctx.DrawRoundBorder(x, y, qsBatteryW, qsBatteryH, 2, col)
	ctx.FillRect(x+qsBatteryW, y+3, 2, qsBatteryH-6, col)
	if w := int(float64(qsBatteryW-4)*charge + 0.5); w > 0 {
		ctx.FillRoundRect(x+2, y+2, w, qsBatteryH-4, 1, col)
	}
	tx := x + qsBatteryW + 2 + 8
	if p.OnAC {
		bolt := image.Rect(tx-2, r.Min.Y, tx-2+14, r.Max.Y)
		drawQSGlyph(ctx, qsGlyphBatPlug, bolt, 14, col)
		tx += 14
	}
	ls := pn.partStyle("w11.label", theme.StateNormal)
	DrawTextLeft(ctx, image.Rect(tx, r.Min.Y, r.Max.X, r.Max.Y), strconv.Itoa(int(charge*100+0.5))+"%", ls)
}

// drawScrollbar рисует тонкий бегунок прокрутки сетки в правом поле панели.
func (pn *qsPanel) drawScrollbar(ctx widget.DrawContext, l *qsLayout, u qsUI) {
	if l.thumb.Empty() {
		return
	}
	z := qsZone{kind: qzThumb}
	st := theme.StateNormal
	switch {
	case u.drag.kind == qdThumb:
		st = theme.StatePressed
	case u.hover == z:
		st = theme.StateHover
	}
	pn.paint(ctx, l.thumb, pn.partStyle("w11.scrollbar", st))
}

// drawFocusRing обводит зону клавиатурного фокуса главной страницы.
func (pn *qsPanel) drawFocusRing(ctx widget.DrawContext, l *qsLayout, u qsUI, list []QuickAction) {
	if !u.ring || u.focus.kind == qzNone || u.focus.kind == qzBack || u.focus.kind == qzContent {
		return
	}
	r := l.focusRect(u.focus)
	if r.Empty() {
		return
	}
	prev := ctx.Clip()
	if u.focus.kind == qzTile || u.focus.kind == qzChevron {
		ctx.SetClip(l.grid.Intersect(prev))
	}
	PaintFocusRing(ctx, r, pn.q.Theme(), pn.partStyle("w11.button", theme.StateNormal))
	ctx.SetClip(prev)
}

// drawDetails рисует вложенную страницу: стрелку «назад», заголовок и
// содержимое потребителя.
func (pn *qsPanel) drawDetails(ctx widget.DrawContext, l *qsLayout, u qsUI) {
	pn.drawButton(ctx, u, qsZone{kind: qzBack}, l.back, qsGlyphBack, true)
	ls := pn.partStyle("w11.title", theme.StateNormal)
	titleR := image.Rect(l.back.Max.X+8, l.header.Min.Y, l.header.Max.X-l.m.pad, l.header.Max.Y)
	if title := pn.detailsTitle(u.details, u.detailsID); title != "" {
		DrawTextLeft(ctx, titleR, Elide(ctx, title, ls, titleR.Dx()), ls)
	}
	if c := u.details.Content; c != nil {
		if c.Bounds() != l.body {
			c.SetBounds(l.body)
		}
		prev := ctx.Clip()
		ctx.SetClip(l.body.Intersect(prev))
		c.Draw(ctx)
		ctx.SetClip(prev)
	}
	if u.ring && u.focus.kind == qzBack {
		PaintFocusRing(ctx, l.back, pn.q.Theme(), pn.partStyle("w11.button", theme.StateNormal))
	}
}

// detailsTitle — заголовок вложенной страницы: свой, а если его нет —
// название плитки (громкость — на текущем языке).
func (pn *qsPanel) detailsTitle(d *QuickDetails, id QuickActionID) string {
	if d == nil {
		return ""
	}
	if d.Title != "" {
		return d.Title
	}
	if id == QuickVolumeID {
		return tr(StrQuickVolume)
	}
	pn.mu.Lock()
	defer pn.mu.Unlock()
	for _, a := range pn.list {
		if a.ID == id {
			return a.Title
		}
	}
	return ""
}
