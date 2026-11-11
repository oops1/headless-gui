package base

import (
	"image"
	"math"
	"time"
)

// mergeview_collapse.go — свёрнутые панели и раскладка верха и итога (GG-96).
//
// Каждую верхнюю панель («наше», «база», «их») можно свернуть в узкую полосу
// с кнопкой «развернуть»; хотя бы одна верхняя панель остаётся развёрнутой.
// SetShowBase(false) сохраняет прежний смысл — базы нет совсем. У свёрнутой
// панели кнопки решения не рисуются; Resolve, ResolveCurrent и ResolveAll из
// кода работают. Итог можно развернуть на всю высоту (верх — полоса шапок) и
// наоборот; граница между верхом и итогом тянется, её доля читается и
// задаётся. Одно событие OnLayoutChanged — на любое изменение раскладки.

const (
	mvStripW         = 28   // ширина полосы свёрнутой панели
	mvFoldBtnW       = 24   // кнопка-шеврон в шапке и на разделителе
	mvSplitDefault   = 0.58 // доля высоты верха по умолчанию
	mvSplitMin       = 0.2
	mvSplitMax       = 0.85
	mvMaxNone        = 0
	mvMaxResult      = 1 // итог на всю высоту
	mvMaxSources     = 2 // верх на всю высоту
	mvHoverResultMax = 3 // hoverFold: кнопка «итог на всю высоту»
	mvHoverSourceMax = 4 // hoverFold: кнопка «верх на всю высоту»
)

// SetPaneCollapsed сворачивает или разворачивает верхнюю панель (MergeOurs,
// MergeBase, MergeTheirs). Свернуть последнюю развёрнутую нельзя — вызов
// ничего не меняет. Если свернули активную панель, ввод уходит в итог.
func (m *MergeView) SetPaneCollapsed(side MergeSide, v bool) {
	if side < MergeOurs || side > MergeTheirs {
		return
	}
	m.do(func() { m.setPaneCollapsedLocked(side, v) })
}

func (m *MergeView) setPaneCollapsedLocked(side MergeSide, v bool) {
	if m.coll[side] == v {
		return
	}
	if v && m.paneShownLocked(side) && m.visibleTopLocked() <= 1 {
		return // хотя бы одна верхняя панель остаётся развёрнутой
	}
	m.coll[side] = v
	if v && m.active == side {
		m.setActiveLocked(MergeResult)
	}
	m.layoutChangedLocked()
}

// PaneCollapsed сообщает, свёрнута ли верхняя панель.
func (m *MergeView) PaneCollapsed(side MergeSide) bool {
	if side < MergeOurs || side > MergeTheirs {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.coll[side]
}

// SetShowCollapseButtons показывает или прячет кнопки «свернуть» в шапках
// верхних панелей и кнопки «на всю высоту» на разделителе — для приложения,
// которое ставит свои. Полоса свёрнутой панели разворачивается щелчком и без
// них.
func (m *MergeView) SetShowCollapseButtons(v bool) {
	m.do(func() { m.foldBtns = v })
}

// ShowCollapseButtons сообщает, показаны ли кнопки сворачивания.
func (m *MergeView) ShowCollapseButtons() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.foldBtns
}

// SetResultMaximized разворачивает итог на всю высоту: от верхних панелей
// остаются шапки. false возвращает прежнюю долю. Снимает SetSourcesMaximized.
func (m *MergeView) SetResultMaximized(v bool) {
	m.do(func() { m.setMaxLocked(mvMaxResult, v) })
}

// ResultMaximized сообщает, развёрнут ли итог на всю высоту.
func (m *MergeView) ResultMaximized() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.maxMode == mvMaxResult
}

// SetSourcesMaximized разворачивает верхние панели на всю высоту: от итога
// остаётся шапка. Снимает SetResultMaximized.
func (m *MergeView) SetSourcesMaximized(v bool) {
	m.do(func() { m.setMaxLocked(mvMaxSources, v) })
}

// SourcesMaximized сообщает, развёрнуты ли верхние панели на всю высоту.
func (m *MergeView) SourcesMaximized() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.maxMode == mvMaxSources
}

func (m *MergeView) setMaxLocked(mode int, v bool) {
	next := m.maxMode
	switch {
	case v:
		next = mode
	case m.maxMode == mode:
		next = mvMaxNone
	}
	if next == m.maxMode {
		return
	}
	m.maxMode = next
	if next == mvMaxResult && m.active != MergeResult {
		m.setActiveLocked(MergeResult) // верхних строк не видно — ввод в итог
	}
	m.layoutChangedLocked()
}

// SetSplit задаёт долю высоты под верхние панели (0.2…0.85; по умолчанию
// 0.58). Снимает развёртывание итога или верха на всю высоту.
func (m *MergeView) SetSplit(ratio float64) {
	m.do(func() { m.setSplitLocked(ratio) })
}

