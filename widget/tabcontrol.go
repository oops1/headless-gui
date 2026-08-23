package widget

import (
	"image"
	"image/color"
	"sync"
)

// TabItem описывает одну вкладку: заголовок + контент (Widget-дерево).
type TabItem struct {
	Header  string
	Content Widget // корневой виджет содержимого вкладки
	Hidden  bool   // true → вкладка скрыта из полосы заголовков (SetTabVisible)

	// Icon — иконка вкладки (рисуется слева от заголовка, 16×16).
	// Пока используется вкладками заголовка окна (TitleTabs).
	Icon image.Image

	// ToolTip — подсказка при наведении на заголовок вкладки.
	ToolTip string
	// SeparatorBefore — визуальный разделитель перед вкладкой
	// (группировка вкладок в полосе).
	SeparatorBefore bool
}

// tabSepW — ширина зазора-разделителя между группами вкладок.
const tabSepW = 9

// TabControl — виджет с вкладками в стиле Windows 10.
//
// Вкладки рисуются горизонтально сверху. Под ними — область содержимого,
// в которой показывается Content активной вкладки.
type TabControl struct {
	Base

	TabBG         color.RGBA
	TabActiveBG   color.RGBA
	TabBorder     color.RGBA
	TabText       color.RGBA
	TabActiveText color.RGBA
	ContentBG     color.RGBA
	AccentColor   color.RGBA

	TabHeight int // высота полосы вкладок (по умолчанию 32)
	TabPadH   int // горизонтальный padding текста вкладки

	mu        sync.Mutex
	tabs      []TabItem
	active    int   // индекс активной вкладки
	hoverIdx  int   // индекс вкладки под курсором
	tabWidths []int // реальные ширины вкладок (обновляются в Draw)
	capMgr    CaptureManager // для инжекции в контент всех вкладок

	OnTabChange func(index int, header string)
}

// NewTabControl создаёт TabControl с заданными вкладками.
func NewTabControl(tabs ...TabItem) *TabControl {
	return &TabControl{
		TabBG:         win10.TabBG,
		TabActiveBG:   win10.TabActiveBG,
		TabBorder:     win10.TabBorder,
		TabText:       win10.TabText,
		TabActiveText: win10.TabActiveText,
		ContentBG:     win10.TabContentBG,
		AccentColor:   win10.Accent,
		TabHeight:     32,
		TabPadH:       16,
		tabs:          tabs,
		active:        0,
		hoverIdx:      -1,
	}
}

// Children возвращает содержимое активной вкладки как дочерний виджет.
// Это переопределение Base.Children() необходимо, чтобы движок мог
// выполнять hit-test и доставлять события (мышь, клавиатура) до виджетов
// внутри вкладки.
func (tc *TabControl) Children() []Widget {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if tc.active >= 0 && tc.active < len(tc.tabs) {
		if c := tc.tabs[tc.active].Content; c != nil {
			return []Widget{c}
		}
	}
	return nil
}

// AddTab добавляет вкладку.
func (tc *TabControl) AddTab(header string, content Widget) {
	tc.mu.Lock()
	cm := tc.capMgr
	tc.tabs = append(tc.tabs, TabItem{Header: header, Content: content})
	tc.mu.Unlock()
	// Если CaptureManager уже инжектирован — раздаём его новому контенту.
	if cm != nil && content != nil {
		injectCaptureManagerTree(content, cm)
	}
	tc.Invalidate() // в полосе появился новый заголовок
}

// SetTabHeader меняет заголовок вкладки idx в рантайме (BUG-4).
// Удобно для динамических счётчиков, напр. "CARRY (3)".
func (tc *TabControl) SetTabHeader(idx int, header string) {
	tc.mu.Lock()
	changed := false
	if idx >= 0 && idx < len(tc.tabs) && tc.tabs[idx].Header != header {
		tc.tabs[idx].Header = header
		changed = true
	}
	tc.mu.Unlock()
	if changed {
		tc.Invalidate()
	}
}

// TabHeader возвращает текущий заголовок вкладки idx (или "" вне диапазона).
func (tc *TabControl) TabHeader(idx int) string {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if idx >= 0 && idx < len(tc.tabs) {
		return tc.tabs[idx].Header
	}
	return ""
}

