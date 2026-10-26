//go:build windows

package window

import (
	"os"
	"reflect"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Клавиши-переключатели на Win32: раскладка INPUT, выбор клавиш, отсев
// собственных синтетических нажатий в wndProc и живое сравнение с системой.

var procSetMessageExtraInfo = user32.NewProc("SetMessageExtraInfo")
var procGetKeyboardState = user32.NewProc("GetKeyboardState")

// lkFakeKeys подменяет GetKeyState и SendInput: состояние берётся из карты,
// отправленное запоминается.
type lkFakeKeys struct {
	state    map[int]bool
	sent     []lkInput
	accept   int // сколько событий «принимает» система; -1 — все
	japanese bool
}

func lkInstallFake(t *testing.T, f *lkFakeKeys) {
	t.Helper()
	oldS, oldT, oldK := lkSend, lkKeyState, lkKeyboardType
	lkKeyState = func(vk int) bool { return f.state[vk] }
	lkKeyboardType = func() int {
		if f.japanese {
			return lkKeyboardJapanese
		}
		return 4
	}
	lkSend = func(in []lkInput) int {
		f.sent = append(f.sent, in...)
		if f.accept >= 0 {
			return min(f.accept, len(in))
		}
		return len(in)
	}
	t.Cleanup(func() { lkSend, lkKeyState, lkKeyboardType = oldS, oldT, oldK })
}

// INPUT должен лежать в памяти так, как его читает система: 40 байт на
// 64-битной Windows (28 на 32-битной), клавиатурный вариант — после типа.
func TestLockKeysWin32_InputLayout(t *testing.T) {
	var in lkInput
	wantSize, wantOff := uintptr(40), uintptr(8)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		wantSize, wantOff = 28, 4
	}
	if unsafe.Sizeof(in) != wantSize {
		t.Errorf("sizeof(INPUT) = %d, ждал %d", unsafe.Sizeof(in), wantSize)
	}
	if unsafe.Offsetof(in.ki) != wantOff {
		t.Errorf("смещение KEYBDINPUT %d, ждал %d", unsafe.Offsetof(in.ki), wantOff)
	}
	if unsafe.Offsetof(in.ki.flags) != 4 || unsafe.Offsetof(in.ki.scan) != 2 {
		t.Errorf("поля KEYBDINPUT: scan@%d flags@%d", unsafe.Offsetof(in.ki.scan), unsafe.Offsetof(in.ki.flags))
	}
	if sz := unsafe.Sizeof(in.ki); sz != 16 && sz != 24 {
		t.Errorf("sizeof(KEYBDINPUT) = %d", sz)
	}
}

func TestLockKeysWin32_BuildInputs(t *testing.T) {
	scan := func(vk int) (uint16, bool) {
		switch vk {
		case VK_NUMLOCK:
			return 0x45, true
		case VK_CAPITAL:
			return 0x3A, false
		}
		return 0, false
	}
	in := lkBuildInputs([]int{VK_CAPITAL, VK_NUMLOCK}, scan)
	want := []lkInput{
		{typ: 1, ki: lkKeybdInput{vk: VK_CAPITAL, scan: 0x3A, extra: lkExtraInfo}},
		{typ: 1, ki: lkKeybdInput{vk: VK_CAPITAL, scan: 0x3A, flags: keyeventfKeyUp, extra: lkExtraInfo}},
		{typ: 1, ki: lkKeybdInput{vk: VK_NUMLOCK, scan: 0x45, flags: keyeventfExtendedKey, extra: lkExtraInfo}},
		{typ: 1, ki: lkKeybdInput{vk: VK_NUMLOCK, scan: 0x45, flags: keyeventfExtendedKey | keyeventfKeyUp, extra: lkExtraInfo}},
	}
	if !reflect.DeepEqual(in, want) {
		t.Errorf("события:\n  %+v\nждал:\n  %+v", in, want)
	}
	if len(lkBuildInputs(nil, scan)) != 0 {
		t.Error("без клавиш события нашлись")
	}
}

