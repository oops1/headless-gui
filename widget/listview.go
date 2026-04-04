package widget

import (
	"image"
	"image/color"
	"sync"
)

// ListView — список элементов с вертикальной прокруткой и выделением.
//
// Каждый элемент — строка с текстом фиксированной высоты.
// Поддерживает: одиночное выделение, hover, скроллбар, клавиатурную навигацию.
type ListView struct {
	Base

	Background   color.RGBA
	TextColor    color.RGBA
	HoverColor   color.RGBA
	SelectColor  color.RGBA
	BorderColor  color.RGBA
	TrackColor   color.RGBA
	ThumbColor   color.RGBA
	ThumbHoverBG color.RGBA
	ShowBorder   bool

	ItemHeight int // высота одного элемента (по умолчанию 28)

	mu       sync.Mutex
	items    []string
	selected int // индекс выделенного элемента (-1 = нет)
	hoverIdx int // индекс элемента под курсором (-1 = нет)
	scrollY  int // смещение прокрутки

	// Скроллбар
	scrollbarWidth int
	dragging       bool
	dragStartY     int
	dragStartScr   int
	thumbHovered   bool

	focused int32 // 0 | 1

	OnSelect func(index int, text string) // вызывается при выборе элемента
}

// NewListView создаёт список с заданными элементами.
func NewListView(items ...string) *ListView {
	return &ListView{
		Background:     win10.WindowBG,
		TextColor:      win10.LabelText,
		HoverColor:     win10.ListItemHover,
		SelectColor:    win10.ListItemSelect,
		BorderColor:    win10.Border,
		TrackColor:     win10.ScrollTrackBG,
		ThumbColor:     win10.ScrollThumbBG,
		ThumbHoverBG:   win10.Accent,
		ShowBorder:     true,
		ItemHeight:     28,
		items:          items,
		selected:       -1,
		hoverIdx:       -1,
		scrollbarWidth: 10,
	}
}

// SetItems заменяет список элементов.
func (lv *ListView) SetItems(items []string) {
	lv.mu.Lock()
	defer lv.mu.Unlock()
	lv.items = items
	lv.selected = -1
	lv.scrollY = 0
}

// Items возвращает копию списка элементов.
func (lv *ListView) Items() []string {
	lv.mu.Lock()
	defer lv.mu.Unlock()
	result := make([]string, len(lv.items))
	copy(result, lv.items)
	return result
}

// AddItem добавляет элемент в конец списка.
func (lv *ListView) AddItem(text string) {
	lv.mu.Lock()
	defer lv.mu.Unlock()
	lv.items = append(lv.items, text)
}

// Clear удаляет все элементы из списка и сбрасывает выделение.
func (lv *ListView) Clear() {
	lv.mu.Lock()
	defer lv.mu.Unlock()
	lv.items = lv.items[:0]
	lv.selected = -1
	lv.hoverIdx = -1
	lv.scrollY = 0
}

// Selected возвращает индекс выделенного элемента (-1 если нет).
func (lv *ListView) Selected() int {
	lv.mu.Lock()
	defer lv.mu.Unlock()
	return lv.selected
}

// SelectedText возвращает текст выделенного элемента.
func (lv *ListView) SelectedText() string {
	lv.mu.Lock()
	defer lv.mu.Unlock()
	if lv.selected >= 0 && lv.selected < len(lv.items) {
		return lv.items[lv.selected]
	}
	return ""
}

// SetSelected программно выделяет элемент.
func (lv *ListView) SetSelected(idx int) {
	lv.mu.Lock()
	defer lv.mu.Unlock()
	if idx >= -1 && idx < len(lv.items) {
		lv.selected = idx
	}
}

func (lv *ListView) contentHeight() int {
	return len(lv.items) * lv.ItemHeight
}

func (lv *ListView) needsScrollbar() bool {
	return lv.contentHeight() > lv.bounds.Dy()
}

func (lv *ListView) maxScroll() int {
	viewH := lv.bounds.Dy()
	ch := lv.contentHeight()
	if ch <= viewH {
		return 0
	}
	return ch - viewH
}

func (lv *ListView) clampScroll() {
	maxS := lv.maxScroll()
	if lv.scrollY < 0 {
		lv.scrollY = 0
	}
	if lv.scrollY > maxS {
		lv.scrollY = maxS
	}
}