func (m *MergeView) setSplitLocked(ratio float64) {
	if math.IsNaN(ratio) {
		ratio = mvSplitDefault
	}
	ratio = math.Max(mvSplitMin, math.Min(mvSplitMax, ratio))
	if ratio == m.split && m.maxMode == mvMaxNone {
		return
	}
	m.split, m.maxMode = ratio, mvMaxNone
	m.layoutChangedLocked()
}

// Split — доля высоты под верхние панели (без учёта развёртывания на всю
// высоту: её возвращает обычная раскладка).
func (m *MergeView) Split() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.split
}

// layoutChangedLocked отмечает изменение раскладки: событие OnLayoutChanged
// уйдёт после операции (при перетаскивании разделителя — когда его отпустят),
// прокрутки зажимаются в новые пределы.
func (m *MergeView) layoutChangedLocked() {
	m.layoutEvent = true
	m.hoverChunk, m.hoverBtn, m.hoverFold = -1, -1, -1
	m.setHScrollLocked(m.hscroll)
	m.clampScrollLocked()
}

// setActiveLocked переводит ввод в панель и шлёт OnActiveSideChange.
func (m *MergeView) setActiveLocked(side MergeSide) {
	if m.active == side {
		return
	}
	m.active = side
	if f := m.OnActiveSideChange; f != nil {
		m.emit(func() { f(side) })
	}
}

// paneShownLocked — верхняя панель есть в раскладке (у базы — SetShowBase).
func (m *MergeView) paneShownLocked(side MergeSide) bool {
	return side != MergeBase || m.showBase
}

// paneOpenLocked — панель видна целиком: есть в раскладке и не свёрнута.
func (m *MergeView) paneOpenLocked(side MergeSide) bool {
	if side == MergeResult {
		return true
	}
	return m.paneShownLocked(side) && !m.coll[side]
}

// visibleTopLocked — число развёрнутых верхних панелей.
func (m *MergeView) visibleTopLocked() int {
	n := 0
	for i := MergeOurs; i <= MergeTheirs; i++ {
		if m.paneOpenLocked(i) {
			n++
		}
	}
	return n
}

// ensureOpenTopLocked разворачивает «наше», если развёрнутых верхних панелей
// не осталось (свернули «наше» и «их», потом спрятали базу).
func (m *MergeView) ensureOpenTopLocked() {
	if m.visibleTopLocked() == 0 {
		m.coll[MergeOurs] = false
	}
}

// layoutPanes раскладывает верхние панели по ширине inner: свёрнутые —
// полосы, развёрнутые делят остаток поровну. Без свёрнутых — прежней
// формулой, кадры не меняются.
func (m *MergeView) layoutPanes(g *mvGeom, inner image.Rectangle) {
	shown, folded := 0, 0
	for i := MergeOurs; i <= MergeTheirs; i++ {
		if !m.paneShownLocked(i) {
			continue
		}
		shown++
		if m.coll[i] {
			folded++
		}
	}
	g.panes = shown
	open := max(shown-folded, 1)
	w := (inner.Dx() - mvGutter*(shown-1) - mvStripW*folded) / open
	x := inner.Min.X
	for i := MergeOurs; i <= MergeTheirs; i++ {
		g.fold[i] = false
		if !m.paneShownLocked(i) {
			g.px[i] = [2]int{0, 0}
			continue
		}
		pw := w
		if m.coll[i] {
			pw, g.fold[i] = mvStripW, true
		}
		g.px[i] = [2]int{x, x + pw}
		x += pw + mvGutter
	}
}

// splitYFor — y разделителя для раскладки: по доле или прижатый к шапкам.
func (m *MergeView) splitYFor(g *mvGeom, inner image.Rectangle, hbar int) int {
	switch m.maxMode {
	case mvMaxResult:
		// Верх — одна полоса шапок: содержимое верхних панелей нулевой высоты.
		return g.headerTopY + g.headerTopH + 6 + mvSplitH/2
	case mvMaxSources:
		// Итог — одна шапка (и полоса прокрутки под ней, если нужна).
		return inner.Max.Y - hbar - (mvSplitH/2 + 4 + g.headerResH + 6)
	}
	return inner.Min.Y + int(float64(inner.Dy())*m.split)
}

// stripRectLocked — полоса свёрнутой верхней панели (пусто — развёрнута).
func (m *MergeView) stripRectLocked(g mvGeom, side MergeSide) image.Rectangle {
	if side > MergeTheirs || !g.fold[side] {
		return image.Rectangle{}
	}
	return image.Rect(g.px[side][0], g.headerTopY, g.px[side][1], max(g.ty1, g.headerTopY+g.headerTopH))
}

// foldBtnRectLocked — кнопка «свернуть» в шапке развёрнутой верхней панели.
func (m *MergeView) foldBtnRectLocked(g mvGeom, side MergeSide) image.Rectangle {
	if !m.foldBtns || side > MergeTheirs || g.fold[side] || !m.paneShownLocked(side) {
		return image.Rectangle{}
	}
	x1 := g.px[side][1]
	y := g.headerTopY + (mvHeaderH-mvFoldBtnW)/2
	return image.Rect(x1-8-mvFoldBtnW, y, x1-8, y+mvFoldBtnW)
}

