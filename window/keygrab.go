package window

// keygrab.go — захват клавиатуры (задание RDP-клиента E3).
//
// Удалённый рабочий стол, виртуальная машина и игра хотят видеть ВСЕ клавиши,
// в том числе те, что система забирает себе: Win, Alt+Tab, Alt+Esc, Ctrl+Esc,
// PrintScreen. Обычное окно их не получает: оболочка обрабатывает такое
// сочетание раньше. SetKeyboardGrab просит систему отдавать их окну, пока оно
// в фокусе:
//
//	Win32   низкоуровневый хук WH_KEYBOARD_LL на потоке окна. Хук работает на
//	        весь рабочий стол, поэтому он трогает клавиши, только пока
//	        активно наше окно (GetForegroundWindow); перехваченное нажатие
//	        уходит окну как обычный KeyDown/KeyUp, а системе не достаётся.
//	        Перехватывается: клавиши Win и всё, что нажато вместе с ними
//	        (Win+E, Win+D, Win+R…), Alt+Tab, Alt+Esc, Ctrl+Esc (и
//	        Ctrl+Shift+Esc), PrintScreen, Alt+F4, Alt+Space.
//	        НЕ перехватывается и не может быть перехвачено ничем: Ctrl+Alt+Del
//	        (Secure Attention Sequence обрабатывает Winlogon в защищённом
//	        рабочем столе), Win+L (блокировка, тоже Winlogon). Для
//	        Ctrl+Alt+Del приложение даёт пользователю свою команду («Послать
//	        Ctrl+Alt+Del», Ctrl+Alt+End) — так делает и mstsc.
//	X11     XGrabKeyboard на окно, пока у него фокус: оконный менеджер и
//	        глобальные горячие клавиши (XGrabKey) перестают получать клавиши,
//	        всё идёт окну. Отпускается при потере фокуса и при выключении.
//	        Ограничение: экран блокировки/переключение виртуальных терминалов
//	        (Ctrl+Alt+F1…) обрабатывает X-сервер или ядро, их не перехватить.
//	Wayland zwp_keyboard_shortcuts_inhibit_manager_v1.inhibit_shortcuts(
//	        surface, seat): композитор перестаёт обрабатывать свои сочетания,
//	        пока поверхность в фокусе. Решение остаётся за композитором:
//	        многие показывают подтверждение или оставляют аварийное сочетание
//	        (GNOME — долгое удержание Super+Esc), и итог приходит отдельно
//	        (active/inactive). Без глобала в реестре композитора —
//	        SetKeyboardGrab отвечает false.
//	Cocoa   не реализовано (нужен CGEventTap с правом «Мониторинг ввода», а
//	        проверить его негде): всегда false.
//
// Все клавиши идут окну штатным путём — теми же KeyDown/KeyUp, что и
// остальные, поэтому приложение не различает «пришло из захвата» и «пришло
// обычным путём».

import (
	"runtime"
	"sync"
)

// keyboardGrabBackend — бэкенд умеет захватывать системные сочетания.
// Отдельный интерфейс по той же причине, что и fullscreenBackend: сигнатуру
// NativeWindow менять нельзя.
type keyboardGrabBackend interface {
	// SetKeyboardGrab включает или выключает захват. true — запрос принят
	// (захват может включиться позже: на X11 и Wayland — когда окно получит
	// фокус); false — платформа захват не умеет или отказала. Выключение всегда
	// успешно.
	SetKeyboardGrab(on bool) bool
}

// keyboardGrabQuerier — необязательная способность: бэкенд знает, держит ли
// он захват ПРЯМО СЕЙЧАС (на Wayland композитор вправе отказать).
type keyboardGrabQuerier interface {
	KeyboardGrabActive() bool
}

// keyGrabState — то, что окно помнит о захвате само: просьба, заданная до Run
// (бэкенда тогда ещё нет).
type keyGrabState struct {
	mu     sync.Mutex
	pre    bool
	preSet bool
}

