//go:build linux && !android

package core

import (
	"encoding/binary"
	"net"
	"sync"
	"testing"
	"time"
)

// x11KeyEv собирает 32-байтовое KeyPress (2) или KeyRelease (3).
func x11KeyEv(typ, keycode byte, tm uint32) []byte {
	b := make([]byte, 32)
	b[0], b[1] = typ, keycode
	binary.LittleEndian.PutUint32(b[4:8], tm)
	return b
}

const x11KeyA = 38 // keycode клавиши A (VK_A)

func TestIsX11RepeatPair(t *testing.T) {
	cases := []struct {
		name         string
		release, prs []byte
		want         bool
	}{
		{"пара автоповтора", x11KeyEv(3, x11KeyA, 500), x11KeyEv(2, x11KeyA, 500), true},
		{"другое время", x11KeyEv(3, x11KeyA, 500), x11KeyEv(2, x11KeyA, 501), false},
		{"другая клавиша", x11KeyEv(3, x11KeyA, 500), x11KeyEv(2, x11KeyA+1, 500), false},
		{"второе не нажатие", x11KeyEv(3, x11KeyA, 500), x11KeyEv(3, x11KeyA, 500), false},
		{"первое не отпускание", x11KeyEv(2, x11KeyA, 500), x11KeyEv(2, x11KeyA, 500), false},
		{"synthetic (send_event)", x11KeyEv(3|0x80, x11KeyA, 500), x11KeyEv(2|0x80, x11KeyA, 500), true},
	}
	for _, c := range cases {
		if got := isX11RepeatPair(c.release, c.prs); got != c.want {
			t.Errorf("%s: %v, ожидалось %v", c.name, got, c.want)
		}
	}
}

func TestX11KeyState_Note(t *testing.T) {
	var k x11KeyState
	if k.note(2, 10) {
		t.Error("первое нажатие помечено повтором")
	}
	if !k.note(2, 10) {
		t.Error("нажатие уже нажатой клавиши не помечено повтором (сервер с DetectableAutoRepeat)")
	}
	k.note(3, 10)
	if k.note(2, 10) {
		t.Error("нажатие после отпускания помечено повтором")
	}
	k.note(2, 11)
	k.note(10, 0) // FocusOut
	if k.note(2, 10) || k.note(2, 11) {
		t.Error("после потери фокуса клавиши остались нажатыми")
	}
}

// keyRig — окно X11 на паре net.Pipe: «сервер» пишет события, окно читает их
// циклом так же, как RunEventLoop, и записывает, что дошло до приложения.
type keyRig struct {
	srv net.Conn
	w   *X11Window

	mu  sync.Mutex
	log []string
}

func newKeyRig(t *testing.T) *keyRig {
	t.Helper()
	srv, cli := net.Pipe()
	r := &keyRig{srv: srv, w: &X11Window{conn: cli}}
	r.w.SetOnKeyDownRepeat(func(vk int, repeat bool) {
		r.add(map[bool]string{false: "down", true: "repeat"}[repeat])
	})
	r.w.SetOnKeyUp(func(vk int) { r.add("up") })
	go func() {
		buf := make([]byte, 32)
		for r.w.readEvent(buf) {
			r.w.handleX11Event(buf)
		}
	}()
	t.Cleanup(func() { srv.Close(); cli.Close() })
	return r
}

func (r *keyRig) add(s string) {
	r.mu.Lock()
	r.log = append(r.log, s)
	r.mu.Unlock()
}

func (r *keyRig) send(t *testing.T, b []byte) {
	t.Helper()
	if _, err := r.srv.Write(b); err != nil {
		t.Fatal(err)
	}
}

