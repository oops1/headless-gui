// cups.go — клиент CUPS: перечень принтеров, принтер по умолчанию и печать PDF
// запросом IPP по HTTP. Работает поверх ipp.go и net/http; платформенных вызовов
// нет, поэтому код компилируется и проверяется тестами везде, а подключается только
// на Linux (printing_linux.go).
//
// Как это устроено. CUPS слушает HTTP на localhost:631 и на локальном сокете
// /run/cups/cups.sock. Запрос IPP — это тело POST с типом application/ipp; для
// Print-Job сразу за атрибутами идут байты документа. Так принтерами пользуется
// сам lp, и эта же дорога открыта любому клиенту: библиотека libcups не нужна.
package printing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"strings"
	"time"
	"unicode/utf8"
)

// cupsEndpoint — где искать CUPS: сокет или TCP-адрес.
type cupsEndpoint struct {
	// Socket — путь Unix-сокета; если задан, сеть не используется.
	Socket string
	// Host — «хост:порт» для TCP.
	Host string
}

// Стандартные места сокета CUPS: /run — современные дистрибутивы, /var/run —
// старые и те, где /var/run не ссылка на /run.
var cupsSocketPaths = []string{"/run/cups/cups.sock", "/var/run/cups/cups.sock"}

const cupsDefaultHost = "localhost:631"

// detectCUPS выбирает адрес CUPS: переменная CUPS_SERVER (её уважают и lp, и
// остальные клиенты CUPS: «хост», «хост:порт» или путь к сокету), затем
// локальный сокет, если он есть, затем localhost:631.
//
// Сокет предпочтительнее TCP: к нему CUPS применяет проверку по учётной записи
// процесса (peer credentials) и принимает локального пользователя без пароля, а
// на 631 тот же запрос от «чужого» хоста мог бы потребовать аутентификацию.
// getenv и exists подменяются в тестах.
func detectCUPS(getenv func(string) string, exists func(string) bool) cupsEndpoint {
	if s := strings.TrimSpace(getenv("CUPS_SERVER")); s != "" {
		if strings.HasPrefix(s, "/") {
			return cupsEndpoint{Socket: s}
		}
		// «хост:порт/version=1.1» — суффикс после '/' относится к версии
		// протокола старых клиентов и для адреса значения не имеет.
		if i := strings.IndexByte(s, '/'); i >= 0 {
			s = s[:i]
		}
		if _, _, err := net.SplitHostPort(s); err != nil {
			s = net.JoinHostPort(strings.Trim(s, "[]"), "631")
		}
		return cupsEndpoint{Host: s}
	}
	for _, p := range cupsSocketPaths {
		if exists(p) {
			return cupsEndpoint{Socket: p}
		}
	}
	return cupsEndpoint{Host: cupsDefaultHost}
}

func osExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode()&os.ModeSocket != 0
}

// cupsClient — клиент одного сервера CUPS.
type cupsClient struct {
	ep   cupsEndpoint
	http *http.Client
	// hostPort — как адрес сервера пишется в URI принтера и в Host.
	hostPort string
}

// newCupsClient создаёт клиента. Прокси из окружения отключены: HTTP_PROXY
// относится к походам в интернет, а не к локальной службе печати, и без этого
// запросы к CUPS на другой машине уходили бы к прокси, который IPP не понимает.
func newCupsClient(ep cupsEndpoint) *cupsClient {
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	c := &cupsClient{ep: ep, hostPort: ep.Host}
	if ep.Socket != "" {
		sock := ep.Socket
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		}
		// Через сокет адрес в URL условный: соединение всё равно идёт в сокет.
		c.hostPort = "localhost"
	}
	c.http = &http.Client{Transport: tr}
	return c
}

