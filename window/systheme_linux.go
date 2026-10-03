//go:build linux && !android

package window

// systheme_linux.go — тема ОС на Linux: GTK_THEME и портал рабочего стола по
// D-Bus.
//
// В Linux нет «системной темы»: есть настройки GNOME (gsettings), KDE
// (kdeglobals), xfconf — у каждой среды свои. Единый ответ дают только
// порталы: org.freedesktop.portal.Settings отдаёт ключ color-scheme в
// пространстве org.freedesktop.appearance, и его реализуют портал GNOME, KDE,
// wlroots и прочие. Раньше приложения спрашивали gsettings подпроцессом, но
// внешних утилит в движке нет, а на KDE gsettings о теме не знает вовсе.
//
// Что НЕ покрыто: среда без портала (голый i3 или openbox, tty-сессия,
// контейнер) и очень старые порталы. Там остаётся Unknown, и приложение
// выбирает тему само — гадать по имени темы GTK из ~/.config/gtk-3.0 мы не
// берёмся: имя «Adwaita» ничего не говорит о том, тёмная она или светлая, а
// парсер ini-файла в движке ради этого заводить незачем.

import (
	"os"
	"strings"
	"sync"
	"time"
)

// portalCallTimeout — сколько ждём портал. Меньше общего таймаута шины:
// DetectSystemTheme зовут перед первым кадром, и окно не должно ждать пять
// секунд из-за портала, который завис. Отсутствие портала шина сообщает сразу
// (ServiceUnknown), так что на нормальной машине время не тратится.
const portalCallTimeout = 3 * time.Second

// detectSystemTheme: сначала переменная окружения, потом портал.
//
// Порядок такой, потому что GTK_THEME ставят нарочно — чтобы запустить
// программу в чужой теме поверх настроек сессии (GTK_THEME=Adwaita:dark
// myapp), и перебить это порталом значило бы ослушаться человека.
func detectSystemTheme() SystemTheme {
	if t := systemThemeFromGTKTheme(os.Getenv("GTK_THEME")); t != SystemThemeUnknown {
		return t
	}
	c, err := dbusSession()
	if err != nil {
		return SystemThemeUnknown
	}
	return readPortalColorScheme(c)
}

// readPortalColorScheme спрашивает у портала color-scheme.
//
// Сначала ReadOne: он появился в версии 2 портала и отдаёт значение без
// лишней обёртки. Старый Read есть везде, но объявлен устаревшим и в будущих
// версиях может исчезнуть; на него переходим только если ReadOne не знают
// (UnknownMethod), а на любую другую ошибку — нет портала, нет ключа, таймаут —
// сразу отвечаем Unknown: второй вызов тем же путём не дал бы иного, а
// таймаут удвоился бы.
func readPortalColorScheme(c *dbusConn) SystemTheme {
	args := []any{portalAppearanceNS, portalColorSchemeKey}
	reply, err := c.callTimeout(portalBusName, portalObjPath, portalSettingsIface,
		"ReadOne", "ss", args, portalCallTimeout)
	if err != nil && strings.Contains(err.Error(), "UnknownMethod") {
		reply, err = c.callTimeout(portalBusName, portalObjPath, portalSettingsIface,
			"Read", "ss", args, portalCallTimeout)
	}
	if err != nil || reply == nil || len(reply.Body) == 0 {
		return SystemThemeUnknown
	}
	return systemThemeFromPortalValue(reply.Body[0])
}

// portalThemeHub — единственная подписка процесса на сигнал портала с
// раздачей всем окнам.
//
// Соединение с сессионной шиной одно на процесс (dbusSession), а снять
// обработчик сигнала с него нельзя: у onSignal нет обратной операции.
// Подпиши каждое окно напрямую — закрытое окно оставило бы на шине
// обработчик, который до конца работы программы звал бы колбэк в мёртвый
// движок. Поэтому на шине висит один обработчик, а окна приходят и уходят в
// списке подписчиков.
type portalThemeHub struct {
	attachMu sync.Mutex // сериализует подключение к шине; не берётся в обработчике
	attached bool

	mu   sync.Mutex
	subs map[int]func(SystemTheme)
	next int
}

var portalThemes portalThemeHub

// watchSystemTheme подписывается на сигнал SettingChanged портала.
//
// Это дёшево: один AddMatch на процесс, дальше сигналы приходят по уже
// открытой шине, а проверка «наш ли ключ» — сравнение двух строк. Поэтому
// наблюдение на Linux есть, в отличие от macOS. Без шины или портала подписка
// ничего не даёт, и возвращается nil: это не ошибка, тема просто не
// меняется на глазах.
func watchSystemTheme(native NativeWindow, emit func(SystemTheme)) (stop func()) {
	c, err := dbusSession()
	if err != nil {
		return nil
	}
	if !portalThemes.attach(c) {
		return nil
	}
	id := portalThemes.add(emit)
	return func() { portalThemes.remove(id) }
}

// attach единожды вешает на шину правило и обработчик.
func (h *portalThemeHub) attach(c *dbusConn) bool {
	h.attachMu.Lock()
	defer h.attachMu.Unlock()
	if h.attached {
		return true
	}
	// Правило с sender и arg0: шина сама отсеет сигналы чужих сервисов и
	// чужих пространств настроек, и читающая горутина не разбирает их зря.
	// AddMatch — вызов с ожиданием ответа, который доставляет та же читающая
	// горутина, поэтому под замком обработчика (h.mu) он не делается: обработчик
	// ждал бы этот замок, а ответ — обработчик.
	rule := "type='signal',sender='" + portalBusName + "',interface='" + portalSettingsIface +
		"',member='SettingChanged',path='" + portalObjPath + "',arg0='" + portalAppearanceNS + "'"
	if err := c.addMatch(rule); err != nil {
		return false
	}
	c.onSignal(func(msg *dbusMessage) {
		if t, ok := systemThemeFromPortalSignal(msg); ok {
			h.dispatch(t)
		}
	})
	h.attached = true
	return true
}

func (h *portalThemeHub) add(fn func(SystemTheme)) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs == nil {
		h.subs = map[int]func(SystemTheme){}
	}
	h.next++
	h.subs[h.next] = fn
	return h.next
}

func (h *portalThemeHub) remove(id int) {
	h.mu.Lock()
	delete(h.subs, id)
	h.mu.Unlock()
}

// dispatch раздаёт тему подписчикам из читающей горутины D-Bus.
//
// Список копируется и звать подписчиков приходится вне замка: подписчик может
// снять подписку (закрыть окно) прямо из колбэка, и под замком это было бы
// взаимной блокировкой.
func (h *portalThemeHub) dispatch(t SystemTheme) {
	h.mu.Lock()
	fns := make([]func(SystemTheme), 0, len(h.subs))
	for _, fn := range h.subs {
		fns = append(fns, fn)
	}
	h.mu.Unlock()
	for _, fn := range fns {
		fn(t)
	}
}