// SetTabToolTip задаёт подсказку заголовка вкладки.
func (tc *TabControl) SetTabToolTip(idx int, tip string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if idx >= 0 && idx < len(tc.tabs) {
		tc.tabs[idx].ToolTip = tip
	}
}

// SetTabSeparator включает/выключает разделитель перед вкладкой.
func (tc *TabControl) SetTabSeparator(idx int, sep bool) {
	tc.mu.Lock()
	changed := false
	if idx >= 0 && idx < len(tc.tabs) && tc.tabs[idx].SeparatorBefore != sep {
		tc.tabs[idx].SeparatorBefore = sep
		changed = true
	}
	tc.mu.Unlock()
	if changed {
		tc.Invalidate()
	}
}

// GetToolTip возвращает подсказку вкладки под курсором (если задана),
// иначе — общий ToolTip виджета. Курсор отслеживается через hoverIdx.
func (tc *TabControl) GetToolTip() string {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if tc.hoverIdx >= 0 && tc.hoverIdx < len(tc.tabs) {
		if tip := tc.tabs[tc.hoverIdx].ToolTip; tip != "" {
			return tip
		}
	}
	return tc.ToolTip
}

// TabContent возвращает контент вкладки idx (или nil вне диапазона).
func (tc *TabControl) TabContent(idx int) Widget {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if idx >= 0 && idx < len(tc.tabs) {
		return tc.tabs[idx].Content
	}
	return nil
}

// SetTabVisible показывает/скрывает вкладку idx в полосе заголовков (BUG-4).
// Скрытая вкладка не рисуется и не реагирует на клики. Если скрывается
// активная вкладка — активной становится первая видимая (если есть).
func (tc *TabControl) SetTabVisible(idx int, visible bool) {
	tc.mu.Lock()
	if idx < 0 || idx >= len(tc.tabs) {
		tc.mu.Unlock()
		return
	}
	changed := tc.tabs[idx].Hidden == visible // состояние фактически меняется
	tc.tabs[idx].Hidden = !visible
	// Если скрыли активную — переключаемся на первую видимую.
	switchTo := -1
	if !visible && tc.active == idx {
		for i := range tc.tabs {
			if !tc.tabs[i].Hidden {
				switchTo = i
				break
			}
		}
	}
	if switchTo >= 0 {
		tc.active = switchTo
	}
	tc.mu.Unlock()
	tc.layoutContent()
	if changed {
		tc.Invalidate()
	}
}

// IsTabVisible сообщает, видима ли вкладка idx (по умолчанию true).
func (tc *TabControl) IsTabVisible(idx int) bool {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if idx >= 0 && idx < len(tc.tabs) {
		return !tc.tabs[idx].Hidden
	}
	return false
}

// RemoveTab удаляет вкладку idx (BUG-4). Активная вкладка корректируется,
// чтобы оставаться в допустимом диапазоне.
func (tc *TabControl) RemoveTab(idx int) {
	tc.mu.Lock()
	if idx < 0 || idx >= len(tc.tabs) {
		tc.mu.Unlock()
		return
	}
	tc.tabs = append(tc.tabs[:idx], tc.tabs[idx+1:]...)
	if tc.active >= len(tc.tabs) {
		tc.active = len(tc.tabs) - 1
	}
	if tc.active < 0 {
		tc.active = 0
	}
	tc.mu.Unlock()
	tc.layoutContent()
	tc.Invalidate() // вкладка удалена — полоса и контент изменились
}

// ClearTabs удаляет все вкладки (BUG-4).
func (tc *TabControl) ClearTabs() {
	tc.mu.Lock()
	changed := len(tc.tabs) > 0
	tc.tabs = nil
	tc.active = 0
	tc.tabWidths = nil
	tc.mu.Unlock()
	if changed {
		tc.Invalidate()
	}
}

// SetActive устанавливает активную вкладку по индексу.
func (tc *TabControl) SetActive(idx int) {
	tc.mu.Lock()
	changed := false
	if idx >= 0 && idx < len(tc.tabs) && idx != tc.active {
		tc.active = idx
		changed = true
	}
	tc.mu.Unlock()
	tc.layoutContent()
	if changed {
		tc.Invalidate()
	}
}

