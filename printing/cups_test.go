package printing

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

// fakeCUPS — тестовый «CUPS»: принимает IPP по HTTP, запоминает запросы и
// отвечает корректными IPP-сообщениями. Ничего настоящего не печатает и в
// настоящий CUPS не ходит.
type fakeCUPS struct {
	t  *testing.T
	mu sync.Mutex

	printers    []Printer
	def         string // принтер по умолчанию; "" — не назначен
	printStatus uint16 // ответ на Print-Job (0 — успех)
	httpStatus  int    // если не 0 — ответ HTTP вместо IPP
	garbage     bool   // отвечать не IPP

	reqs []fakeReq
}

type fakeReq struct {
	Path        string
	RawPath     string
	ContentType string
	Msg         *ippMessage
	Doc         []byte
}

func (f *fakeCUPS) last() fakeReq {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.reqs) == 0 {
		f.t.Fatal("сервер не получил ни одного запроса")
	}
	return f.reqs[len(f.reqs)-1]
}

func (f *fakeCUPS) count(op uint16) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.reqs {
		if r.Msg != nil && r.Msg.Code == op {
			n++
		}
	}
	return n
}

func (f *fakeCUPS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	msg, doc, err := parseIPP(body)
	if err != nil {
		f.t.Errorf("сервер не разобрал запрос клиента: %v", err)
		http.Error(w, "bad", 400)
		return
	}
	f.mu.Lock()
	f.reqs = append(f.reqs, fakeReq{
		Path: r.URL.Path, RawPath: r.URL.EscapedPath(), ContentType: r.Header.Get("Content-Type"),
		Msg: msg, Doc: doc,
	})
	f.mu.Unlock()

	if f.httpStatus != 0 {
		w.WriteHeader(f.httpStatus)
		return
	}
	if f.garbage {
		w.Header().Set("Content-Type", "application/ipp")
		w.Write([]byte("<html>это не IPP</html>"))
		return
	}

	resp := &ippMessage{Major: 2, Code: ippStatusOK, RequestID: msg.RequestID}
	og := resp.group(ippTagOperation)
	og.add(ippTagCharset, "attributes-charset", "utf-8")
	og.add(ippTagLanguage, "attributes-natural-language", "en")

	printerGroup := func(p Printer) {
		resp.Groups = append(resp.Groups, ippGroup{Tag: ippTagPrinter})
		g := &resp.Groups[len(resp.Groups)-1]
		g.add(ippTagName, "printer-name", p.Name)
		g.add(ippTagText, "printer-info", p.Description)
		g.add(ippTagText, "printer-location", p.Location)
		g.add(ippTagText, "printer-make-and-model", p.Model)
		g.addInt(ippTagEnum, "printer-state", int32(p.State))
		g.addBool("printer-is-accepting-jobs", p.Accepting)
	}

	switch msg.Code {
	case ippOpCUPSGetPrint:
		if len(f.printers) == 0 {
			resp.Code = ippStatusClientNotFound
		}
		for _, p := range f.printers {
			printerGroup(p)
		}
	case ippOpCUPSGetDef:
		found := false
		for _, p := range f.printers {
			if p.Name == f.def {
				printerGroup(p)
				found = true
			}
		}
		if !found {
			resp.Code = ippStatusClientNotFound
		}
	case ippOpPrintJob:
		if f.printStatus != 0 {
			resp.Code = f.printStatus
			resp.group(ippTagOperation).add(ippTagText, "status-message", "тест отказ")
			break
		}
		resp.Groups = append(resp.Groups, ippGroup{Tag: ippTagJob})
		jg := &resp.Groups[len(resp.Groups)-1]
		jg.addInt(ippTagInteger, "job-id", 42)
		jg.add(ippTagURI, "job-uri", "ipp://localhost/jobs/42")
	default:
		resp.Code = ippStatusServerUnsupportedO
	}
	out, err := resp.Marshal()
	if err != nil {
		f.t.Errorf("ответ не собран: %v", err)
		return
	}
	w.Header().Set("Content-Type", "application/ipp")
	w.Write(out)
}

