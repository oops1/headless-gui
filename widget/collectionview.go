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
	"time"

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
	handlers []viewHandlerEntry // слушатели пересчёта (с дескрипторами, SEC-11)
	nextHID  int

	// srcOC/srcSubID — подписка на текущий источник (SEC-11). Без снятия
	// подписки каждая смена источника оставляла живое замыкание, которое
	// продолжало пересчитывать представление на изменения старой коллекции.
	srcOC    *dgridPkg.ObservableCollection
	srcSubID int

	// seq — номер последнего СТАРТОВАВШЕГО пересчёта. Refresh считает без
	// лока (PERF-8) и публикует результат, только если за это время не
	// начался более свежий пересчёт: иначе устаревший результат затёр бы
	// актуальный.
	seq uint64
}

// NewCollectionView создаёт представление поверх источника (ObservableCollection
// или срез) и сразу вычисляет начальное представление. Если источник —
// ObservableCollection, представление подписывается на его изменения.
func NewCollectionView(source interface{}) *CollectionView {
	v := &CollectionView{}
	v.SetSource(source)
	return v
}

// Dispose снимает подписку на источник (SEC-11).
func (v *CollectionView) Dispose() {
	v.mu.Lock()
	oc, id := v.srcOC, v.srcSubID
	v.srcOC, v.srcSubID = nil, 0
	v.source = nil
	v.mu.Unlock()
	if oc != nil && id > 0 {
		oc.RemoveCollectionChanged(id)
	}
}

// viewHandlerEntry — слушатель пересчёта с дескриптором для отписки.
type viewHandlerEntry struct {
	id int
	h  func()
}

// AddViewChanged регистрирует слушателя пересчёта представления.
// Совместимая обёртка над AddViewChangedHandle без дескриптора — для кода,
// которому отписка не нужна (представление живёт столько же, сколько слушатель).
func (v *CollectionView) AddViewChanged(h func()) {
	v.AddViewChangedHandle(h)
}

// AddViewChangedHandle регистрирует слушателя пересчёта и возвращает
// дескриптор для RemoveViewChanged (SEC-11: без отписки каждая перебиндовка
// BindingScope оставляла живое замыкание с захваченным деревом виджетов).
// nil-обработчик не регистрируется (возвращает -1).
func (v *CollectionView) AddViewChangedHandle(h func()) int {
	if h == nil {
		return -1
	}
	v.mu.Lock()
	v.nextHID++
	id := v.nextHID
	v.handlers = append(v.handlers, viewHandlerEntry{id: id, h: h})
	v.mu.Unlock()
	return id
}

// RemoveViewChanged снимает слушателя по дескриптору из AddViewChangedHandle.
// Неизвестный/повторный дескриптор — no-op.
func (v *CollectionView) RemoveViewChanged(id int) {
	if id <= 0 {
		return
	}
	v.mu.Lock()
	for i, e := range v.handlers {
		if e.id == id {
			v.handlers = append(v.handlers[:i:i], v.handlers[i+1:]...)
			break
		}
	}
	v.mu.Unlock()
}

