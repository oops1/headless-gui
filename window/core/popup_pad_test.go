package core

import (
	"image"
	"testing"
)

// Попап с запасом под тень (PopupFrame.Pad): в рабочую область монитора
// вписывается ТЕЛО меню. Прозрачный запас с тенью может выйти за край экрана
// и лечь на таскбар, а меню у края не отодвигается от него на ширину тени.

type workAreaNative struct {
	popupFakeNative
	wa   image.Rectangle
	x, y int
}

func (w *workAreaNative) WorkAreaAt(int, int) image.Rectangle { return w.wa }
func (w *workAreaNative) SetPosition(x, y int)                { w.x, w.y = x, y }

func TestPopupHost_PadKeepsBodyInWorkArea(t *testing.T) {
	carrier := &popupFakeNative{} // в (0,0)
	for _, tc := range []struct {
		name   string
		scale  float64
		rest   image.Rectangle // логический, с запасом
		pad    int
		wantXY image.Point
	}{
		// Меню у правого нижнего края рабочей области 800×600: тело
		// вписывается вплотную, тень уходит за край.
		{"правый низ", 1, image.Rect(700, 540, 848, 668), 24, image.Pt(800-148+24, 600-128+24)},
		// У левого верхнего края — тело с нуля, запас левее и выше экрана.
		{"левый верх", 1, image.Rect(-40, -40, 108, 88), 24, image.Pt(-24, -24)},
		// Внутри области — место не меняется.
		{"внутри", 1, image.Rect(100, 100, 248, 228), 24, image.Pt(100, 100)},
		// HiDPI: запас в физических точках.
		{"HiDPI", 2, image.Rect(380, 270, 454, 334), 24, image.Pt(800-148+48, 600-128+48)},
		// Без запаса — как прежде: окно вплотную к краю.
		{"без запаса", 1, image.Rect(700, 540, 848, 668), 0, image.Pt(800-148, 600-128)},
	} {
		h := newPopupHost(carrier, carrier, &queueEngine{}, tc.scale, nil)
		pop := &workAreaNative{wa: image.Rect(0, 0, 800, 600)}
		pw := int(float64(tc.rest.Dx()) * tc.scale)
		ph := int(float64(tc.rest.Dy()) * tc.scale)
		h.positionPopup(pop, carrier, tc.rest, image.Point{}, pw, ph, tc.pad, false)
		if got := image.Pt(pop.x, pop.y); got != tc.wantXY {
			t.Errorf("%s: окно в %v, ждали %v", tc.name, got, tc.wantXY)
		}
	}
}
