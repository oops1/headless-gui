//go:build linux && !android

package window

import (
	"encoding/binary"
	"image"
	"net"
	"testing"
	"time"
)

// buttonPressEvent собирает синтетическое 32-байтовое событие ButtonPress (тип 4).
func buttonPressEvent(x, y int16, button byte) []byte {
	b := make([]byte, 32)
	b[0] = 4
	b[1] = button
	binary.LittleEndian.PutUint16(b[24:26], uint16(x))
	binary.LittleEndian.PutUint16(b[26:28], uint16(y))
	return b
}

// motionEvent собирает синтетическое событие MotionNotify (тип 6).
func motionEvent(x, y int16) []byte {
	b := make([]byte, 32)
	b[0] = 6
	binary.LittleEndian.PutUint16(b[24:26], uint16(x))
	binary.LittleEndian.PutUint16(b[26:28], uint16(y))
	return b
}

// keyPressEvent собирает синтетическое событие KeyPress (тип 2).
func keyPressEvent(keycode byte) []byte {
	b := make([]byte, 32)
	b[0] = 2
	b[1] = keycode
	return b
}

// exposeEvent собирает синтетическое событие Expose (тип 12).
func exposeEvent(x, y, w, h uint16) []byte {
	b := make([]byte, 32)
	b[0] = 12
	binary.LittleEndian.PutUint16(b[8:10], x)
	binary.LittleEndian.PutUint16(b[10:12], y)
	binary.LittleEndian.PutUint16(b[12:14], w)
	binary.LittleEndian.PutUint16(b[14:16], h)
	return b
}

// TestX11_DisabledDropsInput проверяет, что при взведённом флаге disabled
// (SetEnabled(false)) обработчик событий ДРОПАЕТ ввод (Key/Button/Motion),
// но продолжает обрабатывать Expose. Это и обеспечивает модальность на X11.
func TestX11_DisabledDropsInput(t *testing.T) {
	w := &X11Window{}
	var buttons, moves, keys, exposes int
	w.onMouseButton = func(x, y, button int, pressed bool) { buttons++ }
	w.onMouseMove = func(x, y int) { moves++ }
	w.onKeyDown = func(vk int) { keys++ }
	w.onExpose = func(r image.Rectangle) { exposes++ }

	// Пока окно включено — весь ввод проходит.
	w.handleX11Event(buttonPressEvent(10, 20, 1))
	w.handleX11Event(motionEvent(11, 21))
	w.handleX11Event(keyPressEvent(38)) // keycode 38 → VK_A
	w.handleX11Event(exposeEvent(0, 0, 5, 5))
	if buttons != 1 || moves != 1 || keys != 1 || exposes != 1 {
		t.Fatalf("до блокировки: buttons=%d moves=%d keys=%d exposes=%d, ожидалось 1/1/1/1",
			buttons, moves, keys, exposes)
	}

	// Блокируем окно (аналог модального owner'а).
	w.SetEnabled(false)
	if !w.disabled.Load() {
		t.Fatal("SetEnabled(false) не взвёл disabled")
	}
	w.handleX11Event(buttonPressEvent(10, 20, 1))
	w.handleX11Event(motionEvent(11, 21))
	w.handleX11Event(keyPressEvent(38))
	w.handleX11Event(exposeEvent(0, 0, 5, 5)) // Expose должен пройти
	if buttons != 1 || moves != 1 || keys != 1 {
		t.Errorf("во время блокировки ввод не задропан: buttons=%d moves=%d keys=%d",
			buttons, moves, keys)
	}
	if exposes != 2 {
		t.Errorf("Expose не обработан при блокировке: exposes=%d, ожидалось 2", exposes)
	}

	// Разблокируем — ввод снова проходит.
	w.SetEnabled(true)
	if w.disabled.Load() {
		t.Fatal("SetEnabled(true) не снял disabled")
	}
	w.handleX11Event(buttonPressEvent(10, 20, 1))
	if buttons != 2 {
		t.Errorf("после разблокировки ввод не восстановлен: buttons=%d, ожидалось 2", buttons)
	}
}

// TestX11_EventPumpProcessesAndTerminates проверяет, что насос вторичного окна
// читает события своего соединения и корректно завершается при закрытии conn
// (без паники на закрытом соединении).
func TestX11_EventPumpProcessesAndTerminates(t *testing.T) {
	client, server := net.Pipe()
	w := &X11Window{conn: client}

	got := make(chan struct{}, 1)
	w.onExpose = func(r image.Rectangle) {
		select {
		case got <- struct{}{}:
		default:
		}
	}

	done := make(chan struct{})
	go func() {
		w.eventPumpLoop()
		close(done)
	}()

	// Досылаем одно событие Expose — насос обязан его обработать.
	if _, err := server.Write(exposeEvent(0, 0, 4, 4)); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("насос не обработал событие Expose")
	}

	// Закрываем соединение — readFull вернёт false, насос обязан выйти.
	server.Close()
	client.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("насос не завершился после закрытия соединения")
	}
}