// newFakeClient поднимает тестовый сервер и клиента к нему.
func newFakeClient(t *testing.T, f *fakeCUPS) *cupsClient {
	t.Helper()
	f.t = t
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return newCupsClient(cupsEndpoint{Host: strings.TrimPrefix(srv.URL, "http://")})
}

func testJob(pages int) Job {
	j := Job{Name: "Тестовый документ", Setup: PageSetup{Paper: A4, DPI: 72}}
	w, h := j.Setup.SheetSize()
	for i := 0; i < pages; i++ {
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), uint8(i * 40), 255})
			}
		}
		j.Pages = append(j.Pages, img)
	}
	return j
}

var testPrinters = []Printer{
	{Name: "Office", Description: "Принтер бухгалтерии", Location: "2 этаж", Model: "HP LaserJet 4", State: StateIdle, Accepting: true},
	{Name: "HP Laser", Description: "", Location: "", Model: "Brother", State: StateStopped, Accepting: false},
}

func TestCUPSListPrinters(t *testing.T) {
	f := &fakeCUPS{printers: testPrinters, def: "Office"}
	c := newFakeClient(t, f)
	got, err := c.listPrinters(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("принтеров %d: %+v", len(got), got)
	}
	want0 := testPrinters[0]
	want0.Default = true
	if got[0] != want0 {
		t.Errorf("принтер 0:\n got %+v\nwant %+v", got[0], want0)
	}
	if got[1].Default || got[1].Accepting || got[1].State != StateStopped || got[1].Name != "HP Laser" {
		t.Errorf("принтер 1: %+v", got[1])
	}

	// Запрос: нужная операция, путь, тип содержимого и перечень атрибутов.
	if n := f.count(ippOpCUPSGetPrint); n != 1 {
		t.Errorf("CUPS-Get-Printers отправлено %d раз", n)
	}
	var req fakeReq
	for _, r := range f.reqs {
		if r.Msg.Code == ippOpCUPSGetPrint {
			req = r
		}
	}
	if req.ContentType != "application/ipp" || req.Path != "/" {
		t.Errorf("HTTP: %q %q", req.ContentType, req.Path)
	}
	ra := req.Msg.find(ippTagOperation, "requested-attributes")
	if ra == nil || len(ra.Values) != len(cupsPrinterAttrs) {
		t.Errorf("requested-attributes: %+v", ra)
	}
	if len(req.Doc) != 0 {
		t.Errorf("у запроса-справки есть документ (%d байт)", len(req.Doc))
	}
}

func TestCUPSNoPrinters(t *testing.T) {
	f := &fakeCUPS{}
	c := newFakeClient(t, f)
	got, err := c.listPrinters(context.Background())
	if err != nil || len(got) != 0 {
		t.Errorf("пустой список: %v, %v", got, err)
	}
	_, err = c.defaultPrinter(context.Background())
	if !errors.Is(err, ErrNoPrinter) {
		t.Errorf("принтера по умолчанию нет: err = %v", err)
	}
}

