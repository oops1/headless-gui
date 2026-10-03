//go:build linux && !android

// openurl_linux.go — открытие ссылок и файлов на Linux. Основной путь —
// портал xdg-desktop-portal (org.freedesktop.portal.OpenURI) через собственный
// клиент D-Bus (dbus_conn_linux.go): так работают и обычная сессия, и
// песочницы, а внешних утилит не нужно. Показ файла — FileManager1.ShowItems.
// xdg-open — запасной путь, когда портала на шине нет, подробности в
// openurl_portal.go.
package window

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// openCallTimeout — сколько ждём ответ портала или файлового менеджера.
// Больше обычных 5 секунд: к ещё не запущенному менеджеру шина обращается
// через активацию, и первый вызов ждёт старта Nautilus/Dolphin целиком.
// С короткой паузой ShowItems «падал по таймауту», хотя окно секундой позже
// всё равно появлялось.
const openCallTimeout = 10 * time.Second

// linuxOpenBackend собирает рабочие зависимости. Собирается на каждый вызов,
// а не один раз: тесты подменяют openStartCmd уже после загрузки пакета.
func linuxOpenBackend() openBackend {
	return openBackend{
		call: func(dest, path, iface, member, sig string, args []any) error {
			c, err := dbusSession()
			if err != nil {
				// Не просто err: dbusServiceAbsent должен узнать «шины нет».
				return fmt.Errorf("%w: %v", errOpenNoBus, err)
			}
			_, err = c.callTimeout(dest, path, iface, member, sig, args, openCallTimeout)
			return err
		},
		lookPath: exec.LookPath,
		start: func(cmd *exec.Cmd) error {
			// Новая сессия: открытое приложение не должно умирать вместе с
			// нашим терминалом и ловить наш Ctrl+C.
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			return openStartCmd(cmd)
		},
	}
}

func platformOpenURL(u string) error { return linuxOpenBackend().open(u, u) }

func platformOpenFile(p string) error { return linuxOpenBackend().open(pathToFileURL(p), p) }

func platformRevealFile(p string) error { return linuxOpenBackend().reveal(p) }
