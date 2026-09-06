package treeview

import (
	"image"
	"image/color"
	"log"
	"reflect"
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
	// itemsSourceSub — дескриптор подписки на itemsSource. Без него каждая
	// перебиндовка оставляла живой обработчик, захватывающий этот TreeView:
	// N вызовов SetItemsSource = N перестроений дерева на каждое изменение
	// коллекции и утечка самого дерева (аудит SEC-11). 0 = подписки нет.
	itemsSourceSub int
	itemTemplate   *HierarchicalDataTemplate

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

	// ItemStyle — оформление строки узла: цвет текста и начертание.
	//
	// Колбэк, а не только поля узла: «активный репозиторий» — состояние
	// ПРИЛОЖЕНИЯ, а не узла, и держать его копию в каждом узле значит
	// синхронизировать её при каждом переключении. Возврат ok=false означает
	// «решай сам» — тогда смотрятся поля Foreground и Bold самого узла.
	//
	// Зовётся на каждый видимый узел в каждом кадре: тяжёлой работы внутри
	// быть не должно.
	ItemStyle func(item *TreeViewItem) (fg color.RGBA, bold bool, ok bool)

	// CanUserDragNodes — можно ли переставлять узлы перетаскиванием мышью
	// (dragdrop.go).
	//
	// По умолчанию ВЫКЛЮЧЕНО: перетаскивание меняет поведение обычного
	// щелчка — уведённая на несколько точек мышь переставляет узел, — и
	// включать это молча у всех, кто дерево уже использует, нельзя.
	CanUserDragNodes bool

	// CanDropNode — разрешает или запрещает конкретный перенос.
	//
	// Структурные запреты (узел в самого себя и в собственного потомка)
	// дерево проверяет само: ошибиться в них легко, а последствия у них не
	// «неудобно», а «часть данных исчезла с экрана».
	CanDropNode func(dragged, target *TreeViewItem, pos DropPosition) bool

	// OnNodeDrop — узел отпустили над целью.
	//
	// Возврат true означает «справился сам»: дерево не станет переставлять
	// узлы. Так приложение, у которого источник правды — своя модель,
	// перестраивает дерево из неё, а не получает перестановку за спиной.
	// Возврат false (и nil-колбэк) — дерево переставляет узел само.
	OnNodeDrop func(dragged, target *TreeViewItem, pos DropPosition) bool

	// Обратная совместимость: простой callback как в старом TreeView
	OnSelect func(item *TreeViewItem)

	// ── Внутреннее ───────────────────────────────────────────────────────
	// Перетаскивание узла (dragdrop.go). dragItem == nil — нажатия нет.
	dragItem   *TreeViewItem
	dragStartY int
	dragging   bool
	dropTarget *TreeViewItem
	dropPos    DropPosition
	dropIdx    int

	mu       sync.Mutex
	focused  bool
	dirty    bool // нужен пересчёт flat-списка
	updating bool // true между BeginUpdate/EndUpdate — Draw пропускается

	// ── Кэш плоского списка видимых узлов (PERF-10) ──────────────────────
	// Раньше visibleNodes() обходил дерево и аллоцировал новый срез на
	// КАЖДЫЙ Draw, mousemove, клик и нажатие клавиши (а внутри одного
	// обработчика — по три-четыре раза), при том что флаг dirty исправно
	// выставлялся в девяти местах и не читался нигде. Теперь список
	// пересобирается только при dirty, а поиск индекса узла идёт по карте
	// вместо линейного скана.
	//
	// Пересборка ВСЕГДА выделяет новый срез и новую карту — уже отданные
	// вызывающему срезы остаются неизменными, поэтому чтение их вне лока
	// безопасно.
	flatCache []flatItem
	flatIndex map[*TreeViewItem]int

	// ── Точечная инвалидация (damage) ────────────────────────────────────
	// Обработчики ввода накапливают сюда изменившиеся АБСОЛЮТНЫЕ строки;
	// обёртка (widget.TreeViewWidget) забирает их через TakeDirty и делает
	// точечный InvalidateRect. dirtyFull — когда сдвигается весь контент
	// (скролл, разворот/сворачивание узла меняет набор видимых строк).
	dirtyRects []image.Rectangle
	dirtyFull  bool
}

