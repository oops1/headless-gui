package core

// fullscreen.go — полноэкранный режим окна (задание PixGo E1).
//
// «Развёрнуто» и «полный экран» — разные состояния: развёрнутое окно лежит в
// рабочей области и оставляет таскбар, полноэкранное закрывает монитор целиком.
// Раньше API не было ни на одном бэкенде, а Wayland считал полноэкранное окно
// развёрнутым (IsMaximized). Теперь у окна три независимых понятия:
//
//	IsMaximized()  — окно развёрнуто (значение сохраняется на время полного экрана);
//	IsFullscreen() — окно закрывает весь монитор;
//	SetOnFullscreenChanged — уведомление о смене (в том числе по воле
//	                 системы: Esc/F11 у компоновщика, оконный менеджер).
//
// Бэкенды: Win32 — SetWindowPos на прямоугольник монитора без WS_THICKFRAME,
// X11 — _NET_WM_STATE_FULLSCREEN, Wayland — xdg_toplevel.set_fullscreen.
// Cocoa пока не умеет (SetFullscreen возвращает false).

import (
	"runtime"
	"sync"

	"github.com/oops1/headless-gui/v4/platform/native"
)

// fullscreenBackend — бэкенд умеет переключать окно в полный экран.
//
// Отдельный интерфейс, а не метод NativeWindow: сигнатуру NativeWindow менять
// нельзя (её реализуют сторонние бэкенды и тестовые подделки), а бэкенд без
// способности просто не отвечает на SetFullscreen.
type fullscreenBackend interface {
	// SetFullscreen просит перейти в полный экран (on) или вернуться из него.
	// true — запрос принят к исполнению (не значит «уже сделано»: Wayland и
	// X11 отвечают асинхронно, а компоновщик вправе отказать); false —
	// бэкенд или оконная система этого не умеют.
	SetFullscreen(on bool) bool

	// IsFullscreen — окно сейчас на весь монитор, как его видит система.
	IsFullscreen() bool

	// SetOnFullscreenChanged регистрирует уведомление о смене состояния. Для
	// запроса, который компоновщик отклонил, приходит on=false, хотя
	// состояние не менялось: приложению пора переходить в запасной режим.
	// Вызывается из потока событий бэкенда.
	SetOnFullscreenChanged(fn func(on bool))
}

// fullscreenOnBackend — бэкенд умеет встать в полный экран на указанном
// мониторе (SetFullscreenOn). Отдельный интерфейс по той же причине, что и
// fullscreenBackend.
type fullscreenOnBackend interface {
	// SetFullscreenOn просит полный экран на мониторе m. Если окно уже в
	// полном экране — переносит его на m. false — монитор не найден среди
	// текущих (отключён) или оконная система выбор монитора не поддерживает.
	SetFullscreenOn(m native.Monitor) bool
}

// fullscreenState — то, что окно помнит о полном экране само: колбэк
// приложения и запрос, заданный до Run (бэкенда тогда ещё нет).
type fullscreenState struct {
	mu       sync.Mutex
	onChange func(bool)
	pre      bool // запрошенное до Run состояние
	preSet   bool
	preMon   *native.Monitor // монитор запроса до Run (SetFullscreenOn); nil — любой
}

// SetFullscreen переводит окно в полный экран (on) или возвращает обратно.
//
// Возвращает false, если бэкенд (или оконная система) полный экран не умеет:
// приложению стоит развернуть окно обычным путём. true означает «запрос
// отправлен»: на Wayland и X11 ответ приходит позже, а компоновщик вправе
// отказать (WinLine принимает set_fullscreen только при WINLINE_FULLSCREEN).
// Итог всегда виден через IsFullscreen и SetOnFullscreenChanged: при отказе
// колбэк получит false.
//
// До Run запрос запоминается и применяется при создании окна; ответ тогда
// предварительный — «умеет ли платформа вообще».
//
// Потокобезопасно: можно звать из обработчиков виджетов (горутина движка).
func (win *Window) SetFullscreen(on bool) bool {
	if win.native == nil {
		win.fs.mu.Lock()
		win.fs.pre, win.fs.preSet, win.fs.preMon = on, true, nil
		win.fs.mu.Unlock()
		return runtime.GOOS != "darwin"
	}
	fb, ok := win.native.(fullscreenBackend)
	if !ok {
		return false
	}
	return fb.SetFullscreen(on)
}

