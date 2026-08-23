// window_tabs.go — вкладки в заголовке окна (стиль Windows 11 Terminal).
//
// В отличие от TabControl (вкладки ВНУТРИ клиентской области) этот режим
// встраивает полосу вкладок прямо в заголовок Window: вкладки с «×»,
// кнопка «+» и системные кнопки окна делят одну полосу; свободное место
// заголовка остаётся drag-зоной.
//
// Отрисовка подстраивается под стиль активной темы:
//   - современные темы (Win10/Win11) — карточки вкладок, активная выделена
//     фоном клиентской области, у активной/наведённой рисуется «×»;
//   - классика Win2000 (Classic3D) — выпуклые bevel-ярлыки на градиенте
//     заголовка, активный «вдавлен»;
//   - Mac — вкладки после traffic lights, скруглённые, по центру текст.
//
// Содержимым вкладки управляет окно: Content активной вкладки — обычный
// ребёнок Window с bounds = ContentBounds() (ресайз работает штатно).
package widget

import (
	"image"
	"image/color"
	"sync"
)

// titleTabsState — состояние режима вкладок в заголовке.
type titleTabsState struct {
	mu     sync.Mutex
	tabs   []TabItem
	active int

	// hover-состояние (обновляется в OnMouseMove, читается в Draw).
	hoverIdx   int  // вкладка под курсором (-1 — нет)
	hoverClose int  // «×» какой вкладки под курсором (-1 — нет)
	hoverNew   bool // курсор над «+»

	// Геометрия последней отрисовки (для hit-теста).
	tabRects   []image.Rectangle
	closeRects []image.Rectangle // пустой Rect — «×» у вкладки не показан
	plusRect   image.Rectangle
	menuRect   image.Rectangle // кнопка «v» (меню заголовка)
	hoverMenu  bool

	// menu — выпадающее меню кнопки «v» (профили/команды, как в Terminal).
	menu *PopupMenu
}

// Геометрия полосы вкладок заголовка.
const (
	titleTabIconW   = 16 // размер иконки вкладки
	titleTabIconGap = 6  // зазор между иконкой и текстом
	titleTabMenuW   = 20 // ширина кнопки «v» (меню)
	titleTabMaxW   = 200 // максимальная ширина вкладки
	titleTabMinW   = 40  // ширина, ниже которой вкладки не сжимаются
	titleTabPadH   = 10  // горизонтальный padding текста
	titleTabCloseW = 16  // зона «×» внутри вкладки
	titleTabGap    = 2   // зазор между вкладками
	titleTabPlusW  = 24  // ширина кнопки «+»
)

// ─── Публичный API ──────────────────────────────────────────────────────────

// EnableTitleTabs включает режим вкладок в заголовке окна.
// Вкладки добавляются AddTitleTab; Content активной вкладки автоматически
// занимает клиентскую область окна. Текст Title при наличии вкладок
// не рисуется (полосу занимают вкладки).
func (w *Window) EnableTitleTabs() {
	if w.titleTabs == nil {
		w.titleTabs = &titleTabsState{active: -1, hoverIdx: -1, hoverClose: -1}
	}
}

// TitleTabsEnabled сообщает, включён ли режим вкладок в заголовке.
func (w *Window) TitleTabsEnabled() bool { return w.titleTabs != nil }

// titleTabsActive — режим включён и есть хотя бы одна вкладка.
func (w *Window) titleTabsActive() bool {
	if w.titleTabs == nil {
		return false
	}
	w.titleTabs.mu.Lock()
	defer w.titleTabs.mu.Unlock()
	return len(w.titleTabs.tabs) > 0
}

// AddTitleTab добавляет вкладку в заголовок и возвращает её индекс.
// Первая добавленная вкладка становится активной. Требует EnableTitleTabs.
func (w *Window) AddTitleTab(header string, content Widget) int {
	w.EnableTitleTabs()
	tt := w.titleTabs
	tt.mu.Lock()
	tt.tabs = append(tt.tabs, TabItem{Header: header, Content: content})
	idx := len(tt.tabs) - 1
	first := tt.active < 0
	tt.mu.Unlock()

	if content != nil && w.capMgr != nil {
		injectCaptureManagerTree(content, w.capMgr)
	}
	if first {
		w.SetActiveTitleTab(idx)
	} else {
		w.Invalidate()
	}
	return idx
}

