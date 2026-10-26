package window

// lockkeys.go — состояние клавиш-переключателей (Caps Lock, Num Lock, Scroll
// Lock, Kana Lock) и выставление его из приложения (v3.44, задание
// RDP-клиента Tandem).
//
// Клиент удалённого рабочего стола обязан сообщать серверу, какие
// переключатели включены у пользователя (Synchronize, [MS-RDPBCGR]
// 2.2.8.1.1.3.1.1.5), и после подключения, и каждый раз, когда окно снова
// получило фокус: пока оно было неактивно, Caps Lock могли переключить в
// другой программе. А сервер в обратную сторону просит включить индикаторы
// (Set Keyboard Indicators PDU) — их надо выставить и на клавиатуре клиента.
//
// Откуда берётся состояние:
//
//	Win32    GetKeyState(VK_CAPITAL / VK_NUMLOCK / VK_SCROLL / VK_KANA) & 1.
//	         Зовётся на потоке окна (GetKeyState — состояние очереди ввода
//	         потока, и именно на этом потоке оно синхронизируется с системой
//	         при смене активного окна); не отозвался поток — прямым вызовом.
//	         GetAsyncKeyState не годится: его младший бит — «нажимали после
//	         прошлого вызова», а не «включена».
//	X11      расширение XKEYBOARD: GetIndicatorState и имена индикаторов
//	         «Caps Lock», «Num Lock», «Scroll Lock», «Kana»; нет индикатора с
//	         таким именем — зафиксированные модификаторы (XkbGetState) по
//	         маске, на которой стоит клавиша; нет XKB — ядерный протокол
//	         (QueryPointer, маска Lock и модификатор клавиши Num_Lock из
//	         GetModifierMapping). Вопрос уходит в цикл событий окна и там
//	         получает ответ (x11_lockkeys_linux.go).
//	Wayland  последнее wl_keyboard.modifiers: mods_locked, биты — по индексам
//	         модификаторов Lock и Mod2/NumLock из keymap композитора. Пока
//	         после получения фокуса modifiers не пришло, состояние неизвестно.
//	macOS    не реализовано.
//
// Выставление (SetLockKeys) есть там, где его делает сама ОС: Win32 (SendInput
// по клавише-переключателю) и X11 (XkbLatchLockState). На Wayland в основном
// протоколе клиент состояние клавиатуры менять не вправе; v3.44.1 делает это
// виртуальной клавиатурой zwp_virtual_keyboard_v1, если композитор её предлагает
// (wayland_lockkeys_set_linux.go), иначе там false.

import (
	"strings"
	"time"
)

// LockKeys — набор включённых клавиш-переключателей.
type LockKeys uint8

const (
	// CapsLock — включён Caps Lock.
	CapsLock LockKeys = 1 << iota
	// NumLock — включён Num Lock.
	NumLock
	// ScrollLock — включён Scroll Lock.
	ScrollLock
	// KanaLock — включён Kana Lock (японская клавиатура).
	KanaLock
)

// lkAll — все известные клавиши-переключатели.
const lkAll = CapsLock | NumLock | ScrollLock | KanaLock

// Has сообщает, что в наборе включены ВСЕ перечисленные клавиши.
func (k LockKeys) Has(f LockKeys) bool { return f != 0 && k&f == f }

// Empty — ни один переключатель не включён.
func (k LockKeys) Empty() bool { return k&lkAll == 0 }

