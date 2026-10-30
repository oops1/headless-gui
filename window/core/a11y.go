// a11y.go — доступность окна: решение приложения (SetAccessibilityEnabled) и
// подключение моста к службе доступности ОС. Сам мост (UI Automation, AT-SPI)
// — возможность platform/a11y (ADR-0013 §3): она регистрируется в
// platform/native, а окно отдаёт ей себя через native.AccessHost (a11yHost).
// Без неё (window/minimal) окно работает как прежде, только без моста.
package core

import (
	"github.com/oops1/headless-gui/v3/platform/native"
	"github.com/oops1/headless-gui/v3/widget"
)

// SetAccessibilityEnabled принудительно включает (true) или выключает (false)
// платформенный мост доступности.
//
// По умолчанию (метод не вызывался): на Linux мост AT-SPI поднимается, когда
// система сообщает о включённой доступности (org.a11y.Status); на Windows мост
// UI Automation включён всегда — он пассивен, пока клиент доступности не
// пришлёт окну WM_GETOBJECT. Без возможности доступности (window/minimal)
// моста нет вовсе, и вызов ничего не меняет.
//
// Вызывать до Run(). Переменная окружения HEADLESS_GUI_A11Y=1/0 имеет
// наивысший приоритет (удобно для отладки со скринридером).
func (win *Window) SetAccessibilityEnabled(v bool) {
	win.a11yForce = &v
}

// startAccessibility поднимает мост доступности (вызывается из bringUp).
// Ошибки не фатальны: без моста приложение работает как прежде.
func (win *Window) startAccessibility() {
	f, ok := native.Accessibility()
	if !ok || f == nil || win.a11y != nil {
		return
	}
	b := f(a11yHost{win})
	if b == nil {
		return
	}
	if err := b.Start(); err != nil {
		return
	}
	win.a11y = b
}

// stopAccessibility снимает мост (вызывается после цикла событий).
func (win *Window) stopAccessibility() {
	if win.a11y != nil {
		win.a11y.Stop()
		win.a11y = nil
	}
}

// accessibilityTreeProvider — опциональная возможность движка отдать
// семантическое дерево (реализует *engine.Engine). Через интерфейс, чтобы не
// расширять обязательный EngineAPI ради моста доступности.
type accessibilityTreeProvider interface {
	AccessibilityTree() *widget.AccessNode
}

// accessibilityActor — опциональная возможность движка ВЫПОЛНЯТЬ действия
// доступности: перевести фокус и нажать элемент (реализует *engine.Engine).
// Отдельный узкий интерфейс рядом с accessibilityTreeProvider — по той же
// причине: обязательный EngineAPI не должен разрастаться ради моста.
type accessibilityActor interface {
	FocusAccessible(w widget.Widget) bool
	ActivateAccessible(w widget.Widget) bool
}

// accessibilityNodeActor — опциональная возможность движка доставить действие
// узлу владельца (реализует *engine.Engine). Отдельный интерфейс по той же
// причине, что и accessibilityActor.
type accessibilityNodeActor interface {
	AccessAction(w widget.Widget, id string, action widget.AccessActionKind) bool
}

// a11yHost — окно глазами моста доступности (native.AccessHost). Отдельный
// неэкспортированный тип, а не методы Window: методы Window — открытый API
// window.Window.
type a11yHost struct{ win *Window }

var _ native.AccessHost = a11yHost{}

func (h a11yHost) AccessibilityTree() *widget.AccessNode {
	if p, ok := h.win.eng.(accessibilityTreeProvider); ok {
		return p.AccessibilityTree()
	}
	return nil
}

func (h a11yHost) FocusAccessible(w widget.Widget) bool {
	a, ok := h.win.eng.(accessibilityActor)
	return ok && w != nil && a.FocusAccessible(w)
}

func (h a11yHost) ActivateAccessible(w widget.Widget) bool {
	a, ok := h.win.eng.(accessibilityActor)
	return ok && w != nil && a.ActivateAccessible(w)
}

func (h a11yHost) AccessAction(w widget.Widget, id string, kind widget.AccessActionKind) bool {
	a, ok := h.win.eng.(accessibilityNodeActor)
	return ok && a.AccessAction(w, id, kind)
}

func (h a11yHost) PostToEngine(fn func())      { h.win.postToEngine(fn) }
func (h a11yHost) Native() native.NativeWindow { return h.win.native }
func (h a11yHost) Scale() float64              { return h.win.scale }
func (h a11yHost) Title() string               { return h.win.title }

func (h a11yHost) AccessibilityForced() (on, set bool) {
	if f := h.win.a11yForce; f != nil {
		return *f, true
	}
	return false, false
}
