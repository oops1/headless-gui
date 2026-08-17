// Package webstream — стриминг дельта-тайлов движка в браузер.
//
// Сервер отдаёт встроенную HTML/JS-страницу (canvas-вьювер) и WebSocket-
// эндпоинт: тайлы уходят бинарными сообщениями (PNG на тайл), события
// мыши/клавиатуры возвращаются JSON-сообщениями и транслируются в
// engine.SendMouse*/SendKeyEvent. Одно Go-приложение на сервере — UI в
// любом браузере без пересборки.
//
// WebSocket реализован по RFC 6455 без внешних зависимостей (zero-dep,
// как и весь фреймворк): рукопожатие + текстовые/бинарные кадры +
// ping/pong/close. Только серверная сторона.
package webstream

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Опкоды WebSocket-кадров (RFC 6455 §5.2).
const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

// Коды закрытия (RFC 6455 §7.4.1).
const (
	closeProtocol = 1002
	closeTooBig   = 1009
)

// Лимиты на входящие данные: события ввода крошечные.
const (
	maxFrameBytes   = 1 << 20
	maxMessageBytes = 1 << 20
)

// wsGUID — константа рукопожатия из RFC 6455 §1.3.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// Таймауты сокета по умолчанию.
const (
	defaultReadTimeout  = 60 * time.Second
	defaultWriteTimeout = 10 * time.Second
	defaultPingInterval = 20 * time.Second
)

var (
	errWSClosed   = errors.New("webstream: соединение закрыто")
	errWSProtocol = errors.New("webstream: нарушение протокола")
	errWSTooBig   = errors.New("webstream: сообщение слишком большое")
)

// wsConfig — параметры рукопожатия и сокета.
type wsConfig struct {
	allowedOrigins []string
	readTimeout    time.Duration
	writeTimeout   time.Duration
}

// wsConn — серверная сторона WebSocket-соединения.
type wsConn struct {
	c            net.Conn
	br           *bufio.Reader
	readTimeout  time.Duration
	writeTimeout time.Duration
	wmu          sync.Mutex // сериализует записи (broadcast + pong из читателя)
	hdr          [10]byte
}

// wsAccept вычисляет Sec-WebSocket-Accept для ключа клиента.
func wsAccept(key string) string {
	h := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

// originOK: пустой Origin разрешён, иначе Host или allowlist.
func originOK(origin, host string, allowed []string) bool {
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, host) {
		return true
	}
	for _, a := range allowed {
		if strings.EqualFold(a, origin) || strings.EqualFold(a, u.Host) {
			return true
		}
	}
	return false
}

// upgradeWS выполняет рукопожатие RFC 6455 поверх HTTP-запроса.
func upgradeWS(w http.ResponseWriter, r *http.Request, cfg wsConfig) (*wsConn, error) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return nil, errors.New("webstream: метод не GET")
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
		!strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		http.Error(w, "websocket required", http.StatusBadRequest)
		return nil, errors.New("webstream: не websocket-запрос")
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		w.Header().Set("Sec-WebSocket-Version", "13")
		http.Error(w, "unsupported websocket version", http.StatusBadRequest)
		return nil, errors.New("webstream: версия протокола не 13")
	}
	if !originOK(r.Header.Get("Origin"), r.Host, cfg.allowedOrigins) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return nil, errors.New("webstream: чужой Origin")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, errors.New("webstream: нет Sec-WebSocket-Key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return nil, errors.New("webstream: ResponseWriter не Hijacker")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + wsAccept(key) + "\r\n\r\n"
	_ = conn.SetWriteDeadline(time.Now().Add(cfg.writeTimeout))
	if _, err := conn.Write([]byte(resp)); err != nil {
		conn.Close()
		return nil, err
	}
	return &wsConn{
		c:            conn,
		br:           rw.Reader,
		readTimeout:  cfg.readTimeout,
		writeTimeout: cfg.writeTimeout,
	}, nil
}

