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

	// AutoScrollToBottom — режим «live tail»: после SetItems / AddItem
	// прокрутка автоматически встаёт в конец, если пользователь
	// уже находился у нижнего края списка. Если пользователь
	// проскроллил вверх — позиция сохраняется (не «убегает» под
	// добавлением новых строк). Поведение совпадает с типичным
	// логгером: WPF DataGrid.AutoScrollIntoView, Win32 ListBox c
	// LBS_NOTIFY и т.п.
	AutoScrollToBottom bool

	// PreserveScrollOnSetItems — если true, SetItems НЕ сбрасывает
	// scrollY в 0 (старое поведение по умолчанию). Используется,
	// когда коллекция перестраивается, но визуально остаётся
	// сравнимой (например, фильтрация/сортировка in-place).
	// AutoScrollToBottom имеет приоритет над этим флагом.
	PreserveScrollOnSetItems bool

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
//
// Поведение прокрутки определяется флагами:
//
//   - AutoScrollToBottom=true и пользователь был у нижнего края до вызова —
//     scrollY устанавливается в конец (live-tail режим).
//   - PreserveScrollOnSetItems=true — scrollY сохраняется (но clamp'ится
//     по новому maxScroll).
//   - иначе — старое поведение: scrollY=0, как при «новом списке».
//
// Selection всегда сбрасывается (-1), потому что индексы могут уже
// не указывать на тот же элемент.
func (lv *ListView) SetItems(items []string) {
	lv.mu.Lock()

	wasAtBottom := lv.isAtBottom()

	// Guard авто-инвалидации: перерисовка нужна только если содержимое,
	// выделение или прокрутка фактически изменились.
	same := len(items) == len(lv.items)
	if same {
		for i := range items {
			if items[i] != lv.items[i] {
				same = false
				break
			}
		}
	}
	changed := !same || lv.selected != -1
	oldScroll := lv.scrollY

	lv.items = items
	lv.selected = -1

	switch {
	case lv.AutoScrollToBottom && wasAtBottom:
		lv.scrollY = lv.maxScroll()
	case lv.PreserveScrollOnSetItems:
		lv.clampScroll()
	default:
		lv.scrollY = 0
	}
	changed = changed || lv.scrollY != oldScroll
	lv.mu.Unlock()

	if changed {
		lv.Invalidate()
	}
}

// isAtBottom возвращает true, если scrollY уже у нижнего края
// (с допуском в одну строку, чтобы покрыть рассинхрон в один пиксель
// после fractional thumb-drag). Должна вызываться под lv.mu.
func (lv *ListView) isAtBottom() bool {
	maxS := lv.maxScroll()
	if maxS <= 0 {
		// Нет необходимости в скроллбаре — формально всегда «в конце».
		return true
	}
	tol := lv.ItemHeight
	if tol <= 0 {
		tol = 1
	}
	return lv.scrollY >= maxS-tol
}

// ScrollToBottom прокручивает список до самого низа.
// Безопасно вызывать из любой goroutine.
func (lv *ListView) ScrollToBottom() {
	lv.mu.Lock()
	old := lv.scrollY
	lv.scrollY = lv.maxScroll()
	changed := lv.scrollY != old
	lv.mu.Unlock()
	if changed {
		lv.Invalidate()
	}
}