func TestCUPSDefaultPrinter(t *testing.T) {
	f := &fakeCUPS{printers: testPrinters, def: "HP Laser"}
	c := newFakeClient(t, f)
	p, err := c.defaultPrinter(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "HP Laser" || !p.Default {
		t.Errorf("%+v", p)
	}
}

func TestCUPSPrintJobRequest(t *testing.T) {
	f := &fakeCUPS{printers: testPrinters, def: "Office"}
	c := newFakeClient(t, f)
	job := testJob(2)
	if err := printViaCUPS(context.Background(), c, Target{Printer: "HP Laser", Copies: 3, Collate: true}, job); err != nil {
		t.Fatal(err)
	}
	if f.count(ippOpCUPSGetDef) != 0 {
		t.Error("принтер задан явно, а клиент спрашивал принтер по умолчанию")
	}
	req := f.last()
	if req.Msg.Code != ippOpPrintJob {
		t.Fatalf("операция 0x%04x", req.Msg.Code)
	}
	// Имя с пробелом попадает в путь процентной записью, а не сырым пробелом.
	if req.RawPath != "/printers/HP%20Laser" || req.Path != "/printers/HP Laser" {
		t.Errorf("путь %q (%q)", req.RawPath, req.Path)
	}
	if req.ContentType != "application/ipp" {
		t.Errorf("Content-Type %q", req.ContentType)
	}
	op := func(name string) string { return req.Msg.find(ippTagOperation, name).Str() }
	if got := op("printer-uri"); !strings.HasPrefix(got, "ipp://127.0.0.1:") || !strings.HasSuffix(got, "/printers/HP%20Laser") {
		t.Errorf("printer-uri %q", got)
	}
	if op("requesting-user-name") == "" {
		t.Error("пустой requesting-user-name: CUPS отвергает такой запрос")
	}
	if op("job-name") != "Тестовый документ" {
		t.Errorf("job-name %q", op("job-name"))
	}
	if op("document-format") != "application/pdf" {
		t.Errorf("document-format %q", op("document-format"))
	}
	// Первые два атрибута — по RFC 8011.
	first := req.Msg.Groups[0].Attrs
	if first[0].Name != "attributes-charset" || first[1].Name != "attributes-natural-language" {
		t.Errorf("порядок преамбулы: %s, %s", first[0].Name, first[1].Name)
	}
	if v, ok := req.Msg.find(ippTagJob, "copies").Int(); !ok || v != 3 {
		t.Errorf("copies %v %v", v, ok)
	}
	if got := req.Msg.find(ippTagJob, "multiple-document-handling").Str(); got != "separate-documents-collated-copies" {
		t.Errorf("multiple-document-handling %q", got)
	}
	if got := req.Msg.find(ippTagJob, "media").Str(); got != "iso_a4_210x297mm" {
		t.Errorf("media %q", got)
	}

	// Документ — настоящий PDF на две страницы, вплотную за атрибутами.
	if !bytes.HasPrefix(req.Doc, []byte("%PDF-1.")) || !bytes.HasSuffix(req.Doc, []byte("%%EOF\n")) {
		t.Fatalf("документ не PDF: %q … %q", req.Doc[:min(10, len(req.Doc))], req.Doc[max(0, len(req.Doc)-10):])
	}
	if !bytes.Contains(req.Doc, []byte("/Count 2")) {
		t.Error("в PDF не две страницы")
	}
	// A4 при 72 dpi: 595×842 пикселя → 595×842 пт (MediaBox).
	if !bytes.Contains(req.Doc, []byte("/MediaBox [0 0 595 842]")) {
		t.Error("размер страницы PDF не A4")
	}
}

// TestCUPSSingleCopyOmitsCopies — одна копия не порождает лишних атрибутов
// задания про копии: сервер вправе отказать из-за «неподдерживаемого» атрибута.
func TestCUPSSingleCopyOmitsCopies(t *testing.T) {
	f := &fakeCUPS{printers: testPrinters, def: "Office"}
	c := newFakeClient(t, f)
	if err := printViaCUPS(context.Background(), c, Target{Printer: "Office", Copies: 1}, testJob(1)); err != nil {
		t.Fatal(err)
	}
	m := f.last().Msg
	if m.find(ippTagJob, "copies") != nil || m.find(ippTagJob, "multiple-document-handling") != nil {
		t.Errorf("лишние атрибуты копий: %+v", m.Groups)
	}
	// Несколько копий без комплектования — «по страницам».
	if err := printViaCUPS(context.Background(), c, Target{Printer: "Office", Copies: 2}, testJob(1)); err != nil {
		t.Fatal(err)
	}
	if got := f.last().Msg.find(ippTagJob, "multiple-document-handling").Str(); got != "separate-documents-uncollated-copies" {
		t.Errorf("multiple-document-handling %q", got)
	}
}

func TestCUPSPrintToDefault(t *testing.T) {
	f := &fakeCUPS{printers: testPrinters, def: "HP Laser"}
	c := newFakeClient(t, f)
	if err := printViaCUPS(context.Background(), c, Target{}, testJob(1)); err != nil {
		t.Fatal(err)
	}
	if f.count(ippOpCUPSGetDef) != 1 {
		t.Error("принтер по умолчанию не запрашивался")
	}
	if got := f.last().RawPath; got != "/printers/HP%20Laser" {
		t.Errorf("печать ушла на %q", got)
	}
}

func TestCUPSPrintToDefaultMissing(t *testing.T) {
	f := &fakeCUPS{printers: testPrinters, def: ""}
	c := newFakeClient(t, f)
	err := printViaCUPS(context.Background(), c, Target{}, testJob(1))
	if !errors.Is(err, ErrNoPrinter) {
		t.Errorf("err = %v", err)
	}
	if f.count(ippOpPrintJob) != 0 {
		t.Error("задание отправлено без принтера")
	}
}

func TestCUPSPrintJobID(t *testing.T) {
	f := &fakeCUPS{printers: testPrinters}
	c := newFakeClient(t, f)
	id, err := c.printJob(context.Background(), cupsJobOptions{Printer: "Office", Name: "x", User: "u"}, []byte("%PDF-1.4\n"))
	if err != nil || id != 42 {
		t.Errorf("id=%d err=%v", id, err)
	}
}

func TestCUPSErrors(t *testing.T) {
	job := testJob(1)
	cases := []struct {
		name string
		f    *fakeCUPS
		want string
		is   error
	}{
		{"очередь не принимает", &fakeCUPS{printStatus: ippStatusErrNotAccepting}, "server-error-not-accepting-jobs", nil},
		{"сообщение сервера", &fakeCUPS{printStatus: ippStatusClientNotPossible}, "тест отказ", nil},
		{"нет принтера", &fakeCUPS{printStatus: ippStatusClientNotFound}, "", ErrNoPrinter},
		{"формат не принят", &fakeCUPS{printStatus: ippStatusClientDocFormat}, "document-format-not-supported", nil},
		{"HTTP 401", &fakeCUPS{httpStatus: 401}, "аутентификац", nil},
		{"HTTP 426", &fakeCUPS{httpStatus: 426}, "шифрован", nil},
		{"HTTP 500", &fakeCUPS{httpStatus: 500}, "HTTP 500", nil},
		{"ответ не IPP", &fakeCUPS{garbage: true}, "не разобран", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cl := newFakeClient(t, c.f)
			err := printViaCUPS(context.Background(), cl, Target{Printer: "Office", Copies: 1}, job)
			if err == nil {
				t.Fatal("ошибки нет")
			}
			if c.want != "" && !strings.Contains(err.Error(), c.want) {
				t.Errorf("текст %q не содержит %q", err, c.want)
			}
			if c.is != nil && !errors.Is(err, c.is) {
				t.Errorf("%v не %v", err, c.is)
			}
		})
	}
}

