//go:build linux && !android

// printing_linux.go — печать на Linux через CUPS: IPP-запросы по HTTP на
// локальный сокет или localhost:631 (cups.go), документ — PDF (output/pdf).
//
// Чего здесь нет намеренно.
//   - Подпроцессов lp/lpr: пакет WinLine ставится без зависимостей, и этих
//     утилит в нём нет.
//   - Портала org.freedesktop.portal.Print: он принимает документ файловым
//     дескриптором, переданным по D-Bus (SCM_RIGHTS), а собственный клиент D-Bus
//     этого репозитория передавать дескрипторы не умеет (то же препятствие
//     обнаружилось при openurl_portal.go). Реализация потребовала бы сначала
//     научить клиент D-Bus этому. Для приложений без CUPS на машине (песочницы
//     Flatpak/Snap) это ограничение: там печать вернёт ErrNoPrintService.
//   - Системного диалога выбора принтера: общего для всех окружений рабочего
//     стола его нет; приложение показывает свой выбор по Printers().
package printing

import (
	"context"
	"os"
)

const hasDialog = false

// cupsFactory — подменяемое создание клиента: тесты подставляют клиента на
// тестовый сервер, а не на настоящий CUPS машины.
var cupsFactory = func() *cupsClient {
	return newCupsClient(detectCUPS(os.Getenv, osExists))
}

func platformPrinters() ([]Printer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cupsQueryTimeout)
	defer cancel()
	return cupsFactory().listPrinters(ctx)
}

func platformDefaultPrinter() (Printer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cupsQueryTimeout)
	defer cancel()
	return cupsFactory().defaultPrinter(ctx)
}

func platformPrint(t Target, job Job) error {
	ctx, cancel := context.WithTimeout(context.Background(), cupsPrintTimeout)
	defer cancel()
	return printViaCUPS(ctx, cupsFactory(), t, job)
}

func platformDialog(uintptr, Job) (Target, error) { return Target{}, ErrUnsupported }
