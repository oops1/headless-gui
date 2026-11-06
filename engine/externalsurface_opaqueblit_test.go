package engine

import (
	"bytes"
	"fmt"
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/internal/testreset"
)

// Непрозрачная область от клиента (ExternalSurface.SetOpaqueRegion) доходит до
// вывода: у буфера с альфой объявленные части копируются, смешивается только
// остальное (пилот сцены WinLine, ENGINE_ISSUES «SetOpaqueRegion does not reach
// the blit»). Окно со скруглёнными углами раньше смешивалось целиком — или
// выдавало себя за непрозрачное.

// windowBuf — буфер окна w×h с прозрачными углами в corner пикселей и
// полупрозрачной кромкой по периметру: тело непрозрачно. bgra — порядок байт.
func windowBuf(w, h, corner int, bgra bool) *image.RGBA {
	buf := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := buf.PixOffset(x, y)
			inCorner := (x < corner || x >= w-corner) && (y < corner || y >= h-corner)
			edge := x == 0 || y == 0 || x == w-1 || y == h-1
			var r, g, b, a byte
			switch {
			case inCorner:
				// прозрачно
			case edge:
				r, g, b, a = 0x20, 0x30, 0x40, 0x80 // premultiplied
			default:
				r, g, b, a = byte(x*5), byte(y*7), byte(x+y), 0xff
			}
			if bgra {
				r, b = b, r
			}
			buf.Pix[o], buf.Pix[o+1], buf.Pix[o+2], buf.Pix[o+3] = r, g, b, a
		}
	}
	return buf
}

// windowOpaque — тело окна windowBuf без углов и кромки: два перекрывающихся
// прямоугольника, как отдаёт wl_surface.set_opaque_region.
func windowOpaque(w, h, corner int) []image.Rectangle {
	return []image.Rectangle{
		image.Rect(corner, 1, w-corner, h-1),
		image.Rect(1, corner, w-1, h-corner),
	}
}

func frontCopy(e *Engine) []byte {
	return append([]byte(nil), e.canvas.Front().Pix...)
}

// Картинка с объявленной непрозрачной областью побайтно та же, что при
// смешивании всего буфера: форматы холста, порядок байт буфера, масштаб.
func TestExternalSurface_OpaqueRegionBlitMatchesBlend(t *testing.T) {
	testreset.Globals(t)
	const w, h, corner = 40, 28, 6
	for _, format := range []PixelFormat{FormatRGBA, FormatBGRX} {
		for _, bgra := range []bool{false, true} {
			for _, scale := range []float64{1, 1.5} {
				name := fmt.Sprintf("format=%d bgra=%v scale=%v", format, bgra, scale)
				draw := func(withRegion bool) []byte {
					rig := newESRig(t, 96, 80, scale, image.Rect(7, 9, 7+w, 9+h))
					if err := rig.e.SetPixelFormat(format); err != nil {
						t.Fatal(err)
					}
					rig.surf.SetBGRA(bgra)
					rig.surf.SetBuffer(windowBuf(w, h, corner, bgra), false)
					if withRegion {
						rig.surf.SetOpaqueRegion(windowOpaque(w, h, corner))
					}
					rig.frame()
					return frontCopy(rig.e)
				}
				blend, split := draw(false), draw(true)
				if !bytes.Equal(blend, split) {
					t.Errorf("%s: кадр с непрозрачной областью отличается от смешанного целиком", name)
				}
			}
		}
	}
}

