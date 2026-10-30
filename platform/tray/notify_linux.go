//go:build linux && !android

// notify_linux.go — системные уведомления на Linux через D-Bus:
// org.freedesktop.Notifications (ровно тот путь, которым ходит notify-send).
//
// Публичный API общий с Windows: Window.ShowBalloon / SetOnBalloonClick.
// Отличие платформы: на Linux иконка в трее НЕ нужна — уведомление показывает
// демон рабочего стола сам. Клик по уведомлению доезжает до колбэка, если демон
// объявляет возможность "actions" (мы регистрируем действие "default", которое
// по спецификации соответствует щелчку по телу уведомления).

package tray

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/oops1/headless-gui/v4/platform/dbus"
	"github.com/oops1/headless-gui/v4/platform/native"
	"github.com/oops1/headless-gui/v4/widget/base"
)

const (
	notifyBusName   = "org.freedesktop.Notifications"
	notifyObjPath   = "/org/freedesktop/Notifications"
	notifyInterface = "org.freedesktop.Notifications"
)

// errNoNotifyDaemon — на шине нет демона уведомлений (голый WM, tty-сессия).
var errNoNotifyDaemon = errors.New("window: на сессионной шине нет демона уведомлений (org.freedesktop.Notifications)")

// Notifier — состояние уведомлений одного окна; вместе с SNI — трей окна на
// Linux (linuxTray, register_linux.go).
type Notifier struct {
	mu      sync.Mutex
	onClick func()
	caps    map[string]bool // GetCapabilities, читается один раз
	closed  bool            // окно закрыто (Close)
}

// notifyHub — подписка процесса на сигналы демона уведомлений: одно правило
// AddMatch и один обработчик на общем соединении dbus.Session, сколько бы окон
// ни показывало уведомления; id уведомления → его Notifier. У dbus.Conn снятия
// обработчика нет: подписка на окно жила бы до конца процесса и держала бы
// окно с движком.
var notifyHub struct {
	// subMu — подписка. Не mu: AddMatch ждёт ответа из цикла чтения
	// соединения, а обработчик сигналов в этом цикле берёт mu.
	subMu      sync.Mutex
	subscribed bool

	mu   sync.Mutex
	ours map[uint32]*Notifier // id показанных НАМИ уведомлений → чьё
}

// SetBalloonClickHandler регистрирует колбэк клика по уведомлению.
func (n *Notifier) SetBalloonClickHandler(fn func()) {
	n.mu.Lock()
	if !n.closed {
		n.onClick = fn
	}
	n.mu.Unlock()
}

// Close снимает колбэк и уведомления окна из хаба: клик по уведомлению после
// закрытия окна никуда не идёт. После Close ShowBalloon отвечает
// ErrTrayUnsupported.
func (n *Notifier) Close() {
	n.mu.Lock()
	n.closed = true
	n.onClick = nil
	n.mu.Unlock()
	n.forget()
}

// forget убирает из хаба все уведомления окна.
func (n *Notifier) forget() {
	notifyHub.mu.Lock()
	for id, m := range notifyHub.ours {
		if m == n {
			delete(notifyHub.ours, id)
		}
	}
	notifyHub.mu.Unlock()
}

// isClosed — окно закрыто.
func (n *Notifier) isClosed() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.closed
}

// ShowBalloon показывает уведомление. infoFlag — severity в кодировке window/core/tray.go
// (0 none, 1 info, 2 warning, 3 error): выбирает значок и «срочность».
func (n *Notifier) ShowBalloon(title, text string, infoFlag uint32) error {
	if n.isClosed() {
		return native.ErrTrayUnsupported
	}
	c, err := dbus.Session()
	if err != nil {
		return fmt.Errorf("window: уведомления недоступны: %w", err)
	}
	ensureNotifySubscribed(c)

	var actions []string
	n.mu.Lock()
	wantClick := n.onClick != nil
	n.mu.Unlock()
	if wantClick && n.capability(c, "actions") {
		// "default" — действие по щелчку на теле уведомления (spec 1.2).
		actions = []string{"default", base.Tr("dlg.open")}
	}

	hints := map[string]dbus.Variant{
		"urgency":       {Sig: "y", Val: notifyUrgency(infoFlag)},
		"desktop-entry": {Sig: "s", Val: notifyAppName()},
	}
	reply, err := c.Call(notifyBusName, notifyObjPath, notifyInterface, "Notify",
		"susssasa{sv}i", []any{
			notifyAppName(),      // app_name
			uint32(0),            // replaces_id: 0 — новое уведомление
			notifyIcon(infoFlag), // app_icon: имя значка по freedesktop
			title,                // summary
			text,                 // body
			actions,              // actions
			hints,                // hints
			int32(-1),            // expire_timeout: -1 — по усмотрению демона
		})
	if err != nil {
		if strings.Contains(err.Error(), "ServiceUnknown") || strings.Contains(err.Error(), "NameHasNoOwner") {
			return errNoNotifyDaemon
		}
		return fmt.Errorf("window: Notify: %w", err)
	}
	if len(reply.Body) > 0 {
		if id, ok := reply.Body[0].(uint32); ok {
			notifyHub.mu.Lock()
			if notifyHub.ours == nil {
				notifyHub.ours = map[uint32]*Notifier{}
			}
			notifyHub.ours[id] = n
			notifyHub.mu.Unlock()
			if n.isClosed() {
				n.forget() // окно закрылось, пока демон отвечал
			}
		}
	}
	return nil
}

