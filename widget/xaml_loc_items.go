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

	"github.com/oops1/headless-gui/v3/internal/goid"
)

// locItemTarget — одна «свёрнутая» строка: ключ, способ её применить и
// загрузка разметки, которой она принадлежит (nil — ничья).
type locItemTarget struct {
	key   string
	apply func(text string)
	owner *locOwner
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

// locOwner — владелец «свёрнутых» строк: одна загрузка разметки. Через него
// BindingScope.Dispose снимает строки своего дерева, не трогая чужих (GG-75).
// Поле — чтобы указатели разных владельцев не совпадали (у пустой структуры
// они вправе совпасть).
type locOwner struct{ _ byte }

// locOwners — владелец строк, собираемых сейчас на горутине: id горутины →
// *locOwner. Сборщики разметки вложены глубоко и владельца не получают, а
// сборка идёт синхронно на одной горутине.
var locOwners sync.Map

// withLocOwner выполняет fn, записывая строки, зарегистрированные на этой
// горутине, за владельцем o. nil — строки ничьи, как до владельцев.
func withLocOwner(o *locOwner, fn func()) {
	if o == nil {
		fn()
		return
	}
	id := goid.Current()
	prev, had := locOwners.Load(id)
	locOwners.Store(id, o)
	defer func() {
		if had {
			locOwners.Store(id, prev)
		} else {
			locOwners.Delete(id)
		}
	}()
	fn()
}

// currentLocOwner возвращает владельца строк текущей горутины или nil.
func currentLocOwner() *locOwner {
	v, ok := locOwners.Load(goid.Current())
	if !ok {
		return nil
	}
	return v.(*locOwner)
}

// registerLocItem запоминает, как обновить строку при смене языка. Пустой ключ
// игнорируется, поэтому вызывающий код может звать функцию безусловно.
//
// Строка, зарегистрированная при загрузке разметки, принадлежит этой загрузке:
// её переводит и снимает BindingScope дерева. Ничью переводит общий слушатель.
func registerLocItem(key string, apply func(text string)) {
	if key == "" || apply == nil {
		return
	}
	owner := currentLocOwner()
	if owner == nil {
		locItemsOnce.Do(func() {
			AddLanguageListener(func(string) { applyLocItems() })
		})
	}
	locItemsMu.Lock()
	locItems = append(locItems, locItemTarget{key: key, apply: apply, owner: owner})
	locItemsMu.Unlock()
}

// ownedLocItemCount — сколько строк у владельца.
func ownedLocItemCount(o *locOwner) int {
	locItemsMu.Lock()
	defer locItemsMu.Unlock()
	n := 0
	for _, t := range locItems {
		if t.owner == o {
			n++
		}
	}
	return n
}

// applyOwnedLocItems раскладывает переводы по строкам владельца.
func applyOwnedLocItems(o *locOwner) {
	applyLocItemsOf(o)
}

// removeOwnedLocItems забывает строки владельца.
func removeOwnedLocItems(o *locOwner) {
	if o == nil {
		return
	}
	locItemsMu.Lock()
	kept := locItems[:0]
	for _, t := range locItems {
		if t.owner != o {
			kept = append(kept, t)
		}
	}
	clear(locItems[len(kept):]) // не держать замыкания снятого дерева
	locItems = kept
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

// applyLocItems раскладывает переводы по ничьим строкам. Строки загруженных
// деревьев переводит BindingScope своего дерева.
func applyLocItems() {
	applyLocItemsOf(nil)
}

// applyLocItemsOf раскладывает переводы по строкам владельца o.
func applyLocItemsOf(o *locOwner) {
	locItemsMu.Lock()
	targets := make([]locItemTarget, 0, len(locItems))
	for _, t := range locItems {
		if t.owner == o {
			targets = append(targets, t)
		}
	}
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