// Num Lock — расширенная клавиша: с обычным скан-кодом система приняла бы её
// за Pause. Проверяем на настоящей таблице раскладки.
func TestLockKeysWin32_NumLockIsExtended(t *testing.T) {
	scan, ext := lkScanCode(VK_NUMLOCK)
	if scan == 0 {
		t.Skip("система не знает скан-код Num Lock (нет клавиатуры?)")
	}
	if !ext {
		t.Errorf("Num Lock без признака расширенной клавиши: скан %#x", scan)
	}
	if _, ext := lkScanCode(VK_CAPITAL); ext {
		t.Error("Caps Lock признан расширенной клавишей")
	}
}

// Окно на потоке теста: чтение прямое, значения берутся из GetKeyState.
func TestLockKeysWin32_ReadsToggleState(t *testing.T) {
	f := &lkFakeKeys{state: map[int]bool{VK_CAPITAL: true, VK_SCROLL: true}, accept: -1}
	lkInstallFake(t, f)
	w := newFsWindow(t)
	got, ok := w.LockKeys()
	if !ok || got != CapsLock|ScrollLock {
		t.Errorf("LockKeys = %v, %v; ждал caps|scroll", got, ok)
	}
	f.state[VK_NUMLOCK], f.state[VK_CAPITAL] = true, false
	if got, _ := w.LockKeys(); got != NumLock|ScrollLock {
		t.Errorf("после смены LockKeys = %v", got)
	}
	var none Win32Window
	if k, ok := none.LockKeys(); ok || k != 0 {
		t.Errorf("окно без hwnd: %v, %v", k, ok)
	}
}

// Из чужой горутины вопрос уходит потоку окна и там получает ответ.
func TestLockKeysWin32_ReadsOnUIThread(t *testing.T) {
	f := &lkFakeKeys{accept: -1}
	var readOn []uint32
	lkInstallFake(t, f)
	lkKeyState = func(vk int) bool {
		if vk == VK_NUMLOCK {
			readOn = append(readOn, procCurrentThreadID())
			return true
		}
		return false
	}
	w := newFsWindow(t)
	uiThread := procCurrentThreadID()

	type res struct {
		k  LockKeys
		ok bool
	}
	done := make(chan res, 1)
	go func() {
		k, ok := w.LockKeys()
		done <- res{k, ok}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		fsPumpMessages()
		select {
		case r := <-done:
			if !r.ok || r.k != NumLock {
				t.Errorf("LockKeys = %v, %v", r.k, r.ok)
			}
			if len(readOn) != 1 || readOn[0] != uiThread {
				t.Errorf("GetKeyState звали на потоках %v, ждал поток окна %d", readOn, uiThread)
			}
			return
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("LockKeys из другой горутины завис")
		}
		time.Sleep(time.Millisecond)
	}
}

func procCurrentThreadID() uint32 {
	return windows.GetCurrentThreadId()
}