// Active возвращает индекс активной вкладки.
func (tc *TabControl) Active() int {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.active
}

// TabCount возвращает количество вкладок.
func (tc *TabControl) TabCount() int {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return len(tc.tabs)
}

// SetBounds устанавливает границы TabControl и обновляет bounds активного контента.
func (tc *TabControl) SetBounds(r image.Rectangle) {
	tc.bounds = r
	tc.layoutContent()
}

// layoutContent обновляет bounds содержимого активной вкладки.
//
// Для контейнеров с собственной раскладкой (Grid, Canvas, …) достаточно
// SetBounds — они сами пересчитают потомков. Для контента без своей
// раскладки (Panel) нужно дополнительно сдвинуть потомков на дельту, иначе
// они останутся на старых абсолютных координатах (актуально, когда TabControl
// как потомок Grid получает финальные bounds позже момента сборки контента —
// см. BUG-1).
func (tc *TabControl) layoutContent() {
	cr := tc.contentRect()
	tc.mu.Lock()
	var c Widget
	if tc.active >= 0 && tc.active < len(tc.tabs) {
		c = tc.tabs[tc.active].Content
	}
	tc.mu.Unlock()
	if c == nil {
		return
	}
	old := c.Bounds()
	c.SetBounds(cr)
	if !HasOwnLayout(c) && !old.Empty() {
		dx := cr.Min.X - old.Min.X
		dy := cr.Min.Y - old.Min.Y
		if dx != 0 || dy != 0 {
			shiftDescendants(c, dx, dy)
		}
	}
}

// tabRects вычисляет прямоугольники заголовков вкладок.
// Вызывать под мьютексом.
func (tc *TabControl) tabRects(ctx DrawContext) []image.Rectangle {
	b := tc.bounds
	rects := make([]image.Rectangle, len(tc.tabs))
	x := b.Min.X
	for i, tab := range tc.tabs {
		textW := ctx.MeasureText(tab.Header, DefaultFontSizePt)
		tabW := textW + tc.TabPadH*2
		rects[i] = image.Rect(x, b.Min.Y, x+tabW, b.Min.Y+tc.TabHeight)
		x += tabW
	}
	return rects
}

// contentRect возвращает прямоугольник области содержимого.
func (tc *TabControl) contentRect() image.Rectangle {
	b := tc.bounds
	return image.Rect(b.Min.X, b.Min.Y+tc.TabHeight, b.Max.X, b.Max.Y)
}