// TestCUPSNotRunning — на порту никого нет: честная ошибка «служба печати
// недоступна», различимая через errors.Is.
func TestCUPSNotRunning(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	addr := ln.Addr().String()
	ln.Close() // порт освобождён: соединение будет отвергнуто
	c := newCupsClient(cupsEndpoint{Host: addr})
	_, err = c.listPrinters(context.Background())
	if !errors.Is(err, ErrNoPrintService) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), addr) {
		t.Errorf("в ошибке нет адреса: %v", err)
	}
	if err := printViaCUPS(context.Background(), c, Target{Printer: "x"}, testJob(1)); !errors.Is(err, ErrNoPrintService) {
		t.Errorf("печать: %v", err)
	}
}

// TestCUPSNoSocket — сокета нет вовсе.
func TestCUPSNoSocket(t *testing.T) {
	c := newCupsClient(cupsEndpoint{Socket: filepath.Join(t.TempDir(), "нет-такого.sock")})
	_, err := c.listPrinters(context.Background())
	if !errors.Is(err, ErrNoPrintService) {
		t.Errorf("err = %v", err)
	}
}

// TestCUPSOverUnixSocket — тот же обмен через Unix-сокет (путь, по которому
// ходит настоящий CUPS на современных дистрибутивах).
func TestCUPSOverUnixSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "cups.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("Unix-сокеты недоступны: %v", err)
	}
	f := &fakeCUPS{t: t, printers: testPrinters, def: "Office"}
	srv := &http.Server{Handler: f}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })

	c := newCupsClient(cupsEndpoint{Socket: sock})
	got, err := c.listPrinters(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("список через сокет: %v, %v", got, err)
	}
	if err := printViaCUPS(context.Background(), c, Target{Printer: "Office"}, testJob(1)); err != nil {
		t.Fatal(err)
	}
	// URI принтера через сокет — условный localhost без порта.
	if got := f.last().Msg.find(ippTagOperation, "printer-uri").Str(); got != "ipp://localhost/printers/Office" {
		t.Errorf("printer-uri %q", got)
	}
}

