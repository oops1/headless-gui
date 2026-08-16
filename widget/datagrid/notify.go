// Package datagrid — INotifyPropertyChanged и ObservableCollection.
//
// Реализует паттерн наблюдателя для обновления UI при изменении данных:
//   - INotifyPropertyChanged: уведомление об изменении свойства объекта
//   - ObservableCollection: коллекция с уведомлением о добавлении/удалении
package datagrid

import (
	"reflect"
	"sync"
)

// ─── PropertyChangedHandler ────────────────────────────────────────────────

// PropertyChangedHandler — обработчик изменения свойства.
// propertyName — имя изменённого свойства ("" = все свойства).
type PropertyChangedHandler func(sender interface{}, propertyName string)

// INotifyPropertyChanged — интерфейс уведомления об изменении свойства (WPF).
type INotifyPropertyChanged interface {
	AddPropertyChanged(handler PropertyChangedHandler)
	RemovePropertyChanged(handler PropertyChangedHandler)
}

// ─── PropertyNotifier (базовая реализация) ─────────────────────────────────

// pcEntry — запись обработчика с уникальным id.
type pcEntry struct {
	id int
	h  PropertyChangedHandler
}

// PropertyNotifier — встраиваемая структура для реализации INotifyPropertyChanged.
// Использование:
//
//	type User struct {
//	    PropertyNotifier
//	    name string
//	}
//
//	func (u *User) SetName(v string) {
//	    u.name = v
//	    u.NotifyPropertyChanged(u, "Name")
//	}
type PropertyNotifier struct {
	mu      sync.RWMutex
	entries []pcEntry
	nextID  int
}

// AddPropertyChangedHandle регистрирует обработчик и возвращает id для удаления.
func (pn *PropertyNotifier) AddPropertyChangedHandle(handler PropertyChangedHandler) int {
	pn.mu.Lock()
	defer pn.mu.Unlock()
	id := pn.nextID
	pn.nextID++
	pn.entries = append(pn.entries, pcEntry{id: id, h: handler})
	return id
}

// RemovePropertyChangedHandle удаляет обработчик по id (no-op, если нет).
func (pn *PropertyNotifier) RemovePropertyChangedHandle(id int) {
	pn.mu.Lock()
	defer pn.mu.Unlock()
	for i, e := range pn.entries {
		if e.id == id {
			pn.entries = append(pn.entries[:i], pn.entries[i+1:]...)
			return
		}
	}
}

// HandlerCount возвращает число подписчиков (для тестов).
func (pn *PropertyNotifier) HandlerCount() int {
	pn.mu.RLock()
	defer pn.mu.RUnlock()
	return len(pn.entries)
}

// AddPropertyChanged регистрирует обработчик (id игнорируется; для совместимости).
func (pn *PropertyNotifier) AddPropertyChanged(handler PropertyChangedHandler) {
	pn.AddPropertyChangedHandle(handler)
}

// RemovePropertyChanged убирает обработчик по функции. Сохраняет
// совместимость с интерфейсом INotifyPropertyChanged.
//
// Раньше метод снимал ПОСЛЕДНИЙ добавленный обработчик, чей бы он ни был —
// молча убивая чужую подписку (аудит SEC-11). Функции в Go несравнимы, но
// сравнимы их указатели на код: снимаем последний обработчик с тем же
// адресом функции — это ровно тот, что был передан. Для замыканий с одним
// телом, но разными захватами адрес совпадает; поэтому надёжный путь —
// AddPropertyChangedHandle/RemovePropertyChangedHandle по дескриптору.
func (pn *PropertyNotifier) RemovePropertyChanged(handler PropertyChangedHandler) {
	if handler == nil {
		return
	}
	want := reflect.ValueOf(handler).Pointer()
	pn.mu.Lock()
	defer pn.mu.Unlock()
	for i := len(pn.entries) - 1; i >= 0; i-- {
		if reflect.ValueOf(pn.entries[i].h).Pointer() == want {
			pn.entries = append(pn.entries[:i], pn.entries[i+1:]...)
			return
		}
	}
}

// NotifyPropertyChanged уведомляет все зарегистрированные обработчики.
func (pn *PropertyNotifier) NotifyPropertyChanged(sender interface{}, propertyName string) {
	pn.mu.RLock()
	snap := make([]PropertyChangedHandler, len(pn.entries))
	for i, e := range pn.entries {
		snap[i] = e.h
	}
	pn.mu.RUnlock()

	for _, h := range snap {
		h(sender, propertyName)
	}
}

// ─── CollectionChangedAction ───────────────────────────────────────────────

// CollectionChangedAction — тип изменения коллекции.
type CollectionChangedAction int

const (
	// CollectionAdd — добавлен элемент.
	CollectionAdd CollectionChangedAction = iota
	// CollectionRemove — удалён элемент.
	CollectionRemove
	// CollectionReplace — заменён элемент.
	CollectionReplace
	// CollectionReset — коллекция полностью изменена.
	CollectionReset
)

// CollectionChangedEvent — событие изменения коллекции.
type CollectionChangedEvent struct {
	Action   CollectionChangedAction
	Index    int         // индекс затронутого элемента
	OldItem  interface{} // для Replace/Remove
	NewItem  interface{} // для Add/Replace
}

// CollectionChangedHandler — обработчик изменения коллекции.
type CollectionChangedHandler func(event CollectionChangedEvent)

// ─── ObservableCollection ──────────────────────────────────────────────────

// ObservableCollection — коллекция с уведомлением о изменениях (WPF ObservableCollection<T>).
type ObservableCollection struct {
	mu      sync.RWMutex
	items   []interface{}
	entries []collectionEntry
	nextID  int
}

