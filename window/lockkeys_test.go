package window

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

// Клавиши-переключатели (v3.44): набор флагов, выбор клавиш для нажатия и
// доставка состояния вместе с активацией окна. Что не зависит от ОС, проверяется
// здесь на заглушках бэкенда; разбор Win32, X11 и Wayland — в платформенных
// файлах lockkeys_*_test.go.

func TestLockKeys_SetOps(t *testing.T) {
	k := CapsLock | ScrollLock
	if !k.Has(CapsLock) || !k.Has(ScrollLock) || !k.Has(CapsLock|ScrollLock) {
		t.Errorf("Has не видит своих флагов: %v", k)
	}
	if k.Has(NumLock) || k.Has(CapsLock|NumLock) {
		t.Errorf("Has видит чужой флаг: %v", k)
	}
	if k.Has(0) {
		t.Error("Has(0) должно быть false: пустой набор ничего не доказывает")
	}
	if k.Empty() || !LockKeys(0).Empty() {
		t.Error("Empty ошибается")
	}
	// Лишние биты вне известных флагов набор не заполняют.
	if !LockKeys(0x80).Empty() {
		t.Error("неизвестный бит сделал набор непустым")
	}
}

func TestLockKeys_String(t *testing.T) {
	for _, tc := range []struct {
		k    LockKeys
		want string
	}{
		{0, "-"},
		{CapsLock, "caps"},
		{NumLock | CapsLock, "caps|num"},
		{ScrollLock, "scroll"},
		{CapsLock | NumLock | ScrollLock | KanaLock, "caps|num|scroll|kana"},
		{KanaLock, "kana"},
	} {
		if got := tc.k.String(); got != tc.want {
			t.Errorf("String(%d) = %q, ждал %q", tc.k, got, tc.want)
		}
	}
}

// Флаги независимы и умещаются в байт: приложение вправе хранить набор как
// число и сравнивать с ==.
func TestLockKeys_FlagValuesDistinct(t *testing.T) {
	seen := LockKeys(0)
	for _, f := range []LockKeys{CapsLock, NumLock, ScrollLock, KanaLock} {
		if f == 0 || seen&f != 0 || f&(f-1) != 0 {
			t.Fatalf("флаг %d пересекается с другими или не одиночный бит", f)
		}
		seen |= f
	}
	if seen != lkAll {
		t.Errorf("lkAll = %d, а флаги вместе дают %d", lkAll, seen)
	}
}

func TestLockKeys_FromToggles(t *testing.T) {
	on := map[int]bool{VK_CAPITAL: true, VK_SCROLL: true}
	got := lkFromToggles(func(vk int) bool { return on[vk] })
	if got != CapsLock|ScrollLock {
		t.Errorf("lkFromToggles = %v, ждал caps|scroll", got)
	}
	if lkFromToggles(func(int) bool { return false }) != 0 {
		t.Error("ничего не включено, а набор не пуст")
	}
	if lkFromToggles(func(vk int) bool { return vk == VK_KANA }) != KanaLock {
		t.Error("VK_KANA не стал KanaLock")
	}
	if lkFromToggles(func(vk int) bool { return vk == VK_NUMLOCK }) != NumLock {
		t.Error("VK_NUMLOCK не стал NumLock")
	}
}

// Нажимаются только клавиши отличающихся флагов, в порядке Caps, Num, Scroll,
// Kana; совпадающие не трогаются.
func TestLockKeys_ToggleKeys(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cur, want LockKeys
		vks       []int
	}{
		{"одинаково", CapsLock | NumLock, CapsLock | NumLock, nil},
		{"включить Num", 0, NumLock, []int{VK_NUMLOCK}},
		{"Num не трогает Caps", CapsLock, CapsLock | NumLock, []int{VK_NUMLOCK}},
		{"выключить Caps", CapsLock | NumLock, NumLock, []int{VK_CAPITAL}},
		{"заменить Caps на Scroll", CapsLock, ScrollLock, []int{VK_CAPITAL, VK_SCROLL}},
		{"все", 0, CapsLock | NumLock | ScrollLock | KanaLock, []int{VK_CAPITAL, VK_NUMLOCK, VK_SCROLL, VK_KANA}},
		{"все выключить", CapsLock | NumLock | ScrollLock | KanaLock, 0, []int{VK_CAPITAL, VK_NUMLOCK, VK_SCROLL, VK_KANA}},
	} {
		if got := lkToggleKeys(tc.cur, tc.want); !reflect.DeepEqual(got, tc.vks) {
			t.Errorf("%s: нажать %v, ждал %v", tc.name, got, tc.vks)
		}
	}
}

func TestLockKeys_IsLockVK(t *testing.T) {
	for _, vk := range []int{VK_CAPITAL, VK_NUMLOCK, VK_SCROLL, VK_KANA} {
		if !lkIsLockVK(vk) {
			t.Errorf("vk %#x не признан переключателем", vk)
		}
	}
	for _, vk := range []int{VK_A, VK_SHIFT, VK_CONTROL, VK_PAUSE, 0} {
		if lkIsLockVK(vk) {
			t.Errorf("vk %#x признан переключателем", vk)
		}
	}
}

