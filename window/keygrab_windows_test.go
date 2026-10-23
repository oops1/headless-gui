//go:build windows

package window

import (
	"testing"
	"time"
	"unsafe"
)

// Хук WH_KEYBOARD_LL на настоящем окне. Настоящие нажатия в тесте не
// породить: SendInput помечает их LLKHF_INJECTED, а такие хук намеренно не
// трогает. Поэтому хук ставится по-настоящему (SetWindowsHookEx и
// UnhookWindowsHookEx — это и проверяется), а события в его процедуру подаются
// напрямую, с KBDLLHOOKSTRUCT в том виде, в каком его кладёт система.

func grabTestWindow(t *testing.T) *Win32Window {
	t.Helper()
	w := newFsWindow(t) // окно создаётся в этом же потоке (fullscreen_windows_test.go)
	t.Cleanup(func() { kbGrab.release(w, false) })
	return w
}

// grabTestFeedKey подаёт хуку одно событие и сообщает, съел ли он клавишу.
func grabTestFeedKey(vk uint32, msg uintptr, flags uint32) bool {
	kb := kbdLLHookStruct{VkCode: vk, Flags: flags}
	return kbGrab.handle(0, msg, uintptr(unsafe.Pointer(&kb)))
}

// grabTestFG подменяет GetForegroundWindow на время теста.
func grabTestFG(t *testing.T, hwnd uintptr) {
	t.Helper()
	old := kbForegroundWindow
	kbForegroundWindow = func() uintptr { return hwnd }
	t.Cleanup(func() { kbForegroundWindow = old })
}

func TestWin32KeyboardGrab_InstallsAndRemovesHook(t *testing.T) {
	w := grabTestWindow(t)
	if !w.SetKeyboardGrab(true) {
		t.Skip("хук WH_KEYBOARD_LL не ставится в этой сессии")
	}
	if kbGrab.hookHandle() == 0 {
		t.Fatal("SetKeyboardGrab(true) вернул true, а хука нет")
	}
	// Повторное включение — не второй хук.
	h := kbGrab.hookHandle()
	if !w.SetKeyboardGrab(true) || kbGrab.hookHandle() != h {
		t.Error("повторное включение поставило второй хук")
	}
	w.SetKeyboardGrab(false)
	if kbGrab.hookHandle() != 0 {
		t.Error("после SetKeyboardGrab(false) хук остался")
	}
	if !w.SetKeyboardGrab(false) {
		t.Error("повторное выключение должно быть успешным")
	}
}

// Хук один на процесс: со вторым окном он остаётся, пока захват держит хоть
// одно.
func TestWin32KeyboardGrab_SharedHook(t *testing.T) {
	a := grabTestWindow(t)
	b := grabTestWindow(t)
	if !a.SetKeyboardGrab(true) {
		t.Skip("хук не ставится")
	}
	b.SetKeyboardGrab(true)
	h := kbGrab.hookHandle()
	a.SetKeyboardGrab(false)
	if kbGrab.hookHandle() != h || h == 0 {
		t.Error("хук снят, хотя второе окно держит захват")
	}
	b.SetKeyboardGrab(false)
	if kbGrab.hookHandle() != 0 {
		t.Error("хук остался без окон")
	}
}

// Уничтожение окна снимает хук само: он глобальный и не должен пережить окно.
func TestWin32KeyboardGrab_WindowDestroyReleasesHook(t *testing.T) {
	w := grabTestWindow(t)
	if !w.SetKeyboardGrab(true) {
		t.Skip("хук не ставится")
	}
	procDestroyWindow.Call(uintptr(w.hwnd))
	if kbGrab.hookHandle() != 0 {
		t.Error("окно уничтожено, хук остался")
	}
	if w.KeyboardGrabActive() {
		t.Error("KeyboardGrabActive true у уничтоженного окна")
	}
	// Закрытие окна зовёт выключение захвата уже после уничтожения: потока окна
	// может не быть, и ждать его нельзя.
	start := time.Now()
	if !w.SetKeyboardGrab(false) {
		t.Error("выключение захвата у уничтоженного окна вернуло false")
	}
	if d := time.Since(start); d > kbGrabUITimeout/2 {
		t.Errorf("выключение у уничтоженного окна заняло %v", d)
	}
}

