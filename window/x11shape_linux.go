//go:build linux && !android

package window

// x11shape_linux.go — окно-попап X11 там, где альфу смешивать некому.
//
// Попап с 32-битным ARGB visual'ом (x11argb.go) прозрачен по-настоящему
// только у композитного менеджера: без него сервер альфу не смешивает, и
// полупрозрачная тень меню вместе с точками за скруглением показывается
// чёрной — как и у 24-битного окна. Тогда окно выкраивается расширением
// SHAPE по СПЛОШНЫМ точкам кадра (engine.SolidBands): тени и сглаженного
// края в окне нет вовсе, и чёрному неоткуда взяться. Цена — без композитора
// у меню нет мягкой тени, а углы скругления ступенчатые.

import (
	"encoding/binary"
	"image"

	"github.com/oops1/headless-gui/v3/engine"
)

// x11InitPopupShape решает при создании попапа, нужна ли ему форма, и узнаёт
// опкод SHAPE. Зовётся из CreatePopup до первого MapWindow: на соединении
// попапа ещё нет событий, запрос-ответ безопасен.
func (w *X11Window) x11InitPopupShape() {
	if w.argb && w.x11AlphaComposited() {
		w.composited = true
		return // альфу смешает композитор — окно остаётся прямоугольным
	}
	if major, ok := w.x11ExtensionMajor("SHAPE"); ok {
		w.shapeMajor = major
	}
}

// PopupPixelAlpha реализует popupPixelAlpha: альфу точек попапа смешивает
// композитор — переход появления может идти копией кадра с множителем.
func (w *X11Window) PopupPixelAlpha() bool { return w.popup && w.argb && w.composited }

// x11AlphaComposited — смешивает ли кто-то альфу окон на этом экране.
//
// Признак композитного менеджера по EWMH — владелец выделения
// _NET_WM_CM_S<экран>. XWayland смешивает ARGB-окна сам (их композирует
// компоновщик Wayland), даже если такого владельца нет.
func (w *X11Window) x11AlphaComposited() bool {
	if _, ok := w.x11ExtensionMajor("XWAYLAND"); ok {
		return true
	}
	atom := w.x11InternAtom("_NET_WM_CM_S0")
	if atom == 0 {
		return false
	}
	reply, ok := w.x11Call(x11GetSelectionOwnerRequest(atom))
	if !ok || len(reply) < 12 {
		return false
	}
	return binary.LittleEndian.Uint32(reply[8:12]) != 0
}

// blitPopupRegion выкраивает попап по сплошным точкам кадра и кладёт кадр.
// Реализует popupRegionSetter. bands (закрашенное с тенью) здесь не годятся:
// форма из них оставила бы тень, которую некому смешать.
func (w *X11Window) blitPopupRegion(img *image.RGBA, _ []image.Rectangle) {
	if w.shapeMajor != 0 && img != nil {
		solid := engine.SolidBands(img)
		if o := img.Rect.Min; o != (image.Point{}) {
			for i := range solid {
				solid[i] = solid[i].Sub(o)
			}
		}
		if !w.shapeSent || !sameRects(solid, w.shapeLast) {
			w.x11Send(x11ShapeRectanglesRequest(w.shapeMajor, w.wid, solid))
			w.shapeLast, w.shapeSent = solid, true
		}
	}
	w.BlitRGBA(img)
}
