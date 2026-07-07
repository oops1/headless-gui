package webstream

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/oops1/headless-gui/v3/engine"
	"github.com/oops1/headless-gui/v3/output"
	"github.com/oops1/headless-gui/v3/widget"
)

// Вектор из RFC 6455 §1.3.
func TestWSAccept(t *testing.T) {
	got := wsAccept("dGhlIHNhbXBsZSBub25jZQ==")
	if got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("wsAccept = %q", got)
	}
}

// Кодирование тайлов: заголовки + валидный PNG нужного размера.
func TestEncodeTiles(t *testing.T) {
	data := make([]byte, 8*4*4)
	for i := range data {
		data[i] = byte(i)
	}
	msg, err := encodeTiles([]output.DirtyTile{{X: 64, Y: 128, W: 8, H: 4, Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	if msg[0] != msgTiles || binary.BigEndian.Uint16(msg[1:]) != 1 {
		t.Fatalf("заголовок: % x", msg[:3])
	}
	x := binary.BigEndian.Uint16(msg[3:])
	y := binary.BigEndian.Uint16(msg[5:])
	w := binary.BigEndian.Uint16(msg[7:])
	h := binary.BigEndian.Uint16(msg[9:])
	plen := binary.BigEndian.Uint32(msg[11:])
	if x != 64 || y != 128 || w != 8 || h != 4 {
		t.Fatalf("координаты: %d,%d %dx%d", x, y, w, h)
	}
	img, err := png.Decode(bytes.NewReader(msg[15 : 15+plen]))
	if err != nil {
		t.Fatalf("PNG не декодируется: %v", err)
	}
	if img.Bounds().Dx() != 8 || img.Bounds().Dy() != 4 {
		t.Fatalf("размер PNG: %v", img.Bounds())
	}
}

// ─── Сырой WebSocket-клиент для e2e (только для теста) ──────────────────────

type rawWS struct {
	c  net.Conn
	br *bufio.Reader
}

func dialWS(t *testing.T, srvURL string) *rawWS {
	t.Helper()
	u, _ := url.Parse(srvURL)
	c, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	req := "GET /ws HTTP/1.1\r\nHost: " + u.Host + "\r\n" +
		"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := c.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(c)
	line, err := br.ReadString('\n')
	if err != nil || !strings.Contains(line, "101") {
		t.Fatalf("рукопожатие: %q err=%v", line, err)
	}
	for { // остальные заголовки до пустой строки
		l, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if l == "\r\n" {
			break
		}
	}
	return &rawWS{c: c, br: br}
}

// read возвращает следующее бинарное сообщение (кадры сервера не маскированы).
func (r *rawWS) read(t *testing.T) []byte {
	t.Helper()
	r.c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var hdr [2]byte
	if _, err := io.ReadFull(r.br, hdr[:]); err != nil {
		t.Fatalf("чтение кадра: %v", err)
	}
	length := uint64(hdr[1] & 0x7F)
	switch length {
	case 126:
		var e [2]byte
		io.ReadFull(r.br, e[:])
		length = uint64(binary.BigEndian.Uint16(e[:]))
	case 127:
		var e [8]byte
		io.ReadFull(r.br, e[:])
		length = binary.BigEndian.Uint64(e[:])
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r.br, data); err != nil {
		t.Fatalf("чтение payload: %v", err)
	}
	return data
}

// send отправляет текстовое сообщение с маской (клиент обязан маскировать).
func (r *rawWS) send(t *testing.T, s string) {
	t.Helper()
	payload := []byte(s)
	mask := [4]byte{0x12, 0x34, 0x56, 0x78}
	frame := []byte{0x81}
	if len(payload) < 126 {
		frame = append(frame, byte(len(payload))|0x80)
	} else {
		frame = append(frame, 126|0x80, byte(len(payload)>>8), byte(len(payload)))
	}
	frame = append(frame, mask[:]...)
	for i, b := range payload {
		frame = append(frame, b^mask[i%4])
	}
	if _, err := r.c.Write(frame); err != nil {
		t.Fatal(err)
	}
}

// Сквозной тест: init + keyframe при подключении, клик по кнопке из
// «браузера» меняет UI, дельта-тайлы доезжают до клиента.
func TestServer_EndToEnd(t *testing.T) {
	eng := engine.New(256, 128, 30)
	root := widget.NewPanel(color.RGBA{R: 20, G: 20, B: 20, A: 255})
	root.SetBounds(image.Rect(0, 0, 256, 128))
	clicked := make(chan struct{}, 1)
	btn := widget.NewWin10AccentButton("OK")
	btn.SetBounds(image.Rect(10, 10, 110, 44))
	btn.OnClick = func() {
		btn.SetText("DONE")
		clicked <- struct{}{}
	}
	root.AddChild(btn)
	eng.SetRoot(root)
	eng.Start()
	defer eng.Stop()

	srv := New(eng)
	go srv.Run()
	hs := httptest.NewServer(srv)
	defer hs.Close()

	ws := dialWS(t, hs.URL)
	defer ws.c.Close()

	// 1. init
	init := ws.read(t)
	if init[0] != msgInit {
		t.Fatalf("первое сообщение не init: 0x%x", init[0])
	}
	if w := binary.BigEndian.Uint16(init[1:]); w != 256 {
		t.Fatalf("init width=%d", w)
	}

	// 2. keyframe — полная сетка тайлов 256×128 → 4×2 = 8 тайлов.
	kf := ws.read(t)
	if kf[0] != msgTiles {
		t.Fatalf("второе сообщение не tiles: 0x%x", kf[0])
	}
	if n := binary.BigEndian.Uint16(kf[1:]); n != 8 {
		t.Fatalf("keyframe: %d тайлов, ожидалось 8", n)
	}

	// 3. Клик по кнопке «из браузера».
	ws.send(t, `{"t":"mb","x":40,"y":25,"b":0,"p":true}`)
	ws.send(t, `{"t":"mb","x":40,"y":25,"b":0,"p":false}`)
	select {
	case <-clicked:
	case <-time.After(3 * time.Second):
		t.Fatal("клик не дошёл до кнопки")
	}

	// 4. Дельта после изменения текста кнопки.
	delta := ws.read(t)
	if delta[0] != msgTiles {
		t.Fatalf("дельта не tiles: 0x%x", delta[0])
	}
	if n := binary.BigEndian.Uint16(delta[1:]); n == 0 || n > 8 {
		t.Fatalf("дельта: %d тайлов", n)
	}

	// 5. Клавиатура: Escape не должен уронить сервер (нет модалок).
	ws.send(t, `{"t":"kd","c":27}`)
	ws.send(t, `{"t":"ku","c":27}`)
	fmt.Fprintln(io.Discard)
}
