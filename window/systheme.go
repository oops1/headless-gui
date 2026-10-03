package window

// systheme.go — светлая или тёмная тема операционной системы: узнать сейчас и
// получить уведомление о смене.
//
// Движок сам ничего не красит под тему ОС: какую палитру показывать, решает
// приложение. Но спросить у системы ему было негде, и каждое приложение
// (Проводник WinLine, Блокнот) таскало из проекта в проект собственную копию
// «читаем реестр / портал» — с одними и теми же ошибками в каждой. Здесь
// единственная.
//
// Файл без платформенного суффикса намеренно, как xftdpi.go: чтение реестра,
// D-Bus и сообщения окна живут в systheme_<ОС>.go, а разбор того, что они
// прочитали, проверяется на любой машине и идёт в общем прогоне тестов.

import (
	"strings"
	"sync"
)

// SystemTheme — тема оформления, выбранная в ОС.
type SystemTheme int

const (
	// SystemThemeUnknown — узнать не удалось: ОС без понятия темы (Windows до
	// 10 1607), на Linux нет ни GTK_THEME, ни портала рабочего стола, macOS
	// (см. systheme_darwin.go). Это не «светлая»: приложение вправе взять
	// свою тему по умолчанию, и именно об этом ему и сообщается — подставить
	// светлую молча значило бы показать белое окно человеку, у которого
	// просто не работает портал.
	SystemThemeUnknown SystemTheme = iota
	// SystemThemeLight — светлая тема.
	SystemThemeLight
	// SystemThemeDark — тёмная тема.
	SystemThemeDark
)

// String — имя темы для журналов и сообщений об ошибках.
func (t SystemTheme) String() string {
	switch t {
	case SystemThemeLight:
		return "light"
	case SystemThemeDark:
		return "dark"
	}
	return "unknown"
}

