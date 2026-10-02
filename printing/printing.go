// printing.go — публичный API печати: задание, принтеры, печать, сохранение в
// PDF. Системные вызовы вынесены в платформенные файлы (printing_windows.go,
// printing_linux.go, printing_unsupported.go); здесь только то, что одинаково
// везде: проверка входа, выбор страниц, перевод страниц в PDF.
package printing

import (
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"

	"github.com/oops1/headless-gui/v3/output/pdf"
)

// Ошибки. Различимы через errors.Is: приложение показывает пользователю разные
// сообщения для «нет принтера», «отмена» и «печать не поддержана».
var (
	// ErrUnsupported — на этой платформе печать (или этот её способ) не
	// реализована. Сохранение в PDF от платформы не зависит.
	ErrUnsupported = errors.New("printing: печать на этой платформе не поддержана")
	// ErrNoPrinter — принтер не найден или не назначен по умолчанию.
	ErrNoPrinter = errors.New("printing: принтер не найден")
	// ErrCanceled — пользователь закрыл системный диалог печати.
	ErrCanceled = errors.New("printing: печать отменена пользователем")
	// ErrNoPrintService — служба печати недоступна (на Linux — не запущен CUPS).
	ErrNoPrintService = errors.New("printing: служба печати недоступна")
	// ErrNoPages — в задании нет страниц.
	ErrNoPages = errors.New("printing: в задании нет страниц")
)

// MaxCopies — наибольшее число копий. Опечатка «10000» вместо «1» иначе
// выпустила бы пачку бумаги до того, как кто-то успеет нажать «отмена».
const MaxCopies = 999

// Job — задание печати: набор страниц-картинок и параметры листа.
//
// Страницы рисует приложение: каждая — *image.RGBA (или любой image.Image)
// размера Setup.SheetSize() в разрешении Setup.DPI, то есть ровно на лист
// бумаги. Размер страницы в пикселях при этом не обязан совпадать со
// SheetSize — тогда физический размер берётся из пикселей и DPI, — но совпадение
// единственный способ гарантировать, что на бумаге всё займёт запланированное
// место.
type Job struct {
	// Name — имя документа для очереди печати и заголовок PDF.
	Name string
	// Setup — бумага, ориентация, поля, разрешение.
	Setup PageSetup
	// Pages — страницы по порядку.
	Pages []image.Image
}

// Validate проверяет задание.
func (j Job) Validate() error {
	if len(j.Pages) == 0 {
		return ErrNoPages
	}
	if err := j.Setup.Validate(); err != nil {
		return err
	}
	for i, p := range j.Pages {
		if p == nil || p.Bounds().Empty() {
			return fmt.Errorf("printing: страница %d пуста", i+1)
		}
	}
	return nil
}

// PrinterState — состояние принтера по IPP (printer-state).
type PrinterState int

const (
	// StateUnknown — состояние неизвестно (система не сообщила).
	StateUnknown PrinterState = 0
	// StateIdle — свободен.
	StateIdle PrinterState = 3
	// StateProcessing — печатает.
	StateProcessing PrinterState = 4
	// StateStopped — остановлен (кончилась бумага, открыта крышка, пауза очереди).
	StateStopped PrinterState = 5
)

func (s PrinterState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateProcessing:
		return "processing"
	case StateStopped:
		return "stopped"
	}
	return "unknown"
}

// Printer — принтер, известный системе.
type Printer struct {
	// Name — системное имя; именно оно передаётся в Target.Printer.
	Name string
	// Description, Location, Model — описание, место и модель, если система их
	// сообщила: для строки в списке принтеров.
	Description string
	Location    string
	Model       string
	State       PrinterState
	// Accepting — принимает ли очередь новые задания.
	Accepting bool
	// Default — принтер по умолчанию.
	Default bool
}

// Target — куда и как печатать.
type Target struct {
	// Printer — имя принтера; пусто — принтер по умолчанию.
	Printer string
	// Copies — число копий; 0 означает одну.
	Copies int
	// FromPage, ToPage — диапазон страниц задания, с единицы включительно.
	// Нули — все страницы. Диапазон сужается в самом пакете, а не принтером:
	// драйверы по-разному понимают диапазоны, а страницы-картинки режутся без
	// потерь.
	FromPage, ToPage int
	// Collate — раскладывать копии по комплектам (1,2,3, 1,2,3), а не по
	// страницам (1,1, 2,2, 3,3). Учитывается на Windows.
	Collate bool

	// devmode — настройки драйвера принтера (DEVMODE), которые пользователь
	// выбрал в системном диалоге: двусторонняя печать, лоток, качество. Заполняется
	// только PrintDialog; без него настройки вернулись бы к умолчаниям драйвера.
	devmode []byte
}

// Printers возвращает принтеры системы.
//
// Linux: список от CUPS (CUPS-Get-Printers); без запущенного CUPS ошибка
// errors.Is(err, ErrNoPrintService). Windows: EnumPrinters. Прочие платформы:
// ErrUnsupported.
func Printers() ([]Printer, error) { return printersImpl() }

