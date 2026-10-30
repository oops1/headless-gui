//go:build linux && !android

package x11

import (
	"encoding/binary"
	"image"
	"image/color"
	"testing"
)

// Кадр каскадного меню: два сплошных тела, вокруг полупрозрачная тень.
func shapeTestFrame() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	paint := func(r image.Rectangle, a uint8) {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				if img.RGBAAt(x, y).A < a {
					img.SetRGBA(x, y, color.RGBA{A: a})
				}
			}
		}
	}
	paint(image.Rect(2, 2, 30, 40), 60)
	paint(image.Rect(4, 4, 28, 38), 255)
	paint(image.Rect(30, 10, 62, 30), 255)
	return img
}

const shapeTestMajor = 130

func shapeRequests(fx *argbFakeServer) [][]byte {
	var out [][]byte
	for _, r := range fx.saw() {
		if r[0] == shapeTestMajor && r[1] == x11ShapeRectanglesMinor {
			out = append(out, r)
		}
	}
	return out
}

// Без композитора (нет владельца _NET_WM_CM_S0, не XWayland) 32-битный попап
// выкраивается по сплошным точкам: тень, которую некому смешать, вырезана.
// Одинаковая форма не шлётся повторно, изменившаяся — шлётся.
func TestX11Popup_NoCompositorShapesToSolid(t *testing.T) {
	w, fx := newArgbPopupWindow(t, 0x49, false)
	fx.exts = map[string]byte{"SHAPE": shapeTestMajor}
	argbCreatePopup(t, w)
	if w.shapeMajor != shapeTestMajor {
		t.Fatalf("shapeMajor %d: без композитора форма должна включиться", w.shapeMajor)
	}

	img := shapeTestFrame()
	w.BlitPopupRegion(img, nil)
	argbWaitReq(t, fx, 72, 1) // PutImage после формы — значит, форма уже у сервера
	reqs := shapeRequests(fx)
	if len(reqs) != 1 {
		t.Fatalf("ShapeRectangles: %d запросов, ждали 1", len(reqs))
	}
	in := func(pt image.Point, req []byte) bool {
		for o := 16; o+8 <= len(req); o += 8 {
			x, y := int(int16(binary.LittleEndian.Uint16(req[o:]))), int(int16(binary.LittleEndian.Uint16(req[o+2:])))
			wd, ht := int(binary.LittleEndian.Uint16(req[o+4:])), int(binary.LittleEndian.Uint16(req[o+6:]))
			if pt.In(image.Rect(x, y, x+wd, y+ht)) {
				return true
			}
		}
		return false
	}
	if in(image.Pt(3, 20), reqs[0]) {
		t.Error("точка тени попала в форму окна — без композитора она будет чёрной")
	}
	if !in(image.Pt(10, 20), reqs[0]) || !in(image.Pt(50, 20), reqs[0]) {
		t.Error("тела меню выпали из формы")
	}
	if in(image.Pt(50, 40), reqs[0]) {
		t.Error("щель между уровнями попала в форму")
	}

	w.BlitPopupRegion(img, nil)
	argbWaitReq(t, fx, 72, 2)
	if n := len(shapeRequests(fx)); n != 1 {
		t.Errorf("та же форма отправлена повторно: %d запросов", n)
	}
	img2 := shapeTestFrame()
	for y := 30; y < 40; y++ {
		for x := 30; x < 62; x++ {
			img2.SetRGBA(x, y, color.RGBA{A: 255})
		}
	}
	w.BlitPopupRegion(img2, nil)
	argbWaitReq(t, fx, 72, 3)
	if n := len(shapeRequests(fx)); n != 2 {
		t.Errorf("изменившаяся форма не отправлена: %d запросов", n)
	}
}

// С композитором (есть владелец _NET_WM_CM_S0) альфу смешивают: 32-битный
// попап остаётся прямоугольным, с мягкой тенью.
func TestX11Popup_CompositorKeepsAlpha(t *testing.T) {
	w, fx := newArgbPopupWindow(t, 0x49, false)
	fx.exts = map[string]byte{"SHAPE": shapeTestMajor}
	fx.cmOwner = 0x200001
	argbCreatePopup(t, w)
	if w.shapeMajor != 0 {
		t.Fatalf("shapeMajor %d: с композитором форма не нужна", w.shapeMajor)
	}
	w.BlitPopupRegion(shapeTestFrame(), nil)
	argbWaitReq(t, fx, 72, 1)
	if n := len(shapeRequests(fx)); n != 0 {
		t.Errorf("ShapeRectangles при композиторе: %d", n)
	}
}

// XWayland композирует ARGB-окна сам, даже без владельца выделения.
func TestX11Popup_XWaylandKeepsAlpha(t *testing.T) {
	w, fx := newArgbPopupWindow(t, 0x49, false)
	fx.exts = map[string]byte{"SHAPE": shapeTestMajor, "XWAYLAND": 140}
	argbCreatePopup(t, w)
	if w.shapeMajor != 0 {
		t.Fatalf("shapeMajor %d: XWayland смешивает альфу сам", w.shapeMajor)
	}
}

// 24-битный попап альфы не несёт вовсе — выкраивается всегда.
func TestX11Popup_24BitAlwaysShapes(t *testing.T) {
	w, fx := newArgbPopupWindow(t, 0, false)
	fx.exts = map[string]byte{"SHAPE": shapeTestMajor}
	fx.cmOwner = 0x200001
	argbCreatePopup(t, w)
	if w.shapeMajor != shapeTestMajor {
		t.Fatalf("shapeMajor %d: 24-битному окну нужна форма и при композиторе", w.shapeMajor)
	}
}