// SetTitleTabsMenu прикрепляет к полосе вкладок кнопку «v» с выпадающим
// меню (профили/команды — как в Windows Terminal). Меню добавляется в
// дерево окна (Window.SetBounds его не растягивает — PopupMenu
// позиционируется собственным Show) и открывается кликом по шеврону
// под полосой заголовка. nil — убрать кнопку.
func (w *Window) SetTitleTabsMenu(pm *PopupMenu) {
	w.EnableTitleTabs()
	tt := w.titleTabs
	tt.mu.Lock()
	old := tt.menu
	tt.menu = pm
	tt.mu.Unlock()
	if old != nil && old != pm {
		w.RemoveChild(old)
	}
	if pm != nil && old != pm {
		w.AddChild(pm)
	}
	w.Invalidate()
}

// TitleTabsMenu возвращает прикреплённое меню кнопки «v» (nil — нет).
func (w *Window) TitleTabsMenu() *PopupMenu {
	if w.titleTabs == nil {
		return nil
	}
	w.titleTabs.mu.Lock()
	defer w.titleTabs.mu.Unlock()
	return w.titleTabs.menu
}

// TitleTabCount возвращает число вкладок заголовка.
func (w *Window) TitleTabCount() int {
	if w.titleTabs == nil {
		return 0
	}
	w.titleTabs.mu.Lock()
	defer w.titleTabs.mu.Unlock()
	return len(w.titleTabs.tabs)
}

// ActiveTitleTab возвращает индекс активной вкладки (-1 — вкладок нет).
func (w *Window) ActiveTitleTab() int {
	if w.titleTabs == nil {
		return -1
	}
	w.titleTabs.mu.Lock()
	defer w.titleTabs.mu.Unlock()
	return w.titleTabs.active
}

// SetActiveTitleTab делает вкладку idx активной: её Content становится
// ребёнком окна (bounds = ContentBounds), прежний Content снимается.
func (w *Window) SetActiveTitleTab(idx int) {
	tt := w.titleTabs
	if tt == nil {
		return
	}
	tt.mu.Lock()
	if idx < 0 || idx >= len(tt.tabs) {
		tt.mu.Unlock()
		return
	}
	prev := tt.active
	var prevContent, newContent Widget
	if prev >= 0 && prev < len(tt.tabs) {
		prevContent = tt.tabs[prev].Content
	}
	newContent = tt.tabs[idx].Content
	header := tt.tabs[idx].Header
	changed := prev != idx
	tt.active = idx
	tt.mu.Unlock()

	if changed && prevContent != nil {
		w.RemoveChild(prevContent)
	}
	if newContent != nil && (changed || prev < 0) {
		if w.capMgr != nil {
			injectCaptureManagerTree(newContent, w.capMgr)
		}
		newContent.SetBounds(w.ContentBounds())
		w.AddChild(newContent)
	}
	w.Invalidate()
	if changed && w.OnTitleTabChange != nil {
		w.OnTitleTabChange(idx, header)
	}
}