// collectionEntry — подписчик коллекции с дескриптором для отписки.
type collectionEntry struct {
	id int
	h  CollectionChangedHandler
}

// NewObservableCollection создаёт пустую наблюдаемую коллекцию.
func NewObservableCollection() *ObservableCollection {
	return &ObservableCollection{}
}

// NewObservableCollectionFrom создаёт коллекцию из среза элементов.
func NewObservableCollectionFrom(items []interface{}) *ObservableCollection {
	oc := &ObservableCollection{
		items: make([]interface{}, len(items)),
	}
	copy(oc.items, items)
	return oc
}

// AddCollectionChanged регистрирует обработчик изменений коллекции.
// Возвращает дескриптор для RemoveCollectionChanged.
//
// Отписка — не украшение: каждый SetItemsSource у DataGrid/TreeView/
// CollectionView подписывает замыкание, захватывающее виджет; без снятия
// подписки N перебиндовок = N живых обработчиков — утечка деревьев виджетов
// и кратное размножение работы на каждое изменение (аудит SEC-11).
func (oc *ObservableCollection) AddCollectionChanged(handler CollectionChangedHandler) int {
	if handler == nil {
		return -1
	}
	oc.mu.Lock()
	defer oc.mu.Unlock()
	oc.nextID++
	id := oc.nextID
	oc.entries = append(oc.entries, collectionEntry{id: id, h: handler})
	return id
}

// RemoveCollectionChanged снимает подписку по дескриптору (no-op для
// неизвестного/отрицательного id).
func (oc *ObservableCollection) RemoveCollectionChanged(id int) {
	if id <= 0 {
		return
	}
	oc.mu.Lock()
	defer oc.mu.Unlock()
	for i, e := range oc.entries {
		if e.id == id {
			oc.entries = append(oc.entries[:i], oc.entries[i+1:]...)
			return
		}
	}
}

// HandlerCount возвращает число подписчиков (для тестов).
func (oc *ObservableCollection) HandlerCount() int {
	oc.mu.RLock()
	defer oc.mu.RUnlock()
	return len(oc.entries)
}

func (oc *ObservableCollection) notify(event CollectionChangedEvent) {
	oc.mu.RLock()
	handlers := make([]CollectionChangedHandler, len(oc.entries))
	for i, e := range oc.entries {
		handlers[i] = e.h
	}
	oc.mu.RUnlock()
	for _, h := range handlers {
		h(event)
	}
}

// Count возвращает количество элементов.
func (oc *ObservableCollection) Count() int {
	oc.mu.RLock()
	defer oc.mu.RUnlock()
	return len(oc.items)
}

// Get возвращает элемент по индексу.
func (oc *ObservableCollection) Get(index int) interface{} {
	oc.mu.RLock()
	defer oc.mu.RUnlock()
	if index < 0 || index >= len(oc.items) {
		return nil
	}
	return oc.items[index]
}

// Items возвращает копию среза элементов.
func (oc *ObservableCollection) Items() []interface{} {
	oc.mu.RLock()
	defer oc.mu.RUnlock()
	result := make([]interface{}, len(oc.items))
	copy(result, oc.items)
	return result
}

// Add добавляет элемент в конец.
func (oc *ObservableCollection) Add(item interface{}) {
	oc.mu.Lock()
	idx := len(oc.items)
	oc.items = append(oc.items, item)
	oc.mu.Unlock()
	oc.notify(CollectionChangedEvent{
		Action:  CollectionAdd,
		Index:   idx,
		NewItem: item,
	})
}

// Insert вставляет элемент по индексу.
func (oc *ObservableCollection) Insert(index int, item interface{}) {
	oc.mu.Lock()
	if index < 0 || index > len(oc.items) {
		oc.mu.Unlock()
		return
	}
	oc.items = append(oc.items, nil)
	copy(oc.items[index+1:], oc.items[index:])
	oc.items[index] = item
	oc.mu.Unlock()
	oc.notify(CollectionChangedEvent{
		Action:  CollectionAdd,
		Index:   index,
		NewItem: item,
	})
}

// RemoveAt удаляет элемент по индексу.
func (oc *ObservableCollection) RemoveAt(index int) {
	oc.mu.Lock()
	if index < 0 || index >= len(oc.items) {
		oc.mu.Unlock()
		return
	}
	old := oc.items[index]
	oc.items = append(oc.items[:index], oc.items[index+1:]...)
	oc.mu.Unlock()
	oc.notify(CollectionChangedEvent{
		Action:  CollectionRemove,
		Index:   index,
		OldItem: old,
	})
}

// Set заменяет элемент по индексу.
func (oc *ObservableCollection) Set(index int, item interface{}) {
	oc.mu.Lock()
	if index < 0 || index >= len(oc.items) {
		oc.mu.Unlock()
		return
	}
	old := oc.items[index]
	oc.items[index] = item
	oc.mu.Unlock()
	oc.notify(CollectionChangedEvent{
		Action:  CollectionReplace,
		Index:   index,
		OldItem: old,
		NewItem: item,
	})
}

// Clear удаляет все элементы.
func (oc *ObservableCollection) Clear() {
	oc.mu.Lock()
	oc.items = oc.items[:0]
	oc.mu.Unlock()
	oc.notify(CollectionChangedEvent{
		Action: CollectionReset,
		Index:  -1,
	})
}

// SetItems полностью заменяет содержимое коллекции.
func (oc *ObservableCollection) SetItems(items []interface{}) {
	oc.mu.Lock()
	oc.items = make([]interface{}, len(items))
	copy(oc.items, items)
	oc.mu.Unlock()
	oc.notify(CollectionChangedEvent{
		Action: CollectionReset,
		Index:  -1,
	})
}
