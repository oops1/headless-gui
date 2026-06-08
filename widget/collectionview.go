package widget

// collectionview.go — CollectionView в стиле WPF (ICollectionView / CollectionViewSource).
//
// Оборачивает источник данных (ObservableCollection или срез) и предоставляет
// представление с сортировкой, фильтрацией и группировкой. ItemsControl,
// привязанный к CollectionView, перестраивается при изменении источника или
// параметров представления (Filter / Sort / Group).
//
// Пример:
//
//	view := widget.NewCollectionView(people)          // people — *ObservableCollection
//	view.SetFilter(func(it any) bool {                // только взрослые
//	    p := it.(*Person); return p.Age >= 18
//	})
//	view.AddSort(widget.SortDescription{Property: "Name"})
//	view.SetGroup("City")
//	// привязать к ItemsControl: DataContext.People = view

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	dgridPkg "github.com/oops1/headless-gui/v3/widget/datagrid"
)

// SortDirection — направление сортировки.
type SortDirection int

const (
	Ascending  SortDirection = iota // по возрастанию (по умолчанию)
	Descending                      // по убыванию
)

// SortDescription — правило сортировки по свойству (WPF SortDescription).
type SortDescription struct {
	Property  string // имя свойства элемента ("" = сам элемент)
	Direction SortDirection
}

// CollectionViewGroup — группа элементов представления (WPF CollectionViewGroup).
type CollectionViewGroup struct {
	Name  interface{}   // значение группового ключа
	Items []interface{} // элементы группы (в порядке представления)
}

// CollectionView — представление коллекции с сортировкой/фильтром/группировкой.
type CollectionView struct {
	mu       sync.Mutex
	source   interface{}
	filter   func(interface{}) bool
	sorts    []SortDescription
	groupBy  string
	view     []interface{}
	groups   []CollectionViewGroup
	handlers []func()
}

// NewCollectionView создаёт представление поверх источника (ObservableCollection
// или срез) и сразу вычисляет начальное представление. Если источник —
// ObservableCollection, представление подписывается на его изменения.
func NewCollectionView(source interface{}) *CollectionView {
	v := &CollectionView{source: source}
	if oc, ok := source.(*dgridPkg.ObservableCollection); ok {
		oc.AddCollectionChanged(func(dgridPkg.CollectionChangedEvent) {
			v.Refresh()
		})
	}
	v.Refresh()
	return v
}

// AddViewChanged регистрирует слушателя пересчёта представления.
func (v *CollectionView) AddViewChanged(h func()) {
	v.mu.Lock()
	v.handlers = append(v.handlers, h)
	v.mu.Unlock()
}

// SetFilter задаёт предикат фильтрации (nil — без фильтра) и пересчитывает.
func (v *CollectionView) SetFilter(f func(interface{}) bool) {
	v.mu.Lock()
	v.filter = f
	v.mu.Unlock()
	v.Refresh()
}

// SetSort заменяет правила сортировки целиком и пересчитывает.
func (v *CollectionView) SetSort(sorts ...SortDescription) {
	v.mu.Lock()
	v.sorts = append(v.sorts[:0], sorts...)
	v.mu.Unlock()
	v.Refresh()
}

// AddSort добавляет правило сортировки (вторичный ключ и т.д.) и пересчитывает.
func (v *CollectionView) AddSort(s SortDescription) {
	v.mu.Lock()
	v.sorts = append(v.sorts, s)
	v.mu.Unlock()
	v.Refresh()
}

// ClearSort убирает все правила сортировки.
func (v *CollectionView) ClearSort() {
	v.mu.Lock()
	v.sorts = nil
	v.mu.Unlock()
	v.Refresh()
}

// SetGroup задаёт свойство группировки ("" — без группировки) и пересчитывает.
func (v *CollectionView) SetGroup(property string) {
	v.mu.Lock()
	v.groupBy = property
	v.mu.Unlock()
	v.Refresh()
}

// SetSource заменяет источник данных и пересчитывает представление.
func (v *CollectionView) SetSource(source interface{}) {
	v.mu.Lock()
	v.source = source
	v.mu.Unlock()
	if oc, ok := source.(*dgridPkg.ObservableCollection); ok {
		oc.AddCollectionChanged(func(dgridPkg.CollectionChangedEvent) {
			v.Refresh()
		})
	}
	v.Refresh()
}