// CloseTitleTab закрывает вкладку idx: снимает её Content с окна и удаляет
// вкладку. Если закрыта активная — активной становится соседняя слева
// (или первая). После закрытия ПОСЛЕДНЕЙ вкладки вызывается OnClose окна
// (поведение Windows Terminal). После удаления вызывается OnTitleTabClosed.
func (w *Window) CloseTitleTab(idx int) {
	tt := w.titleTabs
	if tt == nil {
		return
	}
	tt.mu.Lock()
	if idx < 0 || idx >= len(tt.tabs) {
		tt.mu.Unlock()
		return
	}
	closed := tt.tabs[idx]
	wasActive := tt.active == idx
	tt.tabs = append(tt.tabs[:idx], tt.tabs[idx+1:]...)
	if tt.active > idx {
		tt.active--
	}
	next := tt.active
	if wasActive {
		next = idx - 1
		if next < 0 && len(tt.tabs) > 0 {
			next = 0
		}
		tt.active = -1 // переактивируем ниже через SetActiveTitleTab
	}
	empty := len(tt.tabs) == 0
	tt.hoverIdx, tt.hoverClose = -1, -1
	tt.mu.Unlock()

	if closed.Content != nil {
		w.RemoveChild(closed.Content)
	}
	if wasActive && !empty {
		w.SetActiveTitleTab(next)
	}
	w.Invalidate()
	if w.OnTitleTabClosed != nil {
		w.OnTitleTabClosed(idx, closed.Header)
	}
	if empty && w.OnClose != nil {
		w.OnClose()
	}
}

// SetTitleTabHeader меняет заголовок вкладки idx.
func (w *Window) SetTitleTabHeader(idx int, header string) {
	tt := w.titleTabs
	if tt == nil {
		return
	}
	tt.mu.Lock()
	changed := false
	if idx >= 0 && idx < len(tt.tabs) && tt.tabs[idx].Header != header {
		tt.tabs[idx].Header = header
		changed = true
	}
	tt.mu.Unlock()
	if changed {
		w.Invalidate()
	}
}

// TitleTabHeader возвращает заголовок вкладки idx ("" вне диапазона).
func (w *Window) TitleTabHeader(idx int) string {
	tt := w.titleTabs
	if tt == nil {
		return ""
	}
	tt.mu.Lock()
	defer tt.mu.Unlock()
	if idx >= 0 && idx < len(tt.tabs) {
		return tt.tabs[idx].Header
	}
	return ""
}

// SetTitleTabIcon задаёт иконку вкладки idx (рисуется слева от заголовка).
func (w *Window) SetTitleTabIcon(idx int, img image.Image) {
	tt := w.titleTabs
	if tt == nil {
		return
	}
	tt.mu.Lock()
	if idx >= 0 && idx < len(tt.tabs) {
		tt.tabs[idx].Icon = img
	}
	tt.mu.Unlock()
	w.Invalidate()
}

// SetTitleTabToolTip задаёт подсказку заголовка вкладки idx.
func (w *Window) SetTitleTabToolTip(idx int, tip string) {
	tt := w.titleTabs
	if tt == nil {
		return
	}
	tt.mu.Lock()
	if idx >= 0 && idx < len(tt.tabs) {
		tt.tabs[idx].ToolTip = tip
	}
	tt.mu.Unlock()
}

// GetToolTip возвращает подсказку вкладки заголовка под курсором (если
// задана), иначе — общий ToolTip окна.
func (w *Window) GetToolTip() string {
	if tt := w.titleTabs; tt != nil {
		tt.mu.Lock()
		if tt.hoverIdx >= 0 && tt.hoverIdx < len(tt.tabs) {
			if tip := tt.tabs[tt.hoverIdx].ToolTip; tip != "" {
				tt.mu.Unlock()
				return tip
			}
		}
		tt.mu.Unlock()
	}
	return w.ToolTip
}

// ─── Геометрия и отрисовка ──────────────────────────────────────────────────

