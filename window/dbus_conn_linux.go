//go:build linux && !android

// dbus_conn_linux.go — транспорт D-Bus: unix-сокет, SASL-рукопожатие,
// маршрутизация ответов и сигналов, экспорт объектов (серверная сторона).
//
// Зачем своё: правило zero new deps. Всё, что нужно уведомлениям
// (org.freedesktop.Notifications) и мосту доступности (AT-SPI), — это
// method_call/reply, сигналы и приём чужих вызовов. Ровно это здесь и есть.
package window

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// dbusMaxMessage — потолок размера сообщения по спецификации (128 МиБ).
// Защищает от аллокации по испорченной длине.
const dbusMaxMessage = 128 * 1024 * 1024

// dbusCallTimeout — сколько ждём ответ на method_call по умолчанию.
const dbusCallTimeout = 5 * time.Second

// dbusReply — результат обработки ВХОДЯЩЕГО вызова экспортированного объекта.
// Непустой ErrName превращается в ответ-ошибку.
type dbusReply struct {
	Sig     string
	Body    []any
	ErrName string
	ErrMsg  string
	// NoReply — вызов обработан, но отвечать не нужно (сигналоподобные методы).
	NoReply bool
}

// dbusConn — соединение с шиной D-Bus.
type dbusConn struct {
	conn   net.Conn
	rd     *bufio.Reader
	unique string // уникальное имя, выданное шиной (:1.42)

	sendMu sync.Mutex
	serial uint32

	pendMu  sync.Mutex
	pending map[uint32]chan *dbusMessage

	hMu     sync.RWMutex
	signals []func(*dbusMessage)
	onCall  func(*dbusMessage) *dbusReply

	closed  atomic.Bool
	closeCh chan struct{}
	err     atomic.Pointer[error]
}

// ─── Подключение ─────────────────────────────────────────────────────────────

// dbusSessionAddress возвращает адрес сессионной шины: DBUS_SESSION_BUS_ADDRESS,
// иначе стандартный сокет /run/user/<uid>/bus.
func dbusSessionAddress() string {
	if a := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" {
		return a
	}
	return "unix:path=/run/user/" + strconv.Itoa(os.Getuid()) + "/bus"
}

// dbusParseAddress выбирает первый пригодный unix-адрес из списка (адреса
// разделены ';'). Возвращает путь для net.Dial: абстрактный сокет — с '@'.
func dbusParseAddress(addr string) (string, error) {
	for _, part := range strings.Split(addr, ";") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "unix:") {
			continue
		}
		var path, abstract string
		for _, kv := range strings.Split(strings.TrimPrefix(part, "unix:"), ",") {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				continue
			}
			switch k {
			case "path":
				path = dbusUnescape(v)
			case "abstract":
				abstract = dbusUnescape(v)
			}
		}
		if abstract != "" {
			return "@" + abstract, nil // Go: '@' → абстрактное пространство имён
		}
		if path != "" {
			return path, nil
		}
	}
	return "", fmt.Errorf("dbus: в адресе %q нет unix-сокета", addr)
}

// dbusUnescape раскрывает %XX в значениях адреса шины.
func dbusUnescape(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// dbusDial подключается к шине по адресу, проходит SASL EXTERNAL и Hello.
func dbusDial(addr string) (*dbusConn, error) {
	sock, err := dbusParseAddress(addr)
	if err != nil {
		return nil, err
	}
	nc, err := net.DialTimeout("unix", sock, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dbus: подключение к %s: %w", sock, err)
	}
	c := &dbusConn{
		conn:    nc,
		rd:      bufio.NewReaderSize(nc, 8192),
		pending: map[uint32]chan *dbusMessage{},
		closeCh: make(chan struct{}),
	}
	if err := c.authenticate(); err != nil {
		nc.Close()
		return nil, err
	}
	go c.readLoop()

	reply, err := c.call("org.freedesktop.DBus", "/org/freedesktop/DBus",
		"org.freedesktop.DBus", "Hello", "", nil)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("dbus: Hello: %w", err)
	}
	if len(reply.Body) > 0 {
		c.unique, _ = reply.Body[0].(string)
	}
	return c, nil
}

