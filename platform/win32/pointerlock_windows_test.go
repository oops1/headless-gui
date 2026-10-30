//go:build windows

package win32

import (
	"os"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/oops1/headless-gui/v4/platform/pointer"
)

// Win32: захват указателя на живом окне — регистрация Raw Input, зажим курсора
// в центре клиентской области, скрытие, снятие при потере фокуса, разбор
// RAWINPUT. Зажим живого курсора снимается в Cleanup, даже если тест упал.

// plwRegistered возвращает окно, на которое сейчас зарегистрирована мышь в
// Raw Input (usage page 1, usage 2) для этого процесса; 0 — нет.
func plwRegistered(t *testing.T) uintptr {
	t.Helper()
	var n uint32
	procGetRegisteredRawInputDevices.Call(0, uintptr(unsafe.Pointer(&n)), unsafe.Sizeof(rawInputDevice{}))
	if n == 0 {
		return 0
	}
	devs := make([]rawInputDevice, n)
	r, _, _ := procGetRegisteredRawInputDevices.Call(uintptr(unsafe.Pointer(&devs[0])),
		uintptr(unsafe.Pointer(&n)), unsafe.Sizeof(rawInputDevice{}))
	if int32(r) <= 0 {
		return 0
	}
	for _, d := range devs[:n] {
		if d.UsagePage == hidUsagePageGen && d.Usage == hidUsageMouse {
			return d.HwndTarget
		}
	}
	return 0
}

func plwClip() rect {
	var r rect
	procGetClipCursor.Call(uintptr(unsafe.Pointer(&r)))
	return r
}

func plwWindow(t *testing.T) *Win32Window {
	t.Helper()
	w := newFsWindow(t)
	w.lock.foreground = func() bool { return true }
	t.Cleanup(func() {
		procClipCursor.Call(0)
		w.lock.release(w)
	})
	return w
}

// Включение регистрирует мышь в Raw Input на это окно и зажимает указатель в
// центре клиентской области; выключение возвращает всё.
func TestWin32PointerLock_EngageAndRelease(t *testing.T) {
	w := plwWindow(t)
	before := plwClip()
	var seen []bool
	w.SetOnPointerLockChanged(func(a bool) { seen = append(seen, a) })

	if !w.SetPointerLock(true) {
		t.Fatal("SetPointerLock(true) отказал")
	}
	if got := plwRegistered(t); got != uintptr(w.hwnd) {
		t.Errorf("Raw Input зарегистрирован на %#x, ждал окно %#x", got, w.hwnd)
	}
	c, ok := w.clientCenter()
	if !ok {
		t.Fatal("нет центра окна")
	}
	if clip := plwClip(); clip != (rect{Left: c.X, Top: c.Y, Right: c.X + 1, Bottom: c.Y + 1}) {
		t.Errorf("зажим %+v, ждал точку центра %+v", clip, c)
	}
	if h, handled := w.setCursorMessage(htClient); !handled || h != 0 {
		t.Errorf("WM_SETCURSOR при захвате: handled=%v h=%#x, ждал спрятанный курсор", handled, h)
	}

	if !w.SetPointerLock(false) {
		t.Fatal("SetPointerLock(false) отказал")
	}
	if got := plwRegistered(t); got != 0 {
		t.Errorf("Raw Input остался зарегистрирован на %#x", got)
	}
	if clip := plwClip(); clip != before {
		t.Errorf("зажим не снят: %+v, был %+v", clip, before)
	}
	if _, handled := w.setCursorMessage(htClient); handled {
		t.Error("курсор остался спрятанным после снятия захвата")
	}
	if len(seen) != 2 || !seen[0] || seen[1] {
		t.Errorf("уведомления %v, ждал [true false]", seen)
	}
}

// Потеря активности снимает зажим и возвращает курсор, но желание приложения
// помнится: получив активность, окно зажимает указатель снова.
func TestWin32PointerLock_FocusLossAndRestore(t *testing.T) {
	w := plwWindow(t)
	before := plwClip()
	var seen []bool
	w.SetOnPointerLockChanged(func(a bool) { seen = append(seen, a) })
	w.SetPointerLock(true)

	w.lock.onActivate(w, false)
	if clip := plwClip(); clip != before {
		t.Errorf("зажим не снят при потере фокуса: %+v", clip)
	}
	if _, handled := w.setCursorMessage(htClient); handled {
		t.Error("курсор спрятан у окна без фокуса")
	}
	if !w.lock.want {
		t.Error("потеря фокуса забыла просьбу приложения")
	}

	w.lock.onActivate(w, true)
	c, _ := w.clientCenter()
	if clip := plwClip(); clip != (rect{Left: c.X, Top: c.Y, Right: c.X + 1, Bottom: c.Y + 1}) {
		t.Errorf("зажим не восстановлен: %+v", clip)
	}
	if len(seen) != 3 || !seen[0] || seen[1] || !seen[2] {
		t.Errorf("уведомления %v, ждал [true false true]", seen)
	}

	// Без просьбы приложения активность ничего не зажимает.
	w.SetPointerLock(false)
	w.lock.onActivate(w, false)
	w.lock.onActivate(w, true)
	if clip := plwClip(); clip != before {
		t.Errorf("активность без захвата зажала указатель: %+v", clip)
	}
}