// maxBtnRectLocked — кнопки в шапке итога, у её правого края: «итог на всю
// высоту» (левая) и «верх на всю высоту» (правая). Счётчик конфликтов стоит
// левее них.
func (m *MergeView) maxBtnRectLocked(g mvGeom, mode int) image.Rectangle {
	if !m.foldBtns {
		return image.Rectangle{}
	}
	x1 := g.resultBounds().Max.X - 8
	if mode == mvMaxResult {
		x1 -= mvFoldBtnW + 4
	}
	y := g.headerResY + (mvHeaderH-mvFoldBtnW)/2
	return image.Rect(x1-mvFoldBtnW, y, x1, y+mvFoldBtnW)
}

// hitFoldLocked — что из элементов сворачивания под точкой: сторона (0…2) —
// её кнопка или полоса, mvHoverResultMax / mvHoverSourceMax — кнопки в шапке
// итога; -1 — мимо.
func (m *MergeView) hitFoldLocked(x, y int) int {
	g := m.geom()
	pt := image.Pt(x, y)
	for i := MergeOurs; i <= MergeTheirs; i++ {
		if pt.In(m.stripRectLocked(g, i)) || pt.In(m.foldBtnRectLocked(g, i)) {
			return int(i)
		}
	}
	if pt.In(m.maxBtnRectLocked(g, mvMaxResult)) {
		return mvHoverResultMax
	}
	if pt.In(m.maxBtnRectLocked(g, mvMaxSources)) {
		return mvHoverSourceMax
	}
	return -1
}

// foldPressLocked — щелчок по элементу сворачивания.
func (m *MergeView) foldPressLocked(hit int) {
	switch hit {
	case mvHoverResultMax:
		m.setMaxLocked(mvMaxResult, m.maxMode != mvMaxResult)
	case mvHoverSourceMax:
		m.setMaxLocked(mvMaxSources, m.maxMode != mvMaxSources)
	default:
		side := MergeSide(hit)
		m.setPaneCollapsedLocked(side, !m.coll[side])
	}
}

// splitPressLocked — нажатие на разделитель: двойной щелчок возвращает долю
// по умолчанию и снимает развёртывание, одинарный начинает перетаскивание.
func (m *MergeView) splitPressLocked() {
	now := time.Now()
	if now.Sub(m.splitClickAt) < 400*time.Millisecond {
		m.splitClickAt = time.Time{}
		m.setSplitLocked(mvSplitDefault)
		return
	}
	m.splitClickAt = now
	m.splitDrag = true
}

// drawStrip рисует свёрнутую верхнюю панель: полоса-карточка с меткой
// стороны и шевроном «развернуть».
func (m *MergeView) drawStrip(ctx DrawContext, g mvGeom, side MergeSide) {
	r := m.stripRectLocked(g, side)
	if r.Empty() {
		return
	}
	p := &m.pal
	fill := p.card
	if m.hoverFold == int(side) {
		fill = p.track
	}
	dvRoundRect(ctx, p, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), fill, p.cardEdge)
	cx := (r.Min.X + r.Max.X) / 2
	cy := r.Min.Y + mvHeaderH/2
	// Развернуть — в сторону остальных панелей: крайняя правая — влево.
	dir := 1
	if side == MergeTheirs || (side == MergeBase && m.coll[MergeTheirs]) {
		dir = -1
	}
	dvChevron(ctx, cx, cy, dir, p.text)
	if cy+18+6 <= r.Max.Y { // в полосу высотой с шапку (итог на всю высоту) метка не лезет
		fillEllipse(ctx, cx, cy+18, 4, 4, m.sideColor(side))
	}
}

// drawFoldButton — кнопка «свернуть» в шапке верхней панели.
func (m *MergeView) drawFoldButton(ctx DrawContext, g mvGeom, side MergeSide) {
	r := m.foldBtnRectLocked(g, side)
	if r.Empty() {
		return
	}
	p := &m.pal
	if m.hoverFold == int(side) {
		ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 4, p.track)
	}
	dir := -1
	if side == MergeTheirs {
		dir = 1
	}
	dvChevron(ctx, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, dir, p.muted)
}

// drawMaxButtons — кнопки «итог на всю высоту» и «верх на всю высоту» в шапке
// итога; включённая подсвечена.
func (m *MergeView) drawMaxButtons(ctx DrawContext, g mvGeom) {
	p := &m.pal
	for _, mode := range []int{mvMaxResult, mvMaxSources} {
		r := m.maxBtnRectLocked(g, mode)
		if r.Empty() {
			continue
		}
		hover := mvHoverResultMax
		if mode == mvMaxSources {
			hover = mvHoverSourceMax
		}
		fill, col := p.card, p.muted
		if m.maxMode == mode {
			fill, col = p.accent, p.card
		} else if m.hoverFold == hover {
			fill = p.track
		}
		ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 4, fill)
		ctx.DrawRoundBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 4, p.cardEdge)
		dir := -1 // итог вверх — шеврон «вверх»
		if mode == mvMaxSources {
			dir = 1
		}
		dvChevronV(ctx, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, dir, col)
	}
}
