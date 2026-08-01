// xaml_loc_items.go — живая локализация строк, которые НЕ являются свойствами
// виджета: заголовков вкладок, пунктов меню и элементов списков.
//
// Обычный {Loc Key} в разметке превращается в живую привязку «виджет +
// свойство» (см. xaml_binding.go): при смене языка достаточно записать новый
// перевод в свойство. Но <TabItem Header="…">, <MenuItem Header="…"> и
// <ComboBoxItem Content="…"> отдельными виджетами НЕ становятся — сборщик
// сворачивает их в родителя (TabControl.AddTab, MenuBar.AddMenu,
// Dropdown/ListView.SetItems). Свойства, которое можно переустановить, у них
// нет, поэтому такие строки регистрируются здесь: ключ + замыкание, которое
// умеет положить перевод на место (по индексу вкладки, пункта, строки).
//
// Из-за этого раньше при переключении языка менялись подписи и кнопки, а
// вкладки, меню и выпадающие списки оставались на прежнем языке.
package widget

import (
	"strings"
	"sync"
)

// locItemTarget — одна «свёрнутая» строка: ключ и способ её применить.
type locItemTarget struct {
	key   string
	apply func(text string)
}

var (
	locItemsMu   sync.Mutex
	locItems     []locItemTarget
	locItemsOnce sync.Once
)

// isFoldedItemTag — тег, который не становится отдельным виджетом: его
// содержимое сборщик складывает в родителя (вкладка, пункт меню, строка
// списка). Для таких элементов {Loc …} обрабатывает сборщик родителя.
func isFoldedItemTag(tag string) bool {
	switch strings.ToLower(tag) {
	case "tabitem", "menuitem", "item", "comboboxitem", "listboxitem", "listviewitem":
		return true
	}
	return false
}

// locMarkupKey распознаёт значение вида {Loc Ключ} и возвращает ключ.
// ok=false — это обычная строка, локализовать нечего.
func locMarkupKey(v string) (string, bool) {
	t := strings.TrimSpace(v)
	if t != "{Loc}" && !strings.HasPrefix(t, "{Loc ") && !strings.HasPrefix(t, "{Loc}") {
		return "", false
	}
	key := parseLocKey(t)
	if key == "" {
		return "", false
	}
	return key, true
}

// locItemText возвращает текст для отображения: если значение — {Loc Ключ},
// то перевод, иначе саму строку. Второе значение — ключ (пустой, если строка
// не локализуемая).
func locItemText(v string) (text, key string) {
	if k, ok := locMarkupKey(v); ok {
		return Tr(k), k
	}
	return v, ""
}

// registerLocItem запоминает, как обновить строку при смене языка. Пустой ключ
// игнорируется, поэтому вызывающий код может звать функцию безусловно.
func registerLocItem(key string, apply func(text string)) {
	if key == "" || apply == nil {
		return
	}
	locItemsOnce.Do(func() {
		AddLanguageListener(func(string) { applyLocItems() })
	})
	locItemsMu.Lock()
	locItems = append(locItems, locItemTarget{key: key, apply: apply})
	locItemsMu.Unlock()
}

// registerLocItemList — то же для списка строк: keys[i] соответствует i-му
// элементу, set кладёт перевод на нужное место. Непереводимые элементы
// (пустой ключ) пропускаются.
func registerLocItemList(keys []string, set func(i int, text string)) {
	for i, k := range keys {
		if k == "" {
			continue
		}
		i := i
		registerLocItem(k, func(s string) { set(i, s) })
	}
}

// applyLocItems раскладывает переводы по всем зарегистрированным местам.
func applyLocItems() {
	locItemsMu.Lock()
	targets := make([]locItemTarget, len(locItems))
	copy(targets, locItems)
	locItemsMu.Unlock()
	for _, t := range targets {
		t.apply(Tr(t.key))
	}
	if len(targets) > 0 {
		notifyUIChanged()
	}
}

// ClearLocalizedItems забывает все зарегистрированные «свёрнутые» строки.
// Нужен, когда разметка перезагружается заново (иначе цели прежнего дерева
// остались бы висеть) и в тестах.
func ClearLocalizedItems() {
	locItemsMu.Lock()
	locItems = nil
	locItemsMu.Unlock()
}