// String — для логов и сообщений: «caps|num», «-» для пустого набора.
func (k LockKeys) String() string {
	var parts []string
	for _, n := range []struct {
		f    LockKeys
		name string
	}{
		{CapsLock, "caps"}, {NumLock, "num"}, {ScrollLock, "scroll"}, {KanaLock, "kana"},
	} {
		if k&n.f != 0 {
			parts = append(parts, n.name)
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "|")
}

// lkKeyTable — соответствие флагов клавишам Windows (VK_*): по ним читается
// состояние и ими же переключается на Win32.
var lkKeyTable = [...]struct {
	flag LockKeys
	vk   int
}{
	{CapsLock, VK_CAPITAL}, {NumLock, VK_NUMLOCK}, {ScrollLock, VK_SCROLL}, {KanaLock, VK_KANA},
}

// lkFromToggles собирает набор из состояния каждой клавиши: on(vk) — включена
// ли клавиша-переключатель с этим кодом.
func lkFromToggles(on func(vk int) bool) LockKeys {
	var k LockKeys
	for _, e := range lkKeyTable {
		if on(e.vk) {
			k |= e.flag
		}
	}
	return k
}

// lkToggleKeys — клавиши, которые нужно нажать, чтобы из набора cur получить
// want: по одной на каждый отличающийся флаг, в порядке Caps, Num, Scroll, Kana.
func lkToggleKeys(cur, want LockKeys) []int {
	var vks []int
	for _, e := range lkKeyTable {
		if (cur^want)&e.flag != 0 {
			vks = append(vks, e.vk)
		}
	}
	return vks
}

// lkIsLockVK — код клавиши-переключателя.
func lkIsLockVK(vk int) bool {
	for _, e := range lkKeyTable {
		if e.vk == vk {
			return true
		}
	}
	return false
}

// lockKeysBackend — бэкенд знает состояние клавиш-переключателей. Отдельный
// интерфейс по той же причине, что и остальные необязательные способности:
// сигнатуру NativeWindow менять нельзя.
type lockKeysBackend interface {
	// LockKeys — состояние переключателей; false — бэкенд его не знает.
	// Звать можно из любой горутины, кроме потока событий самого бэкенда.
	LockKeys() (LockKeys, bool)
}

// lockKeysSetter — бэкенд умеет переключать клавиши-переключатели.
type lockKeysSetter interface {
	// SetLockKeys приводит переключатели к набору want. true — всё, что
	// отличалось от want, переключено (или запрос принят системой).
	SetLockKeys(want LockKeys) bool
}

// lockKeysSettler — необязательная способность бэкенда, у которого состояние
// для только что полученного фокуса приходит отдельным событием, позже
// самого фокуса (Wayland: wl_keyboard.modifiers после enter).
type lockKeysSettler interface {
	// LockKeysSettled — состояние для текущего фокуса уже получено.
	LockKeysSettled() bool
	// SetOnLockKeysSettled регистрирует уведомление о том, что оно пришло.
	// Зовётся потоком событий бэкенда.
	SetOnLockKeysSettled(fn func())
}

// lkSettleTimeout — сколько уведомление об активации ждёт состояния
// переключателей, если композитор его так и не прислал (клавиатуры у сиденья
// нет, modifiers не пришло): без предела уведомление потерялось бы.
var lkSettleTimeout = 300 * time.Millisecond

// LockKeys сообщает, какие клавиши-переключатели включены сейчас. Второе
// значение false — бэкенд состояния не знает: окна ещё нет (до Run), платформа
// не умеет (macOS), на Wayland окно ещё не получало фокуса или он уже
// потерян (композитор присылает состояние только окну с фокусом клавиатуры),
// на X11 нет ответа сервера. Набор при этом нулевой.
//
// Неопределимые клавиши в набор не попадают, а на ответ «известно» это не
// влияет: Scroll Lock на Wayland обычно не модификатор и не виден вовсе, Kana
// Lock есть только на японских клавиатурах.
//
// Звать из обработчика SetOnActivate(true) можно, но на Wayland там состояние
// ещё может не прийти: оно приходит событием клавиатуры, а активацию сообщает
// другое событие, порядок которых композитор выбирает сам. Для значения,
// которое гарантированно относится к полученному фокусу, есть
// SetOnActivateLocks.
//
// Потокобезопасно. На X11 один вызов стоит обращения к серверу (миллисекунды).
func (win *Window) LockKeys() (LockKeys, bool) {
	if lb, ok := win.native.(lockKeysBackend); ok {
		if k, known := lb.LockKeys(); known {
			return k & lkAll, true
		}
	}
	return 0, false
}

// SetLockKeys приводит клавиши-переключатели к набору want: включает те, что
// в нём есть, и выключает остальные. Нужно клиенту удалённого рабочего стола,
// которому сервер прислал, какие индикаторы должны гореть.
//
// true — все клавиши, которые отличались от want, переключены. false — окна
// нет, платформа менять состояние не умеет (macOS: не реализовано; Wayland без
// zwp_virtual_keyboard_manager_v1 или пока окно не в фокусе клавиатуры) либо
// какую-то из клавиш переключить не вышло (на X11 и Wayland — у клавиши нет
// модификатора, например Scroll Lock в большинстве раскладок; Kana Lock на
// Wayland). Остальные клавиши при этом всё равно приведены.
//
//	Win32  SendInput: нажатие и отпускание клавиши-переключателя, только для
//	       отличающихся. Нажатия синтетические и помечены: окно НЕ отдаёт их
//	       приложению как KeyDown/KeyUp (иначе клиент удалённого стола
//	       послал бы серверу ещё и нажатие Caps Lock, тот переключил бы свой
//	       индикатор снова и попросил бы клиента переключить его опять).
//	       Нажатие уходит в окно, которое сейчас в фокусе: у приложения в фоне
//	       его получит чужая программа (в ней, как и всегда, ничего не
//	       случится — Caps Lock переключает сама система). Применяется
//	       асинхронно: LockKeys сразу после вызова может ещё показать старое.
//	X11    XKB LatchLockState на модификаторах Lock (Caps Lock), Num_Lock и
//	       Scroll_Lock; клавиш никто не нажимает. Нужно расширение XKEYBOARD.
//	Wayland виртуальная клавиатура (virtual-keyboard-unstable-v1): раскладка
//	       композитора и modifiers с нужными битами mods_locked; клавиш не шлётся,
//	       приложение KeyDown/KeyUp не получает. true — следующий
//	       wl_keyboard.modifiers показал просимое (ждёт до 300 мс, поэтому звать
//	       из горутины приложения; из обработчика событий окна запрос уходит,
//	       но ответ всегда false). Композитор может применить modifiers только к
//	       событиям самой виртуальной клавиатуры — тогда честный ответ false.
//
// Потокобезопасно.
func (win *Window) SetLockKeys(want LockKeys) bool {
	if lb, ok := win.native.(lockKeysSetter); ok {
		return lb.SetLockKeys(want & lkAll)
	}
	return false
}

// SetOnActivateLocks регистрирует колбэк смены активности окна вместе с
// состоянием клавиш-переключателей на этот момент. Нужен клиенту удалённого
// рабочего стола: Caps Lock мог измениться, пока окно было в фоне, и при
// возврате фокуса серверу нужно сообщить актуальное состояние.
//
// Колбэк вызывается на горутине движка, в тех же случаях, что и колбэк
// SetOnActivate (который это не отменяет и не меняет): только при смене
// состояния, и при потере активности позже того, как отпущены зажатые клавиши.
// Отличие в значении locks (known — его удалось узнать, как у LockKeys):
//
//   - active == true: состояние гарантированно относится к полученному
//     фокусу. Там, где композитор присылает его позже самой активации
//     (Wayland: wl_keyboard.modifiers приходит после enter, а порядок enter и
//     configure(activated) он выбирает сам), колбэк откладывается до его
//     прихода, но не дольше 300 мс: не пришло — колбэк вызывается с
//     known == false. Win32 и X11 спрашивают состояние в момент вызова.
//   - active == false: то, что бэкенд знает в этот момент (на Wayland после
//     потери фокуса — ничего, known == false); для синхронизации оно не
//     нужно, это сведения «как было».
//
// nil отключает колбэк. Вызывать до Run().
func (win *Window) SetOnActivateLocks(fn func(active bool, locks LockKeys, known bool)) {
	if fn == nil {
		win.onActivateLocks.Store(nil)
		return
	}
	win.onActivateLocks.Store(&fn)
}

// lkActivation решает, когда сообщать приложению об активации вместе с
// переключателями. Зовётся потоком бэкенда при каждом уведомлении об
// активности (setupActivation).
func (win *Window) lkActivation(active bool) {
	if win.onActivateLocks.Load() == nil {
		return
	}
	if !active {
		win.lkWait.Store(0) // отложенная активация уже недействительна
		win.post(func() { win.notifyActivationLocks(false) })
		return
	}
	if st, ok := win.native.(lockKeysSettler); ok && !st.LockKeysSettled() {
		// Состояние для этого фокуса ещё в пути. Повторные configure с тем же
		// activated ждут того же события, второго ожидания не заводим.
		gen := win.lkSeq.Add(1)
		if win.lkWait.CompareAndSwap(0, gen) {
			time.AfterFunc(lkSettleTimeout, func() { win.lkExpired(gen) })
			if st.LockKeysSettled() {
				win.lkSettled() // пришло между проверкой и ожиданием
			}
		}
		return
	}
	win.post(func() { win.notifyActivationLocks(true) })
}

// lkSettled завершает отложенную активацию: состояние пришло. Зовётся потоком
// бэкенда при каждом первом состоянии нового фокуса; ожидания нет — ничего не
// делает.
func (win *Window) lkSettled() {
	if win.lkWait.Swap(0) != 0 {
		win.post(func() { win.notifyActivationLocks(true) })
	}
}

// lkExpired — срок ожидания gen вышел. Устаревший таймер (ожидание давно
// закончилось, началось уже другое) ничего не трогает.
func (win *Window) lkExpired(gen uint64) {
	if win.lkWait.CompareAndSwap(gen, 0) {
		win.post(func() { win.notifyActivationLocks(true) })
	}
}

// notifyActivationLocks вызывает колбэк SetOnActivateLocks при смене
// состояния. Горутина движка.
func (win *Window) notifyActivationLocks(active bool) {
	next := activeNo
	if active {
		next = activeYes
	}
	if win.lkNow.Swap(next) == next {
		return
	}
	fn := win.onActivateLocks.Load()
	if fn == nil {
		return
	}
	locks, known := win.LockKeys()
	(*fn)(active, locks, known)
}