// ScrollToTop прокручивает список в начало.
func (lv *ListView) ScrollToTop() {
	lv.mu.Lock()
	changed := lv.scrollY != 0
	lv.scrollY = 0
	lv.mu.Unlock()
	if changed {
		lv.Invalidate()
	}
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
// Если AutoScrollToBottom=true и пользователь был у нижнего края —
// прокрутка автоматически встанет в конец после добавления.
func (lv *ListView) AddItem(text string) {
	lv.mu.Lock()
	wasAtBottom := lv.isAtBottom()
	lv.items = append(lv.items, text)
	if lv.AutoScrollToBottom && wasAtBottom {
		lv.scrollY = lv.maxScroll()
	}
	lv.mu.Unlock()
	lv.Invalidate() // новый элемент всегда меняет содержимое/скроллбар
}

// Clear удаляет все элементы из списка и сбрасывает выделение.
func (lv *ListView) Clear() {
	lv.mu.Lock()
	changed := len(lv.items) > 0 || lv.selected != -1 ||
		lv.hoverIdx != -1 || lv.scrollY != 0
	lv.items = lv.items[:0]
	lv.selected = -1
	lv.hoverIdx = -1
	lv.scrollY = 0
	lv.mu.Unlock()
	if changed {
		lv.Invalidate()
	}
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
	changed := false
	if idx >= -1 && idx < len(lv.items) && idx != lv.selected {
		lv.selected = idx
		changed = true
	}
	lv.mu.Unlock()
	if changed {
		lv.Invalidate()
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
	ch := lv.contentHeight()
	trackX := b.Max.X - lv.scrollbarWidth
	top, workH := sbWorkArea(b, lv.scrollbarWidth) // в классике — между кнопками ▲▼
	ratio := float64(b.Dy()) / float64(ch)
	thumbH := int(ratio * float64(workH))
	if thumbH < 20 {
		thumbH = 20
	}
	if thumbH > workH {
		thumbH = workH
	}
	maxS := lv.maxScroll()
	var thumbY int
	if maxS > 0 {
		thumbY = int(float64(lv.scrollY) / float64(maxS) * float64(workH-thumbH))
	}
	return image.Rect(trackX, top+thumbY, b.Max.X, top+thumbY+thumbH)
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
	if b.Empty() {
		return
	}
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
		if st := currentStyle(); st.Classic3D {
			track := image.Rect(trackX, b.Min.Y, b.Max.X, b.Max.Y)
			drawClassicScrollbar(ctx, track, tr, st, lv.ThumbColor, win10.LabelText)
		} else {
			ctx.FillRoundRect(tr.Min.X+1, tr.Min.Y+1, tr.Dx()-2, tr.Dy()-2, 3, tc)
		}
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

	b := lv.bounds

	if e.Pressed {
		// Скроллбар: клик на ползунке
		if lv.needsScrollbar() {
			tr := lv.thumbRect()
			if image.Pt(e.X, e.Y).In(tr) {
				lv.dragging = true
				lv.dragStartY = e.Y
				lv.dragStartScr = lv.scrollY
				lv.mu.Unlock()
				lv.Invalidate() // ползунок подсвечивается при drag
				return true
			}
			trackX := b.Max.X - lv.scrollbarWidth
			if e.X >= trackX {
				if currentStyle().Classic3D {
					// Кнопки ▲/▼ классического скроллбара — шаг на строку.
					btn := classicSBBtnH(lv.scrollbarWidth)
					if e.Y < b.Min.Y+btn {
						old := lv.scrollY
						lv.scrollY -= lv.ItemHeight
						lv.clampScroll()
						changed := lv.scrollY != old
						lv.mu.Unlock()
						if changed {
							lv.Invalidate()
						}
						return true
					}
					if e.Y >= b.Max.Y-btn {
						old := lv.scrollY
						lv.scrollY += lv.ItemHeight
						lv.clampScroll()
						changed := lv.scrollY != old
						lv.mu.Unlock()
						if changed {
							lv.Invalidate()
						}
						return true
					}
				}
				top, workH := sbWorkArea(b, lv.scrollbarWidth)
				ratio := float64(e.Y-top) / float64(workH)
				old := lv.scrollY
				lv.scrollY = int(ratio * float64(lv.contentHeight()))
				lv.clampScroll()
				changed := lv.scrollY != old
				lv.mu.Unlock()
				if changed {
					lv.Invalidate()
				}
				return true
			}
		}

		// Клик по элементу
		idx := lv.itemIndexAt(e.X, e.Y)
		if idx >= 0 {
			changed := lv.selected != idx
			lv.selected = idx
			onSel := lv.OnSelect
			text := lv.items[idx]
			lv.mu.Unlock()
			if changed {
				lv.Invalidate() // выделение переместилось
			}
			if onSel != nil {
				onSel(idx, text) // синхронно — вне lv.mu
			}
			return true
		}
	} else {
		if lv.dragging {
			lv.dragging = false
			lv.mu.Unlock()
			lv.Invalidate() // подсветка ползунка гаснет
			return true
		}
	}
	lv.mu.Unlock()
	return false
}

// OnMouseMove обрабатывает hover и drag скроллбара.
func (lv *ListView) OnMouseMove(x, y int) {
	if !lv.IsEnabled() {
		return
	}
	lv.mu.Lock()
	defer lv.mu.Unlock()

	// Авто-инвалидация при фактическом изменении (LIFO — выполняется до Unlock).
	oldScroll, oldHover, oldThumb := lv.scrollY, lv.hoverIdx, lv.thumbHovered
	defer func() {
		if lv.scrollY != oldScroll || lv.hoverIdx != oldHover || lv.thumbHovered != oldThumb {
			lv.Invalidate()
		}
	}()

	if lv.dragging {
		dy := y - lv.dragStartY
		_, workH := sbWorkArea(lv.bounds, lv.scrollbarWidth)
		tr := lv.thumbRect()
		thumbH := tr.Dy()
		trackUsable := workH - thumbH
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

	count := len(lv.items)
	if count == 0 {
		lv.mu.Unlock()
		return
	}

	// Снимок для авто-инвалидации при фактическом изменении.
	oldSel, oldScroll := lv.selected, lv.scrollY

	// Отложенный вызов OnSelect — после освобождения lv.mu (синхронно).
	fireIdx, fireText, fire := -1, "", false

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
			fireIdx, fireText, fire = lv.selected, lv.items[lv.selected], true
		}
	}
	onSel := lv.OnSelect
	visChanged := lv.selected != oldSel || lv.scrollY != oldScroll
	lv.mu.Unlock()

	if visChanged {
		lv.Invalidate()
	}
	if fire && onSel != nil {
		onSel(fireIdx, fireText)
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
	old := lv.scrollY
	lv.scrollY += delta
	lv.clampScroll()
	changed := lv.scrollY != old
	lv.mu.Unlock()
	if changed {
		lv.Invalidate()
	}
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