// SetFullscreenOn переводит окно в полный экран на выбранном мониторе m
// (значение из Monitors, MonitorAt или MonitorOf). Выйти из него можно тем же
// SetFullscreen(false); уведомления и итог — те же, что у SetFullscreen
// (IsFullscreen, SetOnFullscreenChanged).
//
// Окно, которое уже в полном экране, переносится на m: на Win32 и Wayland без
// выхода из режима, на X11 — через _NET_WM_FULLSCREEN_MONITORS, а если
// менеджер его не объявил, то выходом, переносом и новым входом (приложение
// получит false и сразу true).
//
//	Win32   окно встаёт на rcMonitor выбранного монитора;
//	X11     окно переносится на монитор, затем _NET_WM_STATE_FULLSCREEN и,
//	        если менеджер объявил, _NET_WM_FULLSCREEN_MONITORS;
//	Wayland xdg_toplevel.set_fullscreen(wl_output) — композитор вправе
//	        поступить по-своему (выбрать другой вывод), итог виден из
//	        IsFullscreen и MonitorOf.
//
// false — запрос не принят: монитор уже отключён (m не нашёлся среди текущих
// ни по ключу, ни по имени, ни по границам), нулевой Monitor{}, или бэкенд
// выбора монитора не умеет (macOS). До Run запрос запоминается, и ответ
// предварительный — «умеет ли платформа вообще»; если монитор к тому времени
// исчез, окно встанет в обычный полный экран.
func (win *Window) SetFullscreenOn(m native.Monitor) bool {
	if native.MonitorKey(m) == 0 && m.Name == "" && m.Bounds.Empty() {
		return false
	}
	if win.native == nil {
		win.fs.mu.Lock()
		win.fs.pre, win.fs.preSet = true, true
		mc := m
		win.fs.preMon = &mc
		win.fs.mu.Unlock()
		return runtime.GOOS != "darwin"
	}
	fb, ok := win.native.(fullscreenOnBackend)
	if !ok {
		return false
	}
	return fb.SetFullscreenOn(m)
}

// IsFullscreen сообщает, закрывает ли окно сейчас весь монитор. До Run и на
// бэкендах без полного экрана — false.
func (win *Window) IsFullscreen() bool {
	if fb, ok := win.native.(fullscreenBackend); ok {
		return fb.IsFullscreen()
	}
	return false
}

// SetOnFullscreenChanged регистрирует уведомление о смене полноэкранного
// состояния. Колбэк выполняется на горутине движка (из него можно менять
// виджеты) и приходит и при ваших вызовах SetFullscreen, и при смене по воле
// системы: пользователь нажал Esc у компоновщика, оконный менеджер вернул окно.
// Если компоновщик отказал в запросе, приходит false.
//
// Можно вызывать до Run.
func (win *Window) SetOnFullscreenChanged(fn func(on bool)) {
	win.fs.mu.Lock()
	win.fs.onChange = fn
	win.fs.mu.Unlock()
}

// setupFullscreen подключает уведомления бэкенда и применяет запрос, заданный
// до Run. Зовётся из bringUp после Create.
func (win *Window) setupFullscreen() {
	fb, ok := win.native.(fullscreenBackend)
	if !ok {
		return
	}
	fb.SetOnFullscreenChanged(func(on bool) {
		// Из потока событий ОС — на горутину движка: колбэк приложения
		// вправе менять виджеты.
		win.post(func() {
			win.fs.mu.Lock()
			fn := win.fs.onChange
			win.fs.mu.Unlock()
			if fn != nil {
				fn(on)
			}
		})
	})
	win.fs.mu.Lock()
	on, set, mon := win.fs.pre, win.fs.preSet, win.fs.preMon
	win.fs.preSet, win.fs.preMon = false, nil
	win.fs.mu.Unlock()
	if set {
		if mon != nil && on {
			// Монитор мог исчезнуть к запуску окна — тогда обычный полный экран.
			if fo, ok := win.native.(fullscreenOnBackend); ok && fo.SetFullscreenOn(*mon) {
				return
			}
		}
		fb.SetFullscreen(on)
	}
}