// Окно не на переднем плане: регистрация идёт, зажим откладывается до активации.
func TestWin32PointerLock_DeferredUntilForeground(t *testing.T) {
	w := plwWindow(t)
	fg := false
	w.lock.foreground = func() bool { return fg }
	before := plwClip()
	if !w.SetPointerLock(true) {
		t.Fatal("отказ")
	}
	if clip := plwClip(); clip != before || w.lock.engaged {
		t.Fatalf("окно без фокуса зажало указатель: %+v", clip)
	}
	fg = true
	w.lock.onActivate(w, true)
	if !w.lock.engaged {
		t.Error("после получения фокуса указатель не зажат")
	}
}

// Сдвиг из RAWINPUT доходит до приложения только при действующем захвате;
// абсолютное положение (планшет, удалённый сеанс) превращается в сдвиг.
func TestWin32PointerLock_HandleRaw(t *testing.T) {
	w := plwWindow(t)
	var got [][2]float64
	w.SetOnRelativeMotion(func(dx, dy float64) { got = append(got, [2]float64{dx, dy}) })
	ptr := int(unsafe.Sizeof(uintptr(0)))

	w.lock.handleRaw(plRawMouse(ptr, pointer.RIMTypeMouse, 0, 5, 6)) // захвата нет
	if len(got) != 0 {
		t.Fatalf("сдвиг доставлен без захвата: %v", got)
	}
	w.SetPointerLock(true)
	w.lock.handleRaw(plRawMouse(ptr, pointer.RIMTypeMouse, 0, 5, -6))
	w.lock.handleRaw(plRawMouse(ptr, pointer.RIMTypeMouse, 0, 0, 0)) // нажатие без движения
	if len(got) != 1 || got[0] != [2]float64{5, -6} {
		t.Fatalf("доставлено %v, ждал [[5 -6]]", got)
	}
	// Абсолютное: первое положение сдвига не даёт, второе — разность.
	w.lock.handleRaw(plRawMouse(ptr, pointer.RIMTypeMouse, pointer.MouseMoveAbsolute, 1000, 1000))
	w.lock.handleRaw(plRawMouse(ptr, pointer.RIMTypeMouse, pointer.MouseMoveAbsolute, 1000+6553, 1000))
	if len(got) != 2 || got[1][0] <= 0 || got[1][1] != 0 {
		t.Errorf("абсолютное положение дало %v", got)
	}
}

// Окно уничтожается — зажим и регистрация не переживают его.
func TestWin32PointerLock_ReleasedOnDestroy(t *testing.T) {
	w := plwWindow(t)
	before := plwClip()
	w.SetPointerLock(true)
	w.lock.release(w) // то, что делает WM_DESTROY
	if clip := plwClip(); clip != before {
		t.Errorf("зажим пережил окно: %+v", clip)
	}
	if plwRegistered(t) != 0 {
		t.Error("Raw Input пережил окно")
	}
}

// Из другого потока вызов уходит на поток окна и возвращает результат.
func TestWin32PointerLock_FromOtherThread(t *testing.T) {
	w := plwWindow(t)
	done := make(chan bool, 1)
	go func() { done <- w.SetPointerLock(true) }()
	// Поток окна — этот; качаем его сообщения, пока вызов не вернётся.
	for i := 0; i < 400; i++ {
		fsPumpMessages()
		select {
		case ok := <-done:
			if !ok {
				t.Fatal("SetPointerLock из другого потока отказал")
			}
			if plwRegistered(t) != uintptr(w.hwnd) {
				t.Error("регистрация не выполнена на потоке окна")
			}
			return
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	t.Fatal("вызов с другого потока не вернулся")
}

// Сквозная проверка с настоящим Raw Input: SendInput двигает мышь, система
// присылает WM_INPUT окну на переднем плане, приложение получает сдвиг. Двигает
// курсор пользователя (на несколько пикселей) и требует, чтобы окно удалось
// вывести на передний план, поэтому включается HEADLESS_GUI_POINTERLOCK_E2E=1.
func TestWin32PointerLock_LiveRawInput(t *testing.T) {
	if os.Getenv("HEADLESS_GUI_POINTERLOCK_E2E") != "1" {
		t.Skip("включается HEADLESS_GUI_POINTERLOCK_E2E=1: двигает курсор пользователя")
	}
	w := plwWindow(t)
	w.lock.foreground = nil // настоящая проверка переднего плана
	procSetForegroundWindow.Call(uintptr(w.hwnd))
	fsPumpMessages()
	if !w.isForeground() {
		t.Skip("окно не удалось вывести на передний план")
	}
	var mu sync.Mutex
	var sx, sy float64
	w.SetOnRelativeMotion(func(dx, dy float64) { mu.Lock(); sx += dx; sy += dy; mu.Unlock() })
	if !w.SetPointerLock(true) {
		t.Fatal("отказ")
	}
	const mouseEventMove = 0x0001
	for i := 0; i < 5; i++ {
		in := mouseInput{typ: 0, dx: 3, dy: -2, flags: mouseEventMove}
		procSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		fsPumpMessages()
		mu.Lock()
		got := sx != 0 || sy != 0
		mu.Unlock()
		if got {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	fsPumpMessages()
	mu.Lock()
	defer mu.Unlock()
	if sx != 15 || sy != -10 {
		t.Errorf("сумма сдвигов (%v,%v), ждал (15,-10)", sx, sy)
	}
}
