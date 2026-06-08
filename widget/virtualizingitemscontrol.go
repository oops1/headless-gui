package widget

import (
	"image"
	"image/color"
	"sync"
)

// VirtualizingItemsControl — список с UI-виртуализацией (аналог WPF
// VirtualizingStackPanel внутри ItemsControl/ListBox).
//
// В отличие от обычного ItemsControl, который создаёт по виджету на КАЖДЫЙ
// элемент, этот контейнер материализует виджеты только для видимого окна
// (+небольшой буфер). Прокрутка пересобирает окно, переиспользуя кэш виджетов
// по индексу. Это позволяет работать со списками из десятков тысяч элементов.
//
// Использование (Go):
//
//	v := widget.NewVirtualizingItemsControl()
//	v.ItemHeight = 28
//	v.SetItemBuilder(func(item any, index int) widget.Widget {
//	    p := item.(*Person)
//	    return widget.NewLabel(p.Name, color.RGBA{255,255,255,255})
//	})
//	v.SetItems(people)            // []any
//	// или из CollectionView:
//	v.BindCollectionView(view)    // авто-обновление при сортировке/фильтре
type VirtualizingItemsControl struct {
	Base

	Background   color.RGBA
	BorderColor  color.RGBA
	TrackColor   color.RGBA
	ThumbColor   color.RGBA
	ThumbHoverBG color.RGBA
	ShowBorder   bool

	ItemHeight int // высота строки (обязательно > 0; по умолчанию 28)
	Buffer     int // сколько строк материализовать сверх видимого окна с каждой стороны

	mu      sync.Mutex
	items   []interface{}
	build   func(item interface{}, index int) Widget
	cache   map[int]Widget // индекс элемента → материализованный виджет
	scrollY int

	// Скроллбар
	scrollbarWidth int
	dragging       bool
	dragStartY     int
	dragStartScr   int
	thumbHovered   bool
}

// NewVirtualizingItemsControl создаёт виртуализированный список.
func NewVirtualizingItemsControl() *VirtualizingItemsControl {
	return &VirtualizingItemsControl{
		Background:     win10.WindowBG,
		BorderColor:    win10.Border,
		TrackColor:     win10.ScrollTrackBG,
		ThumbColor:     win10.ScrollThumbBG,
		ThumbHoverBG:   win10.Accent,
		ShowBorder:     true,
		ItemHeight:     28,
		Buffer:         2,
		cache:          map[int]Widget{},
		scrollbarWidth: 10,
	}
}

// SetItemBuilder задаёт фабрику виджета для элемента данных.
func (v *VirtualizingItemsControl) SetItemBuilder(f func(item interface{}, index int) Widget) {
	v.mu.Lock()
	v.build = f
	v.cache = map[int]Widget{} // сбрасываем кэш — фабрика изменилась
	v.mu.Unlock()
	v.updateVisible()
}

// SetItems заменяет данные и пересобирает видимое окно.
func (v *VirtualizingItemsControl) SetItems(items []interface{}) {
	v.mu.Lock()
	v.items = items
	v.cache = map[int]Widget{}
	v.clampScrollLocked()
	v.mu.Unlock()
	v.updateVisible()
}

// ItemCount возвращает число элементов данных (не виджетов).
func (v *VirtualizingItemsControl) ItemCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.items)
}

// BindCollectionView привязывает контейнер к CollectionView: элементы берутся из
// представления и обновляются при изменении фильтра/сортировки/группы/источника.
func (v *VirtualizingItemsControl) BindCollectionView(cv *CollectionView) {
	if cv == nil {
		return
	}
	v.SetItems(cv.Items())
	cv.AddViewChanged(func() { v.SetItems(cv.Items()) })
}

// ─── скролл/геометрия ───────────────────────────────────────────────────────

func (v *VirtualizingItemsControl) ih() int {
	if v.ItemHeight <= 0 {
		return 28
	}
	return v.ItemHeight
}

func (v *VirtualizingItemsControl) contentHeightLocked() int {
	return len(v.items) * v.ih()
}

func (v *VirtualizingItemsControl) needsScrollbarLocked() bool {
	return v.contentHeightLocked() > v.bounds.Dy()
}