func (lv *ListView) contentWidth() int {
	w := lv.bounds.Dx()
	if lv.needsScrollbar() {
		w -= lv.scrollbarWidth
	}
	return w
}

func (lv *ListView) thumbRect() image.Rectangle {
	b := lv.bounds
	if !lv.needsScrollbar() {
		return image.Rectangle{}
	}
	viewH := b.Dy()
	ch := lv.contentHeight()
	trackX := b.Max.X - lv.scrollbarWidth
	ratio := float64(viewH) / float64(ch)
	thumbH := int(ratio * float64(viewH))
	if thumbH < 20 {
		thumbH = 20
	}
	maxS := lv.maxScroll()
	var thumbY int
	if maxS > 0 {
		thumbY = int(float64(lv.scrollY) / float64(maxS) * float64(viewH-thumbH))
	}
	return image.Rect(trackX, b.Min.Y+thumbY, b.Max.X, b.Min.Y+thumbY+thumbH)
}

// itemIndexAt возвращает индекс элемента по координатам мыши.
func (lv *ListView) itemIndexAt(x, y int) int {
	b := lv.bounds
	if x < b.Min.X || x >= b.Min.X+lv.contentWidth() {
		return -1
	}
	if y < b.Min.Y || y >= b.Max.Y {
		return -1
	}
	idx := (y - b.Min.Y + lv.scrollY) / lv.ItemHeight
	if idx >= 0 && idx < len(lv.items) {
		return idx
	}
	return -1
}

// Draw рисует ListView с элементами, выделением и скроллбаром.
func (lv *ListView) Draw(ctx DrawContext) {
	b := lv.bounds
	lv.mu.Lock()
	items := lv.items
	selected := lv.selected
	hoverIdx := lv.hoverIdx
	scrollY := lv.scrollY
	lv.mu.Unlock()

	// Фон
	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), lv.Background)

	// Клиппинг
	cw := lv.contentWidth()
	ctx.SetClip(image.Rect(b.Min.X, b.Min.Y, b.Min.X+cw, b.Max.Y))

	// Элементы
	startIdx := scrollY / lv.ItemHeight
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := (scrollY + b.Dy()) / lv.ItemHeight
	if endIdx >= len(items) {
		endIdx = len(items) - 1
	}

	for i := startIdx; i <= endIdx; i++ {
		itemY := b.Min.Y + i*lv.ItemHeight - scrollY
		if itemY+lv.ItemHeight < b.Min.Y || itemY > b.Max.Y {
			continue
		}

		// Фон элемента
		if i == selected {
			ctx.FillRectAlpha(b.Min.X, itemY, cw, lv.ItemHeight, lv.SelectColor)
		} else if i == hoverIdx {
			ctx.FillRect(b.Min.X, itemY, cw, lv.ItemHeight, lv.HoverColor)
		}

		// Текст
		textY := itemY + (lv.ItemHeight-13)/2
		ctx.DrawText(items[i], b.Min.X+8, textY, lv.TextColor)
	}

	ctx.ClearClip()

	// Скроллбар
	if lv.needsScrollbar() {
		trackX := b.Max.X - lv.scrollbarWidth
		ctx.FillRect(trackX, b.Min.Y, lv.scrollbarWidth, b.Dy(), lv.TrackColor)

		lv.mu.Lock()
		tr := lv.thumbRect()
		lv.mu.Unlock()

		tc := lv.ThumbColor
		if lv.thumbHovered || lv.dragging {
			tc = lv.ThumbHoverBG
		}
		ctx.FillRoundRect(tr.Min.X+1, tr.Min.Y+1, tr.Dx()-2, tr.Dy()-2, 3, tc)
	}

	// Рамка
	if lv.ShowBorder {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), lv.BorderColor)
	}

	lv.drawDisabledOverlay(ctx)
}

