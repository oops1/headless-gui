//go:build linux && !android

package shell

import (
	"os"
	"strings"
	"testing"

	"github.com/oops1/headless-gui/v3/platform/dbus"
)

// dialTestBus — копия помощника из platform/dbus (dbus_conn_linux_test.go):
// тестовые помощники между пакетами не делятся.

// dialTestBus подключается к сессионной шине; без неё тест пропускается
// (CI без графической сессии, контейнер и т.п.).
func dialTestBus(t *testing.T) *dbus.Conn {
	t.Helper()
	addr := dbus.SessionAddress()
	sock, err := dbus.ParseAddress(addr)
	if err != nil {
		t.Skipf("нет адреса сессионной шины: %v", err)
	}
	if !strings.HasPrefix(sock, "@") {
		if _, err := os.Stat(sock); err != nil {
			t.Skipf("нет сокета шины %s", sock)
		}
	}
	c, err := dbus.Dial(addr)
	if err != nil {
		t.Skipf("шина недоступна: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}
