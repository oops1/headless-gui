//go:build windows

package core

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v4/engine"
	"github.com/oops1/headless-gui/v4/internal/testhook"
)

// Окно-попап Win32 смешивает альфу точек кадра с тем, что под ним
// (UpdateLayeredWindow). Раньше прозрачное в кадре — точки за скруглёнными
// углами меню, его тень — выходило на экране чёрным.

// ulwFrame — кадр 100×80: прозрачный угол 12×12 слева сверху, полупрозрачная
// полоса (premultiplied: чёрный с альфой 128) по нижнему краю, остальное —
// сплошной красный.
func ulwFrame() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 100, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 100; x++ {
			i := img.PixOffset(x, y)
			switch {
			case x < 12 && y < 12:
				// прозрачно: нули
			case y >= 70:
				img.Pix[i+3] = 128
			default:
				img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 220, 30, 40, 255
			}
		}
	}
	return img
}

func TestWin32PopupULW_TransparentCornerShowsBelow(t *testing.T) {
	carrier := newWin32TestWindow(t)
	h := newPopupHost(carrier, carrier, &pcEngine{}, 1, nil)
	t.Cleanup(func() { h.closeAll(); pumpWin32() })
	settleScreen()

	cx, cy := carrier.GetPosition()
	rect := image.Rect(40, 40, 140, 120)
	px, py := cx+rect.Min.X, cy+rect.Min.Y
	under, ok := screenPixel(px+4, py+4)
	if !ok {
		t.Skip("экран не читается")
	}
	underShadow, _ := screenPixel(px+50, py+75)

	h.apply([]engine.PopupFrame{{ID: 1, Rect: rect, Img: ulwFrame()}})
	settleScreen()
	h.mu.Lock()
	hp := h.windows[1]
	h.mu.Unlock()
	if hp == nil || hp.native == nil {
		t.Skip("окно-попап не поднялось (нет рабочего стола?)")
	}
	pop := hp.native.(*Win32Window)
	if x, y := pop.GetPosition(); x != px || y != py {
		t.Fatalf("окно-попап в (%d,%d), ждали (%d,%d)", x, y, px, py)
	}
	if c, _ := screenPixel(px+50, py+40); c != 0x281edc {
		t.Fatalf("тело попапа на экране %06x, ждали картинку 281edc", c)
	}
	if c, _ := screenPixel(px+4, py+4); c != under {
		t.Errorf("прозрачный угол показывает %06x, а под окном было %06x (чёрный угол?)", c, under)
	}
	// Полупрозрачная тень темнит то, что под ней, но не заливает чёрным.
	c, _ := screenPixel(px+50, py+75)
	if c == underShadow && underShadow != 0 {
		t.Errorf("полупрозрачная полоса не легла на экран: %06x", c)
	}
	if c == 0 && underShadow != 0 {
		t.Errorf("полупрозрачная полоса вышла сплошь чёрной")
	}
}

// Кадр, сменивший размер (раскрылось подменю), ложится вместе с новым местом
// окна одним вызовом; после смены картинки окно показывает новую.
func TestWin32PopupULW_ResizeAndMove(t *testing.T) {
	carrier := newWin32TestWindow(t)
	h := newPopupHost(carrier, carrier, &pcEngine{}, 1, nil)
	t.Cleanup(func() { h.closeAll(); pumpWin32() })

	h.apply([]engine.PopupFrame{{ID: 1, Rect: image.Rect(10, 10, 110, 90), Img: ulwFrame()}})
	settleScreen()
	h.mu.Lock()
	hp := h.windows[1]
	h.mu.Unlock()
	if hp == nil || hp.native == nil {
		t.Skip("окно-попап не поднялось (нет рабочего стола?)")
	}
	pop := hp.native.(*Win32Window)

	big := image.NewRGBA(image.Rect(0, 0, 160, 120))
	for i := 0; i < len(big.Pix); i += 4 {
		big.Pix[i], big.Pix[i+1], big.Pix[i+2], big.Pix[i+3] = 20, 200, 60, 255
	}
	h.apply([]engine.PopupFrame{{ID: 1, Rect: image.Rect(30, 20, 190, 140), Img: big}})
	settleScreen()
	cx, cy := carrier.GetPosition()
	r := testhook.Win32WindowRect(pop)
	if r != image.Rect(cx+30, cy+20, cx+190, cy+140) {
		t.Fatalf("окно-попап занимает %v, ждали %v", r, image.Rect(cx+30, cy+20, cx+190, cy+140))
	}
	c, ok := screenPixel(cx+30+80, cy+20+60)
	if !ok {
		t.Skip("экран не читается")
	}
	if c != 0x3cc814 {
		t.Errorf("после смены кадра на экране %06x, ждали 3cc814", c)
	}
}

// Попап, которому тень и скругление даёт ОС (PopupFrame.System, Windows 11
// с тенями DWM): угол окна скруглён — там видно то, что под окном, а под
// окном — тень DWM.
func TestWin32PopupSystem_DWMRoundsAndShadows(t *testing.T) {
	carrier := newWin32TestWindow(t)
	if !carrier.SystemPopupShadow() {
		t.Skip("ОС не даёт тень попапам (не Windows 11 или тени DWM выключены)")
	}
	h := newPopupHost(carrier, carrier, &pcEngine{}, 1, nil)
	t.Cleanup(func() { h.closeAll(); pumpWin32() })
	settleScreen()
	cx, cy := carrier.GetPosition()
	rect := image.Rect(40, 40, 240, 190)
	px, py := cx+rect.Min.X, cy+rect.Min.Y
	under, ok := screenPixel(px, py)
	if !ok {
		t.Skip("экран не читается")
	}
	underBelow, _ := screenPixel(px+100, py+150+4)

	img := image.NewRGBA(image.Rect(0, 0, 200, 150))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 220, 30, 40, 255
	}
	h.apply([]engine.PopupFrame{{ID: 1, Rect: rect, Img: img, System: true, Corner: 8}})
	settleScreen()
	h.mu.Lock()
	hp := h.windows[1]
	h.mu.Unlock()
	if hp == nil || hp.native == nil {
		t.Skip("окно-попап не поднялось")
	}
	if c, _ := screenPixel(px+100, py+75); c != 0x281edc {
		t.Fatalf("тело попапа %06x, ждали 281edc", c)
	}
	if c, _ := screenPixel(px, py); c == 0x281edc || c == 0 {
		t.Errorf("угол окна %06x — не скруглён (под окном было %06x)", c, under)
	}
	if c, _ := screenPixel(px+100, py+150+4); lumOf(c) >= lumOf(underBelow) {
		t.Errorf("под окном %06x не темнее %06x — тени DWM нет", c, underBelow)
	}
}

func lumOf(c uint32) int { return int(c&0xFF) + int(c>>8&0xFF) + int(c>>16&0xFF) }
