// xaml_registry.go — реестр пользовательских XAML-виджетов.
//
// Позволяет внешним пакетам регистрировать свои виджеты для использования
// в XAML-разметке. Зарегистрированные виджеты обрабатываются наравне
// со встроенными при парсинге XAML.
package widget

import (
	"strings"
	"sync"
)

// XAMLAttrs предоставляет доступ к атрибутам XAML-элемента.
// Передаётся в XAMLWidgetBuilder, чтобы внешние пакеты могли
// читать атрибуты без доступа к внутреннему типу xElement.
type XAMLAttrs interface {
	// Attr возвращает значение первого найденного атрибута из списка имён.
	// Если ни один атрибут не найден — возвращает "".
	Attr(names ...string) string

	// Tag возвращает имя тега XAML-элемента.
	Tag() string

	// Text возвращает текстовое содержимое элемента (между открывающим и закрывающим тегами).
	Text() string

	// ChildCount возвращает количество дочерних элементов.
	ChildCount() int

	// ChildAttrs возвращает XAMLAttrs для дочернего элемента по индексу.
	ChildAttrs(index int) XAMLAttrs
}

// XAMLWidgetBuilder — функция-конструктор пользовательского XAML-виджета.
// Получает атрибуты XAML-элемента, возвращает готовый виджет.
type XAMLWidgetBuilder func(attrs XAMLAttrs) (Widget, error)

// ─── Глобальный реестр ──────────────────────────────────────────────────────

var (
	customXAMLMu       sync.RWMutex
	customXAMLBuilders = map[string]XAMLWidgetBuilder{}
)

// RegisterXAMLWidget регистрирует пользовательский виджет для использования в XAML.
// tag — имя тега (регистронезависимое), например "diffview".
// builder — функция-конструктор, вызываемая при встрече тега в XAML.
//
// Пример:
//
//	widget.RegisterXAMLWidget("DiffView", func(attrs widget.XAMLAttrs) (widget.Widget, error) {
//	    dv := diffview.New()
//	    if t := attrs.Attr("Theme"); strings.EqualFold(t, "light") {
//	        dv.Colors = diffview.DefaultLightColors()
//	    }
//	    return dv, nil
//	})
func RegisterXAMLWidget(tag string, builder XAMLWidgetBuilder) {
	customXAMLMu.Lock()
	defer customXAMLMu.Unlock()
	customXAMLBuilders[strings.ToLower(tag)] = builder
}

// UnregisterXAMLWidget удаляет пользовательский виджет из реестра.
func UnregisterXAMLWidget(tag string) {
	customXAMLMu.Lock()
	defer customXAMLMu.Unlock()
	delete(customXAMLBuilders, strings.ToLower(tag))
}

// lookupCustomXAML ищет пользовательский builder для тега.
func lookupCustomXAML(tag string) (XAMLWidgetBuilder, bool) {
	customXAMLMu.RLock()
	defer customXAMLMu.RUnlock()
	b, ok := customXAMLBuilders[strings.ToLower(tag)]
	return b, ok
}

// ─── xElement → XAMLAttrs адаптер ──────────────────────────────────────────

// xElementAttrs оборачивает xElement в интерфейс XAMLAttrs.
type xElementAttrs struct {
	el *xElement
}

func newXAMLAttrs(el *xElement) XAMLAttrs {
	return &xElementAttrs{el: el}
}

func (a *xElementAttrs) Attr(names ...string) string {
	return a.el.attr(names...)
}

func (a *xElementAttrs) Tag() string {
	return a.el.Tag
}

func (a *xElementAttrs) Text() string {
	return a.el.Text
}

func (a *xElementAttrs) ChildCount() int {
	return len(a.el.Children)
}

func (a *xElementAttrs) ChildAttrs(index int) XAMLAttrs {
	if index < 0 || index >= len(a.el.Children) {
		return &xElementAttrs{el: &xElement{}}
	}
	return &xElementAttrs{el: &a.el.Children[index]}
}
