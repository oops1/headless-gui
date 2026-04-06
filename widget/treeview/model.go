// Package treeview — WPF-совместимый TreeView с Data Binding и HierarchicalDataTemplate.
//
// Аналог WPF System.Windows.Controls.TreeView.
// Поддерживает:
//   - Иерархическая модель данных (TreeViewItem)
//   - ItemsSource с ObservableCollection
//   - HierarchicalDataTemplate
//   - Data Binding (Path, Converter, StringFormat)
//   - INotifyPropertyChanged
//   - Виртуализация (рендерятся только видимые узлы)
//   - Клавиатурная навигация
//   - Иконки
package treeview

import (
	"image"
	"sync"

	"github.com/oops1/headless-gui/v3/widget/datagrid"
)

// ─── TreeViewItem ──────────────────────────────────────────────────────────

// TreeViewItem — узел дерева (аналог WPF TreeViewItem).
// Может содержать статические дочерние элементы (Children)
// или привязанную коллекцию (ItemsSource).
//
// Поле Text совместимо со старым TreeNode.Text.
// Поле Header — WPF-совместимый алиас.
type TreeViewItem struct {
	datagrid.PropertyNotifier

	// Text — текст узла (обратная совместимость с TreeNode.Text).
	Text string

	// Header — WPF-совместимый алиас для Text.
	// При чтении: если Header != "", возвращается Header, иначе Text.
	// При установке: устанавливает то же значение что и Text.
	Header string

	// Icon — опциональная иконка (отображается перед текстом).
	Icon image.Image

	// Expanded — раскрыт ли узел (обратная совместимость с TreeNode.Expanded).
	Expanded bool

	// IsSelected — выбран ли узел.
	IsSelected bool

	// IsEnabled — включён ли узел (по умолчанию true).
	IsEnabled bool

	// Tag — произвольные данные, привязанные к узлу.
	Tag interface{}

	// DataContext — объект данных для биндинга (используется с HierarchicalDataTemplate).
	DataContext interface{}

	// Children — дочерние узлы (прямой доступ для обратной совместимости).
	Children []*TreeViewItem

	// ── Привязанная коллекция ────────────────────────────────────────────
	mu          sync.RWMutex
	itemsSource *datagrid.ObservableCollection

	// ── Внутренние поля ─────────────────────────────────────────────────
	parent *TreeViewItem
	depth  int      // уровень вложенности (0 = корень)
	owner  *TreeView // ссылка на TreeView-владельца
}

// NewItem создаёт узел с заданным заголовком.
func NewItem(header string) *TreeViewItem {
	return &TreeViewItem{
		Text:      header,
		Header:    header,
		IsEnabled: true,
	}
}

// DisplayText возвращает текст для отображения.
// Приоритет: Header > Text.
func (item *TreeViewItem) DisplayText() string {
	if item.Header != "" {
		return item.Header
	}
	return item.Text
}

// AddChild добавляет дочерний узел.
func (item *TreeViewItem) AddChild(child *TreeViewItem) {
	child.parent = item
	child.depth = item.depth + 1
	item.Children = append(item.Children, child)

	// Рекурсивно обновляем depth и owner
	if item.owner != nil {
		child.setOwnerRecursive(item.owner)
	}
}

// InsertChild вставляет дочерний узел по индексу.
func (item *TreeViewItem) InsertChild(index int, child *TreeViewItem) {
	if index < 0 {
		index = 0
	}
	if index > len(item.Children) {
		index = len(item.Children)
	}

	child.parent = item
	child.depth = item.depth + 1

	item.Children = append(item.Children, nil)
	copy(item.Children[index+1:], item.Children[index:])
	item.Children[index] = child

	if item.owner != nil {
		child.setOwnerRecursive(item.owner)
	}
}

// RemoveChild удаляет дочерний узел.
func (item *TreeViewItem) RemoveChild(child *TreeViewItem) {
	for i, c := range item.Children {
		if c == child {
			item.Children = append(item.Children[:i], item.Children[i+1:]...)
			child.parent = nil
			child.setOwnerRecursive(nil)
			return
		}
	}
}

// RemoveChildAt удаляет дочерний узел по индексу.
func (item *TreeViewItem) RemoveChildAt(index int) {
	if index < 0 || index >= len(item.Children) {
		return
	}
	child := item.Children[index]
	item.Children = append(item.Children[:index], item.Children[index+1:]...)
	child.parent = nil
	child.setOwnerRecursive(nil)
}

// HasChildren возвращает true, если у узла есть дочерние элементы.
func (item *TreeViewItem) HasChildren() bool {
	return len(item.Children) > 0
}

// Parent возвращает родительский узел (nil для корневых).
func (item *TreeViewItem) Parent() *TreeViewItem {
	return item.parent
}

// Depth возвращает уровень вложенности (0 = корень).
func (item *TreeViewItem) Depth() int {
	return item.depth
}

// ClearChildren удаляет все дочерние узлы.
func (item *TreeViewItem) ClearChildren() {
	for _, c := range item.Children {
		c.parent = nil
		c.setOwnerRecursive(nil)
	}
	item.Children = item.Children[:0]
}

// SetItemsSource привязывает ObservableCollection для автоматической
// генерации дочерних узлов через HierarchicalDataTemplate.
func (item *TreeViewItem) SetItemsSource(oc *datagrid.ObservableCollection) {
	item.mu.Lock()
	defer item.mu.Unlock()
	item.itemsSource = oc
}

// ItemsSource возвращает привязанную коллекцию.
func (item *TreeViewItem) ItemsSource() *datagrid.ObservableCollection {
	item.mu.RLock()
	defer item.mu.RUnlock()
	return item.itemsSource
}

// setOwnerRecursive рекурсивно устанавливает владельца TreeView.
func (item *TreeViewItem) setOwnerRecursive(tv *TreeView) {
	item.owner = tv
	for _, child := range item.Children {
		child.setOwnerRecursive(tv)
	}
}

// updateDepthRecursive пересчитывает depth для узла и всех потомков.
func (item *TreeViewItem) updateDepthRecursive(depth int) {
	item.depth = depth
	for _, child := range item.Children {
		child.updateDepthRecursive(depth + 1)
	}
}

// ─── flatItem ──────────────────────────────────────────────────────────────

// flatItem — видимый узел с глубиной вложенности (для плоского рендеринга).
type flatItem struct {
	item  *TreeViewItem
	depth int
}

// collectVisible собирает плоский список видимых узлов.
func collectVisible(item *TreeViewItem, depth int, out *[]flatItem) {
	*out = append(*out, flatItem{item: item, depth: depth})
	if item.Expanded {
		for _, child := range item.Children {
			collectVisible(child, depth+1, out)
		}
	}
}
