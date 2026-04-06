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
}

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
	defer tc.mu.Unlock()
	tc.tabs = append(tc.tabs, TabItem{Header: header, Content: content})
}

// SetActive устанавливает активную вкладку по индексу.
func (tc *TabControl) SetActive(idx int) {
	tc.mu.Lock()
	if idx >= 0 && idx < len(tc.tabs) {
		tc.active = idx
	}
	tc.mu.Unlock()
	tc.layoutContent()
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
func (tc *TabControl) layoutContent() {
	cr := tc.contentRect()
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if tc.active >= 0 && tc.active < len(tc.tabs) {
		if c := tc.tabs[tc.active].Content; c != nil {
			c.SetBounds(cr)
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

	// Полоса вкладок — фон
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), tc.TabHeight, tc.TabBG)

	// Рисуем каждую вкладку и сохраняем реальные ширины для hit-test.
	widths := make([]int, len(tabs))
	tabX := b.Min.X
	for i, tab := range tabs {
		textW := ctx.MeasureText(tab.Header, DefaultFontSizePt)
		tabW := textW + tc.TabPadH*2
		widths[i] = tabW
		tabRect := image.Rect(tabX, b.Min.Y, tabX+tabW, b.Min.Y+tc.TabHeight)

		// Фон вкладки
		if i == active {
			ctx.FillRect(tabRect.Min.X, tabRect.Min.Y, tabRect.Dx(), tabRect.Dy(), tc.TabActiveBG)
			// Акцентная линия сверху
			ctx.DrawHLine(tabRect.Min.X, tabRect.Min.Y, tabRect.Dx(), tc.AccentColor)
			ctx.DrawHLine(tabRect.Min.X, tabRect.Min.Y+1, tabRect.Dx(), tc.AccentColor)
		} else if i == hoverIdx {
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

		// Разделитель между вкладками
		if i < len(tabs)-1 {
			ctx.DrawVLine(tabRect.Max.X-1, tabRect.Min.Y+4, tabRect.Dy()-8, tc.TabBorder)
		}

		tabX += tabW
	}

	// Сохраняем реальные ширины для hit-test в OnMouseButton/OnMouseMove.
	tc.mu.Lock()
	tc.tabWidths = widths
	tc.mu.Unlock()

	// Линия под вкладками
	ctx.DrawHLine(b.Min.X, b.Min.Y+tc.TabHeight-1, b.Dx(), tc.TabBorder)

	// Область содержимого
	cr := tc.contentRect()
	ctx.FillRect(cr.Min.X, cr.Min.Y, cr.Dx(), cr.Dy(), tc.ContentBG)

	// Рисуем содержимое активной вкладки
	if active >= 0 && active < len(tabs) && tabs[active].Content != nil {
		ctx.SetClip(cr)
		tabs[active].Content.Draw(ctx)
		ctx.ClearClip()
	}

	// Рамка вокруг содержимого
	ctx.DrawBorder(cr.Min.X, cr.Min.Y, cr.Dx(), cr.Dy(), tc.TabBorder)

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
	defer tc.mu.Unlock()

	// Находим вкладку по X-позиции (используем реальные ширины из Draw).
	tabX := b.Min.X
	for i := range tc.tabs {
		tabW := tc.TabPadH*2 + 80 // fallback
		if i < len(tc.tabWidths) {
			tabW = tc.tabWidths[i]
		}
		if e.X >= tabX && e.X < tabX+tabW {
			if tc.active != i {
				tc.active = i
				if tc.OnTabChange != nil {
					go tc.OnTabChange(i, tc.tabs[i].Header)
				}
				// Обновляем bounds нового контента
				go tc.layoutContent()
			}
			return true
		}
		tabX += tabW
	}
	return false
}

// OnMouseMove обрабатывает hover по вкладкам.
func (tc *TabControl) OnMouseMove(x, y int) {
	b := tc.bounds
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.hoverIdx = -1
	if y < b.Min.Y || y >= b.Min.Y+tc.TabHeight {
		return
	}

	tabX := b.Min.X
	for i, tab := range tc.tabs {
		tabW := len(tab.Header)*8 + tc.TabPadH*2 // fallback
		if i < len(tc.tabWidths) {
			tabW = tc.tabWidths[i]
		}
		_ = tab
		if x >= tabX && x < tabX+tabW {
			tc.hoverIdx = i
			return
		}
		tabX += tabW
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
}

// clampAdd добавляет v к a с ограничением до 255.
func clampAdd(a uint8, v uint8) uint8 {
	r := int(a) + int(v)
	if r > 255 {
		return 255
	}
	return uint8(r)
}
