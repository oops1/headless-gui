//go:build windows

// printing_windows.go — печать на Windows: системный диалог PrintDlgEx
// (comdlg32) и печать страниц-картинок через GDI (StartDoc, StretchDIBits,
// EndPage, EndDoc). Всё без CGO: lazy-загрузка DLL, как в window/native_windows.go.
//
// Раскладка страницы и полос — в rasterprint.go, разбор DEVNAMES/DEVMODE — в
// winparse.go; здесь только вызовы системы.
package printing

import (
	"errors"
	"fmt"
	"image"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const hasDialog = true

var (
	winspool = windows.NewLazySystemDLL("winspool.drv")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	comdlg32 = windows.NewLazySystemDLL("comdlg32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")

	procEnumPrintersW      = winspool.NewProc("EnumPrintersW")
	procGetDefaultPrinterW = winspool.NewProc("GetDefaultPrinterW")

	procCreateDCW         = gdi32.NewProc("CreateDCW")
	procDeleteDC          = gdi32.NewProc("DeleteDC")
	procStartDocW         = gdi32.NewProc("StartDocW")
	procStartPage         = gdi32.NewProc("StartPage")
	procEndPage           = gdi32.NewProc("EndPage")
	procEndDoc            = gdi32.NewProc("EndDoc")
	procAbortDoc          = gdi32.NewProc("AbortDoc")
	procGetDeviceCaps     = gdi32.NewProc("GetDeviceCaps")
	procStretchDIBits     = gdi32.NewProc("StretchDIBits")
	procSetStretchBltMode = gdi32.NewProc("SetStretchBltMode")
	procSetBrushOrgEx     = gdi32.NewProc("SetBrushOrgEx")

	procPrintDlgExW = comdlg32.NewProc("PrintDlgExW")

	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	procGlobalFree   = kernel32.NewProc("GlobalFree")
	procGlobalSize   = kernel32.NewProc("GlobalSize")

	procGetActiveWindow     = user32.NewProc("GetActiveWindow")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
)

// Константы GDI и диалога печати (wingdi.h, commdlg.h).
const (
	devPhysicalWidth   = 110 // PHYSICALWIDTH: весь лист в точках принтера
	devPhysicalHeight  = 111 // PHYSICALHEIGHT
	devPhysicalOffsetX = 112 // PHYSICALOFFSETX: где начинается печатная область
	devPhysicalOffsetY = 113
	devHorzRes         = 8 // HORZRES: ширина печатной области
	devVertRes         = 10

	srcCopy         = 0x00CC0020
	stretchColorOn  = 3 // COLORONCOLOR: точки копируются как есть (ближайший сосед)
	stretchHalftone = 4 // HALFTONE: усреднение при уменьшении
	dibRGBColors    = 0

	pdPageNums    = 0x00000002
	pdCollate     = 0x00000010
	pdNoSelection = 0x00000004
	pdNoCurrent   = 0x00800000

	pdResultPrint = 1

	startPageGeneral = 0xFFFFFFFF

	printerEnumLocal       = 0x00000002
	printerEnumConnections = 0x00000004

	errInvalidPrinterName = 1801 // ERROR_INVALID_PRINTER_NAME
	errCancelled          = 1223 // ERROR_CANCELLED
)

// ─── Перечень принтеров ──────────────────────────────────────────────────────

// printerInfo4 — PRINTER_INFO_4W. Уровень 4 выбран намеренно: он отдаёт только
// имя и признаки и не обращается к самим принтерам, а уровень 2 (описание, порт,
// состояние) опрашивает каждый — и зависает на несколько секунд на сетевом
// принтере, который выключен.
type printerInfo4 struct {
	pPrinterName *uint16
	pServerName  *uint16
	attributes   uint32
}

// callErr превращает код последней ошибки в error.
func callErr(e error) error {
	if e == nil || e == syscall.Errno(0) {
		return errors.New("неизвестная ошибка")
	}
	return e
}

func platformPrinters() ([]Printer, error) {
	flags := uintptr(printerEnumLocal | printerEnumConnections)
	var needed, returned uint32
	r, _, e := procEnumPrintersW.Call(flags, 0, 4, 0, 0,
		uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if r == 0 && e != syscall.Errno(windows.ERROR_INSUFFICIENT_BUFFER) {
		return nil, fmt.Errorf("printing: EnumPrinters: %w", callErr(e))
	}
	if needed == 0 {
		return nil, nil // принтеров нет
	}
	// Буфер из uint64, а не []byte: указатели внутри структур требуют
	// выравнивания на 8, а у среза байтов оно гарантировано не везде.
	buf := make([]uint64, (needed+7)/8)
	r, _, e = procEnumPrintersW.Call(flags, 0, 4, uintptr(unsafe.Pointer(&buf[0])), uintptr(needed),
		uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if r == 0 {
		return nil, fmt.Errorf("printing: EnumPrinters: %w", callErr(e))
	}
	def, _ := defaultPrinterName()
	infos := unsafe.Slice((*printerInfo4)(unsafe.Pointer(&buf[0])), int(returned))
	out := make([]Printer, 0, len(infos))
	for _, in := range infos {
		name := windows.UTF16PtrToString(in.pPrinterName)
		if name == "" {
			continue
		}
		out = append(out, Printer{Name: name, Accepting: true, Default: name == def})
	}
	runtime.KeepAlive(buf)
	return out, nil
}

// defaultPrinterName возвращает имя принтера по умолчанию ("" если не назначен).
func defaultPrinterName() (string, error) {
	var n uint32
	r, _, e := procGetDefaultPrinterW.Call(0, uintptr(unsafe.Pointer(&n)))
	if r != 0 {
		return "", nil
	}
	if e == syscall.Errno(windows.ERROR_FILE_NOT_FOUND) || n == 0 {
		return "", nil
	}
	if e != syscall.Errno(windows.ERROR_INSUFFICIENT_BUFFER) {
		return "", fmt.Errorf("printing: GetDefaultPrinter: %w", callErr(e))
	}
	buf := make([]uint16, n)
	r, _, e = procGetDefaultPrinterW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return "", fmt.Errorf("printing: GetDefaultPrinter: %w", callErr(e))
	}
	return windows.UTF16ToString(buf), nil
}

func platformDefaultPrinter() (Printer, error) {
	name, err := defaultPrinterName()
	if err != nil {
		return Printer{}, err
	}
	if name == "" {
		return Printer{}, fmt.Errorf("%w: принтер по умолчанию не назначен", ErrNoPrinter)
	}
	return Printer{Name: name, Accepting: true, Default: true}, nil
}

// ─── Печать через GDI ────────────────────────────────────────────────────────

func platformPrint(t Target, job Job) error {
	name := t.Printer
	if name == "" {
		def, err := platformDefaultPrinter()
		if err != nil {
			return err
		}
		name = def.Name
	}
	// StartDoc … EndDoc выполняются на одном потоке: драйверы печати нередко
	// держат состояние в потоке-владельце (и COM, и окна прогресса), а горутина
	// между вызовами могла бы уйти на другой.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	dev, err := openPrinterDC(name, t.devmode)
	if err != nil {
		return err
	}
	defer dev.close()
	return printRaster(dev, job.Name, job.Pages, t.Copies, t.Collate)
}

// gdiDevice — контекст устройства принтера; реализует rasterDevice.
type gdiDevice struct{ hdc uintptr }

// openPrinterDC создаёт контекст принтера. devmode — настройки из системного
// диалога (nil — умолчания драйвера).
func openPrinterDC(name string, devmode []byte) (*gdiDevice, error) {
	driver, _ := windows.UTF16PtrFromString("WINSPOOL")
	dev, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("printing: имя принтера: %w", err)
	}
	var dm uintptr
	if len(devmode) > 0 {
		dm = uintptr(unsafe.Pointer(&devmode[0]))
	}
	r, _, e := procCreateDCW.Call(uintptr(unsafe.Pointer(driver)), uintptr(unsafe.Pointer(dev)), 0, dm)
	runtime.KeepAlive(devmode)
	if r == 0 {
		if e == syscall.Errno(errInvalidPrinterName) {
			return nil, fmt.Errorf("%w: %q", ErrNoPrinter, name)
		}
		return nil, fmt.Errorf("printing: CreateDC(%q): %w", name, callErr(e))
	}
	return &gdiDevice{hdc: r}, nil
}

func (d *gdiDevice) close() { procDeleteDC.Call(d.hdc) }

func (d *gdiDevice) caps(index uintptr) int {
	r, _, _ := procGetDeviceCaps.Call(d.hdc, index)
	return int(int32(r))
}

// Caps: PHYSICALWIDTH/HEIGHT — весь лист, PHYSICALOFFSET — где на нём начинается
// печатная область. Виртуальные принтеры иногда отдают нули: тогда берём
// размер печатной области и нулевые смещения — страница ляжет на неё целиком.
func (d *gdiDevice) Caps() deviceCaps {
	c := deviceCaps{
		PhysW: d.caps(devPhysicalWidth), PhysH: d.caps(devPhysicalHeight),
		OffX: d.caps(devPhysicalOffsetX), OffY: d.caps(devPhysicalOffsetY),
	}
	if c.PhysW <= 0 || c.PhysH <= 0 {
		c = deviceCaps{PhysW: d.caps(devHorzRes), PhysH: d.caps(devVertRes)}
	}
	return c
}

// gdiOutputFile — если задан, задание печатается не на бумагу, а в этот файл
// (поле lpszOutput структуры DOCINFO: так работают виртуальные принтеры вроде
// «Microsoft Print to PDF», не спрашивая имя файла в окне). Нужен проверочному
// тесту, который гоняет настоящий путь GDI, не тратя бумагу; в обычной работе пуст.
var gdiOutputFile string

// docInfo — DOCINFOW.
type docInfo struct {
	cbSize       int32
	lpszDocName  *uint16
	lpszOutput   *uint16
	lpszDatatype *uint16
	fwType       uint32
}

func (d *gdiDevice) StartDoc(name string) error {
	if name == "" {
		name = "headless-gui"
	}
	n, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return fmt.Errorf("printing: имя документа: %w", err)
	}
	di := docInfo{cbSize: int32(unsafe.Sizeof(docInfo{})), lpszDocName: n}
	var out *uint16
	if gdiOutputFile != "" {
		if out, err = windows.UTF16PtrFromString(gdiOutputFile); err != nil {
			return fmt.Errorf("printing: путь файла вывода: %w", err)
		}
		di.lpszOutput = out
	}
	r, _, e := procStartDocW.Call(d.hdc, uintptr(unsafe.Pointer(&di)))
	runtime.KeepAlive(n)
	runtime.KeepAlive(out)
	if int32(r) <= 0 {
		// 1223 — пользователь закрыл окно «куда сохранить» у виртуального
		// принтера (Microsoft Print to PDF): это отмена, а не поломка.
		if e == syscall.Errno(errCancelled) {
			return ErrCanceled
		}
		return fmt.Errorf("printing: StartDoc: %w", callErr(e))
	}
	return nil
}

