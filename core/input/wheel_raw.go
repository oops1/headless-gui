package input

// wheel_raw.go — сырая дельта колеса и тачпада (v3.39).
//
// SendMouseWheelPixels отдаёт виджетам ПИКСЕЛИ под шаг прокрутки движка — этого
// мало удалённому рабочему столу: протокол ждёт дельту оборудования (120 на
// щелчок) и горизонтальное колесо как есть. Здесь тот же маршрут доставки, но
// с widget.WheelEvent; прежний путь не тронут и работает рядом.

import "github.com/oops1/headless-gui/v4/widget"

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
func (d *Dispatcher) SendMouseWheelRaw(xPhys, yPhys int, dx, dy float64, precise bool) bool {
	defer d.host.GuardInput("SendMouseWheelRaw")()
	if dx == 0 && dy == 0 {
		return false
	}
	x, y := d.toLogical(xPhys, yPhys)

	var dispatchRoot widget.Widget
	if m := d.host.TopModal(); m != nil {
		dispatchRoot = m
	} else {
		dispatchRoot = d.host.Root()
	}
	targets := d.wheelTargets(dispatchRoot, x, y)
	if len(targets) == 0 {
		return false
	}
	mod := d.Modifiers()
	for _, st := range targets {
		h, ok := st.W.(widget.WheelEventHandler)
		if !ok {
			continue
		}
		// Точка — в кадре виджета (ребёнок прокрутки), как у пиксельного пути.
		ev := widget.WheelEvent{
			X: x + st.Off.X, Y: y + st.Off.Y,
			DeltaX: dx, DeltaY: dy,
			Precise: precise, Mod: mod,
		}
		if h.OnWheelEvent(ev) {
			d.invalidateWidget(st.W, st.Off)
			return true
		}
	}
	return false
}
