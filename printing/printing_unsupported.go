//go:build (!windows && !linux) || android

// printing_unsupported.go — платформы без реализации печати: macOS, BSD,
// Android. Функции честно возвращают ErrUnsupported вместо молчаливого
// бездействия: приложение может предложить «Сохранить как PDF» (SavePDF
// работает везде).
//
// Почему не macOS через CUPS. В macOS CUPS действительно есть и слушает
// собственный сокет, и тот же IPP-клиент (cups.go) в принципе применим. Но
// живьём это не проверено — стенда с macOS нет, — а печать, которая «должна
// работать», хуже честного отказа: пользователь узнаёт о проблеме на бумаге. Для
// включения достаточно перенести printing_linux.go под тег darwin и добавить в
// cupsSocketPaths путь /var/run/cupsd; делать это стоит вместе с проверкой на
// настоящем Mac. Родной путь macOS — NSPrintOperation — требует моста
// Objective-C, которого в проекте без CGO нет.
package printing

const hasDialog = false

func platformPrinters() ([]Printer, error) { return nil, ErrUnsupported }

func platformDefaultPrinter() (Printer, error) { return Printer{}, ErrUnsupported }

func platformPrint(Target, Job) error { return ErrUnsupported }

func platformDialog(uintptr, Job) (Target, error) { return Target{}, ErrUnsupported }