func (d *gdiDevice) StartPage() error {
	if r, _, e := procStartPage.Call(d.hdc); int32(r) <= 0 {
		return fmt.Errorf("printing: StartPage: %w", callErr(e))
	}
	return nil
}

func (d *gdiDevice) EndPage() error {
	if r, _, e := procEndPage.Call(d.hdc); int32(r) <= 0 {
		return fmt.Errorf("printing: EndPage: %w", callErr(e))
	}
	return nil
}

func (d *gdiDevice) EndDoc() error {
	if r, _, e := procEndDoc.Call(d.hdc); int32(r) <= 0 {
		return fmt.Errorf("printing: EndDoc: %w", callErr(e))
	}
	return nil
}

func (d *gdiDevice) Abort() { procAbortDoc.Call(d.hdc) }

// bitmapInfoHeader — BITMAPINFOHEADER. 32 бита на пиксель без сжатия не требуют
// таблицы цветов, поэтому заголовка достаточно.
type bitmapInfoHeader struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

// Band отправляет полосу через StretchDIBits.
func (d *gdiDevice) Band(dst image.Rectangle, srcW, srcRows int, bits []byte) error {
	if len(bits) < 4*srcW*srcRows {
		return errors.New("printing: полоса короче заявленного размера")
	}
	// Режим растяжения задаётся здесь, а не один раз: StartPage сбрасывает
	// атрибуты контекста к умолчаниям, а умолчание для принтера, BLACKONWHITE,
	// при уменьшении гасит светлые пиксели логическим И — тонкий серый текст
	// исчезает. HALFTONE нужен при уменьшении; при увеличении (принтер 600 dpi,
	// страница 300) COLORONCOLOR даёт чёткое удвоение точек без размытия.
	mode := uintptr(stretchColorOn)
	if dst.Dx() < srcW {
		mode = stretchHalftone
	}
	procSetStretchBltMode.Call(d.hdc, mode)
	if mode == stretchHalftone {
		// Требование MSDN: после HALFTONE сбросить начало кисти.
		procSetBrushOrgEx.Call(d.hdc, 0, 0, 0)
	}
	bmi := bitmapInfoHeader{
		size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		width:       int32(srcW),
		height:      -int32(srcRows), // минус — строки сверху вниз
		planes:      1,
		bitCount:    32,
		compression: 0, // BI_RGB
	}
	r, _, e := procStretchDIBits.Call(d.hdc,
		uintptr(int32(dst.Min.X)), uintptr(int32(dst.Min.Y)), uintptr(int32(dst.Dx())), uintptr(int32(dst.Dy())),
		0, 0, uintptr(srcW), uintptr(srcRows),
		uintptr(unsafe.Pointer(&bits[0])), uintptr(unsafe.Pointer(&bmi)), dibRGBColors, srcCopy)
	runtime.KeepAlive(bits)
	if r == 0 || uint32(r) == 0xFFFFFFFF { // 0 и GDI_ERROR — отказ драйвера
		return fmt.Errorf("printing: StretchDIBits: %w", callErr(e))
	}
	return nil
}

