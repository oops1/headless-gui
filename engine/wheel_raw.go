package engine

// wheel_raw.go — сырая дельта колеса и тачпада (v3.39).
//
// SendMouseWheelPixels отдаёт виджетам ПИКСЕЛИ под шаг прокрутки движка — этого
// мало удалённому рабочему столу: протокол ждёт дельту оборудования (120 на
// щелчок) и горизонтальное колесо как есть. Здесь тот же маршрут доставки, но
// с widget.WheelEvent; прежний путь не тронут и работает рядом.

import "github.com/oops1/headless-gui/v3/widget"

// SendMouseWheelRaw доставляет сырое событие колеса в точку (xPhys, yPhys —
// физические пиксели окна/кадра, как у SendMouseWheelPixels). dx, dy — в
// единицах widget.WheelNotch (120 на щелчок), dy > 0 — вниз, dx > 0 — вправо;
// precise — высокоточная дельта (тачпад).
//
// Получатели — по порядку: виджет, захвативший мышь, затем открытый оверлей под
// курсором, затем путь hit-test от самого глубокого виджета к корню (при
// модальном окне — внутри него). Первый, кто реализует
// widget.WheelEventHandler и вернул true, поглощает событие. Результат —
// «принято ли»: окно по нему решает, запускать ли прежний пиксельный путь для
// этого же движения колеса (принявшему сырое событие пиксели не нужны, а
// прокручивать дерево дважды нельзя). Headless-код вызывает метод напрямую и
// вправе игнорировать результат.
//
// Дельты не масштабируются HiDPI-коэффициентом: это не пиксели.
func (e *Engine) SendMouseWheelRaw(xPhys, yPhys int, dx, dy float64, precise bool) bool {
	if dx == 0 && dy == 0 {
		return false
	}
	x, y := e.toLogical(xPhys, yPhys)

	var dispatchRoot widget.Widget
	if m := e.topModal(); m != nil {
		dispatchRoot = m
	} else {
		e.mu.RLock()
		dispatchRoot = e.root
		e.mu.RUnlock()
	}
	targets := e.wheelTargets(dispatchRoot, x, y)
	if len(targets) == 0 {
		return false
	}
	mod := e.Modifiers()
	for _, st := range targets {
		h, ok := st.w.(widget.WheelEventHandler)
		if !ok {
			continue
		}
		// Точка — в кадре виджета (ребёнок прокрутки), как у пиксельного пути.
		ev := widget.WheelEvent{
			X: x + st.off.X, Y: y + st.off.Y,
			DeltaX: dx, DeltaY: dy,
			Precise: precise, Mod: mod,
		}
		if h.OnWheelEvent(ev) {
			e.invalidateWidget(st.w, st.off)
			return true
		}
	}
	return false
}
