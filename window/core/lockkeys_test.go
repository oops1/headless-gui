package core

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v4/platform/keyboard"
	"github.com/oops1/headless-gui/v4/platform/native"
)

// Клавиши-переключатели (v3.44): набор флагов, выбор клавиш для нажатия и
// доставка состояния вместе с активацией окна. Что не зависит от ОС, проверяется
// здесь на заглушках бэкенда; разбор Win32, X11 и Wayland — в платформенных
// файлах lockkeys_*_test.go.

// lkNative — окно ОС с состоянием переключателей и (необязательно) ожиданием
// состояния после фокуса, как на Wayland.
type lkNative struct {
	rkNative
	mu        sync.Mutex
	locks     keyboard.LockKeys
	known     bool
	asked     []keyboard.LockKeys
	setResult bool
	settled   bool
	onSettled func()
}

func (n *lkNative) LockKeys() (keyboard.LockKeys, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.locks, n.known
}

func (n *lkNative) SetLockKeys(want keyboard.LockKeys) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.asked = append(n.asked, want)
	return n.setResult
}

// lkSettleNative добавляет к lkNative способность «состояние приходит позже
// фокуса» (lockKeysSettler).
type lkSettleNative struct{ lkNative }

func (n *lkSettleNative) LockKeysSettled() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.settled
}

func (n *lkSettleNative) SetOnLockKeysSettled(fn func()) {
	n.mu.Lock()
	n.onSettled = fn
	n.mu.Unlock()
}