func (v *VirtualizingItemsControl) maxScrollLocked() int {
	ch := v.contentHeightLocked()
	viewH := v.bounds.Dy()
	if ch <= viewH {
		return 0
	}
	return ch - viewH
}

func (v *VirtualizingItemsControl) clampScrollLocked() {
	maxS := v.maxScrollLocked()
	if v.scrollY < 0 {
		v.scrollY = 0
	}
	if v.scrollY > maxS {
		v.scrollY = maxS
	}
}

func (v *VirtualizingItemsControl) contentWidthLocked() int {
	w := v.bounds.Dx()
	if v.needsScrollbarLocked() {
		w -= v.scrollbarWidth
	}
	return w
}

// SetBounds пересобирает видимое окно при изменении размеров.
func (v *VirtualizingItemsControl) SetBounds(r image.Rectangle) {
	v.Base.SetBounds(r)
	v.updateVisible()
}

// ScrollBy прокручивает на delta пикселей и пересобирает окно.
func (v *VirtualizingItemsControl) ScrollBy(delta int) {
	v.mu.Lock()
	v.scrollY += delta
	v.clampScrollLocked()
	v.mu.Unlock()
	v.updateVisible()
}

// ScrollY возвращает текущее смещение.
func (v *VirtualizingItemsControl) ScrollY() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.scrollY
}

// updateVisible материализует виджеты для видимого окна (+буфер), удаляет
// вышедшие за пределы и переустанавливает список детей контейнера.
func (v *VirtualizingItemsControl) updateVisible() {
	v.mu.Lock()
	b := v.bounds
	ih := v.ih()
	n := len(v.items)
	if b.Empty() || n == 0 || v.build == nil {
		v.cache = map[int]Widget{}
		v.mu.Unlock()
		v.ClearChildren()
		return
	}

	start := v.scrollY/ih - v.Buffer
	if start < 0 {
		start = 0
	}
	end := (v.scrollY+b.Dy())/ih + v.Buffer
	if end >= n {
		end = n - 1
	}

	cw := v.contentWidthLocked()

	// Удаляем из кэша всё за пределами окна.
	for idx := range v.cache {
		if idx < start || idx > end {
			delete(v.cache, idx)
		}
	}

	// Материализуем недостающее и собираем упорядоченный список детей.
	children := make([]Widget, 0, end-start+1)
	for i := start; i <= end; i++ {
		w := v.cache[i]
		if w == nil {
			w = v.build(v.items[i], i)
			if w == nil {
				continue
			}
			v.cache[i] = w
		}
		itemY := b.Min.Y + i*ih - v.scrollY
		w.SetBounds(image.Rect(b.Min.X, itemY, b.Min.X+cw, itemY+ih))
		children = append(children, w)
	}
	v.mu.Unlock()

	// Заменяем детей (под защитой Base — без удержания v.mu, т.к. SetBounds
	// детей мог рекурсивно дернуть наш код).
	v.ClearChildren()
	for _, c := range children {
		v.AddChild(c)
	}
}

func (v *VirtualizingItemsControl) thumbRectLocked() image.Rectangle {
	b := v.bounds
	if !v.needsScrollbarLocked() {
		return image.Rectangle{}
	}
	viewH := b.Dy()
	ch := v.contentHeightLocked()
	trackX := b.Max.X - v.scrollbarWidth
	ratio := float64(viewH) / float64(ch)
	thumbH := int(ratio * float64(viewH))
	if thumbH < 20 {
		thumbH = 20
	}
	maxS := v.maxScrollLocked()
	var thumbY int
	if maxS > 0 {
		thumbY = int(float64(v.scrollY) / float64(maxS) * float64(viewH-thumbH))
	}
	return image.Rect(trackX, b.Min.Y+thumbY, b.Max.X, b.Min.Y+thumbY+thumbH)
}

// ─── Draw ─────────────────────────────────────────────────────────────────────

