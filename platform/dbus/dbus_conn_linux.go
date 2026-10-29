//go:build linux && !android

// dbus_conn_linux.go — транспорт D-Bus: unix-сокет, SASL-рукопожатие,
// маршрутизация ответов и сигналов, экспорт объектов (серверная сторона).
//
// Зачем своё: правило zero new deps. Всё, что нужно уведомлениям
// (org.freedesktop.Notifications) и мосту доступности (AT-SPI), — это
// method_call/reply, сигналы и приём чужих вызовов. Ровно это здесь и есть.

package dbus

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

// Reply — результат обработки ВХОДЯЩЕГО вызова экспортированного объекта.
// Непустой ErrName превращается в ответ-ошибку.
type Reply struct {
	Sig     string
	Body    []any
	ErrName string
	ErrMsg  string
	// NoReply — вызов обработан, но отвечать не нужно (сигналоподобные методы).
	NoReply bool
}

// Conn — соединение с шиной D-Bus.
type Conn struct {
	conn   net.Conn
	rd     *bufio.Reader
	Unique string // уникальное имя, выданное шиной (:1.42)

	sendMu sync.Mutex
	serial uint32

	pendMu  sync.Mutex
	pending map[uint32]chan *Message

	hMu     sync.RWMutex
	signals []func(*Message)
	onCall  func(*Message) *Reply

	closed  atomic.Bool
	closeCh chan struct{}
	err     atomic.Pointer[error]
}

// ─── Подключение ─────────────────────────────────────────────────────────────

// SessionAddress возвращает адрес сессионной шины: DBUS_SESSION_BUS_ADDRESS,
// иначе стандартный сокет /run/user/<uid>/bus.
func SessionAddress() string {
	if a := os.Getenv("DBUS_SESSION_BUS_ADDRESS"); a != "" {
		return a
	}
	return "unix:path=/run/user/" + strconv.Itoa(os.Getuid()) + "/bus"
}

// ParseAddress выбирает первый пригодный unix-адрес из списка (адреса
// разделены ';'). Возвращает путь для net.Dial: абстрактный сокет — с '@'.
func ParseAddress(addr string) (string, error) {
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

// Dial подключается к шине по адресу, проходит SASL EXTERNAL и Hello.
func Dial(addr string) (*Conn, error) {
	sock, err := ParseAddress(addr)
	if err != nil {
		return nil, err
	}
	nc, err := net.DialTimeout("unix", sock, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("dbus: подключение к %s: %w", sock, err)
	}
	c := &Conn{
		conn:    nc,
		rd:      bufio.NewReaderSize(nc, 8192),
		pending: map[uint32]chan *Message{},
		closeCh: make(chan struct{}),
	}
	if err := c.authenticate(); err != nil {
		nc.Close()
		return nil, err
	}
	go c.readLoop()

	reply, err := c.Call("org.freedesktop.DBus", "/org/freedesktop/DBus",
		"org.freedesktop.DBus", "Hello", "", nil)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("dbus: Hello: %w", err)
	}
	if len(reply.Body) > 0 {
		c.Unique, _ = reply.Body[0].(string)
	}
	return c, nil
}

// authenticate выполняет текстовое SASL-рукопожатие. Порядок по спецификации:
// нулевой байт (иначе сервер не примет credentials), AUTH EXTERNAL с uid в hex,
// BEGIN. При отказе пробуем ANONYMOUS — так работают некоторые прокси-шины.
func (c *Conn) authenticate() error {
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
func (c *Conn) authCmd(cmd string) (string, error) {
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
func (c *Conn) readLoop() {
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
		case TypeSignal:
			c.hMu.RLock()
			hs := append([]func(*Message){}, c.signals...)
			c.hMu.RUnlock()
			for _, h := range hs {
				h(msg)
			}
		case TypeMethodCall:
			c.hMu.RLock()
			h := c.onCall
			c.hMu.RUnlock()
			go c.serveCall(h, msg)
		}
	}
}