// Draw рисует TabControl: полосу вкладок + содержимое активной.
func (tc *TabControl) Draw(ctx DrawContext) {
	b := tc.bounds
	if b.Empty() {
		return
	}
	tc.mu.Lock()
	tabs := tc.tabs
	active := tc.active
	hoverIdx := tc.hoverIdx
	tc.mu.Unlock()

	if len(tabs) == 0 {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), tc.ContentBG)
		return
	}

	st := currentStyle()

	// classicTabH — высота компактного ярлыка классики Win2000 (активный;
	// неактивные на 2px ниже). Ярлыки прижаты к странице, остаток полосы пуст.
	const classicTabH = 21

	// Полоса вкладок — фон. В классике ярлыки лежат прямо на «лице»
	// (отдельной полосы нет — как в System Properties Win2000).
	if st.Classic3D {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), tc.TabHeight, tc.ContentBG)
	} else {
		ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), tc.TabHeight, tc.TabBG)
	}

	// Рисуем каждую вкладку и сохраняем реальные ширины для hit-test.
	widths := make([]int, len(tabs))
	var activeRect image.Rectangle
	activeHeader := ""
	tabX := b.Min.X
	seenTab := false
	for i, tab := range tabs {
		// Скрытые вкладки не занимают места в полосе заголовков (ширина 0).
		if tab.Hidden {
			widths[i] = 0
			continue
		}
		// Разделитель группы вкладок (не перед первой видимой).
		if tab.SeparatorBefore && seenTab {
			if !st.Classic3D {
				ctx.DrawVLine(tabX+tabSepW/2, b.Min.Y+6, tc.TabHeight-12, tc.TabBorder)
			}
			tabX += tabSepW
		}
		seenTab = true
		textW := ctx.MeasureText(tab.Header, DefaultFontSizePt)
		tabW := textW + tc.TabPadH*2
		widths[i] = tabW
		tabRect := image.Rect(tabX, b.Min.Y, tabX+tabW, b.Min.Y+tc.TabHeight)

		// Фон вкладки
		switch {
		case st.Classic3D:
			// Классика Win2000: ярлыки КОМПАКТНЫЕ (~21px) и прижаты к
			// странице — верх полосы остаётся пустым «лицом». Активный
			// рисуется ПОЗЖЕ — поверх рамки страницы (шире и выше соседей).
			pageTop := b.Min.Y + tc.TabHeight
			top := pageTop - classicTabH // верх активного ярлыка
			if i == active {
				activeRect = image.Rect(tabRect.Min.X, top, tabRect.Max.X, pageTop)
				activeHeader = tab.Header
				tabX += tabW
				continue
			}
			tr2 := image.Rect(tabRect.Min.X, top+2, tabRect.Max.X, pageTop)
			ctx.FillRect(tr2.Min.X, tr2.Min.Y+1, tr2.Dx(), tr2.Dy()-1, tc.TabActiveBG)
			// Верхняя грань со «срезанными» углами (имитация скругления).
			ctx.DrawHLine(tr2.Min.X+2, tr2.Min.Y, tr2.Dx()-4, st.BevelLight)
			ctx.SetPixel(tr2.Min.X+1, tr2.Min.Y+1, st.BevelLight)
			ctx.SetPixel(tr2.Max.X-2, tr2.Min.Y+1, st.BevelDark)
			// Левая светлая грань — только у первого ярлыка: примыкающие
			// разделяет одна тёмная грань соседа (как в оригинале).
			if i == 0 {
				ctx.DrawVLine(tr2.Min.X, tr2.Min.Y+2, tr2.Dy()-2, st.BevelLight)
			}
			// Правая грань — одна тёмная линия.
			ctx.DrawVLine(tr2.Max.X-1, tr2.Min.Y+2, tr2.Dy()-2, st.BevelShadow)
			// Текст неактивного ярлыка — по центру компактного ярлыка.
			ctx.DrawText(tab.Header, tabRect.Min.X+tc.TabPadH,
				tr2.Min.Y+(tr2.Dy()-13)/2, tc.TabText)
			tabX += tabW
			continue
		case i == active:
			ctx.FillRect(tabRect.Min.X, tabRect.Min.Y, tabRect.Dx(), tabRect.Dy(), tc.TabActiveBG)
			// Акцентная линия сверху
			ctx.DrawHLine(tabRect.Min.X, tabRect.Min.Y, tabRect.Dx(), tc.AccentColor)
			ctx.DrawHLine(tabRect.Min.X, tabRect.Min.Y+1, tabRect.Dx(), tc.AccentColor)
		case i == hoverIdx:
			bg := tc.TabBG
			// Слегка светлее при наведении
			bg.R = clampAdd(bg.R, 15)
			bg.G = clampAdd(bg.G, 15)
			bg.B = clampAdd(bg.B, 15)
			ctx.FillRect(tabRect.Min.X, tabRect.Min.Y, tabRect.Dx(), tabRect.Dy(), bg)
		}

		// Текст вкладки
		textColor := tc.TabText
		if i == active {
			textColor = tc.TabActiveText
		}
		textX := tabRect.Min.X + tc.TabPadH
		textY := tabRect.Min.Y + (tc.TabHeight-13)/2
		ctx.DrawText(tab.Header, textX, textY, textColor)

		// Разделитель между вкладками (в классике грани ярлыков сами разделяют).
		if !st.Classic3D && i < len(tabs)-1 {
			ctx.DrawVLine(tabRect.Max.X-1, tabRect.Min.Y+4, tabRect.Dy()-8, tc.TabBorder)
		}

		tabX += tabW
	}

	// Сохраняем реальные ширины для hit-test в OnMouseButton/OnMouseMove.
	tc.mu.Lock()
	tc.tabWidths = widths
	tc.mu.Unlock()

	// Линия под вкладками (в классике её роль играет верхняя грань страницы).
	if !st.Classic3D {
		ctx.DrawHLine(b.Min.X, b.Min.Y+tc.TabHeight-1, b.Dx(), tc.TabBorder)
	}

	// Область содержимого
	cr := tc.contentRect()
	ctx.FillRect(cr.Min.X, cr.Min.Y, cr.Dx(), cr.Dy(), tc.ContentBG)

	// Рисуем содержимое активной вкладки
	if active >= 0 && active < len(tabs) && tabs[active].Content != nil {
		ctx.SetClip(cr)
		tabs[active].Content.Draw(ctx)
		ctx.ClearClip()
	}

	if st.Classic3D {
		// Классика: страница — выпуклая 3D-панель. Рисуем ПОСЛЕ контента
		// (фон контент-канвы иначе затирает грань), активный ярлык — самым
		// последним: он «прорезает» верхнюю грань страницы и сливается с ней.
		drawBevelRaised(ctx, cr.Min.X, cr.Min.Y, cr.Dx(), cr.Dy(), st)

		if !activeRect.Empty() {
			ar := activeRect
			ar.Min.X -= 2 // активный шире соседей на 2px с каждой стороны
			ar.Max.X += 2
			if ar.Min.X < b.Min.X {
				ar.Min.X = b.Min.X
			}
			bot := cr.Min.Y + 1 // перекрываем светлую грань страницы

			ctx.FillRect(ar.Min.X+1, ar.Min.Y+1, ar.Dx()-2, bot-ar.Min.Y-1, tc.TabActiveBG)
			// Верхняя грань со срезанными углами.
			ctx.DrawHLine(ar.Min.X+2, ar.Min.Y, ar.Dx()-4, st.BevelLight)
			ctx.SetPixel(ar.Min.X+1, ar.Min.Y+1, st.BevelLight)
			ctx.SetPixel(ar.Max.X-2, ar.Min.Y+1, st.BevelDark)
			// Боковые грани — до самой страницы.
			ctx.DrawVLine(ar.Min.X, ar.Min.Y+2, bot-ar.Min.Y-2, st.BevelLight)
			ctx.DrawVLine(ar.Max.X-1, ar.Min.Y+2, bot-ar.Min.Y-2, st.BevelDark)
			ctx.DrawVLine(ar.Max.X-2, ar.Min.Y+2, bot-ar.Min.Y-2, st.BevelShadow)
			// Текст активного ярлыка (позиция исходного слота — не сдвигается).
			ctx.DrawText(activeHeader, activeRect.Min.X+tc.TabPadH,
				activeRect.Min.Y+(activeRect.Dy()-13)/2, tc.TabActiveText)
		}
	} else {
		// Рамка вокруг содержимого.
		ctx.DrawBorder(cr.Min.X, cr.Min.Y, cr.Dx(), cr.Dy(), tc.TabBorder)
	}

	tc.drawDisabledOverlay(ctx)
}