// SetKeyboardGrab просит систему отдавать окну клавиши, которые она обычно
// забирает себе — Win, Alt+Tab, Alt+Esc, Ctrl+Esc, PrintScreen (подробнее у
// каждой платформы — в описании файла keygrab.go и ниже), — пока окно в
// фокусе. Нужен удалённым рабочим столам, виртуальным машинам и играм в
// полноэкранном режиме.
//
// Перехваченные клавиши приходят приложению обычными KeyDown и KeyUp. Пока
// окно не в фокусе, система работает как всегда: захват касается только
// активного окна. Выключение (SetKeyboardGrab(false)) и закрытие окна
// отпускают клавиатуру.
//
// Возвращает false, если платформа или композитор захват не поддерживают
// (macOS; Wayland без zwp_keyboard_shortcuts_inhibit_manager_v1). true
// означает «запрос принят»: на Wayland композитор может ещё отказать или
// запросить подтверждение у пользователя, а на X11 захват включится, когда
// окно получит фокус; фактическое состояние — KeyboardGrabbed.
//
// Ctrl+Alt+Del перехватить нельзя нигде (Windows обрабатывает его в
// защищённом рабочем столе), как и Win+L; приложению нужна своя команда.
// Захват с выключенным выходом из него опасен: держите в приложении понятный
// способ его снять (mstsc отпускает клавиатуру по Ctrl+Alt+Home).
//
// До Run запрос запоминается и применяется при создании окна; ответ тогда
// предварительный — «умеет ли платформа вообще». Потокобезопасно.
func (win *Window) SetKeyboardGrab(on bool) bool {
	if win.native == nil {
		win.kg.mu.Lock()
		win.kg.pre, win.kg.preSet = on, true
		win.kg.mu.Unlock()
		return runtime.GOOS != "darwin"
	}
	kb, ok := win.native.(keyboardGrabBackend)
	if !ok {
		return !on // выключить то, чего нет, нетрудно
	}
	return kb.SetKeyboardGrab(on)
}

// KeyboardGrabbed сообщает, действует ли захват клавиатуры прямо сейчас:
// просьба принята, окно в фокусе, и система (композитор) её не отклонила.
// До Run и на бэкендах без захвата — false.
func (win *Window) KeyboardGrabbed() bool {
	if kq, ok := win.native.(keyboardGrabQuerier); ok {
		return kq.KeyboardGrabActive()
	}
	return false
}

// setupKeyboardGrab применяет просьбу, заданную до Run. Зовётся из bringUp
// после Create.
func (win *Window) setupKeyboardGrab() {
	win.kg.mu.Lock()
	on, set := win.kg.pre, win.kg.preSet
	win.kg.preSet = false
	win.kg.mu.Unlock()
	if set && on {
		if kb, ok := win.native.(keyboardGrabBackend); ok {
			kb.SetKeyboardGrab(true)
		}
	}
}

// releaseKeyboardGrab отпускает клавиатуру при закрытии окна: бэкенд вправе
// держать глобальный ресурс (хук на весь рабочий стол), который иначе пережил
// бы окно.
func (win *Window) releaseKeyboardGrab() {
	if kb, ok := win.native.(keyboardGrabBackend); ok {
		kb.SetKeyboardGrab(false)
	}
}

// ─── Фильтр перехвата (общая логика) ─────────────────────────────────────────

// Коды клавиш, которые различает фильтр. Совпадают с VK_* Windows и с
// платформенно-независимыми VK_* пакета; левые/правые Ctrl и Alt — отдельные
// коды, которые видит низкоуровневый хук.
const (
	grabVKLControl = 0xA2
	grabVKRControl = 0xA3
	grabVKLMenu    = 0xA4
	grabVKRMenu    = 0xA5
)

