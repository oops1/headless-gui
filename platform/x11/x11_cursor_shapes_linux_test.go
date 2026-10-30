//go:build linux && !android

package x11

import (
	"encoding/binary"
	"testing"

	"github.com/oops1/headless-gui/v3/platform/pointer"
)

// X11: новые формы курсора и боковые кнопки мыши — на проводе.

func TestX11CursorGlyph_NewShapes(t *testing.T) {
	cases := map[int]int{
		pointer.CursorSizeAll:   xcFleur,
		pointer.CursorCrosshair: xcCrosshair,
		pointer.CursorWait:      xcWatch,
	}
	for shape, want := range cases {
		got, ok := x11CursorGlyph(shape)
		if !ok || got != want {
			t.Errorf("форма %d → глиф %d,%v; ждал %d", shape, got, ok, want)
		}
	}
	// Эти формы глифом не рисуются: невидимая и две руки.
	for _, shape := range []int{pointer.CursorNone, pointer.CursorGrab, pointer.CursorGrabbing} {
		if _, ok := x11CursorGlyph(shape); ok {
			t.Errorf("форма %d получила глиф шрифта, а должна строиться из карт", shape)
		}
	}
	// Значения глифов — из cursorfont.h.
	if xcCrosshair != 34 || xcFleur != 52 || xcWatch != 150 {
		t.Error("номера глифов crosshair/fleur/watch не совпадают с cursorfont.h")
	}
}

func x11CursorTestWindow(t *testing.T) (*X11Window, *x11Captured) {
	t.Helper()
	w, capt := newX11TestWindow(t)
	w.ridBase = 0x600000
	w.ridMask = 0x1FFFFF
	return w, capt
}

// CursorNone на X11: прозрачный курсор из карты 1×1 вешается на окно.
func TestX11SetCursor_NoneWire(t *testing.T) {
	w, capt := x11CursorTestWindow(t)
	w.SetCursor(pointer.CursorNone)
	reqs := capt.requests(t, w)

	var ops []byte
	for _, r := range reqs {
		ops = append(ops, r[0])
	}
	want := []byte{
		x11OpCreatePixmap, x11OpCreatePixmap, x11OpCreateGC, x11OpCreateGC,
		x11OpPolyFillRectangle, x11OpPolyFillRectangle,
		x11OpCreateCursor,
		x11OpFreeGC, x11OpFreeGC, x11OpFreePixmap, x11OpFreePixmap,
		2, // ChangeWindowAttributes
	}
	if string(ops) != string(want) {
		t.Fatalf("запросы на проводе %v, ждал %v", ops, want)
	}
	cursor := binary.LittleEndian.Uint32(reqs[6][4:8])
	cw := reqs[len(reqs)-1]
	if binary.LittleEndian.Uint32(cw[4:8]) != w.wid {
		t.Error("курсор повешен не на окно")
	}
	if binary.LittleEndian.Uint32(cw[12:16]) != cursor {
		t.Errorf("на окно повешен курсор %d, создавали %d", binary.LittleEndian.Uint32(cw[12:16]), cursor)
	}
	// Невидимая карта: ни одной закрашенной точки.
	for _, r := range reqs {
		if r[0] == x11OpPolyPoint {
			t.Error("у невидимого курсора есть закрашенные точки")
		}
	}
}

// Рука раскрытая и сжатая — свои курсоры, повторный запрос ресурса не плодит.
func TestX11SetCursor_GrabResourcesReused(t *testing.T) {
	w, _ := x11CursorTestWindow(t)
	w.cursors.mu.Lock()
	defer w.cursors.mu.Unlock()
	a := w.cursorResourceLocked(pointer.CursorGrab)
	b := w.cursorResourceLocked(pointer.CursorGrabbing)
	a2 := w.cursorResourceLocked(pointer.CursorGrab)
	n := w.cursorResourceLocked(pointer.CursorNone)
	n2 := w.cursorResourceLocked(pointer.CursorNone)
	if a == 0 || b == 0 || n == 0 {
		t.Fatalf("ресурсы не созданы: %d %d %d", a, b, n)
	}
	if a == b || a == n || b == n {
		t.Error("разные формы получили один ресурс")
	}
	if a2 != a || n2 != n {
		t.Error("повторный запрос создал новый ресурс")
	}
}

// Новые формы-глифы создаются из шрифта «cursor», как прежние.
func TestX11SetCursor_GlyphForms(t *testing.T) {
	w, capt := x11CursorTestWindow(t)
	w.SetCursor(pointer.CursorCrosshair)
	reqs := capt.requests(t, w)
	var glyph int
	for _, r := range reqs {
		if r[0] == 94 { // CreateGlyphCursor
			glyph = int(binary.LittleEndian.Uint16(r[16:18]))
		}
	}
	if glyph != xcCrosshair {
		t.Errorf("создан глиф %d, ждал %d", glyph, xcCrosshair)
	}
}

// Боковые кнопки X11: 8 и 9 → X1 и X2, нажатие и отпускание.
func TestX11_SideButtons(t *testing.T) {
	w, _ := newX11TestWindow(t)
	var got []mouseCallLinux
	w.onMouseButton = func(x, y, b int, p bool) { got = append(got, mouseCallLinux{x, y, b, p}) }

	w.handleX11Event(buttonPressEvent(10, 20, 8))
	w.handleX11Event(buttonReleaseEvent(10, 20, 8))
	w.handleX11Event(buttonPressEvent(10, 20, 9))
	w.handleX11Event(buttonReleaseEvent(10, 20, 9))
	want := []mouseCallLinux{
		{10, 20, 5, true}, {10, 20, 5, false},
		{10, 20, 6, true}, {10, 20, 6, false},
	}
	if len(got) != len(want) {
		t.Fatalf("доставлено %v, ждал %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("событие %d: %v, ждал %v", i, got[i], want[i])
		}
	}

	// Прежние кнопки не сдвинулись.
	got = nil
	w.handleX11Event(buttonPressEvent(1, 1, 1))
	w.handleX11Event(buttonPressEvent(1, 1, 2))
	w.handleX11Event(buttonPressEvent(1, 1, 3))
	w.handleX11Event(buttonPressEvent(1, 1, 10)) // неизвестная — отбрасывается
	if len(got) != 3 || got[0].b != 0 || got[1].b != 2 || got[2].b != 1 {
		t.Errorf("прежние кнопки: %v, ждал left=0, middle=2, right=1", got)
	}
}

// buttonReleaseEvent — синтетический ButtonRelease (тип 5).
func buttonReleaseEvent(x, y int16, button byte) []byte {
	b := buttonPressEvent(x, y, button)
	b[0] = 5
	return b
}