// DefaultPrinter возвращает принтер по умолчанию; если он не назначен —
// errors.Is(err, ErrNoPrinter).
func DefaultPrinter() (Printer, error) { return defaultPrinterImpl() }

// Print печатает задание на выбранный принтер.
//
// Вызов блокирует до того, как система приняла задание в очередь (а на Windows —
// пока страницы переданы драйверу): это секунды, поэтому из обработчика кадра его
// следует вызывать в горутине. Успех означает «принято в очередь», а не «бумага
// вышла»: дальше за задание отвечает система печати.
func Print(t Target, job Job) error {
	if err := job.Validate(); err != nil {
		return err
	}
	if t.Copies < 0 || t.Copies > MaxCopies {
		return fmt.Errorf("printing: число копий %d вне 1…%d", t.Copies, MaxCopies)
	}
	if t.Copies == 0 {
		t.Copies = 1
	}
	lo, hi, err := pageRange(len(job.Pages), t.FromPage, t.ToPage)
	if err != nil {
		return err
	}
	job.Pages = job.Pages[lo:hi]
	// Диапазон уже применён к заданию; принтеру он больше не нужен, а повторное
	// применение к уже суженному набору отрезало бы лишнее.
	t.FromPage, t.ToPage = 0, 0
	return printImpl(t, job)
}

// PrintDialog показывает системный диалог выбора принтера и параметров печати и
// возвращает выбор пользователя; сама печать — Print(target, job).
//
// owner — дескриптор окна-владельца (HWND на Windows); 0 — активное окно
// процесса. Диалог модальный: вызов не возвращается, пока пользователь его не
// закроет. Отмена — errors.Is(err, ErrCanceled). Там, где системного диалога
// нет (Linux, macOS), — ErrUnsupported: приложение показывает собственный выбор
// принтера по Printers().
func PrintDialog(owner uintptr, job Job) (Target, error) {
	if err := job.Validate(); err != nil {
		return Target{}, err
	}
	return dialogImpl(owner, job)
}

// HasPrintDialog сообщает, есть ли на этой платформе системный диалог печати.
func HasPrintDialog() bool { return hasDialog }

// Подменяемые точки: тесты проверяют, что дошло бы до системы, ничего не печатая.
var (
	printersImpl       = platformPrinters
	defaultPrinterImpl = platformDefaultPrinter
	printImpl          = platformPrint
	dialogImpl         = platformDialog
)

// pageRange переводит диапазон «с единицы, включительно» в срез [lo,hi).
// Верхняя граница за пределами задания молча урезается («до конца»): так
// диалог, оставивший 1–9999 по умолчанию, работает. Начало за пределами и
// перевёрнутый диапазон — ошибка: это, скорее всего, ошибка вызова, и лучше
// отказать, чем напечатать пустое.
func pageRange(n, from, to int) (lo, hi int, err error) {
	if from == 0 && to == 0 {
		return 0, n, nil
	}
	if from < 1 {
		from = 1
	}
	if to < 1 || to > n {
		to = n
	}
	if from > n || from > to {
		return 0, 0, fmt.Errorf("printing: диапазон страниц %d–%d вне задания из %d страниц", from, to, n)
	}
	return from - 1, to, nil
}

// ─── PDF ─────────────────────────────────────────────────────────────────────

// PDFOptions — параметры PDF (заголовок, автор, сжатие). См. pdf.Options.
type PDFOptions = pdf.Options

// WritePDF записывает задание как PDF-документ: каждая страница — картинка,
// размер страницы в пунктах берётся из пикселей и разрешения задания.
// Доступна на всех платформах: сохранение в PDF от принтеров не зависит.
func WritePDF(w io.Writer, job Job, opt PDFOptions) error {
	if err := job.Validate(); err != nil {
		return err
	}
	if opt.Title == "" {
		opt.Title = job.Name
	}
	dpi := float64(job.Setup.dpi())
	pw, err := pdf.NewWriter(w, opt)
	if err != nil {
		return err
	}
	for i, img := range job.Pages {
		if err := pw.AddPage(pdf.PageFromImage(img, dpi)); err != nil {
			return fmt.Errorf("printing: страница %d: %w", i+1, err)
		}
	}
	return pw.Close()
}

// SavePDF сохраняет задание в PDF-файл. Файл пишется во временный и только
// потом переименовывается: оборванная запись (диск заполнен, сбой посреди
// страниц) иначе оставила бы на месте прежнего хорошего документа обрубок.
func SavePDF(path string, job Job, opt PDFOptions) error {
	if path == "" {
		return errors.New("printing: пустой путь файла")
	}
	if err := job.Validate(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".print-*.pdf.tmp")
	if err != nil {
		return fmt.Errorf("printing: %w", err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	if err := WritePDF(tmp, job, opt); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("printing: %w", err)
	}
	// Временный файл создаётся с правами 0600; обычный документ — 0644 (до umask).
	_ = os.Chmod(tmpName, 0o644)
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("printing: %w", err)
	}
	ok = true
	return nil
}