// OnMouseButton обрабатывает клик по элементу или скроллбару.
func (lv *ListView) OnMouseButton(e MouseEvent) bool {
	if !lv.IsEnabled() {
		return false
	}
	if e.Button != MouseLeft {
		return false
	}

	lv.mu.Lock()
	defer lv.mu.Unlock()

	b := lv.bounds

	if e.Pressed {
		// Скроллбар: клик на ползунке
		if lv.needsScrollbar() {
			tr := lv.thumbRect()
			if image.Pt(e.X, e.Y).In(tr) {
				lv.dragging = true
				lv.dragStartY = e.Y
				lv.dragStartScr = lv.scrollY
				return true
			}
			trackX := b.Max.X - lv.scrollbarWidth
			if e.X >= trackX {
				ratio := float64(e.Y-b.Min.Y) / float64(b.Dy())
				lv.scrollY = int(ratio * float64(lv.contentHeight()))
				lv.clampScroll()
				return true
			}
		}

		// Клик по элементу
		idx := lv.itemIndexAt(e.X, e.Y)
		if idx >= 0 {
			lv.selected = idx
			if lv.OnSelect != nil {
				go lv.OnSelect(idx, lv.items[idx])
			}
			return true
		}
	} else {
		if lv.dragging {
			lv.dragging = false
			return true
		}
	}
	return false
}

// OnMouseMove обрабатывает hover и drag скроллбара.
func (lv *ListView) OnMouseMove(x, y int) {
	if !lv.IsEnabled() {
		return
	}
	lv.mu.Lock()
	defer lv.mu.Unlock()

	if lv.dragging {
		dy := y - lv.dragStartY
		viewH := lv.bounds.Dy()
		tr := lv.thumbRect()
		thumbH := tr.Dy()
		trackUsable := viewH - thumbH
		if trackUsable > 0 {
			scrollDelta := int(float64(dy) / float64(trackUsable) * float64(lv.maxScroll()))
			lv.scrollY = lv.dragStartScr + scrollDelta
			lv.clampScroll()
		}
		return
	}

	// Hover элемента
	lv.hoverIdx = lv.itemIndexAt(x, y)

	// Hover скроллбара
	if lv.needsScrollbar() {
		tr := lv.thumbRect()
		lv.thumbHovered = image.Pt(x, y).In(tr)
	}
}

// OnKeyEvent обрабатывает клавиатурную навигацию (Up/Down, Enter, Home/End).
func (lv *ListView) OnKeyEvent(e KeyEvent) {
	if !lv.IsEnabled() || !e.Pressed {
		return
	}
	lv.mu.Lock()
	defer lv.mu.Unlock()

	count := len(lv.items)
	if count == 0 {
		return
	}

	switch e.Code {
	case KeyUp:
		if lv.selected > 0 {
			lv.selected--
			lv.ensureVisible(lv.selected)
		}
	case KeyDown:
		if lv.selected < count-1 {
			lv.selected++
			lv.ensureVisible(lv.selected)
		}
	case KeyHome:
		lv.selected = 0
		lv.scrollY = 0
	case KeyEnd:
		lv.selected = count - 1
		lv.ensureVisible(lv.selected)
	case KeyEnter:
		if lv.selected >= 0 && lv.selected < count && lv.OnSelect != nil {
			go lv.OnSelect(lv.selected, lv.items[lv.selected])
		}
	}
}

// ensureVisible прокручивает список так, чтобы элемент idx был виден.
func (lv *ListView) ensureVisible(idx int) {
	itemTop := idx * lv.ItemHeight
	itemBot := itemTop + lv.ItemHeight
	viewH := lv.bounds.Dy()

	if itemTop < lv.scrollY {
		lv.scrollY = itemTop
	}
	if itemBot > lv.scrollY+viewH {
		lv.scrollY = itemBot - viewH
	}
	lv.clampScroll()
}

// ScrollBy прокручивает список на delta пикселей.
func (lv *ListView) ScrollBy(delta int) {
	lv.mu.Lock()
	defer lv.mu.Unlock()
	lv.scrollY += delta
	lv.clampScroll()
}

// SetFocused реализует Focusable.
func (lv *ListView) SetFocused(v bool) {
	if v {
		lv.focused = 1
	} else {
		lv.focused = 0
	}
}

// IsFocused реализует Focusable.
func (lv *ListView) IsFocused() bool {
	return lv.focused == 1
}

// ApplyTheme обновляет цвета ListView.
func (lv *ListView) ApplyTheme(t *Theme) {
	lv.Background = t.WindowBG
	lv.TextColor = t.LabelText
	lv.HoverColor = t.ListItemHover
	lv.SelectColor = t.ListItemSelect
	lv.BorderColor = t.Border
	lv.TrackColor = t.ScrollTrackBG
	lv.ThumbColor = t.ScrollThumbBG
	lv.ThumbHoverBG = t.Accent
}
