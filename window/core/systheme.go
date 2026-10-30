package core

// systheme.go — светлая или тёмная тема операционной системы: узнать сейчас и
// получить уведомление о смене.
//
// Движок сам ничего не красит под тему ОС: какую палитру показывать, решает
// приложение. Но спросить у системы ему было негде, и каждое приложение
// (Проводник WinLine, Блокнот) таскало из проекта в проект собственную копию
// «читаем реестр / портал» — с одними и теми же ошибками в каждой. Здесь
// единственная.
//
// Чтение реестра, портал D-Bus и сообщение окна WM_SETTINGCHANGE — службы
// оболочки, возможность platform/shell (ADR-0013 §3); здесь — подписка окна,
// отсев повторов и доставка на горутину движка. Без возможности
// (window/minimal) тема неизвестна, акцента нет, наблюдения нет.

import (
	"image/color"
	"sync"

	"github.com/oops1/headless-gui/v3/platform/native"
)

// detectSystemTheme, detectSystemAccent, watchSystemTheme и watchSystemAccent
// — службы оболочки (native.ShellServices); без них — Unknown, ok=false, nil.

func detectSystemTheme() native.SystemTheme {
	if s, ok := native.ShellServices(); ok && s.Theme != nil {
		return s.Theme()
	}
	return native.SystemThemeUnknown
}

func detectSystemAccent() (color.RGBA, bool) {
	if s, ok := native.ShellServices(); ok && s.Accent != nil {
		return s.Accent()
	}
	return color.RGBA{}, false
}

func watchSystemTheme(nw native.NativeWindow, emit func(native.SystemTheme)) (stop func()) {
	if s, ok := native.ShellServices(); ok && s.WatchTheme != nil {
		return s.WatchTheme(nw, emit)
	}
	return nil
}

func watchSystemAccent(nw native.NativeWindow, emit func(color.RGBA, bool)) (stop func()) {
	if s, ok := native.ShellServices(); ok && s.WatchAccent != nil {
		return s.WatchAccent(nw, emit)
	}
	return nil
}

// DetectSystemTheme возвращает тему, выбранную в ОС прямо сейчас.
//
// Windows: параметр приложений AppsUseLightTheme в реестре пользователя (не
// SystemUsesLightTheme: тот про панель задач и меню «Пуск», а окна программ
// живут по первому). Linux: переменная GTK_THEME, затем портал рабочего
// стола org.freedesktop.portal.Settings по D-Bus. macOS и window/minimal (без
// служб оболочки): SystemThemeUnknown.
//
// Безопасно звать из любой горутины и до создания окна — именно так выбирают
// палитру для первого кадра. На Linux первый вызов может занять до нескольких
// секунд, если шина есть, а портал не отвечает; без шины — мгновенно.
func DetectSystemTheme() native.SystemTheme {
	return detectSystemTheme()
}

// SystemAppearance — оформление, выбранное в ОС: тема и акцентный цвет.
type SystemAppearance struct {
	// Theme — светлая или тёмная тема приложений (см. DetectSystemTheme).
	Theme native.SystemTheme
	// Accent — акцентный цвет, выбранный пользователем (непрозрачный,
	// A == 255). Осмыслен только при HasAccent.
	Accent color.RGBA
	// HasAccent — акцентный цвет удалось узнать. На Linux его нет у сред без
	// портала или с порталом старше 1.18, а также когда пользователь акцент не
	// выбирал; на macOS и в window/minimal — всегда нет.
	HasAccent bool
}

// DetectSystemAccent возвращает акцентный цвет, выбранный в ОС прямо сейчас.
// ok=false — узнать не удалось (см. SystemAppearance.HasAccent).
//
// Windows: DWM\AccentColor в реестре пользователя (при его отсутствии —
// DWM\ColorizationColor). Linux: ключ accent-color пространства
// org.freedesktop.appearance портала рабочего стола. macOS и window/minimal:
// ok=false.
//
// Безопасно звать из любой горутины и до создания окна; на Linux первый вызов
// может занять до нескольких секунд, если шина есть, а портал не отвечает.
func DetectSystemAccent() (c color.RGBA, ok bool) {
	return detectSystemAccent()
}

// DetectSystemAppearance возвращает тему и акцентный цвет одним вызовом.
func DetectSystemAppearance() SystemAppearance {
	a := SystemAppearance{Theme: detectSystemTheme()}
	a.Accent, a.HasAccent = detectSystemAccent()
	return a
}