// drawTitleTabs рисует полосу вкладок в заголовке между left и right
// (границы уже учитывают кнопки окна/бейдж локали/traffic lights).
// Вызывается из drawWinTitleBar / drawMacTitleBar вместо текста Title.
func (w *Window) drawTitleTabs(ctx DrawContext, left, right, y, th int) {
	tt := w.titleTabs
	st := currentStyle()

	tt.mu.Lock()
	tabs := make([]TabItem, len(tt.tabs))
	copy(tabs, tt.tabs)
	active := tt.active
	hoverIdx := tt.hoverIdx
	hoverClose := tt.hoverClose
	hoverNew := tt.hoverNew
	tt.mu.Unlock()

	tt.mu.Lock()
	menu := tt.menu
	hoverMenu := tt.hoverMenu
	tt.mu.Unlock()

	showPlus := w.OnTitleTabNew != nil
	showMenu := menu != nil
	avail := right - left
	if showPlus {
		avail -= titleTabPlusW + titleTabGap
	}
	if showMenu {
		avail -= titleTabMenuW + titleTabGap
	}
	if avail < titleTabMinW || len(tabs) == 0 {
		tt.mu.Lock()
		tt.tabRects, tt.closeRects = nil, nil
		tt.plusRect, tt.menuRect = image.Rectangle{}, image.Rectangle{}
		tt.mu.Unlock()
		return
	}

	// Ширины: текст + паддинги + «×», кламп [titleTabMinW, titleTabMaxW];
	// при переполнении — равномерное сжатие.
	widths := make([]int, len(tabs))
	total := 0
	for i, tab := range tabs {
		tw := ctx.MeasureText(tab.Header, DefaultFontSizePt) + titleTabPadH*2 + titleTabCloseW
		if tab.Icon != nil {
			tw += titleTabIconW + titleTabIconGap
		}
		if tw > titleTabMaxW {
			tw = titleTabMaxW
		}
		if tw < titleTabMinW {
			tw = titleTabMinW
		}
		widths[i] = tw
		total += tw + titleTabGap
	}
	if total > avail {
		k := float64(avail) / float64(total)
		for i := range widths {
			widths[i] = int(float64(widths[i]) * k)
			if widths[i] < titleTabMinW {
				widths[i] = titleTabMinW
			}
		}
	}

	// Вертикальная геометрия по стилю: карточка занимает большую часть
	// полосы с полями по 5px (в Terminal ~30px карточка на ~40px полосе).
	tabTop, tabBot := y+5, y+th-5
	if st.Classic3D {
		tabTop, tabBot = y+2, y+th-2 // bevel-ярлыки внутри градиента
	}

	tabRects := make([]image.Rectangle, len(tabs))
	closeRects := make([]image.Rectangle, len(tabs))
	_, _, tc := w.titleColors()

	x := left
	for i, tab := range tabs {
		if x+widths[i] > right {
			break // не влезающие вкладки не рисуем (rect остаётся пустым)
		}
		r := image.Rect(x, tabTop, x+widths[i], tabBot)
		tabRects[i] = r

		// «×» показываем у активной и наведённой вкладки (если влезает).
		showClose := (i == active || i == hoverIdx) && widths[i] >= titleTabMinW+titleTabCloseW
		if showClose {
			closeRects[i] = image.Rect(r.Max.X-titleTabCloseW-2, r.Min.Y+(r.Dy()-titleTabCloseW)/2,
				r.Max.X-2, r.Min.Y+(r.Dy()+titleTabCloseW)/2)
		}

		switch {
		case st.Classic3D:
			w.drawClassicTitleTab(ctx, r, tab, i == active, closeRects[i], hoverClose == i)
		case w.resolvedTitleStyle() == WindowTitleMac:
			w.drawMacTitleTab(ctx, r, tab, i == active, i == hoverIdx, tc, closeRects[i], hoverClose == i)
		default:
			w.drawModernTitleTab(ctx, r, tab, i == active, i == hoverIdx, tc, closeRects[i], hoverClose == i)
		}
		x += widths[i] + titleTabGap
	}

	// Кнопки «+» и «v» — одинаковой высоты (чуть ниже карточки вкладки),
	// по общему вертикальному центру полосы, с отступом от последней вкладки
	// (как блок кнопок в Windows Terminal).
	btnH := (tabBot - tabTop) - 4
	if btnH < 14 {
		btnH = tabBot - tabTop
	}
	btnTop := tabTop + (tabBot-tabTop-btnH)/2
	x += 4 // зазор между вкладками и блоком кнопок

	// Геометрия обеих кнопок считается заранее — фон рисуется ОДНОЙ общей
	// пилюлей на блок (как сегментная кнопка +/v в Windows Terminal),
	// наведённая половинка подсвечивается внутри.
	var plusRect, menuRect image.Rectangle
	if showPlus && x+titleTabPlusW <= right {
		plusRect = image.Rect(x, btnTop, x+titleTabPlusW, btnTop+btnH)
		x = plusRect.Max.X + titleTabGap
	}
	if showMenu && x+titleTabMenuW <= right {
		menuRect = image.Rect(x, btnTop, x+titleTabMenuW, btnTop+btnH)
	}

	if !plusRect.Empty() || !menuRect.Empty() {
		tbg, _, _ := w.titleColors()
		glyph := tc
		if st.Classic3D {
			// Классика: общий блок не рисуем, hover — выпуклый bevel.
			glyph = color.RGBA{A: 255}
			if hoverNew && !plusRect.Empty() {
				drawBevelRaised(ctx, plusRect.Min.X, plusRect.Min.Y, plusRect.Dx(), plusRect.Dy(), st)
			}
			if hoverMenu && !menuRect.Empty() {
				drawBevelRaised(ctx, menuRect.Min.X, menuRect.Min.Y, menuRect.Dx(), menuRect.Dy(), st)
			}
		} else {
			block := plusRect.Union(menuRect)
			ctx.FillRoundRect(block.Min.X, block.Min.Y, block.Dx(), block.Dy(), 4,
				mixRGBA(tbg, tc, 0.06))
			hl := mixRGBA(tbg, tc, 0.14)
			if hoverNew && !plusRect.Empty() {
				ctx.FillRoundRect(plusRect.Min.X+1, plusRect.Min.Y+1, plusRect.Dx()-2, plusRect.Dy()-2, 3, hl)
			}
			if hoverMenu && !menuRect.Empty() {
				ctx.FillRoundRect(menuRect.Min.X+1, menuRect.Min.Y+1, menuRect.Dx()-2, menuRect.Dy()-2, 3, hl)
			}
		}

		if !plusRect.Empty() {
			pcx, pcy := (plusRect.Min.X+plusRect.Max.X)/2, (plusRect.Min.Y+plusRect.Max.Y)/2
			ctx.DrawHLine(pcx-4, pcy, 9, glyph)
			ctx.DrawVLine(pcx, pcy-4, 9, glyph)
		}
		if !menuRect.Empty() {
			mcx, mcy := (menuRect.Min.X+menuRect.Max.X)/2, (menuRect.Min.Y+menuRect.Max.Y)/2-1
			for i := 0; i <= 3; i++ {
				ctx.SetPixel(mcx-3+i, mcy+i, glyph)
				ctx.SetPixel(mcx-2+i, mcy+i, glyph)
				ctx.SetPixel(mcx+3-i, mcy+i, glyph)
				ctx.SetPixel(mcx+2-i, mcy+i, glyph)
			}
		}
	}

	tt.mu.Lock()
	tt.tabRects, tt.closeRects, tt.plusRect, tt.menuRect = tabRects, closeRects, plusRect, menuRect
	tt.mu.Unlock()
}

