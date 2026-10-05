// systemtray_chevron.go — шеврон трея как элемент клавиатурного фокуса.
package desktop

import (
	"image"

	"github.com/oops1/headless-gui/v3/theme"
	"github.com/oops1/headless-gui/v3/widget"
)

// ─── Шеврон трея ─────────────────────────────────────────────────────────────

// trayChevronButton — кнопка раскрытия скрытых значков как элемент фокуса.
// Сама ничего не рисует и мышь не берёт: картинку и щелчок ведёт SystemTray;
// кнопка нужна, чтобы до шеврона доходили Tab, стрелки, Enter и Space.
type trayChevronButton struct {
	widget.Base
	FocusState

	tray *SystemTray
}

var _ widget.Focusable = (*trayChevronButton)(nil)

// Draw пуст: шеврон рисует трей.
func (c *trayChevronButton) Draw(widget.DrawContext) {}

// SetFocused реализует widget.Focusable.
func (c *trayChevronButton) SetFocused(v bool) {
	if c.FocusState.Set(v) {
		c.Invalidate()
	}
}

// TabIndex: шеврона без скрытых значков нет, и в обход он не входит.
func (c *trayChevronButton) TabIndex() int { return focusTabIndex(c) }

// OnMouseButton лишь отмечает, что фокус пришёл от мыши; щелчок обработает
// трей, до которого событие всплывёт.
func (c *trayChevronButton) OnMouseButton(e widget.MouseEvent) bool {
	if c.NotePointer(e) {
		c.Invalidate()
	}
	return false
}

// OnKeyEvent: Enter и Space раскрывают и закрывают область скрытых значков.
func (c *trayChevronButton) OnKeyEvent(e widget.KeyEvent) {
	c.HandleKey(c, e, func() { c.tray.overflow.Toggle(c.Bounds()) }, c.Invalidate)
}

// FocusRing реализует FocusRinger.
func (c *trayChevronButton) FocusRing() (image.Rectangle, *theme.Style) {
	return c.Bounds(), c.tray.style(ComponentTrayChevron, theme.StateNormal)
}

// syncChevron выравнивает кнопку-ребёнка по шеврону, посчитанному раскладкой
// (пустой прямоугольник — шеврона нет, всё влезло).
func (t *SystemTray) syncChevron() {
	if t.chev != nil {
		t.chev.SetBounds(t.chevron)
	}
}

// SetFocusNavigator реализует FocusNavigable: трей передаёт навигатор панели
// своим значкам и шеврону.
func (t *SystemTray) SetFocusNavigator(n FocusNavigator) {
	t.nav = n
	if t.chev != nil {
		t.chev.SetFocusNavigator(n)
	}
	for _, it := range t.items {
		if f, ok := it.(FocusNavigable); ok {
			f.SetFocusNavigator(n)
		}
	}
}