// ─── Damage tracking (точечная инвалидация) ────────────────────────────────

// markFullDirty помечает, что требуется полная перерисовка виджета.
func (tv *TreeView) markFullDirty() { tv.dirtyFull = true }

// markRectDirty добавляет абсолютный прямоугольник в накопитель damage
// (пересекается с bounds, дубликаты отбрасываются).
func (tv *TreeView) markRectDirty(r image.Rectangle) {
	r = r.Intersect(tv.bounds)
	if r.Empty() {
		return
	}
	for _, e := range tv.dirtyRects {
		if e == r {
			return
		}
	}
	tv.dirtyRects = append(tv.dirtyRects, r)
}

// rowRectAbs возвращает абсолютный прямоугольник видимой части строки с
// индексом idx в flat-списке (пустой, если строка прокручена за пределы).
func (tv *TreeView) rowRectAbs(idx int) image.Rectangle {
	if idx < 0 {
		return image.Rectangle{}
	}
	ih := tv.itemH()
	b := tv.bounds
	y := b.Min.Y + idx*ih - tv.scrollY
	return image.Rect(b.Min.X, y, b.Max.X, y+ih).Intersect(b)
}

// markRowDirty помечает область строки idx как изменившуюся.
func (tv *TreeView) markRowDirty(idx int) { tv.markRectDirty(tv.rowRectAbs(idx)) }

// TakeDirty возвращает накопленные области изменения (абсолютные координаты) и
// сбрасывает накопитель. full=true — требуется полная перерисовка виджета.
func (tv *TreeView) TakeDirty() (rects []image.Rectangle, full bool) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	rects, full = tv.dirtyRects, tv.dirtyFull
	tv.dirtyRects = nil
	tv.dirtyFull = false
	return
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
func (tv *TreeView) SetBounds(r image.Rectangle) { tv.bounds = r; tv.InvalidateLayout() }

// ─── Batch update (двойная буферизация) ────────────────────────────────────

// BeginUpdate приостанавливает отрисовку.
// Все структурные изменения (AddRoot, ClearRoots, SetRoots, AddChild и т.д.)
// накапливаются, а Draw возвращается сразу без отрисовки.
// Вызовите EndUpdate, чтобы применить изменения и отрисовать дерево целиком.
func (tv *TreeView) BeginUpdate() {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.updating = true
}

// EndUpdate возобновляет отрисовку и помечает дерево для пересчёта.
func (tv *TreeView) EndUpdate() {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.updating = false
	tv.dirty = true
}

