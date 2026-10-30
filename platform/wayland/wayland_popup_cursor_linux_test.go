//go:build linux && !android

package wayland

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/platform/pointer"
)

// wlpcDrain вычитывает всё, что окно успело отправить; пустой результат — не ошибка.
func wlpcDrain(c *wlTestConn) []byte {
	var out []byte
	buf := make([]byte, 8192)
	for {
		c.peer.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, err := c.peer.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			return out
		}
	}
}

// Принудительный курсор над попапом Wayland (Engine.SetCursorOverride): попап —
// другая поверхность, но указатель и курсор у них общие. Пока указатель на
// попапе, форма задаётся SetPopupCursor и уходит set_cursor'ом с serial'ом
// enter на ПОПАП.
func TestWayland_PopupCursorOverride(t *testing.T) {
	c := newWlTestWindow(t)
	if err := c.w.OpenChildPopup(1, 10, 10, 80, 60); err != nil {
		t.Fatalf("OpenChildPopup: %v", err)
	}
	p := c.w.popups.byID[1]
	wlpcDrain(c)

	// Курсор в окне: форма окна — рука.
	c.w.handlePointer(wlPointerEvEnter, append(wlU32(40, c.w.surfaceID), wlFixed(5, 5)...))
	c.w.SetCursor(pointer.CursorHand)
	wlpcDrain(c)

	// Указатель перешёл на попап: форма окна (рука) остаётся.
	c.w.handlePointer(wlPointerEvEnter, append(wlU32(41, p.surfaceID), wlFixed(12, 20)...))
	wlpcDrain(c)

	// Приложение спрятало курсор: над попапом он тоже пропадает.
	c.w.SetPopupCursor(pointer.CursorNone, true)
	m, ok := findWlMsg(splitWlMsgs(t, c.read(t)), c.w.pointerID, wlPointerSetCursor)
	if !ok || len(m.args) != 4 || m.args[0] != 41 || m.args[1] != 0 {
		t.Fatalf("курсор над попапом не спрятан set_cursor(41, NULL): %+v %v", m, ok)
	}

	// Повторное значение компоновщика не беспокоит.
	c.w.SetPopupCursor(pointer.CursorNone, true)
	if msgs := splitWlMsgs(t, wlpcDrain(c)); len(msgs) != 0 {
		t.Errorf("повторный SetPopupCursor отправил %v", msgs)
	}

	// Вернулись в окно: там действует форма окна, а не форма попапа.
	c.w.handlePointer(wlPointerEvEnter, append(wlU32(42, c.w.surfaceID), wlFixed(5, 5)...))
	msgs := splitWlMsgs(t, c.read(t))
	if m, ok := findWlMsg(msgs, c.w.pointerID, wlPointerSetCursor); !ok || m.args[0] != 42 || m.args[1] == 0 {
		t.Errorf("в окне курсор должен быть формой окна (рука), получено %+v %v", m, ok)
	}

	// Override снят, пока указатель на попапе: форма окна снова над попапом.
	c.w.handlePointer(wlPointerEvEnter, append(wlU32(43, p.surfaceID), wlFixed(12, 20)...))
	wlpcDrain(c)
	c.w.SetPopupCursor(0, false)
	if m, ok := findWlMsg(splitWlMsgs(t, c.read(t)), c.w.pointerID, wlPointerSetCursor); !ok || m.args[0] != 43 || m.args[1] == 0 {
		t.Errorf("после снятия override над попапом: set_cursor %+v %v", m, ok)
	}

	// Override выставлен, а указатель в окне (не на попапе): сообщений нет —
	// форму окна задаёт surface.SetCursor.
	c.w.handlePointer(wlPointerEvEnter, append(wlU32(44, c.w.surfaceID), wlFixed(5, 5)...))
	wlpcDrain(c)
	c.w.SetPopupCursor(pointer.CursorNone, true)
	if msgs := splitWlMsgs(t, wlpcDrain(c)); len(msgs) != 0 {
		t.Errorf("SetPopupCursor при указателе в окне отправил %v", msgs)
	}
}
