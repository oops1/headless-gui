//go:build windows

package core

import (
	"image"
	"testing"
	"time"
	"unsafe"

	"github.com/oops1/headless-gui/v4/engine"
)

// Настоящее окно-попап Win32 на переходе (popup_fade.go): слоистое окно,
// прозрачность целиком из PopupTransition.Opacity, сдвиг — положением окна.

// screenPixel — цвет точки экрана (COLORREF 0x00BBGGRR); ok=false — экран
// не читается (сеанс заблокирован, нет рабочего стола).
func screenPixel(x, y int) (uint32, bool) {
	dc, _, _ := procScreenGetDC.Call(0)
	if dc == 0 {
		return 0, false
	}
	c, _, _ := procScreenGetPixel.Call(dc, uintptr(x), uintptr(y))
	procScreenReleaseDC.Call(0, dc)
	return uint32(c), uint32(c) != 0xFFFFFFFF
}

// settleScreen даёт DWM досмотреть анимацию показа окна.
func settleScreen() {
	for i := 0; i < 40; i++ {
		pumpWin32()
		time.Sleep(20 * time.Millisecond)
	}
}

// layeredAlpha — текущая прозрачность слоистого окна (0…255).
func layeredAlpha(t *testing.T, w *Win32Window) int {
	t.Helper()
	var key uint32
	var alpha byte
	var flags uint32
	r, _, _ := procGetLayeredWindowAttributes.Call(w.HWND(),
		uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&alpha)), uintptr(unsafe.Pointer(&flags)))
	if r == 0 {
		t.Fatal("GetLayeredWindowAttributes не прочитал окно — оно не слоистое?")
	}
	return int(alpha)
}

func TestWin32PopupFade_Live(t *testing.T) {
	carrier := newWin32TestWindow(t)
	h := newPopupHost(carrier, carrier, &pcEngine{}, 1, nil)
	t.Cleanup(func() { h.closeAll(); pumpWin32() })

	img := image.NewRGBA(image.Rect(0, 0, 100, 80))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	rect := image.Rect(10, 10, 110, 90)
	frame := func(tr *engine.PopupTransition) []engine.PopupFrame {
		return []engine.PopupFrame{{ID: 1, Rect: rect, Img: img, Transition: tr}}
	}

	h.apply(frame(&engine.PopupTransition{Opacity: 0, Offset: image.Pt(0, -6)}))
	pumpWin32()
	h.mu.Lock()
	hp := h.windows[1]
	h.mu.Unlock()
	if hp == nil || hp.native == nil {
		t.Skip("окно-попап не поднялось (нет рабочего стола?)")
	}
	pop := hp.native.(*Win32Window)
	ex, _, _ := procTestGetWindowLongPtrW.Call(pop.HWND(), testGWLExStyle)
	if ex&testWSExLayered == 0 {
		t.Fatal("окно-попап не слоистое")
	}
	if hp.fade != popupFadeWindow {
		t.Fatalf("вид прозрачности %v, ждали прозрачность окна", hp.fade)
	}
	if a := layeredAlpha(t, pop); a != 0 {
		t.Fatalf("окно родилось с прозрачностью %d, ждали 0 — мигнуло бы целиком", a)
	}
	cx, cy := carrier.GetPosition()
	_, y0 := pop.GetPosition()
	if y0 != cy+10-6 {
		t.Errorf("начало выезда: верх окна %d, ждали %d", y0, cy+10-6)
	}

	h.apply(frame(&engine.PopupTransition{Opacity: 0.5, Offset: image.Pt(0, -3)}))
	pumpWin32()
	if a := layeredAlpha(t, pop); a < 126 || a > 129 {
		t.Fatalf("середина: прозрачность %d, ждали около 128", a)
	}
	if _, y := pop.GetPosition(); y != cy+10-3 {
		t.Errorf("середина выезда: верх %d, ждали %d", y, cy+10-3)
	}

	h.apply(frame(nil))
	pumpWin32()
	if a := layeredAlpha(t, pop); a != 255 {
		t.Fatalf("в покое прозрачность %d, ждали 255", a)
	}
	if x, y := pop.GetPosition(); x != cx+10 || y != cy+10 {
		t.Errorf("в покое окно в (%d,%d), ждали (%d,%d)", x, y, cx+10, cy+10)
	}
}

// На экране: слоистое окно с прозрачностью 0 попапа не показывает, в покое —
// показывает его картинку как прежде (StretchDIBits в слоистом окне).
func TestWin32PopupFade_LivePixels(t *testing.T) {
	carrier := newWin32TestWindow(t)
	h := newPopupHost(carrier, carrier, &pcEngine{}, 1, nil)
	t.Cleanup(func() { h.closeAll(); pumpWin32() })
	img := image.NewRGBA(image.Rect(0, 0, 100, 80))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 220, 30, 40, 255
	}
	const want = 0x281edc // COLORREF: B=0x28 G=0x1e R=0xdc
	rect := image.Rect(10, 10, 110, 90)
	frame := func(tr *engine.PopupTransition) []engine.PopupFrame {
		return []engine.PopupFrame{{ID: 1, Rect: rect, Img: img, Transition: tr}}
	}
	h.apply(frame(&engine.PopupTransition{Opacity: 0}))
	settleScreen()
	h.mu.Lock()
	hp := h.windows[1]
	h.mu.Unlock()
	if hp == nil || hp.native == nil {
		t.Skip("окно-попап не поднялось (нет рабочего стола?)")
	}
	pop := hp.native.(*Win32Window)
	x, y := pop.GetPosition()
	c, ok := screenPixel(x+50, y+40)
	if !ok {
		t.Skip("экран не читается")
	}
	if c == want {
		t.Fatal("при прозрачности 0 на экране цвет попапа")
	}
	h.apply(frame(nil))
	settleScreen()
	if c, _ := screenPixel(x+50, y+40); c != want {
		t.Fatalf("в покое на экране %06x, ждали картинку попапа %06x", c, want)
	}
}
