package window

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

// fullscreenState — то, что окно помнит о полном экране само: колбэк
// приложения и запрос, заданный до Run (бэкенда тогда ещё нет).
type fullscreenState struct {
	mu       sync.Mutex
	onChange func(bool)
	pre      bool // запрошенное до Run состояние
	preSet   bool
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
		win.fs.pre, win.fs.preSet = on, true
		win.fs.mu.Unlock()
		return runtime.GOOS != "darwin"
	}
	fb, ok := win.native.(fullscreenBackend)
	if !ok {
		return false
	}
	return fb.SetFullscreen(on)
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
	on, set := win.fs.pre, win.fs.preSet
	win.fs.preSet = false
	win.fs.mu.Unlock()
	if set {
		fb.SetFullscreen(on)
	}
}
