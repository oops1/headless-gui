package treeview

import (
	"image"
	"sync"
	"time"

	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

// ─── Константы ─────────────────────────────────────────────────────────────

const (
	defaultItemHeight  = 22
	defaultIndentSize  = 18
	defaultFontSize    = 10.0
	defaultIconSize    = 16
	arrowZone          = 14     // ширина зоны стрелки (px)
	scrollbarWidth     = 12
	scrollMinThumbH    = 20     // минимальная высота ползунка скроллбара
)

// ─── TreeView ──────────────────────────────────────────────────────────────

// TreeView — WPF-совместимый иерархический список.
type TreeView struct {
	bounds image.Rectangle

	// ── Корневые узлы ────────────────────────────────────────────────────
	roots []*TreeViewItem

	// ── ItemsSource (для data binding) ──────────────────────────────────
	itemsSource *datagrid.ObservableCollection
	itemTemplate *HierarchicalDataTemplate

	// ── Свойства (WPF-совместимые) ──────────────────────────────────────
	ItemHeight  int     // высота одной строки (px). 0 → defaultItemHeight
	IndentSize  int     // отступ уровня вложенности (px). 0 → defaultIndentSize
	FontSize    float64 // размер шрифта. 0 → defaultFontSize
	IconSize    int     // размер иконки (px). 0 → defaultIconSize
	IsReadOnly  bool    // только чтение
	ShowIndentGuides bool // показывать линии иерархии

	// ── Выделение ────────────────────────────────────────────────────────
	selectedItem *TreeViewItem

	// ── Скроллинг ────────────────────────────────────────────────────────
	scrollY         int  // смещение в пикселях
	thumbDragging   bool
	thumbDragStartY int
	thumbDragStartS int
	thumbHovered    bool

	// ── Hover ────────────────────────────────────────────────────────────
	hoverIdx int // индекс в flat-списке (-1 = нет)

	// ── Двойной клик ─────────────────────────────────────────────────────
	lastClickTime int64
	lastClickIdx  int

	// ── Тема ─────────────────────────────────────────────────────────────
	Theme TreeViewTheme

	// ── Callbacks ────────────────────────────────────────────────────────
	OnSelectedItemChanged SelectedItemChangedHandler
	OnExpanded            ExpandedHandler
	OnCollapsed           CollapsedHandler
	OnItemInvoked         ItemInvokedHandler

	// Обратная совместимость: простой callback как в старом TreeView
	OnSelect func(item *TreeViewItem)

	// ── Внутреннее ───────────────────────────────────────────────────────
	mu      sync.Mutex
	focused bool
	dirty   bool // нужен пересчёт flat-списка
}

// New создаёт TreeView с настройками по умолчанию.
func New() *TreeView {
	tv := &TreeView{
		Theme:    DefaultDarkTheme(),
		hoverIdx: -1,
		dirty:    true,
	}
	return tv
}

// ─── Widget interface ──────────────────────────────────────────────────────

func (tv *TreeView) Bounds() image.Rectangle     { return tv.bounds }
func (tv *TreeView) SetBounds(r image.Rectangle)  { tv.bounds = r; tv.dirty = true }

// ─── Roots management ──────────────────────────────────────────────────────

// AddRoot добавляет корневой узел.
func (tv *TreeView) AddRoot(item *TreeViewItem) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	item.depth = 0
	item.setOwnerRecursive(tv)
	tv.roots = append(tv.roots, item)
	tv.dirty = true
}

// Roots возвращает копию списка корневых узлов.
func (tv *TreeView) Roots() []*TreeViewItem {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	result := make([]*TreeViewItem, len(tv.roots))
	copy(result, tv.roots)
	return result
}

// ClearRoots удаляет все корневые узлы.
func (tv *TreeView) ClearRoots() {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	for _, r := range tv.roots {
		r.setOwnerRecursive(nil)
	}
	tv.roots = tv.roots[:0]
	tv.selectedItem = nil
	tv.scrollY = 0
	tv.hoverIdx = -1
	tv.dirty = true
}

// SetRoots заменяет все корневые узлы.
func (tv *TreeView) SetRoots(items []*TreeViewItem) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	for _, r := range tv.roots {
		r.setOwnerRecursive(nil)
	}
	tv.roots = make([]*TreeViewItem, len(items))
	copy(tv.roots, items)
	for _, item := range tv.roots {
		item.depth = 0
		item.setOwnerRecursive(tv)
	}
	tv.selectedItem = nil
	tv.scrollY = 0
	tv.hoverIdx = -1
	tv.dirty = true
}

// ─── ItemsSource (Data Binding) ────────────────────────────────────────────