func TestDetectCUPS(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	none := func(string) bool { return false }
	only := func(p string) func(string) bool { return func(q string) bool { return p == q } }

	cases := []struct {
		name   string
		env    map[string]string
		exists func(string) bool
		want   cupsEndpoint
	}{
		{"по умолчанию", nil, none, cupsEndpoint{Host: "localhost:631"}},
		{"сокет /run", nil, only("/run/cups/cups.sock"), cupsEndpoint{Socket: "/run/cups/cups.sock"}},
		{"сокет /var/run", nil, only("/var/run/cups/cups.sock"), cupsEndpoint{Socket: "/var/run/cups/cups.sock"}},
		{"CUPS_SERVER хост", map[string]string{"CUPS_SERVER": "print.lan"}, only("/run/cups/cups.sock"), cupsEndpoint{Host: "print.lan:631"}},
		{"CUPS_SERVER хост:порт", map[string]string{"CUPS_SERVER": "10.0.0.5:8631"}, none, cupsEndpoint{Host: "10.0.0.5:8631"}},
		{"CUPS_SERVER с версией", map[string]string{"CUPS_SERVER": "print.lan:631/version=1.1"}, none, cupsEndpoint{Host: "print.lan:631"}},
		{"CUPS_SERVER сокет", map[string]string{"CUPS_SERVER": "/tmp/my.sock"}, none, cupsEndpoint{Socket: "/tmp/my.sock"}},
		{"CUPS_SERVER IPv6", map[string]string{"CUPS_SERVER": "[::1]"}, none, cupsEndpoint{Host: "[::1]:631"}},
		{"CUPS_SERVER пробелы", map[string]string{"CUPS_SERVER": "  "}, none, cupsEndpoint{Host: "localhost:631"}},
	}
	for _, c := range cases {
		if got := detectCUPS(env(c.env), c.exists); got != c.want {
			t.Errorf("%s: %+v, ждали %+v", c.name, got, c.want)
		}
	}
}

func TestIPPJobNameAndUser(t *testing.T) {
	if got := ippJobName(""); got == "" {
		t.Error("пустое имя не заменено")
	}
	long := strings.Repeat("я", 300) // 600 байт
	got := ippJobName(long)
	if len(got) > 255 || !utf8.ValidString(got) || len(got) < 254 {
		t.Errorf("обрезка: %d байт, валидно=%v", len(got), utf8.ValidString(got))
	}
	if ippJobName("abc") != "abc" {
		t.Error("короткое имя изменено")
	}
	if stripDomain(`CORP\ivanov`) != "ivanov" || stripDomain("ivanov") != "ivanov" {
		t.Error("stripDomain")
	}
	if currentUserName() == "" {
		t.Error("имя пользователя пусто")
	}
}
