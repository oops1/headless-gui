package core

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