func (v *VirtualizingItemsControl) Draw(ctx DrawContext) {
	b := v.bounds
	if b.Empty() {
		return
	}

	ctx.FillRect(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), v.Background)

	v.mu.Lock()
	cw := v.contentWidthLocked()
	needSB := v.needsScrollbarLocked()
	tr := v.thumbRectLocked()
	v.mu.Unlock()

	// Видимые дети (уже спозиционированы updateVisible) — рисуем с клиппингом.
	ctx.SetClip(image.Rect(b.Min.X, b.Min.Y, b.Min.X+cw, b.Max.Y))
	v.drawChildren(ctx)
	ctx.ClearClip()

	// Скроллбар.
	if needSB {
		trackX := b.Max.X - v.scrollbarWidth
		ctx.FillRect(trackX, b.Min.Y, v.scrollbarWidth, b.Dy(), v.TrackColor)
		tc := v.ThumbColor
		if v.thumbHovered || v.dragging {
			tc = v.ThumbHoverBG
		}
		ctx.FillRoundRect(tr.Min.X+1, tr.Min.Y+1, tr.Dx()-2, tr.Dy()-2, 3, tc)
	}

	if v.ShowBorder {
		ctx.DrawBorder(b.Min.X, b.Min.Y, b.Dx(), b.Dy(), v.BorderColor)
	}

	v.drawDisabledOverlay(ctx)
}

// ─── Mouse ────────────────────────────────────────────────────────────────────

const vicWheelStep = 40

func (v *VirtualizingItemsControl) OnMouseButton(e MouseEvent) bool {
	if !v.IsEnabled() {
		return false
	}

	// Колесо мыши.
	if e.Button == MouseWheelUp {
		v.ScrollBy(-vicWheelStep)
		return true
	}
	if e.Button == MouseWheelDown {
		v.ScrollBy(vicWheelStep)
		return true
	}

	if e.Button != MouseLeft {
		return false
	}

	v.mu.Lock()
	b := v.bounds
	needSB := v.needsScrollbarLocked()
	if e.Pressed {
		if needSB {
			tr := v.thumbRectLocked()
			if image.Pt(e.X, e.Y).In(tr) {
				v.dragging = true
				v.dragStartY = e.Y
				v.dragStartScr = v.scrollY
				v.mu.Unlock()
				return true
			}
			trackX := b.Max.X - v.scrollbarWidth
			if e.X >= trackX {
				ratio := float64(e.Y-b.Min.Y) / float64(b.Dy())
				v.scrollY = int(ratio * float64(v.contentHeightLocked()))
				v.clampScrollLocked()
				v.mu.Unlock()
				v.updateVisible()
				return true
			}
		}
		v.mu.Unlock()
		return false // клики по элементам обрабатывают сами дочерние виджеты
	}
	// release
	if v.dragging {
		v.dragging = false
		v.mu.Unlock()
		return true
	}
	v.mu.Unlock()
	return false
}

func (v *VirtualizingItemsControl) OnMouseMove(x, y int) {
	if !v.IsEnabled() {
		return
	}
	v.mu.Lock()
	if v.dragging {
		dy := y - v.dragStartY
		viewH := v.bounds.Dy()
		tr := v.thumbRectLocked()
		trackUsable := viewH - tr.Dy()
		if trackUsable > 0 {
			delta := int(float64(dy) / float64(trackUsable) * float64(v.maxScrollLocked()))
			v.scrollY = v.dragStartScr + delta
			v.clampScrollLocked()
		}
		v.mu.Unlock()
		v.updateVisible()
		return
	}
	if v.needsScrollbarLocked() {
		v.thumbHovered = image.Pt(x, y).In(v.thumbRectLocked())
	}
	v.mu.Unlock()
}

// WantsCapture захватывает мышь при перетаскивании ползунка.
func (v *VirtualizingItemsControl) WantsCapture(e MouseEvent) bool {
	if e.Button != MouseLeft || !e.Pressed {
		return false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.needsScrollbarLocked() {
		return false
	}
	return image.Pt(e.X, e.Y).In(v.thumbRectLocked())
}

// ApplyTheme обновляет цвета.
func (v *VirtualizingItemsControl) ApplyTheme(t *Theme) {
	v.Background = t.WindowBG
	v.BorderColor = t.Border
	v.TrackColor = t.ScrollTrackBG
	v.ThumbColor = t.ScrollThumbBG
	v.ThumbHoverBG = t.Accent
}