// SetItemsSource привязывает ObservableCollection как источник корневых узлов.
func (tv *TreeView) SetItemsSource(oc *datagrid.ObservableCollection) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.itemsSource = oc
	tv.dirty = true

	// Подписываемся на изменения
	if oc != nil {
		oc.AddCollectionChanged(func(e datagrid.CollectionChangedEvent) {
			tv.rebuildFromItemsSource()
		})
		tv.rebuildFromItemsSourceLocked()
	}
}

// SetItemTemplate устанавливает шаблон для отображения данных.
func (tv *TreeView) SetItemTemplate(tmpl *HierarchicalDataTemplate) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.itemTemplate = tmpl
	tv.dirty = true
}

// rebuildFromItemsSource пересоздаёт дерево из ItemsSource (с блокировкой).
func (tv *TreeView) rebuildFromItemsSource() {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.rebuildFromItemsSourceLocked()
}

// rebuildFromItemsSourceLocked пересоздаёт дерево (вызывать под mu.Lock).
func (tv *TreeView) rebuildFromItemsSourceLocked() {
	if tv.itemsSource == nil {
		return
	}
	tmpl := tv.itemTemplate

	// Очищаем старые корни
	for _, r := range tv.roots {
		r.setOwnerRecursive(nil)
	}
	tv.roots = tv.roots[:0]

	items := tv.itemsSource.Items()
	for _, dataObj := range items {
		item := tv.createItemFromData(dataObj, tmpl, 0)
		tv.roots = append(tv.roots, item)
	}
	tv.dirty = true
}

// createItemFromData создаёт TreeViewItem из объекта данных используя шаблон.
func (tv *TreeView) createItemFromData(dataObj interface{}, tmpl *HierarchicalDataTemplate, depth int) *TreeViewItem {
	item := NewItem("")
	item.DataContext = dataObj
	item.depth = depth
	item.owner = tv

	if tmpl != nil {
		// Заголовок
		if h := tmpl.resolveHeader(dataObj); h != "" {
			item.Header = h
		}

		// Иконка
		if icon := tmpl.resolveIcon(dataObj); icon != nil {
			item.Icon = icon
		}

		// IsExpanded
		if tmpl.IsExpandedPath != "" {
			item.Expanded = tmpl.resolveIsExpanded(dataObj)
		}

		// Дочерние элементы
		if children := tmpl.resolveChildren(dataObj); len(children) > 0 {
			for _, childData := range children {
				child := tv.createItemFromData(childData, tmpl, depth+1)
				child.parent = item
				item.Children = append(item.Children, child)
			}
		}
	}

	return item
}

// ─── Selection ─────────────────────────────────────────────────────────────

// SelectedItem возвращает текущий выделенный узел.
func (tv *TreeView) SelectedItem() *TreeViewItem {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	return tv.selectedItem
}

// SetSelectedItem программно выбирает узел.
func (tv *TreeView) SetSelectedItem(item *TreeViewItem) {
	tv.mu.Lock()
	old := tv.selectedItem
	tv.selectedItem = item
	if old != nil {
		old.IsSelected = false
	}
	if item != nil {
		item.IsSelected = true
	}
	tv.mu.Unlock()

	if old != item {
		tv.fireSelectedItemChanged(old, item)
	}
}

func (tv *TreeView) fireSelectedItemChanged(oldItem, newItem *TreeViewItem) {
	if tv.OnSelectedItemChanged != nil {
		tv.OnSelectedItemChanged(SelectedItemChangedEvent{
			OldItem: oldItem,
			NewItem: newItem,
		})
	}
	// Обратная совместимость
	if tv.OnSelect != nil && newItem != nil {
		tv.OnSelect(newItem)
	}
}

// ─── Expand / Collapse ─────────────────────────────────────────────────────

// ExpandItem раскрывает узел.
func (tv *TreeView) ExpandItem(item *TreeViewItem) {
	if item == nil || item.Expanded {
		return
	}
	item.Expanded = true
	tv.dirty = true
	if tv.OnExpanded != nil {
		tv.OnExpanded(ExpandedEvent{Item: item})
	}
}

// CollapseItem сворачивает узел.
func (tv *TreeView) CollapseItem(item *TreeViewItem) {
	if item == nil || !item.Expanded {
		return
	}
	item.Expanded = false
	tv.dirty = true
	if tv.OnCollapsed != nil {
		tv.OnCollapsed(CollapsedEvent{Item: item})
	}
}

// ToggleExpand переключает раскрытие/свёртывание.
func (tv *TreeView) ToggleExpand(item *TreeViewItem) {
	if item == nil {
		return
	}
	if item.Expanded {
		tv.CollapseItem(item)
	} else {
		tv.ExpandItem(item)
	}
}

// ─── Focus ─────────────────────────────────────────────────────────────────

func (tv *TreeView) SetFocused(v bool) { tv.focused = v }
func (tv *TreeView) IsFocused() bool   { return tv.focused }