// Объявленная часть действительно копируется, а не смешивается: пиксель с
// нулевой альфой внутри неё (нарушенное обещание) затирает фон, а такой же
// пиксель вне её фон оставляет.
func TestExternalSurface_OpaqueRegionIsCopied(t *testing.T) {
	testreset.Globals(t)
	for _, format := range []PixelFormat{FormatRGBA, FormatBGRX} {
		rig := newESRig(t, 64, 64, 1, image.Rect(8, 8, 40, 40))
		if err := rig.e.SetPixelFormat(format); err != nil {
			t.Fatal(err)
		}
		buf := image.NewRGBA(image.Rect(0, 0, 32, 32)) // весь прозрачный
		rig.surf.SetBuffer(buf, false)
		rig.frame()
		bg := rig.backBytes(2, 2)
		if got := rig.backBytes(20, 20); got != bg {
			t.Fatalf("format=%d: прозрачный буфер без области закрыл фон: %x", format, got)
		}

		rig.surf.SetOpaqueRegion([]image.Rectangle{image.Rect(4, 4, 16, 16)})
		rig.frame()
		if got := rig.backBytes(8+8, 8+8); got == bg {
			t.Errorf("format=%d: объявленная непрозрачной часть смешана, а не скопирована", format)
		}
		if got := rig.backBytes(8+24, 8+24); got != bg {
			t.Errorf("format=%d: часть вне области скопирована поверх фона: %x", format, got)
		}
		// Край области: точка сразу за ней — смешивается.
		if got := rig.backBytes(8+16, 8+10); got != bg {
			t.Errorf("format=%d: точка сразу за областью скопирована: %x", format, got)
		}

		// Снятая область — снова смешивание всего буфера.
		rig.surf.SetOpaqueRegion(nil)
		rig.frame()
		if got := rig.backBytes(8+8, 8+8); got != bg {
			t.Errorf("format=%d: после снятия области часть осталась скопированной: %x", format, got)
		}
	}
}

// Частичный кадр: меняется кусок буфера на стыке копируемой части и остатка —
// кадр по повреждению равен кадру, нарисованному целиком.
func TestExternalSurface_OpaqueRegionPartialFrame(t *testing.T) {
	testreset.Globals(t)
	const w, h, corner = 40, 28, 6
	rig := newESRig(t, 96, 80, 1, image.Rect(7, 9, 7+w, 9+h))
	buf := windowBuf(w, h, corner, false)
	rig.surf.SetBuffer(buf, false)
	rig.surf.SetOpaqueRegion(windowOpaque(w, h, corner))
	rig.frame()

	patch := image.Rect(2, 2, 14, 12) // угол, кромка и тело разом
	rig.surf.Update(func(b *image.RGBA) []image.Rectangle {
		for y := patch.Min.Y; y < patch.Max.Y; y++ {
			for x := patch.Min.X; x < patch.Max.X; x++ {
				o := b.PixOffset(x, y)
				if b.Pix[o+3] == 0xff {
					b.Pix[o], b.Pix[o+1], b.Pix[o+2] = 0xee, 0x11, 0x77
				}
			}
		}
		return []image.Rectangle{patch}
	})
	rig.frame()
	partial := frontCopy(rig.e)

	rig.e.Invalidate()
	rig.frame()
	if whole := frontCopy(rig.e); !bytes.Equal(partial, whole) {
		t.Error("частичный кадр с непрозрачной областью отличается от полного")
	}
}

// Слишком дробная область на вывод не влияет: буфер смешивается целиком, и
// нарушенное обещание в ней фон не затирает.
func TestExternalSurface_OpaqueRegionTooFragmentedBlends(t *testing.T) {
	testreset.Globals(t)
	rig := newESRig(t, 64, 64, 1, image.Rect(8, 8, 48, 48))
	rig.surf.SetBuffer(image.NewRGBA(image.Rect(0, 0, 40, 40)), false)
	var rects []image.Rectangle
	for i := 0; i < 20; i++ {
		rects = append(rects, image.Rect(i*2, 0, i*2+1, 40))
	}
	rig.surf.SetOpaqueRegion(rects)
	rig.frame()
	if got, bg := rig.backBytes(8+4, 8+10), rig.backBytes(2, 2); got != bg {
		t.Errorf("область из %d частей дошла до вывода: %x вместо фона %x", len(rects), got, bg)
	}
}
