//go:build linux && !android

package wayland

import (
	"image"
	"image/color"
	"testing"

	"github.com/oops1/headless-gui/v4/core/raster"
)

// Тень вынесенного меню не объявляется непрозрачной (замечание WinLine после
// v3.35.1). Компоновщик, поверивший wl_surface.set_opaque_region (mutter,
// WinLine), копирует эти точки без смешивания, и полупрозрачная тень меню
// Блокнота превращалась в сплошную чёрную рамку. Область ввода тень по-
// прежнему включает: щелчок по ней принадлежит меню, как и раньше.
func TestWaylandPopup_ShadowIsNotOpaque(t *testing.T) {
	c := newWlTestWindow(t)
	if c.w.compositorID == 0 {
		c.w.compositorID = 3
	}
	img := image.NewRGBA(image.Rect(0, 0, 300, 200))
	paint := func(r image.Rectangle, a uint8) {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				if img.RGBAAt(x, y).A < a {
					img.SetRGBA(x, y, color.RGBA{A: a})
				}
			}
		}
	}
	menu, sub := image.Rect(8, 8, 120, 150), image.Rect(128, 40, 290, 120)
	paint(menu.Inset(-6), 60)
	paint(sub.Inset(-6), 60)
	paint(menu, 255)
	paint(sub, 255)

	p := &wlPopup{surfaceID: 900}
	c.w.popupSetRegions(p, raster.OpaqueBands(img), raster.SolidBands(img))
	msgs := c.readAllWl(t)

	regions := map[uint32][]image.Rectangle{}
	set := map[uint16]uint32{}
	for _, m := range msgs {
		switch {
		case m.obj == c.w.compositorID && m.opcode == wlCompositorCreateRegion:
			regions[m.args[0]] = nil
		case m.opcode == wlRegionAdd && len(m.args) == 4:
			if _, ok := regions[m.obj]; ok {
				x, y := int(int32(m.args[0])), int(int32(m.args[1]))
				regions[m.obj] = append(regions[m.obj], image.Rect(x, y, x+int(int32(m.args[2])), y+int(int32(m.args[3]))))
			}
		case m.obj == p.surfaceID && (m.opcode == wlSurfaceSetOpaqueRegion || m.opcode == wlSurfaceSetInputRegion):
			set[m.opcode] = m.args[0]
		}
	}
	in := func(pt image.Point, rs []image.Rectangle) bool {
		for _, r := range rs {
			if pt.In(r) {
				return true
			}
		}
		return false
	}
	opaque, input := regions[set[wlSurfaceSetOpaqueRegion]], regions[set[wlSurfaceSetInputRegion]]
	if len(opaque) == 0 || len(input) == 0 {
		t.Fatalf("области не объявлены: непрозрачная %v, ввода %v", opaque, input)
	}
	shadow := image.Pt(menu.Min.X-3, menu.Min.Y+20)
	if in(shadow, opaque) {
		t.Errorf("точка тени %v объявлена непрозрачной — компоновщик нарисует её чёрной", shadow)
	}
	if !in(shadow, input) {
		t.Errorf("точка тени %v выпала из области ввода", shadow)
	}
	for _, pt := range []image.Point{{60, 80}, {200, 80}} {
		if !in(pt, opaque) {
			t.Errorf("тело меню %v не объявлено непрозрачным", pt)
		}
	}
	if gap := image.Pt(200, 170); in(gap, opaque) || in(gap, input) {
		t.Errorf("дыра между уровнями %v попала в области", gap)
	}
}
