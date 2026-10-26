//go:build windows

package window

// lockkeys_windows.go — клавиши-переключатели Win32 (описание поведения — в
// lockkeys.go).
//
// Чтение: GetKeyState(vk) & 1 на потоке окна. GetKeyState отдаёт состояние
// очереди ввода потока (оно меняется по мере того, как поток разбирает свои
// сообщения клавиатуры), а на потоке окна оно к тому же синхронизируется с
// системой при каждой смене активного окна: вернувшись в окно после Caps Lock,
// нажатого в другой программе, видишь уже новое. GetAsyncKeyState не годится —
// его младший бит значит «нажимали после прошлого вызова», а не «включена»;
// GetKeyboardState возвращает тот же GetKeyState, но сразу для 256 клавиш.
// Поток окна не ответил за lkUITimeout (цикл сообщений занят) — читаем
// прямо: состояние переключателей общее на сеанс, и сбой цикла не повод
// отказывать.
//
// Запись: SendInput по каждой отличающейся клавише — нажатие и отпускание, как
// настоящее. Только так Windows позволяет прикладной программе переключить
// индикатор: SetKeyboardState меняет лишь очередь потока, а светодиод и
// состояние других программ не трогает. Побочный эффект — клавиша приходит
// окну в фокусе как обычное WM_KEYDOWN. Свои нажатия мы помечаем в
// dwExtraInfo и отсеиваем в wndProc по GetMessageExtraInfo (lkSyntheticKey):
// приложению они не доходят, и клиент удалённого рабочего стола не отправит
// серверу «лишнее» нажатие Caps Lock. Низкоуровневый хук захвата клавиатуры
// синтетические нажатия не трогает (LLKHF_INJECTED) — захват не страдает, а
// переключатели в его список и не входят.

import (
	"time"
	"unsafe"
)

var (
	procGetKeyState         = user32.NewProc("GetKeyState")
	procGetMessageExtraInfo = user32.NewProc("GetMessageExtraInfo")
	procGetKeyboardType     = user32.NewProc("GetKeyboardType")
	procLkSendInput         = user32.NewProc("SendInput")
)

const (
	// lkExtraInfo — метка наших синтетических нажатий в dwExtraInfo
	// (KEYBDINPUT) и тем самым в GetMessageExtraInfo при разборе сообщения.
	lkExtraInfo = 0x4C4B5359 // "LKSY"

	keyeventfExtendedKey = 0x0001
	keyeventfKeyUp       = 0x0002
	lkInputKeyboard      = 1 // INPUT_KEYBOARD
	// lkKeyboardJapanese — GetKeyboardType(0) японской клавиатуры: Kana Lock
	// существует только на ней.
	lkKeyboardJapanese = 7
)

// lkUITimeout — сколько читающая горутина ждёт поток окна.
var lkUITimeout = 500 * time.Millisecond

// lkKeybdInput — KEYBDINPUT.
type lkKeybdInput struct {
	vk, scan    uint16
	flags, time uint32
	extra       uintptr
}

// lkInput — INPUT с клавиатурным вариантом объединения. Объединение занимает
// размер наибольшего из вариантов (MOUSEINPUT), отсюда добавка до него: 40 байт
// на 64-битной Windows, 28 на 32-битной.
type lkInput struct {
	typ uint32
	ki  lkKeybdInput
	_   [8]byte
}

// Подмены системных вызовов в тестах.
var (
	lkKeyState = func(vk int) bool {
		r, _, _ := procGetKeyState.Call(uintptr(vk))
		return r&1 != 0
	}
	lkKeyboardType = func() int {
		r, _, _ := procGetKeyboardType.Call(0)
		return int(r)
	}
	// lkSend отправляет события в систему и возвращает, сколько принято.
	lkSend = func(in []lkInput) int {
		if len(in) == 0 {
			return 0
		}
		r, _, _ := procLkSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(in[0]))
		return int(r)
	}
	// lkMessageExtra — dwExtraInfo сообщения, которое сейчас разбирает поток.
	lkMessageExtra = func() uintptr {
		r, _, _ := procGetMessageExtraInfo.Call()
		return r
	}
)

// lkReadNow читает состояние переключателей на текущем потоке.
func lkReadNow() LockKeys { return lkFromToggles(lkKeyState) }

// LockKeys — состояние клавиш-переключателей (lockKeysBackend).
func (w *Win32Window) LockKeys() (LockKeys, bool) {
	if w.hwnd == 0 {
		return 0, false
	}
	if w.onUIThread() {
		return lkReadNow(), true
	}
	res := make(chan LockKeys, 1)
	w.InvokeOnUIThread(func() { res <- lkReadNow() })
	select {
	case k := <-res:
		return k, true
	case <-time.After(lkUITimeout):
		return lkReadNow(), true
	}
}

// lkBuildInputs собирает нажатие и отпускание каждой клавиши. scan отдаёт
// скан-код клавиши и признак «расширенная» (NumLock — расширенная, иначе
// система приняла бы его за Pause).
func lkBuildInputs(vks []int, scan func(vk int) (uint16, bool)) []lkInput {
	in := make([]lkInput, 0, 2*len(vks))
	for _, vk := range vks {
		sc, ext := scan(vk)
		var fl uint32
		if ext {
			fl = keyeventfExtendedKey
		}
		in = append(in,
			lkInput{typ: lkInputKeyboard, ki: lkKeybdInput{vk: uint16(vk), scan: sc, flags: fl, extra: lkExtraInfo}},
			lkInput{typ: lkInputKeyboard, ki: lkKeybdInput{vk: uint16(vk), scan: sc, flags: fl | keyeventfKeyUp, extra: lkExtraInfo}})
	}
	return in
}

// lkScanCode — скан-код клавиши для SendInput и признак «расширенная». Num Lock
// по документации Windows расширенная клавиша, но MapVirtualKey этого не
// сообщает; без признака на части клавиатур она читалась бы как Pause.
func lkScanCode(vk int) (uint16, bool) {
	scan, ext := vkScanCode(vk)
	if vk == VK_NUMLOCK {
		ext = true
	}
	return scan, ext
}

// SetLockKeys приводит переключатели к want нажатием отличающихся клавиш
// (lockKeysSetter).
func (w *Win32Window) SetLockKeys(want LockKeys) bool {
	cur, ok := w.LockKeys()
	if !ok {
		return false
	}
	diff := cur ^ want
	complete := true
	if diff&KanaLock != 0 && lkKeyboardType() != lkKeyboardJapanese {
		// Kana Lock переключается только клавишей японской клавиатуры;
		// нажатие VK_KANA на другой не делает ничего.
		want = want&^KanaLock | cur&KanaLock
		complete = false
	}
	vks := lkToggleKeys(cur, want)
	if len(vks) == 0 {
		return complete
	}
	in := lkBuildInputs(vks, lkScanCode)
	return lkSend(in) == len(in) && complete
}

// lkSyntheticKey — это нажатие или отпускание, которое мы сами только что
// послали через SendInput (SetLockKeys): разбираемое сообщение несёт нашу метку
// в dwExtraInfo. Вызывать только из обработчика сообщения клавиши в wndProc.
func lkSyntheticKey(vk int) bool {
	return lkIsLockVK(vk) && lkMessageExtra() == lkExtraInfo
}