// SetOnSystemThemeChanged подписывает приложение на смену темы ОС: человек
// переключил тёмную тему в параметрах, пока программа открыта.
//
// Колбэк выполняется на горутине движка, как обработчики виджетов, так что
// из него можно сразу менять палитру и перерисовывать дерево. Его зовут только
// при настоящей смене: Windows присылает несколько сообщений на одно
// переключение, портал — сигнал и при смене акцентного цвета, и ни то ни
// другое тему не меняет. Тема «неизвестна» смены не считается — приложение
// остаётся при той, что показывало.
//
// Можно вызывать до Run (подписка начнётся при создании окна) и во время
// работы; nil снимает подписку.
//
// Поддержано на Windows (WM_SETTINGCHANGE окна) и Linux (сигнал
// SettingChanged портала) службами оболочки (platform/shell). На macOS и в
// window/minimal наблюдения нет — как и чтения темы. Дополнительные окна,
// открытые через OpenWindow, подписку получают свою: колбэк задаётся каждому
// окну отдельно.
func (win *Window) SetOnSystemThemeChanged(fn func(native.SystemTheme)) {
	win.sysTheme.mu.Lock()
	win.sysTheme.fn = fn
	running := win.native != nil
	win.sysTheme.mu.Unlock()
	// До Run окна ОС ещё нет: подписку поднимет bringUp.
	if running {
		win.syncSystemThemeWatch()
	}
}

// SetOnSystemAppearanceChanged подписывает приложение на смену оформления ОС:
// светлой или тёмной темы И акцентного цвета. Нужен приложению, которое
// следует за системой целиком («тема из системы»), а не только за режимом.
//
// Колбэк получает оформление целиком (SystemAppearance) и вызывается лишь при
// настоящей перемене любого из его полей: Windows присылает несколько
// сообщений на одно переключение, портал — сигнал на каждый ключ
// пространства appearance. Тема «неизвестна» и акцент «неизвестен» не считаются
// сменой: окно остаётся при том, что показывало (последнее известное значение
// копится между событиями).
//
// Колбэк выполняется на горутине движка. Можно вызывать до Run и во время
// работы; nil снимает подписку. Подписка независима от
// SetOnSystemThemeChanged: можно задать любой из колбэков или оба, платформенное
// наблюдение при этом одно.
//
//	Windows  WM_SETTINGCHANGE "ImmersiveColorSet" (тема и акцент приходят
//	         одной группой), значения перечитываются из реестра;
//	Linux    сигнал SettingChanged портала для color-scheme и accent-color
//	         (X11 и Wayland одинаково);
//	macOS    наблюдения нет;
//	window/minimal (без служб оболочки) — тоже.
func (win *Window) SetOnSystemAppearanceChanged(fn func(SystemAppearance)) {
	win.sysTheme.mu.Lock()
	win.sysTheme.app = fn
	running := win.native != nil
	win.sysTheme.mu.Unlock()
	if running {
		win.syncSystemThemeWatch()
	}
}

// systemThemeState — подписка окна на смену темы и оформления.
type systemThemeState struct {
	mu   sync.Mutex
	fn   func(native.SystemTheme)
	stop func() // снятие платформенной подписки; nil — её нет
	// closed — окна ОС для подписки нет: оно закрыто (tearDown) или не
	// создалось. Подписку тогда не поднимаем, а поднятую в гонке с закрытием
	// снимаем сразу: снять её потом было бы некому, и платформенный
	// обработчик держал бы окно с движком до конца процесса. Снимает флаг
	// bringUp.
	closed bool
	edge   themeEdge

	// app — колбэк оформления (тема + акцент); cur — последнее известное
	// оформление, last — то, о чём приложение уже уведомлено.
	app  func(SystemAppearance)
	cur  SystemAppearance
	last SystemAppearance
}

// syncSystemThemeWatch приводит платформенную подписку в соответствие с тем,
// задан колбэк или нет.
//
// Подписка ленивая: на Linux она означает правило на общей шине и разбор
// каждого чужого сигнала Settings, и платить за это приложению, которому тема
// не нужна, незачем.
func (win *Window) syncSystemThemeWatch() {
	st := &win.sysTheme
	st.mu.Lock()
	hasFn, watching, closed := st.fn != nil || st.app != nil, st.stop != nil, st.closed
	st.mu.Unlock()

	switch {
	case hasFn && !watching && !closed:
		// Исходное значение читаем ДО подписки и вне замка: на Linux это
		// вызов по шине. Без него первое же повторное сообщение с прежней
		// темой сошло бы за смену.
		cur := DetectSystemAppearance()
		stopTheme := watchSystemTheme(win.native, win.systemThemeEvent)
		stopAccent := watchSystemAccent(win.native, win.systemAccentEvent)
		st.mu.Lock()
		if st.stop != nil || st.closed || (st.fn == nil && st.app == nil) {
			// Пока читали, подписку успели поднять или снять, а окно —
			// закрыть: эта не нужна.
			st.mu.Unlock()
			if stopTheme != nil {
				stopTheme()
			}
			if stopAccent != nil {
				stopAccent()
			}
			return
		}
		st.edge.reset(cur.Theme)
		st.cur, st.last = cur, cur
		stop := func() {
			if stopTheme != nil {
				stopTheme()
			}
			if stopAccent != nil {
				stopAccent()
			}
		}
		st.stop = stop // платформа наблюдать не умеет — обе nil: больше не пытаемся
		st.mu.Unlock()
	case !hasFn && watching:
		win.stopSystemThemeWatch()
	}
}

