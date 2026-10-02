package printing

import (
	"bytes"
	"errors"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestPageRange — диапазон «с единицы, включительно».
func TestPageRange(t *testing.T) {
	cases := []struct {
		n, from, to int
		lo, hi      int
		bad         bool
	}{
		{5, 0, 0, 0, 5, false},
		{5, 1, 5, 0, 5, false},
		{5, 2, 4, 1, 4, false},
		{5, 3, 3, 2, 3, false},
		{5, 1, 9999, 0, 5, false}, // «до конца»
		{5, 2, 0, 1, 5, false},    // to=0 при заданном from — «до конца»
		{5, 0, 3, 0, 3, false},    // from=0 при заданном to — «с первой»
		{5, 6, 9, 0, 0, true},
		{5, 4, 2, 0, 0, true},
	}
	for _, c := range cases {
		lo, hi, err := pageRange(c.n, c.from, c.to)
		if c.bad {
			if err == nil {
				t.Errorf("%+v: ошибки нет", c)
			}
			continue
		}
		if err != nil || lo != c.lo || hi != c.hi {
			t.Errorf("%+v: %d,%d,%v", c, lo, hi, err)
		}
	}
}

// stubPlatform подменяет системный слой и запоминает, что до него дошло.
func stubPlatform(t *testing.T) (got *[]printCall) {
	t.Helper()
	var calls []printCall
	old := printImpl
	printImpl = func(tg Target, j Job) error {
		calls = append(calls, printCall{tg, j})
		return nil
	}
	t.Cleanup(func() { printImpl = old })
	return &calls
}

type printCall struct {
	T Target
	J Job
}

func TestPrintFiltersPagesAndDefaults(t *testing.T) {
	calls := stubPlatform(t)
	job := testJob(5)
	if err := Print(Target{Printer: "Office", FromPage: 2, ToPage: 4}, job); err != nil {
		t.Fatal(err)
	}
	c := (*calls)[0]
	if len(c.J.Pages) != 3 || c.J.Pages[0] != job.Pages[1] || c.J.Pages[2] != job.Pages[3] {
		t.Errorf("страницы: %d шт.", len(c.J.Pages))
	}
	if c.T.Copies != 1 {
		t.Errorf("копий по умолчанию %d", c.T.Copies)
	}
	if c.T.FromPage != 0 || c.T.ToPage != 0 {
		t.Errorf("диапазон остался в Target: %d–%d (второй раз он отрезал бы лишнее)", c.T.FromPage, c.T.ToPage)
	}
	if c.T.Printer != "Office" || c.J.Name != job.Name {
		t.Errorf("цель/имя: %+v", c)
	}
	if len(job.Pages) != 5 {
		t.Error("исходное задание изменено")
	}
}

func TestPrintValidation(t *testing.T) {
	calls := stubPlatform(t)
	if err := Print(Target{}, Job{Setup: NewPageSetup(A4)}); !errors.Is(err, ErrNoPages) {
		t.Errorf("без страниц: %v", err)
	}
	bad := testJob(1)
	bad.Setup.DPI = 5
	if err := Print(Target{}, bad); !errors.Is(err, ErrBadSetup) {
		t.Errorf("плохие параметры: %v", err)
	}
	nilPage := testJob(2)
	nilPage.Pages[1] = nil
	if err := Print(Target{}, nilPage); err == nil {
		t.Error("пустая страница принята")
	}
	empty := testJob(1)
	empty.Pages[0] = image.NewRGBA(image.Rect(0, 0, 0, 0))
	if err := Print(Target{}, empty); err == nil {
		t.Error("страница нулевого размера принята")
	}
	for _, n := range []int{-1, MaxCopies + 1} {
		if err := Print(Target{Copies: n}, testJob(1)); err == nil {
			t.Errorf("копий %d принято", n)
		}
	}
	if err := Print(Target{FromPage: 9, ToPage: 12}, testJob(2)); err == nil {
		t.Error("диапазон вне задания принят")
	}
	if len(*calls) != 0 {
		t.Errorf("до системы дошло %d вызовов при неверном входе", len(*calls))
	}
}

func TestPrintersAndDialogPassThrough(t *testing.T) {
	oldP, oldD, oldDlg := printersImpl, defaultPrinterImpl, dialogImpl
	t.Cleanup(func() { printersImpl, defaultPrinterImpl, dialogImpl = oldP, oldD, oldDlg })
	printersImpl = func() ([]Printer, error) { return testPrinters, nil }
	defaultPrinterImpl = func() (Printer, error) { return Printer{}, ErrNoPrinter }
	var gotOwner uintptr
	dialogImpl = func(owner uintptr, j Job) (Target, error) {
		gotOwner = owner
		return Target{Printer: "X", Copies: 2}, nil
	}
	ps, err := Printers()
	if err != nil || !reflect.DeepEqual(ps, testPrinters) {
		t.Errorf("Printers: %v %v", ps, err)
	}
	if _, err := DefaultPrinter(); !errors.Is(err, ErrNoPrinter) {
		t.Errorf("DefaultPrinter: %v", err)
	}
	tg, err := PrintDialog(77, testJob(1))
	if err != nil || gotOwner != 77 || tg.Printer != "X" {
		t.Errorf("PrintDialog: %+v %v owner=%d", tg, err, gotOwner)
	}
	// Диалог не открывается для неверного задания.
	gotOwner = 0
	if _, err := PrintDialog(5, Job{}); !errors.Is(err, ErrNoPages) || gotOwner != 0 {
		t.Errorf("пустое задание: %v owner=%d", err, gotOwner)
	}
}

func TestWritePDFFromJob(t *testing.T) {
	job := testJob(2)
	job.Setup.Orientation = Landscape
	// Страницы нарисованы под книжный лист; в PDF размер берётся из пикселей и DPI.
	var buf bytes.Buffer
	if err := WritePDF(&buf, job, PDFOptions{}); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	if !bytes.HasPrefix(b, []byte("%PDF-1.4")) || !bytes.HasSuffix(b, []byte("%%EOF\n")) {
		t.Fatal("не PDF")
	}
	if !bytes.Contains(b, []byte("/Count 2")) || !bytes.Contains(b, []byte("/MediaBox [0 0 595 842]")) {
		t.Error("страницы или размер")
	}
	// Заголовок документа — из имени задания, в UTF-16BE.
	if !bytes.Contains(b, []byte("/Title <FEFF")) {
		t.Error("нет заголовка документа")
	}
	if err := WritePDF(&buf, Job{}, PDFOptions{}); err == nil {
		t.Error("пустое задание записано")
	}
}

func TestSavePDF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.pdf")
	if err := SavePDF(path, testJob(1), PDFOptions{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.HasPrefix(data, []byte("%PDF-")) {
		t.Fatalf("файл: %v", err)
	}
	// Перезапись существующего.
	if err := SavePDF(path, testJob(3), PDFOptions{}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !bytes.Contains(data, []byte("/Count 3")) {
		t.Error("файл не перезаписан")
	}
	// Временные файлы не остаются.
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Errorf("в каталоге %d файлов: %v", len(ents), ents)
	}

	// Ошибка не портит прежний файл и не оставляет мусора.
	before, _ := os.ReadFile(path)
	if err := SavePDF(path, Job{}, PDFOptions{}); err == nil {
		t.Error("пустое задание сохранено")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Error("неудачное сохранение изменило прежний файл")
	}
	if err := SavePDF(filepath.Join(dir, "нет-такой-папки", "x.pdf"), testJob(1), PDFOptions{}); err == nil {
		t.Error("запись в несуществующую папку удалась")
	}
	if err := SavePDF("", testJob(1), PDFOptions{}); err == nil {
		t.Error("пустой путь принят")
	}
	ents, _ = os.ReadDir(dir)
	if len(ents) != 1 {
		t.Errorf("после ошибок в каталоге %d файлов", len(ents))
	}
}
