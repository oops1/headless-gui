//go:build linux && !android

package window

import (
	"os"
	"strings"
	"testing"
	"time"
)

// dialTestBus подключается к сессионной шине; без неё тест пропускается
// (CI без графической сессии, контейнер и т.п.).
func dialTestBus(t *testing.T) *dbusConn {
	t.Helper()
	addr := dbusSessionAddress()
	sock, err := dbusParseAddress(addr)
	if err != nil {
		t.Skipf("нет адреса сессионной шины: %v", err)
	}
	if !strings.HasPrefix(sock, "@") {
		if _, err := os.Stat(sock); err != nil {
			t.Skipf("нет сокета шины %s", sock)
		}
	}
	c, err := dbusDial(addr)
	if err != nil {
		t.Skipf("шина недоступна: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

// TestDBusParseAddress — разбор адреса шины (путь, абстрактный сокет, %XX).
func TestDBusParseAddressUnit(t *testing.T) {
	cases := []struct{ addr, want string }{
		{"unix:path=/run/user/1000/bus", "/run/user/1000/bus"},
		{"unix:abstract=/tmp/dbus-AbC,guid=deadbeef", "@/tmp/dbus-AbC"},
		{"unix:path=/tmp/a%20b", "/tmp/a b"},
		{"tcp:host=127.0.0.1;unix:path=/run/bus", "/run/bus"},
	}
	for _, c := range cases {
		got, err := dbusParseAddress(c.addr)
		if err != nil {
			t.Fatalf("%q: %v", c.addr, err)
		}
		if got != c.want {
			t.Errorf("%q → %q, want %q", c.addr, got, c.want)
		}
	}
	if _, err := dbusParseAddress("tcp:host=127.0.0.1,port=1"); err == nil {
		t.Error("адрес без unix-сокета должен быть ошибкой")
	}
}

// TestDBusLiveHelloAndCalls — живое рукопожатие с настоящей шиной и вызовы
// org.freedesktop.DBus. Проверяет весь путь: SASL, Hello, маршалинг заголовка
// (демон строго валидирует его), разбор ответа.
func TestDBusLiveHelloAndCalls(t *testing.T) {
	c := dialTestBus(t)
	if !strings.HasPrefix(c.unique, ":") {
		t.Fatalf("уникальное имя %q не похоже на :1.N", c.unique)
	}

	reply, err := c.call("org.freedesktop.DBus", "/org/freedesktop/DBus",
		"org.freedesktop.DBus", "ListNames", "", nil)
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}
	names, ok := reply.Body[0].([]string)
	if !ok {
		t.Fatalf("ListNames вернул %T", reply.Body[0])
	}
	var sawBus, sawSelf bool
	for _, n := range names {
		sawBus = sawBus || n == "org.freedesktop.DBus"
		sawSelf = sawSelf || n == c.unique
	}
	if !sawBus || !sawSelf {
		t.Errorf("в списке имён нет шины (%v) или нас самих (%v)", sawBus, sawSelf)
	}

	if _, err := c.call("org.freedesktop.DBus", "/org/freedesktop/DBus",
		"org.freedesktop.DBus", "GetId", "", nil); err != nil {
		t.Errorf("GetId: %v", err)
	}

	// Ответ-ошибка приходит как error, а не как повисший вызов.
	if _, err := c.call("org.freedesktop.DBus", "/org/freedesktop/DBus",
		"org.freedesktop.DBus", "NoSuchMethod", "", nil); err == nil {
		t.Error("ожидалась ошибка UnknownMethod")
	} else if !strings.Contains(err.Error(), "UnknownMethod") {
		t.Errorf("ошибка не про UnknownMethod: %v", err)
	}
}

// TestDBusLiveExportAndSignal — экспорт объекта и сигналы: одно соединение
// владеет именем и отвечает на вызовы, второе — вызывает и слушает сигнал.
func TestDBusLiveExportAndSignal(t *testing.T) {
	server := dialTestBus(t)
	client := dialTestBus(t)

	const busName = "org.headlessgui.DBusSelfTest"
	const objPath = "/org/headlessgui/DBusSelfTest"
	const iface = "org.headlessgui.DBusSelfTest"

	server.setCallHandler(func(msg *dbusMessage) *dbusReply {
		if msg.Path != objPath || msg.Interface != iface {
			return nil
		}
		switch msg.Member {
		case "Echo":
			s, _ := msg.Body[0].(string)
			return &dbusReply{Sig: "s", Body: []any{"эхо: " + s}}
		case "Complex":
			return &dbusReply{
				Sig: "(so)a{sv}",
				Body: []any{
					dbusStruct{Fields: []any{busName, dbusObjectPath(objPath)}},
					map[string]dbusVariant{"n": {Sig: "i", Val: int32(-7)}},
				},
			}
		case "Boom":
			return &dbusReply{ErrName: "org.headlessgui.Error.Boom", ErrMsg: "так задумано"}
		}
		return nil
	})
	if err := server.requestName(busName, 0x4); err != nil {
		t.Fatalf("RequestName: %v", err)
	}

	reply, err := client.call(busName, objPath, iface, "Echo", "s", []any{"привет"})
	if err != nil {
		t.Fatalf("Echo: %v", err)
	}
	if got, _ := reply.Body[0].(string); got != "эхо: привет" {
		t.Errorf("Echo вернул %q", got)
	}

	reply, err = client.call(busName, objPath, iface, "Complex", "", nil)
	if err != nil {
		t.Fatalf("Complex: %v", err)
	}
	st, ok := reply.Body[0].(dbusStruct)
	if !ok || st.Fields[1] != dbusObjectPath(objPath) {
		t.Errorf("Complex[0] = %#v", reply.Body[0])
	}
	m, ok := reply.Body[1].(map[string]any)
	if !ok {
		t.Fatalf("Complex[1] = %#v", reply.Body[1])
	}
	if v, _ := m["n"].(dbusVariant); v.Val != int32(-7) {
		t.Errorf("Complex[1][n] = %#v", m["n"])
	}

	if _, err := client.call(busName, objPath, iface, "Boom", "", nil); err == nil ||
		!strings.Contains(err.Error(), "так задумано") {
		t.Errorf("ошибка обработчика не доехала: %v", err)
	}
	if _, err := client.call(busName, objPath, iface, "NoSuchMember", "", nil); err == nil {
		t.Error("неизвестный метод должен вернуть ошибку")
	}

	// Сигнал: клиент подписывается, сервер испускает.
	got := make(chan string, 1)
	client.onSignal(func(msg *dbusMessage) {
		if msg.Interface == iface && msg.Member == "Ping" && len(msg.Body) > 0 {
			s, _ := msg.Body[0].(string)
			select {
			case got <- s:
			default:
			}
		}
	})
	if err := client.addMatch("type='signal',interface='" + iface + "'"); err != nil {
		t.Fatalf("AddMatch: %v", err)
	}
	if err := server.emit(objPath, iface, "Ping", "s", []any{"понг"}); err != nil {
		t.Fatalf("emit: %v", err)
	}
	select {
	case s := <-got:
		if s != "понг" {
			t.Errorf("сигнал принёс %q", s)
		}
	case <-time.After(3 * time.Second):
		t.Error("сигнал не доставлен за 3 с")
	}

	if !client.nameHasOwner(busName) {
		t.Error("NameHasOwner не видит занятое имя")
	}
	if client.nameHasOwner("org.headlessgui.NobodyHome") {
		t.Error("NameHasOwner нашёл несуществующее имя")
	}
}
