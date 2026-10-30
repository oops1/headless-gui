package input

// pointerowner.go — движение указателя для виджетов, которым нужен только
// верхний под указателем (widget.PointerOwner: окно чужой программы).
//
// Рассылка движения (broadcastMouseMove) идёт всем виджетам по границам, без
// Z. Владельцу указателя этого мало: окно программы под панелью задач
// не должно получать движение, когда указатель над панелью. Здесь движок
// ищет верхнего владельца по тому же hit-тесту, что и щелчок, и отдаёт
// движение только ему; при смене владельца прежний получает PointerOut.
//
// Пока в процессе нет ни одного владельца (widget.PointerOwnersActive), поиск
// не делается: обычное движение мыши не платит за окна программ.

import (
	"image"

	"github.com/oops1/headless-gui/v4/widget"
)

// routePointerOwner отдаёт движение (x, y — логические экранные координаты)
// владельцу указателя. captured — виджет, держащий захват мыши (nil — нет);
// overlay — оверлей под указателем (nil — нет), он лежит выше всех окон.
func (d *Dispatcher) routePointerOwner(x, y int, dispRoot widget.Widget, captured, overlay widget.Widget) {
	if d.pointerOwner == nil && !widget.PointerOwnersActive() {
		return
	}
	var owner widget.PointerOwner
	var off image.Point
	switch {
	case captured != nil:
		// Захват: движение получает захвативший, где бы ни был указатель —
		// как неявный захват у Wayland при нажатой кнопке.
		if po, ok := captured.(widget.PointerOwner); ok {
			owner, off = po, d.captureOffset(captured)
		}
	case overlay != nil || dispRoot == nil:
		// Над открытым меню или вне дерева окна программы нет.
	default:
		d.ownerPath = appendHitPath(d.ownerPath[:0], dispRoot, x, y, image.Point{}, 0)
		for i := len(d.ownerPath) - 1; i >= 0; i-- {
			if po, ok := d.ownerPath[i].W.(widget.PointerOwner); ok {
				owner, off = po, d.ownerPath[i].Off
				break
			}
		}
		clear(d.ownerPath) // не держать виджеты до следующего движения
	}
	if owner != d.pointerOwner {
		if prev := d.pointerOwner; prev != nil {
			prev.PointerOut()
		}
		d.pointerOwner = owner
	}
	if owner != nil {
		prev := widget.SetEventFrame(off)
		owner.PointerOver(x+off.X, y+off.Y)
		widget.SetEventFrame(prev)
	}
}

// pointerOwnerLeft — указатель ушёл с окна: владелец его теряет.
func (d *Dispatcher) pointerOwnerLeft() {
	if prev := d.pointerOwner; prev != nil {
		d.pointerOwner = nil
		prev.PointerOut()
	}
}