// ReadMessage читает следующее сообщение данных (text/binary), прозрачно
// отвечая на ping и собирая фрагментированные сообщения.
func (ws *wsConn) ReadMessage() (opcode byte, data []byte, err error) {
	var msg []byte
	var msgOp byte
	for {
		fin, op, payload, err := ws.readFrame()
		if err != nil {
			return 0, nil, err
		}
		switch op {
		case opPing:
			_ = ws.WriteMessage(opPong, payload)
			continue
		case opPong:
			continue
		case opClose:
			_ = ws.WriteMessage(opClose, nil)
			return 0, nil, errWSClosed
		case opContinuation, opText, opBinary:
			if len(msg)+len(payload) > maxMessageBytes {
				ws.writeClose(closeTooBig, "message too big")
				return 0, nil, errWSTooBig
			}
			if op != opContinuation {
				msgOp = op
			}
			msg = append(msg, payload...)
		default:
			ws.writeClose(closeProtocol, "bad opcode")
			return 0, nil, fmt.Errorf("webstream: неизвестный опкод 0x%x", op)
		}
		if fin {
			return msgOp, msg, nil
		}
	}
}

// readFrame читает один кадр и снимает клиентскую маску.
func (ws *wsConn) readFrame() (fin bool, opcode byte, payload []byte, err error) {
	if ws.readTimeout > 0 {
		_ = ws.c.SetReadDeadline(time.Now().Add(ws.readTimeout))
	}
	var hdr [2]byte
	if _, err = io.ReadFull(ws.br, hdr[:]); err != nil {
		return
	}
	fin = hdr[0]&0x80 != 0
	opcode = hdr[0] & 0x0F
	masked := hdr[1]&0x80 != 0
	length := uint64(hdr[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(ws.br, ext[:]); err != nil {
			return
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(ws.br, ext[:]); err != nil {
			return
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if opcode >= opClose && (length > 125 || !fin) {
		ws.writeClose(closeProtocol, "bad control frame")
		err = errWSProtocol
		return
	}
	if length > maxFrameBytes {
		ws.writeClose(closeTooBig, "frame too big")
		err = errWSTooBig
		return
	}
	if !masked { // клиент обязан маскировать (RFC 6455 §5.1)
		// Payload дочитываем: иначе закрытие уйдёт через RST.
		_, _ = io.CopyN(io.Discard, ws.br, int64(length))
		ws.writeClose(closeProtocol, "unmasked frame")
		err = errWSProtocol
		return
	}
	var mask [4]byte
	if _, err = io.ReadFull(ws.br, mask[:]); err != nil {
		return
	}
	payload = make([]byte, length)
	if _, err = io.ReadFull(ws.br, payload); err != nil {
		return
	}
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	return
}

// writeClose отправляет кадр Close с кодом и причиной.
func (ws *wsConn) writeClose(code uint16, reason string) {
	var b [125]byte
	binary.BigEndian.PutUint16(b[:2], code)
	n := 2 + copy(b[2:], reason)
	_ = ws.WriteMessage(opClose, b[:n])
}

// WriteMessage отправляет одно сообщение (сервер не маскирует кадры).
func (ws *wsConn) WriteMessage(opcode byte, data []byte) error {
	ws.wmu.Lock()
	defer ws.wmu.Unlock()

	if ws.writeTimeout > 0 {
		if err := ws.c.SetWriteDeadline(time.Now().Add(ws.writeTimeout)); err != nil {
			return err
		}
	}
	hdr := append(ws.hdr[:0], 0x80|opcode) // FIN + опкод
	switch {
	case len(data) < 126:
		hdr = append(hdr, byte(len(data)))
	case len(data) <= 0xFFFF:
		hdr = append(hdr, 126, byte(len(data)>>8), byte(len(data)))
	default:
		hdr = append(hdr, 127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(len(data)))
		hdr = append(hdr, ext[:]...)
	}
	// Заголовок и payload одной записью, не двумя.
	bufs := net.Buffers{hdr, data}
	_, err := bufs.WriteTo(ws.c)
	return err
}

// Close закрывает TCP-соединение.
func (ws *wsConn) Close() error { return ws.c.Close() }
