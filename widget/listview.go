package widget

import (
	"image"
	"image/color"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"
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

	mu         sync.Mutex
	items      []string
	selected   int     // индекс выделенного элемента (-1 = нет)
	hoverIdx   int     // индекс элемента под курсором (-1 = нет)
	scrollY    int     // смещение прокрутки
	scrollFrac float64 // субпиксельный остаток плавной пиксельной прокрутки

	// Скроллбар
	scrollbarWidth int
	dragging       bool
	dragStartY     int
	dragStartScr   int
	thumbHovered   bool

	focused int32 // 0 | 1

	OnSelect func(index int, text string) // вызывается при выборе элемента
	// OnActivate вызывается при «активации» элемента: двойной клик или Enter
	// (аналог открытия). Для файловых списков — вход в папку/выбор файла.
	OnActivate func(index int, text string)

	// Reorderable разрешает переставлять строки перетаскиванием (GG-85).
	// Выключено по умолчанию: в обычном списке перетаскивание меняло бы
	// смысл привычного движения мышью с зажатой кнопкой.
	Reorderable bool
	// OnReorder вызывается после перестановки: from — откуда строку взяли,
	// to — где она оказалась (индекс уже ПОСЛЕ перестановки, как у Move).
	OnReorder func(from, to int)

	// Перетаскивание строки: dragItem — что тащим (-1 — ничего), dragTo —
	// позиция вставки (0..len), dragArmed — нажали, но порог ещё не пройден.
	dragItem  int
	dragTo    int
	dragArmed bool
	dragItemY int

	lastClickIdx  int       // индекс последнего клика (для детекта double-click)
	lastClickTime time.Time // время последнего клика
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
		dragItem:       -1,
		dragTo:         -1,
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
//
// Срез КОПИРУЕТСЯ: элементы виджет читает из потока отрисовки, и запись в
// переданный массив после вызова была бы гонкой за строкой — на экране это
// каша из глифов (указатель от новой строки, длина от старой). По той же
// причине копия нужна стражу изменений: передав тот же срез с поправленным
// элементом, вызывающий сравнивал бы массив сам с собой, и перерисовки не
// было бы вовсе.
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

	lv.items = slices.Clone(items)
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
//
// Строки виджет читает из потока отрисовки, поэтому менять уже добавленные
// нельзя: новый текст элемента задаётся через SetItems.
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

// SetSelected программно выделяет элемент и шлёт OnSelect — как клик и как
// WPF, где SelectionChanged приходит и на программную смену (GG-85).
// Молча — SetSelectedQuiet.
func (lv *ListView) SetSelected(idx int) { lv.setSelected(idx, true) }

// SetSelectedQuiet выделяет элемент БЕЗ OnSelect.
//
// Нужен тому, кто сам ставит выделение в ответ на своё же событие: иначе
// колбэк вернулся бы в тот же обработчик.
func (lv *ListView) SetSelectedQuiet(idx int) { lv.setSelected(idx, false) }

func (lv *ListView) setSelected(idx int, notify bool) {
	lv.mu.Lock()
	changed := false
	text := ""
	if idx >= -1 && idx < len(lv.items) && idx != lv.selected {
		lv.selected = idx
		changed = true
		if idx >= 0 {
			text = lv.items[idx]
		}
	}
	onSel := lv.OnSelect
	lv.mu.Unlock()
	if !changed {
		return
	}
	lv.Invalidate()
	if notify && onSel != nil {
		onSel(idx, text) // синхронно — вне lv.mu, как у клика
	}
}

// ─── Перестановка строк перетаскиванием (GG-85) ─────────────────────────────

// listReorderThreshold — сколько точек нужно провести, чтобы нажатие стало
// перетаскиванием, а не щелчком по строке.
const listReorderThreshold = 4

// insertIndexAtLocked — позиция вставки (0..len) для координаты y: строка
// делится пополам, и ниже середины строка встаёт уже под неё.
func (lv *ListView) insertIndexAtLocked(y int) int {
	if lv.ItemHeight <= 0 {
		return 0
	}
	rel := y - lv.bounds.Min.Y + lv.scrollY
	idx := (rel + lv.ItemHeight/2) / lv.ItemHeight
	if idx < 0 {
		idx = 0
	}
	if idx > len(lv.items) {
		idx = len(lv.items)
	}
	return idx
}

// updateReorderLocked ведёт перетаскивание: до порога — ничего, после —
// двигает полосу вставки.
func (lv *ListView) updateReorderLocked(y int) {
	if lv.dragArmed {
		dy := y - lv.dragItemY
		if dy < 0 {
			dy = -dy
		}
		if dy < listReorderThreshold {
			return
		}
		lv.dragArmed = false // порог пройден: это перетаскивание
	}
	lv.dragTo = lv.insertIndexAtLocked(y)
}

// applyReorderLocked переставляет строку from на позицию вставки to и
// возвращает её новый индекс (-1 — ничего не изменилось).
func (lv *ListView) applyReorderLocked(from, to int) int {
	if from < 0 || from >= len(lv.items) || to < 0 {
		return -1
	}
	final := to
	if final > from {
		final-- // строку сначала вынули, всё после неё сдвинулось
	}
	if final < 0 {
		final = 0
	}
	if final >= len(lv.items) {
		final = len(lv.items) - 1
	}
	if final == from {
		return -1
	}
	item := lv.items[from]
	rest := append(lv.items[:from:from], lv.items[from+1:]...)
	lv.items = append(rest[:final:final], append([]string{item}, rest[final:]...)...)

	// Выделение едет со строкой: тянули именно её.
	switch {
	case lv.selected == from:
		lv.selected = final
	case from < lv.selected && lv.selected <= final:
		lv.selected--
	case final <= lv.selected && lv.selected < from:
		lv.selected++
	}
	return final
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
	dragTo := -1
	if lv.dragItem >= 0 && !lv.dragArmed {
		dragTo = lv.dragTo // порог пройден: показываем, куда встанет строка
	}
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

	// Полоса вставки: куда встанет перетаскиваемая строка (GG-85).
	if dragTo >= 0 {
		y := b.Min.Y + dragTo*lv.ItemHeight - scrollY
		if y < b.Min.Y+1 {
			y = b.Min.Y + 1
		}
		if y > b.Max.Y-1 {
			y = b.Max.Y - 1
		}
		ctx.FillRect(b.Min.X, y-1, cw, 2, lv.ThumbHoverBG)
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

	// Колесо мыши — прокрутка на 3 строки за тик. Поглощаем событие ТОЛЬКО
	// при реальном сдвиге: если содержимое помещается (maxScroll==0) или мы
	// уже у границы — возвращаем false, чтобы колесо всплыло к родительскому
	// ScrollView и не блокировало прокрутку страницы.
	if e.Button == MouseWheelUp || e.Button == MouseWheelDown {
		if !e.Pressed {
			return false
		}
		lv.mu.Lock()
		if !image.Pt(e.X, e.Y).In(lv.bounds) || lv.maxScroll() == 0 {
			lv.mu.Unlock()
			return false
		}
		old := lv.scrollY
		step := 3 * lv.ItemHeight
		if e.Button == MouseWheelUp {
			lv.scrollY -= step
		} else {
			lv.scrollY += step
		}
		lv.clampScroll()
		moved := lv.scrollY != old
		lv.mu.Unlock()
		if moved {
			lv.Invalidate()
		}
		return moved
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
			// Перетаскивание строки начнётся, если мышь пройдёт порог; до
			// этого нажатие остаётся обычным щелчком (GG-85).
			if lv.Reorderable {
				lv.dragArmed = true
				lv.dragItem = idx
				lv.dragTo = idx
				lv.dragItemY = e.Y
			}
			// Детект двойного клика: тот же элемент в пределах 400 мс.
			now := time.Now()
			dbl := idx == lv.lastClickIdx && now.Sub(lv.lastClickTime) < 400*time.Millisecond
			lv.lastClickIdx = idx
			lv.lastClickTime = now
			onSel := lv.OnSelect
			onAct := lv.OnActivate
			text := lv.items[idx]
			lv.mu.Unlock()
			if changed {
				lv.Invalidate() // выделение переместилось
			}
			if onSel != nil {
				onSel(idx, text) // синхронно — вне lv.mu
			}
			if dbl && onAct != nil {
				onAct(idx, text)
			}
			return true
		}
	} else {
		// Отпустили перетаскиваемую строку.
		if lv.dragItem >= 0 {
			from, to, armed := lv.dragItem, lv.dragTo, lv.dragArmed
			lv.dragItem, lv.dragTo, lv.dragArmed = -1, -1, false
			final := -1
			if !armed { // порог пройден — это было перетаскивание
				final = lv.applyReorderLocked(from, to)
			}
			onReorder := lv.OnReorder
			lv.mu.Unlock()
			lv.Invalidate()
			if final >= 0 && onReorder != nil {
				onReorder(from, final)
			}
			return true
		}
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

// OnMouseWheelPixels — плавная прокрутка точной пиксельной дельтой (тачпад/
// колесо высокой точности). dy>0 — вниз. В отличие от тикового колеса
// (3 строки за тик) применяет дельту попиксельно, накапливая субпиксельный
// остаток. Возвращает false у края в сторону жеста — чтобы событие всплыло
// к родительскому ScrollView.
func (lv *ListView) OnMouseWheelPixels(x, y int, dx, dy float64) bool {
	if !lv.IsEnabled() {
		return false
	}
	lv.mu.Lock()
	if !image.Pt(x, y).In(lv.bounds) || lv.maxScroll() == 0 {
		lv.mu.Unlock()
		return false
	}
	if (dy < 0 && lv.scrollY <= 0) || (dy > 0 && lv.scrollY >= lv.maxScroll()) {
		lv.mu.Unlock()
		return false
	}
	lv.scrollFrac += dy
	whole := math.Trunc(lv.scrollFrac)
	lv.scrollFrac -= whole
	old := lv.scrollY
	lv.scrollY += int(whole)
	lv.clampScroll()
	moved := lv.scrollY != old
	lv.mu.Unlock()
	if moved {
		lv.Invalidate()
	}
	return true
}

// WantsCapture захватывает мышь при нажатии ЛКМ на ползунке скроллбара —
// drag не должен обрываться, когда курсор выходит за границы списка.
func (lv *ListView) WantsCapture(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed || !lv.IsEnabled() {
		return false
	}
	lv.mu.Lock()
	defer lv.mu.Unlock()
	// Перетаскивание строки тоже должно переживать выход курсора за список.
	if lv.Reorderable && lv.itemIndexAt(e.X, e.Y) >= 0 {
		return true
	}
	if !lv.needsScrollbar() {
		return false
	}
	return image.Pt(e.X, e.Y).In(lv.thumbRect())
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
	oldTo := lv.dragTo
	defer func() {
		if lv.scrollY != oldScroll || lv.hoverIdx != oldHover ||
			lv.thumbHovered != oldThumb || lv.dragTo != oldTo {
			lv.Invalidate()
		}
	}()

	// Перетаскивание строки ведёт полосу вставки и hover не трогает.
	if lv.dragItem >= 0 {
		lv.updateReorderLocked(y)
		return
	}

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

	// Отложенный вызов callback — после освобождения lv.mu (синхронно).
	fireIdx, fireText, fireActivate := -1, "", false

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
		// Enter = активация выбранного элемента (открыть/выбрать).
		if lv.selected >= 0 && lv.selected < count {
			fireIdx, fireText, fireActivate = lv.selected, lv.items[lv.selected], true
		}
	}
	onSel := lv.OnSelect
	onAct := lv.OnActivate
	visChanged := lv.selected != oldSel || lv.scrollY != oldScroll
	lv.mu.Unlock()

	if visChanged {
		lv.Invalidate()
	}
	if fireActivate {
		// Обратная совместимость: если OnActivate не задан, Enter по-прежнему
		// уведомляет OnSelect (прежнее поведение).
		if onAct != nil {
			onAct(fireIdx, fireText)
		} else if onSel != nil {
			onSel(fireIdx, fireText)
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

// ScrollY возвращает текущее вертикальное смещение прокрутки (в пикселях).
func (lv *ListView) ScrollY() int {
	lv.mu.Lock()
	defer lv.mu.Unlock()
	return lv.scrollY
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

// SetFocused реализует Focusable. Поле объявлено как int32 под атомарный
// доступ: пишется из диспетчера фокуса, читается в Draw (рамка фокуса) из
// рендер-горутины — без atomic это гонка по memory model (SEC-18).
func (lv *ListView) SetFocused(v bool) {
	var n int32
	if v {
		n = 1
	}
	if atomic.SwapInt32(&lv.focused, n) != n {
		lv.Invalidate() // рамка фокуса
	}
}

// IsFocused реализует Focusable.
func (lv *ListView) IsFocused() bool {
	return atomic.LoadInt32(&lv.focused) == 1
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