// Win, Alt+Tab и PrintScreen при активном окне уходят окну и не уходят системе.
func TestWin32KeyboardGrab_DeliversToWindow(t *testing.T) {
	w := grabTestWindow(t)
	if !w.SetKeyboardGrab(true) {
		t.Skip("хук не ставится")
	}
	grabTestFG(t, uintptr(w.hwnd))

	type ev struct {
		vk   int
		down bool
		rep  bool
	}
	var got []ev
	w.onKeyDownRepeat = func(vk int, rep bool) { got = append(got, ev{vk, true, rep}) }
	w.onKeyUp = func(vk int) { got = append(got, ev{vk, false, false}) }

	if !grabTestFeedKey(VK_LWIN, wmKeydown, 0) {
		t.Error("нажатие Win не перехвачено")
	}
	if !grabTestFeedKey(VK_LWIN, wmKeydown, 0) { // автоповтор
		t.Error("автоповтор Win не перехвачен")
	}
	if !grabTestFeedKey(VK_E, wmKeydown, 0) { // Win+E
		t.Error("Win+E не перехвачено")
	}
	grabTestFeedKey(VK_E, wmKeyup, 0)
	if !grabTestFeedKey(VK_LWIN, wmKeyup, 0) {
		t.Error("отпускание Win не перехвачено")
	}
	if !grabTestFeedKey(VK_TAB, wmSyskeydown, llkhfAltdown) { // Alt+Tab
		t.Error("Alt+Tab не перехвачен")
	}
	grabTestFeedKey(VK_TAB, wmSyskeyup, llkhfAltdown)
	if !grabTestFeedKey(VK_SNAPSHOT, wmKeydown, 0) { // PrintScreen: система без хука шлёт окну только отпускание
		t.Error("нажатие PrintScreen не перехвачено")
	}
	if !grabTestFeedKey(VK_SNAPSHOT, wmKeyup, 0) {
		t.Error("отпускание PrintScreen не перехвачено")
	}
	// Обычная клавиша системе и приложению как раньше.
	if grabTestFeedKey(VK_A, wmKeydown, 0) {
		t.Error("обычная клавиша A перехвачена")
	}

	want := []ev{
		{VK_LWIN, true, false}, {VK_LWIN, true, true}, {VK_E, true, false},
		{VK_E, false, false}, {VK_LWIN, false, false},
		{VK_TAB, true, false}, {VK_TAB, false, false},
		{VK_SNAPSHOT, true, false}, {VK_SNAPSHOT, false, false},
	}
	if len(got) != len(want) {
		t.Fatalf("окно получило %d событий, ждал %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("событие %d: %+v, ждал %+v", i, got[i], want[i])
		}
	}
}

// Пока активно другое окно, хук ничего не трогает; синтетические нажатия
// (SendInput) тоже.
func TestWin32KeyboardGrab_OnlyWhenForeground(t *testing.T) {
	w := grabTestWindow(t)
	if !w.SetKeyboardGrab(true) {
		t.Skip("хук не ставится")
	}
	called := false
	w.onKeyDown = func(int) { called = true }

	grabTestFG(t, uintptr(w.hwnd)+1000) // активно чужое окно
	if grabTestFeedKey(VK_LWIN, wmKeydown, 0) {
		t.Error("Win перехвачена при чужом активном окне")
	}
	grabTestFG(t, uintptr(w.hwnd))
	if grabTestFeedKey(VK_LWIN, wmKeydown, llkhfInjected) {
		t.Error("синтетическое нажатие Win перехвачено")
	}
	if called {
		t.Error("окно получило клавишу, которую система не отдавала")
	}
	// Отрицательный nCode — обязательный пропуск по контракту хука.
	kb := kbdLLHookStruct{VkCode: VK_LWIN}
	if kbGrab.handle(^uintptr(0), wmKeydown, uintptr(unsafe.Pointer(&kb))) {
		t.Error("nCode < 0 обработан")
	}
}

// Выключение захвата «отпускает» перехваченные и ещё зажатые клавиши, чтобы
// приложение не считало Win зажатой.
func TestWin32KeyboardGrab_ReleaseSendsKeyUps(t *testing.T) {
	w := grabTestWindow(t)
	if !w.SetKeyboardGrab(true) {
		t.Skip("хук не ставится")
	}
	grabTestFG(t, uintptr(w.hwnd))
	var ups []int
	w.onKeyDown = func(int) {}
	w.onKeyUp = func(vk int) { ups = append(ups, vk) }
	grabTestFeedKey(VK_LWIN, wmKeydown, 0)

	w.SetKeyboardGrab(false)
	if len(ups) != 1 || ups[0] != VK_LWIN {
		t.Errorf("при выключении окно получило отпускания %v, ждал [Win]", ups)
	}
}

func TestWin32KeyboardGrab_ActiveRequiresForeground(t *testing.T) {
	w := grabTestWindow(t)
	if w.KeyboardGrabActive() {
		t.Error("KeyboardGrabActive true до включения")
	}
	if !w.SetKeyboardGrab(true) {
		t.Skip("хук не ставится")
	}
	grabTestFG(t, 12345)
	if w.KeyboardGrabActive() {
		t.Error("KeyboardGrabActive true у неактивного окна")
	}
	grabTestFG(t, uintptr(w.hwnd))
	if !w.KeyboardGrabActive() {
		t.Error("KeyboardGrabActive false у активного окна с захватом")
	}
}
