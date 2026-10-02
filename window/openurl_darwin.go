//go:build darwin

// openurl_darwin.go — открытие ссылок и файлов на macOS подпроцессом open.
// Для этой платформы подпроцесс допустим: /usr/bin/open входит в любую
// систему (это часть самой macOS, а не внешняя зависимость), а прямой путь —
// NSWorkspace через Objective-C — потребовал бы ещё одного моста ради того же
// результата.
package window

import "os/exec"

// macOpenBin — абсолютный путь, а не поиск в PATH: подложенный в PATH
// одноимённый файл не должен получать наши ссылки.
const macOpenBin = "/usr/bin/open"

func platformOpenURL(u string) error {
	return openStartCmd(exec.Command(macOpenBin, u))
}

func platformOpenFile(p string) error {
	return openStartCmd(exec.Command(macOpenBin, p))
}

// platformRevealFile: open -R показывает файл в Finder с выделением.
func platformRevealFile(p string) error {
	return openStartCmd(exec.Command(macOpenBin, "-R", p))
}
