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
	"strings"
	"sync"
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

// wsGUID — константа рукопожатия из RFC 6455 §1.3.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// errWSClosed возвращается ReadMessage после кадра Close.
var errWSClosed = errors.New("webstream: соединение закрыто")

// wsConn — серверная сторона WebSocket-соединения.
type wsConn struct {
	c   net.Conn
	br  *bufio.Reader
	wmu sync.Mutex // сериализует записи (broadcast + pong из читателя)
}

// wsAccept вычисляет Sec-WebSocket-Accept для ключа клиента.
func wsAccept(key string) string {
	h := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

// upgradeWS выполняет рукопожатие RFC 6455 поверх HTTP-запроса.
func upgradeWS(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "websocket required", http.StatusBadRequest)
		return nil, errors.New("webstream: не websocket-запрос")
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
	if _, err := conn.Write([]byte(resp)); err != nil {
		conn.Close()
		return nil, err
	}
	return &wsConn{c: conn, br: rw.Reader}, nil
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
		case opContinuation:
			msg = append(msg, payload...)
		case opText, opBinary:
			msgOp = op
			msg = append(msg, payload...)
		default:
			return 0, nil, fmt.Errorf("webstream: неизвестный опкод 0x%x", op)
		}
		if fin {
			return msgOp, msg, nil
		}
	}
}

// readFrame читает один кадр и снимает клиентскую маску.
func (ws *wsConn) readFrame() (fin bool, opcode byte, payload []byte, err error) {
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
	if length > 1<<20 { // события ввода крошечные; защита от мусора
		err = fmt.Errorf("webstream: слишком большой кадр (%d)", length)
		return
	}
	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(ws.br, mask[:]); err != nil {
			return
		}
	}
	payload = make([]byte, length)
	if _, err = io.ReadFull(ws.br, payload); err != nil {
		return
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return
}

// WriteMessage отправляет одно сообщение (сервер не маскирует кадры).
func (ws *wsConn) WriteMessage(opcode byte, data []byte) error {
	ws.wmu.Lock()
	defer ws.wmu.Unlock()

	hdr := make([]byte, 0, 10)
	hdr = append(hdr, 0x80|opcode) // FIN + опкод
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
	if _, err := ws.c.Write(hdr); err != nil {
		return err
	}
	_, err := ws.c.Write(data)
	return err
}

// Close закрывает TCP-соединение.
func (ws *wsConn) Close() error { return ws.c.Close() }
