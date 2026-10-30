package core

import (
	"github.com/oops1/headless-gui/v4/platform/native"

	"github.com/oops1/headless-gui/v4/platform/keyboard"
)

// activation.go — активность окна наружу и отпускание зажатых клавиш (v3.39).
//
// Окно, потерявшее фокус, больше не видит клавиатуру: Alt+Tab, клик в другое
// приложение, системный диалог. Отпускание зажатой клавиши уходит в чужое окно,
// и движок остаётся с «нажатым» Ctrl: следующий щелчок мыши — уже Ctrl+Click, а
// клиент удалённого рабочего стола держит Ctrl нажатым на сервере. Поэтому при
// деактивации окно само отпускает всё, что считало зажатым, — теми же событиями
// клавиш, что и настоящее отпускание (Pressed == false, со скан-кодом), — и
// сбрасывает модификаторы движка.
//
// Откуда берётся деактивация: Win32 WM_ACTIVATE (переход к своему же окну —
// попапу, диалогу — не считается), X11 FocusIn/FocusOut, Wayland — состояние
// activated в xdg_toplevel.configure.

// noteHeld ведёт список зажатых клавиш: нажатие добавляет, отпускание убирает.
// Автоповтор нового пункта не заводит.
func (s *surface) noteHeld(k keyboard.RawKey) {
	s.heldMu.Lock()
	defer s.heldMu.Unlock()
	idx := -1
	for i, h := range s.held {
		if sameKey(h, k) {
			idx = i
			break
		}
	}
	switch {
	case k.Pressed && idx < 0:
		s.held = append(s.held, k)
	case !k.Pressed && idx >= 0:
		s.held = append(s.held[:idx], s.held[idx+1:]...)
	}
}

// sameKey — одна и та же физическая клавиша: по скан-коду, а если его нет —
// по виртуальному коду.
func sameKey(a, b keyboard.RawKey) bool {
	if a.Scan != 0 || b.Scan != 0 {
		return a.Scan == b.Scan && a.Extended == b.Extended
	}
	return a.VK == b.VK
}

// releaseHeldKeys отпускает в движке всё, что считалось зажатым, и сбрасывает
// модификаторы. Обычные клавиши отпускаются раньше модификаторов, так что
// каждое событие несёт тот же Mod, с каким клавишу нажимали.
//
// Зовётся потоком бэкенда при потере окном активности (setupActivation). Жест
// «Alt нажали и отпустили» при этом не срабатывает: меню в окне, которое уже
// потеряло фокус, открывать некому.
func (s *surface) releaseHeldKeys() {
	s.heldMu.Lock()
	keys := s.held
	s.held = nil
	s.heldMu.Unlock()

	s.altTap.Store(false)
	for pass := 0; pass < 2; pass++ {
		for _, k := range keys {
			if isHeldModifier(k.VK) != (pass == 1) {
				continue
			}
			k.Pressed, k.Repeat = false, false
			s.rawKeyEvent(k)
		}
	}
	// Состояние могло остаться и без записей о клавишах (бэкенд без
	// скан-кодов тоже ведёт их, но Alt+Tab мог проглотить нажатие целиком).
	s.modShift.Store(false)
	s.modCtrl.Store(false)
	s.modAlt.Store(false)
	s.modMeta.Store(false)
	s.altTap.Store(false)
	s.post(func() { s.sendModifiers(0) })
}

// isHeldModifier — клавиша-модификатор (их отпускают после остальных).
func isHeldModifier(vk int) bool {
	switch vk {
	case native.VK_SHIFT, native.VK_CONTROL, native.VK_ALT, native.VK_LWIN, native.VK_RWIN:
		return true
	}
	return false
}

// ─── Window.SetOnActivate ───────────────────────────────────────────────────

// SetOnActivate регистрирует колбэк смены активности окна: true — окно получило
// фокус, false — потеряло (Alt+Tab, клик в другое приложение).
//
// Колбэк вызывается на горутине движка (там же, где обработчики виджетов) и
// только при смене состояния, а не на каждое сообщение платформы. К моменту
// вызова при потере активности уже отпущены все зажатые клавиши (событиями
// KeyEvent{Pressed: false}) и сброшены модификаторы движка, снято наведение
// popup-оверлеев и отменены касания: приложению остаётся отпустить своё —
// захват мыши, перетаскивание, и, например, отправить удалённому серверу
// синхронизацию состояния клавиатуры.
//
// Первый вызов приходит, когда платформа впервые сообщила состояние (на
// X11 и Wayland — при получении окном первого фокуса); начального значения
// до этого у окна нет, IsActive до него возвращает false. nil отключает колбэк.
// Бэкенд, не умеющий сообщать активность (macOS), колбэк не вызывает.
//
// Вызывать до Run().
func (win *Window) SetOnActivate(fn func(active bool)) {
	if fn == nil {
		win.onActivate.Store(nil)
		return
	}
	win.onActivate.Store(&fn)
}

// IsActive сообщает, активно ли окно (последнее состояние, о котором
// сообщила платформа).
func (win *Window) IsActive() bool { return win.activeNow.Load() == activeYes }

const (
	activeUnknown int32 = iota
	activeYes
	activeNo
)

// notifyActivation вызывает колбэк приложения при смене состояния. Горутина
// движка.
func (win *Window) notifyActivation(active bool) {
	next := activeNo
	if active {
		next = activeYes
	}
	if win.activeNow.Swap(next) == next {
		return
	}
	if fn := win.onActivate.Load(); fn != nil {
		(*fn)(active)
	}
}
