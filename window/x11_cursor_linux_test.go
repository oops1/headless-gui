//go:build linux && !android

package window

import "testing"

// Курсор над полем ввода, над краем окна и над ссылкой должен выглядеть
// по-разному — по этому человек и понимает, что здесь можно делать. На X11
// окно всегда показывало стрелку: форм курсора там не было вовсе.

func TestX11CursorGlyph_AllShapesMapped(t *testing.T) {
	cases := map[int]int{
		wlCursorArrow:    xcLeftPtr,
		wlCursorIBeam:    xcXterm,
		wlCursorHand:     xcHand2,
		wlCursorSizeWE:   xcSbHDoubleArrow,
		wlCursorSizeNS:   xcSbVDoubleArrow,
		wlCursorSizeNWSE: xcTopLeftCorner,
		wlCursorSizeNESW: xcTopRightCorner,
	}
	for shape, want := range cases {
		got, ok := x11CursorGlyph(shape)
		if !ok {
			t.Errorf("форма %d не отображена в глиф", shape)
			continue
		}
		if got != want {
			t.Errorf("форма %d → глиф %d, ждал %d", shape, got, want)
		}
	}
	// Каждой форме — свой глиф: одинаковые означали бы, что курсор над
	// полем ввода и над краем окна не отличить.
	seen := map[int]int{}
	for shape := range cases {
		g, _ := x11CursorGlyph(shape)
		if other, dup := seen[g]; dup {
			t.Errorf("формы %d и %d делят глиф %d", other, shape, g)
		}
		seen[g] = shape
	}
}

// Незнакомая форма — не повод рисовать случайный глиф: окно оставит прежний
// курсор.
func TestX11CursorGlyph_UnknownShape(t *testing.T) {
	if _, ok := x11CursorGlyph(wlCursorCount + 3); ok {
		t.Error("незнакомая форма получила глиф")
	}
}

// Ресурс курсора создаётся один раз на форму: курсор меняется сотни раз за
// минуту, и заводить ресурс X-сервера на каждое движение мыши нельзя.
func TestX11Cursors_ResourceReused(t *testing.T) {
	w := &X11Window{wid: 1}
	w.ridBase = 0x600000
	w.ridMask = 0x1FFFFF

	w.cursors.mu.Lock()
	defer w.cursors.mu.Unlock()
	first := w.cursorResourceLocked(wlCursorIBeam)
	again := w.cursorResourceLocked(wlCursorIBeam)
	other := w.cursorResourceLocked(wlCursorHand)

	if first == 0 {
		t.Fatal("ресурс курсора не создан")
	}
	if again != first {
		t.Errorf("повторный запрос дал новый ресурс: %d против %d", again, first)
	}
	if other == first {
		t.Error("разные формы получили один ресурс")
	}
}
