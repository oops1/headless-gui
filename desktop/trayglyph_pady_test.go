package desktop

// Размер глифа значка трея не зависит от PadY стиля и от флага tray.fill.strip
// (замечание WinLine после v3.32.0, п.1).

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/internal/testreset"
	"github.com/oops1/headless-gui/v3/theme"
)

// Без tray.fill.strip значок трея стоит в границах ровно по размеру значка;
// PadY стиля раньше отъедал от глифа (у Windows 2000 вдвое). Теперь глиф —
// tray.icon.size при любом PadY, и тот же размер при подсветке на всю полосу.
func TestTrayGlyph_SizeIgnoresPadY(t *testing.T) {
	testreset.Globals(t)
	for _, padY := range []float64{0, 2, 4, 6, 12} {
		for _, strip := range []bool{false, true} {
			tm := themeWithIcons(t, map[theme.Key]string{"tray.network.icon": "net"}, func(p *theme.Profile) {
				for _, c := range []string{ComponentNetwork, ComponentTrayIcon} {
					p.SetStyle(c, "", theme.StateNormal, theme.StyleDelta{PadX: theme.N(2), PadY: theme.N(padY)})
				}
				p.SetFlag(KeyTrayFillStrip, strip)
			})
			h := 16
			if strip {
				h = 40 // подсветка на всю полосу: границы выше значка
			}
			b := image.Rect(0, 0, 20, h)

			net := NewNetworkStatus(tm, NewFakeSystemStatus())
			net.SetBounds(b)
			ctx := &hiCtx{k: 1}
			net.Draw(ctx)
			net.Close()
			if len(ctx.images) != 1 {
				t.Fatalf("padY=%v strip=%v: сеть нарисована %d картинками", padY, strip, len(ctx.images))
			}
			if r := ctx.images[0]; r.w != 16 || r.h != 16 {
				t.Errorf("padY=%v strip=%v: глиф сети %dx%d, ожидалось 16x16", padY, strip, r.w, r.h)
			} else if want := (h - 16) / 2; r.y != want {
				t.Errorf("padY=%v strip=%v: глиф сети по Y=%d, ожидалось %d (по центру)", padY, strip, r.y, want)
			}

			ic, err := NewTraySVGIcon(tm, []byte(solidSVG))
			if err != nil {
				t.Fatal(err)
			}
			ic.SetBounds(b)
			ctx = &hiCtx{k: 1}
			ic.Draw(ctx)
			if len(ctx.images) != 1 {
				t.Fatalf("padY=%v strip=%v: обобщённый значок нарисован %d картинками", padY, strip, len(ctx.images))
			}
			if got := ctx.images[0].img.Bounds().Dx(); got != 16 {
				t.Errorf("padY=%v strip=%v: растр обобщённого значка %d px, ожидалось 16", padY, strip, got)
			}
		}
	}
}