// drawModernTitleTab — карточка вкладки Win10/Win11 в духе Windows Terminal:
// активная — скруглённая карточка чуть светлее полосы заголовка (в светлых
// темах — чуть темнее) с тонкой рамкой, неактивные — приглушённый текст,
// наведённая — лёгкая подсветка. Цвета выводятся из цветов заголовка, а не
// из TabControl: там «активный» фон совпадает с фоном окна и на полосе
// заголовка был бы невидим.
func (w *Window) drawModernTitleTab(ctx DrawContext, r image.Rectangle, tab TabItem,
	active, hover bool, tc color.RGBA, closeR image.Rectangle, hoverClose bool) {

	tbg, _, _ := w.titleColors()
	cardBG := mixRGBA(tbg, tc, 0.10)   // карточка активной: сдвиг к цвету текста
	cardBorder := mixRGBA(tbg, tc, 0.22)
	hoverBG := mixRGBA(tbg, tc, 0.05)

	switch {
	case active:
		ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 6, cardBG)
		ctx.DrawRoundBorder(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 6, cardBorder)
	case hover:
		ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 6, hoverBG)
	}

	textCol := tc
	if !active {
		textCol = mixRGBA(tc, tbg, 0.4) // неактивная — приглушённый текст
	}
	textX := r.Min.X + titleTabPadH
	if tab.Icon != nil {
		ctx.DrawImageScaled(tab.Icon, textX, r.Min.Y+(r.Dy()-titleTabIconW)/2,
			titleTabIconW, titleTabIconW)
		textX += titleTabIconW + titleTabIconGap
	}
	maxW := r.Max.X - textX - titleTabPadH
	if !closeR.Empty() {
		maxW = closeR.Min.X - textX - 4
	}
	if maxW > 0 {
		txt := ellipsizeText(ctx, tab.Header, maxW, DefaultFontSizePt)
		ctx.DrawText(txt, textX, r.Min.Y+(r.Dy()-13)/2, textCol)
	}
	w.drawTabClose(ctx, closeR, hoverClose, textCol, false)
}