// ensureNotifySubscribed единожды на процесс подписывается на сигналы демона:
// ActionInvoked (щелчок по уведомлению/кнопке) и NotificationClosed
// (снято/просрочено). Неудача AddMatch повторяется при следующем уведомлении.
func ensureNotifySubscribed(c *dbus.Conn) {
	notifyHub.subMu.Lock()
	defer notifyHub.subMu.Unlock()
	if notifyHub.subscribed {
		return
	}
	if err := c.AddMatch("type='signal',interface='" + notifyInterface + "'"); err != nil {
		return
	}
	notifyHub.subscribed = true
	c.OnSignal(notifySignal)
}

// notifySignal — обработчик сигналов демона (цикл чтения соединения).
func notifySignal(msg *dbus.Message) {
	if msg.Interface != notifyInterface || len(msg.Body) == 0 {
		return
	}
	id, _ := msg.Body[0].(uint32)
	switch msg.Member {
	case "ActionInvoked":
		notifyHub.mu.Lock()
		n := notifyHub.ours[id]
		notifyHub.mu.Unlock()
		if n == nil {
			return
		}
		n.mu.Lock()
		fn := n.onClick
		n.mu.Unlock()
		if fn != nil {
			// Колбэк — в отдельной горутине: обработчики сигналов
			// выполняются в цикле чтения сокета, и любой вызов D-Bus
			// изнутри (или долгая работа UI) заблокировал бы приём.
			go fn()
		}
	case "NotificationClosed":
		notifyHub.mu.Lock()
		delete(notifyHub.ours, id)
		notifyHub.mu.Unlock()
	}
}

// capability сообщает, объявляет ли демон указанную возможность
// ("actions", "body-markup", …). Список читается один раз и кэшируется.
func (n *Notifier) capability(c *dbus.Conn, name string) bool {
	n.mu.Lock()
	if n.caps != nil {
		ok := n.caps[name]
		n.mu.Unlock()
		return ok
	}
	n.mu.Unlock()

	caps := map[string]bool{}
	reply, err := c.Call(notifyBusName, notifyObjPath, notifyInterface, "GetCapabilities", "", nil)
	if err == nil && len(reply.Body) > 0 {
		if list, ok := reply.Body[0].([]string); ok {
			for _, s := range list {
				caps[s] = true
			}
		}
	}
	n.mu.Lock()
	n.caps = caps
	n.mu.Unlock()
	return caps[name]
}

// notifyUrgency переводит severity в hint "urgency" (0 low, 1 normal, 2 critical).
func notifyUrgency(infoFlag uint32) byte {
	switch infoFlag {
	case 3: // error
		return 2
	case 0: // none
		return 0
	}
	return 1
}

// notifyIcon переводит severity в имя значка freedesktop icon naming spec.
func notifyIcon(infoFlag uint32) string {
	switch infoFlag {
	case 1:
		return "dialog-information"
	case 2:
		return "dialog-warning"
	case 3:
		return "dialog-error"
	}
	return ""
}

// notifyAppName — имя приложения в уведомлении: имя исполняемого файла.
func notifyAppName() string {
	if len(os.Args) > 0 {
		if base := filepath.Base(os.Args[0]); base != "" && base != "." && base != "/" {
			return strings.TrimSuffix(base, ".test")
		}
	}
	return "headless-gui"
}