// settle — состояние пришло: как на Wayland при первом modifiers после enter.
func (n *lkSettleNative) settle(locks keyboard.LockKeys) {
	n.mu.Lock()
	n.locks, n.known, n.settled = locks, true, true
	fn := n.onSettled
	n.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func lkTestWindow(nw native.NativeWindow, eng *rkEngine) *Window {
	win := &Window{}
	win.surface = surface{eng: eng, native: nw, scale: 1}
	win.setupInput()
	win.setupActivation()
	return win
}

// Бэкенд без способности: «не знаю» и «не умею», без паники и до Run.
func TestLockKeys_UnsupportedBackend(t *testing.T) {
	win := lkTestWindow(&rkNative{}, &rkEngine{})
	if k, ok := win.LockKeys(); ok || k != 0 {
		t.Errorf("LockKeys без поддержки = %v, %v", k, ok)
	}
	if win.SetLockKeys(keyboard.NumLock) {
		t.Error("SetLockKeys без поддержки ответил true")
	}
	before := &Window{}
	if k, ok := before.LockKeys(); ok || k != 0 {
		t.Errorf("до Run LockKeys = %v, %v", k, ok)
	}
	if before.SetLockKeys(keyboard.CapsLock) {
		t.Error("до Run SetLockKeys ответил true")
	}
}

func TestLockKeys_DelegatesToBackend(t *testing.T) {
	n := &lkNative{locks: keyboard.CapsLock | keyboard.NumLock, known: true, setResult: true}
	win := lkTestWindow(n, &rkEngine{})
	if k, ok := win.LockKeys(); !ok || k != keyboard.CapsLock|keyboard.NumLock {
		t.Errorf("LockKeys = %v, %v", k, ok)
	}
	if !win.SetLockKeys(keyboard.NumLock) {
		t.Error("ответ бэкенда true потерян")
	}
	// Лишние биты до бэкенда не доходят.
	if !win.SetLockKeys(keyboard.NumLock | keyboard.LockKeys(0x80)) {
		t.Error("второй вызов")
	}
	if want := []keyboard.LockKeys{keyboard.NumLock, keyboard.NumLock}; !reflect.DeepEqual(n.asked, want) {
		t.Errorf("бэкенд получил %v, ждал %v", n.asked, want)
	}
	n.setResult = false
	if win.SetLockKeys(0) {
		t.Error("отказ бэкенда не дошёл до приложения")
	}
	n.known = false
	if k, ok := win.LockKeys(); ok || k != 0 {
		t.Errorf("«не знаю» бэкенда не дошло: %v, %v", k, ok)
	}
}

type lkActCall struct {
	active bool
	locks  keyboard.LockKeys
	known  bool
}

func lkCollect(win *Window) *[]lkActCall {
	var got []lkActCall
	var mu sync.Mutex
	win.SetOnActivateLocks(func(active bool, locks keyboard.LockKeys, known bool) {
		mu.Lock()
		got = append(got, lkActCall{active, locks, known})
		mu.Unlock()
	})
	return &got
}

// Бэкенд, знающий состояние сразу (Win32, X11): колбэк получает его вместе с
// активацией, только при смене, на горутине движка.
func TestLockKeys_ActivateLocksImmediate(t *testing.T) {
	eng := &rkEngine{}
	n := &lkNative{locks: keyboard.CapsLock, known: true}
	win := lkTestWindow(n, eng)
	got := lkCollect(win)

	n.onActivate(true)
	n.onActivate(true) // Wayland шлёт состояние с каждым configure
	if len(*got) != 0 {
		t.Fatalf("колбэк вызван до очереди движка: %v", *got)
	}
	eng.run()
	if want := []lkActCall{{true, keyboard.CapsLock, true}}; !reflect.DeepEqual(*got, want) {
		t.Fatalf("активация: %v, ждал %v", *got, want)
	}

	// Состояние изменилось, пока окно было в фоне: возврат фокуса несёт новое.
	n.onActivate(false)
	n.mu.Lock()
	n.locks = keyboard.NumLock | keyboard.CapsLock
	n.mu.Unlock()
	n.onActivate(true)
	eng.run()
	if len(*got) != 3 || (*got)[1].active || (*got)[2] != (lkActCall{true, keyboard.CapsLock | keyboard.NumLock, true}) {
		t.Errorf("возврат фокуса: %v", *got)
	}

	// nil отключает; прежний SetOnActivate живёт сам по себе.
	win.SetOnActivateLocks(nil)
	plain := 0
	win.SetOnActivate(func(bool) { plain++ })
	n.onActivate(false)
	eng.run()
	if len(*got) != 3 {
		t.Errorf("отключённый колбэк вызван: %v", *got)
	}
	if plain != 1 {
		t.Errorf("SetOnActivate вызван %d раз, ждал 1", plain)
	}
}

// Оба колбэка работают вместе: оба получают активацию.
func TestLockKeys_ActivateLocksBesideOnActivate(t *testing.T) {
	eng := &rkEngine{}
	n := &lkNative{locks: keyboard.NumLock, known: true}
	win := lkTestWindow(n, eng)
	var seq []string
	win.SetOnActivate(func(a bool) { seq = append(seq, fmt.Sprint("plain ", a)) })
	win.SetOnActivateLocks(func(a bool, l keyboard.LockKeys, k bool) { seq = append(seq, fmt.Sprint("locks ", a, " ", l, " ", k)) })
	n.onActivate(true)
	eng.run()
	if want := []string{"plain true", "locks true num true"}; !reflect.DeepEqual(seq, want) {
		t.Errorf("порядок/состав: %q, ждал %q", seq, want)
	}
}

// Wayland: активация пришла раньше modifiers — колбэк ждёт состояние и получает
// уже верное; повторные configure ждут того же события и не множат вызовы.
func TestLockKeys_ActivateLocksWaitsForState(t *testing.T) {
	eng := &rkEngine{}
	n := &lkSettleNative{}
	win := lkTestWindow(n, eng)
	got := lkCollect(win)

	n.onActivate(true)
	n.onActivate(true)
	eng.run()
	if len(*got) != 0 {
		t.Fatalf("колбэк вызван до прихода состояния: %v", *got)
	}
	n.settle(keyboard.CapsLock | keyboard.NumLock)
	eng.run()
	if want := []lkActCall{{true, keyboard.CapsLock | keyboard.NumLock, true}}; !reflect.DeepEqual(*got, want) {
		t.Fatalf("после прихода состояния: %v, ждал %v", *got, want)
	}
	n.settle(keyboard.CapsLock) // modifiers при каждом изменении: нового колбэка нет
	eng.run()
	if len(*got) != 1 {
		t.Errorf("изменение состояния при фокусе вызвало колбэк: %v", *got)
	}
}

// Состояние уже пришло до configure (enter и modifiers раньше): без ожидания.
func TestLockKeys_ActivateLocksStateAlreadyThere(t *testing.T) {
	eng := &rkEngine{}
	n := &lkSettleNative{}
	n.settled, n.locks, n.known = true, keyboard.ScrollLock, true
	win := lkTestWindow(n, eng)
	got := lkCollect(win)
	n.onActivate(true)
	eng.run()
	if want := []lkActCall{{true, keyboard.ScrollLock, true}}; !reflect.DeepEqual(*got, want) {
		t.Errorf("%v, ждал %v", *got, want)
	}
}

// Композитор состояния так и не прислал: колбэк всё равно приходит, с known=false.
func TestLockKeys_ActivateLocksTimeout(t *testing.T) {
	old := lkSettleTimeout
	lkSettleTimeout = 20 * time.Millisecond
	t.Cleanup(func() { lkSettleTimeout = old })

	eng := &rkEngine{}
	n := &lkSettleNative{}
	win := lkTestWindow(n, eng)
	got := lkCollect(win)
	n.onActivate(true)
	deadline := time.Now().Add(3 * time.Second)
	for {
		eng.run()
		if len(*got) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if want := []lkActCall{{true, 0, false}}; !reflect.DeepEqual(*got, want) {
		t.Fatalf("по сроку: %v, ждал %v", *got, want)
	}
	// Состояние, пришедшее после срока, колбэк не повторяет.
	n.settle(keyboard.CapsLock)
	eng.run()
	if len(*got) != 1 {
		t.Errorf("повтор после срока: %v", *got)
	}
}

// Фокус потерян, пока ждали состояние: «активно» уже не сообщается.
func TestLockKeys_ActivateLocksCanceledByDeactivation(t *testing.T) {
	eng := &rkEngine{}
	n := &lkSettleNative{}
	win := lkTestWindow(n, eng)
	got := lkCollect(win)
	n.onActivate(true)
	n.onActivate(false)
	n.settle(keyboard.CapsLock)
	eng.run()
	for _, c := range *got {
		if c.active {
			t.Errorf("отменённая активация сообщена: %v", *got)
		}
	}
	// Следующая активация ждёт заново и получает своё состояние.
	n.mu.Lock()
	n.settled = false
	n.mu.Unlock()
	n.onActivate(true)
	n.settle(keyboard.NumLock)
	eng.run()
	last := (*got)[len(*got)-1]
	if last != (lkActCall{true, keyboard.NumLock, true}) {
		t.Errorf("повторная активация: %v", *got)
	}
}