// ViewHandlerCount возвращает число зарегистрированных слушателей (для тестов).
func (v *CollectionView) ViewHandlerCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.handlers)
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
//
// Подписка со старого источника снимается (SEC-11) — иначе N перебиндовок
// давали N живых обработчиков, и каждое изменение любой из брошенных
// коллекций продолжало гонять фильтр/сортировку этого представления.
func (v *CollectionView) SetSource(source interface{}) {
	v.mu.Lock()
	oldOC, oldID := v.srcOC, v.srcSubID
	v.source = source
	v.srcOC, v.srcSubID = nil, 0
	v.mu.Unlock()

	if oldOC != nil && oldID > 0 {
		oldOC.RemoveCollectionChanged(oldID)
	}

	if oc, ok := source.(*dgridPkg.ObservableCollection); ok && oc != nil {
		id := oc.AddCollectionChanged(func(dgridPkg.CollectionChangedEvent) {
			v.Refresh()
		})
		v.mu.Lock()
		// Источник мог смениться ещё раз, пока мы подписывались.
		if v.source == source {
			v.srcOC, v.srcSubID = oc, id
			id = 0
		}
		v.mu.Unlock()
		if id > 0 {
			oc.RemoveCollectionChanged(id)
		}
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
//
// PERF-8: под v.mu снимается только слепок параметров, а фильтр, сортировка и
// группировка (на 10k строк — миллисекунды) считаются БЕЗ лока. Раньше на это
// время вставали Items()/Count()/Groups(), а пользовательский предикат
// фильтра, дёрнувший v.Count(), вешал поток намертво.
func (v *CollectionView) Refresh() {
	v.mu.Lock()
	v.seq++
	mySeq := v.seq
	src := v.source
	filter := v.filter
	sorts := append([]SortDescription(nil), v.sorts...)
	groupBy := v.groupBy
	v.mu.Unlock()

	raw := collectionItems(src)

	// 1. Фильтрация.
	out := make([]interface{}, 0, len(raw))
	for _, it := range raw {
		if filter == nil || filter(it) {
			out = append(out, it)
		}
	}

	// 2. Сортировка (стабильная, по нескольким ключам).
	out = sortByDescriptions(out, sorts)

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

	v.mu.Lock()
	if v.seq != mySeq {
		// Пока считали, стартовал более свежий пересчёт — его результат
		// новее нашего; публиковать устаревшее представление нельзя.
		v.mu.Unlock()
		return
	}
	v.view = out
	v.groups = groups
	handlers := make([]viewHandlerEntry, len(v.handlers))
	copy(handlers, v.handlers)
	v.mu.Unlock()

	for _, e := range handlers {
		e.h()
	}
}

// ─── Сортировка: decorate-sort-undecorate (PERF-4) ─────────────────────────

// sortByDescriptions стабильно сортирует items по списку правил.
//
// Ключи извлекаются ОДИН раз на элемент и правило (n·k рефлексивных обходов),
// после чего сравниваются уже типизированные значения. Прежняя версия звала
// на КАЖДОЕ сравнение GetPropertyValue (reflect + strings.Split + FieldByName)
// плюс Sprintf и ToLower — то есть 2·n·log₂n·k рефлексивных обходов.
func sortByDescriptions(items []interface{}, sorts []SortDescription) []interface{} {
	if len(sorts) == 0 || len(items) < 2 {
		return items
	}

	keys := make([][]viewSortKey, len(sorts))
	kinds := make([]viewKeyKind, len(sorts))
	for si, s := range sorts {
		col := make([]viewSortKey, len(items))
		for i, it := range items {
			col[i] = makeViewSortKey(propValue(it, s.Property))
		}
		keys[si] = col
		kinds[si] = commonViewKeyKind(col)
	}

	// Сортируем перестановку: ключи привязаны к исходным позициям.
	perm := make([]int, len(items))
	for i := range perm {
		perm[i] = i
	}
	sort.SliceStable(perm, func(a, b int) bool {
		pa, pb := perm[a], perm[b]
		for si := range sorts {
			c := compareViewSortKeys(&keys[si][pa], &keys[si][pb], kinds[si])
			if c == 0 {
				continue
			}
			if sorts[si].Direction == Descending {
				return c > 0
			}
			return c < 0
		}
		return false
	})

	sorted := make([]interface{}, len(items))
	for i, p := range perm {
		sorted[i] = items[p]
	}
	return sorted
}

// viewKeyKind — тип ключа сортировки в пределах одного правила.
type viewKeyKind uint8

const (
	viewKeyNil    viewKeyKind = iota // значение отсутствует
	viewKeyNumber                    // числа — сравнение по float64
	viewKeyBool                      // false < true
	viewKeyTime                      // time.Time — по моменту времени
	viewKeyString                    // строки и прочее — по нижнему регистру
	viewKeyMixed                     // разные типы — общий compareValues
)

// viewSortKey — предизвлечённый ключ одного элемента.
type viewSortKey struct {
	kind viewKeyKind
	num  float64
	unix int64       // viewKeyTime: UnixNano (float64 потерял бы наносекунды)
	str  string      // viewKeyString: уже в нижнем регистре
	raw  interface{} // сырое значение — только для viewKeyMixed
}

// makeViewSortKey раскладывает значение свойства в типизированный ключ.
func makeViewSortKey(v interface{}) viewSortKey {
	if v == nil {
		return viewSortKey{kind: viewKeyNil}
	}
	switch t := v.(type) {
	case string:
		return viewSortKey{kind: viewKeyString, str: strings.ToLower(t), raw: v}
	case time.Time:
		return viewSortKey{kind: viewKeyTime, unix: t.UnixNano(), raw: v}
	case bool:
		var f float64
		if t {
			f = 1
		}
		return viewSortKey{kind: viewKeyBool, num: f, raw: v}
	}
	rv := reflect.ValueOf(v)
	if f, ok := toFloat(rv); ok {
		return viewSortKey{kind: viewKeyNumber, num: f, raw: v}
	}
	if rv.Kind() == reflect.Bool {
		var f float64
		if rv.Bool() {
			f = 1
		}
		return viewSortKey{kind: viewKeyBool, num: f, raw: v}
	}
	return viewSortKey{kind: viewKeyString, str: strings.ToLower(fmtKey(v)), raw: v}
}

// commonViewKeyKind определяет, однородны ли ключи правила. Неоднородные
// сравниваются прежним compareValues — семантика не меняется, но значения
// уже извлечены, и рефлексия по пути не повторяется на каждое сравнение.
func commonViewKeyKind(keys []viewSortKey) viewKeyKind {
	k := viewKeyNil
	for i := range keys {
		if keys[i].kind == viewKeyNil {
			continue
		}
		if k == viewKeyNil {
			k = keys[i].kind
			continue
		}
		if keys[i].kind != k {
			return viewKeyMixed
		}
	}
	return k
}

// compareViewSortKeys сравнивает ключи: -1/0/1. nil меньше любого значения —
// как и в compareValues.
func compareViewSortKeys(a, b *viewSortKey, kind viewKeyKind) int {
	if a.kind == viewKeyNil || b.kind == viewKeyNil {
		switch {
		case a.kind == viewKeyNil && b.kind == viewKeyNil:
			return 0
		case a.kind == viewKeyNil:
			return -1
		default:
			return 1
		}
	}
	switch kind {
	case viewKeyNumber, viewKeyBool:
		switch {
		case a.num < b.num:
			return -1
		case a.num > b.num:
			return 1
		}
		return 0
	case viewKeyTime:
		switch {
		case a.unix < b.unix:
			return -1
		case a.unix > b.unix:
			return 1
		}
		return 0
	case viewKeyString:
		return strings.Compare(a.str, b.str)
	}
	return compareValues(a.raw, b.raw)
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
//
// В горячем пути сортировки больше не используется (см. sortByDescriptions:
// ключи извлекаются один раз, а не на каждое сравнение) — оставлен как
// точечный помощник для разовых сравнений.
func compareItemProp(a, b interface{}, property string) int {
	return compareValues(propValue(a, property), propValue(b, property))
}

var _ = compareItemProp

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
