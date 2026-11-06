//go:build linux && !android

package x11

// x11shape_linux.go — окно-попап X11 там, где альфу смешивать некому.
//
// Попап с 32-битным ARGB visual'ом (x11argb.go) прозрачен по-настоящему
// только у композитного менеджера: без него сервер альфу не смешивает, и
// полупрозрачная тень меню вместе с точками за скруглением показывается
// чёрной — как и у 24-битного окна. Тогда окно выкраивается расширением
// SHAPE по СПЛОШНЫМ точкам кадра (raster.SolidBands): тени и сглаженного
// края в окне нет вовсе, и чёрному неоткуда взяться. Цена — без композитора
// у меню нет мягкой тени, а углы скругления ступенчатые.

import (
	"encoding/binary"
	"image"

	"github.com/oops1/headless-gui/v4/core/raster"
	"github.com/oops1/headless-gui/v4/platform/native"
)

// x11InitPopupShape решает при создании попапа, нужна ли ему форма, и узнаёт
// опкод SHAPE. Зовётся из CreatePopup до первого MapWindow: на соединении
// попапа ещё нет событий, запрос-ответ безопасен.
func (w *X11Window) x11InitPopupShape() {
	if w.argb && w.x11AlphaComposited() {
		w.composited = true
		// Альфу смешает композитор — окно остаётся прямоугольным, а клики
		// ловит только закрашенное: прозрачный запас под тень вокруг меню
		// (PopupFrame.Pad) не должен перехватывать их у окон под ним.
		if major, ok := w.x11ExtensionMajor("SHAPE"); ok {
			w.inputMajor = major
		}
		return
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

// BlitPopupRegion выкраивает попап по сплошным точкам кадра и кладёт кадр.
// Реализует popupRegionSetter. bands (закрашенное с тенью) для видимой формы
// не годятся: она оставила бы тень, которую некому смешать. Зато у попапа с
// композитором ими задаётся форма ввода (x11InputShape).
func (w *X11Window) BlitPopupRegion(img *image.RGBA, bands []image.Rectangle) {
	w.x11InputShape(img, bands)
	if w.shapeMajor != 0 && img != nil {
		solid := raster.SolidBands(img)
		if o := img.Rect.Min; o != (image.Point{}) {
			for i := range solid {
				solid[i] = solid[i].Sub(o)
			}
		}
		if !w.shapeSent || !native.SameRects(solid, w.shapeLast) {
			w.x11Send(x11ShapeRectanglesRequest(w.shapeMajor, w.wid, solid))
			w.shapeLast, w.shapeSent = solid, true
		}
	}
	w.BlitRGBA(img)
}

// x11InputShape задаёт попапу с композитором форму ввода — закрашенные точки
// кадра (bands, raster.OpaqueBands). Пустые bands означают «выкраивать
// нечего»: кадр закрашен весь (или прозрачен лишь по скруглённым краям), и
// ввод ловит окно целиком.
func (w *X11Window) x11InputShape(img *image.RGBA, bands []image.Rectangle) {
	if w.inputMajor == 0 || img == nil {
		return
	}
	o := img.Rect.Min
	in := make([]image.Rectangle, 0, max(len(bands), 1))
	for _, b := range bands {
		in = append(in, b.Sub(o))
	}
	if len(in) == 0 {
		in = append(in, img.Rect.Sub(o))
	}
	if w.inputSent && native.SameRects(in, w.inputLast) {
		return
	}
	w.x11Send(x11ShapeRequestKind(w.inputMajor, x11ShapeKindInput, w.wid, in))
	w.inputLast, w.inputSent = in, true
}
