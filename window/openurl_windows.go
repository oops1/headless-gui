//go:build windows

// openurl_windows.go — открытие ссылок и файлов на Windows через ShellExecuteW
// (глагол "open"): ту же функцию зовёт Проводник по двойному щелчку, так что
// выбор приложения — настоящий, из «Приложений по умолчанию». Показ файла
// с выделением — explorer.exe /select, подпроцессом: Проводник есть в любой
// Windows, а SHOpenFolderAndSelectItems потребовал бы COM-объектов (PIDL,
// IShellFolder) ради того же результата.
package window

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procShellExecuteW = shell32.NewProc("ShellExecuteW")

// winShellExecute — подменяемая точка вызова ShellExecute (verb "open", цель).
var winShellExecute = shellExecuteOpen

// shellExecuteOpen вызывает ShellExecuteW напрямую, а не через
// windows.ShellExecute из x/sys: та при неудаче теряет код возврата и
// отдаёт «параметр задан неверно», а по коду видно, что именно случилось
// (нет файла, нет назначенного приложения, отказ в доступе).
//
// COM инициализируется на этом же потоке: обработчики типов (расширения
// оболочки, DDE-серверы) нередко требуют COM, и без него ShellExecute на
// части машин возвращал ошибку для файлов, которые Проводник открывает.
// Поток закрепляется, потому что COM-состояние принадлежит потоку, а
// горутина без этого могла бы уйти на другой между вызовами.
func shellExecuteOpen(verb, target string) error {
	vp, err := windows.UTF16PtrFromString(verb)
	if err != nil {
		return err
	}
	tp, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// S_FALSE (1) — COM уже был инициализирован нами же: вызов засчитан,
	// CoUninitialize нужен. RPC_E_CHANGED_MODE — поток уже под другой
	// моделью: пользоваться можно, закрывать чужую инициализацию нельзя.
	// Иная ошибка — продолжаем без COM: ShellExecute чаще всего справится.
	hr := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE)
	if hr == nil || hr == syscall.Errno(1) {
		defer windows.CoUninitialize()
	}

	const swShowNormal = 1
	r, _, _ := procShellExecuteW.Call(0,
		uintptr(unsafe.Pointer(vp)), uintptr(unsafe.Pointer(tp)), 0, 0, swShowNormal)
	if r <= 32 { // значения до 32 включительно — коды ошибок, не дескриптор
		return fmt.Errorf("window: ShellExecute: %s", shellExecuteErrText(r))
	}
	return nil
}

func platformOpenURL(u string) error { return winShellExecute("open", u) }

func platformOpenFile(p string) error { return winShellExecute("open", p) }

// platformRevealFile запускает explorer.exe /select,"путь". Проводник
// возвращает код 1 даже при успехе, поэтому код завершения не смотрим —
// ошибкой считается только невозможность его запустить.
func platformRevealFile(p string) error {
	// Абсолютный путь к Проводнику из %SystemRoot%: поиск по PATH и текущей
	// папке нашёл бы подложенный рядом explorer.exe.
	explorer := "explorer.exe"
	if root := os.Getenv("SystemRoot"); root != "" {
		explorer = filepath.Join(root, "explorer.exe")
	}
	line, err := explorerSelectCmdLine(explorer, filepath.Clean(p))
	if err != nil {
		return err
	}
	cmd := exec.Command(explorer)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: line}
	return openStartCmd(cmd)
}
