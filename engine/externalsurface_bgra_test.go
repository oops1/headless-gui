package engine

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
)

// Буфер в порядке B,G,R,A (ExternalSurface.SetBGRA) — так пишет wl_shm:
// окно программы на холсте BGRX ложится копированием строк, а на холсте
// RGBA — с перестановкой; в обоих случаях в back те же байты, что дал бы
// тот же цвет из буфера RGBA.

// bgraRig — поверхность 8×8 в (4,4) с буфером одного пикселя-цвета: bgra
// задаёт порядок байт буфера, a — альфу (premultiplied).
func bgraRig(t *testing.T, format PixelFormat, bgra bool, r, g, b, a byte) *esRig {
	t.Helper()
	rig := newESRig(t, 32, 32, 1, image.Rect(4, 4, 12, 12))
	if err := rig.e.SetPixelFormat(format); err != nil {
		t.Fatal(err)
	}
	buf := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < len(buf.Pix); i += 4 {
		if bgra {
			buf.Pix[i], buf.Pix[i+1], buf.Pix[i+2], buf.Pix[i+3] = b, g, r, a
		} else {
			buf.Pix[i], buf.Pix[i+1], buf.Pix[i+2], buf.Pix[i+3] = r, g, b, a
		}
	}
	rig.surf.SetBGRA(bgra)
	rig.surf.SetBuffer(buf, a == 0xff)
	rig.frame()
	return rig
}

// backBytes — четыре байта кадра в точке, как они лежат (front: при одном
// буфере кадра back холста движка — окно, ADR-0007).
func (r *esRig) backBytes(x, y int) [4]byte {
	o := r.e.canvas.Front().PixOffset(x, y)
	return [4]byte(r.e.canvas.Front().Pix[o : o+4])
}

func TestExternalSurfaceBGRAOnBGRXCanvasIsCopied(t *testing.T) {
	testreset.Globals(t)
	rig := bgraRig(t, FormatBGRX, true, 0x33, 0x22, 0x11, 0xff)
	if got := rig.backBytes(6, 6); got != [4]byte{0x11, 0x22, 0x33, 0xff} {
		t.Fatalf("back %x, want the buffer's bytes 11 22 33 ff", got)
	}
	ref := bgraRig(t, FormatBGRX, false, 0x33, 0x22, 0x11, 0xff)
	if got, want := rig.backBytes(6, 6), ref.backBytes(6, 6); got != want {
		t.Fatalf("BGRA buffer gives %x, the same colour from RGBA gives %x", got, want)
	}
}

func TestExternalSurfaceBGRAOnRGBACanvasIsSwapped(t *testing.T) {
	testreset.Globals(t)
	rig := bgraRig(t, FormatRGBA, true, 0x33, 0x22, 0x11, 0xff)
	if got := rig.backBytes(6, 6); got != [4]byte{0x33, 0x22, 0x11, 0xff} {
		t.Fatalf("back %x, want 33 22 11 ff", got)
	}
}

// С альфой: смешивание с фоном панели то же, что у буфера RGBA.
func TestExternalSurfaceBGRAWithAlphaBlendsAsRGBA(t *testing.T) {
	testreset.Globals(t)
	for _, f := range []PixelFormat{FormatBGRX, FormatRGBA} {
		rig := bgraRig(t, f, true, 0x10, 0x40, 0x20, 0x80)
		ref := bgraRig(t, f, false, 0x10, 0x40, 0x20, 0x80)
		if got, want := rig.backBytes(6, 6), ref.backBytes(6, 6); got != want {
			t.Fatalf("%v: BGRA buffer with alpha gives %x, RGBA gives %x", f, got, want)
		}
		if got := rig.backBytes(1, 1); got == rig.backBytes(6, 6) {
			t.Fatalf("%v: the surface did not draw", f)
		}
	}
}
