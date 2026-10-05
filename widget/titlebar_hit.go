// titlebar_hit.go — кому принадлежит нажатие на заголовок окна.
//
// Заголовок окна — это зона перетаскивания: Window.WantsCapture отвечает
// «да» на любое нажатие в полосе, и движок отдаёт нажатие захватчику
// (CaptureRequester), а не обычному разбору. Захватчика он ищет от самого
// глубокого ребёнка к корню, поэтому ребёнок, лежащий поверх заголовка,
// получает нажатие ТОЛЬКО если сам вернул true из WantsCapture; иначе нажатие
// достаётся окну, и окно начинает перетаскивание. Это неочевидно: обычный
// виджет без WantsCapture, положенный на заголовок, «не нажимается» и таскает
// окно.
//
// Два штатных исключения — кнопка сворачивания боковой панели и начинка полосы
// (SetTitleBarContent) — окно знает само. Для остального здесь два явных
// способа сказать «эта точка заголовка — не перетаскивание», не требуя от
// виджета захвата мыши.
package widget

import "image"

// TitleBarPressOwner — виджет-потомок окна, лежащий поверх его заголовка, у
// которого нажатие на себя забирает он сам, а не перетаскивание окна.
//
// Достаточно реализовать этот интерфейс: захват мыши (WantsCapture) не нужен —
// нажатие доходит до виджета обычным путём, как до любого виджета окна. Захват
// по-прежнему нужен тому, кому после нажатия нужна мышь целиком (ползунок,
// выделение текста, release-семантика кнопки).
type TitleBarPressOwner interface {
	// OwnsTitleBarPress сообщает, принадлежит ли виджету нажатие в точке pt
	// (в координатах окна) — как правило, «pt внутри моей видимой части».
	// Зовётся только для точек внутри Bounds виджета.
	OwnsTitleBarPress(pt image.Point) bool
}

// SetTitleBarHitTest задаёт решение приложения «нажатие в этой точке заголовка
// не тащит окно» — для областей, которые не принадлежат ни одному виджету
// (нарисованы самим приложением) или принадлежат виджету, не умеющему
// TitleBarPressOwner. Функция зовётся на каждом нажатии в заголовке; true —
// окно перетаскивание не начинает, и нажатие идёт обычным разбором.
// nil снимает решение.
//
// Не заменяет перетаскивание за остальную часть заголовка и кнопки управления,
// которые окно разбирает раньше.
func (w *Window) SetTitleBarHitTest(fn func(pt image.Point) bool) {
	w.titleHitTest = fn
}

// titleBarOwnedByApp — нажатие принадлежит приложению: его функция по точке
// или виджет, назвавшийся владельцем (TitleBarPressOwner).
func (w *Window) titleBarOwnedByApp(pt image.Point) bool {
	// Только точки самой полосы: вопрос задаётся на каждом нажатии в окне, а
	// содержимое под заголовком ему не принадлежит.
	if !pt.In(w.titleBarRect()) {
		return false
	}
	if w.titleHitTest != nil && w.titleHitTest(pt) {
		return true
	}
	return ownsTitleBarPress(w.Children(), pt, 0)
}

// ownsTitleBarPress ищет среди потомков видимый виджет, который владеет
// нажатием в точке pt.
func ownsTitleBarPress(children []Widget, pt image.Point, depth int) bool {
	if depth > 16 {
		return false
	}
	for _, c := range children {
		if c == nil || !IsWidgetVisible(c) || !pt.In(c.Bounds()) {
			continue
		}
		if o, ok := c.(TitleBarPressOwner); ok && o.OwnsTitleBarPress(pt) {
			return true
		}
		if ownsTitleBarPress(c.Children(), pt, depth+1) {
			return true
		}
	}
	return false
}