// Поток окна не крутит цикл сообщений: вопрос не виснет, а по сроку читается
// прямым вызовом.
func TestLockKeysWin32_StuckUIThreadFallsBack(t *testing.T) {
	f := &lkFakeKeys{state: map[int]bool{VK_SCROLL: true}, accept: -1}
	lkInstallFake(t, f)
	old := lkUITimeout
	lkUITimeout = 30 * time.Millisecond
	t.Cleanup(func() { lkUITimeout = old })
	w := newFsWindow(t)

	done := make(chan LockKeys, 1)
	go func() {
		k, _ := w.LockKeys()
		done <- k
	}()
	select {
	case k := <-done:
		if k != ScrollLock {
			t.Errorf("по сроку LockKeys = %v", k)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("LockKeys завис вместе с потоком окна")
	}
}

// Нажимаются только отличающиеся клавиши; остальные состояния не трогаются.
func TestLockKeysWin32_SetSendsOnlyDifferences(t *testing.T) {
	f := &lkFakeKeys{state: map[int]bool{VK_CAPITAL: true}, accept: -1}
	lkInstallFake(t, f)
	w := newFsWindow(t)

	if !w.SetLockKeys(CapsLock | NumLock) {
		t.Fatal("SetLockKeys отказал")
	}
	var vks []int
	for i, in := range f.sent {
		if in.ki.extra != lkExtraInfo {
			t.Errorf("событие %d без метки", i)
		}
		if i%2 == 0 {
			vks = append(vks, int(in.ki.vk))
		}
	}
	if !reflect.DeepEqual(vks, []int{VK_NUMLOCK}) || len(f.sent) != 2 {
		t.Errorf("нажато %v (%d событий), ждал только Num Lock", vks, len(f.sent))
	}

	// Уже как просили: ничего не отправляется, ответ — успех.
	f.sent = nil
	if !w.SetLockKeys(CapsLock) || len(f.sent) != 0 {
		t.Errorf("совпадающий набор: отправлено %d событий", len(f.sent))
	}

	// Сбой SendInput (например, окно рабочего стола защищено) — false.
	f.accept = 0
	if w.SetLockKeys(0) {
		t.Error("SendInput отказал, а SetLockKeys ответил true")
	}
	f.accept = 1 // принял только нажатие, отпускание потеряно
	if w.SetLockKeys(0) {
		t.Error("принята половина событий, а SetLockKeys ответил true")
	}
}

// Kana Lock есть только на японской клавиатуре: на другой нажатие VK_KANA ничего
// не даёт, и честный ответ — false, а остальные клавиши всё равно приводятся.
func TestLockKeysWin32_KanaOnlyOnJapaneseKeyboard(t *testing.T) {
	f := &lkFakeKeys{accept: -1}
	lkInstallFake(t, f)
	w := newFsWindow(t)

	if w.SetLockKeys(KanaLock | NumLock) {
		t.Error("Kana на обычной клавиатуре признана выставленной")
	}
	var vks []int
	for i, in := range f.sent {
		if i%2 == 0 {
			vks = append(vks, int(in.ki.vk))
		}
	}
	if !reflect.DeepEqual(vks, []int{VK_NUMLOCK}) {
		t.Errorf("нажато %v, ждал только Num Lock", vks)
	}

	f.sent, f.japanese = nil, true
	if !w.SetLockKeys(KanaLock) {
		t.Error("на японской клавиатуре Kana не выставилась")
	}
	if len(f.sent) != 2 || f.sent[0].ki.vk != VK_KANA {
		t.Errorf("отправлено %+v", f.sent)
	}
}

// Свои синтетические нажатия переключателей до приложения не доходят — иначе
// клиент удалённого рабочего стола переслал бы серверу ещё и нажатие Caps Lock.
// Чужие (настоящие или с другой меткой) доходят как всегда.
func TestLockKeysWin32_SyntheticKeysNotDelivered(t *testing.T) {
	w := fakeWin32(t)
	var got []rawKey
	w.SetOnRawKey(func(k rawKey) { got = append(got, k) })
	extra := uintptr(0)
	old := lkMessageExtra
	lkMessageExtra = func() uintptr { return extra }
	t.Cleanup(func() { lkMessageExtra = old })

	for _, vk := range []uintptr{VK_CAPITAL, VK_NUMLOCK, VK_SCROLL, VK_KANA} {
		extra = lkExtraInfo
		wndProc(fakeHwnd, wmKeydown, vk, 0x003A0001)
		wndProc(fakeHwnd, wmKeyup, vk, 0xC03A0001)
		wndProc(fakeHwnd, wmSyskeydown, vk, 0x203A0001)
		wndProc(fakeHwnd, wmSyskeyup, vk, 0xE03A0001)
	}
	if len(got) != 0 {
		t.Fatalf("синтетические нажатия дошли до приложения: %+v", got)
	}

	// Настоящее нажатие Caps Lock (метки нет) и чужая подстановка с другой
	// меткой — как обычно.
	extra = 0
	wndProc(fakeHwnd, wmKeydown, VK_CAPITAL, 0x003A0001)
	wndProc(fakeHwnd, wmKeyup, VK_CAPITAL, 0xC03A0001)
	extra = 0x1234
	wndProc(fakeHwnd, wmKeydown, VK_NUMLOCK, 0x01450001)
	// Метка на обычной клавише — не наша забота: отсев касается только переключателей.
	extra = lkExtraInfo
	wndProc(fakeHwnd, wmKeydown, VK_A, 0x001E0001)
	if len(got) != 4 {
		t.Fatalf("дошло %d событий, ждал 4: %+v", len(got), got)
	}
	if got[0].VK != VK_CAPITAL || !got[0].Pressed || got[1].VK != VK_CAPITAL || got[1].Pressed ||
		got[2].VK != VK_NUMLOCK || got[3].VK != VK_A {
		t.Errorf("не те события: %+v", got)
	}
}

// Метка действительно переживает путь через систему: SetMessageExtraInfo
// кладёт значение, откуда его берёт GetMessageExtraInfo.
func TestLockKeysWin32_MessageExtraInfoRoundTrip(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	prev, _, _ := procSetMessageExtraInfo.Call(lkExtraInfo)
	defer procSetMessageExtraInfo.Call(prev)
	if got := lkMessageExtra(); got != lkExtraInfo {
		t.Errorf("GetMessageExtraInfo = %#x, ждал %#x", got, uintptr(lkExtraInfo))
	}
	if !lkSyntheticKey(VK_CAPITAL) || lkSyntheticKey(VK_A) {
		t.Error("lkSyntheticKey не различает клавиши при метке в очереди")
	}
}

// sysToggleState — состояние переключателей по GetKeyboardState: другой путь к
// тому же, что и GetKeyState, но через массив из 256 байт.
func sysToggleState() LockKeys {
	var keys [256]byte
	procGetKeyboardState.Call(uintptr(unsafe.Pointer(&keys[0])))
	return lkFromToggles(func(vk int) bool { return keys[vk]&1 != 0 })
}

// Живое сравнение с тем, что говорит сама система.
func TestLockKeysWin32_LiveMatchesSystem(t *testing.T) {
	w := newFsWindow(t)
	got, ok := w.LockKeys()
	if !ok {
		t.Fatal("окно создано, а LockKeys не знает")
	}
	if want := sysToggleState(); got != want {
		t.Errorf("LockKeys = %v, система (GetKeyboardState) говорит %v", got, want)
	}
	t.Logf("переключатели сейчас: %v", got)
}

// lkLiveToggle переключает клавишу через SetLockKeys и убеждается, что система
// это увидела; исходное состояние возвращается в Cleanup, что бы ни случилось
// (пользователю нельзя оставить включённый Caps Lock).
func lkLiveToggle(t *testing.T, w *Win32Window, flag LockKeys) {
	t.Helper()
	orig, _ := w.LockKeys()
	t.Cleanup(func() {
		if cur, _ := w.LockKeys(); cur&flag != orig&flag {
			w.SetLockKeys(cur ^ flag)
			lkWaitState(t, w, flag, orig&flag != 0)
		}
	})
	want := orig ^ flag
	if !w.SetLockKeys(want) {
		t.Fatalf("SetLockKeys(%v) отказал", want)
	}
	lkWaitState(t, w, flag, want&flag != 0)
	if got := sysToggleState(); got&flag != want&flag {
		t.Errorf("система после переключения: %v, ждал флаг %v = %v", got, flag, want&flag != 0)
	}
	// Остальные клавиши не тронуты.
	if cur, _ := w.LockKeys(); cur&^flag != orig&^flag {
		t.Errorf("задеты чужие переключатели: было %v, стало %v", orig, cur)
	}
}

// lkWaitState ждёт, пока флаг примет значение on: SendInput применяется
// асинхронно.
func lkWaitState(t *testing.T, w *Win32Window, flag LockKeys, on bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		fsPumpMessages()
		if cur, _ := w.LockKeys(); (cur&flag != 0) == on {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("флаг %v не стал %v за 3 с", flag, on)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Живой SendInput по Scroll Lock: самому безобидному переключателю. Исходное
// состояние возвращается в Cleanup.
func TestLockKeysWin32_LiveSetScrollLock(t *testing.T) {
	w := newFsWindow(t)
	lkLiveToggle(t, w, ScrollLock)
}

// Caps Lock и Num Lock мигают индикаторами клавиатуры пользователя, поэтому
// только по просьбе:
//
//	HEADLESS_GUI_LOCKKEYS_LIVE=1 go test -run TestLockKeysWin32_LiveSetCapsNum ./window
func TestLockKeysWin32_LiveSetCapsNum(t *testing.T) {
	if os.Getenv("HEADLESS_GUI_LOCKKEYS_LIVE") != "1" {
		t.Skip("включается HEADLESS_GUI_LOCKKEYS_LIVE=1: на миг переключает Caps Lock и Num Lock пользователя")
	}
	w := newFsWindow(t)
	lkLiveToggle(t, w, NumLock)
	lkLiveToggle(t, w, CapsLock)
}

// Сквозная проверка с окном на переднем плане: настоящий SendInput доходит до
// настоящей очереди окна. Сначала контроль — нажатие БЕЗ нашей метки приходит
// приложению (значит, окно вообще получает вводимое программно); затем
// SetLockKeys — его нажатия приложению не приходят, а состояние меняется.
// Мигает индикаторами и требует права на передний план, поэтому по просьбе:
//
//	HEADLESS_GUI_LOCKKEYS_LIVE=1 go test -run TestLockKeysWin32_LiveForegroundFilter ./window
func TestLockKeysWin32_LiveForegroundFilter(t *testing.T) {
	if os.Getenv("HEADLESS_GUI_LOCKKEYS_LIVE") != "1" {
		t.Skip("включается HEADLESS_GUI_LOCKKEYS_LIVE=1: на миг переключает Scroll Lock пользователя")
	}
	w := newFsWindow(t)
	var got []rawKey
	w.SetOnRawKey(func(k rawKey) { got = append(got, k) })
	w.Activate()
	fsPumpMessages()
	if fg, _, _ := procGetForegroundWindow.Call(); fg != uintptr(w.hwnd) {
		t.Skip("окно не стало активным (нет права на передний план в этой сессии)")
	}

	orig, _ := w.LockKeys()
	t.Cleanup(func() {
		if cur, _ := w.LockKeys(); cur&ScrollLock != orig&ScrollLock {
			w.SetLockKeys(cur ^ ScrollLock)
			lkWaitState(t, w, ScrollLock, orig&ScrollLock != 0)
		}
	})

	// Контроль: две пары нажатий без метки (переключение туда и обратно).
	plain := lkBuildInputs([]int{VK_SCROLL, VK_SCROLL}, lkScanCode)
	for i := range plain {
		plain[i].ki.extra = 0
	}
	if lkSend(plain) != len(plain) {
		t.Fatal("SendInput отказал")
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(got) < 4 && time.Now().Before(deadline) {
		fsPumpMessages()
		time.Sleep(5 * time.Millisecond)
	}
	if len(got) != 4 {
		t.Fatalf("контроль: окно получило %d событий Scroll Lock из 4: %+v", len(got), got)
	}

	// Свои нажатия: состояние меняется, приложению ничего не приходит.
	got = nil
	want := orig ^ ScrollLock
	if !w.SetLockKeys(want) {
		t.Fatal("SetLockKeys отказал")
	}
	lkWaitState(t, w, ScrollLock, want&ScrollLock != 0)
	for end := time.Now().Add(200 * time.Millisecond); time.Now().Before(end); {
		fsPumpMessages()
		time.Sleep(5 * time.Millisecond)
	}
	if len(got) != 0 {
		t.Errorf("синтетические нажатия дошли до приложения: %+v", got)
	}
}