// Items возвращает текущее представление (отфильтровано+отсортировано).
func (v *CollectionView) Items() []interface{} {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]interface{}, len(v.view))
	copy(out, v.view)
	return out
}

// Groups возвращает группы текущего представления (пусто, если группировка off).
func (v *CollectionView) Groups() []CollectionViewGroup {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]CollectionViewGroup, len(v.groups))
	copy(out, v.groups)
	return out
}

// Count возвращает число элементов в представлении.
func (v *CollectionView) Count() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.view)
}

// Refresh пересчитывает представление (фильтр → сортировка → группировка) и
// уведомляет слушателей.
func (v *CollectionView) Refresh() {
	v.mu.Lock()
	raw := collectionItems(v.source)
	filter := v.filter
	sorts := append([]SortDescription(nil), v.sorts...)
	groupBy := v.groupBy

	// 1. Фильтрация.
	out := make([]interface{}, 0, len(raw))
	for _, it := range raw {
		if filter == nil || filter(it) {
			out = append(out, it)
		}
	}

	// 2. Сортировка (стабильная, по нескольким ключам).
	if len(sorts) > 0 {
		sort.SliceStable(out, func(i, j int) bool {
			for _, s := range sorts {
				c := compareItemProp(out[i], out[j], s.Property)
				if c == 0 {
					continue
				}
				if s.Direction == Descending {
					return c > 0
				}
				return c < 0
			}
			return false
		})
	}

	// 3. Группировка (сохраняем порядок появления групп).
	var groups []CollectionViewGroup
	if groupBy != "" {
		idx := map[string]int{}
		for _, it := range out {
			key := propValue(it, groupBy)
			ks := fmtKey(key)
			gi, ok := idx[ks]
			if !ok {
				gi = len(groups)
				idx[ks] = gi
				groups = append(groups, CollectionViewGroup{Name: key})
			}
			groups[gi].Items = append(groups[gi].Items, it)
		}
	}

	v.view = out
	v.groups = groups
	handlers := make([]func(), len(v.handlers))
	copy(handlers, v.handlers)
	v.mu.Unlock()

	for _, h := range handlers {
		h()
	}
}

// ─── helpers сравнения/чтения свойств ──────────────────────────────────────

// propValue читает свойство property у элемента (или сам элемент при "").
func propValue(item interface{}, property string) interface{} {
	if property == "" {
		return item
	}
	if val, ok := dgridPkg.GetPropertyValue(item, property); ok {
		return val
	}
	return nil
}

// compareItemProp сравнивает свойство property двух элементов: -1/0/1.
func compareItemProp(a, b interface{}, property string) int {
	return compareValues(propValue(a, property), propValue(b, property))
}

// compareValues сравнивает два значения: -1 (a<b), 0 (равны), 1 (a>b).
// Поддерживает числа, строки, bool; иначе сравнение по строковому виду.
func compareValues(a, b interface{}) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	ra, rb := reflect.ValueOf(a), reflect.ValueOf(b)

	// Числа (в т.ч. разных типов) — сравнение по float64.
	af, aok := toFloat(ra)
	bf, bok := toFloat(rb)
	if aok && bok {
		switch {
		case af < bf:
			return -1
		case af > bf:
			return 1
		default:
			return 0
		}
	}

	// bool: false < true.
	if ra.Kind() == reflect.Bool && rb.Kind() == reflect.Bool {
		ab, bb := ra.Bool(), rb.Bool()
		switch {
		case !ab && bb:
			return -1
		case ab && !bb:
			return 1
		default:
			return 0
		}
	}

	// Строки и всё остальное — без учёта регистра по строковому представлению.
	as, bs := strings.ToLower(fmtKey(a)), strings.ToLower(fmtKey(b))
	return strings.Compare(as, bs)
}

// toFloat пытается привести числовое значение к float64.
func toFloat(rv reflect.Value) (float64, bool) {
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint()), true
	case reflect.Float32, reflect.Float64:
		return rv.Float(), true
	}
	return 0, false
}

// fmtKey приводит значение к строке (для ключей групп и сравнения).
func fmtKey(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
