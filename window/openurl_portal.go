// openurl_portal.go — порядок действий «открыть/показать» на Linux, вынесенный
// из openurl_linux.go в файл без платформенного суффикса. Сам вызов D-Bus и
// запуск подпроцесса приходят снаружи (openBackend), поэтому ветвления —
// «портал есть», «портала нет», «xdg-open не найден» — проверяются тестами в
// общем прогоне на любой ОС, а не только на Linux-машине.
package window

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Имя и путь самого портала общие с чтением темы (systheme.go):
// portalBusName, portalObjPath.
const (
	portalOpenURIIf = "org.freedesktop.portal.OpenURI"

	fileMgrBusName = "org.freedesktop.FileManager1"
	fileMgrObjPath = "/org/freedesktop/FileManager1"
	fileMgrIface   = "org.freedesktop.FileManager1"
)

// errOpenNoBus — сессионной шины D-Bus нет вовсе (tty-сессия, контейнер).
var errOpenNoBus = errors.New("window: нет сессионной шины D-Bus")

// errOpenNoXdgOpen — запасной путь недоступен: xdg-open нет в PATH.
var errOpenNoXdgOpen = errors.New("window: xdg-open не найден в PATH")

// dbusServiceAbsent — ошибка говорит «сервиса на шине нет», а не «сервис есть,
// но отказал». Только в первом случае разумно идти запасным путём: если
// портал жив и ответил отказом, обход его через xdg-open обошёл бы и политику
// песочницы, и выбор пользователя.
func dbusServiceAbsent(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errOpenNoBus) {
		return true
	}
	s := err.Error()
	// Spawn.* — служба зарегистрирована для активации, но не запустилась.
	for _, mark := range []string{"ServiceUnknown", "NameHasNoOwner", "Error.Spawn."} {
		if strings.Contains(s, mark) {
			return true
		}
	}
	return false
}

// openBackend — три внешние зависимости Linux-реализации. В рабочем коде их
// даёт openurl_linux.go, в тестах — заглушки.
type openBackend struct {
	// call — вызов метода на сессионной шине; возвращает только ошибку.
	call func(dest, path, iface, member, sig string, args []any) error
	// lookPath — поиск исполняемого файла в PATH (exec.LookPath).
	lookPath func(file string) (string, error)
	// start — запуск подпроцесса без ожидания.
	start func(cmd *exec.Cmd) error
}

// portalOpenURIArgs — сигнатура и аргументы OpenURI(parent_window, uri,
// options). Родительское окно пустое: у окон движка нет идентификатора,
// понятного порталу (для X11 это был бы "x11:<xid>", для Wayland —
// экспортированный дескриптор xdg-foreign), а пустая строка допустима и
// означает «без родителя». Параметры пустые: "ask" и прочее решает система.
func portalOpenURIArgs(uri string) (string, []any) {
	return "ssa{sv}", []any{"", uri, map[string]dbusVariant{}}
}

// showItemsArgs — сигнатура и аргументы FileManager1.ShowItems(uris,
// startup_id).
func showItemsArgs(uri string) (string, []any) {
	return "ass", []any{[]string{uri}, ""}
}

// open открывает uri через портал OpenURI; если портала нет — запасным
// xdg-open с аргументом target.
//
// Файлы передаются порталу ссылкой file://, а не дескриптором (метод OpenFile
// с типом 'h'): наш клиент D-Bus не умеет передавать дескрипторы — для этого
// нужны SCM_RIGHTS и согласование UNIX_FD на этапе SASL, а то и другое
// сегодня не реализовано. Для приложения вне песочницы file:// работает.
// В песочнице Flatpak/Snap портал такие ссылки может отклонить — тогда
// вернётся его ошибка, а не молчание.
func (b openBackend) open(uri, target string) error {
	sig, args := portalOpenURIArgs(uri)
	err := b.call(portalBusName, portalObjPath, portalOpenURIIf, "OpenURI", sig, args)
	if err == nil {
		return nil
	}
	if !dbusServiceAbsent(err) {
		return fmt.Errorf("window: портал OpenURI: %w", err)
	}
	// Портала нет (голый WM, контейнер, tty): последняя надежда — xdg-open,
	// но только если он правда есть. В статическом бинарнике WinLine его
	// не будет, и честная ошибка лучше, чем притворный успех.
	if ferr := b.xdgOpen(target); ferr != nil {
		return fmt.Errorf("window: нет портала OpenURI (%v), запасной путь: %w", err, ferr)
	}
	return nil
}

// xdgOpen запускает xdg-open, найденный в PATH. Это ЗАПАСНОЙ путь: основной —
// портал, не требующий внешних утилит. target — либо абсолютный путь, либо
// проверенная ссылка со схемой, то есть начинаться с '-' не может и за
// параметр xdg-open не сойдёт.
func (b openBackend) xdgOpen(target string) error {
	bin, err := b.lookPath("xdg-open")
	if err != nil {
		return errOpenNoXdgOpen
	}
	return b.start(exec.Command(bin, target))
}

// reveal показывает файл в менеджере с выделением: FileManager1.ShowItems —
// то, чем пользуются Nautilus, Dolphin, Thunar, Nemo, PCManFM-Qt. Портальный
// OpenDirectory здесь не берётся: он, как и OpenFile, требует дескриптор.
// Нет менеджера с этим интерфейсом — открываем содержащую папку (без
// выделения, но человек видит, где файл).
func (b openBackend) reveal(path string) error {
	sig, args := showItemsArgs(pathToFileURL(path))
	err := b.call(fileMgrBusName, fileMgrObjPath, fileMgrIface, "ShowItems", sig, args)
	if err == nil {
		return nil
	}
	if !dbusServiceAbsent(err) {
		return fmt.Errorf("window: FileManager1.ShowItems: %w", err)
	}
	dir := filepath.Dir(path)
	return b.open(pathToFileURL(dir), dir)
}
