//go:build linux && !android

package window

import (
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/widget"
)

// TestNotifyUrgencyAndIcon — маппинг severity в значок и «срочность».
func TestNotifyUrgencyAndIcon(t *testing.T) {
	cases := []struct {
		flag    uint32
		icon    string
		urgency byte
	}{
		{0, "", 0},
		{1, "dialog-information", 1},
		{2, "dialog-warning", 1},
		{3, "dialog-error", 2},
	}
	for _, c := range cases {
		if got := notifyIcon(c.flag); got != c.icon {
			t.Errorf("notifyIcon(%d) = %q, want %q", c.flag, got, c.icon)
		}
		if got := notifyUrgency(c.flag); got != c.urgency {
			t.Errorf("notifyUrgency(%d) = %d, want %d", c.flag, got, c.urgency)
		}
	}
	if notifyAppName() == "" {
		t.Error("notifyAppName пустое")
	}
}

// TestNotifyAgainstRealDaemon — если в системе работает НАСТОЯЩИЙ демон
// уведомлений, шлём ему уведомление по-настоящему: чужая реализация строго
// проверяет типы аргументов, чего наш собственный разбор проверить не может.
// Без демона (CI, tty-сессия) — пропуск.
func TestNotifyAgainstRealDaemon(t *testing.T) {
	c := dialTestBus(t)
	if !c.nameHasOwner(notifyBusName) {
		t.Skip("демон уведомлений не запущен")
	}
	n := &linuxNotifier{}
	n.setBalloonClickHandler(func() {})
	if err := n.showBalloon("headless-gui", "Проверка уведомлений", 1); err != nil {
		t.Fatalf("showBalloon: %v", err)
	}
	// Возможности демона читаются и кэшируются без ошибок.
	if _, err := dbusSession(); err != nil {
		t.Fatalf("сессионная шина: %v", err)
	}
}

// TestNotifyAgainstFakeDaemon — сквозная проверка уведомлений: тест сам
// становится демоном org.freedesktop.Notifications на настоящей шине,
// принимает наш Notify и присылает обратно ActionInvoked.
//
// Если демон в системе уже есть (нормальный десктоп), имя занято — тест
// пропускается: подменять живой демон нельзя.
func TestNotifyAgainstFakeDaemon(t *testing.T) {
	daemon := dialTestBus(t)

	type notifyCall struct {
		app, icon, summary, body string
		actions                  []string
		hints                    map[string]any
		timeout                  int32
	}
	calls := make(chan notifyCall, 4)

	daemon.setCallHandler(func(msg *dbusMessage) *dbusReply {
		if msg.Path != notifyObjPath || msg.Interface != notifyInterface {
			return nil
		}
		switch msg.Member {
		case "GetCapabilities":
			return &dbusReply{Sig: "as", Body: []any{[]string{"actions", "body"}}}
		case "Notify":
			if len(msg.Body) != 8 {
				return &dbusReply{ErrName: "org.freedesktop.DBus.Error.InvalidArgs", ErrMsg: "8 аргументов"}
			}
			c := notifyCall{}
			c.app, _ = msg.Body[0].(string)
			c.icon, _ = msg.Body[2].(string)
			c.summary, _ = msg.Body[3].(string)
			c.body, _ = msg.Body[4].(string)
			c.actions, _ = msg.Body[5].([]string)
			c.hints, _ = msg.Body[6].(map[string]any)
			c.timeout, _ = msg.Body[7].(int32)
			calls <- c
			return &dbusReply{Sig: "u", Body: []any{uint32(4242)}}
		case "CloseNotification":
			return &dbusReply{}
		}
		return nil
	})
	if err := daemon.requestName(notifyBusName, 0x4 /*DO_NOT_QUEUE*/); err != nil {
		t.Skipf("демон уведомлений уже запущен, подменять не будем: %v", err)
	}

	clicked := make(chan struct{}, 1)
	n := &linuxNotifier{}
	n.setBalloonClickHandler(func() { clicked <- struct{}{} })

	if err := n.showBalloon("Заголовок", "Тело уведомления", 3); err != nil {
		t.Fatalf("showBalloon: %v", err)
	}

	var got notifyCall
	select {
	case got = <-calls:
	case <-time.After(3 * time.Second):
		t.Fatal("демон не получил Notify")
	}
	if got.summary != "Заголовок" || got.body != "Тело уведомления" {
		t.Errorf("текст: %q / %q", got.summary, got.body)
	}
	if got.icon != "dialog-error" {
		t.Errorf("значок %q, want dialog-error", got.icon)
	}
	if got.timeout != -1 {
		t.Errorf("expire_timeout = %d, want -1", got.timeout)
	}
	if len(got.actions) != 2 || got.actions[0] != "default" || got.actions[1] != widget.Tr("dlg.open") {
		t.Errorf("actions = %v", got.actions)
	}
	if v, ok := got.hints["urgency"].(dbusVariant); !ok || v.Val != byte(2) {
		t.Errorf("urgency = %#v", got.hints["urgency"])
	}
	if v, ok := got.hints["desktop-entry"].(dbusVariant); !ok || v.Val != notifyAppName() {
		t.Errorf("desktop-entry = %#v", got.hints["desktop-entry"])
	}

	// Чужое уведомление не должно дёргать наш колбэк.
	if err := daemon.emit(notifyObjPath, notifyInterface, "ActionInvoked", "us", []any{uint32(1), "default"}); err != nil {
		t.Fatalf("emit чужого: %v", err)
	}
	// Наше — должно.
	if err := daemon.emit(notifyObjPath, notifyInterface, "ActionInvoked", "us", []any{uint32(4242), "default"}); err != nil {
		t.Fatalf("emit нашего: %v", err)
	}
	select {
	case <-clicked:
	case <-time.After(3 * time.Second):
		t.Fatal("клик по уведомлению не доехал до колбэка")
	}
	select {
	case <-clicked:
		t.Error("колбэк сработал на чужое уведомление")
	default:
	}

	// После NotificationClosed id забывается и повторный ActionInvoked молчит.
	if err := daemon.emit(notifyObjPath, notifyInterface, "NotificationClosed", "uu", []any{uint32(4242), uint32(2)}); err != nil {
		t.Fatalf("emit closed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := daemon.emit(notifyObjPath, notifyInterface, "ActionInvoked", "us", []any{uint32(4242), "default"}); err != nil {
		t.Fatalf("emit после закрытия: %v", err)
	}
	select {
	case <-clicked:
		t.Error("колбэк сработал после NotificationClosed")
	case <-time.After(500 * time.Millisecond):
	}

	// Без колбэка действия не регистрируем — демон не покажет лишнюю кнопку.
	n2 := &linuxNotifier{}
	if err := n2.showBalloon("Без клика", "текст", 1); err != nil {
		t.Fatalf("showBalloon без колбэка: %v", err)
	}
	select {
	case got = <-calls:
		if len(got.actions) != 0 {
			t.Errorf("actions без колбэка = %v", got.actions)
		}
		if got.icon != "dialog-information" {
			t.Errorf("значок %q, want dialog-information", got.icon)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("второй Notify не доехал")
	}
}