// serveCall отвечает на входящий вызов. Обработчик выполняется в отдельной
// горутине, чтобы медленный ответ не останавливал чтение сокета.
func (c *Conn) serveCall(h func(*Message) *Reply, msg *Message) {
	var rep *Reply
	if h != nil {
		rep = h(msg)
	}
	if rep == nil {
		rep = &Reply{
			ErrName: "org.freedesktop.DBus.Error.UnknownMethod",
			ErrMsg:  fmt.Sprintf("нет метода %s.%s у %s", msg.Interface, msg.Member, msg.Path),
		}
	}
	if rep.NoReply || msg.Flags&dbusFlagNoReplyExpected != 0 {
		return
	}
	out := &Message{Destination: msg.Sender, ReplySerial: msg.Serial}
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
func (c *Conn) readMessage() (*Message, error) {
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
	return Unmarshal(buf)
}

// fail закрывает соединение с ошибкой и будит всех ожидающих.
func (c *Conn) fail(err error) {
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
func (c *Conn) Close() { c.fail(errors.New("dbus: соединение закрыто")) }

// ─── Отправка ────────────────────────────────────────────────────────────────

// send сериализует и пишет сообщение, назначая serial (если не задан).
func (c *Conn) send(m *Message) error {
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
	raw, err := m.Marshal()
	if err != nil {
		return err
	}
	_, err = c.conn.Write(raw)
	return err
}

// nextSerial резервирует serial под вызов (нужен до отправки, чтобы
// зарегистрировать ожидание ответа без гонки с читающей горутиной).
func (c *Conn) nextSerial() uint32 {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	c.serial++
	if c.serial == 0 {
		c.serial = 1
	}
	return c.serial
}

// call выполняет method_call и ждёт ответ. Ответ-ошибка возвращается как error.
func (c *Conn) Call(dest, path, iface, member, sig string, args []any) (*Message, error) {
	return c.CallTimeout(dest, path, iface, member, sig, args, dbusCallTimeout)
}

func (c *Conn) CallTimeout(dest, path, iface, member, sig string, args []any, timeout time.Duration) (*Message, error) {
	msg := &Message{
		Type: TypeMethodCall, Serial: c.nextSerial(),
		Path: path, Interface: iface, Member: member, Destination: dest,
		Sig: sig, Body: args,
	}
	ch := make(chan *Message, 1)
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

// CallNoReply отправляет вызов, не ожидая ответа (NO_REPLY_EXPECTED).
func (c *Conn) CallNoReply(dest, path, iface, member, sig string, args []any) error {
	return c.send(&Message{
		Type: TypeMethodCall, Flags: dbusFlagNoReplyExpected,
		Path: path, Interface: iface, Member: member, Destination: dest,
		Sig: sig, Body: args,
	})
}

// emit отправляет сигнал.
func (c *Conn) Emit(path, iface, member, sig string, args []any) error {
	return c.send(&Message{
		Type: TypeSignal, Path: path, Interface: iface, Member: member,
		Sig: sig, Body: args,
	})
}

// ─── Подписки и экспорт ──────────────────────────────────────────────────────

// OnSignal добавляет обработчик сигналов (вызывается в горутине чтения —
// внутри нельзя блокироваться и нельзя вызывать call на этом же соединении).
func (c *Conn) OnSignal(fn func(*Message)) {
	c.hMu.Lock()
	c.signals = append(c.signals, fn)
	c.hMu.Unlock()
}

// SetCallHandler задаёт обработчик ВХОДЯЩИХ вызовов (экспортированные объекты).
func (c *Conn) SetCallHandler(fn func(*Message) *Reply) {
	c.hMu.Lock()
	c.onCall = fn
	c.hMu.Unlock()
}

// AddMatch просит шину доставлять сигналы, подходящие под правило.
func (c *Conn) AddMatch(rule string) error {
	_, err := c.Call("org.freedesktop.DBus", "/org/freedesktop/DBus",
		"org.freedesktop.DBus", "AddMatch", "s", []any{rule})
	return err
}

// RequestName просит у шины имя (например org.a11y.atspi.Application).
// flags: 0x4 — DO_NOT_QUEUE, 0x1 — ALLOW_REPLACEMENT.
func (c *Conn) RequestName(name string, flags uint32) error {
	reply, err := c.Call("org.freedesktop.DBus", "/org/freedesktop/DBus",
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

// NameHasOwner — есть ли на шине владелец имени (жив ли сервис).
func (c *Conn) NameHasOwner(name string) bool {
	reply, err := c.Call("org.freedesktop.DBus", "/org/freedesktop/DBus",
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
	dbusSessionConn *Conn
	dbusSessionErr  error
)

// Session возвращает общее подключение к сессионной шине (ленивое,
// одно на процесс). Ошибка кэшируется: на машине без шины не долбимся в сокет
// на каждое уведомление.
func Session() (*Conn, error) {
	dbusSessionOnce.Do(func() {
		dbusSessionConn, dbusSessionErr = Dial(SessionAddress())
	})
	if dbusSessionErr != nil {
		return nil, dbusSessionErr
	}
	if dbusSessionConn != nil && dbusSessionConn.closed.Load() {
		return nil, errors.New("dbus: сессионная шина отключилась")
	}
	return dbusSessionConn, nil
}