// drawClassicTitleTab — bevel-ярлык Win2000 на градиенте заголовка:
// неактивный выпуклый, активный «вдавлен».
func (w *Window) drawClassicTitleTab(ctx DrawContext, r image.Rectangle, tab TabItem,
	active bool, closeR image.Rectangle, hoverClose bool) {

	st := currentStyle()
	ctx.FillRect(r.Min.X+1, r.Min.Y+1, r.Dx()-2, r.Dy()-2, win10.BtnBG)
	if active {
		drawBevelSunken(ctx, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), st)
	} else {
		drawBevelRaised(ctx, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), st)
	}
	off := 0
	if active {
		off = 1 // вдавленный ярлык — контент смещён, как у нажатой кнопки
	}
	textX := r.Min.X + titleTabPadH + off
	if tab.Icon != nil {
		ctx.DrawImageScaled(tab.Icon, textX, r.Min.Y+(r.Dy()-titleTabIconW)/2+off,
			titleTabIconW, titleTabIconW)
		textX += titleTabIconW + titleTabIconGap
	}
	maxW := r.Max.X - textX - titleTabPadH
	if !closeR.Empty() {
		maxW = closeR.Min.X - textX - 4
	}
	if maxW > 0 {
		txt := ellipsizeText(ctx, tab.Header, maxW, DefaultFontSizePt)
		ctx.DrawText(txt, textX, r.Min.Y+(r.Dy()-13)/2+off, win10.BtnText)
	}
	w.drawTabClose(ctx, closeR, hoverClose, win10.BtnText, true)
}

// drawMacTitleTab — вкладка в Mac-стиле: скруглённая пилюля, текст по центру.
func (w *Window) drawMacTitleTab(ctx DrawContext, r image.Rectangle, tab TabItem,
	active, hover bool, tc color.RGBA, closeR image.Rectangle, hoverClose bool) {

	switch {
	case active:
		ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 6, mixRGBA(w.Background, tc, 0.12))
	case hover:
		ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 6, color.RGBA{R: 128, G: 128, B: 128, A: 50})
	}
	iconW := 0
	if tab.Icon != nil {
		iconW = titleTabIconW + titleTabIconGap
	}
	maxW := r.Dx() - titleTabPadH*2 - iconW
	if !closeR.Empty() {
		maxW -= titleTabCloseW
	}
	if maxW > 0 {
		txt := ellipsizeText(ctx, tab.Header, maxW, 10)
		txtW := ctx.MeasureText(txt, 10)
		tx := r.Min.X + (r.Dx()-iconW-txtW)/2 + iconW
		if tx < r.Min.X+titleTabPadH+iconW {
			tx = r.Min.X + titleTabPadH + iconW
		}
		if tab.Icon != nil {
			ctx.DrawImageScaled(tab.Icon, tx-iconW, r.Min.Y+(r.Dy()-titleTabIconW)/2,
				titleTabIconW, titleTabIconW)
		}
		ctx.DrawText(txt, tx, r.Min.Y+(r.Dy()-13)/2, tc)
	}
	w.drawTabClose(ctx, closeR, hoverClose, tc, false)
}