// ─── Системный диалог печати ─────────────────────────────────────────────────

// printPageRange — PRINTPAGERANGE.
type printPageRange struct{ from, to uint32 }

// printDlgEx — PRINTDLGEXW (commdlg.h). Раскладка полей совпадает с C: Go
// выравнивает так же (указатели на 8, DWORD на 4), поэтому размер структуры и
// смещения те же, что ждёт comdlg32.
type printDlgEx struct {
	lStructSize         uint32
	hwndOwner           uintptr
	hDevMode            uintptr
	hDevNames           uintptr
	hDC                 uintptr
	flags               uint32
	flags2              uint32
	exclusionFlags      uint32
	nPageRanges         uint32
	nMaxPageRanges      uint32
	lpPageRanges        *printPageRange
	nMinPage            uint32
	nMaxPage            uint32
	nCopies             uint32
	hInstance           uintptr
	lpPrintTemplateName uintptr
	lpCallback          uintptr
	nPropertyPages      uint32
	lphPropertyPages    uintptr
	nStartPage          uint32
	dwResultAction      uint32
}

// globalBytes копирует содержимое блока HGLOBAL.
func globalBytes(h uintptr) []byte {
	if h == 0 {
		return nil
	}
	size, _, _ := procGlobalSize.Call(h)
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 || size == 0 {
		return nil
	}
	defer procGlobalUnlock.Call(h)
	// Копия сразу: после GlobalFree память недоступна, а значения нужны дольше.
	// Адрес от GlobalLock — блок системной кучи, не память Go. Прямое
	// unsafe.Pointer(p) от uintptr go vet помечает как возможную ошибку, а здесь
	// она исключена (блок закреплён замком до Unlock), поэтому указатель
	// получается через адрес переменной.
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&p))
	return append([]byte(nil), unsafe.Slice((*byte)(ptr), int(size))...)
}

