package chrome

// window_dragarea.go — области перетаскивания окна в клиентской части.

import (
	"image"
)

// AddDragArea объявляет прямоугольник содержимого областью перетаскивания окна.
//
// Нужна borderless-окну (WindowStyleNone): полосы заголовка у него нет, а
// значит и тащить его было не за что — WantsCapture у такого окна отвечал
// «нет» безусловно. Приложение, рисующее свою шапку, теперь говорит движку, где
// она.
//
// Координаты — ОТНОСИТЕЛЬНО левого верхнего угла окна: окно двигают, и
// абсолютные пришлось бы пересчитывать после каждого перетаскивания.
func (w *Window) AddDragArea(r image.Rectangle) {
	if r.Empty() {
		return
	}
	w.dragAreas = append(w.dragAreas, r)
}

// ClearDragAreas забывает объявленные области перетаскивания окна.
func (w *Window) ClearDragAreas() { w.dragAreas = nil }

// dragAreaHit сообщает, попадает ли точка в объявленную область перетаскивания.
func (w *Window) dragAreaHit(pt image.Point) bool {
	if len(w.dragAreas) == 0 {
		return false
	}
	off := w.Base.Bounds().Min
	for _, r := range w.dragAreas {
		if pt.In(r.Add(off)) {
			return true
		}
	}
	return false
}
