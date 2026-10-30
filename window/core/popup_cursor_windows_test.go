//go:build windows

package core

import (
	"image"
	"testing"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/internal/testhook"
	"github.com/oops1/headless-gui/v3/widget"
)

// Принудительный курсор и настоящие окна-попапы Win32: хост поднимает окно
// (CreatePopup на потоке носителя), и форма, заданная Engine.SetCursorOverride,
// должна действовать над ним — и когда override задан до открытия попапа, и
// когда после. Курсор пользователя не двигается: проверяется выбранная окном
// форма (то, что вернёт WM_SETCURSOR), а не положение мыши.

func pcWin32Popup(t *testing.T, h *popupHost, id uintptr) *Win32Window {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 100, 80))
	h.apply([]engine.PopupFrame{{ID: id, Rect: image.Rect(10, 10, 110, 90), Img: img}})
	pumpWin32() // createPopup — вызов потока окна носителя
	h.mu.Lock()
	hp := h.windows[id]
	h.mu.Unlock()
	if hp == nil || hp.native == nil {
		t.Skip("окно-попап не поднялось (нет рабочего стола?)")
	}
	pop, ok := hp.native.(*Win32Window)
	if !ok {
		t.Fatalf("окно попапа %T, ждал *Win32Window", hp.native)
	}
	return pop
}

func TestWin32PopupHost_CursorOverride(t *testing.T) {
	carrier := newWin32TestWindow(t)
	eng := &pcEngine{}
	h := newPopupHost(carrier, carrier, eng, 1, nil)
	t.Cleanup(func() { h.closeAll(); pumpWin32() })

	// Попап открыт — override приходит позже.
	pop := pcWin32Popup(t, h, 1)
	if w32CursorHidden(pop) {
		t.Fatal("курсор попапа спрятан до всякого override")
	}
	eng.cur, eng.set = widget.CursorNone, true
	h.syncCursorOverride()
	pumpWin32()
	if !w32CursorHidden(pop) {
		t.Error("override None не дошёл до открытого окна-попапа")
	}
	if h, handled := testhook.Win32SetCursorMessage(pop, testHTClient); !handled || h != 0 {
		t.Errorf("WM_SETCURSOR попапа: handled=%v h=%#x, ждал спрятанный курсор", handled, h)
	}

	// Сменили форму.
	eng.cur = widget.CursorHand
	h.syncCursorOverride()
	pumpWin32()
	hand := testhook.Win32CursorFor(pop, int(widget.CursorHand))
	if w32CursorHidden(pop) || w32CursorHandle(pop) != hand {
		t.Errorf("после смены на Hand: handle=%#x hidden=%v, ждал %#x", w32CursorHandle(pop), w32CursorHidden(pop), hand)
	}

	// Сняли: стрелка.
	eng.set = false
	h.syncCursorOverride()
	pumpWin32()
	arrow := testhook.Win32CursorFor(pop, int(widget.CursorArrow))
	if w32CursorHidden(pop) || w32CursorHandle(pop) != arrow {
		t.Errorf("после снятия: handle=%#x hidden=%v, ждал стрелку %#x", w32CursorHandle(pop), w32CursorHidden(pop), arrow)
	}
}

// Override уже действует, когда открывается попап: окно получает форму при
// создании, а не после следующего движения мыши.
func TestWin32PopupHost_CursorOverrideBeforeOpen(t *testing.T) {
	carrier := newWin32TestWindow(t)
	eng := &pcEngine{cur: widget.CursorNone, set: true}
	h := newPopupHost(carrier, carrier, eng, 1, nil)
	t.Cleanup(func() { h.closeAll(); pumpWin32() })

	h.syncCursorOverride() // попапов ещё нет — хост лишь запоминает
	pop := pcWin32Popup(t, h, 1)
	if !w32CursorHidden(pop) {
		t.Error("попап, открытый под override None, показывает курсор")
	}
}