func platformDialog(owner uintptr, job Job) (Target, error) {
	if owner == 0 {
		// PrintDlgEx без окна-владельца отказывает (E_HANDLE). Берём активное окно
		// процесса, а если его нет — окно переднего плана: диалог в любом случае
		// лучше, чем ошибка, а печать вызывают по действию пользователя в окне.
		owner, _, _ = procGetActiveWindow.Call()
		if owner == 0 {
			owner, _, _ = procGetForegroundWindow.Call()
		}
	}

	// Диалог печати пользуется COM/OLE: без инициализации на потоке он
	// возвращает CO_E_NOTINITIALIZED. Поток закрепляется — COM-состояние
	// принадлежит потоку. Та же схема, что у ShellExecute в пакете window.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE)
	if hr == nil || hr == syscall.Errno(1) { // S_OK или S_FALSE — вызов засчитан
		defer windows.CoUninitialize()
	}

	n := uint32(len(job.Pages))
	ranges := make([]printPageRange, 1)
	ranges[0] = printPageRange{from: 1, to: n}
	pd := printDlgEx{
		hwndOwner: owner,
		// PD_NOSELECTION/PD_NOCURRENTPAGE: у страницы-картинки нет «выделенного»
		// и «текущей», а лишние переключатели обещали бы то, чего мы не умеем.
		// PD_USEDEVMODECOPIESANDCOLLATE не ставим: копии и комплектность
		// печатает сам пакет (порядок страниц у драйверов разный), и диалог
		// должен вернуть их числами, а не прятать в драйвер.
		flags:          pdNoSelection | pdNoCurrent,
		nPageRanges:    1,
		nMaxPageRanges: 1,
		nMinPage:       1,
		nMaxPage:       n,
		nCopies:        1,
		nStartPage:     startPageGeneral,
	}
	pd.lStructSize = uint32(unsafe.Sizeof(pd))
	pd.lpPageRanges = &ranges[0] // типизированный указатель: сборщик мусора видит его и при росте стека поправит

	r, _, _ := procPrintDlgExW.Call(uintptr(unsafe.Pointer(&pd)))
	runtime.KeepAlive(ranges)
	// Блоки диалога освобождаем при любом исходе: они принадлежат вызывающему.
	devNames := globalBytes(pd.hDevNames)
	devMode := globalBytes(pd.hDevMode)
	if pd.hDevNames != 0 {
		procGlobalFree.Call(pd.hDevNames)
	}
	if pd.hDevMode != 0 {
		procGlobalFree.Call(pd.hDevMode)
	}
	if r != 0 { // HRESULT не S_OK
		return Target{}, fmt.Errorf("printing: PrintDlgEx: HRESULT 0x%08x", uint32(r))
	}
	if pd.dwResultAction != pdResultPrint {
		// Отмена, а также «Применить» (PD_RESULT_APPLY): сохранение настроек без
		// печати — не то, о чём просили.
		return Target{}, ErrCanceled
	}

	name, err := devNamesDevice(devNames)
	if err != nil {
		return Target{}, err
	}
	t := Target{Printer: name, Copies: int(pd.nCopies), Collate: pd.flags&pdCollate != 0}
	if t.Copies < 1 {
		t.Copies = 1
	}
	if t.Copies > MaxCopies {
		t.Copies = MaxCopies
	}
	if size, err := devmodeTotalSize(devMode); err == nil {
		t.devmode = devMode[:size]
		devmodeSingleCopy(t.devmode)
	}
	if pd.flags&pdPageNums != 0 && pd.nPageRanges > 0 {
		t.FromPage, t.ToPage = int(ranges[0].from), int(ranges[0].to)
	}
	return t, nil
}