// stopSystemThemeWatch снимает подписку, когда колбэки сняты.
func (win *Window) stopSystemThemeWatch() {
	st := &win.sysTheme
	st.mu.Lock()
	stop := st.stop
	st.stop = nil
	st.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// closeSystemThemeWatch снимает подписку при закрытии окна (tearDown) или
// неудаче его создания и до следующего bringUp новую не допускает:
// платформенный обработчик (на Linux — общий на процесс, на Windows — в
// карте окон процесса) иначе пережил бы окно и держал движок, которого уже
// нет.
func (win *Window) closeSystemThemeWatch() {
	st := &win.sysTheme
	st.mu.Lock()
	st.closed = true
	stop := st.stop
	st.stop = nil
	st.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// openSystemThemeWatch снова разрешает подписку: bringUp создаёт окно ОС.
func (win *Window) openSystemThemeWatch() {
	win.sysTheme.mu.Lock()
	win.sysTheme.closed = false
	win.sysTheme.mu.Unlock()
}

// systemThemeEvent принимает от платформы «тема, возможно, сменилась» — из
// потока окна ОС или из читающей горутины D-Bus — и решает, стоит ли будить
// приложение.
func (win *Window) systemThemeEvent(t native.SystemTheme) {
	st := &win.sysTheme
	st.mu.Lock()
	fn := st.fn
	changed := st.edge.update(t)
	fire := fn != nil && changed
	if t != native.SystemThemeUnknown {
		st.cur.Theme = t
	}
	app, appVal, fireApp := st.appearanceChangeLocked()
	st.mu.Unlock()
	// Колбэки приложения красят виджеты, то есть работают на горутине
	// движка; поток окна ОС и читающая горутина D-Bus ждать их не должны.
	if fire {
		win.post(func() { fn(t) })
	}
	if fireApp {
		win.post(func() { app(appVal) })
	}
}

// systemAccentEvent принимает от платформы новый акцентный цвет. ok=false —
// акцент узнать не удалось: он не считается сменой и последнее известное
// значение не сбрасывает.
func (win *Window) systemAccentEvent(c color.RGBA, ok bool) {
	st := &win.sysTheme
	st.mu.Lock()
	if ok {
		st.cur.Accent, st.cur.HasAccent = c, true
	}
	app, appVal, fireApp := st.appearanceChangeLocked()
	st.mu.Unlock()
	if fireApp {
		win.post(func() { app(appVal) })
	}
}

// appearanceChangeLocked сверяет текущее оформление с тем, о чём приложение
// уже знает. Замок держит вызывающий.
func (st *systemThemeState) appearanceChangeLocked() (fn func(SystemAppearance), a SystemAppearance, fire bool) {
	if st.app == nil || st.cur == st.last {
		return nil, SystemAppearance{}, false
	}
	st.last = st.cur
	return st.app, st.cur, true
}

// themeEdge отсеивает повторы: платформы сообщают «что-то с темой», а не
// «тема стала такой-то».
//
// Windows на одно переключение шлёт WM_SETTINGCHANGE несколько раз подряд
// (по разу на каждую группу параметров цвета), а портал на Linux — на любой
// ключ пространства appearance, включая акцентный цвет. Приложение, которое
// на каждый такой толчок перекрашивает всё дерево, мигает и тратит кадры
// впустую.
type themeEdge struct {
	last native.SystemTheme
}

// reset запоминает тему, от которой считаются смены.
func (e *themeEdge) reset(t native.SystemTheme) { e.last = t }

// update сообщает, считать ли t сменой темы. «Неизвестно» не считается: если
// портал на миг не ответил, окно не должно прыгать на тему по умолчанию, а
// потом обратно.
func (e *themeEdge) update(t native.SystemTheme) bool {
	if t == native.SystemThemeUnknown || t == e.last {
		return false
	}
	e.last = t
	return true
}