// ─── Visible nodes ─────────────────────────────────────────────────────────

// visibleNodes возвращает плоский список видимых узлов.
func (tv *TreeView) visibleNodes() []flatItem {
	var result []flatItem
	for _, root := range tv.roots {
		collectVisible(root, root.depth, &result)
	}
	return result
}

// ─── Helpers ───────────────────────────────────────────────────────────────

func (tv *TreeView) itemH() int {
	if tv.ItemHeight > 0 {
		return tv.ItemHeight
	}
	return defaultItemHeight
}

func (tv *TreeView) indentW() int {
	if tv.IndentSize > 0 {
		return tv.IndentSize
	}
	return defaultIndentSize
}

func (tv *TreeView) fontSize() float64 {
	if tv.FontSize > 0 {
		return tv.FontSize
	}
	return defaultFontSize
}

func (tv *TreeView) iconSz() int {
	if tv.IconSize > 0 {
		return tv.IconSize
	}
	return defaultIconSize
}

// totalVisibleHeight возвращает полную высоту содержимого (для скроллбара).
func (tv *TreeView) totalVisibleHeight() int {
	flat := tv.visibleNodes()
	return len(flat) * tv.itemH()
}

// maxScrollY возвращает максимальное смещение прокрутки.
func (tv *TreeView) maxScrollY() int {
	b := tv.bounds
	total := tv.totalVisibleHeight()
	maxS := total - b.Dy()
	if maxS < 0 {
		return 0
	}
	return maxS
}

// clampScroll ограничивает scrollY допустимым диапазоном.
func (tv *TreeView) clampScroll() {
	if tv.scrollY < 0 {
		tv.scrollY = 0
	}
	if maxS := tv.maxScrollY(); tv.scrollY > maxS {
		tv.scrollY = maxS
	}
}

// ensureVisible прокручивает так, чтобы узел с индексом idx был виден.
func (tv *TreeView) ensureVisible(idx int) {
	ih := tv.itemH()
	b := tv.bounds

	top := idx * ih
	bottom := top + ih

	if top < tv.scrollY {
		tv.scrollY = top
	} else if bottom > tv.scrollY+b.Dy() {
		tv.scrollY = bottom - b.Dy()
	}
	tv.clampScroll()
}

// indexOfItem ищет индекс узла в плоском списке.
func (tv *TreeView) indexOfItem(item *TreeViewItem, flat []flatItem) int {
	for i, fi := range flat {
		if fi.item == item {
			return i
		}
	}
	return -1
}

// nowMs возвращает текущее время в миллисекундах.
func nowMs() int64 {
	return time.Now().UnixMilli()
}

// ─── Scrollbar helpers ─────────────────────────────────────────────────────

// needsScrollbar возвращает true, если содержимое не помещается.
func (tv *TreeView) needsScrollbar() bool {
	return tv.totalVisibleHeight() > tv.bounds.Dy()
}

// scrollbarRect возвращает прямоугольник области скроллбара.
func (tv *TreeView) scrollbarRect() image.Rectangle {
	b := tv.bounds
	return image.Rect(b.Max.X-scrollbarWidth, b.Min.Y, b.Max.X, b.Max.Y)
}

// thumbRect возвращает прямоугольник ползунка скроллбара.
func (tv *TreeView) thumbRect() image.Rectangle {
	sr := tv.scrollbarRect()
	trackH := sr.Dy()
	totalH := tv.totalVisibleHeight()

	if totalH <= 0 {
		return image.Rectangle{}
	}

	viewH := tv.bounds.Dy()
	thumbH := trackH * viewH / totalH
	if thumbH < scrollMinThumbH {
		thumbH = scrollMinThumbH
	}
	if thumbH > trackH {
		thumbH = trackH
	}

	maxS := tv.maxScrollY()
	thumbY := 0
	if maxS > 0 {
		thumbY = (trackH - thumbH) * tv.scrollY / maxS
	}

	return image.Rect(sr.Min.X+2, sr.Min.Y+thumbY, sr.Max.X-2, sr.Min.Y+thumbY+thumbH)
}

// contentWidth возвращает ширину области контента (без скроллбара).
func (tv *TreeView) contentWidth() int {
	w := tv.bounds.Dx()
	if tv.needsScrollbar() {
		w -= scrollbarWidth
	}
	return w
}

// ─── ApplyTheme ────────────────────────────────────────────────────────────

// ApplyTheme применяет тему.
func (tv *TreeView) ApplyTheme(theme *TreeViewTheme) {
	if theme != nil {
		tv.Theme = *theme
	}
}

// ─── ScrollBy (для колеса мыши) ────────────────────────────────────────────

// ScrollBy прокручивает на delta пикселей.
func (tv *TreeView) ScrollBy(delta int) {
	tv.scrollY += delta
	tv.clampScroll()
}