// DetectSystemTheme возвращает тему, выбранную в ОС прямо сейчас.
//
// Windows: параметр приложений AppsUseLightTheme в реестре пользователя (не
// SystemUsesLightTheme: тот про панель задач и меню «Пуск», а окна программ
// живут по первому). Linux: переменная GTK_THEME, затем портал рабочего
// стола org.freedesktop.portal.Settings по D-Bus. macOS: SystemThemeUnknown.
//
// Безопасно звать из любой горутины и до создания окна — именно так выбирают
// палитру для первого кадра. На Linux первый вызов может занять до нескольких
// секунд, если шина есть, а портал не отвечает; без шины — мгновенно.
func DetectSystemTheme() SystemTheme {
	return detectSystemTheme()
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
// SettingChanged портала). На macOS наблюдения нет — как и чтения темы, см.
// systheme_darwin.go. Дополнительные окна, открытые через OpenWindow,
// подписку получают свою: колбэк задаётся каждому окну отдельно.
func (win *Window) SetOnSystemThemeChanged(fn func(SystemTheme)) {
	win.sysTheme.mu.Lock()
	win.sysTheme.fn = fn
	running := win.native != nil
	win.sysTheme.mu.Unlock()
	// До Run окна ОС ещё нет: подписку поднимет bringUp.
	if running {
		win.syncSystemThemeWatch()
	}
}

// systemThemeState — подписка окна на смену темы.
type systemThemeState struct {
	mu   sync.Mutex
	fn   func(SystemTheme)
	stop func() // снятие платформенной подписки; nil — её нет
	edge themeEdge
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
	hasFn, watching := st.fn != nil, st.stop != nil
	st.mu.Unlock()

	switch {
	case hasFn && !watching:
		// Исходное значение читаем ДО подписки и вне замка: на Linux это
		// вызов по шине. Без него первое же повторное сообщение с прежней
		// темой сошло бы за смену.
		cur := detectSystemTheme()
		stop := watchSystemTheme(win.native, win.systemThemeEvent)
		st.mu.Lock()
		if st.stop != nil || st.fn == nil {
			// Пока читали, подписку успели поднять или снять — вторая
			// не нужна.
			st.mu.Unlock()
			if stop != nil {
				stop()
			}
			return
		}
		st.edge.reset(cur)
		if stop == nil {
			stop = func() {} // платформа наблюдать не умеет: больше не пытаемся
		}
		st.stop = stop
		st.mu.Unlock()
	case !hasFn && watching:
		win.stopSystemThemeWatch()
	}
}

// stopSystemThemeWatch снимает подписку. Зовётся при закрытии окна:
// платформенный обработчик (на Linux — общий на процесс) иначе пережил бы окно
// и слал колбэки в движок, которого уже нет.
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

// systemThemeEvent принимает от платформы «тема, возможно, сменилась» — из
// потока окна ОС или из читающей горутины D-Bus — и решает, стоит ли будить
// приложение.
func (win *Window) systemThemeEvent(t SystemTheme) {
	st := &win.sysTheme
	st.mu.Lock()
	fn := st.fn
	fire := fn != nil && st.edge.update(t)
	st.mu.Unlock()
	if !fire {
		return
	}
	// Колбэк приложения красит виджеты, то есть работает на горутине
	// движка; поток окна ОС и читающая горутина D-Bus ждать его не должны.
	win.post(func() { fn(t) })
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
	last SystemTheme
}

// reset запоминает тему, от которой считаются смены.
func (e *themeEdge) reset(t SystemTheme) { e.last = t }

// update сообщает, считать ли t сменой темы. «Неизвестно» не считается: если
// портал на миг не ответил, окно не должно прыгать на тему по умолчанию, а
// потом обратно.
func (e *themeEdge) update(t SystemTheme) bool {
	if t == SystemThemeUnknown || t == e.last {
		return false
	}
	e.last = t
	return true
}

// systemThemeFromAppsUseLight переводит значение AppsUseLightTheme из реестра
// Windows в тему: 0 — тёмная, 1 — светлая.
//
// Любое другое число — Unknown, а не «светлая»: значение не документировано
// шире нуля и единицы, и угадывать за систему, что значит двойка, значит
// рисковать белым окном в тёмной теме.
func systemThemeFromAppsUseLight(v uint64) SystemTheme {
	switch v {
	case 0:
		return SystemThemeDark
	case 1:
		return SystemThemeLight
	}
	return SystemThemeUnknown
}

// isImmersiveColorSet — это сообщение WM_SETTINGCHANGE про смену цветовой
// схемы?
//
// Сообщение приходит по любому поводу (смена раскладки, переменных среды,
// рабочей области), и различает их только строка в lParam. Нужная нам строка —
// ImmersiveColorSet: так Windows называет группу параметров, куда входят и
// тема приложений, и акцентный цвет.
func isImmersiveColorSet(s string) bool {
	return s == "ImmersiveColorSet"
}

// Портал настроек рабочего стола (XDG Desktop Portal) и его настройка темы.
// Имена лежат здесь, а не в systheme_linux.go: по ним же разбирается
// пришедший сигнал, а разбор проверяется на любой машине.
const (
	portalBusName        = "org.freedesktop.portal.Desktop"
	portalObjPath        = "/org/freedesktop/portal/desktop"
	portalSettingsIface  = "org.freedesktop.portal.Settings"
	portalAppearanceNS   = "org.freedesktop.appearance"
	portalColorSchemeKey = "color-scheme"
)

// Значения ключа color-scheme.
const (
	portalSchemeNoPreference = 0 // человек ничего не выбирал
	portalSchemeDark         = 1
	portalSchemeLight        = 2
)

// systemThemeFromPortalSignal разбирает сигнал SettingChanged(ssv) портала.
//
// Сигнал приходит на любую настройку любого пространства — акцентный цвет,
// контраст, свои ключи GNOME, — и шина доставляет их все, раз правило
// подписки одно на пространство. ok=false — сигнал не про цветовую схему, и
// будить приложение он не должен.
func systemThemeFromPortalSignal(msg *dbusMessage) (t SystemTheme, ok bool) {
	if msg == nil || msg.Interface != portalSettingsIface || msg.Member != "SettingChanged" || len(msg.Body) < 3 {
		return SystemThemeUnknown, false
	}
	ns, _ := msg.Body[0].(string)
	key, _ := msg.Body[1].(string)
	if ns != portalAppearanceNS || key != portalColorSchemeKey {
		return SystemThemeUnknown, false
	}
	return systemThemeFromPortalValue(msg.Body[2]), true
}

// systemThemeFromPortalValue разбирает значение настройки, как оно пришло по
// D-Bus.
//
// Читать значение можно двумя методами портала, и обёрнуто оно по-разному:
// ReadOne (версия 2) отдаёт один variant, а старый Read — variant внутри
// variant, о чём в документации сказано одной строкой, а на деле ломает
// разбор вида «uint32 или ошибка». Поэтому variant снимается циклом, сколько
// бы их ни было. То же значение приходит и в сигнале SettingChanged.
//
// 0 («нет предпочтения») — Unknown: человек ничего не выбирал, и тему решает
// приложение.
func systemThemeFromPortalValue(v any) SystemTheme {
	for {
		vr, ok := v.(dbusVariant)
		if !ok {
			break
		}
		v = vr.Val
	}
	if v == nil {
		return SystemThemeUnknown
	}
	n, err := dbusToUint(v)
	if err != nil {
		return SystemThemeUnknown
	}
	switch n {
	case portalSchemeDark:
		return SystemThemeDark
	case portalSchemeLight:
		return SystemThemeLight
	}
	return SystemThemeUnknown
}

// systemThemeFromGTKTheme разбирает переменную GTK_THEME.
//
// Её задают, когда нужно запустить программу в чужой теме поверх настроек
// рабочего стола, — а значит, она важнее портала: GTK-приложение в такой
// сессии поступает так же. Формат «Имя» или «Имя:вариант», где вариант —
// dark или light (Adwaita:dark).
//
// Имя без слова dark и без варианта — Unknown, а не «светлая»: у стороннего
// набора (Nordic, Dracula) тёмная тема и без этой подсказки в названии, и по
// одному имени нельзя сказать наверняка. Лучше отдать решение порталу, чем
// угадать неверно.
func systemThemeFromGTKTheme(s string) SystemTheme {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return SystemThemeUnknown
	}
	name, variant, _ := strings.Cut(s, ":")
	switch strings.TrimSpace(variant) {
	case "dark":
		return SystemThemeDark
	case "light":
		return SystemThemeLight
	}
	if strings.Contains(name, "dark") {
		return SystemThemeDark
	}
	return SystemThemeUnknown
}
