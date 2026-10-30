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

	"github.com/oops1/headless-gui/v3/platform/dbus"
	"github.com/oops1/headless-gui/v3/widget/base"
)

const (
	notifyBusName   = "org.freedesktop.Notifications"
	notifyObjPath   = "/org/freedesktop/Notifications"
	notifyInterface = "org.freedesktop.Notifications"
)

// errNoNotifyDaemon — на шине нет демона уведомлений (голый WM, tty-сессия).
var errNoNotifyDaemon = errors.New("window: на сессионной шине нет демона уведомлений (org.freedesktop.Notifications)")

// Notifier — состояние уведомлений одного окна. Встраивается в бэкенды
// X11/Wayland: методы ShowBalloon/SetBalloonClickHandler становятся их
// методами, и окно видит их через интерфейс balloonHost (window/core/tray.go).
type Notifier struct {
	mu         sync.Mutex
	onClick    func()
	ours       map[uint32]bool // id показанных НАМИ уведомлений
	subscribed bool            // сигналы уже слушаем
	caps       map[string]bool // GetCapabilities, читается один раз
}

// SetBalloonClickHandler регистрирует колбэк клика по уведомлению.
func (n *Notifier) SetBalloonClickHandler(fn func()) {
	n.mu.Lock()
	n.onClick = fn
	n.mu.Unlock()
}

// ShowBalloon показывает уведомление. infoFlag — severity в кодировке window/core/tray.go
// (0 none, 1 info, 2 warning, 3 error): выбирает значок и «срочность».
func (n *Notifier) ShowBalloon(title, text string, infoFlag uint32) error {
	c, err := dbus.Session()
	if err != nil {
		return fmt.Errorf("window: уведомления недоступны: %w", err)
	}
	n.ensureSubscribed(c)

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
			n.mu.Lock()
			if n.ours == nil {
				n.ours = map[uint32]bool{}
			}
			n.ours[id] = true
			n.mu.Unlock()
		}
	}
	return nil
}

// ensureSubscribed единожды подписывается на сигналы демона: ActionInvoked
// (щелчок по уведомлению/кнопке) и NotificationClosed (снято/просрочено).
func (n *Notifier) ensureSubscribed(c *dbus.Conn) {
	n.mu.Lock()
	if n.subscribed {
		n.mu.Unlock()
		return
	}
	n.subscribed = true
	n.mu.Unlock()

	if err := c.AddMatch("type='signal',interface='" + notifyInterface + "'"); err != nil {
		return
	}
	c.OnSignal(func(msg *dbus.Message) {
		if msg.Interface != notifyInterface || len(msg.Body) == 0 {
			return
		}
		id, _ := msg.Body[0].(uint32)
		switch msg.Member {
		case "ActionInvoked":
			n.mu.Lock()
			mine := n.ours[id]
			fn := n.onClick
			n.mu.Unlock()
			if mine && fn != nil {
				// Колбэк — в отдельной горутине: обработчики сигналов
				// выполняются в цикле чтения сокета, и любой вызов D-Bus
				// изнутри (или долгая работа UI) заблокировал бы приём.
				go fn()
			}
		case "NotificationClosed":
			n.mu.Lock()
			delete(n.ours, id)
			n.mu.Unlock()
		}
	})
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