// wait ждёт, пока приложение не получит n событий, и возвращает журнал.
func (r *keyRig) wait(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		if len(r.log) >= n {
			out := append([]string(nil), r.log...)
			r.mu.Unlock()
			return out
		}
		r.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	t.Fatalf("за 2 с пришло %d событий из %d: %v", len(r.log), n, r.log)
	return nil
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Удержание клавиши: нажатие, затем пары «отпускание+нажатие» с общим временем
// — приложение видит одно нажатие, повторы и ОДНО отпускание в конце.
func TestX11KeyRepeat_PairsBecomeRepeats(t *testing.T) {
	r := newKeyRig(t)
	r.send(t, x11KeyEv(2, x11KeyA, 100))
	for i, tm := range []uint32{600, 633, 666} {
		_ = i
		// Обе половины пары — одной записью, как их шлёт сервер.
		pair := append(x11KeyEv(3, x11KeyA, tm), x11KeyEv(2, x11KeyA, tm)...)
		r.send(t, pair)
	}
	r.send(t, x11KeyEv(3, x11KeyA, 700)) // настоящее отпускание

	got := r.wait(t, 5)
	want := []string{"down", "repeat", "repeat", "repeat", "up"}
	if !same(got, want) {
		t.Fatalf("получено %v, ожидалось %v", got, want)
	}
}

// Настоящее отпускание и нажатие с разным временем — не повтор.
func TestX11KeyRepeat_RealReleasePressIsNotRepeat(t *testing.T) {
	r := newKeyRig(t)
	r.send(t, x11KeyEv(2, x11KeyA, 100))
	r.send(t, append(x11KeyEv(3, x11KeyA, 200), x11KeyEv(2, x11KeyA, 350)...))
	got := r.wait(t, 3)
	want := []string{"down", "up", "down"}
	if !same(got, want) {
		t.Fatalf("получено %v, ожидалось %v", got, want)
	}
}

// Одиночное отпускание доходит до приложения (без парного нажатия за время
// заглядывания), и порядок событий не теряется, если за ним следует другое.
func TestX11KeyRepeat_ReleaseDeliveredAndOrderKept(t *testing.T) {
	r := newKeyRig(t)
	r.send(t, x11KeyEv(2, x11KeyA, 100))
	r.send(t, x11KeyEv(3, x11KeyA, 200))
	time.Sleep(20 * time.Millisecond) // дольше окна заглядывания
	r.send(t, x11KeyEv(2, x11KeyA+1, 300))
	r.send(t, append(x11KeyEv(3, x11KeyA+1, 400), x11KeyEv(3, x11KeyA, 400)...)) // за отпусканием — другое событие
	got := r.wait(t, 4)
	want := []string{"down", "up", "down", "up"}
	// Четвёртое — отпускание клавиши A+1 (38+1 = 39 — тоже клавиша из таблицы);
	// пятое (A, но она не нажата) тоже придёт отпусканием — число не проверяем.
	if !same(got[:4], want) {
		t.Fatalf("получено %v, ожидалось начало %v", got, want)
	}
}

// Событие, пришедшее за отпусканием половинками, не теряется и не
// склеивается неверно: заглядывание дочитывает начатое событие до конца.
func TestX11KeyRepeat_SplitEvent(t *testing.T) {
	r := newKeyRig(t)
	r.send(t, x11KeyEv(2, x11KeyA, 100))
	rel, prs := x11KeyEv(3, x11KeyA, 900), x11KeyEv(2, x11KeyA, 900)
	r.send(t, append(rel, prs[:10]...))
	time.Sleep(15 * time.Millisecond) // остаток приходит уже после окна заглядывания
	r.send(t, prs[10:])
	got := r.wait(t, 2)
	want := []string{"down", "repeat"}
	if !same(got, want) {
		t.Fatalf("получено %v, ожидалось %v", got, want)
	}
}

// Сервер с DetectableAutoRepeat (отпусканий между нажатиями нет): повтор
// распознаётся по таблице нажатых клавиш.
func TestX11KeyRepeat_DetectableAutoRepeat(t *testing.T) {
	r := newKeyRig(t)
	r.send(t, x11KeyEv(2, x11KeyA, 100))
	r.send(t, x11KeyEv(2, x11KeyA, 600))
	r.send(t, x11KeyEv(2, x11KeyA, 633))
	r.send(t, x11KeyEv(3, x11KeyA, 700))
	r.send(t, x11KeyEv(2, x11KeyA, 900))
	got := r.wait(t, 5)
	want := []string{"down", "repeat", "repeat", "up", "down"}
	if !same(got, want) {
		t.Fatalf("получено %v, ожидалось %v", got, want)
	}
}

// Окно, заблокированное модалкой, ввод не получает, но состояние клавиш
// ведёт: отпускание «под модалкой» не оставляет клавишу нажатой.
func TestX11KeyRepeat_DisabledWindowKeepsState(t *testing.T) {
	r := newKeyRig(t)
	r.send(t, x11KeyEv(2, x11KeyA, 100))
	r.wait(t, 1)
	r.w.disabled.Store(true)
	r.send(t, x11KeyEv(3, x11KeyA, 200)) // отпускание проглатывается
	time.Sleep(20 * time.Millisecond)
	r.w.disabled.Store(false)
	r.send(t, x11KeyEv(2, x11KeyA, 300))
	got := r.wait(t, 2)
	want := []string{"down", "down"}
	if !same(got, want) {
		t.Fatalf("получено %v, ожидалось %v (повтор опознан ошибочно)", got, want)
	}
}