// drawTabClose рисует «×» вкладки (если rect не пуст).
func (w *Window) drawTabClose(ctx DrawContext, r image.Rectangle, hover bool,
	col color.RGBA, classic bool) {

	if r.Empty() {
		return
	}
	if hover {
		if classic {
			ctx.FillRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), win10.BtnHoverBG)
		} else {
			ctx.FillRoundRect(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), 3,
				color.RGBA{R: 128, G: 128, B: 128, A: 90})
		}
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	for i := -3; i <= 3; i++ {
		ctx.SetPixel(cx+i, cy+i, col)
		ctx.SetPixel(cx+i, cy-i, col)
	}
}

// ─── Hit-тест и ввод ────────────────────────────────────────────────────────

// titleTabHitZone — точка попадает в интерактивный элемент полосы вкладок
// (вкладку, её «×» или «+»). Такие клики не должны начинать drag/захват.
func (w *Window) titleTabHitZone(pt image.Point) bool {
	tt := w.titleTabs
	if tt == nil {
		return false
	}
	tt.mu.Lock()
	defer tt.mu.Unlock()
	if !tt.plusRect.Empty() && pt.In(tt.plusRect) {
		return true
	}
	if !tt.menuRect.Empty() && pt.In(tt.menuRect) {
		return true
	}
	for _, r := range tt.tabRects {
		if !r.Empty() && pt.In(r) {
			return true
		}
	}
	return false
}

// titleTabsMouseDown обрабатывает нажатие ЛКМ в полосе вкладок.
// Возвращает true, если клик поглощён (вкладка/«×»/«+»).
func (w *Window) titleTabsMouseDown(pt image.Point) bool {
	tt := w.titleTabs
	if tt == nil {
		return false
	}
	tt.mu.Lock()
	clickTab, clickClose := -1, -1
	if !tt.plusRect.Empty() && pt.In(tt.plusRect) {
		tt.mu.Unlock()
		if w.OnTitleTabNew != nil {
			w.OnTitleTabNew()
		}
		return true
	}
	if !tt.menuRect.Empty() && pt.In(tt.menuRect) {
		menu := tt.menu
		mx, my := tt.menuRect.Min.X, tt.menuRect.Max.Y+4
		tt.mu.Unlock()
		if menu != nil {
			if menu.IsOpen() {
				menu.Close()
			} else {
				menu.Show(mx, my)
			}
		}
		return true
	}
	for i, r := range tt.tabRects {
		if r.Empty() || !pt.In(r) {
			continue
		}
		if i < len(tt.closeRects) && !tt.closeRects[i].Empty() && pt.In(tt.closeRects[i]) {
			clickClose = i
		} else {
			clickTab = i
		}
		break
	}
	tt.mu.Unlock()

	switch {
	case clickClose >= 0:
		w.CloseTitleTab(clickClose)
		return true
	case clickTab >= 0:
		w.SetActiveTitleTab(clickTab)
		return true
	}
	return false
}

// titleTabsMouseMove обновляет hover-состояние; true — состояние изменилось.
func (w *Window) titleTabsMouseMove(x, y int) bool {
	tt := w.titleTabs
	if tt == nil {
		return false
	}
	pt := image.Pt(x, y)
	tt.mu.Lock()
	defer tt.mu.Unlock()

	newIdx, newClose, newPlus := -1, -1, false
	newMenu := !tt.menuRect.Empty() && pt.In(tt.menuRect)
	if !tt.plusRect.Empty() && pt.In(tt.plusRect) {
		newPlus = true
	}
	for i, r := range tt.tabRects {
		if r.Empty() || !pt.In(r) {
			continue
		}
		newIdx = i
		if i < len(tt.closeRects) && !tt.closeRects[i].Empty() && pt.In(tt.closeRects[i]) {
			newClose = i
		}
		break
	}
	changed := newIdx != tt.hoverIdx || newClose != tt.hoverClose ||
		newPlus != tt.hoverNew || newMenu != tt.hoverMenu
	tt.hoverIdx, tt.hoverClose, tt.hoverNew, tt.hoverMenu = newIdx, newClose, newPlus, newMenu
	return changed
}