// IsUpdating возвращает true, если отрисовка приостановлена.
func (tv *TreeView) IsUpdating() bool {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	return tv.updating
}

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
//
// Подписка на прежний источник снимается (SEC-11): иначе N перебиндовок
// оставляли N живых обработчиков, каждый из которых перестраивал дерево на
// любое изменение коллекции и удерживал этот TreeView от сборки.
func (tv *TreeView) SetItemsSource(oc *datagrid.ObservableCollection) {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	if tv.itemsSource != nil && tv.itemsSourceSub > 0 {
		tv.itemsSource.RemoveCollectionChanged(tv.itemsSourceSub)
	}
	tv.itemsSourceSub = 0
	tv.itemsSource = oc
	tv.dirty = true

	// Подписываемся на изменения
	if oc != nil {
		tv.itemsSourceSub = oc.AddCollectionChanged(func(e datagrid.CollectionChangedEvent) {
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
	// Состояние сборки одно на всю перестройку: путь очищается при выходе из
	// каждого узла, а флаги «уже сообщили» дают ровно одну запись в журнал
	// на перестройку, а не по одной на каждый корень.
	st := &treeBuildState{path: map[nodeKey]bool{}}
	for _, dataObj := range items {
		item := tv.buildItemFromData(dataObj, tmpl, 0, st)
		tv.roots = append(tv.roots, item)
	}
	tv.dirty = true
}

// ─── Построение дерева из данных ───────────────────────────────────────────

// maxTreeDepth — предельная глубина дерева, строимого из ItemsSource (SEC-7).
// Обход рекурсивен: модель, у которой ItemsSourcePath уводит вглубь без
// конца, исчерпывала стек. Предел с запасом над любым реальным деревом.
const maxTreeDepth = 256

// nodeKey — идентичность объекта данных для детекции циклов. Сравнивать сами
// interface{} нельзя: у несравнимого динамического типа (структура со срезом)
// это паника в рантайме. Поэтому берём адрес — у указателей, карт и каналов
// он есть, а именно ими и связывают узлы в иерархических моделях
// (`type Node struct{ Children []*Node }`).
type nodeKey struct {
	ptr uintptr
	typ reflect.Type
}

// nodeIdentity возвращает идентичность объекта данных, если она определима.
func nodeIdentity(v interface{}) (nodeKey, bool) {
	if v == nil {
		return nodeKey{}, false
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Chan, reflect.UnsafePointer:
		if rv.IsNil() {
			return nodeKey{}, false
		}
		return nodeKey{ptr: rv.Pointer(), typ: rv.Type()}, true
	}
	return nodeKey{}, false // значимый тип — цикл через него невозможен
}

// treeBuildState — состояние одной сборки поддерева: множество объектов на
// ПУТИ от корня (не «всех виденных» — иначе один и тот же узел, честно
// упомянутый у двух разных родителей, был бы ошибочно обрезан) и флаги
// «уже сообщили», чтобы не залить журнал повторами.
type treeBuildState struct {
	path      map[nodeKey]bool
	cycleLog  bool
	depthLog  bool
}

// createItemFromData создаёт TreeViewItem из объекта данных используя шаблон.
func (tv *TreeView) createItemFromData(dataObj interface{}, tmpl *HierarchicalDataTemplate, depth int) *TreeViewItem {
	return tv.buildItemFromData(dataObj, tmpl, depth, &treeBuildState{path: map[nodeKey]bool{}})
}

// buildItemFromData — рекурсивное тело createItemFromData с защитой от
// циклических моделей и от чрезмерной глубины (SEC-7). При обнаружении цикла
// узел создаётся, но НЕ раскрывается — дерево остаётся конечным, а данные
// пользователя не теряются молча (в журнал уходит одно сообщение на сборку).
func (tv *TreeView) buildItemFromData(dataObj interface{}, tmpl *HierarchicalDataTemplate, depth int, st *treeBuildState) *TreeViewItem {
	item := NewItem("")
	item.DataContext = dataObj
	item.depth = depth
	item.owner = tv

	if tmpl == nil {
		return item
	}

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

	if depth >= maxTreeDepth {
		if !st.depthLog {
			st.depthLog = true
			log.Printf("treeview: глубина модели превысила %d — поддерево не раскрыто", maxTreeDepth)
		}
		return item
	}

	// Детекция цикла: объект уже встречался на пути от корня.
	key, hasKey := nodeIdentity(dataObj)
	if hasKey {
		if st.path[key] {
			if !st.cycleLog {
				st.cycleLog = true
				log.Printf("treeview: в модели ItemsSource обнаружен цикл — узел не раскрыт")
			}
			return item
		}
		st.path[key] = true
		defer delete(st.path, key) // снимаем при выходе — это путь, а не «всё виденное»
	}

	// Дочерние элементы
	for _, childData := range tmpl.resolveChildren(dataObj) {
		child := tv.buildItemFromData(childData, tmpl, depth+1, st)
		child.parent = item
		item.Children = append(item.Children, child)
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
	// Под локом — тем же, что защищает кэш flat-списка: раскрытие меняет
	// набор видимых строк, и кэш обязан протухнуть согласованно.
	// Пользовательский обработчик вызывается уже без лока.
	tv.mu.Lock()
	item.Expanded = true
	tv.dirty = true
	tv.mu.Unlock()
	if tv.OnExpanded != nil {
		tv.OnExpanded(ExpandedEvent{Item: item})
	}
}

// CollapseItem сворачивает узел.
func (tv *TreeView) CollapseItem(item *TreeViewItem) {
	if item == nil || !item.Expanded {
		return
	}
	tv.mu.Lock()
	item.Expanded = false
	tv.dirty = true
	tv.mu.Unlock()
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
//
// Список кэшируется и пересобирается только когда модель менялась (dirty).
// Кэш читается и пишется под tv.mu — тем же локом, что защищает roots, —
// поэтому одновременные Draw и обработка ввода не гонятся за него.
func (tv *TreeView) visibleNodes() []flatItem {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	return tv.visibleNodesLocked()
}

// visibleNodesLocked — тело visibleNodes; вызывать под tv.mu.
func (tv *TreeView) visibleNodesLocked() []flatItem {
	if !tv.dirty && tv.flatCache != nil {
		return tv.flatCache
	}
	var result []flatItem
	for _, root := range tv.roots {
		collectVisible(root, root.depth, &result)
	}
	idx := make(map[*TreeViewItem]int, len(result))
	for i, fi := range result {
		// Первое вхождение — как у прежнего линейного скана: один и тот же
		// узел может попасть в список дважды (его подцепили к двум родителям),
		// и индекс обязан совпадать с тем, что вернул бы поиск перебором.
		if _, dup := idx[fi.item]; !dup {
			idx[fi.item] = i
		}
	}
	tv.flatCache = result
	tv.flatIndex = idx
	tv.dirty = false
	return result
}

// InvalidateLayout помечает плоский список видимых узлов устаревшим.
//
// Нужен, если структура дерева или раскрытие узлов менялись в обход методов
// TreeView/TreeViewItem — например прямой записью в публичное поле
// item.Expanded или item.Children. Методы (AddRoot, SetRoots, AddChild,
// ExpandItem, …) помечают кэш сами.
func (tv *TreeView) InvalidateLayout() {
	tv.mu.Lock()
	tv.dirty = true
	tv.mu.Unlock()
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
//
// Пока flat — это актуальный кэш (обычный случай), поиск идёт по карте,
// построенной при пересборке, а не линейным сканом (PERF-10). Если передан
// чужой/устаревший срез, остаётся линейный поиск по нему — результат обязан
// соответствовать именно переданному списку.
func (tv *TreeView) indexOfItem(item *TreeViewItem, flat []flatItem) int {
	if item == nil {
		return -1
	}
	tv.mu.Lock()
	if tv.flatIndex != nil && len(flat) == len(tv.flatCache) &&
		(len(flat) == 0 || &flat[0] == &tv.flatCache[0]) {
		i, ok := tv.flatIndex[item]
		tv.mu.Unlock()
		if !ok {
			return -1
		}
		return i
	}
	tv.mu.Unlock()

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

// ScrollY возвращает текущее вертикальное смещение прокрутки (в пикселях).
func (tv *TreeView) ScrollY() int {
	return tv.scrollY
}

// WheelScroll прокручивает дерево колесом мыши на 3 строки за тик
// (up=true — вверх, иначе вниз). Возвращает true, если прокрутка
// фактически сдвинулась — обёртка использует это, чтобы поглощать
// событие ТОЛЬКО когда есть что прокручивать (иначе колесо всплывает
// к родительскому ScrollView).
func (tv *TreeView) WheelScroll(up bool) bool {
	if tv.maxScrollY() == 0 {
		return false
	}
	old := tv.scrollY
	step := 3 * tv.itemH()
	if up {
		tv.scrollY -= step
	} else {
		tv.scrollY += step
	}
	tv.clampScroll()
	return tv.scrollY != old
}