// keyGrabFilter решает, какие клавиши забирает захват. Чистая логика без
// обращений к системе: хук Win32 кормит её событиями, а тесты — списком
// нажатий.
//
// Состояние нужно, потому что решение зависит от истории: пока зажата Win,
// перехватывается ВСЯ клавиатура; отпускание перехваченной клавиши обязано
// быть перехвачено тоже (иначе система увидит отпускание без нажатия);
// автоповтор — это повторное нажатие уже перехваченной клавиши.
type keyGrabFilter struct {
	win      map[int]bool // зажатые клавиши Win, которые мы забрали
	swallow  map[int]bool // перехваченные и ещё не отпущенные клавиши
	ctrlHeld bool
	altHeld  bool
}

// keyGrabAction — что делать с событием.
type keyGrabAction struct {
	// Swallow — не отдавать систему, вернуть из хука 1.
	Swallow bool
	// Deliver — показать клавишу окну (KeyDown/KeyUp). Всегда вместе с Swallow.
	Deliver bool
	// Repeat — нажатие является автоповтором.
	Repeat bool
}

// event разбирает одно событие клавиши. vk — код клавиши Windows, up —
// отпускание, altFlag — признак «Alt зажат» из самого события (LLKHF_ALTDOWN),
// ctrlNow — Ctrl зажат по данным системы (когда хук его не видел), active —
// наше окно активно.
func (f *keyGrabFilter) event(vk int, up, altFlag, ctrlNow, active bool) keyGrabAction {
	if f.swallow == nil {
		f.swallow = map[int]bool{}
		f.win = map[int]bool{}
	}
	// Ctrl и Alt ведём сами: они нужны как модификаторы сочетаний. Сами эти
	// клавиши не перехватываются — окно получает их обычным путём.
	switch vk {
	case grabVKLControl, grabVKRControl, VK_CONTROL:
		f.ctrlHeld = !up
	case grabVKLMenu, grabVKRMenu, VK_ALT:
		f.altHeld = !up
	}

	if up {
		if f.swallow[vk] {
			delete(f.swallow, vk)
			delete(f.win, vk)
			return keyGrabAction{Swallow: true, Deliver: true}
		}
		return keyGrabAction{}
	}
	if f.swallow[vk] {
		// Автоповтор перехваченной клавиши: система его уже не увидит, а
		// окну он нужен как повтор. Даже если фокус только что ушёл: иначе
		// половина удержанной комбинации утекла бы системе.
		return keyGrabAction{Swallow: true, Deliver: active, Repeat: true}
	}
	if !active {
		return keyGrabAction{}
	}

	alt := altFlag || f.altHeld
	ctrl := ctrlNow || f.ctrlHeld
	grab := false
	switch vk {
	case VK_LWIN, VK_RWIN:
		grab = true
	case VK_TAB:
		grab = alt // Alt+Tab
	case VK_ESCAPE:
		grab = alt || ctrl // Alt+Esc, Ctrl+Esc, Ctrl+Shift+Esc
	case VK_SNAPSHOT:
		grab = true // PrintScreen с любыми модификаторами
	case VK_F4:
		grab = alt // Alt+F4
	case VK_SPACE:
		grab = alt // Alt+Space (оконное меню системы)
	}
	if !grab && len(f.win) > 0 {
		grab = true // Win+что угодно: Win+E, Win+D, Win+R, Win+стрелки
	}
	if !grab {
		return keyGrabAction{}
	}
	f.swallow[vk] = true
	if vk == VK_LWIN || vk == VK_RWIN {
		f.win[vk] = true
	}
	return keyGrabAction{Swallow: true, Deliver: true}
}

// reset забывает всё: захват выключили или окно закрыли.
func (f *keyGrabFilter) reset() {
	f.swallow, f.win = nil, nil
	f.ctrlHeld, f.altHeld = false, false
}

// held — клавиши, перехваченные и ещё не отпущенные. При выключении захвата
// окну нужно «отпустить» их самому: система отпускания уже не получит как
// перехваченное, а приложение должно знать, что клавиша больше не зажата.
func (f *keyGrabFilter) held() []int {
	out := make([]int, 0, len(f.swallow))
	for vk := range f.swallow {
		out = append(out, vk)
	}
	return out
}