// lkNative — окно ОС с состоянием переключателей и (необязательно) ожиданием
// состояния после фокуса, как на Wayland.
type lkNative struct {
	rkNative
	mu        sync.Mutex
	locks     LockKeys
	known     bool
	asked     []LockKeys
	setResult bool
	settled   bool
	onSettled func()
}

func (n *lkNative) LockKeys() (LockKeys, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.locks, n.known
}

func (n *lkNative) SetLockKeys(want LockKeys) bool {
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
func (n *lkSettleNative) settle(locks LockKeys) {
	n.mu.Lock()
	n.locks, n.known, n.settled = locks, true, true
	fn := n.onSettled
	n.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func lkTestWindow(native NativeWindow, eng *rkEngine) *Window {
	win := &Window{}
	win.surface = surface{eng: eng, native: native, scale: 1}
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
	if win.SetLockKeys(NumLock) {
		t.Error("SetLockKeys без поддержки ответил true")
	}
	before := &Window{}
	if k, ok := before.LockKeys(); ok || k != 0 {
		t.Errorf("до Run LockKeys = %v, %v", k, ok)
	}
	if before.SetLockKeys(CapsLock) {
		t.Error("до Run SetLockKeys ответил true")
	}
}

func TestLockKeys_DelegatesToBackend(t *testing.T) {
	n := &lkNative{locks: CapsLock | NumLock, known: true, setResult: true}
	win := lkTestWindow(n, &rkEngine{})
	if k, ok := win.LockKeys(); !ok || k != CapsLock|NumLock {
		t.Errorf("LockKeys = %v, %v", k, ok)
	}
	if !win.SetLockKeys(NumLock) {
		t.Error("ответ бэкенда true потерян")
	}
	// Лишние биты до бэкенда не доходят.
	if !win.SetLockKeys(NumLock | LockKeys(0x80)) {
		t.Error("второй вызов")
	}
	if want := []LockKeys{NumLock, NumLock}; !reflect.DeepEqual(n.asked, want) {
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
	locks  LockKeys
	known  bool
}

func lkCollect(win *Window) *[]lkActCall {
	var got []lkActCall
	var mu sync.Mutex
	win.SetOnActivateLocks(func(active bool, locks LockKeys, known bool) {
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
	n := &lkNative{locks: CapsLock, known: true}
	win := lkTestWindow(n, eng)
	got := lkCollect(win)

	n.onActivate(true)
	n.onActivate(true) // Wayland шлёт состояние с каждым configure
	if len(*got) != 0 {
		t.Fatalf("колбэк вызван до очереди движка: %v", *got)
	}
	eng.run()
	if want := []lkActCall{{true, CapsLock, true}}; !reflect.DeepEqual(*got, want) {
		t.Fatalf("активация: %v, ждал %v", *got, want)
	}

	// Состояние изменилось, пока окно было в фоне: возврат фокуса несёт новое.
	n.onActivate(false)
	n.mu.Lock()
	n.locks = NumLock | CapsLock
	n.mu.Unlock()
	n.onActivate(true)
	eng.run()
	if len(*got) != 3 || (*got)[1].active || (*got)[2] != (lkActCall{true, CapsLock | NumLock, true}) {
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
	n := &lkNative{locks: NumLock, known: true}
	win := lkTestWindow(n, eng)
	var seq []string
	win.SetOnActivate(func(a bool) { seq = append(seq, fmt.Sprint("plain ", a)) })
	win.SetOnActivateLocks(func(a bool, l LockKeys, k bool) { seq = append(seq, fmt.Sprint("locks ", a, " ", l, " ", k)) })
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
	n.settle(CapsLock | NumLock)
	eng.run()
	if want := []lkActCall{{true, CapsLock | NumLock, true}}; !reflect.DeepEqual(*got, want) {
		t.Fatalf("после прихода состояния: %v, ждал %v", *got, want)
	}
	n.settle(CapsLock) // modifiers при каждом изменении: нового колбэка нет
	eng.run()
	if len(*got) != 1 {
		t.Errorf("изменение состояния при фокусе вызвало колбэк: %v", *got)
	}
}

// Состояние уже пришло до configure (enter и modifiers раньше): без ожидания.
func TestLockKeys_ActivateLocksStateAlreadyThere(t *testing.T) {
	eng := &rkEngine{}
	n := &lkSettleNative{}
	n.settled, n.locks, n.known = true, ScrollLock, true
	win := lkTestWindow(n, eng)
	got := lkCollect(win)
	n.onActivate(true)
	eng.run()
	if want := []lkActCall{{true, ScrollLock, true}}; !reflect.DeepEqual(*got, want) {
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
	n.settle(CapsLock)
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
	n.settle(CapsLock)
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
	n.settle(NumLock)
	eng.run()
	last := (*got)[len(*got)-1]
	if last != (lkActCall{true, NumLock, true}) {
		t.Errorf("повторная активация: %v", *got)
	}
}