// OnMouseButton обрабатывает клик по вкладке.
func (tc *TabControl) OnMouseButton(e MouseEvent) bool {
	if !tc.IsEnabled() {
		return false
	}
	if e.Button != MouseLeft || e.Pressed {
		return false
	}

	b := tc.bounds
	// Проверяем только полосу вкладок
	if e.Y < b.Min.Y || e.Y >= b.Min.Y+tc.TabHeight {
		return false
	}

	tc.mu.Lock()

	// Находим вкладку по X-позиции (используем реальные ширины из Draw).
	// Колбэк и layoutContent (сам берёт tc.mu) вызываем ПОСЛЕ Unlock.
	clicked, changed := -1, false
	header := ""
	tabX := b.Min.X
	seenTab := false
	for i := range tc.tabs {
		if tc.tabs[i].Hidden {
			continue
		}
		if tc.tabs[i].SeparatorBefore && seenTab {
			tabX += tabSepW
		}
		seenTab = true
		tabW := tc.TabPadH*2 + 80 // fallback
		if i < len(tc.tabWidths) {
			tabW = tc.tabWidths[i]
		}
		if e.X >= tabX && e.X < tabX+tabW {
			clicked = i
			if tc.active != i {
				tc.active = i
				changed = true
				header = tc.tabs[i].Header
			}
			break
		}
		tabX += tabW
	}
	onTab := tc.OnTabChange
	tc.mu.Unlock()

	if clicked < 0 {
		return false
	}
	if changed {
		// Обновляем bounds нового контента (layoutContent берёт tc.mu сам).
		tc.layoutContent()
		tc.Invalidate() // полоса вкладок и область контента изменились
		if onTab != nil {
			onTab(clicked, header) // синхронно — вне tc.mu
		}
	}
	return true
}

