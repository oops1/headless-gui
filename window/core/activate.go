package core

// activate.go — вывод окна на передний план (задание PixGo E14).
//
// Просмотрщик живёт одним окном: второй запуск (двойной щелчок по файлу в
// проводнике) передаёт путь первому процессу, и тот обязан выйти из-под других
// окон и из свёрнутого состояния. Тремя платформами это решается по-разному,
// потому что везде система не доверяет приложению, которое «само» просит
// фокус, и требует доказательства, что за этим стоит действие пользователя:
//
//	Win32    окно сворачивается/разворачивается просто (SW_RESTORE), а вот
//	         SetForegroundWindow разрешён только процессу, которому передали
//	         право: тот, кто обработал последний ввод, или тот, кому право
//	         отдал AllowSetForegroundWindow. Отправитель (второй процесс)
//	         зовёт window.AllowForeground(pidОсновного) до передачи файла.
//	         Не вышло — окно мигает в панели задач (FlashWindowEx): честный
//	         запасной вариант, Activate возвращает false.
//	X11      _NET_ACTIVE_WINDOW с источником «приложение» и меткой времени
//	         пользовательского действия; менеджеры окон вправе отказать, если
//	         метка старее последнего ввода. Метку даёт стартовое уведомление
//	         (DESKTOP_STARTUP_ID, часть «_TIME<число>»), без него — время
//	         последнего ввода в самом окне.
//	Wayland  xdg_activation_v1: компоновщик поднимает окно только по токену,
//	         который выдал он сам. Токен приходит от запустившего процесса в
//	         XDG_ACTIVATION_TOKEN (TakeActivationToken достаёт и очищает его),
//	         второй процесс передаёт его первому строкой, а тот зовёт
//	         ActivateWithToken. Без токена Activate просит его у компоновщика
//	         под последний ввод окна (стандартная самоактивация).
//
// Свёрнутость: Win32 знает её точно (IsIconic + событие), X11 читает
// _NET_WM_STATE_HIDDEN и WM_STATE, а у Wayland в xdg_toplevel признака нет —
// там IsMinimized всегда false.

import (
	"os"
	"strings"
	"sync"
)

// activateBackend — бэкенд умеет выводить окно на передний план.
//
// Отдельный интерфейс (а не метод NativeWindow): сигнатуру NativeWindow менять
// нельзя, а бэкенд без способности просто не отвечает на Activate.
type activateBackend interface {
	// Activate выводит окно на передний план (разворачивая свёрнутое).
	// Смысл true зависит от платформы и описан у Window.Activate.
	Activate() bool

	// ActivateWithToken то же, но с токеном активации от запустившего
	// процесса (Wayland: xdg_activation_v1; X11: стартовый идентификатор;
	// Win32: токен не нужен, равно Activate).
	ActivateWithToken(token string) bool
}

// minimizedBackend — бэкенд знает, свёрнуто ли окно.
type minimizedBackend interface {
	// IsMinimized — окно сейчас свёрнуто в панель задач / dock.
	IsMinimized() bool

	// SetOnMinimizedChanged регистрирует уведомление о смене свёрнутости.
	// Вызывается из потока событий бэкенда.
	SetOnMinimizedChanged(fn func(minimized bool))
}

// activateState — то, что окно помнит об активации само: колбэк приложения
// (бэкенда до Run ещё нет, а задать колбэк сразу после New — естественно).
type activateState struct {
	mu          sync.Mutex
	onMinimized func(bool)
}

// Activate выводит окно на передний план: разворачивает его, если оно
// свёрнуто, поднимает над остальными и передаёт ему фокус ввода. Для второго
// запуска «открыть в уже работающем окне».
//
// Возвращаемое значение, как у SetFullscreen, — «запрос принят», но смысл
// зависит от того, что платформа вообще может ответить сразу:
//
//	Win32    true — окно действительно стало активным (SetForegroundWindow
//	         ответил успехом). Свёрнутое окно развёрнуто в любом случае. false —
//	         система отказала (процесс не имеет права на передний план): окно
//	         развёрнуто, но осталось под другими, а его кнопка в панели задач
//	         мигает, пока пользователь не перейдёт к окну. Получить право
//	         можно через AllowForeground(pid) со стороны запустившего процесса.
//	X11      true — _NET_ACTIVE_WINDOW отправлен (с меткой времени последнего
//	         ввода окна; для метки запустившего процесса — ActivateWithToken).
//	         Решение за оконным менеджером: он вправе только подсветить окно.
//	Wayland  true — запрос отправлен: если у окна есть свежий ввод, у
//	         компоновщика запрашивается токен и по его ответу окно
//	         активируется (асинхронно). false — компоновщик не предлагает
//	         xdg_activation_v1, ничего поднять нельзя. Без токена от
//	         запустившего процесса GNOME и KDE чаще всего лишь подсвечивают
//	         окно, а не выводят его.
//
// До Run и на бэкендах без поддержки (macOS) — false.
//
// Потокобезопасно: можно звать из любой горутины, в том числе из обработчика
// сообщения от второго процесса.
func (win *Window) Activate() bool {
	if ab, ok := win.native.(activateBackend); ok {
		return ab.Activate()
	}
	return false
}