// roundTrip отправляет IPP-запрос (и данные документа после него) на ресурс
// сервера и возвращает разобранный ответ. Ошибку статуса IPP не возвращает:
// её разбирает вызывающий через msg.check(), так как для «принтера по умолчанию
// нет» это не сбой, а обычный ответ.
func (c *cupsClient) roundTrip(ctx context.Context, resource string, req *ippMessage, doc []byte) (*ippMessage, error) {
	head, err := req.Marshal()
	if err != nil {
		return nil, err
	}
	body := head
	if len(doc) > 0 {
		body = make([]byte, 0, len(head)+len(doc))
		body = append(append(body, head...), doc...)
	}
	u := "http://" + c.hostPort + resource
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/ipp")
	hreq.Header.Set("Accept", "application/ipp")
	resp, err := c.http.Do(hreq)
	if err != nil {
		return nil, c.wrapNetErr(err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("printing: CUPS требует аутентификацию или отказал в доступе (HTTP %d)", resp.StatusCode)
	case http.StatusUpgradeRequired:
		return nil, errors.New("printing: CUPS требует шифрованного соединения (HTTP 426); это не локальный сервер печати")
	default:
		return nil, fmt.Errorf("printing: CUPS ответил HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	// Предел чтения: ответ списка принтеров — килобайты; поток без конца от
	// «сервера» на 631 не должен съесть память.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("printing: чтение ответа CUPS: %w", err)
	}
	msg, _, err := parseIPP(data)
	if err != nil {
		return nil, fmt.Errorf("printing: ответ CUPS не разобран: %w", err)
	}
	return msg, nil
}

// wrapNetErr различает «CUPS нет» (соединение не установлено) и прочие сбои сети:
// в первом случае приложению надо сказать «служба печати не запущена», в
// остальных — показать причину.
func (c *cupsClient) wrapNetErr(err error) error {
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "dial" {
		where := c.ep.Host
		if c.ep.Socket != "" {
			where = c.ep.Socket
		}
		return fmt.Errorf("%w: CUPS не отвечает на %s (не установлен или не запущен): %v", ErrNoPrintService, where, op.Err)
	}
	return fmt.Errorf("printing: запрос к CUPS: %w", err)
}

// printerURI — идентификатор принтера для атрибута printer-uri.
func (c *cupsClient) printerURI(name string) string {
	return "ipp://" + c.hostPort + "/printers/" + url.PathEscape(name)
}

// Атрибуты принтера, которые нужны для списка. Запрашиваем перечень, а не всё:
// полный набор принтера CUPS — это сотни атрибутов с вложенными коллекциями
// (media-col-database и подобные), и ответ вырастает до сотен килобайт на принтер.
var cupsPrinterAttrs = []string{
	"printer-name", "printer-info", "printer-location", "printer-make-and-model",
	"printer-state", "printer-is-accepting-jobs",
}

// parsePrinter строит Printer из группы атрибутов принтера.
func parsePrinter(g *ippGroup) (Printer, bool) {
	name := g.find("printer-name").Str()
	if name == "" {
		return Printer{}, false
	}
	p := Printer{
		Name:        name,
		Description: g.find("printer-info").Str(),
		Location:    g.find("printer-location").Str(),
		Model:       g.find("printer-make-and-model").Str(),
		// Принимает ли очередь задания — если сервер не сообщил, считаем, что
		// да: иначе принтер без этого атрибута был бы навсегда «выключен».
		Accepting: true,
	}
	if st, ok := g.find("printer-state").Int(); ok {
		p.State = PrinterState(st)
	}
	if acc, ok := g.find("printer-is-accepting-jobs").Bool(); ok {
		p.Accepting = acc
	}
	return p, true
}

// listPrinters — CUPS-Get-Printers. Принтер по умолчанию помечается отдельным
// запросом; его сбой не мешает списку.
func (c *cupsClient) listPrinters(ctx context.Context) ([]Printer, error) {
	req := newIPPRequest(ippOpCUPSGetPrint)
	req.group(ippTagOperation).addMulti(ippTagKeyword, "requested-attributes", cupsPrinterAttrs...)
	msg, err := c.roundTrip(ctx, "/", req, nil)
	if err != nil {
		return nil, err
	}
	if err := msg.check(); err != nil {
		// «not-found» здесь — просто «принтеров нет», а не ошибка.
		var ie *ippError
		if errors.As(err, &ie) && ie.Code == ippStatusClientNotFound {
			return nil, nil
		}
		return nil, err
	}
	var out []Printer
	for _, g := range msg.groupsWithTag(ippTagPrinter) {
		if p, ok := parsePrinter(g); ok {
			out = append(out, p)
		}
	}
	if def, err := c.defaultPrinter(ctx); err == nil {
		for i := range out {
			out[i].Default = out[i].Name == def.Name
		}
	}
	return out, nil
}

// defaultPrinter — CUPS-Get-Default. Если принтеров нет или по умолчанию ничего не
// назначено, CUPS отвечает client-error-not-found: это ErrNoPrinter.
func (c *cupsClient) defaultPrinter(ctx context.Context) (Printer, error) {
	req := newIPPRequest(ippOpCUPSGetDef)
	req.group(ippTagOperation).addMulti(ippTagKeyword, "requested-attributes", cupsPrinterAttrs...)
	msg, err := c.roundTrip(ctx, "/", req, nil)
	if err != nil {
		return Printer{}, err
	}
	if err := msg.check(); err != nil {
		var ie *ippError
		if errors.As(err, &ie) && ie.Code == ippStatusClientNotFound {
			return Printer{}, fmt.Errorf("%w: принтер по умолчанию не назначен", ErrNoPrinter)
		}
		return Printer{}, err
	}
	for _, g := range msg.groupsWithTag(ippTagPrinter) {
		if p, ok := parsePrinter(g); ok {
			p.Default = true
			return p, nil
		}
	}
	return Printer{}, fmt.Errorf("%w: принтер по умолчанию не назначен", ErrNoPrinter)
}

// cupsJobOptions — то, что можно передать заданию помимо документа.
type cupsJobOptions struct {
	Printer string
	Name    string
	User    string
	Copies  int
	Collate bool
	Media   string // PWG-имя размера бумаги; "" — не передавать
}

// buildPrintJob собирает запрос Print-Job (без данных документа). Вынесено
// отдельно ради теста: правильность атрибутов проверяется по байтам, без сервера.
func (c *cupsClient) buildPrintJob(o cupsJobOptions) *ippMessage {
	req := newIPPRequest(ippOpPrintJob)
	// Указатель на группу действителен до следующего добавления группы:
	// слайс групп при росте перевыделяется. Поэтому группа операции
	// заполняется целиком до создания группы заданий.
	og := req.group(ippTagOperation)
	og.add(ippTagURI, "printer-uri", c.printerURI(o.Printer))
	og.add(ippTagName, "requesting-user-name", o.User)
	og.add(ippTagName, "job-name", o.Name)
	og.add(ippTagMimeMedia, "document-format", "application/pdf")

	if o.Copies > 1 || o.Media != "" {
		jg := req.group(ippTagJob)
		if o.Copies > 1 {
			jg.addInt(ippTagInteger, "copies", int32(o.Copies))
			// Для копий одного документа порядок — «по комплектам» или «по
			// страницам» — задаётся именно этим атрибутом (RFC 8011, 5.2.4).
			h := "separate-documents-uncollated-copies"
			if o.Collate {
				h = "separate-documents-collated-copies"
			}
			jg.add(ippTagKeyword, "multiple-document-handling", h)
		}
		if o.Media != "" {
			jg.add(ippTagKeyword, "media", o.Media)
		}
	}
	return req
}

// printJob отправляет PDF и возвращает номер задания в очереди.
func (c *cupsClient) printJob(ctx context.Context, o cupsJobOptions, pdfData []byte) (int, error) {
	req := c.buildPrintJob(o)
	msg, err := c.roundTrip(ctx, "/printers/"+url.PathEscape(o.Printer), req, pdfData)
	if err != nil {
		return 0, err
	}
	if err := msg.check(); err != nil {
		var ie *ippError
		if errors.As(err, &ie) && ie.Code == ippStatusClientNotFound {
			return 0, fmt.Errorf("%w: %q", ErrNoPrinter, o.Printer)
		}
		return 0, err
	}
	id := 0
	if jg := msg.groupsWithTag(ippTagJob); len(jg) > 0 {
		if v, ok := jg[0].find("job-id").Int(); ok {
			id = int(v)
		}
	}
	return id, nil
}

// printViaCUPS — печать задания на CUPS целиком: выбор принтера, PDF, Print-Job.
func printViaCUPS(ctx context.Context, c *cupsClient, t Target, job Job) error {
	name := t.Printer
	if name == "" {
		def, err := c.defaultPrinter(ctx)
		if err != nil {
			return err
		}
		name = def.Name
	}
	var pdfBuf bytes.Buffer
	if err := WritePDF(&pdfBuf, job, PDFOptions{}); err != nil {
		return err
	}
	opts := cupsJobOptions{
		Printer: name,
		Name:    ippJobName(job.Name),
		User:    currentUserName(),
		Copies:  t.Copies,
		Collate: t.Collate,
		Media:   job.Setup.Paper.PWGName(),
	}
	_, err := c.printJob(ctx, opts, pdfBuf.Bytes())
	return err
}

// ippJobName — имя задания для очереди: пустое заменяется общим, длинное
// обрезается до 255 байт по границе символа (предел значения name в IPP; сервер
// на более длинное отвечает отказом всего запроса).
func ippJobName(s string) string {
	if s == "" {
		return "headless-gui"
	}
	const max = 255
	if len(s) <= max {
		return s
	}
	s = s[:max]
	for !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// currentUserName — имя пользователя для requesting-user-name. CUPS по нему
// показывает владельца задания и по нему же разрешает отмену. Без него запрос
// отвергается, а «anonymous» хуже настоящего имени только тем, что чужие задания
// от своих не отличить.
func currentUserName() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return stripDomain(u.Username)
	}
	for _, k := range []string{"USER", "LOGNAME", "USERNAME"} {
		if v := os.Getenv(k); v != "" {
			return stripDomain(v)
		}
	}
	return "anonymous"
}

// stripDomain убирает «ДОМЕН\» из имени: в IPP имя пользователя без домена.
func stripDomain(s string) string {
	if i := strings.LastIndexByte(s, '\\'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// Таймауты. Запросы-справки быстрые, и зависший локальный CUPS не должен
// подвешивать приложение; печать включает передачу документа и ожидание, пока
// CUPS примет его, — это дольше.
const (
	cupsQueryTimeout = 10 * time.Second
	cupsPrintTimeout = 2 * time.Minute
)