// OnMouseMove обрабатывает hover по вкладкам.
func (tc *TabControl) OnMouseMove(x, y int) {
	b := tc.bounds
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Авто-инвалидация при смене hover-вкладки (LIFO — выполняется до Unlock).
	oldHover := tc.hoverIdx
	defer func() {
		if tc.hoverIdx != oldHover {
			tc.Invalidate()
		}
	}()

	tc.hoverIdx = -1
	if y < b.Min.Y || y >= b.Min.Y+tc.TabHeight {
		return
	}

	tabX := b.Min.X
	seenTab := false
	for i, tab := range tc.tabs {
		if tab.Hidden {
			continue
		}
		if tab.SeparatorBefore && seenTab {
			tabX += tabSepW
		}
		seenTab = true
		tabW := len(tab.Header)*8 + tc.TabPadH*2 // fallback
		if i < len(tc.tabWidths) {
			tabW = tc.tabWidths[i]
		}
		if x >= tabX && x < tabX+tabW {
			tc.hoverIdx = i
			return
		}
		tabX += tabW
	}
}

// ─── CaptureAware ──────────────────────────────────────────────────────────

// SetCaptureManager сохраняет CaptureManager и рекурсивно раздаёт его
// контенту ВСЕХ вкладок (не только активной), чтобы виджеты на неактивных
// вкладках тоже могли корректно освобождать захват мыши.
func (tc *TabControl) SetCaptureManager(cm CaptureManager) {
	tc.capMgr = cm
	tc.mu.Lock()
	tabs := make([]TabItem, len(tc.tabs))
	copy(tabs, tc.tabs)
	tc.mu.Unlock()
	for _, tab := range tabs {
		if tab.Content != nil {
			injectCaptureManagerTree(tab.Content, cm)
		}
	}
}

// injectCaptureManagerTree рекурсивно раздаёт CaptureManager по дереву.
func injectCaptureManagerTree(w Widget, cm CaptureManager) {
	if ca, ok := w.(CaptureAware); ok {
		ca.SetCaptureManager(cm)
	}
	for _, child := range w.Children() {
		injectCaptureManagerTree(child, cm)
	}
}

// ApplyTheme обновляет цвета TabControl.
func (tc *TabControl) ApplyTheme(t *Theme) {
	tc.TabBG = t.TabBG
	tc.TabActiveBG = t.TabActiveBG
	tc.TabBorder = t.TabBorder
	tc.TabText = t.TabText
	tc.TabActiveText = t.TabActiveText
	tc.ContentBG = t.TabContentBG
	tc.AccentColor = t.Accent

	// Children() отдаёт только активную вкладку (для hit-test) — обход темы
	// движком не доберётся до остальных. Темизируем содержимое НЕАКТИВНЫХ
	// вкладок сами (активную обработает общий обход через Children()).
	tc.mu.Lock()
	var inactive []Widget
	for i := range tc.tabs {
		if i != tc.active && tc.tabs[i].Content != nil {
			inactive = append(inactive, tc.tabs[i].Content)
		}
	}
	tc.mu.Unlock()
	for _, c := range inactive {
		ApplyThemeTree(c, t)
	}
}

// clampAdd добавляет v к a с ограничением до 255.
func clampAdd(a uint8, v uint8) uint8 {
	r := int(a) + int(v)
	if r > 255 {
		return 255
	}
	return uint8(r)
}