// ActivateWithToken то же, что Activate, но с токеном активации запустившего
// процесса — тем, что второй процесс получил из TakeActivationToken и передал
// первому строкой (через Instance.Send или любой другой канал). Пустой токен
// равен Activate().
//
//	Wayland  xdg_activation_v1.activate(token, surface): компоновщик
//	         доверяет токену, который сам выдал запустившему приложению, и
//	         поднимает окно (разворачивая его). Нет глобала — false.
//	X11      в токене ищется метка времени «_TIME<число>» (формат
//	         DESKTOP_STARTUP_ID), с ней уходит _NET_ACTIVE_WINDOW; заодно
//	         окно сообщает launcher'у о конце запуска (_NET_STARTUP_INFO
//	         «remove»), чтобы курсор «занят» не висел до таймаута.
//	Win32    токен не нужен: равно Activate().
//
// Токен одноразовый и чужой: он адресован процессу, которого запустили, и
// после использования недействителен.
func (win *Window) ActivateWithToken(token string) bool {
	if ab, ok := win.native.(activateBackend); ok {
		return ab.ActivateWithToken(token)
	}
	return false
}

// IsMinimized сообщает, свёрнуто ли окно в панель задач / dock.
//
//	Win32    IsIconic — точно.
//	X11      _NET_WM_STATE_HIDDEN либо WM_STATE = Iconic; значение обновляется
//	         по PropertyNotify (запрос между сменой и событием увидит старое).
//	Wayland  всегда false: у xdg_toplevel нет признака свёрнутости (есть
//	         только просьба set_minimized), компоновщик его клиенту не
//	         сообщает. Окно, свёрнутое пользователем, приложению неотличимо
//	         от скрытого.
//
// До Run — false.
func (win *Window) IsMinimized() bool {
	if mb, ok := win.native.(minimizedBackend); ok {
		return mb.IsMinimized()
	}
	return false
}

// SetOnMinimizedChanged регистрирует уведомление о смене свёрнутости окна
// (Win32, X11; Wayland событий не шлёт). Колбэк выполняется на горутине
// движка. Можно вызывать до Run.
func (win *Window) SetOnMinimizedChanged(fn func(minimized bool)) {
	win.act.mu.Lock()
	win.act.onMinimized = fn
	win.act.mu.Unlock()
}

// setupActivate подключает уведомление бэкенда о смене свёрнутости.
// Зовётся из bringUp после Create.
func (win *Window) setupActivate() {
	mb, ok := win.native.(minimizedBackend)
	if !ok {
		return
	}
	mb.SetOnMinimizedChanged(func(minimized bool) {
		// Из потока событий ОС — на горутину движка, как у полного экрана.
		win.post(func() {
			win.act.mu.Lock()
			fn := win.act.onMinimized
			win.act.mu.Unlock()
			if fn != nil {
				fn(minimized)
			}
		})
	})
}

// Имена переменных окружения, в которых запустивший процесс передаёт токен.
const (
	envXDGActivationToken = "XDG_ACTIVATION_TOKEN" // Wayland, xdg-activation-v1
	envDesktopStartupID   = "DESKTOP_STARTUP_ID"   // X11, startup-notification
)

// TakeActivationToken достаёт токен активации, который запустивший процесс
// (рабочий стол, файловый менеджер, другое приложение) положил в окружение
// этого процесса, и стирает его оттуда.
//
// Читается XDG_ACTIVATION_TOKEN (Wayland) и DESKTOP_STARTUP_ID (X11); в
// Wayland-сессии первой берётся первая, в X11-сессии (или при
// HEADLESS_GUI_X11=1) — вторая, а если нужной нет — другая. Обе переменные
// стираются всегда: иначе токен достался бы дочерним процессам, а он
// одноразовый, и чужое приложение «потратило» бы его.
//
// Второй процесс зовёт её в самом начале main, до любых запусков других
// программ, и передаёт строку первому процессу вместе с аргументами; тот
// передаёт её в Window.ActivateWithToken. Нет токена (запуск из терминала) —
// пустая строка. На Windows и macOS токенов нет: всегда пустая строка.
func TakeActivationToken() string {
	return takeActivationToken(os.Getenv, os.Unsetenv, preferXDGActivationToken())
}

// preferXDGActivationToken — какой токен брать первым: сессия Wayland.
func preferXDGActivationToken() bool {
	return os.Getenv("WAYLAND_DISPLAY") != "" && os.Getenv("HEADLESS_GUI_X11") != "1"
}

// takeActivationToken — TakeActivationToken с подставляемыми функциями
// окружения (тестируется без порчи окружения процесса).
func takeActivationToken(getenv func(string) string, unsetenv func(string) error, preferXDG bool) string {
	xdg, startup := getenv(envXDGActivationToken), getenv(envDesktopStartupID)
	_ = unsetenv(envXDGActivationToken)
	_ = unsetenv(envDesktopStartupID)
	first, second := xdg, startup
	if !preferXDG {
		first, second = startup, xdg
	}
	if first != "" {
		return first
	}
	return second
}

// startupIDTime разбирает из идентификатора запуска (DESKTOP_STARTUP_ID,
// startup-notification) метку времени пользовательского действия: идентификатор
// имеет вид "<имя>_TIME<число>", число — время сервера X11 в миллисекундах
// (CurrentTime, 0, не годится: менеджеру нужно настоящее). ok=false — части
// «_TIME» нет, число не разбирается или равно нулю.
func startupIDTime(id string) (t uint32, ok bool) {
	i := strings.LastIndex(id, "_TIME")
	if i < 0 {
		return 0, false
	}
	digits := id[i+len("_TIME"):]
	end := 0
	for end < len(digits) && digits[end] >= '0' && digits[end] <= '9' {
		end++
	}
	if end == 0 || end > 10 {
		return 0, false
	}
	var v uint64
	for _, c := range digits[:end] {
		v = v*10 + uint64(c-'0')
	}
	if v == 0 || v > 0xFFFFFFFF {
		return 0, false
	}
	return uint32(v), true
}