// authenticate выполняет текстовое SASL-рукопожатие. Порядок по спецификации:
// нулевой байт (иначе сервер не примет credentials), AUTH EXTERNAL с uid в hex,
// BEGIN. При отказе пробуем ANONYMOUS — так работают некоторые прокси-шины.
func (c *dbusConn) authenticate() error {
	if err := c.conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	defer c.conn.SetDeadline(time.Time{})

	if _, err := c.conn.Write([]byte{0}); err != nil {
		return fmt.Errorf("dbus: NUL: %w", err)
	}
	uidHex := fmt.Sprintf("%X", []byte(strconv.Itoa(os.Getuid())))
	resp, err := c.authCmd("AUTH EXTERNAL " + uidHex)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resp, "OK") {
		if resp, err = c.authCmd("AUTH ANONYMOUS 6865616465"); err != nil {
			return err
		}
		if !strings.HasPrefix(resp, "OK") {
			return fmt.Errorf("dbus: аутентификация отклонена: %s", resp)
		}
	}
	if _, err := c.conn.Write([]byte("BEGIN\r\n")); err != nil {
		return fmt.Errorf("dbus: BEGIN: %w", err)
	}
	return nil
}

// authCmd отправляет строку команды SASL и читает ответную строку.
func (c *dbusConn) authCmd(cmd string) (string, error) {
	if _, err := c.conn.Write([]byte(cmd + "\r\n")); err != nil {
		return "", fmt.Errorf("dbus: %s: %w", cmd, err)
	}
	line, err := c.rd.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("dbus: ответ на %s: %w", cmd, err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// ─── Приём сообщений ─────────────────────────────────────────────────────────

// readLoop читает сообщения до закрытия соединения и раскладывает их:
// ответы — ожидающим вызовам, сигналы — подписчикам, чужие вызовы —
// обработчику экспортированных объектов.
func (c *dbusConn) readLoop() {
	for {
		msg, err := c.readMessage()
		if err != nil {
			c.fail(err)
			return
		}
		switch msg.Type {
		case dbusTypeMethodReturn, dbusTypeError:
			c.pendMu.Lock()
			ch := c.pending[msg.ReplySerial]
			delete(c.pending, msg.ReplySerial)
			c.pendMu.Unlock()
			if ch != nil {
				ch <- msg
			}
		case dbusTypeSignal:
			c.hMu.RLock()
			hs := append([]func(*dbusMessage){}, c.signals...)
			c.hMu.RUnlock()
			for _, h := range hs {
				h(msg)
			}
		case dbusTypeMethodCall:
			c.hMu.RLock()
			h := c.onCall
			c.hMu.RUnlock()
			go c.serveCall(h, msg)
		}
	}
}

// serveCall отвечает на входящий вызов. Обработчик выполняется в отдельной
// горутине, чтобы медленный ответ не останавливал чтение сокета.
func (c *dbusConn) serveCall(h func(*dbusMessage) *dbusReply, msg *dbusMessage) {
	var rep *dbusReply
	if h != nil {
		rep = h(msg)
	}
	if rep == nil {
		rep = &dbusReply{
			ErrName: "org.freedesktop.DBus.Error.UnknownMethod",
			ErrMsg:  fmt.Sprintf("нет метода %s.%s у %s", msg.Interface, msg.Member, msg.Path),
		}
	}
	if rep.NoReply || msg.Flags&dbusFlagNoReplyExpected != 0 {
		return
	}
	out := &dbusMessage{Destination: msg.Sender, ReplySerial: msg.Serial}
	if rep.ErrName != "" {
		out.Type = dbusTypeError
		out.ErrorName = rep.ErrName
		out.Sig = "s"
		out.Body = []any{rep.ErrMsg}
	} else {
		out.Type = dbusTypeMethodReturn
		out.Sig = rep.Sig
		out.Body = rep.Body
	}
	if err := c.send(out); err != nil && !c.closed.Load() {
		fmt.Fprintf(os.Stderr, "dbus: не отправлен ответ на %s.%s: %v\n", msg.Interface, msg.Member, err)
	}
}

// readMessage читает одно сообщение целиком.
func (c *dbusConn) readMessage() (*dbusMessage, error) {
	head := make([]byte, dbusFixedHeader)
	if _, err := io.ReadFull(c.rd, head); err != nil {
		return nil, err
	}
	total, ok := dbusMessageLen(head)
	if !ok || total < dbusFixedHeader || total > dbusMaxMessage {
		return nil, fmt.Errorf("dbus: некорректная длина сообщения %d", total)
	}
	buf := make([]byte, total)
	copy(buf, head)
	if _, err := io.ReadFull(c.rd, buf[dbusFixedHeader:]); err != nil {
		return nil, err
	}
	return dbusUnmarshal(buf)
}

// fail закрывает соединение с ошибкой и будит всех ожидающих.
func (c *dbusConn) fail(err error) {
	if c.closed.Swap(true) {
		return
	}
	c.err.Store(&err)
	c.conn.Close()
	close(c.closeCh)
	c.pendMu.Lock()
	for serial, ch := range c.pending {
		delete(c.pending, serial)
		close(ch)
	}
	c.pendMu.Unlock()
}

// Close закрывает соединение.
func (c *dbusConn) Close() { c.fail(errors.New("dbus: соединение закрыто")) }

// ─── Отправка ────────────────────────────────────────────────────────────────

// send сериализует и пишет сообщение, назначая serial (если не задан).
func (c *dbusConn) send(m *dbusMessage) error {
	if c.closed.Load() {
		if p := c.err.Load(); p != nil {
			return *p
		}
		return errors.New("dbus: соединение закрыто")
	}
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if m.Serial == 0 {
		c.serial++
		if c.serial == 0 {
			c.serial = 1
		}
		m.Serial = c.serial
	}
	raw, err := m.marshal()
	if err != nil {
		return err
	}
	_, err = c.conn.Write(raw)
	return err
}

// nextSerial резервирует serial под вызов (нужен до отправки, чтобы
// зарегистрировать ожидание ответа без гонки с читающей горутиной).
func (c *dbusConn) nextSerial() uint32 {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	c.serial++
	if c.serial == 0 {
		c.serial = 1
	}
	return c.serial
}

// call выполняет method_call и ждёт ответ. Ответ-ошибка возвращается как error.
func (c *dbusConn) call(dest, path, iface, member, sig string, args []any) (*dbusMessage, error) {
	return c.callTimeout(dest, path, iface, member, sig, args, dbusCallTimeout)
}

func (c *dbusConn) callTimeout(dest, path, iface, member, sig string, args []any, timeout time.Duration) (*dbusMessage, error) {
	msg := &dbusMessage{
		Type: dbusTypeMethodCall, Serial: c.nextSerial(),
		Path: path, Interface: iface, Member: member, Destination: dest,
		Sig: sig, Body: args,
	}
	ch := make(chan *dbusMessage, 1)
	c.pendMu.Lock()
	c.pending[msg.Serial] = ch
	c.pendMu.Unlock()

	if err := c.send(msg); err != nil {
		c.pendMu.Lock()
		delete(c.pending, msg.Serial)
		c.pendMu.Unlock()
		return nil, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case reply, ok := <-ch:
		if !ok || reply == nil {
			if p := c.err.Load(); p != nil {
				return nil, *p
			}
			return nil, errors.New("dbus: соединение закрыто до ответа")
		}
		if reply.Type == dbusTypeError {
			return reply, errors.New(reply.errorText())
		}
		return reply, nil
	case <-timer.C:
		c.pendMu.Lock()
		delete(c.pending, msg.Serial)
		c.pendMu.Unlock()
		return nil, fmt.Errorf("dbus: таймаут вызова %s.%s", iface, member)
	}
}

// callNoReply отправляет вызов, не ожидая ответа (NO_REPLY_EXPECTED).
func (c *dbusConn) callNoReply(dest, path, iface, member, sig string, args []any) error {
	return c.send(&dbusMessage{
		Type: dbusTypeMethodCall, Flags: dbusFlagNoReplyExpected,
		Path: path, Interface: iface, Member: member, Destination: dest,
		Sig: sig, Body: args,
	})
}

// emit отправляет сигнал.
func (c *dbusConn) emit(path, iface, member, sig string, args []any) error {
	return c.send(&dbusMessage{
		Type: dbusTypeSignal, Path: path, Interface: iface, Member: member,
		Sig: sig, Body: args,
	})
}

// ─── Подписки и экспорт ──────────────────────────────────────────────────────

// onSignal добавляет обработчик сигналов (вызывается в горутине чтения —
// внутри нельзя блокироваться и нельзя вызывать call на этом же соединении).
func (c *dbusConn) onSignal(fn func(*dbusMessage)) {
	c.hMu.Lock()
	c.signals = append(c.signals, fn)
	c.hMu.Unlock()
}

// setCallHandler задаёт обработчик ВХОДЯЩИХ вызовов (экспортированные объекты).
func (c *dbusConn) setCallHandler(fn func(*dbusMessage) *dbusReply) {
	c.hMu.Lock()
	c.onCall = fn
	c.hMu.Unlock()
}

// addMatch просит шину доставлять сигналы, подходящие под правило.
func (c *dbusConn) addMatch(rule string) error {
	_, err := c.call("org.freedesktop.DBus", "/org/freedesktop/DBus",
		"org.freedesktop.DBus", "AddMatch", "s", []any{rule})
	return err
}

// requestName просит у шины имя (например org.a11y.atspi.Application).
// flags: 0x4 — DO_NOT_QUEUE, 0x1 — ALLOW_REPLACEMENT.
func (c *dbusConn) requestName(name string, flags uint32) error {
	reply, err := c.call("org.freedesktop.DBus", "/org/freedesktop/DBus",
		"org.freedesktop.DBus", "RequestName", "su", []any{name, flags})
	if err != nil {
		return err
	}
	if len(reply.Body) == 0 {
		return errors.New("dbus: RequestName без результата")
	}
	// 1 = PRIMARY_OWNER, 4 = ALREADY_OWNER — оба нас устраивают.
	if code, _ := reply.Body[0].(uint32); code != 1 && code != 4 {
		return fmt.Errorf("dbus: имя %s не получено (код %d)", name, code)
	}
	return nil
}

// nameHasOwner — есть ли на шине владелец имени (жив ли сервис).
func (c *dbusConn) nameHasOwner(name string) bool {
	reply, err := c.call("org.freedesktop.DBus", "/org/freedesktop/DBus",
		"org.freedesktop.DBus", "NameHasOwner", "s", []any{name})
	if err != nil || len(reply.Body) == 0 {
		return false
	}
	has, _ := reply.Body[0].(bool)
	return has
}

// ─── Сессионная шина (общая на процесс) ──────────────────────────────────────

var (
	dbusSessionOnce sync.Once
	dbusSessionConn *dbusConn
	dbusSessionErr  error
)

// dbusSession возвращает общее подключение к сессионной шине (ленивое,
// одно на процесс). Ошибка кэшируется: на машине без шины не долбимся в сокет
// на каждое уведомление.
func dbusSession() (*dbusConn, error) {
	dbusSessionOnce.Do(func() {
		dbusSessionConn, dbusSessionErr = dbusDial(dbusSessionAddress())
	})
	if dbusSessionErr != nil {
		return nil, dbusSessionErr
	}
	if dbusSessionConn != nil && dbusSessionConn.closed.Load() {
		return nil, errors.New("dbus: сессионная шина отключилась")
	}
	return dbusSessionConn, nil
}
