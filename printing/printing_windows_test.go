//go:build windows

package printing

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

// TestWindowsStructLayouts — размеры структур совпадают с C-шными (wingdi.h,
// commdlg.h). Неверный размер даёт не ошибку, а мусор в полях или отказ
// системы с невнятным кодом, поэтому сверка с известными числами дешевле отладки.
func TestWindowsStructLayouts(t *testing.T) {
	is64 := unsafe.Sizeof(uintptr(0)) == 8
	pick := func(v64, v32 uintptr) uintptr {
		if is64 {
			return v64
		}
		return v32
	}
	for name, c := range map[string]struct{ got, want uintptr }{
		"PRINTDLGEXW":      {unsafe.Sizeof(printDlgEx{}), pick(136, 84)},
		"DOCINFOW":         {unsafe.Sizeof(docInfo{}), pick(40, 20)},
		"BITMAPINFOHEADER": {unsafe.Sizeof(bitmapInfoHeader{}), 40},
		"PRINTER_INFO_4W":  {unsafe.Sizeof(printerInfo4{}), pick(24, 12)},
		"PRINTPAGERANGE":   {unsafe.Sizeof(printPageRange{}), 8},
	} {
		if c.got != c.want {
			t.Errorf("%s: %d байт, ждали %d", name, c.got, c.want)
		}
	}
}

// TestWindowsEnumeratePrinters — только чтение: перечень принтеров и принтер по
// умолчанию запрашиваются у системы, ничего не печатается. Принтеров на
// машине может не быть — это не ошибка.
func TestWindowsEnumeratePrinters(t *testing.T) {
	ps, err := platformPrinters()
	if err != nil {
		t.Fatalf("EnumPrinters: %v", err)
	}
	defaults := 0
	for _, p := range ps {
		if p.Name == "" {
			t.Error("принтер без имени")
		}
		if p.Default {
			defaults++
		}
	}
	t.Logf("принтеров: %d", len(ps))
	if defaults > 1 {
		t.Errorf("по умолчанию помечено %d принтеров", defaults)
	}
	def, err := platformDefaultPrinter()
	if err != nil {
		t.Logf("принтер по умолчанию: %v", err)
	} else if !def.Default || def.Name == "" {
		t.Errorf("принтер по умолчанию: %+v", def)
	}
}

// TestWindowsPrintToPDFPrinterLive прогоняет настоящий путь GDI (CreateDC,
// StartDoc, StretchDIBits …) на виртуальный принтер «Microsoft Print to PDF»,
// с выводом в файл через DOCINFO.lpszOutput. Бумагу не тратит, но выполняет
// живые системные вызовы, поэтому запускается только по явному запросу:
//
//	HEADLESS_GUI_PRINT_LIVE=1 go test ./printing -run PrintToPDFPrinterLive -v
func TestWindowsPrintToPDFPrinterLive(t *testing.T) {
	if os.Getenv("HEADLESS_GUI_PRINT_LIVE") != "1" {
		t.Skip("живая проверка GDI: задайте HEADLESS_GUI_PRINT_LIVE=1")
	}
	const virtual = "Microsoft Print to PDF"
	ps, err := platformPrinters()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range ps {
		found = found || p.Name == virtual
	}
	if !found {
		t.Skipf("нет виртуального принтера %q", virtual)
	}
	out := filepath.Join(t.TempDir(), "live.pdf")
	gdiOutputFile = out
	t.Cleanup(func() { gdiOutputFile = "" })
	if err := platformPrint(Target{Printer: virtual, Copies: 1}, testJob(2)); err != nil {
		t.Fatalf("печать: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("файл вывода: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Errorf("вывод — не PDF: %q", data[:min(16, len(data))])
	}
	t.Logf("напечатано в %s: %d байт", out, len(data))
	if keep := os.Getenv("HEADLESS_GUI_PRINT_KEEP"); keep != "" {
		_ = os.WriteFile(keep, data, 0o644)
	}
}
